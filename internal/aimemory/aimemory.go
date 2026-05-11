// Package aimemory is granit's long-term memory store for the AI
// chat overlay — small key/value-ish facts the assistant should know
// about the user across every thread.
//
// The store is a single JSON file at <vault>/.granit/ai-memory.json;
// reads / writes go through atomicio so a crash mid-write can't tear
// the file. Each fact carries an id (ULID), content, optional tags,
// and a created/updated timestamp.
//
// Privacy / surfacing: facts live in the user's vault alongside
// everything else granit stores; nothing leaves the box. The chat
// overlay injects the fact list into every thread's system prelude
// so the model can reference "user is vegetarian" / "user's wife is
// Anna" without the user re-stating it. The user owns the list —
// adding is explicit (slash command, AI-proposed action chip, or
// the settings UI), removing is one click.
//
// Why a flat list (no key/value): facts are sentences, not symbols.
// "User's mother lives in Vienna" reads better than
// `mother.city = "Vienna"`. The model is also better at folding
// natural sentences into its reply than at navigating a structured
// object.
package aimemory

import (
	"encoding/json"
	"errors"
	"fmt"
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

// Fact is one entry in the memory store. Content is a free-form
// sentence; Tags is an optional list of free-form labels the user
// can use to group/filter (e.g. "family", "health"); timestamps are
// RFC3339Nano UTC.
type Fact struct {
	ID        string   `json:"id"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
}

// Store wraps the on-disk JSON. Version lets a future schema change
// migrate cleanly without breaking old vaults.
type Store struct {
	Version int    `json:"version"`
	Facts   []Fact `json:"facts"`
}

const currentVersion = 1

// MaxFacts caps the store at a sensible upper bound. The chat
// prelude budget can't fit "every fact the user ever recorded" —
// 200 facts × ~80 chars each is ~16 KB of system context, already
// noticeable. Beyond that the user should be writing notes, not
// memory entries.
const MaxFacts = 200

// MaxContentLen caps a single fact at the size of a long tweet so
// the prelude stays bounded. Longer-form context belongs in a note.
const MaxContentLen = 240

// storeMu serialises read-modify-write across the package — same
// pattern annotations + book sidecars use. The "AI proposes a
// remember-this action and the user clicks it twice in rapid
// succession" path is a real way to race the store; the mutex
// makes the second add see the first's commit before its own
// load-mutate-save begins.
var storeMu sync.Mutex

func statePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "ai-memory.json")
}

// Load reads the store. Returns an empty store (Version=1, no facts)
// when the file doesn't exist yet — a fresh vault is a valid state.
// Parse failure surfaces as an error so the caller can decide
// whether to fail loudly or fall back to empty.
func Load(vaultRoot string) (Store, error) {
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

// Save writes the store atomically. Sorts by CreatedAt ascending
// so the JSON is stable across saves — a diff-friendly file matters
// when the vault is under git autocommit.
func Save(vaultRoot string, s Store) error {
	if s.Version == 0 {
		s.Version = currentVersion
	}
	sort.SliceStable(s.Facts, func(i, j int) bool {
		return s.Facts[i].CreatedAt < s.Facts[j].CreatedAt
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

// Add inserts a new fact, validating + assigning id + timestamps.
// Idempotent on duplicate content: if a fact with byte-equal content
// already exists, returns the existing entry without adding a second
// copy (the AI proposing the same memory twice is otherwise easy to
// hit).
func Add(vaultRoot string, content string, tags []string) (Fact, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Fact{}, errors.New("aimemory: empty content")
	}
	if len(content) > MaxContentLen {
		return Fact{}, fmt.Errorf("aimemory: content over %d chars (got %d)", MaxContentLen, len(content))
	}
	tags = normalizeTags(tags)
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := Load(vaultRoot)
	if err != nil {
		return Fact{}, err
	}
	for _, f := range s.Facts {
		if f.Content == content {
			return f, nil
		}
	}
	if len(s.Facts) >= MaxFacts {
		return Fact{}, fmt.Errorf("aimemory: store full (max %d) — delete some facts first", MaxFacts)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	f := Fact{
		ID:        ulid.Make().String(),
		Content:   content,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.Facts = append(s.Facts, f)
	if err := Save(vaultRoot, s); err != nil {
		return Fact{}, err
	}
	return f, nil
}

// Patch updates content/tags of an existing fact. Returns
// ErrNotFound when the id doesn't resolve.
func Patch(vaultRoot, id, content string, tags []string) (Fact, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := Load(vaultRoot)
	if err != nil {
		return Fact{}, err
	}
	for i := range s.Facts {
		if s.Facts[i].ID != id {
			continue
		}
		if c := strings.TrimSpace(content); c != "" {
			if len(c) > MaxContentLen {
				return Fact{}, fmt.Errorf("aimemory: content over %d chars", MaxContentLen)
			}
			s.Facts[i].Content = c
		}
		if tags != nil {
			s.Facts[i].Tags = normalizeTags(tags)
		}
		s.Facts[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := Save(vaultRoot, s); err != nil {
			return Fact{}, err
		}
		return s.Facts[i], nil
	}
	return Fact{}, ErrNotFound
}

// Delete removes a fact by id. Idempotent — no error when the id
// doesn't resolve. The user's intent ("this should not be there")
// is satisfied either way.
func Delete(vaultRoot, id string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	s, err := Load(vaultRoot)
	if err != nil {
		return err
	}
	out := s.Facts[:0]
	for _, f := range s.Facts {
		if f.ID != id {
			out = append(out, f)
		}
	}
	s.Facts = out
	return Save(vaultRoot, s)
}

// Snapshot returns a copy of every fact in CreatedAt order — the
// chat overlay's prelude builder consumes this. Returns nil + nil
// on a fresh vault so the caller can short-circuit the system
// message when there's nothing to inject.
func Snapshot(vaultRoot string) ([]Fact, error) {
	s, err := Load(vaultRoot)
	if err != nil {
		return nil, err
	}
	if len(s.Facts) == 0 {
		return nil, nil
	}
	out := make([]Fact, len(s.Facts))
	copy(out, s.Facts)
	return out, nil
}

// normalizeTags trims, lowercases, drops empties + duplicates while
// preserving the input order. Keeps the JSON stable so the same set
// of tags re-saves to the same byte sequence.
func normalizeTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ErrNotFound is returned by Patch when the id doesn't match a
// stored fact. Callers map to 404.
var ErrNotFound = errors.New("aimemory: not found")
