<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type FinAccount,
    type FinSubscription,
    type FinIncomeStream,
    type FinGoal,
    type FinOverview,
    type Project
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /finance covers the four things that actually matter for tracking
  // a financial life: how much money I have (Accounts → Net worth),
  // recurring drag (Subscriptions), income — both active sources and
  // pipeline ventures (Income), and money goals (Goals). Overview is
  // a single landing page that pulls the headline numbers from the
  // composite endpoint.

  type Tab = 'overview' | 'income' | 'subscriptions' | 'accounts' | 'goals';
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
  let subs = $state<FinSubscription[]>([]);
  let streams = $state<FinIncomeStream[]>([]);
  let goals = $state<FinGoal[]>([]);
  // Projects feed the project-link pickers on income + subscription
  // forms. Loaded alongside the rest so the dropdowns hydrate without
  // a follow-up fetch when the user clicks "+ New".
  let projects = $state<Project[]>([]);
  let loading = $state(false);

  // Account color → CSS variable. Empty / unknown falls through to
  // surface1 so the row pip is just visible without yelling.
  function accColor(c: string | undefined): string {
    if (!c) return 'var(--color-surface2)';
    const map: Record<string, string> = {
      red: 'var(--color-error)',
      orange: 'var(--color-accent)',
      yellow: 'var(--color-warning)',
      green: 'var(--color-success)',
      blue: 'var(--color-secondary)',
      purple: 'var(--color-primary)',
      cyan: 'var(--color-info)'
    };
    return map[c] ?? 'var(--color-surface2)';
  }

  // Render integer cents in the user's locale. Falls back to
  // "<CCY> <amount>" if the browser doesn't know the code.
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
      const [o, a, s, i, g, p] = await Promise.all([
        api.finOverview(),
        api.finListAccounts(),
        api.finListSubscriptions(),
        api.finListIncome(),
        api.finListGoals(),
        // Projects are read-only here — fetched only to populate
        // pickers on income + subscription create/edit. A failure
        // shouldn't break the finance page; fall through with empty.
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 }))
      ]);
      overview = o;
      accounts = a.accounts;
      subs = s.subscriptions;
      streams = i.streams;
      goals = g.goals;
      projects = p.projects;
    } catch (e) {
      toast.error('failed to load finance: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadAll();
    return onWsEvent((ev) => {
      if (ev.type !== 'state.changed') return;
      if (!ev.path?.startsWith('.granit/finance/')) return;
      loadAll();
    });
  });

  // ── status pill colors ─────────────────────────────────────────────
  function statusTone(s: string): { bg: string; text: string; label: string } {
    switch (s) {
      case 'active':  return { bg: 'bg-success/15', text: 'text-success', label: 'Active' };
      case 'planned': return { bg: 'bg-info/15',    text: 'text-info',    label: 'Planned' };
      case 'idea':    return { bg: 'bg-primary/15', text: 'text-primary', label: 'Idea' };
      case 'paused':  return { bg: 'bg-surface1',   text: 'text-dim',     label: 'Paused' };
      default:        return { bg: 'bg-surface1',   text: 'text-subtext', label: s || '—' };
    }
  }

  // ── New-account modal ─────────────────────────────────────────────
  let accOpen = $state(false);
  let accForm = $state({ name: '', kind: 'checking', currency: 'USD', balance: '0', institution: '', color: '', tags: '', notes: '' });
  function openAcc() {
    accForm = { name: '', kind: 'checking', currency: accounts[0]?.currency || 'USD', balance: '0', institution: '', color: '', tags: '', notes: '' };
    accOpen = true;
  }
  async function submitAcc() {
    try {
      await api.finCreateAccount({
        name: accForm.name.trim(),
        kind: accForm.kind,
        currency: accForm.currency.trim(),
        balance_cents: Math.round(parseFloat(accForm.balance || '0') * 100),
        institution: accForm.institution.trim() || undefined,
        color: accForm.color || undefined,
        tags: accForm.tags.split(',').map((t) => t.trim()).filter(Boolean),
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
    try { await api.finDeleteAccount(a.id); await loadAll(); } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  // Inline balance edit on blur — no modal for the most-frequent edit.
  async function saveBalance(a: FinAccount, ev: Event) {
    const v = parseFloat((ev.currentTarget as HTMLInputElement).value);
    if (!Number.isFinite(v)) return;
    const cents = Math.round(v * 100);
    if (cents === a.balance_cents) return;
    try {
      await api.finPatchAccount(a.id, { balance_cents: cents, as_of: new Date().toISOString().slice(0, 10) });
      await loadAll();
    } catch (e) { toast.error('failed: ' + (e instanceof Error ? e.message : String(e))); }
  }

  // ── New-subscription modal ─────────────────────────────────────────
  let subOpen = $state(false);
  let subForm = $state({
    name: '', amount: '', currency: 'USD',
    cadence: 'monthly' as FinSubscription['cadence'],
    next_renewal: new Date(Date.now() + 30 * 86400000).toISOString().slice(0, 10),
    account_id: '', project: '', tags: '', category: '', url: ''
  });
  function openSub() {
    subForm = {
      name: '', amount: '',
      currency: accounts[0]?.currency || 'USD',
      cadence: 'monthly',
      next_renewal: new Date(Date.now() + 30 * 86400000).toISOString().slice(0, 10),
      account_id: accounts[0]?.id ?? '',
      project: '',
      tags: '',
      category: '', url: ''
    };
    subOpen = true;
  }
  async function submitSub() {
    try {
      const amt = parseFloat(subForm.amount || '0');
      // Negate so the schema stays signed-consistent — users type a
      // positive number, the schema records the outflow.
      const cents = -Math.round(Math.abs(amt) * 100);
      await api.finCreateSubscription({
        name: subForm.name.trim(),
        amount_cents: cents,
        currency: subForm.currency.trim() || accounts[0]?.currency || 'USD',
        cadence: subForm.cadence,
        next_renewal: subForm.next_renewal,
        account_id: subForm.account_id || undefined,
        project: subForm.project.trim() || undefined,
        tags: subForm.tags.split(',').map((t) => t.trim()).filter(Boolean),
        category: subForm.category.trim() || undefined,
        url: subForm.url.trim() || undefined,
        active: true
      });
      subOpen = false;
      toast.success('subscription added');
      await loadAll();
    } catch (e) { toast.error('failed: ' + (e instanceof Error ? e.message : String(e))); }
  }
  async function toggleSubActive(s: FinSubscription) {
    try { await api.finPatchSubscription(s.id, { active: !s.active }); await loadAll(); } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deleteSub(s: FinSubscription) {
    if (!confirm(`Delete subscription "${s.name}"?`)) return;
    try { await api.finDeleteSubscription(s.id); await loadAll(); } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── New / edit income stream modal ─────────────────────────────────
  // One modal handles both create and edit — the UX is the same form
  // either way. editingId tracks "we're editing this one" vs "we're
  // making a fresh one"; the submit branches accordingly.
  let incomeOpen = $state(false);
  let editingIncomeId = $state<string | null>(null);
  let incomeForm = $state({
    name: '',
    status: 'idea' as FinIncomeStream['status'],
    kind: 'business' as FinIncomeStream['kind'],
    projected: '',
    actual: '',
    currency: 'USD',
    payout_day: '',
    payout_cadence: 'monthly' as FinIncomeStream['payout_cadence'],
    account_id: '',
    project: '',
    tags: '',
    url: '',
    notes: ''
  });
  function openIncome(s?: FinIncomeStream) {
    if (s) {
      editingIncomeId = s.id;
      incomeForm = {
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
    } else {
      editingIncomeId = null;
      incomeForm = {
        name: '', status: 'idea', kind: 'business',
        projected: '', actual: '',
        currency: accounts[0]?.currency || 'USD',
        payout_day: '',
        payout_cadence: 'monthly',
        account_id: accounts[0]?.id ?? '',
        project: '',
        tags: '',
        url: '', notes: ''
      };
    }
    incomeOpen = true;
  }
  async function submitIncome() {
    try {
      // Empty payout day means "unknown" — send as 0 so the server
      // stores nothing. Days outside 1-31 get clamped at the form
      // level so the schema stays sane regardless of weird input.
      const day = incomeForm.payout_day ? Math.max(0, Math.min(31, parseInt(incomeForm.payout_day, 10))) : 0;
      const body: Partial<FinIncomeStream> = {
        name: incomeForm.name.trim(),
        status: incomeForm.status,
        kind: incomeForm.kind,
        projected_monthly_cents: Math.round(parseFloat(incomeForm.projected || '0') * 100),
        actual_monthly_cents: Math.round(parseFloat(incomeForm.actual || '0') * 100),
        currency: incomeForm.currency.trim() || 'USD',
        payout_day_of_month: day,
        payout_cadence: incomeForm.payout_cadence || 'monthly',
        account_id: incomeForm.account_id || undefined,
        project: incomeForm.project.trim() || undefined,
        tags: incomeForm.tags.split(',').map((t) => t.trim()).filter(Boolean),
        url: incomeForm.url.trim() || undefined,
        notes: incomeForm.notes.trim() || undefined,
        // When the user flips a stream to active, stamp started_at if
        // they haven't already — saves them re-typing the date.
        started_at: incomeForm.status === 'active'
          ? (streams.find((x) => x.id === editingIncomeId)?.started_at
             || new Date().toISOString().slice(0, 10))
          : undefined
      };
      if (editingIncomeId) {
        await api.finPatchIncome(editingIncomeId, body);
        toast.success('updated');
      } else {
        await api.finCreateIncome(body);
        toast.success('income stream added');
      }
      incomeOpen = false;
      editingIncomeId = null;
      await loadAll();
    } catch (e) { toast.error('failed: ' + (e instanceof Error ? e.message : String(e))); }
  }
  async function deleteIncome(s: FinIncomeStream) {
    if (!confirm(`Delete "${s.name}"?`)) return;
    try { await api.finDeleteIncome(s.id); await loadAll(); } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── New goal modal ────────────────────────────────────────────────
  let goalOpen = $state(false);
  let goalForm = $state({
    name: '', kind: 'savings' as FinGoal['kind'],
    target: '', current: '0', currency: 'USD',
    target_date: '', linked_account_id: ''
  });
  function openGoal() {
    goalForm = {
      name: '', kind: 'savings',
      target: '', current: '0',
      currency: accounts[0]?.currency || 'USD',
      target_date: '', linked_account_id: ''
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
    } catch (e) { toast.error('failed: ' + (e instanceof Error ? e.message : String(e))); }
  }
  async function deleteGoal(g: FinGoal) {
    if (!confirm(`Delete goal "${g.name}"?`)) return;
    try { await api.finDeleteGoal(g.id); await loadAll(); } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // Group income streams for the Income tab. Active flow at top,
  // pipeline (idea + planned) below, paused at the bottom — the
  // server already returns them sorted, this just produces the
  // section labels.
  let activeStreams = $derived(streams.filter((s) => s.status === 'active'));
  let pipelineStreams = $derived(streams.filter((s) => s.status === 'idea' || s.status === 'planned'));
  let pausedStreams = $derived(streams.filter((s) => s.status === 'paused'));

  // ── 30-day cashflow timeline ──────────────────────────────────────
  // Concrete dated events in the next 30 days: subscription renewals,
  // income payouts (when the stream has payout_day_of_month set),
  // and financial-goal target dates. Income streams without an
  // explicit payout day are summarised as a footer line — we don't
  // make up dates for them.
  type CashflowEvent = {
    date: string;
    label: string;
    detail?: string;
    cents: number; // signed; >0 income, <0 outflow
    kind: 'subscription' | 'goal' | 'income';
  };
  const HORIZON_DAYS = 30;

  // Mirror of finance.IncomeStream.NextPayoutInWindow — kept in sync
  // with the Go side so the timeline matches what the server would
  // compute. Returns null when the stream has no schedule.
  function nextPayoutInWindow(s: FinIncomeStream, from: Date, to: Date): Date | null {
    const day = s.payout_day_of_month;
    if (!day || day < 1 || day > 31) return null;
    const cad = s.payout_cadence || 'monthly';
    const lastDay = (year: number, month: number) => new Date(year, month + 1, 0).getDate();
    const clamp = (year: number, month: number, requested: number) => Math.min(requested, lastDay(year, month));

    if (cad === 'yearly') {
      // Anchor on started_at month if available.
      let anchorMonth = from.getMonth();
      if (s.started_at) {
        const t = new Date(s.started_at + 'T00:00:00');
        if (!Number.isNaN(t.getTime())) anchorMonth = t.getMonth();
      }
      for (let year = from.getFullYear(); year <= to.getFullYear() + 1; year++) {
        const candidate = new Date(year, anchorMonth, clamp(year, anchorMonth, day));
        if (candidate >= from && candidate <= to) return candidate;
      }
      return null;
    }
    // Monthly (and weekly/quarterly fallbacks).
    let year = from.getFullYear();
    let month = from.getMonth();
    for (let i = 0; i < 32; i++) {
      const candidate = new Date(year, month, clamp(year, month, day));
      if (candidate >= from && candidate <= to) return candidate;
      if (candidate > to) return null;
      month++;
      if (month > 11) { month = 0; year++; }
    }
    return null;
  }
  function isoDate(d: Date): string {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  let cashflowEvents = $derived.by<CashflowEvent[]>(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const horizon = new Date(today);
    horizon.setDate(horizon.getDate() + HORIZON_DAYS);
    const todayISO = isoDate(today);
    const horizonISO = isoDate(horizon);
    const out: CashflowEvent[] = [];

    for (const s of subs) {
      if (!s.active) continue;
      if (s.next_renewal < todayISO || s.next_renewal > horizonISO) continue;
      out.push({
        date: s.next_renewal,
        label: s.name,
        detail: `${s.cadence} renewal`,
        cents: s.amount_cents,
        kind: 'subscription'
      });
    }
    for (const stream of streams) {
      if (stream.status !== 'active') continue;
      if (stream.actual_monthly_cents <= 0) continue;
      const payout = nextPayoutInWindow(stream, today, horizon);
      if (!payout) continue;
      // Yearly cadence: the payout amount is the actual ANNUAL value
      // (yearly bonus, dividends), not monthly. Approximate as 12×
      // the monthly the user told us; users with a real bonus
      // structure should record it differently. Other cadences use
      // the monthly directly.
      const cents = stream.payout_cadence === 'yearly'
        ? stream.actual_monthly_cents * 12
        : stream.actual_monthly_cents;
      out.push({
        date: isoDate(payout),
        label: stream.name,
        detail: stream.payout_cadence === 'yearly' ? 'yearly payout' : 'monthly payout',
        cents,
        kind: 'income'
      });
    }
    for (const g of goals) {
      if (!g.target_date) continue;
      if ((g.status ?? 'active') !== 'active') continue;
      if (g.target_date < todayISO || g.target_date > horizonISO) continue;
      out.push({
        date: g.target_date,
        label: `${g.name} (target)`,
        detail: `${g.kind} goal due`,
        cents: 0,
        kind: 'goal'
      });
    }
    out.sort((a, b) => a.date.localeCompare(b.date) || a.label.localeCompare(b.label));
    return out;
  });

  // Window totals split by sign so the header line can show in / out
  // separately from the running net. Income side counts dated payouts
  // first, falls back to the aggregate "approx monthly" footer for
  // streams without a payout day.
  let cashflowIncomeIn = $derived(cashflowEvents.reduce((s, e) => s + (e.cents > 0 ? e.cents : 0), 0));
  let cashflowSubOut = $derived(cashflowEvents.reduce((s, e) => s + (e.cents < 0 ? -e.cents : 0), 0));
  // Streams with no payout day — we'll show them as "+approx X / mo"
  // in the footer because we don't know the date.
  let undatedIncomeMonthly = $derived(
    streams
      .filter((s) => s.status === 'active' && s.actual_monthly_cents > 0 && (!s.payout_day_of_month || s.payout_day_of_month < 1))
      .reduce((sum, s) => sum + s.actual_monthly_cents, 0)
  );
  let cashflowNet = $derived(cashflowIncomeIn + undatedIncomeMonthly - cashflowSubOut);

  // Day-of-month from a YYYY-MM-DD; used for the timeline pip layout.
  function dayOf(iso: string): number {
    const m = iso.match(/-(\d{2})$/);
    return m ? parseInt(m[1], 10) : 0;
  }
  function dayLabel(iso: string): string {
    const d = new Date(iso + 'T00:00:00');
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-5xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Finance" subtitle="Net worth, subscriptions, income streams, money goals" />

    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-6 flex-wrap">
      {#each [
        { id: 'overview' as Tab, label: 'Overview' },
        { id: 'income' as Tab, label: 'Income', count: streams.length },
        { id: 'subscriptions' as Tab, label: 'Subscriptions', count: subs.length },
        { id: 'accounts' as Tab, label: 'Accounts', count: accounts.length },
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
        <!-- Headline numbers: how much money I have, what's coming
             in, what's leaking out. Three cards instead of four so
             nothing competes with the headline net-worth figure. -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3 mb-6">
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">How much I have</p>
            <p class="text-2xl font-semibold mt-1 {overview.net_worth_cents >= 0 ? 'text-text' : 'text-error'}">
              {fmtMoney(overview.net_worth_cents, overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              {fmtMoney(overview.assets_cents, overview.currency)} assets
              {#if overview.liabilities_cents > 0}
                · −{fmtMoney(overview.liabilities_cents, overview.currency)} debt
              {/if}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">Income / month</p>
            <p class="text-2xl font-semibold mt-1 text-success">
              {fmtMoney(overview.income_monthly_actual_cents, overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              from {overview.income_active_count} active source{overview.income_active_count === 1 ? '' : 's'}
              {#if overview.income_pipeline_count > 0}
                · {overview.income_pipeline_count} in pipeline
              {/if}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-4">
            <p class="text-xs uppercase tracking-wider text-dim">Subscriptions / month</p>
            <p class="text-2xl font-semibold mt-1 text-text">
              {fmtMoney(overview.subscription_monthly_cents, overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              {#if overview.upcoming_subs_count > 0}
                <span class="text-warning">{overview.upcoming_subs_count} due in 7 days</span>
              {:else}
                nothing renewing this week
              {/if}
            </p>
          </div>
        </div>

        <!-- Net flow line: what's the user keeping each month? Plain
             arithmetic so the user can sanity-check it against their
             own spreadsheet without trusting a black-box derivation. -->
        {#if overview.income_monthly_actual_cents > 0 || overview.subscription_monthly_cents > 0}
          {@const net = overview.income_monthly_actual_cents - overview.subscription_monthly_cents}
          <div class="mb-6 px-4 py-3 bg-surface0/40 border border-surface1 rounded text-sm">
            <span class="text-dim">Monthly run rate: </span>
            <span class="text-success">+{fmtMoney(overview.income_monthly_actual_cents, overview.currency)}</span>
            <span class="text-dim"> − </span>
            <span class="text-error">{fmtMoney(overview.subscription_monthly_cents, overview.currency)}</span>
            <span class="text-dim"> = </span>
            <span class="font-semibold {net >= 0 ? 'text-text' : 'text-error'}">{fmtMoney(net, overview.currency)} / month</span>
            <p class="text-[11px] text-dim mt-1">From recurring income & subscriptions only — doesn't include one-off spending.</p>
          </div>
        {/if}

        <!-- 30-day cashflow timeline. Compact horizontal pip layout
             at the top so the user sees the shape of upcoming
             outflows at a glance, then a chronological list with
             running net for detail. Hidden when nothing's coming up
             so empty vaults don't show a dead band. -->
        {#if cashflowEvents.length > 0 || undatedIncomeMonthly > 0}
          <section class="mb-6 bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-baseline gap-3 flex-wrap mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Next 30 days</h3>
              <span class="text-xs text-dim">
                <span class="text-success">+{fmtMoney(cashflowIncomeIn + undatedIncomeMonthly, overview.currency)}</span>
                <span class="mx-1">−</span>
                <span class="text-error">{fmtMoney(cashflowSubOut, overview.currency)}</span>
                <span class="mx-1">=</span>
                <span class="font-semibold {cashflowNet >= 0 ? 'text-text' : 'text-error'}">{fmtMoney(cashflowNet, overview.currency)}</span>
              </span>
            </div>

            <!-- Pip strip: each event in the window shows as a small
                 marker positioned by day-of-month. Income (green),
                 subscription (red), goal target (blue). Hover for
                 the full label + amount. Pure CSS — no chart lib. -->
            {#if cashflowEvents.length > 0}
              <div class="relative h-6 bg-mantle rounded mb-3">
                <div class="absolute inset-y-0 left-0 right-0 flex items-center px-1">
                  {#each Array.from({ length: 30 }, (_, i) => i) as i}
                    <div class="flex-1 border-r last:border-r-0 border-surface1/50 h-2 self-center"></div>
                  {/each}
                </div>
                {#each cashflowEvents as e (e.date + e.label)}
                  {@const today = new Date()}
                  {@const eventDate = new Date(e.date + 'T00:00:00')}
                  {@const daysFromToday = Math.round((eventDate.getTime() - new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()) / 86400000)}
                  {@const pct = Math.max(0, Math.min(100, (daysFromToday / 30) * 100))}
                  {@const tone = e.kind === 'income' ? 'bg-success' : e.kind === 'subscription' ? 'bg-error' : 'bg-info'}
                  <div
                    class="absolute top-0 bottom-0 w-1 -translate-x-1/2 rounded-full {tone}"
                    style="left: {pct}%"
                    title="{dayLabel(e.date)} — {e.label}{e.cents ? ` (${e.cents > 0 ? '+' : '−'}${fmtMoney(Math.abs(e.cents), overview.currency)})` : ''}"
                  ></div>
                {/each}
              </div>
            {/if}

            <ul class="text-sm divide-y divide-surface1/50">
              {#each cashflowEvents as e (e.date + e.label)}
                <li class="py-1.5 flex items-baseline gap-3">
                  <span class="text-xs text-dim font-mono w-16 flex-shrink-0">{dayLabel(e.date)}</span>
                  <span class="text-text flex-1 min-w-0 truncate">{e.label}</span>
                  <span class="text-[11px] text-dim hidden sm:inline">{e.detail}</span>
                  {#if e.cents !== 0}
                    <span class="font-mono {e.cents >= 0 ? 'text-success' : 'text-error'}">
                      {e.cents >= 0 ? '+' : '−'}{fmtMoney(Math.abs(e.cents), overview.currency)}
                    </span>
                  {:else}
                    <span class="text-[11px] text-info">—</span>
                  {/if}
                </li>
              {/each}
            </ul>

            {#if undatedIncomeMonthly > 0}
              <p class="text-[11px] text-dim italic mt-3">
                Plus undated income: <span class="text-success">+{fmtMoney(undatedIncomeMonthly, overview.currency)}</span>/month
                — set a payout day on the active stream to project it onto the timeline above.
              </p>
            {/if}
          </section>
        {/if}

        <div class="flex flex-wrap gap-2">
          <button onclick={() => openIncome()} class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90">+ Income source</button>
          <button onclick={openSub} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Subscription</button>
          <button onclick={openAcc} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Account</button>
          <button onclick={openGoal} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Goal</button>
        </div>

        {#if accounts.length === 0 && streams.length === 0 && subs.length === 0}
          <div class="mt-8 bg-surface0 border border-surface1 rounded-lg p-6 text-center">
            <p class="text-sm text-text">Welcome to your money tracker.</p>
            <p class="text-xs text-dim mt-1">Start by adding an account so you can see your net worth, then track income and subscriptions against it.</p>
          </div>
        {/if}
      {/if}

    {:else if tab === 'income'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">
          {streams.length} stream{streams.length === 1 ? '' : 's'} · active: {fmtMoney(overview?.income_monthly_actual_cents ?? 0, overview?.currency ?? '')} / mo · projected (incl. pipeline): {fmtMoney(overview?.income_monthly_projected_cents ?? 0, overview?.currency ?? '')} / mo
        </p>
        <button onclick={() => openIncome()} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New income source</button>
      </div>
      {#if streams.length === 0}
        <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
          <p class="text-sm text-text">Track every way money comes (or could come) in.</p>
          <p class="text-xs text-dim mt-1">A day job, a SaaS, dividends, a side hustle still in the idea stage — all live here together.</p>
        </div>
      {:else}
        {#if activeStreams.length > 0}
          <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Active</h3>
          <ul class="space-y-2 mb-5">
            {#each activeStreams as s (s.id)}
              {@const tone = statusTone(s.status)}
              {@const variance = s.actual_monthly_cents - s.projected_monthly_cents}
              <li class="bg-surface0 border border-surface1 rounded-lg p-3">
                <div class="flex items-baseline gap-3 flex-wrap">
                  <button onclick={() => openIncome(s)} class="font-medium text-text hover:underline">{s.name}</button>
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded {tone.bg} {tone.text}">{tone.label}</span>
                  <span class="text-[11px] text-dim">{s.kind}</span>
                  <span class="flex-1"></span>
                  <span class="text-sm font-mono text-success">{fmtMoney(s.actual_monthly_cents, s.currency)} / mo</span>
                  <button onclick={() => deleteIncome(s)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
                </div>
                <p class="text-[11px] text-dim mt-1">
                  projected: {fmtMoney(s.projected_monthly_cents, s.currency)}
                  {#if s.projected_monthly_cents > 0}
                    · variance: <span class="{variance >= 0 ? 'text-success' : 'text-warning'}">{variance >= 0 ? '+' : ''}{fmtMoney(variance, s.currency)}</span>
                  {/if}
                  {#if s.payout_day_of_month}· payout day {s.payout_day_of_month}{#if s.payout_cadence && s.payout_cadence !== 'monthly'} ({s.payout_cadence}){/if}{/if}
                  {#if s.account_id}· into {accountName(s.account_id)}{/if}
                  {#if s.started_at}· since {s.started_at}{/if}
                  {#if s.project}· <a href="/projects/{encodeURIComponent(s.project)}" class="text-secondary hover:underline">📁 {s.project}</a>{/if}
                  {#if s.url}· <a href={s.url} target="_blank" rel="noopener" class="text-secondary hover:underline">link ↗</a>{/if}
                </p>
                {#if s.tags && s.tags.length > 0}
                  <div class="flex flex-wrap gap-1 mt-1">
                    {#each s.tags as t}
                      <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                    {/each}
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
        {#if pipelineStreams.length > 0}
          <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Pipeline — ideas & planned ventures</h3>
          <ul class="space-y-2 mb-5">
            {#each pipelineStreams as s (s.id)}
              {@const tone = statusTone(s.status)}
              <li class="bg-surface0 border border-surface1 rounded-lg p-3">
                <div class="flex items-baseline gap-3 flex-wrap">
                  <button onclick={() => openIncome(s)} class="font-medium text-text hover:underline">{s.name}</button>
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded {tone.bg} {tone.text}">{tone.label}</span>
                  <span class="text-[11px] text-dim">{s.kind}</span>
                  <span class="flex-1"></span>
                  <span class="text-sm font-mono text-info">→ {fmtMoney(s.projected_monthly_cents, s.currency)} / mo</span>
                  <button onclick={() => deleteIncome(s)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
                </div>
                {#if s.project || (s.tags && s.tags.length > 0)}
                  <div class="flex flex-wrap items-center gap-1.5 mt-1">
                    {#if s.project}<a href="/projects/{encodeURIComponent(s.project)}" class="text-[11px] text-secondary hover:underline">📁 {s.project}</a>{/if}
                    {#each (s.tags ?? []) as t}
                      <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                    {/each}
                  </div>
                {/if}
                {#if s.notes}
                  <p class="text-[11px] text-subtext mt-1 whitespace-pre-line">{s.notes}</p>
                {/if}
                {#if s.url}
                  <p class="text-[11px] mt-1"><a href={s.url} target="_blank" rel="noopener" class="text-secondary hover:underline">link ↗</a></p>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
        {#if pausedStreams.length > 0}
          <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Paused</h3>
          <ul class="space-y-2 opacity-60">
            {#each pausedStreams as s (s.id)}
              <li class="bg-surface0 border border-surface1 rounded-lg p-3 flex items-baseline gap-3 flex-wrap">
                <button onclick={() => openIncome(s)} class="font-medium text-text hover:underline">{s.name}</button>
                <span class="text-[11px] text-dim">{s.kind} · last actual {fmtMoney(s.actual_monthly_cents, s.currency)}/mo</span>
                <span class="flex-1"></span>
                <button onclick={() => deleteIncome(s)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
              </li>
            {/each}
          </ul>
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
                <span class="flex-1"></span>
                <button onclick={() => toggleSubActive(s)} class="text-xs text-dim hover:text-text">{s.active ? 'pause' : 'resume'}</button>
                <button onclick={() => deleteSub(s)} class="text-xs text-dim hover:text-error">delete</button>
              </div>
              <p class="text-xs text-dim mt-1">
                next: <span class="text-subtext">{s.next_renewal}</span> · <span class="{relDate(s.next_renewal).includes('ago') ? 'text-error' : ''}">{relDate(s.next_renewal)}</span>
                {#if s.account_id}· billed to {accountName(s.account_id)}{/if}
                {#if s.project}· <a href="/projects/{encodeURIComponent(s.project)}" class="text-secondary hover:underline">📁 {s.project}</a>{/if}
                {#if s.category}· {s.category}{/if}
                {#if s.url}· <a href={s.url} target="_blank" rel="noopener" class="text-secondary hover:underline">manage ↗</a>{/if}
              </p>
              {#if s.tags && s.tags.length > 0}
                <div class="flex flex-wrap gap-1 mt-1">
                  {#each s.tags as t}
                    <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                  {/each}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

    {:else if tab === 'accounts'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{accounts.length} accounts · {fmtMoney(overview?.net_worth_cents ?? 0, overview?.currency ?? '')} net worth</p>
        <button onclick={openAcc} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New account</button>
      </div>
      {#if accounts.length === 0}
        <p class="text-sm text-dim italic">No accounts yet — add your first to start tracking.</p>
      {:else}
        <ul class="space-y-2">
          {#each accounts as a (a.id)}
            <li class="bg-surface0 border border-surface1 rounded-lg p-3 {a.archived ? 'opacity-50' : ''}" style="border-left: 3px solid {accColor(a.color)}">
              <div class="flex items-baseline gap-3 flex-wrap">
                <h3 class="font-medium text-text">{a.name}</h3>
                <span class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-subtext">{a.kind}</span>
                {#if a.institution}
                  <span class="text-[11px] text-dim">{a.institution}</span>
                {/if}
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
              </div>
              {#if (a.tags && a.tags.length > 0) || a.as_of}
                <div class="flex flex-wrap items-center gap-1.5 mt-1.5">
                  {#each (a.tags ?? []) as t}
                    <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                  {/each}
                  {#if a.as_of}<span class="text-[11px] text-dim">as of {a.as_of}</span>{/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
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

<!-- ── New / edit income modal ──────────────────────────────────────── -->
{#if incomeOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (incomeOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') incomeOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitIncome(); }} class="w-full max-w-md bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">{editingIncomeId ? 'Edit income source' : 'New income source'}</h2>
      <input bind:value={incomeForm.name} required placeholder="Name (Day job, Side SaaS, Dividends…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="grid grid-cols-2 gap-2">
        <label class="block">
          <span class="text-[11px] text-dim">Status</span>
          <select bind:value={incomeForm.status} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
            <option value="idea">Idea (could bring money)</option>
            <option value="planned">Planned (working on it)</option>
            <option value="active">Active (bringing money now)</option>
            <option value="paused">Paused</option>
          </select>
        </label>
        <label class="block">
          <span class="text-[11px] text-dim">Type</span>
          <select bind:value={incomeForm.kind} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
            <option value="employment">Employment / salary</option>
            <option value="freelance">Freelance / contract</option>
            <option value="business">Business / SaaS</option>
            <option value="investment">Investment / dividends</option>
            <option value="royalty">Royalty</option>
            <option value="other">Other</option>
          </select>
        </label>
      </div>
      <div class="grid grid-cols-3 gap-2 items-end">
        <label class="block col-span-1">
          <span class="text-[11px] text-dim">Projected / mo</span>
          <input type="number" step="0.01" bind:value={incomeForm.projected} placeholder="0.00" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        </label>
        <label class="block col-span-1">
          <span class="text-[11px] text-dim">Actual / mo</span>
          <input type="number" step="0.01" bind:value={incomeForm.actual} placeholder="0.00" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        </label>
        <input bind:value={incomeForm.currency} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </div>

      <!-- Payout schedule. Day-of-month + cadence drives the
           cashflow timeline projection. Empty day = unknown
           schedule; the stream still shows everywhere else but
           doesn't render on the date strip. -->
      <fieldset class="border border-surface1 rounded p-3 space-y-2">
        <legend class="text-[11px] text-dim px-1">Payout schedule</legend>
        <div class="grid grid-cols-2 gap-2">
          <label class="block">
            <span class="text-[11px] text-dim">Day of month (1-31)</span>
            <input type="number" min="0" max="31" bind:value={incomeForm.payout_day} placeholder="e.g. 5" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
          </label>
          <label class="block">
            <span class="text-[11px] text-dim">Cadence</span>
            <select bind:value={incomeForm.payout_cadence} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly (anchor month from started date)</option>
              <option value="quarterly">Quarterly (approx)</option>
              <option value="weekly">Weekly (approx)</option>
            </select>
          </label>
        </div>
        <p class="text-[11px] text-dim">
          Salary on the 5th? Set day=5, cadence=monthly. Leave day blank if you don't want it on the timeline.
        </p>
      </fieldset>

      <!-- Project + account links. Both optional — useful for
           ventures (link to the project that's the venture) and
           dividend streams (link to the investment account). -->
      <div class="grid grid-cols-2 gap-2">
        <label class="block">
          <span class="text-[11px] text-dim">Lands in account</span>
          <select bind:value={incomeForm.account_id} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
            <option value="">— none —</option>
            {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
          </select>
        </label>
        <label class="block">
          <span class="text-[11px] text-dim">Linked project</span>
          <select bind:value={incomeForm.project} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
            <option value="">— none —</option>
            {#each projects as p}<option value={p.name}>{p.name}</option>{/each}
          </select>
        </label>
      </div>
      <input bind:value={incomeForm.tags} placeholder="Tags (comma-separated, e.g. primary, w2)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <input bind:value={incomeForm.url} placeholder="URL (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <textarea bind:value={incomeForm.notes} rows="2" placeholder="Notes (idea details, next steps…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text resize-y focus:outline-none focus:border-primary"></textarea>
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (incomeOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">{editingIncomeId ? 'Save' : 'Add'}</button>
      </div>
    </form>
  </div>
{/if}

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
      <input bind:value={accForm.institution} placeholder="Institution (Chase, Apple Card…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <!-- Color palette swatches — visually pick the row pip rather
           than typing a name. Empty pip = "no color". -->
      <div class="flex items-center gap-2">
        <span class="text-[11px] text-dim">Color</span>
        <button type="button" onclick={() => (accForm.color = '')} class="w-5 h-5 rounded-full border border-surface2 {accForm.color === '' ? 'ring-2 ring-primary' : ''}" aria-label="no color"></button>
        {#each ['red','orange','yellow','green','blue','purple','cyan'] as c}
          <button type="button" onclick={() => (accForm.color = c)} class="w-5 h-5 rounded-full {accForm.color === c ? 'ring-2 ring-primary' : ''}" style="background: {accColor(c)}" aria-label={c}></button>
        {/each}
      </div>
      <input bind:value={accForm.tags} placeholder="Tags (comma-separated)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
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
      <div class="grid grid-cols-2 gap-2">
        {#if accounts.length > 0}
          <select bind:value={subForm.account_id} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
            <option value="">— no account —</option>
            {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
          </select>
        {/if}
        <select bind:value={subForm.project} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
          <option value="">— no project —</option>
          {#each projects as p}<option value={p.name}>{p.name}</option>{/each}
        </select>
      </div>
      <input bind:value={subForm.tags} placeholder="Tags (comma-separated)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <input bind:value={subForm.category} placeholder="Category (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <input bind:value={subForm.url} placeholder="Manage URL (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (subOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
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
