package objects

import (
	"sort"
	"strconv"
	"strings"
)

// Evaluate runs a View against an Index and returns the matching objects in
// the order specified by the view's Sort, capped to view.Limit (when > 0).
//
// Resolution order:
//  1. Pick the candidate set: objects of view.Type, or all objects when the
//     view targets every type (Type == "").
//  2. Apply WHERE clauses — every clause must match (AND).
//  3. Sort by the view's Sort spec (default: Title ASC).
//  4. Limit.
//
// The returned slice is freshly allocated; callers are free to mutate it.
// idx must be non-nil; pass NewIndex() if the vault is empty.
func Evaluate(idx *Index, view View) []*Object {
	if idx == nil {
		return nil
	}

	// Step 1 — candidates.
	var candidates []*Object
	if view.Type == "" {
		candidates = make([]*Object, 0, len(idx.byPath))
		for _, obj := range idx.byPath {
			candidates = append(candidates, obj)
		}
	} else {
		// ByType returns the underlying slice — copy so the eventual sort
		// doesn't reorder the index's stable ordering.
		base := idx.ByType(view.Type)
		candidates = make([]*Object, len(base))
		copy(candidates, base)
	}

	// Step 2 — WHERE.
	if len(view.Where) > 0 {
		filtered := candidates[:0]
		for _, obj := range candidates {
			if matchesAll(obj, view.Where) {
				filtered = append(filtered, obj)
			}
		}
		candidates = filtered
	}

	// Step 3 — sort.
	sortObjects(candidates, view.Sort)

	// Step 4 — limit.
	if view.Limit > 0 && len(candidates) > view.Limit {
		candidates = candidates[:view.Limit]
	}

	return candidates
}

// matchesAll reports whether obj satisfies every clause in the AND-group.
func matchesAll(obj *Object, clauses []ViewClause) bool {
	for _, c := range clauses {
		if !matches(obj, c) {
			return false
		}
	}
	return true
}

// matches evaluates a single clause against an object's properties.
//
// Special-case: clause.Property == "title" reads from obj.Title rather than
// obj.Properties (since title is promoted to a dedicated field). Likewise
// "type" reads from obj.TypeID.
func matches(obj *Object, c ViewClause) bool {
	value := propertyOrPromoted(obj, c.Property)
	target := strings.TrimSpace(c.Value)

	switch c.Op {
	case ViewOpEq:
		return strings.EqualFold(strings.TrimSpace(value), target)
	case ViewOpNe:
		return !strings.EqualFold(strings.TrimSpace(value), target)
	case ViewOpContains:
		return strings.Contains(strings.ToLower(value), strings.ToLower(target))
	case ViewOpExists:
		return strings.TrimSpace(value) != ""
	case ViewOpMissing:
		return strings.TrimSpace(value) == ""
	case ViewOpGt, ViewOpLt:
		// Best-effort numeric. If either side doesn't parse, the clause
		// fails open (returns false) — better to silently exclude than
		// to crash on a typo'd frontmatter value.
		lhs, err1 := strconv.ParseFloat(strings.TrimSpace(value), 64)
		rhs, err2 := strconv.ParseFloat(target, 64)
		if err1 != nil || err2 != nil {
			return false
		}
		if c.Op == ViewOpGt {
			return lhs > rhs
		}
		return lhs < rhs
	}
	return false
}

// propertyOrPromoted reads either a property bag entry or one of the two
// promoted fields (title, type). Used by both matches() and sortObjects().
func propertyOrPromoted(obj *Object, name string) string {
	switch strings.ToLower(name) {
	case "title":
		return obj.Title
	case "type":
		return obj.TypeID
	default:
		return obj.PropertyValue(name)
	}
}

// sortObjects orders the slice in place per the spec. Default (nil spec) is
// Title ASC, matching the Index's natural order.
func sortObjects(objs []*Object, spec *ViewSort) {
	if spec == nil || strings.TrimSpace(spec.Property) == "" {
		sort.SliceStable(objs, func(i, j int) bool {
			return strings.ToLower(objs[i].Title) < strings.ToLower(objs[j].Title)
		})
		return
	}
	prop := spec.Property
	descending := strings.ToLower(spec.Direction) == "desc"

	sort.SliceStable(objs, func(i, j int) bool {
		a := propertyOrPromoted(objs[i], prop)
		b := propertyOrPromoted(objs[j], prop)
		// Best-effort numeric — when both parse, sort numerically.
		// Otherwise fall back to case-insensitive string compare.
		af, errA := strconv.ParseFloat(strings.TrimSpace(a), 64)
		bf, errB := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if errA == nil && errB == nil {
			if descending {
				return af > bf
			}
			return af < bf
		}
		al := strings.ToLower(a)
		bl := strings.ToLower(b)
		if descending {
			return al > bl
		}
		return al < bl
	})
}
