package serveapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"

	"github.com/go-chi/chi/v5"
)

// habitsCRUDTestServer builds a vault containing three daily notes
// with overlapping habits, mounts the habits routes, and returns the
// router + vault root so the test can re-read files after the handler
// rewrites them. Mirrors the metaTestServer shape.
func habitsCRUDTestServer(t *testing.T, dailies map[string]string) (http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	for name, body := range dailies {
		abs := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent {
		out := make([]tasks.NoteContent, 0)
		for _, n := range v.SnapshotNotes() {
			v.EnsureLoaded(n.RelPath)
			out = append(out, tasks.NoteContent{Path: n.RelPath, Content: n.Content})
		}
		return out
	})
	if err != nil {
		t.Fatalf("tasks: %v", err)
	}
	logger := slog.Default()
	s := &Server{
		cfg: Config{
			Vault:     v,
			TaskStore: store,
			Daily:     daily.DailyConfig{Template: daily.DefaultConfig().Template},
			Logger:    logger,
		},
		hub: wshub.New(logger),
	}
	r := chi.NewRouter()
	r.Get("/api/v1/habits", s.handleListHabits)
	r.Post("/api/v1/habits/toggle", s.handleToggleHabit)
	r.Delete("/api/v1/habits/{name}", s.handleDeleteHabit)
	r.Patch("/api/v1/habits/{name}", s.handleRenameHabit)
	return r, root
}

// TestHabits_Delete_RemovesLinesAcrossDailies pins the contract: a
// DELETE removes every matching checkbox line from every daily note
// in the vault, leaves the ## Habits heading + sibling habits intact,
// and reports an accurate file count.
func TestHabits_Delete_RemovesLinesAcrossDailies(t *testing.T) {
	dailies := map[string]string{
		"2026-05-01.md": "---\ntype: daily\n---\n\n## Habits\n\n- [x] gym\n- [ ] read 20 pages\n",
		"2026-05-02.md": "---\ntype: daily\n---\n\n## Habits\n\n- [x] gym !1\n- [x] read 20 pages\n",
		"2026-05-03.md": "---\ntype: daily\n---\n\n## Habits\n\n- [ ] read 20 pages\n",
		"2026-05-04.md": "---\ntype: daily\n---\n\n## Tasks\n\n- [ ] gym\n\n## Habits\n\n- [x] meditate\n",
	}
	h, root := habitsCRUDTestServer(t, dailies)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/habits/gym", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Name         string `json:"name"`
		FilesTouched int    `json:"filesTouched"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.FilesTouched != 2 {
		t.Errorf("filesTouched=%d, want 2 (gym appears in 05-01 and 05-02 ## Habits)", resp.FilesTouched)
	}

	// 05-01: gym line gone, read-20-pages intact, heading intact.
	b1 := readFile(t, root, "2026-05-01.md")
	if strings.Contains(b1, "[x] gym") {
		t.Errorf("05-01 still contains 'gym' checkbox: %s", b1)
	}
	if !strings.Contains(b1, "[ ] read 20 pages") {
		t.Errorf("05-01 lost sibling habit: %s", b1)
	}
	if !strings.Contains(b1, "## Habits") {
		t.Errorf("05-01 lost Habits heading: %s", b1)
	}
	// 05-02: gym line gone (including its `!1` marker).
	b2 := readFile(t, root, "2026-05-02.md")
	if strings.Contains(b2, "gym") {
		t.Errorf("05-02 still mentions 'gym': %s", b2)
	}
	// 05-03: never had gym — unchanged.
	if got, want := readFile(t, root, "2026-05-03.md"), dailies["2026-05-03.md"]; got != want {
		t.Errorf("05-03 mutated even though it had no gym line:\n got=%q\nwant=%q", got, want)
	}
	// 05-04: the gym line is under ## Tasks, NOT ## Habits — must
	// remain. Destructive ops only touch checkbox lines under the
	// Habits section.
	b4 := readFile(t, root, "2026-05-04.md")
	if !strings.Contains(b4, "[ ] gym") {
		t.Errorf("05-04 lost a Tasks-section 'gym' line that should be untouched: %s", b4)
	}
}

// TestHabits_Rename_RewritesLines pins the rename contract: visible
// text changes; checkbox state, per-line markers (!1 / #tag), and
// surrounding lines stay intact.
func TestHabits_Rename_RewritesLines(t *testing.T) {
	dailies := map[string]string{
		"2026-05-01.md": "---\ntype: daily\n---\n\n## Habits\n\n- [x] run\n- [ ] read 20 pages\n",
		"2026-05-02.md": "---\ntype: daily\n---\n\n## Habits\n\n- [x] run !1 #cardio\n",
	}
	h, root := habitsCRUDTestServer(t, dailies)

	body := bytes.NewBufferString(`{"new_name":"running"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/habits/run", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}

	b1 := readFile(t, root, "2026-05-01.md")
	if !strings.Contains(b1, "[x] running") {
		t.Errorf("05-01 didn't rename / preserve checkbox state: %s", b1)
	}
	if strings.Contains(b1, "[x] run\n") || strings.Contains(b1, "[x] run\r\n") {
		t.Errorf("05-01 still contains old 'run' line: %s", b1)
	}
	if !strings.Contains(b1, "[ ] read 20 pages") {
		t.Errorf("05-01 mangled sibling habit: %s", b1)
	}

	b2 := readFile(t, root, "2026-05-02.md")
	if !strings.Contains(b2, "[x] running !1 #cardio") {
		t.Errorf("05-02 didn't preserve markers: %s", b2)
	}
}

// TestHabits_Rename_RejectsEmpty pins the validation contract.
func TestHabits_Rename_RejectsEmpty(t *testing.T) {
	h, _ := habitsCRUDTestServer(t, map[string]string{
		"2026-05-01.md": "---\ntype: daily\n---\n\n## Habits\n\n- [x] run\n",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/habits/run",
		bytes.NewBufferString(`{"new_name":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty new_name, got %d", w.Code)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
