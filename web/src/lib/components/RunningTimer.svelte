<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { activeTimer, minutesByTaskId, elapsedSec, fmtDuration } from '$lib/stores/timer';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';

  // Hydrate the timer store on mount + on auth + on WS frames. Only
  // shown when a timer is actually running, so it doesn't take space
  // 99% of the time.

  async function refresh() {
    if (!$auth) {
      activeTimer.set(null);
      minutesByTaskId.set({});
      return;
    }
    try {
      const r = await api.listTimetracker();
      activeTimer.set(r.active);
      minutesByTaskId.set(r.minutesByTaskId ?? {});
    } catch {
      // Silent — server may be unauthed/unreachable; layout retries on auth change.
    }
  }

  onMount(() => {
    refresh();
    return onWsEvent((ev) => {
      if (ev.type === 'timer.started' || ev.type === 'timer.stopped') refresh();
    });
  });

  $effect(() => {
    void $auth;
    refresh();
  });

  async function stop() {
    try {
      await api.clockOut();
      activeTimer.set(null);
      await refresh();
      toast.success('clocked out');
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
</script>

{#if $activeTimer}
  <button
    onclick={stop}
    class="hidden sm:flex items-center gap-2 px-3 py-1 rounded text-xs font-mono"
    style="background: color-mix(in srgb, var(--color-success) 14%, transparent); color: var(--color-success); border: 1px solid color-mix(in srgb, var(--color-success) 40%, transparent);"
    title={`Tracking: ${$activeTimer.taskText} — click to stop`}
  >
    <span class="w-1.5 h-1.5 rounded-full bg-success animate-pulse"></span>
    <span class="truncate max-w-[10rem]">{$activeTimer.taskText}</span>
    <span class="tabular-nums">{fmtDuration($elapsedSec)}</span>
    <svg viewBox="0 0 24 24" class="w-3 h-3" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
  </button>
{/if}
