import { describe, expect, it } from 'vitest';
import { addIntentToHistory, normaliseHistory, MAX_HISTORY } from './agentHistory';

describe('addIntentToHistory', () => {
	it('prepends a fresh intent', () => {
		expect(addIntentToHistory([], 'archive stale')).toEqual(['archive stale']);
		expect(addIntentToHistory(['old'], 'new')).toEqual(['new', 'old']);
	});

	it('trims whitespace and rejects empty/whitespace intents', () => {
		expect(addIntentToHistory([], '   ')).toEqual([]);
		expect(addIntentToHistory(['a'], '')).toEqual(['a']);
		expect(addIntentToHistory(['a'], '  cleanup  ')).toEqual(['cleanup', 'a']);
	});

	it('dedups case-insensitively, keeping the new value at the front', () => {
		const out = addIntentToHistory(['Archive stale', 'reschedule report'], 'archive STALE');
		expect(out).toEqual(['archive STALE', 'reschedule report']);
	});

	it('caps at MAX_HISTORY entries', () => {
		const seed = Array.from({ length: MAX_HISTORY }, (_, i) => `intent ${i}`);
		const out = addIntentToHistory(seed, 'newest');
		expect(out).toHaveLength(MAX_HISTORY);
		expect(out[0]).toBe('newest');
		expect(out[out.length - 1]).toBe(`intent ${MAX_HISTORY - 2}`);
	});

	it('preserves order of existing entries beyond the dedup move-to-front', () => {
		const out = addIntentToHistory(['a', 'b', 'c'], 'd');
		expect(out).toEqual(['d', 'a', 'b', 'c']);
	});
});

describe('normaliseHistory', () => {
	it('returns [] when input is not an array', () => {
		expect(normaliseHistory(null)).toEqual([]);
		expect(normaliseHistory(undefined)).toEqual([]);
		expect(normaliseHistory('cleanup')).toEqual([]);
		expect(normaliseHistory({ 0: 'cleanup' })).toEqual([]);
	});

	it('drops non-strings, trims, dedups case-insensitively, caps', () => {
		const out = normaliseHistory([
			'  archive stale  ',
			42,
			'archive STALE',
			null,
			'reschedule',
			'',
			'   ',
			'reschedule'
		]);
		expect(out).toEqual(['archive stale', 'reschedule']);
	});

	it('keeps the first MAX_HISTORY survivors when input is long', () => {
		const big = Array.from({ length: MAX_HISTORY + 5 }, (_, i) => `intent ${i}`);
		const out = normaliseHistory(big);
		expect(out).toHaveLength(MAX_HISTORY);
		expect(out[0]).toBe('intent 0');
	});
});
