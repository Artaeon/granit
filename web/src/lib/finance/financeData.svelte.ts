// Data state for the finance surface.
//
// Second extraction step out of routes/finance/+page.svelte. Owns
// every loaded array (overview, accounts, subs, streams, goals,
// shoppingTotals, projects), the loading flag, the loadAll()
// function, and every derivation that operates over the data alone:
//
//   - the income-stream split (active / pipeline / paused) that the
//     income tab renders into three sections
//   - the 30-day cashflow events list (subscription renewals + dated
//     income payouts + active goal target dates) with its three
//     window aggregates (incomeIn, subOut, net) and the un-dated
//     income monthly fallback
//   - the accountName(id) lookup helper used by every list view
//
// The page still owns the onMount install ordering, the WS
// subscription, the AI surfaces, and every modal/form. Both call
// into dataCtl methods; the controller exposes loaded state via
// getter/setter pairs so the modal submit handlers can still
// loadAll() between writes.
//
// Pure helpers (nextPayoutInWindow + isoDate) live next to the
// derivation that consumes them — same pattern as goals'
// staleness/recentCompletionForGoal.

import {
  api,
  type FinAccount,
  type FinSubscription,
  type FinIncomeStream,
  type FinGoal,
  type FinOverview,
  type Project,
  type ShoppingTotals
} from '$lib/api';

export type CashflowEvent = {
  date: string;
  label: string;
  detail?: string;
  /** Signed; >0 income, <0 outflow. */
  cents: number;
  kind: 'subscription' | 'goal' | 'income';
};

const HORIZON_DAYS = 30;

export interface FinanceDataDeps {
  /** Boolean snapshot of the auth store — used as a guard before
   *  loadAll(). The page passes () => !!$auth so the read stays
   *  reactive in the calling context. */
  isAuthed: () => boolean;
  /** Toast hook for the catch branch in loadAll(). Injected so the
   *  controller doesn't have to import the toast singleton — keeps
   *  it pure-data, easier to unit-test. */
  onError: (message: string) => void;
}

export interface FinanceDataController {
  overview: FinOverview | null;
  accounts: FinAccount[];
  subs: FinSubscription[];
  streams: FinIncomeStream[];
  goals: FinGoal[];
  shoppingTotals: ShoppingTotals | null;
  projects: Project[];
  loading: boolean;

  /** Income streams split by status for the income tab's three
   *  sections. Server already returns them sorted; this just
   *  produces the section labels. */
  readonly activeStreams: FinIncomeStream[];
  readonly pipelineStreams: FinIncomeStream[];
  readonly pausedStreams: FinIncomeStream[];

  /** 30-day cashflow timeline: dated events in the next 30 days. */
  readonly cashflowEvents: CashflowEvent[];
  /** Sum of positive event amounts. */
  readonly cashflowIncomeIn: number;
  /** Sum of |negative| event amounts. */
  readonly cashflowSubOut: number;
  /** Approx monthly value of active streams with no payout day. */
  readonly undatedIncomeMonthly: number;
  /** incomeIn + undatedIncomeMonthly − subOut. */
  readonly cashflowNet: number;

  loadAll(): Promise<void>;
  /** Account name lookup by id. Falls back to '(unknown)' for
   *  known-missing ids and '—' for empty/undefined. */
  accountName(id: string | undefined): string;
}

// Mirror of finance.IncomeStream.NextPayoutInWindow on the Go side —
// kept in sync so the timeline matches what the server would
// compute. Returns null when the stream has no schedule.
function nextPayoutInWindow(s: FinIncomeStream, from: Date, to: Date): Date | null {
  const day = s.payout_day_of_month;
  if (!day || day < 1 || day > 31) return null;
  const cad = s.payout_cadence || 'monthly';
  const lastDay = (year: number, month: number) => new Date(year, month + 1, 0).getDate();
  const clamp = (year: number, month: number, requested: number) =>
    Math.min(requested, lastDay(year, month));

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

export function createFinanceData(deps: FinanceDataDeps): FinanceDataController {
  let overview = $state<FinOverview | null>(null);
  let accounts = $state<FinAccount[]>([]);
  let subs = $state<FinSubscription[]>([]);
  let streams = $state<FinIncomeStream[]>([]);
  let goals = $state<FinGoal[]>([]);
  let shoppingTotals = $state<ShoppingTotals | null>(null);
  let projects = $state<Project[]>([]);
  let loading = $state(false);

  async function loadAll() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      const [o, a, s, i, g, p, sh] = await Promise.all([
        api.finOverview(),
        api.finListAccounts(),
        api.finListSubscriptions(),
        api.finListIncome(),
        api.finListGoals(),
        // Projects are read-only here — fetched only to populate
        // pickers on income + subscription create/edit. A failure
        // shouldn't break the finance page; fall through with empty.
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 })),
        // Shopping totals — same defensive pattern. Module disabled
        // → endpoint may return 404 → null, which the render branch
        // auto-hides.
        api.shoppingTotals().catch(() => null)
      ]);
      overview = o;
      accounts = a.accounts;
      subs = s.subscriptions;
      streams = i.streams;
      goals = g.goals;
      projects = p.projects;
      shoppingTotals = sh;
    } catch (e) {
      deps.onError('failed to load finance: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  function accountName(id: string | undefined): string {
    if (!id) return '—';
    return accounts.find((a) => a.id === id)?.name ?? '(unknown)';
  }

  // Group income streams for the Income tab. Active flow at top,
  // pipeline (idea + planned) below, paused at the bottom — the
  // server already returns them sorted, this just produces the
  // section labels.
  let activeStreams = $derived(streams.filter((s) => s.status === 'active'));
  let pipelineStreams = $derived(
    streams.filter((s) => s.status === 'idea' || s.status === 'planned')
  );
  let pausedStreams = $derived(streams.filter((s) => s.status === 'paused'));

  // 30-day cashflow events: subscription renewals, income payouts
  // (when the stream has payout_day_of_month set), and financial-goal
  // target dates. Income streams without an explicit payout day are
  // surfaced via undatedIncomeMonthly — we don't make up dates.
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
  let cashflowIncomeIn = $derived(
    cashflowEvents.reduce((s, e) => s + (e.cents > 0 ? e.cents : 0), 0)
  );
  let cashflowSubOut = $derived(
    cashflowEvents.reduce((s, e) => s + (e.cents < 0 ? -e.cents : 0), 0)
  );
  // Streams with no payout day — surfaced as "+approx X / mo" in the
  // footer because we don't know the date.
  let undatedIncomeMonthly = $derived(
    streams
      .filter((s) => s.status === 'active' && s.actual_monthly_cents > 0
        && (!s.payout_day_of_month || s.payout_day_of_month < 1))
      .reduce((sum, s) => sum + s.actual_monthly_cents, 0)
  );
  let cashflowNet = $derived(cashflowIncomeIn + undatedIncomeMonthly - cashflowSubOut);

  return {
    get overview() { return overview; },
    set overview(v) { overview = v; },
    get accounts() { return accounts; },
    set accounts(v) { accounts = v; },
    get subs() { return subs; },
    set subs(v) { subs = v; },
    get streams() { return streams; },
    set streams(v) { streams = v; },
    get goals() { return goals; },
    set goals(v) { goals = v; },
    get shoppingTotals() { return shoppingTotals; },
    set shoppingTotals(v) { shoppingTotals = v; },
    get projects() { return projects; },
    set projects(v) { projects = v; },
    get loading() { return loading; },
    set loading(v) { loading = v; },

    get activeStreams() { return activeStreams; },
    get pipelineStreams() { return pipelineStreams; },
    get pausedStreams() { return pausedStreams; },

    get cashflowEvents() { return cashflowEvents; },
    get cashflowIncomeIn() { return cashflowIncomeIn; },
    get cashflowSubOut() { return cashflowSubOut; },
    get undatedIncomeMonthly() { return undatedIncomeMonthly; },
    get cashflowNet() { return cashflowNet; },

    loadAll,
    accountName
  };
}
