// Package aiprompts persists the user's saved AI prompts — short
// reusable phrasings they want one-click access to from any AI
// surface (inline editor menu, chat overlay). Distinct from
// recent-prompts (auto-captured history) and from the built-in
// presets (curated, ships with the app): library prompts are
// USER-CURATED and persist deliberately.
//
// Single record per vault. Storage at <vault>/.granit/ai-prompts.json
// via atomicio. Mirrors the shape pattern used by internal/roots —
// pure data + IO, no behaviour.
package aiprompts

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

// Scope describes when an entry should appear in a surface. The
// inline AI menu filters by current cursor state (selection vs
// empty); chat surfaces typically render everything.
//
//   selection — only meaningful when text is selected (rewrite,
//               improve, explain)
//   cursor    — only meaningful at an empty cursor (continue,
//               brainstorm, outline)
//   either    — works in both modes
type Scope string

const (
	ScopeSelection Scope = "selection"
	ScopeCursor    Scope = "cursor"
	ScopeEither    Scope = "either"
)

// Entry is one saved prompt. ID is a ULID stamped server-side on
// create; CreatedAt is RFC3339. Label is the short name shown in
// the UI ("My voice", "5-bullet summary", etc.); Prompt is the
// full text fed to the model.
type Entry struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Prompt    string `json:"prompt"`
	Scope     Scope  `json:"scope"`
	CreatedAt string `json:"created_at"`
}

// Library is the persisted record. One per vault.
type Library struct {
	Entries   []Entry `json:"entries"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

// StatePath is .granit/ai-prompts.json.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "ai-prompts.json")
}

// Load returns the vault's prompt library. Missing or corrupt file →
// zero Library (empty entries slice) so callers can iterate without
// a nil-check.
func Load(vaultRoot string) Library {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return Library{Entries: []Entry{}}
	}
	var lib Library
	if err := json.Unmarshal(data, &lib); err != nil {
		return Library{Entries: []Entry{}}
	}
	if lib.Entries == nil {
		lib.Entries = []Entry{}
	}
	return lib
}

// Save writes the library via atomic tmp+rename. UpdatedAt is
// stamped here so every persisted record carries a truthful "last
// touched" timestamp.
//
// Validates: every entry has a non-empty label + prompt, a valid
// scope, a stable ID (server-side ULID expected), no duplicate IDs.
func Save(vaultRoot string, lib Library) error {
	if vaultRoot == "" {
		return errors.New("aiprompts: empty vault root")
	}
	now := time.Now().UTC()
	seen := make(map[string]bool, len(lib.Entries))
	for i := range lib.Entries {
		e := &lib.Entries[i]
		e.Label = strings.TrimSpace(e.Label)
		e.Prompt = strings.TrimSpace(e.Prompt)
		if e.Label == "" {
			return fmt.Errorf("aiprompts: entry %d has empty label", i)
		}
		if e.Prompt == "" {
			return fmt.Errorf("aiprompts: entry %q has empty prompt", e.Label)
		}
		if e.Scope == "" {
			e.Scope = ScopeEither
		}
		if e.Scope != ScopeSelection && e.Scope != ScopeCursor && e.Scope != ScopeEither {
			return fmt.Errorf("aiprompts: entry %q has invalid scope %q", e.Label, e.Scope)
		}
		if e.ID == "" {
			return fmt.Errorf("aiprompts: entry %q missing id", e.Label)
		}
		if seen[e.ID] {
			return fmt.Errorf("aiprompts: duplicate id %q", e.ID)
		}
		seen[e.ID] = true
		if e.CreatedAt == "" {
			e.CreatedAt = now.Format(time.RFC3339)
		}
	}
	lib.UpdatedAt = now.Format(time.RFC3339)
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lib, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}
