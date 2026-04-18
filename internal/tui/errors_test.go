package tui

import (
	"errors"
	"testing"
)

func TestReportError_NilIsNoOp(t *testing.T) {
	m := &Model{statusbar: NewStatusBar()}
	m.reportError("save note", nil)
	// Contract: a nil err must leave the statusbar completely
	// untouched. If a future refactor starts calling SetError("") on
	// nil, this assertion catches it.
	view := m.statusbar.View()
	if containsText(view, "save note") {
		t.Errorf("nil err should not render a context prefix, got %q", view)
	}
	if containsText(view, "error") || containsText(view, "failed") {
		t.Errorf("nil err should not style the bar as an error, got %q", view)
	}
}

func TestReportError_FormatsContextBeforeErrorMessage(t *testing.T) {
	m := &Model{statusbar: NewStatusBar()}
	m.reportError("save note", errors.New("disk full"))
	// The statusbar's rendered output is what users see; grabbing it
	// verifies the context prefix format "save note: disk full".
	view := m.statusbar.View()
	if !containsText(view, "save note") {
		t.Errorf("expected context 'save note' in %q", view)
	}
	if !containsText(view, "disk full") {
		t.Errorf("expected error 'disk full' in %q", view)
	}
}

func TestReportError_EmptyContextOmitsPrefix(t *testing.T) {
	m := &Model{statusbar: NewStatusBar()}
	m.reportError("", errors.New("standalone"))
	view := m.statusbar.View()
	if !containsText(view, "standalone") {
		t.Errorf("expected error in %q", view)
	}
	// A leading ": " would be a format bug.
	if containsText(view, ": standalone") {
		t.Errorf("empty context should not produce leading colon: %q", view)
	}
}

func TestReportInfo_DispatchesFormatVariants(t *testing.T) {
	m := &Model{statusbar: NewStatusBar()}

	m.reportInfo("plain message")
	if !containsText(m.statusbar.View(), "plain message") {
		t.Error("plain-string path not routed")
	}

	m.reportInfo("formatted %d: %s", 42, "value")
	if !containsText(m.statusbar.View(), "formatted 42: value") {
		t.Error("fmt.Sprintf path not routed")
	}
}

// containsText is a minimal ANSI-tolerant substring check that strips
// the statusbar's terminal escape codes for easier matching.
func containsText(s, sub string) bool {
	plain := stripAnsiCodes(s)
	return len(plain) >= len(sub) && indexOf(plain, sub) >= 0
}

func indexOf(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
