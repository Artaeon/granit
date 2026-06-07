// Quick-jot composer + AI-expand state.
//
// Fourth extraction step out of routes/jots/+page.svelte. Owns the
// "fire a thought into today" textarea state, the expand-on-save
// toggle persisted to localStorage, the AI-expand streaming
// pipeline with Keep/Discard preview, and the submit path that
// splices the entry under a `## Jots` section in today's daily.
//
// The page still owns:
//  - the JotsComposer chrome (text/buttons),
//  - the shortcut-triggered focus("composer"),
//  - the WS-driven feed refetch (scheduleRefetch happens here too,
//    via the feed dep, so today's daily reloads optimistically).
//
// AbortError on the expand stream is filtered so the user-initiated
// Discard stays silent. cancel-on-discard flips expanding + nulls
// the abort handle synchronously so the chrome's Keep/Discard pair
// disappears instantly without waiting for the fetch loop to unwind.

import { api } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { isAbortError } from '$lib/util/aiErrors';

const EXPAND_KEY = 'granit.jots.composerExpand';

export interface JotsComposerDeps {
  /** Schedule a per-date debounced refetch after a save commit so
   *  today's daily reflects the new entry immediately, without
   *  waiting on the WS round-trip. */
  scheduleRefetch: (date: string) => void;
  /** Toast hooks — injected so the controller stays pure-state. */
  toastSuccess: (msg: string) => void;
  toastError: (msg: string) => void;
}

export interface JotsComposerController {
  text: string;
  composerEl: HTMLTextAreaElement | undefined;
  readonly busy: boolean;
  expand: boolean;
  readonly expanding: boolean;
  readonly expandedText: string;

  submit(opts?: { skipExpand?: boolean }): Promise<void>;
  runExpand(): Promise<void>;
  /** Abort the in-flight expand stream and drop the preview; the
   *  raw composer text stays so the user can re-edit it. */
  discardExpand(): void;
  /** Commit the streamed expansion through the normal submit path. */
  keepExpand(): Promise<void>;
  /** Focus the textarea — page keyboard shortcuts call this for `c`. */
  focusComposer(): void;
}

// Splice a single line into today's daily under a `## Jots` section.
// Creates the section if missing. Pure string transform — kept here
// (not in $lib/util) because no other surface needs it and the
// section-name convention is jot-specific.
function appendUnderJotsSection(body: string, line: string): string {
  const lines = body.split('\n');
  const idx = lines.findIndex((l) => /^##\s+Jots\b/i.test(l.trim()));
  if (idx === -1) {
    const sep = body.endsWith('\n') ? '' : '\n';
    return body + `${sep}\n## Jots\n${line}\n`;
  }
  // Walk past the heading to the end of the section (next `## ` or EOF).
  let end = lines.length;
  for (let i = idx + 1; i < lines.length; i++) {
    if (/^##\s+/.test(lines[i].trim())) {
      end = i;
      break;
    }
  }
  // Insert before `end`, trimming trailing empty lines so the new
  // line sits flush with the section content.
  let insertAt = end;
  while (insertAt > idx + 1 && lines[insertAt - 1].trim() === '') insertAt--;
  lines.splice(insertAt, 0, line);
  return lines.join('\n');
}

export function createJotsComposer(deps: JotsComposerDeps): JotsComposerController {
  let text = $state('');
  let busy = $state(false);
  let composerEl = $state<HTMLTextAreaElement | undefined>();

  // Persisted in localStorage so the user's expand preference
  // survives reloads. Read once on init; the $effect below writes
  // through on every toggle.
  let expand = $state<boolean>(
    typeof window !== 'undefined' && window.localStorage.getItem(EXPAND_KEY) === '1'
  );
  let expanding = $state(false);
  let expandedText = $state('');
  let expandAbort: AbortController | null = null;

  $effect(() => {
    if (typeof window === 'undefined') return;
    try { window.localStorage.setItem(EXPAND_KEY, expand ? '1' : '0'); } catch {
      // localStorage can throw (private mode, quota) — silently drop.
    }
  });

  async function runExpand() {
    const raw = text.trim();
    if (!raw || expanding) return;
    expandAbort?.abort();
    expandAbort = new AbortController();
    expanding = true;
    expandedText = '';
    const system =
      'You expand a user\'s terse journal note into a richer entry suitable for a daily ' +
      'log. Preserve every fact and feeling the user wrote — don\'t invent details or ' +
      'embellish. Add gentle scaffolding: link related ideas the user mentioned, expand ' +
      'shorthand, write in the user\'s voice. Return the expanded entry as markdown. ' +
      'Aim for 2-4 short paragraphs or a bullet list, depending on what fits. No preamble.';
    const user = 'Terse note:\n```\n' + raw + '\n```';
    try {
      const t = rafThrottle((full) => {
        if (expandAbort?.signal.aborted) return;
        expandedText = full;
      });
      await api.chatStream(
        [{ role: 'system', content: system }, { role: 'user', content: user }],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => { t.flush(); },
          onError: (err) => {
            t.flush();
            if (isAbortError(err)) return;
            deps.toastError('expand failed: ' + err.message);
            expandedText = '';
          }
        },
        expandAbort.signal
      );
    } finally {
      expanding = false;
      expandAbort = null;
    }
  }

  function discardExpand() {
    expandAbort?.abort();
    expandAbort = null;
    expanding = false;
    expandedText = '';
    composerEl?.focus();
  }

  async function keepExpand() {
    if (!expandedText.trim()) return;
    // Replace the raw composer text with the expanded version and
    // commit through the normal submit path. Saves duplicating the
    // appendUnderJotsSection / putNote / WS-refetch logic.
    text = expandedText.trim();
    expandedText = '';
    await submit({ skipExpand: true });
  }

  async function submit(opts: { skipExpand?: boolean } = {}) {
    const raw = text.trim();
    if (!raw || busy) return;
    // If expand is on and we haven't yet expanded this draft, kick off
    // the AI and STOP — the user gets a preview to review before any
    // save hits the daily note.
    if (expand && !opts.skipExpand) {
      runExpand();
      return;
    }
    busy = true;
    try {
      const note = await api.daily('today');
      const t = new Date();
      const hh = String(t.getHours()).padStart(2, '0');
      const mm = String(t.getMinutes()).padStart(2, '0');
      // Multi-line input collapses to "; " separators so the appended
      // line stays a single bullet. Original line breaks are preserved
      // by markdown viewers since the line ends with a bullet.
      const flat = raw.replace(/\n+/g, '; ');
      const newBody = appendUnderJotsSection(note.body ?? '', `- ${hh}:${mm} — ${flat}`);
      await api.putNote(note.path, {
        frontmatter: note.frontmatter ?? undefined,
        body: newBody
      });
      text = '';
      deps.toastSuccess('jot saved');
      // WS will re-fetch; queue an immediate optimistic refetch too in
      // case the WS round-trip lags.
      const today = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
      deps.scheduleRefetch(today);
      composerEl?.focus();
    } catch (e) {
      deps.toastError('failed to add jot: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  function focusComposer() {
    composerEl?.focus();
  }

  return {
    get text() { return text; },
    set text(v) { text = v; },
    get composerEl() { return composerEl; },
    set composerEl(v) { composerEl = v; },
    get busy() { return busy; },
    get expand() { return expand; },
    set expand(v) { expand = v; },
    get expanding() { return expanding; },
    get expandedText() { return expandedText; },

    submit,
    runExpand,
    discardExpand,
    keepExpand,
    focusComposer
  };
}
