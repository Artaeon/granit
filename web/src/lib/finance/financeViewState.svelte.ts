// View state for the finance surface.
//
// First extraction step out of routes/finance/+page.svelte (1423 LOC).
// Owns the tab selector — the only view-shaping state on this page.
// Persisted in the URL hash (NOT localStorage like the goals view
// mode) so a shared link lands the recipient on the same tab the
// sender was looking at.
//
// The page reads tab via the getter and routes the on-click setter
// through setTab() so the history.replaceState stays here, not at
// the call site.

export type FinanceTab = 'overview' | 'income' | 'subscriptions' | 'accounts' | 'goals';

export interface FinanceViewStateController {
  // Bindable state.
  tab: FinanceTab;
  /** Set the tab and mirror the choice into the URL hash. */
  setTab(t: FinanceTab): void;
}

function initialTab(): FinanceTab {
  if (typeof window === 'undefined') return 'overview';
  const hash = window.location.hash.replace(/^#/, '') as FinanceTab;
  // Anything that isn't a known tab falls back to overview — the URL
  // could carry junk from another page's deep-link, and silently
  // rendering an empty body would confuse the user.
  if (hash === 'overview' || hash === 'income' || hash === 'subscriptions'
    || hash === 'accounts' || hash === 'goals') {
    return hash;
  }
  return 'overview';
}

export function createFinanceViewState(): FinanceViewStateController {
  let tab = $state<FinanceTab>(initialTab());

  function setTab(t: FinanceTab) {
    tab = t;
    if (typeof window !== 'undefined') {
      history.replaceState(null, '', `${window.location.pathname}#${t}`);
    }
  }

  return {
    get tab() {
      return tab;
    },
    set tab(v) {
      tab = v;
    },
    setTab
  };
}
