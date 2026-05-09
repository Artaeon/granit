package books

import (
	"context"
	"testing"
)

func TestSafeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Pride and Prejudice", "Pride and Prejudice"},
		{"Jane Austen — Persuasion", "Jane Austen Persuasion"},
		{"  multi   spaces  ", "multi spaces"},
		// Dot is intentionally stripped — the .epub extension is
		// re-appended by the import path after sanitization.
		{"foo/bar:baz<>?.epub", "foo bar baz epub"},
		{"", ""},
		// Unicode collapses to whitespace under our ASCII-only re;
		// the resulting string trims to the spaces around it.
		{"日本語", ""},
	}
	for _, c := range cases {
		got := safeFilename(c.in)
		if got != c.want {
			t.Errorf("safeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSwapAuthorOrder(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Austen, Jane", "Jane Austen"},
		{"Tolstoy, graf Leo", "graf Leo Tolstoy"},
		{"Anonymous", "Anonymous"},
		{"", ""},
		{"Plato", "Plato"},
		{",", ","},
	}
	for _, c := range cases {
		got := swapAuthorOrder(c.in)
		if got != c.want {
			t.Errorf("swapAuthorOrder(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPickGutenbergEPUB(t *testing.T) {
	// Real Gutendex shape: epub.images preferred, plus a
	// noimages fallback.
	formats := map[string]string{
		"application/epub+zip":  "https://www.gutenberg.org/ebooks/1342.epub.images",
		"text/html":             "https://www.gutenberg.org/files/1342/1342-h/1342-h.htm",
		"application/x-mobipocket-ebook": "https://www.gutenberg.org/ebooks/1342.kindle.images",
	}
	got := pickGutenbergEPUB(formats)
	if got == "" {
		t.Errorf("expected EPUB url, got empty")
	}
	// Empty map → empty
	if v := pickGutenbergEPUB(map[string]string{}); v != "" {
		t.Errorf("expected empty for empty map, got %q", v)
	}
}

func TestStripTags(t *testing.T) {
	got := stripTags("<p>Hello <em>world</em>.</p>  Multiple   spaces.")
	want := "Hello world. Multiple spaces."
	if got != want {
		t.Errorf("stripTags = %q, want %q", got, want)
	}
}

func TestAbsURL(t *testing.T) {
	cases := []struct{ base, in, want string }{
		{"https://standardebooks.org", "/foo/bar", "https://standardebooks.org/foo/bar"},
		{"https://standardebooks.org", "https://other.com/x", "https://other.com/x"},
		{"https://standardebooks.org", "rel/path", "https://standardebooks.org/rel/path"},
		{"https://standardebooks.org", "", ""},
	}
	for _, c := range cases {
		got := absURL(c.base, c.in)
		if got != c.want {
			t.Errorf("absURL(%q, %q) = %q, want %q", c.base, c.in, got, c.want)
		}
	}
}

func TestUniqueFilename(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/test.epub"
	if got := uniqueFilename(p); got != p {
		t.Errorf("expected %q for non-existent path, got %q", p, got)
	}
	// Create the original — uniqueFilename should now suffix.
	if err := writeFileAtomic(p, []byte("PK\x03\x04")); err != nil {
		t.Fatal(err)
	}
	got := uniqueFilename(p)
	if got == p {
		t.Errorf("expected distinct path when original exists, got same")
	}
	if got != dir+"/test-2.epub" {
		t.Errorf("expected -2 suffix, got %q", got)
	}
}

func TestSearchEmptyQueryRejected(t *testing.T) {
	_, err := Search(context.Background(), "  ", DiscoverOptions{})
	if err == nil {
		t.Errorf("expected error for empty query")
	}
}
