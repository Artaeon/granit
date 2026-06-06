// Income-stream create / edit / delete controller for the finance
// surface.
//
// Same dep-injection shape as financeAccountForm and
// financeSubscriptionForm — the page wires reload + toasts in once
// and the income tab + modal bind through incomeForm.X.
//
// One modal handles both create and edit. editingId tracks "we're
// editing this one" vs "we're making a fresh one"; submit() branches
// accordingly. open(s?) hydrates the form from an existing record
// when passed, otherwise resets to defaults pulled from the first
// existing account.
//
// Subtle bits preserved from the page:
//   • Empty payout day means "unknown" → send as 0 so the server
//     stores nothing. Days outside 1-31 get clamped at the form
//     level so the schema stays sane regardless of weird input.
//   • Flipping a stream to active stamps started_at if the user
//     hasn't already — saves them re-typing the date. Needs the
//     existing record to read the prior started_at; the controller
//     resolves it through getStreams() at submit time.

import { api, todayISO, type FinAccount, type FinIncomeStream } from '$lib/api';

export interface FinanceIncomeFormDeps {
  /** Pre-fill currency + default account on a fresh open. */
  getAccounts: () => FinAccount[];
  /** Used to look up the prior started_at when editing — see the
   *  "stamp started_at on flip-to-active" note above. */
  getStreams: () => FinIncomeStream[];
  reload: () => Promise<void>;
  onSuccess: (message: string) => void;
  onError: (message: string) => void;
}

export type IncomeFormState = {
  name: string;
  status: FinIncomeStream['status'];
  kind: FinIncomeStream['kind'];
  projected: string;
  actual: string;
  currency: string;
  payout_day: string;
  payout_cadence: FinIncomeStream['payout_cadence'];
  account_id: string;
  project: string;
  tags: string;
  url: string;
  notes: string;
};

export interface FinanceIncomeFormController {
  open: boolean;
  readonly form: IncomeFormState;
  /** Null when creating a fresh stream; the stream id when editing. */
  editingId: string | null;
  /** Open the modal. Pass an existing stream to edit; omit for a
   *  fresh create. */
  openModal(s?: FinIncomeStream): void;
  close(): void;
  submit(): Promise<void>;
  remove(s: FinIncomeStream): Promise<void>;
}

function emptyForm(currency: string, accountId: string): IncomeFormState {
  return {
    name: '',
    status: 'idea',
    kind: 'business',
    projected: '',
    actual: '',
    currency,
    payout_day: '',
    payout_cadence: 'monthly',
    account_id: accountId,
    project: '',
    tags: '',
    url: '',
    notes: ''
  };
}

function fromStream(s: FinIncomeStream): IncomeFormState {
  return {
    name: s.name,
    status: s.status as FinIncomeStream['status'],
    kind: s.kind as FinIncomeStream['kind'],
    projected: (s.projected_monthly_cents / 100).toFixed(2),
    actual: (s.actual_monthly_cents / 100).toFixed(2),
    currency: s.currency,
    payout_day: s.payout_day_of_month ? String(s.payout_day_of_month) : '',
    payout_cadence: (s.payout_cadence ?? 'monthly') as FinIncomeStream['payout_cadence'],
    account_id: s.account_id ?? '',
    project: s.project ?? '',
    tags: (s.tags ?? []).join(', '),
    url: s.url ?? '',
    notes: s.notes ?? ''
  };
}

export function createFinanceIncomeForm(
  deps: FinanceIncomeFormDeps
): FinanceIncomeFormController {
  let open = $state(false);
  let editingId = $state<string | null>(null);
  let form = $state<IncomeFormState>(emptyForm('USD', ''));

  function openModal(s?: FinIncomeStream) {
    if (s) {
      editingId = s.id;
      form = fromStream(s);
    } else {
      editingId = null;
      const first = deps.getAccounts()[0];
      form = emptyForm(first?.currency || 'USD', first?.id ?? '');
    }
    open = true;
  }

  function close() {
    open = false;
    editingId = null;
  }

  async function submit() {
    try {
      // Empty payout day means "unknown" — send as 0 so the server
      // stores nothing. Days outside 1-31 get clamped at the form
      // level so the schema stays sane regardless of weird input.
      const day = form.payout_day
        ? Math.max(0, Math.min(31, parseInt(form.payout_day, 10)))
        : 0;
      const body: Partial<FinIncomeStream> = {
        name: form.name.trim(),
        status: form.status,
        kind: form.kind,
        projected_monthly_cents: Math.round(parseFloat(form.projected || '0') * 100),
        actual_monthly_cents: Math.round(parseFloat(form.actual || '0') * 100),
        currency: form.currency.trim() || 'USD',
        payout_day_of_month: day,
        payout_cadence: form.payout_cadence || 'monthly',
        account_id: form.account_id || undefined,
        project: form.project.trim() || undefined,
        tags: form.tags.split(',').map((t) => t.trim()).filter(Boolean),
        url: form.url.trim() || undefined,
        notes: form.notes.trim() || undefined,
        // When the user flips a stream to active, stamp started_at if
        // they haven't already — saves them re-typing the date.
        started_at: form.status === 'active'
          ? (deps.getStreams().find((x) => x.id === editingId)?.started_at
             || todayISO())
          : undefined
      };
      if (editingId) {
        await api.finPatchIncome(editingId, body);
        deps.onSuccess('updated');
      } else {
        await api.finCreateIncome(body);
        deps.onSuccess('income stream added');
      }
      open = false;
      editingId = null;
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function remove(s: FinIncomeStream) {
    if (!confirm(`Delete "${s.name}"?`)) return;
    try {
      await api.finDeleteIncome(s.id);
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get open() { return open; },
    set open(v) { open = v; },
    get form() { return form; },
    get editingId() { return editingId; },
    set editingId(v) { editingId = v; },
    openModal,
    close,
    submit,
    remove
  };
}
