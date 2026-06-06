// Topical scripture verses for GoalDetail.
//
// Surfaces a scripture verse whose topic matches the goal — either
// via category (life-area defaults) or a tag whose name lines up
// with a known catalogue topic. Walks the same /scripture topic
// index ScriptureWidget / VerseForMoodWidget use, so the user gets
// a consistent scripture surface across the app without a separate
// backend.

import { api, type Goal, type Scripture } from '$lib/api';

// Category map is intentionally short — picks ONE topic per category
// that the catalogue carries. If we ever want richer overlap, push it
// into a config; today the simplicity is the feature. 'other' is
// deliberately omitted — better no verse than a misaligned one.
const CATEGORY_TO_TOPIC: Record<string, string> = {
  spiritual: 'faith',
  health: 'discipline',
  career: 'diligence',
  learning: 'wisdom',
  relationships: 'love',
  finance: 'generosity',
  creative: 'creation'
};

// Hand-curated list of topics known to exist in scripture.Defaults.
// Used to gate tag-based lookups so we don't burn a round-trip on
// every random tag. Drawn from internal/scripture/scripture.go.
const KNOWN_TOPICS: ReadonlySet<string> = new Set([
  'anxiety', 'fear', 'hope', 'patience', 'gratitude', 'joy', 'grief',
  'anger', 'suffering', 'rest', 'peace', 'guidance', 'faith', 'love',
  'wisdom', 'discipline', 'diligence', 'generosity', 'creation',
  'forgiveness', 'humility', 'mercy', 'prayer', 'trust', 'endurance',
  'courage', 'contentment', 'friendship'
]);

export interface GoalDetailVersesController {
  readonly verses: Scripture[];
  readonly verseCursor: number;
  readonly verseTopic: string | null;
  readonly verseLoading: boolean;
  /** Active verse for the carousel — verses[verseCursor] when
   *  available, else null. Surfaced as a getter so the template
   *  doesn't need to gate on .length. */
  readonly currentVerse: Scripture | null;
  /** Advance to the next verse in the carousel; wraps. */
  next(): void;
}

export interface GoalDetailVersesDeps {
  getGoal: () => Goal | null;
}

export function createGoalDetailVerses(deps: GoalDetailVersesDeps): GoalDetailVersesController {
  let verses = $state<Scripture[]>([]);
  let verseCursor = $state(0);
  let verseTopic = $state<string | null>(null);
  let verseLoading = $state(false);

  // Resolve a candidate topic from the goal. Tags win over category
  // because a user-set tag is a stronger signal of intent than the
  // coarse life-area bucket. Match is case-insensitive against the
  // tag string itself — the catalogue topics are lowercase tokens
  // like "faith", "patience", so a tag "patience" lines up directly.
  const goalTopic = $derived.by<string | null>(() => {
    const goal = deps.getGoal();
    if (!goal) return null;
    const tags = (goal.tags ?? []).map((t) => t.trim().toLowerCase()).filter(Boolean);
    for (const t of tags) {
      // listScriptures() returns empty for unknown topics anyway,
      // so a miss here just costs one round-trip. The KNOWN_TOPICS
      // gate avoids that cost on the common-case random tag.
      if (KNOWN_TOPICS.has(t)) return t;
    }
    if (goal.category && CATEGORY_TO_TOPIC[goal.category]) {
      return CATEGORY_TO_TOPIC[goal.category];
    }
    return null;
  });

  // Fetch verses when the resolved topic changes. Ignored if no topic
  // resolves; verses array stays empty and the section hides.
  $effect(() => {
    const topic = goalTopic;
    if (!topic) {
      verses = [];
      verseTopic = null;
      verseCursor = 0;
      return;
    }
    if (topic === verseTopic && verses.length > 0) return;
    verseLoading = true;
    verseTopic = topic;
    verseCursor = 0;
    api.listScriptures(topic)
      .then((r) => {
        verses = r.scriptures;
      })
      .catch(() => {
        verses = [];
      })
      .finally(() => {
        verseLoading = false;
      });
  });

  const currentVerse = $derived(verses.length > 0 ? verses[verseCursor] ?? null : null);

  function next() {
    if (verses.length <= 1) return;
    verseCursor = (verseCursor + 1) % verses.length;
  }

  return {
    get verses() { return verses; },
    get verseCursor() { return verseCursor; },
    get verseTopic() { return verseTopic; },
    get verseLoading() { return verseLoading; },
    get currentVerse() { return currentVerse; },
    next
  };
}
