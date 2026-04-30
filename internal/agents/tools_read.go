package agents

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/objects"
)

// VaultReader is the minimal interface read-tools need to query
// vault state. Decouples the agents package from internal/vault and
// internal/tasks — the TUI builds adapters on its side and hands
// them in.
//
// All methods MUST be safe to call from any goroutine; the agent
// runtime calls them serially within a session but multiple agents
// can run concurrently in future.
type VaultReader interface {
	// VaultRoot returns the absolute path to the vault root.
	// Tools use it to resolve relative paths and to refuse paths
	// that escape the vault.
	VaultRoot() string
	// NoteContent returns the body of the note at the
	// vault-relative path, or (false, "") when the note doesn't
	// exist. We deliberately return (string, bool) instead of
	// (string, error) — "not found" is the common case for an
	// LLM-driven query and shouldn't surface as an error.
	NoteContent(relPath string) (string, bool)
	// SearchVault returns vault-relative paths whose content
	// matches the query (case-insensitive substring or simple
	// fuzzy — the implementation decides). Limit caps the result
	// count so a wide query doesn't blow the LLM context.
	SearchVault(query string, limit int) []SearchHit
	// ObjectIndex returns the live typed-objects index. nil is
	// allowed — query_objects then returns "no typed objects".
	ObjectIndex() *objects.Index
	// ObjectRegistry returns the live registry; same nil semantics
	// as ObjectIndex.
	ObjectRegistry() *objects.Registry
	// TaskList returns the current set of tasks in the vault as
	// a flat slice. Each TaskRecord must include Text, Done,
	// DueDate, Priority, NotePath at minimum so the LLM can
	// filter intelligently.
	TaskList() []TaskRecord
}

// SearchHit is one result from VaultReader.SearchVault. The Snippet
// field carries a short excerpt around the first match — handed
// straight to the LLM as part of the observation so it can decide
// whether to read the full note.
type SearchHit struct {
	Path    string
	Snippet string
}

// TaskRecord is the agents-package-local view of a task. Mirrors the
// fields the LLM might filter on (Done, DueDate, Priority, Tags) but
// stays decoupled from internal/tasks so the package import-graph
// flows in one direction (tui imports agents, never the reverse).
type TaskRecord struct {
	Text     string
	NotePath string
	Done     bool
	DueDate  string // YYYY-MM-DD or empty
	Priority int    // 0=none, 1=low, 4=highest
	Tags     []string
}

// ReadNote returns a Tool that fetches the body of a markdown note.
// The LLM provides the path; the tool validates it stays inside the
// vault root (cheap defence against `..` escapes — VaultReader
// implementations MUST do the same, this is belt-and-braces).
//
// Output is truncated to ~6 KB by default to keep the LLM's context
// from filling up on a single big note. The LLM sees a footer
// telling it how to fetch more.
func ReadNote(vault VaultReader) Tool {
	return &readNoteTool{vault: vault}
}

type readNoteTool struct{ vault VaultReader }

func (t *readNoteTool) Name() string { return "read_note" }
func (t *readNoteTool) Description() string {
	return "Read the markdown body of a note in the vault. Use this AFTER search_vault or query_objects locates a path you want to inspect."
}
func (t *readNoteTool) Kind() ToolKind { return KindRead }
func (t *readNoteTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "path", Description: "Vault-relative path of the note (e.g. 'People/Sebastian.md')", Required: true},
		{Name: "max_chars", Description: "Truncate the body to this many characters; default 6000"},
	}
}

func (t *readNoteTool) Run(_ context.Context, args map[string]string) ToolResult {
	path := strings.TrimSpace(args["path"])
	if !pathInsideVault(t.vault.VaultRoot(), path) {
		return ToolResult{Err: fmt.Errorf("path %q escapes the vault", path)}
	}
	body, ok := t.vault.NoteContent(path)
	if !ok {
		return ToolResult{Output: fmt.Sprintf("(no note at %q)", path)}
	}
	limit := 6000
	if v, _ := strconv.Atoi(args["max_chars"]); v > 0 {
		limit = v
	}
	if len(body) > limit {
		body = body[:limit] + fmt.Sprintf("\n\n[truncated — %d more chars; call read_note again with a higher max_chars to see them]",
			len(body)-limit)
	}
	return ToolResult{Output: body}
}

// ListNotes returns a Tool that lists vault-relative paths under a
// folder, optionally filtered by an extension. Cheap, useful before
// SearchVault when the agent knows the folder but not the exact
// filename.
func ListNotes(vault VaultReader) Tool {
	return &listNotesTool{vault: vault}
}

type listNotesTool struct{ vault VaultReader }

func (t *listNotesTool) Name() string { return "list_notes" }
func (t *listNotesTool) Description() string {
	return "List vault-relative paths of notes under a folder. Use this when the user names a folder you want to enumerate."
}
func (t *listNotesTool) Kind() ToolKind { return KindRead }
func (t *listNotesTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "folder", Description: "Vault-relative folder (empty for vault root)"},
		{Name: "limit", Description: "Cap on result count; default 50"},
	}
}

func (t *listNotesTool) Run(_ context.Context, args map[string]string) ToolResult {
	folder := strings.TrimSpace(args["folder"])
	if folder != "" && !pathInsideVault(t.vault.VaultRoot(), folder) {
		return ToolResult{Err: fmt.Errorf("folder %q escapes the vault", folder)}
	}
	limit := 50
	if v, _ := strconv.Atoi(args["limit"]); v > 0 {
		limit = v
	}
	root := t.vault.VaultRoot()
	abs := root
	if folder != "" {
		abs = filepath.Join(root, folder)
	}
	var results []string
	err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Folder might not exist — surface as empty list,
			// not error, so the LLM gets actionable feedback
			// without the run dying.
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".granit" || name == ".obsidian" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		results = append(results, rel)
		return nil
	})
	if err != nil {
		return ToolResult{Err: fmt.Errorf("list_notes: %w", err)}
	}
	sort.Strings(results)
	truncated := false
	if len(results) > limit {
		results = results[:limit]
		truncated = true
	}
	if len(results) == 0 {
		return ToolResult{Output: "(no notes)"}
	}
	out := strings.Join(results, "\n")
	if truncated {
		out += fmt.Sprintf("\n\n[truncated to %d; raise limit to see more]", limit)
	}
	return ToolResult{Output: out}
}

// SearchVault returns a Tool that runs a substring search across the
// vault, returning matched paths + short snippets.
func SearchVault(vault VaultReader) Tool {
	return &searchVaultTool{vault: vault}
}

type searchVaultTool struct{ vault VaultReader }

func (t *searchVaultTool) Name() string { return "search_vault" }
func (t *searchVaultTool) Description() string {
	return "Find notes that mention a query string. Returns paths plus a short context snippet around each match."
}
func (t *searchVaultTool) Kind() ToolKind { return KindRead }
func (t *searchVaultTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "query", Description: "Free-text query (case-insensitive substring or fuzzy match)", Required: true},
		{Name: "limit", Description: "Cap on result count; default 10"},
	}
}

func (t *searchVaultTool) Run(_ context.Context, args map[string]string) ToolResult {
	q := strings.TrimSpace(args["query"])
	if q == "" {
		return ToolResult{Err: fmt.Errorf("search_vault: empty query")}
	}
	limit := 10
	if v, _ := strconv.Atoi(args["limit"]); v > 0 {
		limit = v
	}
	hits := t.vault.SearchVault(q, limit)
	if len(hits) == 0 {
		return ToolResult{Output: fmt.Sprintf("(no matches for %q)", q)}
	}
	var b strings.Builder
	for i, h := range hits {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s", i+1, h.Path)
		if h.Snippet != "" {
			fmt.Fprintf(&b, "\n   %s", strings.ReplaceAll(strings.TrimSpace(h.Snippet), "\n", " "))
		}
	}
	return ToolResult{Output: b.String()}
}

// QueryObjects returns a Tool that filters the typed-objects index
// by type and optional property=value constraints. Bridges Phase 1's
// objects.Index to the agent runtime so an LLM can ask "give me all
// books where status=read".
func QueryObjects(vault VaultReader) Tool {
	return &queryObjectsTool{vault: vault}
}

type queryObjectsTool struct{ vault VaultReader }

func (t *queryObjectsTool) Name() string { return "query_objects" }
func (t *queryObjectsTool) Description() string {
	return "Query typed objects by type ID and optional property=value filters. Returns object titles and paths."
}
func (t *queryObjectsTool) Kind() ToolKind { return KindRead }
func (t *queryObjectsTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "type", Description: "Type ID (e.g. 'person', 'book'); empty searches all types", Required: false},
		{Name: "where", Description: "Comma-separated key=value filters (e.g. 'status=read,rating=5')"},
		{Name: "limit", Description: "Cap on result count; default 50"},
	}
}

func (t *queryObjectsTool) Run(_ context.Context, args map[string]string) ToolResult {
	idx := t.vault.ObjectIndex()
	if idx == nil {
		return ToolResult{Output: "(typed-objects index not initialised; vault has no typed notes)"}
	}
	limit := 50
	if v, _ := strconv.Atoi(args["limit"]); v > 0 {
		limit = v
	}
	typeID := strings.TrimSpace(args["type"])
	filters := parseWhereClause(args["where"])

	var pool []*objects.Object
	if typeID == "" {
		// Search across all types — gather then filter.
		reg := t.vault.ObjectRegistry()
		if reg != nil {
			for _, tt := range reg.All() {
				pool = append(pool, idx.ByType(tt.ID)...)
			}
		}
	} else {
		pool = idx.ByType(typeID)
	}

	out := pool[:0]
	for _, o := range pool {
		if matchesFilters(o, filters) {
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		if typeID != "" {
			return ToolResult{Output: fmt.Sprintf("(no %s objects match)", typeID)}
		}
		return ToolResult{Output: "(no objects match)"}
	}

	if len(out) > limit {
		out = out[:limit]
	}
	var b strings.Builder
	for i, o := range out {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "- %s (%s) — %s", o.Title, o.TypeID, o.NotePath)
	}
	return ToolResult{Output: b.String()}
}

// QueryTasks returns a Tool that filters the vault's task list by
// done state, due-date window, and priority. The LLM hands granit's
// own task system into multi-step planning.
func QueryTasks(vault VaultReader) Tool {
	return &queryTasksTool{vault: vault}
}

type queryTasksTool struct{ vault VaultReader }

func (t *queryTasksTool) Name() string { return "query_tasks" }
func (t *queryTasksTool) Description() string {
	return "Query vault tasks by done state, due-date window, and priority."
}
func (t *queryTasksTool) Kind() ToolKind { return KindRead }
func (t *queryTasksTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "status", Description: "'open' (default) or 'done'"},
		{Name: "due", Description: "'today', 'overdue', 'upcoming' (next 7 days), or 'any'"},
		{Name: "min_priority", Description: "Filter to priority >= N (0=none .. 4=highest)"},
		{Name: "limit", Description: "Cap on result count; default 30"},
	}
}

func (t *queryTasksTool) Run(_ context.Context, args map[string]string) ToolResult {
	status := strings.TrimSpace(args["status"])
	if status == "" {
		status = "open"
	}
	due := strings.TrimSpace(args["due"])
	if due == "" {
		due = "any"
	}
	minPrio, _ := strconv.Atoi(args["min_priority"])
	limit := 30
	if v, _ := strconv.Atoi(args["limit"]); v > 0 {
		limit = v
	}
	tasks := t.vault.TaskList()
	today := time.Now().Format("2006-01-02")
	weekFromNow := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	var hits []TaskRecord
	for _, tk := range tasks {
		if status == "open" && tk.Done {
			continue
		}
		if status == "done" && !tk.Done {
			continue
		}
		if tk.Priority < minPrio {
			continue
		}
		switch due {
		case "today":
			if tk.DueDate != today {
				continue
			}
		case "overdue":
			if tk.DueDate == "" || tk.DueDate >= today {
				continue
			}
		case "upcoming":
			if tk.DueDate == "" || tk.DueDate < today || tk.DueDate > weekFromNow {
				continue
			}
		}
		hits = append(hits, tk)
		if len(hits) >= limit {
			break
		}
	}
	if len(hits) == 0 {
		return ToolResult{Output: "(no tasks match)"}
	}
	var b strings.Builder
	for i, tk := range hits {
		if i > 0 {
			b.WriteString("\n")
		}
		marker := "[ ]"
		if tk.Done {
			marker = "[x]"
		}
		fmt.Fprintf(&b, "%s %s", marker, tk.Text)
		if tk.DueDate != "" {
			fmt.Fprintf(&b, " (due: %s)", tk.DueDate)
		}
		if tk.NotePath != "" {
			fmt.Fprintf(&b, " — %s", tk.NotePath)
		}
	}
	return ToolResult{Output: b.String()}
}

// GetToday returns a Tool that exposes the current date. Trivial but
// load-bearing — without it LLMs hallucinate dates ("yesterday" is
// not what they think it is) and any tool that filters by date
// becomes unreliable.
func GetToday() Tool {
	return &getTodayTool{}
}

type getTodayTool struct{}

func (t *getTodayTool) Name() string         { return "get_today" }
func (t *getTodayTool) Description() string  { return "Return today's date in YYYY-MM-DD form. Use this before any date-filtered query." }
func (t *getTodayTool) Kind() ToolKind       { return KindRead }
func (t *getTodayTool) Params() []ToolParam  { return nil }
func (t *getTodayTool) Run(_ context.Context, _ map[string]string) ToolResult {
	return ToolResult{Output: time.Now().Format("2006-01-02")}
}

// pathInsideVault rejects paths that escape the vault root via `..`
// or absolute paths. Used by every read tool that takes a path
// argument — defence in depth even when the caller's VaultReader
// already validates.
func pathInsideVault(root, rel string) bool {
	if rel == "" {
		return true
	}
	if filepath.IsAbs(rel) {
		return false
	}
	abs, err := filepath.Abs(filepath.Join(root, rel))
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	return strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) || abs == rootAbs
}

// parseWhereClause turns "key1=value1,key2=value2" into a
// map[string]string. Used by query_objects for property filters.
// Whitespace around keys/values is trimmed.
func parseWhereClause(s string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(s) == "" {
		return out
	}
	for _, part := range strings.Split(s, ",") {
		eq := strings.Index(part, "=")
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(part[:eq])
		v := strings.TrimSpace(part[eq+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// matchesFilters returns true when every filter key on o has the
// expected value. Comparison is case-insensitive on values to
// match how users casually write filters.
func matchesFilters(o *objects.Object, filters map[string]string) bool {
	for k, v := range filters {
		actual := strings.ToLower(strings.TrimSpace(o.PropertyValue(k)))
		if actual != strings.ToLower(strings.TrimSpace(v)) {
			return false
		}
	}
	return true
}
