package tasks

import "testing"

// TestParse_DeadlineMarker_RoundTrip locks down the `deadline:<ulid>`
// shape: parsing a task line with the marker populates Task.DeadlineID
// with the bare ULID (no `deadline:` prefix). The shape mirrors the
// goal: marker so the TUI parser ignores it gracefully if it doesn't
// know about it yet.
func TestParse_DeadlineMarker_RoundTrip(t *testing.T) {
	const ulid = "01h7v3v3z9q4y0v3y8x6e7m2s1"
	cases := []struct {
		name string
		line string
		want string
	}{
		{"bare", "- [ ] ship doc deadline:" + ulid, ulid},
		{"trailing space", "- [ ] ship doc deadline:" + ulid + " ", ulid},
		{"with goal too", "- [ ] ship doc goal:G001 deadline:" + ulid, ulid},
		{"with priority + due", "- [ ] ship doc !1 due:2026-05-30 deadline:" + ulid, ulid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := ParseNotes([]NoteContent{{Path: "Tasks.md", Content: c.line + "\n"}})
			if len(out) != 1 {
				t.Fatalf("expected 1 task, got %d", len(out))
			}
			if out[0].DeadlineID != c.want {
				t.Errorf("DeadlineID = %q, want %q", out[0].DeadlineID, c.want)
			}
		})
	}
}

// TestParse_DeadlineMarker_NoFalsePositives checks the regex
// requires exactly 26 lowercase Crockford-alphabet chars — typos
// shouldn't accidentally populate DeadlineID.
func TestParse_DeadlineMarker_NoFalsePositives(t *testing.T) {
	cases := []string{
		"- [ ] doc deadline:short",
		"- [ ] doc deadline:UPPERCASE26CHARSAAAAAAAAAA",
		"- [ ] doc deadline:!@#$%^&*()_+-=[]{};:'\",.<>/",
	}
	for _, line := range cases {
		out := ParseNotes([]NoteContent{{Path: "Tasks.md", Content: line + "\n"}})
		if len(out) != 1 {
			t.Fatalf("expected 1 task, got %d", len(out))
		}
		if out[0].DeadlineID != "" {
			t.Errorf("expected no deadline match for %q, got %q", line, out[0].DeadlineID)
		}
	}
}

// TestParse_GoalAndDeadline_BothPopulated verifies that a single line
// with both markers populates GoalID and DeadlineID independently —
// the two regexes must not interfere.
func TestParse_GoalAndDeadline_BothPopulated(t *testing.T) {
	const ulid = "01h7v3v3z9q4y0v3y8x6e7m2s1"
	const goalID = "G042"
	out := ParseNotes([]NoteContent{
		{Path: "Tasks.md", Content: "- [ ] ship phase 2 goal:" + goalID + " deadline:" + ulid + "\n"},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 task, got %d", len(out))
	}
	if out[0].GoalID != goalID {
		t.Errorf("GoalID = %q, want %q", out[0].GoalID, goalID)
	}
	if out[0].DeadlineID != ulid {
		t.Errorf("DeadlineID = %q, want %q", out[0].DeadlineID, ulid)
	}
}
