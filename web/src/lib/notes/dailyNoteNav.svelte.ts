// Daily-note derivations + navigation for the notes route page.
//
// A note is "daily" when its basename is YYYY-MM-DD.md OR its
// frontmatter has type=daily. The pure detection + date math live
// in $lib/notes/dailyNote; this controller wraps them as reactive
// $derived getters so the page consumes:
//
//   dailyDate           — ISO date string, or null when not daily.
//   isDaily             — boolean alias for dailyDate !== null.
//   dayActivitySegments — split of bodyForPreview around the
//                         "## Day Activity" anchor when daily, null
//                         otherwise. Drives the inline activity
//                         widget in the preview pane.
//   dailyLabel          — relative-day label ("Today", "Yesterday",
//                         "Mon · 3d ago") for the header strip.
//
// Plus the imperative gotoDaily(date) which silently flushes a dirty
// edit and navigates to /notes/<date>.md (or the canonical path
// returned by /api/v1/daily/<date> when one exists).

import { goto } from '$app/navigation';
import { api, type Note } from '$lib/api';
import {
  parseDailyDate,
  splitDayActivity,
  formatRelativeDailyLabel,
  todayLocalISO
} from '$lib/notes/dailyNote';

export interface DailyNoteNav {
  readonly dailyDate: string | null;
  readonly isDaily: boolean;
  readonly dayActivitySegments: { before: string; after: string } | null;
  readonly dailyLabel: string;
  /** Silently flushes a dirty edit then navigates to /notes/<date>.md
   *  (or the canonical path returned by /api/v1/daily). */
  gotoDaily: (date: string) => Promise<void>;
}

export interface DailyNoteNavOpts {
  getNote: () => Note | null;
  /** rAF-throttled body — segment split walks the string, so we
   *  drive it off the mirror to coalesce with the preview parse. */
  getBodyForPreview: () => string;
  /** Page-level dirty flag — true means an autosave is pending. */
  getDirty: () => boolean;
  /** Page-level silent save — fired before navigation so a dirty
   *  edit doesn't race the route swap. */
  save: (opts: { silent: boolean }) => Promise<boolean>;
}

export function createDailyNoteNav(opts: DailyNoteNavOpts): DailyNoteNav {
  const dailyDate = $derived(parseDailyDate(opts.getNote()));
  const isDaily = $derived(dailyDate !== null);
  const dayActivitySegments = $derived(
    isDaily ? splitDayActivity(opts.getBodyForPreview()) : null
  );
  const dailyLabel = $derived.by(() => {
    if (!dailyDate) return '';
    return formatRelativeDailyLabel(dailyDate, todayLocalISO());
  });

  async function gotoDaily(date: string): Promise<void> {
    if (opts.getDirty()) void opts.save({ silent: true });
    try {
      // /api/v1/daily/<date> creates today's note if missing; for
      // past/future dates it just returns the existing note (we
      // won't auto-materialize an empty file for arbitrary historical
      // dates).
      const n = await api.daily(date);
      goto(`/notes/${encodeURIComponent(n.path)}`);
    } catch {
      // If no existing daily for that date, just try the canonical
      // path.
      goto(`/notes/${encodeURIComponent(date + '.md')}`);
    }
  }

  return {
    get dailyDate() { return dailyDate; },
    get isDaily() { return isDaily; },
    get dayActivitySegments() { return dayActivitySegments; },
    get dailyLabel() { return dailyLabel; },
    gotoDaily
  };
}
