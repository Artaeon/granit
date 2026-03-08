package vault

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Stress: 5000 notes
// ---------------------------------------------------------------------------

func TestStress_5000Notes(t *testing.T) {
	dir := t.TempDir()
	rng := rand.New(rand.NewSource(99))

	const noteCount = 5000
	for i := 0; i < noteCount; i++ {
		subdir := ""
		if i%4 == 1 {
			subdir = "projects"
		} else if i%4 == 2 {
			subdir = "journal"
		} else if i%4 == 3 {
			subdir = "research"
		}
		if subdir != "" {
			os.MkdirAll(filepath.Join(dir, subdir), 0755)
		}
		// Use filename-matching wikilinks: link target matches the note filename
		// (without .md) so the index can resolve them by basename.
		linkTarget := fmt.Sprintf("note-%04d", rng.Intn(noteCount))
		title := fmt.Sprintf("Note %d", i)
		content := fmt.Sprintf("---\ntitle: %s\ntags: [stress, test]\n---\n\n# %s\n\nBody of note %d. See [[%s]] for more.\n",
			title, title, i, linkTarget)
		filename := fmt.Sprintf("note-%04d.md", i)
		path := filepath.Join(dir, subdir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create note %d: %v", i, err)
		}
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if v.NoteCount() != noteCount {
		t.Errorf("expected %d notes, got %d", noteCount, v.NoteCount())
	}

	paths := v.SortedPaths()
	if len(paths) != noteCount {
		t.Errorf("SortedPaths returned %d entries, expected %d", len(paths), noteCount)
	}

	// Index build should succeed
	idx := NewIndex(v)
	idx.Build()

	// Spot check a few backlinks are populated
	backlinkCount := 0
	for _, bl := range idx.Backlinks {
		backlinkCount += len(bl)
	}
	if backlinkCount == 0 {
		t.Error("expected at least some backlinks across 5000 notes")
	}
}

// ---------------------------------------------------------------------------
// Stress: Note with 50,000 lines
// ---------------------------------------------------------------------------

func TestStress_LargeNote_50000Lines(t *testing.T) {
	dir := t.TempDir()

	var sb strings.Builder
	sb.WriteString("---\ntitle: Giant Note\n---\n\n# Giant Note\n\n")
	for i := 0; i < 50000; i++ {
		sb.WriteString(fmt.Sprintf("Line %d: This is filler content to simulate a very long note.\n", i))
	}
	content := sb.String()

	if err := os.WriteFile(filepath.Join(dir, "giant.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write giant note: %v", err)
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	note := v.GetNote("giant.md")
	if note == nil {
		t.Fatal("expected to find giant.md")
	}
	lines := strings.Split(note.Content, "\n")
	// 50000 body lines + frontmatter + heading + blanks
	if len(lines) < 50000 {
		t.Errorf("expected at least 50000 lines, got %d", len(lines))
	}
}

// ---------------------------------------------------------------------------
// Stress: Extremely long lines (10,000 chars)
// ---------------------------------------------------------------------------

func TestStress_LongLines(t *testing.T) {
	dir := t.TempDir()

	longLine := strings.Repeat("abcdefghij", 1000) // 10,000 chars
	content := "# Long Lines\n\n" + longLine + "\n\n" + longLine + "\n"

	if err := os.WriteFile(filepath.Join(dir, "longlines.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	note := v.GetNote("longlines.md")
	if note == nil {
		t.Fatal("expected to find longlines.md")
	}
	// Content should preserve the long lines
	lines := strings.Split(note.Content, "\n")
	foundLong := false
	for _, line := range lines {
		if len(line) >= 10000 {
			foundLong = true
			break
		}
	}
	if !foundLong {
		t.Error("expected at least one line with 10,000+ characters")
	}
}

// ---------------------------------------------------------------------------
// Stress: Deeply nested folders (20 levels)
// ---------------------------------------------------------------------------

func TestStress_DeeplyNestedFolders(t *testing.T) {
	dir := t.TempDir()

	// Build a path 20 levels deep
	nested := dir
	for i := 0; i < 20; i++ {
		nested = filepath.Join(nested, fmt.Sprintf("level%d", i))
	}
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Put a note at the deepest level
	deepNote := filepath.Join(nested, "deep.md")
	if err := os.WriteFile(deepNote, []byte("# Deep Note\n\nI am 20 levels deep."), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Also put a note at the root
	if err := os.WriteFile(filepath.Join(dir, "root.md"), []byte("# Root\n\nLink to [[deep]]."), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if v.NoteCount() != 2 {
		t.Errorf("expected 2 notes, got %d", v.NoteCount())
	}

	// Verify the deep note was found
	foundDeep := false
	for _, p := range v.SortedPaths() {
		if strings.HasSuffix(p, "deep.md") {
			foundDeep = true
			break
		}
	}
	if !foundDeep {
		t.Error("expected to find deep.md in deeply nested folder")
	}
}

// ---------------------------------------------------------------------------
// Stress: Note with 500+ wikilinks
// ---------------------------------------------------------------------------

func TestStress_ManyWikilinks(t *testing.T) {
	dir := t.TempDir()

	var sb strings.Builder
	sb.WriteString("# Link Hub\n\n")
	for i := 0; i < 500; i++ {
		sb.WriteString(fmt.Sprintf("- [[Note %d]]\n", i))
	}
	content := sb.String()

	if err := os.WriteFile(filepath.Join(dir, "hub.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	note := v.GetNote("hub.md")
	if note == nil {
		t.Fatal("expected hub.md")
	}
	if len(note.Links) < 500 {
		t.Errorf("expected at least 500 links, got %d", len(note.Links))
	}
}

// ---------------------------------------------------------------------------
// Stress: Circular wikilink chains (A -> B -> C -> A)
// ---------------------------------------------------------------------------

func TestStress_CircularWikilinks(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "A.md"), []byte("# A\n\nLink to [[B]]."), 0644)
	os.WriteFile(filepath.Join(dir, "B.md"), []byte("# B\n\nLink to [[C]]."), 0644)
	os.WriteFile(filepath.Join(dir, "C.md"), []byte("# C\n\nLink to [[A]]."), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	idx := NewIndex(v)
	idx.Build() // Must not hang or crash on circular links

	if v.NoteCount() != 3 {
		t.Errorf("expected 3 notes, got %d", v.NoteCount())
	}

	// Each note should have exactly 1 backlink
	aBack := idx.GetBacklinks("A.md")
	bBack := idx.GetBacklinks("B.md")
	cBack := idx.GetBacklinks("C.md")

	if len(aBack) != 1 {
		t.Errorf("A.md expected 1 backlink, got %d", len(aBack))
	}
	if len(bBack) != 1 {
		t.Errorf("B.md expected 1 backlink, got %d", len(bBack))
	}
	if len(cBack) != 1 {
		t.Errorf("C.md expected 1 backlink, got %d", len(cBack))
	}
}

// ---------------------------------------------------------------------------
// Edge: Malformed / corrupted frontmatter
// ---------------------------------------------------------------------------

func TestEdge_MalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{"missing closing dashes", "---\ntitle: Broken\nNo closing dashes here\n\n# Body\n"},
		{"unclosed quotes", "---\ntitle: \"Unclosed\ntags: [a, b]\n---\n\n# Body\n"},
		{"binary junk in frontmatter", "---\ntitle: \x00\x01\x02\x03\n---\n\n# Body\n"},
		{"empty frontmatter", "---\n---\n\n# Empty FM\n"},
		{"only dashes", "---\n---"},
		{"triple dashes inside body", "Some text\n---\ntitle: Not FM\n---\n"},
		{"frontmatter with no key-value pairs", "---\njust a line\n---\n\n# Body\n"},
	}

	for _, tc := range tests {
		path := filepath.Join(dir, tc.name+".md")
		if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
			t.Fatalf("write %s: %v", tc.name, err)
		}
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}

	// Scan must not panic or error on malformed frontmatter
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed on malformed frontmatter: %v", err)
	}

	if v.NoteCount() != len(tests) {
		t.Errorf("expected %d notes, got %d", len(tests), v.NoteCount())
	}

	// ParseFrontmatter should return a map (possibly empty) for each
	for _, tc := range tests {
		fm := ParseFrontmatter(tc.content)
		if fm == nil {
			t.Errorf("%s: ParseFrontmatter returned nil, expected empty map", tc.name)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge: Empty vault (0 notes)
// ---------------------------------------------------------------------------

func TestEdge_EmptyVault(t *testing.T) {
	dir := t.TempDir()

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if v.NoteCount() != 0 {
		t.Errorf("expected 0 notes, got %d", v.NoteCount())
	}

	paths := v.SortedPaths()
	if len(paths) != 0 {
		t.Errorf("expected empty sorted paths, got %d", len(paths))
	}

	// Index should build cleanly on empty vault
	idx := NewIndex(v)
	idx.Build()
	if len(idx.Backlinks) != 0 {
		t.Errorf("expected no backlinks, got %d", len(idx.Backlinks))
	}
}

// ---------------------------------------------------------------------------
// Edge: Vault with only non-markdown files
// ---------------------------------------------------------------------------

func TestEdge_NonMarkdownOnly(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "image.png"), []byte{0x89, 0x50, 0x4E, 0x47}, 0644)
	os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"key": "value"}`), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("Just a text file"), 0644)
	os.WriteFile(filepath.Join(dir, "script.py"), []byte("print('hello')"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if v.NoteCount() != 0 {
		t.Errorf("expected 0 markdown notes, got %d", v.NoteCount())
	}
}

// ---------------------------------------------------------------------------
// Edge: Filenames with special characters
// ---------------------------------------------------------------------------

func TestEdge_SpecialCharFilenames(t *testing.T) {
	dir := t.TempDir()

	specialNames := []struct {
		filename string
		content  string
	}{
		{"note with spaces.md", "# Spaces\n\nNote with spaces in name."},
		{"note-with-dashes.md", "# Dashes\n\nDashes are common."},
		{"note_with_underscores.md", "# Underscores\n\nAlso common."},
		{"note.extra.dots.md", "# Dots\n\nMultiple dots before .md."},
		{"UPPERCASE.md", "# Uppercase\n\nAll caps filename."},
		{"MixedCase Note.md", "# Mixed\n\nMixed case with space."},
		{"\u00fcml\u00e4uts.md", "# Umlauts\n\nUnicode filename."},
		{"\u4e2d\u6587\u7b14\u8bb0.md", "# Chinese\n\nChinese characters in filename."},
		{"note (1).md", "# Parens\n\nParentheses in name."},
		{"note [bracketed].md", "# Brackets\n\nSquare brackets in name."},
	}

	for _, sn := range specialNames {
		path := filepath.Join(dir, sn.filename)
		if err := os.WriteFile(path, []byte(sn.content), 0644); err != nil {
			t.Logf("skipping %q (OS may not support): %v", sn.filename, err)
			continue
		}
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// At least the ASCII-safe names should be found
	if v.NoteCount() < 6 {
		t.Errorf("expected at least 6 notes with special filenames, got %d", v.NoteCount())
	}

	// Each found note should have a non-empty title
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		if note == nil {
			t.Errorf("GetNote(%q) returned nil", p)
			continue
		}
		if note.Title == "" {
			t.Errorf("note at %q has empty title", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Stress: ScanFast + lazy loading with 5000 notes
// ---------------------------------------------------------------------------

func TestStress_ScanFast_5000Notes(t *testing.T) {
	dir := t.TempDir()
	rng := rand.New(rand.NewSource(77))

	const noteCount = 5000
	for i := 0; i < noteCount; i++ {
		title := fmt.Sprintf("Fast Note %d", i)
		content := fmt.Sprintf("---\ntitle: %s\n---\n\n# %s\n\nSee [[Fast Note %d]].\n",
			title, title, rng.Intn(noteCount))
		filename := fmt.Sprintf("fastnote-%04d.md", i)
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.ScanFast(); err != nil {
		t.Fatalf("ScanFast: %v", err)
	}

	if v.NoteCount() != noteCount {
		t.Errorf("expected %d notes, got %d", noteCount, v.NoteCount())
	}

	// Lazy load a note and verify content is populated
	paths := v.SortedPaths()
	note := v.GetNote(paths[0])
	if note == nil {
		t.Fatal("GetNote returned nil for first path")
	}
	if note.Content == "" {
		t.Error("expected content to be loaded lazily")
	}
}

// ---------------------------------------------------------------------------
// Edge: Self-referencing note (links to itself)
// ---------------------------------------------------------------------------

func TestEdge_SelfReference(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "self.md"), []byte("# Self\n\nI link to [[self]]."), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	idx := NewIndex(v)
	idx.Build()

	note := v.GetNote("self.md")
	if note == nil {
		t.Fatal("expected self.md")
	}
	if len(note.Links) != 1 || note.Links[0] != "self" {
		t.Errorf("expected link to 'self', got %v", note.Links)
	}

	// Self-reference should produce a backlink
	backlinks := idx.GetBacklinks("self.md")
	if len(backlinks) != 1 {
		t.Errorf("expected 1 self-backlink, got %d", len(backlinks))
	}
}

// ---------------------------------------------------------------------------
// Edge: Empty file (0 bytes)
// ---------------------------------------------------------------------------

func TestEdge_EmptyFile(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "empty.md"), []byte(""), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if v.NoteCount() != 1 {
		t.Errorf("expected 1 note, got %d", v.NoteCount())
	}

	note := v.GetNote("empty.md")
	if note == nil {
		t.Fatal("expected empty.md")
	}
	if note.Content != "" {
		t.Errorf("expected empty content, got %q", note.Content)
	}
	if len(note.Links) != 0 {
		t.Errorf("expected 0 links, got %d", len(note.Links))
	}
}

// ---------------------------------------------------------------------------
// Edge: Wikilinks with aliases and heading anchors
// ---------------------------------------------------------------------------

func TestEdge_WikilinkVariants(t *testing.T) {
	content := "Link to [[target|display name]] and [[other#heading]] and [[simple]]."
	links := ParseWikiLinks(content)

	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d: %v", len(links), links)
	}

	// ParseWikiLinks should extract the target (before |)
	found := map[string]bool{}
	for _, l := range links {
		found[l] = true
	}
	if !found["target"] {
		t.Error("expected 'target' in links")
	}
	if !found["other#heading"] {
		t.Error("expected 'other#heading' in links")
	}
	if !found["simple"] {
		t.Error("expected 'simple' in links")
	}
}

// ---------------------------------------------------------------------------
// Edge: Duplicate wikilinks in the same note
// ---------------------------------------------------------------------------

func TestEdge_DuplicateWikilinks(t *testing.T) {
	content := "See [[target]] and [[target]] and [[target]] again."
	links := ParseWikiLinks(content)

	// ParseWikiLinks deduplicates
	if len(links) != 1 {
		t.Errorf("expected 1 deduplicated link, got %d: %v", len(links), links)
	}
}
