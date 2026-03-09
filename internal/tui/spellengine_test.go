package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestEngine returns a spellEngine with a small built-in dictionary
// suitable for unit tests (no external tool dependency).
func newTestEngine(words []string) *spellEngine {
	se := &spellEngine{
		backend:       backendBuiltin,
		dict:          make(map[string]bool, len(words)),
		personal:      make(map[string]bool),
		sessionIgnore: make(map[string]bool),
	}
	for _, w := range words {
		se.dict[w] = true
	}
	return se
}

func TestEditDistance_Identical(t *testing.T) {
	d := editDistance("hello", "hello")
	if d != 0 {
		t.Errorf("editDistance(\"hello\", \"hello\") = %d; want 0", d)
	}
}

func TestEditDistance_SingleChar(t *testing.T) {
	d := editDistance("cat", "bat")
	if d != 1 {
		t.Errorf("editDistance(\"cat\", \"bat\") = %d; want 1", d)
	}
}

func TestEditDistance_Insertion(t *testing.T) {
	d := editDistance("cat", "cats")
	if d != 1 {
		t.Errorf("editDistance(\"cat\", \"cats\") = %d; want 1", d)
	}
}

func TestEditDistance_Deletion(t *testing.T) {
	d := editDistance("cats", "cat")
	if d != 1 {
		t.Errorf("editDistance(\"cats\", \"cat\") = %d; want 1", d)
	}
}

func TestEditDistance_Unicode(t *testing.T) {
	// "café" vs "cafe": the é→e substitution should count as 1 edit
	d := editDistance("café", "cafe")
	if d != 1 {
		t.Errorf("editDistance(\"café\", \"cafe\") = %d; want 1", d)
	}
}

func TestEditDistance_Empty(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
	}
	for _, tc := range tests {
		d := editDistance(tc.a, tc.b)
		if d != tc.want {
			t.Errorf("editDistance(%q, %q) = %d; want %d", tc.a, tc.b, d, tc.want)
		}
	}
}

func TestIsSkipWord_AllCaps(t *testing.T) {
	se := newTestEngine(nil)
	if !se.shouldSkipWord("HTTP") {
		t.Error("shouldSkipWord(\"HTTP\") = false; want true (all-caps acronym)")
	}
	if !se.shouldSkipWord("API") {
		t.Error("shouldSkipWord(\"API\") = false; want true (all-caps acronym)")
	}
}

func TestIsSkipWord_URL(t *testing.T) {
	// URLs are stripped by stripMarkdownForSpellCheck before words reach
	// the spell checker, so "https://example.com" should be fully removed.
	cleaned := stripMarkdownForSpellCheck("Visit https://example.com today")
	if cleaned != "Visit  today" {
		t.Errorf("stripMarkdownForSpellCheck did not remove URL; got %q", cleaned)
	}
}

func TestIsSkipWord_WikiLink(t *testing.T) {
	// Wikilinks like [[note]] are reduced to their display text, which means
	// the brackets are removed and should not be flagged.
	cleaned := stripMarkdownForSpellCheck("See [[note]] for details")
	if cleaned != "See note for details" {
		t.Errorf("stripMarkdownForSpellCheck did not strip wikilink; got %q", cleaned)
	}
}

func TestSuggestCorrections(t *testing.T) {
	// Build a small dictionary with known words.
	se := newTestEngine([]string{"cat", "bat", "hat", "mat", "car", "can", "cap"})

	suggestions := se.suggest("cas", 5)
	if len(suggestions) == 0 {
		t.Fatal("suggest(\"cas\") returned no suggestions")
	}

	// All results should be within edit distance 2 and sorted by distance.
	prevDist := 0
	for _, s := range suggestions {
		d := editDistance("cas", s)
		if d > 2 {
			t.Errorf("suggestion %q has edit distance %d (> 2)", s, d)
		}
		if d < prevDist {
			t.Errorf("suggestions not sorted by distance: %q (dist %d) came after dist %d", s, d, prevDist)
		}
		prevDist = d
	}
}

func TestPersonalDictionary(t *testing.T) {
	// Use a temp directory for the personal dictionary file.
	tmpDir := t.TempDir()
	se := newTestEngine([]string{"hello", "world"})
	se.personalPath = filepath.Join(tmpDir, "dictionary.txt")

	// "granit" is not in the built-in dict and should be flagged.
	if se.shouldSkipWord("granit") {
		t.Error("\"granit\" should not be skipped before adding to personal dict")
	}

	// Add to personal dictionary.
	se.addToPersonal("granit")

	// Now it should be skipped (recognized).
	if !se.shouldSkipWord("granit") {
		t.Error("\"granit\" should be skipped after adding to personal dict")
	}

	// Verify persistence: the file should exist and contain the word.
	data, err := os.ReadFile(se.personalPath)
	if err != nil {
		t.Fatalf("failed to read personal dictionary file: %v", err)
	}
	if string(data) != "granit\n" {
		t.Errorf("personal dictionary file content = %q; want \"granit\\n\"", string(data))
	}
}
