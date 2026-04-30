package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/publish"
)

// runPublish wires `granit publish` to the internal/publish package.
//
// Subcommands:
//
//	granit publish build [folder] [flags]   — render to ./dist (or --output)
//	granit publish preview [folder] [flags] — build + serve on :8080
//	granit publish init [folder]            — write .granit/publish.json
//
// Flags:
//
//	--output <dir>     output directory (default ./dist)
//	--title <name>     site title (default folder name)
//	--homepage <file>  relative path of the note that becomes index.html
//	--no-search        skip search index + JS
//	--config <path>    load config from file (defaults to .granit/publish.json
//	                   relative to the source folder, if it exists)
//
// Designed for static hosting on GitHub Pages: output is plain HTML+CSS,
// drops a .nojekyll file so files starting with `_` work, all internal
// links are relative so the site works at any subpath
// (`username.github.io` OR `username.github.io/repo-name/`).
func runPublish(args []string) {
	if len(args) == 0 {
		printPublishUsage()
		os.Exit(1)
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "build":
		runPublishBuild(rest)
	case "preview":
		runPublishPreview(rest)
	case "init":
		runPublishInit(rest)
	case "help", "--help", "-h":
		printPublishUsage()
	default:
		fmt.Fprintf(os.Stderr, "publish: unknown subcommand %q\n\n", sub)
		printPublishUsage()
		os.Exit(1)
	}
}

func printPublishUsage() {
	fmt.Println(`granit publish — render a folder to a static black-and-white site
  Output: plain HTML + one CSS file + a small vanilla-JS search shim.
  No Node.js, no React, no build step. Deployable anywhere static files
  are served (GitHub Pages, Cloudflare Pages, fleetdeck, S3, ...).

USAGE
  granit publish build [folder] [flags]     Build site to ./dist
  granit publish preview [folder] [flags]   Build + serve on http://localhost:8080
  granit publish init [folder]              Write .granit/publish.json template

FLAGS
  --output, -o <dir>  Output directory (default ./dist)
  --title <name>      Site title (default: folder basename)
  --homepage <file>   Note path (relative to folder) to use as index.html
                      e.g. --homepage README.md, --homepage _Index.md
  --site-url <url>    Public root URL (e.g. https://notes.example.com).
                      Enables canonical links, og:url, absolute sitemap.
  --author <name>     Sets <meta name="author"> + JSON-LD author per page
  --cookie-banner     Emit a small bottom cookie banner (off by default)
  --no-branding       Hide the small "Built with Granit" link in the footer
  --hero              Use the hero homepage layout (big title + 3-col card grid)
  --auto-og           Auto-generate a 1200x630 PNG og:image per note
  --og-image <path>   Site-wide default og:image (relative path in source folder)
  --math              Render LaTeX math via KaTeX (loaded from CDN, opt-in)
  --mermaid           Render Mermaid diagram code blocks (loaded from CDN, opt-in)
  --no-search         Skip search index + JS shim (~5 KB savings)
  --config <path>     Load config from this JSON file
                      (default: <folder>/.granit/publish.json if present)

WHAT YOU GET
  index.html              Auto note list, OR your homepage note
  notes/<slug>.html       One page per note, with Contents outline,
                          backlinks panel, prev/next navigation
  graph.html              Force-directed SVG of the wikilink graph
                          (deterministic, JS-free, clickable nodes)
  tags/index.html         All tags with note counts
  tags/<tag>.html         Notes for each tag
  impressum.html          AUTO if a note named impressum.md / imprint.md OR
                          frontmatter "legal: impressum" is present
  datenschutz.html        AUTO if a note named datenschutz.md / privacy.md OR
                          frontmatter "legal: datenschutz" is present
  sitemap.xml             Standards-conformant XML sitemap of all pages
  robots.txt              Allows everything; references the sitemap
  search-index.json       Title + body excerpt per note (excludes legal)
  search.js               ~30 lines of vanilla ES5 fuzzy filter
  style.css               B&W theme, light + dark modes, mobile-responsive
  .nojekyll               GitHub-Pages-friendly (serves files starting with _)

SEO META PER PAGE
  • <title>, <meta description>, <link rel="canonical">
  • Open Graph (og:title, og:description, og:type, og:url, og:site_name)
  • Twitter Card (summary)
  • <meta name="author"> when --author or frontmatter author: is set
  • <meta name="robots" content="noindex"> for legal pages and any note
    with frontmatter "noindex: true"
  • JSON-LD Article schema on every note page (search-engine rich results)

FRONTMATTER (per-note YAML at top of file)
  title: ...              Overrides H1/filename title fallback
  date: 2026-04-08        Shown under the title; sorts the index
  tags: [a, b, c]         Array OR "a, b, c" comma string; merged with inline #tags
  author: Jane Doe        Per-note author (overrides --author / config)
  publish: false          Hides this note even when the folder is published
  noindex: true           Excludes from sitemap; emits robots noindex meta
  legal: impressum        Forces this note to render at /impressum.html
  legal: datenschutz      Forces this note to render at /datenschutz.html

WIKILINKS
  [[Note Name]]           Resolves to that note's published HTML page
  [[Note Name|Display]]   Custom link text
  [[Note#section]]        Preserves the anchor target

EXAMPLES
  granit publish build ~/Notes/Research
                          Build with the folder name as the site title
  granit publish build ~/Notes/Research --title "My Research" --homepage 00_index.md
                          Use 00_index.md as the homepage; custom title
  granit publish preview ~/Notes
                          Build + open a local server on :8080
  granit publish init ~/Notes/Research
                          Drop .granit/publish.json with sensible defaults to edit

GITHUB PAGES WORKFLOW (recommended for getting live in 60 seconds)
  cd ~/your-repo
  granit publish build ~/Notes/Research --output ./docs --title "Research"
  git add docs/ && git commit -m "publish notes" && git push
  # Repo Settings → Pages → Source: Deploy from a branch / docs
  # Site is live at https://<user>.github.io/<repo>/

CLOUDFLARE PAGES / NETLIFY / S3 / VPS
  granit publish build ./Notes --output ./dist
  npx wrangler pages deploy ./dist           # Cloudflare Pages
  netlify deploy --prod --dir ./dist         # Netlify
  aws s3 sync ./dist s3://my-bucket --delete # AWS S3
  rsync -avz --delete ./dist/ user@host:/var/www/notes/   # any VPS

FULL DOCUMENTATION
  docs/PUBLISH.md in the granit repository covers theming, custom domains,
  GitHub Actions auto-rebuild, troubleshooting, and the roadmap.`)
}

// parsePublishFlags peels the source folder (first non-flag positional) and
// the recognised flags from args. Anything unrecognised triggers a usage
// error so typos surface fast.
type publishFlags struct {
	folder         string
	output         string
	title          string
	homepage       string
	noSearch       bool
	config         string
	siteURL        string
	author         string
	cookieBanner   bool
	noBranding     bool
	hero           bool
	autoOG         bool
	defaultOG      string
	math           bool
	mermaid        bool
}

func parsePublishFlags(args []string) (*publishFlags, error) {
	pf := &publishFlags{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--output" || a == "-o":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--output requires a value")
			}
			pf.output = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--output="):
			pf.output = strings.TrimPrefix(a, "--output=")
			i++
		case a == "--title":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--title requires a value")
			}
			pf.title = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--title="):
			pf.title = strings.TrimPrefix(a, "--title=")
			i++
		case a == "--homepage":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--homepage requires a value")
			}
			pf.homepage = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--homepage="):
			pf.homepage = strings.TrimPrefix(a, "--homepage=")
			i++
		case a == "--no-search":
			pf.noSearch = true
			i++
		case a == "--cookie-banner":
			pf.cookieBanner = true
			i++
		case a == "--no-branding":
			pf.noBranding = true
			i++
		case a == "--hero":
			pf.hero = true
			i++
		case a == "--auto-og":
			pf.autoOG = true
			i++
		case a == "--math":
			pf.math = true
			i++
		case a == "--mermaid":
			pf.mermaid = true
			i++
		case a == "--og-image":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--og-image requires a value")
			}
			pf.defaultOG = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--og-image="):
			pf.defaultOG = strings.TrimPrefix(a, "--og-image=")
			i++
		case a == "--site-url":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--site-url requires a value")
			}
			pf.siteURL = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--site-url="):
			pf.siteURL = strings.TrimPrefix(a, "--site-url=")
			i++
		case a == "--author":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--author requires a value")
			}
			pf.author = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--author="):
			pf.author = strings.TrimPrefix(a, "--author=")
			i++
		case a == "--config":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--config requires a value")
			}
			pf.config = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--config="):
			pf.config = strings.TrimPrefix(a, "--config=")
			i++
		case strings.HasPrefix(a, "-"):
			return nil, fmt.Errorf("unknown flag: %s", a)
		default:
			if pf.folder == "" {
				pf.folder = a
			} else {
				return nil, fmt.Errorf("unexpected positional argument: %s", a)
			}
			i++
		}
	}
	if pf.folder == "" {
		pf.folder = "."
	}
	return pf, nil
}

// resolveConfig builds the effective publish.Config from disk + flags. CLI
// flags override config-file values which override defaults.
func resolveConfig(pf *publishFlags) (publish.Config, error) {
	cfg := publish.Config{}

	configPath := pf.config
	if configPath == "" {
		// Conventional location: .granit/publish.json beside the source folder.
		guess := filepath.Join(pf.folder, ".granit", "publish.json")
		if _, err := os.Stat(guess); err == nil {
			configPath = guess
		}
	}
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return cfg, fmt.Errorf("read config %s: %w", configPath, err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", configPath, err)
		}
	}

	cfg.SourceDir = pf.folder
	if pf.output != "" {
		cfg.OutputDir = pf.output
	}
	if pf.title != "" {
		cfg.SiteTitle = pf.title
	}
	if pf.homepage != "" {
		cfg.Homepage = pf.homepage
	}
	if cfg.SiteTitle == "" {
		// Default title: folder basename, prettified.
		base := filepath.Base(pf.folder)
		cfg.SiteTitle = base
	}
	if pf.siteURL != "" {
		cfg.SiteURL = pf.siteURL
	}
	if pf.author != "" {
		cfg.Author = pf.author
	}
	if pf.cookieBanner {
		cfg.CookieBanner = true
	}
	if pf.noBranding {
		cfg.NoBranding = true
	}
	if pf.hero {
		cfg.HomepageStyle = "hero"
	}
	if pf.autoOG {
		cfg.AutoOGImage = true
	}
	if pf.defaultOG != "" {
		cfg.DefaultOGImage = pf.defaultOG
	}
	if pf.math {
		cfg.Math = true
	}
	if pf.mermaid {
		cfg.Mermaid = true
	}
	// CLI defaults that the package's applyDefaults intentionally leaves
	// alone so an explicit user `false` survives. The flag inverts to
	// no-search; everything else defaults to "include and search".
	cfg.Search = !pf.noSearch
	cfg.IncludeUnlisted = true
	return cfg, nil
}

func runPublishBuild(args []string) {
	pf, err := parsePublishFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n\n", err)
		printPublishUsage()
		os.Exit(1)
	}
	cfg, err := resolveConfig(pf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		os.Exit(1)
	}

	// Honor --no-search by patching applyDefaults' force-on. We stamp it
	// AFTER the defaults run by re-checking the flag inside Build via a
	// thin wrapper. For now, do it here and trust applyDefaults to leave
	// non-default values alone — package needs a small fix to honor false.
	res, err := publish.Build(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		os.Exit(1)
	}
	abs, _ := filepath.Abs(res.OutputDir)
	fmt.Printf("✓ Published %d notes (%d tags, %d assets) → %s\n",
		res.NotesPublished, res.TagsPublished, res.AssetsCopied, abs)
	if !pf.noSearch {
		fmt.Println("  Search index: search-index.json + search.js")
	}
	fmt.Println("  RSS feed: feed.xml  ·  Sitemap: sitemap.xml  ·  Robots: robots.txt")
	fmt.Println("  GitHub Pages: commit the output dir, enable Pages on it.")
}

func runPublishPreview(args []string) {
	pf, err := parsePublishFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n\n", err)
		printPublishUsage()
		os.Exit(1)
	}
	cfg, err := resolveConfig(pf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		os.Exit(1)
	}
	res, err := publish.Build(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		os.Exit(1)
	}
	abs, _ := filepath.Abs(res.OutputDir)
	fmt.Printf("✓ Built %d notes → %s\n", res.NotesPublished, abs)
	fmt.Printf("→ Serving on http://localhost:8080  (Ctrl+C to stop)\n")
	if err := http.ListenAndServe(":8080", http.FileServer(http.Dir(res.OutputDir))); err != nil {
		fmt.Fprintf(os.Stderr, "publish preview: %v\n", err)
		os.Exit(1)
	}
}

func runPublishInit(args []string) {
	pf, err := parsePublishFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n\n", err)
		printPublishUsage()
		os.Exit(1)
	}
	cfgDir := filepath.Join(pf.folder, ".granit")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "publish: mkdir %s: %v\n", cfgDir, err)
		os.Exit(1)
	}
	cfgPath := filepath.Join(cfgDir, "publish.json")
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("publish: %s already exists, leaving it alone.\n", cfgPath)
		return
	}
	defaults := publish.Config{
		SiteTitle: filepath.Base(pf.folder),
		Intro:     "",
		Footer:    "Built with Granit",
		OutputDir: "./dist",
		Homepage:  "",
		Search:    true,
	}
	data, _ := json.MarshalIndent(defaults, "", "  ")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "publish: write %s: %v\n", cfgPath, err)
		os.Exit(1)
	}
	fmt.Printf("✓ Wrote %s — edit it, then run 'granit publish build %s'\n", cfgPath, pf.folder)
}
