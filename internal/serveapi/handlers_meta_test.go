package serveapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"

	"github.com/go-chi/chi/v5"
)

// TestValidateEventTimes pins the boundary contract for the events
// API: only valid 24-hour HH:MM times pass through, only YYYY-MM-DD
// dates pass through, and the end-after-start rule is enforced when
// both bounds are set. Empty start/end is allowed (an "all-day-ish"
// event without explicit times) and end-only without start is
// silently accepted because granitmeta tolerates it on read.
func TestValidateEventTimes(t *testing.T) {
	cases := []struct {
		name           string
		date           string
		start, end     string
		wantErr        bool
		wantErrSubstr  string
	}{
		{name: "happy path with times", date: "2026-05-06", start: "09:00", end: "10:30"},
		{name: "happy path date only", date: "2026-05-06"},
		{name: "happy path edge midnight to 23:59", date: "2026-05-06", start: "00:00", end: "23:59"},
		{name: "bad date — month name", date: "May 6 2026", wantErr: true, wantErrSubstr: "YYYY-MM-DD"},
		{name: "bad date — slashes", date: "2026/05/06", wantErr: true, wantErrSubstr: "YYYY-MM-DD"},
		{name: "bad date — short", date: "26-5-6", wantErr: true, wantErrSubstr: "YYYY-MM-DD"},
		{name: "12-hour start with PM", date: "2026-05-06", start: "9:00 PM", end: "10:30", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "out-of-range hour", date: "2026-05-06", start: "25:00", end: "26:00", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "out-of-range minute", date: "2026-05-06", start: "10:60", end: "11:00", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "single-digit hour without leading zero", date: "2026-05-06", start: "9:00", end: "10:00", wantErr: true, wantErrSubstr: "HH:MM"},
		{name: "end before start", date: "2026-05-06", start: "14:00", end: "09:00", wantErr: true, wantErrSubstr: "after"},
		{name: "end equals start", date: "2026-05-06", start: "14:00", end: "14:00", wantErr: true, wantErrSubstr: "after"},
		{name: "end-only is allowed (paired with empty start)", date: "2026-05-06", end: "10:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateEventTimes(c.date, c.start, c.end)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErrSubstr != "" && err != nil {
				if !contains(err.Error(), c.wantErrSubstr) {
					t.Errorf("error %q missing substring %q", err.Error(), c.wantErrSubstr)
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// metaTestServer is the minimal Server wiring needed to drive the
// events handlers — same pattern as financeTestServer, just without
// finance-specific deps. Returns the server + a router that mounts
// the events routes so URL params (chi.URLParam) resolve correctly.
func metaTestServer(t *testing.T) (*Server, http.Handler) {
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
	r.Get("/api/v1/events", s.handleListEvents)
	r.Post("/api/v1/events", s.handleCreateEvent)
	r.Patch("/api/v1/events/{id}", s.handlePatchEvent)
	r.Post("/api/v1/events/{id}/skip", s.handleSkipEventOccurrence)
	r.Post("/api/v1/events/{id}/override", s.handleOverrideEventOccurrence)
	r.Delete("/api/v1/events/{id}", s.handleDeleteEvent)
	r.Get("/api/v1/projects/{name}", s.handleGetProject)
	r.Post("/api/v1/projects", s.handleCreateProject)
	r.Patch("/api/v1/projects/{name}", s.handlePatchProject)
	r.Delete("/api/v1/projects/{name}", s.handleDeleteProject)
	return s, r
}

// TestProjects_NameWithSlash pins the URL-decoding contract for project
// path params. Project names are user content and may contain "/" — chi
// extracts the raw "%2F"-encoded segment without decoding, so handlers
// must call urlParam (which PathUnescapes) before lookup. Without that,
// a project named "client/web" round-trips a 404 on GET/PATCH/DELETE
// even though it sits on disk and the frontend correctly encoded the URL.
func TestProjects_NameWithSlash(t *testing.T) {
	_, h := metaTestServer(t)

	created := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(
		`{"name":"client/web","description":"site rebuild"}`))
	created.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	h.ServeHTTP(cw, created)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", cw.Code, cw.Body.String())
	}

	// GET — frontend encodes "client/web" → "client%2Fweb".
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/client%2Fweb", nil)
	gw := httptest.NewRecorder()
	h.ServeHTTP(gw, getReq)
	if gw.Code != http.StatusOK {
		t.Fatalf("get with %%2F-encoded slash: %d %s", gw.Code, gw.Body.String())
	}

	// PATCH the same project — was 404 before the fix.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/client%2Fweb",
		bytes.NewBufferString(`{"description":"updated"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	pw := httptest.NewRecorder()
	h.ServeHTTP(pw, patchReq)
	if pw.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", pw.Code, pw.Body.String())
	}

	// DELETE the same project — was 404 before the fix.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/client%2Fweb", nil)
	dw := httptest.NewRecorder()
	h.ServeHTTP(dw, delReq)
	if dw.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", dw.Code, dw.Body.String())
	}
}

// TestEvents_PerInstanceOverride pins the per-occurrence override
// path: a recurring event can have ONE Tuesday moved to Wednesday
// without rewriting the SERIES base — the override sits in
// Event.Overrides keyed by the original UTC anchor, and the
// calendar handler consults it during expansion.
func TestEvents_PerInstanceOverride(t *testing.T) {
	_, h := metaTestServer(t)

	// Helper: POST and parse the created event.
	create := func(body string) granitmeta.Event {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var ev granitmeta.Event
		if err := json.Unmarshal(w.Body.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		return ev
	}
	override := func(id, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+id+"/override", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("override: %d != %d; body=%s", w.Code, want, w.Body.String())
		}
		return w
	}

	// Create a daily-recurring event 09:00–10:00 starting 2026-03-02.
	ev := create(`{"title":"standup","date":"2026-03-02","start_time":"09:00","end_time":"10:00","rrule":"FREQ=DAILY;COUNT=10"}`)

	// Override 2026-03-04's instance to 11:00–12:00 same day.
	w := override(ev.ID,
		`{"key":"2026-03-04T08:00:00","override":{"start_time":"11:00","end_time":"12:00"}}`,
		http.StatusOK)
	var withOvr granitmeta.Event
	if err := json.Unmarshal(w.Body.Bytes(), &withOvr); err != nil {
		t.Fatal(err)
	}
	if len(withOvr.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(withOvr.Overrides))
	}
	o, ok := withOvr.Overrides["2026-03-04T08:00:00"]
	if !ok {
		t.Fatalf("override key missing; got %v", withOvr.Overrides)
	}
	if o.StartTime != "11:00" || o.EndTime != "12:00" {
		t.Errorf("override fields: %+v", o)
	}

	// SERIES base is unchanged — round-trip the GET. (We use the
	// router's list endpoint to peek at the on-disk record.)
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, listReq)
	var list struct {
		Events []granitmeta.Event `json:"events"`
	}
	if err := json.Unmarshal(lw.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(list.Events))
	}
	got := list.Events[0]
	if got.StartTime != "09:00" || got.EndTime != "10:00" {
		t.Errorf("series base mutated by override: start=%q end=%q want 09:00/10:00",
			got.StartTime, got.EndTime)
	}

	// Reject malformed override fields — same shape as the regular
	// patch validator. Bad time → 400.
	override(ev.ID,
		`{"key":"2026-03-05T08:00:00","override":{"start_time":"9 PM"}}`,
		http.StatusBadRequest)
	override(ev.ID,
		`{"key":"2026-03-06T08:00:00","override":{"start_time":"15:00","end_time":"14:00"}}`,
		http.StatusBadRequest)
	override(ev.ID,
		`{"key":"2026-03-07T08:00:00","override":{"date":"2026/03/07"}}`,
		http.StatusBadRequest)

	// Empty override for an existing key clears the entry. Round-
	// trip again to confirm the map shrank.
	override(ev.ID,
		`{"key":"2026-03-04T08:00:00","override":{}}`,
		http.StatusOK)
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	lw2 := httptest.NewRecorder()
	h.ServeHTTP(lw2, listReq2)
	var list2 struct {
		Events []granitmeta.Event `json:"events"`
	}
	if err := json.Unmarshal(lw2.Body.Bytes(), &list2); err != nil {
		t.Fatal(err)
	}
	if len(list2.Events[0].Overrides) != 0 {
		t.Errorf("empty override should clear: got %v", list2.Events[0].Overrides)
	}

	// Non-recurring event refuses overrides — overrides only make
	// sense for series. Create a one-off and verify 400.
	oneOff := create(`{"title":"one-off","date":"2026-04-01","start_time":"09:00","end_time":"10:00"}`)
	override(oneOff.ID,
		`{"key":"2026-04-01T08:00:00","override":{"start_time":"11:00"}}`,
		http.StatusBadRequest)

	// Re-overriding an already-overridden occurrence at the SAME key
	// must mutate the existing entry rather than minting a new one
	// at a shifted key. This is the contract the frontend's
	// override_key surfacing relies on: moveEvent passes the
	// canonical anchor key (not the rendered start), so multiple
	// drag-moves of the same Tuesday compose to one override entry.
	override(ev.ID,
		`{"key":"2026-03-05T08:00:00","override":{"start_time":"10:00","end_time":"11:00"}}`,
		http.StatusOK)
	override(ev.ID,
		`{"key":"2026-03-05T08:00:00","override":{"start_time":"13:00","end_time":"14:00"}}`,
		http.StatusOK)
	listReq3 := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	lw3 := httptest.NewRecorder()
	h.ServeHTTP(lw3, listReq3)
	var list3 struct {
		Events []granitmeta.Event `json:"events"`
	}
	if err := json.Unmarshal(lw3.Body.Bytes(), &list3); err != nil {
		t.Fatal(err)
	}
	if len(list3.Events[0].Overrides) != 1 {
		t.Errorf("expected 1 override after double-write at same key, got %d", len(list3.Events[0].Overrides))
	}
	if got := list3.Events[0].Overrides["2026-03-05T08:00:00"]; got.StartTime != "13:00" {
		t.Errorf("override didn't update on second write: %+v", got)
	}
}

// TestEvents_DragEdges locks the boundary the user hit with "drag make
// it longer drag it somewhere else it completely buggy and places it
// somewhere else": events.json carries a single date plus HH:MM
// start/end, so an event whose end falls on the next calendar day
// can't be represented and validateEventTimes refuses it. The frontend
// now clamps end_time to 23:59 to keep the move on-day; this test
// pins the BACKEND contract that triggers the clamp need: end MUST
// be > start, equality is rejected, and a happy-path drag-move PATCH
// (date + start + end shifted together) round-trips cleanly.
func TestEvents_DragEdges(t *testing.T) {
	_, h := metaTestServer(t)

	// Helper: POST /api/v1/events with a JSON body, return the created event.
	create := func(body string) granitmeta.Event {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create: got %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var ev granitmeta.Event
		if err := json.Unmarshal(w.Body.Bytes(), &ev); err != nil {
			t.Fatalf("decode create: %v", err)
		}
		return ev
	}
	// Helper: PATCH /api/v1/events/{id} with a JSON body, asserting expectedStatus.
	patch := func(id, body string, expectedStatus int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/events/"+id, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != expectedStatus {
			t.Fatalf("patch: got %d, want %d; body=%s", w.Code, expectedStatus, w.Body.String())
		}
		return w
	}

	// Happy-path drag-move: original 11:00–12:00 on 2026-05-09, drag to
	// 14:30 same day. Frontend sends date + start_time + end_time all
	// at once (which is exactly what +page.svelte's moveEvent emits).
	// Must round-trip with the new times intact.
	created := create(`{"title":"team sync","date":"2026-05-09","start_time":"11:00","end_time":"12:00"}`)
	moved := patch(created.ID, `{"date":"2026-05-09","start_time":"14:30","end_time":"15:30"}`, http.StatusOK)
	var movedEv granitmeta.Event
	if err := json.Unmarshal(moved.Body.Bytes(), &movedEv); err != nil {
		t.Fatal(err)
	}
	if movedEv.StartTime != "14:30" || movedEv.EndTime != "15:30" {
		t.Errorf("after drag-move: start=%q end=%q want 14:30/15:30", movedEv.StartTime, movedEv.EndTime)
	}

	// The frontend's old (pre-clamp) behavior: drag a 60-min event to
	// 23:30 produced end_time="00:30" (next-day wrap). The schema can't
	// represent that — backend MUST refuse. This is the contract the
	// frontend clamp now respects; if validateEventTimes ever silently
	// accepts an end <= start, the clamp loses its safety net.
	patch(created.ID, `{"start_time":"23:30","end_time":"00:30"}`, http.StatusBadRequest)

	// Same shape via resize: original 11:00 start, push end to "10:59"
	// (resize that would make end < start) — must 400.
	patch(created.ID, `{"end_time":"10:59"}`, http.StatusBadRequest)

	// Edge of the day: an event ending at 23:59 must round-trip — the
	// clamp on the frontend snaps end-time HERE, so the boundary case
	// has to stay accepted forever.
	clamped := patch(created.ID, `{"start_time":"23:00","end_time":"23:59"}`, http.StatusOK)
	var clampedEv granitmeta.Event
	if err := json.Unmarshal(clamped.Body.Bytes(), &clampedEv); err != nil {
		t.Fatal(err)
	}
	if clampedEv.StartTime != "23:00" || clampedEv.EndTime != "23:59" {
		t.Errorf("end-of-day clamp landing: got %q–%q want 23:00–23:59", clampedEv.StartTime, clampedEv.EndTime)
	}

	// Cross-day move on the date field alone — moving a Wed event to
	// Thursday with the same times must work. (The user reported drag
	// "places it somewhere else" — partly the midnight bug above, but
	// also we want to lock that a clean date+times PATCH lands on the
	// new date with no shenanigans.)
	moved2 := patch(created.ID, `{"date":"2026-05-10","start_time":"09:00","end_time":"10:00"}`, http.StatusOK)
	var movedEv2 granitmeta.Event
	if err := json.Unmarshal(moved2.Body.Bytes(), &movedEv2); err != nil {
		t.Fatal(err)
	}
	if movedEv2.Date != "2026-05-10" || movedEv2.StartTime != "09:00" || movedEv2.EndTime != "10:00" {
		t.Errorf("cross-day drag: got date=%q start=%q end=%q want 2026-05-10/09:00/10:00",
			movedEv2.Date, movedEv2.StartTime, movedEv2.EndTime)
	}

	// All-day event drag-move: empty start/end times must round-trip
	// without acquiring a time. Without this, dragging an all-day event
	// would accidentally turn it into a timed event at 00:00.
	allday := create(`{"title":"vacation","date":"2026-06-01"}`)
	movedAD := patch(allday.ID, `{"date":"2026-06-02"}`, http.StatusOK)
	var movedADEv granitmeta.Event
	if err := json.Unmarshal(movedAD.Body.Bytes(), &movedADEv); err != nil {
		t.Fatal(err)
	}
	if movedADEv.Date != "2026-06-02" || movedADEv.StartTime != "" || movedADEv.EndTime != "" {
		t.Errorf("all-day drag: date=%q start=%q end=%q want 2026-06-02//", movedADEv.Date, movedADEv.StartTime, movedADEv.EndTime)
	}
}
