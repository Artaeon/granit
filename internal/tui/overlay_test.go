package tui

import "testing"

// dummyOverlay demonstrates the minimum migration: embed OverlayBase,
// inherit the contract, add domain-specific fields.
type dummyOverlay struct {
	OverlayBase
	count int
}

func TestOverlayBase_DefaultStateIsInactive(t *testing.T) {
	d := &dummyOverlay{}
	if d.IsActive() {
		t.Error("zero-value OverlayBase should report inactive")
	}
	if d.Width() != 0 || d.Height() != 0 {
		t.Errorf("zero-value dimensions = (%d, %d), want (0, 0)", d.Width(), d.Height())
	}
}

func TestOverlayBase_Activate_Close_IsSymmetric(t *testing.T) {
	d := &dummyOverlay{}
	d.Activate()
	if !d.IsActive() {
		t.Error("after Activate, IsActive should be true")
	}
	d.Close()
	if d.IsActive() {
		t.Error("after Close, IsActive should be false")
	}
}

func TestOverlayBase_SetSize_PersistsWidthAndHeight(t *testing.T) {
	d := &dummyOverlay{}
	d.SetSize(120, 40)
	if d.Width() != 120 || d.Height() != 40 {
		t.Errorf("SetSize not persisted: got (%d, %d)", d.Width(), d.Height())
	}
}

func TestOverlayBase_NilReceiver_IsActiveSafe(t *testing.T) {
	// Defensive: calling IsActive on a nil pointer is the cheapest
	// way to ask "does this optional overlay exist and is it open?"
	// The helper must not panic.
	var nilOverlay *OverlayBase
	if nilOverlay.IsActive() {
		t.Error("nil OverlayBase should report inactive, not panic")
	}
}

// Compile-time check that the Overlay interface is satisfied by an
// OverlayBase user — guards against accidental method-signature drift.
func TestOverlayBase_SatisfiesOverlayInterface(t *testing.T) {
	var _ Overlay = (*dummyOverlay)(nil)
}
