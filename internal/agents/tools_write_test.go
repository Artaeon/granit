package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/objects"
)

// fakeWriter complements fakeVault by capturing writes to disk
// underneath t.TempDir. WriteNote and AppendTaskLine actually touch
// the filesystem so tests can verify the on-disk result.
type fakeWriter struct {
	root    string
	writes  []string // log of paths written to (for assertions)
	tasks   []string // log of task lines appended
	notes   *map[string]string
}

func (w *fakeWriter) WriteNote(rel, content string) (string, error) {
	abs := filepath.Join(w.root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	w.writes = append(w.writes, rel)
	if w.notes != nil {
		(*w.notes)[rel] = content
	}
	return abs, nil
}

func (w *fakeWriter) AppendTaskLine(line string) (string, error) {
	w.tasks = append(w.tasks, line)
	path := filepath.Join(w.root, "Tasks.md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return "", err
	}
	return path, nil
}

// write_note creates a new file when none exists at the target
// path. Verifies actual on-disk content because the LLM might
// supply markdown that needs to round-trip exactly.
func TestWriteNote_CreatesNew(t *testing.T) {
	v := newFakeVault(t, map[string]string{})
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := WriteNote(v, w)
	r := tool.Run(context.Background(), map[string]string{
		"path":    "Notes/foo.md",
		"content": "# Hello\nworld",
	})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	body, err := os.ReadFile(filepath.Join(v.root, "Notes/foo.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# Hello\nworld" {
		t.Errorf("on-disk content mismatch: %q", body)
	}
}

// write_note refuses to overwrite an existing note unless
// overwrite=true is set. The error message must mention overwrite
// so the LLM knows the recovery path.
func TestWriteNote_RefusesOverwrite(t *testing.T) {
	v := newFakeVault(t, map[string]string{"existing.md": "original"})
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := WriteNote(v, w)
	r := tool.Run(context.Background(), map[string]string{
		"path":    "existing.md",
		"content": "replacement",
	})
	if r.Err == nil {
		t.Fatal("expected refuse-to-overwrite error")
	}
	if !strings.Contains(r.Err.Error(), "overwrite=true") {
		t.Errorf("error should mention overwrite=true: %v", r.Err)
	}
	body, _ := os.ReadFile(filepath.Join(v.root, "existing.md"))
	if string(body) != "original" {
		t.Errorf("file should not have been changed: %q", body)
	}
}

// write_note honours overwrite=true.
func TestWriteNote_OverwriteWhenAsked(t *testing.T) {
	v := newFakeVault(t, map[string]string{"existing.md": "original"})
	// The note must actually exist on disk for the test — newFakeVault
	// only writes the in-memory map. Re-create it to disk.
	os.WriteFile(filepath.Join(v.root, "existing.md"), []byte("original"), 0o644)
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := WriteNote(v, w)
	r := tool.Run(context.Background(), map[string]string{
		"path":      "existing.md",
		"content":   "replacement",
		"overwrite": "true",
	})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	body, _ := os.ReadFile(filepath.Join(v.root, "existing.md"))
	if string(body) != "replacement" {
		t.Errorf("expected replacement, got %q", body)
	}
}

// write_note refuses paths that escape the vault root via `..` or
// absolute prefix.
func TestWriteNote_RefusesEscape(t *testing.T) {
	v := newFakeVault(t, nil)
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := WriteNote(v, w)
	for _, badPath := range []string{"../../etc/passwd", "/tmp/x.md"} {
		r := tool.Run(context.Background(), map[string]string{
			"path": badPath, "content": "x",
		})
		if r.Err == nil {
			t.Errorf("path %q should be refused", badPath)
		}
	}
}

// create_task assembles a "- [ ] ..." line from structured args
// and appends it via the writer. Verifies the line shape so
// downstream task parsers (granit's own Plan view) pick it up.
func TestCreateTask_BuildsLine(t *testing.T) {
	v := newFakeVault(t, nil)
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := CreateTask(w)
	r := tool.Run(context.Background(), map[string]string{
		"text":     "Buy groceries",
		"due":      "2026-04-30",
		"priority": "3",
		"tag":      "errands",
	})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if len(w.tasks) != 1 {
		t.Fatalf("tasks recorded: got %d, want 1", len(w.tasks))
	}
	got := w.tasks[0]
	for _, want := range []string{"- [ ] Buy groceries", "📅 2026-04-30", "⏫", "#errands"} {
		if !strings.Contains(got, want) {
			t.Errorf("task line missing %q: %q", want, got)
		}
	}
}

// create_task rejects empty text — the LLM occasionally produces
// "- [ ] " with no description; we want a clear error rather than
// a stub task on disk.
func TestCreateTask_EmptyTextErrors(t *testing.T) {
	v := newFakeVault(t, nil)
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := CreateTask(w)
	r := tool.Run(context.Background(), map[string]string{"text": "  "})
	if r.Err == nil {
		t.Error("empty text should error")
	}
}

// create_object assembles frontmatter from structured args and
// places the file under the type's declared folder.
func TestCreateObject_FullFlow(t *testing.T) {
	v := newFakeVault(t, nil)
	w := &fakeWriter{root: v.root, notes: &v.notes}
	v.reg = objects.NewRegistry()
	tool := CreateObject(v, w)

	r := tool.Run(context.Background(), map[string]string{
		"type":       "person",
		"title":      "Sebastian Becker",
		"properties": "email=s@example.com,role=Co-founder",
		"body":       "# Sebastian Becker\n\nNotes from our last call.",
	})
	if r.Err != nil {
		t.Fatal(r.Err)
	}

	// Path should land under the built-in person type's folder.
	wantPath := filepath.Join(v.root, "People", "Sebastian Becker.md")
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected file at %s: %v", wantPath, err)
	}
	for _, want := range []string{
		"type: person",
		"title: Sebastian Becker",
		"email: s@example.com",
		"role: Co-founder",
		"# Sebastian Becker",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body missing %q\n--- body ---\n%s", want, body)
		}
	}
}

// create_object rejects unknown types and the message points the
// LLM at query_objects so it can self-correct.
func TestCreateObject_RejectsUnknownType(t *testing.T) {
	v := newFakeVault(t, nil)
	v.reg = objects.NewRegistry()
	w := &fakeWriter{root: v.root, notes: &v.notes}
	tool := CreateObject(v, w)
	r := tool.Run(context.Background(), map[string]string{
		"type": "spaceship", "title": "USS Voyager",
	})
	if r.Err == nil {
		t.Fatal("expected unknown-type error")
	}
	if !strings.Contains(r.Err.Error(), "query_objects") {
		t.Errorf("error should hint at query_objects: %v", r.Err)
	}
}

// sanitiseFilename strips chars that break Windows / network
// filesystems but keeps spaces (which work everywhere modern).
func TestSanitiseFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Sebastian Becker", "Sebastian Becker"},
		{"a/b\\c:d", "abcd"},
		{"  ", "untitled"},
		{"file?.md", "file.md"},
	}
	for _, c := range cases {
		if got := sanitiseFilename(c.in); got != c.want {
			t.Errorf("sanitiseFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// yamlSingleLine quotes ONLY when necessary: emails / URLs / names
// pass through bare; values that would actually break YAML get
// quoted. Tightening this list avoided over-quoting common plain
// values like "s@example.com" which YAML happily accepts unquoted.
func TestYamlSingleLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"", `""`},
		{"with: colon", `"with: colon"`}, // ": " forces quoting (mapping pattern)
		{`has "quotes"`, `"has \"quotes\""`},
		{"s@example.com", "s@example.com"},  // @ is fine MID-value
		{"@startsWithAt", `"@startsWithAt"`}, // @ at position 0 needs quoting
		{"https://example.com", "https://example.com"},
		{"line1\nline2", `"line1\nline2"`}, // embedded newline
	}
	for _, c := range cases {
		got := yamlSingleLine(c.in)
		// Account for newline rendering in test output without
		// actually writing a literal \n: the wrapped string
		// contains a real newline char, so the test expectation
		// uses a backslash-n for clarity. Match by post-replace.
		want := strings.ReplaceAll(c.want, `\n`, "\n")
		if got != want {
			t.Errorf("yamlSingleLine(%q) = %q, want %q", c.in, got, want)
		}
	}
}
