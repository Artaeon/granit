package tui

import "testing"

func TestParseNoteHistoryLog_Basic(t *testing.T) {
	output := "abc123|abc12|Alice|2 hours ago|Fix typo\ndef456|def45|Bob|1 day ago|Add section\n"
	entries := parseNoteHistoryLog(output)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Hash != "abc123" {
		t.Errorf("expected hash abc123, got %q", entries[0].Hash)
	}
	if entries[0].ShortHash != "abc12" {
		t.Errorf("expected short hash abc12, got %q", entries[0].ShortHash)
	}
	if entries[0].Author != "Alice" {
		t.Errorf("expected author Alice, got %q", entries[0].Author)
	}
	if entries[0].TimeAgo != "2 hours ago" {
		t.Errorf("expected '2 hours ago', got %q", entries[0].TimeAgo)
	}
	if entries[0].Subject != "Fix typo" {
		t.Errorf("expected subject 'Fix typo', got %q", entries[0].Subject)
	}
}

func TestParseNoteHistoryLog_Empty(t *testing.T) {
	entries := parseNoteHistoryLog("")
	if entries != nil {
		t.Errorf("expected nil for empty output, got %v", entries)
	}
}

func TestParseNoteHistoryLog_MalformedLines(t *testing.T) {
	output := "incomplete|line\nvalid|hash|author|time|subject\n"
	entries := parseNoteHistoryLog(output)

	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
	if entries[0].Subject != "subject" {
		t.Errorf("expected subject 'subject', got %q", entries[0].Subject)
	}
}

func TestParseNoteHistoryLog_WhitespaceLines(t *testing.T) {
	output := "\n  \nhash|short|author|time|msg\n  \n"
	entries := parseNoteHistoryLog(output)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (whitespace skipped), got %d", len(entries))
	}
}

func TestParseNoteHistoryLog_SubjectWithPipe(t *testing.T) {
	// Subject may contain | characters — SplitN with 5 parts handles this
	output := "hash|short|author|time|feat: add x | y support\n"
	entries := parseNoteHistoryLog(output)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Subject != "feat: add x | y support" {
		t.Errorf("subject with pipe should be preserved, got %q", entries[0].Subject)
	}
}

func TestNoteHistory_NewDefaults(t *testing.T) {
	nh := NewNoteHistory()
	if nh.IsActive() {
		t.Error("new history should be inactive")
	}
}

func TestNoteHistory_VisibleHeight(t *testing.T) {
	nh := NewNoteHistory()
	nh.SetSize(80, 40)
	vh := nh.visibleHeight()
	if vh <= 0 {
		t.Errorf("visible height should be positive, got %d", vh)
	}
}

func TestNoteHistory_OverlayWidth(t *testing.T) {
	nh := NewNoteHistory()
	nh.SetSize(120, 40)
	ow := nh.overlayWidth()
	if ow <= 0 {
		t.Errorf("overlay width should be positive, got %d", ow)
	}
	if ow > 120 {
		t.Errorf("overlay width should not exceed terminal width, got %d", ow)
	}
}
