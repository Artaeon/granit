package books

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/oklog/ulid/v2"
)

// Sidecar holds per-book state that lives outside the EPUB:
// reading progress + the user's highlights + bookmarks. One JSON
// file per book under <vault>/.granit/books/<id>.json.
//
// Why per-book files (vs one big books.json):
//   - A user with 200 books still has fast O(1) writes — saving
//     scroll progress on chapter 3 of "1984" doesn't rewrite the
//     state for "Walden" (matters if both devices are open).
//   - Easier conflict surface for future sync: a per-book file
//     scopes the merge surface to one book.
type Sidecar struct {
	BookID     string      `json:"bookId"`
	Progress   Progress    `json:"progress"`
	Highlights []Highlight `json:"highlights,omitempty"`
	Bookmarks  []Bookmark  `json:"bookmarks,omitempty"`
}

// Progress tracks the user's current reading position. Chapter
// index + scroll fraction within the chapter (0..1). Persisted on
// every visibility-hidden + on a 2 s throttle while reading; the
// reader page consumes this to restore scroll after a refresh.
//
// LastReadAt drives the shelf's "last read" sort + the "X% read"
// progress bar (chapter index ÷ total chapters as a coarse proxy
// — true page-percent would need pre-paginated EPUB which we
// don't do in v1).
type Progress struct {
	ChapterIdx     int     `json:"chapterIdx"`
	ScrollFraction float64 `json:"scrollFraction"`
	LastReadAt     string  `json:"lastReadAt"`
	// FurthestChapter records the deepest chapter the user has
	// reached, regardless of whether they later jumped back. Useful
	// for "% complete" metrics that don't regress when the user
	// re-reads an earlier passage.
	FurthestChapter int `json:"furthestChapter"`
}

// Highlight is a saved passage. Color follows the GranitColors
// palette tokens (yellow / blue / green / pink) the editor's
// selection toolbar exposes. The CFI-like selector (chapter idx +
// quoted text + leading/trailing context) is robust enough to
// re-anchor through small chapter rewrites — full EPUB CFI is
// overkill for v1.
type Highlight struct {
	ID         string `json:"id"`
	ChapterIdx int    `json:"chapterIdx"`
	Text       string `json:"text"`
	// Prefix / Suffix are the surrounding 30-char snippets used to
	// disambiguate when the same `text` appears multiple times in
	// the chapter — same approach hypothes.is uses.
	Prefix    string `json:"prefix,omitempty"`
	Suffix    string `json:"suffix,omitempty"`
	Color     string `json:"color"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Bookmark is a "remember this spot" marker. Lighter than a
// highlight (no quoted text) — just a chapter + label. Useful for
// long books where a user wants to re-visit "the part where Levin
// goes to the field".
type Bookmark struct {
	ID         string `json:"id"`
	ChapterIdx int    `json:"chapterIdx"`
	// ScrollFraction is optional — when set, the reader can scroll
	// the user back to within the chapter. When 0, the bookmark is
	// chapter-level only.
	ScrollFraction float64 `json:"scrollFraction,omitempty"`
	Label          string  `json:"label"`
	CreatedAt      string  `json:"createdAt"`
}

// SidecarPath is the on-disk location of a book's sidecar.
func SidecarPath(vaultRoot, bookID string) string {
	return filepath.Join(vaultRoot, ".granit", "books", bookID+".json")
}

// LoadSidecar reads a book's sidecar. Returns a fresh empty
// Sidecar if the file doesn't exist (a never-opened book is
// still a valid state). Parse failure surfaces as an error so
// the caller can decide whether to overwrite or back off.
func LoadSidecar(vaultRoot, bookID string) (*Sidecar, error) {
	p := SidecarPath(vaultRoot, bookID)
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &Sidecar{BookID: bookID}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Sidecar
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.BookID == "" {
		s.BookID = bookID
	}
	return &s, nil
}

// SaveSidecar writes a sidecar atomically. Ensures the parent
// directory exists; the rest of granit's per-book state pattern
// expects .granit/books/ to materialize lazily on first write.
func SaveSidecar(vaultRoot string, s *Sidecar) error {
	if s == nil || s.BookID == "" {
		return errors.New("books: sidecar missing book id")
	}
	dir := filepath.Join(vaultRoot, ".granit", "books")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(SidecarPath(vaultRoot, s.BookID), raw)
}

// AddHighlight inserts a new highlight into the sidecar (allocating
// an id + timestamps) and saves. Returns the inserted highlight so
// the caller can echo the assigned id back to the client.
func AddHighlight(vaultRoot, bookID string, h Highlight) (Highlight, error) {
	s, err := LoadSidecar(vaultRoot, bookID)
	if err != nil {
		return Highlight{}, err
	}
	if h.ID == "" {
		h.ID = ulid.Make().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if h.CreatedAt == "" {
		h.CreatedAt = now
	}
	h.UpdatedAt = now
	s.Highlights = append(s.Highlights, h)
	sort.Slice(s.Highlights, func(i, j int) bool {
		if s.Highlights[i].ChapterIdx != s.Highlights[j].ChapterIdx {
			return s.Highlights[i].ChapterIdx < s.Highlights[j].ChapterIdx
		}
		return s.Highlights[i].CreatedAt < s.Highlights[j].CreatedAt
	})
	if err := SaveSidecar(vaultRoot, s); err != nil {
		return Highlight{}, err
	}
	return h, nil
}

// PatchHighlight updates the note / color of an existing highlight
// in place. Returns ErrNotFound if the highlight id doesn't match.
func PatchHighlight(vaultRoot, bookID, hid string, note, color string) (Highlight, error) {
	s, err := LoadSidecar(vaultRoot, bookID)
	if err != nil {
		return Highlight{}, err
	}
	for i, h := range s.Highlights {
		if h.ID == hid {
			if note != "" {
				s.Highlights[i].Note = note
			}
			if color != "" {
				s.Highlights[i].Color = color
			}
			s.Highlights[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := SaveSidecar(vaultRoot, s); err != nil {
				return Highlight{}, err
			}
			return s.Highlights[i], nil
		}
	}
	return Highlight{}, ErrNotFound
}

// DeleteHighlight removes a highlight by id. No-op (and no error)
// if the id doesn't match — the client's intent ("this highlight
// shouldn't be here") is satisfied either way.
func DeleteHighlight(vaultRoot, bookID, hid string) error {
	s, err := LoadSidecar(vaultRoot, bookID)
	if err != nil {
		return err
	}
	out := s.Highlights[:0]
	for _, h := range s.Highlights {
		if h.ID != hid {
			out = append(out, h)
		}
	}
	s.Highlights = out
	return SaveSidecar(vaultRoot, s)
}

// AddBookmark inserts a bookmark and saves.
func AddBookmark(vaultRoot, bookID string, b Bookmark) (Bookmark, error) {
	s, err := LoadSidecar(vaultRoot, bookID)
	if err != nil {
		return Bookmark{}, err
	}
	if b.ID == "" {
		b.ID = ulid.Make().String()
	}
	if b.CreatedAt == "" {
		b.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	s.Bookmarks = append(s.Bookmarks, b)
	if err := SaveSidecar(vaultRoot, s); err != nil {
		return Bookmark{}, err
	}
	return b, nil
}

// DeleteBookmark removes by id (idempotent).
func DeleteBookmark(vaultRoot, bookID, bid string) error {
	s, err := LoadSidecar(vaultRoot, bookID)
	if err != nil {
		return err
	}
	out := s.Bookmarks[:0]
	for _, b := range s.Bookmarks {
		if b.ID != bid {
			out = append(out, b)
		}
	}
	s.Bookmarks = out
	return SaveSidecar(vaultRoot, s)
}

// SaveProgress writes just the progress block, leaving highlights
// + bookmarks untouched. Called on a 2 s throttle while reading.
// Updates FurthestChapter monotonically.
func SaveProgress(vaultRoot, bookID string, p Progress) error {
	s, err := LoadSidecar(vaultRoot, bookID)
	if err != nil {
		return err
	}
	s.Progress.ChapterIdx = p.ChapterIdx
	s.Progress.ScrollFraction = p.ScrollFraction
	s.Progress.LastReadAt = time.Now().UTC().Format(time.RFC3339)
	if p.ChapterIdx > s.Progress.FurthestChapter {
		s.Progress.FurthestChapter = p.ChapterIdx
	}
	return SaveSidecar(vaultRoot, s)
}

// ErrNotFound is returned when a highlight / bookmark id doesn't
// resolve. Callers map to 404.
var ErrNotFound = errors.New("books: not found")
