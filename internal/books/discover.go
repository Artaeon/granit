package books

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// The two sources we support in v1:
//
//   - Project Gutenberg via the Gutendex JSON API (gutendex.com).
//     Real search endpoint, ~70k titles, mostly pre-1928 classics
//     in dozens of languages. Covers + EPUB downloads are
//     direct-link-able.
//
//   - Standard Ebooks via their OPDS catalogue. No search API on
//     their side — we fetch the full feed once per server uptime
//     (with a 24h refresh) and filter locally. ~600 titles, but
//     each is hand-typeset with care that no automated source can
//     match. License: public domain in the US (their metadata is
//     explicit; we surface it on the cards).
//
// We deliberately skip Open Library / Internet Archive for v1
// because their EPUB resolution path goes through Archive.org
// borrows + login flows that don't fit the granit "drop in vault
// and read" model. Easy to add later as a third Source value.
//
// Anna's Archive and similar shadow libraries are NOT integrated
// — most of what they index is copyrighted material distributed
// without permission, and an in-app downloader would turn granit
// into a piracy tool. Out of scope by design.

// Source identifies a discovery backend. Stable string values so
// the import handler can dispatch to the right downloader.
type Source string

const (
	SourceGutenberg     Source = "gutenberg"
	SourceStandardEbook Source = "standardebooks"
)

// DiscoverResult is one row in a search response. The shape is
// shared across sources so the UI can render a uniform card grid.
type DiscoverResult struct {
	Source        Source   `json:"source"`
	ExternalID    string   `json:"externalId"`
	Title         string   `json:"title"`
	Authors       []string `json:"authors,omitempty"`
	Language      string   `json:"language,omitempty"`
	Subjects      []string `json:"subjects,omitempty"`
	PublishedYear int      `json:"publishedYear,omitempty"`
	DownloadURL   string   `json:"downloadUrl"`
	CoverURL      string   `json:"coverUrl,omitempty"`
	ExternalURL   string   `json:"externalUrl,omitempty"`
	License       string   `json:"license,omitempty"`
	Description   string   `json:"description,omitempty"`
}

// DiscoverOptions filters a search call. Empty Sources means
// "search all". Limit caps the per-source page size; the
// aggregate result count can be up to len(sources)*Limit.
type DiscoverOptions struct {
	Sources []Source
	Limit   int
}

// Search runs the query against every requested source in
// parallel and returns the combined result list. Failures from
// one source don't kill the others — degraded state is preferred
// to a search that returns nothing because Project Gutenberg's
// CDN is briefly down.
func Search(ctx context.Context, query string, opts DiscoverOptions) ([]DiscoverResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("books: empty query")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	sources := opts.Sources
	if len(sources) == 0 {
		sources = []Source{SourceGutenberg, SourceStandardEbook}
	}
	var (
		mu      sync.Mutex
		out     []DiscoverResult
		wg      sync.WaitGroup
		firstErr error
	)
	for _, src := range sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			var (
				rs  []DiscoverResult
				err error
			)
			switch s {
			case SourceGutenberg:
				rs, err = searchGutenberg(ctx, query, limit)
			case SourceStandardEbook:
				rs, err = searchStandardEbooks(ctx, query, limit)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			out = append(out, rs...)
		}(src)
	}
	wg.Wait()
	// Stable order: by source then title. Lets the UI render
	// "all Project Gutenberg results, then all Standard Ebooks
	// results" rather than scrambling them.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
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
	req.Header.Set("User-Agent", "granit/1.0 (https://github.com/artaeon/granit)")
	res, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("gutendex: status %d", res.StatusCode)
	}
	var body gutendexResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]DiscoverResult, 0, len(body.Results))
	for _, b := range body.Results {
		dl := pickGutenbergEPUB(b.Formats)
		if dl == "" {
			continue // no EPUB → can't display through our reader
		}
		authors := make([]string, 0, len(b.Authors))
		for _, a := range b.Authors {
			if name := strings.TrimSpace(a.Name); name != "" {
				authors = append(authors, swapAuthorOrder(name))
			}
		}
		lang := ""
		if len(b.Languages) > 0 {
			lang = b.Languages[0]
		}
		// Subjects are very long in Gutenberg metadata; cap at 4
		// for shelf legibility.
		subjects := b.Subjects
		if len(subjects) > 4 {
			subjects = subjects[:4]
		}
		out = append(out, DiscoverResult{
			Source:      SourceGutenberg,
			ExternalID:  fmt.Sprintf("%d", b.ID),
			Title:       b.Title,
			Authors:     authors,
			Language:    lang,
			Subjects:    subjects,
			DownloadURL: dl,
			CoverURL:    pickGutenbergCover(b.Formats),
			ExternalURL: fmt.Sprintf("https://www.gutenberg.org/ebooks/%d", b.ID),
			License:     "Public domain (US, in most cases — check the title page)",
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
// result cards read like book covers. Gutenberg metadata is
// surname-first by archival convention.
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

// ── Standard Ebooks via OPDS feed ─────────────────────────────────

const standardEbooksOPDS = "https://standardebooks.org/opds/all"

// seCache is an in-memory cache of the parsed Standard Ebooks
// catalog. The full feed is ~5MB; we fetch it on first /discover
// hit and refresh after 24 h.
var seCache struct {
	sync.Mutex
	entries  []DiscoverResult
	fetched  time.Time
}

const seCacheTTL = 24 * time.Hour

type opdsFeed struct {
	XMLName xml.Name   `xml:"feed"`
	Entries []opdsEntry `xml:"entry"`
}

type opdsEntry struct {
	Title    string      `xml:"title"`
	Authors  []opdsName  `xml:"author"`
	Updated  string      `xml:"updated"`
	Summary  string      `xml:"summary"`
	Links    []opdsLink  `xml:"link"`
	Subjects []opdsCategory `xml:"category"`
	Language string      `xml:"language"`
	ID       string      `xml:"id"`
}

type opdsName struct {
	Name string `xml:"name"`
}

type opdsLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

type opdsCategory struct {
	Label string `xml:"label,attr"`
	Term  string `xml:"term,attr"`
}

func searchStandardEbooks(ctx context.Context, q string, limit int) ([]DiscoverResult, error) {
	all, err := loadStandardEbooks(ctx)
	if err != nil {
		return nil, err
	}
	qLower := strings.ToLower(q)
	out := make([]DiscoverResult, 0, limit)
	for _, e := range all {
		// Match against title + authors + subjects. Cheap linear
		// scan — 600 entries fits in a millisecond.
		hay := strings.ToLower(e.Title)
		for _, a := range e.Authors {
			hay += " " + strings.ToLower(a)
		}
		for _, s := range e.Subjects {
			hay += " " + strings.ToLower(s)
		}
		if strings.Contains(hay, qLower) {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func loadStandardEbooks(ctx context.Context) ([]DiscoverResult, error) {
	seCache.Lock()
	defer seCache.Unlock()
	if time.Since(seCache.fetched) < seCacheTTL && len(seCache.entries) > 0 {
		return seCache.entries, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, standardEbooksOPDS, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "granit/1.0 (https://github.com/artaeon/granit)")
	req.Header.Set("Accept", "application/atom+xml")
	res, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("standardebooks: status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 25*1024*1024))
	if err != nil {
		return nil, err
	}
	var feed opdsFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	results := make([]DiscoverResult, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		dl := ""
		cover := ""
		external := ""
		for _, l := range e.Links {
			switch {
			case strings.Contains(l.Type, "epub+zip") && dl == "":
				dl = absURL("https://standardebooks.org", l.Href)
			case strings.Contains(l.Rel, "image/thumbnail") && cover == "":
				cover = absURL("https://standardebooks.org", l.Href)
			case strings.Contains(l.Rel, "image") && cover == "":
				cover = absURL("https://standardebooks.org", l.Href)
			case l.Rel == "alternate" && external == "":
				external = absURL("https://standardebooks.org", l.Href)
			}
		}
		if dl == "" {
			continue
		}
		authors := make([]string, 0, len(e.Authors))
		for _, a := range e.Authors {
			if n := strings.TrimSpace(a.Name); n != "" {
				authors = append(authors, n)
			}
		}
		subjects := make([]string, 0, len(e.Subjects))
		for _, s := range e.Subjects {
			if s.Label != "" {
				subjects = append(subjects, s.Label)
			} else if s.Term != "" {
				subjects = append(subjects, s.Term)
			}
		}
		if len(subjects) > 4 {
			subjects = subjects[:4]
		}
		results = append(results, DiscoverResult{
			Source:      SourceStandardEbook,
			ExternalID:  e.ID,
			Title:       strings.TrimSpace(e.Title),
			Authors:     authors,
			Language:    e.Language,
			Subjects:    subjects,
			DownloadURL: dl,
			CoverURL:    cover,
			ExternalURL: external,
			License:     "Public domain in the US",
			Description: stripTags(e.Summary),
		})
	}
	seCache.entries = results
	seCache.fetched = time.Now()
	return results, nil
}

// absURL turns "/foo/bar" into "https://standardebooks.org/foo/bar".
// OPDS feeds mix relative + absolute hrefs.
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

// stripTags pulls the human-readable text out of an OPDS summary
// that may contain inline HTML. Tags collapse to empty string —
// in real prose summaries, words are already separated by whitespace
// so we don't need to inject one when removing `<em>`/`<strong>`.
// Replacing with space would put a stray space before punctuation
// in "<em>x</em>." (becomes "x ."), so empty is the better choice.
var tagStripRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string {
	clean := tagStripRe.ReplaceAllString(s, "")
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 280 {
		clean = clean[:277] + "…"
	}
	return clean
}

// ── Import ────────────────────────────────────────────────────────

// Import streams the EPUB at downloadURL into <vault>/Books/ and
// returns the resulting Summary so the caller can navigate
// straight to /books/<id>. Validates that the response looks like
// an EPUB (zip header + EPUB mimetype entry) before accepting it
// — a bait-and-switch HTML 404 page silently saved as
// "pride-and-prejudice.epub" would be confusing.
func Import(ctx context.Context, vaultRoot string, source Source, downloadURL, suggestedTitle string) (Summary, error) {
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
	req.Header.Set("User-Agent", "granit/1.0 (https://github.com/artaeon/granit)")
	res, err := httpClient().Do(req)
	if err != nil {
		return Summary{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Summary{}, fmt.Errorf("books: download status %d", res.StatusCode)
	}
	// Cap at 50 MB — the largest legitimate EPUBs (illustrated
	// editions of long classics) sit around 30 MB. Anything bigger
	// is almost certainly not what the user intended.
	body, err := io.ReadAll(io.LimitReader(res.Body, 50*1024*1024))
	if err != nil {
		return Summary{}, err
	}
	// Quick zip-header sniff before we touch the filesystem.
	// Real EPUBs start with PK\x03\x04 (zip local file header).
	if len(body) < 4 || body[0] != 'P' || body[1] != 'K' {
		return Summary{}, errors.New("books: response isn't a zip / EPUB")
	}
	// Sanitize a filename. Prefer the suggested title; fall back
	// to the URL's tail. Strip path separators, parentheses, and
	// any bytes that would break Sync (Syncthing/iCloud) on
	// certain filesystems.
	base := suggestedTitle
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(downloadURL), filepath.Ext(downloadURL))
	}
	clean := safeFilename(base)
	if clean == "" {
		clean = "book"
	}
	dir := filepath.Join(vaultRoot, BooksDirName)
	if err := mkdirAll(dir); err != nil {
		return Summary{}, err
	}
	// If a file with the same name already exists, suffix with
	// "-2", "-3", etc. so a re-import doesn't clobber.
	target := uniqueFilename(filepath.Join(dir, clean+".epub"))
	if err := writeFileAtomic(target, body); err != nil {
		return Summary{}, err
	}
	// Open + summary so the caller can navigate.
	sum, err := summaryFromFile(vaultRoot, target)
	if err != nil {
		// Don't leave a corrupt file lying around — if the
		// downloaded zip can't be parsed as an EPUB, roll back
		// the write so the shelf doesn't show a broken row.
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

// httpClient is shared across all discovery calls. 30 s timeout
// is generous for a search; downloads can take longer but the
// per-request context handles cancellation.
func httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
