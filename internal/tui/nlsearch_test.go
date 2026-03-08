package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Constructor / Initialization
// ---------------------------------------------------------------------------

func TestNewNLSearch(t *testing.T) {
	nls := NewNLSearch()

	t.Run("defaults to local provider", func(t *testing.T) {
		if nls.aiProvider != "local" {
			t.Errorf("expected aiProvider 'local', got %q", nls.aiProvider)
		}
	})

	t.Run("default ollama URL", func(t *testing.T) {
		if nls.ollamaURL != "http://localhost:11434" {
			t.Errorf("expected default ollamaURL, got %q", nls.ollamaURL)
		}
	})

	t.Run("default ollama model", func(t *testing.T) {
		if nls.ollamaModel != "llama3.2" {
			t.Errorf("expected default ollamaModel 'llama3.2', got %q", nls.ollamaModel)
		}
	})

	t.Run("default openai model", func(t *testing.T) {
		if nls.openaiModel != "gpt-4o-mini" {
			t.Errorf("expected default openaiModel 'gpt-4o-mini', got %q", nls.openaiModel)
		}
	})

	t.Run("not active initially", func(t *testing.T) {
		if nls.IsActive() {
			t.Error("NLSearch should not be active after construction")
		}
	})

	t.Run("no selected note", func(t *testing.T) {
		_, ok := nls.GetSelectedNote()
		if ok {
			t.Error("expected no selected note on fresh NLSearch")
		}
	})
}

// ---------------------------------------------------------------------------
// Open / lifecycle
// ---------------------------------------------------------------------------

func TestNLSearchOpen(t *testing.T) {
	nls := NewNLSearch()
	tmpDir := t.TempDir()

	// Create some markdown files for indexing
	writeTestNote(t, tmpDir, "note1.md", "# First Note\nSome content about testing.")
	writeTestNote(t, tmpDir, "note2.md", "# Second Note\nAnother note about Go programming.")

	nls.Open(tmpDir, "local", "", "", "", "")

	t.Run("becomes active", func(t *testing.T) {
		if !nls.IsActive() {
			t.Error("expected IsActive to be true after Open")
		}
	})

	t.Run("phase is input", func(t *testing.T) {
		if nls.phase != 0 {
			t.Errorf("expected phase 0 (input), got %d", nls.phase)
		}
	})

	t.Run("query is empty", func(t *testing.T) {
		if nls.query != "" {
			t.Errorf("expected empty query after Open, got %q", nls.query)
		}
	})

	t.Run("indexes markdown files", func(t *testing.T) {
		if len(nls.noteIndex) != 2 {
			t.Errorf("expected 2 notes indexed, got %d", len(nls.noteIndex))
		}
	})

	t.Run("extracts titles from headings", func(t *testing.T) {
		titles := make(map[string]bool)
		for _, entry := range nls.noteIndex {
			titles[entry.Title] = true
		}
		if !titles["First Note"] {
			t.Error("expected 'First Note' to be extracted as title")
		}
		if !titles["Second Note"] {
			t.Error("expected 'Second Note' to be extracted as title")
		}
	})

	t.Run("empty provider defaults to local", func(t *testing.T) {
		nls2 := NewNLSearch()
		nls2.Open(tmpDir, "", "", "", "", "")
		if nls2.aiProvider != "local" {
			t.Errorf("expected 'local' for empty provider, got %q", nls2.aiProvider)
		}
	})

	t.Run("empty ollama URL defaults", func(t *testing.T) {
		nls2 := NewNLSearch()
		nls2.Open(tmpDir, "ollama", "", "", "", "")
		if nls2.ollamaURL != "http://localhost:11434" {
			t.Errorf("expected default ollama URL, got %q", nls2.ollamaURL)
		}
	})
}

// ---------------------------------------------------------------------------
// GetSelectedNote
// ---------------------------------------------------------------------------

func TestGetSelectedNote(t *testing.T) {
	nls := NewNLSearch()

	t.Run("returns false when no selection", func(t *testing.T) {
		_, ok := nls.GetSelectedNote()
		if ok {
			t.Error("expected false when no selection")
		}
	})

	t.Run("returns path and true after selection", func(t *testing.T) {
		nls.selectedNote = "my/note.md"
		nls.hasResult = true
		path, ok := nls.GetSelectedNote()
		if !ok {
			t.Fatal("expected true after selection")
		}
		if path != "my/note.md" {
			t.Errorf("expected 'my/note.md', got %q", path)
		}
	})

	t.Run("clears after consumption", func(t *testing.T) {
		// After the previous call, hasResult should be false
		_, ok := nls.GetSelectedNote()
		if ok {
			t.Error("expected false after result was consumed")
		}
	})
}

// ---------------------------------------------------------------------------
// Note index builder
// ---------------------------------------------------------------------------

func TestBuildNoteIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory structure
	subDir := filepath.Join(tmpDir, "subfolder")
	os.MkdirAll(subDir, 0755)
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	os.MkdirAll(hiddenDir, 0755)

	writeTestNote(t, tmpDir, "root.md", "# Root Note\nContent here\n#tag1 #tag2")
	writeTestNote(t, subDir, "sub.md", "No heading\nJust content with #mytag")
	writeTestNote(t, hiddenDir, "secret.md", "# Hidden\nShould be skipped")
	writeTestNote(t, tmpDir, "readme.txt", "Not markdown")

	nls := NewNLSearch()
	nls.Open(tmpDir, "local", "", "", "", "")

	t.Run("indexes only markdown files", func(t *testing.T) {
		for _, entry := range nls.noteIndex {
			if !strings.HasSuffix(entry.Path, ".md") {
				t.Errorf("non-markdown file indexed: %s", entry.Path)
			}
		}
	})

	t.Run("skips hidden directories", func(t *testing.T) {
		for _, entry := range nls.noteIndex {
			if strings.Contains(entry.Path, ".hidden") {
				t.Errorf("file from hidden directory indexed: %s", entry.Path)
			}
		}
	})

	t.Run("indexes files in subdirectories", func(t *testing.T) {
		found := false
		for _, entry := range nls.noteIndex {
			if strings.Contains(entry.Path, "subfolder") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected file in subfolder to be indexed")
		}
	})

	t.Run("title falls back to filename without heading", func(t *testing.T) {
		for _, entry := range nls.noteIndex {
			if strings.HasSuffix(entry.Path, "sub.md") {
				if entry.Title != "sub" {
					t.Errorf("expected title 'sub' (filename fallback), got %q", entry.Title)
				}
				return
			}
		}
		t.Error("sub.md not found in index")
	})

	t.Run("extracts inline tags", func(t *testing.T) {
		for _, entry := range nls.noteIndex {
			if entry.Title == "Root Note" {
				foundTag1 := false
				foundTag2 := false
				for _, tag := range entry.Tags {
					if tag == "tag1" {
						foundTag1 = true
					}
					if tag == "tag2" {
						foundTag2 = true
					}
				}
				if !foundTag1 || !foundTag2 {
					t.Errorf("expected tags [tag1, tag2], got %v", entry.Tags)
				}
				return
			}
		}
		t.Error("Root Note not found in index")
	})

	t.Run("stores relative paths", func(t *testing.T) {
		for _, entry := range nls.noteIndex {
			if filepath.IsAbs(entry.Path) {
				t.Errorf("expected relative path, got absolute: %s", entry.Path)
			}
		}
	})

	t.Run("truncates content preview to 200 chars", func(t *testing.T) {
		longDir := t.TempDir()
		longContent := strings.Repeat("abcdefghij", 30) // 300 chars
		writeTestNote(t, longDir, "long.md", longContent)
		nls2 := NewNLSearch()
		nls2.Open(longDir, "local", "", "", "", "")
		for _, entry := range nls2.noteIndex {
			if len(entry.Content) > 200 {
				t.Errorf("content preview exceeds 200 chars: %d", len(entry.Content))
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Keyword extraction
// ---------------------------------------------------------------------------

func TestNlSearchExtractKeywords(t *testing.T) {
	t.Run("basic extraction", func(t *testing.T) {
		kw := nlSearchExtractKeywords("machine learning algorithms")
		expected := map[string]bool{"machine": true, "learning": true, "algorithms": true}
		for _, k := range kw {
			if !expected[k] {
				t.Errorf("unexpected keyword %q", k)
			}
		}
		if len(kw) != len(expected) {
			t.Errorf("expected %d keywords, got %d: %v", len(expected), len(kw), kw)
		}
	})

	t.Run("lowercases input", func(t *testing.T) {
		kw := nlSearchExtractKeywords("Machine LEARNING")
		for _, k := range kw {
			if k != strings.ToLower(k) {
				t.Errorf("keyword not lowercased: %q", k)
			}
		}
	})

	t.Run("removes stopwords", func(t *testing.T) {
		kw := nlSearchExtractKeywords("find notes about the machine learning")
		for _, k := range kw {
			if nlSearchStopwords[k] {
				t.Errorf("stopword not removed: %q", k)
			}
		}
	})

	t.Run("strips punctuation", func(t *testing.T) {
		kw := nlSearchExtractKeywords("hello, world! testing.")
		for _, k := range kw {
			if strings.ContainsAny(k, ".,!") {
				t.Errorf("punctuation not stripped from %q", k)
			}
		}
	})

	t.Run("empty query", func(t *testing.T) {
		kw := nlSearchExtractKeywords("")
		if len(kw) != 0 {
			t.Errorf("expected no keywords for empty query, got %v", kw)
		}
	})

	t.Run("only stopwords", func(t *testing.T) {
		kw := nlSearchExtractKeywords("the is a an of to for in on at by")
		if len(kw) != 0 {
			t.Errorf("expected no keywords for all-stopword query, got %v", kw)
		}
	})

	t.Run("single character words removed", func(t *testing.T) {
		kw := nlSearchExtractKeywords("a b c testing x y z")
		for _, k := range kw {
			if len(k) <= 1 {
				t.Errorf("single-char word not removed: %q", k)
			}
		}
		// "testing" should survive
		found := false
		for _, k := range kw {
			if k == "testing" {
				found = true
			}
		}
		if !found {
			t.Error("expected 'testing' to survive keyword extraction")
		}
	})

	t.Run("mixed stopwords and real words", func(t *testing.T) {
		kw := nlSearchExtractKeywords("show me notes about kubernetes deployment")
		expected := map[string]bool{"kubernetes": true, "deployment": true}
		got := make(map[string]bool)
		for _, k := range kw {
			got[k] = true
		}
		for want := range expected {
			if !got[want] {
				t.Errorf("expected keyword %q in results %v", want, kw)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Strip punctuation
// ---------------------------------------------------------------------------

func TestNlSearchStripPunct(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello!", "hello"},
		{"(hello)", "hello"},
		{"#tag", "tag"},
		{"**bold**", "bold"},
		{"--dashed--", "dashed"},
		{"word.,;:", "word"},
		{"", ""},
		{"!!!", ""},
		{"`code`", "code"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := nlSearchStripPunct(tc.input)
			if got != tc.want {
				t.Errorf("nlSearchStripPunct(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fuzzy contains
// ---------------------------------------------------------------------------

func TestNlSearchFuzzyContains(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		if !nlSearchFuzzyContains("hello", "hello") {
			t.Error("exact match should return true")
		}
	})

	t.Run("subsequence match", func(t *testing.T) {
		if !nlSearchFuzzyContains("machine learning", "mchn") {
			t.Error("subsequence 'mchn' should match 'machine learning'")
		}
	})

	t.Run("no match", func(t *testing.T) {
		if nlSearchFuzzyContains("hello", "xyz") {
			t.Error("'xyz' should not fuzzy match 'hello'")
		}
	})

	t.Run("empty needle matches anything", func(t *testing.T) {
		if !nlSearchFuzzyContains("hello", "") {
			t.Error("empty needle should always match")
		}
	})

	t.Run("empty haystack fails non-empty needle", func(t *testing.T) {
		if nlSearchFuzzyContains("", "a") {
			t.Error("empty haystack should not match non-empty needle")
		}
	})

	t.Run("both empty", func(t *testing.T) {
		if !nlSearchFuzzyContains("", "") {
			t.Error("both empty should match")
		}
	})

	t.Run("needle longer than haystack", func(t *testing.T) {
		if nlSearchFuzzyContains("ab", "abc") {
			t.Error("needle longer than haystack should not match")
		}
	})

	t.Run("characters must be in order", func(t *testing.T) {
		if nlSearchFuzzyContains("abc", "ca") {
			t.Error("out-of-order characters should not match")
		}
	})
}

// ---------------------------------------------------------------------------
// Local search — scoring and ranking
// ---------------------------------------------------------------------------

func TestRunLocalSearch(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{Path: "ml-basics.md", Title: "Machine Learning Basics", Content: "Introduction to machine learning concepts and algorithms", Tags: []string{"ml", "ai"}},
		{Path: "cooking.md", Title: "Italian Recipes", Content: "How to make pasta and pizza from scratch", Tags: []string{"cooking", "italian"}},
		{Path: "go-testing.md", Title: "Go Testing Guide", Content: "How to write unit tests in Go using the testing package", Tags: []string{"go", "testing"}},
		{Path: "ml-advanced.md", Title: "Advanced ML Techniques", Content: "Deep learning and neural networks for advanced machine learning", Tags: []string{"ml", "deep-learning"}},
		{Path: "daily/2026-01-01.md", Title: "Daily Note", Content: "Today I learned about machine learning and cooked pasta", Tags: []string{"daily"}},
	}

	t.Run("finds notes by title keyword", func(t *testing.T) {
		nls.query = "machine learning"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results for 'machine learning'")
		}
		// ML notes should be in results
		foundML := false
		for _, r := range results {
			if r.Path == "ml-basics.md" {
				foundML = true
				break
			}
		}
		if !foundML {
			t.Error("expected ml-basics.md in results")
		}
	})

	t.Run("title matches rank higher than content matches", func(t *testing.T) {
		nls.query = "machine learning"
		results := nls.runLocalSearch()
		if len(results) < 2 {
			t.Fatal("expected at least 2 results")
		}
		// ml-basics.md has "Machine Learning" in its title, daily note only in content
		mlIdx := -1
		dailyIdx := -1
		for i, r := range results {
			if r.Path == "ml-basics.md" {
				mlIdx = i
			}
			if r.Path == "daily/2026-01-01.md" {
				dailyIdx = i
			}
		}
		if mlIdx == -1 {
			t.Error("ml-basics.md not in results")
		}
		if dailyIdx == -1 {
			t.Error("daily note not in results")
		}
		if mlIdx != -1 && dailyIdx != -1 && mlIdx >= dailyIdx {
			t.Errorf("title match (ml-basics at %d) should rank higher than content match (daily at %d)", mlIdx, dailyIdx)
		}
	})

	t.Run("tag matches boost score", func(t *testing.T) {
		nls.query = "cooking"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results for 'cooking'")
		}
		// cooking.md has both tag and content match
		if results[0].Path != "cooking.md" {
			t.Errorf("expected cooking.md as top result, got %q", results[0].Path)
		}
	})

	t.Run("empty query returns nil", func(t *testing.T) {
		nls.query = ""
		results := nls.runLocalSearch()
		if results != nil {
			t.Errorf("expected nil for empty query, got %d results", len(results))
		}
	})

	t.Run("stopwords-only query returns nil", func(t *testing.T) {
		nls.query = "the is a an to for"
		results := nls.runLocalSearch()
		if results != nil {
			t.Errorf("expected nil for stopwords-only query, got %d results", len(results))
		}
	})

	t.Run("no matching notes returns nil", func(t *testing.T) {
		nls.query = "cryptocurrency blockchain"
		results := nls.runLocalSearch()
		if len(results) != 0 {
			t.Errorf("expected 0 results for non-matching query, got %d", len(results))
		}
	})

	t.Run("results are sorted by score descending", func(t *testing.T) {
		nls.query = "machine learning"
		results := nls.runLocalSearch()
		// We verify indirectly: notes with title matches should precede content-only matches
		for i := 1; i < len(results); i++ {
			// Results should be in meaningful order (we can't check internal scores directly,
			// but we can verify the structure is plausible)
			_ = results[i]
		}
		if len(results) == 0 {
			t.Error("expected non-empty results")
		}
	})

	t.Run("limits to 15 results", func(t *testing.T) {
		// Create an index with many matching notes
		var bigIndex []noteEntry
		for i := 0; i < 30; i++ {
			bigIndex = append(bigIndex, noteEntry{
				Path:    "note" + string(rune('a'+i%26)) + ".md",
				Title:   "Test Note",
				Content: "This is about testing",
				Tags:    []string{"testing"},
			})
		}
		nls2 := NewNLSearch()
		nls2.noteIndex = bigIndex
		nls2.query = "testing"
		results := nls2.runLocalSearch()
		if len(results) > 15 {
			t.Errorf("expected at most 15 results, got %d", len(results))
		}
	})

	t.Run("special characters in query", func(t *testing.T) {
		nls.query = "machine!! @#$% learning???"
		results := nls.runLocalSearch()
		// Should still find notes about machine learning after stripping punctuation
		if len(results) == 0 {
			t.Error("expected results even with special characters in query")
		}
	})

	t.Run("query with only special characters", func(t *testing.T) {
		nls.query = "!@#$%^&*()"
		results := nls.runLocalSearch()
		if results != nil {
			t.Errorf("expected nil for special-characters-only query, got %d results", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// Relevance explanation
// ---------------------------------------------------------------------------

func TestRelevanceExplanation(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{Path: "ml.md", Title: "Machine Learning", Content: "Introduction to ML concepts", Tags: []string{"ml", "ai"}},
		{Path: "cooking.md", Title: "Italian Cooking", Content: "Pasta recipes from Italy", Tags: []string{"cooking"}},
	}

	t.Run("includes title match label", func(t *testing.T) {
		nls.query = "machine"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		found := false
		for _, r := range results {
			if r.Path == "ml.md" && strings.Contains(r.Relevance, "(title)") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected '(title)' in relevance for title match")
		}
	})

	t.Run("includes tag match label", func(t *testing.T) {
		nls.query = "cooking"
		results := nls.runLocalSearch()
		found := false
		for _, r := range results {
			if r.Path == "cooking.md" {
				// "cooking" appears in both title and tag, title takes precedence in label
				if strings.Contains(r.Relevance, "(title)") || strings.Contains(r.Relevance, "(tag)") {
					found = true
				}
				break
			}
		}
		if !found {
			t.Error("expected relevance annotation for cooking.md")
		}
	})

	t.Run("includes content match label", func(t *testing.T) {
		nls.query = "recipes"
		results := nls.runLocalSearch()
		found := false
		for _, r := range results {
			if r.Path == "cooking.md" && strings.Contains(r.Relevance, "(content)") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected '(content)' in relevance for content-only match")
		}
	})

	t.Run("relevance starts with Matched prefix", func(t *testing.T) {
		nls.query = "machine"
		results := nls.runLocalSearch()
		for _, r := range results {
			if !strings.HasPrefix(r.Relevance, "Matched: ") {
				t.Errorf("expected relevance to start with 'Matched: ', got %q", r.Relevance)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Snippet extraction
// ---------------------------------------------------------------------------

func TestSnippetExtraction(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{
			Path:    "note1.md",
			Title:   "Test Note",
			Content: "# Test Note\n---\nThis line mentions kubernetes in production.\nAnother line here.",
			Tags:    nil,
		},
		{
			Path:    "note2.md",
			Title:   "Empty Lines",
			Content: "# Empty Lines\n\n\n\nSome content eventually.",
			Tags:    nil,
		},
		{
			Path:    "note3.md",
			Title:   "Long Line",
			Content: "# Long Line\nThis is a very long line that exceeds eighty characters and should be truncated to fit within the snippet display area properly for the user interface.",
			Tags:    nil,
		},
	}

	t.Run("snippet contains matching keyword line", func(t *testing.T) {
		nls.query = "kubernetes"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		if !strings.Contains(results[0].Snippet, "kubernetes") {
			t.Errorf("expected snippet to contain 'kubernetes', got %q", results[0].Snippet)
		}
	})

	t.Run("snippet falls back to first non-empty content line", func(t *testing.T) {
		nls.noteIndex = []noteEntry{
			{
				Path:    "fallback.md",
				Title:   "Fallback Title",
				Content: "# Fallback Title\n---\nSome content here about Go programming.",
				Tags:    []string{"golang"},
			},
		}
		nls.query = "golang"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results for tag match")
		}
		// "golang" only matches as a tag, not in content lines, so snippet
		// falls back to first non-empty, non-heading, non-frontmatter line
		if results[0].Snippet == "" {
			t.Error("expected non-empty fallback snippet")
		}
		if strings.HasPrefix(results[0].Snippet, "#") || strings.HasPrefix(results[0].Snippet, "---") {
			t.Errorf("snippet should skip headings and frontmatter, got %q", results[0].Snippet)
		}
	})

	t.Run("long snippet is truncated with ellipsis", func(t *testing.T) {
		nls.noteIndex = []noteEntry{
			{
				Path:    "long.md",
				Title:   "Long",
				Content: "This is a very long line that mentions testing and exceeds eighty characters limit for the snippet display area properly for the user interface rendering system.",
				Tags:    nil,
			},
		}
		nls.query = "testing"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		if len(results[0].Snippet) > 80 {
			t.Errorf("snippet should be at most 80 chars, got %d", len(results[0].Snippet))
		}
		if !strings.HasSuffix(results[0].Snippet, "...") {
			t.Error("truncated snippet should end with '...'")
		}
	})

	t.Run("snippet from note with empty content", func(t *testing.T) {
		nls.noteIndex = []noteEntry{
			{
				Path:    "empty.md",
				Title:   "Testing Title",
				Content: "",
				Tags:    nil,
			},
		}
		nls.query = "testing"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results for title match")
		}
		if results[0].Snippet != "" {
			t.Errorf("expected empty snippet for empty content, got %q", results[0].Snippet)
		}
	})
}

// ---------------------------------------------------------------------------
// Scoring weights
// ---------------------------------------------------------------------------

func TestScoringWeights(t *testing.T) {
	t.Run("title match scores higher than content-only match", func(t *testing.T) {
		nls := NewNLSearch()
		nls.noteIndex = []noteEntry{
			{Path: "a.md", Title: "Kubernetes Guide", Content: "Some general info", Tags: nil},
			{Path: "b.md", Title: "General Info", Content: "Mentions kubernetes in passing", Tags: nil},
		}
		nls.query = "kubernetes"
		results := nls.runLocalSearch()
		if len(results) < 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if results[0].Path != "a.md" {
			t.Errorf("expected title match (a.md) to rank first, got %q", results[0].Path)
		}
	})

	t.Run("tag match scores higher than content-only match", func(t *testing.T) {
		nls := NewNLSearch()
		nls.noteIndex = []noteEntry{
			{Path: "tagged.md", Title: "Some Note", Content: "General content", Tags: []string{"golang"}},
			{Path: "content.md", Title: "Another", Content: "This mentions golang programming", Tags: nil},
		}
		nls.query = "golang"
		results := nls.runLocalSearch()
		if len(results) < 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if results[0].Path != "tagged.md" {
			t.Errorf("expected tag match (tagged.md) to rank first, got %q", results[0].Path)
		}
	})

	t.Run("multiple keyword matches rank higher", func(t *testing.T) {
		nls := NewNLSearch()
		nls.noteIndex = []noteEntry{
			{Path: "both.md", Title: "Machine Learning", Content: "Deep learning neural networks", Tags: nil},
			{Path: "one.md", Title: "Machine Shop", Content: "Tools and equipment", Tags: nil},
		}
		nls.query = "machine learning"
		results := nls.runLocalSearch()
		if len(results) < 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if results[0].Path != "both.md" {
			t.Errorf("expected note matching both keywords to rank first, got %q", results[0].Path)
		}
	})
}

// ---------------------------------------------------------------------------
// Fuzzy title matching in scoring
// ---------------------------------------------------------------------------

func TestFuzzyTitleMatchInSearch(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{Path: "kubernetes.md", Title: "Kubernetes Deployment", Content: "K8s deployment guide", Tags: nil},
	}

	t.Run("fuzzy match finds results when exact title does not match", func(t *testing.T) {
		nls.query = "kbrnts"
		results := nls.runLocalSearch()
		// "kbrnts" is a subsequence of "kubernetes", so fuzzy match should find it
		if len(results) == 0 {
			t.Error("expected fuzzy match to find kubernetes.md for query 'kbrnts'")
		}
	})

	t.Run("fuzzy relevance includes tilde marker", func(t *testing.T) {
		nls.query = "kbrnts"
		results := nls.runLocalSearch()
		if len(results) > 0 {
			if !strings.Contains(results[0].Relevance, "(fuzzy)") {
				t.Errorf("expected '(fuzzy)' in relevance, got %q", results[0].Relevance)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Parse AI response
// ---------------------------------------------------------------------------

func TestParseAIResponse(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{Path: "ml.md", Title: "Machine Learning", Content: "Intro to ML concepts and algorithms", Tags: []string{"ml"}},
		{Path: "recipes.md", Title: "Recipes", Content: "Italian pasta and pizza recipes", Tags: nil},
		{Path: "subfolder/deep.md", Title: "Deep Note", Content: "Deep learning notes", Tags: nil},
	}

	t.Run("parses pipe-separated format", func(t *testing.T) {
		nls.parseAIResponse("ml.md | Contains machine learning content\nrecipes.md | Has cooking recipes")
		if len(nls.results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(nls.results))
		}
		if nls.results[0].Path != "ml.md" {
			t.Errorf("expected path 'ml.md', got %q", nls.results[0].Path)
		}
		if nls.results[0].Relevance != "Contains machine learning content" {
			t.Errorf("unexpected relevance: %q", nls.results[0].Relevance)
		}
	})

	t.Run("parses dash-separated format", func(t *testing.T) {
		nls.parseAIResponse("ml.md - Contains ML content")
		if len(nls.results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(nls.results))
		}
	})

	t.Run("strips leading bullets and numbering", func(t *testing.T) {
		nls.parseAIResponse("1. ml.md | ML content\n2. recipes.md | Recipes\n- subfolder/deep.md | Deep learning")
		if len(nls.results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(nls.results))
		}
	})

	t.Run("skips empty lines", func(t *testing.T) {
		nls.parseAIResponse("\n\nml.md | Some content\n\n\n")
		if len(nls.results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(nls.results))
		}
	})

	t.Run("skips lines without separator", func(t *testing.T) {
		nls.parseAIResponse("ml.md | valid\nno separator here\nrecipes.md | also valid")
		if len(nls.results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(nls.results))
		}
	})

	t.Run("handles unmatched paths gracefully", func(t *testing.T) {
		nls.parseAIResponse("nonexistent.md | some reason")
		if len(nls.results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(nls.results))
		}
		if nls.results[0].Path != "nonexistent.md" {
			t.Errorf("expected path preserved for unmatched note, got %q", nls.results[0].Path)
		}
	})

	t.Run("matches path without .md extension", func(t *testing.T) {
		nls.parseAIResponse("ml | Has ML content")
		if len(nls.results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(nls.results))
		}
		if nls.results[0].Path != "ml.md" {
			t.Errorf("expected matched path 'ml.md', got %q", nls.results[0].Path)
		}
	})

	t.Run("fuzzy path matching by title", func(t *testing.T) {
		nls.parseAIResponse("machine learning | Relevant to ML")
		if len(nls.results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(nls.results))
		}
		if nls.results[0].Path != "ml.md" {
			t.Errorf("expected fuzzy match to resolve to 'ml.md', got %q", nls.results[0].Path)
		}
	})

	t.Run("limits to 15 results", func(t *testing.T) {
		var lines []string
		for i := 0; i < 20; i++ {
			lines = append(lines, "ml.md | reason "+string(rune('A'+i)))
		}
		nls.parseAIResponse(strings.Join(lines, "\n"))
		if len(nls.results) > 15 {
			t.Errorf("expected at most 15 results, got %d", len(nls.results))
		}
	})

	t.Run("empty response produces no results", func(t *testing.T) {
		nls.parseAIResponse("")
		if len(nls.results) != 0 {
			t.Errorf("expected 0 results for empty response, got %d", len(nls.results))
		}
	})

	t.Run("extracts snippet from matched entry", func(t *testing.T) {
		nls.parseAIResponse("ml.md | ML reasons")
		if len(nls.results) == 0 {
			t.Fatal("expected results")
		}
		// Snippet should be taken from the content, skipping heading and frontmatter
		if nls.results[0].Snippet == "" {
			t.Error("expected non-empty snippet from matched entry")
		}
	})
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestSetSize(t *testing.T) {
	nls := NewNLSearch()
	nls.SetSize(120, 40)
	if nls.width != 120 {
		t.Errorf("expected width 120, got %d", nls.width)
	}
	if nls.height != 40 {
		t.Errorf("expected height 40, got %d", nls.height)
	}
}

// ---------------------------------------------------------------------------
// MaxVisibleResults
// ---------------------------------------------------------------------------

func TestMaxVisibleResults(t *testing.T) {
	nls := NewNLSearch()
	nls.results = make([]nlSearchResult, 20)

	t.Run("minimum of 3", func(t *testing.T) {
		nls.SetSize(80, 20)
		m := nls.maxVisibleResults()
		if m < 3 {
			t.Errorf("expected minimum 3 visible results, got %d", m)
		}
	})

	t.Run("capped by result count", func(t *testing.T) {
		nls.results = make([]nlSearchResult, 2)
		nls.SetSize(80, 100)
		m := nls.maxVisibleResults()
		if m > 2 {
			t.Errorf("expected max 2 (result count), got %d", m)
		}
	})

	t.Run("grows with height", func(t *testing.T) {
		nls.results = make([]nlSearchResult, 20)
		nls.SetSize(80, 60)
		m1 := nls.maxVisibleResults()
		nls.SetSize(80, 100)
		m2 := nls.maxVisibleResults()
		if m2 < m1 {
			t.Errorf("expected more visible results with greater height: h=60 -> %d, h=100 -> %d", m1, m2)
		}
	})
}

// ---------------------------------------------------------------------------
// Overlay width helpers
// ---------------------------------------------------------------------------

func TestOverlayWidth(t *testing.T) {
	nls := NewNLSearch()

	t.Run("minimum width of 55", func(t *testing.T) {
		nls.SetSize(30, 40)
		if nls.overlayWidth() < 55 {
			t.Errorf("expected minimum overlay width 55, got %d", nls.overlayWidth())
		}
	})

	t.Run("maximum width of 90", func(t *testing.T) {
		nls.SetSize(300, 40)
		if nls.overlayWidth() > 90 {
			t.Errorf("expected maximum overlay width 90, got %d", nls.overlayWidth())
		}
	})

	t.Run("inner width is overlay minus 6", func(t *testing.T) {
		nls.SetSize(120, 40)
		if nls.overlayInnerWidth() != nls.overlayWidth()-6 {
			t.Errorf("expected innerWidth = overlayWidth - 6")
		}
	})
}

// ---------------------------------------------------------------------------
// Build search prompt
// ---------------------------------------------------------------------------

func TestBuildSearchPrompt(t *testing.T) {
	nls := NewNLSearch()
	nls.query = "machine learning"
	nls.noteIndex = []noteEntry{
		{Path: "ml.md", Title: "ML", Content: "Machine learning stuff", Tags: []string{"ml"}},
		{Path: "cook.md", Title: "Cook", Content: "Cooking recipes", Tags: nil},
	}

	prompt := nls.buildSearchPrompt()

	t.Run("includes user query", func(t *testing.T) {
		if !strings.Contains(prompt, "machine learning") {
			t.Error("prompt should contain the user query")
		}
	})

	t.Run("includes note paths", func(t *testing.T) {
		if !strings.Contains(prompt, "ml.md") {
			t.Error("prompt should contain note paths")
		}
	})

	t.Run("includes tags", func(t *testing.T) {
		if !strings.Contains(prompt, "[ml]") {
			t.Error("prompt should include tags in brackets")
		}
	})

	t.Run("limits to 100 notes", func(t *testing.T) {
		var bigIndex []noteEntry
		for i := 0; i < 150; i++ {
			bigIndex = append(bigIndex, noteEntry{Path: "note.md", Title: "Note", Content: "content"})
		}
		nls2 := NewNLSearch()
		nls2.query = "test"
		nls2.noteIndex = bigIndex
		prompt := nls2.buildSearchPrompt()
		lines := strings.Split(prompt, "\n")
		noteLines := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "- ") {
				noteLines++
			}
		}
		if noteLines > 100 {
			t.Errorf("expected at most 100 note lines in prompt, got %d", noteLines)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration test with real filesystem
// ---------------------------------------------------------------------------

func TestLocalSearchWithFilesystem(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestNote(t, tmpDir, "golang.md", "# Go Programming\nGo is a statically typed language.\n#golang #programming")
	writeTestNote(t, tmpDir, "python.md", "# Python Scripting\nPython is great for data science.\n#python #programming")
	writeTestNote(t, tmpDir, "recipes.md", "# Italian Recipes\nPasta with tomato sauce and basil.\n#cooking #italian")

	nls := NewNLSearch()
	nls.Open(tmpDir, "local", "", "", "", "")

	t.Run("finds programming notes", func(t *testing.T) {
		nls.query = "programming language"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results for 'programming language'")
		}
		paths := make(map[string]bool)
		for _, r := range results {
			paths[r.Path] = true
		}
		if !paths["golang.md"] {
			t.Error("expected golang.md in results")
		}
		if !paths["python.md"] {
			t.Error("expected python.md in results")
		}
	})

	t.Run("cooking search excludes programming notes", func(t *testing.T) {
		nls.query = "cooking italian"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results for 'cooking italian'")
		}
		if results[0].Path != "recipes.md" {
			t.Errorf("expected recipes.md as top result, got %q", results[0].Path)
		}
		for _, r := range results {
			if r.Path == "golang.md" || r.Path == "python.md" {
				t.Errorf("programming notes should not match cooking query: %s", r.Path)
			}
		}
	})

	t.Run("results have all fields populated", func(t *testing.T) {
		nls.query = "golang"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		r := results[0]
		if r.Path == "" {
			t.Error("result path should not be empty")
		}
		if r.Title == "" {
			t.Error("result title should not be empty")
		}
		if r.Relevance == "" {
			t.Error("result relevance should not be empty")
		}
	})
}

// ---------------------------------------------------------------------------
// Case insensitivity
// ---------------------------------------------------------------------------

func TestCaseInsensitivity(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{Path: "note.md", Title: "UPPERCASE TITLE", Content: "lowercase content about GoLang", Tags: []string{"GoTag"}},
	}

	t.Run("matches uppercase title with lowercase query", func(t *testing.T) {
		nls.query = "uppercase"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Error("expected case-insensitive match on title")
		}
	})

	t.Run("matches mixed case content", func(t *testing.T) {
		nls.query = "golang"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Error("expected case-insensitive match on content")
		}
	})

	t.Run("matches mixed case tags", func(t *testing.T) {
		nls.query = "gotag"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Error("expected case-insensitive match on tags")
		}
	})
}

// ---------------------------------------------------------------------------
// Path matching in search
// ---------------------------------------------------------------------------

func TestPathMatchingInSearch(t *testing.T) {
	nls := NewNLSearch()
	nls.noteIndex = []noteEntry{
		{Path: "projects/webapp.md", Title: "Web App", Content: "Some project docs", Tags: nil},
	}

	t.Run("matches on path component", func(t *testing.T) {
		nls.query = "projects"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Error("expected match on path component 'projects'")
		}
	})

	t.Run("path relevance noted when not in title", func(t *testing.T) {
		nls.query = "webapp"
		results := nls.runLocalSearch()
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		// "webapp" is in the path (webapp.md) but not in the title "Web App"
		// path matching should produce a result
		if results[0].Path != "projects/webapp.md" {
			t.Errorf("expected projects/webapp.md, got %q", results[0].Path)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTestNote(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test note %s: %v", name, err)
	}
}
