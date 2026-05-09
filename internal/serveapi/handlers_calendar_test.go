package serveapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"

	"github.com/go-chi/chi/v5"
)

// TestExpandAllDayDates pins the multi-day all-day expansion contract.
// Locks: inclusive start, exclusive end (ICS DTEND semantics), the
// no-end fallback to a single day, and the [from, rangeEnd) window
// clamp so events extending outside the requested calendar window are
// trimmed correctly.
func TestExpandAllDayDates(t *testing.T) {
	d := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		name        string
		start, end  time.Time
		from, until time.Time
		want        []string
	}{
		{
			name:  "ten-day vacation fully inside window",
			start: d(2026, 8, 1), end: d(2026, 8, 11), // ICS DTEND exclusive → 10 days
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{
				"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04", "2026-08-05",
				"2026-08-06", "2026-08-07", "2026-08-08", "2026-08-09", "2026-08-10",
			},
		},
		{
			name:  "single-day event with explicit DTEND = start + 24h",
			start: d(2026, 8, 1), end: d(2026, 8, 2),
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01"},
		},
		{
			name:  "no DTEND falls back to one day",
			start: d(2026, 8, 1), end: time.Time{},
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01"},
		},
		{
			name:  "DTEND <= start defends against malformed ICS",
			start: d(2026, 8, 1), end: d(2026, 7, 31),
			from: d(2026, 7, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01"},
		},
		{
			name:  "trip spans the request window — left clamp",
			start: d(2026, 7, 28), end: d(2026, 8, 5),
			from: d(2026, 8, 1), until: d(2026, 9, 1),
			want: []string{"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04"},
		},
		{
			name:  "trip spans the request window — right clamp",
			start: d(2026, 7, 28), end: d(2026, 8, 5),
			from: d(2026, 7, 1), until: d(2026, 8, 1),
			want: []string{"2026-07-28", "2026-07-29", "2026-07-30", "2026-07-31"},
		},
		{
			name:  "trip entirely outside window",
			start: d(2026, 1, 1), end: d(2026, 1, 5),
			from: d(2026, 8, 1), until: d(2026, 9, 1),
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := expandAllDayDates(c.start, c.end, c.from, c.until)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v\nwant %v", got, c.want)
			}
		})
	}
}

// TestOverrideKey pins the canonical key shape for Event.Overrides
// against the EXDATE format used by isExcluded — both are populated
// from the same UTC-stamp shape so the override and skip paths can
// safely point at the same recurrence anchor without translation.
func TestOverrideKey(t *testing.T) {
	timed := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	if got := overrideKey(timed, false); got != "2026-03-04T09:00:00" {
		t.Errorf("timed key: got %q want 2026-03-04T09:00:00", got)
	}
	allDay := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	if got := overrideKey(allDay, true); got != "2026-03-04" {
		t.Errorf("all-day key: got %q want 2026-03-04", got)
	}
	// Non-UTC input must still produce a UTC-flavoured key — the
	// expander emits times in time.Local on systems where DTSTART
	// carries a TZID, but the key contract stays UTC for parity
	// with how parseICSFile stores EXDATE.
	loc, _ := time.LoadLocation("Europe/Vienna")
	if loc != nil {
		eu := time.Date(2026, 3, 4, 11, 0, 0, 0, loc) // 10:00 UTC in winter, 09:00 in summer
		got := overrideKey(eu, false)
		want := eu.UTC().Format("2006-01-02T15:04:05")
		if got != want {
			t.Errorf("non-UTC input: got %q want %q", got, want)
		}
	}
}

// TestApplyTimedOverride pins the (start, end) transform contract:
//   - StartTime alone preserves duration (drag-move UX)
//   - StartTime + EndTime sets both wall-clock independently (drag-resize)
//   - Date alone shifts the day, time stays put
//   - Empty override is a no-op
//
// Carrier zone is UTC because events.json stores HH:MM as floating
// wall-clock and the calendar pipeline now treats UTC as the zone-free
// frame end-to-end (see comment on applyTimedOverride). Switching from
// time.Local in 2026-05 fixed a +offset drift on non-UTC servers.
func TestApplyTimedOverride(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 3, 4, 9, 0, 0, 0, loc)
	end := time.Date(2026, 3, 4, 10, 0, 0, 0, loc) // 1h duration

	cases := []struct {
		name              string
		ovr               granitmeta.EventOverride
		wantStart, wantEnd time.Time
	}{
		{
			name:      "empty override is no-op",
			ovr:       granitmeta.EventOverride{},
			wantStart: start,
			wantEnd:   end,
		},
		{
			name:      "start only — duration preserved",
			ovr:       granitmeta.EventOverride{StartTime: "11:00"},
			wantStart: time.Date(2026, 3, 4, 11, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 4, 12, 0, 0, 0, loc),
		},
		{
			name:      "start + end — explicit duration",
			ovr:       granitmeta.EventOverride{StartTime: "14:00", EndTime: "16:30"},
			wantStart: time.Date(2026, 3, 4, 14, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 4, 16, 30, 0, 0, loc),
		},
		{
			name:      "date only — same time, different day",
			ovr:       granitmeta.EventOverride{Date: "2026-03-05"},
			wantStart: time.Date(2026, 3, 5, 9, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 5, 10, 0, 0, 0, loc),
		},
		{
			name:      "date + start — drag-move to new day & time",
			ovr:       granitmeta.EventOverride{Date: "2026-03-05", StartTime: "13:30"},
			wantStart: time.Date(2026, 3, 5, 13, 30, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 5, 14, 30, 0, 0, loc),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotS, gotE := applyTimedOverride(start, end, c.ovr)
			if !gotS.Equal(c.wantStart) {
				t.Errorf("start: got %v, want %v", gotS, c.wantStart)
			}
			if !gotE.Equal(c.wantEnd) {
				t.Errorf("end: got %v, want %v", gotE, c.wantEnd)
			}
		})
	}
}

// calendarTestServer wires the minimum Server surface needed to drive
// handleCalendar end-to-end (events.json round-trip + GET /calendar).
// Mirrors metaTestServer's shape but mounts the calendar route too.
func calendarTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent { return nil })
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.Default()
	s := &Server{
		cfg: Config{
			Vault:     v,
			TaskStore: store,
			Daily:     daily.DailyConfig{Template: daily.DefaultConfig().Template},
			Logger:    logger,
		},
		hub: wshub.New(logger),
	}
	r := chi.NewRouter()
	r.Get("/api/v1/calendar", s.handleCalendar)
	return s, r
}

// TestCalendar_FloatingWallClockEmit pins the timezone-floating contract
// for native events.json. Reproduces the user-reported regression: on a
// UTC server with a UTC+2 client, an event entered at 08:00–16:00 was
// rendered at 10:00–18:00 because the server (1) parsed the stored
// wall-clock as time.Local (UTC) and (2) emitted with time.RFC3339
// ("...Z"), which the browser then re-converted to local time, adding
// the +2hr offset on top of numbers that were never zoned to begin with.
//
// The fix: events.json carries wall-clock numbers (no zone). Parse them
// in time.UTC (treat as zone-free) and emit a floating ISO string
// without the trailing Z so `new Date(...)` in the browser parses it
// as the client's local zone — 08:00 in JSON → 08:00 on the grid,
// regardless of server or client offset.
//
// The test simulates a UTC+2 browser by re-parsing the emit in a
// FixedZone(+02:00) and asserting the wall-clock hour is the same
// number the user typed. A developer running on a UTC machine sees
// the same assertion fire either way: the contract is "wall-clock
// digits survive intact through the emit", not "the digits match
// the developer's local zone".
func TestCalendar_FloatingWallClockEmit(t *testing.T) {
	s, h := calendarTestServer(t)

	// Persist an 08:00–16:00 event the way the API would have written it.
	ev := granitmeta.Event{
		ID:        "evt-floating",
		Title:     "Deep work",
		Date:      "2026-05-09",
		StartTime: "08:00",
		EndTime:   "16:00",
	}
	if err := granitmeta.WriteEvents(s.cfg.Vault.Root, []granitmeta.Event{ev}); err != nil {
		t.Fatalf("WriteEvents: %v", err)
	}

	// Hit GET /api/v1/calendar like the web client would.
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
		if out.Events[i].EventID == "evt-floating" {
			got = &out.Events[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("event not found in feed; got %d events", len(out.Events))
	}
	if got.Start == nil || got.End == nil {
		t.Fatalf("expected start+end timestamps, got %+v", got)
	}

	// Contract 1: emitted strings must be floating — no zone designator.
	// `Z` (UTC), `+HH:MM` (offset) both make the browser anchor the time
	// to a specific instant, then re-render in client-local — which is
	// the round-trip that produced the +2hr drift.
	for _, s := range []string{*got.Start, *got.End} {
		if endsWith(s, "Z") {
			t.Errorf("emit must not be UTC-flavoured (trailing Z): %q", s)
		}
		if hasOffset(s) {
			t.Errorf("emit must not carry a numeric offset: %q", s)
		}
	}

	// Contract 2: simulate `new Date(start)` running in a UTC+2 browser.
	// A floating ISO string ("2026-05-09T08:00:00") is parsed by JS
	// engines as the client's local zone. We model that by parsing the
	// string in a +02:00 location and asserting the wall-clock hour is
	// the same number the user typed — 8, not 10.
	vienna := time.FixedZone("UTC+2", 2*60*60)
	startInBrowser, err := time.ParseInLocation("2006-01-02T15:04:05", *got.Start, vienna)
	if err != nil {
		t.Fatalf("client-side parse failed; the emit format isn't floating: %v\nvalue=%q", err, *got.Start)
	}
	if h := startInBrowser.Hour(); h != 8 {
		t.Errorf("UTC+2 browser sees start hour %d, want 8 (the wall-clock the user typed)", h)
	}
	endInBrowser, err := time.ParseInLocation("2006-01-02T15:04:05", *got.End, vienna)
	if err != nil {
		t.Fatalf("client-side end parse failed: %v\nvalue=%q", err, *got.End)
	}
	if h := endInBrowser.Hour(); h != 16 {
		t.Errorf("UTC+2 browser sees end hour %d, want 16", h)
	}
}

// endsWith / hasOffset are local helpers used by the floating-emit test.
// Kept inline (not pulled into utils) so the test's intent is obvious
// at the call site.
func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
func hasOffset(s string) bool {
	// A floating ISO string ends in seconds (digit). RFC3339 with
	// offset ends in "+HH:MM" or "-HH:MM" — we look for the sign in
	// the last 6 chars to avoid matching the "-" inside the date.
	if len(s) < 6 {
		return false
	}
	tail := s[len(s)-6:]
	return (tail[0] == '+' || tail[0] == '-') && tail[3] == ':'
}
