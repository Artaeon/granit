// Package ventures is the canonical schema + IO for top-level
// ventures stored in <vault>/.granit/ventures.json.
//
// A "venture" is the umbrella entity above projects and goals — a
// company, side hustle, ministry, or research initiative that several
// projects/goals roll up to. Project.Venture and Goal.Venture stay
// as free-text strings (so existing data round-trips) and the new
// Venture entity is an OPTIONAL enrichment layer: a venture record
// adds description, mission, color, etc. to a name string projects
// already reference. A venture string with no matching record still
// renders as a chip, just without the extras.
//
// Pure data + IO. No HTTP, no UI — handlers wrap this package.
package ventures

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Status is the lifecycle state of a venture.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

// Venture is the on-disk record. Name is the unique key — Project.Venture
// and Goal.Venture point to it as a free-text string. All optional
// fields are omitempty so older state files round-trip unchanged.
type Venture struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	// Mission is a 1-3 sentence "why this exists" so a venture roll-up
	// can lead with purpose, not metadata. Renders above the linked
	// projects/goals on the detail view.
	Mission   string   `json:"mission,omitempty"`
	Color     string   `json:"color,omitempty"`
	Status    Status   `json:"status,omitempty"`
	URL       string   `json:"url,omitempty"`     // homepage / canonical link
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// StatePath returns the canonical file path so callers don't have to
// know the on-disk layout.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "ventures.json")
}

// LoadAll reads ventures.json. Returns an empty slice (not nil error)
// for both missing and corrupt files — a corrupt file would otherwise
// crash callers; the empty case mirrors how internal/goals handles
// the same situation.
func LoadAll(vaultRoot string) []Venture {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Venture
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// SaveAll writes the canonical list using an atomic tmp+rename so a
// crash mid-write cannot truncate the user's history.
func SaveAll(vaultRoot string, list []Venture) error {
	if vaultRoot == "" {
		return errors.New("ventures: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// Find returns a pointer to the named venture (case-insensitive on the
// name) or nil if not present. Case-insensitive because Project.Venture
// is free-text and a user typing "stoicera" should match a record named
// "Stoicera" — the alternative (silent no-match) is a UX trap.
func Find(list []Venture, name string) *Venture {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil
	}
	for i := range list {
		if strings.EqualFold(list[i].Name, target) {
			return &list[i]
		}
	}
	return nil
}

// Touch stamps UpdatedAt to now. Caller is responsible for SaveAll.
func (v *Venture) Touch() {
	v.UpdatedAt = time.Now().Format("2006-01-02")
}

// Validate reports problems with a venture record before save. Empty
// name is the only hard requirement (status / color / url are all
// free-text by design); the function returns a wrapped error so the
// HTTP layer can surface a 400 with the offending field.
func (v Venture) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("ventures: name is required")
	}
	return nil
}
