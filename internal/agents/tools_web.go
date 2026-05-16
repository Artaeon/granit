package agents

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// WebSearchProvider is the minimal contract the web_search tool
// depends on. Implementations live in internal/websearch (DuckDuckGo
// + Brave + future); this package stays decoupled from the HTTP
// layer so the agents package can be tested with a stub provider
// and so adding a new provider doesn't pull every consumer of
// `agents` into a transitive dep on net/http.
type WebSearchProvider interface {
	// Search runs a query and returns up to `max` results. The
	// agent tool clamps `max` into a small range; providers can
	// further clamp on their side.
	Search(ctx context.Context, query string, max int) ([]WebSearchResult, error)
	// Name returns the provider identifier ("duckduckgo", "brave",
	// …). Used for the observation footer so the LLM can see — and
	// the user can audit — which backend produced the hits.
	Name() string
}

// WebSearchResult mirrors websearch.Result but stays inside the
// agents package's import graph. Same fields, same semantics; the
// internal/agentruntime bridge translates one to the other when
// constructing the tool.
type WebSearchResult struct {
	Title    string
	URL      string
	Snippet  string
	Rank     int
	Provider string
}

// PageFetcher is the contract the fetch_url tool relies on.
// Same decoupling story as WebSearchProvider — the actual HTTP
// fetcher lives in internal/websearch; this package depends only
// on the function signature.
type PageFetcher interface {
	// Fetch retrieves the URL, strips it to readable text, and
	// returns the truncated body. Implementations are responsible
	// for refusing non-http(s) URLs, capping body size, and emitting
	// a clear error on unsupported content-types.
	Fetch(ctx context.Context, url string, maxChars int) (string, error)
}

// ErrWebSearchUnavailable is returned by the web_search tool when no
// provider is wired up (the user disabled the feature, or the
// runtime constructed the registry without a provider). Sentinel so
// the runner can render a single user-friendly hint instead of
// "(nil pointer)".
var ErrWebSearchUnavailable = errors.New("web_search: not configured (enable in Settings → AI → Web research)")

// WebSearch returns a Tool that runs a live web query through the
// provided provider. Read-tool by classification (touches the
// network but doesn't mutate vault state).
//
// Pass a nil provider to register a stub tool that always errors —
// useful when the runner wants the tool catalog to list web_search
// (so a vault-local preset can request it without a registry
// rebuild) but the user hasn't enabled the feature.
func WebSearch(provider WebSearchProvider) Tool {
	return &webSearchTool{provider: provider}
}

type webSearchTool struct {
	provider WebSearchProvider
}

func (t *webSearchTool) Name() string { return "web_search" }
func (t *webSearchTool) Description() string {
	return "Search the web for current information beyond the vault. Returns titled hits with snippets and URLs the agent can cite or follow up on with fetch_url."
}
func (t *webSearchTool) Kind() ToolKind { return KindRead }
func (t *webSearchTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "query", Description: "Search query (plain text; no operators needed)", Required: true},
		{Name: "limit", Description: "Cap on hits to return (default 5; max 10)"},
	}
}

func (t *webSearchTool) Run(ctx context.Context, args map[string]string) ToolResult {
	if t.provider == nil {
		return ToolResult{Err: ErrWebSearchUnavailable}
	}
	q := strings.TrimSpace(args["query"])
	if q == "" {
		return ToolResult{Err: fmt.Errorf("web_search: empty query")}
	}
	limit := 5
	if v, _ := strconv.Atoi(args["limit"]); v > 0 {
		limit = v
	}
	if limit > 10 {
		limit = 10
	}
	hits, err := t.provider.Search(ctx, q, limit)
	if err != nil {
		return ToolResult{Err: fmt.Errorf("web_search: %w", err)}
	}
	if len(hits) == 0 {
		return ToolResult{Output: fmt.Sprintf("(no web results for %q via %s)", q, t.provider.Name())}
	}
	return ToolResult{Output: renderWebHits(hits, t.provider.Name())}
}

// renderWebHits formats search results as a plain-text block the
// LLM consumes as its next Observation. Mirrors the SearchVault
// tool's style — numbered list with one hit per paragraph and a
// trailing source line so the LLM can cite the provider in its
// final answer.
//
// Format chosen so a future "citations chip strip" can parse the
// output back by splitting on "URL:" lines without needing a JSON
// wire format. Stays robust to a single missing field on any hit.
func renderWebHits(hits []WebSearchResult, providerName string) string {
	var b strings.Builder
	for i, h := range hits {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s", i+1, firstNonEmpty(h.Title, h.URL))
		if h.URL != "" {
			fmt.Fprintf(&b, "\n   URL: %s", h.URL)
		}
		if h.Snippet != "" {
			fmt.Fprintf(&b, "\n   %s", collapseSnippet(h.Snippet))
		}
	}
	fmt.Fprintf(&b, "\n\n(source: %s · %d results)", providerName, len(hits))
	return b.String()
}

// FetchURL returns a Tool that pulls a single web page, strips it
// to readable plaintext, and returns the truncated body. Useful as
// a follow-up to web_search hits ("now read this one"). Read-tool;
// must not modify vault state.
//
// Pass a nil fetcher for the same "register a stub that errors"
// pattern as WebSearch.
func FetchURL(fetcher PageFetcher) Tool {
	return &fetchURLTool{fetcher: fetcher}
}

type fetchURLTool struct {
	fetcher PageFetcher
}

func (t *fetchURLTool) Name() string { return "fetch_url" }
func (t *fetchURLTool) Description() string {
	return "Fetch a single web URL and return its readable text content (HTML stripped, truncated to ~8000 chars by default). Use after web_search to read a hit."
}
func (t *fetchURLTool) Kind() ToolKind { return KindRead }
func (t *fetchURLTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "url", Description: "Absolute http(s) URL to fetch", Required: true},
		{Name: "max_chars", Description: "Truncate the readable body to this many characters; default 8000"},
	}
}

func (t *fetchURLTool) Run(ctx context.Context, args map[string]string) ToolResult {
	if t.fetcher == nil {
		return ToolResult{Err: ErrWebSearchUnavailable}
	}
	u := strings.TrimSpace(args["url"])
	if u == "" {
		return ToolResult{Err: fmt.Errorf("fetch_url: empty url")}
	}
	maxChars := 8000
	if v, _ := strconv.Atoi(args["max_chars"]); v > 0 {
		maxChars = v
	}
	body, err := t.fetcher.Fetch(ctx, u, maxChars)
	if err != nil {
		return ToolResult{Err: fmt.Errorf("fetch_url: %w", err)}
	}
	return ToolResult{Output: body}
}

// firstNonEmpty returns the first non-empty string in xs, or "" if
// every entry is empty. Tiny helper so the renderer doesn't carry a
// chain of if/else.
func firstNonEmpty(xs ...string) string {
	for _, s := range xs {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// collapseSnippet wraps long snippets to one line (the LLM doesn't
// benefit from preserved line breaks in a 1-2 sentence search hit)
// and trims to a soft cap. Mirrors what SearchVault does for note
// snippets.
func collapseSnippet(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	const cap = 280
	if len(s) > cap {
		s = s[:cap] + "…"
	}
	return s
}
