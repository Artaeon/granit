// Package vision is the canonical schema + IO for granit's "above
// goals" layer — the user's life mission, core values, and current
// season focus. State lives at <vault>/.granit/vision.json so the
// TUI gets the same source of truth when its surface lands.
//
// Single record per vault. Goals/tasks/projects are lists; vision
// is one statement that anchors everything else, so the on-disk
// shape is a single JSON object (not a wrapped array). The web
// renders it as a persistent dashboard strip the user re-reads
// every morning before drilling into tactics — that's where the
// focus comes from.
//
// Pure data + IO only.
package vision

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Vision is the user's "what am I doing with my life" statement,
// expressed in three layers:
//
//   - Mission: one sentence the user could say to a stranger about
//     why they're here. Long-lived; rarely changes.
//   - Values: 3-5 words/short phrases — the user's compass when a
//     decision isn't covered by tactics. "Faith. Family. Honesty.
//     Craft. Generosity." — that kind of thing.
//   - SeasonFocus: a single phrase for the next ~90 days. Changes
//     with seasons. The narrowing point that converts mission into
//     "what am I actually doing right now."
//
// SeasonStartedAt is recorded when SeasonFocus is first set or
// changed so the UI can show "day N of 90" — a visible runway makes
// season focus feel concrete instead of indefinite.
type Vision struct {
	Mission         string    `json:"mission,omitempty"`
	Values          []string  `json:"values,omitempty"`
	SeasonFocus     string    `json:"season_focus,omitempty"`
	SeasonStartedAt string    `json:"season_started_at,omitempty"` // YYYY-MM-DD
	Notes           string    `json:"notes,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// StatePath is .granit/vision.json. Single file, single object —
// distinct from the .granit/<concept>/ folder pattern used by
// multi-record domains (prayer, finance, measurements) since
// vision is one record per vault.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "vision.json")
}

// Load returns the vault's vision. Missing file → zero Vision (all
// fields empty), which the UI renders as the empty-state "set your
// mission" placeholder. A corrupt file returns the zero value too,
// for the same don't-crash-the-page reason as deadlines/biblebookmarks.
func Load(vaultRoot string) Vision {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return Vision{}
	}
	var v Vision
	if err := json.Unmarshal(data, &v); err != nil {
		return Vision{}
	}
	return v
}

// Save writes the vision via atomic tmp+rename. UpdatedAt is stamped
// here, not at the call site, so every persisted record carries a
// truthful "last touched" timestamp.
func Save(vaultRoot string, v Vision) error {
	if vaultRoot == "" {
		return errors.New("vision: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	v.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// IsEmpty reports whether the vision is unset — every textual field
// blank and no values listed. The dashboard strip uses this to
// decide between rendering the read view or the "set your mission"
// CTA. Notes alone don't count as set: the value of vision lies in
// the structured fields, not stray notes.
func (v Vision) IsEmpty() bool {
	return v.Mission == "" && v.SeasonFocus == "" && len(v.Values) == 0
}

// SeasonDayCount returns "day N of 90 in this season" given today's
// date. Returns (0, 0) when SeasonStartedAt is unset or unparseable
// — handlers should treat that as "season runway not yet anchored,
// hide the day counter."
//
// 90 is the canonical season length. Not configurable on purpose —
// quarter cadence is biblically and biologically rooted, and a
// per-user knob there would just be a knob.
func (v Vision) SeasonDayCount(today time.Time) (day, total int) {
	if v.SeasonStartedAt == "" {
		return 0, 0
	}
	started, err := time.Parse("2006-01-02", v.SeasonStartedAt)
	if err != nil {
		return 0, 0
	}
	// Truncate both ends to local-midnight so day N stays stable
	// across a single calendar day regardless of the wall clock.
	startDay := time.Date(started.Year(), started.Month(), started.Day(), 0, 0, 0, 0, today.Location())
	todayDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	d := int(todayDay.Sub(startDay).Hours()/24) + 1
	if d < 1 {
		d = 1
	}
	if d > 90 {
		d = 90
	}
	return d, 90
}
