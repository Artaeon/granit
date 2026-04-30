package objects

import (
	"encoding/json"
	"strings"
	"testing"
)

// PropertyKind.Validate accepts the documented constants AND the empty
// string (which downstream code treats as "default to text"); rejects
// anything else with a clear error.
func TestPropertyKind_Validate(t *testing.T) {
	for _, k := range []PropertyKind{
		KindText, KindNumber, KindDate, KindURL, KindTag, KindCheckbox,
		KindLink, KindSelect, "",
	} {
		if err := k.Validate(); err != nil {
			t.Errorf("kind %q: got error %v, want nil", k, err)
		}
	}
	if err := PropertyKind("nope").Validate(); err == nil {
		t.Error("unknown kind should error")
	}
}

// Property.Validate rejects empty names, unknown kinds, and select
// kinds with no Options. Each branch is the only thing that should
// fail given the input.
func TestProperty_Validate(t *testing.T) {
	cases := []struct {
		name string
		in   Property
		want string
	}{
		{"empty name", Property{Name: "  ", Kind: KindText}, "name is required"},
		{"unknown kind", Property{Name: "x", Kind: "weird"}, "unknown property kind"},
		{"select no opts", Property{Name: "x", Kind: KindSelect}, "kind=select requires"},
		{"valid text", Property{Name: "title", Kind: KindText}, ""},
		{"valid select", Property{Name: "status", Kind: KindSelect, Options: []string{"a"}}, ""},
		{"empty kind ok", Property{Name: "title"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate()
			if c.want == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
		})
	}
}

// DisplayLabel falls through Label → titlecased Name → empty.
// Underscores in Name become spaces because column headers read
// awkwardly with snake_case ("Last contact" beats "Last_contact").
func TestProperty_DisplayLabel(t *testing.T) {
	cases := []struct{ p Property; want string }{
		{Property{Name: ""}, ""},
		{Property{Name: "title"}, "Title"},
		{Property{Name: "last_contact"}, "Last contact"},
		{Property{Name: "phone", Label: "Phone number"}, "Phone number"},
	}
	for _, c := range cases {
		if got := c.p.DisplayLabel(); got != c.want {
			t.Errorf("p=%+v: got %q, want %q", c.p, got, c.want)
		}
	}
}

// Type.Validate catches missing ID/Name, propagates property errors,
// and detects duplicate property names within a type.
func TestType_Validate(t *testing.T) {
	cases := []struct {
		name string
		in   Type
		want string
	}{
		{"empty id", Type{Name: "X"}, "ID is required"},
		{"empty name", Type{ID: "x"}, "Name is required"},
		{"bad property", Type{ID: "x", Name: "X", Properties: []Property{
			{Name: "", Kind: KindText},
		}}, "name is required"},
		{"duplicate property", Type{ID: "x", Name: "X", Properties: []Property{
			{Name: "title"}, {Name: "title"},
		}}, "duplicate property"},
		{"valid", Type{ID: "x", Name: "X", Properties: []Property{
			{Name: "title", Kind: KindText},
			{Name: "year", Kind: KindNumber},
		}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate()
			if c.want == "" {
				if err != nil {
					t.Errorf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected error containing %q, got %v", c.want, err)
			}
		})
	}
}

// Type.PropertyByName returns nil for unknown names so callers can
// distinguish "type doesn't declare this property" (render raw value)
// from "type declares it but it's empty" (render the type-aware UI).
func TestType_PropertyByName(t *testing.T) {
	tt := Type{ID: "person", Name: "Person", Properties: []Property{
		{Name: "email", Kind: KindURL},
		{Name: "phone", Kind: KindText},
	}}
	if p := tt.PropertyByName("email"); p == nil || p.Kind != KindURL {
		t.Errorf("PropertyByName(email): got %+v", p)
	}
	if p := tt.PropertyByName("nope"); p != nil {
		t.Errorf("PropertyByName(nope): expected nil, got %+v", p)
	}
}

// JSON round-trip preserves every field including Property order.
// This is load-bearing — the disk format IS the JSON shape, so any
// field that doesn't survive Marshal+Unmarshal would silently break
// vault-local overrides.
func TestType_JSONRoundTrip(t *testing.T) {
	original := Type{
		ID: "book", Name: "Book", Description: "A book", Icon: "📚",
		Folder: "Books", FilenamePattern: "{title}",
		Properties: []Property{
			{Name: "title", Kind: KindText, Required: true},
			{Name: "author", Kind: KindText, Description: "Author full name"},
			{Name: "rating", Kind: KindNumber, Default: "0"},
			{Name: "status", Kind: KindSelect, Options: []string{"reading", "read", "abandoned"}},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var parsed Type
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	// Order-preserving: properties must come back in the same sequence.
	if len(parsed.Properties) != len(original.Properties) {
		t.Fatalf("property count mismatch: %d != %d", len(parsed.Properties), len(original.Properties))
	}
	for i := range parsed.Properties {
		if parsed.Properties[i].Name != original.Properties[i].Name {
			t.Errorf("property[%d] name: got %q, want %q",
				i, parsed.Properties[i].Name, original.Properties[i].Name)
		}
	}
	// Spot-check non-trivial fields.
	if parsed.Icon != "📚" {
		t.Errorf("icon: got %q", parsed.Icon)
	}
	statusProp := parsed.PropertyByName("status")
	if statusProp == nil || len(statusProp.Options) != 3 {
		t.Errorf("status property options not preserved: %+v", statusProp)
	}
}
