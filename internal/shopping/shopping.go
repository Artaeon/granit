// Package shopping is the canonical schema + IO for the user's
// shopping list. A single Item record represents one thing-to-buy,
// flagged as `Standard` when it's a recurring need (bread, olive
// oil, basic clothing items the user always restocks). Standards
// don't live in a separate template store — they're regular items
// that the UI groups onto a "Standards" view; re-planning a bought
// standard flips its status back to "planned" without duplicating
// the record. Single source of truth per real-world item.
//
// Storage: <vault>/.granit/shopping.json
//
// Pure data + IO only. No HTTP, no rendering. Stdlib + atomicio.
package shopping

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Status is the lifecycle state of an Item.
//   - planned: on the active list (default for new items)
//   - bought: the user has purchased it. For non-standard items
//     this is a terminal state (kept as history); for standards
//     the user can re-plan to flip back to planned.
//   - skipped: deliberately not buying. Same retention as bought.
type Status string

const (
	StatusPlanned Status = "planned"
	StatusBought  Status = "bought"
	StatusSkipped Status = "skipped"
)

// NormalizeStatus collapses user-supplied status strings to one of
// the canonical values. Unknown / empty defaults to planned so a
// fresh item without an explicit status starts on the active list.
func NormalizeStatus(s string) string {
	switch Status(strings.ToLower(strings.TrimSpace(s))) {
	case StatusPlanned, "":
		return string(StatusPlanned)
	case StatusBought:
		return string(StatusBought)
	case StatusSkipped:
		return string(StatusSkipped)
	default:
		return string(StatusPlanned)
	}
}

// CategorySuggestions is the default set the UI offers as a
// quick-pick. Categories are stored as plain strings so the user
// can introduce new ones without a server migration; this list
// drives the picker / filter chips and exposes "the way granit
// thinks about basic shopping" out of the box. Order matters: the
// UI renders chips in this order so groceries surface first.
var CategorySuggestions = []string{
	"groceries",
	"household",
	"clothing",
	"health",
	"electronics",
	"books",
	"gifts",
	"other",
}

// NormalizeCategory lowercases and trims a category. Empty stays
// empty so the UI can render an "uncategorized" group rather than
// silently bucketing into "other" — a category gap is a meaningful
// signal ("you forgot to organize this") that we shouldn't paper over.
func NormalizeCategory(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Cadence is how often a standard recurs. Used only for items
// flagged Standard=true — non-standard one-off purchases ignore
// this field. The /finance run-rate calc multiplies Price by the
// per-month factor (CadenceMonthlyFactor) to project recurring
// shopping spend alongside subscriptions.
type Cadence string

const (
	// CadenceNone means "no projected recurrence" — a standard
	// without a cadence is still in the standards catalogue but
	// doesn't contribute to the monthly run-rate. The user might
	// pick this up "when needed" rather than on a schedule.
	CadenceNone     Cadence = ""
	CadenceWeekly   Cadence = "weekly"
	CadenceBiweekly Cadence = "biweekly"
	CadenceMonthly  Cadence = "monthly"
	CadenceQuarterly Cadence = "quarterly"
	CadenceYearly   Cadence = "yearly"
)

// CadenceSuggestions lists the canonical values in display order.
var CadenceSuggestions = []string{
	string(CadenceNone),
	string(CadenceWeekly),
	string(CadenceBiweekly),
	string(CadenceMonthly),
	string(CadenceQuarterly),
	string(CadenceYearly),
}

// NormalizeCadence collapses input to a canonical value. Unknown
// → "" (none) so a typo'd PATCH doesn't accidentally project
// monthly spend the user didn't ask for.
func NormalizeCadence(s string) string {
	switch Cadence(strings.ToLower(strings.TrimSpace(s))) {
	case CadenceWeekly:
		return string(CadenceWeekly)
	case CadenceBiweekly:
		return string(CadenceBiweekly)
	case CadenceMonthly:
		return string(CadenceMonthly)
	case CadenceQuarterly:
		return string(CadenceQuarterly)
	case CadenceYearly:
		return string(CadenceYearly)
	default:
		return string(CadenceNone)
	}
}

// CadenceMonthlyFactor returns "how many times per month does this
// recur" — used to project monthly spend from a per-occurrence
// price. Weekly = 52/12 ≈ 4.333, biweekly = 26/12 ≈ 2.167.
// Cadence "" / unknown returns 0 so non-recurring items contribute
// nothing to the projection.
func CadenceMonthlyFactor(c string) float64 {
	switch Cadence(c) {
	case CadenceWeekly:
		return 52.0 / 12.0
	case CadenceBiweekly:
		return 26.0 / 12.0
	case CadenceMonthly:
		return 1
	case CadenceQuarterly:
		return 1.0 / 3.0
	case CadenceYearly:
		return 1.0 / 12.0
	default:
		return 0
	}
}

// Item is one thing-to-buy. Quantity defaults to 1 when zero is
// stored — UI math (Price * Quantity totals) treats Quantity=0
// as 1 so back-fill doesn't break running spend calculations on
// older records.
type Item struct {
	ID          string  `json:"id"`             // ULID, lowercase
	Name        string  `json:"name"`           // required
	Description string  `json:"description,omitempty"`
	URL         string  `json:"url,omitempty"`        // optional product link
	Price       float64 `json:"price,omitempty"`      // expected unit price (ignored when 0)
	Quantity    int     `json:"quantity,omitempty"`
	Category    string  `json:"category,omitempty"`
	Status      string  `json:"status"`
	// Standard flags an Item as a recurring need. The "Standards"
	// view surfaces these together so the user can re-plan a fresh
	// week's groceries by flipping their statuses back to planned.
	Standard bool   `json:"standard,omitempty"`
	// Cadence is the recurrence pattern for a Standard item:
	// weekly / biweekly / monthly / quarterly / yearly, or "" for
	// "no projected recurrence". Drives the /finance run-rate
	// projection — without a cadence a standard counts as a
	// catalogue entry only.
	Cadence string `json:"cadence,omitempty"`
	Notes   string `json:"notes,omitempty"`
	// BoughtAt is the YYYY-MM-DD date the user marked the item
	// bought. Set automatically on the planned→bought transition;
	// cleared on re-plan back to planned. Used by /finance for the
	// "bought this month" rollup.
	BoughtAt  string `json:"bought_at,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// MonthlyRecurringEstimate projects this item's monthly contribution
// to the recurring-spend run-rate. Returns 0 unless Standard=true
// AND a non-empty Cadence AND a positive Price are all set —
// missing any of those signals "we don't know enough to project".
// Quantity multiplies the per-occurrence price (a weekly purchase
// of 2 yogurts at €1.50 each is 2 * 1.50 * 4.33 ≈ €13/month).
func (i Item) MonthlyRecurringEstimate() float64 {
	if !i.Standard || i.Cadence == "" || i.Price <= 0 {
		return 0
	}
	return i.LineTotal() * CadenceMonthlyFactor(i.Cadence)
}

// EffectiveQty returns Quantity with the zero-default-to-1 mapping
// applied. Centralised so totals math (.Price * .EffectiveQty())
// stays consistent across handlers and the future TUI.
func (i Item) EffectiveQty() int {
	if i.Quantity <= 0 {
		return 1
	}
	return i.Quantity
}

// LineTotal is Price * EffectiveQty — the contribution this item
// makes to a planned/bought-spend rollup. Returns 0 when Price is 0
// (price-less items don't pollute the running total).
func (i Item) LineTotal() float64 {
	if i.Price == 0 {
		return 0
	}
	return i.Price * float64(i.EffectiveQty())
}

// Validate reports problems before save. Empty Name is the only
// hard requirement; the rest is optional metadata.
func (i Item) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("shopping: name is required")
	}
	if i.Price < 0 {
		return fmt.Errorf("shopping: price cannot be negative, got %v", i.Price)
	}
	if i.Quantity < 0 {
		return fmt.Errorf("shopping: quantity cannot be negative, got %d", i.Quantity)
	}
	return nil
}

// StatePath returns the canonical .granit/shopping.json path.
// Centralised so a future relocation is a single edit.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "shopping.json")
}

// LoadAll reads shopping.json. Returns nil for both missing and
// corrupt files — same pattern the rest of granit uses, so a
// corrupt file doesn't crash callers.
func LoadAll(vaultRoot string) []Item {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Item
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// SaveAll writes the full list via atomic tmp+rename so a crash
// mid-write cannot truncate the user's history.
func SaveAll(vaultRoot string, list []Item) error {
	if vaultRoot == "" {
		return errors.New("shopping: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if list == nil {
		list = []Item{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// FindByID returns a copy + index, or (Item{}, -1) when not found.
func FindByID(list []Item, id string) (Item, int) {
	for i, x := range list {
		if x.ID == id {
			return x, i
		}
	}
	return Item{}, -1
}

// SortForDisplay orders items for the list view: planned first
// (sorted by category then alphabetical), then bought (newest
// bought first), then skipped. Stable so the order survives
// reloads.
func SortForDisplay(list []Item) []Item {
	out := make([]Item, len(list))
	copy(out, list)
	rank := func(s string) int {
		switch s {
		case string(StatusPlanned):
			return 0
		case string(StatusBought):
			return 1
		case string(StatusSkipped):
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i].Status), rank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		// Within planned: by category then name.
		if out[i].Status == string(StatusPlanned) {
			if out[i].Category != out[j].Category {
				return out[i].Category < out[j].Category
			}
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		// Within bought: newest BoughtAt first (lex sort works for
		// YYYY-MM-DD); empty dates sink to the bottom.
		if out[i].Status == string(StatusBought) {
			if out[i].BoughtAt != out[j].BoughtAt {
				return out[i].BoughtAt > out[j].BoughtAt
			}
		}
		// Within skipped (or fallback): alphabetical.
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Totals aggregates planned, bought-this-month, and projected-
// recurring spend across the list. The /finance overview reads this
// to surface running numbers alongside accounts/income/subscriptions.
// Bought-month is determined by the YYYY-MM prefix of BoughtAt for
// the given Now — timezone-stable because BoughtAt is a date string
// in the user's local zone (set by handler at write time).
type Totals struct {
	PlannedCount     int     `json:"planned_count"`
	PlannedSum       float64 `json:"planned_sum"`
	BoughtMonthCount int     `json:"bought_month_count"`
	BoughtMonthSum   float64 `json:"bought_month_sum"`
	// RecurringMonthlyEstimate sums every standard item's projected
	// per-month spend (see Item.MonthlyRecurringEstimate). Combined
	// with finance subscriptions, this is "what does my baseline
	// month cost me" — a number the user can sanity-check against
	// their actual outflows.
	RecurringMonthlyEstimate float64 `json:"recurring_monthly_estimate"`
	// RecurringStandardsCount is how many standards contribute to
	// that projection. Hidden from the wire when zero so a fresh
	// vault doesn't show "0 recurring" noise.
	RecurringStandardsCount int `json:"recurring_standards_count,omitempty"`
}

// AggregateTotals walks the list and returns spend rollups. now is
// passed in so callers (and tests) can pin the "this month" window
// explicitly without hidden time.Now() reads.
func AggregateTotals(list []Item, now time.Time) Totals {
	var t Totals
	monthPrefix := now.Format("2006-01")
	for _, it := range list {
		switch it.Status {
		case string(StatusPlanned):
			t.PlannedCount++
			t.PlannedSum += it.LineTotal()
		case string(StatusBought):
			if strings.HasPrefix(it.BoughtAt, monthPrefix) {
				t.BoughtMonthCount++
				t.BoughtMonthSum += it.LineTotal()
			}
		}
		if est := it.MonthlyRecurringEstimate(); est > 0 {
			t.RecurringMonthlyEstimate += est
			t.RecurringStandardsCount++
		}
	}
	return t
}
