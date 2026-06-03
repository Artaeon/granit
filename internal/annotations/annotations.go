// Package annotations is the canonical schema + IO for margin
// annotations on notes — user-authored marginalia attached to a
// specific line in a note, displayed as a side column in the
// editor / preview.
//
// Why margin annotations (vs editing the note body):
//
//   - Research workflow: the user wants to question a passage,
//     mark a counter-argument, or note "I disagree because…"
//     without polluting the source text. Marginalia is the
//     classic move — Augustine, Erasmus, Coleridge all annotated
//     books they didn't author. This is the same shape, but
//     digitally tied to the user's vault.
//
//   - Note-on-note feedback: re-reading old notes weeks later
//     often surfaces "this was wrong" or "this matters". A
//     margin layer keeps the temporal stratification visible —
//     you see the past claim and the present challenge side by
//     side, instead of silently overwriting.
//
//   - Highlighting parity: the books feature ships passage
//     highlights; this is the parallel surface for notes. The
//     two aren't merged because notes are user-authored
//     (mutable, edited often) while books are imported (frozen).
//
// Storage: <vault>/.granit/annotations.json — single file holding
// every annotation across the vault, keyed by NotePath. Single
// file (vs per-note) because:
//
//   - the surface stays small (kilobytes for hundreds of
//     annotations, not megabytes)
//   - listing across notes ("show me every annotation about
//     virtue") works without a directory walk
//   - rename surgery is one path-rewrite over one file rather
//     than file moves keyed by hash-of-old-path
//
// Stdlib + atomicio only. No HTTP, no rendering.
package annotations

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/oklog/ulid/v2"
)

// storeMu serialises every read-modify-write against the
// annotations store. The store is single-file by design (see top
// comment); two concurrent Add() / Patch() / Delete() calls would
// otherwise both LoadAll → modify → SaveAll, with the second
// writer's pre-modify read missing the first writer's commit.
// The "AI accept-all" flow can fire 5 POSTs in rapid succession,
// the WS broadcast can trigger a reload mid-write across tabs,
// and the user can rename a note while another tab is creating
// an annotation — all real scenarios this guard covers.
//
// Read-only operations (LoadAll, ListForNote) don't need the
// lock — the atomicio rename is OS-atomic, so a reader either
// sees the pre- or post-write state, never a torn one.
var storeMu sync.Mutex

// AnchorPreviewLen is the cap on how much of the original line we
// snapshot on create. Used by re-anchoring when the line numbers
// shift after edits — match the snapshot text against the live
// note body to find where the line actually moved to.
const AnchorPreviewLen = 80

// Annotation is one margin note. LineNum is 1-indexed (matches
// editor display). AnchorText is the live note's content at
// creation time, stored separately so re-anchoring can find the
// passage even after surrounding edits shifted line numbers.
type Annotation struct {
	ID         string `json:"id"`
	NotePath   string `json:"notePath"`
	LineNum    int    `json:"lineNum"`
	AnchorText string `json:"anchorText"`
	Text       string `json:"text"`
	// Color follows the editor selection-toolbar palette tokens
	// (yellow / blue / green / pink). Empty defaults to yellow on
	// render — kept here as raw string so a future palette change
	// doesn't break stored data.
	Color     string `json:"color,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Store is the on-disk envelope. Version lets a future schema
// change migrate cleanly. Default zero value is a valid empty store.
type Store struct {
	Version     int          `json:"version"`
	Annotations []Annotation `json:"annotations"`
}

const currentVersion = 1

// statePath returns the canonical on-disk location.
func statePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "annotations.json")
}

// LoadAll reads every annotation. Returns an empty store (not nil
// + error) on missing file — a never-annotated vault is a valid
// state. Parse failures DO error so the caller can surface
// "couldn't read annotations" rather than silently masking
// corruption.
func LoadAll(vaultRoot string) (Store, error) {
	raw, err := os.ReadFile(statePath(vaultRoot))
	if errors.Is(err, fs.ErrNotExist) {
		return Store{Version: currentVersion}, nil
	}
	if err != nil {
		return Store{}, err
	}
	var s Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return Store{}, err
	}
	if s.Version == 0 {
		s.Version = currentVersion
	}
	return s, nil
}

// SaveAll writes the store atomically. Sorts annotations by
// (NotePath, LineNum, CreatedAt) on the way out so the JSON is
// stable across saves — a diff-friendly file matters when the
// vault is under git autocommit.
func SaveAll(vaultRoot string, s Store) error {
	if s.Version == 0 {
		s.Version = currentVersion
	}
	sort.SliceStable(s.Annotations, func(i, j int) bool {
		a, b := s.Annotations[i], s.Annotations[j]
		if a.NotePath != b.NotePath {
			return a.NotePath < b.NotePath
		}
		if a.LineNum != b.LineNum {
			return a.LineNum < b.LineNum
		}
		return a.CreatedAt < b.CreatedAt
	})
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(statePath(vaultRoot), raw)
}

// ListForNote returns every annotation attached to the given note
// path, sorted by line number. Cheap O(N) over the whole store —
// the typical N (annotations across a vault) is small enough that
// indexing isn't worth the maintenance.
func ListForNote(vaultRoot, notePath string) ([]Annotation, error) {
	s, err := LoadAll(vaultRoot)
	if err != nil {
		return nil, err
	}
	out := make([]Annotation, 0, 4)
	for _, a := range s.Annotations {
		if a.NotePath == notePath {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LineNum < out[j].LineNum })
	return out, nil
}

// Add inserts a new annotation. Allocates an ID + timestamps + a
// truncated AnchorText so the saved row stays self-contained.
func Add(vaultRoot string, a Annotation) (Annotation, error) {
	if strings.TrimSpace(a.NotePath) == "" {
		return Annotation{}, errors.New("annotations: notePath required")
	}
	if a.LineNum < 1 {
		return Annotation{}, errors.New("annotations: lineNum must be 1-indexed")
	}
	if strings.TrimSpace(a.Text) == "" {
		return Annotation{}, errors.New("annotations: empty annotation text")
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := LoadAll(vaultRoot)
	if err != nil {
		return Annotation{}, err
	}
	if a.ID == "" {
		a.ID = ulid.Make().String()
	}
	a.AnchorText = clipAnchor(a.AnchorText)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	s.Annotations = append(s.Annotations, a)
	if err := SaveAll(vaultRoot, s); err != nil {
		return Annotation{}, err
	}
	return a, nil
}

// Patch applies a partial update. The mutator receives a pointer
// to the live entry so the caller can change Text / Color /
// LineNum / AnchorText without rebuilding the struct. Returns
// ErrNotFound if the id doesn't resolve.
func Patch(vaultRoot, id string, mutate func(*Annotation)) (Annotation, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := LoadAll(vaultRoot)
	if err != nil {
		return Annotation{}, err
	}
	for i := range s.Annotations {
		if s.Annotations[i].ID == id {
			mutate(&s.Annotations[i])
			s.Annotations[i].AnchorText = clipAnchor(s.Annotations[i].AnchorText)
			s.Annotations[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := SaveAll(vaultRoot, s); err != nil {
				return Annotation{}, err
			}
			return s.Annotations[i], nil
		}
	}
	return Annotation{}, ErrNotFound
}

// Delete removes one annotation by id. Idempotent — no-op + nil
// error if the id doesn't exist (the user's intent of "this should
// not be here" is satisfied either way).
func Delete(vaultRoot, id string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := LoadAll(vaultRoot)
	if err != nil {
		return err
	}
	out := s.Annotations[:0]
	for _, a := range s.Annotations {
		if a.ID != id {
			out = append(out, a)
		}
	}
	s.Annotations = out
	return SaveAll(vaultRoot, s)
}

// RewriteNotePath updates every annotation tied to oldPath to
// reference newPath instead. Called when the user renames a note
// so annotations don't dangle. No-op if no annotations match
// oldPath. Returns the number of rewrites.
func RewriteNotePath(vaultRoot, oldPath, newPath string) (int, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := LoadAll(vaultRoot)
	if err != nil {
		return 0, err
	}
	count := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range s.Annotations {
		if s.Annotations[i].NotePath == oldPath {
			s.Annotations[i].NotePath = newPath
			s.Annotations[i].UpdatedAt = now
			count++
		}
	}
	if count == 0 {
		return 0, nil
	}
	return count, SaveAll(vaultRoot, s)
}

// clipAnchor truncates anchor text to AnchorPreviewLen chars.
// Done at write time so the on-disk file stays bounded — a user
// pasting a 5 KB line into the editor won't blow the annotations
// store up to match.
func clipAnchor(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > AnchorPreviewLen {
		s = s[:AnchorPreviewLen]
	}
	return s
}

// Reflow updates LineNum for every annotation tied to notePath so
// each annotation points at the line whose content best matches its
// stored AnchorText. Called after a note body is written so
// annotations don't permanently drift when surrounding edits shift
// line numbers — without this, inserting a paragraph above an
// anchored line silently moves the annotation card to the wrong
// passage and there's no path back without manual editing.
//
// Algorithm per annotation in the named note:
//
//   - AnchorText empty (legacy rows): skip.
//   - Current LineNum-line already matches the anchor: no change.
//   - Scan all lines, collect matches, pick the one closest to the
//     original LineNum (closest-wins when the same passage repeats).
//   - Zero matches: leave LineNum untouched so the orphaned card
//     stays visible — the user can re-anchor or delete it.
//
// Match is "trimmed line starts with anchor OR equals it" (anchor
// was clipped to AnchorPreviewLen at write time, so the live line
// is typically the longer string). Returns the number of
// annotations whose LineNum changed.
func Reflow(vaultRoot, notePath, body string) (int, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := LoadAll(vaultRoot)
	if err != nil {
		return 0, err
	}
	// Early exit: nothing references this note, no work needed and
	// no SaveAll write on a hot path that fires once per save.
	hasAny := false
	for _, a := range s.Annotations {
		if a.NotePath == notePath {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return 0, nil
	}
	lines := strings.Split(body, "\n")
	changed := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range s.Annotations {
		a := &s.Annotations[i]
		if a.NotePath != notePath || a.AnchorText == "" {
			continue
		}
		if a.LineNum >= 1 && a.LineNum <= len(lines) && lineMatchesAnchor(lines[a.LineNum-1], a.AnchorText) {
			continue
		}
		bestLine := 0
		bestDist := len(lines) + 1
		for li, ln := range lines {
			if !lineMatchesAnchor(ln, a.AnchorText) {
				continue
			}
			dist := li + 1 - a.LineNum
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestLine = li + 1
			}
		}
		if bestLine == 0 || bestLine == a.LineNum {
			continue
		}
		a.LineNum = bestLine
		a.UpdatedAt = now
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	return changed, SaveAll(vaultRoot, s)
}

func lineMatchesAnchor(line, anchor string) bool {
	t := strings.TrimSpace(line)
	if t == "" || anchor == "" {
		return false
	}
	return t == anchor || strings.HasPrefix(t, anchor)
}

// ErrNotFound is returned by Patch when the supplied ID doesn't
// match any stored annotation. Callers map to 404.
var ErrNotFound = errors.New("annotations: not found")
