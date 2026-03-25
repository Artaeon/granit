package vault

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// SearchResult represents a single search hit with relevance scoring.
type SearchResult struct {
	Path      string  // relative path of the matched note
	Line      int     // 0-based line number
	Column    int     // column of match start
	MatchLine string  // the actual text of the matching line
	Score     float64 // relevance score (higher = more relevant)
}

// SearchIndex is an inverted index for fast full-text search across vault notes.
type SearchIndex struct {
	mu sync.RWMutex

	// term -> set of document paths that contain this term
	invertedIndex map[string]map[string]bool

	// term -> document path -> list of line numbers where term appears
	positions map[string]map[string][]int

	// document path -> total word count (for TF-IDF scoring)
	docWordCount map[string]int

	// total number of documents
	totalDocs int

	// document path -> line contents (for returning match context)
	docLines map[string][]string

	// whether the index has been fully built
	ready bool
}

// NewSearchIndex creates an empty search index ready to be built.
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		invertedIndex: make(map[string]map[string]bool),
		positions:     make(map[string]map[string][]int),
		docWordCount:  make(map[string]int),
		docLines:      make(map[string][]string),
	}
}

// IsReady reports whether the index has been fully built at least once.
func (si *SearchIndex) IsReady() bool {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.ready
}

// Build scans all notes in the vault and builds the inverted index from scratch.
func (si *SearchIndex) Build(v *Vault) {
	si.mu.Lock()
	defer si.mu.Unlock()

	// Reset all data structures
	si.invertedIndex = make(map[string]map[string]bool)
	si.positions = make(map[string]map[string][]int)
	si.docWordCount = make(map[string]int)
	si.docLines = make(map[string][]string)
	si.totalDocs = 0

	for relPath, note := range v.Notes {
		if !note.loaded {
			// Skip unloaded notes — they'll be indexed when loaded
			continue
		}
		si.indexDocument(relPath, note.Content)
	}

	si.totalDocs = len(si.docWordCount)
	si.ready = true
}

// indexDocument adds a single document to the index. Caller must hold si.mu.
func (si *SearchIndex) indexDocument(path string, content string) {
	lines := strings.Split(content, "\n")
	si.docLines[path] = lines

	wordCount := 0
	for lineIdx, line := range lines {
		tokens := tokenize(line)
		wordCount += len(tokens)
		for _, token := range tokens {
			// Update inverted index
			if si.invertedIndex[token] == nil {
				si.invertedIndex[token] = make(map[string]bool)
			}
			si.invertedIndex[token][path] = true

			// Update positions
			if si.positions[token] == nil {
				si.positions[token] = make(map[string][]int)
			}
			// Only add the line number once per token per line
			lineNums := si.positions[token][path]
			if len(lineNums) == 0 || lineNums[len(lineNums)-1] != lineIdx {
				si.positions[token][path] = append(lineNums, lineIdx)
			}
		}
	}
	si.docWordCount[path] = wordCount
}

// Update re-indexes a single file. Use this after a file is saved or changed.
func (si *SearchIndex) Update(path string, content string) {
	si.mu.Lock()
	defer si.mu.Unlock()

	// Remove old data for this path first
	si.removeDocument(path)

	// Re-index with new content
	si.indexDocument(path, content)

	// Recalculate totalDocs
	si.totalDocs = len(si.docWordCount)
}

// Remove removes a file from the index entirely.
func (si *SearchIndex) Remove(path string) {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.removeDocument(path)
	si.totalDocs = len(si.docWordCount)
}

// removeDocument removes all index entries for a document. Caller must hold si.mu.
func (si *SearchIndex) removeDocument(path string) {
	delete(si.docLines, path)
	delete(si.docWordCount, path)

	// Remove from inverted index and positions
	for term, docs := range si.invertedIndex {
		if docs[path] {
			delete(docs, path)
			if len(docs) == 0 {
				delete(si.invertedIndex, term)
			}
		}
		if posMap, ok := si.positions[term]; ok {
			delete(posMap, path)
			if len(posMap) == 0 {
				delete(si.positions, term)
			}
		}
	}
}

// Search performs a full-text search for the given query string and returns
// results ranked by TF-IDF relevance. The query is split into tokens and
// documents matching all tokens are ranked highest.
func (si *SearchIndex) Search(query string) []SearchResult {
	si.mu.RLock()
	defer si.mu.RUnlock()

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Find candidate documents: those that contain at least one query token
	docScores := make(map[string]float64)
	docMatchedTerms := make(map[string]int) // number of query tokens matched

	for _, token := range queryTokens {
		docs, exists := si.invertedIndex[token]
		if !exists {
			continue
		}

		// IDF for this term
		if si.totalDocs == 0 {
			continue
		}
		idf := math.Log(1.0 + float64(si.totalDocs)/float64(len(docs)))

		for docPath := range docs {
			// TF: count of this term's line appearances / total words
			tf := float64(len(si.positions[token][docPath])) / float64(max(si.docWordCount[docPath], 1))
			docScores[docPath] += tf * idf
			docMatchedTerms[docPath]++
		}
	}

	if len(docScores) == 0 {
		return nil
	}

	// Boost documents that match all query terms
	numQueryTokens := len(queryTokens)
	for docPath, matchedCount := range docMatchedTerms {
		if matchedCount == numQueryTokens {
			docScores[docPath] *= 2.0 // boost for matching all terms
		}
	}

	// Sort documents by score
	type docScore struct {
		path  string
		score float64
	}
	ranked := make([]docScore, 0, len(docScores))
	for path, score := range docScores {
		ranked = append(ranked, docScore{path, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].path < ranked[j].path
	})

	// Build results: find the actual matching lines
	lowerQuery := strings.ToLower(query)
	var results []SearchResult

	for _, ds := range ranked {
		lines := si.docLines[ds.path]
		if lines == nil {
			continue
		}

		// Collect matching lines for this document
		var docResults []SearchResult

		// First pass: find lines that contain the full query as a substring
		for lineIdx, line := range lines {
			lowerLine := strings.ToLower(line)
			col := strings.Index(lowerLine, lowerQuery)
			if col >= 0 {
				docResults = append(docResults, SearchResult{
					Path:      ds.path,
					Line:      lineIdx,
					Column:    col,
					MatchLine: line,
					Score:     ds.score * 2.0, // bonus for full phrase match
				})
			}
		}

		// Second pass: if no full phrase match found, find lines with individual tokens
		if len(docResults) == 0 {
			for _, token := range queryTokens {
				lineNums, ok := si.positions[token][ds.path]
				if !ok {
					continue
				}
				for _, lineIdx := range lineNums {
					if lineIdx < len(lines) {
						line := lines[lineIdx]
						lowerLine := strings.ToLower(line)
						col := strings.Index(lowerLine, token)
						if col < 0 {
							col = 0
						}
						docResults = append(docResults, SearchResult{
							Path:      ds.path,
							Line:      lineIdx,
							Column:    col,
							MatchLine: line,
							Score:     ds.score,
						})
					}
				}
			}
		}

		// Deduplicate lines within this document
		seen := make(map[int]bool)
		for _, r := range docResults {
			if !seen[r.Line] {
				seen[r.Line] = true
				results = append(results, r)
			}
		}

		// Cap results at 200 to avoid overwhelming the UI
		if len(results) >= 200 {
			results = results[:200]
			break
		}
	}

	return results
}

// SearchRegex performs a regex search across all indexed documents.
func (si *SearchIndex) SearchRegex(pattern string) []SearchResult {
	si.mu.RLock()
	defer si.mu.RUnlock()

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}

	var results []SearchResult

	// Sort paths for deterministic output
	paths := make([]string, 0, len(si.docLines))
	for p := range si.docLines {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		lines := si.docLines[path]
		matchCount := 0
		for lineIdx, line := range lines {
			loc := re.FindStringIndex(line)
			if loc == nil {
				continue
			}
			matchCount++
			results = append(results, SearchResult{
				Path:      path,
				Line:      lineIdx,
				Column:    loc[0],
				MatchLine: line,
				Score:     1.0 / float64(max(matchCount, 1)), // earlier matches score higher
			})

			if len(results) >= 200 {
				return results
			}
		}
	}

	return results
}

// tokenize splits text into lowercase search tokens, stripping punctuation.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string

	var current strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// max returns the larger of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
