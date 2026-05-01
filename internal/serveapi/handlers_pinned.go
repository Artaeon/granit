package serveapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const pinnedFileRel = ".granit/sidebar-pinned.json"

type pinView struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

func (s *Server) readPinned() (map[string]bool, error) {
	path := filepath.Join(s.cfg.Vault.Root, pinnedFileRel)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	var m map[string]bool
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]bool{}
	}
	return m, nil
}

func (s *Server) writePinned(m map[string]bool) error {
	dir := filepath.Join(s.cfg.Vault.Root, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.cfg.Vault.Root, pinnedFileRel)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Server) handleListPinned(w http.ResponseWriter, r *http.Request) {
	m, err := s.readPinned()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []pinView{}
	for path, on := range m {
		if !on {
			continue
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if n := s.cfg.Vault.GetNote(path); n != nil {
			title = n.Title
		}
		out = append(out, pinView{Path: path, Title: title})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"pinned": out, "total": len(out)})
}

type pinPatchBody struct {
	Path   string `json:"path"`
	Pinned bool   `json:"pinned"`
}

func (s *Server) handlePatchPinned(w http.ResponseWriter, r *http.Request) {
	var b pinPatchBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if b.Path == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	m, err := s.readPinned()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b.Pinned {
		m[b.Path] = true
	} else {
		delete(m, b.Path)
	}
	if err := s.writePinned(m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Re-emit the full list so the client can refresh in one round-trip.
	s.handleListPinned(w, r)
}
