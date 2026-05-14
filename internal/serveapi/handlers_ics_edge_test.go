package serveapi

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/icswriter"
)

// Mount the FULL ICS route surface for end-to-end edge tests:
// create / patch / delete / skip. Tests below drive every path
// through the real HTTP layer + the real on-disk file so the
// regression catches drift in any segment of the round-trip.
func icsTestServerFull(t *testing.T) (http.Handler, string) {
	t.Helper()
	s, h, root := icsTestServer(t)
	rr := h.(*chi.Mux)
	rr.Post("/api/v1/calendars/{source}/events/{uid}/skip", s.handleSkipICSOccurrence)
	return h, root
}

// ---------------------------------------------------------------------
// All-day events. The previous test surface only exercised timed
// events; all-day events use VALUE=DATE encoding + a different
// end-day convention (DTEND exclusive). Catch any drift in that
// encoding through create + patch + delete.
// ---------------------------------------------------------------------

func TestICS_AllDayEvent_CreateAndDelete(t *testing.T) {
	h, root := icsTestServerFull(t)

	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars", map[string]interface{}{
		"name": "vacation",
	})
	if code != http.StatusCreated {
		t.Fatalf("create calendar: %d %s", code, body)
	}

	// All-day: send YYYY-MM-DD shape; allDay=true.
	code, body = icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/vacation.ics/events", map[string]interface{}{
		"summary": "Holiday",
		"start":   "2026-07-04",
		"end":     "2026-07-05",
		"allDay":  true,
	})
	if code != http.StatusCreated {
		t.Fatalf("create all-day: %d %s", code, body)
	}

	path := filepath.Join(root, "calendars", "vacation.ics")
	raw := readFileT(t, path)
	if !strings.Contains(raw, "DTSTART;VALUE=DATE:20260704") {
		t.Errorf("all-day DTSTART not in VALUE=DATE form:\n%s", raw)
	}
	if !strings.Contains(raw, "DTEND;VALUE=DATE:20260705") {
		t.Errorf("all-day DTEND not in VALUE=DATE form:\n%s", raw)
	}
	// SUMMARY survives.
	if !strings.Contains(raw, "SUMMARY:Holiday") {
		t.Errorf("SUMMARY missing:\n%s", raw)
	}

	// Reader sees one all-day event.
	events, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].AllDay {
		t.Fatalf("want 1 all-day event; got %+v", events)
	}
}

// ---------------------------------------------------------------------
// EXDATE idempotency. Skipping the same occurrence twice should
// NOT produce two EXDATE entries on disk — the user might re-fire
// the skip after a refresh-race + we shouldn't litter the file.
// ---------------------------------------------------------------------

func TestICS_Skip_IsIdempotent(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "w.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "w"}, []icswriter.Event{{
		UID:     "ww-1@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=10",
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape("ww-1@granit")
	for i := 0; i < 3; i++ {
		code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/w.ics/events/"+enc+"/skip", map[string]interface{}{
			"date": "2026-05-11T09:00:00Z",
		})
		if code != http.StatusOK {
			t.Fatalf("skip #%d: %d %s", i, code, body)
		}
	}
	// One EXDATE line; one EXDATE entry on it.
	raw := readFileT(t, path)
	if got := strings.Count(raw, "EXDATE"); got != 1 {
		t.Errorf("expected exactly one EXDATE line after triple-skip, got %d. file:\n%s", got, raw)
	}
	// Dedup is on the writer side, so the value list must not contain
	// the date twice.
	if strings.Count(raw, "20260511T090000Z") != 1 {
		t.Errorf("date should appear once in EXDATE, got %d. file:\n%s",
			strings.Count(raw, "20260511T090000Z"), raw)
	}
}

// ---------------------------------------------------------------------
// "Edit this occurrence only" preserves prior EXDATEs. The frontend
// fires skip + create-standalone; if a previous occurrence was
// already EXDATE'd (the user skipped Wednesday last week, now wants
// to edit Friday), the second skip must NOT drop the first one.
// ---------------------------------------------------------------------

func TestICS_Skip_PreservesPriorExDates(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "team.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "team"}, []icswriter.Event{{
		UID:     "t-1@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=10",
		ExDates: []string{"20260511T090000Z"}, // last week's skip
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape("t-1@granit")
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/team.ics/events/"+enc+"/skip", map[string]interface{}{
		"date": "2026-05-18T09:00:00Z",
	})
	if code != http.StatusOK {
		t.Fatalf("skip: %d %s", code, body)
	}

	raw := readFileT(t, path)
	if !strings.Contains(raw, "20260511T090000Z") {
		t.Errorf("previous EXDATE was dropped:\n%s", raw)
	}
	if !strings.Contains(raw, "20260518T090000Z") {
		t.Errorf("new EXDATE not added:\n%s", raw)
	}
}

// ---------------------------------------------------------------------
// Source name resolution: case-insensitive, with-or-without .ics
// suffix. The frontend has historically sent the source both ways
// depending on which API call; the resolver must accept both.
// ---------------------------------------------------------------------

func TestICS_SourceLookup_CaseAndSuffixInsensitive(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "MixedCase.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "MixedCase"}, []icswriter.Event{{
		UID:     "mc-1@granit",
		Summary: "Hi",
		Start:   mustTime(t, "2026-05-15T09:00:00Z"),
		End:     mustTime(t, "2026-05-15T09:30:00Z"),
	}}); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"MixedCase.ics",
		"mixedcase.ics",
		"MIXEDCASE.ICS",
		"MixedCase",
		"mixedcase",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			code, body := icsDoJSON(t, h, http.MethodPatch,
				"/api/v1/calendars/"+src+"/events/"+url.PathEscape("mc-1@granit"),
				map[string]interface{}{"summary": "Hi from " + src})
			if code != http.StatusOK {
				t.Fatalf("PATCH source=%s: %d %s", src, code, body)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Delete-as-skip: the frontend's recurring-delete flow calls the
// skip endpoint when the user picks "just this one" from the
// delete confirm. Asserts the contract end-to-end through HTTP.
// ---------------------------------------------------------------------

func TestICS_DeleteJustThisOne_RoutesViaSkip(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "w.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "w"}, []icswriter.Event{{
		UID:     "dd-1@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=4",
	}}); err != nil {
		t.Fatal(err)
	}

	// User picks "just this one" → frontend calls skip with the
	// rendered occurrence's anchor date.
	enc := url.PathEscape("dd-1@granit")
	code, _ := icsDoJSON(t, h, http.MethodPost,
		"/api/v1/calendars/w.ics/events/"+enc+"/skip",
		map[string]interface{}{"date": "2026-05-11T09:00:00Z"})
	if code != http.StatusOK {
		t.Fatalf("skip: %d", code)
	}

	// VEVENT block is still there; expansion now emits 3 occurrences.
	raw := readFileT(t, path)
	if strings.Count(raw, "BEGIN:VEVENT") != 1 {
		t.Errorf("expected the series VEVENT to survive a skip-as-delete:\n%s", raw)
	}
	if !strings.Contains(raw, "RRULE:FREQ=WEEKLY;COUNT=4") {
		t.Errorf("RRULE lost:\n%s", raw)
	}
}

// ---------------------------------------------------------------------
// Multi-occurrence "this only" workflow: skip three different
// occurrences from a five-week series. Each skip must accumulate
// (not replace) on the EXDATE list, and the standalone replacements
// land in the file alongside.
// ---------------------------------------------------------------------

func TestICS_MultipleSkipsAndReplacements(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "w.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "w"}, []icswriter.Event{{
		UID:     "wk@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"),
		RRULE:   "FREQ=WEEKLY;COUNT=5",
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape("wk@granit")
	dates := []string{
		"2026-05-11T09:00:00Z",
		"2026-05-18T09:00:00Z",
		"2026-05-25T09:00:00Z",
	}
	for i, d := range dates {
		code, body := icsDoJSON(t, h, http.MethodPost,
			"/api/v1/calendars/w.ics/events/"+enc+"/skip",
			map[string]interface{}{"date": d})
		if code != http.StatusOK {
			t.Fatalf("skip #%d (%s): %d %s", i, d, code, body)
		}
		// Standalone replacement at noon — represents the user's
		// edited "this only" version.
		code, body = icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/w.ics/events", map[string]interface{}{
			"summary":  fmt.Sprintf("Standup (moved %d)", i),
			"start":    strings.Replace(d, "T09", "T12", 1),
			"end":      strings.Replace(d, "T09", "T13", 1),
			"location": "Big Room",
		})
		if code != http.StatusCreated {
			t.Fatalf("create standalone #%d: %d %s", i, code, body)
		}
	}

	raw := readFileT(t, path)
	// Three EXDATE values on ONE line (or up to three lines depending
	// on writer batching — currently one line with comma-separated).
	for _, d := range []string{"20260511T090000Z", "20260518T090000Z", "20260525T090000Z"} {
		if !strings.Contains(raw, d) {
			t.Errorf("missing EXDATE value %s:\n%s", d, raw)
		}
	}
	// 1 series + 3 standalones = 4 VEVENT blocks.
	if got := strings.Count(raw, "BEGIN:VEVENT"); got != 4 {
		t.Errorf("expected 4 VEVENTs (series + 3 standalones), got %d. file:\n%s", got, raw)
	}
}

// ---------------------------------------------------------------------
// Delete the last event in a file. The .ics should still be valid
// (VCALENDAR header + footer survive) so the calendar source stays
// in the picker rather than disappearing.
// ---------------------------------------------------------------------

func TestICS_Delete_LastEvent_LeavesValidFile(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "single.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "single"}, []icswriter.Event{{
		UID:     "one@granit",
		Summary: "Solo",
		Start:   mustTime(t, "2026-05-15T09:00:00Z"),
		End:     mustTime(t, "2026-05-15T09:30:00Z"),
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape("one@granit")
	code, body := icsDoJSON(t, h, http.MethodDelete, "/api/v1/calendars/single.ics/events/"+enc, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, body)
	}

	raw := readFileT(t, path)
	if !strings.Contains(raw, "BEGIN:VCALENDAR") || !strings.Contains(raw, "END:VCALENDAR") {
		t.Errorf("VCALENDAR scaffold lost after deleting last event:\n%s", raw)
	}
	if strings.Contains(raw, "BEGIN:VEVENT") {
		t.Errorf("VEVENT should be gone:\n%s", raw)
	}
}

// ---------------------------------------------------------------------
// PATCH with empty body / unrelated fields = no-op on important
// state. The frontend's "save title only" edit path PATCHes with
// just `summary`; the writer must NOT touch DTSTART, RRULE, or
// other fields the user didn't change.
// ---------------------------------------------------------------------

func TestICS_PatchPreservesUntouchedFields(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "p.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "p"}, []icswriter.Event{{
		UID:      "p-1@granit",
		Summary:  "Original",
		Start:    mustTime(t, "2026-05-15T09:00:00Z"),
		End:      mustTime(t, "2026-05-15T10:00:00Z"),
		Location: "Old place",
		RRULE:    "FREQ=WEEKLY;COUNT=4",
		Kind:     "meeting",
	}}); err != nil {
		t.Fatal(err)
	}

	// Rename only.
	enc := url.PathEscape("p-1@granit")
	code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/p.ics/events/"+enc, map[string]interface{}{
		"summary": "Renamed",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, body)
	}

	raw := readFileT(t, path)
	// Summary updated.
	if !strings.Contains(raw, "SUMMARY:Renamed") || strings.Contains(raw, "SUMMARY:Original") {
		t.Errorf("summary not updated cleanly:\n%s", raw)
	}
	// Times untouched.
	if !strings.Contains(raw, "DTSTART:20260515T090000Z") {
		t.Errorf("DTSTART changed on summary-only patch:\n%s", raw)
	}
	if !strings.Contains(raw, "DTEND:20260515T100000Z") {
		t.Errorf("DTEND changed on summary-only patch:\n%s", raw)
	}
	// RRULE survives.
	if !strings.Contains(raw, "RRULE:FREQ=WEEKLY;COUNT=4") {
		t.Errorf("RRULE lost on summary-only patch:\n%s", raw)
	}
	// Location survives.
	if !strings.Contains(raw, "LOCATION:Old place") {
		t.Errorf("LOCATION lost on summary-only patch:\n%s", raw)
	}
	// Kind survives.
	if !strings.Contains(raw, "X-GRANIT-KIND:meeting") {
		t.Errorf("X-GRANIT-KIND lost on summary-only patch:\n%s", raw)
	}
}

// ---------------------------------------------------------------------
// Drag-resize "this only" via skip + create-standalone: the same
// flow that move uses. Different times — the standalone must
// carry the new end time, not the series end.
// ---------------------------------------------------------------------

func TestICS_ResizeJustThisOne_CreatesStandaloneWithNewEnd(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "w.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "w"}, []icswriter.Event{{
		UID:     "rs@granit",
		Summary: "Standup",
		Start:   mustTime(t, "2026-05-04T09:00:00Z"),
		End:     mustTime(t, "2026-05-04T09:30:00Z"), // 30 min
		RRULE:   "FREQ=WEEKLY;COUNT=4",
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape("rs@granit")
	// Skip May-11.
	code, _ := icsDoJSON(t, h, http.MethodPost,
		"/api/v1/calendars/w.ics/events/"+enc+"/skip",
		map[string]interface{}{"date": "2026-05-11T09:00:00Z"})
	if code != http.StatusOK {
		t.Fatalf("skip: %d", code)
	}
	// Create standalone with 90-min duration (the resize result).
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/w.ics/events", map[string]interface{}{
		"summary": "Standup",
		"start":   "2026-05-11T09:00:00Z",
		"end":     "2026-05-11T10:30:00Z", // 90 min
	})
	if code != http.StatusCreated {
		t.Fatalf("create standalone: %d %s", code, body)
	}

	raw := readFileT(t, path)
	// Series end stays at 09:30; standalone end is 10:30. Both
	// strings appear in the file.
	if !strings.Contains(raw, "DTEND:20260504T093000Z") {
		t.Errorf("series end shifted:\n%s", raw)
	}
	if !strings.Contains(raw, "DTEND:20260511T103000Z") {
		t.Errorf("standalone end not present:\n%s", raw)
	}
}

// ---------------------------------------------------------------------
// Create with the SAME UID twice → 409 Conflict. Without this guard
// a stale frontend re-POST silently shadows the original event.
// ---------------------------------------------------------------------

func TestICS_Create_DuplicateUID_409(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "w.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "w"}, []icswriter.Event{{
		UID:     "dup@granit",
		Summary: "First",
		Start:   mustTime(t, "2026-05-15T09:00:00Z"),
		End:     mustTime(t, "2026-05-15T09:30:00Z"),
	}}); err != nil {
		t.Fatal(err)
	}

	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/w.ics/events", map[string]interface{}{
		"uid":     "dup@granit",
		"summary": "Second (would shadow)",
		"start":   "2026-05-16T09:00:00Z",
		"end":     "2026-05-16T09:30:00Z",
	})
	if code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %d %s", code, body)
	}
}

// ---------------------------------------------------------------------
// X-GRANIT-KIND value survives across edits that don't touch it.
// Already covered partially in TestICS_PatchPreservesUntouchedFields;
// this drills in on the kind=undefined vs kind="" semantics.
// ---------------------------------------------------------------------

func TestICS_Kind_PatchPreservesWhenOmitted(t *testing.T) {
	h, root := icsTestServerFull(t)
	if err := os.MkdirAll(filepath.Join(root, "calendars"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "calendars", "k.ics")
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "k"}, []icswriter.Event{{
		UID:     "k@granit",
		Summary: "x",
		Start:   mustTime(t, "2026-05-15T09:00:00Z"),
		End:     mustTime(t, "2026-05-15T09:30:00Z"),
		Kind:    "focus",
	}}); err != nil {
		t.Fatal(err)
	}

	enc := url.PathEscape("k@granit")
	// Omit kind from PATCH body — must stay "focus".
	code, body := icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/k.ics/events/"+enc, map[string]interface{}{
		"summary": "renamed",
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d %s", code, body)
	}
	raw := readFileT(t, path)
	if !strings.Contains(raw, "X-GRANIT-KIND:focus") {
		t.Errorf("kind dropped when omitted from patch body:\n%s", raw)
	}

	// Explicit kind="" — must remove the line.
	code, body = icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/k.ics/events/"+enc, map[string]interface{}{
		"kind": "",
	})
	if code != http.StatusOK {
		t.Fatalf("patch clear: %d %s", code, body)
	}
	raw = readFileT(t, path)
	if strings.Contains(raw, "X-GRANIT-KIND:") {
		t.Errorf("kind line should be gone after kind=\"\" patch:\n%s", raw)
	}
}
