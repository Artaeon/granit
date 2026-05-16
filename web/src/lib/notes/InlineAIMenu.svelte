<!--
  InlineAIMenu — Notion-style cursor-anchored AI command palette.

  Single entry point for every AI action in the editor. Trigger via
  Cmd-K or by typing `/ai` at the start of a line; both paths route
  through inline-ai-trigger.ts which hands us a positioned event.

  Behaviour
    • Prompt input at top — autofocused, single-line, Enter submits
      as a free-form "Ask AI to..." request.
    • Action list below — keyboard-navigable (↑/↓/Enter), adapts to
      selection state: chips toggle between "operate at cursor" and
      "rewrite this selection" verbs.
    • Context toggles — sit at the bottom: "this note" is always on
      (free, backend already injects); "backlinks" and "recent jots"
      are opt-in and the menu fetches them just before submission.
    • Esc closes; click outside closes; clicking an action streams
      directly into the editor via streamInlineAI and closes the menu.

  The menu DOES NOT render its own preview. Streaming output lands
  as ghost text in the CodeMirror surface — same visual idiom as the
  continue-writing chord. Tab/Cmd-Enter accept, Esc reject, Cmd-R
  regenerate, all handled by inline-ai.ts's keymap. This keeps the
  user's eye on the document, not on a side panel.
-->
<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, type ChatMessage } from '$lib/api';
  import { streamInlineAI } from '$lib/editor/inline-ai';
  import type { InlineAITriggerEvent } from '$lib/editor/inline-ai-trigger';

  interface Props {
    event: InlineAITriggerEvent;
    notePath: string;
    body: string;
    onClose: () => void;
  }
  let { event, notePath, body, onClose }: Props = $props();

  // Reactive shorthand — the menu opens once per trigger, so the
  // event is effectively immutable for our lifetime, but Svelte
  // doesn't know that and re-derives anyway. Cheap.
  let hasSelection = $derived(event.selection.from !== event.selection.to);
  let selectionText = $derived(event.selection.text);
  let selectionLen = $derived(event.selection.to - event.selection.from);

  let promptInput = $state('');
  let promptEl: HTMLInputElement | undefined = $state();
  let menuEl: HTMLDivElement | undefined = $state();
  let highlightedIdx = $state(0);
  let busy = $state(false);

  // Prompt history — last 20 free-form prompts the user has sent for
  // THIS note, scoped by note path. Up/Down arrows in the input cycle
  // through history (most-recent first). Persisted to localStorage so
  // a tab reload doesn't lose history; localStorage scope is the right
  // grain here (per-device, not per-vault) because what the user asks
  // about a note is intimately tied to their current train of thought.
  const HISTORY_LIMIT = 20;
  const historyKey = `granit.ai.history.${notePath}`;
  let history = $state<string[]>(loadHistory());
  let historyIdx = $state(-1); // -1 = live input; 0 = most recent

  function loadHistory(): string[] {
    if (typeof window === 'undefined') return [];
    try {
      const raw = window.localStorage.getItem(historyKey);
      if (!raw) return [];
      const arr = JSON.parse(raw);
      return Array.isArray(arr) ? arr.filter((x) => typeof x === 'string').slice(0, HISTORY_LIMIT) : [];
    } catch {
      return [];
    }
  }
  function pushHistory(prompt: string) {
    const p = prompt.trim();
    if (!p) return;
    // De-dupe — push to front, drop existing copies elsewhere in
    // the list. Keeps the most-recent-first ordering monotonic.
    history = [p, ...history.filter((x) => x !== p)].slice(0, HISTORY_LIMIT);
    try {
      window.localStorage.setItem(historyKey, JSON.stringify(history));
    } catch {
      // localStorage can throw in private mode or when full; we drop
      // the persistence and keep the in-memory list usable.
    }
  }

  // Context toggles.
  //
  //   scope = 'note'    → backend injects full note body (notePath passed
  //                       to chatStream). Default — best for whole-note
  //                       transforms like Improve / Summarize / Outline.
  //
  //   scope = 'section' → only the current ## / ### section the cursor
  //                       is in is sent. Cheaper, tighter, and the
  //                       result reads as "the AI answered just about
  //                       this part" rather than dragging in unrelated
  //                       sections. We omit notePath in this mode and
  //                       prepend the section text ourselves so the
  //                       backend's auto-inject doesn't double-up.
  //
  // The +backlinks / +7d-jots toggles are additive on top of either.
  type Scope = 'note' | 'section';
  let scope = $state<Scope>('note');
  // "Linked notes" toggle includes both backlinks (notes pointing
  // here) AND outgoing wikilinks (notes this note points to). Each
  // contributes a ~400-char body snippet so the AI can reason over
  // actual content, not just titles.
  let useLinkedNotes = $state(false);
  let useRecentJots = $state(false);

  // Detect the current section at the trigger cursor — a contiguous
  // block of lines from the nearest heading down to the next heading
  // at the same or higher level (or EOF). Returns null when the
  // cursor is in pre-heading text (top of doc / no headings).
  function detectSection(): { heading: string; body: string } | null {
    const view = event.view;
    const doc = view.state.doc;
    const pos = event.pos;
    const startLine = doc.lineAt(pos).number;
    let headingLineNum = -1;
    let headingLevel = 0;
    for (let n = startLine; n >= 1; n--) {
      const line = doc.line(n);
      const m = line.text.match(/^(#{1,6})\s+(.+)$/);
      if (m) {
        headingLineNum = n;
        headingLevel = m[1].length;
        break;
      }
    }
    if (headingLineNum === -1) return null;
    const headingLine = doc.line(headingLineNum);
    const headingMatch = headingLine.text.match(/^(#{1,6})\s+(.+)$/);
    if (!headingMatch) return null;
    let endLineNum = doc.lines;
    for (let n = headingLineNum + 1; n <= doc.lines; n++) {
      const line = doc.line(n);
      const m = line.text.match(/^(#{1,6})\s+/);
      if (m && m[1].length <= headingLevel) {
        endLineNum = n - 1;
        break;
      }
    }
    const endLine = doc.line(endLineNum);
    return {
      heading: headingMatch[2].trim(),
      body: doc.sliceString(headingLine.from, endLine.to)
    };
  }
  // Memoize once — the cursor position is fixed for the menu's
  // lifetime (closed + reopened = fresh event), so the section can't
  // change while the menu is open.
  const detectedSection = detectSection();

  // Selection-surround — pulls ~600 chars before and ~300 chars after
  // a selection so the model can rewrite consistently with what's
  // adjacent. Without this, AI rewrites of a single sentence routinely
  // drift in tone, terminology, or claim direction from the
  // surrounding paragraphs. We don't pad symmetrically because
  // "before" is what the reader has already absorbed by the time they
  // hit the selection — usually the more relevant direction.
  const SELECTION_SURROUND_BEFORE = 600;
  const SELECTION_SURROUND_AFTER = 300;
  function readSelectionSurround(
    view: import('@codemirror/view').EditorView,
    from: number,
    to: number
  ): { before: string; after: string } {
    const doc = view.state.doc;
    const beforeStart = Math.max(0, from - SELECTION_SURROUND_BEFORE);
    const afterEnd = Math.min(doc.length, to + SELECTION_SURROUND_AFTER);
    return {
      before: doc.sliceString(beforeStart, from).trimStart(),
      after: doc.sliceString(to, afterEnd).trimEnd()
    };
  }

  // ── presets ──────────────────────────────────────────────────────
  // The same chip carries its cursor-mode and selection-mode prompts.
  // The list adapts at render time based on hasSelection. Ordered by
  // expected use frequency; the top item is what hitting Enter at an
  // empty prompt does.
  type PresetCategory = 'writing' | 'research' | 'planning' | 'learning';
  type Preset = {
    id: string;
    label: string;
    /** Hint shown faded next to the label — what kind of action this
     *  is at a glance ("rewrite", "generate", etc.). */
    hint: string;
    /** Category groups the list by intent — writing, research,
     *  planning, learning. Headers render between groups so the menu
     *  reads like a tools palette, not a flat dump. */
    category: PresetCategory;
    /** Available in cursor mode (no selection)? */
    cursor: boolean;
    /** Available in selection mode? */
    selection: boolean;
    /** System prompt for the LLM. The user message is built at
     *  submission time using the current selection or body. */
    systemForCursor?: string;
    systemForSelection?: string;
    /** Whether this preset operates on the whole note body when in
     *  cursor mode. False means the action only makes sense on a
     *  selection (chip is hidden in cursor mode). */
    wholeNote?: boolean;
  };

  const CATEGORY_LABELS: Record<PresetCategory, string> = {
    writing: 'Writing',
    research: 'Research',
    planning: 'Planning',
    learning: 'Learning'
  };
  // Render order for category groups — most-used first so the menu's
  // top-of-list under the prompt input is always the most common pick.
  const CATEGORY_ORDER: PresetCategory[] = ['writing', 'planning', 'research', 'learning'];

  const PRESETS: Preset[] = [
    // ── Writing ─────────────────────────────────────────────────
    {
      id: 'continue',
      label: 'Continue writing',
      hint: 'extend at cursor',
      category: 'writing',
      cursor: true,
      selection: false,
      systemForCursor:
        "You are a writing assistant continuing the user's prose at the cursor. " +
        'Match their voice, register, and structure. Continue naturally — write 1-3 sentences ' +
        '(or 1-2 short paragraphs at most). Do NOT repeat what came before. ' +
        'Do NOT introduce headers or lists unless the surrounding text already uses them. ' +
        'Return ONLY the continuation text — no preamble, no quotes, no commentary.'
    },
    {
      id: 'improve',
      label: 'Improve writing',
      hint: 'rewrite clearer + tighter',
      category: 'writing',
      cursor: true,
      selection: true,
      wholeNote: true,
      systemForCursor:
        'Rewrite the following note for clarity and concision. Preserve voice, structure, ' +
        'and all the author\'s claims. Drop filler; sharpen verbs; prefer concrete nouns. ' +
        'Return the polished note in full, ready to paste back. No preamble.',
      systemForSelection:
        'Improve the writing of the following passage — clearer, tighter, same voice. ' +
        'Preserve every claim. Return only the rewritten text, no preamble.'
    },
    {
      id: 'fix-grammar',
      label: 'Fix grammar & spelling',
      hint: 'proofread only',
      category: 'writing',
      cursor: true,
      selection: true,
      wholeNote: true,
      systemForCursor:
        'Fix grammar, spelling, and punctuation in the following note. Preserve voice, ' +
        'structure, and meaning exactly. Do NOT shorten, expand, or rephrase. ' +
        'Return the corrected note in full. No preamble.',
      systemForSelection:
        'Fix grammar, spelling, and punctuation in the following. Preserve voice and ' +
        'meaning exactly. Return only the corrected text, no preamble.'
    },
    {
      id: 'shorter',
      label: 'Make shorter',
      hint: 'tighten without losing meaning',
      category: 'writing',
      cursor: true,
      selection: true,
      wholeNote: true,
      systemForCursor:
        'Tighten the following note into the shortest faithful version. Drop redundancy ' +
        'and filler; keep meaning and structure. Return the full shortened note, no preamble.',
      systemForSelection:
        'Tighten the following into the shortest faithful version. Drop redundancy ' +
        'and filler; keep meaning. Return only the shortened text, no preamble.'
    },
    {
      id: 'longer',
      label: 'Make longer',
      hint: 'expand with detail',
      category: 'writing',
      cursor: true,
      selection: true,
      wholeNote: true,
      systemForCursor:
        'Expand the following note with relevant detail and examples. Preserve every claim ' +
        'and the existing structure; add depth, not filler. Return the full expanded note, ' +
        'no preamble.',
      systemForSelection:
        'Expand the following into a fuller paragraph (2-4 sentences) without padding or ' +
        'repetition. Stay in the same voice. Return only the expanded text, no preamble.'
    },
    {
      id: 'match-tone',
      label: 'Match the tone of this note',
      hint: 'rewrite selection in same voice',
      category: 'writing',
      cursor: false,
      selection: true,
      systemForSelection:
        'Rewrite the following passage so its voice, register, and rhythm match the ' +
        'surrounding note (provided to you as system context). Preserve meaning. Return only ' +
        'the rewritten text, no preamble.'
    },
    {
      id: 'translate-en',
      label: 'Translate → English',
      hint: 'natural English',
      category: 'writing',
      cursor: false,
      selection: true,
      systemForSelection:
        'Translate the following into clear, natural English. Preserve markdown formatting. ' +
        'Return only the translation, no preamble.'
    },
    {
      id: 'translate-de',
      label: 'Translate → German',
      hint: 'natürliches Deutsch',
      category: 'writing',
      cursor: false,
      selection: true,
      systemForSelection:
        'Translate the following into clear, natural German. Preserve markdown formatting. ' +
        'Return only the translation, no preamble.'
    },

    // ── Planning ────────────────────────────────────────────────
    {
      id: 'extract-tasks',
      label: 'Extract tasks',
      hint: 'pull action items out',
      category: 'planning',
      cursor: true,
      selection: true,
      wholeNote: true,
      systemForCursor:
        'Read the note and surface every actionable thing in it — implicit or explicit. ' +
        'Return ONLY a markdown task list using `- [ ]` checkboxes. One task per line, ' +
        'each starting with a verb. Skip vague intentions; only items the reader could pick ' +
        'up and start. No preamble.',
      systemForSelection:
        'List every actionable thing in this passage as a markdown `- [ ]` task list. ' +
        'One task per line, each starting with a verb. No preamble.'
    },
    {
      id: 'next-steps',
      label: 'Suggest next steps',
      hint: 'what to do after this note',
      category: 'planning',
      cursor: true,
      selection: false,
      wholeNote: true,
      systemForCursor:
        'Read the note and propose 3-5 concrete next steps for the reader — what to write ' +
        'next, who to talk to, what to read, what to decide. Be specific to the content. ' +
        'Return a markdown bullet list, no preamble.'
    },
    {
      id: 'outline',
      label: 'Outline this note',
      hint: 'H2/H3 structure',
      category: 'planning',
      cursor: true,
      selection: false,
      wholeNote: true,
      systemForCursor:
        'Read the following note and produce a markdown outline of its sections (H2 / H3 ' +
        'headings only, no body text). Use existing section titles when present, propose ' +
        'new ones when there are none. Return only the outline.'
    },
    {
      id: 'summarize',
      label: 'Summarize',
      hint: 'bullet TL;DR',
      category: 'planning',
      cursor: true,
      selection: true,
      wholeNote: true,
      systemForCursor:
        'Summarize the following note in 3-5 concise bullet points. Lead each bullet with ' +
        'the concrete claim. Return ONLY the bullet list, no preamble.',
      systemForSelection:
        'Summarize the following in 3 concise bullet points. Return ONLY the bullets, no preamble.'
    },

    // ── Research ────────────────────────────────────────────────
    {
      id: 'define',
      label: 'Define',
      hint: 'short definition + example',
      category: 'research',
      cursor: false,
      selection: true,
      systemForSelection:
        'Treat the selection as a term or concept the reader wants defined. Return: ' +
        '(1) a one-sentence definition in plain language, (2) the etymology or origin if ' +
        'illuminating, (3) one concrete example. Markdown, no preamble.'
    },
    {
      id: 'steel-man',
      label: 'Steel-man this',
      hint: 'strongest version of the argument',
      category: 'research',
      cursor: false,
      selection: true,
      systemForSelection:
        'Present the strongest version of the argument in this passage — the form a careful ' +
        'opponent would have to actually engage with. Preserve the original claim; tighten the ' +
        'reasoning; supply the best evidence or principle that supports it. Return a short ' +
        'markdown paragraph, no preamble.'
    },
    {
      id: 'counter-argue',
      label: 'Counter-argue',
      hint: 'best objection to this claim',
      category: 'research',
      cursor: false,
      selection: true,
      systemForSelection:
        'Treat the selection as a claim. Give the single strongest objection a thoughtful, ' +
        'charitable critic would raise. Be specific — name the assumption it depends on, the ' +
        'evidence it ignores, or the alternative explanation it doesn\'t address. Return a ' +
        'short markdown paragraph, no preamble.'
    },
    {
      id: 'questions',
      label: 'Open questions',
      hint: 'what this note doesn\'t answer',
      category: 'research',
      cursor: true,
      selection: false,
      wholeNote: true,
      systemForCursor:
        'Read the following note and list 3-5 open questions it raises but doesn\'t answer. ' +
        'Each question should be specific to the content. Return a short markdown bullet ' +
        'list, no preamble.'
    },
    {
      id: 'gaps',
      label: 'Find gaps',
      hint: 'what\'s missing or assumed',
      category: 'research',
      cursor: true,
      selection: false,
      wholeNote: true,
      systemForCursor:
        'Read the following note as a critical editor. What is missing, unclear, or assumed ' +
        'without evidence? Return 3-6 specific gaps as a markdown bullet list — each gap ' +
        'names what\'s missing and what would close it. No preamble.'
    },
    {
      id: 'brainstorm',
      label: 'Brainstorm ideas',
      hint: 'generate options at cursor',
      category: 'research',
      cursor: true,
      selection: false,
      systemForCursor:
        'Brainstorm 5-7 concrete ideas, angles, or directions relevant to what comes before ' +
        'the cursor in this note. Format as a markdown bullet list. Be specific, not generic. ' +
        'Return only the bullets, no preamble.'
    },

    // ── Learning ────────────────────────────────────────────────
    {
      id: 'explain',
      label: 'Explain this',
      hint: 'unpack for a smart non-expert',
      category: 'learning',
      cursor: false,
      selection: true,
      systemForSelection:
        'Explain the following clearly. Assume the reader knows the broad surrounding ' +
        'context but not this specific topic. Return a short markdown explanation: ' +
        'definition, intuition, one concrete example. No preamble.'
    },
    {
      id: 'eli5',
      label: 'Explain like I\'m 5',
      hint: 'simplest possible analogy',
      category: 'learning',
      cursor: false,
      selection: true,
      systemForSelection:
        'Explain the following using language a curious child would follow. Use one concrete ' +
        'analogy from everyday life — not a list, not a definition dump. Be accurate; ' +
        'simplify but don\'t lie. Return a single short paragraph, no preamble.'
    },
    {
      id: 'quiz-me',
      label: 'Quiz me on this note',
      hint: '5 retrieval-practice questions',
      category: 'learning',
      cursor: true,
      selection: false,
      wholeNote: true,
      systemForCursor:
        'Read the note and write 5 retrieval-practice questions that test understanding of ' +
        'the core ideas — not trivia. After each question, give the answer on the next line ' +
        'prefixed with `> `. Markdown, no preamble.'
    },
    {
      id: 'connect',
      label: 'Connect to my other notes',
      hint: 'which linked notes relate, how',
      category: 'learning',
      cursor: true,
      selection: false,
      wholeNote: true,
      systemForCursor:
        'Looking at this note AND the linked notes provided as context (if any), name the ' +
        'specific concepts, claims, or examples that connect them. For each connection: name ' +
        'the linked note in [[wikilink]] form, then one sentence on what connects. If no ' +
        'linked notes were provided, say so plainly. Markdown bullet list, no preamble.'
    }
  ];

  // Filter presets by current mode + text query. Selection mode
  // hides cursor-only chips and vice versa. Sort so categories cluster
  // in CATEGORY_ORDER and the flat list lines up with the grouped
  // render below.
  let visiblePresets = $derived.by(() => {
    const filtered = PRESETS.filter((p) => (hasSelection ? p.selection : p.cursor));
    const q = promptInput.trim().toLowerCase();
    const matched = q
      ? filtered.filter((p) => p.label.toLowerCase().includes(q) || p.hint.toLowerCase().includes(q))
      : filtered;
    // Stable sort by category order; preserve insertion order within
    // each category.
    return matched.slice().sort((a, b) =>
      CATEGORY_ORDER.indexOf(a.category) - CATEGORY_ORDER.indexOf(b.category)
    );
  });

  // Whenever the visible list changes (mode flip, filter), reset
  // highlight so keyboard nav starts from the top.
  $effect(() => {
    visiblePresets.length;
    highlightedIdx = 0;
  });

  // ── context fetch ───────────────────────────────────────────────
  // Linked notes (backlinks + outgoing wikilinks) and recent jots are
  // fetched lazily on submit. Cached for the menu's lifetime so the
  // user toggling on/off doesn't re-hit the server.
  let linkedNotesCache: string | null = null;
  let jotsCache: string | null = null;

  // Per-link snippet budget. The handler caps at 400 chars; we re-
  // truncate here to a tighter ceiling so the total context doesn't
  // explode on densely-linked notes. The cap is on UTF-16 length, not
  // tokens — close enough for our scale.
  const LINKED_NOTE_SNIPPET_MAX = 320;
  const LINKED_NOTES_CAP = 6; // backlinks + outgoing combined

  async function fetchLinkedNotes(): Promise<string> {
    if (linkedNotesCache !== null) return linkedNotesCache;
    try {
      // bodies=1 gets us snippet fields per link entry so the AI sees
      // actual content from connected notes, not just titles. Without
      // bodies the prompt is no better than telling the model "these
      // titles exist" — useless for cross-note reasoning.
      const r = await api.req<{
        outgoing: ({ title: string; path?: string; snippet?: string })[];
        backlinks: ({ title: string; path?: string; snippet?: string })[];
      }>(`/links/${encodeURI(notePath)}?bodies=1`);

      // Interleave backlinks first then outgoing — backlinks tend to
      // carry deliberate connections (the other author chose to link
      // here), outgoing are this note's own references. Both useful
      // but backlinks are usually richer signal.
      const all = [
        ...(r.backlinks ?? []).map((b) => ({ ...b, direction: '←' as const })),
        ...(r.outgoing ?? []).map((o) => ({ ...o, direction: '→' as const }))
      ].slice(0, LINKED_NOTES_CAP);

      if (all.length === 0) {
        linkedNotesCache = '';
        return linkedNotesCache;
      }

      const blocks = all.map((entry) => {
        const snippet = (entry.snippet ?? '').slice(0, LINKED_NOTE_SNIPPET_MAX).trim();
        const head = `${entry.direction} [[${entry.title}]]${entry.path ? ' (' + entry.path + ')' : ''}`;
        return snippet ? `${head}\n${snippet}` : head;
      });

      linkedNotesCache =
        'Linked notes in the user\'s vault (← link IN to this note, → linked OUT from this note). ' +
        'Use these as background only — do not edit them, do not quote them verbatim unless asked.\n\n' +
        blocks.join('\n\n---\n\n');
      return linkedNotesCache;
    } catch {
      linkedNotesCache = '';
      return '';
    }
  }

  async function fetchRecentJots(): Promise<string> {
    if (jotsCache !== null) return jotsCache;
    try {
      const r = await api.listJots({ limit: 7 });
      const blocks = r.jots
        .slice(0, 7)
        .map((j) => `### ${j.date}\n${(j.body ?? '').slice(0, 800)}`);
      jotsCache = blocks.length === 0 ? '' : 'Last week of daily notes:\n\n' + blocks.join('\n\n');
      return jotsCache;
    } catch {
      jotsCache = '';
      return '';
    }
  }

  async function buildContextMessages(systemHead: string): Promise<ChatMessage[]> {
    const messages: ChatMessage[] = [{ role: 'system', content: systemHead }];
    // Section scope: include the section text as a focused system
    // prefix so the model anchors on it. The chatStream call site
    // omits notePath when scope === 'section' (see effectiveNotePath
    // below), preventing the backend from double-injecting the full
    // body on top of our targeted section.
    if (scope === 'section' && detectedSection) {
      messages.push({
        role: 'system',
        content:
          'Focus on the section "## ' + detectedSection.heading +
          '" of the user\'s note. Section content:\n\n```\n' +
          detectedSection.body + '\n```'
      });
    }
    if (useLinkedNotes) {
      const b = await fetchLinkedNotes();
      if (b) messages.push({ role: 'system', content: b });
    }
    if (useRecentJots) {
      const j = await fetchRecentJots();
      if (j) messages.push({ role: 'system', content: j });
    }
    return messages;
  }

  // Whether to pass notePath to chatStream — only for note scope.
  // In section scope we already prepended the section as a focused
  // system message; the backend's full-body auto-inject would dilute
  // that focus.
  let effectiveNotePath = $derived(scope === 'note' ? notePath : '');

  // ── submit ──────────────────────────────────────────────────────

  async function runPreset(p: Preset) {
    if (busy) return;
    busy = true;
    try {
      const view = event.view;
      // If the user typed a custom prompt while a preset was highlighted,
      // append it as an extra steering instruction.
      const extra = promptInput.trim();
      if (hasSelection && p.systemForSelection) {
        const system = extra ? p.systemForSelection + '\n\nAdditional instruction: ' + extra : p.systemForSelection;
        const messages = await buildContextMessages(system);
        // Selection-surround: include ~600 chars before and ~300 chars
        // after the selection as read-only context so the rewrite
        // stays coherent with what's around it. Without this the AI
        // routinely produces edits that disagree in tone or terminology
        // with the surrounding paragraphs.
        const surround = readSelectionSurround(view, event.selection.from, event.selection.to);
        messages.push({
          role: 'user',
          content:
            (surround.before ? 'Text BEFORE the selection (do not modify, just be aware):\n```\n' + surround.before + '\n```\n\n' : '') +
            'Apply the instruction to THIS text:\n```\n' + selectionText + '\n```' +
            (surround.after ? '\n\nText AFTER the selection (do not modify, just be aware):\n```\n' + surround.after + '\n```' : '')
        });
        consumeTriggerRange(view);
        streamInlineAI(view, {
          kind: 'replace',
          from: event.selection.from,
          to: event.selection.to,
          messages,
          notePath: effectiveNotePath
        });
      } else if (p.systemForCursor) {
        const system = extra ? p.systemForCursor + '\n\nAdditional instruction: ' + extra : p.systemForCursor;
        const messages = await buildContextMessages(system);
        if (p.wholeNote) {
          messages.push({
            role: 'user',
            content: 'Note body:\n```\n' + body + '\n```\n\nApply the instruction.'
          });
        } else if (p.id === 'continue' || p.id === 'brainstorm') {
          // For pure continuation, send the context before the cursor
          // so the model writes flowing prose without a doc dump.
          const cur = event.pos;
          const start = Math.max(0, cur - 2000);
          const before = view.state.sliceDoc(start, cur);
          const after = view.state.sliceDoc(cur, Math.min(view.state.doc.length, cur + 400));
          messages.push({
            role: 'user',
            content:
              'Text BEFORE cursor:\n```\n' + before + '\n```\n\n' +
              (after.trim().length > 0
                ? 'Text AFTER cursor (do not overwrite, just be aware):\n```\n' + after + '\n```\n\n'
                : '') +
              'Continue from the cursor:'
          });
        }
        const anchor = consumeTriggerRange(view) ?? event.pos;
        streamInlineAI(view, {
          kind: 'insert',
          anchor,
          messages,
          notePath: effectiveNotePath
        });
      }
    } finally {
      busy = false;
      onClose();
    }
  }

  // Submit a free-form prompt the user typed in. Acts on the selection
  // if there is one (replace mode), otherwise inserts at cursor.
  async function runCustomPrompt() {
    const p = promptInput.trim();
    if (!p || busy) return;
    pushHistory(p);
    busy = true;
    try {
      const view = event.view;
      if (hasSelection) {
        const system =
          'Apply the user\'s instruction to the given text. Return ONLY the resulting text, ' +
          'no preamble, no commentary, no quoted block. Preserve markdown structure unless the ' +
          'instruction explicitly says otherwise.';
        const messages = await buildContextMessages(system);
        const surround = readSelectionSurround(view, event.selection.from, event.selection.to);
        messages.push({
          role: 'user',
          content:
            'Instruction: ' + p + '\n\n' +
            (surround.before ? 'Text BEFORE the selection (context only):\n```\n' + surround.before + '\n```\n\n' : '') +
            'Text to act on:\n```\n' + selectionText + '\n```' +
            (surround.after ? '\n\nText AFTER the selection (context only):\n```\n' + surround.after + '\n```' : '')
        });
        consumeTriggerRange(view);
        streamInlineAI(view, {
          kind: 'replace',
          from: event.selection.from,
          to: event.selection.to,
          messages,
          notePath: effectiveNotePath
        });
      } else {
        const system =
          'You are writing inside the user\'s note at the cursor. Carry out the user\'s ' +
          'instruction and insert the result into the note. Return ONLY the text to insert, ' +
          'no preamble, no commentary, no surrounding quotes. Use markdown where appropriate.';
        const messages = await buildContextMessages(system);
        // Include the surrounding context so the model knows what to anchor against.
        const cur = event.pos;
        const start = Math.max(0, cur - 1500);
        const before = view.state.sliceDoc(start, cur);
        messages.push({
          role: 'user',
          content:
            'Instruction: ' + p + '\n\n' +
            'Context BEFORE cursor:\n```\n' + before + '\n```'
        });
        const anchor = consumeTriggerRange(view) ?? event.pos;
        streamInlineAI(view, {
          kind: 'insert',
          anchor,
          messages,
          notePath: effectiveNotePath
        });
      }
    } finally {
      busy = false;
      onClose();
    }
  }

  /** If the menu was opened by typing "/ai", strip that text out of
   *  the doc before the AI insertion happens. Returns the new anchor
   *  position after the strip (one to the left of the trigger range
   *  start, since the strip itself shifts positions). */
  function consumeTriggerRange(view: import('@codemirror/view').EditorView): number | undefined {
    const t = event.triggerRange;
    if (!t) return undefined;
    view.dispatch({
      changes: { from: t.from, to: t.to, insert: '' },
      selection: { anchor: t.from }
    });
    return t.from;
  }

  // ── keyboard ────────────────────────────────────────────────────

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
      return;
    }
    // History recall — Up/Down with the cursor in the input cycles
    // through previous prompts before falling through to action-list
    // navigation. We only treat the input as a "history field" when
    // the cursor is at the start AND the input is either empty or
    // already showing a history entry; otherwise Up still navigates
    // the list (so power users who don't care about history get the
    // expected behaviour). Mod-modified arrows always go to the list.
    if ((e.key === 'ArrowUp' || e.key === 'ArrowDown') && history.length > 0 && !e.metaKey && !e.ctrlKey) {
      const inHistoryMode = historyIdx >= 0 || promptInput.length === 0;
      if (inHistoryMode) {
        e.preventDefault();
        if (e.key === 'ArrowUp') {
          historyIdx = Math.min(history.length - 1, historyIdx + 1);
          promptInput = history[historyIdx] ?? '';
        } else {
          historyIdx = Math.max(-1, historyIdx - 1);
          promptInput = historyIdx === -1 ? '' : (history[historyIdx] ?? '');
        }
        return;
      }
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIdx = (highlightedIdx + 1) % Math.max(1, visiblePresets.length);
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIdx = (highlightedIdx - 1 + visiblePresets.length) % Math.max(1, visiblePresets.length);
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      // If the prompt input has text AND the focused preset's id is
      // not what the user is filtering toward, run the prompt as a
      // custom Ask. If the prompt is empty, run the highlighted
      // preset. This gives both "type a thought, hit Enter" and
      // "type to filter, arrow, Enter" patterns.
      const filtering = promptInput.trim().length > 0 && visiblePresets.length === PRESETS.length;
      if (filtering) {
        // Filtered list — interpret Enter as picking the highlighted preset.
        const p = visiblePresets[highlightedIdx];
        if (p) runPreset(p);
        return;
      }
      if (promptInput.trim().length > 0) {
        runCustomPrompt();
        return;
      }
      const p = visiblePresets[highlightedIdx];
      if (p) runPreset(p);
    }
  }

  // ── lifecycle ────────────────────────────────────────────────────

  let viewportPos = $state({ left: 0, top: 0 });

  function clampToViewport() {
    if (!menuEl) return;
    const rect = menuEl.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const margin = 8;
    let left = event.x;
    let top = event.y;
    if (left + rect.width > vw - margin) left = vw - margin - rect.width;
    if (left < margin) left = margin;
    if (top + rect.height > vh - margin) {
      // Flip above the trigger anchor when there's no room below.
      top = Math.max(margin, event.y - rect.height - 28);
    }
    viewportPos = { left, top };
  }

  onMount(() => {
    promptEl?.focus();
    // Wait one tick for the menu to lay out so we measure its real
    // size before clamping.
    tick().then(clampToViewport);
    const onResize = () => clampToViewport();
    const onDocClick = (e: MouseEvent) => {
      if (!menuEl) return;
      if (e.target instanceof Node && menuEl.contains(e.target)) return;
      onClose();
    };
    window.addEventListener('resize', onResize);
    document.addEventListener('mousedown', onDocClick);
    return () => {
      window.removeEventListener('resize', onResize);
      document.removeEventListener('mousedown', onDocClick);
    };
  });
</script>

<div
  bind:this={menuEl}
  class="fixed z-50 w-[22rem] max-w-[calc(100vw-1rem)] bg-surface0 border border-surface2 rounded shadow-xl text-text"
  style="left: {viewportPos.left}px; top: {viewportPos.top}px;"
  role="dialog"
  aria-label="AI command menu"
>
  <!-- Prompt input -->
  <div class="flex items-center gap-1.5 px-2 py-1.5 border-b border-surface1">
    <span class="text-[10px] uppercase tracking-[0.18em] text-dim font-mono">AI</span>
    {#if hasSelection}
      <span
        class="text-[10px] px-1 py-0.5 rounded bg-surface1 text-text font-mono"
        title="acting on the current selection"
      >{selectionLen} sel</span>
    {/if}
    <input
      bind:this={promptEl}
      bind:value={promptInput}
      onkeydown={onKey}
      oninput={() => { historyIdx = -1; }}
      placeholder={hasSelection ? 'tell AI what to do with the selection…' : 'ask AI anything, or pick below…'}
      class="flex-1 bg-transparent text-[13px] placeholder-dim focus:outline-none"
      disabled={busy}
    />
    {#if busy}<span class="text-[10px] text-dim font-mono">…</span>{/if}
  </div>

  <!-- Recents — top 3 history items as one-click pills so users don't
       have to hit Up repeatedly to fish out a recent prompt. Hidden
       once the user starts typing a fresh prompt (the list would
       drift out from under their fingers and pop in/out as they
       filter). Click runs the prompt immediately as a custom Ask. -->
  {#if history.length > 0 && promptInput.length === 0 && !busy}
    <div class="flex flex-wrap items-center gap-1 px-2 py-1 border-b border-surface1">
      <span class="text-[10px] text-dim font-mono uppercase tracking-wider">recent:</span>
      {#each history.slice(0, 3) as h, i (h + ':' + i)}
        <button
          type="button"
          onclick={() => { promptInput = h; runCustomPrompt(); }}
          class="text-[11px] px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-text max-w-[12rem] truncate"
          title={h}
        >{h}</button>
      {/each}
    </div>
  {/if}

  <!-- Action list — grouped by category. The flat index (i) still
       drives keyboard nav; headers between groups are zero-cost
       visuals that don't affect highlightedIdx. -->
  <ul class="max-h-[20rem] overflow-y-auto py-1" role="listbox">
    {#each visiblePresets as p, i (p.id)}
      {@const showHeader = i === 0 || visiblePresets[i - 1].category !== p.category}
      {#if showHeader}
        <li role="presentation" class="px-2 pt-2 pb-0.5 text-[9px] uppercase tracking-[0.18em] text-dim/70 font-mono select-none">
          {CATEGORY_LABELS[p.category]}
        </li>
      {/if}
      <li role="option" aria-selected={i === highlightedIdx}>
        <button
          type="button"
          onclick={() => runPreset(p)}
          onmouseenter={() => (highlightedIdx = i)}
          class="w-full flex items-baseline justify-between gap-2 px-2 py-1.5 text-left {i === highlightedIdx ? 'bg-surface1' : 'hover:bg-surface1'}"
          disabled={busy}
        >
          <span class="text-[13px] text-text">{p.label}</span>
          <span class="text-[10px] text-dim font-mono shrink-0">{p.hint}</span>
        </button>
      </li>
    {/each}
    {#if visiblePresets.length === 0}
      <li class="px-2 py-2 text-[11px] text-dim italic">
        No preset matches. Hit Enter to send your prompt as is.
      </li>
    {/if}
  </ul>

  <!-- Context bar -->
  <div class="flex items-center gap-1.5 px-2 py-1.5 border-t border-surface1 text-[10px] font-mono">
    <span class="text-dim">scope:</span>
    <!-- Note vs. section — exclusive toggle. The note button is
         always available; the section button only when the cursor
         actually lives inside a heading section. -->
    <button
      type="button"
      onclick={() => (scope = 'note')}
      class="px-1 py-0.5 rounded {scope === 'note' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
      title="send the entire note body to AI"
    >note</button>
    {#if detectedSection}
      <button
        type="button"
        onclick={() => (scope = 'section')}
        class="px-1 py-0.5 rounded {scope === 'section' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
        title="send only the current section: {detectedSection.heading}"
      >§ {detectedSection.heading.length > 14 ? detectedSection.heading.slice(0, 14) + '…' : detectedSection.heading}</button>
    {/if}
    <span class="text-dim opacity-40 mx-0.5">|</span>
    <button
      type="button"
      onclick={() => (useLinkedNotes = !useLinkedNotes)}
      class="px-1 py-0.5 rounded {useLinkedNotes ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
      title="include short body snippets from up to 6 linked notes (both backlinks and outgoing wikilinks) — the AI then reasons over actual content, not just titles"
    >+ linked notes</button>
    <button
      type="button"
      onclick={() => (useRecentJots = !useRecentJots)}
      class="px-1 py-0.5 rounded {useRecentJots ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
      title="include the last 7 days of daily notes"
    >+ 7d jots</button>
    <span class="ml-auto text-dim opacity-60">
      ↑↓ {history.length > 0 ? 'history/pick' : 'pick'} · ⏎ run · Esc
    </span>
  </div>
</div>
