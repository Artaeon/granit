package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/artaeon/granit/internal/atomicio"
)

// DefaultProfileID is the fallback profile used when the active
// pointer file is missing or names a profile the registry doesn't
// know. Built-in profiles always include "classic" with this ID.
const DefaultProfileID = "classic"

// ErrUnknownProfile is returned by SetActive when the requested
// ID isn't registered. Callers can errors.Is(err, ErrUnknownProfile).
var ErrUnknownProfile = errors.New("profiles: unknown profile ID")

// ProfileRegistry is the single source of truth for "what
// profiles exist" and "which one is active right now." Lifecycle:
//
//  1. Construct via New(vaultRoot)
//  2. RegisterBuiltins (or Register one-by-one) for compiled-in
//     profiles
//  3. Load() — walks ~/.config/granit/profiles/ and
//     <vault>/.granit/profiles/ for user-authored overrides, then
//     reads .granit/active-profile to set the active pointer
//  4. Active() / SetActive() / All() — query/mutate at runtime
//
// Goroutine-safe via RWMutex, same shape as modules.Registry.
type ProfileRegistry struct {
	mu        sync.RWMutex
	profiles  map[string]*Profile
	order     []string // registration order; built-ins first, then disk
	active    string
	vaultRoot string
}

// New creates a registry rooted at vaultRoot (used for the
// .granit/profiles/ scan and the .granit/active-profile pointer
// file). Pass "" if there's no active vault yet (e.g. in tests
// that only exercise the in-memory registration).
func New(vaultRoot string) *ProfileRegistry {
	return &ProfileRegistry{
		profiles:  make(map[string]*Profile),
		vaultRoot: vaultRoot,
	}
}

// Register adds a profile. Same ID re-registered overwrites the
// previous entry (so disk-loaded profiles can override built-ins
// of the same ID). Order is stable: first-registered IDs stay at
// the front of All().
func (r *ProfileRegistry) Register(p *Profile) error {
	if p == nil || p.ID == "" {
		return errors.New("profiles: cannot register nil or empty-ID profile")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, existed := r.profiles[p.ID]; !existed {
		r.order = append(r.order, p.ID)
	}
	// Defensive copy — caller can mutate their pointer afterward
	// without affecting the registered profile.
	pCopy := *p
	r.profiles[p.ID] = &pCopy
	return nil
}

// Get returns the profile with the given ID, if registered.
func (r *ProfileRegistry) Get(id string) (*Profile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

// All returns every registered profile in registration order.
func (r *ProfileRegistry) All() []*Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Profile, 0, len(r.order))
	for _, id := range r.order {
		cp := *r.profiles[id]
		out = append(out, &cp)
	}
	return out
}

// Active returns the active profile. Always non-nil after Load —
// the registry guarantees a fallback to the default profile if the
// pointer file is missing or names an unknown ID.
func (r *ProfileRegistry) Active() *Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := r.active
	if id == "" {
		id = DefaultProfileID
	}
	if p, ok := r.profiles[id]; ok {
		cp := *p
		return &cp
	}
	if def, ok := r.profiles[DefaultProfileID]; ok {
		cp := *def
		return &cp
	}
	// Pathological: nothing registered. Return an empty
	// placeholder so callers can dereference safely.
	return &Profile{ID: DefaultProfileID, Name: "Default"}
}

// ActiveID returns just the active profile's ID — cheaper than
// Active when the caller only needs to check identity.
func (r *ProfileRegistry) ActiveID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active == "" {
		return DefaultProfileID
	}
	return r.active
}

// SetActive selects the profile to apply. Persists the choice to
// <vault>/.granit/active-profile so the next launch picks up the
// same profile. Returns ErrUnknownProfile if id isn't registered.
//
// Caller is responsible for actually applying the profile (toggle
// modules, set layout, refresh dashboard) — the registry is just
// the persisted pointer; the side effects belong to the TUI.
func (r *ProfileRegistry) SetActive(id string) error {
	r.mu.Lock()
	if _, ok := r.profiles[id]; !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownProfile, id)
	}
	r.active = id
	path := r.activePathLocked()
	r.mu.Unlock()

	if path == "" {
		return nil // no vault yet; in-memory only
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("profiles: mkdir for active pointer: %w", err)
	}
	return atomicio.WriteState(path, []byte(id+"\n"))
}

// activePathLocked returns the path to the active-profile pointer.
// Caller holds r.mu.
func (r *ProfileRegistry) activePathLocked() string {
	if r.vaultRoot == "" {
		return ""
	}
	return filepath.Join(r.vaultRoot, ".granit", "active-profile")
}

// ActivePath exposes the on-disk pointer location for diagnostic
// UIs. Empty when the registry has no vault attached.
func (r *ProfileRegistry) ActivePath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activePathLocked()
}

// Load walks the disk layers and resolves the active pointer.
// Built-in profiles must already be Registered before calling
// Load. Disk-loaded profiles override built-ins of the same ID.
//
// Layered scan order (later wins on ID collision):
//
//  1. ~/.config/granit/profiles/*.json  (user-global)
//  2. <vault>/.granit/profiles/*.json   (vault-local)
//
// Then read <vault>/.granit/active-profile to set the active
// pointer. Falls back to DefaultProfileID if the pointer is
// missing or names an unknown ID.
func (r *ProfileRegistry) Load() error {
	if homeDir, err := os.UserHomeDir(); err == nil {
		if perr := r.loadFromDir(filepath.Join(homeDir, ".config", "granit", "profiles")); perr != nil {
			return perr
		}
	}
	if r.vaultRoot != "" {
		if err := r.loadFromDir(filepath.Join(r.vaultRoot, ".granit", "profiles")); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = DefaultProfileID

	if r.vaultRoot == "" {
		return nil
	}
	pointerPath := r.activePathLocked()
	data, err := os.ReadFile(pointerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // pointer missing → keep default
		}
		// Real I/O error — surface it. Caller decides whether to
		// proceed with the default or refuse to launch.
		return fmt.Errorf("profiles: read active pointer: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return nil
	}
	if _, ok := r.profiles[id]; ok {
		r.active = id
	}
	// Unknown ID: silently keep the default. The previously-active
	// profile probably came from a Lua plugin that hasn't loaded
	// yet, or a vault-local profile that was deleted. Logging
	// belongs to the caller (TUI status bar) — the registry stays
	// quiet so it can be used in scripts.
	return nil
}

// loadFromDir reads every *.json under dir and registers each as
// a profile. Files with invalid JSON or missing required fields
// (ID) are skipped silently — a malformed user-authored file
// shouldn't crash the launch. Same-ID later wins (so vault-local
// can override user-global within this single call's order).
//
// Missing dir is not an error — most installs won't have any
// user-authored profiles.
func (r *ProfileRegistry) loadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("profiles: read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			continue // skip unreadable file
		}
		var p Profile
		if jerr := json.Unmarshal(data, &p); jerr != nil {
			continue // skip malformed JSON
		}
		if p.ID == "" {
			continue // skip incomplete
		}
		// Disk profiles are never BuiltIn even if their ID matches
		// a built-in; the override semantically replaces it.
		p.BuiltIn = false
		_ = r.Register(&p)
	}
	return nil
}

// MarkBuiltIn flips the BuiltIn flag on a profile by ID — used by
// RegisterBuiltins to tag the bundled profiles after registration.
// Has no effect for unknown IDs.
func (r *ProfileRegistry) MarkBuiltIn(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.profiles[id]; ok {
		p.BuiltIn = true
	}
}
