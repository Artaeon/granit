package shopping

import (
	"testing"
	"time"
)

// TestEffectiveQty pins the zero-defaults-to-1 contract. Older
// records may have been stored without quantity (proto-default 0);
// math that multiplied by raw Quantity would zero them out from
// running totals — a silent data error. The accessor guarantees
// price * effective-qty stays correct.
func TestEffectiveQty(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero defaults to 1", 0, 1},
		{"negative defaults to 1", -3, 1},
		{"explicit 1 stays 1", 1, 1},
		{"explicit 5 stays 5", 5, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := Item{Quantity: c.in}
			if got := it.EffectiveQty(); got != c.want {
				t.Errorf("EffectiveQty(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// TestLineTotal — price-less items contribute 0 (so the user can
// add a plan-to-buy without forcing an estimate up front), priced
// items multiply by effective qty.
func TestLineTotal(t *testing.T) {
	cases := []struct {
		name string
		it   Item
		want float64
	}{
		{"no price", Item{Price: 0, Quantity: 3}, 0},
		{"price + default qty", Item{Price: 2.5}, 2.5},
		{"price * qty", Item{Price: 2.5, Quantity: 4}, 10},
		{"negative qty falls back to 1", Item{Price: 5, Quantity: -2}, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.it.LineTotal(); got != c.want {
				t.Errorf("LineTotal = %v, want %v", got, c.want)
			}
		})
	}
}

// TestNormalizeStatus locks the canonical-3 contract. Unknown / empty
// → planned so a typo'd PATCH never strands an item in an
// unrenderable state.
func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"":         "planned",
		"PLANNED":  "planned",
		"  bought ": "bought",
		"skipped":  "skipped",
		"foo":      "planned",
	}
	for in, want := range cases {
		if got := NormalizeStatus(in); got != want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSortForDisplay covers the three-bucket ordering: planned
// (alphabetical within category), then bought (newest-first by
// BoughtAt), then skipped. Critical because the list page renders
// in this order with no client-side re-sort.
func TestSortForDisplay(t *testing.T) {
	in := []Item{
		{ID: "1", Name: "Bread", Status: "planned", Category: "groceries"},
		{ID: "2", Name: "Apples", Status: "planned", Category: "groceries"},
		{ID: "3", Name: "Old shirt", Status: "skipped"},
		{ID: "4", Name: "Olive oil", Status: "bought", BoughtAt: "2026-05-01"},
		{ID: "5", Name: "Coffee", Status: "bought", BoughtAt: "2026-05-08"},
		{ID: "6", Name: "Notebook", Status: "planned", Category: "books"},
	}
	out := SortForDisplay(in)

	wantOrder := []string{
		"Notebook",   // planned, books category (alpha-first)
		"Apples",     // planned, groceries category, alpha within
		"Bread",
		"Coffee",     // bought, newest BoughtAt first
		"Olive oil",
		"Old shirt",  // skipped last
	}
	if len(out) != len(wantOrder) {
		t.Fatalf("len = %d, want %d", len(out), len(wantOrder))
	}
	for i, want := range wantOrder {
		if out[i].Name != want {
			t.Errorf("position %d: got %q, want %q", i, out[i].Name, want)
		}
	}
}

// TestAggregateTotals_BoughtMonthFilter verifies the month boundary
// is right: only items with BoughtAt prefixed by the current
// YYYY-MM count toward bought-month-sum, regardless of whether the
// item is also a standard. Pinning `now` makes this deterministic.
func TestAggregateTotals_BoughtMonthFilter(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	items := []Item{
		// Planned items contribute to planned totals only.
		{Name: "A", Status: "planned", Price: 10, Quantity: 1},
		{Name: "B", Status: "planned", Price: 5, Quantity: 2}, // 10
		// Bought THIS month — counts.
		{Name: "C", Status: "bought", Price: 20, BoughtAt: "2026-05-03"},
		// Bought LAST month — does NOT count.
		{Name: "D", Status: "bought", Price: 30, BoughtAt: "2026-04-28"},
		// Bought this month, no price — counts toward count, not sum.
		{Name: "E", Status: "bought", Price: 0, BoughtAt: "2026-05-10"},
		// Skipped — never counts.
		{Name: "F", Status: "skipped", Price: 99},
	}
	got := AggregateTotals(items, now)
	if got.PlannedCount != 2 {
		t.Errorf("PlannedCount = %d, want 2", got.PlannedCount)
	}
	if got.PlannedSum != 20 {
		t.Errorf("PlannedSum = %v, want 20", got.PlannedSum)
	}
	if got.BoughtMonthCount != 2 {
		t.Errorf("BoughtMonthCount = %d, want 2", got.BoughtMonthCount)
	}
	if got.BoughtMonthSum != 20 {
		t.Errorf("BoughtMonthSum = %v, want 20", got.BoughtMonthSum)
	}
}

// TestValidate covers the rejection paths.
func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		it      Item
		wantErr bool
	}{
		{"happy", Item{Name: "Bread"}, false},
		{"empty name", Item{}, true},
		{"whitespace name", Item{Name: "   "}, true},
		{"negative price", Item{Name: "X", Price: -1}, true},
		{"negative qty", Item{Name: "X", Quantity: -1}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.it.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
