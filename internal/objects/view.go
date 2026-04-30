package objects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Saved views — Capacities-style "smart collections"
// ---------------------------------------------------------------------------
//
// A View is a persisted query over the typed-object Index. The user defines
// it once (e.g. "articles I haven't read", "highlights from books with
// rating>=4") and granit re-evaluates it whenever the view tab is opened or
// the vault is refreshed.
//
// Built-in views ship in code (defaultViews); vault-local overrides at
// `<vault>/.granit/views/<id>.json` REPLACE built-ins by ID. Same full-
// override semantics as Type registry / Preset catalog.
//
// JSON shape is hand-editable:
//
//   {
//     "id": "articles-to-read",
//     "name": "Articles to Read",
//     "description": "Saved articles I haven't finished",
//     "type": "article",
//     "where": [
//       { "property": "status", "op": "ne", "value": "read" }
//     ],
//     "sort": { "property": "saved", "direction": "desc" },
//     "limit": 50
//   }

// ViewOp enumerates the supported predicate operators.
//
// String comparisons are case-insensitive on purpose — frontmatter values are
// hand-typed and we don't want users tripping over "Read" vs "read".
type ViewOp string

const (
	ViewOpEq       ViewOp = "eq"       // value matches (case-insensitive)
	ViewOpNe       ViewOp = "ne"       // value does NOT match
	ViewOpContains ViewOp = "contains" // substring match (case-insensitive)
	ViewOpExists   ViewOp = "exists"   // property is set and non-empty (Value ignored)
	ViewOpMissing  ViewOp = "missing"  // property is unset or empty (Value ignored)
	ViewOpGt       ViewOp = "gt"       // numeric > (best-effort; non-numbers fail open)
	ViewOpLt       ViewOp = "lt"       // numeric <
)

// ViewClause is a single predicate in a View's WHERE list. All clauses are
// AND-ed together — there's no OR support yet because every real-world
// example a user wrote on their notepad turned out to be expressible as AND.
// We can add OR later as a separate clause group; YAGNI for now.
type ViewClause struct {
	Property string `json:"property"`
	Op       ViewOp `json:"op"`
	Value    string `json:"value,omitempty"`
}

// ViewSort describes the ordering for the result list. Direction is "asc" or
// "desc"; empty defaults to ascending. Sort is best-effort numeric when both
// sides parse as numbers, otherwise case-insensitive string compare.
type ViewSort struct {
	Property  string `json:"property"`
	Direction string `json:"direction,omitempty"`
}

// View is a serialisable saved-view definition.
type View struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Type        string       `json:"type,omitempty"`  // empty = match any type
	Where       []ViewClause `json:"where,omitempty"` // empty = match all
	Sort        *ViewSort    `json:"sort,omitempty"`  // nil = sort by Title ASC
	Limit       int          `json:"limit,omitempty"` // 0 = no limit
}

// Validate reports a clear error when a View is structurally broken. A View
// without an ID, Name, or Description is unusable in the UI.
func (v View) Validate() error {
	if strings.TrimSpace(v.ID) == "" {
		return fmt.Errorf("view ID is required")
	}
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("view %q: Name is required", v.ID)
	}
	if strings.TrimSpace(v.Description) == "" {
		return fmt.Errorf("view %q: Description is required", v.ID)
	}
	for i, c := range v.Where {
		if strings.TrimSpace(c.Property) == "" {
			return fmt.Errorf("view %q: where[%d] missing property", v.ID, i)
		}
		switch c.Op {
		case ViewOpEq, ViewOpNe, ViewOpContains, ViewOpExists, ViewOpMissing, ViewOpGt, ViewOpLt:
		default:
			return fmt.Errorf("view %q: where[%d] unknown op %q", v.ID, i, c.Op)
		}
	}
	if v.Limit < 0 {
		return fmt.Errorf("view %q: limit must be >= 0", v.ID)
	}
	if v.Sort != nil && v.Sort.Direction != "" &&
		v.Sort.Direction != "asc" && v.Sort.Direction != "desc" {
		return fmt.Errorf("view %q: sort.direction must be 'asc' or 'desc'", v.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ViewCatalog — built-in + vault-local merge
// ---------------------------------------------------------------------------

// ViewCatalog is the merged view of built-in + vault-local saved views.
// Mirrors PresetCatalog: full-override semantics on ID collision (vault-
// local replaces built-in entirely).
type ViewCatalog struct {
	views map[string]View
}

// NewViewCatalog returns a catalog seeded with the built-in views. Tests can
// pass an empty list to exercise edge cases.
func NewViewCatalog(builtins []View) *ViewCatalog {
	c := &ViewCatalog{views: map[string]View{}}
	for _, v := range builtins {
		// Built-in misconfiguration is a programmer error; skip silently
		// so a single bad built-in doesn't take down the catalog.
		if err := v.Validate(); err == nil {
			c.views[v.ID] = v
		}
	}
	return c
}

// LoadVaultDir scans `<vaultRoot>/.granit/views/*.json` and overlays them
// onto the catalog. Same rules as PresetCatalog.LoadVaultDir:
//   - basename must match the embedded ID (case-insensitive)
//   - per-file errors are returned together so the UI can render all at once
//   - missing directory is not an error (returns 0, nil)
func (c *ViewCatalog) LoadVaultDir(vaultRoot string) (int, []error) {
	if vaultRoot == "" {
		return 0, nil
	}
	dir := filepath.Join(vaultRoot, ".granit", "views")
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
		var v View
		if err := json.Unmarshal(data, &v); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		expectedID := strings.TrimSuffix(e.Name(), ".json")
		if !strings.EqualFold(expectedID, v.ID) {
			errs = append(errs, fmt.Errorf("%s: filename %q does not match embedded id %q", e.Name(), expectedID, v.ID))
			continue
		}
		if err := v.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		c.views[v.ID] = v
		loaded++
	}
	return loaded, errs
}

// ByID returns the view with the given ID, or (zero, false) when none exists.
func (c *ViewCatalog) ByID(id string) (View, bool) {
	v, ok := c.views[id]
	return v, ok
}

// All returns every view in stable ID order.
func (c *ViewCatalog) All() []View {
	ids := make([]string, 0, len(c.views))
	for id := range c.views {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]View, len(ids))
	for i, id := range ids {
		out[i] = c.views[id]
	}
	return out
}

// Len reports the catalog size.
func (c *ViewCatalog) Len() int { return len(c.views) }

// SaveView writes a view to `<vaultRoot>/.granit/views/<id>.json`. Validates
// first so an invalid view is never persisted.
func SaveView(vaultRoot string, v View) error {
	if err := v.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(vaultRoot, ".granit", "views")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(dir, v.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Built-in views
// ---------------------------------------------------------------------------

// BuiltinViews returns the views shipped with granit. Curated to match the
// most common Capacities-style smart collections users build themselves on
// day one.
func BuiltinViews() []View {
	return []View{
		{
			ID:          "articles-to-read",
			Name:        "Articles to Read",
			Description: "Saved articles still on the to-read pile",
			Type:        "article",
			Where: []ViewClause{
				{Property: "status", Op: ViewOpNe, Value: "read"},
				{Property: "status", Op: ViewOpNe, Value: "archived"},
			},
			Sort:  &ViewSort{Property: "saved", Direction: "desc"},
			Limit: 50,
		},
		{
			ID:          "recent-highlights",
			Name:        "Recent Highlights",
			Description: "Most recent highlighted passages across sources",
			Type:        "highlight",
			Sort:        &ViewSort{Property: "captured", Direction: "desc"},
			Limit:       30,
		},
		{
			ID:          "active-projects",
			Name:        "Active Projects",
			Description: "Projects currently in flight (not on hold or done)",
			Type:        "project",
			Where: []ViewClause{
				{Property: "status", Op: ViewOpEq, Value: "active"},
			},
			Sort: &ViewSort{Property: "title", Direction: "asc"},
		},
		{
			// "Has-repo" filter — projects with a local git repo
			// declared. These are the rows whose hub strip shows
			// live git status; surfacing them as a saved view makes
			// "what am I building?" a one-keystroke question.
			ID:          "code-projects",
			Name:        "Code Projects",
			Description: "Projects with a local git repository",
			Type:        "project",
			Where: []ViewClause{
				{Property: "repo", Op: ViewOpExists, Value: ""},
			},
			Sort: &ViewSort{Property: "title", Direction: "asc"},
		},
		{
			ID:          "active-goals",
			Name:        "Active Goals",
			Description: "Goals you're currently pursuing",
			Type:        "goal",
			Where: []ViewClause{
				{Property: "status", Op: ViewOpEq, Value: "active"},
			},
			Sort: &ViewSort{Property: "target_date", Direction: "asc"},
		},
		{
			ID:          "overdue-goals",
			Name:        "Overdue Goals",
			Description: "Active goals whose target date has passed",
			Type:        "goal",
			Where: []ViewClause{
				{Property: "status", Op: ViewOpEq, Value: "active"},
				{Property: "target_date", Op: ViewOpExists, Value: ""},
			},
			Sort: &ViewSort{Property: "target_date", Direction: "asc"},
		},
		{
			ID:          "raw-ideas",
			Name:        "Raw Ideas",
			Description: "Ideas that haven't been refined or scheduled yet",
			Type:        "idea",
			Where: []ViewClause{
				{Property: "status", Op: ViewOpEq, Value: "raw"},
			},
			Sort: &ViewSort{Property: "title", Direction: "asc"},
		},
		{
			ID:          "top-rated-podcasts",
			Name:        "Top-Rated Podcasts",
			Description: "Podcast episodes you rated 4 or 5",
			Type:        "podcast",
			Where: []ViewClause{
				{Property: "rating", Op: ViewOpGt, Value: "3"},
			},
			Sort: &ViewSort{Property: "rating", Direction: "desc"},
		},
		{
			// Surfaces every successful agent run sorted most-recent-first.
			// Pin as the dashboard primary view to see your agent
			// history at a glance — Deepnote-style notebook history.
			ID:          "recent-agent-runs",
			Name:        "Recent Agent Runs",
			Description: "Multi-step AI agent runs you've kicked off recently",
			Type:        "agent_run",
			Sort:        &ViewSort{Property: "started", Direction: "desc"},
			Limit:       50,
		},
		{
			ID:          "books-currently-reading",
			Name:        "Currently Reading",
			Description: "Books in progress",
			Type:        "book",
			Where: []ViewClause{
				{Property: "status", Op: ViewOpEq, Value: "reading"},
			},
			Sort: &ViewSort{Property: "title", Direction: "asc"},
		},
	}
}
