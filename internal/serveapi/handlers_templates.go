package serveapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/templates"
)

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	all := templates.All(s.cfg.Vault.Root)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": all,
		"total":     len(all),
	})
}

type fromTemplateBody struct {
	TemplateName string `json:"templateName"`
	Path         string `json:"path"`
	Title        string `json:"title"`
}

// handleFromTemplate creates a new note from a template. The template's
// content is expanded with {{title}} / {{date}} substitutions, then written
// via granit's atomicio so the path is identical to a TUI-created file.
func (s *Server) handleFromTemplate(w http.ResponseWriter, r *http.Request) {
	var b fromTemplateBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if b.Path == "" || strings.Contains(b.Path, "..") || strings.HasPrefix(b.Path, "/") {
		writeError(w, http.StatusBadRequest, "missing or invalid path")
		return
	}
	if !strings.HasSuffix(strings.ToLower(b.Path), ".md") {
		b.Path += ".md"
	}
	if existing := s.cfg.Vault.GetNote(b.Path); existing != nil {
		writeError(w, http.StatusConflict, "note already exists")
		return
	}

	// Find the template (built-in or user). Empty TemplateName → blank note.
	var content string
	if b.TemplateName != "" {
		all := templates.All(s.cfg.Vault.Root)
		var found *templates.Template
		for i := range all {
			if all[i].Name == b.TemplateName {
				found = &all[i]
				break
			}
		}
		if found == nil {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		title := b.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(b.Path), filepath.Ext(b.Path))
		}
		content = templates.Apply(found.Content, title, time.Now())
	}

	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(b.Path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := atomicio.WriteNote(abs, content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()

	n := s.cfg.Vault.GetNote(b.Path)
	if n == nil {
		writeError(w, http.StatusInternalServerError, "post-write: not found")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"path":  n.RelPath,
		"title": n.Title,
	})
}
