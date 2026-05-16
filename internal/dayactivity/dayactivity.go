// Package dayactivity aggregates everything created, completed, or
// touched on a given calendar day across the vault — notes, tasks,
// calendar events, habit toggles, prayer intentions, hub links,
// jots — into a single time-ordered list.
//
// It powers the "What happened that day" surface on the Jots feed
// (and the live-rendered `## Day overview` section in a daily note)
// so the user can scroll back through one day's activity without
// hopping between five surfaces.
//
// All timestamps compare in the vault's LOCAL zone — a daily note
// is keyed by its YYYY-MM-DD filename, and a vault on a Berlin
// laptop must group items by Berlin midnight even when the
// underlying file mtimes are stored in UTC. The package is read-
// only: it reads the canonical sidecar files (.granit/events.json,
// .granit/prayer/intentions.json, etc.) plus the vault snapshot and
// never writes. Callers cap the per-day item count via MaxItems on
// the input Query.
package dayactivity

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/habits"
	"github.com/artaeon/granit/internal/hub"
	"github.com/artaeon/granit/internal/prayer"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// ItemKind tags each aggregated entry so the renderer can group by
// type. Free-form strings rather than an enum so the frontend can
// switch on them without a generated bridge.
type ItemKind string

const (
	KindNoteCreated   ItemKind = "note_created"
	KindTaskCreated   ItemKind = "task_created"
	KindTaskCompleted ItemKind = "task_completed"
	KindEvent         ItemKind = "event"
	KindHabit         ItemKind = "habit"
	KindPrayer        ItemKind = "prayer"
	KindJot           ItemKind = "jot"
	KindHubItem       ItemKind = "hub_item"
)

// Item is one entry in the day's activity feed. Every field is
// optional except Kind + At + Title. Path / TargetID let the
// frontend render a clickable link; Detail is a short context line
// (project, priority, time-of-day) the UI surfaces under the title.
type Item struct {
	Kind     ItemKind  `json:"kind"`
	At       time.Time `json:"at"`
	Title    string    `json:"title"`
	Detail   string    `json:"detail,omitempty"`
	Path     string    `json:"path,omitempty"`      // vault-relative note path, when applicable
	TargetID string    `json:"target_id,omitempty"` // task id / event id / etc.
}

// Query describes the day we're aggregating. Loc must be the vault-
// local zone — a date like "2026-05-16" is interpreted in this zone
// to compute the [00:00, 24:00) window we use to bucket file mtimes
// and timestamped sidecar entries.
//
// MaxItems caps the returned slice; defaults to 200 when zero.
type Query struct {
	Date     time.Time
	Loc      *time.Location
	MaxItems int
}

// Sources bundles every dependency the aggregator reads. Wiring
// goes through this struct so the HTTP handler stays a thin glue
// layer and tests can inject a controlled fixture vault.
//
// DailyFolder is the configured daily-notes subfolder (empty for
// vault root) — used to recognise (and exclude) daily notes from
// the "notes created today" feed so the day's own page doesn't
// appear as content within itself.
type Sources struct {
	Vault       *vault.Vault
	Tasks       *tasks.TaskStore
	VaultRoot   string
	DailyFolder string
}

// dayWindow returns [start, end) in the requested location for the
// given date — i.e. local midnight to local midnight-the-next-day.
// All comparisons in this package run against this window.
func dayWindow(date time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
}

// inWindow is a small helper — clearer than scattering the half-open
// comparison logic across every collector.
func inWindow(t, start, end time.Time) bool {
	return !t.Before(start) && t.Before(end)
}

// dateKeyFor returns the YYYY-MM-DD string for the day — used to
// key into Logs / Intentions whose timestamps are stored as date
// strings rather than full RFC3339 stamps.
func dateKeyFor(date time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return date.In(loc).Format("2006-01-02")
}

// Collect returns every activity entry that falls inside the
// requested day window, sorted ascending by timestamp. Empty days
// return an empty slice (not nil) so JSON consumers don't have to
// special-case null.
//
// Each collector below is conservative: a missing sidecar is
// silently treated as "nothing happened of that kind" — never a
// hard error. Aggregating is a read-only ancillary surface, so
// failing on one corrupt file would defeat the whole point.
func Collect(q Query, src Sources) []Item {
	loc := q.Loc
	if loc == nil {
		loc = time.Local
	}
	start, end := dayWindow(q.Date, loc)
	dateKey := dateKeyFor(q.Date, loc)

	max := q.MaxItems
	if max <= 0 {
		max = 200
	}

	out := make([]Item, 0, 32)

	if src.Vault != nil {
		out = collectNotes(out, src.Vault, src.DailyFolder, start, end, loc)
		out = collectJots(out, src.Vault, src.DailyFolder, dateKey, loc)
	}
	if src.Tasks != nil {
		out = collectTasks(out, src.Tasks, start, end)
	}
	if src.VaultRoot != "" {
		out = collectEvents(out, src.VaultRoot, dateKey, loc)
		out = collectHabits(out, src.VaultRoot, dateKey, loc)
		out = collectPrayer(out, src.VaultRoot, dateKey, loc)
		out = collectHub(out, src.VaultRoot, start, end)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		// Stable secondary sort so output is deterministic across
		// runs — important for tests + for the dedupe pass below.
		return out[i].Title < out[j].Title
	})

	if len(out) > max {
		out = out[:max]
	}
	return out
}

// dailyRe returns the compiled YYYY-MM-DD-style daily-note regex
// scoped to the configured folder (matching jotPathRegex over in
// internal/serveapi/handlers_jots.go). Used to exclude the day's
// own daily note from the "notes created" list — the daily IS the
// container, not an item within itself.
func dailyRe(folder string) *regexp.Regexp {
	folder = strings.Trim(folder, "/")
	if folder == "" {
		return regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
	}
	return regexp.MustCompile(`^` + regexp.QuoteMeta(folder) + `/(\d{4}-\d{2}-\d{2})\.md$`)
}

// noteCreationTime returns the moment a note was "created". Prefers
// frontmatter['created'] (the user's authoritative answer) if it
// parses as RFC3339 or YYYY-MM-DD; otherwise falls back to the
// file's mtime. The fallback is correct for the common case where
// the user creates a note and never edits it again — mtime equals
// ctime there. For long-lived notes the user can always set
// frontmatter explicitly to pin a creation date.
func noteCreationTime(n *vault.Note, loc *time.Location) time.Time {
	if n.Frontmatter != nil {
		if v, ok := n.Frontmatter["created"]; ok {
			if s, ok := v.(string); ok {
				if t, err := time.ParseInLocation(time.RFC3339, s, loc); err == nil {
					return t
				}
				if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
					return t
				}
			}
		}
	}
	return n.ModTime.In(loc)
}

// noteSummary returns the first heading or first non-empty line of
// a note's body, used as the descriptive "Detail" line under the
// title in the day-activity feed.
func noteSummary(content string) string {
	body := content
	// Skip frontmatter.
	if strings.HasPrefix(body, "---\n") || strings.HasPrefix(body, "---\r\n") {
		if idx := strings.Index(body[3:], "\n---"); idx >= 0 {
			body = body[3+idx+4:]
			body = strings.TrimLeft(body, "\r\n")
		}
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// First H1/H2/H3 wins.
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		if strings.HasPrefix(line, "### ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "### "))
		}
		// Otherwise the first non-empty line.
		if len(line) > 140 {
			line = line[:140] + "…"
		}
		return line
	}
	return ""
}

func collectNotes(out []Item, v *vault.Vault, dailyFolder string, start, end time.Time, loc *time.Location) []Item {
	re := dailyRe(dailyFolder)
	for _, n := range v.SnapshotNotes() {
		if re.MatchString(n.RelPath) {
			// Daily note itself — skip; the day IS this note.
			continue
		}
		// Skip granit-internal scratch notes (history snapshots,
		// search index, etc. — anything under .granit/).
		if strings.HasPrefix(n.RelPath, ".granit/") {
			continue
		}
		// Cheap pre-filter: mtime is the upper bound on creation
		// (you can't have been created after your last save), so
		// if mtime is before the day window, the note can't have
		// been created today either.
		if n.ModTime.Before(start) {
			continue
		}
		v.EnsureLoaded(n.RelPath)
		created := noteCreationTime(n, loc)
		if !inWindow(created, start, end) {
			continue
		}
		title := n.Title
		if fmTitle, ok := n.Frontmatter["title"].(string); ok && fmTitle != "" {
			title = fmTitle
		}
		out = append(out, Item{
			Kind:   KindNoteCreated,
			At:     created,
			Title:  title,
			Detail: noteSummary(n.Content),
			Path:   n.RelPath,
		})
	}
	return out
}

// collectTasks reads CreatedAt + CompletedAt off the sidecar. A
// single task can produce two entries — one when it was added,
// another when it was checked off — even if both fell on the same
// day. That symmetry is intentional: the user wants to see "I
// finished what I started today" as two beats on the timeline,
// not one merged event.
func collectTasks(out []Item, store *tasks.TaskStore, start, end time.Time) []Item {
	for _, t := range store.All() {
		if !t.CreatedAt.IsZero() && inWindow(t.CreatedAt, start, end) {
			out = append(out, Item{
				Kind:     KindTaskCreated,
				At:       t.CreatedAt,
				Title:    t.Text,
				Detail:   taskDetail(t),
				Path:     t.NotePath,
				TargetID: t.ID,
			})
		}
		if t.CompletedAt != nil && inWindow(*t.CompletedAt, start, end) {
			out = append(out, Item{
				Kind:     KindTaskCompleted,
				At:       *t.CompletedAt,
				Title:    t.Text,
				Detail:   taskDetail(t),
				Path:     t.NotePath,
				TargetID: t.ID,
			})
		}
	}
	return out
}

// taskDetail produces a short "project · priority" suffix the UI
// can render under the task title. Both fields are optional; an
// empty Detail just renders as no second line.
func taskDetail(t tasks.Task) string {
	var parts []string
	if t.Project != "" {
		parts = append(parts, t.Project)
	}
	if t.Priority > 0 {
		parts = append(parts, "!"+priorityLabel(t.Priority))
	}
	return strings.Join(parts, " · ")
}

func priorityLabel(p int) string {
	switch p {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	}
	return ""
}

// collectEvents reads .granit/events.json and returns every event
// whose Date matches today. Recurring events get one entry per
// occurrence is OUT of scope for now — the day-activity surface is
// "what happened that day" rather than a full calendar; the user
// already has a calendar page for that. Single occurrences cover
// 95% of the value with a fraction of the complexity.
func collectEvents(out []Item, vaultRoot, dateKey string, loc *time.Location) []Item {
	events, err := granitmeta.ReadEvents(vaultRoot)
	if err != nil {
		return out
	}
	for _, ev := range events {
		if ev.Date != dateKey {
			continue
		}
		at := time.Date(0, 0, 0, 0, 0, 0, 0, loc)
		if t, err := time.ParseInLocation("2006-01-02", ev.Date, loc); err == nil {
			at = t
		}
		if ev.StartTime != "" {
			if t, err := time.ParseInLocation("2006-01-02 15:04", ev.Date+" "+ev.StartTime, loc); err == nil {
				at = t
			}
		}
		detail := ev.StartTime
		if ev.StartTime != "" && ev.EndTime != "" {
			detail = ev.StartTime + "–" + ev.EndTime
		}
		if ev.Location != "" {
			if detail != "" {
				detail += " · " + ev.Location
			} else {
				detail = ev.Location
			}
		}
		out = append(out, Item{
			Kind:     KindEvent,
			At:       at,
			Title:    ev.Title,
			Detail:   detail,
			TargetID: ev.ID,
		})
	}
	return out
}

// collectHabits emits one entry per habit toggled on this date.
// The habit log carries no per-toggle timestamp (it's a "completed
// on YYYY-MM-DD" list), so we anchor each entry at local noon —
// late enough to land after most morning routines, early enough to
// stay above evening prayer entries. Stable per-habit ordering
// keeps multiple toggles deterministic across renders.
func collectHabits(out []Item, vaultRoot, dateKey string, loc *time.Location) []Item {
	data := habits.Load(vaultRoot)
	for _, log := range data.Logs {
		if log.Date != dateKey {
			continue
		}
		anchor, _ := time.ParseInLocation("2006-01-02 15:04", dateKey+" 12:00", loc)
		for i, name := range log.Completed {
			// 30-second offset per entry keeps stable ordering
			// without colliding the timestamps.
			at := anchor.Add(time.Duration(i) * 30 * time.Second)
			out = append(out, Item{
				Kind:   KindHabit,
				At:     at,
				Title:  name,
				Detail: "habit completed",
			})
		}
		break // one log entry per date
	}
	return out
}

// collectPrayer surfaces intentions that were added or answered on
// this day. Both timestamps are checked against the day window.
func collectPrayer(out []Item, vaultRoot, dateKey string, loc *time.Location) []Item {
	intentions := prayer.LoadAll(vaultRoot)
	start, end := dayWindow(mustParseDate(dateKey, loc), loc)
	for _, p := range intentions {
		if !p.CreatedAt.IsZero() && inWindow(p.CreatedAt, start, end) {
			out = append(out, Item{
				Kind:     KindPrayer,
				At:       p.CreatedAt,
				Title:    p.Text,
				Detail:   "prayer added",
				TargetID: p.ID,
			})
		}
		if p.AnsweredAt == dateKey && p.AnsweredAt != "" {
			// Answered-at is stored as a YYYY-MM-DD bucket rather
			// than a precise stamp; anchor at noon so it sorts
			// between event-typical morning and habit-noon entries.
			at, _ := time.ParseInLocation("2006-01-02 12:00", dateKey+" 12:00", loc)
			out = append(out, Item{
				Kind:     KindPrayer,
				At:       at,
				Title:    p.Text,
				Detail:   "prayer answered",
				TargetID: p.ID,
			})
		}
	}
	return out
}

// collectHub surfaces hub items created on this day. Hub items
// have an RFC3339 CreatedAt string. Items without one (legacy
// entries) silently fall out of the feed — we'd rather under-
// surface than guess.
func collectHub(out []Item, vaultRoot string, start, end time.Time) []Item {
	items, err := hub.LoadAll(vaultRoot)
	if err != nil {
		return out
	}
	for _, it := range items {
		if it.CreatedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, it.CreatedAt)
		if err != nil {
			continue
		}
		if !inWindow(t, start, end) {
			continue
		}
		detail := it.Category
		if detail == "" {
			detail = "hub link"
		}
		out = append(out, Item{
			Kind:     KindHubItem,
			At:       t,
			Title:    it.Title,
			Detail:   detail,
			TargetID: it.ID,
		})
	}
	return out
}

// collectJots parses the today-daily-note's `## Jots` section and
// emits one Item per bullet line. The composer writes timestamped
// bullets in the form `- HH:MM — text` (see web/src/routes/jots/
// +page.svelte's submitJot); we honour that so multiple jots on
// the same day land at the right time on the timeline. Bullets
// without a leading timestamp fall back to local noon.
func collectJots(out []Item, v *vault.Vault, dailyFolder, dateKey string, loc *time.Location) []Item {
	rel := dateKey + ".md"
	folder := strings.Trim(dailyFolder, "/")
	if folder != "" {
		rel = folder + "/" + rel
	}
	n := v.GetNote(rel)
	if n == nil {
		return out
	}
	v.EnsureLoaded(rel)
	body := n.Content
	idx := strings.Index(body, "## Jots")
	if idx < 0 {
		return out
	}
	section := body[idx:]
	// Stop at the next H2 heading.
	if next := strings.Index(section[1:], "\n## "); next >= 0 {
		section = section[:next+1]
	}
	noonAnchor, _ := time.ParseInLocation("2006-01-02 12:00", dateKey+" 12:00", loc)
	fallback := 0
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if text == "" {
			continue
		}
		at := noonAnchor.Add(time.Duration(fallback) * time.Minute)
		// Recognise "HH:MM — rest" prefix.
		if len(text) >= 5 && text[2] == ':' && isDigit(text[0]) && isDigit(text[1]) && isDigit(text[3]) && isDigit(text[4]) {
			if t, err := time.ParseInLocation("2006-01-02 15:04", dateKey+" "+text[:5], loc); err == nil {
				at = t
				rest := strings.TrimSpace(text[5:])
				rest = strings.TrimPrefix(rest, "—")
				rest = strings.TrimPrefix(rest, "-")
				text = strings.TrimSpace(rest)
			}
		}
		out = append(out, Item{
			Kind:  KindJot,
			At:    at,
			Title: text,
			Path:  rel,
		})
		fallback++
	}
	return out
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// mustParseDate parses a YYYY-MM-DD string against the given zone.
// Used in collectPrayer to derive the day window from the same
// dateKey the other collectors share. Returns the zero time on
// parse failure — the resulting window is unreachable, so prayer
// items silently disappear rather than misbinning.
func mustParseDate(s string, loc *time.Location) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}
