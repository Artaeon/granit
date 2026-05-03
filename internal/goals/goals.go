// Package goals is the canonical schema + IO for top-level goals stored in
// <vault>/.granit/goals.json. The TUI (internal/tui/goalsmode.go) and the
// web API (internal/serveapi/handlers_meta.go) both target this package so
// the on-disk representation has exactly one source of truth — round-tripping
// through the web no longer drops fields the TUI wrote.
//
// Pure data + IO only. No Bubbletea, no rendering.
package goals

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Status is the lifecycle state of a goal.
type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusArchived  Status = "archived"
	StatusPaused    Status = "paused"
)

// Milestone is a sub-step within a goal.
type Milestone struct {
	Text        string `json:"text"`
	Done        bool   `json:"done"`
	DueDate     string `json:"due_date,omitempty"` // YYYY-MM-DD
	CompletedAt string `json:"completed_at,omitempty"`
}

// Review is a periodic check-in snapshot.
type Review struct {
	Date     string `json:"date"`
	Note     string `json:"note"`
	Progress int    `json:"progress"` // snapshot at time of review
}

// Goal is a standalone goal independent of projects or habits. The full
// schema — every field below MUST be preserved by writers (no lossy
// round-trip) so a web PATCH can never silently drop TUI-only fields.
type Goal struct {
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	Description     string      `json:"description,omitempty"`
	Status          Status      `json:"status"`
	Category        string      `json:"category,omitempty"`
	Color           string      `json:"color,omitempty"`
	Tags            []string    `json:"tags,omitempty"`
	TargetDate      string      `json:"target_date,omitempty"` // YYYY-MM-DD
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
	CompletedAt     string      `json:"completed_at,omitempty"`
	Project         string      `json:"project,omitempty"`
	Milestones      []Milestone `json:"milestones"`
	Notes           string      `json:"notes,omitempty"`
	ReviewFrequency string      `json:"review_frequency,omitempty"` // "weekly", "monthly", "quarterly"
	LastReviewed    string      `json:"last_reviewed,omitempty"`    // YYYY-MM-DD
	ReviewLog       []Review    `json:"review_log,omitempty"`
}

// Progress returns milestone completion percentage (0-100). A goal with
// no milestones reports 100 if its status is Completed, else 0 — the
// list view uses this for the progress bar.
func (g Goal) Progress() int {
	if len(g.Milestones) == 0 {
		if g.Status == StatusCompleted {
			return 100
		}
		return 0
	}
	done := 0
	for _, m := range g.Milestones {
		if m.Done {
			done++
		}
	}
	return done * 100 / len(g.Milestones)
}

// DoneCount returns the number of completed milestones.
func (g Goal) DoneCount() int {
	done := 0
	for _, m := range g.Milestones {
		if m.Done {
			done++
		}
	}
	return done
}

// IsOverdue reports true when the target date is past and the goal is
// neither completed nor archived. Compares against today's midnight so
// "today" itself is not yet overdue.
func (g Goal) IsOverdue() bool {
	if g.TargetDate == "" || g.Status == StatusCompleted || g.Status == StatusArchived {
		return false
	}
	target, err := time.Parse("2006-01-02", g.TargetDate)
	if err != nil {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return today.After(target)
}

// DaysRemaining returns days until target date (-1 if no date set or
// the date is unparseable).
func (g Goal) DaysRemaining() int {
	if g.TargetDate == "" {
		return -1
	}
	target, err := time.Parse("2006-01-02", g.TargetDate)
	if err != nil {
		return -1
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return int(target.Sub(today).Hours() / 24)
}

// IsDueForReview reports whether the goal's review period has elapsed.
// Always false for non-active goals or goals without a configured frequency.
func (g Goal) IsDueForReview() bool {
	if g.ReviewFrequency == "" || g.Status != StatusActive {
		return false
	}
	if g.LastReviewed == "" {
		return true
	}
	last, err := time.Parse("2006-01-02", g.LastReviewed)
	if err != nil {
		return true
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch g.ReviewFrequency {
	case "weekly":
		return today.After(last.AddDate(0, 0, 7))
	case "monthly":
		return today.After(last.AddDate(0, 1, 0))
	case "quarterly":
		return today.After(last.AddDate(0, 3, 0))
	}
	return false
}

// NextReviewDate returns the next scheduled review date as YYYY-MM-DD.
// Falls back to CreatedAt as the base when LastReviewed is empty.
func (g Goal) NextReviewDate() string {
	if g.ReviewFrequency == "" {
		return ""
	}
	base := g.LastReviewed
	if base == "" {
		base = g.CreatedAt
	}
	last, err := time.Parse("2006-01-02", base)
	if err != nil {
		return ""
	}
	switch g.ReviewFrequency {
	case "weekly":
		return last.AddDate(0, 0, 7).Format("2006-01-02")
	case "monthly":
		return last.AddDate(0, 1, 0).Format("2006-01-02")
	case "quarterly":
		return last.AddDate(0, 3, 0).Format("2006-01-02")
	}
	return ""
}

// TimeframeLabel returns a human-readable time-remaining label like
// "5d left", "2mo overdue", "1y3mo left". Used by both the TUI list
// row and the web card.
func (g Goal) TimeframeLabel() string {
	days := g.DaysRemaining()
	if days < 0 {
		absDays := -days
		if absDays < 30 {
			return fmt.Sprintf("%dd overdue", absDays)
		}
		return fmt.Sprintf("%dmo overdue", absDays/30)
	}
	if days == 0 {
		return "due today"
	}
	if days == 1 {
		return "1d left"
	}
	if days < 14 {
		return fmt.Sprintf("%dd left", days)
	}
	if days < 60 {
		return fmt.Sprintf("%dw left", days/7)
	}
	if days < 365 {
		return fmt.Sprintf("%dmo left", days/30)
	}
	years := days / 365
	rem := (days % 365) / 30
	if rem > 0 {
		return fmt.Sprintf("%dy%dmo left", years, rem)
	}
	return fmt.Sprintf("%dy left", years)
}

// StatePath returns the canonical path to the goals.json state file.
// Centralised so a future relocation is a single edit.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "goals.json")
}

// LoadAll reads all goals from .granit/goals.json. Returns nil for both
// missing and corrupt files — callers handle the nil slice as the empty
// state (a corrupt file would otherwise crash the TUI on load).
func LoadAll(vaultRoot string) []Goal {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Goal
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// LoadActive returns only the goals with status = "active".
func LoadActive(vaultRoot string) []Goal {
	all := LoadAll(vaultRoot)
	var active []Goal
	for _, g := range all {
		if g.Status == StatusActive {
			active = append(active, g)
		}
	}
	return active
}

// SaveAll writes all goals to .granit/goals.json using an atomic
// tmp+rename so a crash mid-write cannot truncate the user's history.
// Returns nil on success.
func SaveAll(vaultRoot string, goals []Goal) error {
	if vaultRoot == "" {
		return errors.New("goals: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(goals, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// AddMilestone appends a milestone to the goal with the given ID and
// persists the result. No-op if the goal isn't found.
func AddMilestone(vaultRoot, goalID, text, dueDate string) error {
	all := LoadAll(vaultRoot)
	for i, g := range all {
		if g.ID == goalID {
			all[i].Milestones = append(all[i].Milestones, Milestone{
				Text:    text,
				DueDate: dueDate,
			})
			all[i].UpdatedAt = time.Now().Format(time.RFC3339)
			return SaveAll(vaultRoot, all)
		}
	}
	return nil
}
