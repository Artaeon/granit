package serveapi

import (
	"strings"
	"testing"
	"time"
)

// TestUpsertNamedSection_Append verifies the append path: a marker
// not present in the doc lands at the end with proper blank-line
// spacing, regardless of the doc's existing trailing newlines.
func TestUpsertNamedSection_Append(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty doc", ""},
		{"no trailing newline", "intro paragraph"},
		{"one trailing newline", "intro paragraph\n"},
		{"many trailing newlines", "intro paragraph\n\n\n"},
	}
	body := "## Examen — Monday, January 1, 2026\n\ngrace today.\n"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := upsertNamedSection(c.raw, "## Examen", body)
			if !strings.Contains(out, body) {
				t.Errorf("output missing inserted body")
			}
			// No more than two consecutive newlines anywhere — the
			// upserter normalises spacing on append.
			if strings.Contains(out, "\n\n\n") {
				t.Errorf("output has triple newline — over-padded:\n%q", out)
			}
		})
	}
}

// TestUpsertNamedSection_Replace covers the in-place update path:
// a doc that already contains the section gets its body swapped
// without touching surrounding content.
func TestUpsertNamedSection_Replace(t *testing.T) {
	raw := strings.Join([]string{
		"# Daily 2026-01-02",
		"",
		"## Daily Plan — Monday, January 2, 2026",
		"",
		"### Today's Goal",
		"",
		"**ship the thing**",
		"",
		"## Examen — Monday, January 2, 2026",
		"",
		"### Where I saw God",
		"",
		"old reflection",
		"",
		"## Notes",
		"",
		"random follow-up",
		"",
	}, "\n")
	newSection := "## Examen — Monday, January 2, 2026\n\n### Where I saw God\n\nnew reflection\n\n"
	out := upsertNamedSection(raw, "## Examen", newSection)

	// The new body must be present.
	if !strings.Contains(out, "new reflection") {
		t.Error("new reflection missing from output")
	}
	// The old body must be gone.
	if strings.Contains(out, "old reflection") {
		t.Errorf("old reflection still present after replace:\n%s", out)
	}
	// Pre-existing sections must survive untouched.
	if !strings.Contains(out, "ship the thing") {
		t.Error("Daily Plan section was clobbered")
	}
	if !strings.Contains(out, "random follow-up") {
		t.Error("trailing Notes section was clobbered")
	}
}

// TestBuildExamen_OmitsEmptyFields confirms partial-payload rendering:
// a "just gratitude tonight" submission shouldn't leave empty
// "### Where I saw God" headers behind in the output.
func TestBuildExamen_OmitsEmptyFields(t *testing.T) {
	when := time.Date(2026, 5, 3, 21, 0, 0, 0, time.UTC)
	out := buildExamen(ExamenSaveBody{
		Gratitude: "the unexpected call from a friend",
	}, when)
	if !strings.Contains(out, "Gratitude") {
		t.Error("expected Gratitude header in output")
	}
	if strings.Contains(out, "Where I saw God") {
		t.Error("empty SawGod field leaked into output as a header")
	}
	if strings.Contains(out, "Where I missed") {
		t.Error("empty Missed field leaked into output as a header")
	}
	if strings.Contains(out, "For tomorrow") {
		t.Error("empty Tomorrow field leaked into output as a header")
	}
}

// TestBuildExamen_AllEmpty inserts a stub when all fields are blank
// — better UX than a section with just a date header.
func TestBuildExamen_AllEmpty(t *testing.T) {
	when := time.Date(2026, 5, 3, 21, 0, 0, 0, time.UTC)
	out := buildExamen(ExamenSaveBody{}, when)
	if !strings.Contains(out, "(no entries this evening)") {
		t.Errorf("expected stub for empty payload, got:\n%s", out)
	}
}
