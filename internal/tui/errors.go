package tui

// Unified error-reporting surface for the TUI layer.
//
// Before this file, error handling across internal/tui was a patchwork
// of direct m.statusbar.SetError / SetMessage calls (plus overlay-local
// errMsg fields and stray log.Printf lines) with no single entry point,
// so debug logging and user-visible status drifted apart: some errors
// got both, some got neither, some only got a silent `_ =` discard.
//
// reportError and reportInfo route through one place so any future
// change (telemetry, log aggregation, toast promotion) lands in one
// spot. The migration is ongoing — direct statusbar.Set* calls still
// exist, but all *error* paths should funnel through reportError.

import (
	"fmt"
	"log"
)

// reportError surfaces err to the user via the statusbar and also
// writes it to the debug log. context is a short label describing what
// the user was doing ("save note", "apply plan"); it prefixes the
// user-visible message and tags the log line so grep finds related
// failures quickly.
//
// No-op when err is nil so callers can write:
//
//	m.reportError("save note", atomicWriteNote(path, content))
//
// without guarding.
func (m *Model) reportError(context string, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if context != "" {
		msg = context + ": " + msg
	}
	m.statusbar.SetError(msg)
	log.Printf("granit error [%s]: %v", context, err)
}

// reportInfo surfaces an informational (non-error) status message via
// the statusbar. Equivalent to m.statusbar.SetMessage but routed here
// so the "where user messages come from" surface is explicitly a
// single call site alongside reportError.
func (m *Model) reportInfo(format string, args ...any) {
	if len(args) == 0 {
		m.statusbar.SetMessage(format)
		return
	}
	m.statusbar.SetMessage(fmt.Sprintf(format, args...))
}
