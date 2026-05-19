// Pure helpers for the weekly-plan extract → review → commit flow.
// Extracted from /plans/week/+page.svelte so the routing rules
// (default-accept threshold, bucket order, commit-item shape) get
// a tested seam instead of living inside the page's $derived/$state
// soup.
//
// The page still owns the API roundtrips and the UI; everything
// below is shape-in, shape-out so the assertions don't need a DOM.

import type { PlanCommitItem, PlanExtractedItem } from '$lib/api';

// Per-item editable state. Mirrors the inline `ItemEdit` shape
// /plans/week used to declare; exported so the tests can construct
// edits maps directly.
export type ItemEdit = {
  accepted: boolean;
  label: string;
  venture: string;
  project: string;
  goalId: string;
  dueDate: string;
};

// Fuzzy-match confidence threshold above which the page auto-ticks
// the proposed item. Tuned for "the AI was pretty sure" — anything
// below this requires a deliberate user accept.
export const FUZZY_AUTO_ACCEPT_THRESHOLD = 80;

// Decide whether to default-tick an extracted item. Exact matches
// always go in; fuzzy matches above the threshold do too. Everything
// else (new venture, personal, low-confidence fuzzy) waits for the
// user to flip the checkbox.
export function defaultAccept(item: PlanExtractedItem): boolean {
  if (item.match_type === 'exact') return true;
  const conf = item.match_confidence ?? 0;
  return item.match_type === 'fuzzy' && conf >= FUZZY_AUTO_ACCEPT_THRESHOLD;
}

// Build the initial edits map from a freshly-extracted proposal.
// Keys are the proposal index (the same one the bucket grouping +
// the commit step reference).
export function buildInitialEdits(items: PlanExtractedItem[]): Record<number, ItemEdit> {
  const out: Record<number, ItemEdit> = {};
  for (let i = 0; i < items.length; i++) {
    const it = items[i];
    out[i] = {
      accepted: defaultAccept(it),
      label: it.label,
      venture: it.venture_name ?? '',
      project: it.project_name ?? '',
      goalId: it.goal_id ?? '',
      dueDate: it.due_date ?? ''
    };
  }
  return out;
}

// Bucket items by venture for the review UI. Empty-venture items
// land in a single "" bucket that the page renders as "Personal".
// Order: every named venture alphabetically, then Personal last so
// the user's "real" work shows up first when they scan the column.
export type Bucket = { venture: string; idxs: number[] };

export function groupByVenture(items: PlanExtractedItem[]): Bucket[] {
  const map = new Map<string, number[]>();
  for (let i = 0; i < items.length; i++) {
    const v = items[i].venture_name ?? '';
    if (!map.has(v)) map.set(v, []);
    map.get(v)!.push(i);
  }
  return [...map.entries()]
    .map(([venture, idxs]) => ({ venture, idxs }))
    .sort((a, b) => {
      if (a.venture === '' && b.venture !== '') return 1;
      if (b.venture === '' && a.venture !== '') return -1;
      return a.venture.localeCompare(b.venture);
    });
}

// Build the commit payload from the proposal + the user's edits.
// User edits win over the AI's original on label / venture / project
// / goalId / dueDate; the AI-only fields (match_type, match_confidence,
// rationale) drop off because the user has now committed to the
// routing. Items the user didn't tick are filtered out.
export function buildCommitItems(
  items: PlanExtractedItem[],
  edits: Record<number, ItemEdit>
): PlanCommitItem[] {
  const out: PlanCommitItem[] = [];
  for (let i = 0; i < items.length; i++) {
    const e = edits[i];
    if (!e?.accepted) continue;
    const orig = items[i];
    out.push({
      kind: orig.kind,
      label: e.label.trim() || orig.label,
      venture_name: e.venture || undefined,
      project_name: e.project || undefined,
      goal_id: e.goalId || undefined,
      due_date: e.dueDate || undefined,
      source_line: orig.source_line
    });
  }
  return out;
}
