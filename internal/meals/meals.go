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
// Used for the cold-start path where no section exists yet — once
// the section exists, WriteSection does line-preserving edits
// instead of regenerating the whole thing.
func RenderSection(slots []Slot) string {
	var sb strings.Builder
	sb.WriteString("## Meals\n")
	for _, s := range slots {
		sb.WriteString(renderRow(s))
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

// DetectHeading scans the note body for an existing Meals heading
// at any level (`# Meals`, `## Meals`, `### Meals`, …) and returns
// the exact heading line plus its level. The level is the count of
// leading `#` characters. Returns ("## Meals", 0) when no heading
// exists — signalling the caller to use the canonical default.
//
// The case-fold match mirrors Parse, so a user's `### meals` written
// in lowercase still resolves to the same section.
func DetectHeading(body string) (marker string, level int) {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "#") {
			continue
		}
		lvl := 0
		for lvl < len(trim) && trim[lvl] == '#' {
			lvl++
		}
		text := strings.TrimSpace(trim[lvl:])
		if strings.EqualFold(text, "Meals") {
			return trim, lvl
		}
	}
	return "## Meals", 0
}

// RewriteHeadingLevel swaps the leading `## Meals` of a rendered
// section to the requested heading level. RenderSection always writes
// level-2; this helper exists so the upsert path can keep the user's
// chosen level when they wrote e.g. "### Meals" manually.
func RewriteHeadingLevel(section string, level int) string {
	if level <= 0 || level == 2 {
		return section
	}
	prefix := strings.Repeat("#", level) + " Meals"
	return strings.Replace(section, "## Meals", prefix, 1)
}

// WriteSection rewrites the Meals section *in place*, line by line:
//
//   - Existing meal-row lines are replaced with the rendered version
//     of the matching slot (by Time + lowercased Name). If the slot
//     was removed from `slots`, the line is dropped.
//   - Non-meal lines inside the Meals section (free-form notes the
//     user wrote between rows, like "today I tried oats") are kept
//     verbatim — the original RenderSection-based upsert would have
//     silently dropped them, which is a data-loss bug for daily-
//     driver use.
//   - Slots not seen in the original section are appended at the end
//     of the section (just before the next heading or EOF), in the
//     order they appear in `slots`.
//   - If no Meals section exists, a fresh one is appended at the end
//     of the body using RenderSection's format.
//
// Returns the new note body. Callers can pass the result of
// ApplyPatch as `slots` so the rewrite reflects exactly the upsert
// the API received — no global rewrite of unrelated content.
func WriteSection(body string, slots []Slot) string {
	marker, level := DetectHeading(body)
	if level == 0 {
		// No section yet — fall back to the original append-fresh
		// path (caller's upsertNamedSection equivalent). We re-use
		// RenderSection so the format is identical.
		section := RenderSection(slots)
		section = RewriteHeadingLevel(section, level) // no-op when level==0
		trimmed := strings.TrimRight(body, "\n")
		if trimmed == "" {
			return section
		}
		return trimmed + "\n\n" + section
	}

	lines := strings.Split(body, "\n")
	// Locate section bounds: heading line index + first line of next
	// section (or len(lines) if it runs to EOF).
	headingIdx := -1
	for i, raw := range lines {
		if strings.TrimSpace(strings.TrimRight(raw, "\r")) == marker {
			headingIdx = i
			break
		}
	}
	if headingIdx < 0 {
		// Shouldn't happen — DetectHeading just found it. Defensive
		// fallback to the no-section path so we never lose the body.
		section := RenderSection(slots)
		section = RewriteHeadingLevel(section, level)
		return strings.TrimRight(body, "\n") + "\n\n" + section
	}
	endIdx := len(lines)
	for i := headingIdx + 1; i < len(lines); i++ {
		trim := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
		if !strings.HasPrefix(trim, "#") {
			continue
		}
		// New heading found — anything below it is outside our section.
		endIdx = i
		break
	}

	// Index slots by key for O(1) lookups while walking the section.
	// FIRST-write-wins (not last) because ApplyPatch only updates the
	// first occurrence of a given (time, name) key and leaves any
	// duplicate behind. If byKey overwrote with the duplicate, the
	// user's tick would silently disappear on write-back — caught by
	// TestWriteSection_DuplicateRowsBecomeIdenticalNotLost.
	byKey := make(map[string]Slot, len(slots))
	for _, s := range slots {
		k := slotKey(s)
		if _, exists := byKey[k]; !exists {
			byKey[k] = s
		}
	}
	seen := make(map[string]bool, len(slots))

	// Rebuild the section body (everything between headingIdx+1 and
	// endIdx-1, exclusive of the next heading). Trailing blank lines
	// inside the section are preserved by-default since we copy them.
	var out []string
	out = append(out, lines[:headingIdx+1]...) // up to and including heading
	for i := headingIdx + 1; i < endIdx; i++ {
		raw := lines[i]
		m := mealRowRe.FindStringSubmatch(raw)
		if m == nil {
			out = append(out, raw)
			continue
		}
		// Parse this row's identity so we can find the matching slot.
		time := m[2]
		rest := strings.TrimSpace(m[3])
		name := rest
		for _, sep := range []string{" — ", " – ", " - "} {
			if idx := strings.Index(rest, sep); idx >= 0 {
				name = strings.TrimSpace(rest[:idx])
				break
			}
		}
		key := time + "|" + strings.ToLower(name)
		s, ok := byKey[key]
		if !ok {
			// Slot dropped — omit the line. (Not currently triggered
			// by the API; the PATCH path never deletes slots. Kept
			// for future remove-slot support.)
			continue
		}
		seen[key] = true
		out = append(out, renderRow(s))
	}
	// Pop trailing blank lines inside the section before we append
	// new slots — otherwise every concurrent PATCH that appends a
	// row stacks another blank-line gap, and the section drifts
	// into a multi-blank mess after a stress-write burst. Caught
	// by the 5×concurrent PATCH smoke test.
	for len(out) > headingIdx+1 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	// Append any slots not seen in the original section. Preserves
	// the sort order ApplyPatch already imposed.
	appended := 0
	for _, s := range slots {
		if seen[slotKey(s)] {
			continue
		}
		out = append(out, renderRow(s))
		appended++
	}
	// Tail content (next heading + everything after).
	if endIdx < len(lines) {
		// Always ensure exactly one blank line between the last
		// section line and the next heading. The earlier
		// trailing-blank pop normalised the section to end on a
		// non-blank line; without this re-insertion the section
		// would collide with the next heading and look like
		// "row\n## Notes" — visually fused.
		if len(out) > headingIdx+1 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, lines[endIdx:]...)
	}
	_ = appended
	result := strings.Join(out, "\n")
	// Preserve the original body's trailing-newline state. The
	// trailing-blank pop above can strip the final "\n" when the
	// Meals section is the LAST thing in the file (no next heading),
	// leaving a file that ends mid-line. Markdown editors handle
	// this but it's untidy and breaks "POSIX text file" assumptions
	// some tools rely on.
	if strings.HasSuffix(body, "\n") && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

// renderRow is the single-line equivalent of RenderSection's per-slot
// emit. Pulled out so WriteSection can splice individual lines into
// an existing section without rebuilding the heading.
func renderRow(s Slot) string {
	box := "[ ]"
	if s.Done {
		box = "[x]"
	}
	row := "- " + box + " " + s.Time + " " + s.Name
	if txt := strings.TrimSpace(s.Text); txt != "" {
		row += " — " + txt
	}
	return row
}
