<!--
  Editor AI menu — single discoverable button that drops a list of
  every AI action available in the editor. Until now we had:
    - one whole-note ✨ button in the toolbar
    - selection chord (Mod-Shift-A) — invisible to clicks
    - section chord (Mod-Shift-/) — invisible to clicks
    - continue chord (Mod-Alt-Space) — invisible to clicks
    - the link-suggester panel in the right rail (separate)
  The keyboard chords were ungoogleable for click-first users; the
  whole-note button got crowded as features grew. This menu replaces
  the single ✨ button and surfaces every AI affordance as a click
  target plus its chord.

  Three new actions ship inline with this menu so the user has more
  reasons to open it: Suggest title, Pin TL;DR, Tighten note. They
  reuse the audit-gated chat pipeline (chatStream + system prompt),
  no new backend.

  The menu component is dumb beyond the new actions — for the chord-
  dispatched ones it calls back via callbacks so the host page stays
  the single source of truth on how those flows splice into the doc.
-->
<script lang="ts">
  import { api } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { rafThrottle } from '$lib/util/streamThrottle';

  interface Props {
    notePath: string;
    body: string;
    /** Open the AskAIDialog with the whole note pre-filled. */
    onAskWholeNote: () => void;
    /** Dispatch a keymap chord into the editor. */
    onChord: (chord: string) => void;
    /** Open the global AI overlay. */
    onOpenOverlay: () => void;
    /** Apply a generated title (writes to frontmatter via host page). */
    onSetTitle: (title: string) => void;
    /** Insert text at the very start of the body (above frontmatter
     *  is the host's call — typically right after frontmatter so
     *  the TL;DR sits at the top of the rendered note). */
    onInsertAtTop: (text: string) => void;
    /** Replace the entire body. Used by Tighten. Host page should
     *  surface a diff confirm before persisting. */
    onReplaceBody: (next: string) => void;
  }
  let {
    notePath,
    body,
    onAskWholeNote,
    onChord,
    onOpenOverlay,
    onSetTitle,
    onInsertAtTop,
    onReplaceBody
  }: Props = $props();

  let open = $state(false);
  let menuEl: HTMLDivElement | undefined = $state();
  // Trigger button — we measure its bounding rect to position the
  // popup panel viewport-aware (right-anchored when there's room,
  // left-anchored otherwise, never off-screen on narrow phones).
  // The previous version used `class="absolute right-0 top-full"`
  // with a fixed `w-72` (288px). When the trigger sat near the right
  // edge of a narrow viewport, `right-0` anchored to the trigger's
  // own right, but the 288px-wide panel could still spill past the
  // left viewport edge — and on phones with overflow-hidden ancestors
  // somewhere in the toolbar chain the panel got clipped instead.
  let triggerEl: HTMLButtonElement | undefined = $state();
  // Position of the floating menu in viewport coordinates. We render
  // with `position: fixed` and these coordinates so the panel escapes
  // any ancestor `overflow-hidden` and lands inside the viewport
  // regardless of how wide the trigger's row is.
  let menuTop = $state(0);
  let menuLeft = $state(0);
  let menuWidth = $state(288); // matches w-72; clamped to viewport on narrow screens

  function repositionMenu() {
    if (!triggerEl) return;
    const rect = triggerEl.getBoundingClientRect();
    const vw = window.innerWidth;
    const margin = 8;
    // Clamp panel width to the viewport with a safe margin.
    const desired = 288;
    menuWidth = Math.min(desired, vw - margin * 2);
    // Try right-aligning to the trigger first (the original design).
    let left = rect.right - menuWidth;
    // If that pushes the panel off the left edge, slide it back in.
    if (left < margin) left = margin;
    // If somehow it overflows the right edge (e.g. menuWidth was
    // clamped to a smaller value), pull it back from the right.
    if (left + menuWidth > vw - margin) left = vw - margin - menuWidth;
    menuLeft = left;
    menuTop = rect.bottom + 4;
  }

  // ── click-outside + esc to close ────────────────────────────────
  $effect(() => {
    if (!open) return;
    repositionMenu();
    function onDocClick(e: MouseEvent) {
      if (!menuEl || !triggerEl) return;
      if (e.target instanceof Node && menuEl.contains(e.target)) return;
      if (e.target instanceof Node && triggerEl.contains(e.target)) return;
      open = false;
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') open = false;
    }
    function onResize() {
      repositionMenu();
    }
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onResize);
    // Scroll inside the editor toolbar / page can shift the trigger;
    // capture-phase listener so we catch any scroll, not just window.
    window.addEventListener('scroll', onResize, true);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onResize);
      window.removeEventListener('scroll', onResize, true);
    };
  });

  // ── In-flight tracking for the new inline actions. The chord-
  //    dispatched actions (Continue / Section / Selection) close
  //    the menu and let the host's existing flow handle their UX.

  type Action = 'title' | 'tldr' | 'tighten' | 'study' | 'concepts' | 'gaps' | 'translate-de' | 'translate-en' | 'cite' | 'outline' | 'memory';
  let busy = $state<Action | null>(null);
  let titleSuggestions = $state<string[]>([]);
  let titleAbort: AbortController | null = null;
  // Knowledge-gap state — AI reads the note and proposes 3-5 things
  // that, if added, would make it more complete (missing examples,
  // unstated assumptions, counter-arguments left out, definitions
  // that should be there). NOT a 'rewrite for me' surface — the
  // user reads the gaps and decides which to address themselves.
  let gaps = $state<string[]>([]);
  let gapsAbort: AbortController | null = null;
  // Study-mode state: AI generates Q&A self-test questions from the
  // note. Lives in a small bottom drawer of the menu so the user
  // can read the questions, copy them, or insert them at the end of
  // the note as a `## Self-test` section.
  type Card = { q: string; a: string };
  let studyCards = $state<Card[]>([]);
  let studyRaw = $state('');
  let studyAbort: AbortController | null = null;
  // Concept extraction: short glossary of the key terms / concepts
  // / entities the note touches, with one-line definitions. Helps
  // turn a research note into a study aid; appendable as
  // `## Concepts`.
  type Concept = { term: string; def: string };
  let concepts = $state<Concept[]>([]);
  let conceptsAbort: AbortController | null = null;

  // Memory extraction — scans the note for facts about the USER
  // (their preferences, relationships, beliefs, life context) that
  // would be useful for the chat overlay's long-term memory store.
  // Distinct from concepts (which extracts ideas FROM the note);
  // memory looks at sentences the user wrote ABOUT THEMSELVES and
  // proposes them as cross-thread context. Each proposal is a
  // separate +Add chip so the user picks what's kept.
  type MemoryProposal = {
    content: string;
    tags?: string[];
    /** committed flips true after a successful api.addAIMemory so
     *  the +chip turns into a ✓chip without re-fetching. */
    committed?: boolean;
  };
  let memoryProposals = $state<MemoryProposal[]>([]);
  let memoryAbort: AbortController | null = null;

  function noteBodyForAI(): string {
    // Cap to ~12k chars so the prompt stays bounded for long notes.
    const cleaned = body.trim();
    return cleaned.length > 12_000 ? cleaned.slice(0, 12_000) + '\n…(truncated)' : cleaned;
  }

  async function suggestTitle() {
    if (busy) return;
    if (body.trim().length < 30) {
      toast.info('Note is too short for a title suggestion.');
      return;
    }
    busy = 'title';
    titleSuggestions = [];
    titleAbort?.abort();
    titleAbort = new AbortController();
    // Throttle the per-chunk list rebuild — every token re-split the
    // growing buffer + re-rendered the suggestion buttons. Cheap per
    // chunk but compounds badly on slow phones.
    const t = rafThrottle((full) => {
      titleSuggestions = full
        .split(/\n+/)
        .map((l) => l.trim().replace(/^[-•*\d.\s"']+|["']\s*$/g, ''))
        .filter((l) => l.length > 0 && l.length < 120);
    });
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You suggest 3-5 short, specific titles for the user\'s note. Return JUST the titles, one per line. No numbering, no bullets, no preamble. 4-7 words each. Capture the topic, not the genre. Lowercase normal case (not title case). No quotes.'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => { t.flush(); },
          onError: (err) => {
            t.flush();
            toast.error(err.message);
            titleSuggestions = [];
          }
        },
        titleAbort.signal
      );
    } finally {
      busy = null;
      titleAbort = null;
    }
  }
  function applyTitle(t: string) {
    onSetTitle(t);
    titleSuggestions = [];
    open = false;
    toast.success(`Title set: ${t}`);
  }

  async function pinTLDR() {
    if (busy) return;
    if (body.trim().length < 60) {
      toast.info('Note is too short for a TL;DR.');
      return;
    }
    busy = 'tldr';
    let buf = '';
    const abort = new AbortController();
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'Summarise the user\'s note in 1-3 short sentences. Plain prose, no preamble, no markdown formatting, no bullets. Under 60 words total. Reflect what the note is about; do not add commentary or advice. Return ONLY the summary text.'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; },
          onDone: () => {
            const summary = buf.trim().replace(/^["'`]+|["'`]+$/g, '');
            if (!summary) return;
            // Pin as a markdown blockquote callout at the top of the
            // body. The host's onInsertAtTop handles where exactly
            // (after frontmatter, before any heading).
            onInsertAtTop(`> **TL;DR** — ${summary}\n\n`);
            toast.success('TL;DR pinned at the top.');
            open = false;
          },
          onError: (err) => toast.error(err.message)
        },
        abort.signal
      );
    } finally {
      busy = null;
    }
  }

  async function generateStudyQuestions() {
    if (busy) return;
    if (body.trim().length < 100) {
      toast.info('Note is too short for study questions.');
      return;
    }
    studyAbort?.abort();
    studyAbort = new AbortController();
    busy = 'study';
    studyCards = [];
    studyRaw = '';
    const studyT = rafThrottle((full) => {
      studyRaw = full;
    });
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You generate 5-7 study questions from the user\'s note that test understanding (not memorisation of trivia). Each question probes a concept, an implication, an example, or an application. Return STRICTLY a JSON array, no fences, no prose: [{"q": "<the question>", "a": "<one-sentence answer based on the note, max 30 words>"}]. Avoid questions that are answerable from the title alone or from rote phrasing — favour ones the reader could only answer if they understood the material.'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: studyT.onChunk,
          onDone: () => {
            studyT.flush();
            const buf = studyT.value();
            let cleaned = buf.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const arr = JSON.parse(cleaned) as Card[];
              if (Array.isArray(arr)) studyCards = arr.filter((x) => x.q && x.a);
            } catch {
              toast.error('Model didn\'t return parseable JSON.');
            }
          },
          onError: (err) => toast.error(err.message)
        },
        studyAbort.signal
      );
    } finally {
      busy = null;
      studyAbort = null;
    }
  }
  function dismissStudy() {
    studyAbort?.abort();
    studyCards = [];
    studyRaw = '';
  }
  function insertStudyAtBottom() {
    if (studyCards.length === 0) return;
    const md =
      '\n\n## Self-test\n\n' +
      studyCards
        .map((c, i) => `**Q${i + 1}.** ${c.q}\n\n_A:_ ${c.a}\n`)
        .join('\n');
    onInsertAtTop(''); // reuse insertAtTop with empty? no — we need insertAtBottom
    // Actually we don't have an onInsertAtBottom; the callbacks are
    // fixed. Falling back to onReplaceBody with the appended block.
    // The onReplaceBody contract is "replace the whole body"; we do
    // exactly that — body + append. The host page treats this as
    // one undoable transaction.
    onReplaceBody(body.replace(/\s+$/, '') + md);
    studyCards = [];
    open = false;
    toast.success('Self-test added at the end of the note.');
  }

  async function findKnowledgeGaps() {
    if (busy) return;
    if (body.trim().length < 100) {
      toast.info('Note is too short to find gaps.');
      return;
    }
    gapsAbort?.abort();
    gapsAbort = new AbortController();
    busy = 'gaps';
    gaps = [];
    // Same shape as suggestTitle — rebuild the list once per frame.
    const gapsT = rafThrottle((full) => {
      gaps = full.split(/\n+/).map((l) => l.trim()).filter((l) => l.length > 0);
    });
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You read the user\'s note and surface 3-5 SPECIFIC things that would make it more complete or rigorous: missing examples, unstated assumptions, counter-arguments left out, definitions that should be there, evidence the claims need, scope that should be narrowed. Each on its own line. No preamble, no numbering, no bullets. Under 22 words each. Be specific to THIS note — anchor every gap in something the note actually says. Examples of bad gaps: "could use more detail" (too vague), "consider the audience" (generic). Examples of good gaps: "no example of X applied to Y; the abstraction stays untested", "claim that A causes B never says how — mechanism missing".'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: gapsT.onChunk,
          onDone: () => { gapsT.flush(); },
          onError: (err) => { gapsT.flush(); toast.error(err.message); }
        },
        gapsAbort.signal
      );
    } finally {
      busy = null;
      gapsAbort = null;
    }
  }
  function dismissGaps() {
    gapsAbort?.abort();
    gaps = [];
  }

  async function extractConcepts() {
    if (busy) return;
    if (body.trim().length < 100) {
      toast.info('Note is too short to extract concepts.');
      return;
    }
    conceptsAbort?.abort();
    conceptsAbort = new AbortController();
    busy = 'concepts';
    concepts = [];
    let buf = '';
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You extract 5-10 key concepts / terms / entities from the user\'s note and write a one-line definition for each, in the user\'s voice (using the note\'s framing, not Wikipedia\'s). Return STRICTLY a JSON array, no fences, no prose: [{"term": "<2-4 words>", "def": "<single sentence, under 25 words>"}]. Pick concepts that are LOAD-BEARING in the note — not every noun, only the terms the rest of the argument depends on. Lowercase term names unless they\'re proper nouns.'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; },
          onDone: () => {
            let cleaned = buf.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const arr = JSON.parse(cleaned) as Concept[];
              if (Array.isArray(arr)) concepts = arr.filter((x) => x.term && x.def);
            } catch {
              toast.error('Model didn\'t return parseable JSON.');
            }
          },
          onError: (err) => toast.error(err.message)
        },
        conceptsAbort.signal
      );
    } finally {
      busy = null;
      conceptsAbort = null;
    }
  }
  function dismissConcepts() {
    conceptsAbort?.abort();
    concepts = [];
  }
  function insertConceptsAtBottom() {
    if (concepts.length === 0) return;
    const md =
      '\n\n## Concepts\n\n' +
      concepts.map((c) => `**${c.term}** — ${c.def}`).join('\n\n');
    onReplaceBody(body.replace(/\s+$/, '') + md);
    concepts = [];
    open = false;
    toast.success('Glossary added at the end of the note.');
  }

  // ─── Extract long-term memory from note ─────────────────────────
  // Scans the note for sentences the user wrote ABOUT THEMSELVES —
  // preferences, relationships, life context, beliefs that would help
  // the assistant on future, unrelated conversations. Each proposal
  // is rendered as a +Add chip; the user picks which to commit to
  // the long-term memory store. We deliberately reject "fact-like"
  // sentences that aren't about the user (entity facts belong in
  // the note body, not memory).
  async function extractMemoryFacts() {
    if (busy) return;
    if (body.trim().length < 80) {
      toast.info('Note is too short to extract memory.');
      return;
    }
    memoryAbort?.abort();
    memoryAbort = new AbortController();
    busy = 'memory';
    memoryProposals = [];
    let buf = '';
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              `You read the user's note and extract facts ABOUT THE USER — preferences, relationships, life context, beliefs, recurring concerns — that would help an AI assistant on FUTURE, UNRELATED conversations.

Strict shape: return ONLY a JSON array, no prose, no fences:
[{"content": "<single sentence stating the fact>", "tags": ["<1-3 lowercase tags>"]}, ...]

Rules:
- 0-6 entries. Quality over quantity. Skip if nothing about the user is in here.
- "content" is one sentence, under 200 chars, written in third-person about the user ("User is vegetarian", NOT "I am vegetarian").
- Skip entity facts ("The Eiffel Tower is in Paris") — those belong in the note, not memory.
- Skip transient state ("User is tired today") — only durable facts.
- Skip anything already obvious from the note's title or frontmatter.
- Tags: 1-3 single-word lowercase labels for grouping (family, work, health, faith, diet, learning, ...). Omit when nothing fits.

Return [] if nothing in the note rises to the bar.`
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: (c) => {
            buf += c;
          },
          onDone: () => {
            let cleaned = buf.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned
                .replace(/^```json\s*/i, '')
                .replace(/^```\s*/, '')
                .replace(/```\s*$/, '')
                .trim();
            }
            try {
              const arr = JSON.parse(cleaned) as MemoryProposal[];
              if (Array.isArray(arr)) {
                memoryProposals = arr
                  .filter(
                    (x) =>
                      typeof x?.content === 'string' &&
                      x.content.trim().length > 0 &&
                      x.content.length < 240
                  )
                  .slice(0, 8)
                  .map((x) => ({
                    content: x.content.trim(),
                    tags: Array.isArray(x.tags) ? x.tags.filter((t) => typeof t === 'string') : []
                  }));
                if (memoryProposals.length === 0) {
                  toast.info('No memory-worthy facts in this note.');
                }
              }
            } catch {
              toast.error('Model didn\'t return parseable JSON.');
            }
          },
          onError: (err) => toast.error(err.message)
        },
        memoryAbort.signal
      );
    } finally {
      busy = null;
      memoryAbort = null;
    }
  }

  async function commitMemoryProposal(idx: number) {
    const p = memoryProposals[idx];
    if (!p || p.committed) return;
    try {
      await api.addAIMemory(p.content, p.tags && p.tags.length > 0 ? p.tags : undefined);
      memoryProposals = memoryProposals.map((x, i) =>
        i === idx ? { ...x, committed: true } : x
      );
      toast.success('Saved to memory');
    } catch (err) {
      toast.error('Memory add failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }
  function commitAllMemoryProposals() {
    for (let i = 0; i < memoryProposals.length; i++) {
      if (!memoryProposals[i].committed) void commitMemoryProposal(i);
    }
  }
  function dismissMemoryProposals() {
    memoryAbort?.abort();
    memoryProposals = [];
  }

  async function tightenNote() {
    if (busy) return;
    if (body.trim().length < 80) {
      toast.info('Note is too short to tighten.');
      return;
    }
    busy = 'tighten';
    let buf = '';
    const abort = new AbortController();
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'Tighten the user\'s note: clearer, fewer words, same voice, same meaning. Preserve all markdown structure (headings, lists, links, code blocks). Fix typos and grammar. Do NOT add new content, opinions, or sections. Return ONLY the rewritten note body — no preamble, no fences.'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; },
          onDone: () => {
            const out = buf.trim();
            if (!out) return;
            onReplaceBody(out);
            toast.success('Tightened (Cmd+Z to revert).');
            open = false;
          },
          onError: (err) => toast.error(err.message)
        },
        abort.signal
      );
    } finally {
      busy = null;
    }
  }

  // ─── Translate whole note (DE ↔ EN) ─────────────────────────────
  // The user is multilingual (German + English). A button on the AI
  // menu translates the WHOLE note in place — preserving all markdown
  // structure, frontmatter cues, wikilinks, and code blocks. The
  // translated version replaces the body as a single undoable
  // transaction, so the original is one Cmd+Z away.
  //
  // Why not the AskAIDialog's existing EN/DE chips: those work on a
  // selected range (a quote, a paragraph). Translating an entire note
  // would mean Cmd+A → Mod-Shift-A → wait → Replace, four steps. This
  // is one click. Different muscle, both shipped.
  let translatePreview = $state<{ to: 'en' | 'de'; text: string } | null>(null);
  let translateAbort: AbortController | null = null;
  async function translateWholeNote(target: 'en' | 'de') {
    if (busy) return;
    if (body.trim().length < 30) {
      toast.info('Note is too short to translate.');
      return;
    }
    busy = target === 'de' ? 'translate-de' : 'translate-en';
    translateAbort?.abort();
    translateAbort = new AbortController();
    translatePreview = { to: target, text: '' };
    const targetName = target === 'de' ? 'German' : 'English';
    // Same throttle — for a multi-paragraph note the model emits
    // hundreds of tokens, and re-rendering the preview block per
    // token was directly observable as the freeze on translate.
    const translateT = rafThrottle((full) => {
      translatePreview = { to: target, text: full };
    });
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              `You translate the user's markdown note to ${targetName}. PRESERVE ALL of: heading structure, bullet/numbered lists, blockquotes, code blocks (do NOT translate code), inline code spans, [[wikilinks]], [markdown](links), images, footnote markers, tables. Translate prose only. If the note is already in ${targetName}, translate the OTHER way (so DE→EN if it's already German, EN→DE if it's already English) — but bias toward ${targetName} when the source is mixed. Match the user's register (casual stays casual, academic stays academic). Return ONLY the translated note body. No preamble, no fences.`
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: translateT.onChunk,
          onDone: () => { translateT.flush(); },
          onError: (err) => {
            translateT.flush();
            toast.error(err.message);
            translatePreview = null;
          }
        },
        translateAbort.signal
      );
    } finally {
      busy = null;
      translateAbort = null;
    }
  }
  function applyTranslation() {
    if (!translatePreview || !translatePreview.text.trim()) return;
    onReplaceBody(translatePreview.text.trim());
    translatePreview = null;
    open = false;
    toast.success('Translated (Cmd+Z to revert).');
  }
  function dismissTranslation() {
    translateAbort?.abort();
    translatePreview = null;
  }

  // ─── Cite-from-vault ──────────────────────────────────────────────
  // For each top-level claim or paragraph in the note, find supporting
  // notes in the user's vault via the existing AI link suggester
  // (api.aiSuggestLinks already does the RAG-y candidate retrieval +
  // model-side filtering). The user picks which suggestions to commit;
  // accepting a suggestion appends a footnote definition at the end of
  // the note and a `[^id]` reference next to the matching paragraph.
  //
  // Why a separate surface from the rail's LinkSuggestPanel: that one
  // produces flat tag/link chips meant for inline insertion at cursor.
  // This one is structured around CITATIONS — it pairs each suggestion
  // with the specific claim it supports, so the user can audit before
  // committing. The output format (footnote refs) is also distinct
  // from the rail's bare `[[wikilink]]` insertion.
  type Citation = {
    /** The paragraph / claim being cited. */
    claim: string;
    /** Anchor line number in the source body (1-indexed). */
    line: number;
    /** Vault note path (without trailing .md). */
    ref: string;
    /** Display title for the cite. */
    title: string;
    /** Why the model thinks this note supports the claim. */
    rationale: string;
    /** Whether this suggestion has been accepted (committed). */
    accepted?: boolean;
  };
  let citations = $state<Citation[]>([]);
  let citeAbort: AbortController | null = null;
  let citeError = $state<string | null>(null);
  async function findCitations() {
    if (busy) return;
    if (body.trim().length < 100) {
      toast.info('Note is too short to find citations for.');
      return;
    }
    citeAbort?.abort();
    citeAbort = new AbortController();
    busy = 'cite';
    citations = [];
    citeError = null;
    let buf = '';
    try {
      // Step 1: ask the AI to identify cite-worthy claims + which
      // notes from the vault would support them. We pass the note's
      // body PLUS a manifest of the user's existing vault notes (titles
      // + paths, capped) so the model can name real candidates rather
      // than hallucinating titles.
      const manifest = await api.listNotes({ limit: 200 });
      const notesList = manifest.notes
        .filter((n) => n.path !== notePath)
        .slice(0, 200)
        .map((n) => `- ${n.title} (${n.path})`)
        .join('\n');
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You find citations from the user\'s vault for claims in their note. The user gives you (a) the note they\'re writing and (b) a list of notes available in their vault. Return STRICTLY a JSON array, no fences, no prose: [{"claim": "<paraphrase the claim, max 18 words>", "line": <1-indexed line of the body where the claim starts>, "ref": "<vault note path EXACTLY as listed, including .md>", "title": "<title>", "rationale": "<why this note supports it, max 18 words>"}]. Pick 3-6 STRONG matches (skip if nothing in the vault supports the claim). Cite only notes that genuinely support / contradict / illuminate the claim — do not cite a note just because the topic overlaps loosely. Refs MUST be one of the paths the user gave you; no inventing notes.'
          },
          {
            role: 'user',
            content:
              `**My note:**\n\n${noteBodyForAI()}\n\n---\n\n**Available vault notes (path in parens — pick from these):**\n\n${notesList || '(none)'}`
          }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; },
          onDone: () => {
            let cleaned = buf.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const arr = JSON.parse(cleaned) as Citation[];
              if (Array.isArray(arr)) {
                // Validate refs against the manifest so we don't accept
                // hallucinated paths. Drop any cite whose ref doesn't
                // resolve.
                const known = new Set(manifest.notes.map((n) => n.path));
                citations = arr
                  .filter((c) => c.claim && c.ref && known.has(c.ref))
                  .map((c) => ({ ...c, accepted: false }));
                if (citations.length === 0) {
                  citeError = 'No vault notes match the claims in this note.';
                }
              }
            } catch {
              citeError = 'Model didn\'t return parseable JSON.';
            }
          },
          onError: (err) => { citeError = err.message; }
        },
        citeAbort.signal
      );
    } finally {
      busy = null;
      citeAbort = null;
    }
  }
  function dismissCitations() {
    citeAbort?.abort();
    citations = [];
    citeError = null;
  }
  /** Append accepted cites to the body. We add a `[^N]` ref after the
   *  cited paragraph and a definition block at the end of the body.
   *  IDs are auto-numbered starting from the next free index after any
   *  existing footnotes in the doc. */
  function commitCitations() {
    const accepted = citations.filter((c) => c.accepted);
    if (accepted.length === 0) {
      toast.info('No citations selected.');
      return;
    }
    // Find the highest existing [^N] number so we don't collide.
    let next = 1;
    const existing = body.matchAll(/\[\^(\d+)\]/g);
    for (const m of existing) {
      const n = parseInt(m[1], 10);
      if (Number.isFinite(n) && n >= next) next = n + 1;
    }
    const lines = body.split('\n');
    // Two-phase: first assign IDs in *line order* so the body reads
    // [^1], [^2], [^3] top-to-bottom — then apply the marker writes
    // in *reverse line order* so inserting at line N doesn't shift
    // earlier indices. Doing both in one reverse pass (the original
    // version) flipped the IDs and gave a doc that read [^3]...[^1].
    const cites = accepted
      .map((c) => ({ ...c, id: 0 }))
      .sort((a, b) => a.line - b.line);
    for (const c of cites) c.id = next++;
    const writeOrder = [...cites].sort((a, b) => b.line - a.line);
    for (const c of writeOrder) {
      const idx = Math.max(0, Math.min(lines.length - 1, c.line - 1));
      lines[idx] = lines[idx].replace(/\s+$/, '') + `[^${c.id}]`;
    }
    // Definitions are already in ascending-id order (= line order).
    const defs = cites
      .map((c) => {
        const ref = c.ref.replace(/\.md$/, '');
        return `[^${c.id}]: [[${ref}|${c.title}]] — ${c.rationale}`;
      })
      .join('\n');
    let next_body = lines.join('\n').replace(/\s+$/, '');
    next_body += '\n\n' + defs + '\n';
    onReplaceBody(next_body);
    citations = [];
    open = false;
    toast.success(`Added ${cites.length} citation${cites.length === 1 ? '' : 's'}.`);
  }

  // ─── Outline-from-doc ─────────────────────────────────────────────
  // For prose-heavy notes that lack heading structure, the AI proposes
  // a TOC (## headings + optional ### sub-headings) the user can
  // accept wholesale or pick from. Different from TL;DR (which lives
  // at the top as a paragraph) — this is structure injection: the
  // outline gets prepended as a `## Outline` block, OR the user can
  // accept individual sections to insert later. Section accept-mode
  // here is ALL-OR-NOTHING (the prepend) because partial-acceptance
  // of headings without the prose under them creates orphan headings.
  type OutlineSection = { heading: string; level: 2 | 3; gist: string };
  let outlineSections = $state<OutlineSection[]>([]);
  let outlineAbort: AbortController | null = null;
  let outlineError = $state<string | null>(null);
  async function generateOutline() {
    if (busy) return;
    if (body.trim().length < 200) {
      toast.info('Note is too short for an outline.');
      return;
    }
    outlineAbort?.abort();
    outlineAbort = new AbortController();
    busy = 'outline';
    outlineSections = [];
    outlineError = null;
    let buf = '';
    try {
      await api.chatStream(
        [
          {
            role: 'system',
            content:
              'You generate a markdown outline (table of contents) for the user\'s note. Return STRICTLY a JSON array, no fences, no prose: [{"heading": "<title for this section, max 6 words>", "level": 2 | 3, "gist": "<one short sentence describing what this section covers, max 18 words>"}]. Pick 4-8 sections covering the note\'s main ideas in their natural order. Use level 3 for sub-sections under a level 2 heading. The outline should READ as a logical structure — when the user reads heading + gist they should grok the note\'s shape. Lowercase headings unless they\'re proper nouns. Don\'t include "Introduction" / "Conclusion" filler unless the note actually has those.'
          },
          { role: 'user', content: noteBodyForAI() }
        ],
        undefined,
        {
          onChunk: (c) => { buf += c; },
          onDone: () => {
            let cleaned = buf.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const arr = JSON.parse(cleaned) as OutlineSection[];
              if (Array.isArray(arr)) {
                outlineSections = arr.filter(
                  (s) => s.heading && (s.level === 2 || s.level === 3)
                );
              }
            } catch {
              outlineError = 'Model didn\'t return parseable JSON.';
            }
          },
          onError: (err) => { outlineError = err.message; }
        },
        outlineAbort.signal
      );
    } finally {
      busy = null;
      outlineAbort = null;
    }
  }
  function dismissOutline() {
    outlineAbort?.abort();
    outlineSections = [];
    outlineError = null;
  }
  function insertOutlineAtTop() {
    if (outlineSections.length === 0) return;
    const md =
      '## Outline\n\n' +
      outlineSections
        .map((s) => {
          const hashes = '#'.repeat(s.level);
          // Render as markdown headings so the doc gets a real
          // navigable structure; gist sits as italic prose under
          // each heading so the user can flesh it out into the real
          // section body. We DON'T blank the existing body — outline
          // rides on top.
          return `${hashes} ${s.heading}\n\n*${s.gist}*\n`;
        })
        .join('\n');
    onInsertAtTop(md + '\n');
    outlineSections = [];
    open = false;
    toast.success('Outline added at the top.');
  }

  function fireChord(chord: string) {
    onChord(chord);
    open = false;
  }
  function fireWholeNote() {
    onAskWholeNote();
    open = false;
  }
  function fireOverlay() {
    onOpenOverlay();
    open = false;
  }

  // Keyboard chord display helper — Mac uses ⌘ + ⌥, others Ctrl + Alt.
  let modKey = $state('Ctrl');
  let altKey = $state('Alt');
  $effect(() => {
    if (typeof navigator === 'undefined') return;
    if (/Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent)) {
      modKey = '⌘';
      altKey = '⌥';
    }
  });
</script>

<div class="relative">
  <button
    type="button"
    bind:this={triggerEl}
    onclick={() => (open = !open)}
    aria-haspopup="menu"
    aria-expanded={open}
    title="AI actions for this note"
    aria-label="AI actions"
    class="flex items-center gap-1 px-2 h-9 rounded text-subtext hover:text-primary hover:bg-surface0 flex-shrink-0
      {open ? 'text-primary bg-surface0' : ''}"
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8">
      <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z" stroke-linejoin="round"/>
      <path d="M19 14l.7 2.1L22 17l-2.3.9L19 20l-.7-2.1L16 17l2.3-.9L19 14z" stroke-linejoin="round"/>
    </svg>
    <svg viewBox="0 0 24 24" class="w-3 h-3 opacity-60" fill="none" stroke="currentColor" stroke-width="2">
      <polyline points="6 9 12 15 18 9" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>
  </button>

  {#if open}
    <!-- Fixed-position floating panel — coords come from
         repositionMenu() so the panel always lands inside the
         viewport regardless of how wide the toolbar row is or how
         close to the right edge the trigger sits. -->
    <div
      bind:this={menuEl}
      role="menu"
      style="top: {menuTop}px; left: {menuLeft}px; width: {menuWidth}px; max-height: calc(100dvh - {menuTop}px - 0.75rem);"
      class="fixed bg-mantle border border-surface1 rounded-lg shadow-xl z-50 py-1 text-sm overflow-y-auto overscroll-contain"
    >
      <button role="menuitem" onclick={fireWholeNote} class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text">
        <span class="flex-1 text-left">Ask about this note</span>
        <span class="text-[10px] text-dim">whole-note dialog</span>
      </button>
      <button role="menuitem" onclick={() => fireChord('mod+alt+ ')} class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text">
        <span class="flex-1 text-left">Continue writing at cursor</span>
        <kbd class="text-[10px] text-dim font-mono">{modKey}{altKey}␣</kbd>
      </button>
      <button role="menuitem" onclick={() => fireChord('mod+shift+/')} class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text">
        <span class="flex-1 text-left">Ask about current section</span>
        <kbd class="text-[10px] text-dim font-mono">{modKey}⇧/</kbd>
      </button>
      <button role="menuitem" onclick={() => fireChord('mod+shift+a')} class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text">
        <span class="flex-1 text-left">Ask about selection</span>
        <kbd class="text-[10px] text-dim font-mono">{modKey}⇧A</kbd>
      </button>

      <div class="border-t border-surface1 my-1"></div>

      <button
        role="menuitem"
        onclick={suggestTitle}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'title' ? 'Suggesting…' : 'Suggest title'}</span>
        <span class="text-[10px] text-dim">3-5 picks</span>
      </button>
      <button
        role="menuitem"
        onclick={pinTLDR}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'tldr' ? 'Summarising…' : 'Pin TL;DR at top'}</span>
        <span class="text-[10px] text-dim">callout</span>
      </button>
      <button
        role="menuitem"
        onclick={generateOutline}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'outline' ? 'Outlining…' : 'Generate outline'}</span>
        <span class="text-[10px] text-dim">## headings</span>
      </button>
      <button
        role="menuitem"
        onclick={tightenNote}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'tighten' ? 'Tightening…' : 'Tighten whole note'}</span>
        <span class="text-[10px] text-dim">undo: {modKey}Z</span>
      </button>
      <!-- Study mode: AI generates Q&A self-test from the note. The
           result panel lives at the bottom of the menu so the user
           can read the questions, regenerate, dismiss, or append
           them as a `## Self-test` section. The point isn't to
           rewrite the note — it's to surface comprehension probes
           the user can quiz themselves on later. -->
      <button
        role="menuitem"
        onclick={generateStudyQuestions}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'study' ? 'Generating…' : 'Study questions'}</span>
        <span class="text-[10px] text-dim">5-7 Q&A</span>
      </button>
      <button
        role="menuitem"
        onclick={extractConcepts}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'concepts' ? 'Extracting…' : 'Extract concepts'}</span>
        <span class="text-[10px] text-dim">glossary</span>
      </button>
      <button
        role="menuitem"
        onclick={findKnowledgeGaps}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'gaps' ? 'Reading…' : 'Find gaps'}</span>
        <span class="text-[10px] text-dim">what's missing</span>
      </button>
      <!-- Memory extraction. Distinct from Concepts (ideas IN the
           note) and Self-test (comprehension probes) — this one
           scans for facts ABOUT THE USER that the chat overlay
           should remember across future conversations. The +chips
           below let the user pick which facts to commit. -->
      <button
        role="menuitem"
        onclick={extractMemoryFacts}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'memory' ? 'Reading…' : 'Extract to memory'}</span>
        <span class="text-[10px] text-dim">facts about you</span>
      </button>

      <div class="border-t border-surface1 my-1"></div>

      <!-- Translate whole note. Two language buttons live on a single
           row to keep the menu tidy. The translation streams into a
           preview drawer below — user reads, then accepts (replaces
           body) or dismisses. -->
      <div class="px-3 py-2 flex items-baseline gap-2">
        <span class="flex-1 text-text">Translate</span>
        <button
          type="button"
          onclick={() => translateWholeNote('de')}
          disabled={busy !== null}
          class="px-2 py-0.5 rounded text-[11px] font-mono bg-surface0 hover:bg-surface1 text-subtext hover:text-text disabled:opacity-50"
        >{busy === 'translate-de' ? '…' : 'DE'}</button>
        <button
          type="button"
          onclick={() => translateWholeNote('en')}
          disabled={busy !== null}
          class="px-2 py-0.5 rounded text-[11px] font-mono bg-surface0 hover:bg-surface1 text-subtext hover:text-text disabled:opacity-50"
        >{busy === 'translate-en' ? '…' : 'EN'}</button>
      </div>
      <!-- Cite-from-vault: AI scans the note, picks claims that
           need support, finds matching vault notes, and lets the
           user commit footnote-style citations one at a time.
           Different muscle from the rail's link suggester (that
           one drops bare wikilinks at cursor). -->
      <button
        role="menuitem"
        onclick={findCitations}
        disabled={busy !== null}
        class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text disabled:opacity-50"
      >
        <span class="flex-1 text-left">{busy === 'cite' ? 'Searching vault…' : 'Cite from vault'}</span>
        <span class="text-[10px] text-dim">footnote refs</span>
      </button>

      <div class="border-t border-surface1 my-1"></div>

      <button role="menuitem" onclick={fireOverlay} class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text">
        <span class="flex-1 text-left">Open chat overlay</span>
        <kbd class="text-[10px] text-dim font-mono">{modKey}J</kbd>
      </button>

      {#if titleSuggestions.length > 0}
        <div class="border-t border-surface1 mt-1 py-1 px-3 bg-surface1">
          <div class="text-[10px] uppercase tracking-wider text-primary mb-1">title suggestions</div>
          {#each titleSuggestions as t}
            <button
              type="button"
              onclick={() => applyTitle(t)}
              class="block w-full text-left text-sm py-1 hover:text-primary"
            >{t}</button>
          {/each}
        </div>
      {/if}

      {#if gaps.length > 0}
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface0 max-h-72 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-warning">what's missing</span>
            <span class="flex-1"></span>
            <button type="button" onclick={findKnowledgeGaps} disabled={busy !== null} class="text-[11px] text-secondary hover:underline">regenerate</button>
            <button type="button" onclick={dismissGaps} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          <ul class="space-y-1.5">
            {#each gaps as g}
              {@const cleaned = g.replace(/^[-•*\d.\s]+/, '').trim()}
              {#if cleaned}
                <li class="text-xs text-text leading-snug flex gap-2">
                  <span class="text-warning flex-shrink-0">·</span>
                  <span>{cleaned}</span>
                </li>
              {/if}
            {/each}
          </ul>
        </div>
      {/if}

      {#if memoryProposals.length > 0}
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface1 max-h-80 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-primary">long-term memory</span>
            <span class="flex-1"></span>
            <button type="button" onclick={commitAllMemoryProposals} class="text-[11px] text-secondary hover:underline">save all</button>
            <button type="button" onclick={extractMemoryFacts} disabled={busy !== null} class="text-[11px] text-secondary hover:underline">regen</button>
            <button type="button" onclick={dismissMemoryProposals} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          <ul class="space-y-1.5">
            {#each memoryProposals as p, i (i)}
              <li class="flex items-start gap-2 text-xs">
                <button
                  type="button"
                  onclick={() => commitMemoryProposal(i)}
                  disabled={p.committed}
                  class="tap-target flex-shrink-0 inline-flex items-center justify-center w-6 h-6 rounded text-[11px] font-medium transition-colors {p.committed
                    ? 'bg-surface0 text-success cursor-default'
                    : 'bg-surface0 border border-surface1 text-text hover:border-primary hover:bg-surface1'}"
                  aria-label={p.committed ? 'Already saved' : 'Save this fact to memory'}
                  title={p.committed ? 'Already saved' : 'Save to long-term memory'}
                >{p.committed ? '✓' : '+'}</button>
                <div class="flex-1 min-w-0 leading-snug">
                  <div class="text-text">{p.content}</div>
                  {#if p.tags && p.tags.length > 0}
                    <div class="text-[10px] text-dim mt-0.5">{p.tags.join(' · ')}</div>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
          <p class="text-[10px] text-dim mt-2 leading-snug">
            Saved facts inject into every future chat thread so the assistant remembers them across conversations.
          </p>
        </div>
      {/if}

      {#if concepts.length > 0}
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface1 max-h-72 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-primary">concepts</span>
            <span class="flex-1"></span>
            <button type="button" onclick={extractConcepts} disabled={busy !== null} class="text-[11px] text-secondary hover:underline">regenerate</button>
            <button type="button" onclick={dismissConcepts} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          <ul class="space-y-1.5">
            {#each concepts as c (c.term)}
              <li class="text-xs">
                <span class="font-semibold text-text">{c.term}</span>
                <span class="text-subtext"> — {c.def}</span>
              </li>
            {/each}
          </ul>
          <button
            type="button"
            onclick={insertConceptsAtBottom}
            class="mt-2 w-full text-xs px-2 py-1 rounded bg-primary text-on-primary font-medium"
          >
            Append as ## Concepts
          </button>
        </div>
      {/if}

      {#if studyCards.length > 0}
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface1 max-h-72 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-primary">study questions</span>
            <span class="flex-1"></span>
            <button type="button" onclick={generateStudyQuestions} disabled={busy !== null} class="text-[11px] text-secondary hover:underline">regenerate</button>
            <button type="button" onclick={dismissStudy} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          <ol class="space-y-2 list-decimal pl-4">
            {#each studyCards as c, i (i)}
              <li class="text-xs">
                <div class="text-text">{c.q}</div>
                <details class="mt-0.5">
                  <summary class="cursor-pointer text-dim hover:text-text text-[11px]">show answer</summary>
                  <div class="mt-1 text-subtext italic">{c.a}</div>
                </details>
              </li>
            {/each}
          </ol>
          <button
            type="button"
            onclick={insertStudyAtBottom}
            class="mt-2 w-full text-xs px-2 py-1 rounded bg-primary text-on-primary font-medium"
          >
            Append as ## Self-test
          </button>
        </div>
      {/if}

      {#if translatePreview}
        <!-- Translation streaming preview. The user watches the
             translated note arrive, then accepts (replaces body, one
             undoable transaction) or dismisses (no edit). The
             preview is read-only — re-running translation is one
             tap on DE / EN at the top of this section. -->
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface1 max-h-72 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-primary">
              translation → {translatePreview.to === 'de' ? 'German' : 'English'}
            </span>
            <span class="flex-1"></span>
            {#if busy === 'translate-de' || busy === 'translate-en'}
              <button type="button" onclick={() => translateAbort?.abort()} class="text-[11px] text-warning hover:underline">cancel</button>
            {/if}
            <button type="button" onclick={dismissTranslation} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          <pre class="text-[11px] text-subtext leading-snug whitespace-pre-wrap break-words font-sans">{translatePreview.text || (busy ? 'translating…' : '')}</pre>
          {#if translatePreview.text && busy !== 'translate-de' && busy !== 'translate-en'}
            <button
              type="button"
              onclick={applyTranslation}
              class="mt-2 w-full text-xs px-2 py-1 rounded bg-primary text-on-primary font-medium"
            >
              Replace note body
            </button>
          {/if}
        </div>
      {/if}

      {#if outlineSections.length > 0 || outlineError}
        <!-- Outline-from-doc preview. The user reads the proposed
             structure (heading + one-sentence gist) before committing.
             Accept inserts the whole outline as a `## Outline` block
             at the top of the body, with each heading + italic gist
             as a real markdown section the user can then flesh out. -->
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface0 max-h-72 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-info">outline</span>
            <span class="flex-1"></span>
            <button type="button" onclick={generateOutline} disabled={busy !== null} class="text-[11px] text-secondary hover:underline">regenerate</button>
            <button type="button" onclick={dismissOutline} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          {#if outlineError}
            <p class="text-[11px] text-warning italic">{outlineError}</p>
          {:else}
            <ul class="space-y-1">
              {#each outlineSections as s, i (i)}
                <li
                  class="text-[11px] leading-snug"
                  style="padding-left: {(s.level - 2) * 0.75}rem"
                >
                  <div class="text-text font-medium">
                    {'#'.repeat(s.level)} {s.heading}
                  </div>
                  <div class="text-dim">{s.gist}</div>
                </li>
              {/each}
            </ul>
            <button
              type="button"
              onclick={insertOutlineAtTop}
              class="mt-2 w-full text-xs px-2 py-1 rounded bg-primary text-on-primary font-medium"
            >
              Prepend ## Outline block
            </button>
          {/if}
        </div>
      {/if}

      {#if citations.length > 0 || citeError}
        <!-- Cite-from-vault drawer. Each suggestion is a checkbox row:
             user reviews the claim + which vault note supports it,
             ticks the ones to commit, then "Add citations" splices a
             [^N] ref next to each cited line and a definitions block
             at the end of the body. All in one undoable transaction. -->
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-surface1 max-h-80 overflow-y-auto">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-secondary">vault citations</span>
            <span class="flex-1"></span>
            <button type="button" onclick={findCitations} disabled={busy !== null} class="text-[11px] text-secondary hover:underline">regenerate</button>
            <button type="button" onclick={dismissCitations} class="text-[11px] text-dim hover:text-text">dismiss</button>
          </div>
          {#if citeError}
            <p class="text-[11px] text-warning italic">{citeError}</p>
          {:else}
            <ul class="space-y-1.5">
              {#each citations as c, i (i)}
                <li class="flex items-start gap-2 text-[11px] leading-snug">
                  <input
                    type="checkbox"
                    bind:checked={c.accepted}
                    class="mt-0.5 flex-shrink-0 accent-secondary"
                    aria-label="commit citation"
                  />
                  <div class="flex-1 min-w-0">
                    <div class="text-text">"{c.claim}"</div>
                    <div class="text-secondary truncate">→ {c.title}</div>
                    <div class="text-dim">{c.rationale}</div>
                  </div>
                </li>
              {/each}
            </ul>
            {#if citations.some((c) => c.accepted)}
              <button
                type="button"
                onclick={commitCitations}
                class="mt-2 w-full text-xs px-2 py-1 rounded bg-secondary text-on-primary font-medium"
              >
                Add {citations.filter((c) => c.accepted).length} citation{citations.filter((c) => c.accepted).length === 1 ? '' : 's'}
              </button>
            {/if}
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  /* Phone-friendly tap targets — Apple HIG recommends 44x44 for
     touch surfaces. The menu's button rows use `py-2` which renders
     ~32px on text-sm; that's fine for a mouse but cramped for a
     thumb. Bump min-height on coarse pointers (touch) so adjacent
     items don't get fat-fingered. Keeps the desktop density. */
  @media (pointer: coarse) {
    [role='menu'] :global(button[role='menuitem']) {
      min-height: 44px;
    }
  }
</style>
