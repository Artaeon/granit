package books

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		// Historical figures with titles trip the naive surname-first
		// flip — Gutendex stores them as "FullName, Title" rather than
		// the archival "Surname, Given" shape, so we must NOT flip
		// (otherwise "Marcus Aurelius, Emperor of Rome" becomes the
		// nonsensical "Emperor of Rome Marcus Aurelius"). Single-word
		// surname is the heuristic that distinguishes the two shapes.
		{"Marcus Aurelius, Emperor of Rome", "Marcus Aurelius, Emperor of Rome"},
		{"Saint Augustine, Bishop of Hippo", "Saint Augustine, Bishop of Hippo"},
		// Edge: empty given half — leave the original alone rather
		// than producing a stray-comma string.
		{"Dickens,", "Dickens,"},
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

func TestStandardEbooksReturnsPaywalledSentinel(t *testing.T) {
	// Standard Ebooks moved every catalogue OPDS feed behind a
	// paid Patrons Circle subscription in 2026. The sentinel error
	// lets the handler render a friendly "subscription required"
	// notice instead of a generic 502 — assert it survives wrapping
	// so caller code can keep using IsStandardEbooksPaywalled().
	resp, err := Search(context.Background(), "tolstoy", DiscoverOptions{
		Sources: []Source{SourceStandardEbook},
	})
	if err == nil {
		t.Fatalf("expected SE-only search to error, got %+v", resp)
	}
	if len(resp.Warnings) != 1 || resp.Warnings[0].Source != SourceStandardEbook {
		t.Errorf("expected exactly one SE warning; got %+v", resp.Warnings)
	}
	// Direct Import call must hit the same sentinel.
	if _, err := Import(context.Background(), t.TempDir(), SourceStandardEbook, "https://standardebooks.org/x", ""); !IsStandardEbooksPaywalled(err) {
		t.Errorf("Import(SE) should return paywalled sentinel; got %v", err)
	}
}

func TestImportRejectsHTTP(t *testing.T) {
	// We refuse plaintext HTTP — every legitimate catalogue serves
	// HTTPS, and a downgrade would let an MITM swap an EPUB for an
	// arbitrary payload that ends up under <vault>/Books/.
	_, err := Import(context.Background(), t.TempDir(), SourceGutenberg, "http://example.com/x.epub", "")
	if err == nil {
		t.Errorf("expected Import to reject http:// URL")
	}
}

func TestImportRejectsEmptyURL(t *testing.T) {
	_, err := Import(context.Background(), t.TempDir(), SourceGutenberg, "", "")
	if err == nil {
		t.Errorf("expected Import to reject empty URL")
	}
}

func TestImportStreamsFileToVaultBooks(t *testing.T) {
	// End-to-end: serve a real minimal EPUB over httptest, run Import,
	// verify the file lands in <vault>/Books/, the response is a
	// valid Summary, and Scan() picks the file up on the next call —
	// the user-visible "I added a book and reloaded → nothing" bug
	// would manifest as Scan() returning empty after this passed.
	src := buildMinimalEPUB(t)
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	vault := t.TempDir()
	// Import only allows https://, and httptest.NewTLSServer's cert
	// is self-signed — inject the test server's already-trusted
	// client through the package-level seam so importClient()
	// returns it instead of building a fresh one.
	httpClientForTest = srv.Client()
	t.Cleanup(func() { httpClientForTest = nil })

	sum, err := Import(context.Background(), vault, SourceGutenberg, srv.URL+"/pride.epub", "Pride and Prejudice")
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	// File on disk under <vault>/Books/.
	want := filepath.Join(vault, BooksDirName, "Pride and Prejudice.epub")
	if !strings.HasSuffix(filepath.Join(vault, sum.Path), want) {
		t.Errorf("Summary.Path doesn't point inside Books/: got %q", sum.Path)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected file at %s after Import: %v", want, err)
	}
	// Scan must surface the imported book — this is the regression
	// guard for the "reload → empty shelf" bug.
	all, err := Scan(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != sum.ID {
		t.Errorf("Scan didn't find the imported book; got %+v", all)
	}
	// Re-importing the same URL writes a "-2" suffix rather than
	// clobbering the first file (uniqueFilename branch).
	sum2, err := Import(context.Background(), vault, SourceGutenberg, srv.URL+"/pride.epub", "Pride and Prejudice")
	if err != nil {
		t.Fatalf("second Import failed: %v", err)
	}
	if sum.Path == sum2.Path {
		t.Errorf("second Import should produce distinct path; both got %q", sum.Path)
	}
	all2, _ := Scan(vault)
	if len(all2) != 2 {
		t.Errorf("expected 2 books after duplicate Import, got %d", len(all2))
	}

	// No stray temp files lingering after import. Belongs in this
	// test because the .import-*.tmp temp pattern is a sibling of
	// the final file; a regression that swapped it back to .epub
	// would silently leak unparseable garbage into Books/ on every
	// crashed import.
	entries, _ := os.ReadDir(filepath.Join(vault, BooksDirName))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".import-") {
			t.Errorf("Import left a stale temp file in Books/: %s", e.Name())
		}
	}
}

func TestScanIgnoresImportTempFiles(t *testing.T) {
	// A crashed import (server killed mid-stream) leaves a stale
	// .import-*.tmp file behind. Scan must skip it so it neither
	// appears on the shelf nor wastes a zip-parse attempt every
	// list call.
	vault := t.TempDir()
	booksDir := filepath.Join(vault, BooksDirName)
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Real EPUB so any future regression that tried to scan the
	// temp file would actually succeed and surface it (rather than
	// silently failing to parse and hiding the bug).
	tmp := filepath.Join(booksDir, ".import-leftover.tmp")
	body, _ := os.ReadFile(buildMinimalEPUB(t))
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := Scan(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("Scan should ignore .tmp files; got %d entries: %+v", len(all), all)
	}
}

func TestImportRejectsHTMLMasqueradingAsEPUB(t *testing.T) {
	// A common Gutenberg failure mode: the server serves the
	// "ebook removed" HTML page with a 200, not a 404. Without the
	// PK-magic check we'd happily save garbage into Books/ and the
	// user would discover the failure by trying to open it.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>Not the EPUB you wanted.</body></html>"))
	}))
	t.Cleanup(srv.Close)

	httpClientForTest = srv.Client()
	t.Cleanup(func() { httpClientForTest = nil })

	vault := t.TempDir()
	_, err := Import(context.Background(), vault, SourceGutenberg, srv.URL+"/x.epub", "x")
	if err == nil || !strings.Contains(err.Error(), "magic") {
		t.Errorf("expected PK-magic rejection; got %v", err)
	}
	// Critical: no file lingers under Books/ after the rejected import.
	entries, _ := os.ReadDir(filepath.Join(vault, BooksDirName))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			t.Errorf("expected no committed file after rejection; found %s", e.Name())
		}
	}
}
