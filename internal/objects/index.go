package objects

import (
	"sort"
	"strings"
)

// Object is the runtime representation of a single typed note. It
// pairs the Type ID + property values pulled from the note's
// frontmatter with enough source-tracking to navigate back to the
// underlying markdown file.
type Object struct {
	// TypeID is the value of the `type:` frontmatter key. May
	// reference a type the registry doesn't know — see
	// Index.Untyped for those.
	TypeID string

	// NotePath is the vault-relative path of the source note (e.g.
	// "People/Sebastian.md"). Acts as the unique identifier within
	// the index.
	NotePath string

	// Title is the note's display name. Pulled from frontmatter
	// `title:`, falling back to the first H1, falling back to the
	// filename without extension. Always non-empty.
	Title string

	// Properties carries every key/value pair from the note's
	// frontmatter (excluding `type:` and `title:` since those are
	// promoted to TypeID and Title above). Values are kept as
	// strings — the renderer interprets them per the corresponding
	// Property.Kind in the schema.
	Properties map[string]string
}

// PropertyValue returns the string value of a property by name, or
// "" if the property isn't set on this object. Convenience helper
// used by the gallery columns and search filter.
func (o Object) PropertyValue(name string) string {
	if o.Properties == nil {
		return ""
	}
	return o.Properties[name]
}

// Index is the live mapping of TypeID -> []*Object built from a vault
// scan. Held by the TUI and refreshed when the underlying notes
// change.
//
// Concurrency: an Index is read-mostly. Build / Refresh swap the
// internal maps wholesale, so a reader holding a returned slice
// continues to see a stable snapshot. Mutation while iterating is
// not supported (and not needed by callers today).
type Index struct {
	// byType groups objects by their TypeID for O(1) gallery
	// lookups. Within each slice, objects are sorted by Title for
	// stable rendering.
	byType map[string][]*Object

	// byPath maps note paths back to objects so the editor can ask
	// "is the note I just opened a typed object, and if so, what
	// type?" in O(1).
	byPath map[string]*Object

	// untyped is the count of frontmatter `type:` references that
	// don't match any registered type. Surfaced in the Object
	// Browser as a hint that the user might want to register the
	// type or fix typos.
	untyped int
}

// NewIndex returns an empty Index. Use Build to populate from a vault.
func NewIndex() *Index {
	return &Index{
		byType: map[string][]*Object{},
		byPath: map[string]*Object{},
	}
}

// Builder accepts notes one at a time and produces an Index. The TUI
// calls Add for each note in the vault, then Finalize() to get the
// finished Index back. Splitting build into Add+Finalize lets the
// caller stream notes without buffering them all up front.
type Builder struct {
	idx *Index
	r   *Registry
}

// NewBuilder starts a fresh Index build using the given Registry to
// resolve type IDs to schemas. Registry must be non-nil.
func NewBuilder(r *Registry) *Builder {
	return &Builder{idx: NewIndex(), r: r}
}

// Add records a single note in the index. The note's frontmatter must
// have already been parsed by the caller — Builder doesn't re-parse
// markdown to keep this package decoupled from any specific
// frontmatter implementation.
//
// Notes without a `type:` frontmatter are silently skipped (they're
// regular notes, not typed objects).
//
// path is the vault-relative path; title is the resolved display
// title (caller already applied frontmatter > H1 > filename
// fallback); fm is the flat frontmatter map.
func (b *Builder) Add(path, title string, fm map[string]string) {
	typeID := strings.TrimSpace(fm["type"])
	if typeID == "" {
		return
	}
	// Strip type + title from the property bag — they're promoted to
	// dedicated fields. Everything else passes through verbatim.
	props := make(map[string]string, len(fm))
	for k, v := range fm {
		if k == "type" || k == "title" {
			continue
		}
		props[k] = v
	}
	if title == "" {
		title = path
	}
	obj := &Object{
		TypeID: typeID, NotePath: path, Title: title, Properties: props,
	}
	b.idx.byType[typeID] = append(b.idx.byType[typeID], obj)
	b.idx.byPath[path] = obj
	if _, known := b.r.ByID(typeID); !known {
		b.idx.untyped++
	}
}

// Finalize returns the constructed Index after sorting each per-type
// slice by Title (case-insensitive) for stable gallery rendering.
func (b *Builder) Finalize() *Index {
	for typeID := range b.idx.byType {
		objs := b.idx.byType[typeID]
		sort.SliceStable(objs, func(i, j int) bool {
			return strings.ToLower(objs[i].Title) < strings.ToLower(objs[j].Title)
		})
		b.idx.byType[typeID] = objs
	}
	return b.idx
}

// ByType returns the objects of the given type, or an empty slice
// when none exist. Callers must NOT mutate the returned slice — it's
// shared with the index. Order is deterministic (Title ASC).
func (i *Index) ByType(typeID string) []*Object {
	if i == nil {
		return nil
	}
	return i.byType[typeID]
}

// ByPath returns the Object representation of a note, or nil when the
// note isn't a typed object. Used by the editor to ask "is this note
// typed, and if so what's its schema?" right after a tab opens.
func (i *Index) ByPath(path string) *Object {
	if i == nil {
		return nil
	}
	return i.byPath[path]
}

// CountByType returns a map of TypeID → object count, used by the
// Object Browser type list to show "Person (12)" badges. Only types
// that actually have objects appear; types with zero instances are
// not in the map.
func (i *Index) CountByType() map[string]int {
	if i == nil {
		return nil
	}
	out := make(map[string]int, len(i.byType))
	for typeID, objs := range i.byType {
		out[typeID] = len(objs)
	}
	return out
}

// Total reports the total number of typed objects across all types.
// Useful for the empty-state branch of the Object Browser.
func (i *Index) Total() int {
	if i == nil {
		return 0
	}
	n := 0
	for _, objs := range i.byType {
		n += len(objs)
	}
	return n
}

// UntypedCount reports how many objects had a `type:` value the
// registry didn't recognise. Surfaced as a hint in the UI: "3 notes
// reference unknown types — review or register them".
func (i *Index) UntypedCount() int {
	if i == nil {
		return 0
	}
	return i.untyped
}

// Search returns objects of the given type whose Title or any
// property value contains the query string (case-insensitive). Used
// by the Object Browser's filter box.
//
// Empty query returns the full unfiltered list. Pass typeID="" to
// search across all types.
func (i *Index) Search(typeID, query string) []*Object {
	if i == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var pool []*Object
	if typeID == "" {
		for _, objs := range i.byType {
			pool = append(pool, objs...)
		}
		sort.SliceStable(pool, func(i, j int) bool {
			return strings.ToLower(pool[i].Title) < strings.ToLower(pool[j].Title)
		})
	} else {
		pool = i.byType[typeID]
	}
	if q == "" {
		return pool
	}
	var out []*Object
	for _, o := range pool {
		if strings.Contains(strings.ToLower(o.Title), q) {
			out = append(out, o)
			continue
		}
		matched := false
		for _, v := range o.Properties {
			if strings.Contains(strings.ToLower(v), q) {
				matched = true
				break
			}
		}
		if matched {
			out = append(out, o)
		}
	}
	return out
}
