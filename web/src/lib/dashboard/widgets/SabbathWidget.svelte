<script lang="ts">
  import { onMount } from 'svelte';
  import { sabbath, sabbathSchedule, sabbathMinutesRemaining, DAY_LABELS, type SabbathSchedule } from '$lib/stores/sabbath';

  // SabbathWidget — three states, single tile:
  //
  //   ACTIVE     — sabbath is on right now. Show remaining time and a
  //                quiet "rest until …" line. The store handles the
  //                math; we just render it.
  //   SCHEDULED  — sabbath is configured but not active. Show "next
  //                sabbath" + countdown so the user feels the rhythm
  //                approaching.
  //   OFF        — no manual flag, no schedule. Quiet line + "set up"
  //                link to /sabbath. We still render so the tile isn't
  //                a hole on a configured dashboard.
  //
  // No new endpoints — everything reads from the sabbath store, which
  // already syncs the schedule across devices via /api/v1/sabbath.

  // Re-tick once a minute so the countdown stays honest without a
  // full reload. visibilitychange in the store also re-evaluates on
  // focus, so this only matters when the tab is foreground for long
  // stretches.
  let now = $state(Date.now());
  onMount(() => {
    const id = setInterval(() => { now = Date.now(); }, 60_000);
    return () => clearInterval(id);
  });

  // Next-sabbath math — mirrors the schedule-window logic in the
  // store (scheduleWindow), but looks FORWARD instead of backward.
  // We don't export the store's helper because this is the only
  // caller that needs the future direction; cheaper to inline 12
  // lines than to expand the public surface.
  function nextSabbathStart(s: SabbathSchedule, at: Date): Date | null {
    if (!s.enabled || s.durationMinutes <= 0) return null;
    for (let daysAhead = 0; daysAhead < 8; daysAhead++) {
      const cand = new Date(at.getFullYear(), at.getMonth(), at.getDate() + daysAhead,
        s.startHour, s.startMinute, 0, 0);
      if (cand.getDay() !== s.dayOfWeek) continue;
      if (cand.getTime() <= at.getTime()) continue;
      return cand;
    }
    return null;
  }

  function fmtMinutes(mins: number): string {
    if (mins <= 0) return 'less than a minute';
    if (mins < 60) return `${mins} min`;
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (h < 24) return m === 0 ? `${h}h` : `${h}h ${m}m`;
    const d = Math.floor(h / 24);
    const rh = h % 24;
    return rh === 0 ? `${d}d` : `${d}d ${rh}h`;
  }

  let nextStart = $derived.by(() => {
    void now;
    return nextSabbathStart($sabbathSchedule, new Date());
  });
  let nextLabel = $derived.by(() => {
    if (!nextStart) return '';
    const mins = Math.max(0, Math.round((nextStart.getTime() - now) / 60_000));
    return fmtMinutes(mins);
  });
  let nextDay = $derived(nextStart ? DAY_LABELS[nextStart.getDay()] : '');

  let remaining = $derived.by(() => {
    void now;
    return sabbathMinutesRemaining();
  });
</script>

<section class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3 flex flex-col h-full">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs text-dim font-semibold">Sabbath</h2>
    <a href="/sabbath" class="text-xs text-secondary hover:underline">open →</a>
  </div>

  {#if $sabbath}
    <!-- Active — quiet, green accent, no buttons. The user is in rest;
         the dashboard tile shouldn't pull them back into work. -->
    <div class="rounded p-2.5 flex-1" style="background: color-mix(in srgb, var(--color-green, #a6e3a1) 14%, transparent); border-left: 3px solid var(--color-green, #a6e3a1)">
      <div class="text-[10px] uppercase tracking-wider text-dim">resting</div>
      <div class="text-base text-text font-medium mt-0.5">{fmtMinutes(remaining)} remaining</div>
      <div class="text-[11px] text-dim mt-1">work modules paused · scripture, prayer, notes stay</div>
    </div>
  {:else if $sabbathSchedule.enabled && nextStart}
    <!-- Scheduled — countdown to the next sabbath start. Reads as
         anticipation, not pressure. -->
    <div class="flex-1">
      <div class="text-[10px] uppercase tracking-wider text-dim">next sabbath</div>
      <div class="text-base text-text font-medium mt-0.5">{nextDay}</div>
      <div class="text-xs text-subtext mt-0.5">in {nextLabel}</div>
      <div class="text-[11px] text-dim mt-1.5">
        {DAY_LABELS[$sabbathSchedule.dayOfWeek]} {String($sabbathSchedule.startHour).padStart(2, '0')}:{String($sabbathSchedule.startMinute).padStart(2, '0')}
        · {fmtMinutes($sabbathSchedule.durationMinutes)}
      </div>
    </div>
  {:else}
    <!-- Off — quietly suggest setup, never demand. -->
    <div class="flex-1">
      <p class="text-sm text-dim">No sabbath scheduled.</p>
      <a href="/sabbath" class="text-xs text-secondary hover:underline mt-1 inline-block">set a weekly day of rest →</a>
    </div>
  {/if}
</section>
