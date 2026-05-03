package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/biblebookmarks"
	"github.com/artaeon/granit/internal/wshub"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

const statePathBibleBookmarks = ".granit/bible-bookmarks.json"

func (s *Server) broadcastBibleBookmarksChanged() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathBibleBookmarks})
}

// handleListBibleBookmarks returns the saved-passages list, newest
// first. The empty-list case returns `[]`, never null.
func (s *Server) handleListBibleBookmarks(w http.ResponseWriter, r *http.Request) {
	all := biblebookmarks.LoadAll(s.cfg.Vault.Root)
	out := biblebookmarks.SortNewestFirst(all)
	if out == nil {
		out = []biblebookmarks.Bookmark{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookmarks": out,
		"total":     len(out),
	})
}

// handleCreateBibleBookmark accepts a passage from the web (book +
// chapter + verseFrom/verseTo + text snapshot + optional note) and
// appends it. The server fills in ID, timestamps, and the canonical
// reference string so the web can't disagree with itself across
// devices about how "John 3:16" is rendered.
func (s *Server) handleCreateBibleBookmark(w http.ResponseWriter, r *http.Request) {
	var b biblebookmarks.Bookmark
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(b.BookCode) == "" || b.Chapter < 1 || b.VerseFrom < 1 {
		writeError(w, http.StatusBadRequest, "bookCode, chapter, verseFrom required")
		return
	}
	if b.VerseTo < b.VerseFrom {
		b.VerseTo = b.VerseFrom
	}
	if strings.TrimSpace(b.Text) == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}
	if b.Book == "" {
		b.Book = b.BookCode
	}
	// Canonical reference — single verse vs range. The TUI reads this
	// field for list rendering, so the web cannot send a malformed one
	// (e.g. "John 3:16-16") that would surface in the TUI verbatim.
	if b.VerseFrom == b.VerseTo {
		b.Reference = canonicalRef(b.Book, b.Chapter, b.VerseFrom)
	} else {
		b.Reference = canonicalRefRange(b.Book, b.Chapter, b.VerseFrom, b.VerseTo)
	}
	if b.ID == "" {
		b.ID = strings.ToLower(ulid.Make().String())
	}
	now := time.Now().UTC()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	all := biblebookmarks.LoadAll(s.cfg.Vault.Root)
	for _, existing := range all {
		if existing.ID == b.ID {
			writeError(w, http.StatusConflict, "bookmark id already exists")
			return
		}
	}
	all = append(all, b)
	if err := biblebookmarks.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastBibleBookmarksChanged()
	writeJSON(w, http.StatusCreated, b)
}

// handlePatchBibleBookmark currently supports updating just the note
// field. Text/book/chapter/range are immutable — re-bookmark the
// passage instead. Keeps the patch surface small + the disk shape
// stable.
func (s *Server) handlePatchBibleBookmark(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch struct {
		Note *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	all := biblebookmarks.LoadAll(s.cfg.Vault.Root)
	b, idx := biblebookmarks.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	if patch.Note != nil {
		b.Note = *patch.Note
	}
	b.UpdatedAt = time.Now().UTC()
	all[idx] = b
	if err := biblebookmarks.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastBibleBookmarksChanged()
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleDeleteBibleBookmark(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	all := biblebookmarks.LoadAll(s.cfg.Vault.Root)
	_, idx := biblebookmarks.FindByID(all, id)
	if idx == -1 {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	all = append(all[:idx], all[idx+1:]...)
	if err := biblebookmarks.SaveAll(s.cfg.Vault.Root, all); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastBibleBookmarksChanged()
	w.WriteHeader(http.StatusNoContent)
}

// canonicalRef renders "John 3:16". Centralised so the server is the
// single authority on bookmark display strings; the web echoes back
// whatever the server produced rather than computing its own.
func canonicalRef(book string, ch, v int) string {
	return book + " " + itoa(ch) + ":" + itoa(v)
}
func canonicalRefRange(book string, ch, from, to int) string {
	return book + " " + itoa(ch) + ":" + itoa(from) + "-" + itoa(to)
}

// itoa: tiny strconv-free formatter for the reference builder. We
// only ever pass small positives (chapters/verses), so a hand-rolled
// loop avoids strconv import bloat without measurable cost.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
