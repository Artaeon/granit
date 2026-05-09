// Package serveapi — handlers for /api/v1/annotations.
//
// Surface: list-by-note / create / patch / delete. The store is
// flat across the vault; the list endpoint takes a `notePath`
// query param so the typical caller (the editor opening one note)
// fetches just what it needs without paying for unrelated rows.
//
// State path: .granit/annotations.json. Broadcasts state.changed
// on that path after every mutation so other open tabs of the
// same note refresh their margin column without manual reload.
package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/artaeon/granit/internal/annotations"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

const annotationsStatePath = ".granit/annotations.json"

func (s *Server) bcastAnnotations() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: annotationsStatePath})
}

func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	notePath := r.URL.Query().Get("notePath")
	if notePath == "" {
		// Listing the entire store is allowed for the future
		// "all annotations" surface; the editor passes notePath.
		store, err := annotations.LoadAll(s.cfg.Vault.Root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"annotations": store.Annotations,
			"total":       len(store.Annotations),
		})
		return
	}
	rows, err := annotations.ListForNote(s.cfg.Vault.Root, notePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []annotations.Annotation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"annotations": rows,
		"total":       len(rows),
	})
}

func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	var a annotations.Annotation
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := annotations.Add(s.cfg.Vault.Root, a)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.bcastAnnotations()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePatchAnnotation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Text       *string `json:"text,omitempty"`
		Color      *string `json:"color,omitempty"`
		LineNum    *int    `json:"lineNum,omitempty"`
		AnchorText *string `json:"anchorText,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := annotations.Patch(s.cfg.Vault.Root, id, func(a *annotations.Annotation) {
		if body.Text != nil {
			a.Text = *body.Text
		}
		if body.Color != nil {
			a.Color = *body.Color
		}
		if body.LineNum != nil {
			a.LineNum = *body.LineNum
		}
		if body.AnchorText != nil {
			a.AnchorText = *body.AnchorText
		}
	})
	if err != nil {
		if errors.Is(err, annotations.ErrNotFound) {
			writeError(w, http.StatusNotFound, "annotation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastAnnotations()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := annotations.Delete(s.cfg.Vault.Root, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastAnnotations()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
