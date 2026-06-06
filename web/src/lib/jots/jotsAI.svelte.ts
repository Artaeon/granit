// Multi-mode AI panel for the jots feed.
//
// Third extraction step out of routes/jots/+page.svelte. Owns the
// single AI surface that switches between three modes — theme
// detection, free-form Q&A, weekly digest — and the abort plumbing
// that guarantees only one stream runs at a time. Switching modes
// (or hitting Dismiss) aborts any in-flight stream and clears the
// previous result so the panel never shows two writers racing.
//
// Modes:
//   themes  — surface 3-5 recurring topics/people/projects across
//             the loaded jots. The model returns a JSON array; the
//             onDone hook parses + filters it into a Theme[] the
//             chrome renders as clickable search chips.
//   ask     — free-form question answered using the loaded jots as
//             context. Renders streaming markdown.
//   digest  — synthesis of the last 7 days of dailies into a
//             structured weekly summary. Save-as-note writes it to
//             `digest-YYYY-MM-DD.md` under the daily folder and
//             navigates to the new note.
//
// AbortError is filtered out of onError so a user-initiated cancel
// stays silent — the same pattern other Granit streamers (finance,
// tasks-agent) use. cancel() flips busy + nulls the abort handle
// synchronously so the toolbar button swaps from "Stop" to "Rerun"
// instantly without waiting for the fetch loop to unwind.

import { api, fmtDateISO, type Jot } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { isAbortError } from '$lib/util/aiErrors';

export type AIMode = 'none' | 'themes' | 'ask' | 'digest';
export type Theme = { label: string; query: string };

export interface JotsAIDeps {
  /** Reactive jots[] getter so prompts see the freshest loaded feed. */
  getJots: () => Jot[];
  /** Midnight today — anchor for the digest's 7-day window and the
   *  digest-as-note filename. */
  getToday: () => Date;
  /** Daily folder (may be ''); used to scope the digest-as-note path. */
  getDailyFolder: () => string;
  /** Apply a theme chip — set the page's search text and kick off
   *  a search. The page owns search state; the controller just
   *  delegates back. */
  applyThemeSearch: (query: string) => void;
  /** Toast hooks. Injected so the controller stays pure-state and
   *  testable without monkey-patching the toast singleton. */
  toastInfo: (msg: string) => void;
  toastSuccess: (msg: string) => void;
  toastError: (msg: string) => void;
  /** Navigate to a note path after digest-as-note. The page owns
   *  navigation so SSR / Sveltekit `goto` stays the only entrypoint. */
  navigate: (path: string) => void;
}

export interface JotsAIController {
  readonly mode: AIMode;
  readonly busy: boolean;
  readonly error: string;
  readonly themes: Theme[];
  askQuestion: string;
  readonly askAnswer: string;
  askInputEl: HTMLInputElement | undefined;
  readonly digestAnswer: string;

  detectThemes(): Promise<void>;
  startAsk(): void;
  submitAsk(): Promise<void>;
  buildDigest(): Promise<void>;
  saveDigestAsNote(): Promise<void>;
  applyTheme(t: Theme): void;
  copyToClipboard(text: string): Promise<void>;

  /** Abort the in-flight stream, keep partial output. Flips busy +
   *  nulls the abort handle synchronously so the chrome's
   *  Stop->Rerun toggle is instant. */
  cancel(): void;
  /** Abort + wipe every mode's state, return panel to none. */
  dismiss(): void;
}

function buildJotsSeed(jots: Jot[], limit = 30): string {
  // Cap at N jots × ~1200 chars each. The model needs enough signal
  // to spot recurrence without blowing the prompt out.
  const slice = jots.slice(0, limit).map((j) => ({
    date: j.date,
    body: (j.body ?? '').slice(0, 1200)
  }));
  return JSON.stringify(slice, null, 2);
}

export function createJotsAI(deps: JotsAIDeps): JotsAIController {
  let mode = $state<AIMode>('none');
  let busy = $state(false);
  let error = $state('');
  let abort: AbortController | null = null;
  let raw = $state('');

  let themes = $state<Theme[]>([]);
  let askQuestion = $state('');
  let askAnswer = $state('');
  let askInputEl = $state<HTMLInputElement | undefined>();
  let digestAnswer = $state('');

  function cancel() {
    abort?.abort();
    abort = null;
    busy = false;
  }
  function dismiss() {
    abort?.abort();
    abort = null;
    busy = false;
    mode = 'none';
    raw = '';
    error = '';
    themes = [];
    askAnswer = '';
    askQuestion = '';
    digestAnswer = '';
  }

  async function detectThemes() {
    const jots = deps.getJots();
    if (jots.length < 5) {
      deps.toastInfo('Load a few more jots first.');
      return;
    }
    dismiss();
    mode = 'themes';
    abort = new AbortController();
    busy = true;
    const seed = buildJotsSeed(jots);
    const system = 'You analyse recent daily-note entries and surface 3-5 recurring themes. A theme is a topic, person, project, struggle, or joy that shows up across multiple entries. Return STRICTLY a JSON array, no fences, no prose: [{"label": "<short title, 1-3 words, lowercase>", "query": "<single-word search term that finds the theme>"}]. Pick search terms that actually appear in the entries (a hashtag, a name, a recurring word) — not synonyms.';
    const user = `Recent jots:\n\`\`\`json\n${seed}\n\`\`\`\n\nGive me 3-5 themes.`;
    try {
      const t = rafThrottle((full) => {
        if (abort?.signal.aborted) return;
        raw = full;
      });
      await api.chatStream(
        [{ role: 'system', content: system }, { role: 'user', content: user }],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => {
            t.flush();
            // After user-stop, skip parse — partial JSON would
            // surface as "Model didn't return parseable JSON."
            // and overwrite themes.
            if (abort?.signal.aborted) return;
            let cleaned = raw.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const arr = JSON.parse(cleaned) as Theme[];
              if (Array.isArray(arr)) themes = arr.filter((x) => x.label && x.query);
            } catch {
              error = "Model didn't return parseable JSON.";
            }
          },
          onError: (err) => {
            t.flush();
            if (isAbortError(err)) return;
            error = err.message;
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
    }
  }

  function applyTheme(t: Theme) {
    deps.applyThemeSearch(t.query);
  }

  function startAsk() {
    if (deps.getJots().length === 0) {
      deps.toastInfo('No jots loaded yet.');
      return;
    }
    dismiss();
    mode = 'ask';
    // Focus the input on next tick so the user can type immediately.
    queueMicrotask(() => askInputEl?.focus());
  }

  async function submitAsk() {
    const q = askQuestion.trim();
    if (!q || busy) return;
    abort = new AbortController();
    busy = true;
    error = '';
    askAnswer = '';
    const seed = buildJotsSeed(deps.getJots(), 40);
    const system =
      'You answer the user\'s questions about their own journal entries (daily notes). ' +
      'Be specific — cite dates and quote phrases the user actually wrote when relevant. ' +
      'If the answer isn\'t supported by the entries, say so honestly. Return markdown ' +
      'with concise paragraphs and bullet lists where helpful. No preamble.';
    const user =
      'Recent journal entries (JSON, newest first):\n```json\n' + seed + '\n```\n\n' +
      'Question: ' + q;
    try {
      const t = rafThrottle((full) => {
        if (abort?.signal.aborted) return;
        askAnswer = full;
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
            error = err.message;
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
    }
  }

  async function buildDigest() {
    const jots = deps.getJots();
    if (jots.length === 0) {
      deps.toastInfo('No jots loaded yet.');
      return;
    }
    dismiss();
    mode = 'digest';
    abort = new AbortController();
    busy = true;
    // Build a 7-day window from the most recent jot backwards.
    const cutoff = new Date(deps.getToday());
    cutoff.setDate(cutoff.getDate() - 6);
    const cutoffISO = fmtDateISO(cutoff);
    const slice = jots
      .filter((j) => j.date >= cutoffISO)
      .map((j) => ({ date: j.date, body: (j.body ?? '').slice(0, 2000) }));
    if (slice.length === 0) {
      error = 'No jots in the last 7 days.';
      busy = false;
      abort = null;
      return;
    }
    const seed = JSON.stringify(slice, null, 2);
    const system =
      'You write a weekly digest of the user\'s journal entries. Structure the output as ' +
      'markdown with these sections:\n\n' +
      '## Themes\n  3-5 bullets — the topics that recurred across the week.\n' +
      '## Wins\n  Concrete accomplishments or moments worth keeping. Quote when useful.\n' +
      '## Struggles\n  Friction, blockers, or unresolved tensions the user wrote about.\n' +
      '## Open threads\n  Things that started but didn\'t finish — questions, plans, follow-ups.\n' +
      '## Suggested focus\n  One sentence: what would be most valuable to focus on next week, ' +
      'based on what the user wrote.\n\n' +
      'Be specific. Cite dates inline (e.g., "on 2026-05-12") when grounding a claim. ' +
      'Skip sections that don\'t apply rather than padding them with generic prose.';
    const user = 'Past 7 days of dailies:\n```json\n' + seed + '\n```';
    try {
      const t = rafThrottle((full) => {
        if (abort?.signal.aborted) return;
        digestAnswer = full;
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
            error = err.message;
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
    }
  }

  async function saveDigestAsNote() {
    if (!digestAnswer.trim()) return;
    const ds = fmtDateISO(new Date(deps.getToday()));
    const folder = deps.getDailyFolder();
    const path = (folder ? `${folder}/` : '') + `digest-${ds}.md`;
    try {
      await api.putNote(path, {
        frontmatter: { title: `Weekly digest — ${ds}`, type: 'digest', generatedBy: 'ai' },
        body: digestAnswer
      });
      deps.toastSuccess('digest saved as note');
      deps.navigate(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      deps.toastError('failed to save: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function copyToClipboard(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      deps.toastSuccess('copied');
    } catch {
      deps.toastError('clipboard blocked');
    }
  }

  return {
    get mode() { return mode; },
    get busy() { return busy; },
    get error() { return error; },
    get themes() { return themes; },
    get askQuestion() { return askQuestion; },
    set askQuestion(v) { askQuestion = v; },
    get askAnswer() { return askAnswer; },
    get askInputEl() { return askInputEl; },
    set askInputEl(v) { askInputEl = v; },
    get digestAnswer() { return digestAnswer; },

    detectThemes,
    startAsk,
    submitAsk,
    buildDigest,
    saveDigestAsNote,
    applyTheme,
    copyToClipboard,
    cancel,
    dismiss
  };
}
