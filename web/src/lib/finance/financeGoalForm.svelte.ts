// Financial-goal create / delete controller for the finance surface.
//
// Smallest of the four form extractions — goals are simpler than
// streams or subscriptions (no edit modal, no toggle, no schedule).
// Same dep-injection shape as its siblings so the page can wire all
// four controllers identically.
//
// submit() short-circuits when name or target is empty — same guard
// the inline handler used to have. The render also still gates the
// submit button on the same fields, so this matters only when a
// user submits via Enter.

import { api, type FinAccount, type FinGoal } from '$lib/api';

export interface FinanceGoalFormDeps {
  getAccounts: () => FinAccount[];
  reload: () => Promise<void>;
  onError: (message: string) => void;
}

export type GoalFormState = {
  name: string;
  kind: FinGoal['kind'];
  target: string;
  current: string;
  currency: string;
  target_date: string;
  linked_account_id: string;
};

export interface FinanceGoalFormController {
  open: boolean;
  form: GoalFormState;
  openModal(): void;
  close(): void;
  submit(): Promise<void>;
  remove(g: FinGoal): Promise<void>;
}

function emptyForm(currency: string): GoalFormState {
  return {
    name: '',
    kind: 'savings',
    target: '',
    current: '0',
    currency,
    target_date: '',
    linked_account_id: ''
  };
}

export function createFinanceGoalForm(
  deps: FinanceGoalFormDeps
): FinanceGoalFormController {
  let open = $state(false);
  let form = $state<GoalFormState>(emptyForm('USD'));

  function openModal() {
    form = emptyForm(deps.getAccounts()[0]?.currency || 'USD');
    open = true;
  }

  function close() {
    open = false;
  }

  async function submit() {
    if (!form.name.trim() || !form.target) return;
    try {
      await api.finCreateGoal({
        name: form.name.trim(),
        kind: form.kind,
        target_cents: Math.round(parseFloat(form.target) * 100),
        current_cents: Math.round(parseFloat(form.current || '0') * 100),
        currency: form.currency.trim() || 'USD',
        target_date: form.target_date || undefined,
        linked_account_id: form.linked_account_id || undefined
      });
      open = false;
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function remove(g: FinGoal) {
    if (!confirm(`Delete goal "${g.name}"?`)) return;
    try {
      await api.finDeleteGoal(g.id);
      await deps.reload();
    } catch (e) {
      deps.onError('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get open() { return open; },
    set open(v) { open = v; },
    get form() { return form; },
    set form(v) { form = v; },
    openModal,
    close,
    submit,
    remove
  };
}
