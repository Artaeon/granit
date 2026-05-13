package serveapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/icswriter"
)

// TestICS_SkipOccurrence_AppendsEXDATE drives the new
// /calendars/{source}/events/{uid}/skip endpoint end-to-end:
// plant a recurring event, fire skip with a date in the series,
// verify the .ics file gained an EXDATE line, and the recurring
// event still has its RRULE.
func TestICS_SkipOccurrence_AppendsEXDATE(t *testing.T) {
	srv, h, root := icsTestServer(t)
	// New route — register it on the test mux since icsTestServer
	// predates this endpoint and we don't want to spin up the
	// whole server harness.
	h2 := h.(interface {
		Post(pattern string, handler http.HandlerFunc)
	})
	h2.Post("/api/v1/calendars/{source}/events/{uid}/skip", srv.handleSkipICSOccurrence)

	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "weekly.ics")
	ev := icswriter.Event{
		UID:     "weekly-1@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=4",
	}
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "weekly"}, []icswriter.Event{ev}); err != nil {
		t.Fatal(err)
	}

	encoded := url.PathEscape("weekly-1@granit")
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/weekly.ics/events/"+encoded+"/skip", map[string]interface{}{
		"date": "2026-05-11T09:00:00Z",
	})
	if code != http.StatusOK {
		t.Fatalf("skip: status=%d body=%s", code, body)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	on := string(raw)
	if !strings.Contains(on, "EXDATE:20260511T090000Z") {
		t.Errorf("EXDATE line missing or wrong format. file:\n%s", on)
	}
	// RRULE must survive — skip is a NON-destructive edit on the
	// series.
	if !strings.Contains(on, "RRULE:FREQ=WEEKLY;COUNT=4") {
		t.Errorf("RRULE lost after skip:\n%s", on)
	}
	// SEQUENCE bumped from 0 to 1 so downstream calendars accept the
	// modification.
	if !strings.Contains(on, "SEQUENCE:1") {
		t.Errorf("SEQUENCE not bumped:\n%s", on)
	}
}

// TestICS_SkipOccurrence_RejectsNonRecurring covers the
// "user accidentally fires skip on a single VEVENT" case — should
// 400 with a clear message rather than silently appending an
// EXDATE that would never match anything.
func TestICS_SkipOccurrence_RejectsNonRecurring(t *testing.T) {
	srv, h, root := icsTestServer(t)
	h.(interface {
		Post(pattern string, handler http.HandlerFunc)
	}).Post("/api/v1/calendars/{source}/events/{uid}/skip", srv.handleSkipICSOccurrence)

	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "single.ics")
	ev := icswriter.Event{
		UID:     "single-1@granit",
		Summary: "One-off",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
	}
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "single"}, []icswriter.Event{ev}); err != nil {
		t.Fatal(err)
	}

	encoded := url.PathEscape("single-1@granit")
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/single.ics/events/"+encoded+"/skip", map[string]interface{}{
		"date": "2026-05-11T09:00:00Z",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-recurring event, got %d body=%s", code, body)
	}
	if !strings.Contains(string(body), "not recurring") {
		t.Errorf("error message should mention 'not recurring': %s", body)
	}
}
