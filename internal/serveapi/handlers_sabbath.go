package serveapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/artaeon/granit/internal/sabbath"
)

// handleGetSabbath returns the persisted Sabbath state plus a
// derived "active_now" flag the UI can read directly without
// duplicating the window-comparison logic. Also exposes a
// "remaining_minutes" field for the landing-page countdown.
func (s *Server) handleGetSabbath(w http.ResponseWriter, r *http.Request) {
	state, err := sabbath.Load(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	activeNow := sabbath.IsActiveNow(s.cfg.Vault.Root)
	remaining := 0
	if activeNow {
		remaining = sabbath.RemainingMinutes(state, now)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_on":         state.ActiveOn,
		"active_now":        activeNow,
		"active_today":      activeNow, // alias for older clients
		"remaining_minutes": remaining,
		"schedule":          state.Schedule,
	})
}

// handlePutSabbath replaces the persisted Sabbath state. Body shape:
// {active_on?: "YYYY-MM-DD", schedule?: {...}}. Either field is
// optional — clients can update one without re-sending the other.
// Empty active_on disables the manual flag; schedule with Enabled
// false disables auto-activation.
func (s *Server) handlePutSabbath(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ActiveOn *string            `json:"active_on,omitempty"`
		Schedule *sabbath.Schedule  `json:"schedule,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	state, err := sabbath.Load(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	prevActive := state.IsActiveAt(now)
	if body.ActiveOn != nil {
		state.ActiveOn = *body.ActiveOn
	}
	if body.Schedule != nil {
		state.Schedule = sabbath.NormalizeSchedule(*body.Schedule)
	}
	if err := sabbath.Save(s.cfg.Vault.Root, state); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Observation log — record transitions only (entry/exit), not
	// every API write. Avoids noise from devices that re-PUT the
	// same state on every focus.
	nowActive := state.IsActiveAt(now)
	if !prevActive && nowActive {
		_ = sabbath.AppendLog(s.cfg.Vault.Root, sabbath.LogEntry{At: now.Format(time.RFC3339), Event: "begin"})
	} else if prevActive && !nowActive {
		_ = sabbath.AppendLog(s.cfg.Vault.Root, sabbath.LogEntry{At: now.Format(time.RFC3339), Event: "end"})
	}
	remaining := 0
	if nowActive {
		remaining = sabbath.RemainingMinutes(state, now)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_on":         state.ActiveOn,
		"active_now":        nowActive,
		"active_today":      nowActive,
		"remaining_minutes": remaining,
		"schedule":          state.Schedule,
	})
}

// handleGetSabbathLog returns the observation log (most recent first,
// up to ~200 entries). Surfaced on the /sabbath landing as a quiet
// witness of past observances. Not gamified; no streaks.
func (s *Server) handleGetSabbathLog(w http.ResponseWriter, r *http.Request) {
	entries, err := sabbath.LoadLog(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Reverse for most-recent-first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if len(entries) > 200 {
		entries = entries[:200]
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"entries": entries})
}
