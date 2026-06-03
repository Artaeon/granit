package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// backlinkWikiRe matches a wikilink span and captures the target text
// inside the brackets (alias text after | is excluded — we only care
// about the link target, not the display label). Mirrors the regex in
// internal/vault/parser.go; duplicated here to avoid exporting a
// package-private symbol just for the backlinks handler. Tweak both
// in lockstep if the link syntax ever changes.
var backlinkWikiRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

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
	// Reflow margin annotations so each anchored card tracks the
	// post-edit line of its passage. AnchorText was snapshotted at
	// create time; this scan finds the line whose content best
	// matches the anchor and patches LineNum. No-op on notes with
	// zero annotations (cheap early-exit inside Reflow). Failure to
	// reflow is logged but doesn't fail the write — the worst case
	// is the annotation card drifts until next save.
	if reflowed, rerr := annotations.Reflow(s.cfg.Vault.Root, path, b.Body); rerr == nil {
		if reflowed > 0 {
			s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: ".granit/annotations.json"})
		}
	} else {
		fmt.Fprintf(os.Stderr, "annotations reflow %s: %v\n", path, rerr)
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
	// Opt-in body snippets — used by the inline-AI menu to feed the
	// model actual content from linked notes instead of just titles.
	// Off by default so existing callers (BacklinksPanel) keep their
	// cheap response shape.
	withBodies := r.URL.Query().Get("bodies") == "1"
	// Each backlink entry can carry multiple contexts: one per wikilink
	// occurrence in the source note's body. {"line", "snippet"} so the
	// editor's panel can show "...mentioned at line 47: 'see [[X]] for
	// background...'" instead of just the source title. We build a
	// title→relpath + basename→relpath map once here and reuse it inside
	// findBacklinkContexts so the resolver doesn't re-walk the vault
	// per source note (n×N → n+N).
	byTitle, byBase := s.buildLinkResolverMaps()
	type bcontext struct {
		Line    int    `json:"line"`
		Snippet string `json:"snippet"`
	}
	bl := []map[string]any{}
	for _, src := range n.Backlinks {
		other := s.cfg.Vault.GetNote(src)
		if other == nil {
			continue
		}
		entry := map[string]any{"path": other.RelPath, "title": other.Title}
		// Contexts: scan the source body for wikilinks resolving to the
		// target path. Cap at 5 per source so a hub-style note that
		// mentions the target 30 times doesn't bloat the response.
		if ctxs := findBacklinkContexts(other.Content, n.RelPath, byTitle, byBase, 5); len(ctxs) > 0 {
			out := make([]bcontext, len(ctxs))
			for i, c := range ctxs {
				out[i] = bcontext{Line: c.Line, Snippet: c.Snippet}
			}
			entry["contexts"] = out
		}
		if withBodies {
			entry["snippet"] = noteSnippet(other.Content, 400)
		}
		bl = append(bl, entry)
	}
	// Outgoing: when bodies requested, return rich entries shaped like
	// the backlink list so the client uses one code path for both.
	// Legacy mode still returns the bare string list of wikilink titles
	// for BacklinksPanel compatibility.
	if withBodies {
		// One-pass title→note map. O(N) over vault notes, but only on
		// AI-menu open with the linked-notes toggle on — not a hot path.
		// Built once per request rather than per-link to avoid N×N.
		byTitle := make(map[string]*vault.Note, s.cfg.Vault.NoteCount())
		for _, note := range s.cfg.Vault.SnapshotNotes() {
			byTitle[note.Title] = note
		}
		out := []map[string]string{}
		for _, title := range n.Links {
			other := byTitle[title]
			if other == nil {
				out = append(out, map[string]string{"title": title})
				continue
			}
			out = append(out, map[string]string{
				"path":    other.RelPath,
				"title":   other.Title,
				"snippet": noteSnippet(other.Content, 400),
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"outgoing":  out,
			"backlinks": bl,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"outgoing":  n.Links,
		"backlinks": bl,
	})
}

// backlinkContext is the per-mention data the editor's backlinks
// panel renders under each source note: the 1-indexed line number
// where the wikilink appears + a short snippet centred on it.
type backlinkContext struct {
	Line    int
	Snippet string
}

// buildLinkResolverMaps walks the vault once and returns the two
// maps a wikilink resolver needs: title→relpath (the common case,
// since most links are by note title) and basename→relpath
// (Obsidian's shortest-path fallback for when no title match). Built
// up-front by handleGetLinks so the per-source context scan doesn't
// rebuild them for each backlink. Cheap on a 5k-note vault — O(N)
// allocation, no I/O.
func (s *Server) buildLinkResolverMaps() (byTitle, byBase map[string]string) {
	notes := s.cfg.Vault.SnapshotNotes()
	byTitle = make(map[string]string, len(notes))
	byBase = make(map[string]string, len(notes))
	for relPath, note := range notes {
		base := filepath.Base(relPath)
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		// First occurrence wins — matches vault.Index.Build's resolution
		// rule so the API and the UI agree on which path a duplicate
		// title resolves to.
		if _, ok := byBase[noExt]; !ok {
			byBase[noExt] = relPath
		}
		if note.Title != "" {
			if _, ok := byTitle[note.Title]; !ok {
				byTitle[note.Title] = relPath
			}
		}
	}
	return byTitle, byBase
}

// findBacklinkContexts scans `sourceContent` for every wikilink span
// that resolves to `targetRelPath` and returns the surrounding
// context for each match — line number + ~120-char inline snippet
// centred on the link. Capped at `cap` matches because a hub-style
// note can mention the same target 30+ times and the panel only
// needs the first handful to anchor the user's memory.
//
// Resolution mirrors the rules in internal/vault/index.go:
//
//   1. byTitle map (most common — wikilinks are usually by title)
//   2. byBase map (Obsidian's basename-shortest-path fallback)
//   3. literal path with .md suffix (for `[[folder/note.md]]`)
//   4. literal path without .md if the source already wrote it (for
//      `[[folder/note]]`)
//
// Anchor fragments (`[[Note#Heading]]`) are stripped before
// resolution so the match still counts as a link to Note.
func findBacklinkContexts(sourceContent, targetRelPath string, byTitle, byBase map[string]string, cap int) []backlinkContext {
	if sourceContent == "" || targetRelPath == "" || cap <= 0 {
		return nil
	}
	matches := backlinkWikiRe.FindAllStringSubmatchIndex(sourceContent, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]backlinkContext, 0, len(matches))
	for _, m := range matches {
		if len(out) >= cap {
			break
		}
		// m[0..1] = full `[[…]]` span; m[2..3] = the captured target
		// (text inside the brackets, alias stripped by the regex).
		if len(m) < 4 {
			continue
		}
		link := strings.TrimSpace(sourceContent[m[2]:m[3]])
		// Strip anchor fragment — `[[Note#Section]]` still backlinks
		// to Note, the section is just a deep-link hint.
		if i := strings.Index(link, "#"); i >= 0 {
			link = link[:i]
		}
		if link == "" {
			continue
		}
		var resolved string
		switch {
		case byTitle[link] != "":
			resolved = byTitle[link]
		case strings.HasSuffix(link, ".md"):
			// `[[folder/note.md]]` — author wrote the full path.
			resolved = link
		default:
			base := strings.TrimSuffix(filepath.Base(link), filepath.Ext(link))
			if p := byBase[base]; p != "" {
				resolved = p
			} else if !strings.HasSuffix(link, ".md") {
				// `[[folder/note]]` — extension omitted but path written
				// in full. Try with .md appended.
				resolved = link + ".md"
			}
		}
		if resolved != targetRelPath {
			continue
		}
		line := strings.Count(sourceContent[:m[0]], "\n") + 1
		out = append(out, backlinkContext{
			Line:    line,
			Snippet: makeContextSnippet(sourceContent, m[0], m[1], 60),
		})
	}
	return out
}

// makeContextSnippet returns a short inline preview of the wikilink
// match: `pad` chars before the opening bracket + the link itself +
// `pad` chars after the closing bracket. Newlines are collapsed to
// spaces so the snippet renders on one line in the panel; leading /
// trailing ellipses mark whether the snippet was clipped at either
// end. Total length ≈ 2*pad + linkLen, typically ~120-160 chars.
func makeContextSnippet(content string, matchStart, matchEnd, pad int) string {
	start := matchStart - pad
	if start < 0 {
		start = 0
	}
	end := matchEnd + pad
	if end > len(content) {
		end = len(content)
	}
	snippet := content[start:end]
	// Collapse any whitespace run (newlines, tabs, multi-spaces) to a
	// single space — the panel renders this as a one-liner, and the
	// raw newlines would otherwise break out of the row.
	snippet = strings.Join(strings.Fields(snippet), " ")
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(content) {
		snippet = snippet + "…"
	}
	return snippet
}

// noteSnippet returns a plain-text preview of a note suitable for AI
// context: frontmatter stripped, leading/trailing whitespace gone,
// truncated to `max` chars at a word boundary with an ellipsis. The
// goal is "enough for the model to know what this note is about"
// without spending tokens on the whole document.
func noteSnippet(content string, max int) string {
	body := stripFrontmatterBody(content)
	body = strings.TrimSpace(body)
	if len(body) <= max {
		return body
	}
	// Truncate at the last whitespace before max to avoid cutting a
	// word in half. Fall back to a hard cut if there's no whitespace
	// (URL-only content, etc).
	cut := strings.LastIndexAny(body[:max], " \n\t")
	if cut < max/2 {
		cut = max
	}
	return body[:cut] + "…"
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
