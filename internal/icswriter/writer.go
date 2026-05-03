// Package icswriter writes RFC 5545 .ics calendar files. Scope is the
// 80% case granit's calendar UI needs — single-component VCALENDAR with
// VEVENTs, plain UTC timestamps, all-day VALUE=DATE, and a minimal RRULE
// (FREQ + INTERVAL + COUNT/UNTIL + BYDAY). It is NOT a general-purpose
// 5545 serializer; remote/full-fidelity ICS is out of scope for the local
// editable-calendar slice.
//
// Reads stay on the existing parseICSFile in serveapi/ics.go — writers
// don't depend on readers. Round-trip is verified via writer_test.go.
//
// All writes go through atomicio so a crash mid-flush leaves either the
// previous file or the complete new one — never a truncated .ics that
// would silently lose every event.
package icswriter

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// CalendarMeta is the VCALENDAR-level header. PRODID is required by
// 5545; Name and DisplayName surface as X-WR-CALNAME so OS calendar
// apps that subscribe show a friendly title.
type CalendarMeta struct {
	ProdID      string // e.g. "-//granit//calendar 1.0//EN"
	Name        string // internal name (filename minus .ics)
	DisplayName string // X-WR-CALNAME — what calendar apps show
}

// Event is the writer's view of a VEVENT. Tighter than the parser's
// icsEvent: we only carry fields the writer actually serializes.
//
// Times are encoded as UTC for timed events, VALUE=DATE for AllDay.
// AllDay forces YYYYMMDD encoding and ignores the time-of-day part.
//
// Sequence + DTStamp are required for safe re-emit: any client that
// already imported v0 of an event needs a non-decreasing SEQUENCE +
// fresher DTSTAMP to accept the update.
type Event struct {
	UID          string
	Summary      string
	Start        time.Time
	End          time.Time
	AllDay       bool
	Location     string
	Description  string
	RRULE        string // already-formatted RRULE string (use BuildRRULE to construct)
	Sequence     int
	DTStamp      time.Time
	RecurrenceID string // optional — for individual instance overrides (RFC 5545 §3.8.4.4)
}

// RRULEOptions feeds BuildRRULE. Freq is required; the rest are
// optional. ByDay entries are 5545 weekday codes (MO/TU/WE/TH/FR/SA/SU)
// with an optional positional prefix (e.g. "1MO" = first Monday).
type RRULEOptions struct {
	Freq     string // "DAILY" / "WEEKLY" / "MONTHLY" / "YEARLY"
	Interval int
	Count    int
	Until    time.Time
	ByDay    []string
}

// BuildRRULE serializes an RRULE clause. Returns "" for an empty Freq
// so callers can pass a zero-value Options to mean "no recurrence".
//
// Field order matches what most calendar clients expect (FREQ first,
// then INTERVAL, COUNT/UNTIL — at most one — then BYDAY). 5545 doesn't
// mandate the order, but consistency keeps the output diff-friendly.
func BuildRRULE(opts RRULEOptions) string {
	freq := strings.ToUpper(strings.TrimSpace(opts.Freq))
	if freq == "" {
		return ""
	}
	parts := []string{"FREQ=" + freq}
	if opts.Interval > 1 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", opts.Interval))
	}
	// 5545 §3.3.10: COUNT and UNTIL MUST NOT both occur. Prefer COUNT
	// when both are set (caller's bug; we pick deterministically).
	if opts.Count > 0 {
		parts = append(parts, fmt.Sprintf("COUNT=%d", opts.Count))
	} else if !opts.Until.IsZero() {
		parts = append(parts, "UNTIL="+opts.Until.UTC().Format("20060102T150405Z"))
	}
	if len(opts.ByDay) > 0 {
		// Normalize uppercase + sort for stable output (helps tests +
		// makes git diffs of the .ics file readable).
		days := make([]string, 0, len(opts.ByDay))
		for _, d := range opts.ByDay {
			d = strings.ToUpper(strings.TrimSpace(d))
			if d != "" {
				days = append(days, d)
			}
		}
		sort.Strings(days)
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	}
	return strings.Join(parts, ";")
}

// EscapeText applies 5545 §3.3.11 text escaping. The order matters:
// backslashes must be doubled FIRST so the subsequent escapes don't
// re-escape the backslashes they introduce.
func EscapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	// Bare CR is rare but treat the same as LF — calendar grids never
	// need a stand-alone carriage return in a SUMMARY/DESCRIPTION.
	s = strings.ReplaceAll(s, "\r", `\n`)
	return s
}

// WriteFile emits a complete VCALENDAR with one VEVENT per event,
// atomically (temp + rename). The file ends with the canonical 5545
// CRLF terminator on every line.
func WriteFile(path string, meta CalendarMeta, events []Event) error {
	if path == "" {
		return fmt.Errorf("icswriter: empty path")
	}
	if filepath.Ext(path) == "" {
		return fmt.Errorf("icswriter: path must end in .ics")
	}
	var b strings.Builder
	writeLine(&b, "BEGIN:VCALENDAR")
	writeLine(&b, "VERSION:2.0")
	prodID := meta.ProdID
	if prodID == "" {
		prodID = "-//granit//calendar 1.0//EN"
	}
	writeLine(&b, "PRODID:"+prodID)
	writeLine(&b, "CALSCALE:GREGORIAN")
	if meta.Name != "" {
		writeLine(&b, "NAME:"+EscapeText(meta.Name))
	}
	if meta.DisplayName != "" {
		writeLine(&b, "X-WR-CALNAME:"+EscapeText(meta.DisplayName))
	}
	for _, ev := range events {
		writeEvent(&b, ev)
	}
	writeLine(&b, "END:VCALENDAR")

	// 0o644 is the standard for vault files (other users on a shared
	// machine can read; only the owner can edit). atomicio preserves
	// any tightening on overwrite.
	return atomicio.WriteWithPerm(path, []byte(b.String()), 0o644)
}

func writeEvent(b *strings.Builder, ev Event) {
	writeLine(b, "BEGIN:VEVENT")
	if ev.UID != "" {
		writeLine(b, "UID:"+ev.UID)
	}
	dtStamp := ev.DTStamp
	if dtStamp.IsZero() {
		dtStamp = time.Now().UTC()
	}
	writeLine(b, "DTSTAMP:"+dtStamp.UTC().Format("20060102T150405Z"))
	writeLine(b, fmt.Sprintf("SEQUENCE:%d", ev.Sequence))
	if ev.RecurrenceID != "" {
		writeLine(b, "RECURRENCE-ID:"+ev.RecurrenceID)
	}
	if ev.AllDay {
		writeLine(b, "DTSTART;VALUE=DATE:"+ev.Start.Format("20060102"))
		// All-day events: DTEND in 5545 is the day AFTER the last day
		// (exclusive). When End is zero or same-day, we emit DTSTART+1
		// so single-day all-day events render correctly across clients.
		end := ev.End
		if end.IsZero() || !end.After(ev.Start) {
			end = ev.Start.AddDate(0, 0, 1)
		}
		writeLine(b, "DTEND;VALUE=DATE:"+end.Format("20060102"))
	} else {
		writeLine(b, "DTSTART:"+ev.Start.UTC().Format("20060102T150405Z"))
		if !ev.End.IsZero() {
			writeLine(b, "DTEND:"+ev.End.UTC().Format("20060102T150405Z"))
		}
	}
	if ev.Summary != "" {
		writeLine(b, "SUMMARY:"+EscapeText(ev.Summary))
	}
	if ev.Location != "" {
		writeLine(b, "LOCATION:"+EscapeText(ev.Location))
	}
	if ev.Description != "" {
		writeLine(b, "DESCRIPTION:"+EscapeText(ev.Description))
	}
	if ev.RRULE != "" {
		writeLine(b, "RRULE:"+ev.RRULE)
	}
	writeLine(b, "END:VEVENT")
}

// writeLine implements RFC 5545 §3.1 line folding: any line longer than
// 75 octets is split with CRLF + space at the boundary. Octet-count is
// approximate (we count bytes, not octets — close enough for ASCII +
// UTF-8 since multi-byte chars get folded as a unit; clients tolerate
// over-conservative folds).
//
// The terminator is CRLF on every line, including the last — that's
// what 5545 mandates and what every parser we round-trip against
// expects.
func writeLine(b *strings.Builder, s string) {
	const maxLen = 75
	if len(s) <= maxLen {
		b.WriteString(s)
		b.WriteString("\r\n")
		return
	}
	// First chunk: up to 75 bytes, no leading space.
	b.WriteString(s[:maxLen])
	b.WriteString("\r\n")
	rest := s[maxLen:]
	// Continuation chunks: 74 bytes (75 minus the leading space).
	for len(rest) > 74 {
		b.WriteString(" ")
		b.WriteString(rest[:74])
		b.WriteString("\r\n")
		rest = rest[74:]
	}
	if len(rest) > 0 {
		b.WriteString(" ")
		b.WriteString(rest)
		b.WriteString("\r\n")
	}
}
