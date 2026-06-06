// Account create / inline-edit / delete controller for the finance
// surface.
//
// Pulled out of FinancePane.svelte so the page doesn't have to own the
// modal form-state, the cents-from-string parsing, and the three CRUD
// wrappers (create / inline balance patch / delete). The accounts tab
// reads `accForm.X` for two-way `bind:` and calls `open / submit /
// saveBalance / delete` from buttons + the inline balance input.
//
// On every successful write the controller asks the page to reload
// the data slice via `deps.reload()` so the new / updated account
// appears immediately in every list, picker, and the overview.
// Errors get routed through `deps.onError` — same shape as
// financeData, keeps the toast import out of this file.

import { api, todayISO, type FinAccount } from '$lib/api';

export interface FinanceAccountFormDeps {
  /** Read the current account list so the new-account modal can
   *  pre-fill currency from the first existing account. */
  getAccounts: () => FinAccount[];
  /** Re-fetch the finance data after a successful write. The page
   *  binds this to dataCtl.loadAll. */
  reload: () => Promise<void>;
  /** Surface a success message — typically wired to toast.success. */
  onSuccess: (message: string) => void;
  /** Surface a failure message — typically wired to toast.error. */
  onError: (message: string) => void;
}

export type AccountFormState = {
  name: string;
  kind: string;
  currency: string;
  balance: string;
  institution: string;
  color: string;
  tags: string;
  notes: string;
};

export interface FinanceAccountFormController {
  open: boolean;
  readonly form: AccountFormState;
  /** Reset the form to defaults (first existing account's currency)
   *  and open the modal. */
  openModal(): void;
  /** Close without writing. */
  close(): void;
  submit(): Promise<void>;
  /** Inline balance edit on blur — no modal for the most-frequent
   *  edit. Skips writes when the parsed cents match the current value. */
  saveBalance(a: FinAccount, ev: Event): Promise<void>;
  remove(a: FinAccount): Promise<void>;
}

function emptyForm(currency: string): AccountFormState {
  return {
    name: '',
    kind: 'checking',
    currency,
    balance: '0',
    institution: '',
    color: '',
    tags: '',
    notes: ''
  };
}

export function createFinanceAccountForm(
  deps: FinanceAccountFormDeps
): FinanceAccountFormController {
  let open = $state(false);
  let form = $state<AccountFormState>(emptyForm('USD'));

  function openModal() {
    form = emptyForm(deps.getAccounts()[0]?.currency || 'USD');
    open = true;
  }

  function close() {
    open = false;
  }

  async function submit() {
    try {
      await api.finCreateAccount({
        name: form.name.trim(),
        kind: form.kind,
        currency: form.currency.trim(),
        balance_cents: Math.round(parseFloat(form.balance || '0') * 100),
        institution: form.institution.trim() || undefined,
        color: form.color || undefined,
        tags: form.tags.split(',').map((t) => t.trim()).filter(Boolean),
        notes: form.notes.trim() || undefined
      });
      open = false;
      deps.onSuccess('account created');
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function saveBalance(a: FinAccount, ev: Event) {
    const v = parseFloat((ev.currentTarget as HTMLInputElement).value);
    if (!Number.isFinite(v)) return;
    const cents = Math.round(v * 100);
    if (cents === a.balance_cents) return;
    try {
      await api.finPatchAccount(a.id, { balance_cents: cents, as_of: todayISO() });
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function remove(a: FinAccount) {
    if (!confirm(`Delete account "${a.name}"?`)) return;
    try {
      await api.finDeleteAccount(a.id);
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get open() { return open; },
    set open(v) { open = v; },
    // form is read-only; the modal binds individual fields via
    // `bind:value={ctl.form.X}` (Svelte 5 handles nested-property
    // writes through the object reference). openModal() owns reset.
    get form() { return form; },
    openModal,
    close,
    submit,
    saveBalance,
    remove
  };
}
