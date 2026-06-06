// Pure presentation helpers for the venture detail page.
//
// First extraction step out of routes/ventures/[name]/+page.svelte.
// Stateless formatters and tone mappers: color lookup, status pill
// tone, deadline countdown text, note title fallback, and the
// body-excerpt window around the venture-name match. No runes —
// these are plain functions the page component imports.
//
// Pulled out so the page can shrink toward the per-extraction LOC
// budget without losing any rendering behavior.

import { type Deadline, type Note } from '$lib/api';
import { daysUntil } from '$lib/deadlines/util';

/** Map a venture/project palette color name to the CSS variable used
 *  in inline `style` attributes. Falls back to --color-secondary so
 *  un-colored entities still render. */
export function colorVar(c?: string): string {
  const map: Record<string, string> = {
    red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
    blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
    peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
    lavender: 'primary', flamingo: 'error'
  };
  return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
}

/** Tone token for a venture/project/goal status pill. Returned as a
 *  bare token (e.g. `success`) so the caller can interpolate it into
 *  both `var(--color-X)` and class names. */
export function statusTone(s?: string): string {
  if (s === 'active') return 'success';
  if (s === 'paused') return 'warning';
  if (s === 'completed') return 'info';
  if (s === 'archived') return 'subtext';
  return 'subtext';
}

/** Short-form countdown for a deadline — mirrors the /deadlines page
 *  formatter so the language is consistent across surfaces. */
export function countdown(d: Deadline): string {
  if (d.status === 'met') return 'met';
  if (d.status === 'cancelled') return 'cancelled';
  const n = daysUntil(d.date);
  if (n === 0) return 'today';
  if (n === 1) return 'tomorrow';
  if (n === -1) return 'yesterday';
  if (n > 1) return `in ${n}d`;
  return `${-n}d ago`;
}

/** Tone token for a deadline countdown — urgent (overdue / ≤3d) is
 *  error, ≤7d is warning, ≤30d is info, beyond that subtext. */
export function deadlineTone(d: Deadline): string {
  if (d.status === 'met') return 'success';
  if (d.status === 'cancelled') return 'subtext';
  const n = daysUntil(d.date);
  if (n < 0) return 'error';
  if (n <= 3) return 'error';
  if (n <= 7) return 'warning';
  if (n <= 30) return 'info';
  return 'subtext';
}

/** Note title fallback — listNotes returns title from frontmatter if
 *  present, else basename. We strip `.md` defensively. */
export function noteTitle(n: Note): string {
  if (n.title && n.title.trim() !== '') return n.title;
  const base = n.path.split('/').pop() ?? n.path;
  return base.replace(/\.md$/i, '');
}

/** 200-char excerpt around the first occurrence of `name` in `n.body`
 *  (case-insensitive). Falls back to the head of the body when no
 *  match — possible if the match is in frontmatter. Whitespace is
 *  collapsed so the excerpt renders on a single line. */
export function noteBodyExcerpt(n: Note, name: string): string {
  if (!n.body) return '';
  const lower = n.body.toLowerCase();
  const idx = lower.indexOf(name.toLowerCase());
  const start = idx >= 0 ? Math.max(0, idx - 40) : 0;
  return n.body.slice(start, start + 200).replace(/\s+/g, ' ').trim();
}
