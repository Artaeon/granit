<!--
  EditorAIBar — sticky strip above the CodeMirror editor that surfaces
  the AI actions most useful for the current cursor / selection. Two
  modes:

    - No selection: whole-note verbs (Ask, Continue, Summarise,
      Suggest tags, Extract tasks).
    - Has selection: selection verbs (Ask, Rewrite, Explain,
      Summarise, Extract tasks) plus a "{N} chars selected" badge.

  The bar is a thin entry point — it reuses the host's existing
  AI surfaces:
    - Whole-note Ask / preset actions → AskAIRequest with the whole
      body as `text` (via onAskWholeNote with optional preset).
    - Continue at cursor → fires the Mod-Alt-Space chord into the
      editor (same code path as the keyboard shortcut + the
      EditorAIMenu's "Continue writing" entry).
    - Selection presets → AskAIRequest with the selected slice and
      a preset instruction so AskAIDialog auto-fires the right
      transform (Improve / Explain / Summarise / Tasks).
    - Selection "Ask AI about this" with no preset → identical to
      the Mod-Shift-A chord (lets the user pick their own preset
      / type a custom instruction).

  None of this reaches /chat directly — every action ultimately goes
  through chatStream → /chat/stream so the audit + gate pipeline stays
  intact (see project memory: granit_ai_pipeline).

  Why a bar in addition to the existing EditorAIMenu dropdown:
    - The menu is a discoverable LIST of every AI affordance. Good
      for "what can the AI do for me here?" but two clicks to fire.
    - The bar is the SELECTION-AWARE shortcut row. It changes its
      content as the user selects text, so the most useful action
      is always one click away.
  Both surfaces stay; they answer different questions for the user.
-->
<script lang="ts">
  import type { SelectionState } from '$lib/notes/selectionState';

  // The selection presets reuse the same instruction wording as the
  // AskAIDialog's preset chips so the audit log and the response
  // shape are consistent across entry points (bar vs. dialog vs.
  // chord). If a user runs "Rewrite" from the bar and later
  // "Improve" from the dialog, they get the same model prompt.
  const SELECTION_PRESETS = {
    rewrite:
      'Improve the writing of the following — clearer, tighter, same voice. Return only the improved text, no preamble.',
    explain:
      'Explain the following clearly. Assume the reader knows the broad surrounding context but not this specific topic. Return a short markdown explanation — definitions, intuition, one concrete example. No preamble.',
    summarise: 'Summarise the following in 3 concise bullet points.',
    tasks:
      'Extract the actionable items from the following as a markdown task list (`- [ ] task` lines). Each line should be a concrete action. Return ONLY the markdown checklist, no preamble.'
  } as const;

  // Whole-note preset instructions used when the user fires
  // Summarise / Suggest tags / Extract tasks from the no-selection
  // state. These go through onAskWholeNote() which the host wires to
  // the AskAIDialog with the whole body as `text` — same response
  // shape as the selection presets, applied to the whole note.
  const WHOLE_PRESETS = {
    summarise: 'Summarise the following in 3 concise bullet points.',
    tags:
      'Suggest 5-7 short, lowercase, hyphenated hashtags relevant to the topic of the following. Return ONLY a single line of space-separated tags starting with `#`. Example: `#research #notes-app #productivity`.',
    tasks:
      'Extract the actionable items from the following as a markdown task list (`- [ ] task` lines). Each line should be a concrete action. Return ONLY the markdown checklist, no preamble.'
  } as const;

  // Advanced selection actions — surfaced in the More menu so the
  // top row stays focused on the high-frequency five. These are
  // the tone / length / proofread / translate / transform moves the
  // user wants less often but reaches for repeatedly when they do.
  //
  // Items carry a `group` so the popover can section them with
  // small uppercase headers. Labels alone — no emoji glyphs (the
  // bar reads as a professional control surface, not a chat app).
  type MoreItem = { id: string; label: string; preset: string; group: string };
  const SELECTION_MORE: readonly MoreItem[] = [
    { group: 'Tone', id: 'formal', label: 'More formal',
      preset: 'Rewrite the following in a more formal register. Preserve meaning + structure. Return only the rewritten text, no preamble.' },
    { group: 'Tone', id: 'casual', label: 'More casual',
      preset: 'Rewrite the following in a more casual, conversational register. Preserve meaning. Return only the rewritten text, no preamble.' },
    { group: 'Tone', id: 'simpler', label: 'Explain simpler',
      preset: 'Rewrite the following so a smart non-expert can follow on first read. No jargon unless defined inline; use shorter sentences. Preserve meaning. Return only the rewritten text, no preamble.' },
    { group: 'Edit', id: 'grammar', label: 'Fix grammar',
      preset: 'Fix grammar, spelling, and punctuation in the following. Preserve voice and meaning exactly. Return only the corrected text, no preamble.' },
    { group: 'Edit', id: 'expand', label: 'Expand',
      preset: 'Expand the following into a fuller paragraph (2-4 sentences) without padding or repetition. Stay in the same voice. Return only the expanded text, no preamble.' },
    { group: 'Edit', id: 'shorten', label: 'Shorten',
      preset: 'Tighten the following into the shortest faithful version. Drop redundancy and filler; keep meaning. Return only the shortened text, no preamble.' },
    { group: 'Transform', id: 'bullets', label: 'Convert to bullets',
      preset: 'Convert the following into a tight markdown bullet list. One idea per bullet, parallel grammatical structure. Return only the list, no preamble.' },
    { group: 'Transform', id: 'table', label: 'Convert to table',
      preset: 'Convert the following into a markdown table. Infer sensible column headers from the content. If the structure doesn\'t fit a table, say so in one line instead. Return only the table.' },
    { group: 'Transform', id: 'counter', label: 'Counterargument',
      preset: 'Steelman the strongest counterargument to the claim(s) in the following. Be specific and concrete — name the weak link, then state the rebuttal in 2-4 sentences. Return only the counterargument, no preamble.' },
    { group: 'Translate', id: 'translate-en', label: 'Translate to English',
      preset: 'Translate the following into clear, natural English. Return only the translation, no preamble.' },
    { group: 'Translate', id: 'translate-de', label: 'Translate to German',
      preset: 'Translate the following into clear, natural German. Return only the translation, no preamble.' }
  ];
  const WHOLE_MORE: readonly MoreItem[] = [
    { group: 'Inspect', id: 'outline', label: 'Generate outline',
      preset: 'Read the following note and produce a markdown outline of its sections (H2 / H3 headings only, no body text). Use the existing section titles if any. Return only the outline.' },
    { group: 'Inspect', id: 'concepts', label: 'Key concepts',
      preset: 'List the 5-8 key concepts from the following note as a markdown bullet list. Each bullet: bold the concept name, one short sentence after. Return only the list.' },
    { group: 'Inspect', id: 'questions', label: 'Open questions',
      preset: 'What are the 3-5 open questions this note raises but doesn\'t answer? Return a short markdown bullet list. No preamble.' },
    { group: 'Inspect', id: 'gaps', label: 'Find gaps',
      preset: 'Read the following note as a critical editor. What is missing, unclear, or assumed without evidence? Return 3-6 specific gaps as a markdown bullet list — each gap names what\'s missing and what would close it. No preamble.' },
    { group: 'Improve', id: 'tighten', label: 'Tighten prose',
      preset: 'Tighten the prose of the following note — drop filler, sharpen verbs, prefer concrete nouns. Preserve the structure and meaning. Return the full tightened note, ready to paste back.' },
    { group: 'Improve', id: 'abstract', label: 'Write abstract',
      preset: 'Write a 3-5 sentence abstract of the following note suitable for the top of the document. Lead with the central claim; cover scope and findings; no preamble.' },
    { group: 'Improve', id: 'titles', label: 'Alternative titles',
      preset: 'Suggest 5 alternative titles for the following note. Mix registers: precise, evocative, descriptive. Return only the titles as a numbered list — no commentary.' },
    { group: 'Build', id: 'toc', label: 'Table of contents',
      preset: 'Produce a markdown table of contents for the following note from its existing headings. Use indented bullet links (`- [Heading](#heading)`). Return only the TOC.' },
    { group: 'Build', id: 'frontmatter', label: 'Suggest frontmatter',
      preset: 'Suggest YAML frontmatter for the following note: a 1-line `title`, 3-6 `tags` (lowercase, hyphenated), and a 1-sentence `summary`. Return only the frontmatter block enclosed in `---` fences, nothing else.' },
    { group: 'Build', id: 'next', label: 'Suggested next steps',
      preset: 'Based on the following note, what are the 3-5 most useful next steps for the author? Concrete actions, not vague advice. Return a short markdown bullet list. No preamble.' }
  ];

  // Distinct group labels in their original order — used by the
  // popover to render section headers without re-scanning the list
  // for each render.
  function groupsOf(items: readonly MoreItem[]): string[] {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const it of items) {
      if (!seen.has(it.group)) { seen.add(it.group); out.push(it.group); }
    }
    return out;
  }

  let moreOpen = $state(false);

  // Trigger refs + computed popover coordinates. The popover used
  // to be an `absolute` child of the bar — which gave it the
  // bar's stacking context (a position:sticky parent with
  // z-index). When the bar dropped DOWN past the editor body the
  // menu inherited the bar's z-stack and slid behind anything
  // the editor (or Tailwind containers around it) painted later.
  // Switching to position:fixed + computed coords escapes every
  // ancestor stacking context so the menu always paints on top,
  // regardless of what the editor decides to do with z-index.
  let selTriggerEl: HTMLButtonElement | null = $state(null);
  let noteTriggerEl: HTMLButtonElement | null = $state(null);
  let popoverTop = $state(0);
  let popoverRight = $state(0);
  const POPOVER_WIDTH = 224; // matches w-56 in Tailwind (14rem * 16px)

  function openMore(which: 'selection' | 'note') {
    const trigger = which === 'selection' ? selTriggerEl : noteTriggerEl;
    if (!trigger) {
      moreOpen = !moreOpen;
      return;
    }
    const rect = trigger.getBoundingClientRect();
    popoverTop = rect.bottom + 4; // 4px gap below the trigger
    // Right-anchor — distance from right viewport edge to the
    // right edge of the trigger button. Keeps the popover from
    // overflowing the screen on narrow viewports because we
    // clamp at POPOVER_WIDTH below.
    const desiredRight = window.innerWidth - rect.right;
    popoverRight = Math.max(8, desiredRight);
    moreOpen = true;
  }

  interface Props {
    /** Current selection snapshot, updated by the host via the
     *  selectionStateExtension ViewPlugin. */
    selection: SelectionState;
    /** Open the AskAIDialog against the whole note. When `preset`
     *  is provided the dialog auto-fires that instruction. */
    onAskWholeNote: (preset?: string) => void;
    /** Open the AskAIDialog against a CodeMirror range. `preset`
     *  is optional — when omitted the dialog opens empty and the
     *  user picks a preset / types one. The host is responsible
     *  for wiring the apply callbacks (replace/insertAfter) to
     *  the editor range. */
    onAskRange: (from: number, to: number, preset?: string) => void;
    /** Dispatch a keymap chord into the editor — same as the
     *  EditorAIMenu's "Continue" entry. Single source of truth on
     *  how the chord is executed. */
    onChord: (chord: string) => void;
  }

  let { selection, onAskWholeNote, onAskRange, onChord }: Props = $props();

  // Has-selection branch: surface verbs that operate on the slice.
  // Length read off selection.text rather than (to - from) so a
  // selection that crosses a CRLF normalises to the visible char
  // count the user sees in the status bar.
  let hasSelection = $derived(selection.text.length > 0);
  let selLen = $derived(selection.text.length);
  // Two flavours of the selection-length chip so very-narrow
  // viewports (<320px) don't blow the bar onto three lines. The
  // CSS swaps between them at the narrow breakpoint.
  let selLabel = $derived(
    selLen === 1 ? '1 char selected' : `${selLen.toLocaleString()} chars selected`
  );
  let selLabelShort = $derived(selLen.toLocaleString());

  // Helpers — all the bar's actions ultimately go through one of
  // these. Pulling them out here keeps the markup readable and
  // funnels every action through the has-selection guard in one
  // place so a stale click after the user collapsed their selection
  // can't misfire on from === to.
  function askRangePreset(preset: string) {
    if (!hasSelection) return;
    onAskRange(selection.from, selection.to, preset);
  }
  function askRangeFree() {
    if (!hasSelection) return;
    onAskRange(selection.from, selection.to);
  }
  function askWholeNotePreset(preset?: string) {
    onAskWholeNote(preset);
  }
  function fireContinue() {
    // Mod-Alt-Space — the chord the EditorAIMenu's "Continue writing"
    // entry uses. Going through the chord keeps the ghost-text UX
    // (Tab accepts, Esc rejects) identical to the keyboard path.
    onChord('mod+alt+ ');
  }
</script>

<!--
  The bar is part of the editor pane's flex column: header (sticky
  top-0 z-20) → bar (sticky top:var(--editor-header-height) z-10) →
  editor body. CodeMirror owns the scroller inside the editor body,
  so on desktop the bar rarely needs to actually stick — but when the
  editor pane itself scrolls (mobile keyboards push it into a
  scrollable state, the daily view stacks DailyContext/QuickAdd
  above the editor, etc.) the bar pins flush under the header
  instead of scrolling away. The header anchors at top:0 z-20 and
  the bar at top:var(--editor-header-height) z-10 so they never
  overlap — the host writes the live header height to that custom
  property via a ResizeObserver, so every breakpoint and mobile
  tap-target bump is accounted for without hardcoding.

  Backdrop-blur composition: `bg-mantle/85` is the fallback for
  browsers without backdrop-filter; `supports-[backdrop-filter]`
  upgrades to a translucent + blurred surface. position:sticky
  doesn't affect either layer — the bar's painted background still
  composites with what scrolls underneath when it's pinned.

  Catppuccin palette tokens (surface0 / surface1 / text / subtext /
  dim / primary) are wired through the global CSS variables — the
  bar inherits whatever theme the user has selected without any
  extra plumbing.
-->
<div
  class="editor-ai-bar sticky z-10 flex flex-wrap items-center gap-1 px-2 sm:px-3 py-1.5 border-b border-surface1 bg-mantle/85 supports-[backdrop-filter]:bg-mantle/60 supports-[backdrop-filter]:backdrop-blur-md text-xs"
  style="top: var(--editor-header-height, 0px);"
  role="toolbar"
  aria-label="AI actions for {hasSelection ? 'selection' : 'note'}"
>
  <!-- Mode label. Tiny so it doesn't compete with the buttons, but
       present so the user knows *why* the button set just changed
       when they expanded a selection. Hidden on the smallest phones
       to save horizontal room. -->
  <span class="hidden xs:inline text-[10px] uppercase tracking-wider text-dim mr-1 select-none">
    {hasSelection ? 'Selection' : 'Note'}
  </span>

  {#if hasSelection}
    <button
      type="button"
      onclick={askRangeFree}
      title="Ask AI about the selected text (Mod-Shift-A)"
      aria-label="Ask AI about selection"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z" stroke-linejoin="round"/>
      </svg>
      <span class="ai-bar-label">Ask AI</span>
    </button>
    <button
      type="button"
      onclick={() => askRangePreset(SELECTION_PRESETS.rewrite)}
      title="Rewrite the selection — clearer, tighter, same voice"
      aria-label="Rewrite selection"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M12 20h9" stroke-linecap="round"/>
        <path d="M16.5 3.5a2.121 2.121 0 113 3L7 19l-4 1 1-4L16.5 3.5z" stroke-linejoin="round"/>
      </svg>
      <span class="ai-bar-label">Rewrite</span>
    </button>
    <button
      type="button"
      onclick={() => askRangePreset(SELECTION_PRESETS.explain)}
      title="Explain the selection (definitions, intuition, one example)"
      aria-label="Explain selection"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="12" cy="12" r="9"/>
        <path d="M9.5 9a2.5 2.5 0 015 0c0 1.5-2.5 2-2.5 3.5" stroke-linecap="round"/>
        <circle cx="12" cy="17" r="0.6" fill="currentColor"/>
      </svg>
      <span class="ai-bar-label">Explain</span>
    </button>
    <button
      type="button"
      onclick={() => askRangePreset(SELECTION_PRESETS.summarise)}
      title="Summarise the selection in 3 bullet points"
      aria-label="Summarise selection"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M4 6h16M4 12h10M4 18h16" stroke-linecap="round"/>
      </svg>
      <span class="ai-bar-label">Summarise</span>
    </button>
    <button
      type="button"
      onclick={() => askRangePreset(SELECTION_PRESETS.tasks)}
      title="Extract actionable tasks from the selection"
      aria-label="Extract tasks from selection"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <rect x="3" y="5" width="6" height="6" rx="1"/>
        <path d="M5 8l1.5 1.5L9 7" stroke-linecap="round" stroke-linejoin="round"/>
        <path d="M12 7h9M12 12h9M12 17h9" stroke-linecap="round"/>
        <rect x="3" y="14" width="6" height="6" rx="1"/>
      </svg>
      <span class="ai-bar-label">Extract tasks</span>
    </button>

    <!-- More — overflow menu of advanced selection verbs (tone /
         grammar / length / translate). Keeps the top row focused
         on the high-frequency five; power moves live one click
         deeper but with explicit labels (not a mystery menu).
         Popover is rendered at the document root via fixed
         positioning so the sticky bar's stacking context can't
         hide it behind editor content. -->
    <button
      type="button"
      bind:this={selTriggerEl}
      onclick={() => openMore('selection')}
      aria-haspopup="menu"
      aria-expanded={moreOpen}
      title="More AI actions for the selection"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="6" cy="12" r="1.2" fill="currentColor"/>
        <circle cx="12" cy="12" r="1.2" fill="currentColor"/>
        <circle cx="18" cy="12" r="1.2" fill="currentColor"/>
      </svg>
      <span class="ai-bar-label">More</span>
    </button>

    <span class="flex-1" aria-hidden="true"></span>
    <!-- Selection length chip. Lives at the right edge so it acts as
         a status anchor the user can glance at without scanning. Tab-
         ular nums so the digits don't jitter as the count grows. -->
    <span
      class="text-[10px] text-dim font-mono tabular-nums px-1.5 py-0.5 rounded bg-surface0/60 select-none ai-bar-sel-chip"
      aria-live="polite"
      title={selLabel}
    >
      <span class="ai-bar-sel-long">{selLabel}</span>
      <span class="ai-bar-sel-short" aria-hidden="true">{selLabelShort}</span>
    </span>
  {:else}
    <button
      type="button"
      onclick={() => askWholeNotePreset()}
      title="Ask AI about this whole note"
      aria-label="Ask AI about this note"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z" stroke-linejoin="round"/>
      </svg>
      <span class="ai-bar-label">Ask about note</span>
    </button>
    <button
      type="button"
      onclick={fireContinue}
      title="Continue writing at the cursor (Mod-Alt-Space)"
      aria-label="Continue writing at cursor"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M5 12h12" stroke-linecap="round"/>
        <path d="M13 6l6 6-6 6" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      <span class="ai-bar-label">Continue</span>
    </button>
    <button
      type="button"
      onclick={() => askWholeNotePreset(WHOLE_PRESETS.summarise)}
      title="Summarise the whole note"
      aria-label="Summarise the whole note"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M4 6h16M4 12h10M4 18h16" stroke-linecap="round"/>
      </svg>
      <span class="ai-bar-label">Summarise</span>
    </button>
    <button
      type="button"
      onclick={() => askWholeNotePreset(WHOLE_PRESETS.tags)}
      title="Suggest hashtags for this note"
      aria-label="Suggest tags for this note"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <path d="M9 3L7 21M17 3l-2 18M3 9h18M2 15h18" stroke-linecap="round"/>
      </svg>
      <span class="ai-bar-label">Suggest tags</span>
    </button>
    <button
      type="button"
      onclick={() => askWholeNotePreset(WHOLE_PRESETS.tasks)}
      title="Extract actionable tasks from the whole note"
      aria-label="Extract tasks from the note"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <rect x="3" y="5" width="6" height="6" rx="1"/>
        <path d="M5 8l1.5 1.5L9 7" stroke-linecap="round" stroke-linejoin="round"/>
        <path d="M12 7h9M12 12h9M12 17h9" stroke-linecap="round"/>
        <rect x="3" y="14" width="6" height="6" rx="1"/>
      </svg>
      <span class="ai-bar-label">Extract tasks</span>
    </button>
    <!-- More — whole-note advanced verbs (outline, concepts,
         questions, tighten). Same shape as the selection-mode
         More menu. Same fixed-position popover so it can't hide
         behind editor content. -->
    <button
      type="button"
      bind:this={noteTriggerEl}
      onclick={() => openMore('note')}
      aria-haspopup="menu"
      aria-expanded={moreOpen}
      title="More AI actions for this note"
      class="ai-bar-btn"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="6" cy="12" r="1.2" fill="currentColor"/>
        <circle cx="12" cy="12" r="1.2" fill="currentColor"/>
        <circle cx="18" cy="12" r="1.2" fill="currentColor"/>
      </svg>
      <span class="ai-bar-label">More</span>
    </button>
  {/if}
</div>

<!-- Fixed-position popover. Lives OUTSIDE the bar's stacking
     context (root-of-document) so editor z-index / transforms
     can't paint over it. Two action sets share one popover:
     which list to render is driven by hasSelection at the time
     the user opened the menu. -->
{#if moreOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    role="presentation"
    class="fixed inset-0 z-[60]"
    onclick={() => (moreOpen = false)}
  ></div>
  <div
    role="menu"
    class="fixed bg-mantle border border-surface1 rounded-md shadow-xl py-1 z-[61] max-h-[70vh] overflow-y-auto"
    style="top: {popoverTop}px; right: {popoverRight}px; width: {POPOVER_WIDTH}px;"
  >
    {#each groupsOf(hasSelection ? SELECTION_MORE : WHOLE_MORE) as group, gi (group)}
      {#if gi > 0}
        <div class="border-t border-surface1 my-1" role="separator"></div>
      {/if}
      <div class="px-3 pt-1.5 pb-0.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-dim select-none">{group}</div>
      {#each (hasSelection ? SELECTION_MORE : WHOLE_MORE).filter((i) => i.group === group) as item (item.id)}
        <button
          type="button"
          role="menuitem"
          onclick={() => {
            if (hasSelection) askRangePreset(item.preset);
            else askWholeNotePreset(item.preset);
            moreOpen = false;
          }}
          class="w-full flex items-center px-3 py-1.5 text-left text-[13px] text-text hover:bg-surface0 transition-colors"
        >
          {item.label}
        </button>
      {/each}
    {/each}
  </div>
{/if}

<style>
  /* Tailwind doesn't ship an `xs` breakpoint by default; project's
     Catppuccin/Tailwind config uses Tailwind's own defaults (sm @
     640px is the smallest). The label-collapse threshold we want is
     ~480px (4-inch phones in portrait), so we hand-roll it here
     instead of bending the global config. Below this width labels
     hide and only the icons remain — tap target stays 40px so
     thumbs still land cleanly. */
  .editor-ai-bar :global(.ai-bar-btn) {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    height: 1.875rem;
    padding: 0 0.5rem;
    border-radius: 0.25rem;
    color: var(--color-subtext);
    background: transparent;
    transition: background-color 80ms ease, color 80ms ease;
    flex-shrink: 0;
    line-height: 1;
  }
  .editor-ai-bar :global(.ai-bar-btn:hover),
  .editor-ai-bar :global(.ai-bar-btn:focus-visible) {
    color: var(--color-primary);
    background: var(--color-surface0);
    outline: none;
  }
  .editor-ai-bar :global(.ai-bar-btn:focus-visible) {
    /* Visible keyboard focus — primary tint, not the browser default
       blue ring, so it harmonises with the Catppuccin palette. */
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 40%, transparent);
  }
  .editor-ai-bar :global(.ai-bar-btn:active) {
    background: var(--color-surface1);
  }

  /* Hide the per-mode header label on devices narrower than ~24rem.
     Tailwind's `hidden xs:inline` would need a custom breakpoint
     in the config; the inline media query keeps the bar self-
     contained. */
  @media (max-width: 23.9375rem) {
    .editor-ai-bar :global(.xs\:inline) {
      display: none;
    }
  }
  /* >= 24rem reveals it. The class is `hidden xs:inline` in the
     template; without an `xs` breakpoint Tailwind treats `xs:inline`
     as a no-op, so we add the reveal here. */
  @media (min-width: 24rem) {
    .editor-ai-bar :global(.xs\:inline) {
      display: inline;
    }
  }

  /* Mobile (< sm): collapse labels to icons. The tap target stays
     wide enough (≥ 40px) by bumping the button height + horizontal
     padding so thumbs still land cleanly. We rely on the title
     attribute for the verbose name — long-press shows it on most
     touch keyboards and screen readers read aria-label, so no info
     is lost when the visible label hides. */
  @media (max-width: 639px) {
    .editor-ai-bar :global(.ai-bar-label) {
      display: none;
    }
    .editor-ai-bar :global(.ai-bar-btn) {
      height: 2.5rem;
      min-width: 2.5rem;
      padding: 0 0.625rem;
      justify-content: center;
    }
  }

  /* Selection chip swaps to a digits-only flavour on very-narrow
     viewports (<320px ~= iPhone 5 / SE 1) so seven 40-px buttons +
     a long chip don't trip the bar onto three rows. Above 320px
     the verbose "243 chars selected" reads. The title attribute
     still carries the long form for hover / long-press / screen
     readers regardless. */
  .editor-ai-bar :global(.ai-bar-sel-long) {
    display: inline;
  }
  .editor-ai-bar :global(.ai-bar-sel-short) {
    display: none;
  }
  @media (max-width: 319px) {
    .editor-ai-bar :global(.ai-bar-sel-long) {
      display: none;
    }
    .editor-ai-bar :global(.ai-bar-sel-short) {
      display: inline;
    }
    /* Tighter button padding at this width too — gives the bar a
       chance to fit on a single row with the More menu visible. */
    .editor-ai-bar :global(.ai-bar-btn) {
      padding: 0 0.5rem;
    }
  }
</style>
