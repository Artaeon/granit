// Data state for the shopping surface.
//
// Third extraction step out of routes/shopping/+page.svelte. Owns the
// loaded `items` + `totals` arrays, the loading flag, the load()
// function, and every derivation that operates over the data + active
// view alone:
//
//   - viewItems: status / standard filter for the active view
//   - grouped: category-bucketed view for Plan / Standards, with the
//     canonical-category-first sort (matches server CategorySuggestions)
//   - viewTotal: planned-sum / bought-month-sum dispatch for the
//     totals strip above the list
//   - countPlanned / countStandards / countBought: tab badge counts
//     over the unfiltered list
//
// The route shell still owns the onMount install ordering, the WS /
// visibility listeners, the AI surfaces (none today), and every form
// — those write back via dataCtl.items / dataCtl.totals setters.
//
// Pattern mirrors financeData + tasksData: getter/setter pairs on
// every state, getter-only on derivations, deps-injected toast + auth
// guard so the controller stays test-friendly.

import { api, type ShoppingItem, type ShoppingTotals } from '$lib/api';
import { CATEGORY_SUGGESTIONS } from './shoppingHelpers';
import type { ShoppingView } from './shoppingViewState.svelte';

export interface ShoppingDataDeps {
  /** Boolean snapshot of the auth store — used as a guard before
   *  load(). The page passes () => !!$auth so the read stays
   *  reactive in the calling context. */
  isAuthed: () => boolean;
  /** Active view (plan / standards / bought). Reactive getter so the
   *  filtering + grouping derivations re-run when the user flips a
   *  tab. */
  getView: () => ShoppingView;
  /** Toast hook for the catch branch in load(). Injected so the
   *  controller doesn't have to import the toast singleton. */
  onError: (message: string) => void;
}

export interface ShoppingDataController {
  // Loaded sidecars — bindable so load() + per-row mutations on the
  // page can write the API response back into the controller.
  items: ShoppingItem[];
  totals: ShoppingTotals | null;
  loading: boolean;

  // Derived — readonly.
  /** Filtered items for the active view. */
  readonly viewItems: ShoppingItem[];
  /** Category buckets for Plan / Standards (null on Bought — that
   *  view renders flat / chronological). */
  readonly grouped: { category: string; items: ShoppingItem[] }[] | null;
  /** Totals row above the list: planned-sum on Plan, bought-month-sum
   *  on Bought, null on Standards (count-only view). */
  readonly viewTotal: number | null;
  /** Tab badge counts over the unfiltered list. */
  readonly countPlanned: number;
  readonly countStandards: number;
  readonly countBought: number;

  /** Fetch items + totals in parallel. Totals failure is non-fatal
   *  (module may be partially disabled). */
  load(): Promise<void>;
  /** Refresh totals only — used after a per-row mutation where items
   *  are updated optimistically but the rollup may have changed. */
  refreshTotals(): Promise<void>;
}

export function createShoppingData(deps: ShoppingDataDeps): ShoppingDataController {
  let items = $state<ShoppingItem[]>([]);
  let totals = $state<ShoppingTotals | null>(null);
  let loading = $state(false);

  async function load() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      const [r, t] = await Promise.all([
        api.listShopping(),
        api.shoppingTotals().catch(() => null)
      ]);
      items = r.items;
      totals = t;
    } catch (e) {
      deps.onError('failed to load shopping: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  async function refreshTotals() {
    totals = await api.shoppingTotals().catch(() => totals);
  }

  let viewItems = $derived.by<ShoppingItem[]>(() => {
    const view = deps.getView();
    if (view === 'plan') return items.filter((i) => i.status === 'planned');
    if (view === 'standards') return items.filter((i) => i.standard);
    return items.filter((i) => i.status === 'bought');
  });

  // Group by category for the Plan view. Items without a category
  // land in an explicit "uncategorised" bucket so the user can spot
  // (and fix) the gap. For Standards view we group the same way.
  // Bought view is flat (chronological) — returns null so the page
  // renders the chronological list branch.
  let grouped = $derived.by<{ category: string; items: ShoppingItem[] }[] | null>(() => {
    if (deps.getView() === 'bought') return null;
    const map = new Map<string, ShoppingItem[]>();
    for (const it of viewItems) {
      const k = (it.category ?? '').trim() || '—';
      const arr = map.get(k) ?? [];
      arr.push(it);
      map.set(k, arr);
    }
    // Sort categories: canonical order first (matching server
    // CategorySuggestions), then any custom categories alphabetically,
    // and the "uncategorised" bucket last.
    const known = new Map<string, number>();
    CATEGORY_SUGGESTIONS.forEach((c, i) => known.set(c, i));
    const keys = [...map.keys()].sort((a, b) => {
      if (a === '—') return 1;
      if (b === '—') return -1;
      const ka = known.has(a) ? known.get(a)! : 100 + a.charCodeAt(0);
      const kb = known.has(b) ? known.get(b)! : 100 + b.charCodeAt(0);
      return ka - kb;
    });
    return keys.map((k) => ({ category: k, items: map.get(k)! }));
  });

  // Totals for the active view. Plan view shows planned-sum; bought
  // view shows this-month spend. Standards view shows count only — a
  // price total there is misleading because most standards are
  // ongoing not one-shot purchases.
  let viewTotal = $derived.by<number | null>(() => {
    if (!totals) return null;
    const view = deps.getView();
    if (view === 'plan') return totals.planned_sum;
    if (view === 'bought') return totals.bought_month_sum;
    return null;
  });

  let countPlanned = $derived(items.filter((i) => i.status === 'planned').length);
  let countStandards = $derived(items.filter((i) => i.standard).length);
  let countBought = $derived(items.filter((i) => i.status === 'bought').length);

  return {
    get items() { return items; },
    set items(v) { items = v; },
    get totals() { return totals; },
    set totals(v) { totals = v; },
    get loading() { return loading; },
    set loading(v) { loading = v; },
    get viewItems() { return viewItems; },
    get grouped() { return grouped; },
    get viewTotal() { return viewTotal; },
    get countPlanned() { return countPlanned; },
    get countStandards() { return countStandards; },
    get countBought() { return countBought; },
    load,
    refreshTotals
  };
}
