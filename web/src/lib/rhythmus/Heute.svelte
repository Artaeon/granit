<script lang="ts">
  // The Heute-Karte's orchestrator. Owns the day's state, hydrates
  // it from the daily note's frontmatter on mount, and saves any
  // mutation back via a 1-second debounce.
  //
  // What this component takes care of (so its children don't):
  //   - Loading + creating the daily note. A missing note simply
  //     means "no check-in today yet" — we don't pre-create the
  //     file on visit, only on the first save.
  //   - Re-evaluating "are we in evening mode?" every 30s. The
  //     time-based mode switch is visual only; the user can still
  //     hit ⌘K to reach any other surface, so we don't need to
  //     fire a navigation or a notification when the threshold
  //     crosses — just rerender.
  //   - Cross-device reloads. If the TUI or phone writes to the
  //     same daily note we want the desktop view to pick up the
  //     change without a manual refresh.
  //
  // What this component refuses to do:
  //   - It does NOT cache anything in localStorage. The daily note
  //     IS the source of truth. The brief flicker on load is the
  //     honest signal that the data lives in the vault.
  //   - It does NOT pre-create tomorrow's note or preload past
  //     days. One day at a time — that's the whole pivot.

  import { onDestroy, onMount } from 'svelte';
  import { ApiError, api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { fmtDateISO } from '$lib/util/date';
  import { openAIOverlay } from '$lib/stores/ai-overlay';
  import { sabbath } from '$lib/stores/sabbath';
  import { rhythmusConfig } from './minima';
  import {
    dailyNotePath,
    parseDayFrontmatter,
    serializeDayFrontmatter
  } from './dailyNote';
  import {
    emptyDayState,
    emptyShutdown,
    type DayMode,
    type DayState,
    type PillarState,
    type ShutdownState
  } from './dayState';
  import { nextAction } from './nextAction';
  import type { PillarKey } from './pillars';
  import CheckInCard from './CheckInCard.svelte';
  import ModePicker from './ModePicker.svelte';
  import NextActionCard from './NextActionCard.svelte';
  import PillarList from './PillarList.svelte';
  import EveningShutdown from './EveningShutdown.svelte';

  type Props = {
    /** Injectable clock for testing. Defaults to new Date() at load
     *  + a 30-second refresh below. */
    now?: Date;
  };

  let { now: nowProp }: Props = $props();

  let day = $state<DayState>(emptyDayState(fmtDateISO(new Date())));
  let now = $state<Date>(new Date());
  // If the parent injected a clock (tests), use that. Plain
  // `$state(nowProp ?? new Date())` only captures the initial value,
  // which means the reactive prop wouldn't reach us — the $effect
  // below threads it through cleanly.
  $effect(() => {
    if (nowProp) now = nowProp;
  });
  let loaded = $state(false);
  /** True between the first attempt to read the note and a
   *  successful save. Lets us decide whether to PUT (existing) or
   *  POST (create). */
  let noteExists = $state(false);
  /** Server's body — preserved on every save so we never blow away
   *  whatever prose the user wrote alongside the structured fields. */
  let bodyOnDisk = $state('');
  /** Frontmatter as the server saw it last — we merge into this so
   *  hand-written keys ("weather: sunny", a custom win sentence)
   *  survive every rhythmus write. */
  let fmOnDisk = $state<Record<string, unknown>>({});
  let saveError = $state('');

  let dayPath = $derived(dailyNotePath(now));
  let cfg = $derived($rhythmusConfig);

  // Evening mode flips on at the configured time. The check is
  // string-comparison-safe (HH:MM) so we don't have to deal with
  // Date math for the comparison itself.
  function hhmm(d: Date): string {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }
  let evening = $derived(day.mode !== null && hhmm(now) >= cfg.eveningStartsAt);

  let action = $derived(
    day.mode === null
      ? null
      : nextAction(day, {
          now,
          eveningStartsAt: cfg.eveningStartsAt,
          eatNagAfter: cfg.eatNagAfter
        })
  );

  // ── Load + clock ───────────────────────────────────────────────
  onMount(() => {
    void load();
    const tick = setInterval(() => {
      now = new Date();
    }, 30_000);
    const offWs = onWsEvent((ev) => {
      if (ev.type === 'note.changed' && ev.path === dayPath) void load();
      if (ev.type === 'note.removed' && ev.path === dayPath) void load();
    });
    return () => {
      clearInterval(tick);
      offWs();
    };
  });

  // Midnight rollover. When `now` ticks past local midnight, the
  // derived `dayPath` changes from Daily/<yesterday>.md to
  // Daily/<today>.md — but without a reload, the screen would keep
  // showing yesterday's pillar state. Watch the day-string and
  // reload when it flips. Using a date string (not the Date object)
  // so we only fire on actual day changes, not every 30s tick.
  let loadedDate = $state<string>('');
  $effect(() => {
    const cur = fmtDateISO(now);
    if (loaded && loadedDate && cur !== loadedDate) {
      loadedDate = cur;
      void load();
    } else if (loaded && !loadedDate) {
      loadedDate = cur;
    }
  });

  async function load() {
    try {
      const n = await api.getNote(dailyNotePath(now));
      bodyOnDisk = n.body ?? '';
      fmOnDisk = (n.frontmatter ?? {}) as Record<string, unknown>;
      day = parseDayFrontmatter(fmOnDisk, fmtDateISO(now));
      noteExists = true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        bodyOnDisk = '';
        fmOnDisk = {};
        day = emptyDayState(fmtDateISO(now));
        noteExists = false;
      } else {
        saveError = e instanceof Error ? e.message : String(e);
      }
    } finally {
      loaded = true;
    }
  }

  // ── Save (debounced) ───────────────────────────────────────────
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingSave = false;

  function scheduleSave() {
    pendingSave = true;
    if (saveTimer !== null) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      saveTimer = null;
      void doSave();
    }, 1_000);
  }

  async function doSave() {
    if (!loaded) return;
    pendingSave = false;
    saveError = '';
    const fm = serializeDayFrontmatter(day, fmOnDisk);
    try {
      if (noteExists) {
        await api.putNote(dailyNotePath(now), { frontmatter: fm, body: bodyOnDisk });
      } else {
        await api.createNote({
          path: dailyNotePath(now),
          frontmatter: fm,
          body: bodyOnDisk
        });
        noteExists = true;
      }
      fmOnDisk = fm;
    } catch (e) {
      saveError = e instanceof Error ? e.message : String(e);
    }
  }

  onDestroy(() => {
    if (saveTimer !== null) clearTimeout(saveTimer);
    // Best-effort final save on navigate-away so an in-flight edit
    // isn't lost between routes.
    if (pendingSave) void doSave();
  });

  // ── Handlers ──────────────────────────────────────────────────
  function setMode(next: DayMode) {
    day = { ...day, mode: next };
    scheduleSave();
  }
  function setFatigue(next: number) {
    day = { ...day, fatigue: next };
    scheduleSave();
  }
  function setEaten(next: boolean) {
    // Marking eaten also satisfies the food pillar — the user's
    // "yes I ate" is the same gesture as "tick food done" on the
    // pillar row. Don't make the user say it twice.
    day = {
      ...day,
      eaten: next,
      pillars: { ...day.pillars, food: { ...day.pillars.food, done: next } }
    };
    scheduleSave();
  }
  function setMit(next: string) {
    day = { ...day, mit: next };
    scheduleSave();
  }
  function togglePillar(key: PillarKey, next: boolean) {
    const pillar: PillarState = { ...day.pillars[key], done: next };
    day = { ...day, pillars: { ...day.pillars, [key]: pillar } };
    // Ticking food is the same as flipping the eaten flag — keep
    // the two in sync so the next-action engine sees one truth.
    if (key === 'food') day = { ...day, eaten: next };
    scheduleSave();
  }
  function setShutdown(next: ShutdownState) {
    day = { ...day, shutdown: next };
    scheduleSave();
  }
  function closeDay() {
    day = {
      ...day,
      pillars: { ...day.pillars, evening: { ...day.pillars.evening, done: true } }
    };
    scheduleSave();
  }

  // Open the AI overlay pre-filled with a short morning-briefing
  // prompt. Hidden during Sabbath — the overlay would also gate
  // server-side, but skipping the visible button avoids tempting
  // the user with a "Briefing" affordance the rule says is closed.
  function runBriefing() {
    openAIOverlay({
      text:
        'Give me a short morning briefing — top three things I should focus on today and one thing I might be forgetting.',
      send: true
    });
  }

  // The check-in collapses out once a mode is picked. The state
  // (fatigue, MIT) stays editable from the pillar list / mode picker
  // afterwards in future iterations; for v1 the morning check-in is
  // the one shot.
  let needsCheckIn = $derived(loaded && day.mode === null);
</script>

<section aria-label="Heute" class="space-y-4">
  {#if !loaded}
    <div class="bg-mantle border border-surface1 rounded-lg p-5 animate-pulse h-32"></div>
  {:else if needsCheckIn}
    <CheckInCard
      mode={day.mode}
      fatigue={day.fatigue}
      eaten={day.eaten}
      mit={day.mit}
      onModeChange={setMode}
      onFatigueChange={setFatigue}
      onEatenChange={setEaten}
      onMitChange={setMit}
    />
  {:else if day.mode !== null}
    <div class="flex items-center justify-between flex-wrap gap-2">
      <h1 class="text-2xl font-semibold text-text">
        Heute, {now.toLocaleDateString('de-DE', { weekday: 'long', day: 'numeric', month: 'long' })}
      </h1>
      <div class="flex items-center gap-2 flex-wrap">
        {#if !$sabbath}
          <button
            type="button"
            onclick={runBriefing}
            class="text-xs px-2.5 py-1 rounded inline-flex items-center gap-1.5 bg-surface0 border border-surface1 text-subtext hover:border-primary hover:text-text transition-colors"
            title="Kurzes KI-Briefing für heute"
          >
            <span aria-hidden="true">☀</span>
            KI-Briefing
          </button>
        {/if}
        <ModePicker value={day.mode} onChange={setMode} />
      </div>
    </div>

    {#if action}
      <NextActionCard {action} />
    {/if}

    {#if evening}
      <EveningShutdown
        shutdown={day.shutdown ?? emptyShutdown()}
        eveningDone={day.pillars.evening.done}
        onShutdownChange={setShutdown}
        onCloseDay={closeDay}
      />
    {:else}
      <PillarList mode={day.mode} pillars={day.pillars} onToggle={togglePillar} />
    {/if}

    {#if day.mit && !day.pillars.work.done}
      <div class="text-xs text-dim">
        Wichtigste Aufgabe heute: <span class="text-text font-medium">{day.mit}</span>
      </div>
    {/if}
  {/if}

  {#if saveError}
    <p class="text-xs text-error">Speichern fehlgeschlagen: {saveError}</p>
  {/if}
</section>
