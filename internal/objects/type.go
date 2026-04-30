// Package objects implements granit's typed-object system: a thin
// schema layer over markdown notes that lets the editor treat certain
// notes as instances of a Type (Person, Book, Project, Meeting, Idea,
// or anything the user defines).
//
// Storage stays plain markdown — the type association is carried in
// frontmatter (`type: person`) and the schema lives in
// `.granit/types/<id>.json` per vault, with built-in defaults shipped
// in code. No proprietary database, nothing to migrate, fully
// compatible with existing markdown editors.
//
// Public surface:
//
//   - Type / Property / PropertyKind — the schema.
//   - Registry — loads built-ins + per-vault overrides into one view.
//   - Index — keeps a live in-memory map of which notes are which type.
//
// Design constraints intentionally chosen:
//
//   - A note without a `type:` is fine; it just doesn't appear in
//     typed gallery views. Existing notes are not impacted.
//   - Property kinds are deliberately simple: text, number, date, url,
//     tag, checkbox, link, select. That covers ~95% of PKM use cases
//     without requiring users to write a JSON Schema.
//   - Validation is permissive by default — typos in frontmatter
//     produce warnings, never errors that break a build. The schema
//     is a hint to the UI, not a wall.
package objects

import (
	"errors"
	"fmt"
	"strings"
)

// PropertyKind enumerates the shapes a Property can take. New kinds
// should be added sparingly — each one needs a parser, a validator, an
// editor-input widget, and a renderer. The set below was chosen to
// cover the common-case PKM properties without spawning a long tail.
type PropertyKind string

const (
	// KindText is freeform single-line or multi-line text. Default
	// when no kind is specified.
	KindText PropertyKind = "text"
	// KindNumber is a numeric value. Stored as a float64.
	KindNumber PropertyKind = "number"
	// KindDate is YYYY-MM-DD. Stored as a string in that form so
	// it round-trips through frontmatter without timezone games.
	KindDate PropertyKind = "date"
	// KindURL is a URL. Validated as best-effort http/https scheme.
	KindURL PropertyKind = "url"
	// KindTag is a single tag (without leading #). Multiple tags
	// should use kind=text with a comma-separated value, or are
	// already handled by the global #tag system.
	KindTag PropertyKind = "tag"
	// KindCheckbox is true/false. Frontmatter accepts both bool and
	// the strings "true"/"false"/"yes"/"no".
	KindCheckbox PropertyKind = "checkbox"
	// KindLink references another note (by relative path or
	// wikilink target). The Object Browser renders these as
	// clickable rows.
	KindLink PropertyKind = "link"
	// KindSelect restricts values to a fixed set declared in
	// Property.Options. Useful for status fields (e.g. reading /
	// read / abandoned).
	KindSelect PropertyKind = "select"
)

// Validate returns nil if the kind is one of the recognised constants,
// or an error otherwise. Used by Type.Validate.
func (k PropertyKind) Validate() error {
	switch k {
	case KindText, KindNumber, KindDate, KindURL, KindTag,
		KindCheckbox, KindLink, KindSelect:
		return nil
	case "":
		// Empty is allowed at this layer — the caller defaults to
		// KindText. Returning nil here means the empty-string
		// migration path stays cheap.
		return nil
	}
	return fmt.Errorf("unknown property kind %q", string(k))
}

// Property describes a single named field on a Type. The slice of
// Property values on Type is ORDERED; that order drives the column
// order in the Object Browser and the field order in any
// auto-generated note template.
type Property struct {
	// Name is the frontmatter key. Required. Should be lower_snake
	// for Markdown frontmatter compatibility (most YAML readers
	// preserve the literal key, but lowercase is conventional).
	Name string `json:"name"`

	// Label overrides the column header / form label in UI when the
	// raw Name is too terse (e.g. Name="phone", Label="Phone number").
	// Empty means "use Name verbatim with first letter uppercased".
	Label string `json:"label,omitempty"`

	// Kind controls parsing, validation, and rendering. Empty defaults
	// to KindText.
	Kind PropertyKind `json:"kind,omitempty"`

	// Description is shown in the property editor as a tooltip /
	// helper line. Optional.
	Description string `json:"description,omitempty"`

	// Required signals UI to flag the row when the property is empty.
	// We DO NOT enforce required-ness at parse time — a note with a
	// missing required field still loads, it just gets a visual
	// warning. The schema is a hint, not a wall.
	Required bool `json:"required,omitempty"`

	// Options is the allowed value list for KindSelect. Ignored for
	// other kinds. Order matters — used as the picker order.
	Options []string `json:"options,omitempty"`

	// Default is the initial value pre-filled when a note is created
	// from this Type. Empty means "leave blank".
	Default string `json:"default,omitempty"`
}

// DisplayLabel returns Label when set, otherwise the Name with the
// first letter uppercased. Helper used by every UI surface that
// renders a property header.
func (p Property) DisplayLabel() string {
	if p.Label != "" {
		return p.Label
	}
	if p.Name == "" {
		return ""
	}
	r := []rune(p.Name)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	// Common substitution: snake_case -> "Snake case". Friendlier
	// than "Snake_case" in a column header.
	return strings.ReplaceAll(string(r), "_", " ")
}

// Validate checks that the property is structurally well-formed. Empty
// Name is a hard error (we'd have nowhere to read the value from).
// Unknown Kind is a hard error. KindSelect with no Options is a hard
// error (otherwise nothing could ever be picked).
func (p Property) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("property name is required")
	}
	if err := p.Kind.Validate(); err != nil {
		return err
	}
	if p.Kind == KindSelect && len(p.Options) == 0 {
		return fmt.Errorf("property %q: kind=select requires non-empty Options", p.Name)
	}
	return nil
}

// Type is a schema describing a class of notes. It carries an ID
// (short stable handle, e.g. "person"), a human-friendly Name, an
// Icon, and the ORDERED Properties list.
type Type struct {
	// ID is the slug used in frontmatter (`type: person`) and as
	// the filename for vault-local override JSON. Must be lower_snake
	// or kebab-case; we don't enforce that beyond rejecting empty
	// or whitespace.
	ID string `json:"id"`

	// Name is shown in the type picker and as the gallery heading.
	Name string `json:"name"`

	// Description is one-line context shown next to the Name in the
	// type picker.
	Description string `json:"description,omitempty"`

	// Icon is a single character (often an emoji) shown next to the
	// type name everywhere. Optional but recommended — it's the
	// fastest visual delineation in dense lists.
	Icon string `json:"icon,omitempty"`

	// Folder is the default vault folder where new instances of this
	// type are created. Empty means vault root.
	Folder string `json:"folder,omitempty"`

	// FilenamePattern controls auto-generated filenames for new
	// instances. Supports {date} (YYYY-MM-DD), {title}, {slug}
	// substitutions. Empty defaults to "{title}".
	FilenamePattern string `json:"filenamePattern,omitempty"`

	// Properties is the ORDERED list of fields on instances of this
	// type. Order drives the gallery column order and the
	// new-instance template field order.
	Properties []Property `json:"properties"`
}

// Validate checks that the type is structurally well-formed and that
// every property validates too. Returns the FIRST error encountered —
// callers wanting all errors at once should call this then iterate
// Properties manually.
func (t Type) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("type ID is required")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("type Name is required")
	}
	seen := map[string]bool{}
	for _, p := range t.Properties {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("type %q: %w", t.ID, err)
		}
		if seen[p.Name] {
			return fmt.Errorf("type %q: duplicate property name %q", t.ID, p.Name)
		}
		seen[p.Name] = true
	}
	return nil
}

// PropertyByName finds a property by its frontmatter key. Returns
// nil when not found — callers should treat that as "this type
// doesn't declare this property" and fall through to displaying the
// raw frontmatter value as text.
func (t Type) PropertyByName(name string) *Property {
	for i := range t.Properties {
		if t.Properties[i].Name == name {
			return &t.Properties[i]
		}
	}
	return nil
}
