// View state for the shopping surface.
//
// Second extraction step out of routes/shopping/+page.svelte. Owns
// the view selector — the only view-shaping state on this page.
// Unlike finance's URL-hash mirror, shopping persists to localStorage
// (granit.shopping.view) because the user typically returns to the
// same view across sessions; a shared link to /shopping is rare.
// Pattern mirrors goalsViewState / tasksViewState.
//
// The page reads `view` via the getter and writes via the setter pair
// so `bind:` on the tab cluster still works.

import { loadStoredString, saveStoredString } from '$lib/util/storage';

export type ShoppingView = 'plan' | 'standards' | 'bought';

const VIEW_KEY = 'granit.shopping.view';

export interface ShoppingViewStateController {
  view: ShoppingView;
}

function initialView(): ShoppingView {
  const raw = loadStoredString(VIEW_KEY, 'plan');
  if (raw === 'plan' || raw === 'standards' || raw === 'bought') return raw;
  return 'plan';
}

export function createShoppingViewState(): ShoppingViewStateController {
  let view = $state<ShoppingView>(initialView());

  $effect(() => saveStoredString(VIEW_KEY, view));

  return {
    get view() {
      return view;
    },
    set view(v) {
      view = v;
    }
  };
}
