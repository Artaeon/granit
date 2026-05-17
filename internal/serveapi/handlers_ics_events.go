package serveapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// chiURLParamDecoded reads a chi URL parameter and percent-decodes
// it. Chi v5 returns the raw (still-encoded) form when the URL
// contains percent escapes anywhere in the path — so a UID like
// "wu-vienna-project@daily-structure" comes through as
// "wu-vienna-project%40daily-structure". The handler then compares
// the encoded string against the decoded UID stored in the .ics
// file → silent mismatch + the diagnostic "event not found" 404.
//
// PathUnescape is forgiving: malformed escapes (lone "%") return an
// error, in which case we fall back to the raw value rather than
// dropping the request. The match path's strings.TrimSpace covers
// any whitespace introduced by an odd encoder.
func chiURLParamDecoded(r *http.Request, name string) string {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return ""
	}
	if dec, err := url.PathUnescape(raw); err == nil {
		return dec
	}
	return raw
}

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
	// Kind round-trips granit's event-type extension. Empty string
	// clears the X-GRANIT-KIND line (the writer skips empty Kind).
	Kind *string `json:"kind,omitempty"`
}

// findICSSource matches a source name case-insensitively, with or
// without the .ics suffix. Returns nil when nothing matches; the caller
// distinguishes 404 vs 403 based on Writable.
//
// Prefers writable matches when the same filename exists in multiple
// roots (vault root + <vault>/calendars/, say — a common shape when a
// Sync app drops .ics files at the root AND the user has copies under
// the writable directory). Previously the first match won by
// iteration order, so the read-only vault-root copy shadowed the
// writable one and PATCH/DELETE returned 403 "calendar is read-only"
// even though the user clearly intended to edit the writable copy.
// Writable-first resolves that without breaking the pure-read-only
// case (no writable copy exists → falls through to the read-only
// match → caller emits a clear 403).
func (s *Server) findICSSource(name string) *icsSource {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".ics") {
		lower += ".ics"
	}
	var fallback *icsSource
	for _, src := range icsListSources(s.cfg.Vault.Root) {
		if strings.ToLower(src.Source) != lower {
			continue
		}
		if src.Writable {
			return &src
		}
		if fallback == nil {
			s2 := src
			fallback = &s2
		}
	}
	return fallback
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
	// ExDates is the verbatim list of RFC 5545 EXDATE values (one
	// "skip" entry per cancelled occurrence). Preserved on round-
	// trip so a series the user "Skip this occurrence"'d through
	// us doesn't lose the EXDATE when we rewrite the file.
	ExDates []string
	// Kind is granit's event-type extension surfaced via the
	// X-GRANIT-KIND custom property. Preserved on round-trip so a
	// `meeting`/`focus`/`personal`/etc. tag survives PATCH + DELETE.
	// Empty string for un-typed (generic) events.
	Kind string
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
			// Mirror parseICSFile's TrimSpace so the round-trip-view
			// matches the read-side UID byte-for-byte. The strict ==
			// compare in the patch/delete handlers compares trimmed
			// values too, but normalising here means subsequent
			// writes through icswriter also emit the canonical form.
			cur.UID = strings.TrimSpace(val)
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
		case "X-GRANIT-KIND":
			// Granit's event-type extension. Stored verbatim and
			// round-tripped through Extra-less, so a `meeting`
			// tag set in the web UI survives a TUI edit + a
			// PATCH round-trip without ending up in Extra (where
			// it'd be re-emitted as the raw line, fine, but the
			// frontend would lose access to it via the .Kind field).
			cur.Kind = strings.TrimSpace(val)
		case "EXDATE":
			// 5545 §3.8.5.1: EXDATE may pack multiple comma-separated
			// values on one line. Preserve them verbatim in the
			// record so the writer can re-emit the same wire shape;
			// the comparison logic in the calendar feed
			// (parseICSFile + isExcluded) already normalises.
			for _, v := range strings.Split(val, ",") {
				v = strings.TrimSpace(v)
				if v != "" {
					cur.ExDates = append(cur.ExDates, v)
				}
			}
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
		ExDates:      r.ExDates,
		Kind:         r.Kind,
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
	if body.Kind != nil {
		// Trim + lowercase the wire value so a stray space or
		// "Meeting" capitalisation lands in a canonical form on
		// disk. The frontend's allowlist is lowercase; unknown
		// values pass through but display as "generic".
		rec.Kind = strings.ToLower(strings.TrimSpace(*body.Kind))
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

// parseClientTime accepts any of the time shapes the calendar wire
// emits: zoned/UTC RFC3339 ("2026-05-17T09:00:00Z" or "+02:00"),
// floating timed ("2026-05-17T09:00:00" or "2026-05-17T09:00", no zone),
// or all-day ("2026-05-17"). AllDay forces the date-only branch.
//
// Floating timed values are critical for ICS round-trip: the calendar
// feed emits them WITHOUT a zone for events whose DTSTART had no TZID
// and no Z (RFC 5545 §3.3.5 floating time). A previous narrower parser
// rejected those, so any "skip this occurrence" gesture on a floating
// recurring event 400'd and the user was nudged into "delete entire
// series" — wiping all instances. Floating values parse as UTC so the
// wall-clock digits round-trip losslessly.
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
	// Floating WITH seconds (the feed's emitted shape for floating ICS
	// occurrences). Parse in UTC to preserve the literal digits — the
	// downstream EXDATE / overrides path matches by wall-clock string.
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.UTC); err == nil {
		return t, nil
	}
	// Floating without seconds — minute-precision form sometimes used
	// by simpler clients.
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, time.UTC); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339, floating ISO, or YYYY-MM-DD, got %q", s)
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
	src := s.requireWritableICS(w, chiURLParamDecoded(r, "source"))
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
	src := s.requireWritableICS(w, chiURLParamDecoded(r, "source"))
	if src == nil {
		return
	}
	uid := strings.TrimSpace(chiURLParamDecoded(r, "uid"))
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
			// TrimSpace on the stored UID too: some inbound .ics files
			// (Apple Calendar, certain sync apps) emit "UID: foo@bar"
			// with a stray leading space the parser stored verbatim.
			// Strict == match silently failed for those events on edit
			// — the user saw "ics event not found" with no way to tell
			// what was wrong. Tolerant match resolves it without
			// rewriting the source file.
			if strings.TrimSpace(records[i].UID) == uid {
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
		writeError(w, http.StatusNotFound, fmt.Sprintf("event not found: uid=%q in %s (try refreshing — the calendar file may have been re-synced)", uid, src.Source))
		return
	}
	s.broadcastICSChange(*src)
	writeJSON(w, http.StatusOK, recordToCRUDResponse(updated))
}

// handleDeleteICSEvent removes a VEVENT by UID.
func (s *Server) handleDeleteICSEvent(w http.ResponseWriter, r *http.Request) {
	src := s.requireWritableICS(w, chiURLParamDecoded(r, "source"))
	if src == nil {
		return
	}
	uid := strings.TrimSpace(chiURLParamDecoded(r, "uid"))
	if uid == "" {
		writeError(w, http.StatusBadRequest, "uid required")
		return
	}
	found := false
	err := s.rewriteICS(*src, func(records []icsRecord) ([]icsRecord, error) {
		out := records[:0]
		for _, r := range records {
			// Same tolerant match as PATCH — see the note there.
			if strings.TrimSpace(r.UID) == uid {
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
		writeError(w, http.StatusNotFound, fmt.Sprintf("event not found: uid=%q in %s (try refreshing — the calendar file may have been re-synced)", uid, src.Source))
		return
	}
	s.broadcastICSChange(*src)
	w.WriteHeader(http.StatusNoContent)
}

// handleSkipICSOccurrence adds an EXDATE entry to a recurring ICS
// event so a single occurrence is excluded from the rendered series.
// This is the ICS counterpart of events.json's /events/{id}/skip —
// the user's "cancel this one Tuesday standup" gesture without
// touching the rest of the series.
//
// Body: { "date": "YYYY-MM-DDTHH:MM:SSZ" } (timed) or
//
//	{ "date": "YYYY-MM-DD" } (all-day)
//
// The value is converted to 5545's compact ICS-time form
// (YYYYMMDDTHHMMSSZ or YYYYMMDD) before being appended to ExDates.
// Idempotent: appending the same date twice yields one EXDATE entry
// in the rewritten file (the writer dedups).
func (s *Server) handleSkipICSOccurrence(w http.ResponseWriter, r *http.Request) {
	src := s.requireWritableICS(w, chiURLParamDecoded(r, "source"))
	if src == nil {
		return
	}
	uid := strings.TrimSpace(chiURLParamDecoded(r, "uid"))
	if uid == "" {
		writeError(w, http.StatusBadRequest, "uid required")
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Date = strings.TrimSpace(body.Date)
	if body.Date == "" {
		writeError(w, http.StatusBadRequest, "date required")
		return
	}
	// Use the unified parseClientTime so floating timed shapes are
	// accepted alongside RFC3339 and all-day forms. Previously this
	// path used a narrower parser that rejected floating values, so
	// users got 400 errors on "skip this one" for any recurring event
	// whose DTSTART had no zone — and were funnelled into the
	// destructive "delete entire series" path instead.
	t, err := parseClientTime(body.Date, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "date must be RFC3339, floating ISO, or YYYY-MM-DD")
		return
	}
	var ics string
	if len(body.Date) == 10 {
		ics = t.Format("20060102")
	} else {
		// Stored as UTC wall-clock — the in-memory isExcluded check
		// matches against the same shape (t.UTC().Format("...")) and
		// expandRRULE produces the same shape on floating instances.
		ics = t.UTC().Format("20060102T150405Z")
	}

	var updated icsRecord
	found := false
	var notRecurring bool
	err := s.rewriteICS(*src, func(records []icsRecord) ([]icsRecord, error) {
		for i := range records {
			if strings.TrimSpace(records[i].UID) != uid {
				continue
			}
			if records[i].RRULE == "" {
				notRecurring = true
				return records, nil
			}
			records[i].ExDates = append(records[i].ExDates, ics)
			records[i].Sequence++
			records[i].DTStamp = time.Now().UTC()
			updated = records[i]
			found = true
			return records, nil
		}
		return records, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		if notRecurring {
			writeError(w, http.StatusBadRequest, "event is not recurring — use DELETE for a single VEVENT")
			return
		}
		writeError(w, http.StatusNotFound, fmt.Sprintf("event not found: uid=%q in %s", uid, src.Source))
		return
	}
	s.broadcastICSChange(*src)
	writeJSON(w, http.StatusOK, recordToCRUDResponse(updated))
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
	kind := r.Kind
	out.Kind = &kind
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
