// Sabbath mode — a per-device client-side overlay on the nav + dashboard
// that hides work-oriented modules and surfaces rest-oriented ones.
// Mark 2:27: "the sabbath was made for man."
//
// State splits cleanly in two:
//
//   • Schedule (recurring rule) — synced server-side via /api/v1/sabbath.
//     Changes on one device show up on every other device. This is what
//     "every Saturday from midnight" means; also where sundown-to-sundown
//     windows live.
//
//   • Daily on/off (manual "begin now") — per-device, localStorage only.
//     A user toggling sabbath on their laptop doesn't force it on their
//     phone. The server gates work-modules (push, AI, agents) based on
//     the SCHEDULE alone — if you want gating during a manual sabbath,
//     configure the schedule.
//
// The schedule window can span midnight (Friday 18:00 + 24h goes through
// Saturday 18:00), so traditional sundown-to-sundown observance is a
// first-class shape, not a special case.
//
// Auto-expiry: the read-time `isActiveNow()` check uses (a) ActiveOn ==
// today for the manual flag, and (b) `now ∈ [windowStart, windowEnd)`
// for the schedule. Both fall off naturally when their condition stops
// holding — no background timer required, no leak risk.

import { writable, get, type Readable } from 'svelte/store';
import { loadStoredString, saveStoredString } from '$lib/util/storage';
import { api, type SabbathSchedulePayload } from '$lib/api';

const KEY = 'granit.sabbath.activeOn';
const SCHEDULE_KEY = 'granit.sabbath.schedule';
const SKIP_KEY = 'granit.sabbath.skipOn';

// Modules considered "work" — hidden when sabbath is active. The
// list is intentional, not user-configurable: the point of sabbath
// is *the discipline of letting go*, not picking and choosing.
export const SABBATH_HIDE_MODULES = [
  'finance',
  'tasks',
  'projects',
  'agents',
  'deadlines',
  'chat',
  'weekly_review',
  'emails',
  'shopping',
  'ventures',
  'goals',
  'habits',
  'measurements',
  'objects',
  'hub'
];

// Dashboard widget types hidden during sabbath. Mirror of
// SABBATH_HIDE_MODULES at widget granularity — the modules list
// gates the nav and route guards, this list gates the home-page
// widget grid so a "tasks paused" sabbath doesn't still surface a
// today-tasks widget right under the greeting. The string literals
// match the keys in $lib/dashboard/registry.ts.
//
// today-stream is hidden because it merges events + tasks +
// deadlines into one feed and there's no per-row sabbath filter
// inside it. Surface events via /calendar (route guard exempts
// it — calendar isn't in SABBATH_HIDE_MODULES) if you need them.
// today-focus is hidden because the morning routine commitment is
// a work-day artifact.
export const SABBATH_HIDE_WIDGET_TYPES: ReadonlySet<string> = new Set([
  'today-tasks',
  'scheduled-today',
  'inbox',
  'goals-progress',
  'projects-active',
  'ventures',
  'habits',
  'streaks',
  'weekly-plan',
  'today-focus',
  'today-stream'
]);

// Rest modules surfaced as a hint when sabbath starts — nav doesn't
// hide anything from this list even if the user has them disabled
// in normal config.
export const SABBATH_SURFACE_MODULES = [
  'scripture',
  'prayer',
  'people',
  'vision',
  'jots'
];

export const DAY_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// ── Schedule ────────────────────────────────────────────────────
// Camel-cased TS-side shape. Wire shape (snake_case) lives in
// api.ts as SabbathSchedulePayload; the two converters below adapt
// at the network boundary.
export interface SabbathSchedule {
  enabled: boolean;
  dayOfWeek: number;       // 0=Sun … 6=Sat
  startHour: number;       // 0-23
  startMinute: number;     // 0-59
  durationMinutes: number; // 1440 = 24h (midnight-to-midnight)
}

export const DEFAULT_SCHEDULE: SabbathSchedule = {
  enabled: false,
  dayOfWeek: 0,
  startHour: 0,
  startMinute: 0,
  durationMinutes: 1440
};

function toWire(s: SabbathSchedule): SabbathSchedulePayload {
  return {
    enabled: s.enabled,
    day_of_week: s.dayOfWeek,
    start_hour: s.startHour,
    start_minute: s.startMinute,
    duration_minutes: s.durationMinutes
  };
}

function fromWire(p: SabbathSchedulePayload): SabbathSchedule {
  return {
    enabled: Boolean(p.enabled),
    dayOfWeek: clampInt(p.day_of_week, 0, 6, 0),
    startHour: clampInt(p.start_hour, 0, 23, 0),
    startMinute: clampInt(p.start_minute, 0, 59, 0),
    durationMinutes: clampInt(p.duration_minutes, 1, 7 * 1440, 1440)
  };
}

function clampInt(v: unknown, min: number, max: number, fallback: number): number {
  const n = Number(v);
  if (!Number.isFinite(n)) return fallback;
  const i = Math.round(n);
  if (i < min) return min;
  if (i > max) return max;
  return i;
}

function loadScheduleLocal(): SabbathSchedule {
  const raw = loadStoredString(SCHEDULE_KEY, '');
  if (!raw) return { ...DEFAULT_SCHEDULE };
  try {
    const parsed = JSON.parse(raw);
    return {
      enabled: Boolean(parsed.enabled),
      dayOfWeek: clampInt(parsed.dayOfWeek, 0, 6, 0),
      startHour: clampInt(parsed.startHour, 0, 23, 0),
      startMinute: clampInt(parsed.startMinute, 0, 59, 0),
      durationMinutes: clampInt(parsed.durationMinutes, 1, 7 * 1440, 1440)
    };
  } catch {
    return { ...DEFAULT_SCHEDULE };
  }
}

function persistScheduleLocal(sched: SabbathSchedule) {
  try {
    saveStoredString(SCHEDULE_KEY, JSON.stringify(sched));
  } catch {
    // localStorage full / private mode — non-fatal; the server is the
    // source of truth and will be re-fetched on next load anyway.
  }
}

// ── Schedule window math ────────────────────────────────────────
// Mirror of the Go-side scheduleWindow in internal/sabbath. Finds
// the most-recently-started occurrence of (DayOfWeek, StartHour,
// StartMinute) at or before `at`, returns its [start, end) window.
// Returns null when schedule is disabled or no occurrence fits.
//
// Exported so the midnight-rollover + sundown-to-sundown invariants
// can be unit-tested; consumers outside this module should still
// prefer the `sabbath` store for "is sabbath active right now".
export function scheduleWindow(s: SabbathSchedule, at: Date): { start: Date; end: Date } | null {
  if (!s.enabled || s.durationMinutes <= 0) return null;
  // Walk back up to 8 days so today + every preceding day are covered.
  for (let daysBack = 0; daysBack < 8; daysBack++) {
    const cand = new Date(at.getFullYear(), at.getMonth(), at.getDate() - daysBack,
      s.startHour, s.startMinute, 0, 0);
    if (cand.getDay() !== s.dayOfWeek) continue;
    if (cand.getTime() > at.getTime()) continue;
    const end = new Date(cand.getTime() + s.durationMinutes * 60_000);
    return { start: cand, end };
  }
  return null;
}

export function scheduleSaysNow(s: SabbathSchedule, at: Date): boolean {
  const w = scheduleWindow(s, at);
  if (!w) return false;
  return at.getTime() >= w.start.getTime() && at.getTime() < w.end.getTime();
}

// ── Stores ──────────────────────────────────────────────────────
export const sabbathSchedule = writable<SabbathSchedule>(loadScheduleLocal());

// Persist locally on every change; sync to server when the change
// originates from user action (set via updateSchedule()). The store
// subscribe runs for both kinds of update, so we use a sync-suppression
// flag to avoid PUT-loop on server-driven updates.
let suppressServerSync = false;
sabbathSchedule.subscribe((s) => {
  persistScheduleLocal(s);
  if (suppressServerSync) return;
  // Best-effort PUT. Failure is non-fatal — the local copy is now
  // ahead of the server; next successful PUT (or page load if we
  // adopt last-write-wins later) reconciles.
  api.putSabbath({ schedule: toWire(s) }).catch(() => undefined);
});

// updateSchedule is the user-facing setter. Identical to .set() at
// this layer but reads as intent in call sites.
export function updateSchedule(next: SabbathSchedule) {
  sabbathSchedule.set(next);
}

// ── Daily on/off (per-device manual flag) ───────────────────────
// Local-only. The server gates work-modules based on the synced
// schedule, so a manual toggle here doesn't propagate. If you want
// server gating during a manual sabbath, configure the schedule.
function loadManualActive(): boolean {
  return loadStoredString(KEY, '') === todayISO();
}

// Skip — the "exit sabbath" override. When the user dismisses an
// otherwise-scheduled sabbath, we record today's date here AND PUT
// it to the server so server-side gates (push, agents, AI) also
// stop silencing for the day. The schedule itself is untouched;
// next week's sabbath fires normally.
function loadSkipToday(): boolean {
  return loadStoredString(SKIP_KEY, '') === todayISO();
}

function computeInitial(): boolean {
  if (loadSkipToday()) return false;
  if (loadManualActive()) return true;
  return scheduleSaysNow(loadScheduleLocal(), new Date());
}

const { subscribe, set } = writable<boolean>(computeInitial());

export const sabbath: Readable<boolean> & {
  enable(): void;
  disable(): void;
  toggle(): void;
  isActive(): boolean;
} = {
  subscribe,
  enable() {
    // Re-entering manually also clears the skip — if you change your
    // mind and want sabbath back on today, the skip shouldn't keep
    // suppressing the schedule on the next focus.
    saveStoredString(SKIP_KEY, undefined);
    saveStoredString(KEY, todayISO());
    set(true);
    // Tell the server too so server gates re-engage.
    api.putSabbath({ skip_on: '' }).catch(() => undefined);
  },
  disable() {
    // Clear the manual flag AND record a skip-today. The skip is the
    // escape hatch: without it, an active schedule would re-assert
    // immediately and the user would think the button was broken.
    // PUT the skip to the server so push/agent/AI gates also stop
    // silencing for the day. Schedule itself is untouched — next
    // week's sabbath still fires.
    saveStoredString(KEY, undefined);
    const today = todayISO();
    saveStoredString(SKIP_KEY, today);
    set(false);
    api.putSabbath({ skip_on: today, active_on: '' }).catch(() => undefined);
  },
  toggle() {
    if (get({ subscribe })) this.disable();
    else this.enable();
  },
  isActive() {
    return get({ subscribe });
  }
};

// ── Time-remaining ──────────────────────────────────────────────
// Returns minutes until the current sabbath window ends. For
// manual-only activations falls back to midnight tomorrow (the
// original behavior). Returns 0 when not active.
export function sabbathMinutesRemaining(): number {
  if (!get({ subscribe })) return 0;
  const now = new Date();
  const w = scheduleWindow(get(sabbathSchedule), now);
  if (w && now.getTime() >= w.start.getTime() && now.getTime() < w.end.getTime()) {
    return Math.max(0, Math.round((w.end.getTime() - now.getTime()) / 60_000));
  }
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 0, 0);
  return Math.max(0, Math.round((tomorrow.getTime() - now.getTime()) / 60_000));
}

// ── Server sync ─────────────────────────────────────────────────
// Fetch the canonical schedule from the server on first load + on
// every tab-focus. The server is the source of truth for the
// schedule; the local copy is a cache that only matters until the
// fetch resolves.
//
// Conflict resolution: server wins on read. If the user just edited
// the schedule locally and the PUT raced with the next focus-driven
// GET, the local edit might briefly flicker back to the server's
// previous value before the PUT-response reconciles. Acceptable
// tradeoff — the alternative (timestamps + last-write-wins) is more
// machinery than this surface needs.
//
// Coalesce: refetch is fired from the module init AND from every
// visibilitychange. A user toggling between tabs repeatedly while the
// server is slow would otherwise queue overlapping GETs — they all
// land on the same endpoint and the last one's `.set()` wins. The
// inFlight flag drops anything that comes in while a fetch is open;
// the visibilitychange that fires after it lands will pick up any new
// state.
let refetchInFlight = false;
async function refetchSchedule() {
  if (refetchInFlight) return;
  refetchInFlight = true;
  try {
    const res = await api.getSabbath();
    const remote = fromWire(res.schedule);
    const current = get(sabbathSchedule);
    if (!schedulesEqual(remote, current)) {
      suppressServerSync = true;
      try {
        sabbathSchedule.set(remote);
      } finally {
        suppressServerSync = false;
      }
      // Recompute the active flag now that the schedule may have changed.
      set(computeActiveAt(remote, new Date()));
    }
    // Server's skip_on is authoritative — pull it down so other
    // devices honor a skip that originated here, and so a skip set
    // by today's PUT survives a manual page reload.
    if (typeof res.skip_on === 'string') {
      if (res.skip_on) saveStoredString(SKIP_KEY, res.skip_on);
      else if (loadStoredString(SKIP_KEY, '') !== '') saveStoredString(SKIP_KEY, undefined);
    }
  } catch {
    // Server unreachable / auth not yet hydrated — try again on next
    // visibilitychange. The local copy is fine until then.
  } finally {
    refetchInFlight = false;
  }
}

// computeActiveAt is the single source of truth for "is sabbath
// active right now": skip wins → manual wins → schedule. Matches the
// Go IsActiveAt ordering.
function computeActiveAt(s: SabbathSchedule, at: Date): boolean {
  if (loadSkipToday()) return false;
  if (loadManualActive()) return true;
  return scheduleSaysNow(s, at);
}

function schedulesEqual(a: SabbathSchedule, b: SabbathSchedule): boolean {
  return a.enabled === b.enabled
    && a.dayOfWeek === b.dayOfWeek
    && a.startHour === b.startHour
    && a.startMinute === b.startMinute
    && a.durationMinutes === b.durationMinutes;
}

if (typeof window !== 'undefined') {
  // Initial fetch — fire-and-forget, runs as soon as the module loads.
  // The auth token may not be hydrated yet on cold-start; the
  // visibilitychange handler will retry shortly.
  refetchSchedule();

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible') return;
    // Re-fetch the schedule (server may have changed) and re-evaluate
    // local active state (date may have rolled over, or window may
    // have just opened/closed, or a skip may have expired).
    refetchSchedule();
    const fresh = computeActiveAt(get(sabbathSchedule), new Date());
    if (fresh !== get({ subscribe })) set(fresh);
  });
}
