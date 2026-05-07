package serveapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/push"
)

// handleGetVAPID returns the public VAPID key the browser needs
// to call PushManager.subscribe. Lazy-generates a key pair on
// first call. The key is plain-text base64; safe to expose to any
// authenticated client (the public half is by design public).
func (s *Server) handleGetVAPID(w http.ResponseWriter, r *http.Request) {
	pub, err := s.push.PublicKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": pub})
}

// handlePushSubscribe persists a Web Push subscription posted by
// the SW. Idempotent — re-posting an existing endpoint replaces
// the keys, matching the browser's re-subscribe behaviour.
func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var sub push.Subscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.push.Subscribe(sub); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePushUnsubscribe removes a subscription by endpoint.
// Idempotent — unsubscribing an unknown endpoint is a no-op.
func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.push.Unsubscribe(body.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePushTest fires a single test notification to every stored
// subscription. Wired to a "test reminder" button in settings so
// the user can verify their setup without waiting for an event.
func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	successes, errs := s.push.SendAll(push.Payload{
		Title: "Granit",
		Body:  "Test notification — reminders are working.",
		URL:   "/calendar",
		Tag:   "granit-test",
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sent":   successes,
		"errors": fmtErrs(errs),
	})
}

func fmtErrs(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	return out
}

// runReminderScheduler is the background goroutine that fires
// Web Push notifications for events with reminders configured.
// Wakes every 30 seconds, scans the next ~hour of events, and
// sends a push for any whose (start - remindMinutesBefore) crossed
// into the past since the last tick.
//
// Skip rules:
//   - No subscribers → no work, just sleep.
//   - LastReminderFired already set within the last hour for the
//     same event → already notified, skip (handles a double-fire
//     within the same minute window).
//
// Persistence: when we fire a reminder, we update the event's
// LastReminderFired in events.json so a server restart doesn't
// re-notify on already-fired reminders.
func (s *Server) runReminderScheduler() {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for range tick.C {
		s.runReminderTick()
	}
}

// runReminderTick is the per-tick body, factored so tests can call
// it directly without spinning up the ticker.
func (s *Server) runReminderTick() {
	subs, err := s.push.Subscriptions()
	if err != nil || len(subs) == 0 {
		return
	}
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		return
	}
	now := time.Now()
	// Window: from now to now+12h. A reminder more than 12h in
	// advance won't be considered yet (saves us scanning the whole
	// year on every tick).
	horizon := now.Add(12 * time.Hour)
	dirty := false
	for i := range events {
		ev := &events[i]
		if ev.RemindMinutesBefore <= 0 {
			continue
		}
		startTime, ok := parseEventStart(*ev)
		if !ok {
			continue
		}
		fireAt := startTime.Add(-time.Duration(ev.RemindMinutesBefore) * time.Minute)
		// Skip events whose fire-time is in the future or beyond
		// the horizon — we'll catch them on a later tick.
		if fireAt.After(now) || fireAt.After(horizon) {
			continue
		}
		// Skip events whose start has already passed by more than
		// 30 minutes — the reminder is irrelevant after the event
		// has begun (the user knows or doesn't care).
		if startTime.Before(now.Add(-30 * time.Minute)) {
			continue
		}
		// Skip if we've already fired for this event recently.
		if ev.LastReminderFired != "" {
			if last, err := time.Parse(time.RFC3339, ev.LastReminderFired); err == nil {
				// "Recently" = within the last 24 hours. Once-a-day
				// recurring events would otherwise re-fire when
				// they round-trip through the scheduler.
				if now.Sub(last) < 24*time.Hour {
					continue
				}
			}
		}
		// Fire.
		body := buildReminderBody(*ev, startTime, now)
		_, _ = s.push.SendAll(push.Payload{
			Title: ev.Title,
			Body:  body,
			URL:   "/calendar",
			Tag:   "granit-event-" + ev.ID,
		})
		ev.LastReminderFired = now.UTC().Format(time.RFC3339)
		dirty = true
	}
	if dirty {
		_ = granitmeta.WriteEvents(s.cfg.Vault.Root, events)
	}
}

// parseEventStart returns the RFC3339-style start time for an
// Event using its Date + StartTime fields. Returns (zero, false)
// if either is empty or unparseable.
func parseEventStart(e granitmeta.Event) (time.Time, bool) {
	if e.Date == "" {
		return time.Time{}, false
	}
	dt, err := time.Parse("2006-01-02", e.Date)
	if err != nil {
		return time.Time{}, false
	}
	if e.StartTime != "" {
		t, err := time.Parse("15:04", e.StartTime)
		if err == nil {
			dt = time.Date(dt.Year(), dt.Month(), dt.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		}
	}
	return dt, true
}

// buildReminderBody builds the human-readable body of the push
// notification. Examples:
//   "in 15 min · 14:30"
//   "in 3 min · @ Conference Room"
//   "starting now · @ HQ"
func buildReminderBody(e granitmeta.Event, startTime, now time.Time) string {
	mins := int(startTime.Sub(now).Minutes())
	var when string
	switch {
	case mins <= 0:
		when = "starting now"
	case mins == 1:
		when = "in 1 min"
	default:
		when = "in " + strconv.Itoa(mins) + " min"
	}
	when += " · " + startTime.Format("15:04")
	if e.Location != "" {
		when += " · @ " + e.Location
	}
	return when
}
