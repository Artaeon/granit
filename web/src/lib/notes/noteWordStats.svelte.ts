// Word / line / character / reading-time / word-goal derivations
// for the notes editor status bar.
//
// Drives off the rAF-throttled preview-body mirror (not the raw
// `body`) so a fast typist on a long note doesn't pay for trim +
// split + indexOf per keystroke. Each derivation here allocates a
// new array per body update (trim+split for wordCount, split for
// lineCount), which is O(N) in body length. On a 100 KB note that's
// ~3–8 ms per keystroke just to refresh the status bar — a real
// contributor to the editor-freeze on long notes. The status bar
// can absolutely tolerate one frame of lag (16 ms); pinning it to
// the mirror coalesces the work with the preview parse instead of
// firing it 60–200×/s.
//
// Word goal: frontmatter `target_words: 1500` (alias `word_goal`)
// turns the status-bar word count into a progress indicator. Common
// shape for journaling / essay drafts where the user committed to a
// target. The status bar renders a thin progress bar under the count
// + a percentage label so progress is visible at a glance without
// taking footer space when no target is set.

import type { Note } from '$lib/api';

export interface NoteWordStats {
  readonly wordCount: number;
  readonly charCount: number;
  readonly lineCount: number;
  /** ~225 wpm — average silent reading speed. Floor of 1 minute so
   *  a short note doesn't read "0 min". The status bar hides the
   *  value under 50 words because "<1 min" on a tiny note is noise. */
  readonly readingMinutes: number;
  /** Goal in words, or null when frontmatter has no target. */
  readonly wordGoal: number | null;
  /** Progress percentage 0..100 (clamped). 0 when no goal. */
  readonly wordGoalPct: number;
}

export interface NoteWordStatsOpts {
  /** rAF-throttled preview body — read via getter so the $derived
   *  inside the controller can track it. */
  getBodyForPreview: () => string;
  /** Currently-loaded note; null between loads. The frontmatter
   *  read for the word goal happens lazily inside the $derived. */
  getNote: () => Note | null;
}

export function createNoteWordStats(opts: NoteWordStatsOpts): NoteWordStats {
  const wordCount = $derived.by(() => {
    const t = opts.getBodyForPreview().trim();
    return t ? t.split(/\s+/).length : 0;
  });
  const charCount = $derived(opts.getBodyForPreview().length);
  const lineCount = $derived.by(() => {
    const b = opts.getBodyForPreview();
    return b ? b.split('\n').length : 0;
  });
  const readingMinutes = $derived(Math.max(1, Math.round(wordCount / 225)));

  const wordGoal = $derived.by<number | null>(() => {
    const fm = opts.getNote()?.frontmatter as Record<string, unknown> | undefined;
    if (!fm) return null;
    const v = fm.target_words ?? fm.word_goal;
    if (typeof v === 'number' && v > 0) return Math.floor(v);
    if (typeof v === 'string') {
      const n = parseInt(v, 10);
      if (!Number.isNaN(n) && n > 0) return n;
    }
    return null;
  });
  const wordGoalPct = $derived(
    wordGoal ? Math.min(100, Math.round((wordCount / wordGoal) * 100)) : 0
  );

  return {
    get wordCount() { return wordCount; },
    get charCount() { return charCount; },
    get lineCount() { return lineCount; },
    get readingMinutes() { return readingMinutes; },
    get wordGoal() { return wordGoal; },
    get wordGoalPct() { return wordGoalPct; }
  };
}
