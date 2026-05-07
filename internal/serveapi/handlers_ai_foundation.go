package serveapi

import (
	"encoding/json"
	"net/http"

	"github.com/artaeon/granit/internal/aiaudit"
	"github.com/artaeon/granit/internal/aicontext"
	"github.com/artaeon/granit/internal/aiprefs"
)

// handleGetAISnapshot returns the current Context Engine snapshot
// — the same data structure granit's AI features pass to providers
// (after redaction). Backs the "What AI sees" settings panel so
// the user has perfect transparency into what data MIGHT leave
// the device when an AI feature fires.
func (s *Server) handleGetAISnapshot(w http.ResponseWriter, r *http.Request) {
	if s.aiContext == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"snapshot": nil})
		return
	}
	snap := s.aiContext.Build(aicontext.BuildOpts{})
	writeJSON(w, http.StatusOK, map[string]interface{}{"snapshot": snap})
}

// handleGetAIPrefs returns the per-feature consent + provider
// settings. Defaults gracefully — missing file → Defaults().
func (s *Server) handleGetAIPrefs(w http.ResponseWriter, r *http.Request) {
	prefs, err := aiprefs.Load(s.cfg.Vault.Root)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"prefs":   prefs,
			"warning": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"prefs": prefs})
}

// handlePutAIPrefs replaces the prefs sidecar.
func (s *Server) handlePutAIPrefs(w http.ResponseWriter, r *http.Request) {
	var p aiprefs.Preferences
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := aiprefs.Save(s.cfg.Vault.Root, p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"prefs": p})
}

// handleGetAIAudit returns the most recent N entries from the
// audit log, newest first. The UI renders this as a scrollable
// list with timestamp / feature / provider / sizes.
func (s *Server) handleGetAIAudit(w http.ResponseWriter, r *http.Request) {
	if s.aiAudit == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"entries": []aiaudit.Entry{}})
		return
	}
	entries, err := s.aiAudit.List(200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"entries": entries})
}

// handleClearAIAudit deletes the audit log file. The GDPR right-
// to-erasure for the on-device portion. (The cloud providers' own
// retention is their problem; we only control what we log here.)
func (s *Server) handleClearAIAudit(w http.ResponseWriter, r *http.Request) {
	if s.aiAudit == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := s.aiAudit.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
