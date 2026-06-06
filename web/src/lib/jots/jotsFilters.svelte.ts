// Filter state for the jots feed.
//
// Second extraction step out of routes/jots/+page.svelte. Owns the
// three orthogonal filter dimensions — active hashtags, "open
// tasks only" toggle, and rolling timeframe — plus the URL-hash
// round-trip that lets a refresh or shared link land the user on
// the same view.
//
// Derivations live here too because they're pure functions of jots
// + today + filter state:
//   - allTags: distinct hashtags across the loaded feed, ordered by
//     frequency desc then alpha (the chip rail in QuickFilters)
//   - visibleJots: jots that satisfy every active dimension (AND)
//
// The page reads visibleJots / allTags / hasAnyFilter through the
// controller; bind:filterOpenTasks / bind:filterTimeframe goes
// through bind: too so the QuickFilters chrome stays a dumb
// controlled component.

import { fmtDateISO, type Jot } from '$lib/api';

export type Timeframe = 'all' | '7d' | '30d';

export interface JotsFiltersDeps {
  /** Reactive jots[] — the feed's loaded array. Passed as a getter
   *  so the derivations track it through the controller boundary. */
  getJots: () => Jot[];
  /** Midnight today — used as the cutoff anchor for the rolling
   *  timeframe filter. Computed once in the page so all derivations
   *  share the same anchor on a given render. */
  getToday: () => Date;
}

export interface JotsFiltersController {
  activeTags: string[];
  filterOpenTasks: boolean;
  filterTimeframe: Timeframe;
  readonly hasAnyFilter: boolean;
  readonly allTags: string[];
  readonly visibleJots: Jot[];

  toggleTag(t: string): void;
  clearAllFilters(): void;
}

// Hashtag filter — when non-empty, jots must mention EVERY active
// tag (AND filter). Persisted in the URL hash; supports both the new
// form `#tags=a,b,c` and the legacy single-tag form `#tag=foo` from
// earlier versions of this page so shared old-style links still work.
function readTagsFromHash(): string[] {
  if (typeof window === 'undefined') return [];
  const h = window.location.hash;
  const multi = h.match(/^#tags=(.+)$/);
  if (multi) {
    return decodeURIComponent(multi[1])
      .split(',')
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean);
  }
  const single = h.match(/^#tag=(.+)$/);
  if (single) return [decodeURIComponent(single[1]).toLowerCase()];
  return [];
}

export function createJotsFilters(deps: JotsFiltersDeps): JotsFiltersController {
  let activeTags = $state<string[]>(readTagsFromHash());
  let filterOpenTasks = $state(false);
  let filterTimeframe = $state<Timeframe>('all');

  let hasAnyFilter = $derived(
    activeTags.length > 0 || filterOpenTasks || filterTimeframe !== 'all'
  );

  // All distinct hashtags found across the loaded jots, ordered by
  // frequency desc then alpha. Cheap derivation — for a few hundred
  // jots × a few tags each, the linear scan is invisible.
  let allTags = $derived.by(() => {
    const counts = new Map<string, number>();
    const tagRe = /(?:^|\s)#([\p{L}\p{N}_-]+)/gu;
    for (const j of deps.getJots()) {
      const seen = new Set<string>();
      for (const m of (j.body ?? '').matchAll(tagRe)) {
        const t = m[1].toLowerCase();
        if (seen.has(t)) continue;
        seen.add(t);
        counts.set(t, (counts.get(t) ?? 0) + 1);
      }
    }
    return [...counts.entries()]
      .sort((a, b) => (b[1] - a[1]) || a[0].localeCompare(b[0]))
      .map(([t]) => t);
  });

  // Filtered feed = jots that satisfy every active filter dimension:
  //   1. every active tag must appear in the body (AND)
  //   2. if filterOpenTasks, jot.openTasks > 0
  //   3. if filterTimeframe != 'all', jot.date is within the window
  // When no filter is active, returns the full list verbatim.
  let visibleJots = $derived.by(() => {
    let out = deps.getJots();
    if (filterOpenTasks) out = out.filter((j) => j.openTasks > 0);
    if (filterTimeframe !== 'all') {
      const days = filterTimeframe === '7d' ? 7 : 30;
      const cutoff = new Date(deps.getToday());
      cutoff.setDate(cutoff.getDate() - (days - 1));
      const cutoffISO = fmtDateISO(cutoff);
      out = out.filter((j) => j.date >= cutoffISO);
    }
    if (activeTags.length === 0) return out;
    // Build one regex per tag; jot must match all of them.
    const escaped = activeTags.map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
    const regexes = escaped.map((e) => new RegExp(`(?:^|\\s)#${e}\\b`, 'i'));
    return out.filter((j) => {
      const body = j.body ?? '';
      return regexes.every((re) => re.test(body));
    });
  });

  function writeTagsHash() {
    if (typeof window === 'undefined') return;
    const next = activeTags.length > 0
      ? `#tags=${activeTags.map(encodeURIComponent).join(',')}`
      : '';
    // Replace, not push — tag filtering shouldn't pollute browser
    // history with one entry per chip click.
    history.replaceState(null, '', window.location.pathname + window.location.search + next);
  }

  function toggleTag(t: string) {
    const lower = t.toLowerCase();
    if (activeTags.includes(lower)) {
      activeTags = activeTags.filter((x) => x !== lower);
    } else {
      activeTags = [...activeTags, lower];
    }
    writeTagsHash();
  }
  function clearAllFilters() {
    activeTags = [];
    filterOpenTasks = false;
    filterTimeframe = 'all';
    writeTagsHash();
  }

  return {
    get activeTags() { return activeTags; },
    set activeTags(v) { activeTags = v; },
    get filterOpenTasks() { return filterOpenTasks; },
    set filterOpenTasks(v) { filterOpenTasks = v; },
    get filterTimeframe() { return filterTimeframe; },
    set filterTimeframe(v) { filterTimeframe = v; },
    get hasAnyFilter() { return hasAnyFilter; },
    get allTags() { return allTags; },
    get visibleJots() { return visibleJots; },
    toggleTag,
    clearAllFilters
  };
}
