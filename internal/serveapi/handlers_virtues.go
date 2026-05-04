// Package serveapi — handlers for /api/v1/virtues. Wraps the
// internal/virtues package with HTTP CRUD + a check-log endpoint.
//
// Surface mirrors /api/v1/goals: list / create / get / patch /
// delete on the Virtue, plus a dedicated POST .../checks for
// logging a weekly self-evaluation. Keeping the check endpoint
// separate from PATCH is a deliberate concurrency choice — two
// users (web + TUI, or two devices) saving a check at the same
// time should both land in the history without one overwriting
// the other; PATCH on the parent Virtue would need to merge
// arrays, which is fragile.
package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/virtues"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

const virtuesStatePath = ".granit/virtues.json"

func (s *Server) bcastVirtues() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: virtuesStatePath})
}

func (s *Server) handleListVirtues(w http.ResponseWriter, r *http.Request) {
	all := virtues.LoadAll(s.cfg.Vault.Root)
	if all == nil {
		all = []virtues.Virtue{}
	}
	// Sort each virtue's checks newest-first so the wire format
	// matches the order the UI wants. Cheap; saves the client
	// from re-sorting.
	for i := range all {
		virtues.SortChecksByWeek(all[i].Checks)
	}
	writeJSON(w, http.StatusOK, map[string]any{"virtues": all, "total": len(all)})
}

func (s *Server) handleGetVirtue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := virtues.LoadAll(s.cfg.Vault.Root)
	v, idx := virtues.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "virtue not found")
		return
	}
	virtues.SortChecksByWeek(v.Checks)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleCreateVirtue(w http.ResponseWriter, r *http.Request) {
	var v virtues.Virtue
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := v.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if v.ID == "" {
		v.ID = strings.ToLower(ulid.Make().String())
	}
	v.Status = virtues.NormalizeStatus(v.Status)
	now := time.Now().Format(time.RFC3339)
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if v.Checks == nil {
		v.Checks = []virtues.Check{}
	}

	all := virtues.LoadAll(s.cfg.Vault.Root)
	all = append(all, v)
	if err := virtues.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVirtues()
	writeJSON(w, http.StatusCreated, v)
}

// handlePatchVirtue applies a partial update. Checks is intentionally
// NOT in the patch list — the dedicated POST .../checks endpoint
// owns that path so a patch shape can't accidentally clobber the
// check history.
func (s *Server) handlePatchVirtue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all := virtues.LoadAll(s.cfg.Vault.Root)
	v, idx := virtues.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "virtue not found")
		return
	}
	apply := func(field string, into interface{}) error {
		raw, ok := patch[field]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(raw, into); err != nil {
			return fmt.Errorf("field %q: %w", field, err)
		}
		return nil
	}
	for _, step := range []func() error{
		func() error { return apply("name", &v.Name) },
		func() error { return apply("description", &v.Description) },
		func() error { return apply("anchor", &v.Anchor) },
		func() error { return apply("status", &v.Status) },
		func() error { return apply("season", &v.Season) },
		func() error { return apply("color", &v.Color) },
	} {
		if err := step(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := v.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	v.Status = virtues.NormalizeStatus(v.Status)
	v.UpdatedAt = time.Now().Format(time.RFC3339)
	all[idx] = v
	if err := virtues.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVirtues()
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteVirtue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := virtues.LoadAll(s.cfg.Vault.Root)
	out := make([]virtues.Virtue, 0, len(all))
	found := false
	for _, v := range all {
		if v.ID == id {
			found = true
			continue
		}
		out = append(out, v)
	}
	if !found {
		writeError(w, http.StatusNotFound, "virtue not found")
		return
	}
	if err := virtues.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVirtues()
	w.WriteHeader(http.StatusNoContent)
}

// virtueCheckBody is the wire shape for POST .../checks. WeekStart
// is optional — when omitted, the server canonicalises to the
// current week's Monday so a Sunday-evening review and a Monday-
// morning catch-up both land on the same anchor.
type virtueCheckBody struct {
	WeekStart string `json:"week_start"`
	Score     int    `json:"score"`
	Note      string `json:"note"`
}

// handleLogVirtueCheck records (or updates) a weekly check.
// Same-week submissions overwrite — the user is iterating on their
// own reflection, not appending duplicates. The PATCH-style
// "only-this-field-was-sent" semantics don't apply here because a
// check is intrinsically all-or-nothing.
func (s *Server) handleLogVirtueCheck(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b virtueCheckBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	week := strings.TrimSpace(b.WeekStart)
	if week == "" {
		week = virtues.MondayOf(time.Now())
	}
	check := virtues.Check{
		WeekStart: week,
		Score:     virtues.ClampScore(b.Score),
		Note:      strings.TrimSpace(b.Note),
		LoggedAt:  time.Now().Format(time.RFC3339),
	}
	if err := virtues.ValidateCheck(check); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	all := virtues.LoadAll(s.cfg.Vault.Root)
	v, idx := virtues.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "virtue not found")
		return
	}
	v.Checks = virtues.UpsertCheck(v.Checks, check)
	v.UpdatedAt = time.Now().Format(time.RFC3339)
	all[idx] = v
	if err := virtues.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVirtues()
	virtues.SortChecksByWeek(v.Checks)
	writeJSON(w, http.StatusOK, v)
}
