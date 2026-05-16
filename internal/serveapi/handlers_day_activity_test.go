package serveapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/dayactivity"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// TestDayActivity_Empty makes sure an out-of-range date returns
// 200 with an empty items list — NOT a 404 — so the SPA renders
// "nothing happened that day" cleanly. The handler defaults to
// today when date is omitted; an empty vault for that date also
// returns empty.
func TestDayActivity_Empty(t *testing.T) {
	_, h := dayActivityTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/day-activity?date=1999-01-01", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got dayActivityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Date != "1999-01-01" {
		t.Errorf("Date echo: got %q, want 1999-01-01", got.Date)
	}
	if got.Items == nil {
		t.Error("items is nil; expected empty slice")
	}
	if len(got.Items) != 0 {
		t.Errorf("expected 0 items for empty day, got %d", len(got.Items))
	}
}

// TestDayActivity_BadDate covers the parser's contract — non-ISO
// strings get a 400 with a human-readable error message. Pinning
// this so a future refactor of parseDailyParam can't silently
// degrade to a 500 on bad input.
func TestDayActivity_BadDate(t *testing.T) {
	_, h := dayActivityTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/day-activity?date=not-a-date", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d (want 400): %s", w.Code, w.Body.String())
	}
}

// TestDayActivity_TodayDefault — omitting `date` falls back to
// today (via parseDailyParam's "today" shortcut). We don't assert
// content because tests run on arbitrary days; we DO assert the
// response shape stays valid and Date is non-empty.
func TestDayActivity_TodayDefault(t *testing.T) {
	_, h := dayActivityTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/day-activity", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got dayActivityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Date == "" {
		t.Error("Date should be populated even when query omits ?date")
	}
}

// TestDayActivity_SurfacesNotesCreated walks an end-to-end path:
// drop a note with frontmatter `created` matching a known date,
// hit /api/v1/day-activity for that date, and confirm the note
// shows up in the response with the expected Kind + Path.
func TestDayActivity_SurfacesNotesCreated(t *testing.T) {
	root := t.TempDir()
	// Note created on 2026-05-16 per frontmatter — independent of
	// file mtime so the test stays deterministic across hosts.
	if err := os.MkdirAll(filepath.Join(root, "Notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ncreated: 2026-05-16T10:00:00Z\n---\n\n# Idea\n\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "Notes", "idea.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s, h := dayActivityTestServerAt(t, root)
	_ = s

	req := httptest.NewRequest(http.MethodGet, "/api/v1/day-activity?date=2026-05-16", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got dayActivityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range got.Items {
		if it.Kind == dayactivity.KindNoteCreated && it.Path == "Notes/idea.md" {
			found = true
			if it.Title != "idea" && it.Title != "Idea" {
				t.Errorf("unexpected title %q", it.Title)
			}
		}
	}
	if !found {
		t.Errorf("note not in day-activity response: %+v", got.Items)
	}
}

// ─── helpers ─────────────────────────────────────────────────────

func dayActivityTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	return dayActivityTestServerAt(t, t.TempDir())
}

func dayActivityTestServerAt(t *testing.T, root string) (*Server, http.Handler) {
	t.Helper()
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
	r.Get("/api/v1/day-activity", s.handleGetDayActivity)
	return s, r
}
