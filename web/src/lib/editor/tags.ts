// Tag autocomplete: type `#` followed by a partial tag name and the
// editor suggests existing vault tags. Backed by /api/v1/tags so the
// pool reflects what the user has actually been writing — typos and
// stylistic variants surface immediately rather than days later when
// they spot the orphan tag in /tags.

import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { api } from '$lib/api';

interface Tag {
  tag: string;
  count: number;
}

// Cache vault tags for a short window so a long editing session
// doesn't hammer /api/v1/tags on every keystroke. WS events
// (note.changed) elsewhere in the app trigger tags refreshes; for
// editor autocomplete a 60-second TTL is plenty — new tags the user
// just typed will surface on the next refresh and stale tags they
// removed don't matter (auto-complete suggesting a removed tag is
// still semantically valid — the user can re-introduce it).
let cache: { fetched: number; tags: Tag[] } | null = null;
const TTL_MS = 60_000;

async function getTags(): Promise<Tag[]> {
  const now = Date.now();
  if (cache && now - cache.fetched < TTL_MS) return cache.tags;
  try {
    const r = await api.listTags();
    cache = { fetched: now, tags: r.tags };
    return r.tags;
  } catch {
    return cache?.tags ?? [];
  }
}

export function invalidateTagsCache() {
  cache = null;
}

// `#` followed by tag-character set. We allow [A-Za-z0-9_/-] which
// matches the parser's reTagInLine in internal/tasks/parser.go so a
// suggestion accepted here is a tag the rest of granit will recognize.
//
// The lookbehind avoids false-positives in headings (`# Heading`):
// the trigger requires either the document start or whitespace
// immediately before the '#'. A markdown heading's '#' has no
// whitespace before it (it sits at column 0 with following content),
// but the FOLLOWING content there is " text" not a tag character —
// so even without the lookbehind the trigger pattern wouldn't fire on
// a heading. We keep the (^|\s) guard anyway for clarity and to
// reject `foo#bar` shapes.
const triggerRe = /(?:^|\s)(#[A-Za-z0-9_/-]*)$/;

export async function tagComplete(ctx: CompletionContext): Promise<CompletionResult | null> {
  const before = ctx.state.sliceDoc(Math.max(0, ctx.pos - 64), ctx.pos);
  const m = triggerRe.exec(before);
  if (!m) return null;
  const typed = m[1]; // includes the leading '#'
  const from = ctx.pos - typed.length;

  // Only fetch when the user has typed something past the '#'. A bare
  // '#' shouldn't blast the user with a tag dump — most '#' presses
  // are for headings, not tags. A single character of partial filter
  // is the right floor: shows after `#a`, not `#`.
  if (typed.length < 2) return null;

  const tags = await getTags();
  const partial = typed.slice(1).toLowerCase();
  const filtered = tags
    .filter((t) => t.tag.toLowerCase().includes(partial))
    .sort((a, b) => {
      // Prefix matches first, then by use-frequency desc, then alpha.
      const ap = a.tag.toLowerCase().startsWith(partial) ? 0 : 1;
      const bp = b.tag.toLowerCase().startsWith(partial) ? 0 : 1;
      if (ap !== bp) return ap - bp;
      if (a.count !== b.count) return b.count - a.count;
      return a.tag.localeCompare(b.tag);
    })
    .slice(0, 20);

  if (filtered.length === 0) return null;

  return {
    from,
    options: filtered.map((t) => ({
      label: '#' + t.tag,
      detail: `${t.count} note${t.count === 1 ? '' : 's'}`,
      type: 'keyword'
    })),
    validFor: /^#[A-Za-z0-9_/-]*$/
  };
}
