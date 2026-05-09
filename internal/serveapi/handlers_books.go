// Package serveapi — handlers for /api/v1/books.
//
// Surface:
//   GET    /books                            list shelf summaries (with merged progress)
//   GET    /books/{id}                       full reader payload (spine + TOC)
//   GET    /books/{id}/chapter/{idx}         chapter HTML
//   GET    /books/{id}/cover                 cover image (binary)
//   GET    /books/{id}/asset?path=...        passthrough for chapter-referenced assets
//   GET    /books/{id}/sidecar               progress + highlights + bookmarks
//   PUT    /books/{id}/progress              save reading progress
//   POST   /books/{id}/highlights            create highlight
//   PATCH  /books/{id}/highlights/{hid}      update note / color
//   DELETE /books/{id}/highlights/{hid}      remove highlight
//   POST   /books/{id}/bookmarks             create bookmark
//   DELETE /books/{id}/bookmarks/{bid}       remove bookmark
//
// Books open the EPUB per request — fast enough for shelf-sized
// libraries (the zip toc is small) and avoids the cache-invalidation
// problem we'd inherit from a server-side EPUB cache. If shelves
// grow past ~hundreds of books a memo on Scan() is the right fix,
// not a cache on Open().
package serveapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/artaeon/granit/internal/books"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	all, err := books.Scan(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if all == nil {
		all = []books.Summary{}
	}
	// Merge minimal progress so the shelf can render "X% read"
	// without N round-trips. We don't ship the full sidecar here
	// (highlights would balloon the payload) — just the fields
	// the shelf card needs.
	type shelfRow struct {
		books.Summary
		LastReadAt      string  `json:"lastReadAt,omitempty"`
		FurthestChapter int     `json:"furthestChapter"`
		ProgressPct     float64 `json:"progressPct"`
		// TotalChapters lets the UI render "ch 7 of 22" — we'd
		// otherwise need a per-row open() to compute it.
		TotalChapters int `json:"totalChapters"`
	}
	out := make([]shelfRow, 0, len(all))
	for _, sum := range all {
		row := shelfRow{Summary: sum}
		// Cheap-ish: open just to get spine length. EPUB toc is
		// small. If this becomes a hot path we'd cache via
		// content-hash; not worth it for v1.
		if e, _, err := books.FindByID(s.cfg.Vault.Root, sum.ID); err == nil {
			_ = e
			if d, ee, derr := books.LoadDetail(s.cfg.Vault.Root, sum.ID); derr == nil {
				row.TotalChapters = len(d.Chapters)
				ee.Close()
			}
		}
		if sc, err := books.LoadSidecar(s.cfg.Vault.Root, sum.ID); err == nil && sc != nil {
			row.LastReadAt = sc.Progress.LastReadAt
			row.FurthestChapter = sc.Progress.FurthestChapter
			if row.TotalChapters > 0 {
				// (furthestChapter+1)/total — chapter 0 already
				// implies "started reading", so +1 keeps the bar
				// from showing 0% for an opened book.
				pct := float64(sc.Progress.FurthestChapter+1) * 100.0 / float64(row.TotalChapters)
				if pct > 100 {
					pct = 100
				}
				row.ProgressPct = pct
			}
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"books": out, "total": len(out)})
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, e, err := books.LoadDetail(s.cfg.Vault.Root, id)
	if err != nil {
		if errors.Is(err, errors.New("file does not exist")) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer e.Close()
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleGetBookChapter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	idxStr := chi.URLParam(r, "idx")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chapter index")
		return
	}
	_, abs, err := books.FindByID(s.cfg.Vault.Root, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	e, err := books.Open(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer e.Close()
	// Asset prefix points back at our passthrough so chapter HTML
	// can resolve relative images / CSS through the same auth.
	prefix := fmt.Sprintf("/api/v1/books/%s/asset", id)
	htmlBody, err := e.Chapter(idx, prefix)
	if err != nil {
		if errors.Is(err, books.ErrInvalidChapter) {
			writeError(w, http.StatusNotFound, "chapter out of range")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"index": idx,
		"html":  htmlBody,
	})
}

func (s *Server) handleGetBookCover(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, abs, err := books.FindByID(s.cfg.Vault.Root, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	e, err := books.Open(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer e.Close()
	data, mediaType, err := e.CoverBytes()
	if err != nil {
		if errors.Is(err, books.ErrNoCover) {
			writeError(w, http.StatusNotFound, "no cover")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mediaType)
	// Covers are immutable per book id. Cache aggressively — the
	// shelf re-renders cheap and this saves a re-decode every load.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleGetBookAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rel := r.URL.Query().Get("path")
	if rel == "" {
		// chapter HTML rewrites refs to .../asset/<resolved-path>;
		// chi captures the wildcard segment, so let path= or the
		// trailing path fragment work either way.
		rel = chi.URLParam(r, "*")
	}
	if rel == "" {
		writeError(w, http.StatusBadRequest, "asset path required")
		return
	}
	_, abs, err := books.FindByID(s.cfg.Vault.Root, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	e, err := books.Open(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer e.Close()
	data, mediaType, err := e.Asset(rel)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleGetBookSidecar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sc, err := books.LoadSidecar(s.cfg.Vault.Root, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) handlePutBookProgress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p books.Progress
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := books.SaveProgress(s.cfg.Vault.Root, id, p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastBook(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCreateBookHighlight(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var h books.Highlight
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if h.Text == "" {
		writeError(w, http.StatusBadRequest, "highlight text required")
		return
	}
	out, err := books.AddHighlight(s.cfg.Vault.Root, id, h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastBook(id)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePatchBookHighlight(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	hid := chi.URLParam(r, "hid")
	var body struct {
		Note  string `json:"note"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := books.PatchHighlight(s.cfg.Vault.Root, id, hid, body.Note, body.Color)
	if err != nil {
		if errors.Is(err, books.ErrNotFound) {
			writeError(w, http.StatusNotFound, "highlight not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastBook(id)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteBookHighlight(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	hid := chi.URLParam(r, "hid")
	if err := books.DeleteHighlight(s.cfg.Vault.Root, id, hid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastBook(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCreateBookBookmark(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b books.Bookmark
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := books.AddBookmark(s.cfg.Vault.Root, id, b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastBook(id)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteBookBookmark(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bid := chi.URLParam(r, "bid")
	if err := books.DeleteBookmark(s.cfg.Vault.Root, id, bid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastBook(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// bcastBook fans out a state.changed event so other open tabs can
// reload the shelf / sidecar after a write. Path is the per-book
// sidecar so the existing state.changed routing model works.
func (s *Server) bcastBook(id string) {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{
		Type: "state.changed",
		Path: ".granit/books/" + id + ".json",
	})
}
