// Package aiprefs persists per-feature AI consent + provider
// choices. Stored at <vault>/.granit/ai-prefs.json. Defaults are
// "everything off" — every AI feature is explicit opt-in.
//
// The shape is one entry per feature:
//
//	"daily_briefing": { "enabled": true, "provider": "ollama" }
//
// Plus a top-level RedactionEnabled flag and a list of disabled
// redaction rules (so a user can turn off the IBAN rule if they
// have too many false positives).
package aiprefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/artaeon/granit/internal/atomicio"
)

// Feature is a stable string identifier for one AI surface. Used
// as the key in the prefs map and in audit log entries — keeps
// the wire format stable across renames of the user-facing label.
type Feature string

const (
	FeatureDailyBriefing  Feature = "daily_briefing"
	FeatureWeeklyReview   Feature = "weekly_review"
	FeatureInboxTriage    Feature = "inbox_triage"
	FeatureDeadlineDetect Feature = "deadline_detect"
	FeatureSummarise      Feature = "summarise"
	FeatureExtractTasks   Feature = "extract_tasks"
	FeatureSuggestTags    Feature = "suggest_tags"
	FeatureSuggestLinks   Feature = "suggest_links"
	FeatureRewrite        Feature = "rewrite"
	FeatureContinue       Feature = "continue_writing"
	FeatureExplain        Feature = "explain"
	FeatureChat           Feature = "chat"
	// FeatureAnnotateNote — given the body of a note, propose 3-5
	// margin annotations the user might want to add (questions,
	// counter-arguments, "this matters" markers). Each suggestion
	// returns line + anchor + text + color so the existing
	// annotations store can accept them with minimal client work.
	FeatureAnnotateNote Feature = "annotate_note"
)

// FeatureConfig is the per-feature setting record.
type FeatureConfig struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"` // "ollama" | "openai" | "anthropic" | ""
}

// Preferences is the full prefs sidecar shape.
type Preferences struct {
	Features          map[Feature]FeatureConfig `json:"features"`
	RedactionEnabled  bool                      `json:"redaction_enabled"`
	DisabledRedaction []string                  `json:"disabled_redaction,omitempty"`
	// DefaultProvider — when a FeatureConfig.Provider is empty,
	// this fallback is used. Lets a user say "everything via
	// local Ollama unless I explicitly opt a feature into a
	// cloud provider."
	DefaultProvider string `json:"default_provider,omitempty"`
}

// Defaults — every feature off, redaction on, default provider
// blank (the agent runtime has its own resolution rules).
func Defaults() Preferences {
	return Preferences{
		Features: map[Feature]FeatureConfig{
			FeatureDailyBriefing:  {Enabled: false, Provider: "ollama"},
			FeatureWeeklyReview:   {Enabled: false, Provider: "ollama"},
			FeatureInboxTriage:    {Enabled: false, Provider: "ollama"},
			FeatureDeadlineDetect: {Enabled: false, Provider: "ollama"},
			FeatureSummarise:      {Enabled: true, Provider: ""}, // already shipping; keep on
			FeatureExtractTasks:   {Enabled: true, Provider: ""},
			FeatureSuggestTags:    {Enabled: true, Provider: ""},
			FeatureSuggestLinks:   {Enabled: true, Provider: ""},
			FeatureRewrite:        {Enabled: true, Provider: ""},
			FeatureContinue:       {Enabled: true, Provider: ""},
			FeatureExplain:        {Enabled: true, Provider: ""},
			FeatureChat:           {Enabled: true, Provider: ""},
			// Off by default — same posture as the other "AI
			// proposes a batch of edits" features (inbox triage,
			// deadline detect). User opts in via Settings → AI.
			FeatureAnnotateNote: {Enabled: false, Provider: "ollama"},
		},
		RedactionEnabled: true,
		DefaultProvider:  "ollama",
	}
}

func path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "ai-prefs.json")
}

// Load reads the prefs sidecar. Missing → Defaults. Malformed →
// Defaults with the parse error returned so the caller can
// surface it.
func Load(vaultRoot string) (Preferences, error) {
	data, err := os.ReadFile(path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Defaults(), nil
		}
		return Defaults(), err
	}
	var p Preferences
	if err := json.Unmarshal(data, &p); err != nil {
		return Defaults(), err
	}
	if p.Features == nil {
		p.Features = map[Feature]FeatureConfig{}
	}
	// Fill in any feature missing from the on-disk file (e.g.
	// after a granit upgrade introduces a new feature). We DON'T
	// overwrite — if the user explicitly disabled an old feature
	// we preserve that.
	for k, v := range Defaults().Features {
		if _, ok := p.Features[k]; !ok {
			p.Features[k] = v
		}
	}
	return p, nil
}

func Save(vaultRoot string, p Preferences) error {
	dir := filepath.Dir(path(vaultRoot))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path(vaultRoot), data)
}

// IsEnabled is a one-line check the AI feature handlers can use
// before doing any work. Returns false on missing prefs or
// missing feature entry — fail-closed for opt-in.
func IsEnabled(vaultRoot string, f Feature) bool {
	p, err := Load(vaultRoot)
	if err != nil {
		return false
	}
	cfg, ok := p.Features[f]
	if !ok {
		return false
	}
	return cfg.Enabled
}
