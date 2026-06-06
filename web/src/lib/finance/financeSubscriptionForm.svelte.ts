// Subscription create / toggle / delete controller for the finance
// surface.
//
// Pulled out of FinancePane.svelte. Owns the new-subscription modal
// form state, the cents-from-string negation (so users type a
// positive number, the schema records the outflow), and the three
// CRUD wrappers (create / toggle active / delete).
//
// Same dep-injection shape as financeAccountForm — reload + onError +
// onSuccess come from the page so the toast singleton stays out of
// this file.

import { fmtDateISO } from '$lib/util/date';
import { api, type FinAccount, type FinSubscription } from '$lib/api';

export interface FinanceSubscriptionFormDeps {
  /** Used to pre-fill currency + default account from the first
   *  existing entry. */
  getAccounts: () => FinAccount[];
  reload: () => Promise<void>;
  onSuccess: (message: string) => void;
  onError: (message: string) => void;
}

export type SubscriptionFormState = {
  name: string;
  amount: string;
  currency: string;
  cadence: FinSubscription['cadence'];
  next_renewal: string;
  account_id: string;
  project: string;
  tags: string;
  category: string;
  url: string;
};

export interface FinanceSubscriptionFormController {
  open: boolean;
  readonly form: SubscriptionFormState;
  openModal(): void;
  close(): void;
  submit(): Promise<void>;
  toggleActive(s: FinSubscription): Promise<void>;
  remove(s: FinSubscription): Promise<void>;
}

function defaultRenewal(): string {
  return fmtDateISO(new Date(Date.now() + 30 * 86400000));
}

function emptyForm(currency: string, accountId: string): SubscriptionFormState {
  return {
    name: '',
    amount: '',
    currency,
    cadence: 'monthly',
    next_renewal: defaultRenewal(),
    account_id: accountId,
    project: '',
    tags: '',
    category: '',
    url: ''
  };
}

export function createFinanceSubscriptionForm(
  deps: FinanceSubscriptionFormDeps
): FinanceSubscriptionFormController {
  let open = $state(false);
  let form = $state<SubscriptionFormState>(emptyForm('USD', ''));

  function openModal() {
    const first = deps.getAccounts()[0];
    form = emptyForm(first?.currency || 'USD', first?.id ?? '');
    open = true;
  }

  function close() {
    open = false;
  }

  async function submit() {
    try {
      const amt = parseFloat(form.amount || '0');
      // Negate so the schema stays signed-consistent — users type a
      // positive number, the schema records the outflow.
      const cents = -Math.round(Math.abs(amt) * 100);
      await api.finCreateSubscription({
        name: form.name.trim(),
        amount_cents: cents,
        currency: form.currency.trim() || deps.getAccounts()[0]?.currency || 'USD',
        cadence: form.cadence,
        next_renewal: form.next_renewal,
        account_id: form.account_id || undefined,
        project: form.project.trim() || undefined,
        tags: form.tags.split(',').map((t) => t.trim()).filter(Boolean),
        category: form.category.trim() || undefined,
        url: form.url.trim() || undefined,
        active: true
      });
      open = false;
      deps.onSuccess('subscription added');
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function toggleActive(s: FinSubscription) {
    try {
      await api.finPatchSubscription(s.id, { active: !s.active });
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function remove(s: FinSubscription) {
    if (!confirm(`Delete subscription "${s.name}"?`)) return;
    try {
      await api.finDeleteSubscription(s.id);
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get open() { return open; },
    set open(v) { open = v; },
    get form() { return form; },
    openModal,
    close,
    submit,
    toggleActive,
    remove
  };
}
