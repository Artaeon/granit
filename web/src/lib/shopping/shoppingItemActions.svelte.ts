// Item-action controller for the shopping surface.
//
// Fourth extraction step out of routes/shopping/+page.svelte. Owns
// the quick-add form buffer (name + category + price + quantity +
// url + standard) plus every mutation that operates on a single item
// from outside the inline edit form:
//
//   - addItem(): the quick-add submit handler
//   - setStatus(): plan / bought / skipped lifecycle moves (used by
//     the row checkbox, the skip button, and the "re-plan" link on
//     the Bought view)
//   - toggleStandard(): the row ★ / ☆ toggle
//   - removeItem(): the row trash button (with confirm)
//
// All four mutations write back into shoppingData via setters and
// trigger refreshTotals() on the data controller so the totals strip
// stays in sync. The route shell holds the references; the controller
// stays toast-pluggable for testing.
//
// The inline edit form stays in the route shell for now — it's only
// 1 mutation + a 9-key buffer and lives close to the edit template
// branch. A future SH5 could lift it if more surfaces (drawer edit,
// batch edit) start to share the same submit path.

import { api, type ShoppingItem } from '$lib/api';
import type { ShoppingDataController } from './shoppingData.svelte';

export interface ShoppingItemActionsDeps {
  data: ShoppingDataController;
  onError: (message: string) => void;
  /** Confirm hook for destructive actions. Defaults to window.confirm
   *  when omitted; injected so tests don't need a real DOM. */
  confirm?: (message: string) => boolean;
}

export interface ShoppingItemActionsController {
  // Quick-add form buffer. Bindable so the form template can drive
  // them through `bind:value` directly.
  nName: string;
  nCategory: string;
  nPrice: number | '';
  nQuantity: number | '';
  nUrl: string;
  nStandard: boolean;
  /** True while the quick-add submit is in flight. */
  saving: boolean;

  addItem(e?: SubmitEvent): Promise<void>;
  setStatus(it: ShoppingItem, status: 'planned' | 'bought' | 'skipped'): Promise<void>;
  toggleStandard(it: ShoppingItem): Promise<void>;
  removeItem(it: ShoppingItem): Promise<void>;
}

export function createShoppingItemActions(
  deps: ShoppingItemActionsDeps
): ShoppingItemActionsController {
  let nName = $state('');
  let nCategory = $state('groceries');
  let nPrice = $state<number | ''>('');
  let nQuantity = $state<number | ''>('');
  let nUrl = $state('');
  let nStandard = $state(false);
  let saving = $state(false);

  const askConfirm = deps.confirm ?? ((m: string) => window.confirm(m));

  function resetCreate() {
    nName = '';
    nPrice = '';
    nQuantity = '';
    nUrl = '';
    nStandard = false;
    // Keep the category sticky — the user adding 5 groceries shouldn't
    // re-pick "groceries" each time.
  }

  async function addItem(e?: SubmitEvent) {
    e?.preventDefault();
    if (!nName.trim()) return;
    saving = true;
    try {
      const it = await api.createShoppingItem({
        name: nName.trim(),
        category: nCategory.trim() || undefined,
        price: typeof nPrice === 'number' ? nPrice : undefined,
        quantity: typeof nQuantity === 'number' ? nQuantity : undefined,
        url: nUrl.trim() || undefined,
        standard: nStandard,
        status: 'planned'
      });
      // Optimistic prepend; load() reconciles ordering + totals.
      deps.data.items = [it, ...deps.data.items];
      resetCreate();
      await deps.data.load();
    } catch (err) {
      deps.onError('add failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      saving = false;
    }
  }

  async function setStatus(it: ShoppingItem, status: 'planned' | 'bought' | 'skipped') {
    try {
      const updated = await api.patchShoppingItem(it.id, { status });
      deps.data.items = deps.data.items.map((x) => (x.id === it.id ? updated : x));
      // Refresh totals — the rollup may have changed.
      await deps.data.refreshTotals();
    } catch (e) {
      deps.onError('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function toggleStandard(it: ShoppingItem) {
    try {
      const updated = await api.patchShoppingItem(it.id, { standard: !it.standard });
      deps.data.items = deps.data.items.map((x) => (x.id === it.id ? updated : x));
    } catch (e) {
      deps.onError('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function removeItem(it: ShoppingItem) {
    if (!askConfirm(`Delete "${it.name}"?`)) return;
    try {
      await api.deleteShoppingItem(it.id);
      deps.data.items = deps.data.items.filter((x) => x.id !== it.id);
      await deps.data.refreshTotals();
    } catch (e) {
      deps.onError('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get nName() { return nName; },
    set nName(v) { nName = v; },
    get nCategory() { return nCategory; },
    set nCategory(v) { nCategory = v; },
    get nPrice() { return nPrice; },
    set nPrice(v) { nPrice = v; },
    get nQuantity() { return nQuantity; },
    set nQuantity(v) { nQuantity = v; },
    get nUrl() { return nUrl; },
    set nUrl(v) { nUrl = v; },
    get nStandard() { return nStandard; },
    set nStandard(v) { nStandard = v; },
    get saving() { return saving; },
    set saving(v) { saving = v; },
    addItem,
    setStatus,
    toggleStandard,
    removeItem
  };
}
