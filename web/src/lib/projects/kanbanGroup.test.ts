import { describe, expect, it } from 'vitest';
import type { Project } from '$lib/api';
import { groupByStatus, statusLabel, KANBAN_STATUSES } from './kanbanGroup';

// Minimal Project factory — the type is wide (lots of optional
// fields, plus server-decorated counters), but the grouping helper
// only reads `name` + `status`. Build the smallest viable shape
// and cast through unknown so tests don't drift when the schema
// grows.
function mk(name: string, status?: string): Project {
	return { name, status, description: '', folder: '', tags: [], color: '', created_at: '' } as unknown as Project;
}

describe('groupByStatus', () => {
	it('always returns the four canonical buckets in flow order', () => {
		const out = groupByStatus([]);
		expect(out).toHaveLength(4);
		expect(out.map((b) => b.status)).toEqual(['active', 'paused', 'completed', 'archived']);
		// Empty input → every bucket is empty but present (drop target).
		for (const b of out) expect(b.projects).toEqual([]);
	});

	it('places each project in its matching bucket', () => {
		const out = groupByStatus([
			mk('a', 'active'),
			mk('p', 'paused'),
			mk('c', 'completed'),
			mk('z', 'archived')
		]);
		expect(out[0].projects.map((p) => p.name)).toEqual(['a']);
		expect(out[1].projects.map((p) => p.name)).toEqual(['p']);
		expect(out[2].projects.map((p) => p.name)).toEqual(['c']);
		expect(out[3].projects.map((p) => p.name)).toEqual(['z']);
	});

	it('treats missing or empty status as active (matches list page default)', () => {
		const out = groupByStatus([mk('untouched'), mk('blank', ''), mk('whitespace', '   ')]);
		expect(out[0].projects.map((p) => p.name)).toEqual(['untouched', 'blank', 'whitespace']);
		expect(out[1].projects).toEqual([]);
	});

	it('parks unknown statuses in archived rather than dropping them', () => {
		// Forward-compat: a future status added by the TUI shouldn't
		// silently vanish from the board.
		const out = groupByStatus([mk('legacy', 'on-hold' as string)]);
		expect(out[3].projects.map((p) => p.name)).toEqual(['legacy']);
	});

	it('preserves caller-supplied order within each bucket', () => {
		// Caller sorts by priority/name; the grouper must NOT re-sort.
		const out = groupByStatus([
			mk('a-third', 'active'),
			mk('a-first', 'active'),
			mk('a-second', 'active')
		]);
		expect(out[0].projects.map((p) => p.name)).toEqual(['a-third', 'a-first', 'a-second']);
	});

	it('handles a large mixed list without losing any project', () => {
		const inputs: Project[] = [];
		const expectCount = { active: 0, paused: 0, completed: 0, archived: 0 } as Record<string, number>;
		for (let i = 0; i < 50; i++) {
			const s = KANBAN_STATUSES[i % KANBAN_STATUSES.length];
			inputs.push(mk(`p${i}`, s));
			expectCount[s]++;
		}
		const out = groupByStatus(inputs);
		for (const b of out) expect(b.projects.length).toBe(expectCount[b.status]);
	});
});

describe('statusLabel', () => {
	it('returns a capitalised label for each canonical status', () => {
		expect(statusLabel('active')).toBe('Active');
		expect(statusLabel('paused')).toBe('Paused');
		expect(statusLabel('completed')).toBe('Completed');
		expect(statusLabel('archived')).toBe('Archived');
	});
});
