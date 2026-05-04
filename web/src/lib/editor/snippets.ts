// Snippet autocomplete: type `/` then a name, accept the suggestion,
// and the slash-trigger is replaced with the expanded content (with
// {{date}}/{{time}}/{{datetime}} placeholders substituted client-side
// so the inserted text reflects the user's actual now, not the
// server's timestamp). The list itself is fetched once per editor
// session from /api/v1/snippets — same source the TUI reads, so
// adding a snippet on the server side surfaces in both surfaces with
// no client edit.

import type { CompletionContext, CompletionResult, Completion } from '@codemirror/autocomplete';
import { api } from '$lib/api';

interface Snippet {
  trigger: string; // e.g. "/meeting"
  description: string;
  content: string; // may contain {{date}} / {{time}} / {{datetime}}
}

// Cache snippets at module scope so multiple editor instances share
// one fetch. The list is small (~20 entries) and effectively static
// per server build — a fresh fetch on each editor mount would be
// wasteful. invalidateSnippetsCache() exists for the future
// user-snippets feature.
let cachedSnippets: Snippet[] | null = null;
let inFlight: Promise<Snippet[]> | null = null;

async function getSnippets(): Promise<Snippet[]> {
  if (cachedSnippets) return cachedSnippets;
  if (inFlight) return inFlight;
  inFlight = api
    .listSnippets()
    .then((r) => {
      cachedSnippets = r.snippets;
      return r.snippets;
    })
    .catch(() => [] as Snippet[])
    .finally(() => {
      inFlight = null;
    });
  return inFlight;
}

export function invalidateSnippetsCache() {
  cachedSnippets = null;
}

// Client-side placeholder substitution. Mirrors the Go-side
// ExpandPlaceholders so snippets read identically in both surfaces;
// duplicating the logic (rather than fetching pre-expanded content)
// means the timestamp reflects the moment of insertion, not the moment
// of the snippet list fetch — which matters for /datetime in long
// editor sessions.
function pad2(n: number) {
  return n < 10 ? '0' + n : '' + n;
}
function expandPlaceholders(content: string): string {
  const now = new Date();
  const date = `${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())}`;
  const time = `${pad2(now.getHours())}:${pad2(now.getMinutes())}`;
  return content
    .replaceAll('{{datetime}}', `${date} ${time}`)
    .replaceAll('{{date}}', date)
    .replaceAll('{{time}}', time)
    .replaceAll('{{title}}', '');
}

// Match a leading '/' optionally followed by [a-z0-9-]+ at the
// caret. Bounded character class so an arbitrary slash in code (e.g.
// "https://") doesn't trigger the picker — the editor only enters
// snippet-mode when the slash starts a fresh word.
const triggerRe = /(?:^|\s)(\/[a-z0-9-]*)$/i;

export async function snippetComplete(ctx: CompletionContext): Promise<CompletionResult | null> {
  const before = ctx.state.sliceDoc(Math.max(0, ctx.pos - 32), ctx.pos);
  const m = triggerRe.exec(before);
  if (!m) return null;
  const typed = m[1]; // includes the leading '/'
  // The completion's `from` is the position of the leading '/'.
  // ctx.pos is at the end of `typed` (the cursor); the slash sits
  // typed.length characters back from there.
  const from = ctx.pos - typed.length;

  // ctx.explicit means the user hit ctrl+space. Otherwise we only
  // surface options when at least the leading '/' is typed (always
  // true here) — so the picker shows immediately on '/' alone.
  const list = await getSnippets();
  const lower = typed.toLowerCase();
  const filtered = list.filter((s) => s.trigger.toLowerCase().startsWith(lower));
  if (filtered.length === 0) return null;

  const options: Completion[] = filtered.map((s) => ({
    label: s.trigger,
    detail: s.description,
    type: 'snippet',
    apply: (view, _completion, applyFrom, applyTo) => {
      // Replace the entire `/typed` prefix with the expanded content,
      // not just append — otherwise a partial trigger (`/me` accepting
      // /meeting) would leave the partial behind. CodeMirror passes
      // the actual matched range so we honor that, not the captured
      // `from` (which can drift if the doc changed mid-completion).
      const insert = expandPlaceholders(s.content);
      view.dispatch({
        changes: { from: applyFrom, to: applyTo, insert },
        selection: { anchor: applyFrom + insert.length }
      });
    }
  }));
  return { from, options, validFor: /^\/[a-z0-9-]*$/i };
}
