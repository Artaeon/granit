// Package prayer is the canonical schema + IO for granit's prayer
// intentions log. Distinct from internal/biblebookmarks (saved
// passages) and from devotional notes (one-off reflections written
// to disk as markdown). A prayer intention is an *active* entry on
// the user's prayer list — "I'm praying for X" — with a status that
// moves from praying-for → answered or archived over time.
//
// State lives at <vault>/.granit/prayer/intentions.json so the TUI
// gets the same source of truth when its surface lands.
//
// Pure data + IO only.
package prayer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Status tracks where an intention is in its lifecycle. "praying" is
// the active state; "answered" closes the loop with the user's note
// of how it landed; "archived" is for items the user no longer wants
// to pray for actively but doesn't want to delete from history.
type Status string

const (
	StatusPraying  Status = "praying"
	StatusAnswered Status = "answered"
	StatusArchived Status = "archived"
)

// NormalizeStatus collapses user-supplied status strings to one of
// the three canonical values. Unknown / empty defaults to praying so
// a fresh entry without an explicit status starts on the active list.
func NormalizeStatus(s string) string {
	switch Status(s) {
	case StatusPraying, StatusAnswered, StatusArchived:
		return s
	default:
		return string(StatusPraying)
	}
}

// Category is a freeform string the user owns. The UI surfaces a
// distinct-list as autocomplete; we don't validate against a
// canonical taxonomy because everyone's prayer life is different.
// (Family / Self / World / Friends / Work are common starters.)

// Intention is a single prayer entry.
type Intention struct {
	ID         string    `json:"id"`         // ULID, lowercase
	Text       string    `json:"text"`       // the actual ask
	Category   string    `json:"category,omitempty"`
	Status     string    `json:"status"`     // see Status
	StartedAt  string    `json:"started_at,omitempty"`  // YYYY-MM-DD when added to the list
	AnsweredAt string    `json:"answered_at,omitempty"` // YYYY-MM-DD when marked answered
	Answer     string    `json:"answer,omitempty"`      // optional how-it-was-answered note
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// StatePath is the canonical .granit/prayer/intentions.json path
// for the given vault. Centralised so handlers, tests, and any
// future TUI surface all hit the same string.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "prayer", "intentions.json")
}

// LoadAll reads every intention from disk. Returns nil for both
// missing and corrupt files — same pattern as deadlines /
// biblebookmarks. A corrupt file should never crash the prayer page;
// the user can edit-and-fix the JSON or delete the offending entry.
func LoadAll(vaultRoot string) []Intention {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Intention
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// SaveAll writes every intention via atomic tmp+rename so a crash
// mid-write cannot truncate the user's history.
func SaveAll(vaultRoot string, xs []Intention) error {
	if vaultRoot == "" {
		return errors.New("prayer: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit", "prayer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if xs == nil {
		xs = []Intention{}
	}
	data, err := json.MarshalIndent(xs, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// SortForDisplay: praying-for first (newest at top so a fresh add
// surfaces immediately), then answered (newest answered first, so the
// recently-answered list reads as a feed), then archived. ID stable
// tiebreak so the order survives reloads.
func SortForDisplay(xs []Intention) []Intention {
	out := make([]Intention, len(xs))
	copy(out, xs)
	rank := func(s string) int {
		switch Status(s) {
		case StatusPraying:
			return 0
		case StatusAnswered:
			return 1
		case StatusArchived:
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
		// Within praying: newest started first.
		// Within answered: newest answered first.
		// Within archived: newest updated first.
		switch Status(out[i].Status) {
		case StatusAnswered:
			if out[i].AnsweredAt != out[j].AnsweredAt {
				return out[i].AnsweredAt > out[j].AnsweredAt
			}
		default:
			if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
				return out[i].UpdatedAt.After(out[j].UpdatedAt)
			}
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// FindByID returns the intention + its index, or (Intention{}, -1).
// Pointer-to-copy pattern matches deadlines / biblebookmarks.
func FindByID(xs []Intention, id string) (Intention, int) {
	for i, x := range xs {
		if x.ID == id {
			return x, i
		}
	}
	return Intention{}, -1
}
