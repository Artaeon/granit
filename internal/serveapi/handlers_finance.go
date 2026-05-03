package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/finance"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

// One file per concept on disk; one WS path per file so subscribers
// can refetch only what changed (e.g. editing a subscription doesn't
// force the transactions tab to reload).
const (
	statePathAccounts      = ".granit/finance/accounts.json"
	statePathTransactions  = ".granit/finance/transactions.json"
	statePathSubscriptions = ".granit/finance/subscriptions.json"
	statePathHoldings      = ".granit/finance/holdings.json"
	statePathFinGoals      = ".granit/finance/goals.json"
)

func (s *Server) bcastFinance(path string) {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: path})
}

// ── helpers ──────────────────────────────────────────────────────────

func newULID() string { return strings.ToLower(ulid.Make().String()) }

// readJSON decodes a request body into dst, writing a 400 on parse
// failure and returning false. Saves five copies of the same boilerplate.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

// ── Overview ─────────────────────────────────────────────────────────
//
// Single composite endpoint so the dashboard can hydrate in one round
// trip. Each tab still has its own GET for live updates / detail
// views. Numbers are computed (not stored) — keeps the Finance schema
// pure-data and avoids stale aggregates after a TUI edit.

type financeOverview struct {
	Currency           string `json:"currency"`
	NetWorthCents      int64  `json:"net_worth_cents"`
	AssetsCents        int64  `json:"assets_cents"`
	LiabilitiesCents   int64  `json:"liabilities_cents"`
	MonthlyIncomeCents int64  `json:"monthly_income_cents"`   // last 30 days
	MonthlyOutCents    int64  `json:"monthly_outflow_cents"`  // last 30 days, abs
	SubMonthlyCents    int64  `json:"subscription_monthly_cents"`
	UpcomingSubsCount  int    `json:"upcoming_subs_count"`    // due in next 7 days
	AccountsCount      int    `json:"accounts_count"`
	TransactionsCount  int    `json:"transactions_count"`
	GoalsActiveCount   int    `json:"goals_active_count"`
}

func (s *Server) handleFinanceOverview(w http.ResponseWriter, r *http.Request) {
	v := s.cfg.Vault.Root
	accounts := finance.LoadAccounts(v)
	txs := finance.LoadTransactions(v)
	subs := finance.LoadSubscriptions(v)
	goals := finance.LoadFinGoals(v)

	out := financeOverview{}

	// Currency: pick the most-common Account.Currency. Multi-currency
	// vaults keep their per-account display intact; this single field
	// is just for the dashboard summary line. Empty string when no
	// accounts exist yet.
	if c := primaryCurrency(accounts); c != "" {
		out.Currency = c
	}

	// Net worth: assets (kind != credit/loan) - liabilities (kind in
	// credit/loan, with their negative balance treated as debt). Kept
	// straightforward — multi-currency conversion is out of scope; the
	// UI shows totals in `Currency` and notes when accounts use other
	// currencies.
	for _, a := range accounts {
		if a.Archived {
			continue
		}
		if a.Currency != "" && out.Currency != "" && a.Currency != out.Currency {
			continue // skip foreign accounts from the summary number
		}
		switch finance.AccountKind(a.Kind) {
		case finance.AccountCredit, finance.AccountLoan:
			out.LiabilitiesCents += -a.BalanceCents
		default:
			out.AssetsCents += a.BalanceCents
		}
	}
	out.NetWorthCents = out.AssetsCents - out.LiabilitiesCents
	out.AccountsCount = countActive(accounts)

	// Last-30-days cashflow: sum signed transactions whose date >=
	// (today - 30). Treat amounts in non-primary currency the same as
	// accounts above — exclude.
	cutoff := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	for _, t := range txs {
		if t.Date < cutoff {
			continue
		}
		if out.Currency != "" && t.Currency != "" && t.Currency != out.Currency {
			continue
		}
		if t.AmountCents > 0 {
			out.MonthlyIncomeCents += t.AmountCents
		} else {
			out.MonthlyOutCents += -t.AmountCents
		}
	}
	out.TransactionsCount = len(txs)

	// Subscription totals: sum of monthly-normalised costs (always
	// positive for display). Upcoming-7-day count separate from total.
	now := time.Now()
	in7 := now.AddDate(0, 0, 7).Format("2006-01-02")
	today := now.Format("2006-01-02")
	for _, sub := range subs {
		if !sub.Active {
			continue
		}
		if out.Currency != "" && sub.Currency != "" && sub.Currency != out.Currency {
			continue
		}
		monthly := sub.MonthlyCostCents()
		if monthly < 0 {
			monthly = -monthly
		}
		out.SubMonthlyCents += monthly
		if sub.NextRenewal >= today && sub.NextRenewal <= in7 {
			out.UpcomingSubsCount++
		}
	}

	for _, g := range goals {
		if g.Status == "" || g.Status == "active" {
			out.GoalsActiveCount++
		}
	}

	writeJSON(w, http.StatusOK, out)
}

func primaryCurrency(accounts []finance.Account) string {
	c := map[string]int{}
	for _, a := range accounts {
		if a.Archived || a.Currency == "" {
			continue
		}
		c[a.Currency]++
	}
	best, bestN := "", 0
	for k, n := range c {
		if n > bestN {
			best, bestN = k, n
		}
	}
	return best
}
func countActive(accounts []finance.Account) int {
	n := 0
	for _, a := range accounts {
		if !a.Archived {
			n++
		}
	}
	return n
}

// ── Accounts CRUD ────────────────────────────────────────────────────

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	out := finance.SortAccountsForDisplay(finance.LoadAccounts(s.cfg.Vault.Root))
	if out == nil {
		out = []finance.Account{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out, "total": len(out)})
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var a finance.Account
	if !readJSON(w, r, &a) {
		return
	}
	if strings.TrimSpace(a.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	a.Kind = finance.NormalizeAccountKind(a.Kind)
	if a.ID == "" {
		a.ID = newULID()
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	all := finance.LoadAccounts(s.cfg.Vault.Root)
	for _, x := range all {
		if x.ID == a.ID {
			writeError(w, http.StatusConflict, "id exists")
			return
		}
	}
	all = append(all, a)
	if err := finance.SaveAccounts(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathAccounts)
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handlePatchAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadAccounts(s.cfg.Vault.Root)
	idx := -1
	for i, a := range all {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	// Whitelist patch — same map[string]json.RawMessage pattern as
	// deadlines so a buggy client can't silently drop fields the user
	// isn't editing.
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	a := all[idx]
	if raw, ok := patch["name"]; ok {
		_ = json.Unmarshal(raw, &a.Name)
	}
	if raw, ok := patch["kind"]; ok {
		var k string
		_ = json.Unmarshal(raw, &k)
		a.Kind = finance.NormalizeAccountKind(k)
	}
	if raw, ok := patch["currency"]; ok {
		_ = json.Unmarshal(raw, &a.Currency)
	}
	if raw, ok := patch["balance_cents"]; ok {
		_ = json.Unmarshal(raw, &a.BalanceCents)
	}
	if raw, ok := patch["as_of"]; ok {
		_ = json.Unmarshal(raw, &a.AsOf)
	}
	if raw, ok := patch["notes"]; ok {
		_ = json.Unmarshal(raw, &a.Notes)
	}
	if raw, ok := patch["archived"]; ok {
		_ = json.Unmarshal(raw, &a.Archived)
	}
	a.UpdatedAt = time.Now().UTC()
	all[idx] = a
	if err := finance.SaveAccounts(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathAccounts)
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadAccounts(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, a := range all {
		if a.ID == id {
			found = true
			continue
		}
		out = append(out, a)
	}
	if !found {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err := finance.SaveAccounts(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathAccounts)
	w.WriteHeader(http.StatusNoContent)
}

// ── Transactions CRUD ────────────────────────────────────────────────

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	out := finance.SortTransactionsByDate(finance.LoadTransactions(s.cfg.Vault.Root))
	if out == nil {
		out = []finance.Transaction{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": out, "total": len(out)})
}

func (s *Server) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	var t finance.Transaction
	if !readJSON(w, r, &t) {
		return
	}
	if t.AccountID == "" || t.Date == "" {
		writeError(w, http.StatusBadRequest, "account_id + date required")
		return
	}
	if t.ID == "" {
		t.ID = newULID()
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	all := append(finance.LoadTransactions(s.cfg.Vault.Root), t)
	if err := finance.SaveTransactions(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathTransactions)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handlePatchTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadTransactions(s.cfg.Vault.Root)
	idx := -1
	for i, t := range all {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "tx not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	t := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("account_id", &t.AccountID)
	apply("date", &t.Date)
	apply("amount_cents", &t.AmountCents)
	apply("currency", &t.Currency)
	apply("category", &t.Category)
	apply("description", &t.Description)
	apply("tags", &t.Tags)
	apply("goal_id", &t.GoalID)
	t.UpdatedAt = time.Now().UTC()
	all[idx] = t
	if err := finance.SaveTransactions(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathTransactions)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadTransactions(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, t := range all {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		writeError(w, http.StatusNotFound, "tx not found")
		return
	}
	if err := finance.SaveTransactions(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathTransactions)
	w.WriteHeader(http.StatusNoContent)
}

// ── Subscriptions CRUD ───────────────────────────────────────────────

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	out := finance.SortSubscriptionsByRenewal(finance.LoadSubscriptions(s.cfg.Vault.Root))
	if out == nil {
		out = []finance.Subscription{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": out, "total": len(out)})
}

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var sub finance.Subscription
	if !readJSON(w, r, &sub) {
		return
	}
	if strings.TrimSpace(sub.Name) == "" || sub.NextRenewal == "" {
		writeError(w, http.StatusBadRequest, "name + next_renewal required")
		return
	}
	sub.Cadence = finance.NormalizeCadence(sub.Cadence)
	if sub.ID == "" {
		sub.ID = newULID()
	}
	now := time.Now().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now
	all := append(finance.LoadSubscriptions(s.cfg.Vault.Root), sub)
	if err := finance.SaveSubscriptions(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathSubscriptions)
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) handlePatchSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadSubscriptions(s.cfg.Vault.Root)
	idx := -1
	for i, x := range all {
		if x.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "sub not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	x := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("name", &x.Name)
	apply("amount_cents", &x.AmountCents)
	apply("currency", &x.Currency)
	if raw, ok := patch["cadence"]; ok {
		var c string
		_ = json.Unmarshal(raw, &c)
		x.Cadence = finance.NormalizeCadence(c)
	}
	apply("next_renewal", &x.NextRenewal)
	apply("account_id", &x.AccountID)
	apply("category", &x.Category)
	apply("url", &x.URL)
	apply("notes", &x.Notes)
	apply("active", &x.Active)
	x.UpdatedAt = time.Now().UTC()
	all[idx] = x
	if err := finance.SaveSubscriptions(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathSubscriptions)
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadSubscriptions(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, x := range all {
		if x.ID == id {
			found = true
			continue
		}
		out = append(out, x)
	}
	if !found {
		writeError(w, http.StatusNotFound, "sub not found")
		return
	}
	if err := finance.SaveSubscriptions(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathSubscriptions)
	w.WriteHeader(http.StatusNoContent)
}

// ── Holdings CRUD ────────────────────────────────────────────────────

func (s *Server) handleListHoldings(w http.ResponseWriter, r *http.Request) {
	out := finance.LoadHoldings(s.cfg.Vault.Root)
	if out == nil {
		out = []finance.Holding{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"holdings": out, "total": len(out)})
}

func (s *Server) handleCreateHolding(w http.ResponseWriter, r *http.Request) {
	var h finance.Holding
	if !readJSON(w, r, &h) {
		return
	}
	if h.AccountID == "" || strings.TrimSpace(h.Ticker) == "" {
		writeError(w, http.StatusBadRequest, "account_id + ticker required")
		return
	}
	if h.ID == "" {
		h.ID = newULID()
	}
	now := time.Now().UTC()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}
	h.UpdatedAt = now
	all := append(finance.LoadHoldings(s.cfg.Vault.Root), h)
	if err := finance.SaveHoldings(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathHoldings)
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handleDeleteHolding(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadHoldings(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, h := range all {
		if h.ID == id {
			found = true
			continue
		}
		out = append(out, h)
	}
	if !found {
		writeError(w, http.StatusNotFound, "holding not found")
		return
	}
	if err := finance.SaveHoldings(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathHoldings)
	w.WriteHeader(http.StatusNoContent)
}

// ── Financial goals CRUD ─────────────────────────────────────────────

func (s *Server) handleListFinGoals(w http.ResponseWriter, r *http.Request) {
	out := finance.LoadFinGoals(s.cfg.Vault.Root)
	if out == nil {
		out = []finance.FinGoal{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"goals": out, "total": len(out)})
}

func (s *Server) handleCreateFinGoal(w http.ResponseWriter, r *http.Request) {
	var g finance.FinGoal
	if !readJSON(w, r, &g) {
		return
	}
	if strings.TrimSpace(g.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	g.Kind = finance.NormalizeGoalKind(g.Kind)
	if g.ID == "" {
		g.ID = newULID()
	}
	if g.Status == "" {
		g.Status = "active"
	}
	now := time.Now().UTC()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	all := append(finance.LoadFinGoals(s.cfg.Vault.Root), g)
	if err := finance.SaveFinGoals(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathFinGoals)
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handlePatchFinGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadFinGoals(s.cfg.Vault.Root)
	idx := -1
	for i, g := range all {
		if g.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	g := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("name", &g.Name)
	if raw, ok := patch["kind"]; ok {
		var k string
		_ = json.Unmarshal(raw, &k)
		g.Kind = finance.NormalizeGoalKind(k)
	}
	apply("target_cents", &g.TargetCents)
	apply("current_cents", &g.CurrentCents)
	apply("currency", &g.Currency)
	apply("target_date", &g.TargetDate)
	apply("linked_account_id", &g.LinkedAccountID)
	apply("notes", &g.Notes)
	apply("status", &g.Status)
	g.UpdatedAt = time.Now().UTC()
	all[idx] = g
	if err := finance.SaveFinGoals(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathFinGoals)
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteFinGoal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadFinGoals(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, g := range all {
		if g.ID == id {
			found = true
			continue
		}
		out = append(out, g)
	}
	if !found {
		writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if err := finance.SaveFinGoals(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathFinGoals)
	w.WriteHeader(http.StatusNoContent)
}
