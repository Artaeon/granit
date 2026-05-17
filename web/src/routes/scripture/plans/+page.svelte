<script lang="ts">
  // /scripture/plans — reading-plan catalogue + active-plan dashboard.
  //
  // Three jobs in one page:
  //   1. List bundled plans (M'Cheyne, chrono NT, 90-day NT) with a
  //      Start button each.
  //   2. Render any active plans with today's day-of-plan + the four
  //      passages as clickable chips that jump to the bible reader.
  //   3. Offer a "Schedule today's reading" button per active plan
  //      that drops a single task on today's daily note (via the
  //      backend handler — same pattern as scheduleVerseReview).
  //
  // The /scripture page itself stays untouched per the task brief —
  // this page is the new home for the structured-reading surface.
  // Cross-page navigation: chip clicks go to
  // /scripture?book=<Name>&chapter=<N> (the existing scripture page
  // doesn't currently read those params, but the URL is the contract
  // the user asked for; wiring up the page-side hydration is a
  // follow-up).

  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    api,
    type LectionaryPlan,
    type ActiveLectionaryPlan
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import PageHeader from '$lib/components/PageHeader.svelte';

  let plans = $state<LectionaryPlan[]>([]);
  let active = $state<ActiveLectionaryPlan[]>([]);
  let loading = $state(false);
  let busyPlanID = $state<string | null>(null); // disables buttons while a mutation is in flight

  async function loadAll() {
    loading = true;
    try {
      const [pRes, aRes] = await Promise.all([
        api.lectionaryPlans(),
        api.lectionaryActivePlans()
      ]);
      plans = pRes.plans;
      active = aRes.active;
    } catch (e) {
      toast.error('failed to load plans: ' + errorMessage(e));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadAll();
    // Refresh when state changes from another tab (start/stop hits
    // .granit/lectionary-state.json — see broadcastLectionaryChanged).
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/lectionary-state.json') {
        loadAll();
      }
    });
  });

  // Map planId → active record so the card view can show "Active · day X"
  // inline without a per-card filter call.
  let activeByID = $derived.by(() => {
    const m: Record<string, ActiveLectionaryPlan> = {};
    for (const a of active) m[a.planId] = a;
    return m;
  });

  async function startPlan(p: LectionaryPlan) {
    if (busyPlanID) return;
    busyPlanID = p.id;
    try {
      const r = await api.lectionaryStartPlan(p.id);
      active = r.active;
      toast.success(`${p.name} started — day 1 of ${p.lengthDays}.`);
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    } finally {
      busyPlanID = null;
    }
  }

  async function stopPlan(p: LectionaryPlan) {
    if (busyPlanID) return;
    if (!confirm(`Stop ${p.name}? Your progress isn't lost — you can start again any time.`)) return;
    busyPlanID = p.id;
    try {
      const r = await api.lectionaryStopPlan(p.id);
      active = r.active;
      toast.info(`${p.name} stopped.`);
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    } finally {
      busyPlanID = null;
    }
  }

  async function scheduleToday(a: ActiveLectionaryPlan) {
    if (busyPlanID) return;
    busyPlanID = a.planId;
    try {
      const r = await api.lectionaryScheduleToday(a.planId);
      toast.success(`Day ${r.day} scheduled for 09:00 today.`);
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    } finally {
      busyPlanID = null;
    }
  }

  // ── Passage chip → bible reader ────────────────────────────────────
  // Citations come from the backend as "Gen 1" / "1 Cor 13" / "Phlm".
  // We split into book + chapter; chapter defaults to 1 (single-chapter
  // books like Obadiah / Jude / Philemon emit just the book name).
  function parsePassage(passage: string): { book: string; chapter: number } {
    const m = passage.match(/^(.+?)\s+(\d+)\s*$/);
    if (m) return { book: m[1].trim(), chapter: parseInt(m[2], 10) };
    return { book: passage.trim(), chapter: 1 };
  }

  function openPassage(passage: string) {
    const ref = parsePassage(passage);
    const qs = new URLSearchParams({ book: ref.book, chapter: String(ref.chapter) });
    goto(`/scripture?${qs.toString()}`);
  }
</script>

<svelte:head>
  <title>Reading plans · Granit</title>
</svelte:head>

<div class="h-full overflow-y-auto">
  <div class="max-w-4xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader
      title="Reading plans"
      subtitle="Structured Bible reading — pick one, then daily readings appear on the calendar."
    />

    {#if loading && plans.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {/if}

    <!-- ── Active plans ──────────────────────────────────────────── -->
    {#if active.length > 0}
      <section class="mb-6">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-dim mb-2">
          Active {active.length > 1 ? `(${active.length})` : ''}
        </h2>
        <div class="space-y-2">
          {#each active as a (a.planId)}
            <div class="rounded border border-surface1 bg-surface0 p-3">
              <div class="flex flex-wrap items-baseline justify-between gap-2 mb-2">
                <div class="min-w-0">
                  <div class="font-medium text-text">{a.planName}</div>
                  <div class="text-[11px] text-dim">
                    Day {a.dayOfPlan} of {a.lengthDays} ·
                    started {new Date(a.startedAt).toLocaleDateString()}
                    {#if a.finished} · <span class="text-success">finished</span>{/if}
                  </div>
                </div>
                <div class="flex items-center gap-2 flex-wrap">
                  {#if !a.finished && a.todayPassages.length > 0}
                    <button
                      class="px-2 py-1 text-[11px] rounded border border-surface2 hover:bg-surface1 text-subtext disabled:opacity-50"
                      disabled={busyPlanID === a.planId}
                      onclick={() => scheduleToday(a)}
                    >Schedule today</button>
                  {/if}
                  <button
                    class="px-2 py-1 text-[11px] rounded border border-surface2 hover:bg-surface1 text-subtext disabled:opacity-50"
                    disabled={busyPlanID === a.planId}
                    onclick={() => {
                      const p = plans.find((pl) => pl.id === a.planId);
                      if (p) stopPlan(p);
                    }}
                  >Stop</button>
                </div>
              </div>

              {#if a.todayPassages.length > 0}
                <div class="flex flex-wrap gap-1.5">
                  {#each a.todayPassages as ref (ref)}
                    <button
                      class="px-2 py-0.5 text-[12px] rounded bg-surface1 hover:bg-surface2 text-text border border-surface2"
                      onclick={() => openPassage(ref)}
                      title="Open in bible reader"
                    >{ref}</button>
                  {/each}
                </div>
              {:else if a.finished}
                <div class="text-[12px] text-dim italic">
                  You've finished this plan. Start again to begin a fresh cycle.
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- ── Catalogue ─────────────────────────────────────────────── -->
    <section>
      <h2 class="text-xs font-semibold uppercase tracking-wide text-dim mb-2">
        Catalogue ({plans.length})
      </h2>
      <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {#each plans as p (p.id)}
          {@const a = activeByID[p.id]}
          <div class="rounded border border-surface1 bg-surface0 p-3 flex flex-col">
            <div class="font-medium text-text">{p.name}</div>
            <div class="text-[11px] text-dim mt-0.5">{p.lengthDays} days</div>
            <p class="text-[12px] text-subtext mt-2 flex-1">{p.description}</p>
            <div class="mt-3 flex items-center gap-2">
              {#if a}
                <span class="text-[11px] text-success">
                  Active · day {a.dayOfPlan}/{p.lengthDays}
                </span>
                <button
                  class="ml-auto px-2 py-1 text-[11px] rounded border border-surface2 hover:bg-surface1 text-subtext disabled:opacity-50"
                  disabled={busyPlanID === p.id}
                  onclick={() => startPlan(p)}
                  title="Restart from day 1"
                >Restart</button>
                <button
                  class="px-2 py-1 text-[11px] rounded border border-surface2 hover:bg-surface1 text-subtext disabled:opacity-50"
                  disabled={busyPlanID === p.id}
                  onclick={() => stopPlan(p)}
                >Stop</button>
              {:else}
                <button
                  class="ml-auto px-2 py-1 text-[11px] rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-50"
                  disabled={busyPlanID === p.id}
                  onclick={() => startPlan(p)}
                >Start</button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </section>

    <p class="mt-6 text-[11px] text-dim">
      Plans are stateful per vault. Restarting a plan resets your day counter to 1; the
      backing scripture data is the same WEB translation used elsewhere in granit.
    </p>
  </div>
</div>
