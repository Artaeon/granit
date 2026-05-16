package serveapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/dayactivity"
)

// Day activity endpoint — surfaces everything created/completed/
// touched on a calendar day across the vault as a single ordered
// list. Powers the "What happened that day" overview on the Jots
// feed and the live `## Day overview` block on a daily note.
//
// Read-only. The aggregator (internal/dayactivity) is the single
// source of truth for the cross-surface query; this handler is
// thin glue:
//
//   1. parse the date (supports today / yesterday / tomorrow / ISO,
//      same shape as the daily-note handler so URL fragments
//      can be reused without translation).
//   2. resolve the daily-notes folder from per-vault config (kept
//      in lockstep with /jots; settings changes don't need a
//      server restart).
//   3. delegate to dayactivity.Collect.
//   4. emit JSON. An out-of-range date returns an empty list, NOT
//      a 404 — the SPA renders "nothing happened that day" rather
//      than blowing up on an error toast.

type dayActivityResponse struct {
	Date  string             `json:"date"`
	Items []dayactivity.Item `json:"items"`
}

func (s *Server) handleGetDayActivity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	dateParam := strings.TrimSpace(q.Get("date"))
	if dateParam == "" {
		dateParam = "today"
	}
	date, err := parseDailyParam(dateParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	maxItems := 200
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		maxItems = v
	}
	if maxItems > 500 {
		maxItems = 500
	}

	cfg := config.LoadForVault(s.cfg.Vault.Root)
	folder := strings.Trim(cfg.DailyNotesFolder, "/")

	// Vault-local zone: parseDailyParam already returns a time
	// stamped in time.Local (the server's zone). dayactivity
	// honours whichever zone we hand it; we pass it explicitly so
	// the contract is visible at the call site rather than relying
	// on date.Location() being non-nil.
	items := dayactivity.Collect(
		dayactivity.Query{
			Date:     date,
			Loc:      date.Location(),
			MaxItems: maxItems,
		},
		dayactivity.Sources{
			Vault:       s.cfg.Vault,
			Tasks:       s.cfg.TaskStore,
			VaultRoot:   s.cfg.Vault.Root,
			DailyFolder: folder,
		},
	)
	if items == nil {
		items = []dayactivity.Item{}
	}

	writeJSON(w, http.StatusOK, dayActivityResponse{
		Date:  date.Format("2006-01-02"),
		Items: items,
	})
}
