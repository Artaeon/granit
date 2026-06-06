// Two AI-overlay seeders for ProjectDetail.
//
// Both compose the same context (name + status + description + next
// action + open-task count + linked goals) but frame the closing
// question differently:
//
//   • askAIAboutProject   — action-oriented: "what would help me
//                            move this forward?". One-shot question;
//                            the user reads and continues.
//
//   • openProjectResearch — exploration-oriented: "help me think
//                            about this. what angles haven't I
//                            considered?". Pins the overlay as a side
//                            rail so it stays visible while the user
//                            navigates project notes / tasks /
//                            deadlines.
//
// Pure compose + dispatch. No state; the controllers above own the
// data, this just glues it into a chat seed and opens the overlay.

import type { Goal, Project, Task } from '$lib/api';
import { openAIOverlay, aiOverlayPinned } from '$lib/stores/ai-overlay';

export interface ProjectAISeedDeps {
  project: Project;
  projectTasks: Task[];
  linkedGoals: Goal[];
}

/** Open the AI overlay pre-seeded with this project's context.
 *  Action-framed close: "what would help me move this forward?". */
export function askAIAboutProject(deps: ProjectAISeedDeps): void {
  const { project, projectTasks, linkedGoals } = deps;
  const lines = [`I'm working on this project:`, '', `- ${project.name}`];
  if (project.status) lines.push(`- status: ${project.status}`);
  if (project.description && project.description.trim() !== '') {
    lines.push(`- description: ${project.description.trim()}`);
  }
  if (project.next_action && project.next_action.trim() !== '') {
    lines.push(`- next action: ${project.next_action.trim()}`);
  }
  const openTasks = projectTasks.filter((t) => !t.done);
  if (openTasks.length > 0) {
    lines.push(`- ${openTasks.length} open task${openTasks.length === 1 ? '' : 's'}`);
  }
  if (linkedGoals.length > 0) {
    const titles = linkedGoals.map((g) => g.title).join('; ');
    lines.push(`- linked goals: ${titles}`);
  }
  lines.push('', `What would help me move this forward?`);
  openAIOverlay({ text: lines.join('\n'), send: false });
}

/** Open the AI overlay pre-seeded with this project's context AND
 *  pin it as a side rail so it stays visible while the user
 *  navigates project notes / tasks / deadlines. Exploration-framed
 *  close: "help me think about this. What angles haven't I
 *  considered?". */
export function openProjectResearch(deps: ProjectAISeedDeps): void {
  const { project, projectTasks, linkedGoals } = deps;
  const lines = [
    `I'm in research mode on this project:`,
    '',
    `- ${project.name}`
  ];
  if (project.status) lines.push(`- status: ${project.status}`);
  if (project.description && project.description.trim() !== '') {
    lines.push(`- description: ${project.description.trim()}`);
  }
  if (project.category) lines.push(`- category: ${project.category}`);
  const openTasks = projectTasks.filter((t) => !t.done);
  if (openTasks.length > 0) {
    lines.push(`- ${openTasks.length} open task${openTasks.length === 1 ? '' : 's'}`);
  }
  if (linkedGoals.length > 0) {
    const titles = linkedGoals.map((g) => g.title).join('; ');
    lines.push(`- linked goals: ${titles}`);
  }
  lines.push(
    '',
    `Help me think about this. What angles haven't I considered? What questions should I be asking? Don't rush to recommendations — explore with me.`
  );
  // Pin the overlay so it stays as a side rail while the user moves
  // through project notes / tasks / deadlines. The note editor and
  // project view both reserve space for the pinned rail via the
  // document.documentElement.ai-pinned class.
  aiOverlayPinned.set(true);
  openAIOverlay({ text: lines.join('\n'), send: false });
}
