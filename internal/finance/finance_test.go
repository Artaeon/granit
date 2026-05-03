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

func TestTransactionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []Transaction{
		{ID: "01a", AccountID: "acc1", Date: "2026-05-01", AmountCents: -1299, Currency: "USD", Category: "Food", CreatedAt: now, UpdatedAt: now},
		{ID: "01b", AccountID: "acc1", Date: "2026-04-30", AmountCents: 500000, Currency: "USD", Category: "Salary", CreatedAt: now, UpdatedAt: now},
	}
	if err := SaveTransactions(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadTransactions(dir)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch")
	}
	sorted := SortTransactionsByDate(got)
	if sorted[0].Date != "2026-05-01" {
		t.Errorf("sort newest-first failed: got %s first", sorted[0].Date)
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
		// Invalid date passes through unchanged so a corrupt entry
		// doesn't crash the renderer.
		{"not-a-date", string(CadenceMonthly), "not-a-date"},
	}
	for _, c := range cases {
		if got := AdvanceRenewal(c.in, c.cad); got != c.want {
			t.Errorf("AdvanceRenewal(%q, %q) = %q, want %q", c.in, c.cad, got, c.want)
		}
	}
}

func TestSaveEmptyProducesArray(t *testing.T) {
	// Same `[]` not `null` rule as deadlines / biblebookmarks — the
	// web's JSON parser unwraps arrays cleanly without a null branch.
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
}
