// Pure helpers for the two "save chat → markdown note" flows in the
// AI overlay:
//
//   1. saveThreadAsNote — write the entire conversation to a
//      chat-history/YYYY-MM-DD-HHmm-<slug>.md note.
//   2. saveAssistantAsNote — write a single assistant reply to a
//      folder chosen by the active context (Projects / Goals /
//      Calendar / Drafts).
//
// The helpers below produce the path + frontmatter + body strings;
// the actual createNote call stays in the component since (a) it
// owns the toast surface for success/failure and (b) the
// path-collision retry mid-flow needs awareness of the network
// shape (409 vs anything else). Splitting the formatting out gets
// the cohesive ~150 LOC of pure string-building off AIOverlay.svelte
// while keeping the orchestration honest.

import type { ChatMessage } from '$lib/api';
import { slugifyTitle } from '$lib/util/slug';
import type { RagHit } from '$lib/chat/rag';

// ── Save whole thread ──────────────────────────────────────────

export interface SaveThreadInputs {
  messages: ReadonlyArray<ChatMessage>;
  quickTitle: string;
  quickResult: string;
  modeId: string;
  modeLabel: string;
  rag: boolean;
  lastRagHits: ReadonlyArray<RagHit>;
  /** Used for the path stem + frontmatter `captured_at`. The caller
   *  passes new Date() so the helper stays time-injectable. */
  now: Date;
}

export interface SaveThreadResult {
  path: string;
  frontmatter: Record<string, unknown>;
  body: string;
}

/** Build the markdown + path payload for the whole-thread save.
 *  Returns null when there's nothing worth saving (no messages and
 *  no quick-result), so the caller can short-circuit with an
 *  "Nothing to save" toast. */
export function buildSaveThreadPayload(inp: SaveThreadInputs): SaveThreadResult | null {
  if (inp.messages.length === 0 && !inp.quickResult) return null;
  const yyyy = inp.now.getFullYear();
  const mm = String(inp.now.getMonth() + 1).padStart(2, '0');
  const dd = String(inp.now.getDate()).padStart(2, '0');
  const hh = String(inp.now.getHours()).padStart(2, '0');
  const mi = String(inp.now.getMinutes()).padStart(2, '0');
  const firstUser =
    inp.messages.find((m) => m.role === 'user')?.content ?? inp.quickTitle ?? 'chat';
  const slug = slugifyTitle(firstUser) || 'chat';
  const path = `chat-history/${yyyy}-${mm}-${dd}-${hh}${mi}-${slug}.md`;
  const lines: string[] = [
    '# ' + (firstUser.length > 80 ? firstUser.slice(0, 80) + '…' : firstUser),
    '',
    `> mode: **${inp.modeLabel}** · ${inp.rag ? 'RAG on' : 'RAG off'} · captured ${inp.now.toLocaleString()}`,
    ''
  ];
  if (inp.quickResult) {
    lines.push('## ' + (inp.quickTitle || 'Quick result'), '', inp.quickResult, '');
  }
  for (const m of inp.messages) {
    lines.push(m.role === 'user' ? '## You' : '## Assistant', '', m.content, '');
  }
  if (inp.lastRagHits.length > 0) {
    lines.push('## Sources retrieved', '');
    for (const h of inp.lastRagHits) lines.push(`- [[${h.path}|${h.title}]]`);
  }
  const frontmatter = {
    type: 'chat',
    mode: inp.modeId,
    rag: inp.rag,
    captured_at: inp.now.toISOString(),
    tags: ['chat', inp.modeId]
  };
  return { path, frontmatter, body: lines.join('\n') };
}

// ── Save single assistant reply ────────────────────────────────

export interface SaveAssistantPayloadInputs {
  cleanedContent: string;
  title: string;
  modeId: string;
  currentProjectName: string;
  currentGoalId: string;
  onCalendarPage: boolean;
}

export interface AssistantNotePayload {
  basePath: string;
  /** Folder portion of basePath — used by the path-collision retry
   *  to construct `${folder}/${slug}-${HHmm}.md` without re-deriving. */
  folder: string;
  baseSlug: string;
  frontmatter: Record<string, unknown>;
}

/** Build the path + frontmatter for the per-reply save. The body is
 *  the caller's `cleanedContent` (already stripped of structured
 *  blocks), so we don't move it through here. */
export function buildAssistantNotePayload(
  inp: SaveAssistantPayloadInputs
): AssistantNotePayload {
  // Folder picked by context. Project takes precedence (Projects/<name>
  // is where its notes tab looks); goal-mode drafts land in a
  // goal-scoped subfolder; calendar drafts go to Calendar/Drafts;
  // everything else lands under Drafts/.
  const folder = inp.currentProjectName
    ? `Projects/${slugifyTitle(inp.currentProjectName) || inp.currentProjectName}`
    : inp.currentGoalId
    ? `Goals/${slugifyTitle(inp.currentGoalId) || inp.currentGoalId}`
    : inp.onCalendarPage
    ? 'Calendar/Drafts'
    : 'Drafts';
  const baseSlug = slugifyTitle(inp.title) || 'draft';
  const basePath = `${folder}/${baseSlug}.md`;
  // Cross-link frontmatter — carries the source context so the
  // saved note can render a "from chat about X" badge AND so the
  // project's notes tab (or future goal/calendar notes surfaces)
  // can list AI drafts even when they live outside the entity's
  // natural folder. project / goal / calendar are mutually
  // exclusive in autoMode but the user might also be in a
  // contextual mode manually — capture what's actually in scope.
  const frontmatter: Record<string, unknown> = {
    type: 'ai-draft',
    mode: inp.modeId,
    captured_at: new Date().toISOString(),
    tags: ['ai-draft', inp.modeId]
  };
  if (inp.currentProjectName) frontmatter.project = inp.currentProjectName;
  if (inp.currentGoalId) frontmatter.goal = inp.currentGoalId;
  if (inp.onCalendarPage) frontmatter.calendar_window = true;
  return { basePath, folder, baseSlug, frontmatter };
}

/** Build the time-suffixed retry path used when basePath already
 *  exists (409 Conflict). `${folder}/${baseSlug}-${HHmm}.md`. */
export function buildAssistantNoteRetryPath(
  folder: string,
  baseSlug: string,
  now: Date
): string {
  const suffix = `${String(now.getHours()).padStart(2, '0')}${String(now.getMinutes()).padStart(2, '0')}`;
  return `${folder}/${baseSlug}-${suffix}.md`;
}
