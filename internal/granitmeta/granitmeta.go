// Package granitmeta exposes lightweight readers/writers for the JSON
// sidecars granit maintains under <vault>/.granit/. Both the TUI and the
// web API target the same files; granitmeta is the single source of truth
// for the on-disk schema.
package granitmeta

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/artaeon/granit/internal/atomicio"
)

// Event mirrors the schema in <vault>/.granit/events.json.
type Event struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Location  string `json:"location,omitempty"`
	Color     string `json:"color,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func ReadEvents(vaultRoot string) ([]Event, error) {
	return readJSON[[]Event](filepath.Join(vaultRoot, ".granit", "events.json"))
}

func WriteEvents(vaultRoot string, events []Event) error {
	return writeJSON(filepath.Join(vaultRoot, ".granit", "events.json"), events)
}

// ProjectMilestone is a sub-step inside a ProjectGoal.
type ProjectMilestone struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// ProjectGoal is a high-level goal within a project, with optional
// milestones tracked under it.
type ProjectGoal struct {
	Title      string             `json:"title"`
	Done       bool               `json:"done"`
	Milestones []ProjectMilestone `json:"milestones,omitempty"`
}

// Project mirrors a single entry in <vault>/.granit/projects.json. The
// full schema — keep all fields the TUI writes so we can round-trip
// without dropping data on PATCH.
type Project struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Folder      string        `json:"folder"`
	Tags        []string      `json:"tags"`
	Status      string        `json:"status"`
	Color       string        `json:"color"`
	CreatedAt   string        `json:"created_at"`
	Notes       []string      `json:"notes,omitempty"`
	TaskFilter  string        `json:"task_filter,omitempty"`
	Category    string        `json:"category,omitempty"`
	Goals       []ProjectGoal `json:"goals,omitempty"`
	NextAction  string        `json:"next_action,omitempty"`
	Priority    int           `json:"priority,omitempty"`
	DueDate     string        `json:"due_date,omitempty"`
	TimeSpent   int           `json:"time_spent,omitempty"`
	UpdatedAt   string        `json:"updated_at,omitempty"`
}

func ReadProjects(vaultRoot string) ([]Project, error) {
	return readJSON[[]Project](filepath.Join(vaultRoot, ".granit", "projects.json"))
}

func WriteProjects(vaultRoot string, projects []Project) error {
	return writeJSON(filepath.Join(vaultRoot, ".granit", "projects.json"), projects)
}

// Milestone is a sub-step inside a Goal (top-level goals.json entry, not
// to be confused with ProjectMilestone).
type Milestone struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Goal mirrors a single entry in <vault>/.granit/goals.json.
type Goal struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Status      string      `json:"status,omitempty"`
	Category    string      `json:"category,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	TargetDate  string      `json:"target_date,omitempty"`
	CreatedAt   string      `json:"created_at,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
	Project     string      `json:"project,omitempty"`
	Milestones  []Milestone `json:"milestones,omitempty"`
}

func ReadGoals(vaultRoot string) ([]Goal, error) {
	return readJSON[[]Goal](filepath.Join(vaultRoot, ".granit", "goals.json"))
}

func WriteGoals(vaultRoot string, goals []Goal) error {
	return writeJSON(filepath.Join(vaultRoot, ".granit", "goals.json"), goals)
}

func readJSON[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return zero, nil
		}
		return zero, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}
