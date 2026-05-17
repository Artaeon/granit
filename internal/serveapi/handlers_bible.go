package serveapi

import (
	"math/rand"
	"net/http"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/scripture/bible"
	"github.com/go-chi/chi/v5"
)

// handleBibleBooks returns the canonical book list with chapter counts —
// used by the web reader to populate the book/chapter picker. Cheap call;
// data is loaded once and cached for the lifetime of the process.
//
// Optional ?translation=<id> picks an alternate translation (must be
// bundled). Empty/unknown falls back to the default (WEB) — keeping
// existing single-Bible callers working unchanged.
func (s *Server) handleBibleBooks(w http.ResponseWriter, r *http.Request) {
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	b, err := bible.Get(translation)
	if err != nil {
		// Fall back to default rather than 404'ing — old callers
		// don't supply a translation and expect WEB.
		b, err = bible.Default()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	books, err := bible.Books(b.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"books": books,
		"meta": map[string]string{
			"id":           b.ID,
			"name":         b.Name,
			"abbreviation": b.Abbreviation,
			"license":      b.License,
		},
	})
}

// handleBibleChapter returns one chapter as {book, name, chapter, verses}.
// Books resolve case-insensitively by USFM code or display name (see
// bible.FindBook); chapter is a 1-indexed integer.
func (s *Server) handleBibleChapter(w http.ResponseWriter, r *http.Request) {
	bookKey := chi.URLParam(r, "book")
	chStr := chi.URLParam(r, "chapter")
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	bk := bible.FindBook(translation, bookKey)
	if bk == nil {
		writeError(w, http.StatusNotFound, "book not found: "+bookKey)
		return
	}
	chNum, err := strconv.Atoi(chStr)
	if err != nil || chNum < 1 {
		writeError(w, http.StatusBadRequest, "invalid chapter: "+chStr)
		return
	}
	ch := bk.GetChapter(chNum)
	if ch == nil {
		writeError(w, http.StatusNotFound, "chapter not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"book":      bk.Name,
		"bookCode":  bk.Code,
		"testament": bk.Testament,
		"chapter":   ch.Number,
		"verses":    ch.Verses,
		"chapters":  len(bk.Chapters), // so the UI knows when "next" runs out
	})
}

// handleBibleRandom returns a random N-verse passage. Optional query:
//
//	?length=N         clamp [1, 10], default 4
//	?book=Proverbs    restrict to one book (or USFM code, e.g. "PRO")
//	?testament=OT|NT  restrict to one testament (ignored if book set)
//	?seed=<int>       deterministic — handy for testing & reproducible cards
func (s *Server) handleBibleRandom(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := bible.RandomOptions{
		Book:        strings.TrimSpace(q.Get("book")),
		Testament:   strings.TrimSpace(q.Get("testament")),
		Translation: strings.TrimSpace(q.Get("translation")),
	}
	if l := q.Get("length"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			opts.Length = n
		}
	}
	if seedStr := q.Get("seed"); seedStr != "" {
		if seed, err := strconv.ParseInt(seedStr, 10, 64); err == nil {
			opts.RNG = rand.New(rand.NewSource(seed))
		}
	}
	p, err := bible.Random(opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleBibleSearch returns up to 50 verse hits matching a substring
// query (case-insensitive). Empty queries return an empty list rather
// than an error so the UI can debounce without try/catch noise.
func (s *Server) handleBibleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	hits, err := bible.Search(translation, query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hits":  hits,
		"total": len(hits),
		"query": query,
	})
}
