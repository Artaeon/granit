package publish

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// detectLegalKind returns "impressum", "datenschutz", or "" based on
// filename + frontmatter heuristics. Both German legal pages get
// elevated to root-level URLs (/impressum.html, /datenschutz.html) so
// they're easy to link from the footer and don't get buried under
// /notes/.
//
// Detection order:
//  1. Frontmatter `legal: impressum` (or datenschutz / privacy / imprint)
//  2. Filename match: impressum.md, datenschutz.md, privacy.md,
//     imprint.md (case-insensitive, ignoring extension).
//
// Returns the canonical kind name ("impressum" or "datenschutz");
// "privacy" and "imprint" map to their German equivalents so a single
// detection runs the same downstream code regardless of whether the
// vault is German- or English-language.
func detectLegalKind(rel string, frontmatter map[string]interface{}) string {
	if v, ok := frontmatter["legal"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "impressum", "imprint":
			return "impressum"
		case "datenschutz", "privacy", "privacy-policy":
			return "datenschutz"
		}
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)))
	switch base {
	case "impressum", "imprint":
		return "impressum"
	case "datenschutz", "privacy", "privacy-policy":
		return "datenschutz"
	}
	return ""
}

// estimateReadingTime returns a "N min read" string for a body word
// count, using the 220 wpm convention (between conservative 200 and
// "fast reader" 250). Returns "" for very short bodies (<100 words)
// where the chip would be noise.
func estimateReadingTime(words int) string {
	if words < 100 {
		return ""
	}
	mins := words / 220
	if mins < 1 {
		mins = 1
	}
	if mins == 1 {
		return "1 min read"
	}
	return fmt.Sprintf("%d min read", mins)
}

// countWords approximates a word count from plain-text body. Splits on
// whitespace; not language-aware but close enough for reading-time
// estimates in mixed German/English content.
func countWords(s string) int {
	return len(strings.Fields(s))
}

// articleJSONLD builds an Article-schema JSON-LD blob for a note page,
// returned ready to embed inside <script type="application/ld+json">.
// Search engines (Google, Bing) read this for rich-result eligibility
// and authoritative metadata extraction.
//
// Returns empty when SiteURL is unset — without an absolute base, the
// `url` and `mainEntityOfPage` fields would point to nowhere useful.
//
// Returned as template.JS (not template.HTML) because Go's html/template
// applies JavaScript-string escaping inside <script> elements — using
// template.HTML would wrap the JSON in quotes and backslash-escape every
// internal quote, which breaks the structured-data parser. JSON is also
// valid JavaScript-object-literal syntax, so template.JS embeds it raw.
func articleJSONLD(n *Note, siteURL, siteTitle, author string) template.JS {
	if siteURL == "" {
		return ""
	}
	canonical := strings.TrimRight(siteURL, "/") + "/" + n.OutputPath
	doc := map[string]interface{}{
		"@context":         "https://schema.org",
		"@type":            "Article",
		"headline":         n.Title,
		"name":             n.Title,
		"url":              canonical,
		"mainEntityOfPage": canonical,
	}
	if n.Date != "" {
		doc["datePublished"] = n.Date
	}
	if !n.ModTime.IsZero() {
		doc["dateModified"] = n.ModTime.UTC().Format(time.RFC3339)
	}
	if a := firstNonEmpty(n.Author, author); a != "" {
		doc["author"] = map[string]interface{}{
			"@type": "Person",
			"name":  a,
		}
	}
	if siteTitle != "" {
		doc["publisher"] = map[string]interface{}{
			"@type": "Organization",
			"name":  siteTitle,
		}
	}
	if d := firstSentence(n.BodyText, 200); d != "" {
		doc["description"] = d
	}
	buf, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	return template.JS(buf)
}

// firstNonEmpty returns the first non-empty string from the args.
func firstNonEmpty(args ...string) string {
	for _, s := range args {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// canonicalFor returns the absolute URL for a publish-time path under
// SiteURL. Returns "" when SiteURL is unset (the template skips the
// canonical/og:url tags in that case).
func canonicalFor(siteURL, outputPath string) string {
	if siteURL == "" {
		return ""
	}
	return strings.TrimRight(siteURL, "/") + "/" + strings.TrimLeft(outputPath, "/")
}

// writeSitemap writes a standards-conformant sitemap.xml listing every
// public page (notes excluding noindex, plus the index, graph, tag
// pages). Sitemaps require absolute URLs — we still emit a path-only
// version when SiteURL is unset so the file is at least syntactically
// useful for hand-editing later.
//
// The sitemap excludes the search index and any auxiliary JSON files —
// only the pages a human would land on.
type sitemapEntry struct {
	Path    string // outputPath relative to site root
	ModTime time.Time
}

func writeSitemap(outputDir, siteURL string, entries []sitemapEntry) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, e := range entries {
		b.WriteString("  <url>\n")
		var loc string
		if siteURL != "" {
			loc = strings.TrimRight(siteURL, "/") + "/" + strings.TrimLeft(e.Path, "/")
		} else {
			loc = "/" + strings.TrimLeft(e.Path, "/")
		}
		b.WriteString("    <loc>" + xmlEscape(loc) + "</loc>\n")
		if !e.ModTime.IsZero() {
			b.WriteString("    <lastmod>" + e.ModTime.UTC().Format("2006-01-02") + "</lastmod>\n")
		}
		b.WriteString("  </url>\n")
	}
	b.WriteString(`</urlset>` + "\n")
	return os.WriteFile(filepath.Join(outputDir, "sitemap.xml"), []byte(b.String()), 0o644)
}

// writeRobots writes a robots.txt that allows everything and points at
// the sitemap. Bare-bones intentional — for nuanced crawl rules the
// user can override the file after build (the publish command never
// re-touches existing files outside of .html / .css / .js / .json
// outputs unless rebuilt).
func writeRobots(outputDir, siteURL string) error {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	b.WriteString("Allow: /\n")
	if siteURL != "" {
		b.WriteString("Sitemap: " + strings.TrimRight(siteURL, "/") + "/sitemap.xml\n")
	}
	return os.WriteFile(filepath.Join(outputDir, "robots.txt"), []byte(b.String()), 0o644)
}

// xmlEscape — minimum entity-escaping for sitemap URLs. Sitemap spec
// requires ampersands, quotes, angle brackets, and apostrophes encoded
// inside <loc> values.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"'", "&apos;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}

// sortedSitemapEntries gathers every public page from a publish run
// and returns it sorted by path so the sitemap stays diff-stable
// across builds.
func sortedSitemapEntries(notes []*Note, hasGraph, hasTags bool, tagSet map[string][]*Note, legalNotes []*Note, indexModTime time.Time) []sitemapEntry {
	var out []sitemapEntry
	out = append(out, sitemapEntry{Path: "index.html", ModTime: indexModTime})
	if hasGraph {
		out = append(out, sitemapEntry{Path: "graph.html", ModTime: indexModTime})
	}
	for _, n := range notes {
		if n.NoIndex {
			continue
		}
		if n.Legal != "" {
			continue // emitted with the legal pages below
		}
		out = append(out, sitemapEntry{Path: n.OutputPath, ModTime: n.ModTime})
	}
	for _, n := range legalNotes {
		out = append(out, sitemapEntry{Path: n.OutputPath, ModTime: n.ModTime})
	}
	if hasTags {
		out = append(out, sitemapEntry{Path: "tags/index.html", ModTime: indexModTime})
		tagNames := make([]string, 0, len(tagSet))
		for t := range tagSet {
			tagNames = append(tagNames, t)
		}
		sort.Strings(tagNames)
		for _, t := range tagNames {
			out = append(out, sitemapEntry{Path: path.Join("tags", slugify(t)+".html"), ModTime: indexModTime})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
