package dayactivity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/habits"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// TestCollect_FixtureVault exercises the aggregator against a small
// purpose-built vault with one of each kind of activity on the
// target day plus a deliberate near-miss (the day before) that
// must NOT appear. Locks the ordering + ItemKind + Title shape so
// renderer regressions are caught at the data layer.
func TestCollect_FixtureVault(t *testing.T) {
	root := t.TempDir()
	loc := time.UTC // deterministic across CI hosts
	day := time.Date(2026, 5, 16, 0, 0, 0, 0, loc)
	dayKey := "2026-05-16"
	prevKey := "2026-05-15"

	// ── seed: notes ────────────────────────────────────────────────
	// Daily note for the target day (must be EXCLUDED from feed).
	mustWrite(t, root, "Daily/"+dayKey+".md", "# "+dayKey+"\n\n## Jots\n- 09:30 — first thought\n- 14:00 — afternoon idea\n")
	// A regular note created on the day.
	mustWrite(t, root, "Notes/idea.md", "---\ncreated: 2026-05-16T10:15:00Z\n---\n\n# Idea\n\nSome content here.\n")
	// A note created the day BEFORE — must NOT show up.
	mustWrite(t, root, "Notes/old.md", "---\ncreated: 2026-05-15T09:00:00Z\n---\n\n# Old\n")
	// A note with no frontmatter created date — falls back to mtime.
	mtimeOnly := filepath.Join(root, "Notes/mtime-only.md")
	if err := os.MkdirAll(filepath.Dir(mtimeOnly), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mtimeOnly, []byte("# Mtime only\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Touch its mtime to land squarely inside the target day.
	mtimeStamp := time.Date(2026, 5, 16, 16, 30, 0, 0, loc)
	if err := os.Chtimes(mtimeOnly, mtimeStamp, mtimeStamp); err != nil {
		t.Fatal(err)
	}

	// ── seed: events.json ──────────────────────────────────────────
	events := []granitmeta.Event{
		{ID: "ev-day", Title: "Team standup", Date: dayKey, StartTime: "09:00", EndTime: "09:30"},
		{ID: "ev-prev", Title: "Yesterday meeting", Date: prevKey, StartTime: "10:00"},
	}
	mustWriteJSON(t, root, ".granit/events.json", events)

	// ── seed: habits.md + logs ─────────────────────────────────────
	habitData := habits.Data{
		Habits:     []habits.Entry{{Name: "read", Created: dayKey, Streak: 1}},
		Logs:       []habits.Log{{Date: dayKey, Completed: []string{"read", "stretch"}}},
		Frequencies: map[string]string{}, Times: map[string]string{},
		Categories: map[string]string{}, Notes: map[string]string{},
		Archived:   map[string]bool{},
	}
	if err := habits.SaveHabitsMD(root, habitData.Habits, habitData.Logs); err != nil {
		t.Fatal(err)
	}

	// ── seed: prayer intentions ────────────────────────────────────
	intentions := []map[string]interface{}{
		{
			"id":         "p-day",
			"text":       "patience at work",
			"status":     "praying",
			"created_at": "2026-05-16T08:00:00Z",
			"updated_at": "2026-05-16T08:00:00Z",
		},
		{
			"id":         "p-prev",
			"text":       "old intention",
			"status":     "praying",
			"created_at": "2026-05-15T08:00:00Z",
			"updated_at": "2026-05-15T08:00:00Z",
		},
	}
	mustWriteJSON(t, root, ".granit/prayer/intentions.json", intentions)

	// ── seed: hub.json ─────────────────────────────────────────────
	hubFile := map[string]interface{}{
		"version": 1,
		"items": []map[string]interface{}{
			{"id": "h-day", "title": "Today's bookmark", "url": "https://x", "category": "tools", "created_at": "2026-05-16T11:00:00Z"},
			{"id": "h-prev", "title": "Old link", "url": "https://y", "created_at": "2026-05-15T11:00:00Z"},
		},
	}
	mustWriteJSON(t, root, ".granit/hub.json", hubFile)

	// ── vault scan + task store ────────────────────────────────────
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent {
		var out []tasks.NoteContent
		for _, n := range v.SnapshotNotes() {
			v.EnsureLoaded(n.RelPath)
			out = append(out, tasks.NoteContent{Path: n.RelPath, Content: n.Content})
		}
		return out
	})
	if err != nil {
		t.Fatal(err)
	}

	// Seed tasks through the public Create API + UpdateMeta to pin
	// CreatedAt / CompletedAt — reaching into the sidecar schema by
	// hand would couple the test to internal layout, and Create
	// gives us a real task line in a real note (matching how a user
	// would actually surface tasks).
	tWrite, err := store.Create("Write report", tasks.CreateOpts{File: "Notes/idea.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMeta(tWrite.ID, func(x *tasks.Task) {
		x.CreatedAt = time.Date(2026, 5, 16, 9, 0, 0, 0, loc)
		x.Project = "Granit"
		x.Priority = 2
	}); err != nil {
		t.Fatal(err)
	}
	tShip, err := store.Create("Ship feature", tasks.CreateOpts{File: "Notes/idea.md"})
	if err != nil {
		t.Fatal(err)
	}
	completed := time.Date(2026, 5, 16, 18, 0, 0, 0, loc)
	if err := store.UpdateMeta(tShip.ID, func(x *tasks.Task) {
		x.CreatedAt = time.Date(2026, 5, 10, 9, 0, 0, 0, loc)
		x.CompletedAt = &completed
	}); err != nil {
		t.Fatal(err)
	}
	tOld, err := store.Create("Old task", tasks.CreateOpts{File: "Notes/old.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMeta(tOld.ID, func(x *tasks.Task) {
		x.CreatedAt = time.Date(2026, 5, 15, 9, 0, 0, 0, loc)
	}); err != nil {
		t.Fatal(err)
	}

	// ── act ────────────────────────────────────────────────────────
	items := Collect(Query{Date: day, Loc: loc}, Sources{
		Vault: v, Tasks: store, VaultRoot: root, DailyFolder: "Daily",
	})

	// ── assert: kinds present ──────────────────────────────────────
	have := map[ItemKind]int{}
	titles := map[string]bool{}
	for _, it := range items {
		have[it.Kind]++
		titles[it.Title] = true
	}
	wantKinds := []ItemKind{
		KindEvent, KindTaskCreated, KindTaskCompleted, KindNoteCreated,
		KindHabit, KindPrayer, KindHubItem, KindJot,
	}
	for _, k := range wantKinds {
		if have[k] == 0 {
			t.Errorf("missing kind %q in output; got %+v", k, have)
		}
	}

	// Daily note itself must not appear in the Note list. (Its
	// path would be Daily/2026-05-16.md.)
	for _, it := range items {
		if it.Kind == KindNoteCreated && it.Path == "Daily/"+dayKey+".md" {
			t.Errorf("daily note leaked into note-created feed: %+v", it)
		}
	}

	// Items from the previous day must not appear at all.
	if titles["old intention"] || titles["Yesterday meeting"] || titles["Old task"] || titles["Old link"] {
		t.Errorf("previous-day item leaked into today's feed; titles=%v", titles)
	}

	// Two jots, two habit toggles → both surface.
	if have[KindJot] != 2 {
		t.Errorf("expected 2 jot entries, got %d", have[KindJot])
	}
	if have[KindHabit] != 2 {
		t.Errorf("expected 2 habit entries, got %d", have[KindHabit])
	}

	// Ordering check: timestamps must be monotonically non-decreasing.
	for i := 1; i < len(items); i++ {
		if items[i].At.Before(items[i-1].At) {
			t.Errorf("output not sorted ascending at %d: %v then %v", i, items[i-1].At, items[i].At)
		}
	}
}

// TestCollect_EmptyDay returns an empty (not nil) slice for a day
// with nothing on it — JSON consumers don't have to special-case
// null.
func TestCollect_EmptyDay(t *testing.T) {
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent { return nil })
	if err != nil {
		t.Fatal(err)
	}
	out := Collect(Query{Date: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), Loc: time.UTC}, Sources{
		Vault: v, Tasks: store, VaultRoot: root,
	})
	if out == nil {
		t.Fatal("got nil; expected empty slice")
	}
	if len(out) != 0 {
		t.Errorf("got %d items; expected 0; %+v", len(out), out)
	}
}

// TestCollect_RespectsMaxItems caps the slice at MaxItems even when
// the source data is bigger.
func TestCollect_RespectsMaxItems(t *testing.T) {
	root := t.TempDir()
	loc := time.UTC
	day := time.Date(2026, 5, 16, 0, 0, 0, 0, loc)
	dayKey := "2026-05-16"

	// 10 small notes on the day.
	for i := 0; i < 10; i++ {
		mustWrite(t, root, filepath.Join("Notes", "n"+itoa(i)+".md"),
			"---\ncreated: 2026-05-16T10:0"+itoa(i)+":00Z\n---\n\n# N"+itoa(i)+"\n")
	}
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent { return nil })
	if err != nil {
		t.Fatal(err)
	}
	full := Collect(Query{Date: day, Loc: loc}, Sources{Vault: v, Tasks: store, VaultRoot: root})
	if len(full) != 10 {
		t.Fatalf("seed sanity: got %d items, want 10", len(full))
	}
	capped := Collect(Query{Date: day, Loc: loc, MaxItems: 3}, Sources{Vault: v, Tasks: store, VaultRoot: root})
	if len(capped) != 3 {
		t.Errorf("MaxItems=3 not honoured: got %d items", len(capped))
	}
}

// TestNoteSummary covers the "first heading / first line" picker
// behind the Detail field — easy to regress when the parser logic
// drifts.
func TestNoteSummary(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"# Title\nbody", "Title"},
		{"## Sub\nbody", "Sub"},
		{"plain first line", "plain first line"},
		{"---\ntitle: X\n---\n\nbody", "body"},
		{"", ""},
	}
	for _, c := range cases {
		got := noteSummary(c.in)
		if got != c.want {
			t.Errorf("noteSummary(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func mustWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWriteJSON(t *testing.T, root, rel string, v interface{}) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
