// Package roots is the canonical schema + IO for granit's "rooted"
// surface — a contemplative diagram of who the user IS in Christ.
// Distinct from vision (mission + values + season focus): vision
// answers "what am I doing"; roots answers "where am I rooted."
//
// Single record per vault. Center holds the user's centering phrase
// (default "Christ") with an optional scripture anchor (e.g. Col 1:17,
// "and he is before all things, and by him all things consist").
// Around the center sit 4 concentric rings, each holding any number
// of named nodes:
//
//   Ring 1 — Identity: who am I in Christ? (son, beloved, baptized…)
//   Ring 2 — Callings: what am I FOR? (husband, friend, craftsman…)
//   Ring 3 — Gifts:    what has been given? (talents, charisms…)
//   Ring 4 — Longings: what do I yearn toward? (the not-yet…)
//
// What roots is NOT:
//   - A skill tree. Nodes carry no level, no XP, no completion %.
//   - A goal list. Goals are striving forward; roots is standing still.
//   - Auto-populated from tasks/notes. Hand-tended only. The discipline
//     of naming what's true is the point.
//
// See feedback-life-tree-not-gamified.md in user memory — if a future
// change adds a number-that-goes-up to this surface, the change is
// wrong on purpose.
//
// Pure data + IO only. Stored at <vault>/.granit/roots.json.
package roots

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Ring labels live in code, not in user data, so the contemplative
// shape stays stable across vaults. The user can't rename them — the
// ring meaning is part of the discipline, not a setting.
const (
	RingIdentity = 1
	RingCallings = 2
	RingGifts    = 3
	RingLongings = 4
)

// RingLabels maps ring numbers to their human-readable name. Mirrored
// on the frontend; kept here so handlers can stamp labels into the
// response without the client having to know magic numbers.
var RingLabels = map[int]string{
	RingIdentity: "Identity",
	RingCallings: "Callings",
	RingGifts:    "Gifts",
	RingLongings: "Longings",
}

// Node is one item rooted in a ring. Label is the only required
// field. Scripture is a freeform reference string ("Ps 1:3", "John
// 15:5"); we don't parse it — the user types whichever convention
// they like, the UI renders it as-is.
//
// RelatedNotes is a list of vault-relative note paths (the same form
// wikilinks resolve to). The detail panel renders them as click-
// through links. Empty = unlinked node.
type Node struct {
	ID           string    `json:"id"`
	Ring         int       `json:"ring"`
	Label        string    `json:"label"`
	Description  string    `json:"description,omitempty"`
	Scripture    string    `json:"scripture,omitempty"`
	RelatedNotes []string  `json:"related_notes,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Roots is the persisted state. One record per vault.
type Roots struct {
	Center    string    `json:"center,omitempty"`     // default "Christ"
	Anchor    string    `json:"anchor,omitempty"`     // scripture for the center
	Nodes     []Node    `json:"nodes,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultCenter is what an empty Roots renders as the center phrase.
// Not configurable per-vault in code (the user sets it via the UI);
// this only exists so an empty record has a sane fallback.
const DefaultCenter = "Christ"

// StatePath is .granit/roots.json.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "roots.json")
}

// Load returns the vault's roots record. Missing or corrupt file →
// zero Roots so the UI renders the empty-state "name your first
// rooted thing" prompt instead of crashing the page.
func Load(vaultRoot string) Roots {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return Roots{}
	}
	var r Roots
	if err := json.Unmarshal(data, &r); err != nil {
		return Roots{}
	}
	return r
}

// Save writes the record via atomic tmp+rename. UpdatedAt is stamped
// here, not at the call site, so every persisted record carries a
// truthful "last touched" timestamp.
//
// Validates: every node has a non-empty label and a valid ring (1-4),
// no duplicate IDs. A node missing CreatedAt gets one stamped (UI can
// send new nodes with just label+ring and let the server fill the
// audit fields).
func Save(vaultRoot string, r Roots) error {
	if vaultRoot == "" {
		return errors.New("roots: empty vault root")
	}
	now := time.Now().UTC()
	seen := make(map[string]bool, len(r.Nodes))
	for i := range r.Nodes {
		n := &r.Nodes[i]
		n.Label = strings.TrimSpace(n.Label)
		if n.Label == "" {
			return fmt.Errorf("roots: node %d has empty label", i)
		}
		if _, ok := RingLabels[n.Ring]; !ok {
			return fmt.Errorf("roots: node %q has invalid ring %d", n.Label, n.Ring)
		}
		if n.ID == "" {
			return fmt.Errorf("roots: node %q missing id", n.Label)
		}
		if seen[n.ID] {
			return fmt.Errorf("roots: duplicate node id %q", n.ID)
		}
		seen[n.ID] = true
		if n.CreatedAt.IsZero() {
			n.CreatedAt = now
		}
		n.UpdatedAt = now
	}
	r.Center = strings.TrimSpace(r.Center)
	r.Anchor = strings.TrimSpace(r.Anchor)
	r.UpdatedAt = now
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// IsEmpty reports whether the record is unset — no nodes and no
// custom center / anchor. The /roots page uses this to decide
// between the radial diagram and the empty-state prompt.
func (r Roots) IsEmpty() bool {
	return len(r.Nodes) == 0 && r.Center == "" && r.Anchor == ""
}

// NodesByRing returns the subset of nodes on the given ring, in
// stable order (creation order). Used by the handler for rendering;
// kept here so the ring grouping is a pure function of the record,
// not a presentation concern leaking into the API.
func (r Roots) NodesByRing(ring int) []Node {
	out := make([]Node, 0, len(r.Nodes))
	for _, n := range r.Nodes {
		if n.Ring == ring {
			out = append(out, n)
		}
	}
	return out
}
