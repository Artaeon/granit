package serveapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/vault"
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
	if err := atomicio.WriteNote(abs, content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
