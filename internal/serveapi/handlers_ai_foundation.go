package serveapi

import (
	"encoding/json"
	"net/http"

	"github.com/artaeon/granit/internal/aiaudit"
	"github.com/artaeon/granit/internal/aicontext"
	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/sabbath"
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

// handleGetAIStatus surfaces the runtime view of every AI feature so
// the user can see — without firing a request — exactly which provider
// + model granit will route a given feature to. Settings shows this
// next to each toggle so "Daily briefing" and "Inbox triage" can be
// pointed at different backends and the user can confirm.
//
// The response also includes Sabbath state (every AI feature is
// short-circuited during Sabbath) and the file-global provider so
// the UI can flag "uses fallback provider".
func (s *Server) handleGetAIStatus(w http.ResponseWriter, r *http.Request) {
	prefs, _ := aiprefs.Load(s.cfg.Vault.Root)
	cfgFile := config.LoadForVault(s.cfg.Vault.Root)
	type featureStatus struct {
		Enabled  bool   `json:"enabled"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		// Source describes where Provider came from — "feature" if
		// the user set a per-feature override, "default" if the
		// prefs DefaultProvider applied, "global" if we fell all the
		// way back to ai_provider in config.json.
		Source string `json:"source"`
	}
	features := make(map[string]featureStatus, len(prefs.Features))
	for fid, fc := range prefs.Features {
		resolved := resolveLLMConfig(s.cfg.Vault.Root, fc.Provider, prefs.DefaultProvider)
		source := "global"
		if fc.Provider != "" {
			source = "feature"
		} else if prefs.DefaultProvider != "" {
			source = "default"
		}
		features[string(fid)] = featureStatus{
			Enabled:  fc.Enabled,
			Provider: resolved.AIProvider,
			Model:    effectiveModel(resolved),
			Source:   source,
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sabbath_active":   sabbath.IsActiveNow(s.cfg.Vault.Root),
		"global_provider":  cfgFile.AIProvider,
		"global_model":     effectiveModel(cfgFile),
		"redaction":        prefs.RedactionEnabled,
		"default_provider": prefs.DefaultProvider,
		"features":         features,
	})
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
