import { describe, expect, it } from 'vitest';
import { fuzzyScore, fuzzyScoreMulti } from './fuzzy';

describe('fuzzyScore', () => {
  it('returns the neutral score for an empty needle', () => {
    expect(fuzzyScore('', 'anything')).toBe(1);
    expect(fuzzyScore('', '')).toBe(1);
  });

  it('returns null when the needle has no subsequence match', () => {
    expect(fuzzyScore('xyz', 'projects')).toBeNull();
    expect(fuzzyScore('abc', 'def')).toBeNull();
  });

  it('scores exact case-insensitive matches highest', () => {
    expect(fuzzyScore('tasks', 'Tasks')).toBe(1000);
    expect(fuzzyScore('Goals', 'goals')).toBe(1000);
  });

  it('ranks prefix above substring above subsequence', () => {
    // 'pro' against three candidates, all real
    const prefixHit = fuzzyScore('pro', 'projects')!; // prefix
    const substrHit = fuzzyScore('pro', 'my project')!; // substring not at 0
    const subseqHit = fuzzyScore('pn', 'project notes')!; // subseq w/ boundary
    expect(prefixHit).toBeGreaterThan(substrHit);
    expect(substrHit).toBeGreaterThan(subseqHit);
  });

  it('breaks prefix ties by haystack length (shorter wins)', () => {
    const shortHit = fuzzyScore('go', 'goals')!;
    const longHit = fuzzyScore('go', 'goals and milestones')!;
    expect(shortHit).toBeGreaterThan(longHit);
  });

  it('breaks substring ties by earlier index then length', () => {
    const early = fuzzyScore('cal', 'calendar')!; // index 0 — actually prefix
    const mid = fuzzyScore('cal', 'my calendar')!; // index 3
    expect(early).toBeGreaterThan(mid);
  });

  it('does a real subsequence match when chars are not contiguous', () => {
    // 'pn' matches 'p' in "project" + 'n' in "notes"
    const s = fuzzyScore('pn', 'project notes');
    expect(s).not.toBeNull();
    expect(s!).toBeGreaterThan(0);
  });

  it('rewards subsequence matches on word boundaries', () => {
    // Both candidates contain 'pn' as a subsequence, but only
    // 'project notes' has each letter at a word boundary.
    const onBoundaries = fuzzyScore('pn', 'project notes')!;
    const midWord = fuzzyScore('pn', 'opening')!; // p at 3, n at 5 — no boundaries
    expect(onBoundaries).toBeGreaterThan(midWord);
  });

  it('handles single-character queries', () => {
    expect(fuzzyScore('t', 'tasks')).not.toBeNull();
    expect(fuzzyScore('t', 'tasks')!).toBeGreaterThan(0);
    expect(fuzzyScore('z', 'tasks')).toBeNull();
  });

  it('is case-insensitive on both sides', () => {
    expect(fuzzyScore('PROJ', 'projects')).toBe(fuzzyScore('proj', 'PROJECTS'));
  });

  it('treats slash and dash as word boundaries', () => {
    // The subseq matcher should treat path/segment boundaries the
    // same as spaces — both 's' chars land at boundaries.
    const slash = fuzzyScore('ss', 'src/setup')!;
    const space = fuzzyScore('ss', 'src setup')!;
    expect(slash).toBe(space);
  });
});

describe('fuzzyScoreMulti', () => {
  it('returns the best score across haystacks', () => {
    const s = fuzzyScoreMulti('tasks', ['my tasks', 'tasks']);
    // exact match against the second haystack wins
    expect(s).toBe(1000);
  });

  it('returns null only when every haystack fails', () => {
    expect(fuzzyScoreMulti('xyz', ['abc', 'def'])).toBeNull();
  });

  it('falls back to a partial match when one haystack matches', () => {
    const s = fuzzyScoreMulti('not', ['abc', 'notes']);
    expect(s).not.toBeNull();
    expect(s!).toBeGreaterThan(0);
  });

  it('returns the neutral score for an empty needle if any haystack exists', () => {
    expect(fuzzyScoreMulti('', ['anything'])).toBe(1);
  });
});
