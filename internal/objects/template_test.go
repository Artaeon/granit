package objects

import (
	"strings"
	"testing"
)

func TestPathFor_DefaultPattern(t *testing.T) {
	got := PathFor(Type{}, "Quick Note")
	if got != "Quick Note.md" {
		t.Fatalf("got %q", got)
	}
}

func TestPathFor_FolderAndPattern(t *testing.T) {
	tt := Type{Folder: "People", FilenamePattern: "{title}"}
	got := PathFor(tt, "Alice Chen")
	if got != "People/Alice Chen.md" {
		t.Fatalf("got %q", got)
	}
}

func TestPathFor_StripsBadFilesystemChars(t *testing.T) {
	tt := Type{Folder: "X"}
	got := PathFor(tt, `bad: name / really*`)
	if strings.ContainsAny(got, `/:*?"<>|\`[1:]) { // skip the legitimate "/" separator
		t.Fatalf("path still contains forbidden chars: %q", got)
	}
}

func TestPathFor_PatternWithSuffix(t *testing.T) {
	tt := Type{Folder: "X", FilenamePattern: "{title}.note.md"}
	got := PathFor(tt, "hello")
	if got != "X/hello.note.md" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildFrontmatter_RequiredAndDefaults(t *testing.T) {
	tt := Type{
		ID: "article",
		Properties: []Property{
			{Name: "title", Kind: KindText, Required: true},
			{Name: "url", Kind: KindURL, Required: true},
			{Name: "saved", Kind: KindDate, Default: "{today}"},
			{Name: "status", Kind: KindSelect, Default: "to-read"},
		},
	}
	out := BuildFrontmatter(tt, "My Article", nil)
	if !strings.HasPrefix(out, "---\n") || !strings.Contains(out, "\n---\n\n") {
		t.Fatalf("frontmatter delimiters wrong:\n%s", out)
	}
	for _, want := range []string{
		"type: article", "title: My Article", "url:", "saved:", "status: to-read",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// {today} substituted (today's year is reliably "20")
	if !strings.Contains(out, "saved: 20") {
		t.Errorf("expected substituted date in saved:\n%s", out)
	}
}

func TestBuildFrontmatter_ExtrasOverlay(t *testing.T) {
	tt := Type{
		ID: "person",
		Properties: []Property{
			{Name: "title", Kind: KindText, Required: true},
			{Name: "email", Kind: KindText, Required: true},
		},
	}
	out := BuildFrontmatter(tt, "Alice", map[string]string{
		"email":   "alice@example.com",
		"company": "Acme",
	})
	if !strings.Contains(out, "email: alice@example.com") {
		t.Errorf("extras didn't overlay required prop:\n%s", out)
	}
	if !strings.Contains(out, "company: Acme") {
		t.Errorf("extra unknown prop dropped:\n%s", out)
	}
}

func TestBuildFrontmatter_QuotesValueWithColonSpace(t *testing.T) {
	tt := Type{
		ID: "x",
		Properties: []Property{
			{Name: "title", Kind: KindText, Required: true},
			{Name: "summary", Kind: KindText, Default: "Status: open"},
		},
	}
	out := BuildFrontmatter(tt, "Item", nil)
	if !strings.Contains(out, `summary: "Status: open"`) {
		t.Errorf("expected quoted value:\n%s", out)
	}
}

func TestSanitiseFilename(t *testing.T) {
	cases := map[string]string{
		"Alice Chen":   "Alice Chen",
		"a/b":          "ab",
		"Q? :wow":      "Q wow",
		"  ":           "untitled",
		"file<>name":   "filename",
	}
	for in, want := range cases {
		if got := SanitiseFilename(in); got != want {
			t.Errorf("SanitiseFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
