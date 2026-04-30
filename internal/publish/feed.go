package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// writeRSSFeed emits a standards-conformant RSS 2.0 feed at /feed.xml
// listing the most recent regular (non-legal, non-noindex) notes
// ordered by frontmatter date desc. RSS over Atom because RSS readers
// are more universal — most JSON Feed-aware readers also handle RSS,
// and most purely-RSS readers don't grok Atom.
//
// maxItems caps the count of <item> entries; 0 means "include every
// note". Default callers should pass 50 — RSS readers do not benefit
// from unbounded history and the channel <ttl> + lastBuildDate are
// what drive periodic refresh anyway.
//
// SiteURL is required for full <link>/<guid> URLs. When unset, we still
// emit the feed but with relative URLs (some readers tolerate this,
// most don't — better than nothing for local preview).
//
// Description per item is the first sentence/summary of the note,
// matching what we already use for index summaries and meta tags.
func writeRSSFeed(outputDir, siteURL, siteTitle, intro string, notes []*Note, maxItems int) error {
	// Sort newest-first by date string then by ModTime — stable
	// across builds when dates haven't changed.
	sorted := append([]*Note(nil), notes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Date != sorted[j].Date {
			return sorted[i].Date > sorted[j].Date
		}
		return sorted[i].ModTime.After(sorted[j].ModTime)
	})

	now := time.Now().UTC()
	link := siteURL
	if link == "" {
		link = "/"
	}
	link = strings.TrimRight(link, "/") + "/"

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">` + "\n")
	b.WriteString("<channel>\n")
	b.WriteString("  <title>" + xmlEscape(siteTitle) + "</title>\n")
	b.WriteString("  <link>" + xmlEscape(link) + "</link>\n")
	if intro != "" {
		b.WriteString("  <description>" + xmlEscape(intro) + "</description>\n")
	} else {
		b.WriteString("  <description>" + xmlEscape(siteTitle) + "</description>\n")
	}
	b.WriteString("  <language>en</language>\n")
	b.WriteString("  <lastBuildDate>" + now.Format(time.RFC1123Z) + "</lastBuildDate>\n")
	if siteURL != "" {
		b.WriteString(`  <atom:link href="` + xmlEscape(strings.TrimRight(siteURL, "/")+"/feed.xml") + `" rel="self" type="application/rss+xml"/>` + "\n")
	}

	emitted := 0
	for _, n := range sorted {
		if n.NoIndex || n.Legal != "" {
			continue
		}
		if maxItems > 0 && emitted >= maxItems {
			break
		}
		emitted++
		itemURL := link + n.OutputPath
		b.WriteString("  <item>\n")
		b.WriteString("    <title>" + xmlEscape(n.Title) + "</title>\n")
		b.WriteString("    <link>" + xmlEscape(itemURL) + "</link>\n")
		b.WriteString("    <guid isPermaLink=\"true\">" + xmlEscape(itemURL) + "</guid>\n")
		desc := firstSentence(n.BodyText, 280)
		if desc != "" {
			b.WriteString("    <description>" + xmlEscape(desc) + "</description>\n")
		}
		// pubDate parsed from frontmatter date → RFC1123Z. Falls back
		// to file mtime when frontmatter date is empty.
		var pub time.Time
		if t, err := time.Parse("2006-01-02", n.Date); err == nil {
			pub = t
		} else if !n.ModTime.IsZero() {
			pub = n.ModTime
		} else {
			pub = now
		}
		b.WriteString("    <pubDate>" + pub.UTC().Format(time.RFC1123Z) + "</pubDate>\n")
		if n.Author != "" {
			// RSS spec requires email-format author; many feeds just
			// use a name. We follow the relaxed convention.
			b.WriteString("    <author>" + xmlEscape(n.Author) + "</author>\n")
		}
		for _, tag := range n.Tags {
			b.WriteString("    <category>" + xmlEscape(tag) + "</category>\n")
		}
		b.WriteString("  </item>\n")
	}

	b.WriteString("</channel>\n")
	b.WriteString("</rss>\n")

	return os.WriteFile(filepath.Join(outputDir, "feed.xml"), []byte(b.String()), 0o644)
}

// rssAutoLink returns the HTML <link> tag that goes in the document
// head so RSS readers auto-discover the feed. Empty when no SiteURL
// is set (relative feed.xml still works in some readers, but the
// auto-discovery <link> wants an absolute href to be reliably picked
// up by Feedly/Inoreader/etc).
func rssAutoLink(siteURL string) string {
	href := "feed.xml"
	if siteURL != "" {
		href = strings.TrimRight(siteURL, "/") + "/feed.xml"
	}
	return fmt.Sprintf(`<link rel="alternate" type="application/rss+xml" title="RSS Feed" href="%s">`, href)
}
