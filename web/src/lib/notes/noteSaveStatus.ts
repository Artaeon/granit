// Two save-status strings the note header + status bar render:
//   - saveStatus  — "saving…" | "retry?" | "unsaved" | "saved {Ns} ago"
//   - lastSaved   — relative time only ("just now" | "{N}s ago" | …)
//
// Both share the same 1s nowTick the page already drives; a relative
// label only changes on second / minute / hour boundaries so a
// faster-than-1s tick is wasted work.

export interface SaveStatusInputs {
  saving: boolean;
  saveFailed: boolean;
  dirty: boolean;
  lastSavedAt: number | null;
  nowTick: number;
}

export function saveStatus(s: SaveStatusInputs): string {
  if (s.saving) return 'saving…';
  if (s.saveFailed && s.dirty) return 'retry?';
  if (s.dirty) return 'unsaved';
  if (!s.lastSavedAt) return 'saved';
  const ago = Math.floor((s.nowTick - s.lastSavedAt) / 1000);
  if (ago < 4) return 'saved';
  if (ago < 60) return `saved ${ago}s ago`;
  if (ago < 3600) return `saved ${Math.floor(ago / 60)}m ago`;
  return 'saved';
}

export function lastSavedDisplay(lastSavedAt: number | null, nowTick: number): string {
  if (!lastSavedAt) return '—';
  const sec = Math.round((nowTick - lastSavedAt) / 1000);
  if (sec < 5) return 'just now';
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.round(sec / 60)}m ago`;
  return `${Math.round(sec / 3600)}h ago`;
}
