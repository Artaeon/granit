package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/repos"
)

// repoStatusCache memoises repos.StatusOf calls so the hub strip can
// render on every tick without forking `git` each frame. Keyed by
// absolute repo path; entries expire after 30s so dirty-file changes
// the user just made show up within that window.
//
// Package-level + mutex so the cache survives across renders without
// having to live on the Model struct (no callsite churn).
var (
	repoStatusCacheMu sync.Mutex
	repoStatusCache   = map[string]repoStatusEntry{}
)

type repoStatusEntry struct {
	status  repos.Status
	err     error
	fetched time.Time
}

const repoStatusTTL = 30 * time.Second

// cachedRepoStatus returns the Status for path, fetching fresh when
// the cached entry is missing or older than TTL. The fetch can be
// expensive (subprocess + I/O) so we cap it at the package timeout
// inside repos.StatusOf.
func cachedRepoStatus(path string) (repos.Status, error) {
	if strings.TrimSpace(path) == "" {
		return repos.Status{}, repos.ErrNotARepo
	}
	repoStatusCacheMu.Lock()
	entry, ok := repoStatusCache[path]
	repoStatusCacheMu.Unlock()
	if ok && time.Since(entry.fetched) < repoStatusTTL {
		return entry.status, entry.err
	}
	s, err := repos.StatusOf(path)
	repoStatusCacheMu.Lock()
	repoStatusCache[path] = repoStatusEntry{status: s, err: err, fetched: time.Now()}
	repoStatusCacheMu.Unlock()
	return s, err
}

// ---------------------------------------------------------------------------
// Project / Goal Hub strip
// ---------------------------------------------------------------------------
//
// When the active note is a typed-project or typed-goal, render a single
// summary line above the editor pane showing:
//
//   🎯 Project Apollo · 7 tasks (3 done) · 2 ideas linked · Alt+T to add task
//
// Cheap to render — pulls from already-cached state (m.cachedTasks,
// objectsIndex, vault.GetNote). Hidden when the note isn't a typed
// project/goal so we don't waste a row on regular notes.

// renderProjectGoalHub returns the strip for the active note, or "" if
// the note isn't a typed-project or typed-goal. width is the editor
// inner width (minus chrome) so we can truncate cleanly.
func (m *Model) renderProjectGoalHub(width int) string {
	if m.objectsIndex == nil || m.activeNote == "" {
		return ""
	}
	obj := m.objectsIndex.ByPath(m.activeNote)
	if obj == nil {
		return ""
	}
	if obj.TypeID != "project" && obj.TypeID != "goal" {
		return ""
	}

	t, _ := m.objectsRegistry.ByID(obj.TypeID)
	icon := t.Icon
	if strings.TrimSpace(icon) == "" {
		if obj.TypeID == "project" {
			icon = "🎯"
		} else {
			icon = "🏁"
		}
	}

	// Count linked tasks. The enrichment pass already populates
	// Task.Project from a typed-project note's title, so for
	// projects we match by Project == obj.Title. For goals there's
	// no equivalent enrichment yet — match `goal:G…` markdown refs
	// by whatever the user types. Here we fall back to "tasks in
	// the same note" for goals so the strip still shows a count.
	open, done := 0, 0
	switch obj.TypeID {
	case "project":
		for _, task := range m.cachedTasks {
			if task.Project == obj.Title {
				if task.Done {
					done++
				} else {
					open++
				}
			}
		}
	case "goal":
		// Goals: tasks whose containing note IS the goal note (the
		// user wrote checkboxes inside the goal note), counted as
		// linked. A future enrichment pass could honour `goal:G001`
		// references too.
		for _, task := range m.cachedTasks {
			if task.NotePath == obj.NotePath {
				if task.Done {
					done++
				} else {
					open++
				}
			}
		}
	}
	total := open + done

	// Status & target date when present.
	var meta []string
	if status := obj.PropertyValue("status"); status != "" {
		meta = append(meta, hubStatusBadge(status))
	}
	if obj.TypeID == "goal" {
		if td := obj.PropertyValue("target_date"); td != "" {
			meta = append(meta, "→ "+td)
		}
	}
	if obj.TypeID == "project" {
		if dl := obj.PropertyValue("deadline"); dl != "" {
			meta = append(meta, "deadline "+dl)
		}
	}

	// Repo status — for project notes that declare a `repo:` path,
	// surface live git state inline. Cached (30s TTL) so this stays
	// cheap on rapid renders.
	if obj.TypeID == "project" {
		if repoPath := strings.TrimSpace(obj.PropertyValue("repo")); repoPath != "" {
			meta = append(meta, formatRepoStatus(repoPath))
		}
	}

	// Build the strip.
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	parts := []string{
		icon + " " + titleStyle.Render(strings.ToUpper(string(obj.TypeID[0]))+obj.TypeID[1:]+": "+obj.Title),
	}
	if len(meta) > 0 {
		parts = append(parts, dimStyle.Render(strings.Join(meta, " · ")))
	}
	if total > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("· %s tasks (%s done)",
			numStyle.Render(fmt.Sprintf("%d", total)),
			numStyle.Render(fmt.Sprintf("%d", done)))))
	} else {
		parts = append(parts, dimStyle.Render("· no tasks yet"))
	}
	parts = append(parts, dimStyle.Render("· Alt+T to add task"))

	line := strings.Join(parts, " ")
	if width > 0 {
		line = TruncateDisplay(line, width)
	}

	// Underline rule so the strip visually separates from the
	// editor body without stealing focus.
	rule := lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", width))
	return line + "\n" + rule
}

// formatRepoStatus renders the live git status for a repo path as a
// compact chip: `git: branch · 3 dirty · ↑2 ↓0 · 2h ago`. Returns a
// short fallback string when the path isn't a repo or git fails.
func formatRepoStatus(path string) string {
	s, err := cachedRepoStatus(path)
	if err == repos.ErrNotARepo {
		return lipgloss.NewStyle().Foreground(overlay0).Render("git: (no repo at " + path + ")")
	}
	if err != nil {
		return lipgloss.NewStyle().Foreground(yellow).Render("git: error")
	}

	parts := []string{"git: " + s.Branch}
	if s.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d dirty", s.Dirty))
	}
	if s.Ahead > 0 || s.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↑%d ↓%d", s.Ahead, s.Behind))
	}
	if !s.LastCommit.IsZero() {
		parts = append(parts, hubAgeString(s.AgeSinceLastCommit()))
	}
	chip := strings.Join(parts, " · ")
	// Colour: green when clean + recent; yellow when dirty/ahead/behind;
	// dim when stale (> 30 days idle).
	style := lipgloss.NewStyle()
	switch {
	case s.AgeSinceLastCommit() > 30*24*time.Hour:
		style = style.Foreground(overlay0)
	case !s.IsClean():
		style = style.Foreground(yellow)
	default:
		style = style.Foreground(green)
	}
	return style.Render(chip)
}

// hubAgeString collapses a duration into a single short token: "2h",
// "3d", "5w". Exists alongside the dashboard's `relativeTime` because
// the hub wants tighter formatting (no leading space, no "ago").
func hubAgeString(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}

// hubStatusBadge renders a small coloured chip for a status value. Specific
// to project/goal status enums; falls back to a neutral chip for unknown
// values so a custom status doesn't break the layout.
func hubStatusBadge(status string) string {
	st := lipgloss.NewStyle().Bold(true)
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return st.Foreground(green).Render("● active")
	case "completed", "shipped":
		return st.Foreground(blue).Render("✓ " + status)
	case "paused":
		return st.Foreground(yellow).Render("⏸ paused")
	case "archived", "abandoned":
		return st.Foreground(overlay0).Render("⊘ " + status)
	case "backlog":
		return st.Foreground(overlay0).Render("○ backlog")
	default:
		return st.Foreground(overlay1).Render(status)
	}
}
