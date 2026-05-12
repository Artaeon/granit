package serveapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// stoiceraTestServer mounts only the integration routes + the
// management settings endpoints. Tests construct a vault with a few
// projects (some venture=Stoicera, some not) + a task store with
// tasks assigned to those projects, then exercise the endpoints.
func stoiceraTestServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent { return nil })
	if err != nil {
		t.Fatalf("task store: %v", err)
	}
	s := &Server{
		cfg: Config{Vault: v, TaskStore: store, Logger: slog.Default()},
		hub: wshub.New(slog.Default()),
	}
	r := chi.NewRouter()
	// Management endpoints — same auth as the rest of the app in
	// production, but we test handler logic directly (no
	// requireToken wrap) since auth is covered elsewhere.
	r.Get("/api/v1/stoicera-integration/settings", s.handleGetStoiceraSettings)
	r.Patch("/api/v1/stoicera-integration/settings", s.handlePatchStoiceraSettings)
	r.Get("/api/v1/stoicera-integration/token", s.handleGetStoiceraToken)
	// Data endpoints — wrap in requireStoiceraToken so disabled →
	// 404 / missing token → 401 behaviours are exercised.
	r.Group(func(r chi.Router) {
		r.Use(s.requireStoiceraToken)
		r.Get("/api/v1/integrations/stoicera/summary", s.handleStoiceraSummary)
		r.Get("/api/v1/integrations/stoicera/projects", s.handleStoiceraListProjects)
		r.Get("/api/v1/integrations/stoicera/projects/{name}", s.handleStoiceraGetProject)
		r.Get("/api/v1/integrations/stoicera/tasks", s.handleStoiceraListTasks)
		r.Get("/api/v1/integrations/stoicera/goals", s.handleStoiceraListGoals)
	})
	return s, r, root
}

func doStoiceraJSON(t *testing.T, h http.Handler, method, path, token string, body interface{}) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

// TestStoicera_DisabledByDefault confirms that data endpoints
// return 404 (not 401) when the feature is off — the existence of
// the integration should not leak via the response shape.
func TestStoicera_DisabledByDefault(t *testing.T) {
	_, h, _ := stoiceraTestServer(t)
	code, _ := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/summary", "anything", nil)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", code)
	}
}

// TestStoicera_EnableGeneratesToken: PATCHing enabled=true creates
// a token automatically so the user has something to copy.
func TestStoicera_EnableGeneratesToken(t *testing.T) {
	_, h, _ := stoiceraTestServer(t)
	code, body := doStoiceraJSON(t, h, "PATCH", "/api/v1/stoicera-integration/settings", "",
		map[string]interface{}{"enabled": true, "venture_name": "Stoicera"})
	if code != http.StatusOK {
		t.Fatalf("PATCH returned %d: %s", code, body)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(body, &resp)
	if resp["enabled"] != true {
		t.Errorf("expected enabled=true in response, got %v", resp["enabled"])
	}
	if resp["has_token"] != true {
		t.Errorf("expected has_token=true after enable, got %v", resp["has_token"])
	}
	if resp["venture_name"] != "Stoicera" {
		t.Errorf("expected venture_name=Stoicera, got %v", resp["venture_name"])
	}
	// Token endpoint should now return the unmasked value.
	code, body = doStoiceraJSON(t, h, "GET", "/api/v1/stoicera-integration/token", "", nil)
	if code != http.StatusOK {
		t.Fatalf("token endpoint returned %d", code)
	}
	var tokenResp map[string]string
	_ = json.Unmarshal(body, &tokenResp)
	if len(tokenResp["token"]) != 32 {
		t.Errorf("expected 32-char hex token, got %q (%d chars)",
			tokenResp["token"], len(tokenResp["token"]))
	}
}

// TestStoicera_AuthRequired: enabled but missing/wrong token → 401.
// (Disabled returns 404; this exercises the enabled-then-rejected path.)
func TestStoicera_AuthRequired(t *testing.T) {
	_, h, _ := stoiceraTestServer(t)
	_, _ = doStoiceraJSON(t, h, "PATCH", "/api/v1/stoicera-integration/settings", "",
		map[string]interface{}{"enabled": true, "venture_name": "Stoicera"})

	// Missing bearer header.
	code, _ := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/summary", "", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("missing token: expected 401, got %d", code)
	}
	// Wrong bearer value.
	code, _ = doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/summary", "wrong-token", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("wrong token: expected 401, got %d", code)
	}
}

// TestStoicera_ProjectsFilter: only projects with the matching
// Venture surface; case-insensitive match.
func TestStoicera_ProjectsFilter(t *testing.T) {
	_, h, root := stoiceraTestServer(t)
	seedProjects(t, root, []granitmeta.Project{
		{Name: "Alpha", Venture: "Stoicera", Status: "active"},
		{Name: "Beta", Venture: "stoicera", Status: "active"}, // lowercase
		{Name: "Gamma", Venture: "Other", Status: "active"},
		{Name: "Delta", Venture: "", Status: "active"}, // no venture
	})
	tok := enableAndGetToken(t, h, "Stoicera")

	code, body := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/projects", tok, nil)
	if code != http.StatusOK {
		t.Fatalf("projects returned %d: %s", code, body)
	}
	var resp struct {
		Items   []map[string]interface{} `json:"items"`
		Venture string                   `json:"venture"`
	}
	_ = json.Unmarshal(body, &resp)
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 projects (Alpha + Beta case-insensitive), got %d", len(resp.Items))
	}
	names := []string{}
	for _, p := range resp.Items {
		names = append(names, p["name"].(string))
	}
	if !stringSliceContains(names, "Alpha") || !stringSliceContains(names, "Beta") {
		t.Errorf("expected Alpha + Beta, got %v", names)
	}
}

// TestStoicera_SummaryAggregates: the summary endpoint counts only
// venture-tagged projects/tasks/goals.
func TestStoicera_SummaryAggregates(t *testing.T) {
	_, h, root := stoiceraTestServer(t)
	seedProjects(t, root, []granitmeta.Project{
		{Name: "Apollo", Venture: "Stoicera", Status: "active"},
		{Name: "Other", Venture: "Acme", Status: "active"},
	})
	tok := enableAndGetToken(t, h, "Stoicera")

	code, body := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/summary", tok, nil)
	if code != http.StatusOK {
		t.Fatalf("summary returned %d: %s", code, body)
	}
	var s map[string]int
	_ = json.Unmarshal(body, &s)
	if s["projects_active"] != 1 {
		t.Errorf("expected 1 active project for Stoicera venture, got %d", s["projects_active"])
	}
	if s["projects_total"] != 1 {
		t.Errorf("expected 1 total project, got %d", s["projects_total"])
	}
}

// TestStoicera_EmptyVentureClosesDoor: enabled but venture_name=""
// returns empty items rather than the full vault (failsafe against
// accidental over-share).
func TestStoicera_EmptyVentureClosesDoor(t *testing.T) {
	_, h, root := stoiceraTestServer(t)
	seedProjects(t, root, []granitmeta.Project{
		{Name: "Alpha", Venture: "Stoicera", Status: "active"},
	})
	// Enable but leave venture_name unset.
	_, _ = doStoiceraJSON(t, h, "PATCH", "/api/v1/stoicera-integration/settings", "",
		map[string]interface{}{"enabled": true})
	tokResp := getToken(t, h)

	code, body := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/projects", tokResp, nil)
	if code != http.StatusOK {
		t.Fatalf("projects returned %d", code)
	}
	var resp struct {
		Items   []map[string]interface{} `json:"items"`
		Venture string                   `json:"venture"`
	}
	_ = json.Unmarshal(body, &resp)
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items with empty venture, got %d", len(resp.Items))
	}
}

// TestStoicera_TasksFilterByProjectFolder: tasks living under a
// stoicera-tagged project's folder surface through /tasks; tasks
// in unrelated folders don't. Cross-reference uses the same
// projectMatches helper the /api/v1/projects endpoint uses, so the
// integration sees the same task → project mapping the UI does.
func TestStoicera_TasksFilterByProjectFolder(t *testing.T) {
	s, h, root := stoiceraTestServer(t)
	seedProjects(t, root, []granitmeta.Project{
		{Name: "Apollo", Venture: "Stoicera", Status: "active", Folder: "Projects/Apollo"},
		{Name: "Gemini", Venture: "Other", Status: "active", Folder: "Projects/Gemini"},
	})
	// Seed tasks via direct manipulation of the store's reload —
	// inject NoteContent fixtures that the parser will turn into Task
	// records. Each task line follows the standard `- [ ] text` shape.
	notes := []tasks.NoteContent{
		{Path: "Projects/Apollo/plan.md", Content: "# Plan\n- [ ] launch rocket\n- [x] book launchpad\n"},
		{Path: "Projects/Gemini/plan.md", Content: "# Plan\n- [ ] write spec\n"},
		{Path: "Inbox.md", Content: "- [ ] orphan task\n"},
	}
	// Replace the store's scan func so Reload pulls our fixtures.
	scanned := notes
	store, _ := tasks.Load(root, func() []tasks.NoteContent { return scanned })
	s.cfg.TaskStore = store

	tok := enableAndGetToken(t, h, "Stoicera")
	code, body := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/tasks", tok, nil)
	if code != http.StatusOK {
		t.Fatalf("tasks returned %d: %s", code, body)
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	_ = json.Unmarshal(body, &resp)
	// 2 tasks under Projects/Apollo/ (one done, one open). Both
	// should be surfaced — the Stoicera endpoint doesn't filter by
	// done status, that's the intranet's job.
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 Apollo tasks, got %d: %+v", len(resp.Items), resp.Items)
	}
	// All surfaced tasks must come from the Apollo folder; orphan +
	// Gemini tasks must be excluded.
	for _, it := range resp.Items {
		path := it["note_path"].(string)
		if !strings.HasPrefix(path, "Projects/Apollo/") {
			t.Errorf("task from wrong folder surfaced: %q", path)
		}
	}
}

// TestStoicera_RegenerateInvalidatesOldToken: PATCH regenerate=true
// replaces the token; old token no longer authenticates.
func TestStoicera_RegenerateInvalidatesOldToken(t *testing.T) {
	_, h, _ := stoiceraTestServer(t)
	tok1 := enableAndGetToken(t, h, "Stoicera")
	// Regenerate.
	_, _ = doStoiceraJSON(t, h, "PATCH", "/api/v1/stoicera-integration/settings", "",
		map[string]interface{}{"regenerate": true})
	tok2 := getToken(t, h)
	if tok1 == tok2 {
		t.Fatal("regenerate produced the same token")
	}
	// Old token now fails.
	code, _ := doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/summary", tok1, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected old token to be rejected (401), got %d", code)
	}
	// New token works.
	code, _ = doStoiceraJSON(t, h, "GET", "/api/v1/integrations/stoicera/summary", tok2, nil)
	if code != http.StatusOK {
		t.Errorf("expected new token to work (200), got %d", code)
	}
}

// ── helpers ────────────────────────────────────────────────────────

func seedProjects(t *testing.T, vaultRoot string, projects []granitmeta.Project) {
	t.Helper()
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(projects)
	if err := os.WriteFile(filepath.Join(dir, "projects.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func enableAndGetToken(t *testing.T, h http.Handler, venture string) string {
	t.Helper()
	_, _ = doStoiceraJSON(t, h, "PATCH", "/api/v1/stoicera-integration/settings", "",
		map[string]interface{}{"enabled": true, "venture_name": venture})
	return getToken(t, h)
}

func getToken(t *testing.T, h http.Handler) string {
	t.Helper()
	_, body := doStoiceraJSON(t, h, "GET", "/api/v1/stoicera-integration/token", "", nil)
	var tr map[string]string
	_ = json.Unmarshal(body, &tr)
	return tr["token"]
}

func stringSliceContains(s []string, v string) bool {
	for _, x := range s {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
