// Frontmatter `tags:` lands as either an array (idiomatic YAML) or a
// comma/space-separated string (legacy / hand-typed). One parser keeps
// the derivation and the append-path agreeing on shape.

import type { Note } from '$lib/api';
import { toast } from '$lib/components/toast';

export function parseTagsField(fm: Record<string, unknown> | undefined | null): string[] {
  if (!fm) return [];
  const t = fm.tags;
  if (Array.isArray(t)) return t.map((x) => String(x));
  if (typeof t === 'string') return t.split(/[,\s]+/).filter(Boolean);
  return [];
}

export interface AddSuggestedTagCtx {
  note: Note;
  /** Persists the next frontmatter via the page's saveFrontmatter
   *  pipeline. Returns whether the save actually committed — a 412
   *  conflict swallows the error and routes through the banner, so
   *  we MUST only toast the chip-confirmation when the PUT lands. */
  saveFrontmatter: (next: Record<string, unknown>) => Promise<boolean>;
}

export async function addSuggestedTag(tag: string, ctx: AddSuggestedTagCtx): Promise<void> {
  const clean = tag.trim().replace(/^#/, '').toLowerCase();
  if (!clean) return;
  const fm = { ...(ctx.note.frontmatter ?? {}) } as Record<string, unknown>;
  const arr = parseTagsField(fm);
  if (arr.includes(clean)) {
    toast.success(`#${clean} already on this note`);
    return;
  }
  arr.push(clean);
  fm.tags = arr;
  if (await ctx.saveFrontmatter(fm)) toast.success(`+ #${clean}`);
}

export interface InsertSuggestedLinkCtx {
  insertAtCursor?: (text: string) => void;
  /** Fallback append path when the editor isn't mounted (e.g. preview
   *  view). Returns the new body and the caller is expected to mark
   *  the document dirty so autosave picks it up. */
  appendToBody: (markup: string) => void;
}

export function insertSuggestedLink(markup: string, ctx: InsertSuggestedLinkCtx): void {
  if (ctx.insertAtCursor) {
    ctx.insertAtCursor(' ' + markup + ' ');
  } else {
    ctx.appendToBody(markup);
  }
  toast.success('link inserted');
}
