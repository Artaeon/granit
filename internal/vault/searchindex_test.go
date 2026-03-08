package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to create a vault with notes from a map of relPath -> content.
func testVaultWithNotes(t *testing.T, notes map[string]string) *Vault {
	t.Helper()
	dir := t.TempDir()
	for relPath, content := range notes {
		absPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}
	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	return v
}

func TestNewSearchIndex(t *testing.T) {
	si := NewSearchIndex()
	if si == nil {
		t.Fatal("NewSearchIndex returned nil")
	}
	if si.IsReady() {
		t.Error("new search index should not be ready before Build")
	}
}

func TestBuildFromScratch(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"hello.md": "Hello world, this is a test note.",
		"foo.md":   "Foo bar baz quux.\nAnother line here.",
	})

	si := v.SearchIndex
	if si == nil {
		t.Fatal("SearchIndex should be set after Scan")
	}
	if !si.IsReady() {
		t.Error("SearchIndex should be ready after Build via Scan")
	}
	if si.totalDocs != 2 {
		t.Errorf("expected 2 documents, got %d", si.totalDocs)
	}
}

func TestSearchSingleTerm(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"alpha.md": "The quick brown fox jumps over the lazy dog.",
		"beta.md":  "A slow red cat sleeps under the warm blanket.",
		"gamma.md": "The fox and the cat are friends.",
	})

	si := v.SearchIndex

	results := si.Search("fox")
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'fox'")
	}

	// Should find fox in alpha.md and gamma.md
	paths := make(map[string]bool)
	for _, r := range results {
		paths[r.Path] = true
	}
	if !paths["alpha.md"] {
		t.Error("expected to find 'fox' in alpha.md")
	}
	if !paths["gamma.md"] {
		t.Error("expected to find 'fox' in gamma.md")
	}
	if paths["beta.md"] {
		t.Error("should not find 'fox' in beta.md")
	}
}

func TestSearchMultiWordQuery(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"a.md": "Machine learning is a subset of artificial intelligence.",
		"b.md": "Deep learning uses neural networks.",
		"c.md": "Go is a programming language.",
	})

	si := v.SearchIndex

	results := si.Search("machine learning")
	if len(results) == 0 {
		t.Fatal("expected results for 'machine learning'")
	}

	// The first result should be from a.md since it contains both words
	if results[0].Path != "a.md" {
		t.Errorf("expected first result from a.md, got %s", results[0].Path)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "Hello World\nhello world\nHELLO WORLD",
	})

	si := v.SearchIndex

	results := si.Search("hello")
	if len(results) == 0 {
		t.Fatal("expected results for case-insensitive 'hello'")
	}

	// Should find matches on all three lines
	if len(results) < 3 {
		t.Errorf("expected at least 3 matches, got %d", len(results))
	}
}

func TestSearchPhraseMatch(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"phrase.md":  "The quick brown fox jumps over the lazy dog.",
		"spread.md":  "The word quick appeared early.\nMuch later we talk about brown things in a completely different context with lots of extra words to dilute TF.",
		"neither.md": "Nothing relevant at all in this document.",
	})

	si := v.SearchIndex

	results := si.Search("quick brown")
	if len(results) == 0 {
		t.Fatal("expected results for 'quick brown'")
	}

	// phrase.md should rank higher because it has the exact phrase on one line
	// giving it a phrase match bonus
	if results[0].Path != "phrase.md" {
		t.Errorf("expected phrase.md to rank first for phrase 'quick brown', got %s", results[0].Path)
	}
}

func TestSearchReturnsLineAndColumn(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "line zero\nline one with target word\nline two",
	})

	si := v.SearchIndex

	results := si.Search("target")
	if len(results) == 0 {
		t.Fatal("expected results for 'target'")
	}

	r := results[0]
	if r.Line != 1 {
		t.Errorf("expected line 1, got %d", r.Line)
	}
	if r.MatchLine != "line one with target word" {
		t.Errorf("unexpected match line: %q", r.MatchLine)
	}
}

func TestSearchRegex(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"code.md": "func main() {\n\tfmt.Println(\"hello\")\n}\n",
		"note.md": "This is a regular note.",
	})

	si := v.SearchIndex

	results := si.SearchRegex(`func\s+\w+`)
	if len(results) == 0 {
		t.Fatal("expected regex results for 'func\\s+\\w+'")
	}

	if results[0].Path != "code.md" {
		t.Errorf("expected code.md, got %s", results[0].Path)
	}
}

func TestSearchRegexInvalidPattern(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "Some content.",
	})

	si := v.SearchIndex

	results := si.SearchRegex("[invalid")
	if results != nil {
		t.Error("expected nil results for invalid regex")
	}
}

func TestUpdateDocument(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "Original content with xylophone.",
	})

	si := v.SearchIndex

	// Verify original content is indexed
	results := si.Search("xylophone")
	if len(results) == 0 {
		t.Fatal("expected to find original content")
	}

	// Update the document with completely different content
	si.Update("note.md", "Updated content with zeppelin.")

	// Old unique term should no longer be found
	results = si.Search("xylophone")
	if len(results) != 0 {
		t.Error("should not find old unique term after update")
	}

	// New unique term should be found
	results = si.Search("zeppelin")
	if len(results) == 0 {
		t.Fatal("expected to find new unique term after update")
	}

	// Shared word "content" should still be found (present in both old and new)
	results = si.Search("content")
	if len(results) == 0 {
		t.Fatal("expected to find shared word 'content' after update")
	}
}

func TestRemoveDocument(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"keep.md":   "This note stays.",
		"remove.md": "This note gets removed with special_remove_term.",
	})

	si := v.SearchIndex

	// Verify both are indexed
	results := si.Search("special_remove_term")
	if len(results) == 0 {
		t.Fatal("expected to find remove.md content before removal")
	}

	// Remove the document
	si.Remove("remove.md")

	// Should no longer find it
	results = si.Search("special_remove_term")
	if len(results) != 0 {
		t.Error("should not find content after document removal")
	}

	// The other note should still be findable
	results = si.Search("stays")
	if len(results) == 0 {
		t.Fatal("should still find keep.md after removing remove.md")
	}

	// totalDocs should be updated
	si.mu.RLock()
	if si.totalDocs != 1 {
		t.Errorf("expected 1 total doc after removal, got %d", si.totalDocs)
	}
	si.mu.RUnlock()
}

func TestScoringRelevance(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"focused.md":  "golang golang golang golang golang", // high term frequency
		"mentions.md": "I once used golang in a project with many other languages and tools.",
		"none.md":     "Python is a great language for scripting.",
	})

	si := v.SearchIndex

	results := si.Search("golang")
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// focused.md should score higher due to higher term frequency
	if results[0].Path != "focused.md" {
		t.Errorf("expected focused.md to rank first, got %s", results[0].Path)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("expected first result to have higher score: %f vs %f", results[0].Score, results[1].Score)
	}
}

func TestEmptyVault(t *testing.T) {
	dir := t.TempDir()
	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	si := v.SearchIndex
	if si == nil {
		t.Fatal("SearchIndex should not be nil even for empty vault")
	}

	results := si.Search("anything")
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty vault, got %d", len(results))
	}

	results = si.SearchRegex(".*")
	if len(results) != 0 {
		t.Errorf("expected 0 regex results for empty vault, got %d", len(results))
	}
}

func TestEmptyQuery(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "Some content.",
	})

	si := v.SearchIndex

	results := si.Search("")
	if results != nil {
		t.Error("expected nil results for empty query")
	}
}

func TestSpecialCharacters(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"special.md": "C++ is a language.\nC# is another.\nUse @decorators in Python.\nEmail: user@example.com",
	})

	si := v.SearchIndex

	// Should find "decorators" despite @ prefix
	results := si.Search("decorators")
	if len(results) == 0 {
		t.Error("expected to find 'decorators'")
	}

	// Search for "example" should work despite being in an email
	results = si.Search("example")
	if len(results) == 0 {
		t.Error("expected to find 'example' from email address")
	}
}

func TestVeryLongDocument(t *testing.T) {
	// Create a document with many lines
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString(fmt.Sprintf("Line %d: This is a repeated line of content.\n", i))
	}
	sb.WriteString("Line 10000: This line has a unique_needle_word.\n")

	v := testVaultWithNotes(t, map[string]string{
		"long.md": sb.String(),
	})

	si := v.SearchIndex

	results := si.Search("unique_needle_word")
	if len(results) == 0 {
		t.Fatal("expected to find unique_needle_word in long document")
	}

	if results[0].Line != 10000 {
		t.Errorf("expected line 10000, got %d", results[0].Line)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"foo-bar_baz", []string{"foo", "bar", "baz"}},
		{"  spaces  everywhere  ", []string{"spaces", "everywhere"}},
		{"123 numbers 456", []string{"123", "numbers", "456"}},
		{"", nil},
		{"---", nil},
		{"CamelCase", []string{"camelcase"}},
		{"hello, world!", []string{"hello", "world"}},
		{"file.md", []string{"file", "md"}},
		{"don't", []string{"don", "t"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for i, tok := range got {
				if tok != tt.expected[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, tok, tt.expected[i])
				}
			}
		})
	}
}

func TestSearchIndexBuiltDuringScan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("Searchable content here"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}

	// Before scan, SearchIndex should be nil
	if v.SearchIndex != nil {
		t.Error("SearchIndex should be nil before Scan")
	}

	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// After scan, SearchIndex should be set and ready
	if v.SearchIndex == nil {
		t.Fatal("SearchIndex should be set after Scan")
	}
	if !v.SearchIndex.IsReady() {
		t.Error("SearchIndex should be ready after Scan")
	}

	// Should be able to search
	results := v.SearchIndex.Search("searchable")
	if len(results) == 0 {
		t.Error("expected to find 'searchable' after Scan")
	}
}

func TestSearchIndexRebuildOnRescan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("First version content"), 0644)

	v, err := NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	v.Scan()

	// Write new content
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("Second version different"), 0644)
	v.Scan()

	// Old content should not be found
	results := v.SearchIndex.Search("first")
	if len(results) != 0 {
		t.Error("old content should not be found after rescan")
	}

	// New content should be found
	results = v.SearchIndex.Search("second")
	if len(results) == 0 {
		t.Error("new content should be found after rescan")
	}
}

func TestSearchMultipleFilesRanking(t *testing.T) {
	// Create notes where one is more relevant than others
	v := testVaultWithNotes(t, map[string]string{
		"expert.md":   "Kubernetes is a container orchestration platform.\nKubernetes manages containers.\nKubernetes scales workloads.",
		"mentions.md": "I deployed my app using Kubernetes last week.",
		"unrelated.md": "Docker containers are useful for development.",
	})

	si := v.SearchIndex

	results := si.Search("kubernetes")
	if len(results) == 0 {
		t.Fatal("expected results for 'kubernetes'")
	}

	// expert.md should come first as it has higher term frequency
	foundExpertFirst := false
	for i, r := range results {
		if r.Path == "expert.md" {
			if i == 0 {
				foundExpertFirst = true
			}
			break
		}
	}
	if !foundExpertFirst {
		t.Error("expected expert.md to rank first due to higher term frequency")
	}
}

func TestSearchWithSubdirectories(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"root.md":              "Root level note.",
		"sub/nested.md":        "Nested note with special_nested_term.",
		"sub/deep/deeper.md":   "Deeply nested note.",
	})

	si := v.SearchIndex

	results := si.Search("special_nested_term")
	if len(results) == 0 {
		t.Fatal("expected to find term in subdirectory note")
	}

	if results[0].Path != filepath.Join("sub", "nested.md") {
		t.Errorf("expected sub/nested.md, got %s", results[0].Path)
	}
}

func TestSearchRegexColumnPosition(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "prefix TARGET suffix",
	})

	si := v.SearchIndex

	results := si.SearchRegex("TARGET")
	if len(results) == 0 {
		t.Fatal("expected regex result")
	}

	// Column should point to where TARGET starts
	if results[0].Column != 7 {
		t.Errorf("expected column 7, got %d", results[0].Column)
	}
}

func TestUpdateAddsNewDocument(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"existing.md": "Existing content.",
	})

	si := v.SearchIndex

	// Add a new document via Update
	si.Update("new.md", "Brand new document with unique_new_term.")

	results := si.Search("unique_new_term")
	if len(results) == 0 {
		t.Fatal("expected to find content from newly added document")
	}
	if results[0].Path != "new.md" {
		t.Errorf("expected new.md, got %s", results[0].Path)
	}
}

func TestRemoveNonExistentDocument(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "Some content.",
	})

	si := v.SearchIndex

	// Should not panic when removing a document that doesn't exist
	si.Remove("nonexistent.md")

	// Existing content should still be searchable
	results := si.Search("content")
	if len(results) == 0 {
		t.Error("existing content should still be findable after removing nonexistent doc")
	}
}

func TestSearchResultsCapped(t *testing.T) {
	// Create many files that all match
	notes := make(map[string]string)
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("note_%03d.md", i)
		notes[name] = "common_search_term appears here.\nAnd again common_search_term on another line."
	}

	v := testVaultWithNotes(t, notes)
	si := v.SearchIndex

	results := si.Search("common_search_term")
	if len(results) > 200 {
		t.Errorf("results should be capped at 200, got %d", len(results))
	}
}

func TestSearchRegexResultsCapped(t *testing.T) {
	notes := make(map[string]string)
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("note_%03d.md", i)
		notes[name] = "regex_match_target is here."
	}

	v := testVaultWithNotes(t, notes)
	si := v.SearchIndex

	results := si.SearchRegex("regex_match_target")
	if len(results) > 200 {
		t.Errorf("regex results should be capped at 200, got %d", len(results))
	}
}

func TestSearchUnicodeContent(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"unicode.md": "Gesamtkunstwerk is a German concept.\nSchrodinger's cat.\nCafe latte.",
	})

	si := v.SearchIndex

	results := si.Search("gesamtkunstwerk")
	if len(results) == 0 {
		t.Error("expected to find unicode term")
	}
}

func TestSearchSingleCharQuery(t *testing.T) {
	v := testVaultWithNotes(t, map[string]string{
		"note.md": "A quick note about B and C topics.",
	})

	si := v.SearchIndex

	results := si.Search("a")
	if len(results) == 0 {
		t.Error("expected to find single character 'a'")
	}
}

func TestIsReadyThreadSafe(t *testing.T) {
	si := NewSearchIndex()

	// Run concurrent reads and a build
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = si.IsReady()
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
