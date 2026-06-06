<!--
  AIOverlayComposerStrip — the thin row that sits between the
  context-chips strip and the composer textarea.

  Left side:  live mode + persona chips. The full picker still lives
              in the header dropdown; these chips expose the active
              state so the user reads "what mode am I in and which
              persona is loaded" without opening a menu. Clicking
              either chip toggles the same picker.

  Right side: one-tap chips for the most-used slash commands —
              Briefing, Triage, Deadlines, Clear. Each funnels into
              the same handler the slash router already calls so
              behaviour stays consistent (e.g. /clear == Clear == auto-
              save-and-start-new-thread).
-->
<script lang="ts">
  import type { AIContextManager } from '$lib/chat/aiContextManager.svelte';

  type Props = {
    aiCtx: AIContextManager;
    modePickerOpen: boolean;
    busy: boolean;
    sabbathActive: boolean;
    onBriefing: () => void;
    onTriage: () => void;
    onDeadlines: () => void;
    onStartNewThread: () => void;
  };

  let {
    aiCtx,
    modePickerOpen = $bindable(),
    busy,
    sabbathActive,
    onBriefing,
    onTriage,
    onDeadlines,
    onStartNewThread
  }: Props = $props();
</script>

<div class="border-t border-surface1 px-3 pt-2 pb-1 flex items-center gap-1.5 flex-shrink-0 flex-wrap">
  <button
    type="button"
    onclick={() => (modePickerOpen = !modePickerOpen)}
    class="inline-flex items-center gap-1 text-[11px] text-dim hover:text-text px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 transition-colors max-w-[10rem]"
    title="Active mode — click to change ({aiCtx.mode.tagline})"
    aria-haspopup="listbox"
    aria-expanded={modePickerOpen}
  >
    <span class="{aiCtx.mode.kind === 'persona' ? 'text-secondary' : aiCtx.mode.kind === 'contextual' ? 'text-primary' : 'text-subtext'}">●</span>
    <span class="truncate">{aiCtx.mode.label}</span>
  </button>
  {#if aiCtx.lastPersona}
    <button
      type="button"
      onclick={() => (modePickerOpen = !modePickerOpen)}
      class="inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded transition-colors max-w-[10rem] {aiCtx.modeId === aiCtx.lastPersona.id ? 'bg-secondary text-on-primary' : 'text-dim hover:text-text bg-surface0 hover:bg-surface1'}"
      title="Persona — {aiCtx.modeId === aiCtx.lastPersona.id ? 'active' : 'last used'}. Click to open picker."
    >
      <span class="text-[9px] font-mono">{aiCtx.lastPersona.glyph}</span>
      <span class="truncate">{aiCtx.lastPersona.label}</span>
    </button>
  {/if}
  <span class="flex-1"></span>
  <!-- Slash-command shortcut chips. Funnel into the same handlers
       the slash router already calls so behaviour stays consistent. -->
  <button
    type="button"
    onclick={onBriefing}
    disabled={busy || sabbathActive}
    class="text-[11px] text-dim hover:text-text px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
    title="Run daily briefing (/briefing)"
  >Briefing</button>
  <button
    type="button"
    onclick={onTriage}
    disabled={busy || sabbathActive}
    class="text-[11px] text-dim hover:text-text px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
    title="Triage open tasks (/triage)"
  >Triage</button>
  <button
    type="button"
    onclick={onDeadlines}
    disabled={busy || sabbathActive}
    class="text-[11px] text-dim hover:text-text px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
    title="Surface upcoming deadlines (/deadlines)"
  >Deadlines</button>
  <button
    type="button"
    onclick={onStartNewThread}
    disabled={busy}
    class="text-[11px] text-dim hover:text-error px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
    title="Start a new thread (/new) — current thread is auto-saved to history"
  >Clear</button>
</div>
