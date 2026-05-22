<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, fmtDateISO, type Task, type HabitInfo, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';

  // AtAGlanceWidget — the dashboard's headline row. Answers "what's
  // the shape of today?" in four count tiles with strong visual
  // hierarchy:
  //
  //   - Numbers are BIG (text-3xl/4xl) so the eye scans them in one
  //     sweep without having to read the label first.
  //   - Number colour matches the tile's tone — overdue = error red,
  //     due = primary, deadlines = warning, habits = info. The
  //     glance becomes pre-attentive: red number = react now,
  //     dimmed number = nothing here.
  //   - Order is urgency-descending: Overdue (most urgent) → Due
  //     today → Deadlines (week) → Habits. The eye reads left-to-
  //     right; the leftmost slot is the most-important seat.
  //   - All-clear state ("Wide open. Focus on your one thing.")
  //     replaces the strip when every count is zero — a row of
  //     four 0s is visual noise that hides the more useful "today
  //     has no fire on it" signal.

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

  const reload = createCoalescedReload(load, 600);

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed') reload.trigger();
      else if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') reload.trigger();
      else if (ev.type === 'state.changed' && ev.path?.startsWith('.granit/habits/')) reload.trigger();
      else if (ev.type === 'note.changed') reload.trigger();
    });
  });

  onDestroy(() => reload.cancel());

  // ----- Counts -----

  // Overdue: open task with a past due date. Snoozed tasks ride on
  // their snoozedUntil but we count them as overdue here too — they're
  // still on the user's plate, just hidden by default in the list view.
  // Highest urgency, lands in slot 1.
  let overdue = $derived.by(() => {
    if (!openTasks) return null;
    return openTasks.filter((t) => !!t.dueDate && t.dueDate < today).length;
  });

  // Tasks due today: due_date === today OR scheduled_start starts today.
  let dueToday = $derived.by(() => {
    if (!openTasks) return null;
    return openTasks.filter((t) => {
      if (t.dueDate === today) return true;
      if (t.scheduledStart && t.scheduledStart.slice(0, 10) === today) return true;
      return false;
    }).length;
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

  // Tile shape. `tone` drives both the number colour and the left-rail
  // accent so the eye sees "this is the urgent one" without parsing
  // the number itself. Caption reads as a tight one-liner under the
  // label so the tile communicates the period at a glance ("Due today
  // · tasks" vs "Deadlines · next 7d").
  type Tile = {
    label: string;
    caption: string;
    value: number | null;
    href: string;
    tone: 'error' | 'primary' | 'warning' | 'info' | 'dim';
    icon: string;
  };
  let tiles = $derived<Tile[]>([
    {
      label: 'Overdue',
      caption: 'tasks',
      value: overdue,
      href: '/tasks?group=due',
      tone: overdue && overdue > 0 ? 'error' : 'dim',
      icon: '⚠'
    },
    {
      label: 'Due today',
      caption: 'tasks',
      value: dueToday,
      href: '/tasks?group=due',
      tone: dueToday && dueToday > 0 ? 'primary' : 'dim',
      icon: '◉'
    },
    {
      label: 'Deadlines',
      caption: 'next 7d',
      value: weekDeadlines,
      href: '/deadlines',
      tone: weekDeadlines && weekDeadlines > 0 ? 'warning' : 'dim',
      icon: '⚑'
    },
    {
      label: 'Habits',
      caption: 'left today',
      value: habitsRemaining,
      href: '/habits',
      tone: habitsRemaining && habitsRemaining > 0 ? 'info' : 'dim',
      icon: '✓'
    }
  ]);

  // All-clear: every count is loaded AND zero. Renders a single
  // calming row instead of four `0`s, which read as visual noise
  // and undersell "nothing burning today" as a win.
  let allClear = $derived(
    loaded &&
      (overdue ?? 0) === 0 &&
      (dueToday ?? 0) === 0 &&
      (weekDeadlines ?? 0) === 0 &&
      (habitsRemaining ?? 0) === 0
  );

  function display(v: number | null): string {
    if (v === null) return '—';
    return String(v);
  }

  // Tailwind doesn't compile arbitrary `text-{var}` patterns, so we
  // map tone → fixed class string. Same trick the rest of the app
  // uses (TaskCard, NowWidget, etc.).
  const toneText: Record<Tile['tone'], string> = {
    error: 'text-error',
    primary: 'text-primary',
    warning: 'text-warning',
    info: 'text-info',
    dim: 'text-dim'
  };
  const toneAccent: Record<Tile['tone'], string> = {
    error: 'bg-error',
    primary: 'bg-primary',
    warning: 'bg-warning',
    info: 'bg-info',
    dim: 'bg-surface1'
  };
</script>

{#if allClear}
  <!-- All-clear state: every count is zero. Reads as a win, not
       as four empty tiles. Subtle success tint without being
       celebratory-cheesy — granit isn't a gamified app. -->
  <section class="bg-surface0 border border-surface1 rounded-lg px-4 py-3 flex items-center gap-3">
    <span class="text-success text-xl leading-none" aria-hidden="true">✓</span>
    <div class="flex-1 min-w-0">
      <p class="text-sm text-text font-medium">Wide open.</p>
      <p class="text-xs text-dim">Nothing overdue, due, or unhabited. Focus on your one thing.</p>
    </div>
    <a href="/morning" class="text-xs text-secondary hover:underline flex-shrink-0">plan →</a>
  </section>
{:else}
  <section class="bg-surface0 border border-surface1 rounded-lg p-2">
    <div class="grid grid-cols-4 gap-2">
      {#each tiles as t (t.label)}
        {@const value = t.value ?? 0}
        {@const active = loaded && value > 0}
        <a
          href={t.href}
          class="group relative block px-2.5 py-2 rounded bg-mantle hover:bg-black/40 transition-colors overflow-hidden"
          title="{value} {t.label.toLowerCase()} {t.caption}"
        >
          <!-- Tone accent rail on the left. Heavier (3px) when the
               count is non-zero so urgency reads at a glance; thin
               and dim (2px on surface1) when count is zero so the
               tile recedes. -->
          <span
            class="absolute left-0 top-1.5 bottom-1.5 rounded-full {active ? toneAccent[t.tone] + ' w-[3px]' : 'bg-surface1 w-[2px]'}"
            aria-hidden="true"
          ></span>
          <div class="pl-2">
            {#if !loaded && t.value === null}
              <span
                class="inline-block h-7 w-8 rounded bg-surface1 animate-pulse"
                aria-hidden="true"
              ></span>
              <span class="block text-[10px] uppercase tracking-wider text-dim mt-0.5">{t.label}</span>
            {:else}
              <!-- The number is the headline — text-3xl on mobile,
                   text-4xl on sm+ so it dominates the tile. Tabular
                   nums keeps a 2-digit "10" the same width as "11"
                   so the layout doesn't twitch when a task ticks. -->
              <div class="flex items-baseline gap-1.5">
                <span class="text-3xl sm:text-4xl font-bold {active ? toneText[t.tone] : 'text-dim'} tabular-nums leading-none">
                  {display(t.value)}
                </span>
                <span class="text-xs {active ? toneText[t.tone] : 'text-dim'} opacity-70" aria-hidden="true">{t.icon}</span>
              </div>
              <div class="mt-1 leading-tight">
                <span class="block text-[11px] font-medium {active ? 'text-text' : 'text-dim'}">{t.label}</span>
                <span class="block text-[10px] text-dim">{t.caption}</span>
              </div>
            {/if}
          </div>
        </a>
      {/each}
    </div>
  </section>
{/if}
