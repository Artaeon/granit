// Pure formatters for the AI overlay's four "quick actions" — daily
// briefing, weekly synopsis, inbox triage, deadline detection.
//
// The orchestration (busy flag, abort controller, message-clear) stays
// in AIOverlay.svelte because it's tied to the component's reactive
// $state. What lives here is the response-shape → markdown formatter
// for the two list-style results (triage + deadlines) and the labels
// that pair API calls with display titles. Both call sites used
// inline templates; pulling them out makes the empty-state copy
// uniform and tunable without going through the god file.

export interface TriageProposal {
  id: string;
  priority: number;
  schedule: string;
  rationale: string;
}

export interface DeadlineProposal {
  id: string;
  due_date: string;
  rationale: string;
}

/** Render the triage response as a markdown list. Empty list collapses
 *  to a single-italic line so the overlay shows _something_ rather
 *  than a blank quick-result body. */
export function renderTriageProposals(props: ReadonlyArray<TriageProposal>): string {
  if (props.length === 0) return '_No untriaged tasks to review._';
  const lines = props.map(
    (p) => `- **${p.priority === 0 ? 'drop' : `P${p.priority}`}** · ${p.schedule} · ${p.rationale} _(${p.id})_`
  );
  return `${lines.length} suggestion${lines.length === 1 ? '' : 's'} — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
}

/** Same shape for deadline-detect output. */
export function renderDeadlineProposals(props: ReadonlyArray<DeadlineProposal>): string {
  if (props.length === 0) return '_No clear deadlines detected._';
  const lines = props.map((p) => `- **${p.due_date}** · ${p.rationale} _(${p.id})_`);
  return `${lines.length} deadline${lines.length === 1 ? '' : 's'} detected — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
}

/** Title labels used in the quick-result header. Centralised so the
 *  /briefing slash command and the toolbar button can't drift. */
export const QUICK_ACTION_TITLES = {
  briefing: 'Daily briefing',
  synopsis: 'Weekly synopsis',
  triage: 'Inbox triage',
  deadlines: 'Detect deadlines'
} as const;
