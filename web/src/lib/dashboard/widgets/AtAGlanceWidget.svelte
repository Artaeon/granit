<script lang="ts">
  import { onMount } from 'svelte';
  import { api, todayISO, fmtDateISO, type Task, type HabitInfo, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // AtAGlanceWidget — single dense row that answers "what's the shape
  // of today?" in four count tiles (due-today, overdue, deadlines-7d,
  // habits-left). Each tile links to the source page on tap.
  //
  // Why four, not five: Prayer used to live here but it's a different
  // altitude — work counts read as "today's pressure", prayer reads
  // as "today's intention". Mixing them muddied the glance. Prayer
  // has its own widget; this row stays focused on what's due.

  let openTasks = $state<Task[] | null>(null);
  let deadlines = $state<Deadline[] | null>(null);
  let habits = $state<HabitInfo[] | null>(null);
  let loaded = $state(false);

  const today = todayISO();

  async function load() {
    const [t, d, h] = await Promise.allSettled([
      api.listTasks({ status: 'open' }),
      api.tryListDeadlines(),
      api.listHabits()
    ]);
    openTasks = t.status === 'fulfilled' ? t.value.tasks : [];
    deadlines = d.status === 'fulfilled' ? (d.value ?? []) : [];
    habits = h.status === 'fulfilled' ? h.value.habits : [];
    loaded = true;
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
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

  let habitsRemaining = $derived.by(() => {
    if (!habits) return null;
    return habits.filter((h) => !h.doneToday).length;
  });

  // Tone math: a 0 stays dim (nothing on fire = good); a positive
  // count picks a critical-tone so the eye lands on what's pressing.
  type Tile = { label: string; value: number | null; href: string; tone: string };
  let tiles = $derived<Tile[]>([
    {
      label: 'Due',
      value: dueToday,
      href: '/tasks?group=due',
      tone: dueToday && dueToday > 0 ? 'primary' : 'dim'
    },
    {
      label: 'Overdue',
      value: overdue,
      href: '/tasks?group=due',
      tone: overdue && overdue > 0 ? 'error' : 'dim'
    },
    {
      label: 'Deadlines · 7d',
      value: weekDeadlines,
      href: '/deadlines',
      tone: weekDeadlines && weekDeadlines > 0 ? 'warning' : 'dim'
    },
    {
      label: 'Habits left',
      value: habitsRemaining,
      href: '/habits',
      tone: habitsRemaining && habitsRemaining > 0 ? 'info' : 'success'
    }
  ]);

  function display(v: number | null): string {
    if (v === null) return '—';
    return String(v);
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg px-3 py-2">
  <div class="grid grid-cols-4 gap-1.5">
    {#each tiles as t}
      <a
        href={t.href}
        class="block px-2 py-1.5 rounded bg-mantle hover:bg-black/60 border-l-2 transition-colors"
        style="border-left-color: var(--color-{t.tone});"
        title={t.label}
      >
        <div class="flex items-baseline gap-1.5">
          <span class="text-xl font-semibold text-text tabular-nums leading-none">{display(t.value)}</span>
          <span class="text-[10px] uppercase tracking-wider text-dim truncate">{t.label}</span>
        </div>
      </a>
    {/each}
  </div>
</section>
