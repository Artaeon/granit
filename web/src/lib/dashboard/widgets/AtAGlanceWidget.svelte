<script lang="ts">
  import { onMount } from 'svelte';
  import { api, todayISO, fmtDateISO, type Task, type HabitInfo, type PrayerIntention, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // AtAGlanceWidget — compact daily overview row. Single span-2 widget
  // designed to be the first thing the user reads on the dashboard:
  // "what's the shape of today?" — answered in five tiles (tasks due,
  // overdue, this-week deadlines, active prayer, habits remaining)
  // with each tile linking to the source page when tapped.
  //
  // Defensive loading: every endpoint we hit can fail without taking
  // the widget down (a disabled module, a 404, network blip) — each
  // count falls back to '—' so the widget always renders the layout.

  let openTasks = $state<Task[] | null>(null);
  let deadlines = $state<Deadline[] | null>(null);
  let intentions = $state<PrayerIntention[] | null>(null);
  let habits = $state<HabitInfo[] | null>(null);
  let loaded = $state(false);

  const today = todayISO();

  async function load() {
    // Promise.allSettled — we don't want one slow / failing endpoint
    // to gate the others. Each .value is read defensively below.
    const [t, d, p, h] = await Promise.allSettled([
      api.listTasks({ status: 'open' }),
      api.tryListDeadlines(),
      api.listPrayer(),
      api.listHabits()
    ]);
    openTasks = t.status === 'fulfilled' ? t.value.tasks : [];
    deadlines = d.status === 'fulfilled' ? (d.value ?? []) : [];
    intentions = p.status === 'fulfilled' ? p.value.intentions : [];
    habits = h.status === 'fulfilled' ? h.value.habits : [];
    loaded = true;
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/prayer/intentions.json') load();
      if (ev.type === 'state.changed' && ev.path?.startsWith('.granit/habits/')) load();
      if (ev.type === 'note.changed') load();
    });
  });

  // ----- Counts -----

  // Tasks due today: due_date === today OR scheduled_start starts today.
  let dueToday = $derived.by(() => {
    if (!openTasks) return null;
    return openTasks.filter((t) => {
      if (t.dueDate === today) return true;
      if (t.scheduledStart && t.scheduledStart.slice(0, 10) === today) return true;
      return false;
    }).length;
  });

  // Overdue: open task with a past due date. Snoozed tasks ride on
  // their snoozedUntil but we count them as overdue here too — they're
  // still on the user's plate, just hidden by default in the list view.
  let overdue = $derived.by(() => {
    if (!openTasks) return null;
    return openTasks.filter((t) => !!t.dueDate && t.dueDate < today).length;
  });

  // Active deadlines in the next 7 days. We hide met + cancelled —
  // the count is "what should I worry about right now".
  let weekDeadlines = $derived.by(() => {
    if (!deadlines) return null;
    const cutoff = new Date();
    cutoff.setDate(cutoff.getDate() + 7);
    const cutoffISO = fmtDateISO(cutoff);
    return deadlines.filter(
      (d) => d.status !== 'met' && d.status !== 'cancelled' && d.date <= cutoffISO
    ).length;
  });

  let activePrayer = $derived.by(() => {
    if (!intentions) return null;
    return intentions.filter((p) => p.status === 'praying').length;
  });

  let habitsRemaining = $derived.by(() => {
    if (!habits) return null;
    return habits.filter((h) => !h.doneToday).length;
  });

  // Tile defaults: a count of 0 should still render the tile (so the
  // user sees "nothing on fire" as a positive signal), but we tint it
  // dim so a critical-state count visually wins.
  type Tile = { label: string; value: number | null; href: string; icon: string; tone: string };
  let tiles = $derived<Tile[]>([
    {
      label: 'Due today',
      value: dueToday,
      href: '/tasks?group=due',
      icon: '✔',
      tone: dueToday && dueToday > 0 ? 'primary' : 'dim'
    },
    {
      label: 'Overdue',
      value: overdue,
      href: '/tasks?group=due',
      icon: '!',
      tone: overdue && overdue > 0 ? 'error' : 'dim'
    },
    {
      label: 'Deadlines · 7d',
      value: weekDeadlines,
      href: '/deadlines',
      icon: '⏰',
      tone: weekDeadlines && weekDeadlines > 0 ? 'warning' : 'dim'
    },
    {
      label: 'Praying',
      value: activePrayer,
      href: '/prayer',
      icon: '🙏',
      tone: activePrayer && activePrayer > 0 ? 'secondary' : 'dim'
    },
    {
      label: 'Habits left',
      value: habitsRemaining,
      href: '/habits',
      icon: '◇',
      tone: habitsRemaining && habitsRemaining > 0 ? 'info' : 'success'
    }
  ]);

  function display(v: number | null): string {
    if (v === null) return '—';
    return String(v);
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Today at a glance</h2>
    {#if !loaded}
      <span class="text-[11px] text-dim italic">loading…</span>
    {/if}
  </div>
  <div class="grid grid-cols-2 sm:grid-cols-5 gap-2">
    {#each tiles as t}
      <a
        href={t.href}
        class="block px-2.5 py-2 rounded bg-mantle hover:bg-black/60 border-l-2 transition-colors"
        style="border-left-color: var(--color-{t.tone});"
        title={t.label}
      >
        <div class="flex items-baseline gap-1.5">
          <span class="text-base flex-shrink-0" style="color: var(--color-{t.tone});">{t.icon}</span>
          <span class="text-2xl font-semibold text-text tabular-nums leading-none">{display(t.value)}</span>
        </div>
        <div class="text-[11px] text-dim mt-1 truncate">{t.label}</div>
      </a>
    {/each}
  </div>
</section>
