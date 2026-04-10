package tui

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// formatFileSize (imageview.go) — pure formatter
// ---------------------------------------------------------------------------

func TestFormatFileSize_Bytes(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
	}
	for _, tt := range tests {
		if got := formatFileSize(tt.size); got != tt.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestFormatFileSize_Kilobytes(t *testing.T) {
	if got := formatFileSize(1024); got != "1.0KB" {
		t.Errorf("expected '1.0KB', got %q", got)
	}
	if got := formatFileSize(1536); got != "1.5KB" {
		t.Errorf("expected '1.5KB', got %q", got)
	}
}

func TestFormatFileSize_Megabytes(t *testing.T) {
	if got := formatFileSize(1024 * 1024); got != "1.0MB" {
		t.Errorf("expected '1.0MB', got %q", got)
	}
	if got := formatFileSize(int64(2.5 * 1024 * 1024)); got != "2.5MB" {
		t.Errorf("expected '2.5MB', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ClipManager.formatAge — pure-ish (uses time.Since)
// ---------------------------------------------------------------------------

func TestClipManager_FormatAge_JustNow(t *testing.T) {
	cm := ClipManager{}
	if got := cm.formatAge(time.Now()); got != "just now" {
		t.Errorf("expected 'just now', got %q", got)
	}
}

func TestClipManager_FormatAge_Minutes(t *testing.T) {
	cm := ClipManager{}
	got := cm.formatAge(time.Now().Add(-5 * time.Minute))
	if got != "5m ago" {
		t.Errorf("expected '5m ago', got %q", got)
	}
}

func TestClipManager_FormatAge_Hours(t *testing.T) {
	cm := ClipManager{}
	got := cm.formatAge(time.Now().Add(-3 * time.Hour))
	if got != "3h ago" {
		t.Errorf("expected '3h ago', got %q", got)
	}
}

func TestClipManager_FormatAge_Days(t *testing.T) {
	cm := ClipManager{}
	got := cm.formatAge(time.Now().Add(-72 * time.Hour))
	if got != "3d ago" {
		t.Errorf("expected '3d ago', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// truncateClip (clipmanager.go) — local helper used by clip rendering
// ---------------------------------------------------------------------------

func TestTruncateClip_Short(t *testing.T) {
	if got := truncateClip("hi", 10); got != "hi" {
		t.Errorf("expected 'hi', got %q", got)
	}
}

func TestTruncateClip_ZeroLen(t *testing.T) {
	if got := truncateClip("anything", 0); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestTruncateClip_Truncates(t *testing.T) {
	got := truncateClip("hello world", 8)
	if len(got) > 8 {
		t.Errorf("expected len <= 8, got %d (%q)", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// parseLogOutput (githistory.go) — pure parser
// ---------------------------------------------------------------------------

func TestParseLogOutput_Empty(t *testing.T) {
	if got := parseLogOutput(""); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestParseLogOutput_SingleCommit(t *testing.T) {
	out := "abc1234|Alice|2026-04-10|Initial commit"
	got := parseLogOutput(out)
	if len(got) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(got))
	}
	if got[0].Hash != "abc1234" || got[0].Author != "Alice" || got[0].Date != "2026-04-10" || got[0].Message != "Initial commit" {
		t.Errorf("commit fields wrong: %+v", got[0])
	}
}

func TestParseLogOutput_MultipleCommits(t *testing.T) {
	out := strings.Join([]string{
		"a|Alice|2026-04-10|First",
		"b|Bob|2026-04-11|Second",
		"c|Cara|2026-04-12|Third",
	}, "\n")
	got := parseLogOutput(out)
	if len(got) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(got))
	}
	if got[2].Author != "Cara" {
		t.Errorf("third commit author wrong: %q", got[2].Author)
	}
}

func TestParseLogOutput_SkipsMalformed(t *testing.T) {
	out := strings.Join([]string{
		"a|Alice|2026-04-10|Good",
		"this is not a commit",
		"b|Bob|2026-04-11|Also good",
	}, "\n")
	got := parseLogOutput(out)
	if len(got) != 2 {
		t.Errorf("expected 2 valid commits (malformed skipped), got %d", len(got))
	}
}

func TestParseLogOutput_PreservesPipesInMessage(t *testing.T) {
	// Commit messages may legitimately contain pipes; SplitN limit 4 means
	// the message field captures everything after the third pipe.
	out := "h|A|2026-04-10|fix: handle a | b | c case"
	got := parseLogOutput(out)
	if len(got) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(got))
	}
	if got[0].Message != "fix: handle a | b | c case" {
		t.Errorf("message lost pipes: %q", got[0].Message)
	}
}

func TestParseLogOutput_TrimsTrailingNewlines(t *testing.T) {
	out := "a|A|2026-04-10|msg\n\n\n"
	got := parseLogOutput(out)
	if len(got) != 1 {
		t.Errorf("trailing newlines created spurious commits: %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// parseBlogOutline (blogdraft.go) — pure parser
// ---------------------------------------------------------------------------

func TestParseBlogOutline_Empty(t *testing.T) {
	if got := parseBlogOutline(""); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseBlogOutline_SingleSection(t *testing.T) {
	raw := "## Introduction\n- Why this matters\n- The hook"
	got := parseBlogOutline(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 section, got %d", len(got))
	}
	if got[0].Heading != "Introduction" {
		t.Errorf("heading wrong: %q", got[0].Heading)
	}
	if len(got[0].KeyPoints) != 2 {
		t.Errorf("expected 2 key points, got %d", len(got[0].KeyPoints))
	}
}

func TestParseBlogOutline_MultipleSections(t *testing.T) {
	raw := strings.Join([]string{
		"## Section A",
		"- Point A1",
		"- Point A2",
		"## Section B",
		"- Point B1",
	}, "\n")
	got := parseBlogOutline(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}
	if len(got[0].KeyPoints) != 2 || len(got[1].KeyPoints) != 1 {
		t.Errorf("key point counts wrong: %v", got)
	}
}

func TestParseBlogOutline_IgnoresOrphanBullets(t *testing.T) {
	// Bullets before any heading should be dropped (no current section).
	raw := "- orphan1\n- orphan2\n## First\n- valid"
	got := parseBlogOutline(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 section, got %d", len(got))
	}
	if len(got[0].KeyPoints) != 1 || got[0].KeyPoints[0] != "valid" {
		t.Errorf("orphan bullets leaked into section: %v", got[0].KeyPoints)
	}
}

func TestParseBlogOutline_SkipsEmptyHeadings(t *testing.T) {
	raw := "## \n- nope\n## Real\n- yes"
	got := parseBlogOutline(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 section, got %d", len(got))
	}
	if got[0].Heading != "Real" {
		t.Errorf("expected 'Real', got %q", got[0].Heading)
	}
}

// ---------------------------------------------------------------------------
// ProjectDashboard.formatDueDate — pure-ish (uses time.Now)
// ---------------------------------------------------------------------------

func TestProjectDashboard_FormatDueDate_Today(t *testing.T) {
	pd := &ProjectDashboard{}
	today := time.Now().Format("2006-01-02")
	if got := pd.formatDueDate(today); got != "today" {
		t.Errorf("expected 'today', got %q", got)
	}
}

func TestProjectDashboard_FormatDueDate_Tomorrow(t *testing.T) {
	pd := &ProjectDashboard{}
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if got := pd.formatDueDate(tomorrow); got != "tomorrow" {
		t.Errorf("expected 'tomorrow', got %q", got)
	}
}

func TestProjectDashboard_FormatDueDate_Overdue(t *testing.T) {
	pd := &ProjectDashboard{}
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if got := pd.formatDueDate(yesterday); got != "overdue" {
		t.Errorf("expected 'overdue', got %q", got)
	}
}

func TestProjectDashboard_FormatDueDate_Future(t *testing.T) {
	pd := &ProjectDashboard{}
	future := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	got := pd.formatDueDate(future)
	// "Jan 2" style format — must be non-empty and not match the relative labels
	if got == "today" || got == "tomorrow" || got == "overdue" {
		t.Errorf("expected month-day format, got %q", got)
	}
	if got == "" {
		t.Error("expected non-empty format")
	}
}

func TestProjectDashboard_FormatDueDate_Unparseable(t *testing.T) {
	pd := &ProjectDashboard{}
	// Garbage strings still in the future relative to today should round-trip.
	garbage := "9999-12-31"
	if got := pd.formatDueDate(garbage); got == "" {
		t.Error("expected non-empty result for unparseable date")
	}
}
