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
// force the income tab to reload).
const (
	statePathAccounts      = ".granit/finance/accounts.json"
	statePathSubscriptions = ".granit/finance/subscriptions.json"
	statePathIncome        = ".granit/finance/income.json"
	statePathFinGoals      = ".granit/finance/goals.json"
)

func (s *Server) bcastFinance(path string) {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: path})
}

// ── helpers ──────────────────────────────────────────────────────────

func newULID() string { return strings.ToLower(ulid.Make().String()) }

// readJSON decodes a request body into dst, writing a 400 on parse
// failure and returning false. Saves several copies of the same
// boilerplate across handlers.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

// ── Overview ─────────────────────────────────────────────────────────
//
// Single composite endpoint so the dashboard hydrates in one round
// trip. Numbers are computed (not stored) — keeps the schema pure-
// data and avoids stale aggregates after a TUI edit.

type financeOverview struct {
	Currency               string `json:"currency"`
	NetWorthCents          int64  `json:"net_worth_cents"`
	AssetsCents            int64  `json:"assets_cents"`
	LiabilitiesCents       int64  `json:"liabilities_cents"`
	IncomeActualCents      int64  `json:"income_monthly_actual_cents"`    // sum of active streams' actual
	IncomeProjectedCents   int64  `json:"income_monthly_projected_cents"` // sum across all non-paused streams' projected
	SubMonthlyCents        int64  `json:"subscription_monthly_cents"`
	UpcomingSubsCount      int    `json:"upcoming_subs_count"`
	AccountsCount          int    `json:"accounts_count"`
	IncomeActiveCount      int    `json:"income_active_count"`
	IncomePipelineCount    int    `json:"income_pipeline_count"` // idea + planned
	GoalsActiveCount       int    `json:"goals_active_count"`
}

func (s *Server) handleFinanceOverview(w http.ResponseWriter, r *http.Request) {
	v := s.cfg.Vault.Root
	accounts := finance.LoadAccounts(v)
	subs := finance.LoadSubscriptions(v)
	income := finance.LoadIncome(v)
	goals := finance.LoadFinGoals(v)

	out := financeOverview{}

	// Currency: pick the most-common Account.Currency. Multi-currency
	// users keep their per-account display intact; this single field
	// only drives the dashboard summary line. Empty when no accounts.
	out.Currency = primaryCurrency(accounts)

	// Net worth: assets (kind != credit/loan) − liabilities (credit/loan).
	// Multi-currency conversion is out of scope; foreign-currency
	// accounts are excluded from the summary number — the per-account
	// view still shows them.
	for _, a := range accounts {
		if a.Archived {
			continue
		}
		if a.Currency != "" && out.Currency != "" && a.Currency != out.Currency {
			continue
		}
		switch finance.AccountKind(a.Kind) {
		case finance.AccountCredit, finance.AccountLoan:
			out.LiabilitiesCents += -a.BalanceCents
		default:
			out.AssetsCents += a.BalanceCents
		}
	}
	out.NetWorthCents = out.AssetsCents - out.LiabilitiesCents
	out.AccountsCount = countActiveAccounts(accounts)

	// Subscriptions: monthly-normalised total + upcoming-7-day count.
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

	// Income: actual = sum of active streams' actual; projected =
	// sum across every non-paused stream's projected (so the user
	// sees both "today's run rate" and "if everything in the pipeline
	// hits, the future run rate").
	for _, s := range income {
		if out.Currency != "" && s.Currency != "" && s.Currency != out.Currency {
			continue
		}
		switch finance.IncomeStreamStatus(s.Status) {
		case finance.IncomeActive:
			out.IncomeActualCents += s.ActualMonthlyCents
			out.IncomeProjectedCents += s.ProjectedMonthlyCents
			out.IncomeActiveCount++
		case finance.IncomeIdea, finance.IncomePlanned:
			out.IncomeProjectedCents += s.ProjectedMonthlyCents
			out.IncomePipelineCount++
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
func countActiveAccounts(accounts []finance.Account) int {
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
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	a := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("name", &a.Name)
	if raw, ok := patch["kind"]; ok {
		var k string
		_ = json.Unmarshal(raw, &k)
		a.Kind = finance.NormalizeAccountKind(k)
	}
	apply("currency", &a.Currency)
	apply("balance_cents", &a.BalanceCents)
	apply("as_of", &a.AsOf)
	apply("notes", &a.Notes)
	apply("archived", &a.Archived)
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

// ── Income streams CRUD ──────────────────────────────────────────────

func (s *Server) handleListIncome(w http.ResponseWriter, r *http.Request) {
	out := finance.SortIncomeForDisplay(finance.LoadIncome(s.cfg.Vault.Root))
	if out == nil {
		out = []finance.IncomeStream{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"streams": out, "total": len(out)})
}

func (s *Server) handleCreateIncome(w http.ResponseWriter, r *http.Request) {
	var st finance.IncomeStream
	if !readJSON(w, r, &st) {
		return
	}
	if strings.TrimSpace(st.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	st.Status = finance.NormalizeIncomeStatus(st.Status)
	st.Kind = finance.NormalizeIncomeKind(st.Kind)
	if st.ID == "" {
		st.ID = newULID()
	}
	now := time.Now().UTC()
	if st.CreatedAt.IsZero() {
		st.CreatedAt = now
	}
	st.UpdatedAt = now
	all := append(finance.LoadIncome(s.cfg.Vault.Root), st)
	if err := finance.SaveIncome(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathIncome)
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handlePatchIncome(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadIncome(s.cfg.Vault.Root)
	idx := -1
	for i, st := range all {
		if st.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "income stream not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	st := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("name", &st.Name)
	if raw, ok := patch["status"]; ok {
		var s string
		_ = json.Unmarshal(raw, &s)
		st.Status = finance.NormalizeIncomeStatus(s)
	}
	if raw, ok := patch["kind"]; ok {
		var k string
		_ = json.Unmarshal(raw, &k)
		st.Kind = finance.NormalizeIncomeKind(k)
	}
	apply("projected_monthly_cents", &st.ProjectedMonthlyCents)
	apply("actual_monthly_cents", &st.ActualMonthlyCents)
	apply("currency", &st.Currency)
	apply("url", &st.URL)
	apply("started_at", &st.StartedAt)
	apply("notes", &st.Notes)
	st.UpdatedAt = time.Now().UTC()
	all[idx] = st
	if err := finance.SaveIncome(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathIncome)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDeleteIncome(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := finance.LoadIncome(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, st := range all {
		if st.ID == id {
			found = true
			continue
		}
		out = append(out, st)
	}
	if !found {
		writeError(w, http.StatusNotFound, "income stream not found")
		return
	}
	if err := finance.SaveIncome(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastFinance(statePathIncome)
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
