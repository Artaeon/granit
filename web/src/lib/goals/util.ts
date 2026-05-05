// Shared goal-side helpers — kept in their own module so /goals,
// dashboard widgets, and any future surfaces stay aligned on what
// "days until target" / "urgency tone" / etc. mean. Drift here used
// to be a real risk: /goals had its own daysUntilTarget and
// TopGoalsWidget had a copy with the same body but no enforced link.

/**
 * Days from today to the parsed target_date string.
 *
 * Returns null when the string isn't a real calendar date — some
 * goals stash free-text targets like "Q4 2026" or "fall sometime",
 * and those have no place on a numeric urgency axis. Today is
 * computed in local time so a goal targeting "today" reads as 0,
 * not -1 for users east of UTC.
 */
export function daysUntilTarget(s: string | undefined | null): number | null {
  if (!s) return null;
  const d = new Date(s);
  if (isNaN(d.getTime())) return null;
  const t = new Date();
  const aMid = new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const bMid = new Date(t.getFullYear(), t.getMonth(), t.getDate()).getTime();
  return Math.round((aMid - bMid) / (24 * 3600 * 1000));
}

/**
 * Tone token (semantic palette name — error / warning / info /
 * subtext) for a card border given days-until-target. Returns null
 * when the goal is far enough out that no urgency cue is needed —
 * the caller should render a neutral card.
 *
 * Used by both /goals card borders and the TopGoalsWidget chip
 * background, so the visual language stays consistent.
 */
export function targetUrgencyTone(days: number | null): string | null {
  if (days === null) return null;
  if (days < 0) return 'error';
  if (days <= 30) return 'warning';
  if (days <= 90) return 'info';
  return null;
}

/** Hero / widget left-border color CSS value, by urgency. */
export function targetBorderColor(days: number | null): string {
  if (days === null) return 'var(--color-surface2)';
  if (days < 0 || days <= 7) return 'var(--color-error)';
  if (days <= 30) return 'var(--color-warning)';
  if (days <= 90) return 'var(--color-info)';
  return 'var(--color-surface2)';
}

/**
 * Compact countdown chip — "12d past target" / "in 12d" / "in 3w" /
 * "in 4mo" — matching the DeadlinePill rhythm so a glance reads the
 * same shape across goals + deadlines.
 */
export function targetChip(s: string | undefined | null): { label: string; tone: string } | null {
  const days = daysUntilTarget(s);
  if (days === null) return null;
  if (days < 0) return { label: `${Math.abs(days)}d past target`, tone: 'error' };
  if (days === 0) return { label: 'today', tone: 'error' };
  if (days === 1) return { label: 'tomorrow', tone: 'warning' };
  if (days < 14) return { label: `in ${days}d`, tone: days <= 30 ? 'warning' : 'info' };
  if (days < 60) return { label: `in ${Math.round(days / 7)}w`, tone: days <= 30 ? 'warning' : 'info' };
  return { label: `in ${Math.round(days / 30)}mo`, tone: 'subtext' };
}
