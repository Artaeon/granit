// The five pillars of the Heute-Karte. Hardcoded keys, overridable
// labels. The decision is intentional: the discipline of the app
// rests on the *five* (Gott / Essen / Arbeit / Körper / Abend) — if
// the user could add a sixth or drop one, the rhythm stops being a
// rhythm and becomes another todo list.
//
// What the user CAN change:
//   - the display label (e.g. "Sport" instead of "Körper")
//   - the icon glyph
//   - the per-pillar minima text per day mode (in $lib/rhythmus/minima)
//   - the order (later; v1 is fixed)
//
// What's locked:
//   - the five keys themselves — every other module that wires into
//     the day shape (next-action rules, evening flow, daily-note
//     frontmatter) references them by key, never by label.

export type PillarKey = 'spirit' | 'food' | 'work' | 'body' | 'evening';

export const PILLAR_ORDER: PillarKey[] = ['spirit', 'food', 'work', 'body', 'evening'];

export type PillarDef = {
  key: PillarKey;
  label: string;
  icon: string;
};

export const DEFAULT_PILLARS: Record<PillarKey, PillarDef> = {
  spirit:  { key: 'spirit',  label: 'Gott',    icon: '✝' },
  food:    { key: 'food',    label: 'Essen',   icon: '🍞' },
  work:    { key: 'work',    label: 'Arbeit',  icon: '⚒' },
  body:    { key: 'body',    label: 'Körper',  icon: '⚇' },
  evening: { key: 'evening', label: 'Abend',   icon: '☽' }
};
