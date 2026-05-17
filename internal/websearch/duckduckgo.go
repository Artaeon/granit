package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/artaeon/granit/internal/textutil"
)

// DuckDuckGo is the default provider: no API key needed (the user
// already opted in by enabling web_search). We try the Instant Answer
// JSON endpoint first because it gives clean structured results, then
// fall back to scraping the html.duckduckgo.com lite endpoint when
// Instant Answer comes up empty (which it does for most "ordinary"
// queries — Instant Answer is intended for definitions, calculations,
// known entities).
//
// Both endpoints are public; neither requires a key. Both can rate-
// limit aggressively if abused, which is why the tool defaults to
// max=5 per call and the agent runs once per ReAct step.
type DuckDuckGo struct {
	client *http.Client
	// instantAnswerURL is the Instant Answer JSON endpoint. Exposed
	// for tests to point at an httptest.NewServer; production users
	// never touch this.
	instantAnswerURL string
	// htmlSearchURL is the lite scrape endpoint. Same test seam.
	htmlSearchURL string
}

// NewDuckDuckGo constructs the default-config DuckDuckGo provider.
// client may be nil; we fall back to DefaultClient.
func NewDuckDuckGo(client *http.Client) *DuckDuckGo {
	if client == nil {
		client = DefaultClient()
	}
	return &DuckDuckGo{
		client:           client,
		instantAnswerURL: "https://api.duckduckgo.com/",
		htmlSearchURL:    "https://html.duckduckgo.com/html/",
	}
}

// Name implements SearchProvider.
func (d *DuckDuckGo) Name() string { return "duckduckgo" }

// Search runs the query, preferring Instant Answer's structured
// JSON. When that yields no usable hits (the common case for any
// query that isn't a known-entity lookup), it scrapes the HTML
// lite endpoint as a fallback.
//
// Order is deliberate: JSON-first because parsing it is trivial and
// stable, HTML-fallback because the layout of html.duckduckgo.com
// changes infrequently but isn't a contract. If the scrape ever
// breaks, the JSON path still works for at-a-glance lookups.
func (d *DuckDuckGo) Search(ctx context.Context, query string, max int) ([]Result, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("duckduckgo: empty query")
	}
	if max <= 0 {
		max = 5
	}
	if max > 10 {
		max = 10
	}
	// Try Instant Answer first — it's the canonical "did DDG already
	// know the answer?" path. For most real queries we'll fall
	// through to HTML.
	hits, err := d.instantAnswer(ctx, q, max)
	if err == nil && len(hits) > 0 {
		return hits, nil
	}
	return d.htmlSearch(ctx, q, max)
}

// instantAnswer hits the public api.duckduckgo.com Instant Answer
// JSON endpoint. The endpoint sometimes returns useful RelatedTopics
// even when the headline AbstractText is empty; we mine both. When
// neither is present (the common case), returns (nil, nil) and the
// caller falls back to HTML.
func (d *DuckDuckGo) instantAnswer(ctx context.Context, query string, max int) ([]Result, error) {
	u := d.instantAnswerURL + "?" + url.Values{
		"q":             {query},
		"format":        {"json"},
		"no_redirect":   {"1"},
		"no_html":       {"1"},
		"skip_disambig": {"1"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: build request: %w", err)
	}
	setUA(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: instant answer: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, debugErr("duckduckgo", resp.StatusCode, string(body))
	}
	var ia struct {
		AbstractText   string `json:"AbstractText"`
		AbstractURL    string `json:"AbstractURL"`
		AbstractSource string `json:"AbstractSource"`
		Heading        string `json:"Heading"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
		Results []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(body, &ia); err != nil {
		// Don't escalate — DDG occasionally returns html when
		// rate-limited. Caller's fallback handles the void.
		return nil, nil
	}
	var out []Result
	if ia.AbstractURL != "" {
		out = append(out, Result{
			Title:    ia.Heading,
			URL:      ia.AbstractURL,
			Snippet:  ia.AbstractText,
			Rank:     len(out) + 1,
			Provider: d.Name(),
		})
	}
	for _, r := range ia.Results {
		if r.FirstURL == "" {
			continue
		}
		out = append(out, Result{
			Title:    firstSentence(r.Text),
			URL:      r.FirstURL,
			Snippet:  r.Text,
			Rank:     len(out) + 1,
			Provider: d.Name(),
		})
		if len(out) >= max {
			return out, nil
		}
	}
	for _, t := range ia.RelatedTopics {
		if t.FirstURL == "" {
			continue
		}
		out = append(out, Result{
			Title:    firstSentence(t.Text),
			URL:      t.FirstURL,
			Snippet:  t.Text,
			Rank:     len(out) + 1,
			Provider: d.Name(),
		})
		if len(out) >= max {
			break
		}
	}
	return out, nil
}

// htmlSearch scrapes the lite HTML endpoint. The endpoint structure
// is stable enough to walk by class names; we use net/html (already
// a transitive granit dependency) so we don't have to ship a regex
// parser.
//
// The scraped layout is roughly:
//
//	<div class="result">
//	  <a class="result__a" href="URL">Title</a>
//	  <a class="result__snippet">Snippet text</a>
//	</div>
//
// Anything that doesn't fit that shape is skipped — DDG occasionally
// includes sponsored or "did you mean" blocks at the top that we
// ignore by demanding both anchor classes.
func (d *DuckDuckGo) htmlSearch(ctx context.Context, query string, max int) ([]Result, error) {
	form := url.Values{"q": {query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.htmlSearchURL, strings.NewReader(form))
	if err != nil {
		return nil, fmt.Errorf("duckduckgo html: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setUA(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo html: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo html: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, debugErr("duckduckgo html", resp.StatusCode, string(body))
	}
	return parseDuckDuckGoHTML(string(body), max, d.Name()), nil
}

// parseDuckDuckGoHTML extracts the up-to-max hits from the lite
// endpoint's response. Exposed at package level (not as a method) so
// the test harness can pass a static fixture without spinning up a
// real-shaped DuckDuckGo struct.
func parseDuckDuckGoHTML(body string, max int, provider string) []Result {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	var out []Result
	// Walk every node, looking for divs (or any container) carrying
	// class="result". For each one, drill in for the two anchors.
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil || len(out) >= max {
			return
		}
		if n.Type == html.ElementNode && hasClass(n, "result") {
			r := extractResult(n, provider, len(out)+1)
			// Skip ads and skeleton rows where DDG returns no URL.
			if r.URL != "" {
				out = append(out, r)
				if len(out) >= max {
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if len(out) >= max {
				return
			}
		}
	}
	walk(doc)
	return out
}

// extractResult pulls the title, URL, and snippet out of a single
// `<div class="result">` subtree. DDG's redirect-prone URLs use a
// `/l/?uddg=<encoded>` shape — we unwrap that so the agent sees the
// real destination.
func extractResult(node *html.Node, provider string, rank int) Result {
	var r Result
	r.Provider = provider
	r.Rank = rank
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			classes := getAttr(n, "class")
			href := getAttr(n, "href")
			text := strings.TrimSpace(textContent(n))
			switch {
			case strings.Contains(classes, "result__a"):
				r.Title = text
				if r.URL == "" {
					r.URL = unwrapDDGRedirect(href)
				}
			case strings.Contains(classes, "result__snippet"):
				r.Snippet = text
			case r.URL == "" && strings.HasPrefix(href, "http"):
				r.URL = unwrapDDGRedirect(href)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return r
}

// unwrapDDGRedirect turns a `/l/?uddg=<encoded>&...` (or fully
// qualified equivalent) into its target URL. When DDG inlines a
// raw `https://...` href (rare but seen), pass it through.
func unwrapDDGRedirect(href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	if strings.HasPrefix(href, "/l/") || strings.Contains(href, "duckduckgo.com/l/") {
		// Treat as URL-encoded reference.
		ix := strings.Index(href, "uddg=")
		if ix < 0 {
			return href
		}
		raw := href[ix+len("uddg="):]
		if amp := strings.Index(raw, "&"); amp >= 0 {
			raw = raw[:amp]
		}
		decoded, err := url.QueryUnescape(raw)
		if err != nil {
			return href
		}
		return decoded
	}
	return href
}

// hasClass returns true when n carries the given class token in its
// `class` attribute. Token-aware (so `result` matches `result foo`
// but not `result__a`).
func hasClass(n *html.Node, want string) bool {
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, tok := range strings.Fields(a.Val) {
			if tok == want {
				return true
			}
		}
	}
	return false
}

// getAttr returns the named attribute on n, or "" when missing.
// Simpler than the upstream net/html accessors and avoids importing
// `atom`.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// textContent flattens the text descendants of n into a single
// whitespace-collapsed string. Used to extract anchor/snippet text
// without dragging in any heavier text-extraction code.
func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	// Collapse runs of whitespace into single spaces — DDG inlines a
	// lot of newlines inside its anchor bodies.
	out := strings.Join(strings.Fields(b.String()), " ")
	return out
}

// firstSentence returns the leading sentence (or first 80 chars,
// whichever comes first) of s. Used as a synthetic title when DDG
// returned only `Text` + `FirstURL` and we have to derive a heading.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if ix := strings.Index(s, ". "); ix > 0 && ix < 120 {
		return s[:ix]
	}
	// Web snippets are arbitrary text from arbitrary pages —
	// expect every alphabet. Rune-aware truncation avoids splitting
	// a multibyte char at the 80-byte boundary and emitting invalid
	// UTF-8 into the chat surface.
	return textutil.TruncateRunes(s, 80)
}
