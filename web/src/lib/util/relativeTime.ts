// Shared relative-time formatter. Seven near-identical local copies
// were scattered across widgets, the notes index, settings, and
// HistoryPanel — each with slightly different cutoffs and tone
// ("Xm ago" vs "Xm" vs "in X min"). Consolidated here so every
// surface uses the same semantics.
//
// Two shapes:
//   relativeTime(iso | ms | Date)            — past-only, "Xm ago"
//   relativeTime(input, { future: true })    — also handles future, "in Xh"
//   relativeTime(input, { compact: true })   — drops "ago" suffix, "Xm"
//
// Returns '' for unparseable input so call sites don't have to
// guard. The 'just now' threshold is 5 seconds — anything inside
// that window reads as "just now" rather than a flickering "0s".

export interface RelativeTimeOptions {
  /** Allow future timestamps. Default false; future input returns ''. */
  future?: boolean;
  /** Drop the "ago" suffix. "5m ago" → "5m". Default false. */
  compact?: boolean;
  /** Days threshold above which the formatter falls back to a calendar
   *  date. Default Infinity (never). */
  dateThresholdDays?: number;
  /** Custom calendar-date formatter when above dateThresholdDays.
   *  Defaults to "Mon DD". */
  dateFormatter?: (d: Date) => string;
}

const DEFAULT_DATE_FORMATTER = (d: Date) =>
  d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });

export function relativeTime(
  input: string | number | Date | null | undefined,
  opts: RelativeTimeOptions = {}
): string {
  if (input === null || input === undefined) return '';
  const ms =
    input instanceof Date
      ? input.getTime()
      : typeof input === 'number'
        ? input
        : new Date(input).getTime();
  if (!Number.isFinite(ms)) return '';

  const diffMs = Date.now() - ms;
  const future = diffMs < 0;
  if (future && !opts.future) return '';

  const absSec = Math.max(0, Math.floor(Math.abs(diffMs) / 1000));
  if (absSec < 5) return future ? 'soon' : 'just now';

  const compact = !!opts.compact;
  const min = Math.round(absSec / 60);
  const hours = Math.round(min / 60);
  const days = Math.round(hours / 24);
  const weeks = Math.round(days / 7);
  const months = Math.round(days / 30);

  if (opts.dateThresholdDays !== undefined && days >= opts.dateThresholdDays) {
    return (opts.dateFormatter ?? DEFAULT_DATE_FORMATTER)(new Date(ms));
  }

  let label: string;
  if (absSec < 60) label = compact ? `${absSec}s` : `${absSec}s`;
  else if (min < 60) label = compact ? `${min}m` : `${min}m`;
  else if (hours < 24) label = compact ? `${hours}h` : `${hours}h`;
  else if (days < 7) label = compact ? `${days}d` : `${days}d`;
  else if (days < 30) label = compact ? `${weeks}w` : `${weeks}w`;
  else label = compact ? `${months}mo` : `${months}mo`;

  if (compact) return label;
  return future ? `in ${label}` : `${label} ago`;
}
