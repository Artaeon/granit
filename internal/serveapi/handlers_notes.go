package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/annotations"
	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/history"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
	"gopkg.in/yaml.v3"
)

type noteSummary struct {
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	ModTime     time.Time              `json:"modTime"`
	Size        int64                  `json:"size"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
}

type noteFull struct {
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	ModTime     time.Time              `json:"modTime"`
	Size        int64                  `json:"size"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body"`
	Links       []string               `json:"links,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
}

func tagsFor(n *vault.Note) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(t string) {
		t = strings.TrimPrefix(strings.TrimSpace(t), "#")
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	if n.Frontmatter != nil {
		switch v := n.Frontmatter["tags"].(type) {
		case string:
			for _, t := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
				add(t)
			}
		case []interface{}:
			for _, t := range v {
				if s, ok := t.(string); ok {
					add(s)
				}
			}
		}
	}
	for _, m := range inlineTagRe.FindAllStringSubmatch(n.Content, -1) {
		add(m[2])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 100
	}
	typeF := q.Get("type")
	tagF := q.Get("tag")
	folderF := strings.TrimSuffix(q.Get("folder"), "/")
	qStr := strings.ToLower(q.Get("q"))

	notes := s.cfg.Vault.SnapshotNotes()
	out := make([]*vault.Note, 0, len(notes))
	for _, n := range notes {
		if folderF != "" && !strings.HasPrefix(n.RelPath, folderF+"/") {
			continue
		}
		if typeF != "" {
			if t, _ := n.Frontmatter["type"].(string); t != typeF {
				continue
			}
		}
		if tagF != "" {
			tags := tagsFor(n)
			has := false
			for _, t := range tags {
				if t == tagF {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		if qStr != "" {
			if !strings.Contains(strings.ToLower(n.Title), qStr) && !strings.Contains(strings.ToLower(n.Content), qStr) {
				continue
			}
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	total := len(out)
	if offset > 0 && offset < len(out) {
		out = out[offset:]
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	resp := make([]noteSummary, 0, len(out))
	for _, n := range out {
		resp = append(resp, noteSummary{
			Path:        n.RelPath,
			Title:       n.Title,
			ModTime:     n.ModTime,
			Size:        n.Size,
			Frontmatter: n.Frontmatter,
			Tags:        tagsFor(n),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notes":  resp,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing path")
		return
	}
	// Defense in depth: vault.GetNote currently looks up by canonical
	// relpath so traversal attempts hit a 404 anyway, but a future
	// refactor that adds disk fallback would be a real escape. Reject
	// `..` and absolute paths up front. Cheap and matches PUT/POST.
	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	n := s.cfg.Vault.GetNote(path)
	if n == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	s.cfg.Vault.EnsureLoaded(path)
	body := stripFrontmatterBody(n.Content)
	w.Header().Set("ETag", s.etagFor(n.ModTime, n.Size))
	writeJSON(w, http.StatusOK, noteFull{
		Path:        n.RelPath,
		Title:       n.Title,
		ModTime:     n.ModTime,
		Size:        n.Size,
		Frontmatter: n.Frontmatter,
		Body:        body,
		Links:       n.Links,
		Tags:        tagsFor(n),
	})
}

type writeNoteBody struct {
	Frontmatter map[string]interface{} `json:"frontmatter"`
	Body        string                 `json:"body"`
}

func (s *Server) handlePutNote(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	var b writeNoteBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(path))
	if existing := s.cfg.Vault.GetNote(path); existing != nil {
		if ifMatch := r.Header.Get("If-Match"); ifMatch != "" {
			if s.etagFor(existing.ModTime, existing.Size) != ifMatch {
				writeError(w, http.StatusPreconditionFailed, "etag mismatch")
				return
			}
		}
	}
	content, err := serializeNote(b.Frontmatter, b.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// File history: snapshot the prior on-disk content BEFORE the
	// new write lands. If the file doesn't exist yet (first save of
	// a brand-new note created via this PUT path) the read fails
	// with ENOENT and we skip the snapshot — there's no prior state
	// to preserve. The snapshot package dedupes on content hash, so
	// a chain of identical autosaves produces only one history
	// entry. Errors are logged but never fail the write — losing a
	// snapshot is degraded but not catastrophic; failing the save
	// because we couldn't snapshot would be.
	if prior, rerr := os.ReadFile(abs); rerr == nil {
		if _, herr := history.Snap(s.cfg.Vault.Root, path, prior); herr != nil {
			// Log via stderr; the API itself doesn't have a hook
			// for structured logging, but the ops user (the user)
			// runs granit attached to a terminal so a fmt to
			// stderr is visible.
			fmt.Fprintf(os.Stderr, "history snap %s: %v\n", path, herr)
		}
	}
	if err := atomicio.WriteNote(abs, content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Notify the autocommit manager — debounces internally and
	// no-ops when disabled or vault isn't a git repo, so this is
	// always a cheap call regardless of the user's setup.
	if s.autocommit != nil {
		s.autocommit.Notify(path)
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()
	n := s.cfg.Vault.GetNote(path)
	if n == nil {
		writeError(w, http.StatusInternalServerError, "post-write: note not found")
		return
	}
	w.Header().Set("ETag", s.etagFor(n.ModTime, n.Size))
	writeJSON(w, http.StatusOK, noteFull{
		Path:        n.RelPath,
		Title:       n.Title,
		ModTime:     n.ModTime,
		Size:        n.Size,
		Frontmatter: n.Frontmatter,
		Body:        stripFrontmatterBody(n.Content),
		Links:       n.Links,
		Tags:        tagsFor(n),
	})
}

type createNoteBody struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	Body        string                 `json:"body"`
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var b createNoteBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if b.Path == "" || strings.Contains(b.Path, "..") || strings.HasPrefix(b.Path, "/") {
		writeError(w, http.StatusBadRequest, "missing or invalid path")
		return
	}
	if !strings.HasSuffix(strings.ToLower(b.Path), ".md") {
		b.Path += ".md"
	}
	if existing := s.cfg.Vault.GetNote(b.Path); existing != nil {
		writeError(w, http.StatusConflict, "note already exists")
		return
	}
	content, err := serializeNote(b.Frontmatter, b.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(b.Path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := atomicio.WriteNote(abs, content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()
	n := s.cfg.Vault.GetNote(b.Path)
	if n == nil {
		writeError(w, http.StatusInternalServerError, "post-write: note not found")
		return
	}
	w.Header().Set("ETag", s.etagFor(n.ModTime, n.Size))
	writeJSON(w, http.StatusCreated, noteFull{
		Path:        n.RelPath,
		Title:       n.Title,
		ModTime:     n.ModTime,
		Size:        n.Size,
		Frontmatter: n.Frontmatter,
		Body:        stripFrontmatterBody(n.Content),
		Links:       n.Links,
		Tags:        tagsFor(n),
	})
}

func (s *Server) handleGetLinks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	n := s.cfg.Vault.GetNote(path)
	if n == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	bl := []map[string]string{}
	for _, src := range n.Backlinks {
		if other := s.cfg.Vault.GetNote(src); other != nil {
			bl = append(bl, map[string]string{"path": other.RelPath, "title": other.Title})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"outgoing":  n.Links,
		"backlinks": bl,
	})
}

// stripFrontmatterBody removes the leading `---` YAML block if present,
// matching granit's vault parser.
func stripFrontmatterBody(content string) string {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return content
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(content, "---\n"), "---\r\n")
	for i := 0; i < len(rest); {
		nl := strings.IndexByte(rest[i:], '\n')
		var line string
		if nl < 0 {
			line = rest[i:]
			i = len(rest)
		} else {
			line = rest[i : i+nl]
			i += nl + 1
		}
		if strings.TrimRight(line, "\r") == "---" {
			if i < len(rest) {
				return rest[i:]
			}
			return ""
		}
	}
	return content
}

// serializeNote produces the markdown file content. If frontmatter is nil
// and body has no leading `---` block, returns body as-is. Otherwise serializes
// frontmatter as YAML before the body. Body's existing frontmatter is replaced.
func serializeNote(fm map[string]interface{}, body string) (string, error) {
	plain := stripFrontmatterBody(body)
	if fm == nil {
		return plain, nil
	}
	data, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	yamlText := strings.TrimRight(string(data), "\n")
	return "---\n" + yamlText + "\n---\n" + plain, nil
}

var inlineTagRe = mustCompile(`(^|\s)#([\p{L}\p{N}_/-]+)`)

// handleDeleteNote removes a note from the vault. Hard delete — no
// trash folder yet (a future enhancement). The same path-safety
// shape used by handlePutNote / handleGetFile: refuse absolute paths,
// any `..` component, and ensure the cleaned absolute lives under
// the vault root. Returns 204 on success, 404 when missing, 400 on
// path violation.
func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	abs := filepath.Clean(filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(path)))
	rootClean := filepath.Clean(s.cfg.Vault.Root)
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "path escapes vault")
		return
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Remove(abs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// rescanMu protects the vault index AND the search index — they're
	// not internally thread-safe and the runWatcher holds this same
	// lock around its own scan/reload/search.Remove. Without it, a
	// concurrent search request can fatal-panic on a map read/write
	// race. Discovered the hard way; fix is to mirror the watcher's
	// shape.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.search.Remove(path)
	s.rescanMu.Unlock()
	s.hub.Broadcast(wshub.Event{Type: "note.removed", Path: path})
	w.WriteHeader(http.StatusNoContent)
}

type renameNoteBody struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// handleRenameNote moves a note from one vault-relative path to
// another. Used by the notes tab to rename or move a file without
// the user opening the editor. Path safety mirrors delete; refuses
// to overwrite an existing file at the destination.
func (s *Server) handleRenameNote(w http.ResponseWriter, r *http.Request) {
	var b renameNoteBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	from := strings.TrimPrefix(b.From, "/")
	to := strings.TrimPrefix(b.To, "/")
	if from == "" || to == "" || strings.Contains(from, "..") || strings.Contains(to, "..") ||
		strings.HasPrefix(from, "/") || strings.HasPrefix(to, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	root := filepath.Clean(s.cfg.Vault.Root)
	fromAbs := filepath.Clean(filepath.Join(root, filepath.FromSlash(from)))
	toAbs := filepath.Clean(filepath.Join(root, filepath.FromSlash(to)))
	for _, p := range []string{fromAbs, toAbs} {
		if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
			writeError(w, http.StatusBadRequest, "path escapes vault")
			return
		}
	}
	if _, err := os.Stat(fromAbs); err != nil {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	if _, err := os.Stat(toAbs); err == nil {
		writeError(w, http.StatusConflict, "destination exists")
		return
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// rescanMu wraps vault scan + task reload + search index mutation —
	// see handleDeleteNote comment.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.search.Remove(from)
	if n := s.cfg.Vault.GetNote(to); n != nil {
		s.cfg.Vault.EnsureLoaded(to)
		s.search.Update(to, n.Content)
	}
	s.rescanMu.Unlock()
	// Rewrite every annotation tied to the old path so margin notes
	// follow the rename instead of dangling. Best-effort — a write
	// failure here doesn't roll back the rename, but we broadcast
	// a state.changed so any open editor refetches its margin column
	// and surfaces any partial state to the user.
	if n, err := annotations.RewriteNotePath(s.cfg.Vault.Root, from, to); err == nil && n > 0 {
		s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/annotations.json"})
	}
	s.hub.Broadcast(wshub.Event{Type: "note.removed", Path: from})
	s.hub.Broadcast(wshub.Event{Type: "note.changed", Path: to})
	writeJSON(w, http.StatusOK, map[string]string{"from": from, "to": to})
}

// handleListHistory returns the version history for a single note.
// GET /api/v1/notes/*?action=history is too clever; we expose this
// at GET /api/v1/notes/*\/history (chi handles the literal `/history`
// suffix below the wildcard via a sub-route, but chi doesn't support
// that out of the box for `*` routes — so we use a separate path
// prefix /api/v1/history/*  registered in server.go).
//
// Response shape:
//
//	{ "path": "foo/bar.md", "versions": [ {timestamp, size, hash}, ... ] }
//
// Newest first, capped at history.MaxVersionsListed.
func (s *Server) handleListHistory(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	versions, err := history.List(s.cfg.Vault.Root, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":     path,
		"versions": versions,
	})
}

// handleGetHistoryVersion returns the body of a single historical
// snapshot. The timestamp comes via `?ts=<stamp>` rather than a path
// segment because timestamps contain colons / dots that are awkward
// in URL paths.
//
//	GET /api/v1/history/<path>?ts=2026-05-06T12:34:56.789Z
func (s *Server) handleGetHistoryVersion(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	ts := r.URL.Query().Get("ts")
	if ts == "" {
		writeError(w, http.StatusBadRequest, "missing ts query parameter")
		return
	}
	body, err := history.Read(s.cfg.Vault.Root, path, ts)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":      path,
		"timestamp": ts,
		"body":      string(body),
	})
}

type restoreHistoryBody struct {
	Timestamp string `json:"timestamp"`
}

// handleRestoreHistoryVersion writes a chosen snapshot's content
// back to the live note path. The current live content is itself
// snapshotted first by the regular Snap-on-write path inside the
// handler, so a restore is itself reversible — restoring v3 over
// v5 leaves v5 in the history list as the most-recent pre-restore
// snapshot, and the user can restore THAT to undo the restore.
//
//	POST /api/v1/history/<path>/restore  body: {timestamp}
//
// Returns the new note state on success (mirrors PUT /notes/* shape).
func (s *Server) handleRestoreHistoryVersion(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	var b restoreHistoryBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if b.Timestamp == "" {
		writeError(w, http.StatusBadRequest, "missing timestamp")
		return
	}
	body, err := history.Read(s.cfg.Vault.Root, path, b.Timestamp)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(path))
	// Snapshot current live content before overwrite so the restore
	// is itself reversible.
	if prior, rerr := os.ReadFile(abs); rerr == nil {
		if _, herr := history.Snap(s.cfg.Vault.Root, path, prior); herr != nil {
			fmt.Fprintf(os.Stderr, "history snap on restore %s: %v\n", path, herr)
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := atomicio.WriteNote(abs, string(body)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()
	n := s.cfg.Vault.GetNote(path)
	if n == nil {
		writeError(w, http.StatusInternalServerError, "post-restore: note not found")
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "note.changed", Path: path})
	w.Header().Set("ETag", s.etagFor(n.ModTime, n.Size))
	writeJSON(w, http.StatusOK, noteFull{
		Path:        n.RelPath,
		Title:       n.Title,
		ModTime:     n.ModTime,
		Size:        n.Size,
		Frontmatter: n.Frontmatter,
		Body:        stripFrontmatterBody(n.Content),
		Links:       n.Links,
		Tags:        tagsFor(n),
	})
}
