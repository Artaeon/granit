// Strong's-tagged bible — same canon as web.json but with each verse
// expanded into a word-level array carrying the underlying Strong's
// code for every translatable token. This is the source the reader
// uses to render tappable words; pair it with strongs.go to resolve
// each tap into a word-study card.
//
// Fetched by scripts/fetch-strongs.sh (typically a public-domain
// KJV-Strong's) and dropped at tagged_kjv.json next to web.json. Like
// strongs.json this file is NOT checked in (~30MB) — we ship a
// one-byte placeholder ("{}") so go:embed succeeds and the loader
// reports "not bundled" gracefully when no real data is present.
package bible

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

//go:embed tagged_kjv.json
var taggedJSON []byte

// TaggedWord is one renderable token. Text is what the reader prints;
// Strongs is the lookup key into the lexicon (may be empty for
// untagged glue words like "the", "and", or punctuation depending on
// the upstream dataset's granularity).
type TaggedWord struct {
	Text    string `json:"text"`
	Strongs string `json:"strongs,omitempty"`
}

// TaggedVerse is one verse decomposed into tagged words. We retain the
// verse number so the client can render a specific verse without
// scanning the whole chapter.
type TaggedVerse struct {
	N     int          `json:"n"`
	Words []TaggedWord `json:"words"`
}

// TaggedChapter mirrors the bible.Chapter shape but with tagged
// verses. Chapters are 1-indexed via Number for consistency with the
// untagged bible.
type taggedChapter struct {
	Number int           `json:"number"`
	Verses []TaggedVerse `json:"verses"`
}

// TaggedBook is one canonical book in the tagged dataset.
type TaggedBook struct {
	Code     string          `json:"code"` // 3-letter USFM, matches bible.Book.Code
	Name     string          `json:"name"`
	Chapters []taggedChapter `json:"chapters"`
}

// TaggedBible is the root document for the tagged bible.
type TaggedBible struct {
	Name         string       `json:"name"`
	Abbreviation string       `json:"abbreviation"`
	License      string       `json:"license"`
	Source       string       `json:"source"`
	Books        []TaggedBook `json:"books"`
}

var (
	taggedOnce    sync.Once
	taggedLoaded  *TaggedBible
	taggedPresent bool // true iff a real tagged bible was bundled
	taggedErr     error
)

// LoadTagged returns the parsed tagged bible, or (nil, nil) when no
// dataset is bundled — same graceful-degradation contract as
// LoadStrongs. Idempotent + concurrency-safe.
func LoadTagged() (*TaggedBible, error) {
	taggedOnce.Do(func() {
		trimmed := strings.TrimSpace(string(taggedJSON))
		if trimmed == "" || trimmed == "{}" {
			return
		}
		var t TaggedBible
		if err := json.Unmarshal(taggedJSON, &t); err != nil {
			taggedErr = fmt.Errorf("decode tagged bible: %w", err)
			return
		}
		if len(t.Books) == 0 {
			return
		}
		taggedLoaded = &t
		taggedPresent = true
	})
	return taggedLoaded, taggedErr
}

// TaggedBundled reports whether a real tagged bible (not the
// placeholder) was compiled into the binary.
func TaggedBundled() bool {
	_, _ = LoadTagged()
	return taggedPresent
}

// TaggedChapter returns the tagged verses for one chapter, resolving
// the book the same way bible.FindBook does (USFM code or display
// name, case-insensitive). Returns an error when the tagged bible
// isn't bundled, the book is unknown, or the chapter is out of range.
func TaggedChapter(bookQuery string, chapter int) ([]TaggedVerse, error) {
	t, err := LoadTagged()
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errors.New("tagged bible not bundled")
	}
	if chapter < 1 {
		return nil, fmt.Errorf("invalid chapter: %d", chapter)
	}
	q := normalizeBookKey(bookQuery)
	if q == "" {
		return nil, errors.New("book required")
	}
	for bi := range t.Books {
		bk := &t.Books[bi]
		if normalizeBookKey(bk.Code) != q && normalizeBookKey(bk.Name) != q {
			continue
		}
		for ci := range bk.Chapters {
			ch := &bk.Chapters[ci]
			if ch.Number == chapter {
				return ch.Verses, nil
			}
		}
		return nil, fmt.Errorf("chapter %d not found in %s", chapter, bk.Code)
	}
	return nil, fmt.Errorf("book not found: %q", bookQuery)
}
