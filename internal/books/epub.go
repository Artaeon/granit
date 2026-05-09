// Package books provides EPUB parsing + a thin reader API for the
// /books surface. The user drops .epub files into <vault>/Books/
// (or any subfolder) and the package serves enough metadata for a
// shelf view + per-chapter HTML for an in-vault reader. No third-
// party dependency: an EPUB is a ZIP archive of XML/XHTML, both of
// which the standard library handles.
//
// Parsing scope is intentionally minimum-viable:
//
//   - Title + first author + cover image (for the shelf)
//   - Spine reading order (the chapter list in the reader)
//   - Chapter HTML on demand (lazy — we don't pre-load every
//     chapter at scan time)
//   - TOC if the file ships nav.xhtml (EPUB3) or toc.ncx (EPUB2)
//
// What we deliberately skip in v1: footnotes, ruby annotations,
// fixed-layout pagination, encryption (DRM-free EPUBs only — the
// user owns these). All can ship later behind the same reader UI.
//
// Storage / discovery:
//   - The vault has a sibling top-level "Books/" folder. We DON'T
//     stash EPUBs in .granit/ — they're user-visible content the
//     user might also open in Calibre or a Kindle desktop app.
//   - Per-book progress + highlights live in
//     <vault>/.granit/books/<id>.json (sidecar pattern matching
//     deadlines / shopping / hub).
//
// IDs are slug(title) + 8 char sha1(path) suffix so two files
// titled "1984" don't collide and the URL is human-readable.
//
// Stdlib + atomicio only. No HTTP, no rendering.
package books

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// EPUB is a parsed EPUB ready for chapter extraction. It holds the
// open zip reader for the lifetime of the request — close it when
// done. The struct is safe for concurrent reads (zip.Reader is).
type EPUB struct {
	zr *zip.ReadCloser
	// opfDir is the directory containing the OPF (every manifest
	// href is relative to this). EPUBs vary — some put it at root,
	// some under OEBPS/ — so we resolve once at parse time.
	opfDir string

	Title   string
	Authors []string
	// Spine is the ordered list of chapter manifest IDs.
	Spine []SpineItem
	// Manifest maps id → ManifestItem for every resource in the
	// EPUB (XHTML chapters, images, CSS, fonts).
	Manifest map[string]ManifestItem
	// CoverID is the manifest id of the cover image, if detected.
	CoverID string
	// TOC is the parsed table of contents (nav.xhtml or toc.ncx).
	TOC []TOCEntry
}

// SpineItem points at one chapter via its manifest id. Linear is
// the EPUB linearity flag — non-linear items (e.g. footnote
// inserts) get rendered through the reader the same way; we keep
// the flag so future "skip non-linear" toggles can use it without
// a re-parse.
type SpineItem struct {
	IDRef  string
	Linear bool
}

// ManifestItem is one resource entry from the OPF manifest.
type ManifestItem struct {
	ID         string
	Href       string // path relative to opfDir
	MediaType  string
	Properties string // "cover-image", "nav", etc. (EPUB3)
}

// TOCEntry is one row of the table of contents. SpineIdx is the
// index into Spine that the entry resolves to (so the reader can
// jump to that chapter without re-resolving). Children carry
// nested headings — most EPUBs only nest one or two levels.
//
// JSON tags are explicit to keep the wire shape camelCase
// (matches the rest of the granit API surface; without tags the
// default capitalised field names would leak through).
type TOCEntry struct {
	Title    string     `json:"title"`
	SpineIdx int        `json:"spineIdx"`
	Children []TOCEntry `json:"children,omitempty"`
}

// Open parses the EPUB at the given filesystem path. Caller must
// Close() the returned EPUB to release the underlying zip.
func Open(absPath string) (*EPUB, error) {
	zr, err := zip.OpenReader(absPath)
	if err != nil {
		return nil, fmt.Errorf("books: open zip: %w", err)
	}
	e := &EPUB{zr: zr, Manifest: make(map[string]ManifestItem)}
	if err := e.parse(); err != nil {
		zr.Close()
		return nil, err
	}
	return e, nil
}

// Close releases the zip handle.
func (e *EPUB) Close() error {
	if e.zr == nil {
		return nil
	}
	return e.zr.Close()
}

// parse reads container.xml → OPF → manifest → spine → TOC.
func (e *EPUB) parse() error {
	containerXML, err := e.readFile("META-INF/container.xml")
	if err != nil {
		return fmt.Errorf("books: missing META-INF/container.xml: %w", err)
	}
	var c container
	if err := xml.Unmarshal(containerXML, &c); err != nil {
		return fmt.Errorf("books: parse container.xml: %w", err)
	}
	if len(c.Rootfiles.Rootfile) == 0 {
		return errors.New("books: no rootfile in container.xml")
	}
	opfPath := c.Rootfiles.Rootfile[0].FullPath
	if opfPath == "" {
		return errors.New("books: empty rootfile path")
	}
	e.opfDir = path.Dir(opfPath)
	if e.opfDir == "." {
		e.opfDir = ""
	}
	opfBytes, err := e.readFile(opfPath)
	if err != nil {
		return fmt.Errorf("books: read OPF %q: %w", opfPath, err)
	}
	var pkg packageDoc
	if err := xml.Unmarshal(opfBytes, &pkg); err != nil {
		return fmt.Errorf("books: parse OPF: %w", err)
	}
	e.Title = strings.TrimSpace(pkg.Metadata.Title)
	for _, c := range pkg.Metadata.Creator {
		v := strings.TrimSpace(c.Value)
		if v != "" {
			e.Authors = append(e.Authors, v)
		}
	}
	for _, item := range pkg.Manifest.Item {
		e.Manifest[item.ID] = ManifestItem{
			ID:         item.ID,
			Href:       item.Href,
			MediaType:  item.MediaType,
			Properties: item.Properties,
		}
		if e.CoverID == "" && strings.Contains(item.Properties, "cover-image") {
			e.CoverID = item.ID
		}
	}
	// EPUB2-style cover lookup as fallback: <meta name="cover" content="<id>">.
	if e.CoverID == "" {
		for _, m := range pkg.Metadata.Meta {
			if strings.EqualFold(m.Name, "cover") && m.Content != "" {
				if _, ok := e.Manifest[m.Content]; ok {
					e.CoverID = m.Content
				}
			}
		}
	}
	// Last-resort cover: any manifest item whose href looks like
	// "cover" and is an image. Common in older EPUBs that didn't
	// declare cover-image properties.
	if e.CoverID == "" {
		for id, item := range e.Manifest {
			if strings.HasPrefix(item.MediaType, "image/") &&
				strings.Contains(strings.ToLower(item.Href), "cover") {
				e.CoverID = id
				break
			}
		}
	}
	for _, ref := range pkg.Spine.Itemref {
		linear := ref.Linear == "" || strings.EqualFold(ref.Linear, "yes")
		e.Spine = append(e.Spine, SpineItem{IDRef: ref.IDRef, Linear: linear})
	}
	if len(e.Spine) == 0 {
		return errors.New("books: empty spine")
	}
	e.parseTOC(pkg)
	return nil
}

// parseTOC tries EPUB3 nav.xhtml first, then EPUB2 toc.ncx. Either
// is best-effort: a missing TOC degrades to "just walk the spine".
func (e *EPUB) parseTOC(pkg packageDoc) {
	// EPUB3: manifest item with properties="nav".
	for _, item := range pkg.Manifest.Item {
		if !strings.Contains(item.Properties, "nav") {
			continue
		}
		body, err := e.readManifestHref(item.Href)
		if err != nil {
			return
		}
		e.TOC = parseNav(body, e.spineHrefIndex())
		return
	}
	// EPUB2: spine.toc points at the NCX manifest id.
	if pkg.Spine.TOC != "" {
		if item, ok := e.Manifest[pkg.Spine.TOC]; ok {
			body, err := e.readManifestHref(item.Href)
			if err != nil {
				return
			}
			e.TOC = parseNCX(body, e.spineHrefIndex())
		}
	}
}

// spineHrefIndex maps "manifest href (path within opfDir)" → spine
// index. parseNav / parseNCX use this to resolve a TOC <a href>
// back to the matching spine row.
func (e *EPUB) spineHrefIndex() map[string]int {
	m := make(map[string]int)
	for i, s := range e.Spine {
		if item, ok := e.Manifest[s.IDRef]; ok {
			m[item.Href] = i
			// Strip fragment so a TOC link "ch01.xhtml#sec2" still
			// resolves to ch01's spine row.
			m[strings.SplitN(item.Href, "#", 2)[0]] = i
		}
	}
	return m
}

// Chapter returns the raw HTML body for spine index idx, with all
// href/src attributes rewritten through `assetPrefix` so the
// frontend can resolve `images/foo.png` → `<assetPrefix>/images/foo.png`.
// If idx is out of range it returns ErrInvalidChapter.
func (e *EPUB) Chapter(idx int, assetPrefix string) (string, error) {
	if idx < 0 || idx >= len(e.Spine) {
		return "", ErrInvalidChapter
	}
	item, ok := e.Manifest[e.Spine[idx].IDRef]
	if !ok {
		return "", fmt.Errorf("books: spine[%d] points at unknown id %q", idx, e.Spine[idx].IDRef)
	}
	body, err := e.readManifestHref(item.Href)
	if err != nil {
		return "", err
	}
	return rewriteRefs(string(body), path.Dir(item.Href), assetPrefix), nil
}

// Asset reads an arbitrary manifest-relative path and returns the
// raw bytes + media type. Used by the asset passthrough handler so
// images / CSS / fonts referenced inside chapter HTML resolve.
func (e *EPUB) Asset(rel string) ([]byte, string, error) {
	rel = path.Clean(rel)
	if strings.HasPrefix(rel, "..") {
		return nil, "", errors.New("books: asset path escapes opfDir")
	}
	for _, item := range e.Manifest {
		if item.Href == rel {
			data, err := e.readManifestHref(item.Href)
			if err != nil {
				return nil, "", err
			}
			return data, item.MediaType, nil
		}
	}
	// Fallback: read by raw zip path under opfDir. Fonts often
	// aren't in the manifest in older books but still get
	// referenced from CSS.
	full := joinPath(e.opfDir, rel)
	data, err := e.readFile(full)
	if err != nil {
		return nil, "", err
	}
	return data, mimeFromExt(filepath.Ext(rel)), nil
}

// CoverBytes returns the cover image bytes + media type.
// Returns ErrNoCover if the EPUB doesn't have a cover entry.
func (e *EPUB) CoverBytes() ([]byte, string, error) {
	if e.CoverID == "" {
		return nil, "", ErrNoCover
	}
	item, ok := e.Manifest[e.CoverID]
	if !ok {
		return nil, "", ErrNoCover
	}
	data, err := e.readManifestHref(item.Href)
	if err != nil {
		return nil, "", err
	}
	return data, item.MediaType, nil
}

// ErrInvalidChapter is returned by Chapter when idx is out of
// range. ErrNoCover is returned when the EPUB lacks a cover image.
var (
	ErrInvalidChapter = errors.New("books: invalid chapter index")
	ErrNoCover        = errors.New("books: no cover image")
)

// readFile reads an arbitrary path inside the zip, case-sensitive.
// Returns the first match — EPUB spec says paths are unique.
func (e *EPUB) readFile(p string) ([]byte, error) {
	for _, f := range e.zr.File {
		if f.Name == p {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("books: file not found in zip: %q", p)
}

// readManifestHref reads a manifest-relative href (relative to opfDir).
func (e *EPUB) readManifestHref(href string) ([]byte, error) {
	href = strings.SplitN(href, "#", 2)[0]
	return e.readFile(joinPath(e.opfDir, href))
}

func joinPath(dir, rel string) string {
	if dir == "" {
		return rel
	}
	return path.Clean(dir + "/" + rel)
}

// ── XML schema mirrors ───────────────────────────────────────────

type container struct {
	XMLName   xml.Name `xml:"container"`
	Rootfiles struct {
		Rootfile []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

type packageDoc struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title   string `xml:"title"`
		Creator []struct {
			Value string `xml:",chardata"`
		} `xml:"creator"`
		Meta []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Item []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		TOC     string `xml:"toc,attr"`
		Itemref []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// ── HTML rewriting + sanitising ──────────────────────────────────

// (?:^|\s) anchors before the attribute name so we don't match the
// "href" inside `data-href` etc. The lookahead semantics are achieved
// with a non-capturing alternation that consumes the leading char.
var refAttrRe = regexp.MustCompile(`(?i)(^|\s)(href|src)=["']([^"']+)["']`)

// rewriteRefs scopes every relative href/src in chapter HTML so
// "images/foo.png" → "<prefix>/<rel>" — needed because the browser
// can't fetch zip-internal resources directly. Absolute / external
// URLs (http://, https://, mailto:, # fragments) pass through.
func rewriteRefs(body, chapterDir, prefix string) string {
	return refAttrRe.ReplaceAllStringFunc(body, func(match string) string {
		groups := refAttrRe.FindStringSubmatch(match)
		if len(groups) < 4 {
			return match
		}
		lead, attr, val := groups[1], groups[2], groups[3]
		// Skip absolute / external / fragment-only refs.
		if val == "" || strings.HasPrefix(val, "#") ||
			strings.Contains(val, "://") ||
			strings.HasPrefix(val, "mailto:") ||
			strings.HasPrefix(val, "data:") {
			return match
		}
		// Resolve relative to the chapter's directory inside opfDir.
		resolved := path.Clean(joinPath(chapterDir, val))
		return fmt.Sprintf(`%s%s="%s/%s"`, lead, attr, prefix, resolved)
	})
}


// ── ID generation ────────────────────────────────────────────────

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SlugID derives a stable URL-safe id from title + path. Title
// drives the human-readable prefix; path-hash suffix prevents
// collisions when two files share a title.
func SlugID(title, absPath string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		t = strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
		t = strings.ToLower(t)
	}
	t = nonSlugRe.ReplaceAllString(t, "-")
	t = strings.Trim(t, "-")
	if len(t) > 60 {
		t = t[:60]
	}
	if t == "" {
		t = "book"
	}
	h := sha1.Sum([]byte(absPath))
	return t + "-" + hex.EncodeToString(h[:4])
}

// mimeFromExt is a small mime sniffer for assets the EPUB ships
// outside its manifest (web fonts referenced from CSS, mostly).
func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".css":
		return "text/css"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".html", ".xhtml", ".htm":
		return "application/xhtml+xml"
	default:
		return "application/octet-stream"
	}
}
