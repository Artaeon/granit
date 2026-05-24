// Package visions is the multi-document vision catalogue —
// "Hauptvision", "Kurzversion", "Mission", "Stoicera / Arbeit",
// "Körper / Training", "Glaube" — each a named markdown narrative
// with its own edit history. Distinct from the older sibling package
// `vision` (singular), which still owns the values list + season
// focus + notes sidecar; those are different shapes (list / dated
// phrase / freeform) that don't fit the doc-per-narrative model.
//
// Storage: single file at .granit/visions.json. Vision edits are
// low-frequency human writes, so we don't fan out per-doc files —
// one atomic write per save is fine. Each Doc carries its own
// History (capped at HistoryCap entries), so revising a vision
// records the previous content plus the reason the user typed.
//
// Pure data + IO. The HTTP layer lives in serveapi/handlers_visions.go.
package visions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

const (
	FileRel        = ".granit/visions.json"
	DefaultVersion = 1
	// HistoryCap bounds the per-doc history slice so a user with
	// frequent micro-edits doesn't grow the file unboundedly. 50 is
	// well past any plausible "look at the arc of my vision" use case;
	// older entries get dropped when we cross the cap.
	HistoryCap = 50
)

// Doc is one named vision narrative. Keys are stable identifiers
// used in URLs (PUT /api/v1/visions/{key}) and by the Kurzvision
// widget on the dashboard to find the doc to surface. The seeded
// keys (main / short / mission / work / body / faith) ship by
// default; users can add custom keys (e.g., "family") via POST.
type Doc struct {
	Key       string         `json:"key"`
	Label     string         `json:"label"`
	Content   string         `json:"content,omitempty"`
	// Pinned docs are eligible for surfacing on the home page —
	// today the Kurzvision widget reads the first pinned doc. Could
	// extend later to multiple pinned surfaces.
	Pinned    bool           `json:"pinned,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
	History   []HistoryEntry `json:"history,omitempty"`
}

// HistoryEntry snapshots the content BEFORE a successful edit, along
// with the user's reason for the change. Stored newest-first; older
// entries trim off the tail when the slice exceeds HistoryCap.
type HistoryEntry struct {
	When    time.Time `json:"when"`
	Reason  string    `json:"reason"`
	// Content is the previous content (pre-edit snapshot). The new
	// content lives in the Doc.Content field. Restoring a history
	// entry == set Doc.Content to entry.Content and append a new
	// history record with reason "restored from <when>".
	Content string `json:"content"`
}

// Store is the on-disk catalogue. Single-file layout keeps the
// edit-history surface together with the docs, which matters when
// auditing what the user has changed and why.
type Store struct {
	Version int   `json:"version"`
	Docs    []Doc `json:"docs"`
}

func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, FileRel)
}

// Load returns the vault's vision catalogue. Missing file → seed
// store with the six default keys; corrupted file returns the seed
// too (don't crash the page on a hand-edited JSON). The migration
// from the legacy singular-vision file is handled by the HTTP
// handler on first load, not here, so this stays pure-IO.
func Load(vaultRoot string) (Store, error) {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SeedStore(), nil
		}
		return Store{}, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return SeedStore(), nil
	}
	if s.Version == 0 {
		s.Version = DefaultVersion
	}
	return s, nil
}

// Save atomically writes the catalogue. UpdatedAt on each doc is the
// caller's responsibility — we don't touch existing timestamps so a
// "reorder docs without editing" pass doesn't fake an edit.
func Save(vaultRoot string, s Store) error {
	if vaultRoot == "" {
		return errors.New("visions: empty vault root")
	}
	if s.Version == 0 {
		s.Version = DefaultVersion
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// SeedStore returns the default catalogue for fresh vaults. Order
// reflects life-anchor priority: the long-form main vision is the
// frame, the short version is its today-view distillation, the
// mission is the personal "why I'm here" statement, and the three
// domain docs (work / body / faith) carve the main vision into
// life areas the user wants to track distinctly. User can reorder
// (future) or add custom keys via POST.
func SeedStore() Store {
	return Store{
		Version: DefaultVersion,
		Docs: []Doc{
			{Key: "main", Label: "Hauptvision"},
			{Key: "short", Label: "Kurzversion", Pinned: true},
			{Key: "mission", Label: "Mission"},
			{Key: "work", Label: "Stoicera / Arbeit"},
			{Key: "body", Label: "Körper / Training"},
			{Key: "faith", Label: "Glaube"},
		},
	}
}

// FindDoc returns a pointer to the named doc in the slice, or nil
// if no doc with that key exists. Callers mutate the returned doc
// in place then call Save on the full Store.
func FindDoc(s *Store, key string) *Doc {
	for i := range s.Docs {
		if s.Docs[i].Key == key {
			return &s.Docs[i]
		}
	}
	return nil
}

// ApplyEdit updates a doc's Content and records the previous content
// as a history entry with the user's reason. Newest history entry
// goes first; oldest are dropped when crossing HistoryCap.
//
// An empty Reason is allowed — the UI requires it for user edits but
// programmatic seeding (e.g., the legacy-mission migration) skips
// reason. The history entry is still recorded for traceability;
// callers can distinguish auto-migrations by checking Reason == "".
func ApplyEdit(d *Doc, nextContent, reason string) {
	prev := d.Content
	d.Content = nextContent
	d.UpdatedAt = time.Now().UTC()
	// Don't push a history entry if nothing actually changed — saves
	// the user from polluting history when they hit "save" without
	// making changes.
	if prev == nextContent {
		return
	}
	entry := HistoryEntry{
		When:    d.UpdatedAt,
		Reason:  reason,
		Content: prev,
	}
	d.History = append([]HistoryEntry{entry}, d.History...)
	if len(d.History) > HistoryCap {
		d.History = d.History[:HistoryCap]
	}
}

// FindPinned returns the first pinned doc, or nil. Used by the
// Kurzvision dashboard widget to find the doc to surface on the
// home page. If the user pins multiple, only the first wins —
// "Kurzversion" is a single-slot concept by design.
func FindPinned(s Store) *Doc {
	for i := range s.Docs {
		if s.Docs[i].Pinned {
			return &s.Docs[i]
		}
	}
	return nil
}
