package agentruntime

import (
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// Bridge is what the agents package's read+write tools see. It wraps
// granit's live vault/objects/tasks so the same agent that runs in the
// TUI can run from the web server with no behavior change.
//
// One bridge is constructed per agent run so it always reflects the
// freshest state. Cheap — no caching, just thin pointer wrapping.
type Bridge struct {
	vault *vault.Vault
	idx   *objects.Index
	reg   *objects.Registry
	store *tasks.TaskStore
}

// NewBridge constructs a Bridge. registry/index may be nil — the agents'
// read-tools handle that gracefully (query_objects then returns "no
// typed objects" instead of crashing).
func NewBridge(v *vault.Vault, store *tasks.TaskStore, reg *objects.Registry, idx *objects.Index) *Bridge {
	return &Bridge{vault: v, store: store, reg: reg, idx: idx}
}

// ----- VaultReader -----

func (b *Bridge) VaultRoot() string { return b.vault.Root }

func (b *Bridge) NoteContent(rel string) (string, bool) {
	n := b.vault.GetNote(rel)
	if n == nil {
		return "", false
	}
	// Notes may be in lazy-loaded state — content lives on disk only.
	// EnsureLoaded fills it in; cheap if already loaded.
	b.vault.EnsureLoaded(rel)
	n = b.vault.GetNote(rel)
	if n == nil {
		return "", false
	}
	return n.Content, true
}

// SearchVault runs a substring scan across every loaded note's content.
// Mirrors the TUI bridge's behaviour — for typical PKM scale (thousands
// of notes) substring is fast enough and we avoid pulling in the gob
// search-index dep.
func (b *Bridge) SearchVault(query string, limit int) []agents.SearchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var hits []agents.SearchHit
	for rel := range b.vault.Notes {
		b.vault.EnsureLoaded(rel)
		n := b.vault.GetNote(rel)
		if n == nil {
			continue
		}
		body := n.Content
		idx := strings.Index(strings.ToLower(body), q)
		if idx < 0 {
			continue
		}
		start := idx - 60
		if start < 0 {
			start = 0
		}
		end := idx + len(q) + 60
		if end > len(body) {
			end = len(body)
		}
		snippet := strings.TrimSpace(strings.ReplaceAll(body[start:end], "\n", " "))
		hits = append(hits, agents.SearchHit{Path: n.RelPath, Snippet: snippet})
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

func (b *Bridge) ObjectIndex() *objects.Index       { return b.idx }
func (b *Bridge) ObjectRegistry() *objects.Registry { return b.reg }

func (b *Bridge) TaskList() []agents.TaskRecord {
	if b.store == nil {
		return nil
	}
	all := b.store.All()
	out := make([]agents.TaskRecord, 0, len(all))
	for _, t := range all {
		out = append(out, agents.TaskRecord{
			Text:     t.Text,
			NotePath: t.NotePath,
			Done:     t.Done,
			DueDate:  t.DueDate,
			Priority: t.Priority,
			Tags:     append([]string(nil), t.Tags...),
		})
	}
	return out
}

// ----- VaultWriter -----

// WriteNote writes content to a vault-relative path atomically. Used by
// the write_note + create_object tools when a preset has IncludeWrite=true.
//
// The tools already validated the path stays inside the vault, but we
// belt-and-brace it again here — defence in depth against a future
// runtime change that bypasses the tool layer.
func (b *Bridge) WriteNote(rel, content string) (string, error) {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		return "", errOutsideVault
	}
	abs := filepath.Join(b.vault.Root, rel)
	if err := atomicio.WriteNote(abs, content); err != nil {
		return "", err
	}
	// The fs watcher (serveapi.runWatcher) will pick up the write,
	// rebuild affected notes, and broadcast WS events. We don't poke
	// the in-memory cache directly — letting the watcher own that
	// keeps a single source of truth for reactivity.
	return rel, nil
}

// AppendTaskLine adds a "- [ ] {text}" line to today's daily note (or
// creates it if missing). Mirrors the TUI's appender — uses tasks.Create
// so the sidecar gets a stable ID and the new line gets placed under
// "## Tasks" when present.
func (b *Bridge) AppendTaskLine(line string) (string, error) {
	if b.store == nil {
		return "", errNoTaskStore
	}
	// Strip the leading "- [ ] " that the agent's create_task tool
	// builds — tasks.Create takes plain text.
	text := strings.TrimSpace(line)
	for _, prefix := range []string{"- [ ] ", "- [x] ", "* [ ] ", "* [x] "} {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(text[len(prefix):])
			break
		}
	}
	if text == "" {
		return "", errEmptyTaskText
	}
	// Default home: today's daily note. The tool adapter could pick a
	// different note via parameter, but we keep the no-arg default here.
	notePath := todayDailyPath(b.vault.Root)
	t, err := b.store.Create(text, tasks.CreateOpts{
		File:    notePath,
		Section: "## Tasks",
	})
	if err != nil {
		return "", err
	}
	return t.NotePath, nil
}

// todayDailyPath mirrors the daily-folder convention used elsewhere.
// Cheap fallback: if the vault has no daily folder configured we land
// the task in Jots/, which is the default.
func todayDailyPath(_ string) string {
	// We can't import internal/daily here without a dep loop; the
	// TaskStore.Create call works on whatever path we hand it and
	// creates the file if missing.
	return "Jots/" + nowYMD() + ".md"
}

type bridgeErr string

func (e bridgeErr) Error() string { return string(e) }

const (
	errOutsideVault  bridgeErr = "agent bridge: path escapes vault root"
	errNoTaskStore   bridgeErr = "agent bridge: no task store wired"
	errEmptyTaskText bridgeErr = "agent bridge: empty task text"
)
