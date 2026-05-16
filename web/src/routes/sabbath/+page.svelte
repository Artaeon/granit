<script lang="ts">
  import { onMount } from 'svelte';
  import {
    sabbath,
    sabbathSchedule,
    sabbathMinutesRemaining,
    DAY_LABELS,
    updateSchedule,
    type SabbathSchedule
  } from '$lib/stores/sabbath';
  import { api } from '$lib/api';

  // /sabbath — a contemplative landing surface. Two roles:
  //   1. While sabbath is active: lead with verse + time remaining,
  //      offer a single prompt for the day, and quietly witness past
  //      observances (log). No dashboard chrome.
  //   2. While sabbath is NOT active: explain what the toggle does,
  //      let the user configure the auto-schedule including
  //      sundown-to-sundown windows, and offer a "begin now" button.

  let now = $state(Date.now());
  onMount(() => {
    const id = setInterval(() => { now = Date.now(); }, 60_000);
    return () => clearInterval(id);
  });

  let minutesRemaining = $derived.by(() => {
    void now; // depend on tick
    return sabbathMinutesRemaining();
  });

  function fmtRemaining(mins: number): string {
    if (mins <= 0) return 'less than a minute';
    if (mins < 60) return `${mins} minute${mins === 1 ? '' : 's'}`;
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (m === 0) return `${h} hour${h === 1 ? '' : 's'}`;
    return `${h}h ${m}m`;
  }

  // ── Schedule form bindings ────────────────────────────────────
  // The store update flows server-side via updateSchedule(); we
  // bind to derived locals so the inputs read the current store
  // value but writes go through the setter that triggers PUT.
  let sched = $derived($sabbathSchedule);

  function patch(p: Partial<SabbathSchedule>) {
    updateSchedule({ ...sched, ...p });
  }

  // Sundown-to-sundown preset — the configurable that matters most.
  // Friday 18:00 + 24h captures the Jewish observance; Saturday 18:00
  // + 24h captures Saturday night → Sunday night. Users can tweak.
  function applyPreset(name: 'midnight-saturday' | 'midnight-sunday' | 'sundown-friday') {
    if (name === 'midnight-saturday') {
      patch({ enabled: true, dayOfWeek: 6, startHour: 0, startMinute: 0, durationMinutes: 1440 });
    } else if (name === 'midnight-sunday') {
      patch({ enabled: true, dayOfWeek: 0, startHour: 0, startMinute: 0, durationMinutes: 1440 });
    } else if (name === 'sundown-friday') {
      patch({ enabled: true, dayOfWeek: 5, startHour: 18, startMinute: 0, durationMinutes: 1440 });
    }
  }

  // Hours preset for the duration dropdown — the common cases. Users
  // who want something off-grid can edit numerically (future polish).
  const DURATION_PRESETS: { label: string; mins: number }[] = [
    { label: '24 hours', mins: 1440 },
    { label: '12 hours', mins: 720 },
    { label: '36 hours', mins: 36 * 60 },
    { label: '48 hours', mins: 2880 }
  ];

  // ── Verses ────────────────────────────────────────────────────
  // Rotates with day-of-month so the user gets variety without true
  // randomness (same date → same verse).
  const SABBATH_VERSES: { ref: string; text: string }[] = [
    { ref: 'Mark 2:27', text: 'And he said unto them, The sabbath was made for man, and not man for the sabbath.' },
    { ref: 'Psalm 46:10', text: 'Be still, and know that I am God: I will be exalted among the heathen, I will be exalted in the earth.' },
    { ref: 'Matthew 11:28', text: 'Come unto me, all ye that labour and are heavy laden, and I will give you rest.' },
    { ref: 'Exodus 20:8-10', text: 'Remember the sabbath day, to keep it holy. Six days shalt thou labour, and do all thy work: but the seventh day is the sabbath of the Lord thy God: in it thou shalt not do any work.' },
    { ref: 'Hebrews 4:9-10', text: 'There remaineth therefore a rest to the people of God. For he that is entered into his rest, he also hath ceased from his own works, as God did from his.' },
    { ref: 'Isaiah 30:15', text: 'In returning and rest shall ye be saved; in quietness and in confidence shall be your strength.' }
  ];
  let verse = $derived.by(() => {
    void now;
    const d = new Date();
    return SABBATH_VERSES[(d.getDate() + d.getMonth()) % SABBATH_VERSES.length];
  });

  // ── Prompts ───────────────────────────────────────────────────
  // Rotating examen-style cue. The user is meant to settle, not
  // answer in a form field — these are reading material, not a quiz.
  const PROMPTS: string[] = [
    'What is one thing this week that I can release into rest — a worry, a striving, an outcome I do not control? Carry it gently into prayer, and let it stay there.',
    'Where did I see God\'s hand this week, even in small things? Name three, slowly.',
    'What part of me have I been pushing too hard? Offer it back to the Father who made it.',
    'Where have I sought to be seen this week — and where, instead, did I simply serve? Give thanks for both, and let go of the first.',
    'For what am I most grateful today? Stay with it long enough that it stops being a list and becomes a prayer.',
    'What is one thing I have been carrying that does not belong to me? Set it down at the door of the day of rest.'
  ];
  let prompt = $derived.by(() => {
    void now;
    const d = new Date();
    return PROMPTS[(d.getDate() + d.getMonth() * 3) % PROMPTS.length];
  });

  // ── Observation log ───────────────────────────────────────────
  // Most-recent-first list of {begin, end} events. Surfaced as a
  // quiet witness. NEVER a streak. NEVER a percentage. NEVER an
  // achievement. If you find yourself tempted to add a number that
  // goes up, re-read feedback-life-tree-not-gamified.
  type LogEntry = { at: string; event: 'begin' | 'end' };
  let logEntries = $state<LogEntry[]>([]);
  let logLoading = $state(false);
  let logError = $state('');

  async function loadLog() {
    logLoading = true;
    logError = '';
    try {
      const res = await api.getSabbathLog();
      logEntries = res.entries;
    } catch (e) {
      logError = e instanceof Error ? e.message : 'failed to load log';
    } finally {
      logLoading = false;
    }
  }

  onMount(() => { loadLog(); });

  // Pair begin→end into "observance" rows for display. A naked
  // "begin" with no matching "end" yet (currently in sabbath) is
  // labelled "in progress"; a stray "end" without "begin" is
  // dropped (defensive against log corruption).
  type Observance = { beganAt: Date; endedAt: Date | null };
  let observances = $derived.by(() => {
    const items: Observance[] = [];
    // logEntries is newest-first; we iterate oldest-first to pair.
    const oldestFirst = [...logEntries].reverse();
    let openBegin: Date | null = null;
    for (const e of oldestFirst) {
      if (e.event === 'begin') {
        if (openBegin) items.push({ beganAt: openBegin, endedAt: null });
        openBegin = new Date(e.at);
      } else if (e.event === 'end' && openBegin) {
        items.push({ beganAt: openBegin, endedAt: new Date(e.at) });
        openBegin = null;
      }
    }
    if (openBegin) items.push({ beganAt: openBegin, endedAt: null });
    // Show most recent first, cap at 12.
    return items.reverse().slice(0, 12);
  });

  function fmtDateShort(d: Date): string {
    return d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });
  }
  function fmtTimeShort(d: Date): string {
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }
</script>

<div class="h-full overflow-y-auto bg-mantle">
  <div class="max-w-2xl mx-auto px-5 py-10 sm:py-16">
    {#if $sabbath}
      <!-- ACTIVE STATE — the user is in sabbath. -->
      <header class="text-center mb-10">
        <div class="inline-flex items-center justify-center w-16 h-16 rounded-full bg-surface0 text-success mb-4">
          <svg viewBox="0 0 24 24" class="w-8 h-8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
          </svg>
        </div>
        <h1 class="text-3xl font-light text-text">Sabbath</h1>
        <p class="text-sm text-dim mt-2">a day made for you, not the other way around</p>
      </header>

      <blockquote class="text-center my-12 px-4">
        <p class="text-lg sm:text-xl text-text leading-relaxed font-serif italic">
          {verse.text}
        </p>
        <footer class="mt-3 text-sm text-secondary">— {verse.ref}</footer>
      </blockquote>

      <section class="bg-surface0 border border-surface1 rounded-lg p-5 my-8 text-center">
        <p class="text-sm text-subtext">
          Rest until <span class="text-text font-medium">{fmtRemaining(minutesRemaining)}</span> from now.
        </p>
        <p class="text-xs text-dim mt-2">
          Tasks, finance, projects, agents, and other work modules are paused. Scripture, prayer, and notes remain.
        </p>
      </section>

      <section class="my-10 px-2">
        <h2 class="text-xs uppercase tracking-wider text-dim mb-3">A prompt for the day</h2>
        <p class="text-base text-subtext leading-relaxed">{prompt}</p>
        <div class="mt-5 flex flex-wrap gap-2 text-sm">
          <a href="/prayer" class="px-3 py-1.5 rounded bg-surface1 text-primary border border-surface2 hover:bg-surface2">→ /prayer</a>
          <a href="/jots" class="px-3 py-1.5 rounded bg-surface0 text-subtext border border-surface1 hover:bg-surface1">→ /jots</a>
          <a href="/scripture" class="px-3 py-1.5 rounded bg-surface0 text-subtext border border-surface1 hover:bg-surface1">→ /scripture</a>
        </div>
      </section>

      <!-- Observance log — quiet witness. No streak. No score.
           Sabbaths kept, plainly listed. The point is to see the
           pattern, not to win it. -->
      {#if observances.length > 0}
        <section class="my-12">
          <h2 class="text-xs uppercase tracking-wider text-dim mb-3">Past sabbaths observed</h2>
          <ul class="text-sm text-subtext space-y-1.5">
            {#each observances as o}
              <li class="flex items-baseline gap-3">
                <span class="text-text font-mono text-[12px] w-28">{fmtDateShort(o.beganAt)}</span>
                <span class="text-dim text-[11px]">
                  {fmtTimeShort(o.beganAt)}
                  {#if o.endedAt}
                    → {fmtTimeShort(o.endedAt)}
                  {:else}
                    → <span class="text-success">in progress</span>
                  {/if}
                </span>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <div class="mt-16 text-center">
        <button
          type="button"
          onclick={() => sabbath.disable()}
          class="text-xs text-dim hover:text-text underline"
        >exit sabbath mode</button>
      </div>
    {:else}
      <!-- IDLE STATE — sabbath is off. -->
      <header class="mb-8">
        <h1 class="text-2xl font-semibold text-text">Sabbath mode</h1>
        <p class="text-sm text-dim mt-2 max-w-prose">
          A weekly day of rest. While sabbath is on, work modules
          (tasks, finance, deadlines, agents, …) hide from the rail
          and the server pauses AI features, chat, agent runs, and
          push notifications. Scripture, prayer, and notes stay.
          Mark 2:27.
        </p>
      </header>

      <button
        type="button"
        onclick={() => { sabbath.enable(); }}
        class="w-full sm:w-auto px-5 py-2.5 rounded bg-primary text-on-primary font-medium hover:opacity-90"
      >Begin sabbath now</button>
      <p class="text-xs text-dim mt-2 max-w-prose">
        Manual "begin now" stays on this device only. To gate the server (push, agents,
        AI) for the day of rest, enable the auto-schedule below — it syncs across devices.
      </p>

      <section class="mt-10 bg-surface0 border border-surface1 rounded-lg p-5">
        <header class="flex items-baseline gap-2 mb-4">
          <h2 class="text-base font-medium text-text">Auto-schedule</h2>
          <span class="text-xs text-dim">recurring rule, synced across devices</span>
        </header>

        <!-- Presets — one-click for the common shapes. -->
        <div class="flex flex-wrap gap-2 mb-5">
          <button
            type="button"
            onclick={() => applyPreset('midnight-sunday')}
            class="text-xs px-2.5 py-1 rounded border border-surface1 bg-surface0 hover:bg-surface1 text-subtext"
          >Sunday (midnight → midnight)</button>
          <button
            type="button"
            onclick={() => applyPreset('midnight-saturday')}
            class="text-xs px-2.5 py-1 rounded border border-surface1 bg-surface0 hover:bg-surface1 text-subtext"
          >Saturday (midnight → midnight)</button>
          <button
            type="button"
            onclick={() => applyPreset('sundown-friday')}
            class="text-xs px-2.5 py-1 rounded border border-surface1 bg-surface0 hover:bg-surface1 text-subtext"
          >Friday sundown → Saturday sundown</button>
        </div>

        <div class="space-y-3">
          <label class="flex items-center gap-2 text-sm text-subtext">
            <input
              type="checkbox"
              checked={sched.enabled}
              onchange={(e) => patch({ enabled: (e.target as HTMLInputElement).checked })}
              class="rounded"
            />
            <span>enable weekly schedule</span>
          </label>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 {sched.enabled ? '' : 'opacity-50'}">
            <label class="flex flex-col gap-1 text-sm text-subtext">
              <span class="text-xs text-dim uppercase tracking-wider">day</span>
              <select
                disabled={!sched.enabled}
                value={sched.dayOfWeek}
                onchange={(e) => patch({ dayOfWeek: Number((e.target as HTMLSelectElement).value) })}
                class="px-2 py-1 bg-mantle border border-surface1 rounded text-text"
              >
                {#each DAY_LABELS as label, i}
                  <option value={i}>{label}</option>
                {/each}
              </select>
            </label>

            <label class="flex flex-col gap-1 text-sm text-subtext">
              <span class="text-xs text-dim uppercase tracking-wider">starts at</span>
              <input
                type="time"
                disabled={!sched.enabled}
                value="{String(sched.startHour).padStart(2, '0')}:{String(sched.startMinute).padStart(2, '0')}"
                onchange={(e) => {
                  const v = (e.target as HTMLInputElement).value;
                  const [h, m] = v.split(':').map(Number);
                  if (Number.isFinite(h) && Number.isFinite(m)) {
                    patch({ startHour: h, startMinute: m });
                  }
                }}
                class="px-2 py-1 bg-mantle border border-surface1 rounded text-text font-mono"
              />
            </label>

            <label class="flex flex-col gap-1 text-sm text-subtext">
              <span class="text-xs text-dim uppercase tracking-wider">duration</span>
              <select
                disabled={!sched.enabled}
                value={sched.durationMinutes}
                onchange={(e) => patch({ durationMinutes: Number((e.target as HTMLSelectElement).value) })}
                class="px-2 py-1 bg-mantle border border-surface1 rounded text-text"
              >
                {#each DURATION_PRESETS as p}
                  <option value={p.mins}>{p.label}</option>
                {/each}
                {#if !DURATION_PRESETS.some((p) => p.mins === sched.durationMinutes)}
                  <option value={sched.durationMinutes}>{sched.durationMinutes} min</option>
                {/if}
              </select>
            </label>
          </div>

          <p class="text-xs text-dim leading-relaxed">
            The window may span midnight — e.g. Friday 18:00 + 24h covers through
            Saturday evening (the traditional sundown-to-sundown observance). The
            server uses this rule to silence work-modules; every connected device
            sees the same schedule.
          </p>
        </div>
      </section>

      {#if observances.length > 0}
        <section class="mt-10">
          <h2 class="text-xs uppercase tracking-wider text-dim mb-3">Sabbaths observed</h2>
          <ul class="text-sm text-subtext space-y-1.5">
            {#each observances as o}
              <li class="flex items-baseline gap-3">
                <span class="text-text font-mono text-[12px] w-28">{fmtDateShort(o.beganAt)}</span>
                <span class="text-dim text-[11px]">
                  {fmtTimeShort(o.beganAt)}
                  {#if o.endedAt}
                    → {fmtTimeShort(o.endedAt)}
                  {:else}
                    → <span class="text-success">in progress</span>
                  {/if}
                </span>
              </li>
            {/each}
          </ul>
        </section>
      {:else if logLoading}
        <p class="mt-10 text-xs text-dim italic">loading log…</p>
      {/if}
      {#if logError}
        <p class="mt-2 text-xs text-error">{logError}</p>
      {/if}
    {/if}
  </div>
</div>
