// Package aicontext builds a personalised "what's going on right
// now" snapshot of the user's vault for AI features to consume.
// One central producer means every AI feature works from the same
// curated view, the user can audit exactly what gets sent to a
// provider, and adding a new AI feature doesn't reinvent context-
// gathering.
//
// What the snapshot contains (and deliberately doesn't):
//   - Today's calendar events (all-day + timed)
//   - Open tasks: top 20 sorted by urgency (overdue → today → soon)
//   - Recent notes: top 10 by mod time (just title + first 200 chars)
//   - Active goals with status + days-until-target
//   - Active habits with current streaks
//   - Upcoming deadlines (next 30 days)
//   - Last 3 days of daily-note titles (NOT bodies — keeps payload
//     small; if a feature needs more it requests a daily-note read)
//
// Excluded by design: email bodies (PII-heavy), people records
// (PII-heavy), full note bodies (size + privacy), shopping items
// (low signal). Features that need these can pull them through
// dedicated endpoints with explicit user consent.
//
// Caching: BuildSnapshot is invoked per-request with a 60-second
// in-memory cache so a flurry of AI features (briefing + triage +
// suggestions firing within seconds) doesn't re-scan the vault.
package aicontext

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/deadlines"
	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// Snapshot is the curated view. Field order optimised for prompt
// readability — events first (most time-sensitive), tasks next,
// then ambient context (notes / goals / habits / deadlines).
type Snapshot struct {
	GeneratedAt   time.Time         `json:"generated_at"`
	Today         string            `json:"today"`           // YYYY-MM-DD
	TodayEvents   []EventSummary    `json:"today_events"`
	OpenTasks     []TaskSummary     `json:"open_tasks"`
	RecentNotes   []NoteSummary     `json:"recent_notes"`
	ActiveGoals   []GoalSummary     `json:"active_goals"`
	Deadlines     []DeadlineSummary `json:"upcoming_deadlines"`
	DailySummary  []DailyTouch      `json:"recent_dailies"`
}

type EventSummary struct {
	Title    string `json:"title"`
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	Location string `json:"location,omitempty"`
	AllDay   bool   `json:"all_day,omitempty"`
}

type TaskSummary struct {
	Title         string `json:"title"`
	NotePath      string `json:"note_path,omitempty"`
	DueDate       string `json:"due_date,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	ScheduledStart string `json:"scheduled_start,omitempty"`
	Project       string `json:"project,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

type NoteSummary struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	ModTime  string `json:"mod_time"`
	Excerpt  string `json:"excerpt,omitempty"` // first 200 chars
}

type GoalSummary struct {
	Title       string `json:"title"`
	Status      string `json:"status,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	DaysUntil   int    `json:"days_until,omitempty"`
}

type DeadlineSummary struct {
	Title      string `json:"title"`
	Date       string `json:"date"`
	DaysUntil  int    `json:"days_until"`
	Importance string `json:"importance,omitempty"`
}

type DailyTouch struct {
	Date  string `json:"date"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

// BuildOpts lets callers tune what the snapshot includes (some
// features only want today's events; other features want broader
// context). All zero values produce the default-balanced shape.
type BuildOpts struct {
	MaxOpenTasks    int // default 20
	MaxRecentNotes  int // default 10
	DeadlineHorizon int // days; default 30
	DailyHistory    int // days; default 3
}

// Builder reads from the same Vault + TaskStore + sidecars the API
// reads. Constructing one is cheap; share it across tick scopes.
type Builder struct {
	vault     *vault.Vault
	taskStore *tasks.TaskStore
	vaultRoot string

	mu       sync.Mutex
	cached   *Snapshot
	cachedAt time.Time
}

func New(v *vault.Vault, ts *tasks.TaskStore, vaultRoot string) *Builder {
	return &Builder{vault: v, taskStore: ts, vaultRoot: vaultRoot}
}

// Build returns a snapshot. Re-uses the in-memory cache when the
// last build is < 60 seconds old to avoid repeated vault scans
// when a flurry of AI calls fires near the same instant.
func (b *Builder) Build(opts BuildOpts) *Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cached != nil && time.Since(b.cachedAt) < 60*time.Second {
		return b.cached
	}
	if opts.MaxOpenTasks == 0 {
		opts.MaxOpenTasks = 20
	}
	if opts.MaxRecentNotes == 0 {
		opts.MaxRecentNotes = 10
	}
	if opts.DeadlineHorizon == 0 {
		opts.DeadlineHorizon = 30
	}
	if opts.DailyHistory == 0 {
		opts.DailyHistory = 3
	}

	now := time.Now()
	s := &Snapshot{
		GeneratedAt: now,
		Today:       now.Format("2006-01-02"),
	}
	s.TodayEvents = b.todayEvents(now)
	s.OpenTasks = b.openTasks(opts.MaxOpenTasks)
	s.RecentNotes = b.recentNotes(opts.MaxRecentNotes)
	s.ActiveGoals = b.activeGoals(now)
	s.Deadlines = b.upcomingDeadlines(now, opts.DeadlineHorizon)
	s.DailySummary = b.recentDailies(opts.DailyHistory)
	b.cached = s
	b.cachedAt = now
	return s
}

// Invalidate drops the cache; called after writes that meaningfully
// change the snapshot (task created, event saved, etc.) so AI
// features asked immediately after see fresh data.
func (b *Builder) Invalidate() {
	b.mu.Lock()
	b.cached = nil
	b.mu.Unlock()
}

// JSON returns prettified snapshot JSON for the "What AI sees"
// settings view. Same shape sent to providers (after redaction).
func (b *Builder) JSON(opts BuildOpts) ([]byte, error) {
	return json.MarshalIndent(b.Build(opts), "", "  ")
}

func (b *Builder) todayEvents(now time.Time) []EventSummary {
	events, err := granitmeta.ReadEvents(b.vaultRoot)
	if err != nil {
		return nil
	}
	today := now.Format("2006-01-02")
	var out []EventSummary
	for _, e := range events {
		if e.Date != today {
			continue
		}
		out = append(out, EventSummary{
			Title:    e.Title,
			Start:    e.StartTime,
			End:      e.EndTime,
			Location: e.Location,
			AllDay:   e.StartTime == "",
		})
	}
	return out
}

func (b *Builder) openTasks(limit int) []TaskSummary {
	if b.taskStore == nil {
		return nil
	}
	all := b.taskStore.All()
	now := time.Now()
	today := now.Format("2006-01-02")
	type ranked struct {
		t        tasks.Task
		urgency  int
	}
	var pool []ranked
	for _, t := range all {
		if t.Done {
			continue
		}
		// Snoozed → skip
		if t.SnoozedUntil != "" {
			if su, err := time.Parse(time.RFC3339, t.SnoozedUntil); err == nil && su.After(now) {
				continue
			}
		}
		// Urgency score: lower = sooner
		score := 1000
		if t.DueDate != "" {
			if t.DueDate < today {
				score = -100 // overdue
			} else if t.DueDate == today {
				score = 0
			} else {
				if d, err := time.Parse("2006-01-02", t.DueDate); err == nil {
					score = int(d.Sub(now).Hours() / 24)
				}
			}
		}
		// Tighten by priority (P1 = -10, P2 = -5, P3 = -1).
		switch t.Priority {
		case 1:
			score -= 10
		case 2:
			score -= 5
		case 3:
			score -= 1
		}
		pool = append(pool, ranked{t, score})
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].urgency < pool[j].urgency })
	if len(pool) > limit {
		pool = pool[:limit]
	}
	out := make([]TaskSummary, 0, len(pool))
	for _, r := range pool {
		ts := TaskSummary{
			Title:    r.t.Text,
			NotePath: r.t.NotePath,
			DueDate:  r.t.DueDate,
			Priority: r.t.Priority,
			Project:  r.t.Project,
			Tags:     r.t.Tags,
		}
		if r.t.ScheduledStart != nil {
			ts.ScheduledStart = r.t.ScheduledStart.Format(time.RFC3339)
		}
		out = append(out, ts)
	}
	return out
}

func (b *Builder) recentNotes(limit int) []NoteSummary {
	if b.vault == nil {
		return nil
	}
	type pair struct {
		path string
		mod  time.Time
	}
	var notes []pair
	for path, n := range b.vault.Notes {
		notes = append(notes, pair{path, n.ModTime})
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].mod.After(notes[j].mod) })
	if len(notes) > limit {
		notes = notes[:limit]
	}
	out := make([]NoteSummary, 0, len(notes))
	for _, p := range notes {
		n := b.vault.GetNote(p.path)
		if n == nil {
			continue
		}
		excerpt := strings.TrimSpace(stripFrontmatter(n.Content))
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "…"
		}
		out = append(out, NoteSummary{
			Path:    n.RelPath,
			Title:   n.Title,
			ModTime: n.ModTime.Format(time.RFC3339),
			Excerpt: excerpt,
		})
	}
	return out
}

func (b *Builder) activeGoals(now time.Time) []GoalSummary {
	all := goals.LoadAll(b.vaultRoot)
	var out []GoalSummary
	for _, g := range all {
		if g.Status != "" && g.Status != "active" {
			continue
		}
		gs := GoalSummary{Title: g.Title, Status: string(g.Status), TargetDate: g.TargetDate}
		if g.TargetDate != "" {
			if d, err := time.Parse("2006-01-02", g.TargetDate); err == nil {
				gs.DaysUntil = int(d.Sub(now).Hours() / 24)
			}
		}
		out = append(out, gs)
	}
	return out
}

func (b *Builder) upcomingDeadlines(now time.Time, horizonDays int) []DeadlineSummary {
	all := deadlines.LoadAll(b.vaultRoot)
	today := now.Truncate(24 * time.Hour)
	horizon := today.AddDate(0, 0, horizonDays)
	var out []DeadlineSummary
	for _, d := range all {
		if d.Status != "active" {
			continue
		}
		dt, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		dd := dt.Truncate(24 * time.Hour)
		if dd.Before(today) || dd.After(horizon) {
			continue
		}
		out = append(out, DeadlineSummary{
			Title:      d.Title,
			Date:       d.Date,
			DaysUntil:  int(dd.Sub(today).Hours() / 24),
			Importance: d.Importance,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DaysUntil < out[j].DaysUntil })
	return out
}

func (b *Builder) recentDailies(days int) []DailyTouch {
	now := time.Now()
	var out []DailyTouch
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -i)
		date := d.Format("2006-01-02")
		// Common daily-note paths in granit. We don't open the file
		// to avoid loading bodies; a feature that wants the body
		// should pull /api/v1/notes/<path> directly.
		path := date + ".md"
		if b.vault != nil {
			if n := b.vault.GetNote(path); n != nil {
				out = append(out, DailyTouch{Date: date, Path: n.RelPath, Title: n.Title})
				continue
			}
			// Try Daily/<date>.md too.
			alt := "Daily/" + path
			if n := b.vault.GetNote(alt); n != nil {
				out = append(out, DailyTouch{Date: date, Path: n.RelPath, Title: n.Title})
			}
		}
	}
	return out
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return content
	}
	rest := content[4+end+4:]
	rest = strings.TrimLeft(rest, "\n")
	return rest
}
