package serveapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/objects"
)

// buildIndex constructs a fresh objects.Index from the current vault state.
// Cheap enough to do per-request given typical vault sizes (<10k notes).
func (s *Server) buildIndex() (*objects.Registry, *objects.Index) {
	r := objects.NewRegistry()
	r.LoadVaultDir(s.cfg.Vault.Root)
	b := objects.NewBuilder(r)
	for _, n := range s.cfg.Vault.SnapshotNotes() {
		fmStr := flattenFrontmatter(n.Frontmatter)
		b.Add(n.RelPath, n.Title, fmStr)
	}
	return r, b.Finalize()
}

func flattenFrontmatter(fm map[string]interface{}) map[string]string {
	if fm == nil {
		return nil
	}
	out := make(map[string]string, len(fm))
	for k, v := range fm {
		switch x := v.(type) {
		case string:
			out[k] = x
		case int, int64, float64, bool:
			out[k] = fmt.Sprint(x)
		case []interface{}:
			parts := make([]string, 0, len(x))
			for _, e := range x {
				if s, ok := e.(string); ok {
					parts = append(parts, s)
				}
			}
			out[k] = strings.Join(parts, ", ")
		}
	}
	return out
}

func (s *Server) handleListTypes(w http.ResponseWriter, r *http.Request) {
	reg, idx := s.buildIndex()
	counts := idx.CountByType()
	types := reg.All()
	out := make([]map[string]interface{}, 0, len(types))
	for _, t := range types {
		out = append(out, map[string]interface{}{
			"id":          t.ID,
			"name":        t.Name,
			"icon":        t.Icon,
			"description": t.Description,
			"folder":      t.Folder,
			"properties":  t.Properties,
			"count":       counts[t.ID],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ci, cj := out[i]["count"].(int), out[j]["count"].(int)
		if ci != cj {
			return ci > cj
		}
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"types":   out,
		"total":   len(out),
		"untyped": idx.UntypedCount(),
	})
}

func (s *Server) handleListTypeObjects(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing type id")
		return
	}
	_, idx := s.buildIndex()
	objs := idx.ByType(id)
	out := make([]map[string]interface{}, 0, len(objs))
	for _, o := range objs {
		entry := map[string]interface{}{
			"path":       o.NotePath,
			"title":      o.Title,
			"properties": o.Properties,
		}
		// Populate created/modified epoch-millis from the file system so
		// the web can offer recently-touched / newest-first sort. Stat
		// is cheap (a few hundred typed notes per type at most). Errors
		// silently leave the timestamps off — the web treats missing as
		// "0 epoch" which sorts to the bottom of a desc list, the least-
		// surprising fallback.
		if info, err := os.Stat(filepath.Join(s.cfg.Vault.Root, o.NotePath)); err == nil {
			entry["modifiedTime"] = info.ModTime().UnixMilli()
			// On Linux, ctime ≠ creation time (it's "inode change time"),
			// so we use mtime as a stable proxy for "when did the user
			// last touch this." Real created-time would need birthtime
			// (statx on Linux 4.11+) — out of scope for this slice.
			entry["createdTime"] = info.ModTime().UnixMilli()
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"typeId":  id,
		"objects": out,
		"total":   len(out),
	})
}
