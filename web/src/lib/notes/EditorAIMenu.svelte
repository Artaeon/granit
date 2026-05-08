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

  type Action = 'title' | 'tldr' | 'tighten' | 'study' | 'concepts' | 'gaps';
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
    let buf = '';
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
          onChunk: (c) => {
            buf += c;
            titleSuggestions = buf
              .split(/\n+/)
              .map((l) => l.trim().replace(/^[-•*\d.\s"']+|["']\s*$/g, ''))
              .filter((l) => l.length > 0 && l.length < 120);
          },
          onDone: () => {},
          onError: (err) => {
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
    let buf = '';
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
          onChunk: (c) => { buf += c; studyRaw = buf; },
          onDone: () => {
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
    let buf = '';
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
          onChunk: (c) => {
            buf += c;
            gaps = buf.split(/\n+/).map((l) => l.trim()).filter((l) => l.length > 0);
          },
          onDone: () => {},
          onError: (err) => toast.error(err.message)
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

      <div class="border-t border-surface1 my-1"></div>

      <button role="menuitem" onclick={fireOverlay} class="w-full flex items-baseline gap-2 px-3 py-2 hover:bg-surface0 text-text">
        <span class="flex-1 text-left">Open chat overlay</span>
        <kbd class="text-[10px] text-dim font-mono">{modKey}J</kbd>
      </button>

      {#if titleSuggestions.length > 0}
        <div class="border-t border-surface1 mt-1 py-1 px-3 bg-primary/5">
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
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-warning/5 max-h-72 overflow-y-auto">
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

      {#if concepts.length > 0}
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-primary/5 max-h-72 overflow-y-auto">
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
        <div class="border-t border-surface1 mt-1 py-2 px-3 bg-primary/5 max-h-72 overflow-y-auto">
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
    </div>
  {/if}
</div>
