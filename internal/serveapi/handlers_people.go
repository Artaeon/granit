package serveapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/people"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const statePathPeople = ".granit/people.json"

func (s *Server) bcastPeople() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathPeople})
}

// List response carries the sorted people plus a derived
// "upcoming birthdays" list — it's cheap enough to compute on every
// list call and saves the web from a separate round trip on the
// page-load critical path.
func (s *Server) handleListPeople(w http.ResponseWriter, r *http.Request) {
	all := people.LoadAll(s.cfg.Vault.Root)
	today := time.Now()
	sorted := people.SortForDisplay(all, today)
	if sorted == nil {
		sorted = []people.Person{}
	}
	// Configurable window: ?birthdays_within=N (default 30, max 90).
	window := 30
	if v := r.URL.Query().Get("birthdays_within"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 90 {
			window = n
		}
	}
	bdays := people.UpcomingBirthdays(all, today, window)
	if bdays == nil {
		bdays = []people.Person{}
	}
	// Stale count is denormalised so the dashboard pill / nav badge
	// don't have to load the whole list.
	staleCount := 0
	for _, p := range all {
		if p.IsStale(today) {
			staleCount++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"people":              sorted,
		"total":               len(sorted),
		"stale_count":         staleCount,
		"upcoming_birthdays":  bdays,
	})
}

func (s *Server) handleCreatePerson(w http.ResponseWriter, r *http.Request) {
	var p people.Person
	if !readJSON(w, r, &p) {
		return
	}
	if strings.TrimSpace(p.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if p.ID == "" {
		p.ID = newULID()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	all := append(people.LoadAll(s.cfg.Vault.Root), p)
	if err := people.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPeople()
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePatchPerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := people.LoadAll(s.cfg.Vault.Root)
	idx := -1
	for i, p := range all {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "person not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	p := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("name", &p.Name)
	apply("email", &p.Email)
	apply("phone", &p.Phone)
	apply("birthday", &p.Birthday)
	apply("relationship", &p.Relationship)
	apply("tags", &p.Tags)
	apply("last_contacted_at", &p.LastContactedAt)
	apply("cadence_days", &p.CadenceDays)
	apply("note_path", &p.NotePath)
	apply("notes", &p.Notes)
	apply("archived", &p.Archived)
	p.UpdatedAt = time.Now().UTC()
	all[idx] = p
	if err := people.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPeople()
	writeJSON(w, http.StatusOK, p)
}

// "Ping" — convenience endpoint that stamps last_contacted_at to
// today on the current request. Saves the web from POSTing a custom
// PATCH body just for the most common touch action.
func (s *Server) handlePingPerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := people.LoadAll(s.cfg.Vault.Root)
	idx := -1
	for i, p := range all {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "person not found")
		return
	}
	p := all[idx]
	p.LastContactedAt = time.Now().Format("2006-01-02")
	p.UpdatedAt = time.Now().UTC()
	all[idx] = p
	if err := people.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPeople()
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := people.LoadAll(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, p := range all {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		writeError(w, http.StatusNotFound, "person not found")
		return
	}
	if err := people.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastPeople()
	w.WriteHeader(http.StatusNoContent)
}
