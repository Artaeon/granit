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

import { writable, get, type Readable } from 'svelte/store';

const KEY = 'granit.sabbath.activeOn';

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

const { subscribe, set } = writable<boolean>(loadActive());

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
// that triggers when the tab regains focus.
if (typeof window !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      const fresh = loadActive();
      if (fresh !== get({ subscribe })) set(fresh);
    }
  });
}
