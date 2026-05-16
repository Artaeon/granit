package websearch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// MaxFetchBytes caps the size of any single page we'll pull. 512 KB
// covers ~99% of real article pages while keeping a runaway "agent
// fetched a 200 MB PDF" worst case out of memory. Callers can also
// pass a smaller max to FetchReadable directly.
const MaxFetchBytes = 512 * 1024

// MaxReadableChars is the default truncation cap for the strip-to-
// text output. The agent's ReAct loop has a finite context budget;
// even on a generous Anthropic context window, dumping a full
// 50,000-token web page into one observation crowds out everything
// else. Callers can override with FetchOptions.MaxChars.
const MaxReadableChars = 8000

// FetchOptions configures a single FetchReadable call. Zero values
// fall through to package defaults — most callers pass the zero
// struct.
type FetchOptions struct {
	// Client overrides the HTTP client used for the fetch. nil
	// falls through to DefaultClient(). Tests inject an
	// httptest.NewServer client to stub responses.
	Client *http.Client
	// MaxBytes overrides MaxFetchBytes. 0 keeps the default.
	MaxBytes int64
	// MaxChars overrides MaxReadableChars for the post-strip
	// truncation step. 0 keeps the default.
	MaxChars int
}

// FetchReadable pulls a URL, strips HTML to plaintext, and returns
// the truncated readable body. Used by the fetch_url tool so the
// agent can follow up after a web_search hit — "now read that page".
//
// Constraints:
//
//   - HTTPS-only by default (HTTP works but is logged differently in
//     audit, so callers should still wrap it through resolveURL).
//   - Response body is capped at MaxBytes — anything larger is
//     truncated mid-stream.
//   - Non-HTML content-types (PDF, video) return an explanatory
//     error rather than gibberish; the agent's observation will
//     tell it to pick a different URL.
//   - Inline <script> and <style> blocks are removed entirely;
//     their text content is markup, not readable prose.
func FetchReadable(ctx context.Context, target string, opts FetchOptions) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("fetch_url: empty URL")
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return "", fmt.Errorf("fetch_url: only http(s) URLs are allowed; got %q", target)
	}
	client := opts.Client
	if client == nil {
		client = DefaultClient()
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxFetchBytes
	}
	maxChars := opts.MaxChars
	if maxChars <= 0 {
		maxChars = MaxReadableChars
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("fetch_url: build request: %w", err)
	}
	setUA(req)
	// Hint to servers that we want HTML; some sites return a
	// stripped-down landing page when no Accept header is set.
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch_url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("fetch_url: http %d on %s", resp.StatusCode, target)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !isHTMLContentType(ct) {
		return "", fmt.Errorf("fetch_url: unsupported content-type %q (need text/html-ish); pick a different URL", ct)
	}
	// LimitReader caps the body at maxBytes so a 1 GB page can't
	// OOM the server. We deliberately *don't* return an error when
	// the cap is hit — partial text is more useful to the agent
	// than nothing.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return "", fmt.Errorf("fetch_url: read body: %w", err)
	}
	text := ExtractReadable(string(body))
	if len(text) > maxChars {
		text = text[:maxChars] + fmt.Sprintf("\n\n[truncated — %d more chars]", len(text)-maxChars)
	}
	return text, nil
}

// ExtractReadable strips HTML to plaintext, preserving paragraph
// breaks. Public so tests can exercise it on a static fixture
// without bringing in the http layer.
//
// The extractor is intentionally simple — net/html walks the tree,
// we skip <script>/<style>/<head>, and we emit block-level
// boundaries as blank lines so the output reads like prose. We
// don't try to reproduce <article>-only extraction (à la Mozilla
// Readability) because the agent benefits from more rather than
// less text — it can re-summarise downstream.
func ExtractReadable(htmlBody string) string {
	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return strings.TrimSpace(htmlBody)
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript", "iframe", "svg", "head":
				// Whole subtree is non-prose markup.
				return
			case "p", "div", "br", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "section", "article", "header", "footer", "blockquote":
				// Emit a paragraph break before descending so
				// adjacent block-level chunks don't smash
				// together as one run of text.
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
					b.WriteString("\n\n")
				}
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), " ") && !strings.HasSuffix(b.String(), "\n") {
					b.WriteByte(' ')
				}
				b.WriteString(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	// Collapse runs of >2 newlines into 2 (paragraph) and trim
	// the leading/trailing whitespace. Single-line runs are kept
	// so the output reads naturally on output without re-wrapping.
	raw := b.String()
	out := collapseBlankLines(raw)
	return strings.TrimSpace(out)
}

// collapseBlankLines reduces 3+ consecutive newlines to a single
// double-newline. The walker above can emit doubles between every
// block-level node it visits; left alone, that produces ~15-line
// gaps which waste LLM context.
func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// isHTMLContentType returns true for content-types we know how to
// strip. text/html and the xhtml flavours qualify; everything else
// (json, pdf, video) does not. We deliberately don't try to handle
// PDFs — the agent has no use for a corrupted text extraction.
func isHTMLContentType(ct string) bool {
	ct = strings.ToLower(ct)
	if semi := strings.Index(ct, ";"); semi >= 0 {
		ct = ct[:semi]
	}
	ct = strings.TrimSpace(ct)
	switch ct {
	case "text/html", "application/xhtml+xml", "text/plain":
		return true
	}
	return false
}
