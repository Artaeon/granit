package serveapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

// goalsTestServer mounts only the /api/v1/goals routes — keeping the
// fixture small (we don't need the auth / daily / task stack for a goals
// CRUD round-trip). Mirrors the route registrations in server.go.
func goalsTestServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	s := &Server{
		cfg: Config{Vault: v, Logger: slog.Default()},
		hub: wshub.New(slog.Default()),
	}
	r := chi.NewRouter()
	r.Get("/api/v1/goals", s.handleListGoals)
	r.Post("/api/v1/goals", s.handleCreateGoal)
	r.Patch("/api/v1/goals/{id}", s.handlePatchGoal)
	r.Delete("/api/v1/goals/{id}", s.handleDeleteGoal)
	r.Post("/api/v1/goals/{id}/milestones", s.handleAddMilestone)
	r.Patch("/api/v1/goals/{id}/milestones/{idx}", s.handlePatchMilestone)
	r.Delete("/api/v1/goals/{id}/milestones/{idx}", s.handleDeleteMilestone)
	r.Post("/api/v1/goals/{id}/review", s.handleLogReview)
	return s, r, root
}

func doJSON(t *testing.T, h http.Handler, method, path string, body interface{}) (int, []byte) {
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
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

// TestGoalsCRUD_RoundTrip exercises the full surface: create → patch
// → add milestone → toggle milestone → delete milestone → log review
// → delete goal. Each step asserts on the persisted file via LoadAll
// so we catch silent drops, not just response bodies.
func TestGoalsCRUD_RoundTrip(t *testing.T) {
	_, h, root := goalsTestServer(t)

	// 1. CREATE
	code, body := doJSON(t, h, http.MethodPost, "/api/v1/goals", map[string]interface{}{
		"title":            "Ship parity audit",
		"description":      "wire goals CRUD",
		"target_date":      "2026-12-31",
		"category":         "engineering",
		"tags":             []string{"granit", "web"},
		"review_frequency": "weekly",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", code, body)
	}
	var created goals.Goal
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Status != goals.StatusActive || created.CreatedAt == "" {
		t.Errorf("create returned bad goal: %+v", created)
	}
	if got := goals.LoadAll(root); len(got) != 1 || got[0].Title != "Ship parity audit" {
		t.Fatalf("after create, on-disk: %+v", got)
	}

	id := created.ID

	// 2. PATCH — change a few fields, including a malformed-tags rejection
	code, body = doJSON(t, h, http.MethodPatch, "/api/v1/goals/"+id, map[string]interface{}{
		"description": "updated copy",
		"tags":        []string{"granit", "web", "ship"},
		"notes":       "important context",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: status=%d body=%s", code, body)
	}
	var patched goals.Goal
	if err := json.Unmarshal(body, &patched); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if patched.Description != "updated copy" || patched.Notes != "important context" {
		t.Errorf("patch dropped fields: %+v", patched)
	}
	if len(patched.Tags) != 3 {
		t.Errorf("patch tags: %v", patched.Tags)
	}
	// On-disk Notes must round-trip — this is the granitmeta truncation
	// regression we are guarding against.
	if got := goals.LoadAll(root); got[0].Notes != "important context" {
		t.Errorf("notes lost on round-trip: %+v", got[0])
	}

	// 2a. PATCH with malformed shape rejected with 400
	code, body = doJSON(t, h, http.MethodPatch, "/api/v1/goals/"+id, map[string]interface{}{
		"tags": "should-be-array",
	})
	if code != http.StatusBadRequest {
		t.Errorf("malformed patch should 400, got status=%d body=%s", code, body)
	}

	// 3. ADD MILESTONE
	code, body = doJSON(t, h, http.MethodPost, "/api/v1/goals/"+id+"/milestones", map[string]interface{}{
		"text":     "First step",
		"due_date": "2026-06-01",
	})
	if code != http.StatusCreated {
		t.Fatalf("add milestone: status=%d body=%s", code, body)
	}
	if got := goals.LoadAll(root); len(got) != 1 || len(got[0].Milestones) != 1 {
		t.Fatalf("milestone not persisted: %+v", got)
	}

	// 4. PATCH MILESTONE — toggle done
	doneTrue := true
	code, body = doJSON(t, h, http.MethodPatch, "/api/v1/goals/"+id+"/milestones/0", map[string]interface{}{
		"done": doneTrue,
	})
	if code != http.StatusOK {
		t.Fatalf("patch milestone: status=%d body=%s", code, body)
	}
	if got := goals.LoadAll(root); !got[0].Milestones[0].Done {
		t.Errorf("milestone not marked done: %+v", got[0].Milestones[0])
	} else if got[0].Milestones[0].CompletedAt == "" {
		t.Errorf("completed_at not stamped on done flip: %+v", got[0].Milestones[0])
	}

	// 5. LOG REVIEW
	code, body = doJSON(t, h, http.MethodPost, "/api/v1/goals/"+id+"/review", map[string]interface{}{
		"note": "Halfway through milestones, on track.",
	})
	if code != http.StatusOK {
		t.Fatalf("log review: status=%d body=%s", code, body)
	}
	if got := goals.LoadAll(root); len(got[0].ReviewLog) != 1 || got[0].LastReviewed == "" {
		t.Errorf("review log not persisted: %+v", got[0])
	}

	// 6. DELETE MILESTONE
	code, body = doJSON(t, h, http.MethodDelete, "/api/v1/goals/"+id+"/milestones/0", nil)
	if code != http.StatusOK {
		t.Fatalf("delete milestone: status=%d body=%s", code, body)
	}
	if got := goals.LoadAll(root); len(got[0].Milestones) != 0 {
		t.Errorf("milestone not removed: %+v", got[0].Milestones)
	}

	// 7. DELETE GOAL
	code, body = doJSON(t, h, http.MethodDelete, "/api/v1/goals/"+id, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete goal: status=%d body=%s", code, body)
	}
	if got := goals.LoadAll(root); len(got) != 0 {
		t.Errorf("goal not removed from disk: %+v", got)
	}

	// 7a. DELETE again is 404
	code, _ = doJSON(t, h, http.MethodDelete, "/api/v1/goals/"+id, nil)
	if code != http.StatusNotFound {
		t.Errorf("repeat delete: status=%d, want 404", code)
	}
}

// TestCreateGoal_TitleRequired catches the empty-title rejection before
// it lands a half-built goal in the file.
func TestCreateGoal_TitleRequired(t *testing.T) {
	_, h, _ := goalsTestServer(t)
	code, _ := doJSON(t, h, http.MethodPost, "/api/v1/goals", map[string]interface{}{"title": ""})
	if code != http.StatusBadRequest {
		t.Errorf("empty title: status=%d, want 400", code)
	}
}

// TestPatchGoal_StatusToCompletedStampsCompletedAt verifies the implicit
// timestamp when the status transitions — and that flipping back to
// active wipes it.
func TestPatchGoal_StatusToCompletedStampsCompletedAt(t *testing.T) {
	_, h, _ := goalsTestServer(t)
	code, body := doJSON(t, h, http.MethodPost, "/api/v1/goals", map[string]interface{}{
		"title": "Test",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	var g goals.Goal
	_ = json.Unmarshal(body, &g)

	code, body = doJSON(t, h, http.MethodPatch, "/api/v1/goals/"+g.ID, map[string]interface{}{
		"status": "completed",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, body)
	}
	var done goals.Goal
	_ = json.Unmarshal(body, &done)
	if done.Status != goals.StatusCompleted || done.CompletedAt == "" {
		t.Errorf("status=completed didn't stamp completed_at: %+v", done)
	}

	code, body = doJSON(t, h, http.MethodPatch, "/api/v1/goals/"+g.ID, map[string]interface{}{
		"status": "active",
	})
	if code != http.StatusOK {
		t.Fatalf("revert: %d %s", code, body)
	}
	var back goals.Goal
	_ = json.Unmarshal(body, &back)
	if back.CompletedAt != "" {
		t.Errorf("status=active should clear completed_at, got %q", back.CompletedAt)
	}
}
