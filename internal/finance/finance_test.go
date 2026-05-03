package finance

import (
	"reflect"
	"testing"
	"time"
)

func TestAccountRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []Account{{
		ID:           "01a",
		Name:         "Main Checking",
		Kind:         string(AccountChecking),
		Currency:     "USD",
		BalanceCents: 245078,
		AsOf:         "2026-05-01",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}
	if err := SaveAccounts(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadAccounts(dir)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch:\n got=%+v\n want=%+v", got, in)
	}
}

func TestSubscriptionRoundTripAndCadence(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []Subscription{{
		ID: "01a", Name: "Netflix", AmountCents: -1599, Currency: "USD",
		Cadence: string(CadenceMonthly), NextRenewal: "2026-05-15",
		Active: true, CreatedAt: now, UpdatedAt: now,
	}}
	if err := SaveSubscriptions(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := LoadSubscriptions(dir); !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch")
	}

	// Cadence math: yearly $120 = $10/month = 1000 cents
	yearly := Subscription{AmountCents: 12000, Cadence: string(CadenceYearly)}
	if got := yearly.MonthlyCostCents(); got != 1000 {
		t.Errorf("yearly: got %d, want 1000", got)
	}
	// Weekly $7 ≈ $30.33/month — we use 52/12 ratio
	weekly := Subscription{AmountCents: 700, Cadence: string(CadenceWeekly)}
	want := int64(700) * 52 / 12
	if got := weekly.MonthlyCostCents(); got != want {
		t.Errorf("weekly: got %d, want %d", got, want)
	}
}

func TestAdvanceRenewal(t *testing.T) {
	cases := []struct {
		in, cad, want string
	}{
		{"2026-05-15", string(CadenceMonthly), "2026-06-15"},
		{"2026-12-15", string(CadenceMonthly), "2027-01-15"},
		{"2026-05-15", string(CadenceYearly), "2027-05-15"},
		{"2026-05-15", string(CadenceWeekly), "2026-05-22"},
		{"2026-05-15", string(CadenceQuarterly), "2026-08-15"},
		{"not-a-date", string(CadenceMonthly), "not-a-date"},
	}
	for _, c := range cases {
		if got := AdvanceRenewal(c.in, c.cad); got != c.want {
			t.Errorf("AdvanceRenewal(%q, %q) = %q, want %q", c.in, c.cad, got, c.want)
		}
	}
}

func TestIncomeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []IncomeStream{
		{
			ID:                    "01a",
			Name:                  "Day job",
			Status:                string(IncomeActive),
			Kind:                  string(IncomeKindEmployment),
			ProjectedMonthlyCents: 800000,
			ActualMonthlyCents:    810000,
			Currency:              "USD",
			StartedAt:             "2024-01-15",
			CreatedAt:             now,
			UpdatedAt:             now,
		},
		{
			ID:                    "01b",
			Name:                  "Side SaaS",
			Status:                string(IncomeIdea),
			Kind:                  string(IncomeKindBusiness),
			ProjectedMonthlyCents: 200000,
			ActualMonthlyCents:    0,
			Currency:              "USD",
			URL:                   "https://example.com",
			CreatedAt:             now,
			UpdatedAt:             now,
		},
	}
	if err := SaveIncome(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadIncome(dir)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch:\n got = %+v\n want = %+v", got, in)
	}
}

func TestSortIncomeForDisplay(t *testing.T) {
	// Mixed bag: active high, idea high, planned medium, paused, active low.
	// Active should come first — within active, higher actual first.
	streams := []IncomeStream{
		{ID: "idea", Status: string(IncomeIdea), ProjectedMonthlyCents: 300000},
		{ID: "active-low", Status: string(IncomeActive), ActualMonthlyCents: 200000},
		{ID: "paused", Status: string(IncomePaused), ProjectedMonthlyCents: 500000},
		{ID: "active-high", Status: string(IncomeActive), ActualMonthlyCents: 800000},
		{ID: "planned", Status: string(IncomePlanned), ProjectedMonthlyCents: 400000},
	}
	out := SortIncomeForDisplay(streams)
	want := []string{"active-high", "active-low", "planned", "idea", "paused"}
	for i, s := range out {
		if s.ID != want[i] {
			t.Errorf("sort[%d] = %q, want %q", i, s.ID, want[i])
		}
	}
}

func TestSaveEmptyProducesArray(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAccounts(dir, nil); err != nil {
		t.Fatalf("save nil: %v", err)
	}
	got := LoadAccounts(dir)
	if got == nil || len(got) != 0 {
		t.Errorf("LoadAccounts of empty file should return non-nil empty slice; got %v", got)
	}
}

func TestNormalizers(t *testing.T) {
	if NormalizeAccountKind("xyz") != string(AccountChecking) {
		t.Error("unknown kind should default to checking")
	}
	if NormalizeCadence("foo") != string(CadenceMonthly) {
		t.Error("unknown cadence should default to monthly")
	}
	if NormalizeGoalKind("foo") != string(GoalSavings) {
		t.Error("unknown goal kind should default to savings")
	}
	if NormalizeIncomeStatus("nope") != string(IncomeIdea) {
		t.Error("unknown income status should default to idea")
	}
	if NormalizeIncomeKind("nope") != string(IncomeKindOther) {
		t.Error("unknown income kind should default to other")
	}
}
