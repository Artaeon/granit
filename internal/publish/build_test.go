package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestFile creates a file at path with content under tmpDir; returns
// the absolute path. Helper for the build tests below.
func writeTestFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

// Build on a small folder produces every expected output file. Smoke test
// for the high-level pipeline — if any one of these is missing it means
// a refactor broke the file-emission contract documented in PUBLISH.md.
func TestBuild_EmitsExpectedFiles(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeTestFile(t, src, "alpha.md", "# Alpha\n\nLinks to [[Beta]].\n")
	writeTestFile(t, src, "beta.md", "# Beta\n\nLinks to [[Alpha]] and references #research.\n")

	res, err := Build(Config{
		SiteTitle: "Test Site",
		SourceDir: src,
		OutputDir: out,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.NotesPublished != 2 {
		t.Errorf("NotesPublished = %d, want 2", res.NotesPublished)
	}

	want := []string{
		"index.html", "404.html", "feed.xml",
		"sitemap.xml", "robots.txt", "style.css",
		".nojekyll",
		"notes/alpha.html", "notes/beta.html",
		"tags/research.html", "tags/index.html",
	}
	for _, f := range want {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected %s: %v", f, err)
		}
	}
}

// Wikilinks resolve to the right HTML URL across the published set.
// [[Beta]] in alpha.md should land on notes/beta.html, with the
// canonical Beta title as link text (since the source used the bare
// "Beta" target without |Display).
func TestBuild_WikilinksResolveToHTML(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeTestFile(t, src, "alpha.md", "# Alpha\n\nLinks to [[Beta]].\n")
	writeTestFile(t, src, "beta.md", "# Beta\n\nFollow-up note.\n")

	if _, err := Build(Config{SiteTitle: "T", SourceDir: src, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(out, "notes/alpha.html"))
	if !strings.Contains(string(html), `href="beta.html"`) {
		t.Errorf("alpha.html should link to beta.html; got:\n%s", html)
	}
	if !strings.Contains(string(html), ">Beta</a>") {
		t.Errorf("alpha.html link text should be the canonical title 'Beta'")
	}
}

// Backlinks panel auto-populates: if A links to B, B's page must show A
// in its "Linked from" section.
func TestBuild_BacklinksAutoPopulate(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeTestFile(t, src, "alpha.md", "# Alpha\n\nLinks to [[Beta]].\n")
	writeTestFile(t, src, "beta.md", "# Beta\n\n")

	if _, err := Build(Config{SiteTitle: "T", SourceDir: src, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(out, "notes/beta.html"))
	if !strings.Contains(string(html), `<aside class="backlinks">`) {
		t.Error("beta.html missing backlinks aside")
	}
	if !strings.Contains(string(html), ">Alpha</a>") {
		t.Error("beta.html backlinks should list 'Alpha'")
	}
}

// Frontmatter `publish: false` excludes the note from the build entirely.
// And `legal: impressum` routes to /impressum.html at the root, not under
// /notes/.
func TestBuild_FrontmatterDirectives(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeTestFile(t, src, "secret.md", "---\npublish: false\n---\n# Secret\n")
	writeTestFile(t, src, "imp.md", "---\nlegal: impressum\n---\n# Impressum\n")
	writeTestFile(t, src, "real.md", "# Real\n")

	if _, err := Build(Config{SiteTitle: "T", SourceDir: src, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "notes/secret.html")); err == nil {
		t.Error("publish:false note must NOT be emitted")
	}
	if _, err := os.Stat(filepath.Join(out, "impressum.html")); err != nil {
		t.Errorf("legal: impressum must render to /impressum.html at root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "notes/imp.html")); err == nil {
		t.Error("legal page must NOT also render under /notes/")
	}
	// Footer on the real note must include the Impressum link.
	html, _ := os.ReadFile(filepath.Join(out, "notes/real.html"))
	if !strings.Contains(string(html), `href="../impressum.html"`) {
		t.Error("regular note footer must link to ../impressum.html")
	}
}

// Mermaid post-processing must NOT corrupt unrelated code blocks. This
// is a regression guard for the bug where ReplaceAll("</code></pre>",
// "</pre>") stripped the closing tag from every chroma-highlighted code
// block on the same page — leaving an unclosed <code> in the HTML.
func TestBuild_MermaidDoesNotBreakOtherCodeBlocks(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeTestFile(t, src, "mixed.md", "# Mixed\n\n"+
		"```mermaid\nflowchart LR\nA --> B\n```\n\n"+
		"```go\nfunc main() { fmt.Println(\"hi\") }\n```\n")

	if _, err := Build(Config{
		SiteTitle: "T", SourceDir: src, OutputDir: out,
		Mermaid: true,
	}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(out, "notes/mixed.html"))
	s := string(html)
	// Mermaid block: <pre class="mermaid">...</pre>
	if !strings.Contains(s, `<pre class="mermaid">`) {
		t.Error("mermaid block missing rewrite to <pre class=\"mermaid\">")
	}
	// Go code block: chroma renders <pre style="..."><code>...</code></pre>.
	// The closing </code></pre> MUST survive — historically the mermaid
	// rewrite stripped these too.
	openCode := strings.Count(s, "<code")
	closeCode := strings.Count(s, "</code>")
	if openCode != closeCode {
		t.Errorf("unbalanced <code>/</code>: %d open, %d close — mermaid rewrite likely corrupted other code blocks\n%s",
			openCode, closeCode, s)
	}
}

// Image asset copying mirrors non-md files into the output directory
// preserving relative paths.
func TestBuild_ImageAssetCopying(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	writeTestFile(t, src, "note.md", "# N\n\n![alt](./pic.png)\n")
	writeTestFile(t, src, "pic.png", "fake-png-bytes")
	writeTestFile(t, src, "subdir/diagram.svg", "<svg/>")

	res, err := Build(Config{SiteTitle: "T", SourceDir: src, OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if res.AssetsCopied != 2 {
		t.Errorf("AssetsCopied = %d, want 2", res.AssetsCopied)
	}
	if _, err := os.Stat(filepath.Join(out, "pic.png")); err != nil {
		t.Errorf("pic.png missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "subdir/diagram.svg")); err != nil {
		t.Errorf("subdir/diagram.svg missing: %v", err)
	}
}

// Image path rewriting outputs the right number of ../ hops based on
// the rendered page's depth. Regular notes (notes/<slug>.html) get one
// ../, legal pages (root) get none.
func TestRewriteImagePaths_DepthAware(t *testing.T) {
	in := "![alt](./pic.png)"
	if got := rewriteImagePaths(in, "note.md", "notes/note.html"); got != "![alt](../pic.png)" {
		t.Errorf("regular note: got %q, want \"![alt](../pic.png)\"", got)
	}
	if got := rewriteImagePaths(in, "imp.md", "impressum.html"); got != "![alt](pic.png)" {
		t.Errorf("legal page: got %q, want \"![alt](pic.png)\"", got)
	}
	// Absolute URLs are left untouched.
	abs := "![logo](https://example.com/logo.png)"
	if got := rewriteImagePaths(abs, "note.md", "notes/note.html"); got != abs {
		t.Errorf("absolute URL must not be rewritten; got %q", got)
	}
}

// Frontmatter parsing handles both array and comma-string tag formats,
// extracts author + date + title overrides, and flips noindex/publish
// flags correctly.
func TestParseNote_FrontmatterFields(t *testing.T) {
	body := []byte(`---
title: Custom Title
date: 2026-04-08
tags: [alpha, beta]
author: Jane
noindex: true
---

# Body H1
Hello world.
`)
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "x.md")
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	n, err := parseNote("x.md", body, &fakeDirEntry{info: d})
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Custom Title" {
		t.Errorf("Title: got %q, want %q", n.Title, "Custom Title")
	}
	if n.Date != "2026-04-08" {
		t.Errorf("Date: got %q, want %q", n.Date, "2026-04-08")
	}
	if n.Author != "Jane" {
		t.Errorf("Author: got %q, want %q", n.Author, "Jane")
	}
	if !n.NoIndex {
		t.Errorf("NoIndex: expected true")
	}
	if len(n.Tags) != 2 || n.Tags[0] != "alpha" || n.Tags[1] != "beta" {
		t.Errorf("Tags: got %v, want [alpha beta]", n.Tags)
	}
}

// fakeDirEntry satisfies fs.DirEntry just enough for parseNote.Info().
type fakeDirEntry struct{ info os.FileInfo }

func (f *fakeDirEntry) Name() string               { return f.info.Name() }
func (f *fakeDirEntry) IsDir() bool                { return f.info.IsDir() }
func (f *fakeDirEntry) Type() os.FileMode          { return f.info.Mode().Type() }
func (f *fakeDirEntry) Info() (os.FileInfo, error) { return f.info, nil }

// detectLegalKind picks up filenames AND frontmatter, with English
// aliases mapping to canonical German names.
func TestDetectLegalKind(t *testing.T) {
	cases := []struct {
		rel, frontmatterLegal, want string
	}{
		{"impressum.md", "", "impressum"},
		{"IMPRESSUM.md", "", "impressum"},
		{"imprint.md", "", "impressum"},
		{"datenschutz.md", "", "datenschutz"},
		{"privacy.md", "", "datenschutz"},
		{"privacy-policy.md", "", "datenschutz"},
		{"random.md", "impressum", "impressum"},
		{"random.md", "datenschutz", "datenschutz"},
		{"random.md", "imprint", "impressum"},
		{"random.md", "privacy", "datenschutz"},
		{"plain.md", "", ""},
	}
	for _, c := range cases {
		fm := map[string]interface{}{}
		if c.frontmatterLegal != "" {
			fm["legal"] = c.frontmatterLegal
		}
		got := detectLegalKind(c.rel, fm)
		if got != c.want {
			t.Errorf("rel=%q legal=%q: got %q, want %q", c.rel, c.frontmatterLegal, got, c.want)
		}
	}
}

// RSS feed honors the FeedItems cap; with 0, every regular note is
// emitted, with N>0 only the newest N appear.
func TestBuild_RSSFeedCap(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	for i, d := range []string{"2026-01-01", "2026-02-01", "2026-03-01", "2026-04-01"} {
		writeTestFile(t, src,
			"note"+string(rune('a'+i))+".md",
			"---\ndate: "+d+"\n---\n# N"+string(rune('a'+i))+"\n",
		)
	}

	// Cap at 2 → only 2 items in feed.
	if _, err := Build(Config{
		SiteTitle: "T", SourceDir: src, OutputDir: out,
		FeedItems: 2,
	}); err != nil {
		t.Fatal(err)
	}
	feed, _ := os.ReadFile(filepath.Join(out, "feed.xml"))
	if got := strings.Count(string(feed), "<item>"); got != 2 {
		t.Errorf("FeedItems=2 → got %d items, want 2", got)
	}
}
