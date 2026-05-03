// Local-time date helpers shared across the codebase. These work in
// the user's wall-clock time (not UTC) because every place that calls
// them is rendering for the user — calendar grids, daily notes,
// "today" filters all use the local day boundary, never UTC midnight.
//
// Kept tiny on purpose: anything more elaborate (weekday math, range
// arithmetic, parsing) lives in lib/calendar/utils.ts. This file only
// hosts the two helpers that were copy-pasted across api.ts,
// calendar/utils.ts, and a TaskBacklog local function before the
// consolidation.

// fmtDateISO returns YYYY-MM-DD for a Date in local time. Used for
// dictionary keys (eventsByDay, etc.) and as the canonical day form
// the server reads / writes back.
export function fmtDateISO(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// todayISO returns YYYY-MM-DD for `new Date()` in local time. Equivalent
// to `fmtDateISO(new Date())` but reads more clearly at call sites that
// only care about "is this today?".
export function todayISO(): string {
  return fmtDateISO(new Date());
}
