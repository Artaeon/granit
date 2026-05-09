package books

import (
	"encoding/xml"
	"path"
	"strings"
)

// parseNav handles EPUB3 nav.xhtml. The interesting node is
// <nav epub:type="toc"><ol>…</ol></nav>; we walk its <li><a> rows
// and resolve each href back to a spine index.
//
// hrefIndex maps "manifest href" → spine index (with and without
// fragment). The TOC anchors typically point to "Text/ch01.xhtml"
// or "ch01.xhtml#sec2" — we strip the fragment when matching.
func parseNav(body []byte, hrefIndex map[string]int) []TOCEntry {
	var doc navDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	for _, n := range doc.Body.Nav {
		if n.EpubType == "toc" || n.EpubType == "" {
			return walkNavList(n.OL.Items, hrefIndex)
		}
	}
	// Some EPUB3s omit epub:type — fall back to the first nav we find.
	for _, n := range doc.Body.Nav {
		return walkNavList(n.OL.Items, hrefIndex)
	}
	return nil
}

func walkNavList(items []navLI, hrefIndex map[string]int) []TOCEntry {
	out := make([]TOCEntry, 0, len(items))
	for _, li := range items {
		entry := TOCEntry{Title: strings.TrimSpace(li.A.Text)}
		// A nav row may have multiple href forms — try as-is then
		// stripped of fragment.
		raw := li.A.Href
		if raw == "" {
			entry.SpineIdx = -1
		} else {
			candidates := []string{raw, strings.SplitN(raw, "#", 2)[0]}
			entry.SpineIdx = -1
			for _, c := range candidates {
				if idx, ok := hrefIndex[path.Clean(c)]; ok {
					entry.SpineIdx = idx
					break
				}
			}
		}
		if len(li.OL.Items) > 0 {
			entry.Children = walkNavList(li.OL.Items, hrefIndex)
		}
		// Skip entries with no resolvable target AND no children —
		// they'd render as dead links.
		if entry.SpineIdx < 0 && len(entry.Children) == 0 {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// parseNCX handles EPUB2 toc.ncx. Schema is similar but uses
// navMap/navPoint/content@src instead of nav/ol/li/a@href.
func parseNCX(body []byte, hrefIndex map[string]int) []TOCEntry {
	var doc ncxDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	return walkNavPoints(doc.NavMap.NavPoint, hrefIndex)
}

func walkNavPoints(pts []ncxNavPoint, hrefIndex map[string]int) []TOCEntry {
	out := make([]TOCEntry, 0, len(pts))
	for _, p := range pts {
		entry := TOCEntry{Title: strings.TrimSpace(p.NavLabel.Text)}
		raw := p.Content.Src
		if raw == "" {
			entry.SpineIdx = -1
		} else {
			candidates := []string{raw, strings.SplitN(raw, "#", 2)[0]}
			entry.SpineIdx = -1
			for _, c := range candidates {
				if idx, ok := hrefIndex[path.Clean(c)]; ok {
					entry.SpineIdx = idx
					break
				}
			}
		}
		if len(p.NavPoint) > 0 {
			entry.Children = walkNavPoints(p.NavPoint, hrefIndex)
		}
		if entry.SpineIdx < 0 && len(entry.Children) == 0 {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// nav.xhtml schema — just the slice we read.
type navDoc struct {
	XMLName xml.Name `xml:"html"`
	Body    struct {
		Nav []struct {
			EpubType string `xml:"type,attr"`
			OL       navOL  `xml:"ol"`
		} `xml:"nav"`
	} `xml:"body"`
}

type navOL struct {
	Items []navLI `xml:"li"`
}

type navLI struct {
	A struct {
		Href string `xml:"href,attr"`
		Text string `xml:",chardata"`
	} `xml:"a"`
	OL navOL `xml:"ol"`
}

// toc.ncx schema (subset we actually use).
type ncxDoc struct {
	XMLName xml.Name `xml:"ncx"`
	NavMap  struct {
		NavPoint []ncxNavPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

type ncxNavPoint struct {
	NavLabel struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	NavPoint []ncxNavPoint `xml:"navPoint"`
}
