package serveapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/artaeon/granit/internal/sabbath"
)

// handleGetSabbath returns the persisted Sabbath state plus a
// derived "active_today" flag the UI can read directly without
// duplicating the date-comparison logic.
func (s *Server) handleGetSabbath(w http.ResponseWriter, r *http.Request) {
	state, err := sabbath.Load(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	today := time.Now().Format("2006-01-02")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_on":    state.ActiveOn,
		"active_today": state.ActiveOn == today,
	})
}

// handlePutSabbath replaces the persisted Sabbath state. Body
// shape: {active_on: "YYYY-MM-DD"} to enable, {active_on: ""} to
// disable. The server doesn't validate the date format strictly —
// IsActiveToday only matches an exact today-string, so a malformed
// value silently no-ops.
func (s *Server) handlePutSabbath(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ActiveOn string `json:"active_on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := sabbath.Save(s.cfg.Vault.Root, sabbath.State{ActiveOn: body.ActiveOn}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	today := time.Now().Format("2006-01-02")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_on":    body.ActiveOn,
		"active_today": body.ActiveOn == today,
	})
}
