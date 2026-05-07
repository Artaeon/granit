package serveapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/artaeon/granit/internal/deadlines"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/push"
	"github.com/artaeon/granit/internal/sabbath"
	"github.com/artaeon/granit/internal/tasks"
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

// handleGetNotificationPrefs returns the persisted notification
// preferences (per-category toggles, quiet hours, defaults).
// Missing file → DefaultPreferences so a fresh vault Just Works.
func (s *Server) handleGetNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	prefs, err := push.LoadPrefs(s.cfg.Vault.Root)
	if err != nil {
		// Surface the parse error but still return defaults so
		// the UI can show something.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"prefs":   prefs,
			"warning": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"prefs": prefs})
}

// handlePutNotificationPrefs replaces the prefs sidecar with the
// posted JSON. Validation is light — bad time strings get
// silently rejected at MatchesAtTime; bad days_before values are
// clamped to non-negative ints.
func (s *Server) handlePutNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	var p push.Preferences
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Clamp DaysBefore to non-negative + dedupe.
	clean := make([]int, 0, len(p.Deadlines.DaysBefore))
	seen := map[int]bool{}
	for _, d := range p.Deadlines.DaysBefore {
		if d < 0 || seen[d] {
			continue
		}
		seen[d] = true
		clean = append(clean, d)
	}
	p.Deadlines.DaysBefore = clean
	if err := push.SavePrefs(s.cfg.Vault.Root, p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"prefs": p})
}

// handlePushMe returns the server-side state for the subscription
// matching the supplied endpoint (in particular: whether it's
// paused). Used by the frontend's settings page to render the
// pause toggle in its actual state. Returns 404 when no record
// matches, which the UI treats as "not subscribed".
func (s *Server) handlePushMe(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		writeError(w, http.StatusBadRequest, "missing endpoint")
		return
	}
	subs, err := s.push.Subscriptions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, sub := range subs {
		if sub.Endpoint == endpoint {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"subscribed": true,
				"paused":     sub.Paused,
				"label":      sub.Label,
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"subscribed": false})
}

// handlePushPause toggles the Paused flag on a subscription. The
// frontend uses this for the "Pause notifications" toggle in
// settings — keeps the subscription alive (no need to re-grant
// permission later) but tells the scheduler to skip pushing to
// the endpoint while paused.
func (s *Server) handlePushPause(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Paused   bool   `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.push.SetPaused(body.Endpoint, body.Paused); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "paused": body.Paused})
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
//
// Order of work each tick:
//   1. Bail if there are no subscriptions to push to.
//   2. Load preferences. Quiet-hours window? skip everything.
//   3. Calendar events (if calendar prefs enabled) — fire when
//      crossing event.start - remind_minutes_before.
//   4. Tasks (if tasks prefs enabled) — fire daily "due today"
//      reminder at the configured time-of-day.
//   5. Deadlines (if deadlines prefs enabled) — fire reminders
//      at each days_before milestone, at the configured time.
func (s *Server) runReminderTick() {
	subs, err := s.push.Subscriptions()
	if err != nil || len(subs) == 0 {
		return
	}
	// Sabbath silences ALL pushes for the user's day of rest. This
	// is checked BEFORE prefs.IsQuiet so even a user who hasn't
	// configured quiet hours gets the protection.
	if sabbath.IsActiveToday(s.cfg.Vault.Root) {
		return
	}
	prefs, _ := push.LoadPrefs(s.cfg.Vault.Root)
	now := time.Now()
	if prefs.IsQuiet(now) {
		return
	}
	if prefs.Calendar.Enabled {
		s.fireEventReminders(now)
	}
	if prefs.Tasks.Enabled {
		s.fireTaskReminders(now, prefs.Tasks)
	}
	if prefs.Deadlines.Enabled {
		s.fireDeadlineReminders(now, prefs.Deadlines)
	}
}

// fireEventReminders scans the events.json sidecar for upcoming
// events that have crossed their reminder window and fires a push
// for each. Stamps LastReminderFired on each fired event.
func (s *Server) fireEventReminders(now time.Time) {
	events, err := granitmeta.ReadEvents(s.cfg.Vault.Root)
	if err != nil {
		return
	}
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
		if fireAt.After(now) || fireAt.After(horizon) {
			continue
		}
		if startTime.Before(now.Add(-30 * time.Minute)) {
			continue
		}
		if ev.LastReminderFired != "" {
			if last, err := time.Parse(time.RFC3339, ev.LastReminderFired); err == nil {
				if now.Sub(last) < 24*time.Hour {
					continue
				}
			}
		}
		_, _ = s.push.SendAll(push.Payload{
			Title:    ev.Title,
			Body:     buildReminderBody(*ev, startTime, now),
			URL:      "/calendar",
			Tag:      "granit-event-" + ev.ID,
			Category: "event",
		})
		ev.LastReminderFired = now.UTC().Format(time.RFC3339)
		dirty = true
	}
	if dirty {
		_ = granitmeta.WriteEvents(s.cfg.Vault.Root, events)
	}
}

// fireTaskReminders fires one "task due today" reminder per task
// per day, at the configured time-of-day. The push lists up to
// three task titles when multiple tasks are due — one push, not
// N pushes, so the user gets a single morning summary instead of
// a flurry. LastReminderFired stamps each task to dedupe.
func (s *Server) fireTaskReminders(now time.Time, prefs push.TaskPrefs) {
	if !push.MatchesAtTime(now, prefs.DueTodayTime) {
		return
	}
	if s.cfg.TaskStore == nil {
		return
	}
	all := s.cfg.TaskStore.All()
	today := now.Format("2006-01-02")
	var due []string
	var fired []*tasksRef
	for i := range all {
		t := &all[i]
		if t.Done || t.DueDate != today {
			continue
		}
		// Skip snoozed tasks.
		if t.SnoozedUntil != "" {
			if su, err := time.Parse(time.RFC3339, t.SnoozedUntil); err == nil && su.After(now) {
				continue
			}
		}
		// Already fired today?
		if t.LastReminderFired == today {
			continue
		}
		due = append(due, t.Text)
		fired = append(fired, &tasksRef{notePath: t.NotePath, id: t.ID, today: today})
	}
	if len(due) == 0 {
		return
	}
	title := "Tasks due today"
	body := summariseTasks(due)
	_, _ = s.push.SendAll(push.Payload{
		Title:    title,
		Body:     body,
		URL:      "/tasks",
		Tag:      "granit-tasks-" + today,
		Category: "task",
	})
	// Stamp LastReminderFired on each fired task. The TaskStore
	// keeps tasks in markdown + a sidecar; UpdateMeta is the
	// safe path for sidecar-only fields like this.
	for _, ref := range fired {
		_ = s.cfg.TaskStore.UpdateMeta(ref.id, func(t *tasks.Task) {
			t.LastReminderFired = ref.today
		})
	}
}

type tasksRef struct {
	notePath string
	id       string
	today    string
}

// fireDeadlineReminders fires reminders at each configured
// days-before milestone. For each deadline whose date matches
// (today + offset) for any offset in DaysBefore, fire a single
// notification. Per-offset dedup via Deadline.LastReminderFired
// keyed by the offset string.
func (s *Server) fireDeadlineReminders(now time.Time, prefs push.DeadlinePrefs) {
	if !push.MatchesAtTime(now, prefs.AtTime) {
		return
	}
	all := deadlines.LoadAll(s.cfg.Vault.Root)
	today := now.Truncate(24 * time.Hour)
	dirty := false
	for i := range all {
		d := &all[i]
		if d.Status != "active" {
			continue
		}
		dd, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		dueDay := dd.Truncate(24 * time.Hour)
		daysUntil := int(dueDay.Sub(today).Hours() / 24)
		matched := false
		for _, off := range prefs.DaysBefore {
			if daysUntil == off {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		// Dedup: one fire per offset per day.
		key := strconv.Itoa(daysUntil)
		todayISO := now.Format("2006-01-02")
		if d.LastReminderFired != nil {
			if last, ok := d.LastReminderFired[key]; ok && last == todayISO {
				continue
			}
		}
		body := buildDeadlineBody(*d, daysUntil)
		_, _ = s.push.SendAll(push.Payload{
			Title:    "Deadline: " + d.Title,
			Body:     body,
			URL:      "/deadlines",
			Tag:      "granit-deadline-" + d.ID,
			Category: "deadline",
		})
		if d.LastReminderFired == nil {
			d.LastReminderFired = map[string]string{}
		}
		d.LastReminderFired[key] = todayISO
		dirty = true
	}
	if dirty {
		_ = deadlines.SaveAll(s.cfg.Vault.Root, all)
	}
}

func summariseTasks(titles []string) string {
	if len(titles) == 1 {
		return titles[0]
	}
	if len(titles) <= 3 {
		out := ""
		for i, t := range titles {
			if i > 0 {
				out += " · "
			}
			out += t
		}
		return out
	}
	return titles[0] + " · " + titles[1] + " · +" + strconv.Itoa(len(titles)-2) + " more"
}

func buildDeadlineBody(d deadlines.Deadline, daysUntil int) string {
	switch {
	case daysUntil == 0:
		return "Due today · " + d.Date
	case daysUntil == 1:
		return "Due tomorrow · " + d.Date
	default:
		return "Due in " + strconv.Itoa(daysUntil) + " days · " + d.Date
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
