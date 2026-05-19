// Rule-based "what should I do next?" engine.
//
// The Heute-Karte's main verb is one line: a single sentence telling
// the user the one thing the rhythm is asking of them right now.
// Not a list. Not a ranked queue. One sentence. The whole point of
// the app is to remove decision fatigue at the moment the user is
// most likely to spiral — and a list of options doesn't remove
// fatigue, it adds it.
//
// Why rule-based and not an LLM call:
//   1. Deterministic — the user gets the same answer if they ask
//      twice in a row. An LLM would re-roll the framing and feel
//      less trustworthy at exactly the moment trust matters.
//   2. Offline — the rules run on the client. A user on a train
//      with no signal still gets the next action.
//   3. Testable — every branch has a unit test below in the .test.
//      An LLM tells you "trust the model"; the rules tell you why.
//
// The rule order itself is the product:
//   1. Eating before everything else — a hungry user makes worse
//      decisions about the next four hours than a fed one.
//   2. Evening protects sleep — past the user's configured shutdown
//      time, the next action is always "stop", regardless of what
//      else is open.
//   3. The MIT (Most Important Task) gets a focused block when set
//      and the work-pillar is still open.
//   4. Body is opportunistic — only proposed when fatigue is low
//      enough that movement is restoration rather than punishment.
//   5. If everything minimum is done, the next action is rest. Not
//      "find more to do". That's the whole pivot.

import type { DayState } from './dayState';

export type NextAction = {
  /** Short imperative — one sentence the user sees in the big card. */
  label: string;
  /** Which pillar the action belongs to, for visual linking + the
   *  "click to mark done" affordance on the pillar row. */
  pillar: 'spirit' | 'food' | 'work' | 'body' | 'evening' | 'rest';
  /** Why this fired — surfaced as a small dim line under the label
   *  so the user can sanity-check the rule. */
  reason: string;
};

export type NextActionContext = {
  /** Current local time. Injected (not read from new Date()) so the
   *  rules are unit-testable without time-travel hacks. */
  now: Date;
  /** Time of day, expressed as HH:MM, after which the evening
   *  shutdown takes precedence over any open work pillar. User-
   *  configurable in /rhythmus; defaults to 20:30. */
  eveningStartsAt: string;
  /** Time of day, before which the "eat first" rule waits. Lets a
   *  user skip the breakfast prompt at 06:00 and only see it once
   *  the morning is meaningfully underway. Default 10:00 per the
   *  brainstorm's reminder cadence. */
  eatNagAfter: string;
};

const DEFAULT_CONTEXT: Omit<NextActionContext, 'now'> = {
  eveningStartsAt: '20:30',
  eatNagAfter: '10:00'
};

// Parse "HH:MM" relative to `now`'s local day. Returns a Date on the
// same day so comparisons stay tz-safe.
function timeOf(now: Date, hhmm: string): Date {
  const [h, m] = hhmm.split(':').map((x) => parseInt(x, 10));
  const d = new Date(now);
  d.setHours(h || 0, m || 0, 0, 0);
  return d;
}

export function nextAction(
  state: DayState,
  ctxIn: Partial<NextActionContext> & { now: Date }
): NextAction {
  const ctx: NextActionContext = { ...DEFAULT_CONTEXT, ...ctxIn };
  const { now, mode, fatigue, eaten, mit, pillars } = withDefaults(state, ctxIn);

  // 1) Eat first — but not before the user's "eat nag" cutoff.
  //    Emergency mode tightens the leash: even at 06:00 we already
  //    ask, because if the user has flagged emergency they've usually
  //    already missed regular meal cues.
  const eatGate = mode === 'emergency' ? now : timeOf(now, ctx.eatNagAfter);
  if (!eaten && now >= eatGate) {
    return {
      label: 'Iss zuerst.',
      pillar: 'food',
      reason:
        mode === 'emergency'
          ? 'Notfallmodus — Essen vor allem anderen.'
          : 'Noch nichts gegessen — der Rest des Tages wartet darauf.'
    };
  }

  // 2) Evening wins past the shutdown threshold. The work-pillar may
  //    still be open; that's fine — we're protecting sleep, not
  //    finishing the todo list. The earlier "Mod+J / ⌘K" routes
  //    remain open for the user who really wants to override.
  if (now >= timeOf(now, ctx.eveningStartsAt) && !pillars.evening.done) {
    return {
      label: 'Shutdown. Arbeit für heute zu.',
      pillar: 'evening',
      reason: `Es ist nach ${ctx.eveningStartsAt} — Schlaf ist jetzt Training.`
    };
  }

  // 3) Most-important task gets a focus block when set and work is
  //    still open. Phrasing differs by mode so the chaotic / emergency
  //    user doesn't get punched in the face with a full pomodoro.
  if (mit.trim() && !pillars.work.done) {
    const minutes = mode === 'chaotic' || mode === 'emergency' ? 25 : 45;
    return {
      label: `${minutes} Minuten fokussiert: ${mit.trim()}`,
      pillar: 'work',
      reason: 'Eine wichtigste Aufgabe — der Rest wartet.'
    };
  }

  // 4) Body — only when fatigue allows. High fatigue collapses to a
  //    walk; low fatigue suggests real movement. Emergency skips
  //    body entirely.
  if (mode !== 'emergency' && !pillars.body.done) {
    if (fatigue <= 3) {
      return {
        label: '10 Minuten Bewegung.',
        pillar: 'body',
        reason: 'Heute noch nichts gemacht.'
      };
    }
    return {
      label: 'Spaziergang reicht heute.',
      pillar: 'body',
      reason: 'Müde — Bewegung soll heute kein Training sein.'
    };
  }

  // 5) Spirit pillar still open and not in chaotic/emergency mode
  //    (those collapse the spirit minimum elsewhere; we don't nag
  //    when the user is already barely upright).
  if (mode === 'normal' && !pillars.spirit.done) {
    return {
      label: 'Kurzes Gebet oder ein Psalm.',
      pillar: 'spirit',
      reason: 'Säule Gott steht noch offen.'
    };
  }

  // 6) All minima done — the explicit "you may stop" answer. This is
  //    the whole pivot of the app: rest is a legal answer, not a
  //    failure to find more work.
  return {
    label: 'Alles Minimum heute. Frei. Lesen. Familie.',
    pillar: 'rest',
    reason: 'Tag steht. Mehr braucht es heute nicht.'
  };
}

// withDefaults fills in the small holes a not-yet-fully-checked-in
// day might have. Concentrated here so the rule body above reads as
// rules, not as defensive plumbing.
function withDefaults(state: DayState, ctx: { now: Date }): DayState & { now: Date } {
  return {
    ...state,
    mode: state.mode ?? 'normal',
    fatigue: Number.isFinite(state.fatigue) ? state.fatigue : 3,
    eaten: !!state.eaten,
    mit: state.mit ?? '',
    pillars: state.pillars ?? {
      spirit:  { done: false },
      food:    { done: false },
      work:    { done: false },
      body:    { done: false },
      evening: { done: false }
    },
    now: ctx.now
  } as DayState & { now: Date };
}
