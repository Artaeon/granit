// "Research Mode" pins the AI overlay as a side rail and seeds it
// with the current note's title + tags + a body excerpt, framed as
// exploration rather than action. Mirrors the same button on
// ProjectDetail; the excerpt cap balances giving the AI enough
// grounding without burning context budget the rest of the
// conversation needs.

import type { Note } from '$lib/api';
import { aiOverlayPinned, openAIOverlay } from '$lib/stores/ai-overlay';

/** Cap on the body excerpt seeded into the AI overlay. Bigger
 *  excerpts pay for themselves at the model tier but cost context
 *  budget the rest of the conversation needs. */
export const RESEARCH_EXCERPT_MAX = 800;

export function openResearchMode(note: Note, body: string): void {
  // Trim FIRST, then slice. The truncation marker should reflect
  // whether the meaningful content was cut, not whether the raw
  // body (possibly with leading/trailing whitespace) crosses the
  // cap.
  const trimmed = (body ?? '').trim();
  const excerpt = trimmed.slice(0, RESEARCH_EXCERPT_MAX);
  const truncated = trimmed.length > RESEARCH_EXCERPT_MAX;
  const lines = [
    `I'm in research mode on this note:`,
    '',
    `- ${note.title || note.path}`
  ];
  const tags = note.tags ?? [];
  if (tags.length > 0) lines.push(`- tags: ${tags.map((t) => '#' + t).join(' ')}`);
  if (excerpt) {
    lines.push('', 'Excerpt:', excerpt + (truncated ? '…' : ''));
  }
  lines.push(
    '',
    `Help me think about this. What angles haven't I considered? What questions should I be asking? Don't rush to recommendations — explore with me.`
  );
  aiOverlayPinned.set(true);
  openAIOverlay({ text: lines.join('\n'), send: false });
}
