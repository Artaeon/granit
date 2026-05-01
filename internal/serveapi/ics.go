package serveapi

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// icsEvent is the parsed view we surface to the calendar feed.
type icsEvent struct {
	Title    string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
	UID      string
	RRule    string // raw rule, e.g. "FREQ=DAILY;COUNT=10"
}

// icsSource is one .ics file discovered in the vault. Source is what the
// user sees in the calendar sources panel; Path is the absolute path.
type icsSource struct {
	Source string `json:"source"` // filename (e.g. "training.ics")
	Path   string `json:"path"`   // absolute path on disk
}

// icsListSources walks the vault for *.ics files (one level deep under
// vault root, calendars/, and Calendars/) and returns one entry per file.
func icsListSources(vaultRoot string) []icsSource {
	var out []icsSource
	roots := []string{vaultRoot, filepath.Join(vaultRoot, "calendars"), filepath.Join(vaultRoot, "Calendars")}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.EqualFold(filepath.Ext(e.Name()), ".ics") {
				continue
			}
			out = append(out, icsSource{
				Source: e.Name(),
				Path:   filepath.Join(root, e.Name()),
			})
		}
	}
	return out
}

// isICSDisabled mirrors the TUI's substring-match semantics so the same
// config.json `disabled_calendars` list silences the same files in both
// frontends. A non-empty entry that appears anywhere in the filename or
// path is treated as a match.
func isICSDisabled(src icsSource, disabled []string) bool {
	for _, dc := range disabled {
		dc = strings.TrimSpace(dc)
		if dc == "" {
			continue
		}
		if strings.Contains(src.Source, dc) || strings.Contains(src.Path, dc) {
			return true
		}
	}
	return false
}

// icsScan walks the vault for .ics files and returns parsed events from
// every file NOT matched by `disabled` (a list of substrings — same
// semantics as the TUI's `m.config.DisabledCalendars`).
func icsScan(vaultRoot string, disabled []string) []icsEvent {
	var out []icsEvent
	for _, src := range icsListSources(vaultRoot) {
		if isICSDisabled(src, disabled) {
			continue
		}
		evs, err := parseICSFile(src.Path)
		if err != nil {
			continue
		}
		out = append(out, evs...)
	}
	return out
}

func parseICSFile(path string) ([]icsEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Read & unfold (continuation lines start with a space or tab)
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

	var events []icsEvent
	var cur *icsEvent
	in := false
	for _, line := range lines {
		switch line {
		case "BEGIN:VEVENT":
			in = true
			cur = &icsEvent{}
		case "END:VEVENT":
			if in && cur != nil && cur.Title != "" && !cur.Start.IsZero() {
				events = append(events, *cur)
			}
			in = false
			cur = nil
		}
		if !in || cur == nil {
			continue
		}
		key, val := splitKV(line)
		base, params := splitParams(key)
		switch base {
		case "SUMMARY":
			cur.Title = unescape(val)
		case "LOCATION":
			cur.Location = unescape(val)
		case "UID":
			cur.UID = val
		case "RRULE":
			cur.RRule = val
		case "DTSTART":
			if t, allDay, ok := parseICSTime(val, params["TZID"]); ok {
				cur.Start = t
				cur.AllDay = allDay
			}
		case "DTEND":
			if t, _, ok := parseICSTime(val, params["TZID"]); ok {
				cur.End = t
			}
		}
	}
	// Default end = start + 1h for timed, +24h for all-day
	for i := range events {
		if events[i].End.IsZero() {
			if events[i].AllDay {
				events[i].End = events[i].Start.Add(24 * time.Hour)
			} else {
				events[i].End = events[i].Start.Add(time.Hour)
			}
		}
	}
	return events, nil
}

func splitKV(line string) (string, string) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return line, ""
	}
	return line[:i], line[i+1:]
}

func splitParams(key string) (string, map[string]string) {
	parts := strings.Split(key, ";")
	out := map[string]string{}
	for _, p := range parts[1:] {
		if eq := strings.IndexByte(p, '='); eq >= 0 {
			out[strings.ToUpper(p[:eq])] = p[eq+1:]
		}
	}
	return parts[0], out
}

func unescape(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// parseICSTime handles:
//   YYYYMMDD                       (all-day)
//   YYYYMMDDTHHMMSS                (floating local time — use Local)
//   YYYYMMDDTHHMMSSZ               (UTC)
//   With TZID param: floating local time interpreted in the named zone.
func parseICSTime(value, tzid string) (time.Time, bool, bool) {
	value = strings.TrimSpace(value)
	if len(value) == 8 {
		t, err := time.Parse("20060102", value)
		if err != nil {
			return time.Time{}, false, false
		}
		return t, true, true
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, false, false
		}
		return t, false, true
	}
	if len(value) == 15 {
		loc := time.Local
		if tzid != "" {
			if l, err := time.LoadLocation(tzid); err == nil {
				loc = l
			}
		}
		t, err := time.ParseInLocation("20060102T150405", value, loc)
		if err != nil {
			return time.Time{}, false, false
		}
		return t, false, true
	}
	return time.Time{}, false, false
}

// expandRRULE returns instances of ev within [from, to] inclusive, given the
// RRULE. Supports FREQ=DAILY|WEEKLY|MONTHLY|YEARLY with INTERVAL, COUNT, UNTIL.
// More exotic rules (BYDAY, BYMONTHDAY, etc.) are best-effort: we just emit
// the base series.
func expandRRULE(ev icsEvent, from, to time.Time) []icsEvent {
	if ev.RRule == "" {
		// Single occurrence
		if ev.Start.Before(to) && ev.End.After(from) {
			return []icsEvent{ev}
		}
		return nil
	}

	parts := map[string]string{}
	for _, kv := range strings.Split(ev.RRule, ";") {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		parts[strings.ToUpper(kv[:eq])] = kv[eq+1:]
	}
	freq := strings.ToUpper(parts["FREQ"])
	interval := 1
	if v := parts["INTERVAL"]; v != "" {
		if n := atoiSafe(v); n > 0 {
			interval = n
		}
	}
	count := 0
	if v := parts["COUNT"]; v != "" {
		count = atoiSafe(v)
	}
	var until time.Time
	if v := parts["UNTIL"]; v != "" {
		if t, _, ok := parseICSTime(v, ""); ok {
			until = t
		}
	}

	var step func(time.Time) time.Time
	switch freq {
	case "DAILY":
		step = func(t time.Time) time.Time { return t.AddDate(0, 0, interval) }
	case "WEEKLY":
		step = func(t time.Time) time.Time { return t.AddDate(0, 0, 7*interval) }
	case "MONTHLY":
		step = func(t time.Time) time.Time { return t.AddDate(0, interval, 0) }
	case "YEARLY":
		step = func(t time.Time) time.Time { return t.AddDate(interval, 0, 0) }
	default:
		// Unknown freq — emit base
		if ev.Start.Before(to) && ev.End.After(from) {
			return []icsEvent{ev}
		}
		return nil
	}

	dur := ev.End.Sub(ev.Start)
	var out []icsEvent
	cur := ev.Start
	emitted := 0
	const maxEmit = 1000

	for emitted < maxEmit {
		if !until.IsZero() && cur.After(until) {
			break
		}
		if cur.After(to) {
			break
		}
		if !cur.Before(from.Add(-dur)) {
			inst := ev
			inst.Start = cur
			inst.End = cur.Add(dur)
			inst.RRule = ""
			out = append(out, inst)
			emitted++
			if count > 0 && emitted >= count {
				break
			}
		}
		next := step(cur)
		if !next.After(cur) {
			break // safety: prevent infinite loop on a malformed step
		}
		cur = next
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ensure imports stay tidy if a future change drops a fn
var _ = fs.SkipDir
