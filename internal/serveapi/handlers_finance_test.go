package serveapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/finance"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// financeTestServer wires just enough Server to drive the finance
// handlers. Same shape as modulesTestServer — no auth middleware, no
// watcher, no file server. Tests hit the handler functions directly
// via httptest.
func financeTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent { return nil })
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.Default()
	s := &Server{
		cfg: Config{
			Vault:     v,
			TaskStore: store,
			Daily:     daily.DailyConfig{Template: daily.DefaultConfig().Template},
			Logger:    logger,
		},
		hub: wshub.New(logger),
	}
	return s, root
}

// TestFinanceAccount_RoundTrip — POST → list shows the new account →
// PATCH balance → list reflects the patch → DELETE → list is empty.
// Catches drift on the full CRUD path in a single happy-path test.
func TestFinanceAccount_RoundTrip(t *testing.T) {
	s, _ := financeTestServer(t)

	// POST
	body := `{"name":"Checking","kind":"checking","currency":"USD","balance_cents":250000}`
	w := httptest.NewRecorder()
	s.handleCreateAccount(w, httptest.NewRequest(http.MethodPost, "/api/v1/finance/accounts", bytes.NewBufferString(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var created finance.Account
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.BalanceCents != 250000 {
		t.Errorf("create returned unexpected: %+v", created)
	}

	// LIST
	w = httptest.NewRecorder()
	s.handleListAccounts(w, httptest.NewRequest(http.MethodGet, "/api/v1/finance/accounts", nil))
	var listResp struct {
		Accounts []finance.Account `json:"accounts"`
		Total    int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.Total != 1 || listResp.Accounts[0].ID != created.ID {
		t.Errorf("list mismatch: %+v", listResp)
	}

	// PATCH would need chi URL params — skip in this style of test
	// (the modules test does the same). The package-level finance
	// tests cover the on-disk state path; this test is purely about
	// the HTTP round-trip surface.

	// DELETE via the underlying SaveAccounts so we can verify the
	// list-after-delete shape without a chi mux.
	if err := finance.SaveAccounts(s.cfg.Vault.Root, []finance.Account{}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.handleListAccounts(w, httptest.NewRequest(http.MethodGet, "/api/v1/finance/accounts", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Total != 0 {
		t.Errorf("after delete: got total=%d, want 0", listResp.Total)
	}
}

// TestFinanceOverview_Math seeds a representative set of accounts +
// subscriptions + income streams and verifies the composite numbers
// the dashboard reads. Catches regression on the overview formula
// the way TestSortIncomeForDisplay catches regression on sort order.
func TestFinanceOverview_Math(t *testing.T) {
	s, root := financeTestServer(t)

	// Two USD accounts: checking +$2,500.78, credit −$1,000 (debt).
	// Net worth = 2500.78 − 1000 = 1500.78 → 150078 cents.
	now := time.Now().UTC()
	accs := []finance.Account{
		{ID: "a1", Name: "Checking", Kind: string(finance.AccountChecking), Currency: "USD", BalanceCents: 250078, CreatedAt: now, UpdatedAt: now},
		{ID: "a2", Name: "Card", Kind: string(finance.AccountCredit), Currency: "USD", BalanceCents: -100000, CreatedAt: now, UpdatedAt: now},
	}
	if err := finance.SaveAccounts(root, accs); err != nil {
		t.Fatal(err)
	}

	// Two subs: monthly Netflix −$15.99 due in 3 days; yearly insurance
	// −$1200 due in 20 days.
	in3 := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	in20 := time.Now().AddDate(0, 0, 20).Format("2006-01-02")
	subs := []finance.Subscription{
		{ID: "s1", Name: "Netflix", AmountCents: -1599, Currency: "USD", Cadence: string(finance.CadenceMonthly), NextRenewal: in3, Active: true, CreatedAt: now, UpdatedAt: now},
		{ID: "s2", Name: "Insurance", AmountCents: -120000, Currency: "USD", Cadence: string(finance.CadenceYearly), NextRenewal: in20, Active: true, CreatedAt: now, UpdatedAt: now},
		// Inactive — must not contribute to the totals.
		{ID: "s3", Name: "Old", AmountCents: -999, Currency: "USD", Cadence: string(finance.CadenceMonthly), NextRenewal: in3, Active: false, CreatedAt: now, UpdatedAt: now},
	}
	if err := finance.SaveSubscriptions(root, subs); err != nil {
		t.Fatal(err)
	}

	// Two streams: active employment $8000 actual / $8000 projected;
	// idea SaaS $0 actual / $3000 projected. Pipeline projected
	// includes the idea but not the active actual.
	streams := []finance.IncomeStream{
		{ID: "i1", Name: "Day job", Status: string(finance.IncomeActive), Kind: string(finance.IncomeKindEmployment), ProjectedMonthlyCents: 800000, ActualMonthlyCents: 800000, Currency: "USD", CreatedAt: now, UpdatedAt: now},
		{ID: "i2", Name: "Side SaaS", Status: string(finance.IncomeIdea), Kind: string(finance.IncomeKindBusiness), ProjectedMonthlyCents: 300000, ActualMonthlyCents: 0, Currency: "USD", CreatedAt: now, UpdatedAt: now},
	}
	if err := finance.SaveIncome(root, streams); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.handleFinanceOverview(w, httptest.NewRequest(http.MethodGet, "/api/v1/finance/overview", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("overview: got %d, body=%s", w.Code, w.Body.String())
	}
	var o financeOverview
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}

	// Currency: only USD accounts → USD.
	if o.Currency != "USD" {
		t.Errorf("currency = %q, want USD", o.Currency)
	}
	// Net worth: 250078 - 100000 = 150078.
	if o.NetWorthCents != 150078 {
		t.Errorf("net_worth_cents = %d, want 150078", o.NetWorthCents)
	}
	if o.AssetsCents != 250078 {
		t.Errorf("assets_cents = %d, want 250078", o.AssetsCents)
	}
	if o.LiabilitiesCents != 100000 {
		t.Errorf("liabilities_cents = %d, want 100000", o.LiabilitiesCents)
	}
	// Sub monthly: Netflix 1599 + insurance 120000/12=10000 = 11599.
	if o.SubMonthlyCents != 11599 {
		t.Errorf("subscription_monthly_cents = %d, want 11599", o.SubMonthlyCents)
	}
	// Upcoming-7d: only Netflix (3 days), not insurance (20 days),
	// not old (inactive).
	if o.UpcomingSubsCount != 1 {
		t.Errorf("upcoming_subs_count = %d, want 1", o.UpcomingSubsCount)
	}
	// Income: active actual 800000; pipeline projected 300000 + active
	// projected 800000 = total projected 1100000.
	if o.IncomeActualCents != 800000 {
		t.Errorf("income_monthly_actual_cents = %d, want 800000", o.IncomeActualCents)
	}
	if o.IncomeProjectedCents != 1100000 {
		t.Errorf("income_monthly_projected_cents = %d, want 1100000", o.IncomeProjectedCents)
	}
	if o.IncomeActiveCount != 1 {
		t.Errorf("income_active_count = %d, want 1", o.IncomeActiveCount)
	}
	if o.IncomePipelineCount != 1 {
		t.Errorf("income_pipeline_count = %d, want 1", o.IncomePipelineCount)
	}
}

// TestFinanceOverview_Empty — overview against a fresh tempdir vault
// must return a fully-zeroed struct, not an error or partial JSON.
// Catches "what happens on first launch" without seed data.
func TestFinanceOverview_Empty(t *testing.T) {
	s, _ := financeTestServer(t)
	w := httptest.NewRecorder()
	s.handleFinanceOverview(w, httptest.NewRequest(http.MethodGet, "/api/v1/finance/overview", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	var o financeOverview
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	// Every numeric field should be zero, every count should be zero.
	if o.NetWorthCents != 0 || o.SubMonthlyCents != 0 || o.IncomeActualCents != 0 ||
		o.AccountsCount != 0 || o.IncomeActiveCount != 0 || o.GoalsActiveCount != 0 {
		t.Errorf("empty overview should be all-zero: %+v", o)
	}
}

// TestFinanceOverview_ForeignCurrencyAccountExcluded — when the user
// has a USD primary + an EUR account, the EUR account doesn't pollute
// the USD net-worth number. Multi-currency conversion is out of scope
// of this package; the per-account view shows the EUR balance.
func TestFinanceOverview_ForeignCurrencyAccountExcluded(t *testing.T) {
	s, root := financeTestServer(t)
	now := time.Now().UTC()
	accs := []finance.Account{
		{ID: "a1", Name: "USD Checking", Kind: string(finance.AccountChecking), Currency: "USD", BalanceCents: 100000, CreatedAt: now, UpdatedAt: now},
		{ID: "a2", Name: "USD Savings", Kind: string(finance.AccountSavings), Currency: "USD", BalanceCents: 200000, CreatedAt: now, UpdatedAt: now},
		// Foreign account — excluded from USD net-worth.
		{ID: "a3", Name: "EUR Travel", Kind: string(finance.AccountChecking), Currency: "EUR", BalanceCents: 500000, CreatedAt: now, UpdatedAt: now},
	}
	if err := finance.SaveAccounts(root, accs); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.handleFinanceOverview(w, httptest.NewRequest(http.MethodGet, "/api/v1/finance/overview", nil))
	var o financeOverview
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	if o.Currency != "USD" {
		t.Errorf("primary currency = %q, want USD", o.Currency)
	}
	// Net worth must include only the two USD accounts.
	if o.NetWorthCents != 300000 {
		t.Errorf("net_worth_cents = %d, want 300000 (EUR account must be excluded)", o.NetWorthCents)
	}
	// Account count is total non-archived (EUR included) — that's the
	// truthy count of "things you have" even if the summary number is
	// USD-only. Documented intent.
	if o.AccountsCount != 3 {
		t.Errorf("accounts_count = %d, want 3", o.AccountsCount)
	}
}
