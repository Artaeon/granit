// View-state for the /hub launcher — the read-only projection over
// the loaded items.
//
// Second extraction step out of routes/hub/+page.svelte. Owns the
// search term + the active category filter and the derived views the
// template iterates over: the category chip row (counted + sorted
// by frequency), the filtered flat list, and the per-category
// grouping with favorite-pinning.
//
// Why a separate controller: the data loader owns the source-of-
// truth array, but the filter + group derivation is a non-trivial
// chunk of $derived.by closures that have nothing to do with fetch
// plumbing — they're a presentation layer. Splitting them lets the
// page bind <input> and chip buttons against q / categoryFilter
// directly while reading `categories` / `grouped` as plain getters.
//
// All of the search/group/sort knobs documented here are intentional
// product choices — see the inline comments. Search deliberately
// skips the password field; favorites stay pinned within each group;
// "Other" sinks to the bottom of the section list.

import type { HubItem } from '$lib/api';

export type HubGroup = { key: string; items: HubItem[] };

export interface HubViewController {
  /** Free-text search bound to the search input. */
  q: string;
  /** Active category chip ('' = "All"). */
  categoryFilter: string;
  /** [category, count] pairs sorted by frequency desc — chip row.
   *  Items without a category land under "Other". */
  readonly categories: [string, number][];
  /** Flat list after search + category narrowing. */
  readonly visibleItems: HubItem[];
  /** Visible items grouped by category, favorites pinned to the top
   *  of each bucket, "Other" sorted last. */
  readonly grouped: HubGroup[];
}

export interface HubViewDeps {
  /** The full hub-items array — owned by the data controller. */
  getItems: () => HubItem[];
}

export function createHubView(deps: HubViewDeps): HubViewController {
  let q = $state('');
  let categoryFilter = $state('');

  // Categories with counts, sorted by frequency desc — the most-
  // used categories surface first in the chip row. Items without
  // a category land under "Other".
  const categories = $derived.by(() => {
    const m = new Map<string, number>();
    for (const it of deps.getItems()) {
      const c = (it.category ?? '').trim() || 'Other';
      m.set(c, (m.get(c) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  });

  // Filtered + grouped view. Search matches title / url / category /
  // notes / username (NOT password — that would surface secrets via
  // the search field). Category filter narrows to a single bucket.
  const visibleItems = $derived.by(() => {
    let out = deps.getItems();
    if (categoryFilter) {
      const cf = categoryFilter.toLowerCase();
      out = out.filter((it) => (it.category ?? 'Other').toLowerCase() === cf);
    }
    const term = q.trim().toLowerCase();
    if (term) {
      out = out.filter((it) =>
        it.title.toLowerCase().includes(term) ||
        (it.url ?? '').toLowerCase().includes(term) ||
        (it.category ?? '').toLowerCase().includes(term) ||
        (it.notes ?? '').toLowerCase().includes(term) ||
        (it.username ?? '').toLowerCase().includes(term)
      );
    }
    return out;
  });

  // Group the visible items by category so the page reads as
  // clusters rather than a flat list. Favorites stay pinned across
  // all groups by sorting them to the top of each bucket.
  const grouped = $derived.by((): HubGroup[] => {
    const m = new Map<string, HubItem[]>();
    for (const it of visibleItems) {
      const cat = (it.category ?? '').trim() || 'Other';
      const arr = m.get(cat) ?? [];
      arr.push(it);
      m.set(cat, arr);
    }
    const out: HubGroup[] = [];
    for (const [key, list] of m) {
      list.sort((a, b) => {
        if (!!a.favorite !== !!b.favorite) return a.favorite ? -1 : 1;
        return a.title.localeCompare(b.title);
      });
      out.push({ key, items: list });
    }
    out.sort((a, b) => {
      // Other always last — known categories before the catch-all
      if (a.key === 'Other' && b.key !== 'Other') return 1;
      if (b.key === 'Other' && a.key !== 'Other') return -1;
      return a.key.localeCompare(b.key);
    });
    return out;
  });

  return {
    get q() {
      return q;
    },
    set q(v) {
      q = v;
    },
    get categoryFilter() {
      return categoryFilter;
    },
    set categoryFilter(v) {
      categoryFilter = v;
    },
    get categories() {
      return categories;
    },
    get visibleItems() {
      return visibleItems;
    },
    get grouped() {
      return grouped;
    }
  };
}
