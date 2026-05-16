<script lang="ts">
  // FindTime — modal that surfaces the first N gaps in the active
  // calendar feed that fit a chosen duration inside a chosen window.
  // Composes with the active filters (project, kind, hidden types)
  // because the caller passes the already-filtered `events` list.
  //
  // Picking a gap closes the modal and seeds the UnifiedCreate flow
  // with the chosen start + end so the user goes from "I need 90 min"
  // to a typed event title in one click.

  import type { CalendarEvent } from '$lib/api';
  import { findFreeGaps, type FreeGap } from './findTime';
  import { addDays, fmtDateISO } from './utils';

  let {
    open = $bindable(false),
    events,
    onPick
  }: {
    open?: boolean;
    events: CalendarEvent[];
    /** Called with the chosen gap's start Date + duration minutes.
     *  Caller seeds whichever create-flow it prefers. */
    onPick: (start: Date, durationMinutes: number) => void;
  } = $props();

  // Form state — sensible defaults: 60 min, this week, weekdays only.
  let duration = $state(60);
  let windowKey = $state<'today' | 'week' | '2weeks' | 'month'>('week');
  let weekdaysOnly = $state(true);
  let workStart = $state(9);
  let workEnd = $state(18);

  // Reset to defaults on each open so a previous session's "today"
  // pick doesn't surprise the user on the next launch.
  $effect(() => {
    if (open) {
      duration = 60;
      windowKey = 'week';
      weekdaysOnly = true;
      workStart = 9;
      workEnd = 18;
    }
  });

  const windowDays = $derived.by(() => {
    switch (windowKey) {
      case 'today': return 0;
      case 'week': return 7;
      case '2weeks': return 14;
      case 'month': return 30;
    }
  });

  let gaps = $derived<FreeGap[]>(
    open
      ? findFreeGaps(events, {
          fromISO: fmtDateISO(new Date()),
          toISO: fmtDateISO(addDays(new Date(), windowDays)),
          minDurationMin: duration,
          workStartHour: workStart,
          workEndHour: workEnd,
          weekdaysOnly,
          limit: 12
        })
      : []
  );

  function close() { open = false; }
  function pick(g: FreeGap) {
    onPick(g.startDate, duration);
    close();
  }
</script>

{#if open}
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="find-time-title"
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={(e) => { if (e.target === e.currentTarget) close(); }}
    onkeydown={(e) => { if (e.key === 'Escape') close(); }}
    tabindex="-1"
  >
    <div class="bg-mantle border border-surface1 rounded-lg shadow-xl w-full max-w-md max-h-[90dvh] overflow-y-auto">
      <header class="flex items-center justify-between px-4 py-3 border-b border-surface1">
        <h3 id="find-time-title" class="text-sm font-semibold text-text">Find time</h3>
        <button onclick={close} aria-label="close" class="text-dim hover:text-text w-6 h-6 flex items-center justify-center">×</button>
      </header>

      <form class="px-4 py-3 space-y-3 text-sm" onsubmit={(e) => e.preventDefault()}>
        <div class="flex items-center gap-2">
          <label for="ft-duration" class="text-xs text-dim w-20">Duration</label>
          <select
            id="ft-duration"
            bind:value={duration}
            class="flex-1 px-2 py-1 bg-surface0 border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary"
          >
            <option value={15}>15 min</option>
            <option value={30}>30 min</option>
            <option value={45}>45 min</option>
            <option value={60}>1 hour</option>
            <option value={90}>90 min</option>
            <option value={120}>2 hours</option>
            <option value={180}>3 hours</option>
            <option value={240}>4 hours</option>
          </select>
        </div>

        <div class="flex items-center gap-2">
          <label for="ft-window" class="text-xs text-dim w-20">Window</label>
          <select
            id="ft-window"
            bind:value={windowKey}
            class="flex-1 px-2 py-1 bg-surface0 border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary"
          >
            <option value="today">Today only</option>
            <option value="week">Next 7 days</option>
            <option value="2weeks">Next 14 days</option>
            <option value="month">Next 30 days</option>
          </select>
        </div>

        <div class="flex items-center gap-2">
          <label for="ft-start" class="text-xs text-dim w-20">Hours</label>
          <input
            id="ft-start"
            type="number"
            min="0"
            max="23"
            bind:value={workStart}
            class="w-16 px-2 py-1 bg-surface0 border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary"
          />
          <span class="text-dim">to</span>
          <input
            aria-label="working hours end"
            type="number"
            min="1"
            max="24"
            bind:value={workEnd}
            class="w-16 px-2 py-1 bg-surface0 border border-surface1 rounded text-text text-sm focus:outline-none focus:border-primary"
          />
          <label class="flex items-center gap-1 text-xs text-dim cursor-pointer select-none ml-auto">
            <input type="checkbox" bind:checked={weekdaysOnly} class="accent-primary" />
            weekdays
          </label>
        </div>
      </form>

      <section class="px-4 pb-4 border-t border-surface1">
        <h4 class="text-[11px] uppercase tracking-wider text-dim my-2">
          {gaps.length === 0 ? 'No gaps' : `First ${gaps.length} gap${gaps.length !== 1 ? 's' : ''}`}
        </h4>
        {#if gaps.length === 0}
          <p class="text-xs text-dim italic py-3">
            No {duration}-minute slot fits in the chosen window. Try a shorter duration,
            a wider window, or extending the working hours.
          </p>
        {:else}
          <ul class="space-y-1 max-h-72 overflow-y-auto">
            {#each gaps as g}
              <li>
                <button
                  type="button"
                  onclick={() => pick(g)}
                  class="w-full flex items-baseline gap-2 px-2 py-1.5 rounded hover:bg-surface0 text-left"
                >
                  <span class="text-xs text-dim w-28 flex-shrink-0">{g.dayLabel}</span>
                  <span class="font-mono text-sm text-text">{g.startLabel}</span>
                  <span class="text-dim">→</span>
                  <span class="font-mono text-sm text-text">{g.endLabel}</span>
                  <span class="ml-auto text-[10px] text-dim">{g.durationMinutes} min</span>
                </button>
              </li>
            {/each}
          </ul>
          <p class="text-[10px] text-dim italic mt-2 px-1">Tasks aren't counted as conflicts — they can be moved.</p>
        {/if}
      </section>
    </div>
  </div>
{/if}
