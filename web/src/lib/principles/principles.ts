// The Tagesordnung — 16 Leitbegriffe that anchor the day.
//
// Single source of truth. Imported by the TagesordnungWidget today;
// later phases (Check-in, Daily Review, Task/Project/Goal tagging)
// pull from the same `PRINCIPLES` array so adding or renaming a
// principle propagates everywhere without hardcoded duplicates.
//
// Order is intentional and runs from physical → inner → outward
// action → posture → Christus. Read top-to-bottom it sketches the
// shape of a faithful day.
//
// IDs are stable kebab-case keys safe for persistence (frontmatter
// tags, task-binding strings, JSON keys). Display names stay German
// so the surface reads in the user's voice. Don't change IDs after
// the data model lands in phase 2 — they become persistent foreign
// keys.

export interface Principle {
  /** Stable kebab-case key. Persisted to tasks/projects/goals in
   *  later phases — never rename without a migration. */
  id: string;
  /** German display name shown in the UI. */
  name: string;
  /** One-line meaning. Kept short on purpose; the widget reads as a
   *  quiet anchor, not an essay. */
  short: string;
}

export const PRINCIPLES: readonly Principle[] = [
  { id: 'schlaf', name: 'Schlaf', short: 'Körper und Geist erneuern.' },
  { id: 'gebet', name: 'Gebet', short: 'Ehrlich vor Gott kommen.' },
  { id: 'schrift', name: 'Schrift', short: 'Wahrheit aufnehmen.' },
  { id: 'demut', name: 'Demut', short: 'Nicht überheben, nicht verachten.' },
  { id: 'ehrfurcht', name: 'Ehrfurcht', short: 'Gott ernst nehmen.' },
  { id: 'arbeit', name: 'Arbeit', short: 'Ruhig und sauber schaffen.' },
  { id: 'training', name: 'Training', short: 'Körper stärken, Disziplin leben.' },
  { id: 'essen', name: 'Essen', short: 'Körper versorgen.' },
  { id: 'schreiben', name: 'Schreiben', short: 'Gedanken ordnen, Wahrheit formulieren.' },
  { id: 'pruefung', name: 'Prüfung', short: 'Alles prüfen, nicht im Misstrauen leben.' },
  { id: 'frieden', name: 'Frieden', short: 'Sich nicht erschüttern lassen.' },
  { id: 'treue', name: 'Treue', short: 'Kleine Dinge regelmäßig tun.' },
  { id: 'liebe', name: 'Liebe', short: 'Gut bleiben, auch wenn es niemand sieht.' },
  { id: 'selbstbeherrschung', name: 'Selbstbeherrschung', short: 'Nicht jedem Impuls folgen.' },
  { id: 'ordnung', name: 'Ordnung', short: 'Kalender, Aufgaben, Körper und Geist ordnen.' },
  { id: 'christus', name: 'Christus', short: 'Christus trägt. Ich folge.' }
];

// Lookup index. Built once at module load so callers in later phases
// (task tagging, goal tagging) don't pay O(n) per lookup. Kept in
// sync with PRINCIPLES automatically — the Map is derived from the
// array literal above.
const BY_ID = new Map<string, Principle>(PRINCIPLES.map((p) => [p.id, p]));

export function principleById(id: string): Principle | undefined {
  return BY_ID.get(id);
}

// The Leitsatz in long form — the spec's "wichtigster Leitsatz".
// Surfaced in the widget footer as a quiet signature. Built from
// PRINCIPLES so the two never drift.
export const PRINCIPLES_LEITSATZ: string = PRINCIPLES.map((p) => p.name).join('. ') + '.';

// Verb form — shorter daily refrain. Hand-curated rather than derived
// since the verb mapping isn't 1:1 (Schlaf → Schlafen, but Prüfung →
// Prüfen, Frieden → Ruhen, Christus → Christus folgen).
export const PRINCIPLES_KURZ =
  'Schlafen. Beten. Lesen. Arbeiten. Trainieren. Essen. Schreiben. Prüfen. Lieben. Ruhen. Christus folgen.';
