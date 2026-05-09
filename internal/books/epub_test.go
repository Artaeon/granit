package books

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildMinimalEPUB writes a tiny valid EPUB to disk and returns
// the path. Used by every test below — keeps fixture creation in
// one place so a future schema change to our reader stays
// expressed via this helper rather than scattered XML literals.
func buildMinimalEPUB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.epub")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("mimetype", "application/epub+zip")
	write("META-INF/container.xml", `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)
	write("OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:creator>Jane Tester</dc:creator>
  </metadata>
  <manifest>
    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover" href="cover.png" media-type="image/png" properties="cover-image"/>
    <item id="img1" href="images/fig.png" media-type="image/png"/>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
    <itemref idref="ch2"/>
  </spine>
</package>`)
	write("OEBPS/ch1.xhtml", `<html><body><h1>Chapter One</h1><p>Hello <img src="images/fig.png" /></p></body></html>`)
	write("OEBPS/ch2.xhtml", `<html><body><h1>Chapter Two</h1><a href="#footnote">jump</a></body></html>`)
	write("OEBPS/cover.png", "PNG-COVER-BYTES")
	write("OEBPS/images/fig.png", "PNG-FIG-BYTES")
	write("OEBPS/nav.xhtml", `<html><body><nav epub:type="toc"><ol>
  <li><a href="ch1.xhtml">One</a></li>
  <li><a href="ch2.xhtml">Two</a><ol><li><a href="ch2.xhtml#sec">Two.A</a></li></ol></li>
</ol></nav></body></html>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenAndMetadata(t *testing.T) {
	p := buildMinimalEPUB(t)
	e, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer e.Close()
	if e.Title != "Test Book" {
		t.Errorf("Title = %q, want %q", e.Title, "Test Book")
	}
	if len(e.Authors) != 1 || e.Authors[0] != "Jane Tester" {
		t.Errorf("Authors = %v, want [Jane Tester]", e.Authors)
	}
	if e.CoverID != "cover" {
		t.Errorf("CoverID = %q, want cover", e.CoverID)
	}
	if len(e.Spine) != 2 {
		t.Fatalf("Spine len = %d, want 2", len(e.Spine))
	}
	if e.Spine[0].IDRef != "ch1" || e.Spine[1].IDRef != "ch2" {
		t.Errorf("Spine = %+v", e.Spine)
	}
}

func TestChapterRewritesAssetRefs(t *testing.T) {
	p := buildMinimalEPUB(t)
	e, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	html, err := e.Chapter(0, "/api/v1/books/test/asset")
	if err != nil {
		t.Fatalf("Chapter: %v", err)
	}
	// Image src must have been rewritten through the asset prefix.
	if !strings.Contains(html, `src="/api/v1/books/test/asset/images/fig.png"`) {
		t.Errorf("img src not rewritten: %s", html)
	}
	// Fragment-only refs must NOT be rewritten.
	html2, _ := e.Chapter(1, "/api/v1/books/test/asset")
	if !strings.Contains(html2, `href="#footnote"`) {
		t.Errorf("fragment ref clobbered: %s", html2)
	}
}

func TestChapterOutOfRange(t *testing.T) {
	p := buildMinimalEPUB(t)
	e, _ := Open(p)
	defer e.Close()
	if _, err := e.Chapter(99, ""); err != ErrInvalidChapter {
		t.Errorf("expected ErrInvalidChapter, got %v", err)
	}
}

func TestCoverBytes(t *testing.T) {
	p := buildMinimalEPUB(t)
	e, _ := Open(p)
	defer e.Close()
	data, mt, err := e.CoverBytes()
	if err != nil {
		t.Fatalf("CoverBytes: %v", err)
	}
	if !bytes.Equal(data, []byte("PNG-COVER-BYTES")) {
		t.Errorf("cover bytes mismatch: %q", data)
	}
	if mt != "image/png" {
		t.Errorf("media type %q, want image/png", mt)
	}
}

func TestAsset(t *testing.T) {
	p := buildMinimalEPUB(t)
	e, _ := Open(p)
	defer e.Close()
	data, mt, err := e.Asset("images/fig.png")
	if err != nil {
		t.Fatalf("Asset: %v", err)
	}
	if !bytes.Equal(data, []byte("PNG-FIG-BYTES")) {
		t.Errorf("asset bytes mismatch")
	}
	if mt != "image/png" {
		t.Errorf("media type %q", mt)
	}
}

func TestParseNavTOC(t *testing.T) {
	p := buildMinimalEPUB(t)
	e, _ := Open(p)
	defer e.Close()
	if len(e.TOC) != 2 {
		t.Fatalf("TOC top-level len = %d, want 2: %+v", len(e.TOC), e.TOC)
	}
	if e.TOC[0].Title != "One" || e.TOC[0].SpineIdx != 0 {
		t.Errorf("TOC[0] = %+v", e.TOC[0])
	}
	if len(e.TOC[1].Children) != 1 {
		t.Errorf("TOC[1] missing nested entry: %+v", e.TOC[1])
	}
}

func TestRewriteRefsLeavesDataHrefAlone(t *testing.T) {
	// Earlier versions of the regex rewrote the `href` inside
	// `data-href="x"` because there was no boundary on the attribute
	// name. The fix anchors with (^|\s) so only true href/src land.
	in := `<a data-href="x.html" href="real.html">Click</a>`
	got := rewriteRefs(in, "", "/api/v1/books/test/asset")
	if !strings.Contains(got, `data-href="x.html"`) {
		t.Errorf("data-href was clobbered: %s", got)
	}
	if !strings.Contains(got, `href="/api/v1/books/test/asset/real.html"`) {
		t.Errorf("real href not rewritten: %s", got)
	}
}

func TestSlugIDStableAcrossOpens(t *testing.T) {
	a := SlugID("The Great Gatsby", "/foo/gatsby.epub")
	b := SlugID("The Great Gatsby", "/foo/gatsby.epub")
	if a != b {
		t.Errorf("SlugID not stable: %q vs %q", a, b)
	}
	c := SlugID("The Great Gatsby", "/bar/gatsby.epub")
	if a == c {
		t.Errorf("SlugID should differ for different paths: %q", a)
	}
}

func TestSidecarRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := AddHighlight(dir, "test-book", Highlight{
		ChapterIdx: 1,
		Text:       "hello world",
		Color:      "yellow",
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveProgress(dir, "test-book", Progress{ChapterIdx: 3, ScrollFraction: 0.5}); err != nil {
		t.Fatal(err)
	}
	sc, err := LoadSidecar(dir, "test-book")
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Highlights) != 1 || sc.Highlights[0].Text != "hello world" {
		t.Errorf("highlights round-trip failed: %+v", sc.Highlights)
	}
	if sc.Progress.ChapterIdx != 3 || sc.Progress.FurthestChapter != 3 {
		t.Errorf("progress round-trip failed: %+v", sc.Progress)
	}
	// Furthest must NOT regress when user jumps back.
	if err := SaveProgress(dir, "test-book", Progress{ChapterIdx: 1}); err != nil {
		t.Fatal(err)
	}
	sc, _ = LoadSidecar(dir, "test-book")
	if sc.Progress.FurthestChapter != 3 {
		t.Errorf("furthest regressed: %d", sc.Progress.FurthestChapter)
	}
}
