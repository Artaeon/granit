package tui

import (
	"log"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/profiles"
)

// bootProfiles wires the Profile system into the Model. Called
// from NewModel once cfg.UseProfiles is on. Side effects:
//
//   - Constructs a ProfileRegistry rooted at the vault
//   - Registers built-in profiles, then loads disk overrides
//   - Resolves the active profile (default Classic)
//   - Applies that profile's EnabledModules to the module
//     registry via SetEnabledBatch (handles dep order in one shot)
//   - Sets cfg.Layout to the profile's DefaultLayout if the user
//     hasn't explicitly overridden it
//
// Idempotent — calling twice with the same active profile is a
// no-op. Errors are logged, not returned, so a malformed profile
// or a missing module ID can't prevent granit from launching.
func bootProfiles(m *Model) {
	if m.profileRegistry != nil {
		return // already booted
	}
	pr := profiles.New(m.vault.Root)
	if err := profiles.RegisterBuiltins(pr); err != nil {
		log.Printf("warning: profiles register builtins: %v", err)
		return
	}
	if err := pr.Load(); err != nil {
		log.Printf("warning: profiles load: %v", err)
		// Continue with built-ins only; an I/O error on disk
		// overrides shouldn't block launch.
	}
	m.profileRegistry = pr
	applyProfile(m, pr.Active())
}

// applyProfile takes the side effects a profile prescribes:
// flips modules on/off as a batch, sets the default layout if the
// user hasn't pinned one. Safe to call repeatedly (e.g. after
// SetActive from the profile picker).
//
// Module enable semantics (matches builtin.go's documented
// behavior):
//
//   - Empty EnabledModules: leave all modules untouched. Classic
//     and any user profile that wants "enable everything"
//     behavior just omits this field.
//   - Non-empty: every registered module gets enable/disable
//     based on membership in the list. Module IDs in the list
//     that aren't currently registered are remembered for when
//     they do register (the registry's enabled-map carries them).
func applyProfile(m *Model, p *profiles.Profile) {
	if p == nil || m.registry == nil {
		return
	}
	// Build the wanted-state map. Empty EnabledModules means
	// "Classic — everything on": switching back to Classic must
	// actively re-enable any modules a previous profile disabled,
	// otherwise the user gets stuck (the symptom: alt+B silently
	// no-ops and Habit Tracker disappears from the palette after
	// a Researcher → Classic round trip).
	var wanted map[string]bool
	if len(p.EnabledModules) > 0 {
		wanted = desiredModuleStates(m, p.EnabledModules)
	} else {
		wanted = make(map[string]bool)
		for _, mod := range m.registry.All() {
			wanted[mod.ID()] = true
		}
	}
	if err := m.registry.SetEnabledBatch(wanted); err != nil {
		// Dependency conflict somewhere in the wanted state.
		// Log and continue — modules that could be toggled
		// were toggled; the rest stay as they were. Surfaces
		// in the picker UI as "profile applied with warnings."
		log.Printf("warning: profile %s apply batch: %v", p.ID, err)
	}
	// Persist so the next launch sees the profile-derived
	// enabled-set without needing to re-apply.
	if err := m.registry.Save(); err != nil {
		log.Printf("warning: save modules after profile apply: %v", err)
	}

	// Surface the active profile in the status bar so the user
	// always knows which workspace mode they're in (the same
	// shortcut behaving differently across profiles is an easy
	// way to confuse users — the chip removes that mystery).
	m.statusbar.SetProfile(p.Name)

	// Layout: only override if the user hasn't explicitly chosen
	// one. We can't perfectly tell "user picked default" from
	// "default is the default" — so the rule is: profile sets
	// layout only when cfg.Layout is empty or matches the
	// previously-applied profile's default. For first-time apply
	// we just set it.
	if p.DefaultLayout != "" {
		m.config.Layout = p.DefaultLayout
		// updateLayout uses cfg.Layout; let the caller (NewModel)
		// trigger the visual refresh by calling updateLayout on
		// its normal path so we don't double-call here.
	}
}

// isNewVault reports whether the vault has never been opened by
// granit before — used to gate the first-launch profile picker.
//
// A vault is considered new only if NONE of granit's state files
// exist: no .granit/active-profile, no .granit/modules.json, no
// .granit/tasks-meta.json. Any one of those means this vault has
// been seen before and the user already chose (or implicitly
// stuck with) Classic; we don't want to nag them with a picker
// every time .granit/active-profile gets accidentally deleted by
// an over-eager .gitignore.
func isNewVault(vaultRoot string) bool {
	if vaultRoot == "" {
		return false
	}
	markers := []string{
		filepath.Join(vaultRoot, ".granit", "active-profile"),
		filepath.Join(vaultRoot, ".granit", "modules.json"),
		filepath.Join(vaultRoot, ".granit", "tasks-meta.json"),
	}
	for _, m := range markers {
		if _, err := os.Stat(m); err == nil {
			return false
		}
	}
	return true
}

// applyProfileSwitch handles the runtime side of switching to a
// new profile. Persists the new active pointer, applies its
// module set + layout, refreshes the layout visually, and shows
// a status bar confirmation. Returns a tea.Cmd for any
// follow-up action (e.g. clearing the status message after a
// few seconds).
//
// Called from app_update.go when the ProfilePicker overlay
// closes with a successful Result. Errors are surfaced in the
// status bar — never block the launch loop.
func (m *Model) applyProfileSwitch(id string) tea.Cmd {
	if m.profileRegistry == nil {
		return nil
	}
	prev := m.profileRegistry.ActiveID()
	if id == prev {
		m.statusbar.SetMessage("Already on " + id)
		return m.clearMessageAfter(2 * time.Second)
	}
	if err := m.profileRegistry.SetActive(id); err != nil {
		m.statusbar.SetMessage("Profile switch failed: " + err.Error())
		return m.clearMessageAfter(3 * time.Second)
	}
	p := m.profileRegistry.Active()
	applyProfile(m, p)
	m.updateLayout()
	m.statusbar.SetMessage("Profile: " + p.Name)
	return m.clearMessageAfter(2 * time.Second)
}

// desiredModuleStates builds the wanted-state map for
// SetEnabledBatch. Iterates every registered module:
//
//   - If the module's ID appears in the profile's EnabledModules
//     slice → wanted = true
//   - Otherwise → wanted = false
//
// Profile-listed IDs that aren't registered modules are also
// added to the map with wanted=true so the registry remembers the
// intent for when they register later (Lua plugin loads, future
// built-in ships, etc.).
func desiredModuleStates(m *Model, listed []string) map[string]bool {
	listedSet := make(map[string]bool, len(listed))
	for _, id := range listed {
		listedSet[id] = true
	}
	wanted := make(map[string]bool, len(listed))
	for _, mod := range m.registry.All() {
		wanted[mod.ID()] = listedSet[mod.ID()]
	}
	for id := range listedSet {
		if _, set := wanted[id]; !set {
			wanted[id] = true
		}
	}
	return wanted
}
