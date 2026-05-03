package serveapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/icswriter"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// icsTestServer mounts only the calendar-source + ICS-event routes so
// the fixture is small (no auth / tasks / daily required for these
// CRUD paths).
func icsTestServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	s := &Server{
		cfg: Config{Vault: v, Logger: slog.Default()},
		hub: wshub.New(slog.Default()),
	}
	r := chi.NewRouter()
	r.Get("/api/v1/calendar/sources", s.handleListCalendarSources)
	r.Post("/api/v1/calendars", s.handleCreateCalendar)
	r.Post("/api/v1/calendars/{source}/events", s.handleCreateICSEvent)
	r.Patch("/api/v1/calendars/{source}/events/{uid}", s.handlePatchICSEvent)
	r.Delete("/api/v1/calendars/{source}/events/{uid}", s.handleDeleteICSEvent)
	return s, r, root
}

func icsDoJSON(t *testing.T, h http.Handler, method, path string, body interface{}) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

// TestICSWritable_403OnReadOnly verifies the read-only path stays
// read-only — a calendar dropped at vault root is NOT writable, and
// any event mutation against it must 403. This is the contract the
// task description calls out explicitly.
func TestICSWritable_403OnReadOnly(t *testing.T) {
	_, h, root := icsTestServer(t)

	// Plant a read-only .ics at vault root (NOT under calendars/).
	readonly := filepath.Join(root, "readonly.ics")
	if err := icswriter.WriteFile(readonly, icswriter.CalendarMeta{Name: "readonly"}, nil); err != nil {
		t.Fatal(err)
	}

	// POST event to readonly source — must be 403.
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/readonly.ics/events", map[string]interface{}{
		"summary": "should fail",
		"start":   "2026-05-10T10:00:00Z",
	})
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 on read-only calendar, got %d: %s", code, body)
	}
}

// TestICSWritable_404OnMissing verifies a non-existent source 404s
// (not 403) so the client can distinguish "you typo'd the name" from
// "this is a remote subscription".
func TestICSWritable_404OnMissing(t *testing.T) {
	_, h, _ := icsTestServer(t)
	code, _ := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/ghost.ics/events", map[string]interface{}{
		"summary": "x",
		"start":   "2026-05-10T10:00:00Z",
	})
	if code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", code)
	}
}

// TestICS_CreateCalendar_AndEventRoundTrip exercises the full local
// path: create calendar → create event → patch → delete. After each
// step we re-parse the file with the existing reader (parseICSFile)
// and assert the on-disk truth.
func TestICS_CreateCalendar_AndEventRoundTrip(t *testing.T) {
	_, h, root := icsTestServer(t)

	// 1. Create calendar.
	code, body := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars", map[string]interface{}{
		"name":         "test",
		"display_name": "Test Cal",
	})
	if code != http.StatusCreated {
		t.Fatalf("create calendar: status=%d body=%s", code, body)
	}
	icsPath := filepath.Join(root, "calendars", "test.ics")
	if _, err := os.Stat(icsPath); err != nil {
		t.Fatalf("expected file at %s: %v", icsPath, err)
	}

	// Duplicate-create should 409.
	code2, _ := icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars", map[string]interface{}{"name": "test"})
	if code2 != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate, got %d", code2)
	}

	// 2. Create event.
	code, body = icsDoJSON(t, h, http.MethodPost, "/api/v1/calendars/test.ics/events", map[string]interface{}{
		"summary":     "Lunch with Sam",
		"start":       "2026-05-15T12:00:00Z",
		"end":         "2026-05-15T13:00:00Z",
		"location":    "Cafe",
		"description": "agenda; bring laptop",
	})
	if code != http.StatusCreated {
		t.Fatalf("create event: status=%d body=%s", code, body)
	}
	var created icsEventCRUD
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.UID == "" {
		t.Fatal("expected auto-generated UID")
	}

	// Reader sees the event.
	events, err := parseICSFile(icsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Title != "Lunch with Sam" {
		t.Fatalf("expected 1 event 'Lunch with Sam', got %+v", events)
	}
	if events[0].Location != "Cafe" {
		t.Fatalf("location not preserved: %q", events[0].Location)
	}

	// 3. PATCH summary; verify SEQUENCE bumped on disk.
	newSum := "Dinner with Sam"
	code, body = icsDoJSON(t, h, http.MethodPatch, "/api/v1/calendars/test.ics/events/"+created.UID, map[string]interface{}{
		"summary": newSum,
	})
	if code != http.StatusOK {
		t.Fatalf("patch: status=%d body=%s", code, body)
	}
	recs, err := readICSRecords(icsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Summary != newSum {
		t.Fatalf("summary not updated: %q", recs[0].Summary)
	}
	if recs[0].Sequence != 1 {
		t.Fatalf("expected SEQUENCE=1 after patch, got %d", recs[0].Sequence)
	}

	// 4. DELETE.
	code, _ = icsDoJSON(t, h, http.MethodDelete, "/api/v1/calendars/test.ics/events/"+created.UID, nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete: status=%d", code)
	}
	events2, _ := parseICSFile(icsPath)
	if len(events2) != 0 {
		t.Fatalf("expected 0 events after delete, got %d", len(events2))
	}
}

// TestICS_RoundTrip_PreservesFields confirms the writer's output is
// re-readable with the existing parser — the contract the task
// description calls out for writer_test.go (we put it here so it can
// use parseICSFile without an import cycle).
func TestICS_RoundTrip_PreservesFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rt.ics")

	src := []icswriter.Event{{
		UID:         "rt-1@granit",
		Summary:     "Standup; daily",
		Start:       mustTime(t, "2026-05-15T09:00:00Z"),
		End:         mustTime(t, "2026-05-15T09:30:00Z"),
		Location:    "Zoom, room 4",
		Description: "agenda\nbring notes",
		RRULE:       "FREQ=DAILY;COUNT=10",
		Sequence:    2,
	}}
	if err := icswriter.WriteFile(path, icswriter.CalendarMeta{Name: "rt"}, src); err != nil {
		t.Fatal(err)
	}
	got, err := parseICSFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	g := got[0]
	if g.Title != src[0].Summary {
		t.Errorf("title mismatch: %q vs %q", g.Title, src[0].Summary)
	}
	if g.Location != src[0].Location {
		t.Errorf("location mismatch: %q vs %q", g.Location, src[0].Location)
	}
	if g.UID != src[0].UID {
		t.Errorf("uid mismatch: %q vs %q", g.UID, src[0].UID)
	}
	if g.RRule != src[0].RRULE {
		t.Errorf("rrule mismatch: %q vs %q", g.RRule, src[0].RRULE)
	}
	if !g.Start.Equal(src[0].Start) {
		t.Errorf("start mismatch: %v vs %v", g.Start, src[0].Start)
	}
	if !g.End.Equal(src[0].End) {
		t.Errorf("end mismatch: %v vs %v", g.End, src[0].End)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	got, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("mustTime: %v", err)
	}
	return got
}
