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

// TestParse_FrontmatterOptOut_SkipsNote verifies the
// `tasks: false` (or no/skip/none) frontmatter flag suppresses the
// scanner for the entire note. Bullet-list-style `- [ ]` lines in
// reading notes / templates / brainstorm pages must not pollute the
// global task list once opted out.
func TestParse_FrontmatterOptOut_SkipsNote(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name: "tasks: false",
			content: "---\ntitle: brainstorm\ntasks: false\n---\n\n- [ ] not a task\n- [ ] also not\n",
		},
		{
			name: "tasks: no",
			content: "---\ntasks: no\n---\n- [ ] still no\n",
		},
		{
			name: "tasks: skip",
			content: "---\ntasks: skip\n---\n- [ ] skipped\n",
		},
		{
			name: "tasks: none",
			content: "---\ntasks: none\n---\n- [ ] nope\n",
		},
		{
			name:    "CRLF line endings",
			content: "---\r\ntasks: false\r\n---\r\n- [ ] crlf\r\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := ParseNotes([]NoteContent{{Path: "Brainstorm.md", Content: c.content}})
			if len(out) != 0 {
				t.Errorf("expected 0 tasks for opted-out note, got %d", len(out))
			}
		})
	}
}

// TestParse_FrontmatterOptOut_DoesNotMatchInBody guards against
// false-positive opt-outs from a line of body text that happens to
// contain `tasks: false` (e.g. inside a code fence or a quote).
func TestParse_FrontmatterOptOut_DoesNotMatchInBody(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "tasks: true in frontmatter",
			content: "---\ntasks: true\n---\n- [ ] real task\n",
		},
		{
			name:    "no frontmatter, tasks: false in body",
			content: "Some intro.\ntasks: false\n- [ ] real task\n",
		},
		{
			name:    "tasks: false in code fence below frontmatter",
			content: "---\ntitle: x\n---\n\n```\ntasks: false\n```\n- [ ] real task\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := ParseNotes([]NoteContent{{Path: "x.md", Content: c.content}})
			if len(out) != 1 {
				t.Errorf("expected 1 task, got %d (content suppressed)", len(out))
			}
		})
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

// TestParse_HabitsSection_Excluded pins the section-skip rule:
// `## Habits` is owned by the habits subsystem, so its checkbox lines
// must NOT be surfaced as tasks. Verifies entry on the Habits heading,
// exit on the next heading, and case-insensitive heading match.
func TestParse_HabitsSection_Excluded(t *testing.T) {
	content := `# 2026-05-05

## Tasks
- [ ] finish report
- [x] morning email

## Habits
- [ ] morning movement
- [ ] read 20 pages
- [x] evening prayer

## Notes
- [ ] follow up on RFC

### habits
- [ ] case-insensitive habit
`
	out := ParseNotes([]NoteContent{{Path: "Daily/2026-05-05.md", Content: content}})
	// Expect exactly 3 tasks: finish report, morning email, follow up on RFC.
	// All 4 habit checkboxes (3 in ## Habits + 1 in ### habits) are skipped.
	if len(out) != 3 {
		got := make([]string, len(out))
		for i, t := range out {
			got[i] = t.Text
		}
		t.Fatalf("expected 3 tasks, got %d: %v", len(out), got)
	}
	wantTexts := map[string]bool{
		"finish report":      true,
		"morning email":      true,
		"follow up on RFC":   true,
	}
	for _, task := range out {
		if !wantTexts[task.Text] {
			t.Errorf("unexpected task surfaced from Habits section: %q", task.Text)
		}
	}
}

// TestParse_HabitsSection_IgnoresCodeFences pins the code-fence
// guard: a `# Habits` line inside a triple-backtick (or triple-
// tilde) code block must NOT toggle the section skip. Without this,
// a daily note with a bash example containing "# Habits" would
// silently drop every task after the fence.
func TestParse_HabitsSection_IgnoresCodeFences(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name: "backtick fence with comment",
			content: "## Tasks\n" +
				"- [ ] real task A\n" +
				"\n" +
				"```bash\n" +
				"# Habits configuration\n" +
				"echo hi\n" +
				"```\n" +
				"\n" +
				"- [ ] real task B\n",
		},
		{
			name: "tilde fence",
			content: "## Tasks\n" +
				"- [ ] real task A\n" +
				"\n" +
				"~~~yaml\n" +
				"### Habits\n" +
				"foo: bar\n" +
				"~~~\n" +
				"\n" +
				"- [ ] real task B\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := ParseNotes([]NoteContent{{Path: "x.md", Content: c.content}})
			if len(out) != 2 {
				got := make([]string, len(out))
				for i, t := range out {
					got[i] = t.Text
				}
				t.Fatalf("expected 2 tasks (fence content ignored), got %d: %v", len(out), got)
			}
		})
	}
}

// TestParse_HabitsSection_NestedHeading verifies that a sub-heading
// under ## Habits keeps the section closed (we exit Habits on any
// heading, including deeper ones — matches the habits parser's
// section-end behavior).
func TestParse_HabitsSection_NestedHeading(t *testing.T) {
	content := `## Habits
- [ ] habit one

### Sub
- [ ] not a habit anymore
`
	out := ParseNotes([]NoteContent{{Path: "x.md", Content: content}})
	if len(out) != 1 {
		t.Fatalf("expected 1 task, got %d", len(out))
	}
	if out[0].Text != "not a habit anymore" {
		t.Errorf("got task %q, want %q", out[0].Text, "not a habit anymore")
	}
}
