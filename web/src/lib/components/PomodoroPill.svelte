<!--
  Floating Pomodoro pill — tiny bottom-right indicator that expands
  into a control panel on click. Only visible when the timer is
  running OR the user manually pops it open. Lives in the layout so
  state survives navigation.

  Why bottom-right (and not the editor toolbar): a focus timer
  that disappears when the user navigates away (to peek at the
  calendar / tasks) defeats the purpose. Layout-level placement
  keeps the timer visible during a focus session no matter where
  the user wanders.

  Implementation note: the countdown is read from `pomoRemaining`
  derived store which subscribes to a 250ms interval — fine for a
  monospace MM:SS display, cheap on idle (the derived stops the
  interval when the store reports idle).
-->
<script lang="ts">
  import { fly } from 'svelte/transition';
  import {
    pomodoro,
    pomoRemaining,
    startFocus,
    startBreak,
    stopTimer,
    setDurations,
    setLastTask,
    fmtMMSS
  } from '$lib/stores/pomodoro';

  let panelOpen = $state(false);

  // Hide the pill when there's nothing to show — idle timer + no
  // recent finish + panel closed = render nothing. The layout's
  // bottom-right corner stays clean for users who don't use the
  // timer.
  let visible = $derived.by(() => {
    if ($pomodoro.mode !== 'idle') return true;
    if (panelOpen) return true;
    // Show 'finished N min ago' for 30 minutes after a focus
    // session completes so the win is visible.
    if ($pomodoro.lastFinishedAt > 0 && Date.now() - $pomodoro.lastFinishedAt < 30 * 60_000) return true;
    return false;
  });

  // When the timer hits 0, beep gently. We poll the remaining via
  // the derived; once it crosses 0 from positive we ping. Using a
  // simple oscillator avoids loading an audio asset.
  let lastTickPositive = true;
  $effect(() => {
    const ms = $pomoRemaining;
    const wasPositive = lastTickPositive;
    lastTickPositive = ms > 0;
    if (wasPositive && ms <= 0 && $pomodoro.mode !== 'idle') {
      void chime();
      // Stop the timer; lastFinishedAt is set by the store load
      // logic when we next read it. Set it now too so 'finished N
      // min ago' surfaces immediately.
      const wasFocus = $pomodoro.mode === 'focus';
      pomodoro.update((s) => ({
        ...s,
        mode: 'idle',
        endsAt: 0,
        lastFinishedAt: wasFocus ? Date.now() : s.lastFinishedAt
      }));
    }
  });

  async function chime() {
    if (typeof window === 'undefined') return;
    try {
      const Ctor =
        window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (!Ctor) return;
      const ctx = new Ctor();
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.frequency.value = 660;
      osc.type = 'sine';
      gain.gain.setValueAtTime(0, ctx.currentTime);
      gain.gain.linearRampToValueAtTime(0.18, ctx.currentTime + 0.05);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 1.0);
      osc.start();
      osc.stop(ctx.currentTime + 1.05);
    } catch {}
  }

  let label = $state($pomodoro.lastTask);
  let focusMin = $state($pomodoro.focusMin);
  let breakMin = $state($pomodoro.breakMin);

  function handleStartFocus() {
    if (focusMin !== $pomodoro.focusMin || breakMin !== $pomodoro.breakMin) {
      setDurations(focusMin, breakMin);
    }
    setLastTask(label.trim());
    startFocus(label.trim());
    panelOpen = false;
  }
  function handleStartBreak() {
    if (focusMin !== $pomodoro.focusMin || breakMin !== $pomodoro.breakMin) {
      setDurations(focusMin, breakMin);
    }
    startBreak();
    panelOpen = false;
  }

  // 'Finished N min ago' label.
  let finishedAgoLabel = $derived.by(() => {
    if ($pomodoro.lastFinishedAt === 0 || $pomodoro.mode !== 'idle') return '';
    const minAgo = Math.floor((Date.now() - $pomodoro.lastFinishedAt) / 60_000);
    if (minAgo < 1) return 'just now';
    if (minAgo < 60) return `${minAgo}m ago`;
    return '';
  });

  let pillTone = $derived.by(() => {
    if ($pomodoro.mode === 'focus') return 'bg-error/15 border-error/40 text-error';
    if ($pomodoro.mode === 'break') return 'bg-success/15 border-success/40 text-success';
    if ($pomodoro.lastFinishedAt > 0) return 'bg-success/15 border-success/40 text-success';
    return 'bg-mantle border-surface1 text-text';
  });
</script>

{#if visible}
  <!-- Position: above the bottom-right FAB cluster on desktop
       (bottom-20 ≈ 5rem clears a 3rem capture FAB + 0.5rem gap),
       above the mobile bottom-nav on phones (bottom-24 ≈ 6rem
       clears a typical ~3.5rem bottom-nav + safe area). The panel
       itself expands upward when opened, so even with a long
       task label the cluster doesn't crowd the corner. -->
  <div
    in:fly={{ y: 10, duration: 200 }}
    class="fixed bottom-24 md:bottom-20 right-3 md:right-5 z-30 flex flex-col items-end gap-2"
  >
    {#if panelOpen}
      <div class="w-72 bg-mantle border border-surface1 rounded-lg shadow-xl p-3 space-y-3" role="dialog" aria-label="Pomodoro timer">
        <div class="flex items-baseline gap-2">
          <h3 class="text-sm font-semibold text-text flex-1">Pomodoro</h3>
          <button onclick={() => (panelOpen = false)} aria-label="Close" class="text-dim hover:text-text">×</button>
        </div>
        <div>
          <label for="pomo-label" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Working on</label>
          <input
            id="pomo-label"
            bind:value={label}
            placeholder="what's the focus?"
            class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
          />
        </div>
        <div class="flex items-baseline gap-2 text-xs">
          <label class="text-dim flex items-center gap-1.5">
            focus
            <input
              type="number"
              bind:value={focusMin}
              min="5"
              max="120"
              class="w-14 px-1 py-0.5 bg-surface0 border border-surface1 rounded text-text text-right tabular-nums"
            />
            min
          </label>
          <label class="text-dim flex items-center gap-1.5">
            break
            <input
              type="number"
              bind:value={breakMin}
              min="1"
              max="60"
              class="w-12 px-1 py-0.5 bg-surface0 border border-surface1 rounded text-text text-right tabular-nums"
            />
            min
          </label>
        </div>
        <div class="flex items-center gap-2">
          {#if $pomodoro.mode === 'idle'}
            <button onclick={handleStartFocus} class="flex-1 px-3 py-1.5 bg-error text-white rounded text-sm font-medium hover:opacity-90">Start focus</button>
            <button onclick={handleStartBreak} class="px-3 py-1.5 bg-surface0 border border-surface1 text-subtext rounded text-sm hover:border-primary">Break</button>
          {:else}
            <button onclick={stopTimer} class="flex-1 px-3 py-1.5 bg-surface0 border border-surface1 text-subtext rounded text-sm hover:border-error hover:text-error">Stop</button>
          {/if}
        </div>
        <p class="text-[10px] text-dim leading-snug">
          25 min focus + 5 min break is the classic ratio. The timer survives navigation and tab close — start it once, work
          on whatever, the chime tells you when to break.
        </p>
      </div>
    {/if}
    <button
      type="button"
      onclick={() => (panelOpen = !panelOpen)}
      class="px-3 py-1.5 rounded-full border shadow-md text-xs font-mono tabular-nums inline-flex items-center gap-1.5 transition-colors {pillTone} {$pomodoro.mode === 'focus' ? 'pomo-pulse' : ''}"
      aria-label="Toggle Pomodoro panel"
      title={$pomodoro.mode === 'focus'
        ? 'Focus session — click to manage'
        : $pomodoro.mode === 'break'
          ? 'Break — click to manage'
          : 'Pomodoro timer'}
    >
      {#if $pomodoro.mode === 'focus'}
        <span aria-hidden="true">🎯</span>
        <span>{fmtMMSS($pomoRemaining)}</span>
        {#if $pomodoro.lastTask}
          <span class="opacity-70 max-w-[10rem] truncate">· {$pomodoro.lastTask}</span>
        {/if}
      {:else if $pomodoro.mode === 'break'}
        <span aria-hidden="true">☕</span>
        <span>{fmtMMSS($pomoRemaining)}</span>
        <span class="opacity-70">· break</span>
      {:else if $pomodoro.lastFinishedAt > 0}
        <span aria-hidden="true">✓</span>
        <span>session done {finishedAgoLabel}</span>
      {:else}
        <span aria-hidden="true">🍅</span>
        <span>focus</span>
      {/if}
    </button>
  </div>
{/if}

<style>
  .pomo-pulse {
    animation: pomo-pulse 2s ease-in-out infinite;
  }
  @keyframes pomo-pulse {
    0%, 100% { box-shadow: 0 0 0 0 rgba(243, 139, 168, 0.4); }
    50%      { box-shadow: 0 0 0 6px rgba(243, 139, 168, 0); }
  }
</style>
