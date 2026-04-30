// Package publish renders a folder of markdown notes into a static black-and-
// white site. Designed to be hosted on GitHub Pages, Cloudflare Pages, or any
// static-file server (e.g. Traefik via fleetdeck).
//
// The builder:
//   1. Walks a source folder for .md files
//   2. Parses frontmatter (title, tags, date, publish:false override)
//   3. Resolves [[wikilinks]] across the published note set, with name/slug
//      fallback so links keep working when filenames differ in case or spaces
//   4. Renders each note's body via goldmark to HTML
//   5. Builds a backlinks index (which notes mention this note) and a tag
//      index (which notes carry which tag)
//   6. Writes index.html, notes/*.html, tags/*.html, style.css, search-index.json
//   7. Drops a .nojekyll file at the root so GitHub Pages serves files
//      starting with underscores or dots correctly
//
// The output is a fully self-contained directory — `cd dist && python -m
// http.server` works, and `git push` to a Pages-enabled repo Just Works.
package publish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// Config drives a publish run. Loaded from .granit/publish.json or constructed
// directly by callers (CLI subcommand, tests).
type Config struct {
	// SiteTitle appears in the header and <title>. Defaults to "Notes".
	SiteTitle string `json:"siteTitle,omitempty"`

	// Intro renders below the H1 on the index page.
	Intro string `json:"intro,omitempty"`

	// Footer text. Plain string for v1; HTML support could come later.
	Footer string `json:"footer,omitempty"`

	// Lang is the html[lang] attribute. Defaults to "en".
	Lang string `json:"lang,omitempty"`

	// SourceDir is the absolute path to the folder being published. The
	// CLI sets this from the positional [folder] argument; tests inject
	// directly.
	SourceDir string `json:"-"`

	// OutputDir is where the generated site is written.
	OutputDir string `json:"outputDir,omitempty"`

	// Homepage is the relative path (under SourceDir) of the note that
	// becomes index.html. Empty → fall through to the auto-generated
	// note list.
	Homepage string `json:"homepage,omitempty"`

	// IncludeUnlisted, when true, also publishes notes that do NOT
	// frontmatter `publish: true` — i.e. publish everything except
	// notes with `publish: false`. False (default): opt-in via
	// frontmatter. We default to including-everything-in-folder since
	// the user is explicitly choosing the folder; the override is
	// `publish: false` to exclude a single note.
	IncludeUnlisted bool `json:"includeUnlisted,omitempty"`

	// Search toggles the client-side search index + JS shim.
	Search bool `json:"search,omitempty"`

	// SiteURL is the public root URL the site will live at, including
	// scheme but no trailing slash (e.g. https://notes.example.com or
	// https://user.github.io/repo). Used to:
	//   - Emit absolute canonical <link> tags so search engines pick a
	//     single source of truth even when the site is mirrored or
	//     proxied.
	//   - Populate Open Graph + Twitter Card og:url tags.
	//   - Generate sitemap.xml (which requires absolute URLs).
	// Optional — when empty, canonical/og:url are omitted and the
	// sitemap uses path-only URLs (still valid but less informative).
	SiteURL string `json:"siteURL,omitempty"`

	// Author is set as the Article's author in JSON-LD and
	// <meta name="author">.
	Author string `json:"author,omitempty"`

	// CookieBanner enables a small bottom banner on every page. The
	// page is otherwise cookieless — the banner is for users who plan
	// to add analytics later, or for compliance signalling on EU sites.
	CookieBanner bool `json:"cookieBanner,omitempty"`

	// CookieMessage replaces the default banner text. Empty → use
	// the default which references the auto-detected datenschutz page
	// when present.
	CookieMessage string `json:"cookieMessage,omitempty"`

	// NoBranding suppresses the small "Built with Granit" link below
	// the user's footer. Off by default — the link is unobtrusive
	// (grey, small, centered) and acts as a credit + breadcrumb so
	// visitors can find the tool when they like the look.
	NoBranding bool `json:"noBranding,omitempty"`

	// HomepageStyle picks the index.html layout when the user has
	// NOT specified Homepage (a note acting as the index).
	//   "list" — the default; clean vertical list of every note.
	//   "hero" — large title block + search + 3-column note grid.
	HomepageStyle string `json:"homepageStyle,omitempty"`

	// DefaultOGImage is a path (relative to source folder) to a
	// site-wide image used as og:image when a note has no per-note
	// image: frontmatter and AutoOGImage is off. Get copied to the
	// output along with other assets.
	DefaultOGImage string `json:"defaultOGImage,omitempty"`

	// AutoOGImage, when true, generates a 1200×630 PNG og:image for
	// every note that doesn't have its own image: frontmatter or a
	// site-wide DefaultOGImage. The generated image is title-only,
	// black on white, using the embedded Go font — readable, small
	// (~10 KB), no external font dependency.
	AutoOGImage bool `json:"autoOGImage,omitempty"`

	// Math, when true, renders LaTeX math via KaTeX (auto-loaded
	// from CDN, only on pages that contain $...$ or $$...$$).
	Math bool `json:"math,omitempty"`

	// Mermaid, when true, renders ` + "```mermaid" + ` code blocks via
	// the Mermaid library (auto-loaded from CDN, only on pages that
	// contain a mermaid block).
	Mermaid bool `json:"mermaid,omitempty"`

	// FeedItems caps the number of newest notes included in feed.xml.
	// Default 50 — RSS readers don't benefit from unbounded history,
	// and a 500-note vault would otherwise produce a multi-megabyte
	// feed. Set to 0 to include every published note.
	FeedItems int `json:"feedItems,omitempty"`
}

// applyDefaults fills missing string-valued fields so callers don't need to
// specify everything for a useful result. Bool fields are left to the
// caller — the CLI layer sets the conventional defaults (Search=true,
// IncludeUnlisted=true) before calling Build, so an explicit `false` from
// a user flag or config file isn't silently overridden here.
//
// Footer no longer defaults to "Built with Granit" — that's now a
// separate Branding element (clickable link to the project) which
// renders below the user's footer and survives even when Footer is
// empty. Avoids the awkward two-line "Built with Granit / Built with
// Granit" stacking when both fields existed.
func (c *Config) applyDefaults() {
	if c.SiteTitle == "" {
		c.SiteTitle = "Notes"
	}
	if c.Lang == "" {
		c.Lang = "en"
	}
	if c.OutputDir == "" {
		c.OutputDir = "./dist"
	}
}

// Note is a parsed markdown file with everything the renderer needs.
type Note struct {
	SourcePath string    // absolute path on disk
	RelPath    string    // path relative to SourceDir (key for wikilinks)
	Slug       string    // URL-safe identifier (drives the output filename)
	Title      string    // human-friendly title (frontmatter or first H1 or filename)
	Date       string    // optional date string from frontmatter
	Tags       []string  // tag names (no leading #)
	Body       string    // raw markdown after frontmatter strip
	HTML       string    // rendered HTML (filled by build())
	BodyText   string    // plain-text view used by the search index
	ModTime    time.Time // file mtime — fallback ordering when no date is set
	Outlinks   []string  // RelPaths of other notes this note links to

	// SEO / metadata extras pulled from frontmatter when present.
	Author     string // <meta name="author"> + JSON-LD author
	NoIndex    bool   // frontmatter `noindex: true` → skip from sitemap, add robots meta
	WordCount  int    // body word count (used for reading-time estimate)
	Legal      string // "impressum", "datenschutz", or "" — see detectLegalKind
	OutputPath string // resolved publish-time URL path (e.g. notes/foo.html or impressum.html)
	OGImage    string // per-note og:image path (resolved post-rewrite)
	HasMath    bool   // body contains $math$ — triggers KaTeX include on this page
	HasMermaid bool   // body contains ```mermaid — triggers Mermaid include on this page
}

// Build is the entry point. Parses the source folder, renders the site, and
// returns a small Result summarising what was written. Errors are wrapped
// with enough context to be useful in CLI output.
type Result struct {
	NotesPublished  int
	TagsPublished   int
	AssetsCopied    int
	OutputDir       string
}

func Build(cfg Config) (Result, error) {
	cfg.applyDefaults()

	if cfg.SourceDir == "" {
		return Result{}, fmt.Errorf("publish: SourceDir is required")
	}
	absSrc, err := filepath.Abs(cfg.SourceDir)
	if err != nil {
		return Result{}, fmt.Errorf("publish: resolve source: %w", err)
	}
	cfg.SourceDir = absSrc

	notes, err := loadNotes(cfg)
	if err != nil {
		return Result{}, err
	}
	if len(notes) == 0 {
		return Result{}, fmt.Errorf("publish: no notes found under %s", cfg.SourceDir)
	}

	// Build slug→note + title→note maps so wikilinks can find their
	// target by either filename or human title.
	bySlug := make(map[string]*Note, len(notes))
	byTitle := make(map[string]*Note, len(notes))
	for _, n := range notes {
		bySlug[n.Slug] = n
		byTitle[strings.ToLower(n.Title)] = n
		// Also accept the filename without extension (case-insensitive)
		// so [[KEM Manager - Rolle und Aufgaben]] matches a note whose
		// filename was that exact string before slugification.
		base := strings.TrimSuffix(filepath.Base(n.RelPath), filepath.Ext(n.RelPath))
		byTitle[strings.ToLower(base)] = n
	}

	// Compute output path per note up-front so the wikilink resolver
	// can return correct URLs (legal pages live at the root, regular
	// notes under notes/).
	for _, n := range notes {
		if n.Legal != "" {
			n.OutputPath = n.Legal + ".html"
		} else {
			n.OutputPath = "notes/" + n.Slug + ".html"
		}
	}

	// Render bodies with wikilinks resolved to relative HTML URLs.
	// Targets that resolve to legal pages link to the root-level URL,
	// so cross-references work even when the source markdown points at
	// "datenschutz" / "impressum" by title.
	resolver := func(target string) (url, title string, ok bool) {
		key := strings.ToLower(strings.TrimSpace(target))
		if n, hit := byTitle[key]; hit {
			return n.OutputPath, n.Title, true
		}
		if n, hit := bySlug[slugify(target)]; hit {
			return n.OutputPath, n.Title, true
		}
		return "", "", false
	}

	md := newMarkdown()
	for _, n := range notes {
		// Rewrite relative image paths so they resolve from the
		// published note's URL rather than the source-folder layout.
		// OutputPath was computed earlier in this function so legal
		// pages (root) and regular notes (notes/<slug>.html) get
		// the right number of ../ prefixes.
		bodyWithImages := rewriteImagePaths(n.Body, n.RelPath, n.OutputPath)
		// Then resolve wikilinks.
		bodyWithLinks, outlinks := resolveWikilinks(bodyWithImages, resolver, bySlug, byTitle)
		n.Outlinks = outlinks
		// Detect math + mermaid presence so we only inject the
		// CDN scripts on pages that need them.
		if cfg.Math && containsMath(bodyWithLinks) {
			n.HasMath = true
		}
		if cfg.Mermaid && strings.Contains(bodyWithLinks, "```mermaid") {
			n.HasMermaid = true
		}
		var buf bytes.Buffer
		if err := md.Convert([]byte(bodyWithLinks), &buf); err != nil {
			return Result{}, fmt.Errorf("publish: render %s: %w", n.RelPath, err)
		}
		n.HTML = buf.String()
		// Mermaid: goldmark renders ```mermaid as <pre><code class="language-mermaid">…</code></pre>.
		// Mermaid.js looks for <pre class="mermaid">…</pre>. Rewrite the
		// block as a single unit using a regex so we DON'T strip the
		// closing </code> from unrelated code blocks on the same page —
		// the previous implementation did `ReplaceAll("</code></pre>", "</pre>")`
		// which corrupted every chroma-highlighted block sharing the
		// same closing tag pattern.
		if n.HasMermaid {
			n.HTML = reMermaidBlock.ReplaceAllString(n.HTML, `<pre class="mermaid">$1</pre>`)
		}
		n.BodyText = stripMD(bodyWithLinks)
	}

	// Compute backlinks from outlinks.
	backlinks := make(map[string][]*Note, len(notes))
	for _, n := range notes {
		for _, target := range n.Outlinks {
			backlinks[target] = append(backlinks[target], n)
		}
	}

	// Output directory.
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("publish: mkdir output: %w", err)
	}
	notesDir := filepath.Join(cfg.OutputDir, "notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("publish: mkdir notes: %w", err)
	}
	tagsDir := filepath.Join(cfg.OutputDir, "tags")
	if err := os.MkdirAll(tagsDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("publish: mkdir tags: %w", err)
	}

	// Tags index.
	tagSet := make(map[string][]*Note)
	for _, n := range notes {
		for _, tag := range n.Tags {
			tagSet[tag] = append(tagSet[tag], n)
		}
	}
	hasTags := len(tagSet) > 0

	// Separate legal pages from the regular note set so the index,
	// graph, and prev/next nav don't include them. They render to the
	// root, get their own footer links, and stay out of the search
	// index (they're boilerplate, not content).
	var regularNotes, legalNotes []*Note
	var impressumNote, datenschutzNote *Note
	for _, n := range notes {
		if n.Legal == "" {
			regularNotes = append(regularNotes, n)
			continue
		}
		legalNotes = append(legalNotes, n)
		switch n.Legal {
		case "impressum":
			impressumNote = n
		case "datenschutz":
			datenschutzNote = n
		}
	}

	// Graph SVG — only meaningful with 2+ regular notes (legal pages
	// would be loose nodes since nothing references them).
	graphSVG := renderGraphSVG(regularNotes)
	hasGraph := graphSVG != ""
	edgeCount := 0
	for _, n := range notes {
		edgeCount += len(n.Outlinks)
	}

	// Prev/Next pairs — only across regular notes (legal pages don't
	// belong in a "next chapter" sequence).
	prevNext := buildPrevNext(regularNotes)

	// Cookie banner default message — referenced from inside the
	// banner block we emit on every page if cfg.CookieBanner is true.
	cookieMsg := cfg.CookieMessage
	if cookieMsg == "" {
		if datenschutzNote != nil {
			cookieMsg = `This site uses minimal cookies to remember preferences. By using it, you agree. See our <a href="{datenschutzURL}">Datenschutz</a>.`
		} else {
			cookieMsg = "This site uses minimal cookies to remember preferences. By using it, you agree."
		}
	}

	tpls := mustParseTemplates()
	render := func(outPath string, body string, page pageData) error {
		page.Body = template.HTML(body)
		page.HasTags = hasTags
		page.HasGraph = hasGraph
		page.Search = cfg.Search
		page.Author = firstNonEmpty(page.Author, cfg.Author)
		// Footer legal links — relative URL recomputed per page so
		// pages under notes/ correctly point ../impressum.html etc.
		if impressumNote != nil {
			page.HasImpressum = true
			page.ImpressumURL = page.RelRoot + "impressum.html"
		}
		if datenschutzNote != nil {
			page.HasDatenschutz = true
			page.DatenschutzURL = page.RelRoot + "datenschutz.html"
		}
		// Cookie banner — only if config asked for it.
		if cfg.CookieBanner {
			page.CookieBanner = true
			msg := cookieMsg
			if datenschutzNote != nil {
				msg = strings.ReplaceAll(msg, "{datenschutzURL}", page.DatenschutzURL)
			}
			page.CookieMessage = template.HTML(msg)
		}
		// Default OG type if caller didn't set it.
		if page.OGType == "" {
			page.OGType = "website"
		}
		// RSS feed auto-discovery <link> — present on every page
		// when the build emits feed.xml (which it always does).
		page.FeedLink = template.HTML(rssAutoLink(cfg.SiteURL))
		// Branding line — small clickable credit. The link target is
		// the canonical project URL; the visible text stays "Granit"
		// so it reads naturally inside the "Built with X" sentence
		// and matches the visual weight of the rest of the footer.
		if !cfg.NoBranding {
			page.Branding = template.HTML(
				`Built with <a href="https://github.com/Artaeon/granit" rel="noopener" target="_blank">Granit</a>`,
			)
		}
		var buf bytes.Buffer
		if err := tpls.base.Execute(&buf, page); err != nil {
			return err
		}
		return os.WriteFile(outPath, buf.Bytes(), 0o644)
	}

	// Note pages.
	for i, n := range regularNotes {
		var bodyBuf bytes.Buffer
		nd := noteData{
			Title:       n.Title,
			Date:        n.Date,
			ReadingTime: estimateReadingTime(n.WordCount),
			Content:     template.HTML(n.HTML),
			Outline:     extractOutline(n.Body),
			RelRoot:     "../",
		}
		for _, t := range n.Tags {
			nd.Tags = append(nd.Tags, tagRef{Name: t, Slug: slugify(t)})
		}
		for _, b := range backlinks[n.RelPath] {
			nd.Backlinks = append(nd.Backlinks, noteRef{Title: b.Title, URL: "../notes/" + b.Slug + ".html"})
		}
		if pn := prevNext[i]; pn.prev != nil {
			nd.Prev = &noteRef{Title: pn.prev.Title, URL: pn.prev.Slug + ".html"}
		}
		if pn := prevNext[i]; pn.next != nil {
			nd.Next = &noteRef{Title: pn.next.Title, URL: pn.next.Slug + ".html"}
		}
		if err := tpls.note.Execute(&bodyBuf, nd); err != nil {
			return Result{}, fmt.Errorf("publish: note template %s: %w", n.RelPath, err)
		}
		out := filepath.Join(notesDir, n.Slug+".html")
		// Resolve og:image for THIS note, in priority order:
		//   1. frontmatter image: (relative path in the source folder)
		//   2. site-wide DefaultOGImage config
		//   3. auto-generated PNG when AutoOGImage is on
		// The resolved path is converted to absolute when SiteURL is
		// set — most social-media crawlers want absolute og:image
		// URLs.
		ogImage := ""
		switch {
		case n.OGImage != "":
			ogImage = n.OGImage
		case cfg.DefaultOGImage != "":
			ogImage = cfg.DefaultOGImage
		case cfg.AutoOGImage:
			// OG image generation is best-effort — a single bad
			// title (exotic Unicode the embedded font can't measure,
			// disk-full mid-write, etc.) shouldn't kill an
			// otherwise-successful publish run. Log and continue
			// without an og:image for this note; the page just
			// falls back to the no-image Twitter Card variant.
			path, err := generateOGImage(cfg.OutputDir, n.Slug, n.Title, cfg.SiteTitle)
			if err != nil {
				fmt.Fprintf(os.Stderr, "publish: skip og image for %s: %v\n", n.RelPath, err)
			} else {
				ogImage = path
			}
		}
		ogImageURL := ""
		ogImageType := ""
		if ogImage != "" {
			if isAbsoluteURL(ogImage) {
				ogImageURL = ogImage
			} else if cfg.SiteURL != "" {
				ogImageURL = strings.TrimRight(cfg.SiteURL, "/") + "/" + strings.TrimLeft(ogImage, "/")
			} else {
				ogImageURL = "../" + strings.TrimLeft(ogImage, "/")
			}
			switch strings.ToLower(filepath.Ext(ogImage)) {
			case ".png":
				ogImageType = "image/png"
			case ".jpg", ".jpeg":
				ogImageType = "image/jpeg"
			case ".webp":
				ogImageType = "image/webp"
			case ".svg":
				ogImageType = "image/svg+xml"
			}
		}
		page := pageData{
			Lang:         cfg.Lang,
			SiteTitle:    cfg.SiteTitle,
			PageTitle:    n.Title,
			Description:  firstSentence(n.BodyText, 160),
			Footer:       cfg.Footer,
			RelRoot:      "../",
			CanonicalURL: canonicalFor(cfg.SiteURL, n.OutputPath),
			OGType:       "article",
			OGImage:      ogImageURL,
			OGImageType:  ogImageType,
			NoIndex:      n.NoIndex,
			Author:       firstNonEmpty(n.Author, cfg.Author),
			JSONLD:       articleJSONLD(n, cfg.SiteURL, cfg.SiteTitle, cfg.Author),
			HasMath:      n.HasMath,
			HasMermaid:   n.HasMermaid,
		}
		if err := render(out, bodyBuf.String(), page); err != nil {
			return Result{}, fmt.Errorf("publish: write %s: %w", out, err)
		}
	}

	// Legal pages — render to root with their own header/footer treatment.
	// The body is the rendered article HTML, no Outline (legal pages
	// rarely benefit from a TOC), no prev/next, no backlinks (most
	// notes won't link to them).
	for _, n := range legalNotes {
		var bodyBuf bytes.Buffer
		nd := noteData{
			Title:   n.Title,
			Date:    n.Date,
			Content: template.HTML(n.HTML),
			RelRoot: "",
		}
		if err := tpls.note.Execute(&bodyBuf, nd); err != nil {
			return Result{}, fmt.Errorf("publish: legal template %s: %w", n.RelPath, err)
		}
		out := filepath.Join(cfg.OutputDir, n.Legal+".html")
		page := pageData{
			Lang:         cfg.Lang,
			SiteTitle:    cfg.SiteTitle,
			PageTitle:    n.Title,
			Description:  firstSentence(n.BodyText, 160),
			Footer:       cfg.Footer,
			RelRoot:      "",
			CanonicalURL: canonicalFor(cfg.SiteURL, n.OutputPath),
			OGType:       "website",
			NoIndex:      true, // legal pages don't benefit from being indexed
			Author:       firstNonEmpty(n.Author, cfg.Author),
		}
		if err := render(out, bodyBuf.String(), page); err != nil {
			return Result{}, fmt.Errorf("publish: write %s: %w", out, err)
		}
	}

	// Index page — either a note acting as homepage, or the auto note list.
	var indexBody bytes.Buffer
	indexPage := pageData{
		Lang:         cfg.Lang,
		SiteTitle:    cfg.SiteTitle,
		PageTitle:    cfg.SiteTitle,
		Description:  cfg.Intro,
		Footer:       cfg.Footer,
		RelRoot:      "",
		CanonicalURL: canonicalFor(cfg.SiteURL, "index.html"),
		OGType:       "website",
	}
	if cfg.Homepage != "" {
		// Find note matching cfg.Homepage (relative to SourceDir).
		// Searches the regular set only — using a legal page as the
		// homepage doesn't make sense.
		var home *Note
		for _, n := range regularNotes {
			if n.RelPath == cfg.Homepage || filepath.Base(n.RelPath) == cfg.Homepage {
				home = n
				break
			}
		}
		if home == nil {
			return Result{}, fmt.Errorf("publish: homepage %q not found in source folder", cfg.Homepage)
		}
		nd := noteData{
			Title:   home.Title,
			Date:    home.Date,
			Content: template.HTML(home.HTML),
			RelRoot: "",
		}
		for _, b := range backlinks[home.RelPath] {
			nd.Backlinks = append(nd.Backlinks, noteRef{Title: b.Title, URL: "notes/" + b.Slug + ".html"})
		}
		if err := tpls.note.Execute(&indexBody, nd); err != nil {
			return Result{}, fmt.Errorf("publish: index template: %w", err)
		}
		indexPage.PageTitle = home.Title
		indexPage.Description = firstSentence(home.BodyText, 160)
	} else {
		idx := indexData{
			SiteTitle: cfg.SiteTitle,
			Intro:     cfg.Intro,
			Search:    cfg.Search,
		}
		// Sort by date desc, then title. Legal pages are excluded.
		sorted := append([]*Note(nil), regularNotes...)
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].Date != sorted[j].Date {
				return sorted[i].Date > sorted[j].Date
			}
			return sorted[i].Title < sorted[j].Title
		})
		for _, n := range sorted {
			idx.Notes = append(idx.Notes, noteRef{
				Title:   n.Title,
				URL:     "notes/" + n.Slug + ".html",
				Summary: firstSentence(n.BodyText, 140),
			})
		}
		// Pick the layout: "hero" gets a big title block + featured
		// note grid; default "list" gets the dense vertical list.
		// We accept "" / "list" / "default" all as the list layout
		// so config-file authors don't get punished for a typo.
		switch strings.ToLower(strings.TrimSpace(cfg.HomepageStyle)) {
		case "hero":
			if err := tpls.hero.Execute(&indexBody, idx); err != nil {
				return Result{}, fmt.Errorf("publish: hero template: %w", err)
			}
		default:
			if err := tpls.index.Execute(&indexBody, idx); err != nil {
				return Result{}, fmt.Errorf("publish: index template: %w", err)
			}
		}
	}
	if err := render(filepath.Join(cfg.OutputDir, "index.html"), indexBody.String(), indexPage); err != nil {
		return Result{}, err
	}

	// Tag pages.
	if hasTags {
		var tagsIdx tagsIndexData
		tagNames := make([]string, 0, len(tagSet))
		for t := range tagSet {
			tagNames = append(tagNames, t)
		}
		sort.Strings(tagNames)
		for _, t := range tagNames {
			ns := tagSet[t]
			tagsIdx.Tags = append(tagsIdx.Tags, tagRef{Name: t, Slug: slugify(t), Count: len(ns)})
		}
		var tiBody bytes.Buffer
		if err := tpls.tagIndex.Execute(&tiBody, tagsIdx); err != nil {
			return Result{}, fmt.Errorf("publish: tags index template: %w", err)
		}
		page := pageData{
			Lang: cfg.Lang, SiteTitle: cfg.SiteTitle, PageTitle: "Tags",
			Footer: cfg.Footer, RelRoot: "../",
			CanonicalURL: canonicalFor(cfg.SiteURL, "tags/index.html"),
			OGType:       "website",
		}
		if err := render(filepath.Join(tagsDir, "index.html"), tiBody.String(), page); err != nil {
			return Result{}, err
		}
		for _, t := range tagNames {
			tp := tagPageData{Name: t, RelRoot: "../"}
			notesSorted := append([]*Note(nil), tagSet[t]...)
			sort.SliceStable(notesSorted, func(i, j int) bool { return notesSorted[i].Title < notesSorted[j].Title })
			for _, n := range notesSorted {
				tp.Notes = append(tp.Notes, noteRef{
					Title:   n.Title,
					URL:     "notes/" + n.Slug + ".html",
					Summary: firstSentence(n.BodyText, 140),
				})
			}
			var tpBuf bytes.Buffer
			if err := tpls.tagPage.Execute(&tpBuf, tp); err != nil {
				return Result{}, fmt.Errorf("publish: tag page %s: %w", t, err)
			}
			page := pageData{
				Lang: cfg.Lang, SiteTitle: cfg.SiteTitle, PageTitle: "#" + t,
				Footer: cfg.Footer, RelRoot: "../",
				CanonicalURL: canonicalFor(cfg.SiteURL, "tags/"+slugify(t)+".html"),
				OGType:       "website",
			}
			if err := render(filepath.Join(tagsDir, slugify(t)+".html"), tpBuf.String(), page); err != nil {
				return Result{}, err
			}
		}
	}

	// Graph page — only when at least one edge exists; otherwise the
	// page would be a screenful of disconnected dots and offer nothing
	// over the index.
	if hasGraph {
		var gBuf bytes.Buffer
		gd := graphData{
			NoteCount: len(regularNotes),
			EdgeCount: edgeCount,
			SVG:       template.HTML(graphSVG),
		}
		if err := tpls.graph.Execute(&gBuf, gd); err != nil {
			return Result{}, fmt.Errorf("publish: graph template: %w", err)
		}
		page := pageData{
			Lang: cfg.Lang, SiteTitle: cfg.SiteTitle, PageTitle: "Graph",
			Footer: cfg.Footer, RelRoot: "",
			CanonicalURL: canonicalFor(cfg.SiteURL, "graph.html"),
			OGType:       "website",
		}
		if err := render(filepath.Join(cfg.OutputDir, "graph.html"), gBuf.String(), page); err != nil {
			return Result{}, err
		}
	}

	// Static assets: stylesheet, search shim, search index, .nojekyll.
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, "style.css"), []byte(defaultCSS), 0o644); err != nil {
		return Result{}, fmt.Errorf("publish: write style.css: %w", err)
	}
	if cfg.Search {
		if err := os.WriteFile(filepath.Join(cfg.OutputDir, "search.js"), []byte(defaultSearchJS), 0o644); err != nil {
			return Result{}, fmt.Errorf("publish: write search.js: %w", err)
		}
		var idx []searchDoc
		// Skip legal pages and noindex notes from the search index —
		// users don't expect to find boilerplate via in-site search.
		for _, n := range regularNotes {
			if n.NoIndex {
				continue
			}
			idx = append(idx, searchDoc{
				Title: n.Title,
				URL:   "notes/" + n.Slug + ".html",
				Body:  truncate(n.BodyText, 800),
			})
		}
		buf, err := json.Marshal(idx)
		if err != nil {
			return Result{}, fmt.Errorf("publish: search index: %w", err)
		}
		if err := os.WriteFile(filepath.Join(cfg.OutputDir, "search-index.json"), buf, 0o644); err != nil {
			return Result{}, fmt.Errorf("publish: write search-index.json: %w", err)
		}
	}
	// .nojekyll tells GitHub Pages "do not run the legacy Jekyll build" so
	// folders/files starting with _ or . are served as-is. Always emit it
	// since it's a no-op on other hosts.
	if err := os.WriteFile(filepath.Join(cfg.OutputDir, ".nojekyll"), nil, 0o644); err != nil {
		return Result{}, fmt.Errorf("publish: write .nojekyll: %w", err)
	}

	// 404 page — GitHub Pages serves /404.html on any unknown URL,
	// other static hosts can be configured to do the same. Always
	// emit; the file is small (~2 KB) and there's no downside.
	{
		var nfBody bytes.Buffer
		if err := tpls.notFound.Execute(&nfBody, struct{ RelRoot string }{RelRoot: ""}); err != nil {
			return Result{}, fmt.Errorf("publish: 404 template: %w", err)
		}
		page := pageData{
			Lang: cfg.Lang, SiteTitle: cfg.SiteTitle, PageTitle: "Not found",
			Footer: cfg.Footer, RelRoot: "",
			NoIndex: true, // a 404 page should never appear in search results
		}
		if err := render(filepath.Join(cfg.OutputDir, "404.html"), nfBody.String(), page); err != nil {
			return Result{}, err
		}
	}

	// RSS feed — universally consumable, drives auto-discovery via
	// the <link rel="alternate"> we already emit on every page.
	feedCap := cfg.FeedItems
	if feedCap == 0 {
		feedCap = 50 // sensible default — readers don't need unbounded history
	}
	if err := writeRSSFeed(cfg.OutputDir, cfg.SiteURL, cfg.SiteTitle, cfg.Intro, regularNotes, feedCap); err != nil {
		return Result{}, fmt.Errorf("publish: rss feed: %w", err)
	}

	// Asset copying — anything non-markdown in the source folder
	// (images, PDFs, attachments) gets mirrored into the output so
	// markdown image refs and downloadable links keep working after
	// publish.
	assetCount, err := copyAssets(cfg.SourceDir, cfg.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("publish: assets: %w", err)
	}

	// Sitemap + robots — emitted even when SiteURL is empty (sitemap
	// falls back to path-only URLs which are less ideal but still
	// machine-readable; users can hand-edit afterwards).
	indexModTime := time.Now()
	if home := findHomepageNote(regularNotes, cfg.Homepage); home != nil && !home.ModTime.IsZero() {
		indexModTime = home.ModTime
	}
	sitemapEntries := sortedSitemapEntries(regularNotes, hasGraph, hasTags, tagSet, legalNotes, indexModTime)
	if err := writeSitemap(cfg.OutputDir, cfg.SiteURL, sitemapEntries); err != nil {
		return Result{}, fmt.Errorf("publish: sitemap: %w", err)
	}
	if err := writeRobots(cfg.OutputDir, cfg.SiteURL); err != nil {
		return Result{}, fmt.Errorf("publish: robots.txt: %w", err)
	}

	return Result{
		NotesPublished: len(regularNotes) + len(legalNotes),
		TagsPublished:  len(tagSet),
		AssetsCopied:   assetCount,
		OutputDir:      cfg.OutputDir,
	}, nil
}

// findHomepageNote locates a homepage note by relative path or filename
// match — same logic the index renderer uses, lifted into a helper for
// reuse by sitemap mtime computation.
func findHomepageNote(regular []*Note, homepage string) *Note {
	if homepage == "" {
		return nil
	}
	for _, n := range regular {
		if n.RelPath == homepage || filepath.Base(n.RelPath) == homepage {
			return n
		}
	}
	return nil
}

// loadNotes walks the source folder, skipping non-md and notes opted out via
// `publish: false` frontmatter.
func loadNotes(cfg Config) ([]*Note, error) {
	var out []*Note
	err := filepath.WalkDir(cfg.SourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".granit" || name == ".git" || name == ".obsidian" || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, _ := filepath.Rel(cfg.SourceDir, path)
		n, err := parseNote(rel, data, d)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		// Frontmatter-driven opt-out: publish: false hides the note even
		// when the user asked to publish the whole folder.
		if hasOptOut(data) {
			return nil
		}
		out = append(out, n)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("publish: walk: %w", err)
	}
	return out, nil
}

func parseNote(rel string, data []byte, d fs.DirEntry) (*Note, error) {
	body, frontmatter := splitFrontmatter(data)
	n := &Note{
		RelPath:  filepath.ToSlash(rel),
		Slug:     slugify(strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))),
		Body:     string(body),
		BodyText: stripMD(string(body)),
	}
	if fi, err := d.Info(); err == nil {
		n.ModTime = fi.ModTime()
	}
	// Title precedence: frontmatter title → first H1 → filename
	if t, ok := frontmatter["title"].(string); ok && strings.TrimSpace(t) != "" {
		n.Title = strings.TrimSpace(t)
	}
	if n.Title == "" {
		if h := firstH1(string(body)); h != "" {
			n.Title = h
		}
	}
	if n.Title == "" {
		n.Title = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	// Strip a leading H1 that duplicates the title — many notes lead
	// with `# Title` in the body and we already render the title from
	// the template, so without this strip every page shows its title
	// twice (once big from the template, once from the body H1).
	n.Body = stripLeadingH1IfTitle(n.Body, n.Title)
	// Date.
	if d, ok := frontmatter["date"].(string); ok {
		n.Date = strings.TrimSpace(d)
	} else if !n.ModTime.IsZero() {
		n.Date = n.ModTime.Format("2006-01-02")
	}
	// Tags from frontmatter (yaml list or comma-separated string) plus
	// inline #tag occurrences.
	tagSet := map[string]struct{}{}
	if raw, ok := frontmatter["tags"]; ok {
		switch v := raw.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					tagSet[strings.TrimSpace(s)] = struct{}{}
				}
			}
		case string:
			for _, s := range strings.Split(v, ",") {
				s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
				if s != "" {
					tagSet[s] = struct{}{}
				}
			}
		}
	}
	for _, m := range reInlineTag.FindAllStringSubmatch(string(body), -1) {
		tagSet[m[1]] = struct{}{}
	}
	for t := range tagSet {
		n.Tags = append(n.Tags, t)
	}
	sort.Strings(n.Tags)

	// SEO / legal extras from frontmatter.
	if a, ok := frontmatter["author"].(string); ok {
		n.Author = strings.TrimSpace(a)
	}
	if v, ok := frontmatter["noindex"]; ok {
		switch t := v.(type) {
		case bool:
			n.NoIndex = t
		case string:
			n.NoIndex = strings.EqualFold(strings.TrimSpace(t), "true")
		}
	}
	if img, ok := frontmatter["image"].(string); ok {
		n.OGImage = strings.TrimSpace(img)
	}
	n.Legal = detectLegalKind(rel, frontmatter)
	n.WordCount = countWords(n.BodyText)
	return n, nil
}

// containsMath returns true when the body has at least one inline
// `$...$` or display `$$...$$` math span. Used to decide whether to
// inject KaTeX on a per-page basis. Conservative — false positives
// (e.g. plain dollar amounts) are tolerable since KaTeX silently
// passes non-math through.
var (
	reInlineMath  = regexp.MustCompile(`\$[^\s$][^$\n]*\$`)
	reDisplayMath = regexp.MustCompile(`(?s)\$\$.+?\$\$`)
)

func containsMath(s string) bool {
	return reDisplayMath.MatchString(s) || reInlineMath.MatchString(s)
}

// reMermaidBlock matches goldmark's rendering of a fenced ```mermaid block
// in its entirety so we can rewrite the whole element to <pre class="mermaid">
// without disturbing other code blocks. (?s) makes . match newlines so a
// multi-line mermaid diagram is captured in one go.
var reMermaidBlock = regexp.MustCompile(`(?s)<pre><code class="language-mermaid">(.*?)</code></pre>`)

// hasOptOut returns true if the document's YAML frontmatter contains a
// `publish: false` directive. Cheap regex match avoids a full second-pass
// YAML parse — we already parsed it during parseNote, but caller hands
// raw bytes for the gate check before constructing the Note.
var rePublishFalse = regexp.MustCompile(`(?m)^publish:\s*false\s*$`)

func hasOptOut(data []byte) bool {
	body, _ := splitFrontmatter(data)
	if len(body) == len(data) {
		return false // no frontmatter
	}
	front := data[:len(data)-len(body)]
	return rePublishFalse.Match(front)
}

// splitFrontmatter pulls a YAML block delimited by --- at the start of the
// file, returning (bodyAfterFrontmatter, parsedFrontmatterMap). When no
// frontmatter is present, returns (data, empty map).
func splitFrontmatter(data []byte) ([]byte, map[string]interface{}) {
	out := map[string]interface{}{}
	// goldmark-meta does this for us during render, but we want frontmatter
	// before render so wikilink resolution can use the title field. So we
	// run a tiny in-house parser here.
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return data, out
	}
	rest := data[4:]
	if bytes.HasPrefix(data, []byte("---\r\n")) {
		rest = data[5:]
	}
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return data, out
	}
	front := rest[:end]
	body := rest[end+4:]
	body = bytes.TrimLeft(body, "\r\n")
	// Hand the front matter to goldmark-meta's underlying yaml parser
	// indirectly: we use a small reusable goldmark instance just to parse.
	// Cheaper path: use yaml.Unmarshal directly. We have yaml.v2 transitively
	// from goldmark-meta.
	if err := unmarshalYAML(front, &out); err != nil {
		// Tolerate broken frontmatter — return body without metadata
		// rather than failing the whole publish.
		return body, map[string]interface{}{}
	}
	return body, out
}

// reInlineTag matches "#tag-name" tokens used in the markdown body. Avoids
// matching inside URLs (we exclude when preceded by a non-space alphanumeric
// or punctuation that signals it's a URL fragment).
var reInlineTag = regexp.MustCompile(`(?m)(?:^|\s)#([a-zA-Z][\w-]*)`)

// firstH1 returns the text of the first level-1 heading in the body, or "".
var reFirstH1 = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// stripLeadingH1IfTitle removes the first H1 line from body when it matches
// the rendered note title (case-insensitive, whitespace-tolerant). Avoids
// the "title shown twice" effect when authors use `# Title` as the first
// body line and frontmatter/filename produces the same title.
func stripLeadingH1IfTitle(body, title string) string {
	lines := strings.SplitN(body, "\n", 4)
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			h := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			if strings.EqualFold(h, title) {
				// Drop this line and any blank line that follows it.
				rest := strings.Join(lines[i+1:], "\n")
				rest = strings.TrimLeft(rest, "\r\n")
				return strings.Join(append(lines[:i], rest), "\n")
			}
		}
		// Non-empty non-H1 line first — nothing to strip.
		return body
	}
	return body
}

func firstH1(body string) string {
	m := reFirstH1.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// firstSentence returns up to maxLen chars of the first sentence-ish chunk,
// used for index summaries and meta descriptions.
func firstSentence(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// First paragraph break = first \n\n or end of string.
	if i := strings.Index(s, "\n\n"); i > 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxLen {
		s = s[:maxLen-1] + "…"
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// stripMD returns a plain-text-ish version of markdown, used for search
// indexing and summaries. Coarse but cheap — not a full parser.
var (
	reCodeFence = regexp.MustCompile("(?s)```.*?```")
	reInlineRun = regexp.MustCompile("`[^`]+`")
	reMDLink    = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reWikiLink  = regexp.MustCompile(`\[\[([^\]|]+)(\|[^\]]+)?\]\]`)
	reHTML      = regexp.MustCompile(`<[^>]+>`)
)

func stripMD(s string) string {
	s = reCodeFence.ReplaceAllString(s, " ")
	s = reInlineRun.ReplaceAllString(s, " ")
	s = reMDLink.ReplaceAllString(s, "$1")
	s = reWikiLink.ReplaceAllString(s, "$1")
	s = reHTML.ReplaceAllString(s, " ")
	s = strings.NewReplacer(
		"#", "", "*", "", "_", "", ">", "", "`", "",
	).Replace(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// slugify turns a filename or title into a URL-safe slug.
var reSlugSep = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = reSlugSep.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}

// newMarkdown constructs the goldmark renderer with the extensions we need:
// GFM (tables, autolinks, strikethrough, task lists), frontmatter (so any
// frontmatter inside the body is dropped from output), code highlighting
// via chroma (b&w-friendly), unsafe HTML (so users can drop in raw HTML
// deliberately).
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			highlighting.NewHighlighting(
				highlighting.WithStyle("bw"),
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(false),
					chromahtml.TabWidth(2),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
}

// prevNextRef is the per-note neighbouring pair, computed once and indexed
// by note position.
type prevNextRef struct {
	prev, next *Note
}

// buildPrevNext returns prev/next neighbours for each note in the slice.
// Index ordering is the source-walk order (effectively filename order),
// which gives a sensible "next note" sequence when authors prefix files
// with 00_, 01_, ... — common convention for ordered research folders.
func buildPrevNext(notes []*Note) []prevNextRef {
	out := make([]prevNextRef, len(notes))
	for i := range notes {
		if i > 0 {
			out[i].prev = notes[i-1]
		}
		if i < len(notes)-1 {
			out[i].next = notes[i+1]
		}
	}
	return out
}

// pageData is the wrapper handed to the base template.
type pageData struct {
	Lang         string
	SiteTitle    string
	PageTitle    string
	Description  string
	Footer       string
	Branding     template.HTML // "Built with Granit" link, or empty when --no-branding
	RelRoot      string
	HasTags      bool
	HasGraph     bool
	Search       bool

	// SEO
	CanonicalURL string      // absolute URL (or empty when SiteURL unset)
	OGType       string      // "website" for index/tags/graph; "article" for note pages
	OGImage      string      // per-note og:image absolute URL (or empty)
	OGImageType  string      // "image/png" or "image/jpeg" — set when OGImage is non-empty
	NoIndex      bool        // emits <meta name="robots" content="noindex">
	JSONLD       template.JS // JSON-LD structured data block (Article schema for notes)
	Author       string      // <meta name="author">
	FeedLink     template.HTML // <link rel="alternate" type="application/rss+xml"> for feed auto-discovery

	// Legal / cookie
	HasImpressum   bool   // adds Impressum link to footer
	HasDatenschutz bool   // adds Datenschutz link to footer
	ImpressumURL   string // relative URL from the current page
	DatenschutzURL string // relative URL from the current page
	CookieBanner   bool   // emit the banner markup + JS
	CookieMessage  template.HTML

	// Math + Mermaid (per-page so the script tags only appear where needed)
	HasMath    bool
	HasMermaid bool

	Body template.HTML
}

type indexData struct {
	SiteTitle string
	Intro     string
	Search    bool
	Notes     []noteRef
}

type noteData struct {
	Title       string
	Date        string
	ReadingTime string // "5 min read" or "" for short notes
	Tags        []tagRef
	Content     template.HTML
	Outline     []outlineEntry
	Backlinks   []noteRef
	Prev        *noteRef
	Next        *noteRef
	RelRoot     string
}

type graphData struct {
	NoteCount int
	EdgeCount int
	SVG       template.HTML
}

type tagsIndexData struct{ Tags []tagRef }

type tagPageData struct {
	Name    string
	Notes   []noteRef
	RelRoot string
}

type tagRef struct {
	Name  string
	Slug  string
	Count int
}

type noteRef struct {
	Title   string
	URL     string
	Summary string
}

type searchDoc struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Body  string `json:"body"`
}
