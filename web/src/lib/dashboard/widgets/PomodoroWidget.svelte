<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { toast } from '$lib/components/toast';

  // Pure-client pomodoro. State persists in localStorage so tab reloads
  // pick up where you left off; runs in the foreground only (no service-
  // worker timer for v1 — keep the page open).

  type Phase = 'idle' | 'work' | 'break';

  const STATE_KEY = 'granit.pomodoro';
  const WORK_MIN = 25;
  const BREAK_MIN = 5;

  interface Stored {
    phase: Phase;
    /** epoch ms */
    endsAt: number;
    /** ms remaining at last pause; 0 if running */
    paused: number;
  }

  let phase = $state<Phase>('idle');
  let endsAt = $state(0);
  let paused = $state(0);
  let now = $state(Date.now());
  let tick: ReturnType<typeof setInterval> | undefined;

  function save() {
    const s: Stored = { phase, endsAt, paused };
    try { localStorage.setItem(STATE_KEY, JSON.stringify(s)); } catch {}
  }
  function load() {
    try {
      const v = localStorage.getItem(STATE_KEY);
      if (!v) return;
      const s = JSON.parse(v) as Stored;
      if (s.phase === 'idle' || s.phase === 'work' || s.phase === 'break') {
        phase = s.phase;
        endsAt = s.endsAt;
        paused = s.paused;
      }
    } catch {}
  }

  onMount(() => {
    load();
    tick = setInterval(() => {
      now = Date.now();
      if (phase !== 'idle' && paused === 0 && now >= endsAt) finishPhase();
    }, 1000);
  });
  onDestroy(() => clearInterval(tick));

  function finishPhase() {
    const wasPhase = phase;
    if (wasPhase === 'work') {
      phase = 'break';
      endsAt = Date.now() + BREAK_MIN * 60_000;
      paused = 0;
      toast.success('Work session done — break time');
      tryNotify('Pomodoro', 'Time for a 5-minute break.');
    } else {
      phase = 'idle';
      endsAt = 0;
      paused = 0;
      toast.success('Break done — ready for another?');
      tryNotify('Pomodoro', 'Break over.');
    }
    save();
  }

  function start() {
    if ((typeof Notification !== 'undefined') && Notification.permission === 'default') {
      Notification.requestPermission();
    }
    phase = 'work';
    endsAt = Date.now() + WORK_MIN * 60_000;
    paused = 0;
    save();
  }
  function pause() {
    if (phase === 'idle' || paused !== 0) return;
    paused = Math.max(0, endsAt - Date.now());
    save();
  }
  function resume() {
    if (paused === 0) return;
    endsAt = Date.now() + paused;
    paused = 0;
    save();
  }
  function reset() {
    phase = 'idle';
    endsAt = 0;
    paused = 0;
    save();
  }

  function tryNotify(title: string, body: string) {
    if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
    try { new Notification(title, { body, icon: '/icon.svg' }); } catch {}
  }

  let remaining = $derived.by(() => {
    if (phase === 'idle') return WORK_MIN * 60_000;
    if (paused !== 0) return paused;
    return Math.max(0, endsAt - now);
  });

  function fmt(ms: number): string {
    const total = Math.ceil(ms / 1000);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }

  let pct = $derived.by(() => {
    const total = (phase === 'break' ? BREAK_MIN : WORK_MIN) * 60_000;
    return phase === 'idle' ? 0 : Math.max(0, Math.min(100, ((total - remaining) / total) * 100));
  });

  let phaseLabel = $derived(phase === 'idle' ? 'ready' : phase === 'work' ? 'focus' : 'break');
  let isRunning = $derived(phase !== 'idle' && paused === 0);
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Pomodoro</h2>
    <span class="text-[10px] uppercase tracking-wider {phase === 'work' ? 'text-error' : phase === 'break' ? 'text-success' : 'text-dim'}">
      {phaseLabel}
    </span>
  </div>

  <div class="flex items-center justify-center mb-4">
    <div class="text-4xl sm:text-5xl font-mono tabular-nums text-text leading-none">
      {fmt(remaining)}
    </div>
  </div>

  <div class="h-1.5 bg-mantle rounded-full overflow-hidden mb-4">
    <div
      class="h-full transition-all {phase === 'break' ? 'bg-success' : 'bg-error/70'}"
      style="width: {pct}%"
    ></div>
  </div>

  <div class="flex items-center gap-2">
    {#if phase === 'idle'}
      <button onclick={start} class="flex-1 px-3 py-2 bg-primary text-mantle rounded text-sm font-medium">Start 25/5</button>
    {:else if isRunning}
      <button onclick={pause} class="flex-1 px-3 py-2 bg-surface1 text-text rounded text-sm font-medium">Pause</button>
      <button onclick={reset} class="px-3 py-2 text-dim hover:text-error text-sm">Reset</button>
    {:else}
      <button onclick={resume} class="flex-1 px-3 py-2 bg-primary text-mantle rounded text-sm font-medium">Resume</button>
      <button onclick={reset} class="px-3 py-2 text-dim hover:text-error text-sm">Reset</button>
    {/if}
  </div>
</section>
