package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/timetracker"
	"github.com/artaeon/granit/internal/wshub"
)

// handleListTimetracker returns recent entries (newest first) and the
// active timer if one is running. Limit defaults to 200 — enough for
// "show me this week's sessions" without shipping the whole history.
func (s *Server) handleListTimetracker(w http.ResponseWriter, r *http.Request) {
	all, err := timetracker.Load(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Sort newest first by EndTime so the typical "what was I doing
	// recently?" query reads off the top of the list.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	s.timerMu.Lock()
	var active map[string]any
	if s.activeTimer != nil {
		active = map[string]any{
			"notePath":   s.activeTimer.NotePath,
			"taskText":   s.activeTimer.TaskText,
			"taskId":     s.activeTimer.TaskID,
			"startTime":  s.activeTimer.StartTime.Format(time.RFC3339),
			"elapsedSec": int(time.Since(s.activeTimer.StartTime).Seconds()),
		}
	}
	s.timerMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"entries":         all,
		"total":           len(all),
		"active":          active,
		"minutesByTaskId": timetracker.MinutesByTaskID(all),
		"minutesToday":    timetracker.MinutesToday(all),
	})
}

type clockInBody struct {
	NotePath string `json:"notePath"`
	TaskText string `json:"taskText"`
	TaskID   string `json:"taskId,omitempty"`
}

// handleClockIn starts a timer. If one is already running, that one
// gets stopped + saved first so the caller doesn't accidentally
// abandon time. Returns the newly-active timer state.
func (s *Server) handleClockIn(w http.ResponseWriter, r *http.Request) {
	var body clockInBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.TaskText) == "" && strings.TrimSpace(body.TaskID) == "" {
		writeError(w, http.StatusBadRequest, "taskText or taskId required")
		return
	}

	now := time.Now()
	s.timerMu.Lock()
	// Clock out the existing timer (if any) so the user's previous
	// session gets recorded.
	if prev := s.activeTimer; prev != nil {
		entry := timetracker.Entry{
			NotePath:  prev.NotePath,
			TaskText:  prev.TaskText,
			TaskID:    prev.TaskID,
			StartTime: prev.StartTime,
			EndTime:   now,
			Duration:  now.Sub(prev.StartTime),
			Date:      prev.StartTime.Format("2006-01-02"),
		}
		s.timerMu.Unlock()
		if err := timetracker.Append(s.cfg.Vault.Root, entry); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.timerMu.Lock()
	}
	s.activeTimer = &activeTimer{
		NotePath:  strings.TrimSpace(body.NotePath),
		TaskText:  strings.TrimSpace(body.TaskText),
		TaskID:    strings.TrimSpace(body.TaskID),
		StartTime: now,
	}
	at := *s.activeTimer
	s.timerMu.Unlock()

	s.hub.Broadcast(wshub.Event{Type: "timer.started", ID: at.TaskID, Data: map[string]any{"taskText": at.TaskText}})
	writeJSON(w, http.StatusOK, map[string]any{
		"notePath":  at.NotePath,
		"taskText":  at.TaskText,
		"taskId":    at.TaskID,
		"startTime": at.StartTime.Format(time.RFC3339),
	})
}

// handleClockOut stops the active timer and persists the session.
// 204 if no timer is running — idempotent on the user's end.
func (s *Server) handleClockOut(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	s.timerMu.Lock()
	prev := s.activeTimer
	s.activeTimer = nil
	s.timerMu.Unlock()
	if prev == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	entry := timetracker.Entry{
		NotePath:  prev.NotePath,
		TaskText:  prev.TaskText,
		TaskID:    prev.TaskID,
		StartTime: prev.StartTime,
		EndTime:   now,
		Duration:  now.Sub(prev.StartTime),
		Date:      prev.StartTime.Format("2006-01-02"),
	}
	if err := timetracker.Append(s.cfg.Vault.Root, entry); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "timer.stopped", ID: prev.TaskID, Data: map[string]any{"minutes": int(entry.Duration.Minutes())}})
	writeJSON(w, http.StatusOK, map[string]any{
		"taskId":   entry.TaskID,
		"taskText": entry.TaskText,
		"minutes":  int(entry.Duration.Minutes()),
		"endTime":  entry.EndTime.Format(time.RFC3339),
	})
}
