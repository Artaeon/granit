<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type FinAccount,
    type FinTransaction,
    type FinSubscription,
    type FinHolding,
    type FinGoal,
    type FinOverview
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /finance is a single page with tabs because the entities share a
  // dashboard view (overview) and frequent cross-references (a tx links
  // to an account; a sub links to an account). One page = one fetch
  // batch, one WS subscription, no inter-route handoff for the
  // common edit flows.

  type Tab = 'overview' | 'subscriptions' | 'transactions' | 'accounts' | 'holdings' | 'goals';
  let tab = $state<Tab>(
    typeof window !== 'undefined'
      ? ((window.location.hash.replace(/^#/, '') as Tab) || 'overview')
      : 'overview'
  );
  function setTab(t: Tab) {
    tab = t;
    if (typeof window !== 'undefined') {
      history.replaceState(null, '', `${window.location.pathname}#${t}`);
    }
  }

  let overview = $state<FinOverview | null>(null);
  let accounts = $state<FinAccount[]>([]);
  let txs = $state<FinTransaction[]>([]);
  let subs = $state<FinSubscription[]>([]);
  let holdings = $state<FinHolding[]>([]);
  let goals = $state<FinGoal[]>([]);
  let loading = $state(false);

  // ── shared formatters ──────────────────────────────────────────────
  // Render integer cents in the user's locale, with the right sign +
  // currency symbol. Falls back to the bare number if the browser
  // doesn't know the currency code (offline locale data).
  function fmtMoney(cents: number, currency: string): string {
    if (!Number.isFinite(cents)) return '—';
    const value = cents / 100;
    if (!currency) return value.toFixed(2);
    try {
      return new Intl.NumberFormat(undefined, {
        style: 'currency',
        currency,
        currencyDisplay: 'narrowSymbol'
      }).format(value);
    } catch {
      return `${currency} ${value.toFixed(2)}`;
    }
  }

  // Friendly relative date — "today", "in 3 days", "5 days ago" — for
  // subscription next-renewal labels. Detail view still shows the full
  // date so this only fires on the list cards.
  function relDate(iso: string): string {
    if (!iso) return '';
    const d = new Date(iso + 'T00:00:00');
    if (Number.isNaN(d.getTime())) return iso;
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return 'today';
    if (diff === 1) return 'tomorrow';
    if (diff === -1) return 'yesterday';
    if (diff > 0) return `in ${diff} days`;
    return `${-diff} days ago`;
  }

  function accountName(id: string | undefined): string {
    if (!id) return '—';
    return accounts.find((a) => a.id === id)?.name ?? '(unknown)';
  }

  // ── load ───────────────────────────────────────────────────────────
  async function loadAll() {
    if (!$auth) return;
    loading = true;
    try {
      const [o, a, t, s, h, g] = await Promise.all([
        api.finOverview(),
        api.finListAccounts(),
        api.finListTransactions(),
        api.finListSubscriptions(),
        api.finListHoldings(),
        api.finListGoals()
      ]);
      overview = o;
      accounts = a.accounts;
      txs = t.transactions;
      subs = s.subscriptions;
      holdings = h.holdings;
      goals = g.goals;
    } catch (e) {
      toast.error('failed to load finance: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadAll();
    return onWsEvent((ev) => {
      // Refetch only what changed. The five state files use distinct
      // path constants so we don't reload the whole tab on every edit.
      if (ev.type !== 'state.changed') return;
      if (!ev.path?.startsWith('.granit/finance/')) return;
      loadAll(); // overview always refetches; the per-list arrays do too
    });
  });

  // ── New-account modal ─────────────────────────────────────────────
  let accOpen = $state(false);
  let accForm = $state({
    name: '',
    kind: 'checking' as FinAccount['kind'],
    currency: 'USD',
    balance: '0',
    notes: ''
  });
  function openAcc() {
    accForm = { name: '', kind: 'checking', currency: 'USD', balance: '0', notes: '' };
    accOpen = true;
  }
  async function submitAcc() {
    try {
      await api.finCreateAccount({
        name: accForm.name.trim(),
        kind: accForm.kind,
        currency: accForm.currency.trim(),
        balance_cents: Math.round(parseFloat(accForm.balance || '0') * 100),
        notes: accForm.notes.trim() || undefined
      });
      accOpen = false;
      toast.success('account created');
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteAcc(a: FinAccount) {
    if (!confirm(`Delete account "${a.name}"?`)) return;
    try {
      await api.finDeleteAccount(a.id);
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  // Inline balance edit: save on blur. Keeps the row read-mostly and
  // doesn't require a full edit modal for the most-common edit.
  async function saveBalance(a: FinAccount, ev: Event) {
    const v = parseFloat((ev.currentTarget as HTMLInputElement).value);
    if (!Number.isFinite(v)) return;
    const cents = Math.round(v * 100);
    if (cents === a.balance_cents) return;
    try {
      await api.finPatchAccount(a.id, {
        balance_cents: cents,
        as_of: new Date().toISOString().slice(0, 10)
      });
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── New-subscription modal ─────────────────────────────────────────
  let subOpen = $state(false);
  let subForm = $state({
    name: '',
    amount: '',
    currency: 'USD',
    cadence: 'monthly' as FinSubscription['cadence'],
    next_renewal: new Date().toISOString().slice(0, 10),
    account_id: '',
    category: '',
    url: ''
  });
  function openSub() {
    subForm = {
      name: '',
      amount: '',
      currency: accounts[0]?.currency || 'USD',
      cadence: 'monthly',
      next_renewal: new Date(Date.now() + 30 * 86400000).toISOString().slice(0, 10),
      account_id: accounts[0]?.id ?? '',
      category: '',
      url: ''
    };
    subOpen = true;
  }
  async function submitSub() {
    try {
      const amt = parseFloat(subForm.amount || '0');
      // Subscriptions are outflows by convention — the UI accepts a
      // positive number from the user (people don't write "-9.99")
      // and we negate at the boundary so the schema stays signed-
      // consistent with transactions.
      const cents = -Math.round(Math.abs(amt) * 100);
      await api.finCreateSubscription({
        name: subForm.name.trim(),
        amount_cents: cents,
        currency: subForm.currency.trim() || accounts[0]?.currency || 'USD',
        cadence: subForm.cadence,
        next_renewal: subForm.next_renewal,
        account_id: subForm.account_id || undefined,
        category: subForm.category.trim() || undefined,
        url: subForm.url.trim() || undefined,
        active: true
      });
      subOpen = false;
      toast.success('subscription added');
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function toggleSubActive(s: FinSubscription) {
    try {
      await api.finPatchSubscription(s.id, { active: !s.active });
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteSub(s: FinSubscription) {
    if (!confirm(`Delete subscription "${s.name}"?`)) return;
    try {
      await api.finDeleteSubscription(s.id);
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── New-transaction modal ──────────────────────────────────────────
  let txOpen = $state(false);
  let txForm = $state({
    account_id: '',
    date: new Date().toISOString().slice(0, 10),
    amount: '',
    currency: 'USD',
    category: '',
    description: ''
  });
  let txKind = $state<'expense' | 'income'>('expense');
  function openTx() {
    txForm = {
      account_id: accounts[0]?.id ?? '',
      date: new Date().toISOString().slice(0, 10),
      amount: '',
      currency: accounts[0]?.currency || 'USD',
      category: '',
      description: ''
    };
    txKind = 'expense';
    txOpen = true;
  }
  async function submitTx() {
    if (!txForm.account_id) {
      toast.error('pick an account first');
      return;
    }
    try {
      const v = Math.round(Math.abs(parseFloat(txForm.amount || '0')) * 100);
      const signed = txKind === 'expense' ? -v : v;
      await api.finCreateTransaction({
        account_id: txForm.account_id,
        date: txForm.date,
        amount_cents: signed,
        currency: txForm.currency.trim() || accounts[0]?.currency || 'USD',
        category: txForm.category.trim() || undefined,
        description: txForm.description.trim() || undefined
      });
      txOpen = false;
      toast.success('transaction added');
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteTx(t: FinTransaction) {
    if (!confirm('Delete this transaction?')) return;
    try {
      await api.finDeleteTransaction(t.id);
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── Holding + goal create (smaller forms — single submit) ──────────
  let holdingOpen = $state(false);
  let holdingForm = $state({ account_id: '', ticker: '', quantity: '', cost_basis: '', currency: 'USD' });
  function openHolding() {
    holdingForm = {
      account_id: accounts.find((a) => a.kind === 'investment')?.id ?? accounts[0]?.id ?? '',
      ticker: '',
      quantity: '',
      cost_basis: '',
      currency: accounts[0]?.currency || 'USD'
    };
    holdingOpen = true;
  }
  async function submitHolding() {
    if (!holdingForm.account_id || !holdingForm.ticker.trim()) return;
    try {
      await api.finCreateHolding({
        account_id: holdingForm.account_id,
        ticker: holdingForm.ticker.trim().toUpperCase(),
        quantity: parseFloat(holdingForm.quantity || '0'),
        cost_basis_cents: Math.round(parseFloat(holdingForm.cost_basis || '0') * 100),
        currency: holdingForm.currency.trim() || 'USD'
      });
      holdingOpen = false;
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteHolding(h: FinHolding) {
    if (!confirm(`Delete ${h.ticker}?`)) return;
    try {
      await api.finDeleteHolding(h.id);
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  let goalOpen = $state(false);
  let goalForm = $state({
    name: '',
    kind: 'savings' as FinGoal['kind'],
    target: '',
    current: '0',
    currency: 'USD',
    target_date: '',
    linked_account_id: ''
  });
  function openGoal() {
    goalForm = {
      name: '',
      kind: 'savings',
      target: '',
      current: '0',
      currency: accounts[0]?.currency || 'USD',
      target_date: '',
      linked_account_id: ''
    };
    goalOpen = true;
  }
  async function submitGoal() {
    if (!goalForm.name.trim() || !goalForm.target) return;
    try {
      await api.finCreateGoal({
        name: goalForm.name.trim(),
        kind: goalForm.kind,
        target_cents: Math.round(parseFloat(goalForm.target) * 100),
        current_cents: Math.round(parseFloat(goalForm.current || '0') * 100),
        currency: goalForm.currency.trim() || 'USD',
        target_date: goalForm.target_date || undefined,
        linked_account_id: goalForm.linked_account_id || undefined
      });
      goalOpen = false;
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteGoal(g: FinGoal) {
    if (!confirm(`Delete goal "${g.name}"?`)) return;
    try {
      await api.finDeleteGoal(g.id);
      await loadAll();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Distinct categories from existing transactions — surfaced as
  // <datalist> on the new-transaction form so the user gets a free
  // taxonomy without re-typing "Groceries" every time.
  let txCategories = $derived.by(() => {
    const s = new Set<string>();
    for (const t of txs) if (t.category) s.add(t.category);
    return [...s].sort();
  });
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-5xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Finance" subtitle="Net worth, subscriptions, transactions, holdings, goals" />

    <!-- Tabs. Hash-mirrored so refresh keeps the user where they were. -->
    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-6 flex-wrap">
      {#each [
        { id: 'overview' as Tab, label: 'Overview' },
        { id: 'subscriptions' as Tab, label: 'Subscriptions', count: subs.length },
        { id: 'transactions' as Tab, label: 'Transactions', count: txs.length },
        { id: 'accounts' as Tab, label: 'Accounts', count: accounts.length },
        { id: 'holdings' as Tab, label: 'Holdings', count: holdings.length },
        { id: 'goals' as Tab, label: 'Goals', count: goals.length }
      ] as t}
        <button
          class="px-3 sm:px-4 py-2 {tab === t.id ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => setTab(t.id)}
        >
          {t.label}{#if t.count !== undefined && t.count > 0}<span class="ml-1 text-xs opacity-70">{t.count}</span>{/if}
        </button>
      {/each}
    </div>

    {#if loading && !overview}
      <p class="text-sm text-dim">loading…</p>
    {:else if tab === 'overview'}
      {#if overview}
        <div class="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">Net worth</p>
            <p class="text-2xl font-semibold mt-1 {overview.net_worth_cents >= 0 ? 'text-text' : 'text-error'}">
              {fmtMoney(overview.net_worth_cents, overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              {fmtMoney(overview.assets_cents, overview.currency)} assets
              {#if overview.liabilities_cents > 0}
                · {fmtMoney(overview.liabilities_cents, overview.currency)} debt
              {/if}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">30-day flow</p>
            <p class="text-lg font-semibold mt-1 text-success">
              + {fmtMoney(overview.monthly_income_cents, overview.currency)}
            </p>
            <p class="text-lg font-semibold text-error">
              − {fmtMoney(overview.monthly_outflow_cents, overview.currency)}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">Subscriptions</p>
            <p class="text-2xl font-semibold mt-1 text-text">
              {fmtMoney(overview.subscription_monthly_cents, overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">/ month
              {#if overview.upcoming_subs_count > 0}
                · <span class="text-warning">{overview.upcoming_subs_count} due in 7 days</span>
              {/if}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">Active goals</p>
            <p class="text-2xl font-semibold mt-1 text-text">{overview.goals_active_count}</p>
            <p class="text-[11px] text-dim mt-1">{overview.accounts_count} accounts · {overview.transactions_count} transactions</p>
          </div>
        </div>

        <!-- Quick actions -->
        <div class="flex flex-wrap gap-2">
          <button onclick={openTx} class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90">+ Transaction</button>
          <button onclick={openSub} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Subscription</button>
          <button onclick={openAcc} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Account</button>
          <button onclick={openGoal} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Goal</button>
        </div>

        {#if accounts.length === 0 && subs.length === 0 && txs.length === 0}
          <div class="mt-8 bg-surface0 border border-surface1 rounded-lg p-6 text-center">
            <p class="text-sm text-text">Welcome to your money tracker.</p>
            <p class="text-xs text-dim mt-1">Start by adding an account, then track subscriptions and transactions against it.</p>
          </div>
        {/if}
      {/if}

    {:else if tab === 'subscriptions'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{subs.length} subscriptions · {fmtMoney(overview?.subscription_monthly_cents ?? 0, overview?.currency ?? '')}/mo</p>
        <button onclick={openSub} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New subscription</button>
      </div>
      {#if subs.length === 0}
        <p class="text-sm text-dim italic">No subscriptions yet — add your first to start tracking recurring outflows.</p>
      {:else}
        <ul class="space-y-2">
          {#each subs as s (s.id)}
            <li class="bg-surface0 border border-surface1 rounded-lg p-3 {s.active ? '' : 'opacity-60'}">
              <div class="flex items-baseline gap-3 flex-wrap">
                <h3 class="font-medium text-text">{s.name}</h3>
                <span class="text-sm text-error font-mono">{fmtMoney(s.amount_cents, s.currency)}</span>
                <span class="text-xs text-dim">/ {s.cadence}</span>
                <span class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-subtext">≈ {fmtMoney(Math.abs((s.amount_cents * (s.cadence === 'yearly' ? 1 : s.cadence === 'quarterly' ? 4 : s.cadence === 'weekly' ? 52 : 12)) / 12), s.currency)}/mo</span>
                <span class="flex-1"></span>
                <button onclick={() => toggleSubActive(s)} class="text-xs text-dim hover:text-text">{s.active ? 'pause' : 'resume'}</button>
                <button onclick={() => deleteSub(s)} class="text-xs text-dim hover:text-error">delete</button>
              </div>
              <p class="text-xs text-dim mt-1">
                next: <span class="text-subtext">{s.next_renewal}</span> · <span class="{relDate(s.next_renewal).includes('ago') ? 'text-error' : ''}">{relDate(s.next_renewal)}</span>
                {#if s.account_id}· billed to {accountName(s.account_id)}{/if}
                {#if s.category}· {s.category}{/if}
                {#if s.url}· <a href={s.url} target="_blank" rel="noopener" class="text-secondary hover:underline">manage ↗</a>{/if}
              </p>
            </li>
          {/each}
        </ul>
      {/if}

    {:else if tab === 'transactions'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{txs.length} transactions · last 30d: <span class="text-success">+{fmtMoney(overview?.monthly_income_cents ?? 0, overview?.currency ?? '')}</span> / <span class="text-error">−{fmtMoney(overview?.monthly_outflow_cents ?? 0, overview?.currency ?? '')}</span></p>
        <button onclick={openTx} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ Transaction</button>
      </div>
      {#if txs.length === 0}
        <p class="text-sm text-dim italic">No transactions yet.</p>
      {:else}
        <ul class="divide-y divide-surface1 bg-surface0/40 border border-surface1 rounded-lg">
          {#each txs.slice(0, 200) as t (t.id)}
            <li class="px-3 py-2 flex items-baseline gap-3">
              <span class="text-xs text-dim font-mono w-20 flex-shrink-0">{t.date}</span>
              <div class="flex-1 min-w-0">
                <p class="text-sm text-text truncate">{t.description || t.category || '(no description)'}</p>
                <p class="text-[11px] text-dim">{accountName(t.account_id)}{#if t.category && t.description}· {t.category}{/if}</p>
              </div>
              <span class="text-sm font-mono {t.amount_cents >= 0 ? 'text-success' : 'text-error'}">
                {t.amount_cents >= 0 ? '+' : '−'}{fmtMoney(Math.abs(t.amount_cents), t.currency)}
              </span>
              <button onclick={() => deleteTx(t)} aria-label="delete" class="text-xs text-dim hover:text-error">×</button>
            </li>
          {/each}
        </ul>
        {#if txs.length > 200}<p class="text-[11px] text-dim mt-2 italic">showing first 200 of {txs.length}</p>{/if}
      {/if}

    {:else if tab === 'accounts'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{accounts.length} accounts</p>
        <button onclick={openAcc} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New account</button>
      </div>
      {#if accounts.length === 0}
        <p class="text-sm text-dim italic">No accounts yet — add your first to start tracking.</p>
      {:else}
        <ul class="space-y-2">
          {#each accounts as a (a.id)}
            <li class="bg-surface0 border border-surface1 rounded-lg p-3 flex items-baseline gap-3 flex-wrap {a.archived ? 'opacity-50' : ''}">
              <h3 class="font-medium text-text">{a.name}</h3>
              <span class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-subtext">{a.kind}</span>
              <span class="text-xs text-dim">{a.currency}</span>
              <span class="flex-1"></span>
              <label class="text-xs text-dim flex items-center gap-1.5">balance
                <input
                  type="number"
                  step="0.01"
                  value={(a.balance_cents / 100).toFixed(2)}
                  onblur={(e) => saveBalance(a, e)}
                  class="w-28 bg-mantle border border-surface1 rounded px-1.5 py-0.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary"
                />
              </label>
              <button onclick={() => deleteAcc(a)} class="text-xs text-dim hover:text-error">delete</button>
            </li>
          {/each}
        </ul>
      {/if}

    {:else if tab === 'holdings'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{holdings.length} positions across {new Set(holdings.map((h) => h.account_id)).size} accounts</p>
        <button onclick={openHolding} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New position</button>
      </div>
      {#if holdings.length === 0}
        <p class="text-sm text-dim italic">No holdings yet — add a position to track cost basis.</p>
      {:else}
        <ul class="divide-y divide-surface1 bg-surface0/40 border border-surface1 rounded-lg">
          {#each holdings as h (h.id)}
            <li class="px-3 py-2 flex items-baseline gap-3">
              <span class="font-mono text-text font-semibold w-16 flex-shrink-0">{h.ticker}</span>
              <span class="text-xs text-dim flex-1 min-w-0 truncate">{h.name || ''} · {accountName(h.account_id)}</span>
              <span class="text-sm font-mono text-text">{h.quantity}</span>
              <span class="text-sm font-mono text-subtext">@ {fmtMoney(h.cost_basis_cents, h.currency)}</span>
              <span class="text-sm font-mono text-text">= {fmtMoney(Math.round(h.cost_basis_cents * h.quantity), h.currency)}</span>
              <button onclick={() => deleteHolding(h)} aria-label="delete" class="text-xs text-dim hover:text-error">×</button>
            </li>
          {/each}
        </ul>
        <p class="text-[11px] text-dim italic mt-2">
          Cost basis only — live prices not fetched. Total: {fmtMoney(holdings.reduce((s, h) => s + Math.round(h.cost_basis_cents * h.quantity), 0), overview?.currency ?? 'USD')}
        </p>
      {/if}

    {:else if tab === 'goals'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{goals.length} financial goals</p>
        <button onclick={openGoal} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New goal</button>
      </div>
      {#if goals.length === 0}
        <p class="text-sm text-dim italic">No financial goals yet.</p>
      {:else}
        <ul class="space-y-3">
          {#each goals as g (g.id)}
            {@const pct = g.target_cents > 0 ? Math.min(100, Math.round((g.current_cents / g.target_cents) * 100)) : 0}
            <li class="bg-surface0 border border-surface1 rounded-lg p-3">
              <div class="flex items-baseline gap-3 flex-wrap">
                <h3 class="font-medium text-text">{g.name}</h3>
                <span class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-subtext">{g.kind}</span>
                <span class="text-sm font-mono text-text">{fmtMoney(g.current_cents, g.currency)} / {fmtMoney(g.target_cents, g.currency)}</span>
                <span class="text-xs text-dim">{pct}%</span>
                <span class="flex-1"></span>
                {#if g.target_date}<span class="text-xs text-dim">by {g.target_date}</span>{/if}
                <button onclick={() => deleteGoal(g)} class="text-xs text-dim hover:text-error">delete</button>
              </div>
              <div class="h-1.5 mt-2 bg-mantle rounded-full overflow-hidden">
                <div class="h-full bg-primary transition-all" style="width: {pct}%"></div>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  </div>
</div>

<!-- ── New-account modal ────────────────────────────────────────────── -->
{#if accOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (accOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') accOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitAcc(); }} class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">New account</h2>
      <input bind:value={accForm.name} required placeholder="Name" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <select bind:value={accForm.kind} class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
        <option value="checking">Checking</option>
        <option value="savings">Savings</option>
        <option value="cash">Cash</option>
        <option value="credit">Credit card</option>
        <option value="investment">Investment</option>
        <option value="loan">Loan</option>
      </select>
      <div class="flex gap-2">
        <input bind:value={accForm.currency} placeholder="USD" class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
        <input type="number" step="0.01" bind:value={accForm.balance} placeholder="0.00" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
      </div>
      <input bind:value={accForm.notes} placeholder="Notes (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (accOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Create</button>
      </div>
    </form>
  </div>
{/if}

<!-- ── New-subscription modal ───────────────────────────────────────── -->
{#if subOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (subOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') subOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitSub(); }} class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">New subscription</h2>
      <input bind:value={subForm.name} required placeholder="Name (Netflix, Spotify…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="flex gap-2">
        <input type="number" step="0.01" bind:value={subForm.amount} required placeholder="9.99" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input bind:value={subForm.currency} class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
        <select bind:value={subForm.cadence} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
          <option value="weekly">/ week</option>
          <option value="monthly">/ month</option>
          <option value="quarterly">/ quarter</option>
          <option value="yearly">/ year</option>
        </select>
      </div>
      <label class="block text-xs text-dim">Next renewal
        <input type="date" bind:value={subForm.next_renewal} class="block mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </label>
      {#if accounts.length > 0}
        <select bind:value={subForm.account_id} class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
          <option value="">— no account link —</option>
          {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
        </select>
      {/if}
      <input bind:value={subForm.category} placeholder="Category (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <input bind:value={subForm.url} placeholder="Manage URL (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (subOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Add</button>
      </div>
    </form>
  </div>
{/if}

<!-- ── New-transaction modal ────────────────────────────────────────── -->
{#if txOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (txOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') txOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitTx(); }} class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">New transaction</h2>
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
        <button type="button" onclick={() => (txKind = 'expense')} class="flex-1 px-3 py-1.5 {txKind === 'expense' ? 'bg-error/15 text-error' : 'text-subtext'}">Expense</button>
        <button type="button" onclick={() => (txKind = 'income')} class="flex-1 px-3 py-1.5 {txKind === 'income' ? 'bg-success/15 text-success' : 'text-subtext'}">Income</button>
      </div>
      <select bind:value={txForm.account_id} required class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
        <option value="">— pick an account —</option>
        {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
      </select>
      <div class="flex gap-2">
        <input type="date" bind:value={txForm.date} class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
        <input type="number" step="0.01" bind:value={txForm.amount} required placeholder="0.00" class="w-28 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
      </div>
      <input bind:value={txForm.category} list="tx-cat-list" placeholder="Category (Groceries, Salary…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <datalist id="tx-cat-list">
        {#each txCategories as c}<option value={c}></option>{/each}
      </datalist>
      <input bind:value={txForm.description} placeholder="Description (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (txOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Add</button>
      </div>
    </form>
  </div>
{/if}

<!-- ── New-holding modal ────────────────────────────────────────────── -->
{#if holdingOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (holdingOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') holdingOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitHolding(); }} class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">New position</h2>
      <select bind:value={holdingForm.account_id} required class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
        <option value="">— pick an account —</option>
        {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
      </select>
      <input bind:value={holdingForm.ticker} required placeholder="Ticker (VTI, BTC…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono uppercase focus:outline-none focus:border-primary" />
      <div class="flex gap-2">
        <input type="number" step="any" bind:value={holdingForm.quantity} required placeholder="qty" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input type="number" step="0.01" bind:value={holdingForm.cost_basis} placeholder="cost / unit" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input bind:value={holdingForm.currency} class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </div>
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (holdingOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Add</button>
      </div>
    </form>
  </div>
{/if}

<!-- ── New-goal modal ───────────────────────────────────────────────── -->
{#if goalOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (goalOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') goalOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitGoal(); }} class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">New financial goal</h2>
      <input bind:value={goalForm.name} required placeholder="Name (Emergency fund, Pay off card…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <select bind:value={goalForm.kind} class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
        <option value="savings">Savings (build up to target)</option>
        <option value="payoff">Payoff (shrink debt to zero)</option>
        <option value="networth">Net worth (aggregate target)</option>
      </select>
      <div class="flex gap-2">
        <input type="number" step="0.01" bind:value={goalForm.target} required placeholder="Target" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input type="number" step="0.01" bind:value={goalForm.current} placeholder="Current" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input bind:value={goalForm.currency} class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </div>
      <label class="block text-xs text-dim">Target date (optional)
        <input type="date" bind:value={goalForm.target_date} class="block mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </label>
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (goalOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Add</button>
      </div>
    </form>
  </div>
{/if}
