// Context-aware quick-action chip prompts for the AI overlay.
//
// The chip row above the composer shows different prompts depending
// on which entity the user is currently looking at:
//   - Project page  → Draft brief / Status update / Brainstorm / What's next?
//   - Calendar page → Week shape / Find focus block / Overdue triage / Clear one meeting
//   - Goal focused  → Review note / Reframe / Highest-leverage / New milestones
//
// Each prompt was an inline template literal inside AIOverlay.svelte;
// pulled into a single table here so the prompt copy is auditable +
// editable without scrolling through 100 lines of button markup.
//
// projectPrompts is a function so it can fold in the current project
// name (the same applies to onCalendarPage / currentGoalId, but those
// don't need interpolation — the chip prompts speak generically about
// "this goal" / "my week").

export interface ContextChip {
  label: string;
  prompt: string;
}

export function projectContextChips(projectName: string): ContextChip[] {
  return [
    {
      label: 'Draft brief',
      prompt: `Draft a one-page project brief for ${projectName} — Why · Scope · Out of scope · Definition of done · Stakeholders. Markdown, paste-ready.`
    },
    {
      label: 'Status update',
      prompt: `Write a crisp status update for ${projectName} — what shipped, what's open, what's blocked, what's next. 1 short paragraph, no filler.`
    },
    {
      label: 'Brainstorm',
      prompt: `Brainstorm 3-5 distinct directions for ${projectName}'s next milestone. For each: the move, the main risk, what would prove or kill it.`
    },
    {
      label: `What's next?`,
      prompt: `Looking at the open tasks + linked goals on ${projectName}, what's the ONE thing I should do next, and why? Pick one, defend it briefly.`
    }
  ];
}

export const CALENDAR_CONTEXT_CHIPS: ReadonlyArray<ContextChip> = [
  {
    label: 'Week shape',
    prompt: `Describe what my week looks like — heaviest day, lightest day, where the deep-work blocks are or aren't, what's the dominant theme.`
  },
  {
    label: 'Find focus block',
    prompt: `Find me a 2-hour focus block in the next 5 days. Propose ONE specific day + start time + reasoning. Don't list options.`
  },
  {
    label: 'Overdue triage',
    prompt: `What's overdue and worth doing vs. worth declaring dead? Walk me through it.`
  },
  {
    label: 'Clear one meeting',
    prompt: `If I had to clear one meeting from this week to protect a deep-work block, which one and why? Name the trade-off explicitly.`
  }
];

export const GOAL_CONTEXT_CHIPS: ReadonlyArray<ContextChip> = [
  {
    label: 'Review note',
    prompt: `Write a goal review note for this goal — progress so far, what's working, what's stuck, what to change. 1 short paragraph each section.`
  },
  {
    label: 'Reframe',
    prompt: `Reframe this goal sharper — what does success look like specifically, by when, and how will I know I've hit it?`
  },
  {
    label: 'Highest-leverage next step',
    prompt: `Looking at the open tasks attached to this goal, which ONE moves it forward most this week? Pick one, defend it briefly.`
  },
  {
    label: 'New milestones',
    prompt: `Brainstorm 3-5 new milestones for this goal — concrete checkpoints I'd accept as proof of progress. For each: outcome statement + how I'd measure it.`
  }
];
