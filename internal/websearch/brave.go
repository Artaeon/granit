package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Brave wraps the Brave Search API. Key-gated and rate-limited but
// the cleanest of the major engines for programmatic use — returns
// stable JSON with explicit title/snippet/url fields, no HTML
// scraping required.
//
// The user pastes a key in Settings → AI → Web research. Get one at
// https://api.search.brave.com/. The free tier (2024-era) was 2k
// requests/month, enough for hobby agent runs.
type Brave struct {
	key      string
	client   *http.Client
	endpoint string // exposed for tests
}

// NewBrave constructs a Brave provider. Use Resolve to construct
// the right provider for a Config; this constructor is the lower-
// level seam tests use directly.
func NewBrave(key string, client *http.Client) *Brave {
	if client == nil {
		client = DefaultClient()
	}
	return &Brave{
		key:      key,
		client:   client,
		endpoint: "https://api.search.brave.com/res/v1/web/search",
	}
}

// Name implements SearchProvider.
func (b *Brave) Name() string { return "brave" }

// Search hits the v1 endpoint. Brave's response shape (only the
// fields we use):
//
//	{
//	  "web": {
//	    "results": [
//	      { "title": "...", "url": "...", "description": "..." },
//	      ...
//	    ]
//	  }
//	}
//
// `description` carries HTML highlight markers (`<strong>foo</strong>`)
// which we strip — the LLM doesn't benefit from inline markup.
func (b *Brave) Search(ctx context.Context, query string, max int) ([]Result, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("brave: empty query")
	}
	if strings.TrimSpace(b.key) == "" {
		return nil, fmt.Errorf("brave: missing API key (paste one in Settings → AI → Web research)")
	}
	if max <= 0 {
		max = 5
	}
	if max > 10 {
		max = 10
	}
	u := b.endpoint + "?" + url.Values{
		"q":     {q},
		"count": {fmt.Sprintf("%d", max)},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("brave: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.key)
	setUA(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("brave: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		// Wrap with status so the audit log records 401 (bad key)
		// vs 429 (rate-limited) vs 5xx (Brave outage). The agent
		// surfaces the error to the LLM as an observation.
		return nil, debugErr("brave", resp.StatusCode, string(body))
	}
	var doc struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("brave: parse json: %w", err)
	}
	if len(doc.Web.Results) == 0 {
		return nil, ErrNoResults
	}
	out := make([]Result, 0, len(doc.Web.Results))
	for i, r := range doc.Web.Results {
		if i >= max {
			break
		}
		out = append(out, Result{
			Title:    stripHighlights(r.Title),
			URL:      r.URL,
			Snippet:  stripHighlights(r.Description),
			Rank:     i + 1,
			Provider: b.Name(),
		})
	}
	return out, nil
}

// stripHighlights removes the inline <strong>…</strong> markup Brave
// uses to bold the query term inside snippets. The agent's LLM
// doesn't render HTML, so leaving the tags in just wastes context.
//
// Implemented as a manual scan instead of regex to avoid a regex
// dep and to match the codebase's "line-walk parsers" preference.
func stripHighlights(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '<' {
			// Skip until matching '>' or EOF.
			j := strings.IndexByte(s[i:], '>')
			if j < 0 {
				b.WriteString(s[i:])
				break
			}
			i += j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return strings.TrimSpace(b.String())
}
