package serveapi

// Daily Routine AI — Phase 2.
//
// Two endpoints:
//
//   POST /api/v1/calendar/routine-proposal — streams an SSE proposal for
//        the day. Body: {"date":"YYYY-MM-DD"} (defaults to today). The
//        stream emits two event kinds:
//          event: proposal — data is the partial / final JSON object
//                            (see routineProposal below)
//          event: done     — data is {"ok":true}
//          event: error    — data is {"message":"…"}
//
//   POST /api/v1/calendar/apply-routine — applies a (possibly user-edited)
//        proposal. Body: {"date":"YYYY-MM-DD","dailyPlan":"…","eventOps":[…]}.
//        Returns {"applied":N,"failed":[…]} — partial-safe: a mid-batch op
//        failure does NOT abort the rest; the failed op IDs / indices are
//        reported back so the UI can surface which rows didn't land.
//
// Constraints:
//   - Only native granit events (events.json) are mutated. ICS files
//     under <vault>/Calendars/ are externally-synced mirrors and stay
//     read-only.
//   - The daily-plan rewrite reuses upsertNamedSection from
//     handlers_morning.go so the section parser stays in one place.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/sabbath"
)

// routineProposalRequest is the optional body for the streaming endpoint.
// Empty body → today. Bad date → 400. We accept YYYY-MM-DD only.
type routineProposalRequest struct {
	Date string `json:"date"`
}

// routineEventOp is one event mutation the AI proposes. Op is one of
// "create" / "update" / "delete". The relevant fields vary by op:
//   - create: Event is required (title + date + start/end times).
//   - update: EventID + Patch are required.
//   - delete: EventID is required.
//
// Kept as a single struct (rather than an interface or three types) so the
// JSON shape matches the wire format the AI emits + the frontend posts
// back; the apply path branches on Op.
type routineEventOp struct {
	Op      string            `json:"op"`
	Event   *granitmeta.Event `json:"event,omitempty"`
	EventID string            `json:"eventId,omitempty"`
	Patch   map[string]any    `json:"patch,omitempty"`
}

// routineProposal is the JSON shape the SSE stream emits + the apply
// endpoint expects. Match this exactly in the frontend's TS types.
type routineProposal struct {
	Rationale string           `json:"rationale"`
	DailyPlan string           `json:"dailyPlan"`
	EventOps  []routineEventOp `json:"eventOps"`
}

// handleCalendarRoutineProposal streams a routine proposal for the given
// date. Stub for commit 1: returns a hardcoded fake proposal as a single
// SSE event so the route + wire shape can be exercised end-to-end before
// wiring the snapshot + AI call.
func (s *Server) handleCalendarRoutineProposal(w http.ResponseWriter, r *http.Request) {
	var body routineProposalRequest
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	date := strings.TrimSpace(body.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if !eventDateRe.MatchString(date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported by transport")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(event, data string) {
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Sabbath gate — same posture as the calendar agent.
	if sabbath.IsActiveNow(s.cfg.Vault.Root) {
		send("error", mustJSON(map[string]string{"message": "AI features are paused during Sabbath — exit Sabbath mode to use them"}))
		return
	}

	// Stub: emit one hardcoded proposal so the route + wire shape can be
	// exercised before the real AI call lands in a later commit.
	stub := routineProposal{
		Rationale: "Stub proposal — wiring check only. Real AI call lands in a follow-up commit.",
		DailyPlan: "## Daily Plan — " + date + "\n\n_(stub — no real plan yet)_\n",
		EventOps:  []routineEventOp{},
	}
	send("proposal", mustJSON(stub))
	send("done", `{"ok":true}`)
}

// routineApplyRequest is the body for /api/v1/calendar/apply-routine.
// Date scopes the dailyPlan rewrite to that day's daily note. eventOps
// is the user's possibly-edited subset of the proposed ops.
type routineApplyRequest struct {
	Date      string           `json:"date"`
	DailyPlan string           `json:"dailyPlan"`
	EventOps  []routineEventOp `json:"eventOps"`
}

// routineApplyFailure records one failed op for the partial-safe response.
// Index is the position in the request's eventOps array (so the UI can
// highlight the row that didn't land); Message is the underlying error.
type routineApplyFailure struct {
	Index   int    `json:"index"`
	Op      string `json:"op,omitempty"`
	EventID string `json:"eventId,omitempty"`
	Message string `json:"message"`
}

type routineApplyResponse struct {
	Applied int                   `json:"applied"`
	Failed  []routineApplyFailure `json:"failed"`
}

// handleCalendarApplyRoutine applies a proposal. Stub for commit 1 —
// validates the body shape + returns an empty applied/failed response so
// the frontend wiring has something to call. Real apply lands in a later
// commit.
func (s *Server) handleCalendarApplyRoutine(w http.ResponseWriter, r *http.Request) {
	var body routineApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !eventDateRe.MatchString(strings.TrimSpace(body.Date)) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	writeJSON(w, http.StatusOK, routineApplyResponse{
		Applied: 0,
		Failed:  []routineApplyFailure{},
	})
}
