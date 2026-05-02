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
func (s *Server) handleBibleBooks(w http.ResponseWriter, r *http.Request) {
	books, err := bible.Books()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"books": books,
		"meta": map[string]string{
			"name":         "World English Bible",
			"abbreviation": "WEB",
			"license":      "Public Domain",
		},
	})
}

// handleBibleChapter returns one chapter as {book, name, chapter, verses}.
// Books resolve case-insensitively by USFM code or display name (see
// bible.FindBook); chapter is a 1-indexed integer.
func (s *Server) handleBibleChapter(w http.ResponseWriter, r *http.Request) {
	bookKey := chi.URLParam(r, "book")
	chStr := chi.URLParam(r, "chapter")
	bk := bible.FindBook(bookKey)
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
		Book:      strings.TrimSpace(q.Get("book")),
		Testament: strings.TrimSpace(q.Get("testament")),
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
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	hits, err := bible.Search(query, limit)
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
