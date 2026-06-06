<!--
  AIOverlayHeader — the AI panel's top header bar.

  Lays out (left → right):

    Mode picker button        — opens the ChatModePicker dropdown.
                                Shows the current mode glyph, label,
                                and an "auto · <source>" badge when
                                the contextual auto-switch is active.
    Status pill               — provider · model the next turn will
                                hit. Truncated harder on narrow
                                panels so the mode label wins.
    Busy / thinking pill      — animated 3-dot indicator + cancel
                                button while a request is in flight.
                                Reduced-motion users get a static row.
    History toggle            — opens the side rail of saved threads
                                + pinned messages.
    New thread                — auto-saves the current thread, starts
                                fresh.
    Pin to right (desktop)    — turns the floating panel into a
                                fixed right gutter.
    Close                     — dismiss; unpins first if pinned, so
                                the X always has an obvious effect.

  Pure presentation + click-through. State lives in the parent (mode
  picker open, history open, busy, statusInfo); this component just
  wires the right callbacks/bindings to the buttons.
-->
<script lang="ts">
  import ChatModePicker from './ChatModePicker.svelte';
  import type { AIContextManager } from '$lib/chat/aiContextManager.svelte';

  type Props = {
    aiCtx: AIContextManager;
    statusInfo: { provider: string; model: string; sabbath: boolean } | null;
    busy: boolean;
    modePickerOpen: boolean;
    historyOpen: boolean;
    pinned: boolean;
    currentProjectName: string | null;
    currentGoalId: string | null;
    onCalendarPage: boolean;
    onCancelInflight: () => void;
    onStartNewThread: () => void;
    onTogglePinned: () => void;
    onClose: () => void;
  };

  let {
    aiCtx,
    statusInfo,
    busy,
    modePickerOpen = $bindable(),
    historyOpen = $bindable(),
    pinned,
    currentProjectName,
    currentGoalId,
    onCalendarPage,
    onCancelInflight,
    onStartNewThread,
    onTogglePinned,
    onClose
  }: Props = $props();
</script>

<header class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
  <!-- Mode picker — click to open a popover of agent modes, each
       with a one-line tagline. Mode is the headline UX choice; status
       pill + cancel + close pack to the right. -->
  <div class="relative flex-shrink-0">
    <button
      type="button"
      onclick={() => (modePickerOpen = !modePickerOpen)}
      aria-haspopup="listbox"
      aria-expanded={modePickerOpen}
      class="tap-target inline-flex items-center gap-1.5 px-2 py-1 rounded hover:bg-surface0 active:bg-surface1 text-text transition-colors"
      title={aiCtx.autoPMActive
        ? `Mode: ${aiCtx.mode.label} (auto — you're on a project page). Click to override.`
        : `Mode: ${aiCtx.mode.label} — ${aiCtx.mode.tagline}`}
    >
      <span class="text-[10px] font-semibold tracking-tight leading-none inline-flex items-center justify-center w-6 h-6 rounded-md {aiCtx.mode.kind === 'persona' ? 'bg-secondary text-on-primary' : aiCtx.mode.kind === 'contextual' ? 'bg-primary text-on-primary' : 'bg-surface1 text-subtext'}">{aiCtx.mode.glyph}</span>
      <span class="text-sm font-semibold truncate max-w-[8rem] sm:max-w-none">{aiCtx.mode.label}</span>
      {#if aiCtx.autoPMActive}
        <!-- Tiny "auto · <source>" badge. The source word makes the
             contextual switch self-explanatory — the user reads
             "auto · project" and knows where the mode came from
             (and that picking anything else takes control back).
             Clears the moment they pick. -->
        <span class="text-[9px] uppercase tracking-wider px-1 rounded bg-primary text-on-primary leading-tight whitespace-nowrap">auto · {aiCtx.autoMode}</span>
      {/if}
      <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="6 9 12 15 18 9" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </button>
    {#if modePickerOpen}
      <ChatModePicker
        modeId={aiCtx.modeId}
        {currentProjectName}
        {currentGoalId}
        {onCalendarPage}
        onSelect={aiCtx.selectMode}
        onDismiss={() => (modePickerOpen = false)}
      />
    {/if}
  </div>
  {#if statusInfo}
    <!-- Status pill — provider · model. Visible on every viewport so
         the user always knows which backend the next turn will hit
         (matters for cost transparency on paid providers and for
         "why is this slow?" when ollama is local). On narrow panels
         we truncate harder so the mode label doesn't ellipsize. -->
    <span
      class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext truncate inline-block max-w-[5.5rem] sm:max-w-[10rem]"
      title="{statusInfo.provider} · {statusInfo.model} — default backend (per-feature overrides apply individually)"
    >{statusInfo.model}</span>
  {/if}
  <span class="flex-1"></span>
  {#if busy}
    <!-- Animated thinking-pill. Replaces the earlier tiny "cancel"
         text link. Three pulsing dots + label + clear cancel button
         make it obvious work is in flight + give a prominent
         affordance to stop. Reduced-motion users get a static row
         instead of breathing dots (CSS rules in the parent). -->
    <div
      class="inline-flex items-center gap-2 px-2 py-0.5 rounded-md bg-surface1 text-[11px]"
      aria-live="polite"
    >
      <span class="ai-thinking-dots inline-flex items-center gap-0.5" aria-hidden="true">
        <span class="ai-thinking-dot block w-1 h-1 rounded-full bg-primary"></span>
        <span class="ai-thinking-dot block w-1 h-1 rounded-full bg-primary"></span>
        <span class="ai-thinking-dot block w-1 h-1 rounded-full bg-primary"></span>
      </span>
      <span class="text-subtext font-medium hidden sm:inline">thinking</span>
      <button
        onclick={onCancelInflight}
        class="text-warning hover:text-error font-medium px-1 -mx-1 rounded hover:bg-surface0 transition-colors"
        title="Stop the in-flight request (Esc)"
      >stop</button>
    </div>
  {/if}
  <!-- History toggle — opens the side rail of saved threads + pinned
       messages; auto-saves so the user never loses a good chat. -->
  <button
    type="button"
    onclick={() => { historyOpen = !historyOpen; }}
    aria-pressed={historyOpen}
    aria-label="Chat history"
    title="Chat history (saved threads + pinned messages)"
    class="tap-target inline-flex items-center justify-center px-1.5 py-1 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-colors {historyOpen ? 'text-primary bg-surface1' : ''}"
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="12" cy="12" r="9"/>
      <polyline points="12 7 12 12 15 14" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>
  </button>
  <!-- New thread — starts fresh while preserving the previous one in
       history. -->
  <button
    type="button"
    onclick={onStartNewThread}
    aria-label="New thread"
    title="Start a new conversation (current one is saved)"
    class="tap-target inline-flex items-center justify-center px-1.5 py-1 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-colors"
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M12 4v16M4 12h16" stroke-linecap="round"/>
    </svg>
  </button>
  <!-- Pin to right — desktop only. When pinned, the panel becomes a
       fixed right column rather than an overlapping sheet; the main
       page reserves a right gutter (set via the --ai-pinned-w CSS
       variable on <html>) so content reflows around it. -->
  <button
    type="button"
    onclick={onTogglePinned}
    aria-label={pinned ? 'Unpin AI panel' : 'Pin AI panel to right'}
    aria-pressed={pinned}
    title={pinned ? 'Unpin from right' : 'Pin to right edge'}
    class="hidden md:inline-flex tap-target items-center justify-center px-1.5 py-1 rounded transition-colors {pinned ? 'text-primary bg-surface1' : 'text-dim hover:text-text hover:bg-surface0 active:bg-surface1'}"
  >
    <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M9 4h6l-1 7 4 3v2h-5v6l-1 1-1-1v-6H6v-2l4-3z"/>
    </svg>
  </button>
  <button
    onclick={onClose}
    aria-label="close"
    class="tap-target inline-flex items-center justify-center text-dim hover:text-text hover:bg-surface0 active:bg-surface1 rounded px-2 py-1 text-lg leading-none transition-colors"
  >×</button>
</header>
