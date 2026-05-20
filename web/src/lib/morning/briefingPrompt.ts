// Morning briefing prompt builder. Pure — easy to unit-test.
// The Svelte page imports the system prompt + the buildUserPrompt
// helper, then drives api.chatStream itself. Keeping the prompt
// shape here means a future product tweak ("brief should mention
// energy state too") gets covered by a unit test rather than
// silently drifting in the .svelte file.

import type { CalendarEvent, Task, Goal, Deadline } from '$lib/api';

export const BRIEFING_SYSTEM_PROMPT =
  'You are a brief, calm personal assistant writing a one-screen morning brief for the user. ' +
  'Voice: declarative, concrete, no corporate sludge. Style: like a friend who looked over your shoulder and noticed three things.\n\n' +
  'Output STRICTLY three short paragraphs. No headers, no bullets, no preamble, no sign-off. Total length 60-110 words.\n\n' +
  "  Paragraph 1 (1-2 sentences): The shape of the day. Number of meetings, the biggest one if any, energy peaks worth defending. If the calendar is empty, name that as opportunity.\n" +
  "  Paragraph 2 (1-2 sentences): What to focus on if only one thing got done. Pick from the open-tasks list — name it specifically, not \"the top task\". When nothing is urgent, suggest a focused outcome.\n" +
  "  Paragraph 3 (1 sentence): One thing to watch for or protect — e.g. a deadline closing in, a back-to-back stretch with no break, an active goal that hasn't moved.\n\n" +
  "Constraints: never invent meetings, tasks, or deadlines that aren't in the input. If the input is sparse, write a shorter brief — under 60 words is fine. No quotation marks around proper nouns. No exclamation marks.";

export interface BriefingInputs {
  todayISO: string;
  events: CalendarEvent[];
  tasks: Task[];
  goals: Goal[];
  /** Deadlines tagged with their days-out. Caller filters to the
   *  upcoming subset (typically ≤7 days). */
  deadlines: { d: Deadline; days: number }[];
}

/** Builds the compact markdown context block fed to the model.
 *  Caps each list to keep the prompt bounded — 8 events / 8 tasks /
 *  3 goals / 3 deadlines is plenty for a 100-word read.
 *
 *  Returns an empty string when there's nothing useful — caller can
 *  decide whether to skip the call entirely or show the no-data
 *  fallback prompt.
 */
export function buildBriefingUserPrompt(input: BriefingInputs): string {
  // Detect TRULY empty input — no events, no tasks, no goals, no
  // deadlines — and use the no-data fallback rather than emitting
  // a context block that's just "nothing scheduled" with no other
  // content. The fallback prompts the model to write a short calm
  // note instead of trying to summarise emptiness.
  const hasAnyData =
    input.events.length > 0 ||
    input.tasks.length > 0 ||
    input.goals.length > 0 ||
    input.deadlines.length > 0;

  const header = `It's the morning of ${input.todayISO}.\n\n`;
  if (!hasAnyData) {
    return (
      header +
      '(No data available — write a short calm note suggesting the user log a few tasks or events.)'
    );
  }

  const parts: string[] = [];

  if (input.events.length > 0) {
    const lines = input.events.slice(0, 8).map((e) => {
      const t = e.start
        ? new Date(e.start).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
            hour12: false
          })
        : 'all-day';
      const loc = e.location ? ` @ ${e.location}` : '';
      const kind = e.kind ? ` [${e.kind}]` : '';
      return `- ${t} · ${e.title}${kind}${loc}`;
    });
    parts.push(`Today's calendar:\n${lines.join('\n')}`);
  } else {
    // We have tasks/goals/deadlines but no calendar — name it as
    // an open day rather than a missing data point. Keeps the
    // first paragraph from the model honest.
    parts.push("Today's calendar: nothing scheduled.");
  }

  if (input.tasks.length > 0) {
    const lines = input.tasks.slice(0, 8).map((t) => {
      const p = t.priority > 0 ? `P${t.priority} ` : '';
      const due = t.dueDate ? ` (due ${t.dueDate})` : '';
      const est = t.estimatedMinutes ? ` ~${t.estimatedMinutes}m` : '';
      return `- ${p}${t.text}${due}${est}`;
    });
    parts.push(`Top open tasks:\n${lines.join('\n')}`);
  }

  if (input.goals.length > 0) {
    parts.push(
      `Active goals:\n${input.goals.slice(0, 3).map((g) => `- ${g.title}`).join('\n')}`
    );
  }

  if (input.deadlines.length > 0) {
    parts.push(
      `Upcoming deadlines (≤7d):\n${input.deadlines
        .slice(0, 3)
        .map(({ d, days }) => `- ${d.title} in ${days}d (${d.importance})`)
        .join('\n')}`
    );
  }

  return header + parts.join('\n\n');
}
