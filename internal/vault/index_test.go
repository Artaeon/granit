package vault

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestNewIndex(t *testing.T) {
	dir := t.TempDir()
	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}

	idx := NewIndex(v)

	if idx.Backlinks == nil {
		t.Fatal("expected Backlinks map to be initialized, got nil")
	}
	if len(idx.Backlinks) != 0 {
		t.Errorf("expected empty Backlinks map, got %d entries", len(idx.Backlinks))
	}
}

func TestBuildCreatesBacklinks(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "A.md"), []byte("Link to [[B]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "B.md"), []byte("# Note B\nJust content."), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("B.md")
	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink to B.md, got %d", len(backlinks))
	}
	if backlinks[0] != "A.md" {
		t.Errorf("expected backlink from 'A.md', got '%s'", backlinks[0])
	}
}

func TestBuildCrossDirectoryLinkResolution(t *testing.T) {
	// Obsidian shortest-path: [[nested]] should resolve to sub/nested.md
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "root.md"), []byte("Link to [[nested]]"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "sub", "nested.md"), []byte("# Nested note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks(filepath.Join("sub", "nested.md"))
	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink to sub/nested.md, got %d", len(backlinks))
	}
	if backlinks[0] != "root.md" {
		t.Errorf("expected backlink from 'root.md', got '%s'", backlinks[0])
	}
}

func TestBuildCrossDirectoryDeepNesting(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "x"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "a", "b", "deep.md"), []byte("Link to [[shallow]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "x", "shallow.md"), []byte("# Shallow"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks(filepath.Join("x", "shallow.md"))
	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink to x/shallow.md, got %d", len(backlinks))
	}
	if backlinks[0] != filepath.Join("a", "b", "deep.md") {
		t.Errorf("expected backlink from 'a/b/deep.md', got '%s'", backlinks[0])
	}
}

func TestGetBacklinksReturnsCorrectSources(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "src1.md"), []byte("[[target]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "src2.md"), []byte("See [[target]] for info"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "unrelated.md"), []byte("# No links here"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("target.md")
	sort.Strings(backlinks)

	if len(backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d: %v", len(backlinks), backlinks)
	}
	if backlinks[0] != "src1.md" {
		t.Errorf("expected 'src1.md', got '%s'", backlinks[0])
	}
	if backlinks[1] != "src2.md" {
		t.Errorf("expected 'src2.md', got '%s'", backlinks[1])
	}
}

func TestGetBacklinksNonExistentNote(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("does-not-exist.md")
	if backlinks != nil {
		t.Errorf("expected nil backlinks for non-existent note, got %v", backlinks)
	}
}

func TestGetOutgoingLinksReturnsCorrectTargets(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "hub.md"), []byte("Links to [[A]] and [[B]] and [[C]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "A.md"), []byte("# A"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "B.md"), []byte("# B"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "C.md"), []byte("# C"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	outgoing := idx.GetOutgoingLinks("hub.md")

	if len(outgoing) != 3 {
		t.Fatalf("expected 3 outgoing links, got %d: %v", len(outgoing), outgoing)
	}

	sort.Strings(outgoing)
	expected := []string{"A", "B", "C"}
	for i, exp := range expected {
		if outgoing[i] != exp {
			t.Errorf("outgoing[%d]: expected '%s', got '%s'", i, exp, outgoing[i])
		}
	}
}

func TestGetOutgoingLinksNonExistentNote(t *testing.T) {
	dir := t.TempDir()
	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	outgoing := idx.GetOutgoingLinks("ghost.md")

	if outgoing != nil {
		t.Errorf("expected nil for non-existent note, got %v", outgoing)
	}
}

func TestBuildCircularLinks(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "A.md"), []byte("Link to [[B]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "B.md"), []byte("Link to [[A]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	// Should not hang or panic
	idx.Build()

	backlinkA := idx.GetBacklinks("A.md")
	backlinkB := idx.GetBacklinks("B.md")

	if len(backlinkA) != 1 || backlinkA[0] != "B.md" {
		t.Errorf("expected A.md backlink from B.md, got %v", backlinkA)
	}
	if len(backlinkB) != 1 || backlinkB[0] != "A.md" {
		t.Errorf("expected B.md backlink from A.md, got %v", backlinkB)
	}
}

func TestBuildCircularTriangle(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "A.md"), []byte("[[B]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "B.md"), []byte("[[C]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "C.md"), []byte("[[A]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	if bl := idx.GetBacklinks("A.md"); len(bl) != 1 || bl[0] != "C.md" {
		t.Errorf("A backlinks: expected [C.md], got %v", bl)
	}
	if bl := idx.GetBacklinks("B.md"); len(bl) != 1 || bl[0] != "A.md" {
		t.Errorf("B backlinks: expected [A.md], got %v", bl)
	}
	if bl := idx.GetBacklinks("C.md"); len(bl) != 1 || bl[0] != "B.md" {
		t.Errorf("C backlinks: expected [B.md], got %v", bl)
	}
}

func TestBuildLinksToNonExistentNote(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "note.md"), []byte("Link to [[phantom]] that does not exist"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	// Should not panic
	idx.Build()

	// Outgoing links still recorded in note
	outgoing := idx.GetOutgoingLinks("note.md")
	if len(outgoing) != 1 || outgoing[0] != "phantom" {
		t.Errorf("expected outgoing link 'phantom', got %v", outgoing)
	}

	// No backlink entry for the phantom note since it does not exist
	backlinks := idx.GetBacklinks("phantom.md")
	if backlinks != nil {
		t.Errorf("expected nil backlinks for non-existent target, got %v", backlinks)
	}
}

func TestBuildMultipleLinksToNonExistentNotes(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "linker.md"), []byte("[[ghost1]] and [[ghost2]] and [[ghost3]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	// Should not panic even with all links dangling
	idx.Build()

	if len(idx.Backlinks) != 0 {
		t.Errorf("expected no backlink entries (all targets missing), got %d", len(idx.Backlinks))
	}
}

func TestBuildMultipleNotesLinkToSameTarget(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "hub.md"), []byte("# Hub"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "spoke1.md"), []byte("Back to [[hub]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "spoke2.md"), []byte("Also see [[hub]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "spoke3.md"), []byte("Ref [[hub]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("hub.md")
	sort.Strings(backlinks)

	if len(backlinks) != 3 {
		t.Fatalf("expected 3 backlinks to hub.md, got %d: %v", len(backlinks), backlinks)
	}

	expected := []string{"spoke1.md", "spoke2.md", "spoke3.md"}
	for i, exp := range expected {
		if backlinks[i] != exp {
			t.Errorf("backlinks[%d]: expected '%s', got '%s'", i, exp, backlinks[i])
		}
	}
}

func TestBuildPopulatesNoteBacklinksField(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target note"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "linker1.md"), []byte("See [[target]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "linker2.md"), []byte("Also [[target]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	targetNote := v.GetNote("target.md")
	if targetNote == nil {
		t.Fatal("expected target note, got nil")
	}

	sort.Strings(targetNote.Backlinks)

	if len(targetNote.Backlinks) != 2 {
		t.Fatalf("expected Note.Backlinks to have 2 entries, got %d: %v",
			len(targetNote.Backlinks), targetNote.Backlinks)
	}
	if targetNote.Backlinks[0] != "linker1.md" {
		t.Errorf("expected 'linker1.md', got '%s'", targetNote.Backlinks[0])
	}
	if targetNote.Backlinks[1] != "linker2.md" {
		t.Errorf("expected 'linker2.md', got '%s'", targetNote.Backlinks[1])
	}
}

func TestBuildNoteWithNoBacklinksHasNilField(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "lonely.md"), []byte("# All alone"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "other.md"), []byte("# Other note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	lonely := v.GetNote("lonely.md")
	if lonely == nil {
		t.Fatal("expected lonely note, got nil")
	}
	if lonely.Backlinks != nil {
		t.Errorf("expected nil Backlinks for note with no incoming links, got %v", lonely.Backlinks)
	}
}

func TestBuildResetsBacklinksOnRebuild(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "A.md"), []byte("[[B]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "B.md"), []byte("# B"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	if len(idx.GetBacklinks("B.md")) != 1 {
		t.Fatal("expected 1 backlink after first build")
	}

	// Modify vault: remove the link from A
	v.Notes["A.md"].Links = []string{}
	v.Notes["A.md"].Content = "# A with no links"

	idx.Build()

	if bl := idx.GetBacklinks("B.md"); len(bl) != 0 {
		t.Errorf("expected 0 backlinks after rebuild, got %d: %v", len(bl), bl)
	}
}

func TestBuildSelfLink(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "self.md"), []byte("Link to [[self]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("self.md")
	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink (self-reference), got %d", len(backlinks))
	}
	if backlinks[0] != "self.md" {
		t.Errorf("expected self-link from 'self.md', got '%s'", backlinks[0])
	}
}

func TestBuildWithMixedExistingAndDanglingLinks(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "note.md"), []byte("[[real]] and [[fake]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "real.md"), []byte("# Real note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	// Real note gets backlink
	if bl := idx.GetBacklinks("real.md"); len(bl) != 1 {
		t.Errorf("expected 1 backlink to real.md, got %d", len(bl))
	}

	// Outgoing links still show both
	outgoing := idx.GetOutgoingLinks("note.md")
	sort.Strings(outgoing)
	if len(outgoing) != 2 {
		t.Fatalf("expected 2 outgoing links, got %d", len(outgoing))
	}
	if outgoing[0] != "fake" || outgoing[1] != "real" {
		t.Errorf("expected [fake, real], got %v", outgoing)
	}
}

func TestBuildLargeVault(t *testing.T) {
	dir := t.TempDir()
	// Create a hub note linked to by many spokes
	_ = os.WriteFile(filepath.Join(dir, "hub.md"), []byte("# Central Hub"), 0644)
	for i := 0; i < 50; i++ {
		name := filepath.Join(dir, "spoke"+string(rune('A'+i%26))+string(rune('0'+i/26))+".md")
		_ = os.WriteFile(name, []byte("Ref [[hub]]"), 0644)
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("hub.md")
	if len(backlinks) != 50 {
		t.Errorf("expected 50 backlinks to hub, got %d", len(backlinks))
	}
}

func TestBuildWithDisplayTextLinks(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "source.md"), []byte("See [[target|display text]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	// The parser strips display text, so the link is just "target"
	backlinks := idx.GetBacklinks("target.md")
	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink (display text link), got %d", len(backlinks))
	}
	if backlinks[0] != "source.md" {
		t.Errorf("expected backlink from source.md, got '%s'", backlinks[0])
	}
}

func TestBuildEmptyVault(t *testing.T) {
	dir := t.TempDir()

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	if len(idx.Backlinks) != 0 {
		t.Errorf("expected no backlinks in empty vault, got %d", len(idx.Backlinks))
	}
}

func TestBuildNoteWithMultipleLinksToSameTarget(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "repeater.md"), []byte("[[X]] and again [[X]]"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "X.md"), []byte("# X"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	// ParseWikiLinks deduplicates, so only 1 link from repeater to X
	backlinks := idx.GetBacklinks("X.md")
	if len(backlinks) != 1 {
		t.Errorf("expected 1 backlink (deduped), got %d: %v", len(backlinks), backlinks)
	}
}

func TestResolveLinkExactMatch(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Note"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)

	// resolveLink is unexported, but we can test via Build behavior
	// Link with .md extension should match directly
	_ = os.WriteFile(filepath.Join(dir, "linker.md"), []byte("[[note.md]]"), 0644)
	_ = v.Scan()
	idx.Build()

	backlinks := idx.GetBacklinks("note.md")
	if len(backlinks) != 1 {
		t.Errorf("expected 1 backlink (exact .md match), got %d", len(backlinks))
	}
}

func TestResolveLinkWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "linker.md"), []byte("[[target]]"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	backlinks := idx.GetBacklinks("target.md")
	if len(backlinks) != 1 {
		t.Errorf("expected 1 backlink (auto .md extension), got %d", len(backlinks))
	}
}
