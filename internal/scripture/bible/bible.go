// Package bible exposes the bundled public-domain Bible translations
// (World English Bible, and optionally ASV / KJV / BBE if their JSON
// files are dropped alongside web.json) for the scripture page's
// random-passage / reader / search / side-by-side compare features.
//
// The full 66-book Protestant canon is embedded as a single ~4.5 MB JSON
// per translation at compile time via go:embed. That keeps the binary
// self-contained (no runtime fetch, no on-disk cache to manage) and
// gives us O(1) random access without a parser at startup. We pay for
// it once with a JSON unmarshal on first call (sync.Once-guarded),
// then everything is in-memory pointer chasing.
//
// Translation files
//
// Drop `<id>.json` into this directory — id is a short lowercase
// translation code like "web", "asv", "kjv", "bbe". The schema matches
// web.json. The top-level metadata block (name / abbreviation / license /
// year) is optional; missing fields fall back to filename-derived
// defaults. New translations are picked up automatically at build time
// via the //go:embed *.json directive.
//
// Public-domain sources:
//   - WEB:  https://ebible.org/web/                  (bundled by default)
//   - ASV:  https://ebible.org/asv/                  (1901)
//   - KJV:  https://ebible.org/eng-kjv2006/          (1611/1769)
//   - BBE:  https://ebible.org/bbe/                  (1965 — Bible in Basic English)
//
// The scripts/fetch-bible-translations.sh helper documents how to
// produce additional JSON files in the right shape.
package bible

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"sort"
	"strings"
	"sync"
)

// DefaultTranslation is the translation id used when callers don't
// specify one. WEB is always bundled (web.json is in-tree), so this
// is a safe fallback for every code path.
const DefaultTranslation = "web"

//go:embed *.json
var translationFS embed.FS

// Verse is a single verse: number within its chapter + the text.
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

// Bible is the root document for one translation.
type Bible struct {
	// ID is the lowercase translation code (e.g. "web", "asv"). When
	// the JSON file doesn't supply it, it's derived from the filename.
	ID           string `json:"id"`
	Name         string `json:"name"`         // "World English Bible"
	Abbreviation string `json:"abbreviation"` // "WEB"
	License      string `json:"license"`      // "Public Domain"
	Year         int    `json:"year"`         // e.g. 1901 for ASV; 0 = unknown
	Source       string `json:"source"`
	Books        []Book `json:"books"`
}

// TranslationInfo is the slim metadata record exposed by the
// /bible/translations endpoint. The full Bible payload stays in memory;
// callers that just want to render a translation picker get this thin
// shape instead.
type TranslationInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	License      string `json:"license"`
	Year         int    `json:"year,omitempty"`
}

var (
	loadOnce sync.Once
	loadErr  error
	// loaded is the keyed-by-translation-id map. Always non-nil after
	// loadOnce.Do returns successfully; "web" is guaranteed present.
	loaded map[string]*Bible
	// loadedIDs preserves a stable display order (default first, then
	// alphabetical) so /bible/translations responses are deterministic.
	loadedIDs []string
)

// Load reads and parses every embedded translation JSON file. Returns
// the full keyed map: {translationID: *Bible}. Idempotent +
// concurrency-safe; the first call does all JSON unmarshals and every
// subsequent call returns the cached map.
//
// "web" is guaranteed present (web.json is checked in). Other
// translations appear only if a sibling JSON file has been added
// (typically via scripts/fetch-bible-translations.sh).
func Load() (map[string]*Bible, error) {
	loadOnce.Do(doLoad)
	return loaded, loadErr
}

func doLoad() {
	entries, err := fs.ReadDir(translationFS, ".")
	if err != nil {
		loadErr = fmt.Errorf("bible: read embed dir: %w", err)
		return
	}
	out := make(map[string]*Bible)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// Skip sibling JSON files that aren't plain-text translations:
		//   - tagged_*.json: Strong's-tagged datasets, loaded by
		//     tagged.go with its own schema.
		//   - strongs.json:  Strong's lexicon, loaded by strongs.go.
		// Both ship as `{}` placeholders by default, so our
		// //go:embed *.json glob would otherwise pick them up and the
		// "no books" check would error the whole load out.
		if strings.HasPrefix(e.Name(), "tagged_") || e.Name() == "strongs.json" {
			continue
		}
		raw, err := fs.ReadFile(translationFS, e.Name())
		if err != nil {
			loadErr = fmt.Errorf("bible: read %s: %w", e.Name(), err)
			return
		}
		var b Bible
		if err := json.Unmarshal(raw, &b); err != nil {
			loadErr = fmt.Errorf("bible: decode %s: %w", e.Name(), err)
			return
		}
		if len(b.Books) == 0 {
			loadErr = fmt.Errorf("bible: %s has no books", e.Name())
			return
		}
		// Filename-derived id is the source of truth when the JSON
		// doesn't carry one. We always lowercase + strip the .json
		// suffix; collisions between filename and embedded id resolve
		// in favour of the filename so the map key matches the URL.
		idFromFile := strings.TrimSuffix(strings.ToLower(e.Name()), ".json")
		if b.ID == "" {
			b.ID = idFromFile
		}
		b.ID = strings.ToLower(b.ID)
		if b.Name == "" {
			b.Name = defaultName(idFromFile)
		}
		if b.Abbreviation == "" {
			b.Abbreviation = strings.ToUpper(idFromFile)
		}
		if b.License == "" {
			b.License = "Public Domain"
		}
		out[b.ID] = &b
	}
	if _, ok := out[DefaultTranslation]; !ok {
		loadErr = errors.New("bible: default translation (web) not embedded")
		return
	}
	loaded = out
	loadedIDs = sortedIDs(out)
}

// defaultName supplies a friendly fallback name for a translation when
// the JSON doesn't include one — only used if a freshly-downloaded
// translation file forgets its metadata block.
func defaultName(id string) string {
	switch id {
	case "web":
		return "World English Bible"
	case "asv":
		return "American Standard Version"
	case "kjv":
		return "King James Version"
	case "bbe":
		return "Bible in Basic English"
	default:
		return strings.ToUpper(id)
	}
}

// sortedIDs returns the translation ids in display order: the default
// translation first, then the rest alphabetical. Stable so the picker
// chip strip doesn't reshuffle between requests.
func sortedIDs(m map[string]*Bible) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i] == DefaultTranslation {
			return true
		}
		if ids[j] == DefaultTranslation {
			return false
		}
		return ids[i] < ids[j]
	})
	return ids
}

// Default returns the default translation (WEB). Convenience helper
// for the older single-Bible call sites that don't care about
// translation selection.
func Default() (*Bible, error) {
	m, err := Load()
	if err != nil {
		return nil, err
	}
	return m[DefaultTranslation], nil
}

// Get returns the named translation. An empty id maps to the default
// translation. Returns an error if the translation isn't bundled —
// callers that want a "skip silently" semantics should check the error
// type or pre-filter their list against Translations().
func Get(id string) (*Bible, error) {
	m, err := Load()
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = DefaultTranslation
	}
	id = strings.ToLower(id)
	b, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("bible: translation %q not bundled", id)
	}
	return b, nil
}

// Translations returns metadata for every bundled translation, in
// display order. Used to populate the translation picker.
func Translations() ([]TranslationInfo, error) {
	if _, err := Load(); err != nil {
		return nil, err
	}
	out := make([]TranslationInfo, 0, len(loadedIDs))
	for _, id := range loadedIDs {
		b := loaded[id]
		out = append(out, TranslationInfo{
			ID:           b.ID,
			Name:         b.Name,
			Abbreviation: b.Abbreviation,
			License:      b.License,
			Year:         b.Year,
		})
	}
	return out, nil
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

// Books returns one summary per canonical book, in canonical order,
// for the given translation. Empty `translation` resolves to the
// default (WEB) — the book-list is identical across translations in
// practice but we still scope it so a caller reading e.g. an ASV-only
// pipeline doesn't accidentally fall back to WEB silently.
func Books(translation string) ([]BookSummary, error) {
	b, err := Get(translation)
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
// or by display name (e.g. "John", "1 Corinthians", "1corinthians") in
// the given translation. Empty `translation` resolves to the default.
// Returns nil if not found.
func FindBook(translation, query string) *Book {
	b, err := Get(translation)
	if err != nil {
		return nil
	}
	return findIn(b, query)
}

func findIn(b *Bible, query string) *Book {
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
	// Common aliases — "psalm" → "psalms", "song of songs" →
	// "song of solomon", etc. Re-enter through findIn so each alias
	// stays a single lookup against the same translation.
	switch q {
	case "psalm":
		return findIn(b, "Psalms")
	case "songofsongs", "canticles":
		return findIn(b, "Song of Solomon")
	case "revelations":
		return findIn(b, "Revelation")
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
	Book        string  `json:"book"`        // "Proverbs"
	BookCode    string  `json:"bookCode"`    // "PRO"
	Chapter     int     `json:"chapter"`     // 3
	StartV      int     `json:"startV"`      // 5
	EndV        int     `json:"endV"`        // 8
	Reference   string  `json:"reference"`   // "Proverbs 3:5-8"
	Translation string  `json:"translation"` // "web" — id of the source translation
	Verses      []Verse `json:"verses"`
}

// RandomOptions controls Random()'s sampling.
type RandomOptions struct {
	Length      int    // verses per passage; clamped to [1, 10]; default 4
	Book        string // optional filter — book name/code
	Testament   string // optional filter — "OT" / "NT" (ignored if Book set)
	Translation string // optional — translation id; default ""=WEB
	RNG         *rand.Rand
}

// Random returns a passage of `Length` verses chosen uniformly across
// all eligible verses (i.e. weighted by chapter length, NOT by book) so
// long books like Psalms aren't under-represented. The starting verse
// is uniform within its chapter; if it's near the end we shrink the
// passage rather than spilling into the next chapter.
//
// The book/testament filters narrow the eligibility pool first. The
// translation field selects the source bible; empty == WEB.
func Random(opts RandomOptions) (*Passage, error) {
	b, err := Get(opts.Translation)
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
		bk := findIn(b, opts.Book)
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
					Book:        bk.Name,
					BookCode:    bk.Code,
					Chapter:     ch.Number,
					StartV:      slice[0].N,
					EndV:        slice[len(slice)-1].N,
					Reference:   formatRef(bk.Name, ch.Number, slice[0].N, slice[len(slice)-1].N),
					Translation: b.ID,
					Verses:      slice,
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
	Book        string `json:"book"`
	BookCode    string `json:"bookCode"`
	Chapter     int    `json:"chapter"`
	Verse       int    `json:"verse"`
	Text        string `json:"text"`
	Reference   string `json:"reference"`   // "Proverbs 3:5"
	Translation string `json:"translation"` // "web"
}

// Search runs a case-insensitive substring scan over every verse in the
// chosen translation and returns up to `limit` hits in canonical order.
// Empty/whitespace queries return zero results without error. Linear
// scan over the whole 4MB corpus runs in single-digit milliseconds, so
// we don't bother with an index.
//
// Empty translation defaults to WEB.
func Search(translation, query string, limit int) ([]SearchHit, error) {
	b, err := Get(translation)
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
						Book:        bk.Name,
						BookCode:    bk.Code,
						Chapter:     ch.Number,
						Verse:       v.N,
						Text:        v.Text,
						Reference:   fmt.Sprintf("%s %d:%d", bk.Name, ch.Number, v.N),
						Translation: b.ID,
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
