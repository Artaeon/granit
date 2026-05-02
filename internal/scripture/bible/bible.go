// Package bible exposes the bundled World English Bible (WEB) — a
// public-domain modern-English translation — for the scripture page's
// random-passage / reader / search features.
//
// The full 66-book Protestant canon is embedded as a single ~4.5 MB JSON
// at compile time via go:embed. That keeps the binary self-contained
// (no runtime fetch, no on-disk cache to manage) and gives us O(1)
// random access without a parser at startup. We pay for it once with
// a JSON unmarshal on first call (sync.Once-guarded), then everything
// is in-memory pointer chasing.
//
// Source: https://ebible.org/web/  (Public Domain — no attribution
// required, but we ship the upstream COPYRIGHT.html alongside the JSON
// for completeness.)
package bible

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

//go:embed web.json
var webJSON []byte

// Verse is a single verse: number within its chapter + the WEB text.
type Verse struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

// Chapter holds an ordered list of verses for one chapter.
type Chapter struct {
	Number int     `json:"number"`
	Verses []Verse `json:"verses"`
}

// Book is one canonical book of the Bible.
type Book struct {
	Code      string    `json:"code"`      // 3-letter USFM code, e.g. "GEN", "JHN"
	Name      string    `json:"name"`      // display name, e.g. "Genesis", "1 Corinthians"
	Testament string    `json:"testament"` // "OT" or "NT"
	Chapters  []Chapter `json:"chapters"`
}

// Bible is the root document.
type Bible struct {
	Name         string `json:"name"`         // "World English Bible"
	Abbreviation string `json:"abbreviation"` // "WEB"
	License      string `json:"license"`      // "Public Domain"
	Source       string `json:"source"`
	Books        []Book `json:"books"`
}

var (
	loadOnce sync.Once
	loaded   *Bible
	loadErr  error
)

// Load returns the parsed embedded Bible. Idempotent + concurrency-safe;
// the first call does the JSON unmarshal and every subsequent call
// returns the cached pointer.
func Load() (*Bible, error) {
	loadOnce.Do(func() {
		var b Bible
		if err := json.Unmarshal(webJSON, &b); err != nil {
			loadErr = fmt.Errorf("decode bible: %w", err)
			return
		}
		if len(b.Books) == 0 {
			loadErr = errors.New("bible: no books loaded")
			return
		}
		loaded = &b
	})
	return loaded, loadErr
}

// BookSummary is a slim {code, name, chapters} record for the books-list
// endpoint. We don't ship verse counts because the front-end only needs
// chapter ranges to render the picker.
type BookSummary struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Testament string `json:"testament"`
	Chapters  int    `json:"chapters"`
}

// Books returns one summary per canonical book, in canonical order.
func Books() ([]BookSummary, error) {
	b, err := Load()
	if err != nil {
		return nil, err
	}
	out := make([]BookSummary, len(b.Books))
	for i, bk := range b.Books {
		out[i] = BookSummary{
			Code:      bk.Code,
			Name:      bk.Name,
			Testament: bk.Testament,
			Chapters:  len(bk.Chapters),
		}
	}
	return out, nil
}

// FindBook resolves a book by USFM code (case-insensitive, e.g. "JHN")
// or by display name (e.g. "John", "1 Corinthians", "1corinthians").
// Returns nil if not found.
func FindBook(query string) *Book {
	b, err := Load()
	if err != nil {
		return nil
	}
	q := normalizeBookKey(query)
	if q == "" {
		return nil
	}
	for i := range b.Books {
		if normalizeBookKey(b.Books[i].Code) == q ||
			normalizeBookKey(b.Books[i].Name) == q {
			return &b.Books[i]
		}
	}
	// Common aliases — "psalm" → "psalms", "song of songs" → "song of solomon", etc.
	switch q {
	case "psalm":
		return FindBook("Psalms")
	case "songofsongs", "canticles":
		return FindBook("Song of Solomon")
	case "revelations":
		return FindBook("Revelation")
	}
	return nil
}

// normalizeBookKey lowercases + strips whitespace and punctuation so
// "1 Corinthians", "1Corinthians", "1corinthians", and "1 CORINTHIANS"
// all collapse to the same key.
func normalizeBookKey(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// GetChapter returns a chapter by 1-indexed number, or nil if out of range.
func (b *Book) GetChapter(n int) *Chapter {
	if n < 1 {
		return nil
	}
	for i := range b.Chapters {
		if b.Chapters[i].Number == n {
			return &b.Chapters[i]
		}
	}
	return nil
}

// Passage is a contiguous run of verses from one chapter, plus a
// human-readable reference like "Proverbs 3:5-8". The reference always
// targets a single chapter; we don't span chapter boundaries because
// that complicates citation rendering and the random algorithm clamps
// to chapter ends anyway.
type Passage struct {
	Book      string  `json:"book"`      // "Proverbs"
	BookCode  string  `json:"bookCode"`  // "PRO"
	Chapter   int     `json:"chapter"`   // 3
	StartV    int     `json:"startV"`    // 5
	EndV      int     `json:"endV"`      // 8
	Reference string  `json:"reference"` // "Proverbs 3:5-8"
	Verses    []Verse `json:"verses"`
}

// RandomOptions controls Random()'s sampling.
type RandomOptions struct {
	Length    int    // verses per passage; clamped to [1, 10]; default 4
	Book      string // optional filter — book name/code
	Testament string // optional filter — "OT" / "NT" (ignored if Book set)
	RNG       *rand.Rand
}

// Random returns a passage of `Length` verses chosen uniformly across
// all eligible verses (i.e. weighted by chapter length, NOT by book) so
// long books like Psalms aren't under-represented. The starting verse
// is uniform within its chapter; if it's near the end we shrink the
// passage rather than spilling into the next chapter.
//
// The book/testament filters narrow the eligibility pool first.
func Random(opts RandomOptions) (*Passage, error) {
	b, err := Load()
	if err != nil {
		return nil, err
	}
	length := opts.Length
	if length < 1 {
		length = 4
	}
	if length > 10 {
		length = 10
	}

	// Build the candidate book list according to filters.
	var candidates []*Book
	if opts.Book != "" {
		bk := FindBook(opts.Book)
		if bk == nil {
			return nil, fmt.Errorf("book not found: %q", opts.Book)
		}
		candidates = []*Book{bk}
	} else {
		want := strings.ToUpper(opts.Testament)
		for i := range b.Books {
			if want == "" || b.Books[i].Testament == want {
				candidates = append(candidates, &b.Books[i])
			}
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("no eligible books")
	}

	// Total verse count across candidates → uniform pick weights long
	// books proportionally so we don't over-sample Obadiah.
	total := 0
	for _, bk := range candidates {
		for _, ch := range bk.Chapters {
			total += len(ch.Verses)
		}
	}
	if total == 0 {
		return nil, errors.New("no verses available")
	}

	rng := opts.RNG
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}
	pick := rng.Intn(total)

	// Locate the verse we picked + its chapter.
	for _, bk := range candidates {
		for ci := range bk.Chapters {
			ch := &bk.Chapters[ci]
			if pick < len(ch.Verses) {
				start := pick
				end := start + length
				if end > len(ch.Verses) {
					end = len(ch.Verses)
				}
				slice := ch.Verses[start:end]
				return &Passage{
					Book:      bk.Name,
					BookCode:  bk.Code,
					Chapter:   ch.Number,
					StartV:    slice[0].N,
					EndV:      slice[len(slice)-1].N,
					Reference: formatRef(bk.Name, ch.Number, slice[0].N, slice[len(slice)-1].N),
					Verses:    slice,
				}, nil
			}
			pick -= len(ch.Verses)
		}
	}
	// Shouldn't reach here — pick was modulo total.
	return nil, errors.New("internal: failed to locate random verse")
}

// formatRef builds "Book C:V" / "Book C:V-W" — single-verse passages
// drop the dash so cites look natural.
func formatRef(name string, ch, sv, ev int) string {
	if sv == ev {
		return fmt.Sprintf("%s %d:%d", name, ch, sv)
	}
	return fmt.Sprintf("%s %d:%d-%d", name, ch, sv, ev)
}

// SearchHit is one search result.
type SearchHit struct {
	Book      string `json:"book"`
	BookCode  string `json:"bookCode"`
	Chapter   int    `json:"chapter"`
	Verse     int    `json:"verse"`
	Text      string `json:"text"`
	Reference string `json:"reference"` // "Proverbs 3:5"
}

// Search runs a case-insensitive substring scan over every verse and
// returns up to `limit` hits in canonical order. Empty/whitespace
// queries return zero results without error. Linear scan over the
// whole 4MB corpus runs in single-digit milliseconds, so we don't
// bother with an index.
func Search(query string, limit int) ([]SearchHit, error) {
	b, err := Load()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	var hits []SearchHit
	for bi := range b.Books {
		bk := &b.Books[bi]
		for ci := range bk.Chapters {
			ch := &bk.Chapters[ci]
			for vi := range ch.Verses {
				v := &ch.Verses[vi]
				if strings.Contains(strings.ToLower(v.Text), q) {
					hits = append(hits, SearchHit{
						Book:      bk.Name,
						BookCode:  bk.Code,
						Chapter:   ch.Number,
						Verse:     v.N,
						Text:      v.Text,
						Reference: fmt.Sprintf("%s %d:%d", bk.Name, ch.Number, v.N),
					})
					if len(hits) >= limit {
						return hits, nil
					}
				}
			}
		}
	}
	return hits, nil
}
