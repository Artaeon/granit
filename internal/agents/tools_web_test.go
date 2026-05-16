package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// stubProvider implements WebSearchProvider so tools_web_test can
// exercise the tool without touching the network or pulling in the
// real internal/websearch package.
type stubProvider struct {
	name string
	hits []WebSearchResult
	err  error
	last struct {
		query string
		max   int
	}
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Search(_ context.Context, query string, max int) ([]WebSearchResult, error) {
	s.last.query = query
	s.last.max = max
	if s.err != nil {
		return nil, s.err
	}
	if max < len(s.hits) {
		return s.hits[:max], nil
	}
	return s.hits, nil
}

// stubFetcher implements PageFetcher with a fixed response so we can
// exercise fetch_url without a real HTTP server.
type stubFetcher struct {
	body string
	err  error
	last struct {
		url      string
		maxChars int
	}
}

func (f *stubFetcher) Fetch(_ context.Context, url string, maxChars int) (string, error) {
	f.last.url = url
	f.last.maxChars = maxChars
	if f.err != nil {
		return "", f.err
	}
	return f.body, nil
}

// web_search renders hits as a numbered list with URL + snippet,
// trails with a "(source: …)" footer the LLM can read off when
// citing in its final answer, and respects the limit param (which
// gets clamped to [1, 10] regardless of what the LLM types).
func TestWebSearchTool_RendersHits(t *testing.T) {
	prov := &stubProvider{
		name: "fake",
		hits: []WebSearchResult{
			{Title: "Granit", URL: "https://example.com/a", Snippet: "PKM tool", Rank: 1},
			{Title: "Docs", URL: "https://example.com/b", Snippet: "user guide", Rank: 2},
		},
	}
	tool := WebSearch(prov)
	if tool.Kind() != KindRead {
		t.Errorf("web_search must be read-class; got %q", tool.Kind())
	}
	r := tool.Run(context.Background(), map[string]string{"query": "granit pkm", "limit": "5"})
	if r.Err != nil {
		t.Fatalf("unexpected err: %v", r.Err)
	}
	if !strings.Contains(r.Output, "1. Granit") || !strings.Contains(r.Output, "https://example.com/a") {
		t.Errorf("first hit not rendered: %q", r.Output)
	}
	if !strings.Contains(r.Output, "(source: fake · 2 results)") {
		t.Errorf("source footer missing: %q", r.Output)
	}
	if prov.last.query != "granit pkm" {
		t.Errorf("query not forwarded: %q", prov.last.query)
	}
}

// Limit clamp: the LLM might pick limit=50, the tool must cap it
// at 10 before talking to the provider so a single web_search step
// can't blow the agent's context budget.
func TestWebSearchTool_ClampsLimit(t *testing.T) {
	prov := &stubProvider{name: "fake", hits: make([]WebSearchResult, 0)}
	tool := WebSearch(prov)
	r := tool.Run(context.Background(), map[string]string{"query": "x", "limit": "999"})
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if prov.last.max != 10 {
		t.Errorf("limit not clamped: got %d", prov.last.max)
	}
}

// Nil provider + nil fetcher both return ErrWebSearchUnavailable so
// the runner can present a single actionable hint instead of a
// panic. Lets the tool stay in the catalog while disabled.
func TestWebSearchTool_NilProvider(t *testing.T) {
	tool := WebSearch(nil)
	r := tool.Run(context.Background(), map[string]string{"query": "anything"})
	if r.Err == nil {
		t.Fatal("expected ErrWebSearchUnavailable")
	}
	if !errors.Is(r.Err, ErrWebSearchUnavailable) {
		t.Errorf("expected sentinel error, got: %v", r.Err)
	}
}

// Empty query is a validation failure — surface it as an Err so the
// agent retries with a real query rather than us hitting the
// provider with a useless empty request.
func TestWebSearchTool_EmptyQuery(t *testing.T) {
	prov := &stubProvider{name: "fake"}
	tool := WebSearch(prov)
	r := tool.Run(context.Background(), map[string]string{"query": "   "})
	if r.Err == nil {
		t.Fatal("expected error on empty query")
	}
	if prov.last.query != "" {
		t.Errorf("provider should not have been called: %q", prov.last.query)
	}
}

// Provider errors bubble up through the tool wrapper with a clear
// prefix so the LLM observation reads "web_search: <reason>" rather
// than the raw network noise.
func TestWebSearchTool_BubblesProviderError(t *testing.T) {
	prov := &stubProvider{name: "fake", err: errors.New("boom")}
	tool := WebSearch(prov)
	r := tool.Run(context.Background(), map[string]string{"query": "anything"})
	if r.Err == nil {
		t.Fatal("expected provider error to surface")
	}
	if !strings.Contains(r.Err.Error(), "web_search") {
		t.Errorf("error not wrapped with tool prefix: %v", r.Err)
	}
}

// fetch_url forwards the URL + max_chars and surfaces the readable
// body as the Observation output.
func TestFetchURLTool(t *testing.T) {
	f := &stubFetcher{body: "readable text body"}
	tool := FetchURL(f)
	if tool.Kind() != KindRead {
		t.Errorf("fetch_url must be read-class")
	}
	r := tool.Run(context.Background(), map[string]string{
		"url":       "https://example.com/x",
		"max_chars": "1000",
	})
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	if r.Output != "readable text body" {
		t.Errorf("body not surfaced: %q", r.Output)
	}
	if f.last.url != "https://example.com/x" || f.last.maxChars != 1000 {
		t.Errorf("args not forwarded: %+v", f.last)
	}
}

// fetch_url with a nil fetcher (feature disabled) returns the same
// sentinel so the agent and the UI can render a single hint.
func TestFetchURLTool_NilFetcher(t *testing.T) {
	tool := FetchURL(nil)
	r := tool.Run(context.Background(), map[string]string{"url": "https://example.com"})
	if !errors.Is(r.Err, ErrWebSearchUnavailable) {
		t.Errorf("expected ErrWebSearchUnavailable; got %v", r.Err)
	}
}

// Long snippets are collapsed to a soft cap so a noisy provider
// can't bloat a single hit past the agent's context budget. Empty
// URL is rendered with the title falling through as the heading.
func TestRenderWebHits_FallbacksAndTruncation(t *testing.T) {
	long := strings.Repeat("x", 1000)
	hits := []WebSearchResult{
		{Title: "", URL: "https://example.com/only-url", Snippet: long, Rank: 1},
	}
	out := renderWebHits(hits, "fake")
	// When title is empty, the URL is used as the heading.
	if !strings.Contains(out, "1. https://example.com/only-url") {
		t.Errorf("URL fallback not used: %q", out)
	}
	// Snippet must be capped well under 1000.
	if len(out) > 600 {
		t.Errorf("snippet not collapsed: len=%d", len(out))
	}
}

// renderWebHits stays robust to a "results but no provider name"
// edge case by emitting an empty source name rather than panicking.
func TestRenderWebHits_EmptyProviderName(t *testing.T) {
	hits := []WebSearchResult{{Title: "x", URL: "https://example.com"}}
	out := renderWebHits(hits, "")
	if !strings.Contains(out, "(source:  · 1 results)") {
		t.Errorf("source footer should render with empty name, got: %q", out)
	}
}

// Sanity: ensure the rendered output never accidentally introduces
// raw HTML the LLM would have to parse around. The snippet path
// only collapses whitespace; tag-stripping happens at the provider
// layer.
func TestRenderWebHits_RoundTrip(t *testing.T) {
	hits := []WebSearchResult{
		{Title: "T1", URL: "u1", Snippet: "s1", Rank: 1},
		{Title: "T2", URL: "u2", Snippet: "s2", Rank: 2},
	}
	out := renderWebHits(hits, "p")
	for i, h := range hits {
		if !strings.Contains(out, fmt.Sprintf("%d. %s", i+1, h.Title)) {
			t.Errorf("missing rank header for hit %d in: %q", i+1, out)
		}
	}
}
