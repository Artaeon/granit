package objects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Registry holds the active set of Types for a vault — built-ins plus
// any vault-local overrides loaded from `.granit/types/<id>.json`.
//
// Lifecycle:
//
//   r := NewRegistry()                     // built-ins only
//   r.LoadVaultDir(vaultRoot)              // overlay vault overrides
//   t, ok := r.ByID("person")              // lookup
//   for _, t := range r.All() { ... }      // iterate (sorted)
//
// Concurrency: a Registry is read-mostly; load it once, share many
// readers. Mutating methods (LoadVaultDir, Set) are NOT thread-safe.
// The TUI rebuilds the registry on vault refresh rather than mutating
// in place, so this is fine.
type Registry struct {
	// types maps ID → Type. The ordering of All() comes from the
	// sorted keys of this map for stability across UI refreshes.
	types map[string]Type
}

// NewRegistry returns a Registry pre-loaded with the built-in starter
// types. Vault-local overrides should be applied with LoadVaultDir
// after construction.
func NewRegistry() *Registry {
	r := &Registry{types: map[string]Type{}}
	for _, t := range builtinTypes() {
		r.types[t.ID] = t
	}
	return r
}

// LoadVaultDir overlays type definitions from `.granit/types/*.json`
// inside vaultRoot. Each file's basename (without extension) must
// match the embedded `id` field — mismatches are skipped with a
// warning so a typo'd filename doesn't silently shadow a different
// type by coincidence.
//
// Vault-local types REPLACE built-ins of the same ID rather than
// merging — see builtin.go for the rationale (full override is the
// simpler mental model for users editing schemas by hand).
//
// Returns the count of vault-local types that were successfully
// loaded. Per-file errors are returned as a multi-error joined slice
// so the caller can surface the full list at once instead of
// stopping on the first malformed file.
func (r *Registry) LoadVaultDir(vaultRoot string) (loaded int, errs []error) {
	if vaultRoot == "" {
		return 0, nil
	}
	dir := filepath.Join(vaultRoot, ".granit", "types")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// No vault-local overrides — perfectly normal for a
			// fresh vault. Not an error.
			return 0, nil
		}
		return 0, []error{fmt.Errorf("read %s: %w", dir, err)}
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		var t Type
		if err := json.Unmarshal(data, &t); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		// Sanity check: filename should match ID. We forgive
		// case differences but flag obvious mismatches.
		expectedID := strings.TrimSuffix(e.Name(), ".json")
		if !strings.EqualFold(expectedID, t.ID) {
			errs = append(errs, fmt.Errorf("%s: filename %q does not match embedded id %q", e.Name(), expectedID, t.ID))
			continue
		}
		if err := t.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		r.types[t.ID] = t
		loaded++
	}
	return loaded, errs
}

// NewRegistryEmpty returns a Registry with no built-in types loaded.
// Used by tests that need to exercise the browser/index against a
// known-small set of types (so cursor arithmetic stays predictable
// across registry growth). Production callers should always use
// NewRegistry — built-ins are part of the user contract.
func NewRegistryEmpty() *Registry {
	return &Registry{types: map[string]Type{}}
}

// ByID returns the Type for the given id, or (zero, false) if no such
// type is registered. Callers should treat false the same as "this
// note has a frontmatter type that doesn't map to any known schema —
// fall through to displaying it as an untyped note".
func (r *Registry) ByID(id string) (Type, bool) {
	t, ok := r.types[id]
	return t, ok
}

// All returns every registered type sorted by ID for deterministic
// iteration. The Object Browser uses this order in its type list, so
// stable sort means the UI doesn't reshuffle on every refresh.
func (r *Registry) All() []Type {
	ids := make([]string, 0, len(r.types))
	for id := range r.types {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Type, len(ids))
	for i, id := range ids {
		out[i] = r.types[id]
	}
	return out
}

// Set installs or replaces a Type by ID. Used by tests and by future
// "add type" UI flows. Validates the type first; an invalid type is
// not installed and the error is returned.
func (r *Registry) Set(t Type) error {
	if err := t.Validate(); err != nil {
		return err
	}
	r.types[t.ID] = t
	return nil
}

// IDs returns just the registered type IDs sorted alphabetically.
// Convenience helper for UI code that needs to populate a picker
// without iterating the full Type values.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.types))
	for id := range r.types {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Len reports the number of registered types. Useful for "no types"
// rendering branches without forcing a slice allocation.
func (r *Registry) Len() int { return len(r.types) }

// SaveType writes a type as `.granit/types/<id>.json` inside vaultRoot,
// creating the directory if needed. Future "edit type in TUI" flows
// will round-trip through this. We pretty-print the JSON so the
// on-disk file is hand-editable.
//
// Used directly by tests and the (future) type-editor UI.
func SaveType(vaultRoot string, t Type) error {
	if err := t.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(vaultRoot, ".granit", "types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(dir, t.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

