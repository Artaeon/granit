package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Resolve picks DuckDuckGo when the config is empty / unknown, and
// upgrades to Brave only when both provider="brave" AND a key is set.
// Anything else fails-soft to DuckDuckGo so a stale config can't
// lock the user out of the feature.
func TestResolve_FallsBackToDuckDuckGo(t *testing.T) {
	cases := []struct {
		name     string
		cfg      Config
		wantName string
	}{
		{"defaults", Defaults(), "duckduckgo"},
		{"empty provider", Config{}, "duckduckgo"},
		{"explicit ddg", Config{Provider: "DuckDuckGo"}, "duckduckgo"},
		{"brave without key", Config{Provider: "brave"}, "duckduckgo"},
		{"brave with key", Config{Provider: "brave", BraveKey: "k"}, "brave"},
		{"unknown name", Config{Provider: "bing"}, "duckduckgo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Resolve(tc.cfg, nil)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if p.Name() != tc.wantName {
				t.Errorf("want provider %q, got %q", tc.wantName, p.Name())
			}
		})
	}
}

// EffectiveMaxResults clamps to [1, 10]. 0 becomes 5 (the
// agent-friendly default); 50 becomes 10 (the hard cap to keep the
// observation small).
func TestEffectiveMaxResults_Clamps(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 5},
		{-1, 5},
		{1, 1},
		{5, 5},
		{10, 10},
		{50, 10},
	}
	for _, c := range cases {
		got := EffectiveMaxResults(Config{MaxResults: c.in})
		if got != c.want {
			t.Errorf("EffectiveMaxResults(%d): want %d, got %d", c.in, c.want, got)
		}
	}
}

// DuckDuckGo Instant Answer path: when the API returns an Abstract
// + Results block, the provider surfaces both with sequential ranks
// and a populated Snippet. Confirms that the JSON-first branch
// short-circuits the HTML fallback.
func TestDuckDuckGo_InstantAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("unexpected DDG IA path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"AbstractText": "Granit is a self-hosted PKM.",
			"AbstractURL":  "https://example.com/granit",
			"Heading":      "Granit",
			"Results": [
				{"Text": "Granit homepage", "FirstURL": "https://example.com/home"}
			],
			"RelatedTopics": [
				{"Text": "More on Granit", "FirstURL": "https://example.com/more"}
			]
		}`))
	}))
	t.Cleanup(srv.Close)

	d := NewDuckDuckGo(srv.Client())
	d.instantAnswerURL = srv.URL + "/"
	// Point HTML at a URL that would 500 if hit — we expect the
	// JSON path to short-circuit before fallback.
	d.htmlSearchURL = srv.URL + "/should-not-be-called"

	hits, err := d.Search(context.Background(), "granit", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	if hits[0].URL != "https://example.com/granit" {
		t.Errorf("abstract first; got URL %q", hits[0].URL)
	}
	if hits[0].Rank != 1 || hits[1].Rank != 2 {
		t.Errorf("ranks should be sequential 1..n, got %d/%d", hits[0].Rank, hits[1].Rank)
	}
	if hits[0].Provider != "duckduckgo" {
		t.Errorf("provider tag missing, got %q", hits[0].Provider)
	}
}

// HTML fallback: when Instant Answer returns nothing, scrape the
// lite endpoint. Confirms the class-walking parser pulls title+
// snippet from a minimal lookalike page and that the `/l/?uddg=…`
// redirect wrapper is unwrapped to the real destination URL.
func TestDuckDuckGo_HTMLFallback(t *testing.T) {
	htmlBody := `<!DOCTYPE html><html><body>
<div class="result">
  <a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2Fone&amp;rut=abc">One</a>
  <a class="result__snippet">snippet for one</a>
</div>
<div class="result">
  <a class="result__a" href="https://example.com/two">Two</a>
  <a class="result__snippet">snippet for two</a>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ia":
			// Empty Instant Answer payload — forces fallback.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(htmlBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	d := NewDuckDuckGo(srv.Client())
	d.instantAnswerURL = srv.URL + "/ia"
	d.htmlSearchURL = srv.URL + "/html"

	hits, err := d.Search(context.Background(), "anything", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d (%+v)", len(hits), hits)
	}
	if hits[0].URL != "https://example.com/one" {
		t.Errorf("redirect wrapper not unwrapped: got %q", hits[0].URL)
	}
	if hits[0].Title != "One" {
		t.Errorf("title not extracted: %+v", hits[0])
	}
	if hits[0].Snippet != "snippet for one" {
		t.Errorf("snippet not extracted: %+v", hits[0])
	}
	if hits[1].URL != "https://example.com/two" {
		t.Errorf("direct href not preserved: %q", hits[1].URL)
	}
}

// Brave provider: stubs the v1 endpoint and confirms the headers
// (X-Subscription-Token), JSON parsing, and the <strong> highlight
// stripping in snippets.
func TestBrave_Search(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Subscription-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"web": {
				"results": [
					{"title": "Granit <strong>PKM</strong>", "url": "https://example.com/a", "description": "self-<strong>hosted</strong> note app"},
					{"title": "About", "url": "https://example.com/b", "description": "plain"}
				]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	b := NewBrave("test-key", srv.Client())
	b.endpoint = srv.URL + "/search"

	hits, err := b.Search(context.Background(), "granit", 5)
	if err != nil {
		t.Fatalf("brave search: %v", err)
	}
	if gotKey != "test-key" {
		t.Errorf("subscription header not set: got %q", gotKey)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	if strings.Contains(hits[0].Title, "<strong>") {
		t.Errorf("highlights not stripped from title: %q", hits[0].Title)
	}
	if strings.Contains(hits[0].Snippet, "<strong>") {
		t.Errorf("highlights not stripped from snippet: %q", hits[0].Snippet)
	}
	if hits[0].Provider != "brave" || hits[0].Rank != 1 {
		t.Errorf("provider/rank: %+v", hits[0])
	}
}

// Brave without a key fails fast with a hint pointing at Settings,
// rather than letting the request go out and 401.
func TestBrave_NoKey(t *testing.T) {
	b := NewBrave("", nil)
	_, err := b.Search(context.Background(), "any", 5)
	if err == nil {
		t.Fatal("expected error when key is empty")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("error should mention key: %v", err)
	}
}

// Brave 4xx surfaces with status code so the audit log can
// distinguish bad-key (401) from rate-limit (429).
func TestBrave_BubblesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`rate limited`))
	}))
	t.Cleanup(srv.Close)

	b := NewBrave("key", srv.Client())
	b.endpoint = srv.URL + "/x"

	_, err := b.Search(context.Background(), "q", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected status 429 in error, got: %v", err)
	}
}

// FetchReadable strips HTML to plaintext, drops <script>/<style>,
// collapses block-level whitespace into paragraph breaks, and
// truncates to MaxChars with an explicit footer so the agent knows
// the doc was cut.
func TestFetchReadable(t *testing.T) {
	page := `<html><head><title>X</title><style>body{color:red}</style></head>
<body>
<script>alert('no')</script>
<h1>Headline</h1>
<p>First paragraph.</p>
<p>Second paragraph with <strong>bold</strong> in it.</p>
<div>Third block via div.</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)

	text, err := FetchReadable(context.Background(), srv.URL+"/p", FetchOptions{
		Client: srv.Client(),
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if strings.Contains(text, "alert") || strings.Contains(text, "color:red") {
		t.Errorf("script/style leaked: %q", text)
	}
	if !strings.Contains(text, "Headline") || !strings.Contains(text, "First paragraph") {
		t.Errorf("missing expected content: %q", text)
	}
	if !strings.Contains(text, "Third block") {
		t.Errorf("div content not surfaced: %q", text)
	}
}

// fetch_url rejects file:// and other non-http(s) URLs — the agent
// must not be able to read arbitrary disk paths via this tool.
func TestFetchReadable_RejectsNonHTTP(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"",
		"   ",
	}
	for _, u := range cases {
		_, err := FetchReadable(context.Background(), u, FetchOptions{})
		if err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
}

// fetch_url surfaces a clear error for non-HTML content-types so
// the agent gets actionable feedback instead of base64 gibberish.
func TestFetchReadable_RejectsNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 binary"))
	}))
	t.Cleanup(srv.Close)
	_, err := FetchReadable(context.Background(), srv.URL+"/x.pdf", FetchOptions{Client: srv.Client()})
	if err == nil {
		t.Fatal("expected error for PDF content-type")
	}
	if !strings.Contains(err.Error(), "content-type") {
		t.Errorf("expected content-type hint, got: %v", err)
	}
}

// Load/Save round-trip: write a Config to a temp vault root, read
// it back, confirm the fields match. Missing file path returns
// Defaults (no error).
func TestStore_RoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if cfg.Provider != "duckduckgo" {
		t.Errorf("default provider wrong: %+v", cfg)
	}
	want := Config{Provider: "brave", BraveKey: "abc", MaxResults: 7}
	if err := Save(root, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch: want %+v, got %+v", want, got)
	}
}
