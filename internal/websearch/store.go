package websearch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/artaeon/granit/internal/atomicio"
)

// configRelPath is where the per-vault web-search settings live.
// Same pattern as aiprefs/ai-prefs.json — sidecar JSON under
// .granit so it travels with the vault and the user can edit it
// by hand.
const configRelPath = ".granit/web-search.json"

// ConfigPath returns the absolute path of the per-vault settings
// file given the vault root. Useful for status / debug reporting.
func ConfigPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, configRelPath)
}

// Load reads the per-vault settings sidecar. Missing file →
// Defaults. Malformed file → Defaults with the parse error
// returned so the settings UI can surface "your file is broken,
// I'm using defaults until you fix it" rather than silently
// reverting.
func Load(vaultRoot string) (Config, error) {
	path := ConfigPath(vaultRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Defaults(), nil
		}
		return Defaults(), err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Defaults(), err
	}
	// Normalise: empty provider → default. Out-of-range MaxResults
	// stays as-is; EffectiveMaxResults clamps at call time so the
	// user can keep their preferred value across upgrades.
	if c.Provider == "" {
		c.Provider = "duckduckgo"
	}
	return c, nil
}

// Save writes the settings sidecar atomically.
func Save(vaultRoot string, c Config) error {
	path := ConfigPath(vaultRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}
