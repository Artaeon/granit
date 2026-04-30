package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/objects"
)

// fakeVault implements VaultReader against a t.TempDir. Lets every
// read-tool test exercise real filesystem semantics (paths, walks,
// missing files) without depending on internal/vault.
type fakeVault struct {
	root  string
	notes map[string]string
	idx   *objects.Index
	reg   *objects.Registry
	tasks []TaskRecord
}

func (v *fakeVault) VaultRoot() string { return v.root }
func (v *fakeVault) NoteContent(rel string) (string, bool) {
	body, ok := v.notes[rel]
	return body, ok
}
func (v *fakeVault) SearchVault(query string, limit int) []SearchHit {
	var hits []SearchHit
	for path, body := range v.notes {
		if strings.Contains(strings.ToLower(body), strings.ToLower(query)) {
			snippet := body
			if len(snippet) > 80 {
				snippet = snippet[:80]
			}
			hits = append(hits, SearchHit{Path: path, Snippet: snippet})
			if len(hits) >= limit {
				break
			}
		}
	}
	return hits
}
func (v *fakeVault) ObjectIndex() *objects.Index       { return v.idx }
func (v *fakeVault) ObjectRegistry() *objects.Registry { return v.reg }
func (v *fakeVault) TaskList() []TaskRecord            { return v.tasks }

func newFakeVault(t *testing.T, files map[string]string) *fakeVault {
	t.Helper()
	if files == nil {
		files = map[string]string{}
	}
	root := t.TempDir()
	for relPath, body := range files {
		full := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &fakeVault{root: root, notes: files}
}

// read_note returns the body when the path resolves, a friendly
// not-found marker when it doesn't, and refuses absolute paths or
// `..` escapes (defence in depth alongside VaultReader's own checks).
func TestReadNote(t *testing.T) {
	v := newFakeVault(t, map[string]string{
		"alpha.md": "# Alpha\nbody here.\n",
	})
	tool := ReadNote(v)

	r := tool.Run(context.Background(), map[string]string{"path": "alpha.md"})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if !strings.Contains(r.Output, "body here") {
		t.Errorf("output missing body: %q", r.Output)
	}

	r = tool.Run(context.Background(), map[string]string{"path": "missing.md"})
	if r.Err != nil {
		t.Errorf("missing note should be soft-fail (output marker), got err: %v", r.Err)
	}
	if !strings.Contains(r.Output, "no note") {
		t.Errorf("expected 'no note' marker, got %q", r.Output)
	}

	r = tool.Run(context.Background(), map[string]string{"path": "../escape.md"})
	if r.Err == nil {
		t.Error("escape path should error")
	}
}

// read_note truncates large bodies to keep LLM context bounded, and
// the truncation footer tells the LLM how to fetch more.
func TestReadNote_Truncates(t *testing.T) {
	big := strings.Repeat("x", 8000)
	v := newFakeVault(t, map[string]string{"big.md": big})
	tool := ReadNote(v)
	r := tool.Run(context.Background(), map[string]string{"path": "big.md"})
	if !strings.Contains(r.Output, "[truncated") {
		t.Error("expected truncation footer")
	}
	if !strings.Contains(r.Output, "max_chars") {
		t.Error("truncation footer should hint at max_chars override")
	}
}

// list_notes walks a folder, sorts results, and respects the limit
// arg. Skips .git / .granit / .obsidian dirs that aren't user content.
func TestListNotes(t *testing.T) {
	v := newFakeVault(t, map[string]string{
		"People/Alice.md":            "alice",
		"People/Bob.md":               "bob",
		"Books/AtomicHabits.md":       "book",
		".granit/types/person.json":   "{}",
		".obsidian/workspace.json":    "{}",
	})
	tool := ListNotes(v)
	r := tool.Run(context.Background(), map[string]string{"folder": "People"})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if !strings.Contains(r.Output, "Alice.md") || !strings.Contains(r.Output, "Bob.md") {
		t.Errorf("expected People notes: %q", r.Output)
	}
	if strings.Contains(r.Output, "AtomicHabits") {
		t.Error("Books/ should not appear when folder=People")
	}
	if strings.Contains(r.Output, ".granit") {
		t.Error(".granit should be skipped")
	}

	// limit honoured.
	r = tool.Run(context.Background(), map[string]string{"limit": "1"})
	lines := strings.Split(strings.TrimSpace(r.Output), "\n")
	// 1 result + truncation footer (which itself contains \n\n).
	hasNote := false
	for _, l := range lines {
		if strings.HasSuffix(l, ".md") {
			hasNote = true
		}
	}
	if !hasNote {
		t.Errorf("expected at least one note line: %q", r.Output)
	}
	if !strings.Contains(r.Output, "truncated") {
		t.Error("limit overflow should add truncation footer")
	}
}

// search_vault calls through to VaultReader's search and renders hits
// with snippets. Empty query is rejected (the LLM might forget the
// arg; surface as Err so the runtime tells it to retry).
func TestSearchVault(t *testing.T) {
	v := newFakeVault(t, map[string]string{
		"a.md": "hello world",
		"b.md": "another note",
	})
	tool := SearchVault(v)

	r := tool.Run(context.Background(), map[string]string{"query": ""})
	if r.Err == nil {
		t.Error("empty query should error")
	}
	r = tool.Run(context.Background(), map[string]string{"query": "hello"})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if !strings.Contains(r.Output, "a.md") {
		t.Errorf("expected a.md in hits: %q", r.Output)
	}
	r = tool.Run(context.Background(), map[string]string{"query": "nope"})
	if !strings.Contains(r.Output, "no matches") {
		t.Errorf("expected no-match marker: %q", r.Output)
	}
}

// query_objects filters by typeID and where clauses. Built against
// the real objects.Builder from Phase 1.
func TestQueryObjects(t *testing.T) {
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("People/Alice.md", "Alice", map[string]string{
		"type": "person", "city": "Vienna", "role": "Engineer",
	})
	b.Add("People/Bob.md", "Bob", map[string]string{
		"type": "person", "city": "Berlin", "role": "Designer",
	})
	b.Add("Books/AH.md", "Atomic Habits", map[string]string{
		"type": "book", "rating": "5",
	})
	idx := b.Finalize()

	v := newFakeVault(t, nil)
	v.idx = idx
	v.reg = reg

	tool := QueryObjects(v)

	// Type filter only.
	r := tool.Run(context.Background(), map[string]string{"type": "person"})
	if !strings.Contains(r.Output, "Alice") || !strings.Contains(r.Output, "Bob") {
		t.Errorf("expected both people: %q", r.Output)
	}
	if strings.Contains(r.Output, "Atomic Habits") {
		t.Errorf("book should not appear in person query: %q", r.Output)
	}

	// where filter.
	r = tool.Run(context.Background(), map[string]string{"type": "person", "where": "city=Vienna"})
	if !strings.Contains(r.Output, "Alice") || strings.Contains(r.Output, "Bob") {
		t.Errorf("city=Vienna should match Alice only: %q", r.Output)
	}

	// No type → search all.
	r = tool.Run(context.Background(), map[string]string{"type": ""})
	if !strings.Contains(r.Output, "Atomic Habits") || !strings.Contains(r.Output, "Alice") {
		t.Errorf("no-type query should span all types: %q", r.Output)
	}
}

// query_tasks filters by status, due window, priority. Multiple
// orthogonal filters compose AND-style.
func TestQueryTasks(t *testing.T) {
	v := newFakeVault(t, nil)
	v.tasks = []TaskRecord{
		{Text: "Open today", DueDate: "2026-04-29", Priority: 3},
		{Text: "Open overdue", DueDate: "2026-04-01", Priority: 4},
		{Text: "Open upcoming", DueDate: "2026-05-01", Priority: 1},
		{Text: "Done task", Done: true, Priority: 4},
		{Text: "Open future", DueDate: "2027-01-01", Priority: 4},
	}

	tool := QueryTasks(v)

	r := tool.Run(context.Background(), map[string]string{"status": "open"})
	for _, want := range []string{"Open today", "Open overdue", "Open upcoming"} {
		if !strings.Contains(r.Output, want) {
			t.Errorf("status=open missing %q: %q", want, r.Output)
		}
	}
	if strings.Contains(r.Output, "Done task") {
		t.Error("status=open included a done task")
	}

	r = tool.Run(context.Background(), map[string]string{"status": "done"})
	if !strings.Contains(r.Output, "Done task") || strings.Contains(r.Output, "Open today") {
		t.Errorf("status=done filter wrong: %q", r.Output)
	}

	r = tool.Run(context.Background(), map[string]string{"min_priority": "3"})
	if strings.Contains(r.Output, "Open upcoming") {
		t.Error("min_priority=3 should exclude priority 1")
	}
}

// get_today returns today's date in ISO form. Cheap but load-bearing
// — without it the LLM invents dates on every date-filtered call.
func TestGetToday(t *testing.T) {
	r := GetToday().Run(context.Background(), nil)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	// YYYY-MM-DD = 10 chars, two dashes at fixed positions.
	if len(r.Output) != 10 || r.Output[4] != '-' || r.Output[7] != '-' {
		t.Errorf("unexpected date format: %q", r.Output)
	}
}

// pathInsideVault rejects ../, absolute paths, and explicitly accepts
// relative paths inside the root. Confirms the defence-in-depth
// layer.
func TestPathInsideVault(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		rel  string
		want bool
	}{
		{"alpha.md", true},
		{"sub/beta.md", true},
		{"", true},
		{"../escape.md", false},
		{"/absolute.md", false},
	}
	for _, c := range cases {
		if got := pathInsideVault(root, c.rel); got != c.want {
			t.Errorf("pathInsideVault(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

// parseWhereClause handles whitespace, multiple filters, and
// invalid entries (no `=`).
func TestParseWhereClause(t *testing.T) {
	out := parseWhereClause("city=Vienna, status=read , garbage,empty=")
	if out["city"] != "Vienna" {
		t.Errorf("city: %q", out["city"])
	}
	if out["status"] != "read" {
		t.Errorf("status: %q", out["status"])
	}
	if _, has := out["garbage"]; has {
		t.Errorf("'garbage' (no =) should be skipped")
	}
	if v, has := out["empty"]; !has || v != "" {
		t.Errorf("empty value should still register: %q (%v)", v, has)
	}
}
