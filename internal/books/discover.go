package books

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Discover surfaces — search proxies for legal e-book sources so
// the user can browse → preview → save without leaving granit.
//
// v1 source: Project Gutenberg via the Gutendex JSON API
// (gutendex.com). Real search endpoint, ~70k titles, mostly
// pre-1928 classics in dozens of languages. Covers + EPUB
// downloads are direct-link-able without auth.
//
// Standard Ebooks (formerly part of the v1 set) put their full-
// catalog OPDS feed behind Patrons Circle Basic auth in 2026 —
// every /opds/* endpoint now returns 401 to unauthenticated
// callers, so we can't search or import without paid
// credentials. We keep the SourceStandardEbook constant for API
// compatibility but the implementation immediately errors with a
// clear message; the UI hides the source until we add an auth
// surface.
//
// We deliberately skip Open Library / Internet Archive for v1
// because their EPUB resolution path goes through Archive.org
// borrows + login flows that don't fit the granit "drop in vault
// and read" model.
//
// Anna's Archive and similar shadow libraries are NOT integrated
// — most of what they index is copyrighted material distributed
// without permission. Out of scope by design.

// Source identifies a discovery backend. Stable string values so
// the import handler can dispatch to the right downloader.
type Source string

const (
	SourceGutenberg     Source = "gutenberg"
	SourceStandardEbook Source = "standardebooks"
)

// errStandardEbooksPaywalled is returned by every SE code path.
// Stable sentinel so handlers / UI can branch on it (display a
// "subscription required" notice rather than a generic 502).
var errStandardEbooksPaywalled = errors.New(
	"books: Standard Ebooks moved their catalogue feed behind a paid Patrons Circle subscription — search and import are disabled in v1",
)

// IsStandardEbooksPaywalled reports whether the error is the
// sentinel above. Lets the handler emit a friendly 503 with a
// dedicated copy block rather than a generic upstream-failed message.
func IsStandardEbooksPaywalled(err error) bool {
	return errors.Is(err, errStandardEbooksPaywalled)
}

// DiscoverResult is one row in a search response. The shape is
// shared across sources so the UI can render a uniform card grid.
//
// AuthorDeathYear is the latest death year across all authors —
// surfaced so the UI can show "(d. 1817)" next to the title.
// Gives users in life+70 jurisdictions (most of the EU/UK/AU) a
// quick self-check for whether a US-public-domain title is also
// free in their country: life+70 cleared if the author died
// before <current year - 71>.
type DiscoverResult struct {
	Source          Source   `json:"source"`
	ExternalID      string   `json:"externalId"`
	Title           string   `json:"title"`
	Authors         []string `json:"authors,omitempty"`
	AuthorDeathYear int      `json:"authorDeathYear,omitempty"`
	Language        string   `json:"language,omitempty"`
	Subjects        []string `json:"subjects,omitempty"`
	PublishedYear   int      `json:"publishedYear,omitempty"`
	DownloadURL     string   `json:"downloadUrl"`
	CoverURL      string   `json:"coverUrl,omitempty"`
	ExternalURL   string   `json:"externalUrl,omitempty"`
	License       string   `json:"license,omitempty"`
	Description   string   `json:"description,omitempty"`
}

// DiscoverOptions filters a search call. Empty Sources means
// "search all enabled sources". Limit caps the per-source page size.
type DiscoverOptions struct {
	Sources []Source
	Limit   int
}

// SourceWarning is a non-fatal per-source failure surfaced to the
// caller. Lets the UI render "Project Gutenberg unavailable" inline
// instead of treating a partial outage as a total search failure.
type SourceWarning struct {
	Source  Source `json:"source"`
	Message string `json:"message"`
}

// SearchResponse bundles the merged result list with any per-source
// warnings. Empty Results + non-empty Warnings means every source
// failed; empty Results + no Warnings means "no matches found".
type SearchResponse struct {
	Results  []DiscoverResult `json:"results"`
	Warnings []SourceWarning  `json:"warnings,omitempty"`
}

// Search runs the query against every requested source in
// parallel and returns the combined result list plus per-source
// warnings. A single source's failure never kills the others —
// degraded results beat an empty page.
//
// Returns an error only when every requested source failed. An
// empty result list with no warnings is a valid "no matches" outcome.
func Search(ctx context.Context, query string, opts DiscoverOptions) (SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResponse{}, errors.New("books: empty query")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	sources := opts.Sources
	if len(sources) == 0 {
		// Gutenberg only by default — Standard Ebooks is paywalled.
		sources = []Source{SourceGutenberg}
	}

	type srcResult struct {
		source Source
		rs     []DiscoverResult
		err    error
	}
	out := make(chan srcResult, len(sources))
	var wg sync.WaitGroup
	for _, src := range sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			var rs []DiscoverResult
			var err error
			switch s {
			case SourceGutenberg:
				rs, err = searchGutenberg(ctx, query, limit)
			case SourceStandardEbook:
				err = errStandardEbooksPaywalled
			default:
				err = fmt.Errorf("books: unknown source %q", s)
			}
			out <- srcResult{s, rs, err}
		}(src)
	}
	wg.Wait()
	close(out)

	var (
		results  []DiscoverResult
		warnings []SourceWarning
		okCount  int
	)
	for r := range out {
		if r.err != nil {
			warnings = append(warnings, SourceWarning{
				Source:  r.source,
				Message: r.err.Error(),
			})
			continue
		}
		okCount++
		results = append(results, r.rs...)
	}

	// Stable order: by source then title.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Source != results[j].Source {
			return results[i].Source < results[j].Source
		}
		return strings.ToLower(results[i].Title) < strings.ToLower(results[j].Title)
	})

	// Every source failed → propagate the first error so the caller
	// can render a hard error state. Otherwise return whatever we
	// got (results may be empty if no matches but at least one
	// source returned cleanly).
	if okCount == 0 && len(warnings) > 0 {
		return SearchResponse{Warnings: warnings}, errors.New(warnings[0].Message)
	}
	return SearchResponse{Results: results, Warnings: warnings}, nil
}

// ── Project Gutenberg via Gutendex ────────────────────────────────

// gutendexBook maps the subset of the Gutendex response we care
// about. Format URLs are nested in a string→string map keyed by
// MIME type ("application/epub+zip", "image/jpeg", etc.).
type gutendexBook struct {
	ID        int               `json:"id"`
	Title     string            `json:"title"`
	Authors   []gutendexPerson  `json:"authors"`
	Languages []string          `json:"languages"`
	Subjects  []string          `json:"subjects"`
	Formats   map[string]string `json:"formats"`
}

type gutendexPerson struct {
	Name      string `json:"name"`
	BirthYear int    `json:"birth_year"`
	DeathYear int    `json:"death_year"`
}

type gutendexResponse struct {
	Count   int            `json:"count"`
	Results []gutendexBook `json:"results"`
}

const gutendexBase = "https://gutendex.com/books"

func searchGutenberg(ctx context.Context, q string, limit int) ([]DiscoverResult, error) {
	u := gutendexBase + "?search=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := searchClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gutendex: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("gutendex: status %d", res.StatusCode)
	}
	var body gutendexResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("gutendex: decode: %w", err)
	}
	out := make([]DiscoverResult, 0, len(body.Results))
	for _, b := range body.Results {
		dl := pickGutenbergEPUB(b.Formats)
		if dl == "" {
			continue // no EPUB → can't display through our reader
		}
		authors := make([]string, 0, len(b.Authors))
		latestDeath := 0
		for _, a := range b.Authors {
			if name := strings.TrimSpace(a.Name); name != "" {
				authors = append(authors, swapAuthorOrder(name))
			}
			if a.DeathYear > latestDeath {
				latestDeath = a.DeathYear
			}
		}
		lang := ""
		if len(b.Languages) > 0 {
			lang = b.Languages[0]
		}
		subjects := b.Subjects
		if len(subjects) > 4 {
			subjects = subjects[:4]
		}
		out = append(out, DiscoverResult{
			Source:          SourceGutenberg,
			ExternalID:      fmt.Sprintf("%d", b.ID),
			Title:           b.Title,
			Authors:         authors,
			AuthorDeathYear: latestDeath,
			Language:        lang,
			Subjects:        subjects,
			DownloadURL:     dl,
			CoverURL:        pickGutenbergCover(b.Formats),
			ExternalURL:     fmt.Sprintf("https://www.gutenberg.org/ebooks/%d", b.ID),
			License:         "Public domain in the US — verify life+70 in your jurisdiction if outside the US",
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Gutendex returns multiple EPUB links — "epub.images" (with
// embedded illustrations) and "epub.noimages". Prefer images when
// both are present, fall back to the bare key.
func pickGutenbergEPUB(formats map[string]string) string {
	for _, key := range []string{
		"application/epub+zip; charset=utf-8",
		"application/epub+zip",
	} {
		if v, ok := formats[key]; ok && !strings.Contains(v, ".zip") {
			return v
		}
	}
	for k, v := range formats {
		if strings.HasPrefix(k, "application/epub+zip") {
			return v
		}
	}
	return ""
}

func pickGutenbergCover(formats map[string]string) string {
	for _, key := range []string{"image/jpeg", "image/png"} {
		if v, ok := formats[key]; ok && !strings.HasSuffix(v, ".small.jpg") {
			return v
		}
	}
	for k, v := range formats {
		if strings.HasPrefix(k, "image/") {
			return v
		}
	}
	return ""
}

// swapAuthorOrder turns "Austen, Jane" into "Jane Austen" so the
// result cards read like book covers.
func swapAuthorOrder(name string) string {
	if i := strings.Index(name, ","); i > 0 {
		surname := strings.TrimSpace(name[:i])
		given := strings.TrimSpace(name[i+1:])
		if given != "" {
			return given + " " + surname
		}
	}
	return name
}

// stripTags pulls human-readable text out of an OPDS / HTML summary.
var tagStripRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string {
	clean := tagStripRe.ReplaceAllString(s, "")
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 280 {
		clean = clean[:277] + "…"
	}
	return clean
}

// absURL turns "/foo/bar" into "https://host/foo/bar". Used for
// OPDS-style relative refs.
func absURL(base, href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if strings.HasPrefix(href, "/") {
		return base + href
	}
	return base + "/" + href
}

// ── Import ────────────────────────────────────────────────────────

// MaxImportBytes caps every download at 50 MB. The largest
// legitimate EPUBs (illustrated editions of long classics) sit
// around 30 MB; anything bigger is almost certainly a mis-resolved
// ZIP wrapping the whole catalogue.
const MaxImportBytes = 50 * 1024 * 1024

// Import streams the EPUB at downloadURL into <vault>/Books/ and
// returns the resulting Summary so the caller can navigate
// straight to /books/<id>.
//
// Streaming (vs buffering bytes in memory) means a 30 MB import
// does not pin 30 MB of RES — important for the small VPS profiles
// granit ships on. A temp file lives next to the destination so
// the final rename is atomic on the same filesystem.
//
// Validates the response is actually an EPUB (zip header) before
// committing — a bait-and-switch HTML 404 page silently saved as
// "pride-and-prejudice.epub" would be confusing.
func Import(ctx context.Context, vaultRoot string, source Source, downloadURL, suggestedTitle string) (Summary, error) {
	if source == SourceStandardEbook {
		return Summary{}, errStandardEbooksPaywalled
	}
	if downloadURL == "" {
		return Summary{}, errors.New("books: empty download URL")
	}
	if !strings.HasPrefix(downloadURL, "https://") {
		return Summary{}, errors.New("books: download URL must be https")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return Summary{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/epub+zip,application/zip;q=0.9,*/*;q=0.5")
	res, err := importClient().Do(req)
	if err != nil {
		return Summary{}, fmt.Errorf("books: download failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Summary{}, fmt.Errorf("books: download status %d", res.StatusCode)
	}

	dir := filepath.Join(vaultRoot, BooksDirName)
	if err := mkdirAll(dir); err != nil {
		return Summary{}, fmt.Errorf("books: mkdir %s: %w", dir, err)
	}

	base := suggestedTitle
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(downloadURL), filepath.Ext(downloadURL))
	}
	clean := safeFilename(base)
	if clean == "" {
		clean = "book"
	}
	target := uniqueFilename(filepath.Join(dir, clean+".epub"))

	// Stream into a sibling temp file. We can't reuse atomicio.WriteWithPerm
	// here because it takes []byte (we'd lose the streaming win), so we
	// inline the same temp+rename shape with explicit bounded copy.
	tmp, err := os.CreateTemp(dir, ".import-*.epub")
	if err != nil {
		return Summary{}, fmt.Errorf("books: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	written, err := io.Copy(tmp, io.LimitReader(res.Body, MaxImportBytes+1))
	if err != nil {
		cleanupTmp()
		return Summary{}, fmt.Errorf("books: download body: %w", err)
	}
	if written > MaxImportBytes {
		cleanupTmp()
		return Summary{}, fmt.Errorf("books: download exceeds %d bytes (got at least %d)", MaxImportBytes, written)
	}
	if written < 4 {
		cleanupTmp()
		return Summary{}, errors.New("books: response too short to be an EPUB")
	}
	// Quick zip-header sniff before we commit. Real EPUBs start
	// with PK\x03\x04 (zip local file header).
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanupTmp()
		return Summary{}, err
	}
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(tmp, hdr); err != nil {
		cleanupTmp()
		return Summary{}, fmt.Errorf("books: read header: %w", err)
	}
	if !(hdr[0] == 'P' && hdr[1] == 'K' && hdr[2] == 0x03 && hdr[3] == 0x04) {
		cleanupTmp()
		return Summary{}, errors.New("books: response isn't a zip / EPUB (wrong magic bytes)")
	}
	if err := tmp.Sync(); err != nil {
		cleanupTmp()
		return Summary{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return Summary{}, err
	}
	// Match user-content perms: 0o644 so the user can open the
	// EPUB in Calibre / Kindle desktop without permission shuffling.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return Summary{}, err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return Summary{}, fmt.Errorf("books: rename to %s: %w", target, err)
	}

	// Validate by opening the saved file. If we can't parse it as
	// an EPUB, roll back so the shelf doesn't show a broken row.
	sum, err := summaryFromFile(vaultRoot, target)
	if err != nil {
		removeFile(target)
		return Summary{}, fmt.Errorf("books: imported file isn't a valid EPUB: %w", err)
	}
	return sum, nil
}

var unsafeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9 _-]+`)

func safeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = unsafeFilenameRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func uniqueFilename(p string) string {
	if !fileExists(p) {
		return p
	}
	dir := filepath.Dir(p)
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	ext := filepath.Ext(p)
	for i := 2; i < 100; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if !fileExists(candidate) {
			return candidate
		}
	}
	return p // last resort
}

// userAgent is what we present to upstream catalogues. A clean
// product UA with a real homepage URL keeps us above the bar of
// the heuristics most public APIs run.
const userAgent = "Granit/1.0 (https://github.com/artaeon/granit; +book-discover)"

// httpClientForTest is a package-level seam tests use to inject a
// client that trusts httptest.NewTLSServer's self-signed cert.
// nil → production code paths build their own per-call client.
var httpClientForTest *http.Client

// searchClient is shared across catalogue search calls. 30 s
// timeout fits the small JSON / XML feed responses.
func searchClient() *http.Client {
	if httpClientForTest != nil {
		return httpClientForTest
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// importClient is the long-tail download client. EPUB downloads
// from www.gutenberg.org's primary mirror occasionally take
// 60+ seconds when the CDN is cold; 120 s gives slow connections
// enough headroom without leaking goroutines forever.
func importClient() *http.Client {
	if httpClientForTest != nil {
		return httpClientForTest
	}
	return &http.Client{Timeout: 120 * time.Second}
}
