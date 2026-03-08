package tui

import (
	"strings"
	"testing"
)

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		want  string
	}{
		{
			name: "simple title",
			html: `<html><head><title>Hello World</title></head></html>`,
			want: "Hello World",
		},
		{
			name: "title with whitespace",
			html: `<title>  Spaced Title  </title>`,
			want: "Spaced Title",
		},
		{
			name: "title with HTML entities",
			html: `<title>Tom &amp; Jerry</title>`,
			want: "Tom & Jerry",
		},
		{
			name: "title with nested tags",
			html: `<title><span>Nested</span> Title</title>`,
			want: "Nested Title",
		},
		{
			name: "no title tag",
			html: `<html><body>No title here</body></html>`,
			want: "",
		},
		{
			name: "empty title",
			html: `<title></title>`,
			want: "",
		},
		{
			name: "title with attributes",
			html: `<title lang="en">Attributed</title>`,
			want: "Attributed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTitle(tc.html)
			if got != tc.want {
				t.Errorf("extractTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHtmlToMarkdown_Headings(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		md   string
	}{
		{"h1", "<h1>Title</h1>", "# Title"},
		{"h2", "<h2>Section</h2>", "## Section"},
		{"h3", "<h3>Sub</h3>", "### Sub"},
		{"h4", "<h4>Deep</h4>", "#### Deep"},
		{"h5", "<h5>Deeper</h5>", "##### Deeper"},
		{"h6", "<h6>Deepest</h6>", "###### Deepest"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToMarkdown(tc.tag)
			got = strings.TrimSpace(got)
			if got != tc.md {
				t.Errorf("htmlToMarkdown(%q) = %q, want %q", tc.tag, got, tc.md)
			}
		})
	}
}

func TestHtmlToMarkdown_Paragraphs(t *testing.T) {
	html := "<p>First paragraph</p><p>Second paragraph</p>"
	got := htmlToMarkdown(html)

	if !strings.Contains(got, "First paragraph") {
		t.Error("expected 'First paragraph' in output")
	}
	if !strings.Contains(got, "Second paragraph") {
		t.Error("expected 'Second paragraph' in output")
	}
}

func TestHtmlToMarkdown_Links(t *testing.T) {
	html := `<a href="https://example.com">Click here</a>`
	got := htmlToMarkdown(html)
	got = strings.TrimSpace(got)

	if !strings.Contains(got, "[Click here](https://example.com)") {
		t.Errorf("expected markdown link, got %q", got)
	}
}

func TestHtmlToMarkdown_BoldItalic(t *testing.T) {
	t.Run("bold with strong", func(t *testing.T) {
		got := strings.TrimSpace(htmlToMarkdown("<strong>bold</strong>"))
		if !strings.Contains(got, "**bold**") {
			t.Errorf("expected **bold**, got %q", got)
		}
	})

	t.Run("bold with b tag", func(t *testing.T) {
		got := strings.TrimSpace(htmlToMarkdown("<b>bold</b>"))
		if !strings.Contains(got, "**bold**") {
			t.Errorf("expected **bold**, got %q", got)
		}
	})

	t.Run("italic with em", func(t *testing.T) {
		got := strings.TrimSpace(htmlToMarkdown("<em>italic</em>"))
		if !strings.Contains(got, "*italic*") {
			t.Errorf("expected *italic*, got %q", got)
		}
	})

	t.Run("italic with i tag", func(t *testing.T) {
		got := strings.TrimSpace(htmlToMarkdown("<i>italic</i>"))
		if !strings.Contains(got, "*italic*") {
			t.Errorf("expected *italic*, got %q", got)
		}
	})
}

func TestHtmlToMarkdown_Lists(t *testing.T) {
	html := "<ul><li>Item one</li><li>Item two</li><li>Item three</li></ul>"
	got := htmlToMarkdown(html)

	if !strings.Contains(got, "- Item one") {
		t.Errorf("expected '- Item one' in output, got %q", got)
	}
	if !strings.Contains(got, "- Item two") {
		t.Errorf("expected '- Item two' in output, got %q", got)
	}
	if !strings.Contains(got, "- Item three") {
		t.Errorf("expected '- Item three' in output, got %q", got)
	}
}

func TestHtmlToMarkdown_Blockquotes(t *testing.T) {
	html := "<blockquote>Quoted text here</blockquote>"
	got := htmlToMarkdown(html)

	if !strings.Contains(got, "> Quoted text here") {
		t.Errorf("expected blockquote markdown, got %q", got)
	}
}

func TestHtmlToMarkdown_Code(t *testing.T) {
	html := "<code>fmt.Println</code>"
	got := strings.TrimSpace(htmlToMarkdown(html))

	if !strings.Contains(got, "`fmt.Println`") {
		t.Errorf("expected backtick code, got %q", got)
	}
}

func TestHtmlToMarkdown_StripsUnwantedBlocks(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		absent  string
	}{
		{
			name:   "strips script",
			html:   `<p>Keep</p><script>alert("bad")</script>`,
			absent: "alert",
		},
		{
			name:   "strips style",
			html:   `<p>Keep</p><style>body { color: red; }</style>`,
			absent: "color: red",
		},
		{
			name:   "strips nav",
			html:   `<p>Keep</p><nav><a href="/">Home</a></nav>`,
			absent: "Home",
		},
		{
			name:   "strips footer",
			html:   `<p>Keep</p><footer>Copyright 2024</footer>`,
			absent: "Copyright",
		},
		{
			name:   "strips header",
			html:   `<p>Keep</p><header>Site Header</header>`,
			absent: "Site Header",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToMarkdown(tc.html)
			if strings.Contains(got, tc.absent) {
				t.Errorf("expected %q to be stripped, got %q", tc.absent, got)
			}
			if !strings.Contains(got, "Keep") {
				t.Error("expected 'Keep' to be preserved")
			}
		})
	}
}

func TestHtmlToMarkdown_EmptyInput(t *testing.T) {
	got := htmlToMarkdown("")
	if got != "" {
		t.Errorf("expected empty output for empty input, got %q", got)
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ampersand", "Tom &amp; Jerry", "Tom & Jerry"},
		{"less than", "a &lt; b", "a < b"},
		{"greater than", "a &gt; b", "a > b"},
		{"double quote", "&quot;hello&quot;", `"hello"`},
		{"nbsp", "hello&nbsp;world", "hello world"},
		{"apostrophe &#39;", "it&#39;s", "it's"},
		{"apostrophe &#039;", "it&#039;s", "it's"},
		{"numeric entity", "&#65;", "A"},
		{"numeric entity small", "&#97;", "a"},
		{"multiple entities", "&amp;&lt;&gt;", "&<>"},
		{"no entities", "plain text", "plain text"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeHTMLEntities(tc.input)
			if got != tc.want {
				t.Errorf("decodeHTMLEntities(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "within limit",
			input:  "short",
			maxLen: 10,
			want:   "short",
		},
		{
			name:   "exactly at limit",
			input:  "exact",
			maxLen: 5,
			want:   "exact",
		},
		{
			name:   "over limit adds ellipsis",
			input:  "this is a long string",
			maxLen: 10,
			want:   "this is...",
		},
		{
			name:   "zero limit",
			input:  "anything",
			maxLen: 0,
			want:   "",
		},
		{
			name:   "negative limit",
			input:  "anything",
			maxLen: -5,
			want:   "",
		},
		{
			name:   "maxLen 1 truncates without ellipsis",
			input:  "hello",
			maxLen: 1,
			want:   "h",
		},
		{
			name:   "maxLen 3 truncates without ellipsis",
			input:  "hello",
			maxLen: 3,
			want:   "hel",
		},
		{
			name:   "maxLen 4 with ellipsis",
			input:  "hello world",
			maxLen: 4,
			want:   "h...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}
