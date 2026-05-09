import { describe, expect, it } from 'vitest';
import { slugifyTitle } from './slug';

describe('slugifyTitle', () => {
  it('lowercases the input', () => {
    expect(slugifyTitle('My Note')).toBe('my-note');
  });

  it('strips diacriticals (NFKD)', () => {
    expect(slugifyTitle('café résumé naïve')).toBe('cafe-resume-naive');
  });

  it('collapses runs of non-alphanumerics to a single dash', () => {
    expect(slugifyTitle('hello!!! ---world')).toBe('hello-world');
  });

  it('trims leading + trailing dashes', () => {
    expect(slugifyTitle('---hello---')).toBe('hello');
  });

  it('clamps long titles at 80 chars', () => {
    const long = 'a'.repeat(200);
    expect(slugifyTitle(long).length).toBe(80);
  });

  it('returns empty string for all-symbols input', () => {
    expect(slugifyTitle('!!!')).toBe('');
  });

  it('preserves digits', () => {
    expect(slugifyTitle('Book 1984')).toBe('book-1984');
  });

  it('handles non-Latin scripts by stripping them entirely', () => {
    // Greek + Cyrillic land outside [a-z0-9] after normalisation,
    // so they collapse via the dash replacement rule.
    expect(slugifyTitle('hello αβγ привет world')).toBe('hello-world');
  });
});
