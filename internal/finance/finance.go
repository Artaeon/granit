// Package finance is the canonical schema + IO for granit's financial
// domain: accounts, transactions, subscriptions, holdings, and money
// goals. State lives under <vault>/.granit/finance/*.json — one file
// per concept — so the TUI, web server, and any future agent share
// one source of truth on disk. A round-trip through any surface
// preserves every field.
//
// Pure data + IO only. No HTTP, no rendering, no balance projections
// (those live in serveapi handlers / web derivations where they can
// reuse the cached load-all on the request path).
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
// row. Balance is the user-confirmed snapshot; live balance equals
// Balance + sum(Transactions since AsOf), but we keep that derivation
// out of this package (it belongs in handlers).
type Account struct {
	ID          string    `json:"id"`               // ULID, lowercase
	Name        string    `json:"name"`             // user-facing label
	Kind        string    `json:"kind"`             // see AccountKind
	Currency    string    `json:"currency"`         // ISO 4217
	BalanceCents int64    `json:"balance_cents"`    // signed
	AsOf        string    `json:"as_of,omitempty"`  // YYYY-MM-DD of the snapshot
	Notes       string    `json:"notes,omitempty"`
	Archived    bool      `json:"archived,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Transaction is a single in/out movement of money. Sign convention:
// AmountCents > 0 is income / inbound; < 0 is expense / outbound.
// Linking to an Account is required (orphan transactions don't make
// sense for net-worth math). Category is freeform string so the user
// owns their taxonomy — the UI surfaces a unique-list as autocomplete.
type Transaction struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	Date        string    `json:"date"`             // YYYY-MM-DD
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`         // usually = Account.Currency, allow override for FX
	Category    string    `json:"category,omitempty"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	GoalID      string    `json:"goal_id,omitempty"` // links to a FinGoal (e.g. savings deposit)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
type Subscription struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`             // "Netflix"
	AmountCents   int64     `json:"amount_cents"`     // signed: usually < 0 (outflow)
	Currency      string    `json:"currency"`
	Cadence       string    `json:"cadence"`          // see SubCadence
	NextRenewal   string    `json:"next_renewal"`     // YYYY-MM-DD
	AccountID     string    `json:"account_id,omitempty"` // billed against
	Category      string    `json:"category,omitempty"`
	URL           string    `json:"url,omitempty"`        // cancellation / login link
	Notes         string    `json:"notes,omitempty"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Holding is a single position in a portfolio account. Quantity is a
// float64 because shares can be fractional; cost basis is cents-per-
// quantity-unit at acquisition (so total cost = Quantity *
// CostBasisCents). Live price is fetched out-of-band by the UI (or
// not at all) — this struct only stores user-entered facts.
type Holding struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	Ticker         string    `json:"ticker"`            // "VTI", "BTC", free-form
	Name           string    `json:"name,omitempty"`    // friendly name
	Quantity       float64   `json:"quantity"`
	CostBasisCents int64     `json:"cost_basis_cents"`  // per-unit, in Currency
	Currency       string    `json:"currency"`
	AsOf           string    `json:"as_of,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
func TransactionsPath(v string) string  { return filepath.Join(dir(v), "transactions.json") }
func SubscriptionsPath(v string) string { return filepath.Join(dir(v), "subscriptions.json") }
func HoldingsPath(v string) string      { return filepath.Join(dir(v), "holdings.json") }
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

func LoadAccounts(v string) []Account     { return loadAll[Account](AccountsPath(v)) }
func SaveAccounts(v string, x []Account) error { return saveAll(v, AccountsPath(v), x) }

func LoadTransactions(v string) []Transaction     { return loadAll[Transaction](TransactionsPath(v)) }
func SaveTransactions(v string, x []Transaction) error { return saveAll(v, TransactionsPath(v), x) }

func LoadSubscriptions(v string) []Subscription     { return loadAll[Subscription](SubscriptionsPath(v)) }
func SaveSubscriptions(v string, x []Subscription) error { return saveAll(v, SubscriptionsPath(v), x) }

func LoadHoldings(v string) []Holding     { return loadAll[Holding](HoldingsPath(v)) }
func SaveHoldings(v string, x []Holding) error { return saveAll(v, HoldingsPath(v), x) }

func LoadFinGoals(v string) []FinGoal     { return loadAll[FinGoal](FinGoalsPath(v)) }
func SaveFinGoals(v string, x []FinGoal) error { return saveAll(v, FinGoalsPath(v), x) }

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

// SortTransactionsByDate: newest first, then ID for stable order.
func SortTransactionsByDate(xs []Transaction) []Transaction {
	out := make([]Transaction, len(xs))
	copy(out, xs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].ID > out[j].ID
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
