package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Preset is a serialisable agent definition that bundles persona,
// tool selection, and write-access opt-in. Built-in presets are
// declared in code (defaultPresets); vault-local overrides live at
// `<vault>/.granit/agents/<id>.json` and replace built-ins by ID.
//
// The JSON shape is hand-editable on purpose — users should be able
// to write a custom agent without touching Go. Field names match
// what a user would expect from reading docs/AGENTS.md.
type Preset struct {
	// ID is the stable handle used for vault-local override
	// filenames and command-palette routing. Lower-snake case.
	ID string `json:"id"`

	// Name is the human-friendly label shown in the runner's
	// preset picker.
	Name string `json:"name"`

	// Description is the one-line summary under the name.
	Description string `json:"description"`

	// SystemPrompt is the persistent persona block prepended to
	// every iteration. Empty falls through to the built-in
	// generic helper preamble.
	SystemPrompt string `json:"systemPrompt"`

	// Tools is the explicit allow-list of tool names this preset
	// can use. Empty means "every read tool" — the safe default.
	// Listing tools also drives what the LLM sees in the system
	// prompt, so a preset that only needs search_vault doesn't
	// distract the model with a 9-tool catalog.
	Tools []string `json:"tools,omitempty"`

	// IncludeWrite, when true, registers the package's three
	// write tools (write_note, create_task, create_object)
	// alongside the Tools allow-list. Only takes effect when the
	// runtime caller has supplied an Approve callback — without
	// that, agent construction fails the safety gate.
	IncludeWrite bool `json:"includeWrite,omitempty"`

	// MaxSteps overrides the runtime's default step budget for
	// this preset. Useful when an agent's expected work is large
	// enough that the default 8-step cap is too tight (research
	// synthesis across 20 notes), or small enough that 8 is
	// wasteful (single-shot triage). Zero falls through to the
	// runtime default.
	MaxSteps int `json:"maxSteps,omitempty"`

	// Model overrides the AI model used for THIS preset's runs,
	// independent of the user's global Settings choice. Pattern:
	// fast cheap models for simple multi-step routing (Inbox
	// Triager picks a tag — qwen2.5:0.5b is fine), bigger smarter
	// models for synthesis (Research Synthesizer benefits from
	// llama3.1:8b or gpt-4o-mini). Empty falls through to the
	// global model. Provider is NEVER overridden — the preset
	// rides the user's configured provider, just with a
	// different model name on it.
	Model string `json:"model,omitempty"`
}

// Validate reports a clear error when a Preset is missing fields
// that the runtime requires. Empty Tools is valid (means "all read
// tools"); empty SystemPrompt is valid (means "use generic
// preamble"). The hard requirements are an ID + Name + Description
// — those are user-facing labels that have no sensible fallback.
func (p Preset) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("preset ID is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("preset %q: Name is required", p.ID)
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("preset %q: Description is required", p.ID)
	}
	for _, name := range p.Tools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("preset %q: empty tool name in Tools list", p.ID)
		}
	}
	return nil
}

// PresetCatalog is the merged view of built-in + vault-local
// presets. Built-ins ship in code (callers register them at
// startup); vault-local overrides at `.granit/agents/<id>.json`
// REPLACE the built-in with the same ID rather than merging.
//
// Same rationale as the Type registry's full-override semantics:
// merge semantics on user-edited JSON make for surprising
// behaviour ("which fields win?"), full override is the simpler
// mental model.
type PresetCatalog struct {
	presets map[string]Preset
}

// NewPresetCatalog returns a catalog seeded with the given
// built-in presets. The TUI passes its hardcoded list at
// startup; tests can pass an empty list to exercise edge cases.
func NewPresetCatalog(builtins []Preset) *PresetCatalog {
	c := &PresetCatalog{presets: map[string]Preset{}}
	for _, p := range builtins {
		// Built-in misconfiguration is a programmer error, not a
		// runtime fault — skip silently rather than panic so a
		// bad built-in doesn't take down the whole runner.
		if err := p.Validate(); err == nil {
			c.presets[p.ID] = p
		}
	}
	return c
}

// LoadVaultDir scans `<vaultRoot>/.granit/agents/` for `*.json`
// files and overlays them onto the catalog. Same rules as the
// type registry: filename basename must match the embedded ID
// (case-insensitive), each file is validated independently,
// per-file errors are returned together so the caller can render
// them all at once.
//
// Returns (loadedCount, errors) — errors is nil-slice when no
// problems occurred. The catalog mutates in place.
func (c *PresetCatalog) LoadVaultDir(vaultRoot string) (int, []error) {
	if vaultRoot == "" {
		return 0, nil
	}
	dir := filepath.Join(vaultRoot, ".granit", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, []error{fmt.Errorf("read %s: %w", dir, err)}
	}
	loaded := 0
	var errs []error
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
		var p Preset
		if err := json.Unmarshal(data, &p); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		expectedID := strings.TrimSuffix(e.Name(), ".json")
		if !strings.EqualFold(expectedID, p.ID) {
			errs = append(errs, fmt.Errorf("%s: filename %q does not match embedded id %q", e.Name(), expectedID, p.ID))
			continue
		}
		if err := p.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		c.presets[p.ID] = p
		loaded++
	}
	return loaded, errs
}

// ByID returns the preset with the given ID, or (zero, false) when
// none exists. Used by the runner to look up the preset the user
// picked from the list.
func (c *PresetCatalog) ByID(id string) (Preset, bool) {
	p, ok := c.presets[id]
	return p, ok
}

// All returns every preset in stable ID order so the picker
// renders deterministically across vault rebuilds.
func (c *PresetCatalog) All() []Preset {
	ids := make([]string, 0, len(c.presets))
	for id := range c.presets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Preset, len(ids))
	for i, id := range ids {
		out[i] = c.presets[id]
	}
	return out
}

// Len reports the catalog size.
func (c *PresetCatalog) Len() int { return len(c.presets) }

// SavePreset writes a preset to `<vaultRoot>/.granit/agents/<id>.json`,
// creating the directory if needed. Validates first; an invalid
// preset is not written. Used by future "save as preset" UI flows.
func SavePreset(vaultRoot string, p Preset) error {
	if err := p.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(vaultRoot, ".granit", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(dir, p.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// BuildRegistryForPreset constructs a Registry containing the tools
// the preset opts into. allReadTools and allWriteTools are the
// caller-supplied factories — the agents package doesn't know about
// VaultReader/VaultWriter directly, so the TUI hands in pre-built
// tools and we filter by the preset's Tools allow-list.
//
// When preset.Tools is empty, ALL provided readTools are registered
// (the "no allow-list = all reads" convention). preset.IncludeWrite
// adds writeTools regardless of the Tools allow-list.
func BuildRegistryForPreset(preset Preset, readTools, writeTools []Tool) (*Registry, error) {
	r := NewRegistry()
	wantedRead := map[string]bool{}
	if len(preset.Tools) == 0 {
		// No allow-list: all read tools.
		for _, t := range readTools {
			wantedRead[t.Name()] = true
		}
	} else {
		for _, name := range preset.Tools {
			wantedRead[strings.TrimSpace(name)] = true
		}
	}
	for _, t := range readTools {
		if t.Kind() != KindRead {
			continue // safety: a "read" factory that returns a write tool stays out
		}
		if wantedRead[t.Name()] {
			if err := r.Register(t); err != nil {
				return nil, err
			}
		}
	}
	if preset.IncludeWrite {
		for _, t := range writeTools {
			if t.Kind() != KindWrite {
				continue
			}
			if err := r.Register(t); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}
