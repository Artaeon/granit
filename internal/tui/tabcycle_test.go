package tui

import "testing"

// TestPassthroughChord_ReliableTabCycleEscapes guards the additions
// from Phase 12 — the alternative tab-cycle shortcuts must escape
// any active feature tab the same way Ctrl+Tab/Ctrl+Shift+Tab do.
// Otherwise users on terminals that intercept Ctrl+Tab would be
// stuck inside a feature tab with no way to switch.
func TestPassthroughChord_ReliableTabCycleEscapes(t *testing.T) {
	mustEscape := []string{
		"ctrl+pgup", "ctrl+pgdown",
		"alt+,", "alt+.",
	}
	for _, key := range mustEscape {
		if !isPassthroughChord(key) {
			t.Errorf("%q must be a passthrough chord — feature tabs would trap the user", key)
		}
	}
}
