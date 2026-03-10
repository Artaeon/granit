package tui

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// spellBackend describes the underlying spell-checking strategy.
type spellBackend int

const (
	backendNone     spellBackend = iota
	backendAspell                // aspell pipe mode
	backendHunspell              // hunspell pipe mode
	backendBuiltin               // built-in word list + edit distance
)

// spellEngine provides spell-checking without requiring external binaries.
// It first tries aspell/hunspell (pipe mode) and falls back to a built-in
// dictionary loaded from common system word list paths.
type spellEngine struct {
	backend  spellBackend
	toolPath string // path to aspell or hunspell binary

	// Built-in dictionary: lowercase words loaded from system word list
	dict map[string]bool

	// Personal dictionary: user-added words (persisted to disk)
	personal     map[string]bool
	personalPath string

	// Session-only ignores (not persisted)
	sessionIgnore map[string]bool
}

// newSpellEngine probes for aspell or hunspell and falls back to a built-in
// dictionary loaded from common system word list locations.
func newSpellEngine() *spellEngine {
	se := &spellEngine{
		personal:      make(map[string]bool),
		sessionIgnore: make(map[string]bool),
	}

	// Try aspell first, then hunspell
	if path, err := exec.LookPath("aspell"); err == nil && path != "" {
		se.backend = backendAspell
		se.toolPath = path
	} else if path, err := exec.LookPath("hunspell"); err == nil && path != "" {
		se.backend = backendHunspell
		se.toolPath = path
	} else {
		// Fall back to built-in dictionary
		se.loadBuiltinDict()
		if len(se.dict) > 0 {
			se.backend = backendBuiltin
		}
	}

	// Load personal dictionary
	se.personalPath = filepath.Join(configDir(), "dictionary.txt")
	se.loadPersonalDict()

	return se
}

// configDir returns ~/.config/granit
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "granit")
}

// isAvailable reports whether any spell-checking backend was found.
func (se *spellEngine) isAvailable() bool {
	return se.backend != backendNone
}

// backendName returns a human-readable name for the active backend.
func (se *spellEngine) backendName() string {
	switch se.backend {
	case backendAspell:
		return "aspell"
	case backendHunspell:
		return "hunspell"
	case backendBuiltin:
		return "built-in"
	default:
		return "none"
	}
}

// loadBuiltinDict tries to load a system word list from common locations.
func (se *spellEngine) loadBuiltinDict() {
	paths := []string{
		"/usr/share/dict/words",
		"/usr/share/dict/american-english",
		"/usr/share/dict/british-english",
		"/usr/share/dict/english",
		"/usr/share/hunspell/en_US.dic",
		"/usr/share/myspell/dicts/en_US.dic",
	}

	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		se.dict = make(map[string]bool, 120000)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			word := strings.TrimSpace(scanner.Text())
			if word == "" || strings.HasPrefix(word, "#") {
				continue
			}
			// hunspell .dic files have affix flags after /
			if idx := strings.IndexByte(word, '/'); idx > 0 {
				word = word[:idx]
			}
			se.dict[strings.ToLower(word)] = true
		}
		_ = f.Close()
		if len(se.dict) > 100 {
			return
		}
		// Too few words — try next path
		se.dict = nil
	}
}

// loadPersonalDict reads the user's personal dictionary from disk.
func (se *spellEngine) loadPersonalDict() {
	f, err := os.Open(se.personalPath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		w := strings.TrimSpace(scanner.Text())
		if w != "" {
			se.personal[strings.ToLower(w)] = true
		}
	}
}

// savePersonalDict writes the personal dictionary to disk.
func (se *spellEngine) savePersonalDict() error {
	if err := os.MkdirAll(filepath.Dir(se.personalPath), 0700); err != nil {
		return err
	}

	var words []string
	for w := range se.personal {
		words = append(words, w)
	}
	sort.Strings(words)

	f, err := os.Create(se.personalPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	for _, w := range words {
		_, _ = f.WriteString(w + "\n")
	}
	return nil
}

// addToPersonal adds a word to the personal dictionary and persists it.
func (se *spellEngine) addToPersonal(word string) {
	se.personal[strings.ToLower(word)] = true
	_ = se.savePersonalDict()
}

// addSessionIgnore marks a word as ignored for this session only.
func (se *spellEngine) addSessionIgnore(word string) {
	se.sessionIgnore[strings.ToLower(word)] = true
}

// isIgnored checks whether a word is in the personal dictionary or session ignores.
func (se *spellEngine) isIgnored(word string) bool {
	lw := strings.ToLower(word)
	return se.personal[lw] || se.sessionIgnore[lw]
}

// check runs the spell checker on the given content and returns misspelled words.
// It strips markdown, frontmatter, code blocks, wikilinks, URLs, and ALL_CAPS words.
func (se *spellEngine) check(content string) []MisspelledWord {
	if !se.isAvailable() {
		return nil
	}

	switch se.backend {
	case backendAspell, backendHunspell:
		return se.checkExternal(content)
	case backendBuiltin:
		return se.checkBuiltin(content)
	default:
		return nil
	}
}

// checkExternal pipes content through aspell or hunspell and parses the output.
func (se *spellEngine) checkExternal(content string) []MisspelledWord {
	cleaned := stripMarkdownForSpellCheck(content)
	originalLines := strings.Split(content, "\n")

	var args []string
	if se.backend == backendAspell {
		args = []string{"pipe"}
	} else {
		args = []string{"-a"}
	}

	cmd := exec.Command(se.toolPath, args...)
	cmd.Stdin = strings.NewReader(cleaned)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var words []MisspelledWord
	outputLines := strings.Split(string(out), "\n")

	inputLine := 0
	firstLine := true

	for _, ol := range outputLines {
		if firstLine {
			firstLine = false
			continue
		}

		if ol == "" {
			inputLine++
			continue
		}

		if strings.HasPrefix(ol, "&") {
			word, offset, suggestions := parseAmpersandLine(ol)
			if word == "" {
				continue
			}
			if se.shouldSkipWord(word) {
				continue
			}

			col := findWordCol(originalLines, inputLine, word, offset)

			if len(suggestions) > 5 {
				suggestions = suggestions[:5]
			}

			words = append(words, MisspelledWord{
				Word:    word,
				Line:    inputLine,
				Col:     col,
				Suggest: suggestions,
			})
		} else if strings.HasPrefix(ol, "#") {
			word, offset := parseHashLine(ol)
			if word == "" {
				continue
			}
			if se.shouldSkipWord(word) {
				continue
			}

			col := findWordCol(originalLines, inputLine, word, offset)

			words = append(words, MisspelledWord{
				Word:    word,
				Line:    inputLine,
				Col:     col,
				Suggest: nil,
			})
		}
	}

	return words
}

// checkBuiltin checks content against the built-in dictionary.
func (se *spellEngine) checkBuiltin(content string) []MisspelledWord {
	originalLines := strings.Split(content, "\n")
	cleanedLines := strings.Split(stripMarkdownForSpellCheck(content), "\n")

	var words []MisspelledWord

	// Regex to extract words (letters and apostrophes)
	wordRe := regexp.MustCompile(`[a-zA-Z][a-zA-Z']*[a-zA-Z]|[a-zA-Z]`)

	for lineIdx, cleaned := range cleanedLines {
		if cleaned == "" {
			continue
		}

		matches := wordRe.FindAllStringIndex(cleaned, -1)
		for _, loc := range matches {
			word := cleaned[loc[0]:loc[1]]
			if se.shouldSkipWord(word) {
				continue
			}

			lw := strings.ToLower(word)
			if se.dict[lw] {
				continue
			}

			// Try without trailing 's
			if strings.HasSuffix(lw, "'s") {
				base := lw[:len(lw)-2]
				if se.dict[base] {
					continue
				}
			}

			// Find position in original line
			col := findWordCol(originalLines, lineIdx, word, loc[0]+1)

			suggestions := se.suggest(lw, 5)

			words = append(words, MisspelledWord{
				Word:    word,
				Line:    lineIdx,
				Col:     col,
				Suggest: suggestions,
			})
		}
	}

	return words
}

// shouldSkipWord returns true if the word should not be spell-checked.
func (se *spellEngine) shouldSkipWord(word string) bool {
	// Skip very short words
	if len(word) <= 1 {
		return true
	}

	// Skip ALL_CAPS (acronyms)
	allCaps := true
	for _, r := range word {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			allCaps = false
			break
		}
	}
	if allCaps && len(word) >= 2 {
		return true
	}

	// Skip words with digits
	for _, r := range word {
		if unicode.IsDigit(r) {
			return true
		}
	}

	// Skip words in personal dictionary or session ignore list
	if se.isIgnored(word) {
		return true
	}

	return false
}

// suggest returns up to n spelling suggestions for a misspelled word using
// edit distance (Levenshtein). Only considers words within edit distance 2.
func (se *spellEngine) suggest(word string, n int) []string {
	if se.dict == nil {
		return nil
	}

	type candidate struct {
		word string
		dist int
	}

	var candidates []candidate
	wLen := len(word)

	for dw := range se.dict {
		// Quick length filter: edit distance can't be less than length difference
		diff := len(dw) - wLen
		if diff < 0 {
			diff = -diff
		}
		if diff > 2 {
			continue
		}

		d := editDistance(word, dw)
		if d <= 2 {
			candidates = append(candidates, candidate{dw, d})
		}
	}

	// Sort by distance, then alphabetically
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].dist != candidates[j].dist {
			return candidates[i].dist < candidates[j].dist
		}
		return candidates[i].word < candidates[j].word
	})

	var result []string
	for i, c := range candidates {
		if i >= n {
			break
		}
		result = append(result, c.word)
	}
	return result
}

// editDistance computes the Levenshtein distance between two strings.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row optimization
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			min := ins
			if del < min {
				min = del
			}
			if sub < min {
				min = sub
			}
			curr[j] = min
		}
		prev = curr
	}

	return prev[lb]
}

