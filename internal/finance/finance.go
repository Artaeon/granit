// Package finance is the canonical schema + IO for granit's financial
// domain: accounts, subscriptions, income streams, and money goals.
// State lives under <vault>/.granit/finance/*.json — one file per
// concept — so the TUI, web server, and any future agent share one
// source of truth on disk. A round-trip through any surface preserves
// every field.
//
// Deliberately scoped to what a single user actually wants to track
// week-to-week, not a full ledger. No per-transaction history, no
// portfolio (those would shift the design from "track my financial
// life" toward "be my accounting software"). The primary numbers —
// net worth, monthly subscription drag, monthly income, goal
// progress — fall out of the four concepts directly.
//
// Money convention: amounts are stored as integer cents (`int64
// AmountCents`) to dodge float drift on summation. Currency codes are
// ISO 4217 strings ("USD", "EUR", "CHF"); the empty string means
// "user's primary currency" (resolved at the UI layer — the schema
// stays simple).
package finance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// AccountKind classifies an account so the UI can group / colour /
// project balances correctly. Kept open-ended (string, not enum) so
// future kinds (e.g. "crypto-wallet") don't require a schema bump,
// but the canonical six are validated below for typos at write time.
type AccountKind string

const (
	AccountCash       AccountKind = "cash"
	AccountChecking   AccountKind = "checking"
	AccountSavings    AccountKind = "savings"
	AccountCredit     AccountKind = "credit"
	AccountInvestment AccountKind = "investment"
	AccountLoan       AccountKind = "loan"
)

// NormalizeAccountKind canonicalises user-supplied kind strings to one
// of the six known values, defaulting to "checking" for empty / unknown
// input. Centralised so a typo in a TUI patch doesn't escape into the
// web's UI as an unrecognised pill.
func NormalizeAccountKind(s string) string {
	switch AccountKind(s) {
	case AccountCash, AccountChecking, AccountSavings,
		AccountCredit, AccountInvestment, AccountLoan:
		return s
	default:
		return string(AccountChecking)
	}
}

// Account is a money container — one bank/credit/cash/investment per
// row. Balance is the user-confirmed snapshot; the AsOf field captures
// when the user last reconciled it.
//
// Optional fields beyond the basics:
//   - Institution: bank/issuer name ("Chase", "Apple Card") so the
//     UI can group multiple accounts at the same institution and
//     show it as a small pill on the row.
//   - Color: visual distinguisher — palette key (e.g. "blue",
//     "purple") that the UI maps to a CSS variable. Empty = default.
//   - Tags: user-owned taxonomy — same convention as tasks/people.
type Account struct {
	ID           string    `json:"id"`               // ULID, lowercase
	Name         string    `json:"name"`             // user-facing label
	Kind         string    `json:"kind"`             // see AccountKind
	Currency     string    `json:"currency"`         // ISO 4217
	BalanceCents int64     `json:"balance_cents"`    // signed
	AsOf         string    `json:"as_of,omitempty"`  // YYYY-MM-DD of the snapshot
	Institution  string    `json:"institution,omitempty"`
	Color        string    `json:"color,omitempty"`  // palette key
	Tags         []string  `json:"tags,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	Archived     bool      `json:"archived,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IncomeStreamStatus tracks where a stream is in its lifecycle.
// "idea" and "planned" are forward-looking — the user is exploring or
// preparing a venture that could bring money. "active" means it's
// actually paying out today; "paused" is income that ran but isn't
// running right now.
type IncomeStreamStatus string

const (
	IncomeIdea    IncomeStreamStatus = "idea"
	IncomePlanned IncomeStreamStatus = "planned"
	IncomeActive  IncomeStreamStatus = "active"
	IncomePaused  IncomeStreamStatus = "paused"
)

func NormalizeIncomeStatus(s string) string {
	switch IncomeStreamStatus(s) {
	case IncomeIdea, IncomePlanned, IncomeActive, IncomePaused:
		return s
	default:
		return string(IncomeIdea)
	}
}

// IncomeKind classifies how the income shows up. Open-string with a
// canonical set so a typo doesn't escape into the UI as a stray pill.
type IncomeKind string

const (
	IncomeKindEmployment IncomeKind = "employment" // salary, regular paycheck
	IncomeKindFreelance  IncomeKind = "freelance"  // contracts, gigs
	IncomeKindBusiness   IncomeKind = "business"   // SaaS, product sales
	IncomeKindInvestment IncomeKind = "investment" // dividends, rent, interest
	IncomeKindRoyalty    IncomeKind = "royalty"
	IncomeKindOther      IncomeKind = "other"
)

func NormalizeIncomeKind(s string) string {
	switch IncomeKind(s) {
	case IncomeKindEmployment, IncomeKindFreelance, IncomeKindBusiness,
		IncomeKindInvestment, IncomeKindRoyalty, IncomeKindOther:
		return s
	default:
		return string(IncomeKindOther)
	}
}

// IncomeStream is a way money comes (or could come) in. One concept
// covers both "active income" (your day job, your SaaS) and
// "ventures" (a side project still in idea / planning) because the
// difference is exactly Status — same shape, different stage. The
// UI surfaces the status as a colored pill and groups the list by
// active-vs-pipeline.
//
// Projected vs actual: Projected is the user's expectation
// ("when this is running, it should make $X/mo"). Actual is what's
// flowing right now. For an idea/planned stream Actual is 0; for an
// active one the user updates it as months close.
//
// PayoutDayOfMonth (1-31, 0 = unknown) + PayoutCadence are the
// concrete schedule — "salary lands on the 5th, monthly". The
// cashflow timeline projects each active stream's next payout in
// the 30-day window using these.
//
// AccountID + ProjectName + Tags hook the stream into the rest of
// granit: which account does it land in, which project does it
// belong to (a venture's project, the day job's "career" project,
// dividends from the "investment" project), and freeform tags.
type IncomeStream struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Status                string    `json:"status"`              // see IncomeStreamStatus
	Kind                  string    `json:"kind"`                // see IncomeKind
	ProjectedMonthlyCents int64     `json:"projected_monthly_cents"`
	ActualMonthlyCents    int64     `json:"actual_monthly_cents"`
	Currency              string    `json:"currency"`
	PayoutDayOfMonth      int       `json:"payout_day_of_month,omitempty"` // 1-31; 0 = unknown
	PayoutCadence         string    `json:"payout_cadence,omitempty"`      // see SubCadence; empty defaults to monthly
	AccountID             string    `json:"account_id,omitempty"`          // where the money lands
	ProjectName           string    `json:"project,omitempty"`             // matches Project.Name
	Tags                  []string  `json:"tags,omitempty"`
	URL                   string    `json:"url,omitempty"`                 // landing page / dashboard
	StartedAt             string    `json:"started_at,omitempty"`          // YYYY-MM-DD when it became active
	Notes                 string    `json:"notes,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// NextPayoutInWindow returns the next payout date that falls between
// `from` (inclusive) and `to` (inclusive), or zero-time + false if
// none fits or the schedule is unknown.
//
// Cadence behaviour:
//   - "monthly" or empty: PayoutDayOfMonth applied to each calendar
//     month. If the month has fewer days (Feb 30 → ?), we clamp to
//     the last day of the month.
//   - "yearly": the day-of-month + month from StartedAt; falls back
//     to today's month if StartedAt is unset.
//   - "weekly" / "quarterly": fall back to monthly approximation —
//     these are rare for income (paycheck is monthly; quarterly is
//     dividends with too much variation to project precisely
//     without more fields). The UI label flags them as "approx".
//
// Used by the cashflow timeline.
func (s IncomeStream) NextPayoutInWindow(from, to time.Time) (time.Time, bool) {
	if s.PayoutDayOfMonth < 1 || s.PayoutDayOfMonth > 31 {
		return time.Time{}, false
	}
	cad := s.PayoutCadence
	if cad == "" {
		cad = string(CadenceMonthly)
	}
	switch SubCadence(cad) {
	case CadenceYearly:
		// Anchor to the started_at month if available; else today's month.
		var month time.Month
		if t, err := time.Parse("2006-01-02", s.StartedAt); err == nil {
			month = t.Month()
		} else {
			month = from.Month()
		}
		for year := from.Year(); year <= to.Year()+1; year++ {
			day := clampDay(year, month, s.PayoutDayOfMonth)
			candidate := time.Date(year, month, day, 0, 0, 0, 0, from.Location())
			if !candidate.Before(from) && !candidate.After(to) {
				return candidate, true
			}
		}
		return time.Time{}, false
	default:
		// Monthly (and the weekly/quarterly fallbacks). Walk forward
		// month-by-month from `from` until we find one in the window
		// or pass the end. Loop bound: at most ~32 months would fit
		// in any reasonable window, so the linear scan is cheap.
		year, month := from.Year(), from.Month()
		for i := 0; i < 32; i++ {
			day := clampDay(year, month, s.PayoutDayOfMonth)
			candidate := time.Date(year, month, day, 0, 0, 0, 0, from.Location())
			if !candidate.Before(from) && !candidate.After(to) {
				return candidate, true
			}
			if candidate.After(to) {
				return time.Time{}, false
			}
			month++
			if month > 12 {
				month = 1
				year++
			}
		}
		return time.Time{}, false
	}
}

// clampDay returns the smallest of the requested day and the actual
// last day of (year, month). Lets callers say "the 31st" without
// special-casing February / 30-day months.
func clampDay(year int, month time.Month, requested int) int {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if requested > last {
		return last
	}
	return requested
}

// SubCadence is how often a subscription recurs. Same open-string +
// canonical-set pattern as AccountKind.
type SubCadence string

const (
	CadenceMonthly  SubCadence = "monthly"
	CadenceYearly   SubCadence = "yearly"
	CadenceWeekly   SubCadence = "weekly"
	CadenceQuarterly SubCadence = "quarterly"
)

func NormalizeCadence(s string) string {
	switch SubCadence(s) {
	case CadenceMonthly, CadenceYearly, CadenceWeekly, CadenceQuarterly:
		return s
	default:
		return string(CadenceMonthly)
	}
}

// Subscription is a recurring outflow the user wants to track. The
// next-renewal date is the user-edited snapshot; computed roll-forward
// (advance by cadence on each tick past today) lives in serveapi
// handlers so this package stays IO-only.
//
// ProjectName lets the user attribute a SaaS spend to a project ("VPS
// is for the side-SaaS venture"). It's a name, not an ID, to match
// the existing project schema in granitmeta which keys by Name.
type Subscription struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`             // "Netflix"
	AmountCents   int64     `json:"amount_cents"`     // signed: usually < 0 (outflow)
	Currency      string    `json:"currency"`
	Cadence       string    `json:"cadence"`          // see SubCadence
	NextRenewal   string    `json:"next_renewal"`     // YYYY-MM-DD
	AccountID     string    `json:"account_id,omitempty"` // billed against
	ProjectName   string    `json:"project,omitempty"`     // matches Project.Name
	Category      string    `json:"category,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	URL           string    `json:"url,omitempty"`        // cancellation / login link
	Notes         string    `json:"notes,omitempty"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// FinGoalKind keeps the goal types canonical. Savings = grow toward a
// target balance; Payoff = shrink debt to zero; NetWorth = aggregate
// every account.
type FinGoalKind string

const (
	GoalSavings  FinGoalKind = "savings"
	GoalPayoff   FinGoalKind = "payoff"
	GoalNetworth FinGoalKind = "networth"
)

func NormalizeGoalKind(s string) string {
	switch FinGoalKind(s) {
	case GoalSavings, GoalPayoff, GoalNetworth:
		return s
	default:
		return string(GoalSavings)
	}
}

// FinGoal is a money goal — distinct from the broader internal/goals
// package, which is for life goals with milestones. Keeping these
// separate so a "save $10k" doesn't end up jammed into the milestone
// schema where it doesn't fit.
type FinGoal struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Kind              string    `json:"kind"`              // see FinGoalKind
	TargetCents       int64     `json:"target_cents"`
	CurrentCents      int64     `json:"current_cents"`     // user-edited or derived
	Currency          string    `json:"currency"`
	TargetDate        string    `json:"target_date,omitempty"` // YYYY-MM-DD
	LinkedAccountID   string    `json:"linked_account_id,omitempty"`
	Notes             string    `json:"notes,omitempty"`
	Status            string    `json:"status,omitempty"`  // active | met | abandoned
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ── State paths ──────────────────────────────────────────────────────

// Each concept lives in its own file under .granit/finance/. Keeping
// them split (not one fat file) means the TUI / web don't lock-step
// rewrite four megabytes of transactions when the user edits one
// subscription, and a partial corruption on disk is contained.

func dir(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "finance")
}
func AccountsPath(v string) string      { return filepath.Join(dir(v), "accounts.json") }
func SubscriptionsPath(v string) string { return filepath.Join(dir(v), "subscriptions.json") }
func IncomePath(v string) string        { return filepath.Join(dir(v), "income.json") }
func FinGoalsPath(v string) string      { return filepath.Join(dir(v), "goals.json") }

// ── Generic load/save ────────────────────────────────────────────────
//
// All five files have the same shape: a JSON array of structs. A
// single generic pair below avoids five hand-written copies, each of
// which would have drifted independently.

func loadAll[T any](path string) []T {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func saveAll[T any](vaultRoot, path string, items []T) error {
	if vaultRoot == "" {
		return errors.New("finance: empty vault root")
	}
	if err := os.MkdirAll(dir(vaultRoot), 0o755); err != nil {
		return err
	}
	if items == nil {
		items = []T{}
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}

// ── Per-concept exposed wrappers ─────────────────────────────────────
//
// Wrappers (rather than naked generic calls) keep the public surface
// idiomatic and let the editor + LSP autocomplete the right return
// type per concept.

func LoadAccounts(v string) []Account                    { return loadAll[Account](AccountsPath(v)) }
func SaveAccounts(v string, x []Account) error           { return saveAll(v, AccountsPath(v), x) }
func LoadSubscriptions(v string) []Subscription          { return loadAll[Subscription](SubscriptionsPath(v)) }
func SaveSubscriptions(v string, x []Subscription) error { return saveAll(v, SubscriptionsPath(v), x) }
func LoadIncome(v string) []IncomeStream                 { return loadAll[IncomeStream](IncomePath(v)) }
func SaveIncome(v string, x []IncomeStream) error        { return saveAll(v, IncomePath(v), x) }
func LoadFinGoals(v string) []FinGoal                    { return loadAll[FinGoal](FinGoalsPath(v)) }
func SaveFinGoals(v string, x []FinGoal) error           { return saveAll(v, FinGoalsPath(v), x) }

// ── Sort helpers (stable, copy-returning) ────────────────────────────

// SortAccountsForDisplay: archived to the bottom; otherwise alpha by
// Name with ID as tiebreak so the order is stable across reloads.
func SortAccountsForDisplay(xs []Account) []Account {
	out := make([]Account, len(xs))
	copy(out, xs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Archived != out[j].Archived {
			return !out[i].Archived
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// SortIncomeForDisplay: active first (the income that's actually
// flowing), then planned/idea ranked by Status order, then paused at
// the bottom. Within a status bucket, highest projected first so the
// big numbers surface — useful when scanning to see "what's actually
// moving the needle this month."
func SortIncomeForDisplay(xs []IncomeStream) []IncomeStream {
	out := make([]IncomeStream, len(xs))
	copy(out, xs)
	rank := func(s string) int {
		switch IncomeStreamStatus(s) {
		case IncomeActive:
			return 0
		case IncomePlanned:
			return 1
		case IncomeIdea:
			return 2
		case IncomePaused:
			return 3
		default:
			return 4
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i].Status), rank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		// Use whichever number better represents the stream's "size":
		// active → actual; planned/idea → projected.
		size := func(s IncomeStream) int64 {
			if IncomeStreamStatus(s.Status) == IncomeActive && s.ActualMonthlyCents > 0 {
				return s.ActualMonthlyCents
			}
			return s.ProjectedMonthlyCents
		}
		if size(out[i]) != size(out[j]) {
			return size(out[i]) > size(out[j])
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// SortSubscriptionsByRenewal: active subs by next-renewal asc; inactive
// always at the bottom regardless of date.
func SortSubscriptionsByRenewal(xs []Subscription) []Subscription {
	out := make([]Subscription, len(xs))
	copy(out, xs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		if out[i].NextRenewal != out[j].NextRenewal {
			return out[i].NextRenewal < out[j].NextRenewal
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ── Cadence math ─────────────────────────────────────────────────────
//
// AdvanceRenewal returns the next renewal date in YYYY-MM-DD form,
// given the current renewal date and the cadence. Used by the
// "process subscriptions" pass that auto-rolls past-due renewals
// forward when the user (or the daemon) asks for it.

func AdvanceRenewal(currentISO string, cadence string) string {
	t, err := time.Parse("2006-01-02", currentISO)
	if err != nil {
		return currentISO
	}
	switch SubCadence(cadence) {
	case CadenceWeekly:
		t = t.AddDate(0, 0, 7)
	case CadenceMonthly:
		t = t.AddDate(0, 1, 0)
	case CadenceQuarterly:
		t = t.AddDate(0, 3, 0)
	case CadenceYearly:
		t = t.AddDate(1, 0, 0)
	default:
		t = t.AddDate(0, 1, 0)
	}
	return t.Format("2006-01-02")
}

// MonthlyCostCents normalises any subscription to its monthly cost so
// the UI can show a "you spend $N/month on subscriptions" total
// without per-cadence branching at every render. AmountCents is signed;
// callers usually want absolute value for displaying expense totals.
func (s Subscription) MonthlyCostCents() int64 {
	switch SubCadence(s.Cadence) {
	case CadenceWeekly:
		return s.AmountCents * 52 / 12
	case CadenceMonthly:
		return s.AmountCents
	case CadenceQuarterly:
		return s.AmountCents / 3
	case CadenceYearly:
		return s.AmountCents / 12
	default:
		return s.AmountCents
	}
}
