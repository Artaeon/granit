package serveapi

import (
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/scripture/lectionary"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/wshub"
)

// Lectionary — bundled Bible reading plans (M'Cheyne 1-year,
// chronological NT, 90-day NT). Six handlers:
//
//   GET    /api/v1/scripture/plans              → list catalogue (no Readings — cheap summary)
//   GET    /api/v1/scripture/plans/active       → list active plans + today's readings
//   GET    /api/v1/scripture/plans/{id}         → full plan with all Readings
//   GET    /api/v1/scripture/plans/{id}/day/{n} → one day's readings
//   POST   /api/v1/scripture/plans/{id}/start   → mark active starting today
//   DELETE /api/v1/scripture/plans/{id}/start   → stop the plan
//   POST   /api/v1/scripture/plans/{id}/schedule-today → drop today's readings as a task on the daily note
//
// State lives at <vault>/.granit/lectionary-state.json (see
// scripture/lectionary/state.go). The catalogue itself is in-memory only;
// no on-disk plan definitions to migrate.

// planSummary is the shape returned by the list endpoint — no Readings
// to keep the payload small. The detail endpoint /plans/{id} returns
// the full Plan with all DayReadings.
type planSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LengthDays  int    `json:"lengthDays"`
}

// activePlanView merges state (PlanID, StartedAt) with a snapshot of
// the plan's metadata and today's reading. Returned by /plans/active so
// the UI can render "M'Cheyne · day 47 of 365 · [Gen 47, Job 13, …]"
// without three round trips per active plan.
type activePlanView struct {
	PlanID         string   `json:"planId"`
	PlanName       string   `json:"planName"`
	LengthDays     int      `json:"lengthDays"`
	StartedAt      string   `json:"startedAt"`
	DayOfPlan      int      `json:"dayOfPlan"`      // 1-indexed, may exceed lengthDays if user kept going past the end
	Finished       bool     `json:"finished"`       // dayOfPlan > lengthDays
	TodayPassages  []string `json:"todayPassages"`  // empty when finished
}

func (s *Server) handleListLectionaryPlans(w http.ResponseWriter, r *http.Request) {
	all := lectionary.Plans()
	out := make([]planSummary, len(all))
	for i, p := range all {
		out[i] = planSummary{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			LengthDays:  p.LengthDays,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plans": out,
		"total": len(out),
	})
}

// handleGetLectionaryPlan returns the full plan including every day's
// readings. Payload can be ~10kB for M'Cheyne (365 days × ~4 passages);
// fine on a one-shot detail endpoint, not used in the list view.
func (s *Server) handleGetLectionaryPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := lectionary.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleGetLectionaryDay(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := lectionary.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}
	nStr := chi.URLParam(r, "n")
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 1 || n > len(p.Readings) {
		writeError(w, http.StatusNotFound, "day out of range")
		return
	}
	writeJSON(w, http.StatusOK, p.Readings[n-1])
}

func (s *Server) handleListActiveLectionary(w http.ResponseWriter, r *http.Request) {
	st, err := lectionary.LoadState(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	out := make([]activePlanView, 0, len(st.Active))
	for _, a := range st.Active {
		p, ok := lectionary.Get(a.PlanID)
		if !ok {
			// Stale state — plan id no longer in catalogue. Skip silently
			// rather than 500; a future migration can clean these up.
			continue
		}
		day := lectionary.DayOfPlan(a, now)
		var passages []string
		finished := day > p.LengthDays
		if !finished && day >= 1 && day <= len(p.Readings) {
			passages = append(passages, p.Readings[day-1].Passages...)
		}
		out = append(out, activePlanView{
			PlanID:        a.PlanID,
			PlanName:      p.Name,
			LengthDays:    p.LengthDays,
			StartedAt:     a.StartedAt.Format(time.RFC3339),
			DayOfPlan:     day,
			Finished:      finished,
			TodayPassages: passages,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active": out,
		"total":  len(out),
	})
}

func (s *Server) handleStartLectionary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := lectionary.Get(id); !ok {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}
	if err := lectionary.StartPlan(s.cfg.Vault.Root, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastLectionaryChanged()
	// Return the new state so the UI can update without a follow-up GET.
	s.handleListActiveLectionary(w, r)
}

func (s *Server) handleStopLectionary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := lectionary.StopPlan(s.cfg.Vault.Root, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastLectionaryChanged()
	s.handleListActiveLectionary(w, r)
}

// handleScheduleLectionaryToday drops today's readings as a single
// task on today's daily note. Pattern lifted from the frontend's
// scheduleVerseReview() but hoisted server-side because the readings
// are already known here (no extra round trip to fetch the plan).
//
// Task shape:
//   text:           "Read · {Plan name} day {N} · {passages joined}"
//   tags:           ["lectionary"]
//   scheduledStart: today 09:00 local time
//   durationMinutes: 20 (rough reading time for the M'Cheyne 4-passage day)
//
// Returns 409 when the plan isn't active (you can't schedule what you
// haven't started) and 410 Gone when the plan has run past its end.
func (s *Server) handleScheduleLectionaryToday(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := lectionary.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}
	st, err := lectionary.LoadState(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var active *lectionary.ActivePlan
	for i := range st.Active {
		if st.Active[i].PlanID == id {
			active = &st.Active[i]
			break
		}
	}
	if active == nil {
		writeError(w, http.StatusConflict, "plan is not active — start it first")
		return
	}
	now := time.Now()
	day := lectionary.DayOfPlan(*active, now)
	if day < 1 || day > len(p.Readings) {
		writeError(w, http.StatusGone, "plan has run past its end")
		return
	}
	dr := p.Readings[day-1]

	// Resolve today's daily note path. EnsureDaily creates the file if
	// it doesn't exist yet (the frontend's scheduleVerseReview gets
	// this via api.daily('today'); we do the equivalent inline).
	dcfg := s.dailyConfigFor()
	_, _, _ = daily.EnsureDaily(s.cfg.Vault.Root, dcfg)
	filename := now.Format("2006-01-02") + ".md"
	notePath := filename
	if dcfg.Folder != "" {
		notePath = filepath.ToSlash(filepath.Join(dcfg.Folder, filename))
	}

	// Compose task text. The passages are joined with a comma — the
	// existing reference-parser on the scripture page (parseRefs) will
	// turn them back into clickable chips on the calendar hover view.
	joined := joinPassages(dr.Passages)
	text := "Read · " + p.Name + " day " + strconv.Itoa(day) + " · " + joined
	textWithMarkers := buildTaskTextLine(text, 0, "", []string{"lectionary"})

	opts := tasks.CreateOpts{
		File:    notePath,
		Origin:  tasks.OriginManual,
		Section: "## Tasks",
	}
	t, err := s.cfg.TaskStore.Create(textWithMarkers, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Schedule for 09:00 local today — same default the frontend
	// scheduleVerseReview uses for the first review day.
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	dur := 20 * time.Minute
	_ = s.cfg.TaskStore.Schedule(t.ID, start, dur)
	t, _ = s.cfg.TaskStore.GetByID(t.ID)
	s.broadcastTaskChange(t.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"task":     taskToView(t),
		"day":      day,
		"passages": dr.Passages,
	})
}

func joinPassages(passages []string) string {
	out := ""
	for i, p := range passages {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

func (s *Server) broadcastLectionaryChanged() {
	if s == nil || s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{
		Type: "state.changed",
		Path: ".granit/lectionary-state.json",
	})
}
