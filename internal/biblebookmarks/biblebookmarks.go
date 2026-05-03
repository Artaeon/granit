// Package biblebookmarks is the canonical schema + IO for bible
// passage bookmarks stored in <vault>/.granit/bible-bookmarks.json. A
// bookmark is a saved passage (single verse or contiguous range) plus
// an optional personal note. The text is snapshotted at save time —
// even if the underlying translation file changes, the user's saved
// quote stays intact.
//
// Lifted into its own package so the TUI, web server, and any future
// agent share one source of truth on disk: the round-trip through
// either surface preserves every field.
//
// Pure data + IO only. No HTTP, no rendering.
package biblebookmarks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Bookmark is a single saved passage. VerseTo == VerseFrom for a
// single-verse bookmark; ranges are inclusive on both ends. Reference
// is denormalised (e.g. "John 3:16-17") so list views don't have to
// rebuild it from book + chapter + range.
type Bookmark struct {
	ID        string    `json:"id"`         // ULID, lowercase
	BookCode  string    `json:"bookCode"`   // USFM code, e.g. "JHN"
	Book      string    `json:"book"`       // display name, e.g. "John"
	Chapter   int       `json:"chapter"`    // 1-indexed
	VerseFrom int       `json:"verseFrom"`  // inclusive
	VerseTo   int       `json:"verseTo"`    // inclusive (== VerseFrom for single)
	Reference string    `json:"reference"`  // pre-rendered, e.g. "John 3:16"
	Text      string    `json:"text"`       // snapshot, joined with space
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StatePath returns the canonical .granit/bible-bookmarks.json path
// inside the given vault root. Centralised so handlers, tests, and
// any future TUI bookmark UI all hit the same string.
func StatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "bible-bookmarks.json")
}

// LoadAll reads every bookmark from disk. Returns nil for both
// missing and corrupt files — callers handle nil as the empty state.
// A corrupt file should never crash a bookmark list; the user can
// edit-and-fix the JSON or delete the bookmark in question.
func LoadAll(vaultRoot string) []Bookmark {
	data, err := os.ReadFile(StatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var all []Bookmark
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	return all
}

// SaveAll writes every bookmark via atomic tmp+rename so a crash
// mid-write cannot truncate the user's saved passages.
func SaveAll(vaultRoot string, bs []Bookmark) error {
	if vaultRoot == "" {
		return errors.New("biblebookmarks: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// `[]` not `null` for the empty case — matches the deadlines /
	// goals files so the web's JSON parser unwraps to an empty array
	// without an extra null-check on every read.
	if bs == nil {
		bs = []Bookmark{}
	}
	data, err := json.MarshalIndent(bs, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(StatePath(vaultRoot), data)
}

// SortNewestFirst returns a copy ordered by CreatedAt desc, with ID
// as a stable tiebreak. Most-recent-first matches how a user expects
// "saved passages" to surface — they just bookmarked it, they want
// to see it at the top.
func SortNewestFirst(bs []Bookmark) []Bookmark {
	out := make([]Bookmark, len(bs))
	copy(out, bs)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// FindByID returns the bookmark + its index, or (Bookmark{}, -1) if
// missing. Pointer-to-copy pattern matches deadlines / granitmeta —
// callers mutate then re-save the whole slice.
func FindByID(bs []Bookmark, id string) (Bookmark, int) {
	for i, b := range bs {
		if b.ID == id {
			return b, i
		}
	}
	return Bookmark{}, -1
}
