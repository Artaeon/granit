package serveapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/artaeon/granit/internal/biblereading"
	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/wshub"
)

// Bible reading streak — habit-tracker on top of "did the user
// open scripture today". Two thin handlers:
//
//   GET  /api/v1/bible/streak    → current/longest consecutive-day stats
//   POST /api/v1/bible/read      → record TODAY as read (idempotent)
//
// The streak math reuses daily.ComputeStreak (same forgiving rule
// for "today still in progress" that the daily-note streak surfaces).
// Both features speak the same shape so the frontend's StreakBadge
// component can render either with no per-source branching.

func (s *Server) handleBibleStreak(w http.ResponseWriter, r *http.Request) {
	dates, err := biblereading.Snapshot(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Local clock for "today" — matches the daily-note streak's
	// boundary semantics (a Berlin user reading at 23:30 marks
	// that day, not the UTC day already over). Pure function; no
	// vault state needed beyond the date list itself.
	today := time.Now().Local().Format("2006-01-02")
	writeJSON(w, http.StatusOK, daily.ComputeStreak(dates, today))
}

// handleBibleReadRecord marks today as read. Body is optional —
// when absent we default to today's local date. Passing an explicit
// date lets a future "I read yesterday but forgot to mark" flow
// pre-fill cleanly, but the common path is a body-less POST.
func (s *Server) handleBibleReadRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date string `json:"date"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	date := body.Date
	if date == "" {
		date = time.Now().Local().Format("2006-01-02")
	}
	added, err := biblereading.RecordRead(s.cfg.Vault.Root, date)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if added && s.hub != nil {
		// Broadcast so any open StreakBadge in another tab re-fetches
		// without waiting on the page-revisit refresh interval.
		s.hub.Broadcast(wshub.Event{
			Type: "state.changed",
			Path: ".granit/bible-reading-log.json",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"added": added,
		"date":  date,
	})
}
