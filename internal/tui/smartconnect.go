package tui

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Markdown punctuation set for token cleaning
// ---------------------------------------------------------------------------

var scMarkdownPunct = map[rune]bool{
	'#': true, '*': true, '-': true, '>': true,
	'[': true, ']': true, '(': true, ')': true,
	'`': true, '~': true, '|': true, '!': true,
	'{': true, '}': true, '_': true, '=': true,
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type noteScore struct {
	Path        string
	Name        string
	Score       float64
	SharedTerms []string // top shared keywords
}

// SmartConnections finds semantically related notes using TF-IDF similarity.
type SmartConnections struct {
	active      bool
	width       int
	height      int
	vaultRoot   string
	currentNote string

	connections []noteScore
	cursor      int
	scroll      int

	// Preview
	previewMode   bool
	previewText   string
	previewScroll int

	scanning  bool
	statusMsg string

	// Consumed-once: path of note user wants to insert as wikilink
	insertLink string
	wantInsert bool
}

func NewSmartConnections() SmartConnections {
	return SmartConnections{}
}

func (sc SmartConnections) IsActive() bool {
	return sc.active
}

func (sc *SmartConnections) SetSize(w, h int) {
	sc.width = w
	sc.height = h
}

// GetSelectedNote returns the path of the selected note and whether the user
// requested a wikilink insertion. The value is consumed once.
func (sc *SmartConnections) GetSelectedNote() (string, bool) {
	if !sc.wantInsert {
		return "", false
	}
	path := sc.insertLink
	sc.insertLink = ""
	sc.wantInsert = false
	return path, true
}

// ---------------------------------------------------------------------------
// Text processing helpers
// ---------------------------------------------------------------------------

// scExtractWords tokenizes text: lowercases, strips punctuation and markdown
// syntax, filters stopwords and words shorter than 3 characters.
func scExtractWords(text string) []string {
	var words []string
	for _, raw := range strings.Fields(text) {
		// Strip markdown punctuation from both ends and interior
		cleaned := strings.Map(func(r rune) rune {
			if scMarkdownPunct[r] || unicode.IsPunct(r) || unicode.IsSymbol(r) {
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
		// Skip pure numbers
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

// scTermFrequency returns a map of word -> frequency for the given word list.
func scTermFrequency(words []string) map[string]float64 {
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

// ---------------------------------------------------------------------------
// TF-IDF computation
// ---------------------------------------------------------------------------

type scDocVec struct {
	path  string
	name  string
	tf    map[string]float64
	words map[string]bool // unique words for IDF counting
}

func (sc *SmartConnections) computeConnections() {
	sc.connections = nil
	sc.statusMsg = "Scanning vault..."
	sc.scanning = true

	// 1. Collect all documents
	var docs []scDocVec
	// Track document frequency: how many documents contain each term
	docFreq := make(map[string]int)

	_ = filepath.Walk(sc.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories and .granit-trash
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		// Only .md files
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		// Skip files > 100KB
		if info.Size() > 100*1024 {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		relPath, _ := filepath.Rel(sc.vaultRoot, path)
		words := scExtractWords(string(data))
		tf := scTermFrequency(words)
		unique := make(map[string]bool, len(tf))
		for w := range tf {
			unique[w] = true
			docFreq[w]++
		}

		docs = append(docs, scDocVec{
			path:  relPath,
			name:  name,
			tf:    tf,
			words: unique,
		})
		return nil
	})

	if len(docs) < 2 {
		sc.statusMsg = "Not enough notes to compare"
		sc.scanning = false
		return
	}

	// 2. Compute IDF for all terms
	numDocs := float64(len(docs))
	idf := make(map[string]float64, len(docFreq))
	for term, df := range docFreq {
		idf[term] = math.Log(numDocs / (1.0 + float64(df)))
	}

	// 3. Find the current note's document index
	currentIdx := -1
	for i, doc := range docs {
		if doc.path == sc.currentNote {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		sc.statusMsg = "Current note not found in vault"
		sc.scanning = false
		return
	}

	currentDoc := docs[currentIdx]

	// 4. Build TF-IDF vector for current document
	currentVec := make(map[string]float64, len(currentDoc.tf))
	for term, tf := range currentDoc.tf {
		currentVec[term] = tf * idf[term]
	}

	// Check that the current note has meaningful content
	hasContent := false
	for _, v := range currentVec {
		if v > 0 {
			hasContent = true
			break
		}
	}
	if !hasContent {
		sc.statusMsg = "Current note has no meaningful content"
		sc.scanning = false
		return
	}

	// 5. Compare with all other documents using cosine similarity
	type scored struct {
		idx   int
		score float64
	}
	var scores []scored

	for i, doc := range docs {
		if i == currentIdx {
			continue
		}

		// Build TF-IDF vector for comparison document
		otherVec := make(map[string]float64, len(doc.tf))
		for term, tf := range doc.tf {
			otherVec[term] = tf * idf[term]
		}

		sim := cosineSimilarity(currentVec, otherVec)
		if sim > 0.01 { // threshold: ignore near-zero similarity
			scores = append(scores, scored{idx: i, score: sim})
		}
	}

	// 6. Sort by score descending
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[i].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// 7. Limit to top 20
	if len(scores) > 20 {
		scores = scores[:20]
	}

	// 8. Build result list with shared terms
	for _, s := range scores {
		doc := docs[s.idx]
		shared := scFindSharedTerms(currentDoc.tf, doc.tf, idf, 5)
		sc.connections = append(sc.connections, noteScore{
			Path:        doc.path,
			Name:        doc.name,
			Score:       s.score,
			SharedTerms: shared,
		})
	}

	sc.scanning = false
	count := len(sc.connections)
	if count == 0 {
		sc.statusMsg = "No related notes found"
	} else {
		sc.statusMsg = smallNum(count) + " connections found"
	}
}

// scFindSharedTerms returns the top-N shared terms between two documents,
// ranked by combined TF-IDF weight.
func scFindSharedTerms(tfA, tfB map[string]float64, idf map[string]float64, n int) []string {
	type termWeight struct {
		term   string
		weight float64
	}
	var shared []termWeight
	for term, tfa := range tfA {
		if tfb, ok := tfB[term]; ok {
			w := (tfa + tfb) * idf[term]
			shared = append(shared, termWeight{term: term, weight: w})
		}
	}
	// Sort by weight descending
	for i := 0; i < len(shared); i++ {
		for j := i + 1; j < len(shared); j++ {
			if shared[j].weight > shared[i].weight {
				shared[i], shared[j] = shared[j], shared[i]
			}
		}
	}
	if len(shared) > n {
		shared = shared[:n]
	}
	result := make([]string, len(shared))
	for i, tw := range shared {
		result[i] = tw.term
	}
	return result
}

// ---------------------------------------------------------------------------
// OpenForNote triggers the scan and populates connections.
// ---------------------------------------------------------------------------

func (sc *SmartConnections) OpenForNote(vaultRoot, notePath, noteContent string) {
	sc.active = true
	sc.vaultRoot = vaultRoot
	sc.currentNote = notePath
	sc.cursor = 0
	sc.scroll = 0
	sc.previewMode = false
	sc.previewText = ""
	sc.previewScroll = 0
	sc.insertLink = ""
	sc.wantInsert = false
	sc.connections = nil

	sc.computeConnections()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (sc SmartConnections) Update(msg tea.Msg) (SmartConnections, tea.Cmd) {
	if !sc.active {
		return sc, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if sc.previewMode {
			return sc.updatePreview(msg)
		}
		return sc.updateList(msg)
	}
	return sc, nil
}

func (sc SmartConnections) updateList(msg tea.KeyMsg) (SmartConnections, tea.Cmd) {
	switch msg.String() {
	case "esc":
		sc.active = false
	case "up", "k":
		if sc.cursor > 0 {
			sc.cursor--
			if sc.cursor < sc.scroll {
				sc.scroll = sc.cursor
			}
		}
	case "down", "j":
		if sc.cursor < len(sc.connections)-1 {
			sc.cursor++
			visH := sc.visibleHeight()
			if sc.cursor >= sc.scroll+visH {
				sc.scroll = sc.cursor - visH + 1
			}
		}
	case "enter":
		if len(sc.connections) > 0 && sc.cursor < len(sc.connections) {
			// Load preview
			conn := sc.connections[sc.cursor]
			fullPath := filepath.Join(sc.vaultRoot, conn.Path)
			data, err := os.ReadFile(fullPath)
			if err != nil {
				sc.previewText = "Error reading file: " + err.Error()
			} else {
				sc.previewText = string(data)
			}
			sc.previewMode = true
			sc.previewScroll = 0
		}
	case "l":
		if len(sc.connections) > 0 && sc.cursor < len(sc.connections) {
			sc.insertLink = sc.connections[sc.cursor].Path
			sc.wantInsert = true
			sc.active = false
		}
	}
	return sc, nil
}

func (sc SmartConnections) updatePreview(msg tea.KeyMsg) (SmartConnections, tea.Cmd) {
	switch msg.String() {
	case "esc":
		sc.previewMode = false
		sc.previewText = ""
		sc.previewScroll = 0
	case "up", "k":
		if sc.previewScroll > 0 {
			sc.previewScroll--
		}
	case "down", "j":
		lines := strings.Split(sc.previewText, "\n")
		maxScroll := len(lines) - sc.visibleHeight()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if sc.previewScroll < maxScroll {
			sc.previewScroll++
		}
	}
	return sc, nil
}

func (sc SmartConnections) visibleHeight() int {
	h := sc.height - 14
	if h < 5 {
		h = 5
	}
	return h
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (sc SmartConnections) View() string {
	width := sc.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerWidth := width - 6

	var b strings.Builder

	if sc.previewMode {
		sc.renderPreview(&b, innerWidth)
	} else {
		sc.renderList(&b, innerWidth)
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (sc SmartConnections) renderList(b *strings.Builder, innerWidth int) {
	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	noteName := strings.TrimSuffix(sc.currentNote, ".md")
	noteName = TruncateDisplay(noteName, innerWidth-30)
	b.WriteString(titleStyle.Render(IconGraphChar + " Smart Connections"))
	b.WriteString("\n")
	noteLabel := lipgloss.NewStyle().Foreground(overlay0).Render("  for: ")
	noteNameStyled := lipgloss.NewStyle().Foreground(blue).Render(noteName)
	b.WriteString(noteLabel + noteNameStyled)
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	if sc.scanning {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  Scanning vault..."))
		b.WriteString("\n")
		return
	}

	if len(sc.connections) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + sc.statusMsg))
		b.WriteString("\n")
	} else {
		// Status line
		statusStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(statusStyle.Render("  " + sc.statusMsg))
		b.WriteString("\n\n")

		visH := sc.visibleHeight()
		end := sc.scroll + visH
		if end > len(sc.connections) {
			end = len(sc.connections)
		}

		scoreStyle := lipgloss.NewStyle().Bold(true)
		sharedStyle := lipgloss.NewStyle().Foreground(overlay0)

		for i := sc.scroll; i < end; i++ {
			conn := sc.connections[i]

			// Score as percentage
			pct := int(conn.Score * 100)
			if pct > 99 {
				pct = 99
			}

			// Color the score based on similarity strength
			var scoreColor lipgloss.Color
			switch {
			case pct >= 70:
				scoreColor = green
			case pct >= 40:
				scoreColor = yellow
			default:
				scoreColor = peach
			}

			scoreStr := scoreStyle.Foreground(scoreColor).Render(scPadLeft(smallNum(pct), 3) + "%")

			// Note name
			displayName := strings.TrimSuffix(conn.Name, ".md")
			maxNameLen := innerWidth - 12
			if maxNameLen < 10 {
				maxNameLen = 10
			}
			if len(displayName) > maxNameLen {
				displayName = displayName[:maxNameLen-3] + "..."
			}

			if i == sc.cursor {
				line := "  " + scoreStr + "  " + displayName
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(innerWidth).
					Render(line))
			} else {
				nameStyled := NormalItemStyle.Render(displayName)
				b.WriteString("  " + scoreStr + "  " + nameStyled)
			}
			b.WriteString("\n")

			// Shared terms line
			if len(conn.SharedTerms) > 0 {
				terms := strings.Join(conn.SharedTerms, ", ")
				maxTermLen := innerWidth - 18
				if maxTermLen < 10 {
					maxTermLen = 10
				}
				if len(terms) > maxTermLen {
					terms = terms[:maxTermLen-3] + "..."
				}
				b.WriteString("       " + sharedStyle.Render("shared: "+terms))
				b.WriteString("\n")
			}

			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"j/k", "nav"}, {"Enter", "preview"}, {"l", "insert link"}, {"Esc", "close"},
	}))
}

func (sc SmartConnections) renderPreview(b *strings.Builder, innerWidth int) {
	// Preview header
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if sc.cursor < len(sc.connections) {
		conn := sc.connections[sc.cursor]
		displayName := strings.TrimSuffix(conn.Name, ".md")
		b.WriteString(titleStyle.Render(IconFileChar + " Preview: " + displayName))
	} else {
		b.WriteString(titleStyle.Render(IconFileChar + " Preview"))
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	lines := strings.Split(sc.previewText, "\n")
	visH := sc.visibleHeight()

	end := sc.previewScroll + visH
	if end > len(lines) {
		end = len(lines)
	}
	start := sc.previewScroll
	if start > len(lines) {
		start = len(lines)
	}

	contentStyle := lipgloss.NewStyle().Foreground(text)
	lineNumStyle := lipgloss.NewStyle().Foreground(surface2).Width(4).Align(lipgloss.Right)

	for i := start; i < end; i++ {
		line := lines[i]
		// Truncate long lines
		line = TruncateDisplay(line, innerWidth-6)
		num := lineNumStyle.Render(smallNum(i + 1))
		b.WriteString(num + " " + contentStyle.Render(line))
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scrollbar indicator
	if len(lines) > visH {
		b.WriteString("\n")
		pos := smallNum(sc.previewScroll+1) + "-" + smallNum(end) + "/" + smallNum(len(lines))
		b.WriteString(DimStyle.Render("  " + pos))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"j/k", "scroll"}, {"Esc", "back"},
	}))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// scPadLeft pads a string with spaces on the left to reach the desired width.
func scPadLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}
