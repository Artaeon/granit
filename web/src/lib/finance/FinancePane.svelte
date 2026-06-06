<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import FinanceIncomeModal from '$lib/finance/FinanceIncomeModal.svelte';
  import FinanceAccountModal from '$lib/finance/FinanceAccountModal.svelte';
  import FinanceSubscriptionModal from '$lib/finance/FinanceSubscriptionModal.svelte';
  import FinanceGoalModal from '$lib/finance/FinanceGoalModal.svelte';
  import {
    SNAPSHOT_SYSTEM_PROMPT,
    SUB_AUDIT_SYSTEM_PROMPT,
    buildSnapshotPrompt,
    buildSubAuditPrompt
  } from '$lib/finance/aiPrompts';
  import {
    createFinanceViewState,
    type FinanceTab
  } from '$lib/finance/financeViewState.svelte';
  import { createFinanceData } from '$lib/finance/financeData.svelte';
  import { createFinanceAI } from '$lib/finance/financeAI.svelte';
  import { createFinanceAccountForm } from '$lib/finance/financeAccountForm.svelte';
  import { createFinanceSubscriptionForm } from '$lib/finance/financeSubscriptionForm.svelte';
  import { createFinanceIncomeForm } from '$lib/finance/financeIncomeForm.svelte';
  import { createFinanceGoalForm } from '$lib/finance/financeGoalForm.svelte';
  import {
    accColor,
    fmtMoney,
    relDate,
    statusTone,
    dayLabel
  } from '$lib/finance/financeFmt';

  // /finance covers the four things that actually matter for tracking
  // a financial life: how much money I have (Accounts → Net worth),
  // recurring drag (Subscriptions), income — both active sources and
  // pipeline ventures (Income), and money goals (Goals). Overview is
  // a single landing page that pulls the headline numbers from the
  // composite endpoint.

  const viewCtl = createFinanceViewState();
  const dataCtl = createFinanceData({
    isAuthed: () => !!$auth,
    onError: (m) => toast.error(m)
  });
  const accountForm = createFinanceAccountForm({
    getAccounts: () => dataCtl.accounts,
    reload: () => dataCtl.loadAll(),
    onSuccess: (m) => toast.success(m),
    onError: (m) => toast.error(m)
  });
  const subscriptionForm = createFinanceSubscriptionForm({
    getAccounts: () => dataCtl.accounts,
    reload: () => dataCtl.loadAll(),
    onSuccess: (m) => toast.success(m),
    onError: (m) => toast.error(m)
  });
  const incomeForm = createFinanceIncomeForm({
    getAccounts: () => dataCtl.accounts,
    getStreams: () => dataCtl.streams,
    reload: () => dataCtl.loadAll(),
    onSuccess: (m) => toast.success(m),
    onError: (m) => toast.error(m)
  });
  const goalForm = createFinanceGoalForm({
    getAccounts: () => dataCtl.accounts,
    reload: () => dataCtl.loadAll(),
    onError: (m) => toast.error(m)
  });
  const aiCtl = createFinanceAI({
    getOverview: () => dataCtl.overview,
    getSubs: () => dataCtl.subs,
    getStreams: () => dataCtl.streams,
    getGoals: () => dataCtl.goals,
    snapshotSystemPrompt: SNAPSHOT_SYSTEM_PROMPT,
    subAuditSystemPrompt: SUB_AUDIT_SYSTEM_PROMPT,
    buildSnapshotPrompt,
    buildSubAuditPrompt,
    chatStream: api.chatStream
  });

  // Everything user-facing now lives behind a controller: dataCtl
  // (loaders + derives + accountName lookup), accountForm /
  // subscriptionForm / incomeForm / goalForm (each modal's CRUD
  // wrappers), aiCtl (snapshot + sub-audit streams), viewCtl (tab),
  // and financeFmt (pure formatters). This file just wires them.

  onMount(() => {
    dataCtl.loadAll();
    return onWsEvent((ev) => {
      if (ev.type !== 'state.changed') return;
      if (!ev.path?.startsWith('.granit/finance/')) return;
      dataCtl.loadAll();
    });
  });
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-5xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="Finance" subtitle="Net worth, subscriptions, income streams, money goals" />

    <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm mb-4 flex-wrap">
      {#each [
        { id: 'overview' as FinanceTab, label: 'Overview' },
        { id: 'income' as FinanceTab, label: 'Income', count: dataCtl.streams.length },
        { id: 'subscriptions' as FinanceTab, label: 'Subscriptions', count: dataCtl.subs.length },
        { id: 'accounts' as FinanceTab, label: 'Accounts', count: dataCtl.accounts.length },
        { id: 'goals' as FinanceTab, label: 'Goals', count: dataCtl.goals.length }
      ] as t}
        <button
          class="px-3 sm:px-4 py-2 {viewCtl.tab === t.id ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => viewCtl.setTab(t.id)}
        >
          {t.label}{#if t.count !== undefined && t.count > 0}<span class="ml-1 text-xs opacity-70">{t.count}</span>{/if}
        </button>
      {/each}
    </div>

    {#if dataCtl.loading && !dataCtl.overview}
      <p class="text-sm text-dim">loading…</p>
    {:else if viewCtl.tab === 'overview'}
      {#if dataCtl.overview}
        <!-- Headline numbers: how much money I have, what's coming
             in, what's leaking out. Three cards instead of four so
             nothing competes with the headline net-worth figure. -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3 mb-4">
          <div class="bg-surface0 border border-surface1 rounded-lg p-3">
            <p class="text-xs uppercase tracking-wider text-dim">How much I have</p>
            <p class="text-2xl font-semibold mt-1 {dataCtl.overview.net_worth_cents >= 0 ? 'text-text' : 'text-error'}">
              {fmtMoney(dataCtl.overview.net_worth_cents, dataCtl.overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              {fmtMoney(dataCtl.overview.assets_cents, dataCtl.overview.currency)} assets
              {#if dataCtl.overview.liabilities_cents > 0}
                · −{fmtMoney(dataCtl.overview.liabilities_cents, dataCtl.overview.currency)} debt
              {/if}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-3">
            <p class="text-xs uppercase tracking-wider text-dim">Income / month</p>
            <p class="text-2xl font-semibold mt-1 text-success">
              {fmtMoney(dataCtl.overview.income_monthly_actual_cents, dataCtl.overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              from {dataCtl.overview.income_active_count} active source{dataCtl.overview.income_active_count === 1 ? '' : 's'}
              {#if dataCtl.overview.income_pipeline_count > 0}
                · {dataCtl.overview.income_pipeline_count} in pipeline
              {/if}
            </p>
          </div>
          <div class="bg-surface0 border border-surface1 rounded-lg p-3">
            <p class="text-xs uppercase tracking-wider text-dim">Subscriptions / month</p>
            <p class="text-2xl font-semibold mt-1 text-text">
              {fmtMoney(dataCtl.overview.subscription_monthly_cents, dataCtl.overview.currency)}
            </p>
            <p class="text-[11px] text-dim mt-1">
              {#if dataCtl.overview.upcoming_subs_count > 0}
                <span class="text-warning">{dataCtl.overview.upcoming_subs_count} due in 7 days</span>
              {:else}
                nothing renewing this week
              {/if}
            </p>
          </div>
        </div>

        <!-- Net flow line: what's the user keeping each month? Plain
             arithmetic so the user can sanity-check it against their
             own spreadsheet without trusting a black-box derivation.
             When recurring shopping standards exist (weekly groceries,
             monthly vitamins, ...) we fold their projection in too —
             the run-rate becomes "income − subscriptions − recurring
             groceries", a closer match to actual baseline outflow. -->
        {#if dataCtl.overview.income_monthly_actual_cents > 0 || dataCtl.overview.subscription_monthly_cents > 0}
          {@const recurringShoppingCents = dataCtl.shoppingTotals ? Math.round(dataCtl.shoppingTotals.recurring_monthly_estimate * 100) : 0}
          {@const net = dataCtl.overview.income_monthly_actual_cents - dataCtl.overview.subscription_monthly_cents - recurringShoppingCents}
          <div class="mb-4 px-4 py-3 bg-surface0 border border-surface1 rounded text-sm">
            <span class="text-dim">Monthly run rate: </span>
            <span class="text-success">+{fmtMoney(dataCtl.overview.income_monthly_actual_cents, dataCtl.overview.currency)}</span>
            <span class="text-dim"> − </span>
            <span class="text-error">{fmtMoney(dataCtl.overview.subscription_monthly_cents, dataCtl.overview.currency)}</span>
            {#if recurringShoppingCents > 0}
              <span class="text-dim"> − </span>
              <span class="text-error">{fmtMoney(recurringShoppingCents, dataCtl.overview.currency)}</span>
              <span class="text-[11px] text-dim">(shopping)</span>
            {/if}
            <span class="text-dim"> = </span>
            <span class="font-semibold {net >= 0 ? 'text-text' : 'text-error'}">{fmtMoney(net, dataCtl.overview.currency)} / month</span>
            <p class="text-[11px] text-dim mt-1">
              {#if recurringShoppingCents > 0}
                Recurring income + subscriptions + recurring shopping standards. One-off spending sits in the shopping rollup below.
              {:else}
                From recurring income & subscriptions only — doesn't include one-off spending.
              {/if}
            </p>
          </div>
        {/if}

        <!-- AI financial snapshot — 3-paragraph read of where the
             user stands. Calm, frank, names specific levers. Same
             rafThrottle pattern as the morning brief so a fast
             model can't choke the page. -->
        <div class="mb-4 px-3 py-3 bg-mantle border border-surface1 rounded">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-dim">AI snapshot</span>
            {#if aiCtl.snapshotBusy}
              <span class="text-[10px] text-secondary">streaming…</span>
            {/if}
            <span class="flex-1"></span>
            {#if aiCtl.snapshotBusy}
              <button
                type="button"
                onclick={aiCtl.cancelSnapshot}
                class="text-[11px] text-warning hover:text-error"
              >cancel</button>
            {:else if aiCtl.snapshotText.trim() || aiCtl.snapshotError}
              <button
                type="button"
                onclick={aiCtl.runSnapshot}
                class="text-[11px] text-secondary hover:underline"
                title="Re-run the snapshot"
              >↻ regenerate</button>
              <button
                type="button"
                onclick={aiCtl.dismissSnapshot}
                class="text-[11px] text-dim hover:text-error"
              >dismiss</button>
            {/if}
          </div>
          {#if aiCtl.snapshotError}
            <p class="text-sm text-error">{aiCtl.snapshotError}</p>
          {:else if aiCtl.snapshotText.trim()}
            <div class="text-sm text-text leading-relaxed space-y-2">
              {#each aiCtl.snapshotText.trim().split(/\n{2,}/) as para}
                {#if para.trim()}<p>{para.trim()}</p>{/if}
              {/each}
            </div>
          {:else if aiCtl.snapshotBusy}
            <p class="text-sm text-dim italic">Reading the numbers…</p>
          {:else}
            <p class="text-sm text-dim mb-2">A frank 70-130 word read of net worth, run rate, biggest lever, and one thing to watch next month.</p>
            <button
              type="button"
              onclick={aiCtl.runSnapshot}
              class="text-xs px-2 py-1 bg-surface0 hover:bg-surface1 text-secondary border border-secondary"
            >Generate snapshot</button>
          {/if}
        </div>

        <!-- Shopping rollup — bridges the user's plan-to-buy list into
             the money picture. Shows planned spend (the queued purchases
             not yet bought) and bought-this-month spend (one-off
             outflow that the run-rate above doesn't capture). Hidden
             when the shopping module is disabled or the user has no
             items yet. The shopping API stores prices in user-currency
             floats (EUR by default); fmtMoney here expects integer
             cents so we multiply by 100. -->
        {#if dataCtl.shoppingTotals && (dataCtl.shoppingTotals.planned_count > 0 || dataCtl.shoppingTotals.bought_month_count > 0)}
          <div class="mb-4 px-4 py-3 bg-surface0 border border-surface1 rounded">
            <div class="flex items-baseline justify-between gap-3 flex-wrap">
              <div class="flex items-baseline gap-4 flex-wrap">
                <span class="text-xs uppercase tracking-wider text-dim font-medium">Shopping</span>
                {#if dataCtl.shoppingTotals.planned_count > 0}
                  <span class="text-sm">
                    <span class="text-dim">planned</span>
                    <span class="text-text font-medium ml-1">{fmtMoney(Math.round(dataCtl.shoppingTotals.planned_sum * 100), dataCtl.overview.currency)}</span>
                    <span class="text-dim text-xs">· {dataCtl.shoppingTotals.planned_count} items</span>
                  </span>
                {/if}
                {#if dataCtl.shoppingTotals.bought_month_count > 0}
                  <span class="text-sm">
                    <span class="text-dim">bought this month</span>
                    <span class="text-text font-medium ml-1">{fmtMoney(Math.round(dataCtl.shoppingTotals.bought_month_sum * 100), dataCtl.overview.currency)}</span>
                    <span class="text-dim text-xs">· {dataCtl.shoppingTotals.bought_month_count} items</span>
                  </span>
                {/if}
              </div>
              <a href="/shopping" class="text-xs text-secondary hover:underline">open list →</a>
            </div>
            <p class="text-[11px] text-dim mt-1">
              From your <a href="/shopping" class="text-secondary hover:underline">shopping plan</a> — separate from subscriptions, so this captures one-off intent and actual outflows.
            </p>
          </div>
        {/if}

        <!-- 30-day cashflow timeline. Compact horizontal pip layout
             at the top so the user sees the shape of upcoming
             outflows at a glance, then a chronological list with
             running net for detail. Hidden when nothing's coming up
             so empty vaults don't show a dead band. -->
        {#if dataCtl.cashflowEvents.length > 0 || dataCtl.undatedIncomeMonthly > 0}
          <section class="mb-4 bg-surface0 border border-surface1 rounded-lg p-3">
            <div class="flex items-baseline gap-3 flex-wrap mb-3">
              <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Next 30 days</h3>
              <span class="text-xs text-dim">
                <span class="text-success">+{fmtMoney(dataCtl.cashflowIncomeIn + dataCtl.undatedIncomeMonthly, dataCtl.overview.currency)}</span>
                <span class="mx-1">−</span>
                <span class="text-error">{fmtMoney(dataCtl.cashflowSubOut, dataCtl.overview.currency)}</span>
                <span class="mx-1">=</span>
                <span class="font-semibold {dataCtl.cashflowNet >= 0 ? 'text-text' : 'text-error'}">{fmtMoney(dataCtl.cashflowNet, dataCtl.overview.currency)}</span>
              </span>
            </div>

            <!-- Pip strip: each event in the window shows as a small
                 marker positioned by day-of-month. Income (green),
                 subscription (red), goal target (blue). Hover for
                 the full label + amount. Pure CSS — no chart lib. -->
            {#if dataCtl.cashflowEvents.length > 0}
              <div class="relative h-6 bg-mantle rounded mb-3">
                <div class="absolute inset-y-0 left-0 right-0 flex items-center px-1">
                  {#each Array.from({ length: 30 }, (_, i) => i) as i}
                    <div class="flex-1 border-r last:border-r-0 border-surface1 h-2 self-center"></div>
                  {/each}
                </div>
                {#each dataCtl.cashflowEvents as e (e.date + e.label)}
                  {@const today = new Date()}
                  {@const eventDate = new Date(e.date + 'T00:00:00')}
                  {@const daysFromToday = Math.round((eventDate.getTime() - new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()) / 86400000)}
                  {@const pct = Math.max(0, Math.min(100, (daysFromToday / 30) * 100))}
                  {@const tone = e.kind === 'income' ? 'bg-success' : e.kind === 'subscription' ? 'bg-error' : 'bg-info'}
                  <div
                    class="absolute top-0 bottom-0 w-1 -translate-x-1/2 rounded-full {tone}"
                    style="left: {pct}%"
                    title="{dayLabel(e.date)} — {e.label}{e.cents ? ` (${e.cents > 0 ? '+' : '−'}${fmtMoney(Math.abs(e.cents), dataCtl.overview.currency)})` : ''}"
                  ></div>
                {/each}
              </div>
            {/if}

            <ul class="text-sm divide-y divide-surface1/50">
              {#each dataCtl.cashflowEvents as e (e.date + e.label)}
                <li class="py-1.5 flex items-baseline gap-3">
                  <span class="text-xs text-dim font-mono w-16 flex-shrink-0">{dayLabel(e.date)}</span>
                  <span class="text-text flex-1 min-w-0 truncate">{e.label}</span>
                  <span class="text-[11px] text-dim hidden sm:inline">{e.detail}</span>
                  {#if e.cents !== 0}
                    <span class="font-mono {e.cents >= 0 ? 'text-success' : 'text-error'}">
                      {e.cents >= 0 ? '+' : '−'}{fmtMoney(Math.abs(e.cents), dataCtl.overview.currency)}
                    </span>
                  {:else}
                    <span class="text-[11px] text-info">—</span>
                  {/if}
                </li>
              {/each}
            </ul>

            {#if dataCtl.undatedIncomeMonthly > 0}
              <p class="text-[11px] text-dim italic mt-3">
                Plus undated income: <span class="text-success">+{fmtMoney(dataCtl.undatedIncomeMonthly, dataCtl.overview.currency)}</span>/month
                — set a payout day on the active stream to project it onto the timeline above.
              </p>
            {/if}
          </section>
        {/if}

        <div class="flex flex-wrap gap-2">
          <button onclick={() => incomeForm.openModal()} class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90">+ Income source</button>
          <button onclick={() => subscriptionForm.openModal()} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Subscription</button>
          <button onclick={() => accountForm.openModal()} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Account</button>
          <button onclick={() => goalForm.openModal()} class="px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm hover:border-primary">+ Goal</button>
        </div>

        {#if dataCtl.accounts.length === 0 && dataCtl.streams.length === 0 && dataCtl.subs.length === 0}
          <div class="mt-8 bg-surface0 border border-surface1 rounded-lg p-6 text-center">
            <p class="text-sm text-text">Welcome to your money tracker.</p>
            <p class="text-xs text-dim mt-1">Start by adding an account so you can see your net worth, then track income and subscriptions against it.</p>
          </div>
        {/if}
      {/if}

    {:else if viewCtl.tab === 'income'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">
          {dataCtl.streams.length} stream{dataCtl.streams.length === 1 ? '' : 's'} · active: {fmtMoney(dataCtl.overview?.income_monthly_actual_cents ?? 0, dataCtl.overview?.currency ?? '')} / mo · projected (incl. pipeline): {fmtMoney(dataCtl.overview?.income_monthly_projected_cents ?? 0, dataCtl.overview?.currency ?? '')} / mo
        </p>
        <button onclick={() => incomeForm.openModal()} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New income source</button>
      </div>
      {#if dataCtl.streams.length === 0}
        <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
          <p class="text-sm text-text">Track every way money comes (or could come) in.</p>
          <p class="text-xs text-dim mt-1">A day job, a SaaS, dividends, a side hustle still in the idea stage — all live here together.</p>
        </div>
      {:else}
        {#if dataCtl.activeStreams.length > 0}
          <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Active</h3>
          <ul class="space-y-2 mb-5">
            {#each dataCtl.activeStreams as s (s.id)}
              {@const tone = statusTone(s.status)}
              {@const variance = s.actual_monthly_cents - s.projected_monthly_cents}
              <li class="bg-surface0 border border-surface1 rounded-lg p-3">
                <div class="flex items-baseline gap-3 flex-wrap">
                  <button onclick={() => incomeForm.openModal(s)} class="font-medium text-text hover:underline">{s.name}</button>
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded {tone.bg} {tone.text}">{tone.label}</span>
                  <span class="text-[11px] text-dim">{s.kind}</span>
                  <span class="flex-1"></span>
                  <span class="text-sm font-mono text-success">{fmtMoney(s.actual_monthly_cents, s.currency)} / mo</span>
                  <button onclick={() => incomeForm.remove(s)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
                </div>
                <p class="text-[11px] text-dim mt-1">
                  projected: {fmtMoney(s.projected_monthly_cents, s.currency)}
                  {#if s.projected_monthly_cents > 0}
                    · variance: <span class="{variance >= 0 ? 'text-success' : 'text-warning'}">{variance >= 0 ? '+' : ''}{fmtMoney(variance, s.currency)}</span>
                  {/if}
                  {#if s.payout_day_of_month}· payout day {s.payout_day_of_month}{#if s.payout_cadence && s.payout_cadence !== 'monthly'} ({s.payout_cadence}){/if}{/if}
                  {#if s.account_id}· into {dataCtl.accountName(s.account_id)}{/if}
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
        {#if dataCtl.pipelineStreams.length > 0}
          <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Pipeline — ideas & planned ventures</h3>
          <ul class="space-y-2 mb-5">
            {#each dataCtl.pipelineStreams as s (s.id)}
              {@const tone = statusTone(s.status)}
              <li class="bg-surface0 border border-surface1 rounded-lg p-3">
                <div class="flex items-baseline gap-3 flex-wrap">
                  <button onclick={() => incomeForm.openModal(s)} class="font-medium text-text hover:underline">{s.name}</button>
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded {tone.bg} {tone.text}">{tone.label}</span>
                  <span class="text-[11px] text-dim">{s.kind}</span>
                  <span class="flex-1"></span>
                  <span class="text-sm font-mono text-info">→ {fmtMoney(s.projected_monthly_cents, s.currency)} / mo</span>
                  <button onclick={() => incomeForm.remove(s)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
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
        {#if dataCtl.pausedStreams.length > 0}
          <h3 class="text-xs uppercase tracking-wider text-dim mt-2 mb-2">Paused</h3>
          <ul class="space-y-2 opacity-60">
            {#each dataCtl.pausedStreams as s (s.id)}
              <li class="bg-surface0 border border-surface1 rounded-lg p-3 flex items-baseline gap-3 flex-wrap">
                <button onclick={() => incomeForm.openModal(s)} class="font-medium text-text hover:underline">{s.name}</button>
                <span class="text-[11px] text-dim">{s.kind} · last actual {fmtMoney(s.actual_monthly_cents, s.currency)}/mo</span>
                <span class="flex-1"></span>
                <button onclick={() => incomeForm.remove(s)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}

    {:else if viewCtl.tab === 'subscriptions'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{dataCtl.subs.length} subscriptions · {fmtMoney(dataCtl.overview?.subscription_monthly_cents ?? 0, dataCtl.overview?.currency ?? '')}/mo</p>
        <div class="flex items-center gap-1">
          {#if dataCtl.subs.length >= 3}
            <button
              onclick={aiCtl.runSubAudit}
              disabled={aiCtl.auditBusy}
              class="text-xs px-2.5 py-1 bg-surface0 hover:bg-surface1 text-secondary border border-secondary disabled:opacity-50"
              title="AI audit: surfaces 3-6 candidates to cancel / downgrade / consolidate, ordered by annual saving"
            >{aiCtl.auditBusy ? 'auditing…' : 'AI audit'}</button>
          {/if}
          <button onclick={() => subscriptionForm.openModal()} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New subscription</button>
        </div>
      </div>
      {#if aiCtl.auditText.trim() || aiCtl.auditError || aiCtl.auditBusy}
        <div class="mb-3 px-3 py-3 bg-mantle border border-surface1 rounded">
          <div class="flex items-baseline gap-2 mb-2">
            <span class="text-[10px] uppercase tracking-wider text-dim">AI audit</span>
            {#if aiCtl.auditBusy}
              <span class="text-[10px] text-secondary">streaming…</span>
            {/if}
            <span class="flex-1"></span>
            {#if aiCtl.auditBusy}
              <button
                type="button"
                onclick={aiCtl.cancelSubAudit}
                class="text-[11px] text-warning hover:text-error"
              >cancel</button>
            {:else if aiCtl.auditText.trim() || aiCtl.auditError}
              <button
                type="button"
                onclick={aiCtl.runSubAudit}
                class="text-[11px] text-secondary hover:underline"
              >↻ regenerate</button>
              <button
                type="button"
                onclick={aiCtl.dismissSubAudit}
                class="text-[11px] text-dim hover:text-error"
              >dismiss</button>
            {/if}
          </div>
          {#if aiCtl.auditError}
            <p class="text-sm text-error">{aiCtl.auditError}</p>
          {:else if aiCtl.auditText.trim()}
            <pre class="text-xs text-text leading-relaxed whitespace-pre-wrap font-sans m-0">{aiCtl.auditText.trim()}</pre>
          {:else}
            <p class="text-sm text-dim italic">Auditing your subscriptions…</p>
          {/if}
        </div>
      {/if}
      {#if dataCtl.subs.length === 0}
        <p class="text-sm text-dim italic">No subscriptions yet — add your first to start tracking recurring outflows.</p>
      {:else}
        <ul class="space-y-2">
          {#each dataCtl.subs as s (s.id)}
            <li class="bg-surface0 border border-surface1 rounded-lg p-3 {s.active ? '' : 'opacity-60'}">
              <div class="flex items-baseline gap-3 flex-wrap">
                <h3 class="font-medium text-text">{s.name}</h3>
                <span class="text-sm text-error font-mono">{fmtMoney(s.amount_cents, s.currency)}</span>
                <span class="text-xs text-dim">/ {s.cadence}</span>
                <span class="flex-1"></span>
                <button onclick={() => subscriptionForm.toggleActive(s)} class="text-xs text-dim hover:text-text">{s.active ? 'pause' : 'resume'}</button>
                <button onclick={() => subscriptionForm.remove(s)} class="text-xs text-dim hover:text-error">delete</button>
              </div>
              <p class="text-xs text-dim mt-1">
                next: <span class="text-subtext">{s.next_renewal}</span> · <span class="{relDate(s.next_renewal).includes('ago') ? 'text-error' : ''}">{relDate(s.next_renewal)}</span>
                {#if s.account_id}· billed to {dataCtl.accountName(s.account_id)}{/if}
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

    {:else if viewCtl.tab === 'accounts'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{dataCtl.accounts.length} accounts · {fmtMoney(dataCtl.overview?.net_worth_cents ?? 0, dataCtl.overview?.currency ?? '')} net worth</p>
        <button onclick={() => accountForm.openModal()} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New account</button>
      </div>
      {#if dataCtl.accounts.length === 0}
        <p class="text-sm text-dim italic">No accounts yet — add your first to start tracking.</p>
      {:else}
        <ul class="space-y-2">
          {#each dataCtl.accounts as a (a.id)}
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
                    onblur={(e) => accountForm.saveBalance(a, e)}
                    class="w-28 bg-mantle border border-surface1 rounded px-1.5 py-0.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary"
                  />
                </label>
                <button onclick={() => accountForm.remove(a)} class="text-xs text-dim hover:text-error">delete</button>
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

    {:else if viewCtl.tab === 'goals'}
      <div class="flex justify-between items-center mb-3">
        <p class="text-xs text-dim">{dataCtl.goals.length} financial dataCtl.goals</p>
        <button onclick={() => goalForm.openModal()} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New goal</button>
      </div>
      {#if dataCtl.goals.length === 0}
        <p class="text-sm text-dim italic">No financial goals yet.</p>
      {:else}
        <ul class="space-y-3">
          {#each dataCtl.goals as g (g.id)}
            {@const pct = g.target_cents > 0 ? Math.min(100, Math.round((g.current_cents / g.target_cents) * 100)) : 0}
            <li class="bg-surface0 border border-surface1 rounded-lg p-3">
              <div class="flex items-baseline gap-3 flex-wrap">
                <h3 class="font-medium text-text">{g.name}</h3>
                <span class="text-[11px] px-1.5 py-0.5 rounded bg-surface1 text-subtext">{g.kind}</span>
                <span class="text-sm font-mono text-text">{fmtMoney(g.current_cents, g.currency)} / {fmtMoney(g.target_cents, g.currency)}</span>
                <span class="text-xs text-dim">{pct}%</span>
                <span class="flex-1"></span>
                {#if g.target_date}<span class="text-xs text-dim">by {g.target_date}</span>{/if}
                <button onclick={() => goalForm.remove(g)} class="text-xs text-dim hover:text-error">delete</button>
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

<!-- Per-modal markup lives in dedicated components. Each receives
     its controller plus any extra slices it needs from dataCtl
     (account / project pickers). The page just owns the wire-up. -->
<FinanceIncomeModal {incomeForm} accounts={dataCtl.accounts} projects={dataCtl.projects} />
<FinanceAccountModal {accountForm} />
<FinanceSubscriptionModal {subscriptionForm} accounts={dataCtl.accounts} projects={dataCtl.projects} />
<FinanceGoalModal {goalForm} />
