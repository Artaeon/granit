package serveapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/icswriter"
)

// The Kind field must round-trip through the calendar feed wire
// shape so the frontend can render the glyph + tint on chips.
// Pre-fix this was tested at the file-IO layer
// (TestICS_KindRoundTrip) and the patch handler, but the actual
// path the browser takes — GET /api/v1/calendar?from=…&to=… —
// wasn't covered. Drift in the feed handler (forgetting a Kind
// field on a new emit branch, e.g.) would silently break chip
// rendering until a user noticed and reported.
func TestCalendarFeed_EmitsKind_NativeEvents(t *testing.T) {
	s, h := calendarTestServer(t)

	ev := granitmeta.Event{
		ID:        "evt-meeting",
		Title:     "Sync with Sam",
		Date:      "2026-05-09",
		StartTime: "10:00",
		EndTime:   "10:30",
		Kind:      "meeting",
	}
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, []granitmeta.Event{ev}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar?from=2026-05-09&to=2026-05-09", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("calendar: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Events []calendarEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var got *calendarEvent
	for i := range out.Events {
		if out.Events[i].EventID == "evt-meeting" {
			got = &out.Events[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("event not found in feed; got %d events", len(out.Events))
	}
	if got.Kind != "meeting" {
		t.Errorf("Kind not on wire shape; got %q", got.Kind)
	}
}

// Same check for ICS events: X-GRANIT-KIND on disk must surface as
// `kind` in the JSON the calendar feed returns.
func TestCalendarFeed_EmitsKind_ICSEvents(t *testing.T) {
	s, h := calendarTestServer(t)
	if err := os.MkdirAll(filepath.Join(s.cfg.Vault.Root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.cfg.Vault.Root, "calendars", "work.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "work"}, []icswriter.Event{{
		UID:     "ic-focus-1@granit",
		Summary: "Deep work",
		Start:   mustTime(t, "2026-05-09T14:00:00Z"),
		End:     mustTime(t, "2026-05-09T15:30:00Z"),
		Kind:    "focus",
	}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar?from=2026-05-09&to=2026-05-09", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("calendar: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Events []calendarEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var got *calendarEvent
	for i := range out.Events {
		if out.Events[i].EventID == "ic-focus-1@granit" {
			got = &out.Events[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("ICS event not found in feed; got %d events", len(out.Events))
	}
	if got.Kind != "focus" {
		t.Errorf("ICS Kind not on wire shape; got %q", got.Kind)
	}
}

// Recurring events with a Kind: every occurrence emitted by
// expandRRULE must carry the Kind too (it's per-series metadata).
func TestCalendarFeed_EmitsKind_OnEveryRecurringOccurrence(t *testing.T) {
	s, h := calendarTestServer(t)

	ev := granitmeta.Event{
		ID:        "rec-1",
		Title:     "Daily standup",
		Date:      "2026-05-04",
		StartTime: "09:00",
		EndTime:   "09:15",
		RRule:     "FREQ=DAILY;COUNT=3",
		Kind:      "meeting",
	}
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, []granitmeta.Event{ev}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar?from=2026-05-04&to=2026-05-06", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("calendar: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Events []calendarEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, ce := range out.Events {
		if ce.EventID == "rec-1" {
			count++
			if ce.Kind != "meeting" {
				t.Errorf("occurrence %s lost Kind; got %q", ce.Date, ce.Kind)
			}
		}
	}
	if count != 3 {
		t.Errorf("want 3 occurrences, got %d", count)
	}
}

// Untyped events: Kind on the wire is the empty string (Go's
// omitempty drops it from JSON entirely, so the JS sees
// `kind: undefined`). Verify the JSON doesn't carry a stale
// kind from leakage.
func TestCalendarFeed_OmitsKind_WhenUnset(t *testing.T) {
	s, h := calendarTestServer(t)

	ev := granitmeta.Event{
		ID:        "evt-untyped",
		Title:     "untyped",
		Date:      "2026-05-09",
		StartTime: "11:00",
		EndTime:   "11:15",
		// Kind intentionally unset.
	}
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, []granitmeta.Event{ev}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar?from=2026-05-09&to=2026-05-09", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("calendar: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The omitempty tag on calendarEvent.Kind should keep "kind"
	// out of the JSON for events without one. Verify directly on
	// the wire bytes rather than the decoded shape, since decoding
	// a missing field is indistinguishable from an empty string.
	if want := `"kind":`; bodyContainsForEvent(body, "evt-untyped", want) {
		t.Errorf(`untyped event should NOT serialise a "kind" key; body=%s`, body)
	}
}

// bodyContainsForEvent does a coarse "does the JSON for the named
// event contain this substring" check by finding the eventId span
// + searching backward to the prior `{`. Cheap; covers the
// "did `kind` get serialised on THIS event" question without
// needing a real JSON walker.
func bodyContainsForEvent(body, eventID, needle string) bool {
	marker := `"eventId":"` + eventID + `"`
	idx := indexOf(body, marker)
	if idx < 0 {
		return false
	}
	start := lastIndexBefore(body, idx, '{')
	end := indexAfter(body, idx, '}')
	if start < 0 || end < 0 {
		return false
	}
	return indexOf(body[start:end], needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func lastIndexBefore(s string, end int, ch byte) int {
	for i := end - 1; i >= 0; i-- {
		if s[i] == ch {
			return i
		}
	}
	return -1
}
func indexAfter(s string, start int, ch byte) int {
	for i := start; i < len(s); i++ {
		if s[i] == ch {
			return i
		}
	}
	return -1
}
