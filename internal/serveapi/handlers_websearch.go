package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/websearch"
)

// handleGetWebSearchConfig returns the per-vault web-search settings:
// chosen provider, whether a Brave key is set, default result count.
// Mirrors the GET-shaped responses /ai/prefs and /config use — the
// API key itself is never echoed back, only a `brave_key_set` flag,
// so a refresh of the settings page doesn't accidentally leak the
// secret into the network tab.
func (s *Server) handleGetWebSearchConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := websearch.Load(s.cfg.Vault.Root)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"provider":       cfg.Provider,
			"brave_key_set":  strings.TrimSpace(cfg.BraveKey) != "",
			"max_results":    cfg.MaxResults,
			"warning":        err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":      cfg.Provider,
		"brave_key_set": strings.TrimSpace(cfg.BraveKey) != "",
		"max_results":   cfg.MaxResults,
	})
}

// webSearchConfigPatch is the PATCH payload shape. All fields
// optional; the handler merges into the existing on-disk config so
// the user can update provider without re-pasting their Brave key.
type webSearchConfigPatch struct {
	// Provider chooses the backend. Empty string is treated as
	// "no change" — the user must explicitly set "duckduckgo" or
	// "brave" to switch.
	Provider *string `json:"provider,omitempty"`
	// BraveKey, when non-nil, replaces the stored key. Empty string
	// CLEARS the key (matching the openai_key pattern in /config).
	BraveKey *string `json:"brave_key,omitempty"`
	// MaxResults, when non-nil, replaces the cap. Out-of-range values
	// are clamped on read (EffectiveMaxResults), not on write — so
	// the user can store their preferred value across upgrades.
	MaxResults *int `json:"max_results,omitempty"`
}

// handlePatchWebSearchConfig merges patch fields into the existing
// config. Mirrors the /config PATCH semantics — only fields the
// client explicitly included are touched, leaving everything else
// alone.
func (s *Server) handlePatchWebSearchConfig(w http.ResponseWriter, r *http.Request) {
	var patch webSearchConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cfg, _ := websearch.Load(s.cfg.Vault.Root) // missing-file → Defaults
	if patch.Provider != nil {
		p := strings.ToLower(strings.TrimSpace(*patch.Provider))
		// Validate explicitly so a typo doesn't silently disable
		// the feature later when Resolve falls through to default.
		switch p {
		case "duckduckgo", "brave":
			cfg.Provider = p
		case "":
			// Treat empty as "keep current" — same as omitting.
		default:
			writeError(w, http.StatusBadRequest, "provider must be 'duckduckgo' or 'brave'")
			return
		}
	}
	if patch.BraveKey != nil {
		cfg.BraveKey = strings.TrimSpace(*patch.BraveKey)
	}
	if patch.MaxResults != nil {
		cfg.MaxResults = *patch.MaxResults
	}
	if err := websearch.Save(s.cfg.Vault.Root, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":      cfg.Provider,
		"brave_key_set": strings.TrimSpace(cfg.BraveKey) != "",
		"max_results":   cfg.MaxResults,
	})
}
