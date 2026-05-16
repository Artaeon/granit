// Package roots is the canonical schema + IO for granit's life-tree
// surface — a stats dashboard of the four domains of a life under
// God: spirit, mind, body, vocation. The web /roots page renders
// auto-pulled metrics from existing modules (bible, virtues, habits,
// measurements, books, goals, finance) alongside hand-tended items
// the user adds for things that don't have their own module yet
// (languages spoken, current interests, etc).
//
// This file holds the storage for the hand-tended items: any number
// of named nodes, grouped by ring (Spirit, Mind, Body, Vocation),
// with optional scripture references and related-note links. Center
// holds a centering phrase (default "Christ") with an optional
// scripture anchor (e.g. Col 1:17).
//
// History — earlier framing was "contemplative not gamified". The
// user explicitly overrode that on 2026-05-16, asking for the stats
// dashboard shape. This package's storage continues to support
// hand-tended additions; the metrics live in their own modules.
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

// Ring labels live in code, not in user data, so the layout stays
// stable across vaults. Rings group the four domains of a life under
// God: spirit (faith, virtues, prayer), mind (knowledge, language,
// learning), body (health, habits, measurements), vocation (work,
// goals, finance). The user can't rename them — the categorisation
// is part of the product.
const (
	RingSpirit   = 1
	RingMind     = 2
	RingBody     = 3
	RingVocation = 4
)

// RingLabels maps ring numbers to their human-readable name. Mirrored
// on the frontend; kept here so handlers can stamp labels into the
// response without the client having to know magic numbers.
var RingLabels = map[int]string{
	RingSpirit:   "Spirit",
	RingMind:     "Mind",
	RingBody:     "Body",
	RingVocation: "Vocation",
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
