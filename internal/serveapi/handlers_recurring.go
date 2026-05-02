package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/recurring"
	"github.com/artaeon/granit/internal/tasks"
)

// recurringRunMu serialises the "create today's due tasks" pass.
// Only one runner at a time so concurrent requests can't double-
// generate when two clients hit the same day boundary.
var recurringRunMu sync.Mutex

// recurringLastRun is the date string of the most recent successful
// run. Used so the cheap fast-path on every request short-circuits
// when we already ran today.
var recurringLastRun string

// runRecurringIfDue creates any due recurring-task instances and stamps
// their LastCreated. Idempotent within a single calendar day — calling
// it 100 times on the same date does the work once.
//
// Called from three places:
//   1. Server boot, so a freshly-restarted server catches up on
//      missed days.
//   2. The midnight cron goroutine.
//   3. The handler that lists or mutates rules — cheap defence
//      against the goroutine missing a tick (laptop wake-from-sleep,
//      timer drift on long uptimes).
func (s *Server) runRecurringIfDue() (created int) {
	recurringRunMu.Lock()
	defer recurringRunMu.Unlock()

	today := time.Now()
	todayStr := today.Format("2006-01-02")
	if recurringLastRun == todayStr {
		return 0
	}

	rules, err := recurring.Load(s.cfg.Vault.Root)
	if err != nil || len(rules) == 0 {
		recurringLastRun = todayStr
		return 0
	}

	for i := range rules {
		t := rules[i]
		if !t.Enabled || !recurring.IsDue(t, today) {
			continue
		}
		// Append the line through the live TaskStore so the new
		// task gets a stable ID + an OriginRecurring marker on
		// the sidecar (matches what the TUI does).
		text := fmt.Sprintf("%s 📅 %s", t.Text, todayStr)
		if _, err := s.cfg.TaskStore.Create(text, tasks.CreateOpts{
			Origin:  tasks.OriginRecurring,
			Section: "## Tasks",
		}); err != nil {
			s.cfg.Logger.Warn("recurring: create failed", "text", t.Text, "err", err)
			continue
		}
		rules[i].LastCreated = todayStr
		created++
	}
	if created > 0 {
		if err := recurring.Save(s.cfg.Vault.Root, rules); err != nil {
			s.cfg.Logger.Warn("recurring: save failed", "err", err)
		}
	}
	recurringLastRun = todayStr
	return created
}

// startRecurringLoop runs in the background, waking at the start of
// every local-time day to fire runRecurringIfDue. Cancellation flows
// in via ctx.
func (s *Server) startRecurringLoop(ctx interface{ Done() <-chan struct{} }) {
	go func() {
		// Run once on boot so a server restarted mid-day catches up.
		if n := s.runRecurringIfDue(); n > 0 {
			s.cfg.Logger.Info("recurring: created tasks at boot", "count", n)
		}
		for {
			next := nextMidnight()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				if n := s.runRecurringIfDue(); n > 0 {
					s.cfg.Logger.Info("recurring: created tasks at midnight", "count", n)
				}
			}
		}
	}()
}

func nextMidnight() time.Time {
	now := time.Now()
	t := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 30, 0, now.Location())
	return t
}

// ----- HTTP handlers -----

func (s *Server) handleListRecurring(w http.ResponseWriter, r *http.Request) {
	s.runRecurringIfDue() // belt-and-braces — see comment on the func
	rules, err := recurring.Load(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []recurring.Task{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules, "total": len(rules)})
}

type recurringWriteBody struct {
	Rules []recurring.Task `json:"rules"`
}

// handlePutRecurring replaces the entire rule set. Simpler model than
// per-rule POST/PATCH/DELETE since the user is editing a small list,
// the file is small, and atomic-replace gives us trivial conflict
// semantics (last writer wins). The web ships the canonical list it
// wants persisted.
func (s *Server) handlePutRecurring(w http.ResponseWriter, r *http.Request) {
	var body recurringWriteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	for i := range body.Rules {
		body.Rules[i].Frequency = strings.ToLower(strings.TrimSpace(body.Rules[i].Frequency))
		body.Rules[i].Text = strings.TrimSpace(body.Rules[i].Text)
		if body.Rules[i].Frequency != "daily" && body.Rules[i].Frequency != "weekly" && body.Rules[i].Frequency != "monthly" {
			writeError(w, http.StatusBadRequest, "frequency must be daily/weekly/monthly")
			return
		}
		if body.Rules[i].Text == "" {
			writeError(w, http.StatusBadRequest, "rule text required")
			return
		}
	}
	if err := recurring.Save(s.cfg.Vault.Root, body.Rules); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Invalidate the "ran today" cache so a freshly-added rule
	// fires immediately on the next request rather than waiting
	// for tomorrow.
	recurringRunMu.Lock()
	recurringLastRun = ""
	recurringRunMu.Unlock()
	s.runRecurringIfDue()
	writeJSON(w, http.StatusOK, map[string]any{"rules": body.Rules, "total": len(body.Rules)})
}
