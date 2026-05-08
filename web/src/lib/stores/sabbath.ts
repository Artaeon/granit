// Sabbath mode — a per-day client-side overlay on the nav + dashboard
// that hides work-oriented modules (finance, tasks, projects, agents,
// deadlines) and surfaces rest-oriented ones (scripture, prayer,
// people, vision, jots). Mark 2:27: "the sabbath was made for man."
//
// Stored in localStorage under granit.sabbath.activeOn = "YYYY-MM-DD".
// Auto-clears the next calendar day — the toggle expires on its own
// so a user who forgets to flip it off in the evening doesn't wake
// up with their work modules still hidden. No server round-trip;
// this is purely a UI affordance, not a profile or a module config
// change.
//
// Why client-only:
//   - Every device has its own pace; one device's sabbath isn't
//     necessarily another's
//   - The modules.json file is the persistent module config — using
//     it for a temporal toggle would muddle "this is what I want
//     hidden" with "this is what I want hidden today"
//   - Cheap, undoable, doesn't survive the device
//
// Schedule (added later): an optional day-of-week rule that
// auto-enables sabbath on the chosen day. Christian tradition is
// Sunday (0); Jewish observance is Saturday (6). User picks which.
// Schedule is opt-in — if disabled the manual toggle is the only
// way in. Either way the auto-expiry at the next calendar day
// applies; a Sunday sabbath ends Monday at midnight.

import { writable, get, type Readable } from 'svelte/store';

const KEY = 'granit.sabbath.activeOn';
const SCHEDULE_KEY = 'granit.sabbath.schedule';

// Modules considered "work" — hidden when sabbath is active. The
// list is intentional, not user-configurable: the point of sabbath
// is *the discipline of letting go*, not picking and choosing.
// Keeping this constant means the toggle does what it says.
export const SABBATH_HIDE_MODULES = [
  'finance',
  'tasks',
  'projects',
  'agents',
  'deadlines',
  'chat',
  'weekly_review',
  // Round 2: more work-coded surfaces. The principle: anything
  // about transacting, measuring, optimising, or planning hides;
  // anything about presence, reflection, and people stays.
  'emails',       // CRM-style tracking is inherently transactional
  'shopping',     // errands belong to a workday
  'ventures',     // companies / side hustles
  'goals',        // long-term striving conflicts with rest
  'habits',       // measurement of self; the discipline of not measuring is the point
  'measurements', // numeric tracking of any kind
  'objects',      // typed-objects browser feeds the systematising impulse
  'hub'           // launcher pad for tools → tools = work
];

// Rest modules surfaced as a hint when sabbath starts — nav doesn't
// hide anything from this list even if the user has them disabled
// in normal config. (Toggling sabbath shouldn't override their
// long-term preferences; just nudge.)
export const SABBATH_SURFACE_MODULES = [
  'scripture',
  'prayer',
  'people',
  'vision',
  'jots'
];

function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// ── Schedule ────────────────────────────────────────────────────
// Day-of-week auto-enable. The user picks the day; we re-check on
// each load + each visibility change. dayOfWeek matches JS Date
// getDay(): 0=Sun, 1=Mon, ..., 6=Sat. dayOfWeek=-1 means "no
// schedule" (manual toggle only).
export interface SabbathSchedule {
  enabled: boolean;
  dayOfWeek: number; // -1 = off
}

const DEFAULT_SCHEDULE: SabbathSchedule = { enabled: false, dayOfWeek: 0 };

function loadSchedule(): SabbathSchedule {
  if (typeof localStorage === 'undefined') return DEFAULT_SCHEDULE;
  try {
    const raw = localStorage.getItem(SCHEDULE_KEY);
    if (!raw) return DEFAULT_SCHEDULE;
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') {
      const dow = Number(parsed.dayOfWeek);
      const en = Boolean(parsed.enabled);
      if (Number.isInteger(dow) && dow >= -1 && dow <= 6) {
        return { enabled: en, dayOfWeek: dow };
      }
    }
  } catch {}
  return DEFAULT_SCHEDULE;
}

function persistSchedule(sched: SabbathSchedule) {
  if (typeof localStorage === 'undefined') return;
  try { localStorage.setItem(SCHEDULE_KEY, JSON.stringify(sched)); } catch {}
}

export const sabbathSchedule = writable<SabbathSchedule>(loadSchedule());
sabbathSchedule.subscribe((s) => persistSchedule(s));

export const DAY_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

// Returns true when the schedule says "today is the sabbath" — the
// store init + visibilitychange handler use this to flip activeOn
// automatically. Idempotent: if already active for today, no-op.
function scheduleSaysToday(): boolean {
  const sched = loadSchedule();
  if (!sched.enabled || sched.dayOfWeek < 0) return false;
  const dow = new Date().getDay();
  return dow === sched.dayOfWeek;
}

// Time-remaining to next midnight (when sabbath auto-clears) — used
// by the sabbath landing screen + ribbon to show "rest until …".
// Returned in minutes; consumer formats. Returns 0 when sabbath
// isn't currently active.
export function sabbathMinutesRemaining(): number {
  if (!loadActive()) return 0;
  const now = new Date();
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 0, 0);
  return Math.max(0, Math.round((tomorrow.getTime() - now.getTime()) / 60000));
}

// Read the persisted value, treating any value other than today as
// "not active". The auto-expiry is a read-time check, not a
// background timer — simpler, no leak risk.
function loadActive(): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    const v = localStorage.getItem(KEY);
    return v === todayISO();
  } catch {
    return false;
  }
}

// Initial value: persisted activeOn OR schedule says today is the
// sabbath. The schedule path also writes activeOn so subsequent
// reads (and the server sync) see the same source-of-truth.
function computeInitial(): boolean {
  if (loadActive()) return true;
  if (scheduleSaysToday()) {
    const today = todayISO();
    try { localStorage.setItem(KEY, today); } catch {}
    // Server sync runs lazily — first fetch'll be after the auth
    // store hydrates. For initial computation we just persist
    // locally; the manual enable() path handles its own sync.
    return true;
  }
  return false;
}

const { subscribe, set } = writable<boolean>(computeInitial());

// Mirror local toggle to the server so server-side surfaces (push
// scheduler, future agents) can silently skip work during the
// day of rest. The server sidecar is .granit/sabbath.json. Best-
// effort: server unreachable → UI overlay still works (the local
// flag is the source of truth for navigation), only the push
// silencing degrades. We don't await — the toggle should feel
// instant.
function syncToServer(activeOn: string) {
  if (typeof fetch === 'undefined') return;
  let token: string | null = null;
  try {
    token = localStorage.getItem('everything.token');
  } catch {}
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  fetch('/api/v1/sabbath', {
    method: 'PUT',
    headers,
    body: JSON.stringify({ active_on: activeOn })
  }).catch(() => undefined);
}

export const sabbath: Readable<boolean> & {
  enable(): void;
  disable(): void;
  toggle(): void;
  isActive(): boolean;
} = {
  subscribe,
  enable() {
    const today = todayISO();
    try { localStorage.setItem(KEY, today); } catch {}
    set(true);
    syncToServer(today);
  },
  disable() {
    try { localStorage.removeItem(KEY); } catch {}
    set(false);
    syncToServer('');
  },
  toggle() {
    if (get({ subscribe })) this.disable();
    else this.enable();
  },
  isActive() {
    return get({ subscribe });
  }
};

// Re-evaluate on focus — if a user toggles sabbath on at 11pm and
// returns at 1am the next day, the store should reflect "no longer
// active" without a page reload. visibilitychange is the cheap event
// that triggers when the tab regains focus. Same path also picks
// up the schedule auto-enable for users who left the tab open
// across midnight on a Saturday→Sunday boundary.
if (typeof window !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      let fresh = loadActive();
      if (!fresh && scheduleSaysToday()) {
        const today = todayISO();
        try { localStorage.setItem(KEY, today); } catch {}
        syncToServer(today);
        fresh = true;
      }
      if (fresh !== get({ subscribe })) set(fresh);
    }
  });
}
