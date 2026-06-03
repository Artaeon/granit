<!--
  InlineAIMenu — Notion-style cursor-anchored AI command palette.

  Single entry point for every AI action in the editor. Trigger via
  Cmd-/ (Ctrl-/ on non-Mac) or by typing `/ai` at the start of a line;
  both paths route through inline-ai-trigger.ts which hands us a
  positioned event.

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
  import { onDestroy, onMount, tick } from 'svelte';
  import { api, type ChatMessage, type AIPromptEntry } from '$lib/api';
  import { streamInlineAI } from '$lib/editor/inline-ai';
  import type { InlineAITriggerEvent } from '$lib/editor/inline-ai-trigger';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { record as recordSharedPrompt, list as listSharedPrompts } from '$lib/ai/recentPrompts';
  import {
    type Preset,
    type PresetCategory,
    PRESETS,
    CATEGORY_LABELS,
    CATEGORY_ORDER
  } from './inline-ai-presets';

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
  //
  // historyKey is $derived from notePath so a parent reusing this
  // component across notes (without remount) gets the right per-note
  // storage bucket. The previous form was `const historyKey = …` which
  // froze the original notePath value forever — pushHistory would
  // then write to the OLD bucket even after the prop changed. svelte-
  // check rightly flagged this as state_referenced_locally; the
  // practical risk was low (menu is transient per-trigger) but the
  // fix is one line.
  const HISTORY_LIMIT = 20;
  let historyKey = $derived(`granit.ai.history.${notePath}`);
  let history = $state<string[]>(loadHistory());
  let historyIdx = $state(-1); // -1 = live input; 0 = most recent

  // Cross-source recents — prompts the user wrote in the global chat
  // overlay that this note's per-note history hasn't already seen.
  // Filtered to source=chat so we don't show the user their own
  // inline prompts twice. Computed once on mount; not reactive
  // because the menu lifecycle is short — opening it again rebuilds.
  let crossRecents = $state<{ prompt: string }[]>([]);
  function refreshCrossRecents() {
    const seen = new Set(history.map((h) => h.toLowerCase()));
    crossRecents = listSharedPrompts({ source: 'chat', limit: 6 })
      .filter((r) => !seen.has(r.prompt.toLowerCase()))
      .slice(0, 2);
  }

  // ── Prompt library — user-curated saved prompts ─────────────────
  // Fetched once on mount; filtered to entries that match the current
  // cursor state (selection vs empty) so the user only sees library
  // items that make sense to fire right now. Rendered as a separate
  // "Library" group after the curated categories — kept distinct so
  // the user can tell "this is mine" from "this ships with granit."
  let libraryAll = $state<AIPromptEntry[]>([]);
  let libraryFiltered = $derived.by(() => {
    const q = promptInput.trim().toLowerCase();
    return libraryAll.filter((e) => {
      // Scope filter — strict per cursor state, but 'either' shows in both.
      if (hasSelection && e.scope === 'cursor') return false;
      if (!hasSelection && e.scope === 'selection') return false;
      // Text filter — same fuzzy substring as the preset list.
      if (q && !e.label.toLowerCase().includes(q) && !e.prompt.toLowerCase().includes(q)) return false;
      return true;
    });
  });

  async function loadLibrary() {
    try {
      const lib = await api.getAIPrompts();
      libraryAll = lib.entries ?? [];
    } catch {
      libraryAll = [];
    }
  }

  // Run a library entry. Library entries are user-authored prompts —
  // they don't carry preset-style system/cursor prompt pairs, just a
  // single prompt string. Route them through the same custom-prompt
  // path as a free-form typed prompt, prefilling the input so the
  // user sees what's about to fire (and could edit before submit
  // if they wanted, though most clicks should commit immediately).
  function runLibraryEntry(entry: AIPromptEntry) {
    promptInput = entry.prompt;
    runCustomPrompt();
  }

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
    // Also write to the shared recent-prompts log so the chat
    // overlay can offer this prompt as a recent. Non-fatal if the
    // log write fails for any reason — per-note history above is the
    // primary record for this surface.
    recordSharedPrompt({ prompt: p, source: 'inline', notePath });
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
  // The static preset catalog lives in ./inline-ai-presets.ts — type
  // defs, ordering, prompts. This component focuses on the runtime
  // behaviour (filtering, keyboard nav, streaming, accept/reject).
  // Adding a preset is a one-block edit in the sibling module.

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
    void visiblePresets;
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
  // that focus. `undefined` (not `''`) so the field is omitted from
  // the request body entirely — chatStream's notePath param is
  // `string | undefined` and an explicit empty string would still
  // round-trip a `"notePath": ""` the backend has to filter out.
  let effectiveNotePath = $derived(scope === 'note' ? notePath : undefined);

  // Set when the menu is closed (either explicitly or by parent-driven
  // unmount). runPreset/runCustomPrompt await on buildContextMessages
  // BEFORE consumeTriggerRange + streamInlineAI; if the user clicks
  // outside the editor (or otherwise dismisses the menu) during that
  // await, the chain would otherwise still strip the trigger range
  // and start a stream against a torn-down menu — orphaned ghost
  // text in the editor.
  let closed = false;
  onDestroy(() => { closed = true; });

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
        if (closed) return;
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
        if (closed) return;
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
        if (closed) return;
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
        if (closed) return;
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

  // ── send-to-chat bridge ─────────────────────────────────────────
  // Escape hatch from inline edit → conversation. When a user's
  // intent grows past "rewrite this passage" into "let's talk about
  // this", the inline menu would force them to either commit a stub
  // and continue elsewhere or close + reopen the Cmd+J overlay
  // manually. This button seeds the overlay with the current note
  // path, the selection (if any), and the prompt they typed, then
  // closes the inline menu. The overlay opens with the prefilled
  // composer; the user reviews and sends. Nothing is inserted into
  // the doc by this path.
  function sendToChat() {
    if (busy) return;
    const userPrompt = promptInput.trim();
    // Build a seed that names the source (so the chat reply isn't
    // contextless) and frames the conversation around what the user
    // was about to do. Selection text is included verbatim when
    // short; truncated with an ellipsis when long.
    const sourceLine = `(From [[${notePath}]])`;
    let body = userPrompt || 'Help me think about this.';
    if (hasSelection) {
      const sel = selectionText.length > 600
        ? selectionText.slice(0, 600) + '…'
        : selectionText;
      body += '\n\nSelection:\n```\n' + sel + '\n```';
    }
    openAIOverlay({
      text: sourceLine + '\n\n' + body,
      send: false
    });
    onClose();
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
      // If the prompt input has text AND there's still at least one
      // preset visible, the user is filtering — Enter picks the
      // highlighted preset. If the query cleared the list (matched
      // nothing), Enter runs the prompt as a custom Ask. This gives
      // both "type a thought, hit Enter" and "type to filter,
      // arrow, Enter" patterns. The earlier comparison against
      // `PRESETS.length` was always true in practice (mode-filter
      // narrows visiblePresets BELOW the unfiltered total before any
      // query runs), so it added complexity without doing work.
      const filtering = promptInput.trim().length > 0 && visiblePresets.length > 0;
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
    refreshCrossRecents();
    void loadLibrary();
    // Wait one tick for the menu to lay out so we measure its real
    // size before clamping.
    tick().then(clampToViewport);
    const onResize = () => clampToViewport();
    const onDocClick = (e: MouseEvent) => {
      if (!menuEl) return;
      // Only the primary button closes the menu on outside-click.
      // Right-click in the editor would otherwise dismiss the menu
      // before the OS spell-check menu opened, eating the chance to
      // act on the menu's current state.
      if (e.button !== 0) return;
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
       filter). Click runs the prompt immediately as a custom Ask.
       Below the per-note recents, an optional row of recents from
       the chat overlay so prompts the user wrote in conversation
       are one click away here too. -->
  {#if (history.length > 0 || crossRecents.length > 0) && promptInput.length === 0 && !busy}
    <div class="px-2 py-1 border-b border-surface1 space-y-0.5">
      {#if history.length > 0}
        <div class="flex flex-wrap items-center gap-1">
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
      {#if crossRecents.length > 0}
        <div class="flex flex-wrap items-center gap-1">
          <span class="text-[10px] text-dim font-mono uppercase tracking-wider" title="from the Cmd+J chat sidebar">from chat:</span>
          {#each crossRecents as r, i (r.prompt + ':' + i)}
            <button
              type="button"
              onclick={() => { promptInput = r.prompt; runCustomPrompt(); }}
              class="text-[11px] px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-subtext max-w-[12rem] truncate"
              title={r.prompt}
            >↗ {r.prompt}</button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Action list — grouped by category. The flat index (i) still
       drives keyboard nav; headers between groups are zero-cost
       visuals that don't affect highlightedIdx.
       max-h adapts to viewport so a phone with the keyboard up
       doesn't get a list that runs off-screen. -->
  <ul class="max-h-[20rem] sm:max-h-[20rem] [max-height:50vh] overflow-y-auto py-1" role="listbox">
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
    {#if visiblePresets.length === 0 && libraryFiltered.length === 0}
      <li class="px-2 py-2 text-[11px] text-dim italic">
        No preset matches. Hit Enter to send your prompt as is.
      </li>
    {/if}
    <!-- Library — user-saved prompts. Separate section after curated
         categories so the user can distinguish their own prompts from
         the built-in presets at a glance. Hidden when empty so the
         menu doesn't show a useless heading on a fresh vault. -->
    {#if libraryFiltered.length > 0}
      <li role="presentation" class="px-2 pt-2 pb-0.5 text-[9px] uppercase tracking-[0.18em] text-secondary/70 font-mono select-none">
        Library
      </li>
      {#each libraryFiltered as e (e.id)}
        <!-- aria-selected={false}: library entries aren't keyboard-
             navigable via the highlightedIdx scheme (presets are);
             they're click/tap targets. ARIA still requires the
             attribute on every role="option" so screen readers can
             announce list position correctly. -->
        <li role="option" aria-selected={false}>
          <button
            type="button"
            onclick={() => runLibraryEntry(e)}
            class="w-full flex items-baseline justify-between gap-2 px-2 py-1.5 text-left hover:bg-surface1"
            disabled={busy}
            title={e.prompt}
          >
            <span class="text-[13px] text-text truncate">{e.label}</span>
            <span class="text-[10px] text-dim font-mono shrink-0">{e.scope === 'either' ? '' : e.scope}</span>
          </button>
        </li>
      {/each}
    {/if}
  </ul>

  <!-- Context bar — wraps on narrow screens so the toggles don't
       overflow the menu width; keyboard hint hides on touch since
       there are no chords to read. -->
  <div class="flex items-center flex-wrap gap-x-1.5 gap-y-1 px-2 py-1.5 border-t border-surface1 text-[10px] font-mono">
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
    <!-- Hand-off to the global chat sidebar. Seeded with the note
         path + selection + the prompt the user was typing; nothing
         gets inserted into the doc by this path. -->
    <button
      type="button"
      onclick={sendToChat}
      disabled={busy}
      class="px-1 py-0.5 rounded bg-surface0 text-dim hover:bg-surface1 hover:text-text"
      title="open the Cmd+J chat sidebar pre-filled with this note + your prompt"
    >↗ chat</button>
    <span class="ml-auto text-dim opacity-60 hidden sm:inline">
      ↑↓ {history.length > 0 ? 'history/pick' : 'pick'} · ⏎ run · Esc
    </span>
  </div>
</div>
