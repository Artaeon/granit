package tui

import (
	"math"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// TFIDFIndex holds the computed TF-IDF vectors for a set of documents.
type TFIDFIndex struct {
	DocFreq   map[string]int                // term → number of docs containing it
	TermFreqs map[string]map[string]float64 // doc → term → TF-IDF score
	DocCount  int
	Docs      []string
}

// SimilarNote represents a note similar to a query note.
type SimilarNote struct {
	Path        string
	Score       float64
	CommonTerms []string // shared important terms
}

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

// tfidfTokenize lowercases, splits on non-alphanumeric characters, removes
// stopwords (reuses the package-level stopwords map from bots.go), and
// removes tokens shorter than 3 characters.
func tfidfTokenize(text string) []string {
	lower := strings.ToLower(text)

	// Split on any non-alphanumeric character.
	var tokens []string
	start := -1
	for i, ch := range lower {
		isAlnum := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if isAlnum {
			if start < 0 {
				start = i
			}
		} else {
			if start >= 0 {
				tokens = append(tokens, lower[start:i])
				start = -1
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, lower[start:])
	}

	// Filter stopwords and short tokens.
	filtered := tokens[:0]
	for _, t := range tokens {
		if len(t) >= 3 && !stopwords[t] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// ---------------------------------------------------------------------------
// Index construction
// ---------------------------------------------------------------------------

// BuildTFIDF constructs a TF-IDF index from a map of notePath → content.
func BuildTFIDF(notes map[string]string) *TFIDFIndex {
	idx := &TFIDFIndex{
		DocFreq:   make(map[string]int),
		TermFreqs: make(map[string]map[string]float64),
		DocCount:  len(notes),
	}

	// Collect document paths in deterministic order.
	docs := make([]string, 0, len(notes))
	for path := range notes {
		docs = append(docs, path)
	}
	sort.Strings(docs)
	idx.Docs = docs

	// Phase 1: compute raw term frequencies and document frequencies.
	rawTF := make(map[string]map[string]float64, len(docs))
	for _, path := range docs {
		tokens := tfidfTokenize(notes[path])
		counts := make(map[string]int)
		for _, t := range tokens {
			counts[t]++
		}
		total := len(tokens)
		tf := make(map[string]float64, len(counts))
		for term, count := range counts {
			tf[term] = float64(count) / math.Max(float64(total), 1)
		}
		rawTF[path] = tf

		// Document frequency: count each term once per document.
		seen := make(map[string]bool, len(counts))
		for term := range counts {
			if !seen[term] {
				idx.DocFreq[term]++
				seen[term] = true
			}
		}
	}

	// Phase 2: compute TF-IDF = TF * IDF.
	for _, path := range docs {
		tfidf := make(map[string]float64, len(rawTF[path]))
		for term, tf := range rawTF[path] {
			df := idx.DocFreq[term]
			idf := math.Log(float64(idx.DocCount) / math.Max(float64(df), 1))
			tfidf[term] = tf * idf
		}
		idx.TermFreqs[path] = tfidf
	}

	return idx
}

// ---------------------------------------------------------------------------
// Cosine similarity
// ---------------------------------------------------------------------------

// cosineSimilarity computes the cosine similarity between two TF-IDF vectors.
func cosineSimilarity(a, b map[string]float64) float64 {
	var dot, normA, normB float64
	for term, va := range a {
		normA += va * va
		if vb, ok := b[term]; ok {
			dot += va * vb
		}
	}
	for _, vb := range b {
		normB += vb * vb
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// commonImportantTerms returns the top shared terms (by combined TF-IDF)
// between two documents, capped at 10.
func commonImportantTerms(a, b map[string]float64) []string {
	type termScore struct {
		term  string
		score float64
	}
	var shared []termScore
	for term, va := range a {
		if vb, ok := b[term]; ok {
			shared = append(shared, termScore{term, va + vb})
		}
	}
	sort.Slice(shared, func(i, j int) bool {
		return shared[i].score > shared[j].score
	})
	cap := 10
	if len(shared) < cap {
		cap = len(shared)
	}
	result := make([]string, cap)
	for i := 0; i < cap; i++ {
		result[i] = shared[i].term
	}
	return result
}

// ---------------------------------------------------------------------------
// Query functions
// ---------------------------------------------------------------------------

// FindSimilar returns the topN notes most similar to the note at notePath,
// ranked by cosine similarity of their TF-IDF vectors.
func FindSimilar(index *TFIDFIndex, notePath string, topN int) []SimilarNote {
	if index == nil {
		return nil
	}
	queryVec, ok := index.TermFreqs[notePath]
	if !ok {
		return nil
	}

	type scored struct {
		path  string
		score float64
	}
	var results []scored
	for _, path := range index.Docs {
		if path == notePath {
			continue
		}
		docVec := index.TermFreqs[path]
		sim := cosineSimilarity(queryVec, docVec)
		if sim > 0 {
			results = append(results, scored{path, sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topN > len(results) {
		topN = len(results)
	}

	similar := make([]SimilarNote, topN)
	for i := 0; i < topN; i++ {
		similar[i] = SimilarNote{
			Path:        results[i].path,
			Score:       results[i].score,
			CommonTerms: commonImportantTerms(queryVec, index.TermFreqs[results[i].path]),
		}
	}
	return similar
}

// SuggestMissingLinks finds notes that are similar to notePath but are not
// already linked from it.  existingLinks should contain the paths of notes
// that are already linked.
func SuggestMissingLinks(index *TFIDFIndex, notePath string, existingLinks []string) []SimilarNote {
	if index == nil {
		return nil
	}

	linked := make(map[string]bool, len(existingLinks))
	for _, l := range existingLinks {
		linked[l] = true
	}

	// Get a generous set of similar notes.
	candidates := FindSimilar(index, notePath, len(index.Docs))

	var suggestions []SimilarNote
	for _, c := range candidates {
		if linked[c.Path] {
			continue
		}
		suggestions = append(suggestions, c)
	}

	// Cap at a reasonable number.
	maxSuggestions := 10
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	return suggestions
}
