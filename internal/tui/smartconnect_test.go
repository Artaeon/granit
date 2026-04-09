package tui

import (
	"math"
	"testing"
)

func TestScExtractWords_Basic(t *testing.T) {
	words := scExtractWords("The quick brown fox jumps over the lazy dog")
	if len(words) == 0 {
		t.Fatal("expected some words extracted")
	}
	// "the" is a stopword, should be filtered
	for _, w := range words {
		if w == "the" {
			t.Error("stopword 'the' should be filtered")
		}
	}
}

func TestScExtractWords_MarkdownStripped(t *testing.T) {
	words := scExtractWords("**bold** text with `code` and [links](url)")
	for _, w := range words {
		if w == "**bold**" || w == "`code`" {
			t.Errorf("markdown punctuation should be stripped: %q", w)
		}
	}
	// "bold" should remain
	found := false
	for _, w := range words {
		if w == "bold" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'bold' after stripping markdown")
	}
}

func TestScExtractWords_ShortWordsFiltered(t *testing.T) {
	words := scExtractWords("I am a go developer")
	for _, w := range words {
		if len(w) < 3 {
			t.Errorf("words shorter than 3 chars should be filtered: %q", w)
		}
	}
}

func TestScExtractWords_NumbersFiltered(t *testing.T) {
	words := scExtractWords("test 123 456 hello")
	for _, w := range words {
		if w == "123" || w == "456" {
			t.Errorf("pure numbers should be filtered: %q", w)
		}
	}
}

func TestScExtractWords_Empty(t *testing.T) {
	words := scExtractWords("")
	if len(words) != 0 {
		t.Errorf("expected 0 words for empty input, got %d", len(words))
	}
}

func TestScTermFrequency_Basic(t *testing.T) {
	words := []string{"hello", "world", "hello", "test"}
	tf := scTermFrequency(words)

	if math.Abs(tf["hello"]-0.5) > 0.01 {
		t.Errorf("expected tf[hello]=0.5, got %f", tf["hello"])
	}
	if math.Abs(tf["world"]-0.25) > 0.01 {
		t.Errorf("expected tf[world]=0.25, got %f", tf["world"])
	}
}

func TestScTermFrequency_Empty(t *testing.T) {
	tf := scTermFrequency(nil)
	if len(tf) != 0 {
		t.Errorf("expected empty map for nil words, got %d entries", len(tf))
	}
}

func TestScTermFrequency_SingleWord(t *testing.T) {
	tf := scTermFrequency([]string{"only"})
	if math.Abs(tf["only"]-1.0) > 0.01 {
		t.Errorf("single word should have tf=1.0, got %f", tf["only"])
	}
}

func TestScFindSharedTerms(t *testing.T) {
	tfA := map[string]float64{"golang": 0.5, "programming": 0.3, "test": 0.2}
	tfB := map[string]float64{"golang": 0.4, "programming": 0.1, "other": 0.5}
	idf := map[string]float64{"golang": 2.0, "programming": 1.5, "test": 1.0, "other": 1.0}

	shared := scFindSharedTerms(tfA, tfB, idf, 3)
	if len(shared) == 0 {
		t.Fatal("expected shared terms")
	}
	// "golang" should be the top shared term (highest combined TF-IDF)
	if shared[0] != "golang" {
		t.Errorf("expected 'golang' as top shared term, got %q", shared[0])
	}
}

func TestScFindSharedTerms_NoOverlap(t *testing.T) {
	tfA := map[string]float64{"foo": 0.5}
	tfB := map[string]float64{"bar": 0.5}
	idf := map[string]float64{"foo": 1.0, "bar": 1.0}

	shared := scFindSharedTerms(tfA, tfB, idf, 3)
	if len(shared) != 0 {
		t.Errorf("expected no shared terms, got %v", shared)
	}
}
