// AI streaming state for the habits surface.
//
// Second extraction step out of routes/habits/+page.svelte. Owns the
// two streaming AI panels:
//
//   1. Pattern insight — reads each habit's last 30 days and produces
//      2-3 short observations: weekday patterns (consistent Wed
//      misses), streak risks (no-doomscrolling decayed Mon -> Sun),
//      or wins (longest streak ever this week). Streamed via the
//      audit-gated chat pipeline, no separate Tier 1 endpoint — it's
//      a chat with a structured prompt, same as people / examen.
//
//   2. Suggest habits from goals — distinct surface: insight observes
//      existing data, this one is *generative*. Given the user's
//      active goals, propose 2-4 fresh habits whose daily practice
//      would ladder toward those goals. Each suggestion is a one-click
//      "add" wired through the page's addHabit() (via deps.adopt).
//
// Both share the same shape — busy + error + AbortController, run +
// cancel + clear (the established Stop/Close split: cancel keeps the
// partial output, clear wipes everything). The two surfaces live in
// one controller because their UI chrome on the page is parallel and
// they're never visible together.
//
// External state (the loaded HabitsResponse, the listGoals fetch, the
// page's addHabit handler) is reached via the deps bundle so this
// file stays free of $lib/habits circular imports.

import { api, type Goal, type HabitsResponse } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import { isAbortError } from '$lib/util/aiErrors';

export interface HabitsAIDeps {
  /** Reactive getter for the loaded habits payload — read at runtime
   *  so the prompt sees the freshest data. */
  getData: () => HabitsResponse | null;
  /** Adopt a suggested habit by name. The page's addHabit() does the
   *  toggleHabit + load() round-trip; we just hand off the name. */
  adopt: (name: string) => Promise<void>;
}

export interface HabitsAIController {
  // ── Pattern insight ───────────────────────────────────────────────
  readonly insightBusy: boolean;
  readonly insightLines: string[];
  readonly insightError: string;
  runInsight(): Promise<void>;
  /** Stop the stream, keep the partial output (the established
   *  Stop/Close split). */
  cancelInsight(): void;
  /** Stop + wipe the panel. */
  dismissInsight(): void;

  // ── Suggest from goals ────────────────────────────────────────────
  readonly suggestBusy: boolean;
  readonly suggested: { name: string; rationale: string }[];
  readonly suggestError: string;
  runSuggest(): Promise<void>;
  cancelSuggest(): void;
  dismissSuggest(): void;
  /** Adopt a suggestion: delegates to deps.adopt(), then drops the
   *  adopted entry from the visible list. */
  adoptSuggestion(name: string): Promise<void>;
}

function buildHabitsSeed(data: HabitsResponse): string {
  const today = data.today;
  const habits = data.habits.slice(0, 12).map((h) => {
    // Compact day grid — a string of 30 chars where 1 = done,
    // 0 = missed, . = before-tracking. Cheaper than 30 objects
    // and the model reads patterns from it just fine.
    const grid = h.days.slice(-30).map((d) => (d.done ? '1' : '0')).join('');
    return {
      name: h.name,
      currentStreak: h.currentStreak,
      longestStreak: h.longestStreak,
      last7Pct: h.last7Pct,
      last30Pct: h.last30Pct,
      last30: grid,
      doneToday: h.doneToday
    };
  });
  return JSON.stringify({ today, habits }, null, 2);
}

export function createHabitsAI(deps: HabitsAIDeps): HabitsAIController {
  // ── Pattern insight ─────────────────────────────────────────────
  let insightBusy = $state(false);
  let insightAbort: AbortController | null = null;
  let insightLines = $state<string[]>([]);
  let insightError = $state('');

  async function runInsight() {
    const data = deps.getData();
    if (insightBusy || !data) return;
    insightAbort?.abort();
    insightAbort = new AbortController();
    insightBusy = true;
    insightError = '';
    insightLines = [];
    const seed = buildHabitsSeed(data);
    const system = 'You analyse the user\'s habit data and surface 2-3 short, specific observations. Examples of good observations: a weekday that consistently fails ("you miss Wednesdays consistently this month"), a streak risk ("no-doomscrolling decayed from 80% week 1 to 30% week 4"), a win ("morning-movement is at the longest streak it\'s ever been"). Examples of BAD observations: generic praise, vague advice, "keep going!". Each observation on its own line. No preamble, no numbering, no bullets. Under 22 words each. The grid string is 30 days oldest-to-newest; 1 = done, 0 = missed.';
    const user = `Today is ${data.today}. Here are the habits:\n\n\`\`\`json\n${seed}\n\`\`\`\n\nGive me 2-3 sharp observations.`;
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        (() => {
          // rAF throttle — split + filter + reactive insightLines write
          // per chunk repaints the insights list per token.
          const habitT = rafThrottle((full) => {
            // Gate on abort — dismissInsight() wipes insightLines;
            // a queued rAF frame would repopulate otherwise.
            if (insightAbort?.signal.aborted) return;
            insightLines = full.split(/\n+/).map((l) => l.trim()).filter((l) => l.length > 0);
          });
          return {
            onChunk: habitT.onChunk,
            onDone: () => { habitT.flush(); },
            onError: (err: Error) => {
              habitT.flush();
              if (isAbortError(err)) return;
              insightError = err.message;
            }
          };
        })(),
        insightAbort.signal
      );
    } finally {
      insightBusy = false;
      insightAbort = null;
    }
  }

  function cancelInsight() {
    insightAbort?.abort();
    insightAbort = null;
    insightBusy = false;
  }

  function dismissInsight() {
    insightAbort?.abort();
    insightBusy = false;
    insightLines = [];
    insightError = '';
  }

  // ── Suggest from goals ──────────────────────────────────────────
  // We deliberately keep the prompt scoped to goals, not the whole
  // life-context blob — habits proposed from goals stay grounded
  // ("learn German" -> "10 min Anki" beats a goal-less suggestion
  // like "drink more water"). When there are no active goals the
  // button stays clickable but the helper text on the empty state
  // points the user at /goals.
  let suggestBusy = $state(false);
  let suggestAbort: AbortController | null = null;
  let suggested = $state<{ name: string; rationale: string }[]>([]);
  let suggestError = $state('');

  async function runSuggest() {
    if (suggestBusy) return;
    suggestAbort?.abort();
    suggestAbort = new AbortController();
    suggestBusy = true;
    suggestError = '';
    suggested = [];
    let goals: Goal[] = [];
    try {
      const r = await api.listGoals();
      goals = (r.goals ?? []).filter((g) => g.status === 'active' || !g.status);
    } catch (e) {
      suggestError = e instanceof Error ? e.message : String(e);
      suggestBusy = false;
      suggestAbort = null;
      return;
    }
    if (goals.length === 0) {
      suggestError = 'No active goals — open /goals and set one first.';
      suggestBusy = false;
      suggestAbort = null;
      return;
    }
    // Compact goal payload. Truncate descriptions so the prompt
    // stays bounded even when the user has a dozen verbose goals.
    const goalLines = goals
      .slice(0, 15)
      .map((g) => {
        const desc = g.description ? ' — ' + g.description.slice(0, 120) : '';
        const cat = g.category ? ` [${g.category}]` : '';
        return `- ${g.title}${cat}${desc}`;
      })
      .join('\n');
    const data = deps.getData();
    const existingHabits = (data?.habits ?? []).map((h) => h.name).join(', ');
    const system =
      'You propose new daily / weekly HABITS the user could add to ladder toward their stated goals. ' +
      'Output STRICT JSON, no fences, no preamble, no commentary — exactly:\n' +
      '{"habits":[{"name":"<2-6 word habit>","rationale":"<one sentence linking it to a goal>"},...]}\n\n' +
      'Rules:\n' +
      '- 2-4 habits, no more.\n' +
      '- Each habit must be specific + repeatable in one day (e.g. "10 min Anki", not "study more").\n' +
      '- Each habit must clearly ladder toward at least one named goal.\n' +
      '- Do NOT propose habits the user is already tracking. Existing habits: ' + (existingHabits || '(none)') + '.';
    const user = 'Active goals:\n' + goalLines + '\n\nPropose habits.';
    let buf = '';
    try {
      const throttle = rafThrottle((full) => { buf = full; });
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: throttle.onChunk,
          onDone: () => {
            throttle.flush();
            // Parse leniently — strip ``` fences, find first {...}.
            let s = buf.trim().replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '');
            if (!s.startsWith('{')) {
              const m = s.match(/\{[\s\S]*\}/);
              if (m) s = m[0];
            }
            try {
              const parsed = JSON.parse(s) as { habits?: { name?: unknown; rationale?: unknown }[] };
              const list = (parsed.habits ?? [])
                .map((h) => ({
                  name: typeof h.name === 'string' ? h.name.trim() : '',
                  rationale: typeof h.rationale === 'string' ? h.rationale.trim() : ''
                }))
                .filter((h) => h.name.length > 0)
                .slice(0, 4);
              suggested = list;
              if (list.length === 0) suggestError = 'AI returned no habits — try again.';
            } catch {
              suggestError = 'AI returned malformed output — try again.';
            }
          },
          onError: (err) => {
            throttle.flush();
            if (isAbortError(err)) return;
            suggestError = err.message;
          }
        },
        suggestAbort.signal
      );
    } finally {
      suggestBusy = false;
      suggestAbort = null;
    }
  }

  function cancelSuggest() {
    suggestAbort?.abort();
    suggestAbort = null;
    suggestBusy = false;
  }

  function dismissSuggest() {
    suggestAbort?.abort();
    suggestBusy = false;
    suggested = [];
    suggestError = '';
  }

  async function adoptSuggestion(name: string) {
    await deps.adopt(name);
    suggested = suggested.filter((h) => h.name !== name);
  }

  return {
    get insightBusy() { return insightBusy; },
    get insightLines() { return insightLines; },
    get insightError() { return insightError; },
    runInsight,
    cancelInsight,
    dismissInsight,

    get suggestBusy() { return suggestBusy; },
    get suggested() { return suggested; },
    get suggestError() { return suggestError; },
    runSuggest,
    cancelSuggest,
    dismissSuggest,
    adoptSuggestion
  };
}
