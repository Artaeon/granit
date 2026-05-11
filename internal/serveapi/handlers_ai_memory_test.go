package serveapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/aimemory"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// aiMemoryTestServer wires the four AI memory routes to a fresh
// tmp-vault Server. The pure aimemory package has its own tests
// (race-safety, content cap, dedupe, etc.) — these tests pin the
// JSON-shape round-trip + the HTTP status codes the frontend
// expects.
func aiMemoryTestServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		cfg: Config{Vault: v, Logger: slog.Default()},
		hub: wshub.New(slog.Default()),
	}
	r := chi.NewRouter()
	r.Get("/api/v1/ai/memory", s.handleListAIMemory)
	r.Post("/api/v1/ai/memory", s.handleAddAIMemory)
	r.Patch("/api/v1/ai/memory/{id}", s.handlePatchAIMemory)
	r.Delete("/api/v1/ai/memory/{id}", s.handleDeleteAIMemory)
	return s, r, root
}

func memDoJSON(t *testing.T, h http.Handler, method, path string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

func TestAIMemory_AddListDelete_RoundTrip(t *testing.T) {
	_, h, _ := aiMemoryTestServer(t)

	// Empty list on a fresh vault — and crucially, "facts" must be
	// an empty array, NOT null, so the frontend can .map without
	// a null-check.
	code, body := memDoJSON(t, h, http.MethodGet, "/api/v1/ai/memory", nil)
	if code != http.StatusOK {
		t.Fatalf("list empty: status %d: %s", code, body)
	}
	var listed struct {
		Facts []aimemory.Fact `json:"facts"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Facts == nil {
		t.Errorf("Facts must be [], not null, on empty vault")
	}
	if listed.Total != 0 {
		t.Errorf("Total = %d, want 0", listed.Total)
	}

	// Add a fact.
	code, body = memDoJSON(t, h, http.MethodPost, "/api/v1/ai/memory", map[string]any{
		"content": "User's wife is Anna",
		"tags":    []string{"family"},
	})
	if code != http.StatusCreated {
		t.Fatalf("add: status %d: %s", code, body)
	}
	var added aimemory.Fact
	if err := json.Unmarshal(body, &added); err != nil {
		t.Fatal(err)
	}
	if added.ID == "" || added.Content != "User's wife is Anna" {
		t.Errorf("added fact malformed: %+v", added)
	}
	if len(added.Tags) != 1 || added.Tags[0] != "family" {
		t.Errorf("tags = %v, want [family]", added.Tags)
	}

	// List should now return one fact.
	code, body = memDoJSON(t, h, http.MethodGet, "/api/v1/ai/memory", nil)
	if code != http.StatusOK {
		t.Fatalf("list one: status %d", code)
	}
	_ = json.Unmarshal(body, &listed)
	if listed.Total != 1 || len(listed.Facts) != 1 {
		t.Errorf("expected 1 fact after add, got total=%d facts=%d", listed.Total, len(listed.Facts))
	}

	// Delete it (idempotent contract — second delete must also 204).
	code, _ = memDoJSON(t, h, http.MethodDelete, "/api/v1/ai/memory/"+added.ID, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", code)
	}
	code, _ = memDoJSON(t, h, http.MethodDelete, "/api/v1/ai/memory/"+added.ID, nil)
	if code != http.StatusNoContent {
		t.Errorf("idempotent delete: status %d, want 204 on the second call", code)
	}

	// And the list is empty again.
	code, body = memDoJSON(t, h, http.MethodGet, "/api/v1/ai/memory", nil)
	if code != http.StatusOK {
		t.Fatalf("list after delete: status %d", code)
	}
	_ = json.Unmarshal(body, &listed)
	if listed.Total != 0 {
		t.Errorf("expected 0 facts after delete, got %d", listed.Total)
	}
}

func TestAIMemory_Add_RejectsEmptyContent(t *testing.T) {
	// Empty content body must 400 with a clear message — the frontend
	// /remember slash command relies on this to surface a usage hint
	// rather than silently storing whitespace.
	_, h, _ := aiMemoryTestServer(t)
	for _, c := range []string{"", "   ", "\t\n"} {
		code, body := memDoJSON(t, h, http.MethodPost, "/api/v1/ai/memory", map[string]any{
			"content": c,
		})
		if code != http.StatusBadRequest {
			t.Errorf("content %q: status %d, want 400; body=%s", c, code, body)
		}
	}
}

func TestAIMemory_Add_RejectsMalformedJSON(t *testing.T) {
	_, h, _ := aiMemoryTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/memory", bytes.NewReader([]byte("{ not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON: status %d, want 400", rr.Code)
	}
}

func TestAIMemory_Patch_NotFound(t *testing.T) {
	// Patching a non-existent id must 404 (not 500) so the UI can
	// distinguish "this fact was already deleted by another tab"
	// from "the server is broken".
	_, h, _ := aiMemoryTestServer(t)
	code, _ := memDoJSON(t, h, http.MethodPatch, "/api/v1/ai/memory/nonexistent", map[string]any{
		"content": "x",
	})
	if code != http.StatusNotFound {
		t.Errorf("patch missing: status %d, want 404", code)
	}
}

func TestAIMemory_Patch_UpdatesContent(t *testing.T) {
	_, h, _ := aiMemoryTestServer(t)
	// Add → patch → list and assert the patched value shows.
	_, body := memDoJSON(t, h, http.MethodPost, "/api/v1/ai/memory", map[string]any{
		"content": "initial",
	})
	var added aimemory.Fact
	_ = json.Unmarshal(body, &added)
	code, body := memDoJSON(t, h, http.MethodPatch, "/api/v1/ai/memory/"+added.ID, map[string]any{
		"content": "updated",
		"tags":    []string{"new-tag"},
	})
	if code != http.StatusOK {
		t.Fatalf("patch: status %d: %s", code, body)
	}
	var patched aimemory.Fact
	_ = json.Unmarshal(body, &patched)
	if patched.Content != "updated" {
		t.Errorf("content didn't apply: %q", patched.Content)
	}
	if len(patched.Tags) != 1 || patched.Tags[0] != "new-tag" {
		t.Errorf("tags didn't apply: %v", patched.Tags)
	}
	if patched.UpdatedAt == added.UpdatedAt {
		t.Errorf("UpdatedAt must advance on patch")
	}
}

func TestAIMemory_DedupesOnAdd(t *testing.T) {
	// Two POSTs with byte-equal content must return the SAME id
	// (the existing fact), not create a duplicate. Mirrors the
	// "action chip clicked twice on a regen" scenario.
	_, h, _ := aiMemoryTestServer(t)
	_, body1 := memDoJSON(t, h, http.MethodPost, "/api/v1/ai/memory", map[string]any{
		"content": "duplicate me",
	})
	_, body2 := memDoJSON(t, h, http.MethodPost, "/api/v1/ai/memory", map[string]any{
		"content": "duplicate me",
	})
	var a, b aimemory.Fact
	_ = json.Unmarshal(body1, &a)
	_ = json.Unmarshal(body2, &b)
	if a.ID != b.ID {
		t.Errorf("duplicate Add returned different ids: %s vs %s", a.ID, b.ID)
	}
	// And only one fact exists.
	_, listBody := memDoJSON(t, h, http.MethodGet, "/api/v1/ai/memory", nil)
	var listed struct{ Total int }
	_ = json.Unmarshal(listBody, &listed)
	if listed.Total != 1 {
		t.Errorf("expected total=1 after dedupe, got %d", listed.Total)
	}
}
