package objects

import "testing"

// Every built-in type must validate. Catches regressions where someone
// adds a new built-in with a typo'd kind or duplicate property name —
// shipping that would silently break the Object Browser for everyone
// using the default set.
func TestBuiltinTypes_AllValidate(t *testing.T) {
	for _, tt := range builtinTypes() {
		if err := tt.Validate(); err != nil {
			t.Errorf("built-in type %q invalid: %v", tt.ID, err)
		}
	}
}

// Every built-in must have a unique ID — duplicate IDs collide in the
// registry and only the last-loaded one would survive.
func TestBuiltinTypes_UniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, tt := range builtinTypes() {
		if seen[tt.ID] {
			t.Errorf("duplicate built-in type ID %q", tt.ID)
		}
		seen[tt.ID] = true
	}
}

// Every built-in must declare an Icon. The Object Browser column
// layout assumes a one-character icon prefix; missing icons would
// leave dead space in the type list.
func TestBuiltinTypes_AllHaveIcon(t *testing.T) {
	for _, tt := range builtinTypes() {
		if tt.Icon == "" {
			t.Errorf("built-in type %q missing icon", tt.ID)
		}
	}
}

// Every built-in must have at least one Required property. A type
// without ANY required field is hard to use in the gallery (no
// natural anchor column) and signals an under-specified schema.
func TestBuiltinTypes_AtLeastOneRequiredField(t *testing.T) {
	for _, tt := range builtinTypes() {
		hasRequired := false
		for _, p := range tt.Properties {
			if p.Required {
				hasRequired = true
				break
			}
		}
		if !hasRequired {
			t.Errorf("built-in type %q has no required property — every type should anchor on one mandatory field", tt.ID)
		}
	}
}

// Sanity: the starter set should cover the canonical PKM object kinds
// (person, book, project, meeting, idea, plus Capacities-parity types
// shipped in 5.1). If someone refactors the list and accidentally
// drops one, this test flags it.
func TestBuiltinTypes_CoversStarterSet(t *testing.T) {
	want := []string{
		"person", "book", "project", "goal", "meeting", "idea",
		"article", "podcast", "video", "quote", "place", "recipe", "highlight",
		"agent_run",
	}
	got := map[string]bool{}
	for _, tt := range builtinTypes() {
		got[tt.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("starter type %q missing from built-ins", id)
		}
	}
}
