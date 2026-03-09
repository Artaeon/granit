package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLink_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// Exact path with .md extension should resolve directly.
	resolved := idx.ResolveLink("note.md")
	if resolved != "note.md" {
		t.Errorf("expected 'note.md', got %q", resolved)
	}
}

func TestResolveLink_WithMdExtension(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Readme"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// "readme" without extension should resolve to "readme.md".
	resolved := idx.ResolveLink("readme")
	if resolved != "readme.md" {
		t.Errorf("expected 'readme.md', got %q", resolved)
	}
}

func TestResolveLink_BaseNameMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subfolder"), 0755)
	os.WriteFile(filepath.Join(dir, "subfolder", "deep-note.md"), []byte("# Deep"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// "deep-note" should resolve to "subfolder/deep-note.md" via basename matching.
	resolved := idx.ResolveLink("deep-note")
	expected := filepath.Join("subfolder", "deep-note.md")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}

func TestResolveLink_HeadingAnchor(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target\n## Heading"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// "target#heading" should strip the anchor and resolve to "target.md".
	resolved := idx.ResolveLink("target#heading")
	if resolved != "target.md" {
		t.Errorf("expected 'target.md', got %q", resolved)
	}
}

func TestResolveLink_NotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "existing.md"), []byte("# Existing"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// Non-existent note should return empty string.
	resolved := idx.ResolveLink("nonexistent")
	if resolved != "" {
		t.Errorf("expected empty string, got %q", resolved)
	}
}

func TestResolveLink_EmptyLink(t *testing.T) {
	dir := t.TempDir()

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// Empty string should return empty.
	resolved := idx.ResolveLink("")
	if resolved != "" {
		t.Errorf("expected empty string, got %q", resolved)
	}
}

func TestIndex_CircularLinks(t *testing.T) {
	// A->B->A should not cause an infinite loop during Build.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "A.md"), []byte("Link to [[B]]"), 0644)
	os.WriteFile(filepath.Join(dir, "B.md"), []byte("Link to [[A]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	// Should complete without hanging.
	idx.Build()

	blA := idx.GetBacklinks("A.md")
	blB := idx.GetBacklinks("B.md")

	if len(blA) != 1 || blA[0] != "B.md" {
		t.Errorf("A backlinks: expected [B.md], got %v", blA)
	}
	if len(blB) != 1 || blB[0] != "A.md" {
		t.Errorf("B backlinks: expected [A.md], got %v", blB)
	}
}

func TestIndex_SelfLink(t *testing.T) {
	// A note linking to itself should create a backlink from itself.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recursive.md"), []byte("See [[recursive]] for more"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	bl := idx.GetBacklinks("recursive.md")
	if len(bl) != 1 {
		t.Fatalf("expected 1 self-backlink, got %d: %v", len(bl), bl)
	}
	if bl[0] != "recursive.md" {
		t.Errorf("expected self-link from 'recursive.md', got %q", bl[0])
	}

	// Outgoing links should also contain the self-reference.
	out := idx.GetOutgoingLinks("recursive.md")
	if len(out) != 1 || out[0] != "recursive" {
		t.Errorf("expected outgoing link 'recursive', got %v", out)
	}
}

func TestResolveLink_HeadingOnlyAnchor(t *testing.T) {
	// A link like "#heading" (heading-only, no note name) should return empty
	// since stripping the anchor leaves an empty link.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	resolved := idx.ResolveLink("#heading")
	if resolved != "" {
		t.Errorf("expected empty string for heading-only anchor, got %q", resolved)
	}
}

func TestResolveLink_SubfolderPathWithAnchor(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "folder"), 0755)
	os.WriteFile(filepath.Join(dir, "folder", "doc.md"), []byte("# Doc"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// "doc#section" should resolve to "folder/doc.md" after stripping anchor.
	resolved := idx.ResolveLink("doc#section")
	expected := filepath.Join("folder", "doc.md")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}

func TestIndex_BuildWithHeadingLinks(t *testing.T) {
	// Ensure heading anchors in wikilinks still create proper backlinks.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "source.md"), []byte("Go to [[target#details]]"), 0644)
	os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target\n## Details"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	bl := idx.GetBacklinks("target.md")
	if len(bl) != 1 {
		t.Fatalf("expected 1 backlink, got %d", len(bl))
	}
	if bl[0] != "source.md" {
		t.Errorf("expected backlink from 'source.md', got %q", bl[0])
	}
}
