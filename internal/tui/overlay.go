package tui

// Shared overlay contract.
//
// ~108 overlays in internal/tui originally re-implemented some subset
// of the same shape: an `active bool` flag plus `SetSize`, `IsActive`,
// `Close`, and an `Open` entry point. The lack of a declared interface
// meant app_update.go's dispatch (200+ `IsActive()` branches) couldn't
// collapse to a loop.
//
// This file introduces the interface and an OverlayBase embed so the
// next refactor pass — or a new overlay written today — has a single,
// documented shape to follow. Migration is incremental: each overlay
// moves to OverlayBase in its own commit. Future callers that want to
// treat overlays uniformly (e.g. "is anything active?", "close the
// topmost overlay") can type-assert against the interface.
//
// Keeping Update/View off the interface *for now* is deliberate:
// bubbletea's value-receiver pattern for those two methods means every
// overlay's Update returns its own concrete type (e.g.
// `func (k Kanban) Update(msg tea.Msg) (Kanban, tea.Cmd)`), and
// unifying those into an interface would mean rewriting every
// overlay's Update to return `Overlay`. That's the "big lever"
// refactor, not part of the initial contract.

// Overlay is the minimal contract every overlay in internal/tui ought
// to satisfy. Update and View are intentionally NOT part of the
// interface — see the file-header comment. New overlay code should
// embed OverlayBase to satisfy this interface for free.
type Overlay interface {
	// IsActive reports whether the overlay is currently visible.
	IsActive() bool
	// SetSize informs the overlay of the host terminal's dimensions.
	// Called on every resize and before first render.
	SetSize(width, height int)
	// Close deactivates the overlay and resets any transient state.
	Close()
}

// OverlayBase is an embeddable struct providing the canonical fields
// and methods that satisfy the Overlay interface. Overlays should embed
// it to remove ~15 lines of active/width/height boilerplate:
//
//	type MyOverlay struct {
//	    OverlayBase
//	    // domain-specific fields only
//	}
//
//	func (o *MyOverlay) Open() { o.active = true }
//
// The receiver is a pointer type because SetSize and Close mutate state.
type OverlayBase struct {
	active        bool
	width, height int
}

// IsActive satisfies Overlay.
func (o *OverlayBase) IsActive() bool { return o != nil && o.active }

// SetSize satisfies Overlay.
func (o *OverlayBase) SetSize(width, height int) {
	o.width = width
	o.height = height
}

// Close satisfies Overlay. It resets the active flag; overlays that
// need additional teardown (cancel in-flight requests, clear caches)
// should override Close on their own receiver.
func (o *OverlayBase) Close() { o.active = false }

// Activate flips the overlay to active and is the canonical alternative
// to open methods that don't take parameters. Overlays with non-trivial
// Open signatures (loading data, starting background work) should
// define their own Open method and call Activate from it.
func (o *OverlayBase) Activate() { o.active = true }

// Width returns the overlay's current width; helper for callers that
// don't want to touch the embedded field directly.
func (o *OverlayBase) Width() int { return o.width }

// Height returns the overlay's current height.
func (o *OverlayBase) Height() int { return o.height }

// Compile-time assertion that OverlayBase satisfies Overlay. Rebuilding
// will fail loudly if the interface or the embed drift apart.
var _ Overlay = (*OverlayBase)(nil)
