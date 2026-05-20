// Package meals is the parser + renderer for the daily-note `## Meals`
// section. It exists for the same reason habits has its own package:
// keep a single canonical source of "what does this row mean" so the
// web API, the calendar synthesizer, and (eventually) the TUI all
// agree.
//
// On-disk layout: meals live inside the user's daily note as a
// markdown section.
//
//	## Meals
//	- [x] 08:00 Breakfast — Haferflocken
//	- [x] 12:30 Lunch — Reste Pasta
//	- [ ] 19:00 Dinner
//
// The section is excluded from the task parser (internal/tasks/parser.go)
// so meals don't pollute /tasks views — they're tracked here.
//
// Pure data + parsing. No HTTP, no IO. The serveapi layer owns the
// daily-note read/write; this package just transforms strings.
package meals

import (
	"regexp"
	"sort"
	"strings"
)

// Slot is a single meal for one day. Time is HH:MM in 24h, Name is
// the user-visible label ("Breakfast"), Text is the optional free-form
// "what I actually ate" capture.
type Slot struct {
	Time string `json:"time"`
	Name string `json:"name"`
	Done bool   `json:"done"`
	Text string `json:"text,omitempty"`
}

// DefaultSlots returns the v1 default meal lineup: three slots at
// breakfast / lunch / dinner times. The user can override these later
// via .granit/meals-defaults.json — until they do, this list is the
// canonical baseline used both to seed empty days and to fill in
// missing slots when rendering an existing day.
func DefaultSlots() []Slot {
	return []Slot{
		{Time: "08:00", Name: "Breakfast"},
		{Time: "12:30", Name: "Lunch"},
		{Time: "19:00", Name: "Dinner"},
	}
}

// mealRowRe matches `- [x] HH:MM Name — text` (and variants):
//
//	group 1 — the checkbox char (' ', 'x', 'X')
//	group 2 — the HH:MM time
//	group 3 — the rest (Name + optional " — text")
var mealRowRe = regexp.MustCompile(`^\s*-\s+\[([ xX])\]\s+(\d{1,2}:\d{2})\s+(.+)$`)

// Parse extracts meal rows from any heading-level "Meals" section in
// the note body. Returns slots in file order. A missing section is an
// empty slice — never an error — so callers can always treat the
// result as a clean read.
func Parse(noteBody string) []Slot {
	var out []Slot
	in := false
	for _, raw := range strings.Split(noteBody, "\n") {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			text := strings.TrimSpace(strings.TrimLeft(trim, "#"))
			in = strings.EqualFold(text, "Meals")
			continue
		}
		if !in {
			continue
		}
		m := mealRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		done := m[1] == "x" || m[1] == "X"
		rest := strings.TrimSpace(m[3])
		// Split on em-dash / en-dash / hyphen-with-spaces; first
		// match wins. Falls back to whole rest as name when no
		// separator is present — matches the "- [ ] 08:00 Breakfast"
		// shape the default slots use before the user adds text.
		name := rest
		text := ""
		for _, sep := range []string{" — ", " – ", " - "} {
			if idx := strings.Index(rest, sep); idx >= 0 {
				name = strings.TrimSpace(rest[:idx])
				text = strings.TrimSpace(rest[idx+len(sep):])
				break
			}
		}
		out = append(out, Slot{Time: m[2], Name: name, Done: done, Text: text})
	}
	return out
}

// MergeWithDefaults returns the rendered slot set for a day: parsed
// slots are kept verbatim, missing defaults are appended unticked,
// and the result is sorted by time so morning → evening reads
// naturally regardless of insertion order. Two slots are "the same"
// when their (Time, lowercased Name) tuple matches.
func MergeWithDefaults(parsed, defaults []Slot) []Slot {
	seen := map[string]bool{}
	out := make([]Slot, 0, len(parsed)+len(defaults))
	for _, s := range parsed {
		key := slotKey(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, d := range defaults {
		key := slotKey(d)
		if seen[key] {
			continue
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func slotKey(s Slot) string { return s.Time + "|" + strings.ToLower(s.Name) }

// RenderSection builds the `## Meals` section body (heading + rows
// + trailing blank line) ready to feed into upsertNamedSection.
func RenderSection(slots []Slot) string {
	var sb strings.Builder
	sb.WriteString("## Meals\n")
	for _, s := range slots {
		box := "[ ]"
		if s.Done {
			box = "[x]"
		}
		sb.WriteString("- ")
		sb.WriteString(box)
		sb.WriteString(" ")
		sb.WriteString(s.Time)
		sb.WriteString(" ")
		sb.WriteString(s.Name)
		txt := strings.TrimSpace(s.Text)
		if txt != "" {
			sb.WriteString(" — ")
			sb.WriteString(txt)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// ApplyPatch finds the slot matching `time` (and `name` when given —
// disambiguates the rare case where two slots share a time) and
// applies done/text updates. Returns the updated slot list plus a
// changed flag so callers can skip the file write on no-ops.
//
// If the targeted slot doesn't exist yet (common case: the user is
// ticking a default that hasn't been materialised into the daily note)
// and the patch carries any non-zero payload, the slot is appended
// and the list re-sorted.
func ApplyPatch(slots []Slot, time, name string, done *bool, text *string) ([]Slot, bool) {
	out := make([]Slot, len(slots))
	copy(out, slots)
	for i := range out {
		if out[i].Time != time {
			continue
		}
		if name != "" && !strings.EqualFold(out[i].Name, name) {
			continue
		}
		changed := false
		if done != nil && out[i].Done != *done {
			out[i].Done = *done
			changed = true
		}
		if text != nil && out[i].Text != *text {
			out[i].Text = *text
			changed = true
		}
		return out, changed
	}
	nonEmpty := (done != nil && *done) || (text != nil && *text != "")
	if !nonEmpty {
		return out, false
	}
	ns := Slot{Time: time, Name: name}
	if done != nil {
		ns.Done = *done
	}
	if text != nil {
		ns.Text = *text
	}
	out = append(out, ns)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out, true
}

// Aggregate returns the done-count + total-count for a slot list.
// Used by the dashboard header and the calendar event styling.
func Aggregate(slots []Slot) (done, total int) {
	total = len(slots)
	for _, s := range slots {
		if s.Done {
			done++
		}
	}
	return
}
