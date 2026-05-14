package serveapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/icswriter"
)

// Real-world UID shapes seen in the wild: Google Calendar bakes
// "@google.com" into every UID; Apple Calendar uses UUIDs; some
// sync apps put account separators ("@daily-structure" was the
// user's actual production case). Drive each through PATCH +
// DELETE to make sure the URL-decoded handler resolves them all.
func icsFullRouter(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	s, r, root := icsTestServer(t)
	// The base test router doesn't include the skip route — wire
	// it here so the scope-picker e2e flow works too.
	rr := r.(*chi.Mux)
	rr.Post("/api/v1/calendars/{source}/events/{uid}/skip", s.handleSkipICSOccurrence)
	return s, r, root
}

// TestICS_EditFlow_NonRecurring covers the user's "I can't edit a
// one-off ICS event" complaint: write a single VEVENT with an @ in
// the UID, PATCH the summary, confirm the file actually changed +
// SEQUENCE bumped.
func TestICS_EditFlow_NonRecurring(t *testing.T) {
	cases := []struct {
		name string
		uid  string
	}{
		{"google-style", "_abc123def@google.com"},
		{"user-prod", "wu-vienna-project@daily-structure"},
		{"apple-style", "0E5C20E8-9A2C-4F7A-A412-C7DD8C2A5B3D"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, h, root := icsFullRouter(t)
			if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "calendars", "work.ics")
			if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "work"}, []icswriter.Event{{
				UID:     tc.uid,
				Summary: "Original",
				Start:   mustTime(t, "2026-05-15T09:00:00Z"),
				End:     mustTime(t, "2026-05-15T10:00:00Z"),
			}}); err != nil {
				t.Fatal(err)
			}

			// PATCH the summary via the encoded URL — same path the
			// frontend's encodeURIComponent produces.
			enc := url.PathEscape(tc.uid)
			code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/work.ics/events/"+enc, map[string]interface{}{
				"summary": "Edited",
			})
			if code != http.StatusOK {
				t.Fatalf("PATCH: status=%d body=%s", code, body)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if !strings.Contains(text, "SUMMARY:Edited") {
				t.Errorf("summary not updated on disk:\n%s", text)
			}
			if strings.Contains(text, "SUMMARY:Original") {
				t.Errorf("old summary still present:\n%s", text)
			}
			if !strings.Contains(text, "SEQUENCE:1") {
				t.Errorf("SEQUENCE not bumped:\n%s", text)
			}
			// UID survived round-trip exactly. icswriter preserves it
			// verbatim so a downstream calendar client doesn't see a
			// new UID and mis-attribute the edit.
			if !strings.Contains(text, "UID:"+tc.uid) {
				t.Errorf("UID changed in round-trip; expected %q in file:\n%s", tc.uid, text)
			}

			// DELETE through the same encoded path — must clear it.
			code, _ = icsDoJSON(t, h, http.MethodDelete, "/api/v1/calendars/work.ics/events/"+enc, nil)
			if code != http.StatusNoContent {
				t.Fatalf("DELETE: status=%d", code)
			}
			raw2, _ := os.ReadFile(path)
			if strings.Contains(string(raw2), "BEGIN:VEVENT") {
				t.Errorf("VEVENT still in file after DELETE:\n%s", string(raw2))
			}
		})
	}
}

// TestICS_EditFlow_RecurringSeries verifies the "edit entire
// series" path: PATCH the base VEVENT, RRULE must survive, every
// occurrence the expander emits picks up the change.
func TestICS_EditFlow_RecurringSeries(t *testing.T) {
	_, h, root := icsFullRouter(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "weekly.ics")
	uid := "weekly-standup@granit"
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "weekly"}, []icswriter.Event{{
		UID:     uid,
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=4",
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape(uid)
	code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/weekly.ics/events/"+enc, map[string]interface{}{
		"summary": "Team Sync",
	})
	if code != http.StatusOK {
		t.Fatalf("PATCH: status=%d body=%s", code, body)
	}
	on := readFileT(t, path)
	if !strings.Contains(on, "SUMMARY:Team Sync") {
		t.Errorf("summary not updated:\n%s", on)
	}
	if !strings.Contains(on, "RRULE:FREQ=WEEKLY;COUNT=4") {
		t.Errorf("RRULE lost on series edit:\n%s", on)
	}
}

// TestICS_EditFlow_ThisOccurrenceOnly drives the full UI flow the
// frontend uses when the user picks "Just this occurrence" on a
// recurring ICS event:
//
//	1. POST /skip with the original date — adds EXDATE to series.
//	2. POST /events       — creates a standalone VEVENT for the
//	                        edited instance in the same .ics file.
//
// After both, the file should contain: (a) original series with
// EXDATE for the skipped date, (b) one new VEVENT with the edited
// title/time, (c) both retain their respective UIDs.
func TestICS_EditFlow_ThisOccurrenceOnly(t *testing.T) {
	_, h, root := icsFullRouter(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "team.ics")
	seriesUID := "team-standup@granit"
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "team"}, []icswriter.Event{{
		UID:     seriesUID,
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=4",
	}}); err != nil {
		t.Fatal(err)
	}

	// Step 1: skip the May-11 occurrence.
	enc := url.PathEscape(seriesUID)
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/team.ics/events/"+enc+"/skip", map[string]interface{}{
		"date": "2026-05-11T09:00:00Z",
	})
	if code != http.StatusOK {
		t.Fatalf("skip: status=%d body=%s", code, body)
	}

	// Step 2: create a standalone VEVENT for the edited occurrence.
	code, body = icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/team.ics/events", map[string]interface{}{
		"summary":  "Team Sync (special)",
		"start":    "2026-05-11T10:00:00Z",
		"end":      "2026-05-11T11:00:00Z",
		"location": "Big Room",
	})
	if code != http.StatusCreated {
		t.Fatalf("create standalone: status=%d body=%s", code, body)
	}

	// Verify the file:
	//   - Original series intact with RRULE
	//   - EXDATE for the May-11 date
	//   - New standalone VEVENT for the rescheduled meeting
	on := readFileT(t, path)
	if !strings.Contains(on, "RRULE:FREQ=WEEKLY;COUNT=4") {
		t.Errorf("series RRULE missing:\n%s", on)
	}
	if !strings.Contains(on, "EXDATE:20260511T090000Z") {
		t.Errorf("EXDATE line missing or wrong shape:\n%s", on)
	}
	if !strings.Contains(on, "SUMMARY:Team Sync (special)") {
		t.Errorf("standalone replacement VEVENT missing:\n%s", on)
	}
	if !strings.Contains(on, "LOCATION:Big Room") {
		t.Errorf("standalone replacement lost location:\n%s", on)
	}
	// Series + standalone should be two BEGIN:VEVENT blocks.
	if got := strings.Count(on, "BEGIN:VEVENT"); got != 2 {
		t.Errorf("expected 2 VEVENT blocks (series + standalone), got %d. file:\n%s", got, on)
	}
}

// TestICS_EditFlow_RecurringExpansionHonoursEXDATE confirms the
// occurrence expander filters the skipped instance out — the
// observable effect of the EXDATE in the previous test.
func TestICS_EditFlow_RecurringExpansionHonoursEXDATE(t *testing.T) {
	_, _, root := icsFullRouter(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "weekly.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "weekly"}, []icswriter.Event{{
		UID:     "weekly-1@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=4",
		ExDates: []string{"20260511T090000Z"},
	}}); err != nil {
		t.Fatal(err)
	}

	events, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 base event, got %d", len(events))
	}
	base := events[0]
	if _, ok := base.ExDates["2026-05-11T09:00:00"]; !ok {
		t.Errorf("ExDates map missing the May-11 entry: %#v", base.ExDates)
	}
	// expandRRULE between May 4 and May 25 should produce 3 instances
	// (4 in the COUNT - 1 EXDATE'd) — May 4, May 18, May 25. May 11
	// is filtered.
	from := mustTime(t, "2026-05-04T00:00:00Z")
	to := mustTime(t, "2026-05-25T23:59:59Z")
	out := expandRRULE(base, from, to)
	if len(out) != 3 {
		t.Errorf("want 3 occurrences after EXDATE, got %d", len(out))
		for i, ev := range out {
			t.Logf("  %d: %s", i, ev.Start.Format("2006-01-02"))
		}
	}
	for _, ev := range out {
		if ev.Start.Format("2006-01-02") == "2026-05-11" {
			t.Errorf("May-11 occurrence not filtered — EXDATE didn't take effect")
		}
	}
}

// TestICS_EditFlow_UIDWithAtSign_FullRoundTrip is the regression
// guard for the user's actual reported failure: a UID containing
// "@" and the encoded form arriving at the handler. Pre-fix this
// raised 404 "event not found"; post-fix the PATCH succeeds.
func TestICS_EditFlow_UIDWithAtSign_FullRoundTrip(t *testing.T) {
	_, h, root := icsFullRouter(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "work.ics")
	uid := "wu-vienna-project@daily-structure"
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "work"}, []icswriter.Event{{
		UID:     uid,
		Summary: "Project Sync",
		Start:   mustTime(t, "2026-05-13T11:00:00Z"),
		End:     mustTime(t, "2026-05-13T15:00:00Z"),
	}}); err != nil {
		t.Fatal(err)
	}

	// JavaScript's encodeURIComponent encodes @ to %40 (Go's
	// url.PathEscape leaves @ untouched because RFC 3986 allows it
	// as a sub-delim in path segments). The user's actual URL had
	// %40 in it, so hand-encode here to reproduce production behavior.
	enc := strings.ReplaceAll(uid, "@", "%40")
	if !strings.Contains(enc, "%40") {
		t.Fatalf("test setup: hand-encode of @ failed; got %q", enc)
	}
	code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/work.ics/events/"+enc, map[string]interface{}{
		"start": "2026-05-13T13:00:00Z",
		"end":   "2026-05-13T17:00:00Z",
	})
	if code != http.StatusOK {
		t.Fatalf("PATCH: status=%d body=%s", code, body)
	}
	on := readFileT(t, path)
	if !strings.Contains(on, "DTSTART:20260513T130000Z") {
		t.Errorf("start not updated to 13:00:\n%s", on)
	}
	if !strings.Contains(on, "DTEND:20260513T170000Z") {
		t.Errorf("end not updated to 17:00:\n%s", on)
	}
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
