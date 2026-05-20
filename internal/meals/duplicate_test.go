package meals

import (
	"strings"
	"testing"
)

// Regression test for a silent-data-loss bug discovered on the third
// hardening pass: when the daily note had two rows with the same
// (time, name) — a user-mistake or a result of a sync glitch —
// ApplyPatch updated the FIRST row but WriteSection's byKey map
// captured the SECOND (last-write-wins), so writing back rendered
// BOTH lines untouched. The user's tick simply disappeared.
//
// The fix is first-write-wins inside WriteSection's byKey map, which
// keeps it aligned with ApplyPatch's "first match" semantics. The
// pragmatic outcome of duplicate rows is: both become identical and
// reflect the ticked state. The user can clean up the dup manually
// in the daily note.
func TestWriteSection_DuplicateRowsPropagateTickInsteadOfSwallowingIt(t *testing.T) {
	body := strings.Join([]string{
		"## Meals",
		"- [ ] 08:00 Breakfast",
		"- [ ] 08:00 Breakfast",
		"",
	}, "\n")
	parsed := Parse(body)
	if len(parsed) != 2 {
		t.Fatalf("want 2 parsed, got %d", len(parsed))
	}
	tr := true
	updated, _ := ApplyPatch(parsed, "08:00", "Breakfast", &tr, nil)
	out := WriteSection(body, updated)

	ticked := strings.Count(out, "- [x] 08:00 Breakfast")
	if ticked < 1 {
		t.Errorf("tick disappeared on write-back; output was:\n%s", out)
	}
	rows := strings.Count(out, "- [")
	if rows != 2 {
		t.Errorf("row count changed (want 2): %d\n%s", rows, out)
	}
}
