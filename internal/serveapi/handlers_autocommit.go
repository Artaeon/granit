package serveapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// Autocommit settings live at <vault>/.granit/autocommit.json.
// Tiny JSON sidecar — single boolean today, room to grow (e.g.
// commit-message template, exclude paths) without breaking
// callers.
type autocommitSettings struct {
	Enabled bool `json:"enabled"`
}

func autocommitSettingsPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "autocommit.json")
}

// loadAutocommitSetting returns the persisted enabled state. Missing
// file or any error returns false — autocommit is strictly opt-in,
// so failing-closed is the only safe default.
func loadAutocommitSetting(vaultRoot string) bool {
	data, err := os.ReadFile(autocommitSettingsPath(vaultRoot))
	if err != nil {
		return false
	}
	var s autocommitSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return false
	}
	return s.Enabled
}

func saveAutocommitSetting(vaultRoot string, enabled bool) error {
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(autocommitSettings{Enabled: enabled}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(autocommitSettingsPath(vaultRoot), data, 0o600)
}

// handleGetAutocommit returns the current enabled state + whether
// the vault is a git repo (so the UI can grey out the toggle when
// it would do nothing).
func (s *Server) handleGetAutocommit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":   s.autocommit.IsEnabled(),
		"isGitRepo": s.autocommit.IsGitRepo(),
	})
}

type autocommitPatch struct {
	Enabled bool `json:"enabled"`
}

// handlePutAutocommit toggles the setting. Persists to the JSON
// sidecar and updates the live Manager. If the user turns it on
// while the vault isn't a git repo, we still accept the toggle —
// the Manager just won't do anything until the user sets up git.
func (s *Server) handlePutAutocommit(w http.ResponseWriter, r *http.Request) {
	var b autocommitPatch
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := saveAutocommitSetting(s.cfg.Vault.Root, b.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.autocommit.SetEnabled(b.Enabled)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":   s.autocommit.IsEnabled(),
		"isGitRepo": s.autocommit.IsGitRepo(),
	})
}
