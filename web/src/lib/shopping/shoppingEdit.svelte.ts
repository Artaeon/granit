// Inline-edit controller for the shopping surface.
//
// Fifth extraction step out of routes/shopping/+page.svelte. Owns the
// per-row edit state (which row is open + a 9-field buffer for the
// edit form) plus the three handlers that drive it:
//
//   - startEdit(): seeds the buffer from a ShoppingItem (with
//     normalizeCadence so legacy server values round-trip cleanly)
//   - cancelEdit(): closes the form, discards the buffer
//   - commitEdit(): PATCH + writes the updated item back into the
//     shoppingData controller, then refreshes totals
//
// The template still does its own `editingId === it.id` check to
// decide which row to render in edit mode — exposed as `isEditing(it)`
// so the template stays a one-liner.
//
// Cadence is sent on PATCH even when standard=false: if the user
// later flips an item to standard, the existing intent is already in
// place and we don't have to ask twice.

import { api, type ShoppingItem } from '$lib/api';
import { type Cadence, normalizeCadence } from './shoppingHelpers';
import type { ShoppingDataController } from './shoppingData.svelte';

export interface ShoppingEditDeps {
  data: ShoppingDataController;
  onError: (message: string) => void;
}

export interface ShoppingEditController {
  // Bindable buffer state.
  editingId: string | null;
  eName: string;
  eCategory: string;
  ePrice: number | '';
  eQuantity: number | '';
  eUrl: string;
  eStandard: boolean;
  eCadence: Cadence;
  eNotes: string;

  /** True iff the given item's row should render the edit form. */
  isEditing(it: ShoppingItem): boolean;

  startEdit(it: ShoppingItem): void;
  cancelEdit(): void;
  commitEdit(): Promise<void>;
}

export function createShoppingEdit(deps: ShoppingEditDeps): ShoppingEditController {
  let editingId = $state<string | null>(null);
  let eName = $state('');
  let eCategory = $state('');
  let ePrice = $state<number | ''>('');
  let eQuantity = $state<number | ''>('');
  let eUrl = $state('');
  let eStandard = $state(false);
  let eCadence = $state<Cadence>('');
  let eNotes = $state('');

  function startEdit(it: ShoppingItem) {
    editingId = it.id;
    eName = it.name;
    eCategory = it.category ?? '';
    ePrice = it.price ?? '';
    eQuantity = it.quantity ?? '';
    eUrl = it.url ?? '';
    eStandard = !!it.standard;
    // Server may return any string but the picker only honours
    // canonical values; everything else round-trips as "no schedule".
    eCadence = normalizeCadence(it.cadence);
    eNotes = it.notes ?? '';
  }

  function cancelEdit() {
    editingId = null;
  }

  async function commitEdit() {
    if (!editingId) return;
    try {
      const updated = await api.patchShoppingItem(editingId, {
        name: eName.trim(),
        category: eCategory.trim() || undefined,
        price: typeof ePrice === 'number' ? ePrice : undefined,
        quantity: typeof eQuantity === 'number' ? eQuantity : undefined,
        url: eUrl.trim() || undefined,
        standard: eStandard,
        // Cadence only meaningful when standard=true; we still send
        // it on non-standard items so a later "mark standard" picks
        // up an existing user intent without re-asking.
        cadence: eCadence,
        notes: eNotes.trim() || undefined
      });
      deps.data.items = deps.data.items.map((x) => (x.id === updated.id ? updated : x));
      editingId = null;
      await deps.data.refreshTotals();
    } catch (e) {
      deps.onError('save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function isEditing(it: ShoppingItem): boolean {
    return editingId === it.id;
  }

  return {
    get editingId() { return editingId; },
    set editingId(v) { editingId = v; },
    get eName() { return eName; },
    set eName(v) { eName = v; },
    get eCategory() { return eCategory; },
    set eCategory(v) { eCategory = v; },
    get ePrice() { return ePrice; },
    set ePrice(v) { ePrice = v; },
    get eQuantity() { return eQuantity; },
    set eQuantity(v) { eQuantity = v; },
    get eUrl() { return eUrl; },
    set eUrl(v) { eUrl = v; },
    get eStandard() { return eStandard; },
    set eStandard(v) { eStandard = v; },
    get eCadence() { return eCadence; },
    set eCadence(v) { eCadence = v; },
    get eNotes() { return eNotes; },
    set eNotes(v) { eNotes = v; },
    isEditing,
    startEdit,
    cancelEdit,
    commitEdit
  };
}
