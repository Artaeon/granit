import { describe, expect, it } from 'vitest';
import type { PlanExtractedItem } from '$lib/api';
import {
  buildCommitItems,
  buildInitialEdits,
  defaultAccept,
  FUZZY_AUTO_ACCEPT_THRESHOLD,
  groupByVenture,
  type ItemEdit
} from './extractHelpers';

function item(
  partial: Partial<PlanExtractedItem> & { label: string }
): PlanExtractedItem {
  return {
    kind: partial.kind ?? 'task',
    label: partial.label,
    venture_name: partial.venture_name,
    project_name: partial.project_name,
    goal_id: partial.goal_id,
    due_date: partial.due_date,
    source_line: partial.source_line,
    match_type: partial.match_type,
    match_confidence: partial.match_confidence,
    rationale: partial.rationale
  };
}

describe('defaultAccept', () => {
  it('auto-accepts exact matches regardless of confidence', () => {
    expect(defaultAccept(item({ label: 'x', match_type: 'exact' }))).toBe(true);
    expect(defaultAccept(item({ label: 'x', match_type: 'exact', match_confidence: 0 }))).toBe(true);
  });

  it('auto-accepts fuzzy matches at or above the threshold', () => {
    expect(defaultAccept(item({ label: 'x', match_type: 'fuzzy', match_confidence: FUZZY_AUTO_ACCEPT_THRESHOLD }))).toBe(true);
    expect(defaultAccept(item({ label: 'x', match_type: 'fuzzy', match_confidence: 95 }))).toBe(true);
  });

  it('does not auto-accept fuzzy matches below the threshold', () => {
    expect(defaultAccept(item({ label: 'x', match_type: 'fuzzy', match_confidence: 79 }))).toBe(false);
    expect(defaultAccept(item({ label: 'x', match_type: 'fuzzy' }))).toBe(false);
  });

  it('leaves new + personal matches for the user to confirm', () => {
    expect(defaultAccept(item({ label: 'x', match_type: 'new' }))).toBe(false);
    expect(defaultAccept(item({ label: 'x', match_type: 'personal' }))).toBe(false);
  });
});

describe('buildInitialEdits', () => {
  it('seeds one entry per item, accepted iff defaultAccept passes', () => {
    const items = [
      item({ label: 'a', match_type: 'exact' }),
      item({ label: 'b', match_type: 'fuzzy', match_confidence: 60 }),
      item({ label: 'c', match_type: 'new', venture_name: 'Saas', project_name: 'V1' })
    ];
    const edits = buildInitialEdits(items);
    expect(edits[0].accepted).toBe(true);
    expect(edits[1].accepted).toBe(false);
    expect(edits[2].accepted).toBe(false);
    expect(edits[2].venture).toBe('Saas');
    expect(edits[2].project).toBe('V1');
  });

  it('defaults missing optional fields to empty strings', () => {
    const edits = buildInitialEdits([item({ label: 'a' })]);
    expect(edits[0]).toEqual({
      accepted: false,
      label: 'a',
      venture: '',
      project: '',
      goalId: '',
      dueDate: ''
    });
  });
});

describe('groupByVenture', () => {
  it('groups items under their venture name', () => {
    const items = [
      item({ label: 'a', venture_name: 'Alpha' }),
      item({ label: 'b', venture_name: 'Beta' }),
      item({ label: 'c', venture_name: 'Alpha' })
    ];
    const buckets = groupByVenture(items);
    expect(buckets.map((b) => b.venture)).toEqual(['Alpha', 'Beta']);
    expect(buckets[0].idxs).toEqual([0, 2]);
    expect(buckets[1].idxs).toEqual([1]);
  });

  it('places the no-venture bucket last and named ones alphabetical', () => {
    const items = [
      item({ label: 'a' }),                          // Personal
      item({ label: 'b', venture_name: 'Zeta' }),
      item({ label: 'c', venture_name: 'Alpha' })
    ];
    const buckets = groupByVenture(items);
    expect(buckets.map((b) => b.venture)).toEqual(['Alpha', 'Zeta', '']);
  });

  it('returns an empty array for an empty proposal', () => {
    expect(groupByVenture([])).toEqual([]);
  });
});

describe('buildCommitItems', () => {
  it('omits items the user did not tick', () => {
    const items = [
      item({ label: 'kept', match_type: 'exact' }),
      item({ label: 'dropped', match_type: 'fuzzy', match_confidence: 90 })
    ];
    const edits: Record<number, ItemEdit> = {
      0: { accepted: true, label: '', venture: '', project: '', goalId: '', dueDate: '' },
      1: { accepted: false, label: '', venture: '', project: '', goalId: '', dueDate: '' }
    };
    const out = buildCommitItems(items, edits);
    expect(out).toHaveLength(1);
    expect(out[0].label).toBe('kept');
  });

  it('prefers the user-edited label, falling back to the AI label when blank', () => {
    const items = [item({ label: 'AI label' })];
    const edits = {
      0: { accepted: true, label: '   ', venture: '', project: '', goalId: '', dueDate: '' }
    };
    expect(buildCommitItems(items, edits)[0].label).toBe('AI label');

    edits[0].label = 'user override';
    expect(buildCommitItems(items, edits)[0].label).toBe('user override');
  });

  it('drops blank optional fields to undefined so the wire payload stays tight', () => {
    const items = [item({ label: 'x', source_line: '- x' })];
    const edits = {
      0: { accepted: true, label: 'x', venture: '', project: '', goalId: '', dueDate: '' }
    };
    const out = buildCommitItems(items, edits)[0];
    expect(out.venture_name).toBeUndefined();
    expect(out.project_name).toBeUndefined();
    expect(out.goal_id).toBeUndefined();
    expect(out.due_date).toBeUndefined();
    expect(out.source_line).toBe('- x');
  });

  it('carries user edits through (venture, project, goal, due, kind)', () => {
    const items = [item({ label: 'a', kind: 'milestone' })];
    const edits = {
      0: {
        accepted: true,
        label: 'a',
        venture: 'V',
        project: 'P',
        goalId: 'g-1',
        dueDate: '2026-05-20'
      }
    };
    const out = buildCommitItems(items, edits)[0];
    expect(out).toEqual({
      kind: 'milestone',
      label: 'a',
      venture_name: 'V',
      project_name: 'P',
      goal_id: 'g-1',
      due_date: '2026-05-20',
      source_line: undefined
    });
  });
});
