import { describe, expect, it } from 'vitest';
import { mergeProposals, extractActions, type ProposalFlags } from './core';

// Toy action shape — the merge function only needs to extract a
// key. Test against the smallest possible action so the cases
// exercise the semantics, not the entity schema.
type ToyAction = { id: string; kind: string; arg?: number };
const k = (a: ToyAction) => `${a.id}::${a.kind}`;

describe('mergeProposals', () => {
	it('keeps order from the new parse for live rows', () => {
		const out = mergeProposals<ToyAction>(
			[],
			[
				{ id: 'a', kind: 'archive' },
				{ id: 'b', kind: 'snooze' }
			],
			k
		);
		expect(out.map((r) => r.id)).toEqual(['a', 'b']);
	});

	it('preserves applied state across a re-parse with the same row', () => {
		const prev: (ToyAction & ProposalFlags)[] = [{ id: 'a', kind: 'archive', applied: true }];
		const out = mergeProposals<ToyAction>(prev, [{ id: 'a', kind: 'archive' }], k);
		expect(out).toHaveLength(1);
		expect(out[0].applied).toBe(true);
	});

	it('keeps an applied row even when the new parse drops it', () => {
		const prev: (ToyAction & ProposalFlags)[] = [{ id: 'a', kind: 'archive', applied: true }];
		const out = mergeProposals<ToyAction>(prev, [], k);
		expect(out).toHaveLength(1);
		expect(out[0].applied).toBe(true);
	});

	it('keeps a rejected row even when the new parse drops it', () => {
		const prev: (ToyAction & ProposalFlags)[] = [{ id: 'a', kind: 'archive', rejected: true }];
		const out = mergeProposals<ToyAction>(prev, [], k);
		expect(out).toHaveLength(1);
		expect(out[0].rejected).toBe(true);
	});

	it('drops pending rows the new parse no longer mentions', () => {
		const prev: (ToyAction & ProposalFlags)[] = [{ id: 'a', kind: 'archive' }];
		const out = mergeProposals<ToyAction>(prev, [], k);
		expect(out).toEqual([]);
	});

	it('uses NEW args for pending rows', () => {
		const prev: (ToyAction & ProposalFlags)[] = [{ id: 'a', kind: 'set', arg: 1 }];
		const out = mergeProposals<ToyAction>(prev, [{ id: 'a', kind: 'set', arg: 3 }], k);
		expect(out[0].arg).toBe(3);
	});

	it('FREEZES args for applied rows', () => {
		const prev: (ToyAction & ProposalFlags)[] = [
			{ id: 'a', kind: 'set', arg: 1, applied: true }
		];
		const out = mergeProposals<ToyAction>(prev, [{ id: 'a', kind: 'set', arg: 3 }], k);
		expect(out[0].arg).toBe(1);
		expect(out[0].applied).toBe(true);
	});

	it('places engaged-but-dropped rows AFTER the new parse output', () => {
		const prev: (ToyAction & ProposalFlags)[] = [
			{ id: 'a', kind: 'archive', applied: true },
			{ id: 'b', kind: 'snooze' } // pending, will be dropped
		];
		const out = mergeProposals<ToyAction>(prev, [{ id: 'c', kind: 'set', arg: 1 }], k);
		expect(out.map((r) => r.id)).toEqual(['c', 'a']);
	});

	it('keyFn determines identity: same id, different kind = two rows', () => {
		const prev: (ToyAction & ProposalFlags)[] = [
			{ id: 'a', kind: 'set', applied: true }
		];
		const out = mergeProposals<ToyAction>(prev, [{ id: 'a', kind: 'archive' }], k);
		expect(out).toHaveLength(2); // the new 'archive' + the frozen 'set'
		expect(out.map((r) => r.kind)).toEqual(['archive', 'set']);
	});
});

describe('extractActions', () => {
	const ok = '{"actions":[{"id":"a","kind":"archive"}]}';

	it('parses a clean response', () => {
		expect(extractActions(ok)).toEqual([{ id: 'a', kind: 'archive' }]);
	});

	it('strips ```json fences', () => {
		expect(extractActions('```json\n' + ok + '\n```')).toHaveLength(1);
	});

	it('slices JSON out of trailing prose', () => {
		expect(extractActions('Here you go:\n' + ok + '\nLet me know.')).toHaveLength(1);
	});

	it('returns [] on garbage', () => {
		expect(extractActions('')).toEqual([]);
		expect(extractActions('not json')).toEqual([]);
		expect(extractActions('{not closed')).toEqual([]);
	});

	it('returns [] when actions is missing or not an array', () => {
		expect(extractActions('{"foo":1}')).toEqual([]);
		expect(extractActions('{"actions":"nope"}')).toEqual([]);
	});

	it('preserves entries verbatim — no validation here', () => {
		// extractActions is the SHAPE step; per-entity validators
		// drop malformed entries. So a row missing required fields
		// still surfaces here.
		expect(extractActions('{"actions":[{"foo":"bar"},{}]}')).toEqual([{ foo: 'bar' }, {}]);
	});
});
