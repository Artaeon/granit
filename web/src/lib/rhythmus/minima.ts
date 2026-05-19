// Per-pillar minima — the *content* the user has agreed to call
// "enough" for each of the three day modes. Per-device localStorage
// rather than vault-synced because what counts as "enough movement
// on a chaotic day" is intensely personal and changes with seasons
// of life — a server-shared default would surface someone else's
// rhythm in a place where the user is supposed to listen to theirs.
//
// Labels live next to minima even though the pillar *keys* are
// hard-coded in $lib/rhythmus/pillars. That's deliberate: the
// discipline ("there are five") is locked, but the language ("Sport"
// vs "Körper", "Stille" vs "Gott") is the user's.
//
// Defaults below come from the user's own brainstorm — they're a
// reasonable starting shape, not a prescription. The Rhythmus tab
// (Phase E) edits them; nothing else writes to this store.

import { persistedWritable } from '$lib/util/persistedWritable';
import { DEFAULT_PILLARS, PILLAR_ORDER, type PillarKey } from './pillars';
import type { DayMode } from './dayState';

export type PillarMinima = {
  normal: string;
  chaotic: string;
  emergency: string;
  /** If true, the pillar collapses out of the Heute-Karte entirely
   *  in emergency mode rather than showing a (potentially nagging)
   *  minimum text. The brainstorm explicitly does this for "real"
   *  work on a notfall day — keeping it shown would betray the
   *  whole point of the mode. */
  hideInEmergency?: boolean;
};

export type Reminder = {
  /** Stable id — kept identical across edits so a "last fired"
   *  bookmark in localStorage survives a label change. */
  id: string;
  /** HH:MM local. The ticker fires once per day inside a 30-minute
   *  window starting at this time. */
  time: string;
  /** Text the toast + (when permitted) browser notification show. */
  label: string;
  enabled: boolean;
};

export type RhythmusConfig = {
  /** User-facing label per pillar. Falls back to DEFAULT_PILLARS
   *  when missing. The pillar's key (PillarKey) is never edited. */
  labels: Partial<Record<PillarKey, string>>;
  minima: Record<PillarKey, PillarMinima>;
  /** Time-of-day (HH:MM, local) after which the Heute-Karte enters
   *  the soft Abendmodus. Anything past this hides the work-focus
   *  view in favour of the shutdown flow. */
  eveningStartsAt: string;
  /** Time after which the "have you eaten yet?" rule fires. Lets
   *  early risers skip a 06:00 breakfast prompt. */
  eatNagAfter: string;
  /** Local-time reminder schedule. The defaults map straight to the
   *  brainstorm's cadence (10:00 / 13:30 / 18:30 / 20:30 / 21:30).
   *  Order does not matter — the ticker iterates the list. */
  reminders: Reminder[];
};

export const DEFAULT_CONFIG: RhythmusConfig = {
  labels: {},
  minima: {
    spirit: {
      normal:    'kurzes Gebet / 1 Psalm',
      chaotic:   'kurzes Gebet',
      emergency: '1 Atemzug zu Gott'
    },
    food: {
      normal:    'erste Mahlzeit',
      chaotic:   'irgendwas essen',
      emergency: 'Wasser + Brot'
    },
    work: {
      normal:    '1 wichtigste Aufgabe',
      chaotic:   '1 wichtigste Aufgabe',
      emergency: '— nichts heute',
      hideInEmergency: true
    },
    body: {
      normal:    '10 Min Bewegung / Training',
      chaotic:   '10 Min Bewegung',
      emergency: '5 Minuten raus'
    },
    evening: {
      normal:    'Shutdown + Handy weg',
      chaotic:   'Abend retten — Handy weg',
      emergency: 'Laptop schließen'
    }
  },
  eveningStartsAt: '20:30',
  eatNagAfter: '10:00',
  reminders: [
    { id: 'eat-morning', time: '10:00', label: 'Hast du schon gegessen? Wenn nein: jetzt minimal essen.', enabled: true },
    { id: 'eat-midday',  time: '13:30', label: 'Erste echte Mahlzeit oder Post-Work-Meal.',               enabled: true },
    { id: 'wind-down',   time: '18:30', label: 'Was ist noch wirklich nötig — und was kann morgen warten?', enabled: true },
    { id: 'shutdown',    time: '20:30', label: 'Shutdown. Arbeit schließen. Morgen notieren.',            enabled: true },
    { id: 'phone-away',  time: '21:30', label: 'Handy weg. Schlaf ist jetzt Training.',                   enabled: true }
  ]
};

const KEY = 'granit.rhythmus.config';

// validate runs on the stored JSON to make sure an older shape (or
// hand-edited junk) doesn't poison the live store. Everything missing
// falls back to the default; everything present is type-checked.
function validate(raw: unknown): RhythmusConfig {
  if (!raw || typeof raw !== 'object') return DEFAULT_CONFIG;
  const r = raw as Partial<RhythmusConfig>;
  const minima = { ...DEFAULT_CONFIG.minima };
  if (r.minima && typeof r.minima === 'object') {
    for (const key of PILLAR_ORDER) {
      const m = (r.minima as Record<string, unknown>)[key];
      if (m && typeof m === 'object') {
        const cur = m as Partial<PillarMinima>;
        minima[key] = {
          normal:    typeof cur.normal    === 'string' ? cur.normal    : minima[key].normal,
          chaotic:   typeof cur.chaotic   === 'string' ? cur.chaotic   : minima[key].chaotic,
          emergency: typeof cur.emergency === 'string' ? cur.emergency : minima[key].emergency,
          hideInEmergency:
            typeof cur.hideInEmergency === 'boolean' ? cur.hideInEmergency : minima[key].hideInEmergency
        };
      }
    }
  }
  const labels: Partial<Record<PillarKey, string>> = {};
  if (r.labels && typeof r.labels === 'object') {
    for (const key of PILLAR_ORDER) {
      const v = (r.labels as Record<string, unknown>)[key];
      if (typeof v === 'string' && v.trim()) labels[key] = v;
    }
  }
  // Reminders: keep the user's list when valid, fall back to
  // defaults when missing. Each entry has to look like a Reminder;
  // unknown extras inside an entry are dropped silently so a hand-
  // edited JSON with stray keys parses without poisoning the store.
  const reminders: Reminder[] = [];
  if (Array.isArray(r.reminders)) {
    for (const item of r.reminders) {
      if (!item || typeof item !== 'object') continue;
      const o = item as Partial<Reminder>;
      if (typeof o.id !== 'string' || typeof o.time !== 'string') continue;
      reminders.push({
        id: o.id,
        time: o.time,
        label: typeof o.label === 'string' ? o.label : '',
        enabled: typeof o.enabled === 'boolean' ? o.enabled : true
      });
    }
  }
  return {
    labels,
    minima,
    eveningStartsAt: typeof r.eveningStartsAt === 'string' ? r.eveningStartsAt : DEFAULT_CONFIG.eveningStartsAt,
    eatNagAfter:     typeof r.eatNagAfter     === 'string' ? r.eatNagAfter     : DEFAULT_CONFIG.eatNagAfter,
    reminders: reminders.length > 0 ? reminders : DEFAULT_CONFIG.reminders.map((r) => ({ ...r }))
  };
}

export const rhythmusConfig = persistedWritable<RhythmusConfig>(KEY, DEFAULT_CONFIG, { validate });

// Convenience reader: resolve the display label for a pillar, with
// the hard-coded default as fallback when the user hasn't overridden.
export function pillarLabel(cfg: RhythmusConfig, key: PillarKey): string {
  return cfg.labels[key] ?? DEFAULT_PILLARS[key].label;
}

// Convenience reader: the minimum text for a pillar in the user's
// current mode. Centralised so the Heute-Karte never has to branch
// on mode itself.
export function pillarMinimumFor(cfg: RhythmusConfig, key: PillarKey, mode: DayMode): string {
  return cfg.minima[key][mode];
}

// Should the pillar render at all in this mode? Most modes always
// show; emergency optionally hides the work pillar because the whole
// point of the mode is permission to drop work entirely.
export function pillarVisibleIn(cfg: RhythmusConfig, key: PillarKey, mode: DayMode): boolean {
  if (mode === 'emergency' && cfg.minima[key].hideInEmergency) return false;
  return true;
}
