package tui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// VocabEntry represents a single vocabulary word and its metadata.
type VocabEntry struct {
	Word         string
	Translation  string
	Language     string
	Level        int
	LastReviewed string
	Correct      int
}

// PracticeSession tracks a single practice attempt during a session.
type PracticeSession struct {
	Date    string
	Correct int
	Total   int
}

// LanguageLearning is the overlay for the language learning companion.
type LanguageLearning struct {
	OverlayBase

	vaultRoot string
	tab       int // 0=vocabulary, 1=practice, 2=grammar, 3=dashboard

	// Vocabulary tab
	vocab       []VocabEntry
	vocabCursor int
	vocabScroll int
	addingWord  bool
	addField    int // 0=word, 1=translation, 2=language
	addWord     string
	addTransl   string
	addLangIdx  int

	// Practice tab
	practicing    bool
	practiceQueue []int // indices into vocab
	practiceIdx   int
	practiceInput string
	showAnswer    bool
	practiceRight bool
	streak        int
	sessionRight  int
	sessionTotal  int
	reverse       bool // true = show translation, type word

	// Grammar tab
	grammarFiles  []string
	grammarCursor int
	grammarScroll int
	grammarView   string // non-empty = viewing a file
	creatingNote  bool
	newNoteName   string
	newNoteLang   int

	// Dashboard
	dashScroll int

	// lastSaveErr stores the most recent vocabulary or grammar-file
	// write error so the host Model can surface it via reportError.
	// Consumed once.
	lastSaveErr error
}

// ConsumeSaveError returns and clears the last language-learning write
// error. The host Model calls this after each Update.
func (ll *LanguageLearning) ConsumeSaveError() error {
	if ll == nil {
		return nil
	}
	err := ll.lastSaveErr
	ll.lastSaveErr = nil
	return err
}

var langChoices = []string{
	"Spanish", "French", "German", "Japanese",
	"Chinese", "Italian", "Portuguese", "Korean", "Other",
}

// NewLanguageLearning creates a new LanguageLearning overlay.
func NewLanguageLearning() LanguageLearning {
	return LanguageLearning{}
}

// Open activates the overlay and loads data from the vault.
func (ll *LanguageLearning) Open(vaultRoot string) {
	ll.Activate()
	ll.vaultRoot = vaultRoot
	ll.tab = 0
	ll.vocabCursor = 0
	ll.vocabScroll = 0
	ll.addingWord = false
	ll.practicing = false
	ll.grammarView = ""
	ll.creatingNote = false
	ll.dashScroll = 0
	ll.loadVocabulary()
	ll.loadGrammarFiles()
}

// ---------- file I/O ----------

func (ll *LanguageLearning) vocabPath() string {
	return filepath.Join(ll.vaultRoot, "Languages", "vocabulary.md")
}

func (ll *LanguageLearning) grammarDir() string {
	return filepath.Join(ll.vaultRoot, "Languages", "grammar")
}

func (ll *LanguageLearning) loadVocabulary() {
	ll.vocab = nil
	data, err := os.ReadFile(ll.vocabPath())
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	inTable := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "| Word") || strings.HasPrefix(line, "|---") {
			inTable = true
			continue
		}
		if !inTable || !strings.HasPrefix(line, "|") {
			continue
		}
		parts := strings.Split(line, "|")
		// Expect: empty, word, translation, language, level, lastReviewed, correct, empty
		if len(parts) < 7 {
			continue
		}
		level := 1
		_, _ = fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &level)
		correct := 0
		_, _ = fmt.Sscanf(strings.TrimSpace(parts[6]), "%d", &correct)
		ll.vocab = append(ll.vocab, VocabEntry{
			Word:         strings.TrimSpace(parts[1]),
			Translation:  strings.TrimSpace(parts[2]),
			Language:     strings.TrimSpace(parts[3]),
			Level:        level,
			LastReviewed: strings.TrimSpace(parts[5]),
			Correct:      correct,
		})
	}
}

func (ll *LanguageLearning) saveVocabulary() {
	dir := filepath.Dir(ll.vocabPath())
	_ = os.MkdirAll(dir, 0755)

	var b strings.Builder
	b.WriteString("---\ntype: vocabulary\n---\n# Vocabulary\n\n")
	b.WriteString("| Word | Translation | Language | Level | Last Reviewed | Correct |\n")
	b.WriteString("|------|-------------|----------|-------|---------------|--------|\n")
	for _, v := range ll.vocab {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s | %d |\n",
			v.Word, v.Translation, v.Language, v.Level, v.LastReviewed, v.Correct))
	}
	if err := atomicWriteNote(ll.vocabPath(), b.String()); err != nil {
		ll.lastSaveErr = err
	}
}

func (ll *LanguageLearning) loadGrammarFiles() {
	ll.grammarFiles = nil
	entries, err := os.ReadDir(ll.grammarDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			ll.grammarFiles = append(ll.grammarFiles, e.Name())
		}
	}
	sort.Strings(ll.grammarFiles)
}

// ---------- practice helpers ----------

func (ll *LanguageLearning) startPractice() {
	if len(ll.vocab) == 0 {
		return
	}
	ll.practicing = true
	ll.practiceInput = ""
	ll.showAnswer = false
	ll.streak = 0
	ll.sessionRight = 0
	ll.sessionTotal = 0
	ll.reverse = false

	// Build queue weighted by lower levels (spaced repetition lite).
	var queue []int
	for i, v := range ll.vocab {
		// Lower levels get more entries in the queue.
		weight := 6 - v.Level
		if weight < 1 {
			weight = 1
		}
		for j := 0; j < weight; j++ {
			queue = append(queue, i)
		}
	}
	// Shuffle the queue.
	rand.Shuffle(len(queue), func(i, j int) {
		queue[i], queue[j] = queue[j], queue[i]
	})
	// Remove duplicate consecutive indices.
	var deduped []int
	prev := -1
	for _, idx := range queue {
		if idx != prev {
			deduped = append(deduped, idx)
			prev = idx
		}
	}
	ll.practiceQueue = deduped
	ll.practiceIdx = 0
}

func (ll *LanguageLearning) submitAnswer() {
	if ll.practiceIdx >= len(ll.practiceQueue) {
		return
	}
	idx := ll.practiceQueue[ll.practiceIdx]
	entry := &ll.vocab[idx]

	expected := entry.Translation
	if ll.reverse {
		expected = entry.Word
	}

	ll.sessionTotal++
	if strings.EqualFold(strings.TrimSpace(ll.practiceInput), strings.TrimSpace(expected)) {
		ll.practiceRight = true
		ll.streak++
		ll.sessionRight++
		entry.Correct++
		if entry.Correct%3 == 0 && entry.Level < 5 {
			entry.Level++
		}
	} else {
		ll.practiceRight = false
		ll.streak = 0
		if entry.Level > 1 && entry.Correct > 0 {
			entry.Level--
		}
	}
	entry.LastReviewed = time.Now().Format("2006-01-02")
	ll.showAnswer = true
	ll.saveVocabulary()
}

func (ll *LanguageLearning) nextCard() {
	ll.practiceIdx++
	ll.practiceInput = ""
	ll.showAnswer = false
	if ll.practiceIdx >= len(ll.practiceQueue) {
		ll.practicing = false
	}
}

// ---------- Update ----------

func (ll LanguageLearning) Update(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	if !ll.active {
		return ll, nil
	}

	key := msg.String()

	// --- Practice mode input ---
	if ll.practicing {
		return ll.updatePractice(msg)
	}

	// --- Adding vocabulary word ---
	if ll.addingWord {
		return ll.updateAddWord(msg)
	}

	// --- Creating grammar note ---
	if ll.creatingNote {
		return ll.updateCreateGrammar(msg)
	}

	// --- Viewing grammar note ---
	if ll.grammarView != "" {
		switch key {
		case "esc":
			ll.grammarView = ""
		case "up", "k":
			// scroll handled by grammarScroll in view mode
			if ll.grammarScroll > 0 {
				ll.grammarScroll--
			}
		case "down", "j":
			ll.grammarScroll++
		}
		return ll, nil
	}

	// --- Global keys ---
	switch key {
	case "esc":
		ll.active = false
		return ll, nil
	case "tab":
		ll.tab = (ll.tab + 1) % 4
		ll.resetTabState()
		return ll, nil
	case "1":
		ll.tab = 0
		ll.resetTabState()
		return ll, nil
	case "2":
		ll.tab = 1
		ll.resetTabState()
		return ll, nil
	case "3":
		ll.tab = 2
		ll.resetTabState()
		return ll, nil
	case "4":
		ll.tab = 3
		ll.resetTabState()
		return ll, nil
	}

	// --- Tab-specific keys ---
	switch ll.tab {
	case 0:
		return ll.updateVocabTab(msg)
	case 1:
		return ll.updatePracticeTab(msg)
	case 2:
		return ll.updateGrammarTab(msg)
	case 3:
		return ll.updateDashboardTab(msg)
	}

	return ll, nil
}

func (ll *LanguageLearning) resetTabState() {
	ll.vocabCursor = 0
	ll.vocabScroll = 0
	ll.grammarCursor = 0
	ll.grammarScroll = 0
	ll.dashScroll = 0
	ll.grammarView = ""
	ll.addingWord = false
	ll.creatingNote = false
	ll.practicing = false
}

func (ll LanguageLearning) updateVocabTab(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	switch msg.String() {
	case "n":
		ll.addingWord = true
		ll.addField = 0
		ll.addWord = ""
		ll.addTransl = ""
		ll.addLangIdx = 0
	case "up", "k":
		if ll.vocabCursor > 0 {
			ll.vocabCursor--
			if ll.vocabCursor < ll.vocabScroll {
				ll.vocabScroll = ll.vocabCursor
			}
		}
	case "down", "j":
		if ll.vocabCursor < len(ll.vocab)-1 {
			ll.vocabCursor++
			visH := ll.contentHeight() - 4
			if visH < 1 {
				visH = 1
			}
			if ll.vocabCursor >= ll.vocabScroll+visH {
				ll.vocabScroll = ll.vocabCursor - visH + 1
			}
		}
	case "d", "delete":
		if len(ll.vocab) > 0 && ll.vocabCursor < len(ll.vocab) {
			ll.vocab = append(ll.vocab[:ll.vocabCursor], ll.vocab[ll.vocabCursor+1:]...)
			if ll.vocabCursor >= len(ll.vocab) && ll.vocabCursor > 0 {
				ll.vocabCursor--
			}
			ll.saveVocabulary()
		}
	}
	return ll, nil
}

func (ll LanguageLearning) updatePracticeTab(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	switch msg.String() {
	case "enter":
		ll.startPractice()
	}
	return ll, nil
}

func (ll LanguageLearning) updatePractice(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	key := msg.String()

	if ll.showAnswer {
		switch key {
		case "enter", " ":
			ll.nextCard()
		case "esc":
			ll.practicing = false
		}
		return ll, nil
	}

	switch key {
	case "esc":
		ll.practicing = false
	case "enter":
		ll.submitAnswer()
	case "backspace":
		if len(ll.practiceInput) > 0 {
			ll.practiceInput = TrimLastRune(ll.practiceInput)
		}
	default:
		if len(key) == 1 || key == " " {
			ll.practiceInput += key
		}
	}
	return ll, nil
}

func (ll LanguageLearning) updateGrammarTab(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	switch msg.String() {
	case "n":
		ll.creatingNote = true
		ll.newNoteName = ""
		ll.newNoteLang = 0
		ll.addField = 0
	case "up", "k":
		if ll.grammarCursor > 0 {
			ll.grammarCursor--
			if ll.grammarCursor < ll.grammarScroll {
				ll.grammarScroll = ll.grammarCursor
			}
		}
	case "down", "j":
		if ll.grammarCursor < len(ll.grammarFiles)-1 {
			ll.grammarCursor++
			visH := ll.contentHeight() - 4
			if visH < 1 {
				visH = 1
			}
			if ll.grammarCursor >= ll.grammarScroll+visH {
				ll.grammarScroll = ll.grammarCursor - visH + 1
			}
		}
	case "enter":
		if len(ll.grammarFiles) > 0 && ll.grammarCursor < len(ll.grammarFiles) {
			path := filepath.Join(ll.grammarDir(), ll.grammarFiles[ll.grammarCursor])
			data, err := os.ReadFile(path)
			if err == nil {
				ll.grammarView = string(data)
				ll.grammarScroll = 0
			}
		}
	}
	return ll, nil
}

func (ll LanguageLearning) updateDashboardTab(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if ll.dashScroll > 0 {
			ll.dashScroll--
		}
	case "down", "j":
		ll.dashScroll++
	}
	return ll, nil
}

func (ll LanguageLearning) updateAddWord(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		ll.addingWord = false
		return ll, nil
	case "tab":
		ll.addField = (ll.addField + 1) % 3
		return ll, nil
	case "enter":
		if ll.addField < 2 {
			ll.addField++
		} else {
			// Submit the new word.
			word := strings.TrimSpace(ll.addWord)
			transl := strings.TrimSpace(ll.addTransl)
			if word != "" && transl != "" {
				lang := langChoices[ll.addLangIdx]
				ll.vocab = append(ll.vocab, VocabEntry{
					Word:         word,
					Translation:  transl,
					Language:     lang,
					Level:        1,
					LastReviewed: time.Now().Format("2006-01-02"),
					Correct:      0,
				})
				ll.saveVocabulary()
			}
			ll.addingWord = false
		}
		return ll, nil
	}

	switch ll.addField {
	case 0:
		ll.addWord = ll.handleTextInput(ll.addWord, msg)
	case 1:
		ll.addTransl = ll.handleTextInput(ll.addTransl, msg)
	case 2:
		switch key {
		case "left", "h":
			if ll.addLangIdx > 0 {
				ll.addLangIdx--
			}
		case "right", "l":
			if ll.addLangIdx < len(langChoices)-1 {
				ll.addLangIdx++
			}
		}
	}
	return ll, nil
}

func (ll LanguageLearning) updateCreateGrammar(msg tea.KeyMsg) (LanguageLearning, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		ll.creatingNote = false
		return ll, nil
	case "tab":
		ll.addField = (ll.addField + 1) % 2
		return ll, nil
	case "enter":
		if ll.addField == 0 {
			ll.addField = 1
		} else {
			name := strings.TrimSpace(ll.newNoteName)
			if name != "" {
				ll.createGrammarNote(name, langChoices[ll.newNoteLang])
				ll.loadGrammarFiles()
			}
			ll.creatingNote = false
		}
		return ll, nil
	}

	switch ll.addField {
	case 0:
		ll.newNoteName = ll.handleTextInput(ll.newNoteName, msg)
	case 1:
		switch key {
		case "left", "h":
			if ll.newNoteLang > 0 {
				ll.newNoteLang--
			}
		case "right", "l":
			if ll.newNoteLang < len(langChoices)-1 {
				ll.newNoteLang++
			}
		}
	}
	return ll, nil
}

func (ll *LanguageLearning) handleTextInput(current string, msg tea.KeyMsg) string {
	key := msg.String()
	switch key {
	case "backspace":
		if len(current) > 0 {
			return current[:len(current)-1]
		}
	default:
		if len(key) == 1 || key == " " {
			return current + key
		}
	}
	return current
}

func (ll *LanguageLearning) createGrammarNote(topic, language string) {
	dir := ll.grammarDir()
	_ = os.MkdirAll(dir, 0755)

	filename := strings.ReplaceAll(strings.ToLower(topic), " ", "-") + ".md"
	path := filepath.Join(dir, filename)

	content := fmt.Sprintf(`---
language: %s
topic: %s
level: beginner
---
# %s

## Rule


## Examples


## Exceptions


## Practice Sentences

`, language, topic, topic)

	if err := atomicWriteNote(path, content); err != nil {
		ll.lastSaveErr = err
	}
}

func (ll LanguageLearning) contentHeight() int {
	h := ll.height - 14
	if h < 5 {
		h = 5
	}
	return h
}

// ---------- View ----------

func (ll LanguageLearning) View() string {
	width := ll.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconBookmarkChar + " Language Learning"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Tab bar
	b.WriteString(ll.renderTabBar())
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	// Content
	switch ll.tab {
	case 0:
		b.WriteString(ll.viewVocabulary(innerWidth))
	case 1:
		b.WriteString(ll.viewPractice(innerWidth))
	case 2:
		b.WriteString(ll.viewGrammar(innerWidth))
	case 3:
		b.WriteString(ll.viewDashboard(innerWidth))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")
	b.WriteString(ll.renderFooter())

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (ll LanguageLearning) renderTabBar() string {
	activeTabStyle := lipgloss.NewStyle().
		Foreground(crust).
		Background(mauve).
		Bold(true).
		Padding(0, 1)

	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(overlay0).
		Background(surface0).
		Padding(0, 1)

	tabs := []string{"Vocabulary", "Practice", "Grammar", "Dashboard"}
	var parts []string
	for i, name := range tabs {
		if i == ll.tab {
			parts = append(parts, activeTabStyle.Render(name))
		} else {
			parts = append(parts, inactiveTabStyle.Render(name))
		}
	}
	return strings.Join(parts, " ")
}

func (ll LanguageLearning) renderFooter() string {
	pairs := []struct{ Key, Desc string }{
		{"Tab", "switch"}, {"1-4", "tabs"},
	}

	switch ll.tab {
	case 0:
		if ll.addingWord {
			pairs = append(pairs, struct{ Key, Desc string }{"Enter", "next/save"}, struct{ Key, Desc string }{"Esc", "cancel"})
		} else {
			pairs = append(pairs, struct{ Key, Desc string }{"n", "add"}, struct{ Key, Desc string }{"d", "delete"}, struct{ Key, Desc string }{"Esc", "close"})
		}
	case 1:
		if ll.practicing {
			pairs = append(pairs, struct{ Key, Desc string }{"Enter", "submit"}, struct{ Key, Desc string }{"Esc", "stop"})
		} else {
			pairs = append(pairs, struct{ Key, Desc string }{"Enter", "start"}, struct{ Key, Desc string }{"Esc", "close"})
		}
	case 2:
		if ll.creatingNote {
			pairs = append(pairs, struct{ Key, Desc string }{"Enter", "next/save"}, struct{ Key, Desc string }{"Esc", "cancel"})
		} else if ll.grammarView != "" {
			pairs = append(pairs, struct{ Key, Desc string }{"j/k", "scroll"}, struct{ Key, Desc string }{"Esc", "back"})
		} else {
			pairs = append(pairs, struct{ Key, Desc string }{"n", "new"}, struct{ Key, Desc string }{"Enter", "open"}, struct{ Key, Desc string }{"Esc", "close"})
		}
	case 3:
		pairs = append(pairs, struct{ Key, Desc string }{"j/k", "scroll"}, struct{ Key, Desc string }{"Esc", "close"})
	}

	return RenderHelpBar(pairs)
}

// ---------- Tab views ----------

func (ll LanguageLearning) viewVocabulary(innerWidth int) string {
	var b strings.Builder

	if ll.addingWord {
		b.WriteString(ll.viewAddWord(innerWidth))
		return b.String()
	}

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString(sectionStyle.Render("  " + IconEditChar + " Vocabulary"))
	countStr := DimStyle.Render(fmt.Sprintf(" (%d words)", len(ll.vocab)))
	b.WriteString(countStr)
	b.WriteString("\n\n")

	if len(ll.vocab) == 0 {
		b.WriteString(DimStyle.Render("  No vocabulary entries yet\n"))
		b.WriteString(DimStyle.Render("  Press n to add your first word"))
		return b.String()
	}

	// Table header
	headerStyle := lipgloss.NewStyle().Foreground(teal).Bold(true)
	wordCol := 16
	translCol := 16
	langCol := 10
	lvlCol := 5

	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s",
		wordCol, "Word", translCol, "Translation", langCol, "Lang", lvlCol, "Lvl")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	visH := ll.contentHeight() - 4
	if visH < 1 {
		visH = 1
	}
	end := ll.vocabScroll + visH
	if end > len(ll.vocab) {
		end = len(ll.vocab)
	}

	for i := ll.vocabScroll; i < end; i++ {
		v := ll.vocab[i]
		word := truncStr(v.Word, wordCol)
		transl := truncStr(v.Translation, translCol)
		lang := truncStr(v.Language, langCol)
		lvl := ll.renderLevel(v.Level)

		line := fmt.Sprintf("  %-*s %-*s %-*s %s",
			wordCol, word, translCol, transl, langCol, lang, lvl)

		if i == ll.vocabCursor {
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(innerWidth).
				Render(line))
		} else {
			b.WriteString(NormalItemStyle.Render(line))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (ll LanguageLearning) renderLevel(level int) string {
	filled := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("*", level))
	empty := DimStyle.Render(strings.Repeat("*", 5-level))
	return filled + empty
}

func (ll LanguageLearning) viewAddWord(innerWidth int) string {
	var b strings.Builder

	formTitle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(formTitle.Render("  " + IconNewChar + " Add New Word"))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(text)
	activeStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text).Padding(0, 1)

	// Word field
	label := "  Word:        "
	if ll.addField == 0 {
		b.WriteString(activeStyle.Render(label))
		b.WriteString(inputBg.Render(ll.addWord + "_"))
	} else {
		b.WriteString(labelStyle.Render(label))
		if ll.addWord != "" {
			b.WriteString(NormalItemStyle.Render(ll.addWord))
		} else {
			b.WriteString(DimStyle.Render("(empty)"))
		}
	}
	b.WriteString("\n\n")

	// Translation field
	label = "  Translation: "
	if ll.addField == 1 {
		b.WriteString(activeStyle.Render(label))
		b.WriteString(inputBg.Render(ll.addTransl + "_"))
	} else {
		b.WriteString(labelStyle.Render(label))
		if ll.addTransl != "" {
			b.WriteString(NormalItemStyle.Render(ll.addTransl))
		} else {
			b.WriteString(DimStyle.Render("(empty)"))
		}
	}
	b.WriteString("\n\n")

	// Language selector
	label = "  Language:    "
	if ll.addField == 2 {
		b.WriteString(activeStyle.Render(label))
	} else {
		b.WriteString(labelStyle.Render(label))
	}
	b.WriteString(ll.renderLangSelector(ll.addLangIdx, ll.addField == 2))

	return b.String()
}

func (ll LanguageLearning) renderLangSelector(idx int, active bool) string {
	var parts []string
	for i, lang := range langChoices {
		if i == idx {
			style := lipgloss.NewStyle().Foreground(crust).Background(mauve).Bold(true).Padding(0, 1)
			parts = append(parts, style.Render(lang))
		} else if active {
			style := lipgloss.NewStyle().Foreground(overlay0).Background(surface0).Padding(0, 1)
			parts = append(parts, style.Render(lang))
		}
	}
	if !active {
		return lipgloss.NewStyle().Foreground(teal).Render(langChoices[idx])
	}
	// Show only a window of choices to keep it manageable.
	start := idx - 2
	if start < 0 {
		start = 0
	}
	end := start + 5
	if end > len(parts) {
		end = len(parts)
		start = end - 5
		if start < 0 {
			start = 0
		}
	}
	return strings.Join(parts[start:end], " ")
}

func (ll LanguageLearning) viewPractice(innerWidth int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

	if !ll.practicing {
		b.WriteString(sectionStyle.Render("  " + IconSearchChar + " Practice Mode"))
		b.WriteString("\n\n")

		if len(ll.vocab) == 0 {
			b.WriteString(DimStyle.Render("  No vocabulary to practice\n"))
			b.WriteString(DimStyle.Render("  Add words in the Vocabulary tab first"))
			return b.String()
		}

		// Show practice overview.
		labelStyle := lipgloss.NewStyle().Foreground(text)
		numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

		b.WriteString(labelStyle.Render("  Total words: "))
		b.WriteString(numStyle.Render(smallNum(len(ll.vocab))))
		b.WriteString("\n")

		// Count by level
		levels := [6]int{}
		for _, v := range ll.vocab {
			if v.Level >= 1 && v.Level <= 5 {
				levels[v.Level]++
			}
		}
		levelNames := []string{"", "Beginner", "Elementary", "Intermediate", "Advanced", "Mastered"}
		for i := 1; i <= 5; i++ {
			if levels[i] > 0 {
				b.WriteString(labelStyle.Render(fmt.Sprintf("  Level %d (%s): ", i, levelNames[i])))
				b.WriteString(numStyle.Render(smallNum(levels[i])))
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(promptStyle.Render("  Press Enter to start a practice session"))

		return b.String()
	}

	// Active practice
	if ll.practiceIdx >= len(ll.practiceQueue) {
		// Session complete
		b.WriteString(sectionStyle.Render("  Session Complete!"))
		b.WriteString("\n\n")

		numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		labelStyle := lipgloss.NewStyle().Foreground(text)
		b.WriteString(labelStyle.Render("  Correct: "))
		b.WriteString(numStyle.Render(fmt.Sprintf("%d/%d", ll.sessionRight, ll.sessionTotal)))
		if ll.sessionTotal > 0 {
			pct := ll.sessionRight * 100 / ll.sessionTotal
			b.WriteString(DimStyle.Render(fmt.Sprintf(" (%d%%)", pct)))
		}
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Press Esc to exit"))
		return b.String()
	}

	idx := ll.practiceQueue[ll.practiceIdx]
	entry := ll.vocab[idx]

	// Streak
	if ll.streak > 0 {
		streakStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString(streakStyle.Render(fmt.Sprintf("  Streak: %d ", ll.streak)))
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render(strings.Repeat("*", ll.streak)))
		b.WriteString("\n\n")
	}

	// Progress
	progressStyle := DimStyle
	b.WriteString(progressStyle.Render(fmt.Sprintf("  Card %d/%d", ll.practiceIdx+1, len(ll.practiceQueue))))
	b.WriteString("  ")
	b.WriteString(DimStyle.Render(fmt.Sprintf("[%d/%d correct]", ll.sessionRight, ll.sessionTotal)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n\n")

	// Flashcard
	cardStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	langLabel := lipgloss.NewStyle().Foreground(teal).Render("  [" + entry.Language + "] ")

	var prompt, expected string
	if ll.reverse {
		prompt = entry.Translation
		expected = entry.Word
	} else {
		prompt = entry.Word
		expected = entry.Translation
	}

	b.WriteString(langLabel)
	b.WriteString(cardStyle.Render(prompt))
	b.WriteString("\n\n")

	if ll.showAnswer {
		if ll.practiceRight {
			correctStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
			b.WriteString(correctStyle.Render("  Correct!"))
		} else {
			wrongStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
			b.WriteString(wrongStyle.Render("  Incorrect"))
			b.WriteString("\n")
			answerStyle := lipgloss.NewStyle().Foreground(yellow)
			b.WriteString(answerStyle.Render("  Answer: " + expected))
			b.WriteString("\n")
			yourStyle := DimStyle
			b.WriteString(yourStyle.Render("  Yours:  " + ll.practiceInput))
		}
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Press Enter for next card"))
	} else {
		inputPrompt := lipgloss.NewStyle().Foreground(peach).Bold(true)
		inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text).Padding(0, 1)
		b.WriteString(inputPrompt.Render("  Your answer: "))
		b.WriteString(inputBg.Render(ll.practiceInput + "_"))
	}

	return b.String()
}

func (ll LanguageLearning) viewGrammar(innerWidth int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

	if ll.creatingNote {
		return ll.viewCreateGrammar(innerWidth)
	}

	if ll.grammarView != "" {
		// Read-only preview
		b.WriteString(sectionStyle.Render("  " + IconFileChar + " Grammar Note"))
		if ll.grammarCursor < len(ll.grammarFiles) {
			b.WriteString(DimStyle.Render("  " + ll.grammarFiles[ll.grammarCursor]))
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
		b.WriteString("\n\n")

		lines := strings.Split(ll.grammarView, "\n")
		visH := ll.contentHeight() - 4
		if visH < 1 {
			visH = 1
		}
		maxScroll := len(lines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := ll.grammarScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}
		end := scroll + visH
		if end > len(lines) {
			end = len(lines)
		}

		for i := scroll; i < end; i++ {
			line := lines[i]
			if strings.HasPrefix(line, "# ") {
				b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  " + line))
			} else if strings.HasPrefix(line, "## ") {
				b.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  " + line))
			} else if strings.HasPrefix(line, "---") {
				b.WriteString(DimStyle.Render("  " + line))
			} else {
				b.WriteString(NormalItemStyle.Render("  " + line))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
		return b.String()
	}

	b.WriteString(sectionStyle.Render("  " + IconFileChar + " Grammar Notes"))
	b.WriteString("\n\n")

	if len(ll.grammarFiles) == 0 {
		b.WriteString(DimStyle.Render("  No grammar notes found\n"))
		b.WriteString(DimStyle.Render("  Press n to create one"))
		return b.String()
	}

	visH := ll.contentHeight() - 4
	if visH < 1 {
		visH = 1
	}
	end := ll.grammarScroll + visH
	if end > len(ll.grammarFiles) {
		end = len(ll.grammarFiles)
	}

	fileIcon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
	for i := ll.grammarScroll; i < end; i++ {
		name := strings.TrimSuffix(ll.grammarFiles[i], ".md")
		if i == ll.grammarCursor {
			line := "  " + fileIcon + " " + name
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(innerWidth).
				Render(line))
		} else {
			b.WriteString("  " + fileIcon + " " + NormalItemStyle.Render(name))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (ll LanguageLearning) viewCreateGrammar(innerWidth int) string {
	var b strings.Builder

	formTitle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(formTitle.Render("  " + IconNewChar + " New Grammar Note"))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(text)
	activeStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text).Padding(0, 1)

	// Topic
	label := "  Topic:    "
	if ll.addField == 0 {
		b.WriteString(activeStyle.Render(label))
		b.WriteString(inputBg.Render(ll.newNoteName + "_"))
	} else {
		b.WriteString(labelStyle.Render(label))
		if ll.newNoteName != "" {
			b.WriteString(NormalItemStyle.Render(ll.newNoteName))
		} else {
			b.WriteString(DimStyle.Render("(empty)"))
		}
	}
	b.WriteString("\n\n")

	// Language
	label = "  Language: "
	if ll.addField == 1 {
		b.WriteString(activeStyle.Render(label))
	} else {
		b.WriteString(labelStyle.Render(label))
	}
	b.WriteString(ll.renderLangSelector(ll.newNoteLang, ll.addField == 1))

	return b.String()
}

func (ll LanguageLearning) viewDashboard(innerWidth int) string {
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(mauve)

	lines = append(lines, sectionStyle.Render("  "+IconGraphChar+" Dashboard"))
	lines = append(lines, "")

	if len(ll.vocab) == 0 {
		lines = append(lines, DimStyle.Render("  No vocabulary data yet"))
		lines = append(lines, DimStyle.Render("  Add words in the Vocabulary tab to see stats"))
		return strings.Join(lines, "\n")
	}

	// Words per language
	langCount := make(map[string]int)
	for _, v := range ll.vocab {
		langCount[v.Language]++
	}

	lines = append(lines, sectionStyle.Render("  Words by Language"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	for lang, count := range langCount {
		lines = append(lines, labelStyle.Render("  "+padRight(lang, 15))+numStyle.Render(smallNum(count)+" words"))
	}
	lines = append(lines, "")

	// Level distribution
	levels := [6]int{}
	for _, v := range ll.vocab {
		if v.Level >= 1 && v.Level <= 5 {
			levels[v.Level]++
		}
	}

	levelNames := []string{"", "Beginner", "Elementary", "Intermediate", "Advanced", "Mastered"}
	lines = append(lines, sectionStyle.Render("  Words by Level"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))

	maxLvl := 0
	for i := 1; i <= 5; i++ {
		if levels[i] > maxLvl {
			maxLvl = levels[i]
		}
	}

	for i := 1; i <= 5; i++ {
		barLen := 0
		if maxLvl > 0 {
			barLen = levels[i] * 20 / maxLvl
		}
		if barLen < 1 && levels[i] > 0 {
			barLen = 1
		}
		bar := barStyle.Render(strings.Repeat("█", barLen)) + DimStyle.Render(strings.Repeat("░", 20-barLen))
		label := fmt.Sprintf("Lv%d %s", i, levelNames[i])
		lines = append(lines, "  "+labelStyle.Render(padRight(label, 20))+bar+" "+numStyle.Render(smallNum(levels[i])))
	}
	lines = append(lines, "")

	// Suggested review — words reviewed longest ago with low levels.
	lines = append(lines, sectionStyle.Render("  Suggested Review"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))

	type reviewCandidate struct {
		entry VocabEntry
		score int // lower = more urgent
	}
	var candidates []reviewCandidate
	today := time.Now().Format("2006-01-02")
	for _, v := range ll.vocab {
		if v.Level >= 5 {
			continue
		}
		daysSince := 0
		if v.LastReviewed != "" && v.LastReviewed != today {
			t1, err := time.Parse("2006-01-02", v.LastReviewed)
			if err == nil {
				daysSince = int(time.Since(t1).Hours() / 24)
			}
		}
		score := v.Level*10 - daysSince
		candidates = append(candidates, reviewCandidate{entry: v, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	shown := 5
	if shown > len(candidates) {
		shown = len(candidates)
	}
	if shown == 0 {
		lines = append(lines, DimStyle.Render("  All words are mastered!"))
	} else {
		for i := 0; i < shown; i++ {
			c := candidates[i]
			wordStyle := lipgloss.NewStyle().Foreground(yellow)
			translStyle := DimStyle
			lines = append(lines, "  "+wordStyle.Render(padRight(c.entry.Word, 16))+
				translStyle.Render(c.entry.Translation)+
				DimStyle.Render(" ["+c.entry.Language+"]"))
		}
	}
	lines = append(lines, "")

	// Total stats
	lines = append(lines, sectionStyle.Render("  Summary"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, labelStyle.Render("  Total words:      ")+numStyle.Render(smallNum(len(ll.vocab))))
	lines = append(lines, labelStyle.Render("  Languages:        ")+numStyle.Render(smallNum(len(langCount))))

	totalCorrect := 0
	for _, v := range ll.vocab {
		totalCorrect += v.Correct
	}
	lines = append(lines, labelStyle.Render("  Total reviews:    ")+numStyle.Render(smallNum(totalCorrect)))

	// Build view with scroll
	var b strings.Builder

	visH := ll.contentHeight()
	if visH < 5 {
		visH = 5
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := ll.dashScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// truncStr truncates a string to the given width, adding "..." if needed.
func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
