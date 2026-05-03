package serveapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/measurements"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const (
	statePathMeasurementsSeries  = ".granit/measurements/series.json"
	statePathMeasurementsEntries = ".granit/measurements/entries.json"
)

func (s *Server) bcastMeasurementsSeries() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathMeasurementsSeries})
}
func (s *Server) bcastMeasurementsEntries() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathMeasurementsEntries})
}

// ── Series ──────────────────────────────────────────────────────────

// List response carries series + a derived "latest entry per series"
// table so the index page can render current-value cards in one
// round trip. Avoids the N+1 fetch the web would otherwise have to
// do (one entries call per series card).
func (s *Server) handleListMeasurementSeries(w http.ResponseWriter, r *http.Request) {
	v := s.cfg.Vault.Root
	series := measurements.SortSeries(measurements.LoadSeries(v))
	if series == nil {
		series = []measurements.Series{}
	}
	entries := measurements.LoadEntries(v)
	latest := map[string]measurements.Entry{}
	for _, ser := range series {
		if e, ok := measurements.LatestForSeries(entries, ser.ID); ok {
			latest[ser.ID] = e
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"series":      series,
		"total":       len(series),
		"latest":      latest,
		"entry_count": len(entries),
	})
}

func (s *Server) handleCreateMeasurementSeries(w http.ResponseWriter, r *http.Request) {
	var ser measurements.Series
	if !readJSON(w, r, &ser) {
		return
	}
	if strings.TrimSpace(ser.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	ser.Direction = measurements.NormalizeDirection(ser.Direction)
	if ser.ID == "" {
		ser.ID = newULID()
	}
	now := time.Now().UTC()
	if ser.CreatedAt.IsZero() {
		ser.CreatedAt = now
	}
	ser.UpdatedAt = now
	all := append(measurements.LoadSeries(s.cfg.Vault.Root), ser)
	if err := measurements.SaveSeries(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastMeasurementsSeries()
	writeJSON(w, http.StatusCreated, ser)
}

func (s *Server) handlePatchMeasurementSeries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := measurements.LoadSeries(s.cfg.Vault.Root)
	idx := -1
	for i, x := range all {
		if x.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "series not found")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	x := all[idx]
	apply := func(k string, dst any) {
		if raw, ok := patch[k]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	apply("name", &x.Name)
	apply("unit", &x.Unit)
	apply("target", &x.Target)
	if raw, ok := patch["direction"]; ok {
		var d string
		_ = json.Unmarshal(raw, &d)
		x.Direction = measurements.NormalizeDirection(d)
	}
	apply("notes", &x.Notes)
	apply("archived", &x.Archived)
	x.UpdatedAt = time.Now().UTC()
	all[idx] = x
	if err := measurements.SaveSeries(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastMeasurementsSeries()
	writeJSON(w, http.StatusOK, x)
}

// Deleting a series cascades — the entries belonging to it become
// orphans otherwise. Doing the cascade server-side keeps the cleanup
// atomic relative to the WS broadcast.
func (s *Server) handleDeleteMeasurementSeries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v := s.cfg.Vault.Root
	allSer := measurements.LoadSeries(v)
	keepSer := allSer[:0]
	found := false
	for _, x := range allSer {
		if x.ID == id {
			found = true
			continue
		}
		keepSer = append(keepSer, x)
	}
	if !found {
		writeError(w, http.StatusNotFound, "series not found")
		return
	}
	allEntries := measurements.LoadEntries(v)
	keepEntries := allEntries[:0]
	for _, e := range allEntries {
		if e.SeriesID == id {
			continue
		}
		keepEntries = append(keepEntries, e)
	}
	if err := measurements.SaveSeries(v, keepSer); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := measurements.SaveEntries(v, keepEntries); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastMeasurementsSeries()
	s.bcastMeasurementsEntries()
	w.WriteHeader(http.StatusNoContent)
}

// ── Entries ─────────────────────────────────────────────────────────

// List entries — optionally filtered by ?series=<id> so the detail
// view doesn't have to filter client-side. The /measurements index
// uses the series list endpoint with the embedded latest map and
// doesn't need this; the detail page does.
func (s *Server) handleListMeasurementEntries(w http.ResponseWriter, r *http.Request) {
	all := measurements.LoadEntries(s.cfg.Vault.Root)
	if seriesID := r.URL.Query().Get("series"); seriesID != "" {
		all = measurements.EntriesForSeries(all, seriesID)
	}
	if all == nil {
		all = []measurements.Entry{}
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": all, "total": len(all)})
}

func (s *Server) handleCreateMeasurementEntry(w http.ResponseWriter, r *http.Request) {
	var e measurements.Entry
	if !readJSON(w, r, &e) {
		return
	}
	if e.SeriesID == "" || e.Date == "" {
		writeError(w, http.StatusBadRequest, "series_id + date required")
		return
	}
	if e.ID == "" {
		e.ID = newULID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	all := append(measurements.LoadEntries(s.cfg.Vault.Root), e)
	if err := measurements.SaveEntries(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastMeasurementsEntries()
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleDeleteMeasurementEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := measurements.LoadEntries(s.cfg.Vault.Root)
	out := all[:0]
	found := false
	for _, e := range all {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}
	if err := measurements.SaveEntries(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastMeasurementsEntries()
	w.WriteHeader(http.StatusNoContent)
}
