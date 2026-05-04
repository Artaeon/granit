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
//
// Kind/Venture/RepoURL were added later; older projects.json files
// pre-dating these fields round-trip correctly because every new field
// is `omitempty` and the JSON decoder leaves missing keys at the zero
// value.
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
	// Kind is the project type — drives which extra fields the UI
	// surfaces (e.g. RepoURL only renders when Kind == "software").
	// Free-form so the UI can introduce new types without a server
	// migration; the canonical set today is software, content,
	// research, business, personal, creative, client, other.
	Kind string `json:"kind,omitempty"`
	// Venture groups projects under a parent organization, company,
	// or umbrella initiative. Free-text by design — projects can be
	// grouped without first creating a formal venture record.
	Venture string `json:"venture,omitempty"`
	// RepoURL is the source-control URL for software projects.
	// Persisted regardless of Kind so a project can be reclassified
	// without losing the link, but the UI hides the field unless
	// Kind == "software".
	RepoURL string `json:"repo_url,omitempty"`
}

func ReadProjects(vaultRoot string) ([]Project, error) {
	return readJSON[[]Project](filepath.Join(vaultRoot, ".granit", "projects.json"))
}

func WriteProjects(vaultRoot string, projects []Project) error {
	return writeJSON(filepath.Join(vaultRoot, ".granit", "projects.json"), projects)
}

// NOTE: the top-level Goal schema previously lived here as granitmeta.Goal.
// It was a stripped subset that silently dropped fields the TUI wrote
// (Notes, ReviewFrequency, LastReviewed, ReviewLog, CompletedAt, Color,
// per-milestone DueDate / CompletedAt). It was retired in favour of the
// internal/goals package, which is the single source of truth for the
// .granit/goals.json on-disk schema. Use internal/goals from now on.

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
