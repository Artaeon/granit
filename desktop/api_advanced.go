package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ==================== Auto-Link Suggestions ====================

// LinkSuggestionDTO represents an unlinked mention of a note title found
// in the current note's content.
type LinkSuggestionDTO struct {
	Target  string `json:"target"`
	Context string `json:"context"`
	Line    int    `json:"line"`
}

// GetAutoLinkSuggestions analyzes the note at relPath and finds mentions
// of other note titles that are not already wrapped in [[ ]].
func (a *GranitApp) GetAutoLinkSuggestions(relPath string) ([]LinkSuggestionDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(relPath)
	if note == nil {
		return nil, fmt.Errorf("note not found")
	}

	currentBase := strings.TrimSuffix(filepath.Base(relPath), ".md")
	currentBaseLower := strings.ToLower(currentBase)

	// Collect all note names as candidates.
	type candidate struct {
		name     string
		nameLow  string
		relPath  string
	}
	var candidates []candidate
	seen := make(map[string]bool)
	for _, p := range a.vault.SortedPaths() {
		name := strings.TrimSuffix(filepath.Base(p), ".md")
		if len(name) < 3 {
			continue
		}
		nameLow := strings.ToLower(name)
		if nameLow == currentBaseLower {
			continue
		}
		if seen[nameLow] {
			continue
		}
		seen[nameLow] = true
		candidates = append(candidates, candidate{name: name, nameLow: nameLow, relPath: p})
	}

	lines := strings.Split(note.Content, "\n")
	wikilinkRe := regexp.MustCompile(`\[\[[^\]]*\]\]`)

	// Track fenced code blocks.
	inCodeBlock := make([]bool, len(lines))
	fenced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			inCodeBlock[i] = true
			continue
		}
		inCodeBlock[i] = fenced
	}

	var suggestions []LinkSuggestionDTO
	mentionSeen := make(map[string]bool)

	for _, c := range candidates {
		for lineIdx, line := range lines {
			if inCodeBlock[lineIdx] {
				continue
			}
			lineLower := strings.ToLower(line)
			searchFrom := 0
			for searchFrom < len(lineLower) {
				idx := strings.Index(lineLower[searchFrom:], c.nameLow)
				if idx < 0 {
					break
				}
				colStart := searchFrom + idx
				colEnd := colStart + len(c.nameLow)

				// Word boundary check.
				validStart := colStart == 0 || !advIsAlphaNum(line[colStart-1])
				validEnd := colEnd >= len(line) || !advIsAlphaNum(line[colEnd])

				if validStart && validEnd {
					// Check not inside [[ ]].
					insideLink := false
					for _, loc := range wikilinkRe.FindAllStringIndex(line, -1) {
						if colStart >= loc[0] && colEnd <= loc[1] {
							insideLink = true
							break
						}
					}
					if !insideLink && !mentionSeen[c.nameLow] {
						mentionSeen[c.nameLow] = true
						// Build context snippet.
						contextLine := strings.TrimSpace(line)
						if len(contextLine) > 120 {
							contextLine = contextLine[:120] + "..."
						}
						suggestions = append(suggestions, LinkSuggestionDTO{
							Target:  c.name,
							Context: contextLine,
							Line:    lineIdx + 1,
						})
					}
				}
				searchFrom = colEnd
			}
		}
	}

	return suggestions, nil
}

func advIsAlphaNum(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

// ==================== Blog Publisher ====================

// PublishToBlog exports a note as blog-ready HTML or clean markdown.
// Format: "html" or "markdown". Returns the output file path.
func (a *GranitApp) PublishToBlog(relPath string, format string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(relPath)
	if note == nil {
		return "", fmt.Errorf("note not found")
	}

	// Extract frontmatter metadata for blog header.
	title := note.Title
	dateStr := time.Now().Format("2006-01-02")
	if d, ok := note.Frontmatter["date"]; ok {
		if ds, ok := d.(string); ok && ds != "" {
			dateStr = ds
		}
	}

	var tags []string
	if t, ok := note.Frontmatter["tags"]; ok {
		tagMap := make(map[string]int)
		extractTagsFromValue(t, tagMap)
		for tag := range tagMap {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
	}

	// Create output directory.
	outDir := filepath.Join(a.vaultRoot, "_blog")
	os.MkdirAll(outDir, 0755)

	baseName := advSanitizeFilename(title)

	switch format {
	case "html":
		// Convert content to HTML with blog wrapper.
		body := markdownToHTML(note.Content)

		// Add blog metadata header.
		var metaHTML strings.Builder
		metaHTML.WriteString(fmt.Sprintf("<header>\n<h1>%s</h1>\n", htmlEscape(title)))
		metaHTML.WriteString(fmt.Sprintf("<time datetime=\"%s\">%s</time>\n", dateStr, dateStr))
		if len(tags) > 0 {
			metaHTML.WriteString("<div class=\"tags\">")
			for _, tag := range tags {
				metaHTML.WriteString(fmt.Sprintf("<span class=\"tag\">#%s</span> ", htmlEscape(tag)))
			}
			metaHTML.WriteString("</div>\n")
		}
		metaHTML.WriteString("</header>\n<hr>\n")

		wrapped := wrapBlogHTML(title, metaHTML.String()+body)
		outPath := filepath.Join(outDir, baseName+".html")
		if err := atomicWriteFile(outPath, []byte(wrapped), 0644); err != nil {
			return "", err
		}
		return outPath, nil

	case "markdown":
		// Clean markdown: add proper frontmatter and strip vault-specific syntax.
		var buf strings.Builder
		buf.WriteString("---\n")
		buf.WriteString(fmt.Sprintf("title: \"%s\"\n", title))
		buf.WriteString(fmt.Sprintf("date: %s\n", dateStr))
		if len(tags) > 0 {
			buf.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(tags, ", ")))
		}
		buf.WriteString("---\n\n")

		// Strip frontmatter from original content and append clean body.
		content := note.Content
		if strings.HasPrefix(content, "---") {
			end := strings.Index(content[3:], "---")
			if end >= 0 {
				content = strings.TrimSpace(content[3+end+3:])
			}
		}

		// Convert [[wikilinks]] to plain text for blog.
		wlRe := regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
		content = wlRe.ReplaceAllStringFunc(content, func(m string) string {
			parts := wlRe.FindStringSubmatch(m)
			if len(parts) > 2 && parts[2] != "" {
				return parts[2]
			}
			return parts[1]
		})
		buf.WriteString(content)

		outPath := filepath.Join(outDir, baseName+".md")
		if err := atomicWriteFile(outPath, []byte(buf.String()), 0644); err != nil {
			return "", err
		}
		return outPath, nil

	default:
		return "", fmt.Errorf("unsupported format: %s (use \"html\" or \"markdown\")", format)
	}
}

func advSanitizeFilename(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")
	re := regexp.MustCompile(`[^a-z0-9\-]`)
	s = re.ReplaceAllString(s, "")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}

func wrapBlogHTML(title, body string) string {
	title = htmlEscape(title)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;max-width:720px;margin:2rem auto;padding:0 1.5rem;line-height:1.8;color:#333;background:#fff}
header{margin-bottom:2rem}
h1{font-size:2rem;margin-bottom:.5rem}
time{color:#666;font-size:.9rem}
.tags{margin-top:.5rem}.tag{display:inline-block;background:#f0f0f0;padding:.1em .6em;border-radius:3px;font-size:.8rem;margin-right:.4em;color:#555}
a{color:#2563eb}code{background:#f5f5f5;padding:.2em .4em;border-radius:3px;font-size:.9em}
pre{background:#f5f5f5;padding:1em;border-radius:6px;overflow-x:auto}pre code{background:none;padding:0}
blockquote{border-left:3px solid #7c3aed;margin-left:0;padding-left:1em;color:#666}
h1,h2,h3,h4,h5,h6{color:#111}
hr{border:none;border-top:1px solid #e5e5e5;margin:2rem 0}
</style>
</head>
<body>
%s
</body>
</html>`, title, body)
}

// ==================== Encryption ====================

const (
	advEncSaltLen  = 16
	advEncNonceLen = 12
	advEncKeyLen   = 32
	advEncIter     = 100000
)

// advDeriveKey produces a 32-byte key from passphrase + salt using
// iterated HMAC-SHA256 (PBKDF2-like, stdlib only).
func advDeriveKey(passphrase string, salt []byte) []byte {
	prf := hmac.New(sha256.New, []byte(passphrase))
	input := make([]byte, len(salt)+4)
	copy(input, salt)
	input[len(salt)+3] = 1

	prf.Write(input)
	u := prf.Sum(nil)

	dk := make([]byte, len(u))
	copy(dk, u)

	for i := 1; i < advEncIter; i++ {
		prf.Reset()
		prf.Write(u)
		u = prf.Sum(nil)
		for j := range dk {
			dk[j] ^= u[j]
		}
	}
	return dk[:advEncKeyLen]
}

// EncryptNote encrypts the note content at relPath with AES-256-GCM
// and replaces the file content with the encrypted base64 blob.
func (a *GranitApp) EncryptNote(relPath string, password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault == nil {
		return fmt.Errorf("no vault open")
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	absPath, err := a.validatePath(relPath)
	if err != nil {
		return err
	}

	plaintext, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("reading note: %w", err)
	}

	// Check if already encrypted (base64 blob with GRANIT-ENC header).
	content := strings.TrimSpace(string(plaintext))
	if strings.HasPrefix(content, "GRANIT-ENC:") {
		return fmt.Errorf("note is already encrypted")
	}

	// Generate random salt.
	salt := make([]byte, advEncSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}

	key := advDeriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, advEncNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Assemble blob: salt + nonce + ciphertext.
	blob := make([]byte, 0, advEncSaltLen+advEncNonceLen+len(ciphertext))
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ciphertext...)

	encoded := "GRANIT-ENC:" + base64.StdEncoding.EncodeToString(blob)
	if err := atomicWriteFile(absPath, []byte(encoded), 0644); err != nil {
		return fmt.Errorf("writing encrypted file: %w", err)
	}

	// Update in-memory vault.
	if n := a.vault.GetNote(relPath); n != nil {
		n.Content = encoded
	}

	return nil
}

// DecryptNote decrypts the note at relPath and returns the plaintext.
// Does NOT save the decrypted content back to disk.
func (a *GranitApp) DecryptNote(relPath string, password string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.decryptNoteInternal(relPath, password)
}

// decryptNoteInternal is the lock-free version of DecryptNote.
// Callers must hold at least a.mu.RLock().
func (a *GranitApp) decryptNoteInternal(relPath string, password string) (string, error) {
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	absPath, err := a.validatePath(relPath)
	if err != nil {
		return "", err
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("reading note: %w", err)
	}

	content := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(content, "GRANIT-ENC:") {
		return "", fmt.Errorf("note is not encrypted")
	}

	encoded := strings.TrimPrefix(content, "GRANIT-ENC:")
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted data: %w", err)
	}

	minLen := advEncSaltLen + advEncNonceLen + 1
	if len(blob) < minLen {
		return "", fmt.Errorf("encrypted data too short")
	}

	salt := blob[:advEncSaltLen]
	nonce := blob[advEncSaltLen : advEncSaltLen+advEncNonceLen]
	ciphertext := blob[advEncSaltLen+advEncNonceLen:]

	key := advDeriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed — wrong password or corrupted data")
	}

	return string(plaintext), nil
}

// IsNoteEncrypted checks whether the note at relPath has been encrypted.
func (a *GranitApp) IsNoteEncrypted(relPath string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return false
	}
	note := a.vault.GetNote(relPath)
	if note == nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(note.Content), "GRANIT-ENC:")
}

// SaveDecryptedNote decrypts and saves the plaintext back to disk.
func (a *GranitApp) SaveDecryptedNote(relPath string, password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	plaintext, err := a.decryptNoteInternal(relPath, password)
	if err != nil {
		return err
	}
	absPath, err := a.validatePath(relPath)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(absPath, []byte(plaintext), 0644); err != nil {
		return err
	}
	if n := a.vault.GetNote(relPath); n != nil {
		n.Content = plaintext
	}
	return nil
}

// ==================== Recurring Tasks ====================

// RecurringTaskDTO represents a recurring task found by scanning notes.
type RecurringTaskDTO struct {
	Text     string `json:"text"`
	Pattern  string `json:"pattern"`
	NotePath string `json:"notePath"`
	Line     int    `json:"line"`
	NextDue  string `json:"nextDue"`
}

var recurrencePatterns = []struct {
	pattern string
	re      *regexp.Regexp
}{
	{"daily", regexp.MustCompile(`(?i)\b(daily|every\s+day)\b`)},
	{"weekly", regexp.MustCompile(`(?i)\b(weekly|every\s+week)\b`)},
	{"monthly", regexp.MustCompile(`(?i)\b(monthly|every\s+month)\b`)},
	{"every monday", regexp.MustCompile(`(?i)\bevery\s+monday\b`)},
	{"every tuesday", regexp.MustCompile(`(?i)\bevery\s+tuesday\b`)},
	{"every wednesday", regexp.MustCompile(`(?i)\bevery\s+wednesday\b`)},
	{"every thursday", regexp.MustCompile(`(?i)\bevery\s+thursday\b`)},
	{"every friday", regexp.MustCompile(`(?i)\bevery\s+friday\b`)},
	{"every saturday", regexp.MustCompile(`(?i)\bevery\s+saturday\b`)},
	{"every sunday", regexp.MustCompile(`(?i)\bevery\s+sunday\b`)},
}

var taskLineRe = regexp.MustCompile(`^(\s*)- \[([ xX])\]\s+(.+)`)

// GetRecurringTasks scans all notes for task lines with recurrence
// patterns (e.g., "every monday", "daily", "weekly").
func (a *GranitApp) GetRecurringTasks() ([]RecurringTaskDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	var tasks []RecurringTaskDTO
	today := time.Now()

	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}

		for lineIdx, line := range strings.Split(note.Content, "\n") {
			m := taskLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			taskText := m[3]

			for _, rp := range recurrencePatterns {
				if rp.re.MatchString(taskText) {
					nextDue := computeNextDue(rp.pattern, today)
					tasks = append(tasks, RecurringTaskDTO{
						Text:     taskText,
						Pattern:  rp.pattern,
						NotePath: p,
						Line:     lineIdx + 1,
						NextDue:  nextDue,
					})
					break
				}
			}
		}
	}

	return tasks, nil
}

func computeNextDue(pattern string, now time.Time) string {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	switch pattern {
	case "daily":
		return today.Format("2006-01-02")
	case "weekly":
		// Next Monday.
		daysUntil := (8 - int(today.Weekday())) % 7
		if daysUntil == 0 {
			daysUntil = 7
		}
		return today.AddDate(0, 0, daysUntil).Format("2006-01-02")
	case "monthly":
		next := time.Date(today.Year(), today.Month()+1, 1, 0, 0, 0, 0, today.Location())
		return next.Format("2006-01-02")
	}

	// Handle "every <weekday>".
	weekdays := map[string]time.Weekday{
		"every monday":    time.Monday,
		"every tuesday":   time.Tuesday,
		"every wednesday": time.Wednesday,
		"every thursday":  time.Thursday,
		"every friday":    time.Friday,
		"every saturday":  time.Saturday,
		"every sunday":    time.Sunday,
	}
	if wd, ok := weekdays[pattern]; ok {
		daysUntil := (int(wd) - int(today.Weekday()) + 7) % 7
		if daysUntil == 0 {
			return today.Format("2006-01-02") // Due today.
		}
		return today.AddDate(0, 0, daysUntil).Format("2006-01-02")
	}

	return today.Format("2006-01-02")
}

// ==================== Smart Connections ====================

// SmartConnectionDTO represents a note related to the current one.
type SmartConnectionDTO struct {
	RelPath string  `json:"relPath"`
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Reason  string  `json:"reason"`
}

// Markdown punctuation set for token cleaning.
var advMarkdownPunct = map[rune]bool{
	'#': true, '*': true, '-': true, '>': true,
	'[': true, ']': true, '(': true, ')': true,
	'`': true, '~': true, '|': true, '!': true,
	'{': true, '}': true, '_': true, '=': true,
}

// advExtractWords tokenizes text for TF-IDF: lowercases, strips
// punctuation and markdown syntax, filters stopwords.
func advExtractWords(text string) []string {
	var words []string
	for _, raw := range strings.Fields(text) {
		cleaned := strings.Map(func(r rune) rune {
			if advMarkdownPunct[r] || unicode.IsPunct(r) || unicode.IsSymbol(r) {
				return -1
			}
			return unicode.ToLower(r)
		}, raw)
		if len(cleaned) < 3 {
			continue
		}
		if stopwords[cleaned] {
			continue
		}
		allDigit := true
		for _, r := range cleaned {
			if !unicode.IsDigit(r) {
				allDigit = false
				break
			}
		}
		if allDigit {
			continue
		}
		words = append(words, cleaned)
	}
	return words
}

// advTermFrequency returns normalized word frequencies.
func advTermFrequency(words []string) map[string]float64 {
	counts := make(map[string]int)
	for _, w := range words {
		counts[w]++
	}
	total := float64(len(words))
	if total == 0 {
		total = 1
	}
	tf := make(map[string]float64, len(counts))
	for w, c := range counts {
		tf[w] = float64(c) / total
	}
	return tf
}

// advCosineSimilarity computes cosine similarity between two TF-IDF vectors.
func advCosineSimilarity(a, b map[string]float64) float64 {
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
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// GetSmartConnections finds notes similar to the one at relPath using
// TF-IDF cosine similarity, shared tags, and mutual links.
func (a *GranitApp) GetSmartConnections(relPath string) ([]SmartConnectionDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}
	currentNote := a.vault.GetNote(relPath)
	if currentNote == nil {
		return nil, fmt.Errorf("note not found")
	}

	// Build document vectors for all notes.
	type docVec struct {
		path  string
		title string
		tf    map[string]float64
		tags  map[string]bool
		links map[string]bool
	}

	docFreq := make(map[string]int)
	var docs []docVec

	for _, p := range a.vault.SortedPaths() {
		n := a.vault.GetNote(p)
		if n == nil {
			continue
		}
		words := advExtractWords(n.Content)
		tf := advTermFrequency(words)
		for w := range tf {
			docFreq[w]++
		}

		tagSet := make(map[string]bool)
		if tags, ok := n.Frontmatter["tags"]; ok {
			tagMap := make(map[string]int)
			extractTagsFromValue(tags, tagMap)
			for t := range tagMap {
				tagSet[t] = true
			}
		}

		linkSet := make(map[string]bool)
		for _, l := range n.Links {
			linkSet[strings.ToLower(l)] = true
		}

		docs = append(docs, docVec{
			path: p, title: n.Title, tf: tf, tags: tagSet, links: linkSet,
		})
	}

	if len(docs) < 2 {
		return nil, nil
	}

	// Compute IDF.
	numDocs := float64(len(docs))
	idf := make(map[string]float64, len(docFreq))
	for term, df := range docFreq {
		idf[term] = math.Log(numDocs / (1.0 + float64(df)))
	}

	// Find current note's index.
	currentIdx := -1
	for i, d := range docs {
		if d.path == relPath {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		return nil, fmt.Errorf("current note not indexed")
	}
	currentDoc := docs[currentIdx]

	// Build TF-IDF vector for current document.
	currentVec := make(map[string]float64, len(currentDoc.tf))
	for term, tf := range currentDoc.tf {
		currentVec[term] = tf * idf[term]
	}

	// Score all other documents.
	type scored struct {
		idx    int
		score  float64
		reason string
	}
	var scores []scored

	for i, doc := range docs {
		if i == currentIdx {
			continue
		}

		// TF-IDF cosine similarity.
		otherVec := make(map[string]float64, len(doc.tf))
		for term, tf := range doc.tf {
			otherVec[term] = tf * idf[term]
		}
		contentSim := advCosineSimilarity(currentVec, otherVec)

		// Shared tags bonus.
		sharedTags := 0
		var tagNames []string
		for tag := range currentDoc.tags {
			if doc.tags[tag] {
				sharedTags++
				tagNames = append(tagNames, tag)
			}
		}
		tagBonus := float64(sharedTags) * 0.1

		// Mutual links bonus.
		mutualLinks := 0
		currentTitle := strings.ToLower(strings.TrimSuffix(filepath.Base(relPath), ".md"))
		otherTitle := strings.ToLower(strings.TrimSuffix(filepath.Base(doc.path), ".md"))
		if currentDoc.links[otherTitle] {
			mutualLinks++
		}
		if doc.links[currentTitle] {
			mutualLinks++
		}
		linkBonus := float64(mutualLinks) * 0.15

		totalScore := contentSim + tagBonus + linkBonus
		if totalScore < 0.02 {
			continue
		}
		// Cap at 1.0 for display.
		if totalScore > 1.0 {
			totalScore = 1.0
		}

		// Build reason string.
		var reasons []string
		if contentSim > 0.01 {
			// Find shared terms.
			shared := advFindSharedTerms(currentDoc.tf, doc.tf, idf, 3)
			if len(shared) > 0 {
				reasons = append(reasons, "shared: "+strings.Join(shared, ", "))
			}
		}
		if sharedTags > 0 {
			sort.Strings(tagNames)
			reasons = append(reasons, fmt.Sprintf("tags: %s", strings.Join(tagNames, ", ")))
		}
		if mutualLinks > 0 {
			reasons = append(reasons, "mutual links")
		}

		reason := strings.Join(reasons, " | ")
		if reason == "" {
			reason = "similar content"
		}

		scores = append(scores, scored{idx: i, score: totalScore, reason: reason})
	}

	// Sort by score descending.
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	// Limit to top 20.
	if len(scores) > 20 {
		scores = scores[:20]
	}

	var result []SmartConnectionDTO
	for _, s := range scores {
		doc := docs[s.idx]
		result = append(result, SmartConnectionDTO{
			RelPath: doc.path,
			Title:   doc.title,
			Score:   math.Round(s.score*100) / 100,
			Reason:  s.reason,
		})
	}
	return result, nil
}

// advFindSharedTerms returns top N shared terms ranked by combined TF-IDF weight.
func advFindSharedTerms(tfA, tfB, idf map[string]float64, n int) []string {
	type tw struct {
		term   string
		weight float64
	}
	var shared []tw
	for term, tfa := range tfA {
		if tfb, ok := tfB[term]; ok {
			w := (tfa + tfb) * idf[term]
			shared = append(shared, tw{term: term, weight: w})
		}
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i].weight > shared[j].weight })
	if len(shared) > n {
		shared = shared[:n]
	}
	result := make([]string, len(shared))
	for i, s := range shared {
		result[i] = s.term
	}
	return result
}
