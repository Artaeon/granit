// Package serveapi — handlers for /api/v1/shopping.
//
// Surface: list / create / get / patch / delete on the Item, plus
// a /totals rollup for the /finance overview integration. The
// "re-plan a bought standard" workflow doesn't need its own endpoint
// — flipping the status back to planned via PATCH (and clearing
// BoughtAt) is a regular update; we just guarantee the patch handler
// honors that transition correctly.
package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/shopping"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

const shoppingStatePath = ".granit/shopping.json"

func (s *Server) bcastShopping() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: shoppingStatePath})
}

func (s *Server) handleListShopping(w http.ResponseWriter, r *http.Request) {
	all := shopping.LoadAll(s.cfg.Vault.Root)
	if all == nil {
		all = []shopping.Item{}
	}
	out := shopping.SortForDisplay(all)
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) handleGetShoppingItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := shopping.LoadAll(s.cfg.Vault.Root)
	it, idx := shopping.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "shopping item not found")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleCreateShoppingItem(w http.ResponseWriter, r *http.Request) {
	var it shopping.Item
	if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := it.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if it.ID == "" {
		it.ID = strings.ToLower(ulid.Make().String())
	}
	it.Status = shopping.NormalizeStatus(it.Status)
	it.Category = shopping.NormalizeCategory(it.Category)
	it.Cadence = shopping.NormalizeCadence(it.Cadence)
	now := time.Now().Format(time.RFC3339)
	if it.CreatedAt == "" {
		it.CreatedAt = now
	}
	it.UpdatedAt = now

	all := shopping.LoadAll(s.cfg.Vault.Root)
	all = append(all, it)
	if err := shopping.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastShopping()
	writeJSON(w, http.StatusCreated, it)
}

// handlePatchShoppingItem applies a partial update. Two transitions
// are special-cased:
//
//   - planned → bought: stamp BoughtAt with today's date (in the
//     server's local zone) so the /finance month rollup has a date
//     to filter on. Caller-supplied bought_at takes precedence so
//     a back-fill ("I bought this last Tuesday") still works.
//   - any → planned: clear BoughtAt so a re-plan doesn't keep a
//     stale "last bought" date attached to a fresh plan-cycle.
func (s *Server) handlePatchShoppingItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all := shopping.LoadAll(s.cfg.Vault.Root)
	it, idx := shopping.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "shopping item not found")
		return
	}
	prevStatus := it.Status

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
		func() error { return apply("name", &it.Name) },
		func() error { return apply("description", &it.Description) },
		func() error { return apply("url", &it.URL) },
		func() error { return apply("price", &it.Price) },
		func() error { return apply("quantity", &it.Quantity) },
		func() error { return apply("category", &it.Category) },
		func() error { return apply("status", &it.Status) },
		func() error { return apply("standard", &it.Standard) },
		func() error { return apply("cadence", &it.Cadence) },
		func() error { return apply("notes", &it.Notes) },
		func() error { return apply("bought_at", &it.BoughtAt) },
	} {
		if err := step(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := it.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	it.Status = shopping.NormalizeStatus(it.Status)
	it.Category = shopping.NormalizeCategory(it.Category)
	it.Cadence = shopping.NormalizeCadence(it.Cadence)

	// Lifecycle-driven BoughtAt management.
	if prevStatus != string(shopping.StatusBought) && it.Status == string(shopping.StatusBought) {
		// Caller-supplied bought_at wins (back-fill case); else stamp now.
		if _, supplied := patch["bought_at"]; !supplied || strings.TrimSpace(it.BoughtAt) == "" {
			it.BoughtAt = time.Now().Format("2006-01-02")
		}
	}
	if it.Status == string(shopping.StatusPlanned) {
		it.BoughtAt = ""
	}

	it.UpdatedAt = time.Now().Format(time.RFC3339)
	all[idx] = it
	if err := shopping.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastShopping()
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleDeleteShoppingItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := shopping.LoadAll(s.cfg.Vault.Root)
	out := make([]shopping.Item, 0, len(all))
	found := false
	for _, it := range all {
		if it.ID == id {
			found = true
			continue
		}
		out = append(out, it)
	}
	if !found {
		writeError(w, http.StatusNotFound, "shopping item not found")
		return
	}
	if err := shopping.SaveAll(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastShopping()
	w.WriteHeader(http.StatusNoContent)
}

// handleShoppingTotals returns the rollup that /finance overview
// reads to surface "you've planned €X and bought €Y this month".
// Centralised here so the finance page doesn't need to download the
// full item list just to compute totals.
func (s *Server) handleShoppingTotals(w http.ResponseWriter, r *http.Request) {
	all := shopping.LoadAll(s.cfg.Vault.Root)
	if all == nil {
		all = []shopping.Item{}
	}
	totals := shopping.AggregateTotals(all, time.Now())
	writeJSON(w, http.StatusOK, totals)
}
