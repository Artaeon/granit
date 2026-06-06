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
  import type { AIPromptEntry } from '$lib/api';
  import { streamInlineAI } from '$lib/editor/inline-ai';
  import type { InlineAITriggerEvent } from '$lib/editor/inline-ai-trigger';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { createPromptHistoryController } from './inlineAIPromptHistory.svelte';
  import {
    createContextScopeController,
    readSelectionSurround
  } from './inlineAIContextScope.svelte';
  import { createPresetFilterController } from './inlineAIPresetFilter.svelte';
  import {
    createMenuPositionController,
    installMenuDismissHandlers
  } from './inlineAIMenuPosition.svelte';
  import { createMenuKeyHandler } from './inlineAIMenuKeys';
  import {
    type Preset,
    CATEGORY_LABELS
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
  let busy = $state(false);

  // Prompt history controller — per-note recents (Up/Down cycles),
  // cross-source recents (chat overlay prompts), and the pushHistory
  // de-dupe + shared-log mirror. Lives in inlineAIPromptHistory.svelte.ts
  // so this component stays focused on layout + streaming.
  const historyCtl = createPromptHistoryController({
    getNotePath: () => notePath
  });
  let history = $derived(historyCtl.history);
  let crossRecents = $derived(historyCtl.crossRecents);

  // Preset + library filter controller. Owns the static-preset
  // filter (selection-mode aware, text-query fuzzy), the user-saved
  // library fetch/filter, and the keyboard-nav highlight cursor.
  const presetCtl = createPresetFilterController({
    getHasSelection: () => hasSelection,
    getQuery: () => promptInput
  });
  let visiblePresets = $derived(presetCtl.visiblePresets);
  let libraryFiltered = $derived(presetCtl.libraryFiltered);

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


  // Context-scope controller. Owns the note/section toggle, the
  // additive linked-notes / recent-jots toggles, the section
  // detection (memoized once per trigger), the per-toggle fetch +
  // cache for cross-note and jot bodies, and the effectiveNotePath
  // derived from the current scope. Lives in
  // inlineAIContextScope.svelte.ts so the menu stays focused on UI.
  const contextCtl = createContextScopeController({
    view: event.view,
    pos: event.pos,
    notePath
  });
  let detectedSection = $derived(contextCtl.detectedSection);
  let effectiveNotePath = $derived(contextCtl.effectiveNotePath);

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
        const messages = await contextCtl.buildContextMessages(system);
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
        const messages = await contextCtl.buildContextMessages(system);
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
    historyCtl.pushHistory(p);
    busy = true;
    try {
      const view = event.view;
      if (hasSelection) {
        const system =
          'Apply the user\'s instruction to the given text. Return ONLY the resulting text, ' +
          'no preamble, no commentary, no quoted block. Preserve markdown structure unless the ' +
          'instruction explicitly says otherwise.';
        const messages = await contextCtl.buildContextMessages(system);
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
        const messages = await contextCtl.buildContextMessages(system);
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
  // Chord precedence and full rules live in inlineAIMenuKeys.ts;
  // this just wires the getters/setters / callbacks.
  const onKey = createMenuKeyHandler({
    getPromptInput: () => promptInput,
    setPromptInput: (s) => { promptInput = s; },
    getHistory: () => history,
    getHistoryIdx: () => historyCtl.historyIdx,
    setHistoryIdx: (n) => { historyCtl.historyIdx = n; },
    getVisiblePresets: () => visiblePresets,
    getHighlightedIdx: () => presetCtl.highlightedIdx,
    setHighlightedIdx: (n) => { presetCtl.highlightedIdx = n; },
    runPreset: (p) => runPreset(p),
    runCustomPrompt: () => runCustomPrompt(),
    onClose: () => onClose()
  });

  // ── lifecycle ────────────────────────────────────────────────────

  // Menu position controller — viewport-clamped {left, top} the fixed
  // wrapper consumes. Re-clamped via $effect whenever the visible
  // content set changes (library fetch grows the menu after mount,
  // user query shrinks/grows the preset list). tick() defers to the
  // post-render frame so the measurement reflects the new height.
  const positionCtl = createMenuPositionController({
    getAnchor: () => ({ x: event.x, y: event.y }),
    getMenuEl: () => menuEl
  });
  $effect(() => {
    void visiblePresets;
    void libraryFiltered;
    tick().then(positionCtl.clamp);
  });

  onMount(() => {
    promptEl?.focus();
    historyCtl.refreshCrossRecents();
    void presetCtl.loadLibrary();
    // Wait one tick for the menu to lay out so we measure its real
    // size before clamping.
    tick().then(positionCtl.clamp);
    return installMenuDismissHandlers({
      getMenuEl: () => menuEl,
      onResize: positionCtl.clamp,
      onOutsideClick: onClose
    });
  });
</script>

<div
  bind:this={menuEl}
  class="fixed z-50 w-[22rem] max-w-[calc(100vw-1rem)] bg-surface0 border border-surface2 rounded shadow-xl text-text"
  style="left: {positionCtl.pos.left}px; top: {positionCtl.pos.top}px;"
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
      oninput={() => { historyCtl.historyIdx = -1; }}
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
       visuals that don't affect presetCtl.highlightedIdx.
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
      <li role="option" aria-selected={i === presetCtl.highlightedIdx}>
        <button
          type="button"
          onclick={() => runPreset(p)}
          onmouseenter={() => (presetCtl.highlightedIdx = i)}
          class="w-full flex items-baseline justify-between gap-2 px-2 py-1.5 text-left {i === presetCtl.highlightedIdx ? 'bg-surface1' : 'hover:bg-surface1'}"
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
             navigable via the presetCtl.highlightedIdx scheme (presets are);
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
      onclick={() => (contextCtl.scope = 'note')}
      class="px-1 py-0.5 rounded {contextCtl.scope === 'note' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
      title="send the entire note body to AI"
    >note</button>
    {#if detectedSection}
      <button
        type="button"
        onclick={() => (contextCtl.scope = 'section')}
        class="px-1 py-0.5 rounded {contextCtl.scope === 'section' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
        title="send only the current section: {detectedSection.heading}"
      >§ {detectedSection.heading.length > 14 ? detectedSection.heading.slice(0, 14) + '…' : detectedSection.heading}</button>
    {/if}
    <span class="text-dim opacity-40 mx-0.5">|</span>
    <button
      type="button"
      onclick={() => (contextCtl.useLinkedNotes = !contextCtl.useLinkedNotes)}
      class="px-1 py-0.5 rounded {contextCtl.useLinkedNotes ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
      title="include short body snippets from up to 6 linked notes (both backlinks and outgoing wikilinks) — the AI then reasons over actual content, not just titles"
    >+ linked notes</button>
    <button
      type="button"
      onclick={() => (contextCtl.useRecentJots = !contextCtl.useRecentJots)}
      class="px-1 py-0.5 rounded {contextCtl.useRecentJots ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
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
