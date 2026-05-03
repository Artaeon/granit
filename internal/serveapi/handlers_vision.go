package serveapi

import (
	"net/http"
	"time"

	"github.com/artaeon/granit/internal/vision"
	"github.com/artaeon/granit/internal/wshub"
)

const statePathVision = ".granit/vision.json"

func (s *Server) bcastVision() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathVision})
}

// visionResponse decorates the on-disk Vision with derived fields the
// UI uses every render — season day-count + 90-day total. Keeping
// the derivation server-side means the dashboard widget can render
// without recomputing day math; the schema file stays minimal.
type visionResponse struct {
	vision.Vision
	SeasonDay   int `json:"season_day,omitempty"`
	SeasonTotal int `json:"season_total,omitempty"`
}

func (s *Server) handleGetVision(w http.ResponseWriter, r *http.Request) {
	v := vision.Load(s.cfg.Vault.Root)
	day, total := v.SeasonDayCount(time.Now())
	writeJSON(w, http.StatusOK, visionResponse{Vision: v, SeasonDay: day, SeasonTotal: total})
}

// handlePutVision is a full upsert — the client sends the whole
// record. Patch-merge isn't worth the complexity for a five-field
// flat object the user edits via a form, and a full PUT is the
// least-surprising shape for "save my settings."
//
// Behaviour worth knowing:
//   - When SeasonFocus changes (or it transitions from empty to
//     set), we stamp SeasonStartedAt to today unless the client
//     already supplied one. That keeps the day-counter honest:
//     "day 1 of 90" starts the day the user actually committed
//     to a focus, not the day they edited an unrelated field.
//   - Empty body → equivalent to clearing the vision. The user
//     can also do this from the UI; the server doesn't insist on
//     non-empty input because rare cases (testing, factory reset)
//     legitimately want to clear.
func (s *Server) handlePutVision(w http.ResponseWriter, r *http.Request) {
	var incoming vision.Vision
	if !readJSON(w, r, &incoming) {
		return
	}
	prev := vision.Load(s.cfg.Vault.Root)
	// Auto-stamp season start when the focus is newly set or changed
	// AND the client didn't override SeasonStartedAt explicitly.
	if incoming.SeasonFocus != "" && incoming.SeasonFocus != prev.SeasonFocus && incoming.SeasonStartedAt == "" {
		incoming.SeasonStartedAt = time.Now().Format("2006-01-02")
	}
	if err := vision.Save(s.cfg.Vault.Root, incoming); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVision()
	saved := vision.Load(s.cfg.Vault.Root)
	day, total := saved.SeasonDayCount(time.Now())
	writeJSON(w, http.StatusOK, visionResponse{Vision: saved, SeasonDay: day, SeasonTotal: total})
}
