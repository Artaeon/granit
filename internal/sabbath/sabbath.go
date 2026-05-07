// Package sabbath persists the user's Sabbath-mode state to a
// vault-side JSON sidecar so server-side surfaces (push scheduler,
// background agents) can respect it. The frontend already drives
// the UI overlay from localStorage; this package mirrors the same
// flag to disk so the server knows too.
//
// State auto-expires: ActiveOn is a YYYY-MM-DD date; IsActiveToday
// returns true only when ActiveOn matches today's local date. A
// user who forgets to disable Sabbath at 11pm doesn't wake up
// tomorrow with the server still suppressing pushes — the date
// comparison handles the auto-clear without a background timer.
//
// Stored at <vault>/.granit/sabbath.json with 0o600 perms via
// atomicio. Tiny file (one field today; room for per-day-of-week
// schedule etc. later).
package sabbath

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// State is the persisted Sabbath flag. ActiveOn is the YYYY-MM-DD
// date the user enabled the mode. Empty → Sabbath off.
type State struct {
	ActiveOn string `json:"active_on,omitempty"`
}

// Path returns the absolute path of the sidecar.
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "sabbath.json")
}

// Load reads the sidecar. Missing file → empty State (not an error).
func Load(vaultRoot string) (State, error) {
	data, err := os.ReadFile(Path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// Save persists the state.
func Save(vaultRoot string, s State) error {
	dir := filepath.Dir(Path(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(Path(vaultRoot), data)
}

// IsActiveToday returns true when the state's ActiveOn equals
// today's local date. Use this from server-side schedulers /
// agents to silently skip work during the user's day of rest.
// Errors are treated as "not active" — failing-open here is safer
// than failing-closed (a missing sidecar shouldn't suppress the
// user's expected notifications).
func IsActiveToday(vaultRoot string) bool {
	s, err := Load(vaultRoot)
	if err != nil || s.ActiveOn == "" {
		return false
	}
	today := time.Now().Format("2006-01-02")
	return s.ActiveOn == today
}
