// Task-oriented date formatters that don't fit in date.ts (which is
// intentionally narrow — fmtDateISO / todayISO only). These render
// timestamps for chips and tooltips in the task UI; centralising them
// here keeps the wording consistent across TaskCard, TaskDetail, and
// any future surface that wants to label a snooze / wake / due time.

// Format a snooze ISO timestamp as a compact relative label.
//   "12m" / "3h" / "2d" for soon-ish wake times,
//   localised date ("May 27") once we're more than a week out.
// Returns the raw input if it isn't a parseable date.
export function relSnooze(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const diff = d.getTime() - Date.now();
  const mins = Math.round(diff / 60_000);
  if (mins < 60) return `${mins}m`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h`;
  const days = Math.round(hrs / 24);
  if (days < 7) return `${days}d`;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
