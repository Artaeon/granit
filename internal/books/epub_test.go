package books

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// makeEPUBWithChapter writes a minimal one-chapter EPUB with the
// supplied raw chapter body to disk at `path`. Lets sanitiser-end-
// to-end tests inject hostile XHTML without re-stating the whole
// container/OPF skeleton inline.
func makeEPUBWithChapter(t *testing.T, path, chapterBody string) {
	t.Helper()
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
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Hostile</dc:title></metadata>
  <manifest><item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`)
	write("OEBPS/ch1.xhtml", `<?xml version="1.0"?><html>`+chapterBody+`</html>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

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

func TestChapterStripsDocumentEnvelope(t *testing.T) {
	// Chapter() must hand the frontend just the inner <body>
	// content. The reader pastes the result inside an <article> via
	// {@html ...}; if we leak the doctype / <html> / <head> /
	// <title> / <meta> / <link> / <style> wrapping, the head tags
	// render as visible text and the EPUB's CSS fights our
	// reader-prose typography — surfaces as the user-visible
	// "ereader looks completely buggy" bug.
	dir := t.TempDir()
	p := filepath.Join(dir, "envelope.epub")
	makeEPUBWithChapter(t, p, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en">
<head>
<title>The Wrong Title</title>
<meta charset="utf-8"/>
<link rel="stylesheet" type="text/css" href="../css/style.css"/>
<style>p { color: red; font-family: "ComicSans"; }</style>
</head>
<body class="chapter">
<h1>Chapter One</h1>
<p>Real content the reader should display.</p>
</body>
</html>`)

	e, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	html, err := e.Chapter(0, "/api/v1/books/test/asset")
	if err != nil {
		t.Fatal(err)
	}

	// Document envelope must be gone.
	for _, fragment := range []string{
		"<!DOCTYPE", "<?xml", "<html", "</html>", "<head", "</head>",
		"<title>", "<meta", "<link", "ComicSans", "<style",
	} {
		if strings.Contains(html, fragment) {
			t.Errorf("chapter HTML still contains %q (full output: %s)", fragment, html)
		}
	}
	// Real content survives.
	if !strings.Contains(html, "<h1>Chapter One</h1>") {
		t.Errorf("chapter heading missing: %s", html)
	}
	if !strings.Contains(html, "Real content the reader should display.") {
		t.Errorf("chapter paragraph missing: %s", html)
	}
}

func TestChapterFragmentWithoutBodyTag(t *testing.T) {
	// Some EPUBs ship pre-cleaned chapter fragments without an
	// outer <body> — extractChapterBody must fall through to the
	// original input rather than returning empty.
	got := extractChapterBody(`<h1>Just a heading</h1><p>and a paragraph</p>`)
	if !strings.Contains(got, "Just a heading") {
		t.Errorf("fragment input lost: got %q", got)
	}
}

func TestSanitiseStripsStylesheets(t *testing.T) {
	// Independent regression guard for the style-stripping branch
	// of sanitiseChapter — the reader's typography should never
	// have to fight the EPUB's CSS.
	cases := []string{
		`<link rel="stylesheet" href="x.css"/>`,
		`<link href="x.css" rel="stylesheet" type="text/css"/>`,
		`<link rel='stylesheet' href='x.css'>`,
		`<style>body { font: 12px wingdings; }</style>`,
		`<STYLE type="text/css">.x { display: none }</STYLE>`,
	}
	for _, c := range cases {
		got := sanitiseChapter(c)
		if got != "" {
			t.Errorf("sanitiseChapter(%q) = %q, want empty", c, got)
		}
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

func TestSanitiseChapterStripsScripts(t *testing.T) {
	cases := []struct {
		in   string
		gone []string // substrings that must NOT be in the output
		kept []string // substrings that MUST be in the output
	}{
		{
			in:   `<p>Hello</p><script>alert(1)</script>`,
			gone: []string{"<script", "alert(1)"},
			kept: []string{"<p>Hello</p>"},
		},
		{
			// Self-closing form (rare but legal).
			in:   `<p>x</p><script src="evil.js"/>`,
			gone: []string{"<script", "evil.js"},
			kept: []string{"<p>x</p>"},
		},
		{
			// Event handler attribute.
			in:   `<img src="ok.png" onerror="alert(1)" />`,
			gone: []string{"onerror", "alert(1)"},
			kept: []string{`src="ok.png"`},
		},
		{
			// javascript: URL.
			in:   `<a href="javascript:alert(1)">click</a>`,
			gone: []string{"javascript:", "alert(1)"},
			kept: []string{`href=""`, "click"},
		},
		{
			// Multi-line script block — (?is) flags handle it.
			in:   "<script>\nalert(1);\nfoo();\n</script>",
			gone: []string{"alert", "foo"},
		},
	}
	for i, c := range cases {
		got := sanitiseChapter(c.in)
		for _, bad := range c.gone {
			if strings.Contains(got, bad) {
				t.Errorf("case %d: expected %q stripped, got: %s", i, bad, got)
			}
		}
		for _, ok := range c.kept {
			if !strings.Contains(got, ok) {
				t.Errorf("case %d: expected %q kept, got: %s", i, ok, got)
			}
		}
	}
}

func TestChapterIsSanitised(t *testing.T) {
	// End-to-end: malicious chapter HTML inside an EPUB returns
	// scrubbed body to the caller, not the raw zip content.
	dir := t.TempDir()
	path := dir + "/scary.epub"
	makeEPUBWithChapter(t, path, `<body><p>Real text</p><script>alert('xss')</script></body>`)
	e, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	html, err := e.Chapter(0, "/asset")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "alert") || strings.Contains(html, "<script") {
		t.Errorf("script not stripped from chapter: %s", html)
	}
	if !strings.Contains(html, "Real text") {
		t.Errorf("real content stripped too: %s", html)
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

func TestSidecarConcurrentHighlightsAllPersist(t *testing.T) {
	// Same race fix as the annotations package — AddHighlight does
	// LoadSidecar → mutate → SaveSidecar. Without sidecarMu, two
	// rapid highlights could both read the pre-write state and the
	// second writer's commit clobbers the first.
	//
	// User-visible scenario: a fast reader selecting passage after
	// passage in quick succession; the 2 s scroll-progress saver
	// firing concurrently with a highlight add.
	dir := t.TempDir()
	const N = 30
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := AddHighlight(dir, "race-book", Highlight{
				ChapterIdx: 0,
				Text:       fmt.Sprintf("passage-%d", idx),
				Color:      "yellow",
			})
			if err != nil {
				t.Errorf("highlight %d failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
	sc, err := LoadSidecar(dir, "race-book")
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Highlights) != N {
		t.Errorf("expected %d highlights to persist; got %d (lost writes)", N, len(sc.Highlights))
	}
}

func TestSidecarConcurrentProgressAndHighlights(t *testing.T) {
	// Mixed-mode race: scroll-progress saver fires interleaved with
	// highlight adds. Both touch the same sidecar file. The lock
	// covers both code paths so a progress save mid-highlight-add
	// can't lose the highlight (or vice-versa).
	dir := t.TempDir()
	var wg sync.WaitGroup
	const N = 20
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			_, _ = AddHighlight(dir, "mix-book", Highlight{
				ChapterIdx: idx % 5,
				Text:       fmt.Sprintf("passage-%d", idx),
				Color:      "blue",
			})
		}(i)
		go func(idx int) {
			defer wg.Done()
			_ = SaveProgress(dir, "mix-book", Progress{
				ChapterIdx:     idx % 5,
				ScrollFraction: float64(idx) / float64(N),
			})
		}(i)
	}
	wg.Wait()
	sc, err := LoadSidecar(dir, "mix-book")
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Highlights) != N {
		t.Errorf("expected %d highlights to survive concurrent progress saves; got %d", N, len(sc.Highlights))
	}
}
