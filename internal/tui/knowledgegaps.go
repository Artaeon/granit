package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// GapFinding represents a single knowledge gap finding.
// ---------------------------------------------------------------------------

// GapFinding holds one analysis result from the knowledge gaps scanner.
type GapFinding struct {
	Type        string   // "topic", "stale", "missing_link", "orphan", "structure"
	Severity    int      // 0=info, 1=low, 2=medium, 3=high
	Title       string
	Description string
	NotePath    string   // primary note involved
	NotePath2   string   // secondary note (for missing links)
	Keywords    []string
}

// ---------------------------------------------------------------------------
// KnowledgeGaps overlay
// ---------------------------------------------------------------------------

// KnowledgeGaps is a TUI overlay that analyses the vault and identifies
// gaps in knowledge, underdeveloped topics, stale notes, unlinked clusters,
// and missing connections.
type KnowledgeGaps struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	tab    int // 0=Topics, 1=Stale, 2=Missing Links, 3=Orphans, 4=Structure
	cursor int
	scroll int

	// Per-tab findings
	topicFindings      []GapFinding
	staleFindings      []GapFinding
	missingLinkFindings []GapFinding
	orphanFindings     []GapFinding
	structureFindings  []GapFinding

	// Selected note to jump to (consumed once)
	selectedNote string
	wantJump     bool
}

// NewKnowledgeGaps creates a new KnowledgeGaps overlay.
func NewKnowledgeGaps() KnowledgeGaps {
	return KnowledgeGaps{}
}

// IsActive returns whether the overlay is visible.
func (kg KnowledgeGaps) IsActive() bool {
	return kg.active
}

// SetSize updates the available terminal dimensions.
func (kg *KnowledgeGaps) SetSize(w, h int) {
	kg.width = w
	kg.height = h
}

// GetSelectedNote returns the path of the note the user wants to jump to.
// The value is consumed once.
func (kg *KnowledgeGaps) GetSelectedNote() (string, bool) {
	if !kg.wantJump {
		return "", false
	}
	path := kg.selectedNote
	kg.selectedNote = ""
	kg.wantJump = false
	return path, true
}

// Open triggers the analysis and shows the overlay.
func (kg *KnowledgeGaps) Open(vaultRoot string) {
	kg.active = true
	kg.vaultRoot = vaultRoot
	kg.tab = 0
	kg.cursor = 0
	kg.scroll = 0
	kg.selectedNote = ""
	kg.wantJump = false
	kg.analyze()
}

// ---------------------------------------------------------------------------
// Internal note info gathered during the scan
// ---------------------------------------------------------------------------

type kgNoteInfo struct {
	relPath   string
	name      string
	content   string
	words     int
	modTime   time.Time
	tags      []string
	headings  []string
	links     []string    // outgoing wikilinks
	backlinks []string    // incoming links
	folder    string
	hasTodo   bool
}

// ---------------------------------------------------------------------------
// Analysis
// ---------------------------------------------------------------------------

func (kg *KnowledgeGaps) analyze() {
	kg.topicFindings = nil
	kg.staleFindings = nil
	kg.missingLinkFindings = nil
	kg.orphanFindings = nil
	kg.structureFindings = nil

	// 1. Gather all notes
	notes := kg.gatherNotes()
	if len(notes) < 2 {
		return
	}

	// 2. Build TF-IDF index for similarity analysis
	noteContents := make(map[string]string, len(notes))
	for _, n := range notes {
		noteContents[n.relPath] = n.content
	}
	tfidf := BuildTFIDF(noteContents)

	// 3. Run each analysis
	kg.analyzeTopics(notes, tfidf)
	kg.analyzeStale(notes)
	kg.analyzeMissingLinks(notes, tfidf)
	kg.analyzeOrphans(notes)
	kg.analyzeStructure(notes)
}

func (kg *KnowledgeGaps) gatherNotes() []kgNoteInfo {
	var notes []kgNoteInfo

	_ = filepath.Walk(kg.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		// Skip very large files
		if info.Size() > 200*1024 {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		relPath, _ := filepath.Rel(kg.vaultRoot, path)
		content := string(data)
		words := strings.Fields(content)

		n := kgNoteInfo{
			relPath: relPath,
			name:    strings.TrimSuffix(name, ".md"),
			content: content,
			words:   len(words),
			modTime: info.ModTime(),
			folder:  filepath.Dir(relPath),
		}

		// Extract tags from frontmatter and inline #tags
		n.tags = kgExtractTags(content)

		// Extract headings
		n.headings = kgExtractHeadings(content)

		// Extract outgoing wikilinks
		n.links = kgExtractWikilinks(content)

		// Check for TODO/FIXME markers
		lowerContent := strings.ToLower(content)
		n.hasTodo = strings.Contains(lowerContent, "todo") ||
			strings.Contains(lowerContent, "fixme") ||
			strings.Contains(lowerContent, "- [ ]")

		notes = append(notes, n)
		return nil
	})

	// Build backlinks map
	backlinkMap := make(map[string][]string)
	for _, n := range notes {
		for _, link := range n.links {
			target := kgResolveLink(link, notes)
			if target != "" {
				backlinkMap[target] = append(backlinkMap[target], n.relPath)
			}
		}
	}
	for i := range notes {
		notes[i].backlinks = backlinkMap[notes[i].relPath]
	}

	return notes
}

// kgResolveLink tries to match a wikilink name to a note's relPath.
func kgResolveLink(link string, notes []kgNoteInfo) string {
	if !strings.HasSuffix(link, ".md") {
		link = link + ".md"
	}
	// Direct match
	for _, n := range notes {
		if n.relPath == link {
			return n.relPath
		}
	}
	// Basename match
	baseName := filepath.Base(link)
	for _, n := range notes {
		if filepath.Base(n.relPath) == baseName {
			return n.relPath
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// A. Topic Coverage Analysis
// ---------------------------------------------------------------------------

func (kg *KnowledgeGaps) analyzeTopics(notes []kgNoteInfo, tfidf *TFIDFIndex) {
	// 1. Count tag usage across the vault
	tagCounts := make(map[string]int)
	tagNotes := make(map[string][]string) // tag -> list of note names
	for _, n := range notes {
		for _, tag := range n.tags {
			tagCounts[tag]++
			tagNotes[tag] = append(tagNotes[tag], n.name)
		}
	}

	// Find orphan tags (used only once)
	for tag, count := range tagCounts {
		if count == 1 {
			noteName := ""
			if len(tagNotes[tag]) > 0 {
				noteName = tagNotes[tag][0]
			}
			kg.topicFindings = append(kg.topicFindings, GapFinding{
				Type:        "topic",
				Severity:    1,
				Title:       fmt.Sprintf("Orphan tag: #%s", tag),
				Description: fmt.Sprintf("Appears in only 1 note (%s). Consider expanding this topic.", noteName),
				NotePath:    kgFindPathByName(notes, noteName),
				Keywords:    []string{tag},
			})
		}
	}

	// 2. Cluster notes by top TF-IDF terms to detect topic imbalances
	// Get top 3 terms per note as its "topics"
	topicClusters := make(map[string][]string) // term -> list of note paths
	for _, n := range notes {
		if vec, ok := tfidf.TermFreqs[n.relPath]; ok {
			topTerms := kgTopTerms(vec, 3)
			for _, term := range topTerms {
				topicClusters[term] = append(topicClusters[term], n.relPath)
			}
		}
	}

	// Find terms that appear as a top-term in many notes (well-covered) vs few
	type topicSize struct {
		term  string
		count int
	}
	var topics []topicSize
	for term, paths := range topicClusters {
		if len(paths) >= 1 {
			topics = append(topics, topicSize{term: term, count: len(paths)})
		}
	}
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].count > topics[j].count
	})

	// Report underdeveloped topics (1-2 notes) compared to well-covered ones
	if len(topics) > 0 {
		maxCount := topics[0].count
		for _, t := range topics {
			if t.count <= 2 && maxCount >= 5 {
				kg.topicFindings = append(kg.topicFindings, GapFinding{
					Type:     "topic",
					Severity: 2,
					Title:    fmt.Sprintf("Underdeveloped topic: %q", t.term),
					Description: fmt.Sprintf(
						"Only %d note(s) cover this term, while your most popular topic has %d notes.",
						t.count, maxCount),
					NotePath: topicClusters[t.term][0],
					Keywords: []string{t.term},
				})
			}
		}
	}

	// Limit topic findings to top 20
	if len(kg.topicFindings) > 20 {
		kg.topicFindings = kg.topicFindings[:20]
	}

	// Sort by severity descending
	sort.Slice(kg.topicFindings, func(i, j int) bool {
		return kg.topicFindings[i].Severity > kg.topicFindings[j].Severity
	})
}

// ---------------------------------------------------------------------------
// B. Stale Knowledge Detection
// ---------------------------------------------------------------------------

func (kg *KnowledgeGaps) analyzeStale(notes []kgNoteInfo) {
	now := time.Now()
	staleThreshold := 90 * 24 * time.Hour // 90 days

	for _, n := range notes {
		age := now.Sub(n.modTime)
		if age < staleThreshold {
			continue
		}

		daysOld := int(age.Hours() / 24)

		// Important but stale: has incoming links
		if len(n.backlinks) >= 2 {
			kg.staleFindings = append(kg.staleFindings, GapFinding{
				Type:     "stale",
				Severity: 3,
				Title:    fmt.Sprintf("Stale hub: %s", n.name),
				Description: fmt.Sprintf(
					"Last modified %d days ago, but has %d incoming links. May need updating.",
					daysOld, len(n.backlinks)),
				NotePath: n.relPath,
			})
			continue
		}

		// Has unresolved TODOs
		if n.hasTodo {
			kg.staleFindings = append(kg.staleFindings, GapFinding{
				Type:     "stale",
				Severity: 2,
				Title:    fmt.Sprintf("Stale TODO: %s", n.name),
				Description: fmt.Sprintf(
					"Last modified %d days ago, still contains TODO/FIXME markers.",
					daysOld),
				NotePath: n.relPath,
			})
			continue
		}

		// Generally stale (only report for notes older than 180 days)
		if age > 180*24*time.Hour {
			kg.staleFindings = append(kg.staleFindings, GapFinding{
				Type:     "stale",
				Severity: 1,
				Title:    fmt.Sprintf("Very old note: %s", n.name),
				Description: fmt.Sprintf(
					"Last modified %d days ago. Consider reviewing or archiving.",
					daysOld),
				NotePath: n.relPath,
			})
		}
	}

	// Sort by severity descending, then by title
	sort.Slice(kg.staleFindings, func(i, j int) bool {
		if kg.staleFindings[i].Severity != kg.staleFindings[j].Severity {
			return kg.staleFindings[i].Severity > kg.staleFindings[j].Severity
		}
		return kg.staleFindings[i].Title < kg.staleFindings[j].Title
	})

	// Limit to top 30
	if len(kg.staleFindings) > 30 {
		kg.staleFindings = kg.staleFindings[:30]
	}
}

// ---------------------------------------------------------------------------
// C. Missing Links Prediction
// ---------------------------------------------------------------------------

func (kg *KnowledgeGaps) analyzeMissingLinks(notes []kgNoteInfo, tfidf *TFIDFIndex) {
	// Build a set of existing links for fast lookup
	linkedPairs := make(map[string]bool)
	for _, n := range notes {
		for _, link := range n.links {
			target := kgResolveLink(link, notes)
			if target != "" {
				// Store both directions to avoid duplication
				pair := kgPairKey(n.relPath, target)
				linkedPairs[pair] = true
			}
		}
	}

	// Compare all pairs using TF-IDF cosine similarity
	type missingLink struct {
		pathA, pathB string
		nameA, nameB string
		score        float64
		sharedTerms  []string
	}
	var candidates []missingLink

	for i := 0; i < len(notes); i++ {
		vecA, okA := tfidf.TermFreqs[notes[i].relPath]
		if !okA {
			continue
		}
		for j := i + 1; j < len(notes); j++ {
			vecB, okB := tfidf.TermFreqs[notes[j].relPath]
			if !okB {
				continue
			}

			pair := kgPairKey(notes[i].relPath, notes[j].relPath)
			if linkedPairs[pair] {
				continue // already linked
			}

			sim := cosineSimilarity(vecA, vecB)
			if sim < 0.3 {
				continue // not similar enough
			}

			shared := commonImportantTerms(vecA, vecB)
			if len(shared) > 5 {
				shared = shared[:5]
			}

			candidates = append(candidates, missingLink{
				pathA:       notes[i].relPath,
				pathB:       notes[j].relPath,
				nameA:       notes[i].name,
				nameB:       notes[j].name,
				score:       sim,
				sharedTerms: shared,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Limit to top 20
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}

	for _, c := range candidates {
		severity := 1
		if c.score >= 0.6 {
			severity = 3
		} else if c.score >= 0.4 {
			severity = 2
		}

		kg.missingLinkFindings = append(kg.missingLinkFindings, GapFinding{
			Type:     "missing_link",
			Severity: severity,
			Title:    fmt.Sprintf("%s <-> %s", c.nameA, c.nameB),
			Description: fmt.Sprintf(
				"%.0f%% similar but not linked. Shared: %s",
				c.score*100, strings.Join(c.sharedTerms, ", ")),
			NotePath:  c.pathA,
			NotePath2: c.pathB,
			Keywords:  c.sharedTerms,
		})
	}
}

// ---------------------------------------------------------------------------
// D. Orphan Detection
// ---------------------------------------------------------------------------

func (kg *KnowledgeGaps) analyzeOrphans(notes []kgNoteInfo) {
	for _, n := range notes {
		isOrphan := len(n.links) == 0 && len(n.backlinks) == 0

		if isOrphan {
			severity := 2
			if n.words < 100 {
				severity = 3 // short + orphan = likely a stub
			}

			kg.orphanFindings = append(kg.orphanFindings, GapFinding{
				Type:     "orphan",
				Severity: severity,
				Title:    n.name,
				Description: fmt.Sprintf(
					"No incoming or outgoing links. %d words. Consider linking to related notes.",
					n.words),
				NotePath: n.relPath,
			})
		}
	}

	// Sort by severity descending
	sort.Slice(kg.orphanFindings, func(i, j int) bool {
		if kg.orphanFindings[i].Severity != kg.orphanFindings[j].Severity {
			return kg.orphanFindings[i].Severity > kg.orphanFindings[j].Severity
		}
		return kg.orphanFindings[i].Title < kg.orphanFindings[j].Title
	})

	// Limit to 30
	if len(kg.orphanFindings) > 30 {
		kg.orphanFindings = kg.orphanFindings[:30]
	}
}

// ---------------------------------------------------------------------------
// E. Structure Suggestions
// ---------------------------------------------------------------------------

func (kg *KnowledgeGaps) analyzeStructure(notes []kgNoteInfo) {
	// 1. Folders with too many notes
	folderCounts := make(map[string]int)
	for _, n := range notes {
		folderCounts[n.folder]++
	}

	for folder, count := range folderCounts {
		if count > 20 {
			displayFolder := folder
			if displayFolder == "." {
				displayFolder = "vault root"
			}
			kg.structureFindings = append(kg.structureFindings, GapFinding{
				Type:     "structure",
				Severity: 2,
				Title:    fmt.Sprintf("Large folder: %s (%d notes)", displayFolder, count),
				Description: fmt.Sprintf(
					"Has %d notes. Consider splitting into sub-folders for better organization.",
					count),
			})
		}
	}

	// 2. Notes that are too long
	for _, n := range notes {
		if n.words > 3000 {
			kg.structureFindings = append(kg.structureFindings, GapFinding{
				Type:     "structure",
				Severity: 2,
				Title:    fmt.Sprintf("Long note: %s (%d words)", n.name, n.words),
				Description: "Consider splitting into smaller, more focused notes.",
				NotePath: n.relPath,
			})
		}
	}

	// 3. Related notes in different folders
	// Find pairs of notes in different folders that share 3+ tags
	for i := 0; i < len(notes); i++ {
		if len(notes[i].tags) < 2 {
			continue
		}
		for j := i + 1; j < len(notes); j++ {
			if notes[i].folder == notes[j].folder {
				continue
			}
			if len(notes[j].tags) < 2 {
				continue
			}
			shared := kgSharedTags(notes[i].tags, notes[j].tags)
			if len(shared) >= 3 {
				kg.structureFindings = append(kg.structureFindings, GapFinding{
					Type:     "structure",
					Severity: 1,
					Title:    fmt.Sprintf("Related notes in different folders"),
					Description: fmt.Sprintf(
						"%s (in %s) and %s (in %s) share %d tags: %s",
						notes[i].name, notes[i].folder,
						notes[j].name, notes[j].folder,
						len(shared), strings.Join(shared, ", ")),
					NotePath:  notes[i].relPath,
					NotePath2: notes[j].relPath,
					Keywords:  shared,
				})
			}
		}
	}

	// Sort by severity descending
	sort.Slice(kg.structureFindings, func(i, j int) bool {
		if kg.structureFindings[i].Severity != kg.structureFindings[j].Severity {
			return kg.structureFindings[i].Severity > kg.structureFindings[j].Severity
		}
		return kg.structureFindings[i].Title < kg.structureFindings[j].Title
	})

	// Limit to 20
	if len(kg.structureFindings) > 20 {
		kg.structureFindings = kg.structureFindings[:20]
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key messages while the overlay is active.
func (kg KnowledgeGaps) Update(msg tea.Msg) (KnowledgeGaps, tea.Cmd) {
	if !kg.active {
		return kg, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			kg.active = false
		case "tab":
			kg.tab = (kg.tab + 1) % 5
			kg.cursor = 0
			kg.scroll = 0
		case "shift+tab":
			kg.tab = (kg.tab + 4) % 5
			kg.cursor = 0
			kg.scroll = 0
		case "1":
			kg.tab = 0
			kg.cursor = 0
			kg.scroll = 0
		case "2":
			kg.tab = 1
			kg.cursor = 0
			kg.scroll = 0
		case "3":
			kg.tab = 2
			kg.cursor = 0
			kg.scroll = 0
		case "4":
			kg.tab = 3
			kg.cursor = 0
			kg.scroll = 0
		case "5":
			kg.tab = 4
			kg.cursor = 0
			kg.scroll = 0
		case "j", "down":
			findings := kg.currentFindings()
			if kg.cursor < len(findings)-1 {
				kg.cursor++
				visH := kg.visibleHeight()
				if kg.cursor >= kg.scroll+visH {
					kg.scroll = kg.cursor - visH + 1
				}
			}
		case "k", "up":
			if kg.cursor > 0 {
				kg.cursor--
				if kg.cursor < kg.scroll {
					kg.scroll = kg.cursor
				}
			}
		case "enter":
			findings := kg.currentFindings()
			if len(findings) > 0 && kg.cursor < len(findings) {
				f := findings[kg.cursor]
				if f.NotePath != "" {
					kg.selectedNote = f.NotePath
					kg.wantJump = true
					kg.active = false
				}
			}
		case "r":
			kg.analyze()
			kg.cursor = 0
			kg.scroll = 0
		}
	}
	return kg, nil
}

// currentFindings returns the findings for the active tab.
func (kg *KnowledgeGaps) currentFindings() []GapFinding {
	switch kg.tab {
	case 0:
		return kg.topicFindings
	case 1:
		return kg.staleFindings
	case 2:
		return kg.missingLinkFindings
	case 3:
		return kg.orphanFindings
	case 4:
		return kg.structureFindings
	}
	return nil
}

func (kg KnowledgeGaps) visibleHeight() int {
	// Each finding takes 2 lines (title + description) plus spacing
	h := (kg.height - 14) / 3
	if h < 5 {
		h = 5
	}
	return h
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the overlay.
func (kg KnowledgeGaps) View() string {
	width := kg.width * 3 / 4
	if width < 70 {
		width = 70
	}
	if width > 110 {
		width = 110
	}
	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconGraphChar + " AI Knowledge Gaps Analysis"))
	b.WriteString("\n")

	// Summary line
	total := len(kg.topicFindings) + len(kg.staleFindings) +
		len(kg.missingLinkFindings) + len(kg.orphanFindings) + len(kg.structureFindings)
	summaryStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(summaryStyle.Render(fmt.Sprintf("  %d findings across your vault", total)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Tab bar
	activeTabStyle := lipgloss.NewStyle().
		Foreground(crust).
		Background(mauve).
		Bold(true).
		Padding(0, 1)

	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(overlay0).
		Background(surface0).
		Padding(0, 1)

	tabLabels := []struct {
		name  string
		count int
	}{
		{"Topics", len(kg.topicFindings)},
		{"Stale", len(kg.staleFindings)},
		{"Missing Links", len(kg.missingLinkFindings)},
		{"Orphans", len(kg.orphanFindings)},
		{"Structure", len(kg.structureFindings)},
	}

	for i, tab := range tabLabels {
		label := fmt.Sprintf(" %s (%d) ", tab.name, tab.count)
		if i == kg.tab {
			b.WriteString(activeTabStyle.Render(label))
		} else {
			b.WriteString(inactiveTabStyle.Render(label))
		}
		if i < len(tabLabels)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Content area
	findings := kg.currentFindings()

	if len(findings) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No findings in this category."))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Your vault looks good here!"))
		b.WriteString("\n")
	} else {
		visH := kg.visibleHeight()
		end := kg.scroll + visH
		if end > len(findings) {
			end = len(findings)
		}

		for i := kg.scroll; i < end; i++ {
			f := findings[i]
			b.WriteString("\n")

			// Severity icon
			severityIcon := kg.severityIcon(f.Severity)

			// Title line
			if i == kg.cursor {
				accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
				nameStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
				title := f.Title
				maxTitleLen := innerWidth - 8
				if len(title) > maxTitleLen {
					title = title[:maxTitleLen-3] + "..."
				}
				b.WriteString(accentBar + " " + severityIcon + " " + nameStyle.Render(title))
			} else {
				nameStyle := lipgloss.NewStyle().Foreground(text)
				title := f.Title
				maxTitleLen := innerWidth - 8
				if len(title) > maxTitleLen {
					title = title[:maxTitleLen-3] + "..."
				}
				b.WriteString("  " + severityIcon + " " + nameStyle.Render(title))
			}
			b.WriteString("\n")

			// Description line
			descStyle := lipgloss.NewStyle().Foreground(overlay0)
			desc := f.Description
			maxDescLen := innerWidth - 6
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}
			b.WriteString("     " + descStyle.Render(desc))
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(findings) > visH {
			b.WriteString("\n")
			pos := fmt.Sprintf("  %d/%d", kg.cursor+1, len(findings))
			b.WriteString(DimStyle.Render(pos))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Tab", "switch"}, {"Enter", "open"}, {"r", "refresh"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// severityIcon returns a colored icon for the given severity level.
func (kg KnowledgeGaps) severityIcon(severity int) string {
	switch severity {
	case 3:
		return lipgloss.NewStyle().Foreground(red).Render("●")
	case 2:
		return lipgloss.NewStyle().Foreground(yellow).Render("●")
	case 1:
		return lipgloss.NewStyle().Foreground(green).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(overlay0).Render("○")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var kgTagRegex = regexp.MustCompile(`(?:^|\s)#([a-zA-Z][a-zA-Z0-9_-]*)`)
var kgFrontmatterTagRegex = regexp.MustCompile(`(?m)^tags:\s*\[([^\]]*)\]`)

var kgHeadingRegex = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
var kgWikilinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

// kgExtractTags extracts tags from both YAML frontmatter and inline #tags.
func kgExtractTags(content string) []string {
	tagSet := make(map[string]bool)

	// Inline #tags
	for _, match := range kgTagRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			tag := strings.ToLower(match[1])
			// Skip common markdown false positives
			if tag != "heading" && tag != "todo" && tag != "fixme" && len(tag) > 1 {
				tagSet[tag] = true
			}
		}
	}

	// Frontmatter tags: [tag1, tag2]
	for _, match := range kgFrontmatterTagRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			for _, tag := range strings.Split(match[1], ",") {
				tag = strings.TrimSpace(tag)
				tag = strings.Trim(tag, "\"'")
				tag = strings.ToLower(tag)
				if tag != "" {
					tagSet[tag] = true
				}
			}
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// kgExtractHeadings extracts markdown headings from content.
func kgExtractHeadings(content string) []string {
	var headings []string
	for _, match := range kgHeadingRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			headings = append(headings, strings.TrimSpace(match[1]))
		}
	}
	return headings
}

// kgExtractWikilinks extracts [[wikilink]] targets from content.
func kgExtractWikilinks(content string) []string {
	var links []string
	seen := make(map[string]bool)
	for _, match := range kgWikilinkRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			link := strings.TrimSpace(match[1])
			if !seen[link] {
				seen[link] = true
				links = append(links, link)
			}
		}
	}
	return links
}

// kgTopTerms returns the top N terms by TF-IDF weight from a vector.
func kgTopTerms(vec map[string]float64, n int) []string {
	type tw struct {
		term   string
		weight float64
	}
	var terms []tw
	for term, w := range vec {
		if w > 0 {
			terms = append(terms, tw{term, w})
		}
	}
	sort.Slice(terms, func(i, j int) bool {
		return terms[i].weight > terms[j].weight
	})
	if len(terms) > n {
		terms = terms[:n]
	}
	result := make([]string, len(terms))
	for i, t := range terms {
		result[i] = t.term
	}
	return result
}

// kgPairKey returns a canonical key for a pair of paths (order-independent).
func kgPairKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

// kgFindPathByName finds a note's relPath by its display name.
func kgFindPathByName(notes []kgNoteInfo, name string) string {
	for _, n := range notes {
		if n.name == name {
			return n.relPath
		}
	}
	return ""
}

// kgSharedTags returns tags shared between two tag lists.
func kgSharedTags(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, tag := range a {
		set[tag] = true
	}
	var shared []string
	for _, tag := range b {
		if set[tag] {
			shared = append(shared, tag)
		}
	}
	return shared
}
