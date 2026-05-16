package serveapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/hub"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"

	"github.com/go-chi/chi/v5"
)

// hubToolsTestServer wires the minimum a hub-tools handler needs:
// a tempdir vault, a no-op tasks store, and a chi router with the
// five tools routes mounted so {id} path params resolve. Mirrors
// metaTestServer.
func hubToolsTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
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
	r.Get("/api/v1/hub/tools", s.handleListHubTools)
	r.Post("/api/v1/hub/tools", s.handleCreateHubTool)
	r.Post("/api/v1/hub/tools/reorder", s.handleReorderHubTools)
	r.Post("/api/v1/hub/tools/seed", s.handleSeedHubTools)
	r.Patch("/api/v1/hub/tools/{id}", s.handlePatchHubTool)
	r.Delete("/api/v1/hub/tools/{id}", s.handleDeleteHubTool)
	return s, r
}

// TestHubTools_CRUDRoundTrip exercises every entry point: list when
// empty, create with commands, list returns the created card, patch
// renames + replaces commands, delete drops the card. Pins the
// wire shape downstream code (web client + tests) relies on.
func TestHubTools_CRUDRoundTrip(t *testing.T) {
	_, h := hubToolsTestServer(t)

	// 1. List is empty (not 404) on a fresh vault.
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/tools", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list (empty): %d %s", w.Code, w.Body.String())
		}
		var got struct {
			Tools []hub.Tool `json:"tools"`
			Total int        `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Total != 0 || len(got.Tools) != 0 {
			t.Errorf("expected empty list, got total=%d tools=%v", got.Total, got.Tools)
		}
	}

	// 2. Create. Trailing whitespace is trimmed; an empty command row
	//    (label + command both blank) is dropped.
	body := `{
	  "name": "  neovim  ",
	  "description": "modal editor",
	  "icon": "📝",
	  "color": "green",
	  "tags": ["editor", " EDITOR ", ""],
	  "commands": [
	    {"label": "install via brew", "command": "brew install neovim"},
	    {"label": "", "command": ""},
	    {"label": "open config", "command": "nvim ~/.config/nvim/init.lua"}
	  ]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/tools", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created hub.Tool
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Error("created tool missing ID")
	}
	if created.Name != "neovim" {
		t.Errorf("name not trimmed: %q", created.Name)
	}
	if len(created.Commands) != 2 {
		t.Errorf("empty command row not dropped: %+v", created.Commands)
	}
	if len(created.Tags) != 1 || created.Tags[0] != "editor" {
		t.Errorf("tag normalisation failed: %+v", created.Tags)
	}
	if created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Error("timestamps not set on create")
	}

	// 3. Create without name → 400.
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/tools",
			bytes.NewBufferString(`{"description":"no name"}`))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("create without name: %d %s", w.Code, w.Body.String())
		}
	}

	// 4. List shows the created tool.
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/tools", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list: %d", w.Code)
		}
		var got struct {
			Tools []hub.Tool `json:"tools"`
			Total int        `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Total != 1 || got.Tools[0].ID != created.ID {
			t.Errorf("list after create: %+v", got)
		}
	}

	// 5. Patch: rename + replace commands. Unset fields stay.
	patch := `{
	  "name": "Neovim",
	  "commands": [
	    {"label": "install", "command": "brew install neovim"},
	    {"label": "config",  "command": "nvim ~/.config/nvim/init.lua", "notes": "lua-based"}
	  ]
	}`
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/hub/tools/"+created.ID, bytes.NewBufferString(patch))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	var patched hub.Tool
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Name != "Neovim" {
		t.Errorf("rename not applied: %q", patched.Name)
	}
	if patched.Description != "modal editor" {
		t.Errorf("unset field clobbered: description=%q", patched.Description)
	}
	if patched.Icon != "📝" {
		t.Errorf("unset icon clobbered: %q", patched.Icon)
	}
	if len(patched.Commands) != 2 || patched.Commands[1].Notes != "lua-based" {
		t.Errorf("commands not replaced: %+v", patched.Commands)
	}

	// 6. Patch on missing ID → 404.
	{
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/hub/tools/missing",
			bytes.NewBufferString(`{"name":"x"}`))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("patch missing: %d %s", w.Code, w.Body.String())
		}
	}

	// 7. Delete drops it; second delete is 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hub/tools/"+created.ID, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hub/tools/"+created.ID, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("second delete: %d", w.Code)
	}
}

// TestHubTools_Reorder pins the SortOrder rewrite: dragging a tool
// to a new position writes 1-based positions in array order; tools
// outside the supplied ID list keep their existing order. This is
// the same contract the link reorder honours, and the UI relies on
// it for partial-list drags.
func TestHubTools_Reorder(t *testing.T) {
	_, h := hubToolsTestServer(t)

	mkTool := func(name string) hub.Tool {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"name": name, "commands": []hub.Command{{Label: "x", Command: "echo x"}}})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/tools", bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, w.Code, w.Body.String())
		}
		var t1 hub.Tool
		_ = json.Unmarshal(w.Body.Bytes(), &t1)
		return t1
	}
	a := mkTool("alpha")
	b := mkTool("bravo")
	c := mkTool("charlie")

	// Reorder: [c, a, b] — the new front-to-back ordering.
	body, _ := json.Marshal(map[string]any{"ids": []string{c.ID, a.ID, b.ID}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/tools/reorder", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", w.Code, w.Body.String())
	}

	// List should come back in c / a / b order — LoadAllTools sorts
	// by SortOrder (1-based), so the freshly-rewritten positions
	// take precedence over the alpha fallback.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/hub/tools", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var got struct {
		Tools []hub.Tool `json:"tools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(got.Tools))
	}
	want := []string{c.ID, a.ID, b.ID}
	for i, id := range want {
		if got.Tools[i].ID != id {
			t.Errorf("position %d: got %q want %q", i, got.Tools[i].ID, id)
		}
	}
	// SortOrder is 1-based — the head must be 1, not 0, so an
	// unmoved tool appended later sorts at the end (not in the
	// middle where SortOrder=0 would land it).
	if got.Tools[0].SortOrder != 1 {
		t.Errorf("head SortOrder: got %d want 1", got.Tools[0].SortOrder)
	}

	// Reorder with empty IDs → 400 (no-op + bad request, not a 200
	// that silently does nothing).
	req = httptest.NewRequest(http.MethodPost, "/api/v1/hub/tools/reorder",
		bytes.NewBufferString(`{"ids":[]}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("reorder empty: %d", w.Code)
	}
}

// TestHubTools_SeedStarterSet pins the additive + idempotent
// semantics: a fresh seed adds N cards, a second seed adds 0 (the
// user can spam the button without ending up with duplicates), and
// hand-rolled cards with the same name as a starter are respected
// (no overwrite). The user keeps full ownership of the catalogue
// once it's seeded.
func TestHubTools_SeedStarterSet(t *testing.T) {
	_, h := hubToolsTestServer(t)

	seed := func() (added, total int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/tools/seed", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("seed: %d %s", w.Code, w.Body.String())
		}
		var got struct {
			Added int `json:"added"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.Added, got.Total
	}

	// 1. Fresh seed lands all starter cards.
	added, total := seed()
	starters := starterToolSet()
	if added != len(starters) || total != len(starters) {
		t.Errorf("first seed: added=%d total=%d, want both = %d", added, total, len(starters))
	}

	// 2. Second seed is a no-op — same names already present.
	added2, total2 := seed()
	if added2 != 0 || total2 != total {
		t.Errorf("second seed: added=%d total=%d, want added=0 total=%d", added2, total2, total)
	}

	// 3. Hand-rolled "git" with custom commands is NOT overwritten:
	//    delete + recreate one card, re-seed, verify the hand-rolled
	//    version still wins (additive seeding skips by name).
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/hub/tools", nil)
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, listReq)
	var listed struct {
		Tools []hub.Tool `json:"tools"`
	}
	_ = json.Unmarshal(lw.Body.Bytes(), &listed)
	var gitID string
	for _, t := range listed.Tools {
		if t.Name == "git" {
			gitID = t.ID
			break
		}
	}
	if gitID == "" {
		t.Fatal("starter set did not include a git card")
	}
	patchBody := `{"commands":[{"label":"my custom","command":"git log --oneline -1"}]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/hub/tools/"+gitID, bytes.NewBufferString(patchBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("custom patch: %d %s", w.Code, w.Body.String())
	}
	// Re-seed: should add nothing.
	added3, _ := seed()
	if added3 != 0 {
		t.Errorf("re-seed after custom edit added %d (should be 0)", added3)
	}
	// Confirm the custom command survived.
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/hub/tools", nil)
	lw2 := httptest.NewRecorder()
	h.ServeHTTP(lw2, listReq2)
	var listed2 struct {
		Tools []hub.Tool `json:"tools"`
	}
	_ = json.Unmarshal(lw2.Body.Bytes(), &listed2)
	for _, t := range listed2.Tools {
		if t.ID == gitID {
			if len(t.Commands) != 1 || t.Commands[0].Label != "my custom" {
				t.Errorf("custom git card mutated by re-seed: %+v", t.Commands)
			}
			return
		}
	}
	t.Errorf("custom git card vanished after re-seed")
}
