package serveapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/scripture/bible"
	"github.com/go-chi/chi/v5"
)

// handleStrongsStatus reports whether the Strong's lexicon and the
// tagged bible were compiled into the binary. The web reader hits this
// once on mount so it can decide whether to render tappable words at
// all — without the tagged bible there's nothing to tap, and without
// the lexicon every tap would be a dead end. Cheap call; both loaders
// are sync.Once-guarded.
func (s *Server) handleStrongsStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"lexicon": bible.StrongsBundled(),
		"tagged":  bible.TaggedBundled(),
	})
}

// handleStrongsEntry returns one lexicon entry by Strong's code
// (e.g. /api/v1/bible/strongs/G1722). Case-insensitive lookup; 404s
// when the code is missing OR the lexicon isn't bundled — the client
// distinguishes those cases via the /status endpoint.
func (s *Server) handleStrongsEntry(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(chi.URLParam(r, "code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}
	entry, ok := bible.LookupStrong(code)
	if !ok {
		writeError(w, http.StatusNotFound, "strongs code not found: "+code)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// handleTaggedChapter returns the Strong's-tagged words for a single
// chapter — book + chapter come in as query params (?book=JHN&chapter=3)
// because chi can't easily wedge them into the existing /bible/{book}/{chapter}
// path without conflicting with handleBibleChapter. 404 when the
// tagged bible isn't bundled OR the requested chapter is missing; the
// client checks /status to disambiguate.
func (s *Server) handleTaggedChapter(w http.ResponseWriter, r *http.Request) {
	book := strings.TrimSpace(r.URL.Query().Get("book"))
	chStr := strings.TrimSpace(r.URL.Query().Get("chapter"))
	if book == "" || chStr == "" {
		writeError(w, http.StatusBadRequest, "book and chapter required")
		return
	}
	ch, err := strconv.Atoi(chStr)
	if err != nil || ch < 1 {
		writeError(w, http.StatusBadRequest, "invalid chapter: "+chStr)
		return
	}
	verses, err := bible.TaggedChapter(book, ch)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"book":    book,
		"chapter": ch,
		"verses":  verses,
	})
}
