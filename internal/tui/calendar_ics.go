package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// ICS Parsing
// ---------------------------------------------------------------------------

// icsEvent is an intermediate struct with recurrence info.
type icsEvent struct {
	CalendarEvent
	rrule string // raw RRULE value
}

func ParseICSFile(path string) ([]CalendarEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ics file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var parsed []icsEvent
	var current *icsEvent
	inEvent := false

	// Read all lines and unfold (ICS continuation lines start with space/tab)
	scanner := bufio.NewScanner(f)
	var rawLines []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(rawLines) > 0 {
			rawLines[len(rawLines)-1] += line[1:] // unfold
		} else {
			rawLines = append(rawLines, line)
		}
	}

	for _, line := range rawLines {

		if line == "BEGIN:VEVENT" {
			inEvent = true
			current = &icsEvent{}
			continue
		}

		if line == "END:VEVENT" {
			if inEvent && current != nil {
				parsed = append(parsed, *current)
			}
			inEvent = false
			current = nil
			continue
		}

		if !inEvent || current == nil {
			continue
		}

		key, value := icsKeyValue(line)
		baseProp := icsBaseProp(key)

		switch baseProp {
		case "SUMMARY":
			current.Title = value
		case "LOCATION":
			current.Location = value
		case "DTSTART":
			t, allDay, err := parseICSTime(value)
			if err != nil {
				continue
			}
			current.Date = t
			current.AllDay = allDay
		case "DTEND":
			t, _, err := parseICSTime(value)
			if err != nil {
				continue
			}
			current.EndDate = t
		case "RRULE":
			current.rrule = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ics file: %w", err)
	}

	// Expand recurring events for the next 90 days
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	horizon := today.AddDate(0, 3, 0)

	var events []CalendarEvent
	for _, ie := range parsed {
		if ie.rrule == "" {
			events = append(events, ie.CalendarEvent)
			continue
		}
		// Parse RRULE
		freq, interval, count, until := parseICSRRule(ie.rrule)
		if freq == "" {
			events = append(events, ie.CalendarEvent)
			continue
		}
		// Generate occurrences
		dur := ie.EndDate.Sub(ie.Date)
		d := ie.Date
		occurrences := 0
		for !d.After(horizon) {
			if count > 0 && occurrences >= count {
				break
			}
			if !until.IsZero() && d.After(until) {
				break
			}
			if !d.Before(today.AddDate(0, -1, 0)) {
				ev := ie.CalendarEvent
				ev.Date = time.Date(d.Year(), d.Month(), d.Day(),
					ie.Date.Hour(), ie.Date.Minute(), 0, 0, time.Local)
				ev.EndDate = ev.Date.Add(dur)
				events = append(events, ev)
			}
			occurrences++
			switch freq {
			case "DAILY":
				d = d.AddDate(0, 0, interval)
			case "WEEKLY":
				d = d.AddDate(0, 0, 7*interval)
			case "MONTHLY":
				d = d.AddDate(0, interval, 0)
			case "YEARLY":
				d = d.AddDate(interval, 0, 0)
			default:
				d = horizon.AddDate(0, 0, 1) // break
			}
		}
	}

	return events, nil
}

func parseICSRRule(rrule string) (freq string, interval int, count int, until time.Time) {
	interval = 1 // default interval
	for _, part := range strings.Split(rrule, ";") {
		switch {
		case strings.HasPrefix(part, "FREQ="):
			freq = strings.TrimPrefix(part, "FREQ=")
		case strings.HasPrefix(part, "INTERVAL="):
			if n, err := strconv.Atoi(strings.TrimPrefix(part, "INTERVAL=")); err == nil && n > 0 {
				interval = n
			}
		case strings.HasPrefix(part, "COUNT="):
			if n, err := strconv.Atoi(strings.TrimPrefix(part, "COUNT=")); err == nil && n > 0 {
				count = n
			}
		case strings.HasPrefix(part, "UNTIL="):
			val := strings.TrimPrefix(part, "UNTIL=")
			if t, _, err := parseICSTime(val); err == nil {
				until = t
			}
		}
	}
	return
}

func icsKeyValue(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], line[idx+1:]
}

func icsBaseProp(key string) string {
	if idx := strings.Index(key, ";"); idx >= 0 {
		return key[:idx]
	}
	return key
}

func parseICSTime(value string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	// UTC format: 20060102T150405Z
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t.Local(), false, nil
	}
	// Local format: 20060102T150405
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t, false, nil
	}
	// Date only: 20060102
	if t, err := time.Parse("20060102", value); err == nil {
		return t, true, nil
	}
	// ISO format: 2006-01-02T15:04:05Z
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local(), false, nil
	}
	// ISO without timezone: 2006-01-02T15:04:05
	if t, err := time.Parse("2006-01-02T15:04:05", value); err == nil {
		return t, false, nil
	}
	// ISO date: 2006-01-02
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("unrecognized ICS time format: %q", value)
}
// UI configuration updated.
// Grid/Timeline UI implemented

