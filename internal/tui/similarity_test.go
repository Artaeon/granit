package tui

import (
	"math"
	"testing"
)

func TestTfidfTokenize(t *testing.T) {
	t.Run("lowercases input", func(t *testing.T) {
		tokens := tfidfTokenize("Hello WORLD FooBar")
		for _, tok := range tokens {
			for _, ch := range tok {
				if ch >= 'A' && ch <= 'Z' {
					t.Errorf("expected lowercase tokens, found %q in result", tok)
					break
				}
			}
		}
	})

	t.Run("splits on non-alphanumeric", func(t *testing.T) {
		tokens := tfidfTokenize("hello-world foo_bar baz.qux")
		// Should produce tokens split on -, _, .
		found := make(map[string]bool)
		for _, tok := range tokens {
			found[tok] = true
		}
		for _, want := range []string{"hello", "world", "foo", "bar", "baz", "qux"} {
			if !found[want] {
				t.Errorf("expected token %q in result, got tokens: %v", want, tokens)
			}
		}
	})

	t.Run("removes stopwords", func(t *testing.T) {
		tokens := tfidfTokenize("the quick brown fox and the lazy dog")
		for _, tok := range tokens {
			if stopwords[tok] {
				t.Errorf("stopword %q should have been removed", tok)
			}
		}
	})

	t.Run("removes short tokens", func(t *testing.T) {
		tokens := tfidfTokenize("I am a Go developer in NY")
		for _, tok := range tokens {
			if len(tok) < 3 {
				t.Errorf("token %q is shorter than 3 characters and should have been removed", tok)
			}
		}
	})

	t.Run("empty input", func(t *testing.T) {
		tokens := tfidfTokenize("")
		if len(tokens) != 0 {
			t.Errorf("expected no tokens for empty input, got %v", tokens)
		}
	})

	t.Run("only stopwords and short tokens", func(t *testing.T) {
		tokens := tfidfTokenize("the is a an it of in to")
		if len(tokens) != 0 {
			t.Errorf("expected no tokens when all are stopwords/short, got %v", tokens)
		}
	})

	t.Run("preserves content words", func(t *testing.T) {
		tokens := tfidfTokenize("machine learning algorithms neural networks")
		found := make(map[string]bool)
		for _, tok := range tokens {
			found[tok] = true
		}
		for _, want := range []string{"machine", "learning", "algorithms", "neural", "networks"} {
			if !found[want] {
				t.Errorf("expected content word %q, got tokens: %v", want, tokens)
			}
		}
	})
}

func TestBuildTFIDF(t *testing.T) {
	notes := map[string]string{
		"note1.md": "machine learning algorithms deep learning",
		"note2.md": "machine learning neural networks",
		"note3.md": "cooking recipes italian pasta",
	}

	idx := BuildTFIDF(notes)

	t.Run("doc count", func(t *testing.T) {
		if idx.DocCount != 3 {
			t.Errorf("expected DocCount 3, got %d", idx.DocCount)
		}
	})

	t.Run("docs are sorted", func(t *testing.T) {
		if len(idx.Docs) != 3 {
			t.Fatalf("expected 3 docs, got %d", len(idx.Docs))
		}
		for i := 1; i < len(idx.Docs); i++ {
			if idx.Docs[i] < idx.Docs[i-1] {
				t.Errorf("docs not sorted: %v", idx.Docs)
				break
			}
		}
	})

	t.Run("term freqs populated for each doc", func(t *testing.T) {
		for _, doc := range idx.Docs {
			tf, ok := idx.TermFreqs[doc]
			if !ok {
				t.Errorf("no TermFreqs entry for %q", doc)
				continue
			}
			if len(tf) == 0 {
				t.Errorf("empty TermFreqs for %q", doc)
			}
		}
	})

	t.Run("shared terms have lower IDF", func(t *testing.T) {
		// "machine" and "learning" appear in 2 docs; "cooking" in 1
		// The IDF of "machine" should be less than "cooking"
		machineDF := idx.DocFreq["machine"]
		cookingDF := idx.DocFreq["cooking"]

		if machineDF <= cookingDF {
			t.Errorf("expected 'machine' to have higher DF than 'cooking': machine=%d, cooking=%d", machineDF, cookingDF)
		}
	})

	t.Run("doc freq counts", func(t *testing.T) {
		// "learning" appears in note1 and note2
		if idx.DocFreq["learning"] != 2 {
			t.Errorf("expected DocFreq[learning]=2, got %d", idx.DocFreq["learning"])
		}
		// "cooking" appears only in note3
		if idx.DocFreq["cooking"] != 1 {
			t.Errorf("expected DocFreq[cooking]=1, got %d", idx.DocFreq["cooking"])
		}
	})

	t.Run("empty notes map", func(t *testing.T) {
		emptyIdx := BuildTFIDF(map[string]string{})
		if emptyIdx.DocCount != 0 {
			t.Errorf("expected DocCount 0 for empty input, got %d", emptyIdx.DocCount)
		}
		if len(emptyIdx.Docs) != 0 {
			t.Errorf("expected 0 docs for empty input, got %d", len(emptyIdx.Docs))
		}
	})
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical vectors", func(t *testing.T) {
		a := map[string]float64{"x": 1, "y": 2, "z": 3}
		sim := cosineSimilarity(a, a)
		if math.Abs(sim-1.0) > 0.0001 {
			t.Errorf("identical vectors should have similarity ~1.0, got %f", sim)
		}
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		a := map[string]float64{"x": 1, "y": 0}
		b := map[string]float64{"x": 0, "y": 1}
		sim := cosineSimilarity(a, b)
		if math.Abs(sim) > 0.0001 {
			t.Errorf("orthogonal vectors should have similarity ~0, got %f", sim)
		}
	})

	t.Run("no overlap", func(t *testing.T) {
		a := map[string]float64{"x": 1}
		b := map[string]float64{"y": 1}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("no overlap should give 0, got %f", sim)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		a := map[string]float64{"x": 1, "y": 1}
		b := map[string]float64{"x": 1, "z": 1}
		sim := cosineSimilarity(a, b)
		if sim <= 0 || sim >= 1 {
			t.Errorf("partial overlap should give 0 < sim < 1, got %f", sim)
		}
	})

	t.Run("empty vectors", func(t *testing.T) {
		a := map[string]float64{}
		b := map[string]float64{"x": 1}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("empty vector should give 0, got %f", sim)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		a := map[string]float64{}
		b := map[string]float64{}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("both empty should give 0, got %f", sim)
		}
	})

	t.Run("scaled vectors same direction", func(t *testing.T) {
		a := map[string]float64{"x": 1, "y": 2}
		b := map[string]float64{"x": 10, "y": 20}
		sim := cosineSimilarity(a, b)
		if math.Abs(sim-1.0) > 0.0001 {
			t.Errorf("parallel vectors should have similarity ~1.0, got %f", sim)
		}
	})
}

func TestFindSimilar(t *testing.T) {
	notes := map[string]string{
		"ml.md":       "machine learning deep learning neural networks algorithms",
		"ai.md":       "artificial intelligence machine learning models training",
		"cooking.md":  "recipes pasta italian cooking tomato sauce",
		"garden.md":   "plants flowers garden soil watering sunlight",
	}

	idx := BuildTFIDF(notes)

	t.Run("similar notes to ML note", func(t *testing.T) {
		results := FindSimilar(idx, "ml.md", 3)
		if len(results) == 0 {
			t.Fatal("expected at least one similar note")
		}
		// The AI note should be most similar to the ML note
		if results[0].Path != "ai.md" {
			t.Errorf("expected ai.md as most similar to ml.md, got %q", results[0].Path)
		}
	})

	t.Run("scores are descending", func(t *testing.T) {
		results := FindSimilar(idx, "ml.md", 10)
		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("results not in descending order: [%d].Score=%f > [%d].Score=%f",
					i, results[i].Score, i-1, results[i-1].Score)
			}
		}
	})

	t.Run("does not include self", func(t *testing.T) {
		results := FindSimilar(idx, "ml.md", 10)
		for _, r := range results {
			if r.Path == "ml.md" {
				t.Error("FindSimilar should not return the query note itself")
			}
		}
	})

	t.Run("scores are between 0 and 1", func(t *testing.T) {
		results := FindSimilar(idx, "ml.md", 10)
		for _, r := range results {
			if r.Score < 0 || r.Score > 1.0+0.0001 {
				t.Errorf("score %f for %q is outside [0, 1]", r.Score, r.Path)
			}
		}
	})

	t.Run("common terms populated", func(t *testing.T) {
		results := FindSimilar(idx, "ml.md", 1)
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		if len(results[0].CommonTerms) == 0 {
			t.Error("expected CommonTerms to be populated for similar notes")
		}
	})

	t.Run("topN limits results", func(t *testing.T) {
		results := FindSimilar(idx, "ml.md", 1)
		if len(results) > 1 {
			t.Errorf("expected at most 1 result with topN=1, got %d", len(results))
		}
	})

	t.Run("nil index returns nil", func(t *testing.T) {
		results := FindSimilar(nil, "ml.md", 5)
		if results != nil {
			t.Error("expected nil result for nil index")
		}
	})

	t.Run("missing note returns nil", func(t *testing.T) {
		results := FindSimilar(idx, "nonexistent.md", 5)
		if results != nil {
			t.Error("expected nil result for missing note path")
		}
	})
}

func TestSuggestMissingLinks(t *testing.T) {
	notes := map[string]string{
		"ml.md":       "machine learning deep learning algorithms",
		"ai.md":       "artificial intelligence machine learning",
		"cooking.md":  "recipes pasta italian cooking",
	}

	idx := BuildTFIDF(notes)

	t.Run("excludes existing links", func(t *testing.T) {
		existing := []string{"ai.md"}
		suggestions := SuggestMissingLinks(idx, "ml.md", existing)
		for _, s := range suggestions {
			if s.Path == "ai.md" {
				t.Error("should not suggest already-linked note")
			}
		}
	})

	t.Run("nil index returns nil", func(t *testing.T) {
		results := SuggestMissingLinks(nil, "ml.md", nil)
		if results != nil {
			t.Error("expected nil for nil index")
		}
	})

	t.Run("no existing links", func(t *testing.T) {
		suggestions := SuggestMissingLinks(idx, "ml.md", nil)
		// Should return similar notes without filtering
		if len(suggestions) == 0 {
			t.Error("expected suggestions when no existing links")
		}
	})
}
