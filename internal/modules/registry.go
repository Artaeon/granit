package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// stateVersion is bumped only when the on-disk format changes in a
// non-additive way.
const stateVersion = 1

// stateFile is the persisted enabled-set, written under
// .granit/modules.json. The Profile system (later phase) will own
// this file; for now the registry writes it directly.
type stateFile struct {
	Version int             `json:"version"`
	Enabled map[string]bool `json:"enabled"`
}

// Registry holds the set of registered modules and the current
// enabled-state. The zero value is not usable — call New.
type Registry struct {
	mu      sync.RWMutex
	mods    map[string]Module
	order   []string // registration order, for stable iteration
	enabled map[string]bool
	path    string // .granit/modules.json
}

// New creates a Registry that persists state to
// <vaultRoot>/.granit/modules.json. The state file is loaded lazily
// via Load().
func New(vaultRoot string) *Registry {
	return &Registry{
		mods:    make(map[string]Module),
		enabled: make(map[string]bool),
		path:    filepath.Join(vaultRoot, ".granit", "modules.json"),
	}
}

// Register adds a module to the registry. Returns an error on
// duplicate ID. Registration order is preserved across All() calls.
func (r *Registry) Register(m Module) error {
	id := m.ID()
	if id == "" {
		return errors.New("modules: module has empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.mods[id]; exists {
		return fmt.Errorf("modules: duplicate module ID %q", id)
	}
	r.mods[id] = m
	r.order = append(r.order, id)
	return nil
}

// Get returns the module with the given ID, if registered.
func (r *Registry) Get(id string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.mods[id]
	return m, ok
}

// All returns every registered module in registration order.
func (r *Registry) All() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Module, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.mods[id])
	}
	return out
}

// Enabled reports whether the module with the given ID is currently
// enabled.
//
// An explicit entry in the enabled-set wins regardless of whether the
// module is registered — this is what lets a value mirrored from
// legacy config.CorePlugins (or persisted in modules.json by an
// earlier session) gate a feature whose Module declaration ships in a
// later commit. With no explicit entry, returns true: that's the
// migration-safety fallback that keeps unmigrated features visible
// during rollout.
func (r *Registry) Enabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isEnabledLocked(id)
}

// SetEnabled toggles a module on or off. Enabling fails if any
// declared dependency is not enabled. Disabling fails if any other
// enabled module depends on this one. Caller is expected to Save()
// after a successful toggle.
func (r *Registry) SetEnabled(id string, on bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	mod, ok := r.mods[id]
	if !ok {
		return fmt.Errorf("modules: unknown module %q", id)
	}
	if on {
		for _, dep := range mod.DependsOn() {
			if !r.isEnabledLocked(dep) {
				return fmt.Errorf("modules: cannot enable %q — dependency %q is disabled", id, dep)
			}
		}
	} else {
		for _, otherID := range r.order {
			if otherID == id {
				continue
			}
			if !r.isEnabledLocked(otherID) {
				continue
			}
			for _, dep := range r.mods[otherID].DependsOn() {
				if dep == id {
					return fmt.Errorf("modules: cannot disable %q — module %q depends on it", id, otherID)
				}
			}
		}
	}
	r.enabled[id] = on
	return nil
}

// isEnabledLocked is the read-side of Enabled assuming the caller
// already holds the lock. See Enabled for the resolution rules.
func (r *Registry) isEnabledLocked(id string) bool {
	if v, set := r.enabled[id]; set {
		return v
	}
	return true
}

// EnabledModules returns only the enabled modules, in registration
// order.
func (r *Registry) EnabledModules() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Module, 0, len(r.order))
	for _, id := range r.order {
		if r.isEnabledLocked(id) {
			out = append(out, r.mods[id])
		}
	}
	return out
}

// EnabledCommands returns the flat list of commands contributed by
// enabled modules, suitable for the command palette.
func (r *Registry) EnabledCommands() []CommandRef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []CommandRef
	for _, id := range r.order {
		if !r.isEnabledLocked(id) {
			continue
		}
		out = append(out, r.mods[id].Commands()...)
	}
	return out
}

// EnabledKeybinds returns key→commandID for every keybind contributed
// by enabled modules. Duplicate keys are reported via the second
// return value as a list of conflict keys; on conflict, the
// first-registered binding wins.
func (r *Registry) EnabledKeybinds() (map[string]string, []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string)
	var conflicts []string
	for _, id := range r.order {
		if !r.isEnabledLocked(id) {
			continue
		}
		for _, kb := range r.mods[id].Keybinds() {
			if _, taken := out[kb.Key]; taken {
				conflicts = append(conflicts, kb.Key)
				continue
			}
			out[kb.Key] = kb.CommandID
		}
	}
	return out, conflicts
}

// Dependents returns the IDs of enabled modules that declare a
// dependency on the given module. Useful for the settings UI to warn
// before a disable.
func (r *Registry) Dependents(id string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for _, otherID := range r.order {
		if otherID == id {
			continue
		}
		for _, dep := range r.mods[otherID].DependsOn() {
			if dep == id {
				out = append(out, otherID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// Load reads the persisted enabled-set from disk. Missing file is not
// an error — it means "all defaults" (every known module enabled).
// Unknown IDs in the file are kept in the enabled map so a temporary
// disable of a Lua module survives a granit restart that loaded
// before the Lua plugin re-registered.
func (r *Registry) Load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var s stateFile
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("modules: parse %s: %w", r.path, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Enabled != nil {
		r.enabled = s.Enabled
	}
	return nil
}

// SetEnabledBatch atomically applies a desired enable-state to
// every registered module in one call. Used by Phase 3's
// profile-apply step where a single profile switch may flip a
// dozen modules at once and naive per-call SetEnabled would fail
// when it tries to disable a base before its dependent.
//
// The wanted map keys are module IDs; values are the desired
// enabled state. Modules NOT in the map are left at their current
// state — passing an empty map is a no-op.
//
// Algorithm: iterate up to len(registered) times, each pass
// trying to apply changes that don't violate dep constraints.
// Settles in O(D) passes where D is the longest dep chain;
// bounded above by the module count so it always terminates.
func (r *Registry) SetEnabledBatch(wanted map[string]bool) error {
	if len(wanted) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	maxPasses := len(r.order) + 1
	for pass := 0; pass < maxPasses; pass++ {
		progressed := false
		stillBlocked := false
		for id, want := range wanted {
			cur := r.isEnabledLocked(id)
			if cur == want {
				continue
			}
			// Check whether this change would currently violate the
			// dep graph. Disable: refuse if any enabled module
			// depends on this one (and that dependent isn't itself
			// being disabled this batch). Enable: refuse if any dep
			// is disabled (and isn't being enabled this batch).
			if want {
				ok := true
				if mod, exists := r.mods[id]; exists {
					for _, dep := range mod.DependsOn() {
						if !r.isEnabledLocked(dep) {
							if depWant, planned := wanted[dep]; !planned || !depWant {
								ok = false
								break
							}
						}
					}
				}
				if !ok {
					stillBlocked = true
					continue
				}
			} else {
				ok := true
				for _, otherID := range r.order {
					if otherID == id {
						continue
					}
					if !r.isEnabledLocked(otherID) {
						continue
					}
					for _, dep := range r.mods[otherID].DependsOn() {
						if dep == id {
							if otherWant, planned := wanted[otherID]; !planned || otherWant {
								ok = false
							}
						}
					}
					if !ok {
						break
					}
				}
				if !ok {
					stillBlocked = true
					continue
				}
			}
			r.enabled[id] = want
			progressed = true
		}
		if !stillBlocked {
			return nil
		}
		if !progressed {
			// No move possible this pass and still blocked — the
			// wanted state is internally inconsistent (e.g. enable
			// X without enabling its disabled dep, or disable X
			// while a dependent is being kept enabled).
			return fmt.Errorf("modules: SetEnabledBatch cannot satisfy wanted state — dependency conflict among %v", unfinishedKeys(wanted, r))
		}
	}
	return nil
}

func unfinishedKeys(wanted map[string]bool, r *Registry) []string {
	var out []string
	for id, want := range wanted {
		if r.isEnabledLocked(id) != want {
			out = append(out, id)
		}
	}
	return out
}

// MirrorLegacy seeds the enabled-map from a legacy enabled-flag
// source (e.g. config.CorePlugins) for IDs that don't already have
// an explicit setting. Existing entries in the registry win — this is
// only a one-way migration helper. Caller must hold no locks.
func (r *Registry) MirrorLegacy(legacy map[string]bool) {
	if len(legacy) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, on := range legacy {
		if _, set := r.enabled[id]; set {
			continue
		}
		r.enabled[id] = on
	}
}

// Save persists the current enabled-set to disk. Uses a temp+rename
// to avoid leaving a half-written file on crash.
func (r *Registry) Save() error {
	r.mu.RLock()
	enabledCopy := make(map[string]bool, len(r.enabled))
	for k, v := range r.enabled {
		enabledCopy[k] = v
	}
	r.mu.RUnlock()
	s := stateFile{Version: stateVersion, Enabled: enabledCopy}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Path returns the persistence path — exposed so callers (e.g. the
// settings UI) can show users where state lives.
func (r *Registry) Path() string {
	return r.path
}
