package serveapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/artaeon/granit/internal/icswriter"
	"github.com/artaeon/granit/internal/wshub"
)

// icsEventCRUD is the wire-side shape for create/patch on a writable
// .ics calendar. Times are RFC3339 (matches events.json + the calendar
// feed) — we convert to UTC before serializing into the .ics. AllDay
// drops the time-of-day component and uses VALUE=DATE.
//
// All fields optional on PATCH. Pointers distinguish "not provided" from
// "explicitly cleared" (empty string clears Location/Description/RRULE).
type icsEventCRUD struct {
	UID         string  `json:"uid,omitempty"`
	Summary     *string `json:"summary,omitempty"`
	Start       *string `json:"start,omitempty"` // RFC3339
	End         *string `json:"end,omitempty"`   // RFC3339
	AllDay      *bool   `json:"allDay,omitempty"`
	Location    *string `json:"location,omitempty"`
	Description *string `json:"description,omitempty"`
	RRULE       *string `json:"rrule,omitempty"`
}

// findICSSource matches a source name case-insensitively, with or
// without the .ics suffix. Returns nil when nothing matches; the caller
// distinguishes 404 vs 403 based on Writable.
func (s *Server) findICSSource(name string) *icsSource {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".ics") {
		lower += ".ics"
	}
	for _, src := range icsListSources(s.cfg.Vault.Root) {
		if strings.ToLower(src.Source) == lower {
			return &src
		}
	}
	return nil
}

// icsRecord is the round-trip-aware view of a VEVENT — richer than
// icsEvent (the read-side calendar struct) because we need to preserve
// fields the read path doesn't surface (DESCRIPTION, SEQUENCE, DTSTAMP)
// when re-emitting unchanged events. Anything we don't recognise is
// preserved verbatim in Extra to minimise the chance of dropping a
// 5545 field on round-trip.
type icsRecord struct {
	UID          string
	Summary      string
	Start        time.Time
	End          time.Time
	AllDay       bool
	Location     string
	Description  string
	RRULE        string
	Sequence     int
	DTStamp      time.Time
	RecurrenceID string
	// Extra carries verbatim "KEY:VALUE" lines we didn't model — emitted
	// back into the VEVENT block on round-trip so a custom X-MOZ-* or
	// CATEGORIES doesn't get silently dropped.
	Extra []string
}

// readICSRecords parses a writable .ics file into the round-trip-aware
// icsRecord slice. Tolerant: unknown fields land in Extra, malformed
// times collapse to zero (the writer skips zero-DTSTART events).
//
// This is a write-path-only parser — the read path keeps the smaller
// icsEvent + parseICSFile in ics.go untouched, since the calendar feed
// doesn't need DTSTAMP / SEQUENCE / DESCRIPTION.
func readICSRecords(path string) ([]icsRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Unfold (continuation lines start with space/tab — 5545 §3.1).
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r\n")
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
		} else {
			lines = append(lines, l)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	var out []icsRecord
	var cur *icsRecord
	in := false
	for _, line := range lines {
		switch line {
		case "BEGIN:VEVENT":
			in = true
			cur = &icsRecord{}
			continue
		case "END:VEVENT":
			if in && cur != nil {
				out = append(out, *cur)
			}
			in = false
			cur = nil
			continue
		}
		if !in || cur == nil {
			continue
		}
		key, val := splitKV(line)
		base, params := splitParams(key)
		switch base {
		case "UID":
			cur.UID = val
		case "SUMMARY":
			cur.Summary = unescape(val)
		case "LOCATION":
			cur.Location = unescape(val)
		case "DESCRIPTION":
			cur.Description = unescape(val)
		case "RRULE":
			cur.RRULE = val
		case "SEQUENCE":
			if n, err := strconv.Atoi(val); err == nil {
				cur.Sequence = n
			}
		case "DTSTAMP":
			if t, _, ok := parseICSTime(val, params["TZID"]); ok {
				cur.DTStamp = t
			}
		case "DTSTART":
			if t, allDay, ok := parseICSTime(val, params["TZID"]); ok {
				cur.Start = t
				cur.AllDay = allDay
			}
		case "DTEND":
			if t, _, ok := parseICSTime(val, params["TZID"]); ok {
				cur.End = t
			}
		case "RECURRENCE-ID":
			cur.RecurrenceID = val
		default:
			cur.Extra = append(cur.Extra, line)
		}
	}
	return out, nil
}

// readICSMeta extracts the VCALENDAR-level NAME / X-WR-CALNAME / PRODID
// from an existing file so we can preserve them on rewrite. Anything
// missing falls through to icswriter defaults.
func readICSMeta(path string) (icswriter.CalendarMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return icswriter.CalendarMeta{}, err
	}
	defer func() { _ = f.Close() }()
	meta := icswriter.CalendarMeta{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	inEvent := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		switch line {
		case "BEGIN:VEVENT":
			inEvent = true
			continue
		case "END:VEVENT":
			inEvent = false
			continue
		}
		if inEvent {
			continue
		}
		key, val := splitKV(line)
		base, _ := splitParams(key)
		switch base {
		case "PRODID":
			meta.ProdID = val
		case "NAME":
			meta.Name = unescape(val)
		case "X-WR-CALNAME":
			meta.DisplayName = unescape(val)
		}
	}
	return meta, nil
}

// recordToWriterEvent converts our round-trip view into the writer's
// Event shape. Extra lines are dropped here — we'd need to extend the
// writer to round-trip them, and our 80%-case scope deliberately
// doesn't. Extra is preserved on read but only matters if a future
// write path opts in to passing it through.
func recordToWriterEvent(r icsRecord) icswriter.Event {
	return icswriter.Event{
		UID:          r.UID,
		Summary:      r.Summary,
		Start:        r.Start,
		End:          r.End,
		AllDay:       r.AllDay,
		Location:     r.Location,
		Description:  r.Description,
		RRULE:        r.RRULE,
		Sequence:     r.Sequence,
		DTStamp:      r.DTStamp,
		RecurrenceID: r.RecurrenceID,
	}
}

// rewriteICS reads, mutates, then writes back the .ics file
// atomically. mutate runs against the in-memory slice; returning an
// error aborts the write. This is the single chokepoint for every
// VEVENT mutation so concurrency / atomicity / WS broadcast all live
// in one place.
func (s *Server) rewriteICS(src icsSource, mutate func(records []icsRecord) ([]icsRecord, error)) error {
	records, err := readICSRecords(src.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	meta, err := readICSMeta(src.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated, err := mutate(records)
	if err != nil {
		return err
	}
	out := make([]icswriter.Event, 0, len(updated))
	for _, r := range updated {
		out = append(out, recordToWriterEvent(r))
	}
	return icswriter.WriteFile(src.Path, meta, out)
}

// applyCRUDToRecord takes a partial wire-side patch and mutates the
// record. Returns an error if any provided time fails to parse — better
// to 400 than silently keep the old start.
func applyCRUDToRecord(rec *icsRecord, body icsEventCRUD) error {
	if body.Summary != nil {
		rec.Summary = *body.Summary
	}
	if body.Location != nil {
		rec.Location = *body.Location
	}
	if body.Description != nil {
		rec.Description = *body.Description
	}
	if body.RRULE != nil {
		rec.RRULE = *body.RRULE
	}
	if body.AllDay != nil {
		rec.AllDay = *body.AllDay
	}
	if body.Start != nil && *body.Start != "" {
		t, err := parseClientTime(*body.Start, rec.AllDay)
		if err != nil {
			return fmt.Errorf("start: %w", err)
		}
		rec.Start = t
	}
	if body.End != nil && *body.End != "" {
		t, err := parseClientTime(*body.End, rec.AllDay)
		if err != nil {
			return fmt.Errorf("end: %w", err)
		}
		rec.End = t
	}
	return nil
}

// parseClientTime accepts either an RFC3339 timestamp (timed event) or
// a YYYY-MM-DD all-day date. AllDay forces the latter.
func parseClientTime(s string, allDay bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if allDay || len(s) == 10 {
		// YYYY-MM-DD — anchor at local midnight; the writer renders as
		// VALUE=DATE so the time-of-day component is dropped anyway.
		return time.ParseInLocation("2006-01-02", s, time.Local)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD, got %q", s)
}

func (s *Server) requireWritableICS(w http.ResponseWriter, source string) *icsSource {
	src := s.findICSSource(source)
	if src == nil {
		writeError(w, http.StatusNotFound, "calendar not found")
		return nil
	}
	if !src.Writable {
		writeError(w, http.StatusForbidden, "calendar is read-only (move to <vault>/calendars/ to enable writes)")
		return nil
	}
	return src
}

// handleCreateICSEvent creates a new VEVENT in the named writable
// calendar. UID is auto-generated when missing; SEQUENCE starts at 0.
func (s *Server) handleCreateICSEvent(w http.ResponseWriter, r *http.Request) {
	src := s.requireWritableICS(w, chi.URLParam(r, "source"))
	if src == nil {
		return
	}
	var body icsEventCRUD
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Summary == nil || strings.TrimSpace(*body.Summary) == "" {
		writeError(w, http.StatusBadRequest, "summary required")
		return
	}
	if body.Start == nil || *body.Start == "" {
		writeError(w, http.StatusBadRequest, "start required")
		return
	}
	now := time.Now().UTC()
	uid := strings.TrimSpace(body.UID)
	if uid == "" {
		uid = strings.ToLower(ulid.Make().String()) + "@granit"
	}
	rec := icsRecord{
		UID:      uid,
		Sequence: 0,
		DTStamp:  now,
	}
	if err := applyCRUDToRecord(&rec, body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := s.rewriteICS(*src, func(records []icsRecord) ([]icsRecord, error) {
		// Reject UID collisions — the user accidentally re-POSTing should
		// 409 instead of silently shadowing an existing event. No matching
		// UID? Append.
		for _, ex := range records {
			if ex.UID == rec.UID {
				return nil, fmt.Errorf("uid already exists: %s", rec.UID)
			}
		}
		return append(records, rec), nil
	})
	if err != nil {
		// Distinguish UID-collision (409) from generic write failures.
		if strings.HasPrefix(err.Error(), "uid already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastICSChange(*src)
	writeJSON(w, http.StatusCreated, recordToCRUDResponse(rec))
}

// handlePatchICSEvent applies a partial update to a VEVENT identified
// by its UID. SEQUENCE bumps by one; DTSTAMP refreshes — both are
// required for downstream calendar clients to accept the modification.
func (s *Server) handlePatchICSEvent(w http.ResponseWriter, r *http.Request) {
	src := s.requireWritableICS(w, chi.URLParam(r, "source"))
	if src == nil {
		return
	}
	uid := chi.URLParam(r, "uid")
	if uid == "" {
		writeError(w, http.StatusBadRequest, "uid required")
		return
	}
	var body icsEventCRUD
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var updated icsRecord
	found := false
	err := s.rewriteICS(*src, func(records []icsRecord) ([]icsRecord, error) {
		for i := range records {
			if records[i].UID == uid {
				if err := applyCRUDToRecord(&records[i], body); err != nil {
					return nil, err
				}
				records[i].Sequence++
				records[i].DTStamp = time.Now().UTC()
				updated = records[i]
				found = true
				return records, nil
			}
		}
		return records, nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	s.broadcastICSChange(*src)
	writeJSON(w, http.StatusOK, recordToCRUDResponse(updated))
}

// handleDeleteICSEvent removes a VEVENT by UID.
func (s *Server) handleDeleteICSEvent(w http.ResponseWriter, r *http.Request) {
	src := s.requireWritableICS(w, chi.URLParam(r, "source"))
	if src == nil {
		return
	}
	uid := chi.URLParam(r, "uid")
	if uid == "" {
		writeError(w, http.StatusBadRequest, "uid required")
		return
	}
	found := false
	err := s.rewriteICS(*src, func(records []icsRecord) ([]icsRecord, error) {
		out := records[:0]
		for _, r := range records {
			if r.UID == uid {
				found = true
				continue
			}
			out = append(out, r)
		}
		return out, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	s.broadcastICSChange(*src)
	w.WriteHeader(http.StatusNoContent)
}

// broadcastICSChange notifies subscribers that the .ics file mutated so
// the calendar page reloads. Path is vault-relative + slash-normalised
// (matches the note.changed shape the watcher already broadcasts).
func (s *Server) broadcastICSChange(src icsSource) {
	rel, err := filepath.Rel(s.cfg.Vault.Root, src.Path)
	if err != nil {
		rel = src.Source
	}
	s.hub.Broadcast(wshub.Event{
		Type: "state.changed",
		Path: filepath.ToSlash(rel),
	})
}

// recordToCRUDResponse renders the round-trip view back as the wire
// shape so create/patch responses echo what the client sent.
func recordToCRUDResponse(r icsRecord) icsEventCRUD {
	out := icsEventCRUD{UID: r.UID}
	allDay := r.AllDay
	out.AllDay = &allDay
	sum := r.Summary
	out.Summary = &sum
	loc := r.Location
	out.Location = &loc
	desc := r.Description
	out.Description = &desc
	rule := r.RRULE
	out.RRULE = &rule
	if !r.Start.IsZero() {
		var s string
		if r.AllDay {
			s = r.Start.Format("2006-01-02")
		} else {
			s = r.Start.Format(time.RFC3339)
		}
		out.Start = &s
	}
	if !r.End.IsZero() {
		var e string
		if r.AllDay {
			e = r.End.Format("2006-01-02")
		} else {
			e = r.End.Format(time.RFC3339)
		}
		out.End = &e
	}
	return out
}
