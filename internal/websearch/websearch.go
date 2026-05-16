// Package websearch implements an opt-in live web-search bridge for
// granit's agent runtime. It is the only place in the codebase that
// reaches out to non-user-controlled hosts on behalf of the agent,
// and stays consistent with granit's "no outbound traffic unless the
// user explicitly enables it" stance:
//
//   - Every Provider is constructed only when the user toggled the
//     `web_search` feature on in Settings → AI.
//   - The DuckDuckGo provider is the default because it needs no API
//     key (the user pastes nothing); Brave needs the user to provide
//     their own key.
//   - All HTTP traffic flows through one shared http.Client that
//     accepts a custom Transport, so tests can stub responses with
//     httptest.NewServer and never hit the real internet.
//
// Public surface for callers:
//
//	prov := websearch.NewDuckDuckGo(nil)           // default client
//	hits, err := prov.Search(ctx, "granit pkm", 5) // []Result
//	text, err := websearch.FetchReadable(ctx, url) // strip-to-text
//
// Adding more providers (SerpAPI, Bing, Kagi) is mechanical: implement
// SearchProvider on a new struct and register it from Resolve below.
package websearch

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Result is one web-search hit returned to the agent. Field shapes
// mirror what the LLM needs to cite a source ({Title} — {Snippet}
// → {URL}) plus the original Rank so a downstream filter can sort or
// re-weight without losing provider order. Anything beyond these
// fields is provider-specific and lives in the provider's audit log,
// not the public type.
type Result struct {
	// Title is the headline text the provider returned for the hit.
	// May be empty when a provider returns only a URL+snippet (rare,
	// but DuckDuckGo's instant-answer JSON does this for raw URLs).
	Title string `json:"title"`
	// URL is the canonical hit URL. Always absolute; providers that
	// emit relative URLs are normalised before reaching the caller.
	URL string `json:"url"`
	// Snippet is the short summary the provider returned. May be
	// empty. Length is provider-controlled — we don't truncate here
	// because the tool layer is responsible for clamping observation
	// size to the LLM context budget.
	Snippet string `json:"snippet"`
	// Rank is the 1-based position the provider returned the hit in.
	// Useful for ordered citations (`[1] title`) and for debugging
	// "is the provider ranking sane?" questions.
	Rank int `json:"rank"`
	// Provider names which backend produced this result. Lets a UI
	// surface "from: brave" / "from: duckduckgo" without the caller
	// tracking it separately.
	Provider string `json:"provider,omitempty"`
}

// SearchProvider is the minimal contract every backend must satisfy.
// Keep it small on purpose — anything beyond Search forces all
// providers to implement (or stub) it, and we'd rather add a second
// interface (e.g. Pricer) than bloat this one.
//
// Search MUST honour ctx — long DNS timeouts and slow-server
// stalls are otherwise the worst-case for an agent step budget.
type SearchProvider interface {
	// Search runs a query against the backend and returns at most
	// `max` results. Implementations clamp max into a sane range
	// (no infinite scrolling — agent context is finite).
	//
	// Returns (nil, nil) when the query yields no hits — callers
	// distinguish "no results" from "error" via the error value.
	Search(ctx context.Context, query string, max int) ([]Result, error)
	// Name returns the stable identifier of the provider. Used in
	// audit logs and the Result.Provider field.
	Name() string
}

// Config is the persisted user choice — which provider to route to,
// plus any keys the chosen provider needs. The same struct serialises
// out of settings, sits inside aiprefs (as a feature-level extension),
// and resolves to a concrete SearchProvider via Resolve.
//
// Stored at <vault>/.granit/web-search.json so the prefs file stays
// schema-stable (we don't want to grow aiprefs every time a feature
// needs a single extra field). Defaults: provider=duckduckgo,
// MaxResults=5, no key. The user has to paste a key for Brave to work.
type Config struct {
	// Provider chooses the backend. Valid values: "duckduckgo",
	// "brave". Unknown values fall through to duckduckgo at Resolve
	// time — fail-soft so a stale config doesn't lock the user out
	// of the feature.
	Provider string `json:"provider"`
	// BraveKey is the user's Brave Search API key. Stored
	// per-vault (same posture as openai_key in config.json).
	BraveKey string `json:"brave_key,omitempty"`
	// MaxResults caps the number of hits returned per Search call.
	// Clamped to [1, 10] at Resolve time so a typo here can't waste
	// the agent's context window. 0 means "use default" (5).
	MaxResults int `json:"max_results,omitempty"`
}

// Defaults returns the safe default config — DuckDuckGo (no key),
// 5 results, no API key set. Settings UI seeds with these on first
// load; Load(vaultRoot) calls this on missing file.
func Defaults() Config {
	return Config{
		Provider:   "duckduckgo",
		MaxResults: 5,
	}
}

// Resolve picks a SearchProvider matching cfg, falling back to
// DuckDuckGo when the chosen provider isn't usable (e.g. Brave
// without a key). Returns an error only when no provider at all can
// be constructed — currently impossible because DuckDuckGo always
// works, but we keep the contract honest so future "all providers
// are key-gated" deployments can fail loudly.
//
// client may be nil; Resolve hands a shared default to the provider.
func Resolve(cfg Config, client *http.Client) (SearchProvider, error) {
	if client == nil {
		client = DefaultClient()
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "brave":
		if strings.TrimSpace(cfg.BraveKey) == "" {
			// Fail-soft: pretend "brave-without-key" means
			// "default to ddg" so the user gets results while
			// the settings UI nudges them to paste a key.
			return NewDuckDuckGo(client), nil
		}
		return NewBrave(cfg.BraveKey, client), nil
	case "duckduckgo", "":
		return NewDuckDuckGo(client), nil
	default:
		// Unknown name — same fail-soft posture as above.
		return NewDuckDuckGo(client), nil
	}
}

// EffectiveMaxResults clamps cfg.MaxResults into the supported
// [1, 10] range. Centralised so every provider's Search clamps
// identically (catches the "I asked for 5 but got 7" footguns).
func EffectiveMaxResults(cfg Config) int {
	n := cfg.MaxResults
	if n <= 0 {
		n = 5
	}
	if n > 10 {
		n = 10
	}
	return n
}

// DefaultClient returns a shared *http.Client with a sane timeout
// and the granit user-agent. Used by every provider that doesn't
// have a test-injected client of its own.
//
// 15 s is the upper bound — a single failing search must not eat
// half the agent's ReAct step budget while it stalls on a slow DNS.
func DefaultClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
	}
}

// ErrNoResults is returned when the provider responded successfully
// but produced zero hits. Callers (the agent tool) usually want to
// surface that as an observation rather than an error — keeping the
// sentinel as an exported value makes that branch explicit.
var ErrNoResults = errSentinel("websearch: no results")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// setUA stamps the canonical granit user-agent on an outgoing
// request. Tiny helper so every provider does the same thing and
// remote endpoints get a consistent header without us copy-pasting
// the version string everywhere.
func setUA(req *http.Request) {
	req.Header.Set("User-Agent", "granit-web-search/1.0 (+https://github.com/artaeon/granit)")
}

// debugErr formats a one-line provider error with status code and
// truncated body so the audit log carries enough to diagnose the
// failure without dumping the full response.
func debugErr(provider string, status int, body string) error {
	const cap = 200
	if len(body) > cap {
		body = body[:cap] + "..."
	}
	return fmt.Errorf("%s: http %d: %s", provider, status, strings.TrimSpace(body))
}
