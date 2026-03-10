package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeVocabFile writes a vocabulary markdown table to the given vault root.
func makeVocabFile(t *testing.T, root string, entries []VocabEntry) {
	t.Helper()
	dir := filepath.Join(root, "Languages")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("---\ntype: vocabulary\n---\n# Vocabulary\n\n")
	b.WriteString("| Word | Translation | Language | Level | Last Reviewed | Correct |\n")
	b.WriteString("|------|-------------|----------|-------|---------------|--------|\n")
	for _, v := range entries {
		b.WriteString("| " + v.Word + " | " + v.Translation + " | " + v.Language +
			" | " + smallNum(v.Level) + " | " + v.LastReviewed + " | " + smallNum(v.Correct) + " |\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "vocabulary.md"), []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func makeGrammarFile(t *testing.T, root, filename, content string) {
	t.Helper()
	dir := filepath.Join(root, "Languages", "grammar")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func sampleVocab() []VocabEntry {
	return []VocabEntry{
		{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 0},
		{Word: "gato", Translation: "cat", Language: "Spanish", Level: 2, LastReviewed: "2026-01-02", Correct: 3},
		{Word: "maison", Translation: "house", Language: "French", Level: 3, LastReviewed: "2026-01-03", Correct: 6},
		{Word: "Buch", Translation: "book", Language: "German", Level: 4, LastReviewed: "2026-01-04", Correct: 9},
		{Word: "neko", Translation: "cat", Language: "Japanese", Level: 5, LastReviewed: "2026-01-05", Correct: 15},
	}
}

func llKeyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func llSpecialKeyMsg(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

// ---------------------------------------------------------------------------
// 1. Initialization and supported languages
// ---------------------------------------------------------------------------

func TestNewLanguageLearning_Defaults(t *testing.T) {
	ll := NewLanguageLearning()
	if ll.IsActive() {
		t.Error("newly created LanguageLearning should not be active")
	}
	if ll.tab != 0 {
		t.Errorf("expected tab 0, got %d", ll.tab)
	}
	if ll.practicing {
		t.Error("should not be practicing by default")
	}
}

func TestSupportedLanguages(t *testing.T) {
	expected := []string{"Spanish", "French", "German", "Japanese", "Chinese", "Italian", "Portuguese", "Korean", "Other"}
	if len(langChoices) != len(expected) {
		t.Fatalf("expected %d languages, got %d", len(expected), len(langChoices))
	}
	for i, lang := range expected {
		if langChoices[i] != lang {
			t.Errorf("langChoices[%d] = %q, want %q", i, langChoices[i], lang)
		}
	}
}

func TestOpen_SetsActiveAndLoadsVocab(t *testing.T) {
	root := t.TempDir()
	entries := sampleVocab()
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)

	if !ll.IsActive() {
		t.Error("expected overlay to be active after Open()")
	}
	if len(ll.vocab) != len(entries) {
		t.Errorf("expected %d vocab entries, got %d", len(entries), len(ll.vocab))
	}
	if ll.tab != 0 {
		t.Errorf("expected tab 0 after open, got %d", ll.tab)
	}
}

func TestClose_SetsInactive(t *testing.T) {
	ll := NewLanguageLearning()
	ll.active = true
	ll.Close()
	if ll.IsActive() {
		t.Error("expected overlay to be inactive after Close()")
	}
}

func TestSetSize_LanguageLearning(t *testing.T) {
	ll := NewLanguageLearning()
	ll.SetSize(120, 40)
	if ll.width != 120 || ll.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", ll.width, ll.height)
	}
}

// ---------------------------------------------------------------------------
// 2. Vocabulary entry creation
// ---------------------------------------------------------------------------

func TestAddWord_FullFlow(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil) // empty table

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	if len(ll.vocab) != 0 {
		t.Fatalf("expected 0 vocab entries initially, got %d", len(ll.vocab))
	}

	// Press 'n' to begin adding
	ll, _ = ll.updateVocabTab(llKeyMsg("n"))
	if !ll.addingWord {
		t.Fatal("expected addingWord to be true after pressing n")
	}
	if ll.addField != 0 {
		t.Errorf("expected addField 0 (word), got %d", ll.addField)
	}

	// Type the word "perro"
	for _, ch := range "perro" {
		ll, _ = ll.updateAddWord(llKeyMsg(string(ch)))
	}
	if ll.addWord != "perro" {
		t.Errorf("expected addWord 'perro', got %q", ll.addWord)
	}

	// Press Enter to move to translation field
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	if ll.addField != 1 {
		t.Errorf("expected addField 1 (translation), got %d", ll.addField)
	}

	// Type the translation "dog"
	for _, ch := range "dog" {
		ll, _ = ll.updateAddWord(llKeyMsg(string(ch)))
	}
	if ll.addTransl != "dog" {
		t.Errorf("expected addTransl 'dog', got %q", ll.addTransl)
	}

	// Press Enter to move to language field
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	if ll.addField != 2 {
		t.Errorf("expected addField 2 (language), got %d", ll.addField)
	}
	// Default language should be index 0 (Spanish)
	if ll.addLangIdx != 0 {
		t.Errorf("expected addLangIdx 0, got %d", ll.addLangIdx)
	}

	// Press Enter to submit
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	if ll.addingWord {
		t.Error("expected addingWord to be false after submit")
	}
	if len(ll.vocab) != 1 {
		t.Fatalf("expected 1 vocab entry, got %d", len(ll.vocab))
	}

	v := ll.vocab[0]
	if v.Word != "perro" {
		t.Errorf("expected word 'perro', got %q", v.Word)
	}
	if v.Translation != "dog" {
		t.Errorf("expected translation 'dog', got %q", v.Translation)
	}
	if v.Language != "Spanish" {
		t.Errorf("expected language 'Spanish', got %q", v.Language)
	}
	if v.Level != 1 {
		t.Errorf("expected level 1, got %d", v.Level)
	}
	if v.Correct != 0 {
		t.Errorf("expected correct 0, got %d", v.Correct)
	}

	// Verify file was saved
	data, err := os.ReadFile(filepath.Join(root, "Languages", "vocabulary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "perro") {
		t.Error("expected saved file to contain 'perro'")
	}
}

func TestAddWord_EmptyWordIsRejected(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	// Start adding
	ll, _ = ll.updateVocabTab(llKeyMsg("n"))

	// Skip word (empty), go to translation
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	// Type translation
	for _, ch := range "dog" {
		ll, _ = ll.updateAddWord(llKeyMsg(string(ch)))
	}
	// Go to language
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	// Submit — should not add because word is empty
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))

	if len(ll.vocab) != 0 {
		t.Errorf("expected 0 entries (empty word), got %d", len(ll.vocab))
	}
}

func TestAddWord_EmptyTranslationIsRejected(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll, _ = ll.updateVocabTab(llKeyMsg("n"))

	// Type word
	for _, ch := range "perro" {
		ll, _ = ll.updateAddWord(llKeyMsg(string(ch)))
	}
	// Go to translation, leave empty
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	// Go to language
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))
	// Submit
	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEnter))

	if len(ll.vocab) != 0 {
		t.Errorf("expected 0 entries (empty translation), got %d", len(ll.vocab))
	}
}

func TestAddWord_CancelWithEsc(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll, _ = ll.updateVocabTab(llKeyMsg("n"))
	if !ll.addingWord {
		t.Fatal("expected addingWord true")
	}

	ll, _ = ll.updateAddWord(llSpecialKeyMsg(tea.KeyEsc))
	if ll.addingWord {
		t.Error("expected addingWord false after Esc")
	}
}

func TestAddWord_LanguageSelectorNavigation(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll, _ = ll.updateVocabTab(llKeyMsg("n"))

	// Jump to language field
	ll.addField = 2
	ll.addLangIdx = 0

	// Navigate right to "French"
	ll, _ = ll.updateAddWord(llKeyMsg("l"))
	if ll.addLangIdx != 1 {
		t.Errorf("expected addLangIdx 1 after right, got %d", ll.addLangIdx)
	}

	// Navigate left back to "Spanish"
	ll, _ = ll.updateAddWord(llKeyMsg("h"))
	if ll.addLangIdx != 0 {
		t.Errorf("expected addLangIdx 0 after left, got %d", ll.addLangIdx)
	}

	// Navigate left at boundary - should stay at 0
	ll, _ = ll.updateAddWord(llKeyMsg("h"))
	if ll.addLangIdx != 0 {
		t.Errorf("expected addLangIdx 0 at boundary, got %d", ll.addLangIdx)
	}

	// Navigate to last
	ll.addLangIdx = len(langChoices) - 1
	ll, _ = ll.updateAddWord(llKeyMsg("l"))
	if ll.addLangIdx != len(langChoices)-1 {
		t.Errorf("expected addLangIdx at max boundary, got %d", ll.addLangIdx)
	}
}

func TestDeleteVocabEntry(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	original := len(ll.vocab)
	ll.vocabCursor = 1 // select "gato"

	ll, _ = ll.updateVocabTab(llKeyMsg("d"))
	if len(ll.vocab) != original-1 {
		t.Errorf("expected %d entries after delete, got %d", original-1, len(ll.vocab))
	}
	// The second entry should now be "maison" (French)
	if ll.vocab[1].Word != "maison" {
		t.Errorf("expected second word 'maison' after deleting 'gato', got %q", ll.vocab[1].Word)
	}
}

// ---------------------------------------------------------------------------
// 3. Spaced repetition interval calculation
// ---------------------------------------------------------------------------

func TestSubmitAnswer_CorrectLevelUp(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 2}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	// Start practice
	ll.startPractice()
	if !ll.practicing {
		t.Fatal("expected practicing to be true")
	}

	// Submit correct answer — entry has Correct=2, so after +1 it becomes 3 (divisible by 3 -> level up)
	ll.practiceInput = "hello"
	ll.submitAnswer()

	if !ll.practiceRight {
		t.Error("expected answer to be marked correct")
	}
	if ll.vocab[0].Correct != 3 {
		t.Errorf("expected correct count 3, got %d", ll.vocab[0].Correct)
	}
	if ll.vocab[0].Level != 2 {
		t.Errorf("expected level 2 after level-up, got %d", ll.vocab[0].Level)
	}
}

func TestSubmitAnswer_CorrectNoLevelUp(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 0}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "hello"
	ll.submitAnswer()

	// Correct goes 0 -> 1, not divisible by 3, no level up
	if ll.vocab[0].Correct != 1 {
		t.Errorf("expected correct 1, got %d", ll.vocab[0].Correct)
	}
	if ll.vocab[0].Level != 1 {
		t.Errorf("expected level 1 (no level-up), got %d", ll.vocab[0].Level)
	}
}

func TestSubmitAnswer_WrongLevelDown(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 3, LastReviewed: "2026-01-01", Correct: 5}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "wrong answer"
	ll.submitAnswer()

	if ll.practiceRight {
		t.Error("expected answer to be marked wrong")
	}
	if ll.vocab[0].Level != 2 {
		t.Errorf("expected level 2 after wrong answer (was 3), got %d", ll.vocab[0].Level)
	}
}

func TestSubmitAnswer_WrongAtLevel1_NoFurtherDecrease(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 1}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "wrong"
	ll.submitAnswer()

	// Level should not go below 1 (the condition requires level > 1)
	if ll.vocab[0].Level != 1 {
		t.Errorf("expected level 1 (can't go below), got %d", ll.vocab[0].Level)
	}
}

func TestSubmitAnswer_MaxLevel5_NoFurtherIncrease(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "neko", Translation: "cat", Language: "Japanese", Level: 5, LastReviewed: "2026-01-01", Correct: 14}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "cat"
	ll.submitAnswer()

	// Correct becomes 15, 15 % 3 == 0, but level is already 5 (max)
	if ll.vocab[0].Level != 5 {
		t.Errorf("expected level 5 (max), got %d", ll.vocab[0].Level)
	}
}

func TestSubmitAnswer_CaseInsensitive(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "Hola", Translation: "Hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 0}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "hello" // lowercase vs "Hello"
	ll.submitAnswer()

	if !ll.practiceRight {
		t.Error("expected case-insensitive match to be correct")
	}
}

func TestSubmitAnswer_WhitespaceTrimmed(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 0}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "  hello  "
	ll.submitAnswer()

	if !ll.practiceRight {
		t.Error("expected answer with extra whitespace to be correct")
	}
}

func TestSubmitAnswer_SetsLastReviewed(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2020-01-01", Correct: 0}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.practiceInput = "hello"
	ll.submitAnswer()

	today := time.Now().Format("2006-01-02")
	if ll.vocab[0].LastReviewed != today {
		t.Errorf("expected LastReviewed %q, got %q", today, ll.vocab[0].LastReviewed)
	}
}

func TestSubmitAnswer_ReverseMode(t *testing.T) {
	root := t.TempDir()
	entry := VocabEntry{Word: "hola", Translation: "hello", Language: "Spanish", Level: 1, LastReviewed: "2026-01-01", Correct: 0}
	makeVocabFile(t, root, []VocabEntry{entry})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	ll.reverse = true
	ll.practiceInput = "hola" // In reverse mode, expected is the Word
	ll.submitAnswer()

	if !ll.practiceRight {
		t.Error("expected reverse mode answer to be correct")
	}
}

// ---------------------------------------------------------------------------
// 4. Practice mode session (card selection, answer checking)
// ---------------------------------------------------------------------------

func TestStartPractice_BuildsWeightedQueue(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
		{Word: "b", Translation: "2", Language: "Spanish", Level: 5, LastReviewed: "", Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	if !ll.practicing {
		t.Fatal("expected practicing true")
	}
	if len(ll.practiceQueue) == 0 {
		t.Fatal("expected non-empty practice queue")
	}

	// Level 1 word (weight=5) should appear more than level 5 word (weight=1)
	countA, countB := 0, 0
	for _, idx := range ll.practiceQueue {
		if idx == 0 {
			countA++
		} else {
			countB++
		}
	}
	// After deduplication, we can't guarantee exact counts since consecutive
	// duplicates are removed. But index 0 should appear at least once.
	if countA == 0 {
		t.Error("expected level-1 word (index 0) in queue at least once")
	}
	if countB == 0 {
		t.Error("expected level-5 word (index 1) in queue at least once")
	}
}

func TestStartPractice_EmptyVocab_NoPractice(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	if ll.practicing {
		t.Error("expected practicing false when vocab is empty")
	}
}

func TestStartPractice_ResetsSessionCounters(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	// Set some previous state
	ll.sessionRight = 10
	ll.sessionTotal = 20
	ll.streak = 5

	ll.startPractice()

	if ll.sessionRight != 0 {
		t.Errorf("expected sessionRight 0, got %d", ll.sessionRight)
	}
	if ll.sessionTotal != 0 {
		t.Errorf("expected sessionTotal 0, got %d", ll.sessionTotal)
	}
	if ll.streak != 0 {
		t.Errorf("expected streak 0, got %d", ll.streak)
	}
}

func TestNextCard_AdvancesIndex(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()
	initialIdx := ll.practiceIdx

	ll.nextCard()
	if ll.practiceIdx != initialIdx+1 {
		t.Errorf("expected practiceIdx %d, got %d", initialIdx+1, ll.practiceIdx)
	}
	if ll.showAnswer {
		t.Error("expected showAnswer false after nextCard")
	}
	if ll.practiceInput != "" {
		t.Error("expected practiceInput cleared after nextCard")
	}
}

func TestNextCard_EndOfQueue_StopsPracticing(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
	})

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	// Advance past all cards
	for ll.practiceIdx < len(ll.practiceQueue) {
		ll.nextCard()
	}
	if ll.practicing {
		t.Error("expected practicing false after exhausting queue")
	}
}

func TestPracticeQueueNoDuplicateConsecutive(t *testing.T) {
	root := t.TempDir()
	// Use a single word — without dedup it would have many consecutive entries
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	for i := 1; i < len(ll.practiceQueue); i++ {
		if ll.practiceQueue[i] == ll.practiceQueue[i-1] {
			t.Errorf("found consecutive duplicate at index %d", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. Grammar note templates
// ---------------------------------------------------------------------------

func TestCreateGrammarNote_CreatesFile(t *testing.T) {
	root := t.TempDir()

	ll := NewLanguageLearning()
	ll.vaultRoot = root

	ll.createGrammarNote("Present Tense", "Spanish")

	path := filepath.Join(root, "Languages", "grammar", "present-tense.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("grammar note not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "language: Spanish") {
		t.Error("expected frontmatter to contain language")
	}
	if !strings.Contains(content, "topic: Present Tense") {
		t.Error("expected frontmatter to contain topic")
	}
	if !strings.Contains(content, "# Present Tense") {
		t.Error("expected heading with topic name")
	}
	if !strings.Contains(content, "## Rule") {
		t.Error("expected Rule section")
	}
	if !strings.Contains(content, "## Examples") {
		t.Error("expected Examples section")
	}
	if !strings.Contains(content, "## Exceptions") {
		t.Error("expected Exceptions section")
	}
	if !strings.Contains(content, "## Practice Sentences") {
		t.Error("expected Practice Sentences section")
	}
}

func TestCreateGrammarNote_FilenameSlug(t *testing.T) {
	root := t.TempDir()

	ll := NewLanguageLearning()
	ll.vaultRoot = root

	ll.createGrammarNote("Past Participle Usage", "French")

	path := filepath.Join(root, "Languages", "grammar", "past-participle-usage.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected filename with spaces converted to hyphens")
	}
}

func TestLoadGrammarFiles_SortedAlpha(t *testing.T) {
	root := t.TempDir()
	makeGrammarFile(t, root, "verbs.md", "# Verbs")
	makeGrammarFile(t, root, "articles.md", "# Articles")
	makeGrammarFile(t, root, "nouns.md", "# Nouns")

	ll := NewLanguageLearning()
	ll.vaultRoot = root
	ll.loadGrammarFiles()

	if len(ll.grammarFiles) != 3 {
		t.Fatalf("expected 3 grammar files, got %d", len(ll.grammarFiles))
	}
	expected := []string{"articles.md", "nouns.md", "verbs.md"}
	for i, name := range expected {
		if ll.grammarFiles[i] != name {
			t.Errorf("grammarFiles[%d] = %q, want %q", i, ll.grammarFiles[i], name)
		}
	}
}

func TestLoadGrammarFiles_IgnoresNonMD(t *testing.T) {
	root := t.TempDir()
	makeGrammarFile(t, root, "notes.md", "# Notes")
	dir := filepath.Join(root, "Languages", "grammar")
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not md"), 0644)

	ll := NewLanguageLearning()
	ll.vaultRoot = root
	ll.loadGrammarFiles()

	if len(ll.grammarFiles) != 1 {
		t.Errorf("expected 1 grammar file (.md only), got %d", len(ll.grammarFiles))
	}
}

func TestCreateGrammarViaOverlay(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "Languages", "grammar"), 0755)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)
	ll.tab = 2 // grammar tab

	// Press 'n' to start creating
	ll, _ = ll.updateGrammarTab(llKeyMsg("n"))
	if !ll.creatingNote {
		t.Fatal("expected creatingNote true")
	}

	// Type topic name
	for _, ch := range "Subjunctive" {
		ll, _ = ll.updateCreateGrammar(llKeyMsg(string(ch)))
	}
	if ll.newNoteName != "Subjunctive" {
		t.Errorf("expected newNoteName 'Subjunctive', got %q", ll.newNoteName)
	}

	// Press enter to move to language selector
	ll, _ = ll.updateCreateGrammar(llSpecialKeyMsg(tea.KeyEnter))
	if ll.addField != 1 {
		t.Errorf("expected addField 1 (language), got %d", ll.addField)
	}

	// Press enter to submit
	ll, _ = ll.updateCreateGrammar(llSpecialKeyMsg(tea.KeyEnter))
	if ll.creatingNote {
		t.Error("expected creatingNote false after submit")
	}

	// Verify file was created
	path := filepath.Join(root, "Languages", "grammar", "subjunctive.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected grammar note file to be created")
	}
}

// ---------------------------------------------------------------------------
// 6. Progress dashboard data aggregation
// ---------------------------------------------------------------------------

func TestDashboard_WordsByLanguage(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 60)
	ll.tab = 3

	// Count languages manually
	langCount := make(map[string]int)
	for _, v := range ll.vocab {
		langCount[v.Language]++
	}

	// We should have Spanish=2, French=1, German=1, Japanese=1
	if langCount["Spanish"] != 2 {
		t.Errorf("expected 2 Spanish words, got %d", langCount["Spanish"])
	}
	if langCount["French"] != 1 {
		t.Errorf("expected 1 French word, got %d", langCount["French"])
	}
	if langCount["German"] != 1 {
		t.Errorf("expected 1 German word, got %d", langCount["German"])
	}
	if langCount["Japanese"] != 1 {
		t.Errorf("expected 1 Japanese word, got %d", langCount["Japanese"])
	}

	// Verify dashboard renders without panic
	view := ll.viewDashboard(80)
	if !strings.Contains(view, "Dashboard") {
		t.Error("expected dashboard view to contain 'Dashboard'")
	}
	if !strings.Contains(view, "Summary") {
		t.Error("expected dashboard view to contain 'Summary'")
	}
}

func TestDashboard_TotalReviews(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 60)

	totalCorrect := 0
	for _, v := range ll.vocab {
		totalCorrect += v.Correct
	}
	// 0 + 3 + 6 + 9 + 15 = 33
	if totalCorrect != 33 {
		t.Errorf("expected total correct 33, got %d", totalCorrect)
	}
}

func TestDashboard_SuggestedReview_ExcludesMastered(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "mastered", Translation: "m", Language: "Spanish", Level: 5, LastReviewed: "2020-01-01", Correct: 100},
		{Word: "learn", Translation: "l", Language: "Spanish", Level: 1, LastReviewed: "2020-01-01", Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 60)

	// The dashboard view should mention the non-mastered word in suggestions
	view := ll.viewDashboard(80)
	if strings.Contains(view, "All words are mastered") {
		t.Error("should not show 'all mastered' when there are unmastered words")
	}
}

// ---------------------------------------------------------------------------
// 7. Streak tracking
// ---------------------------------------------------------------------------

func TestStreak_IncreasesOnCorrect(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
		{Word: "b", Translation: "2", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	// Answer first card correctly
	idx0 := ll.practiceQueue[0]
	ll.practiceInput = ll.vocab[idx0].Translation
	ll.submitAnswer()
	if ll.streak != 1 {
		t.Errorf("expected streak 1, got %d", ll.streak)
	}

	// Move to next card
	ll.nextCard()

	// Answer second card correctly
	if ll.practiceIdx < len(ll.practiceQueue) {
		idx1 := ll.practiceQueue[ll.practiceIdx]
		ll.practiceInput = ll.vocab[idx1].Translation
		ll.submitAnswer()
		if ll.streak != 2 {
			t.Errorf("expected streak 2, got %d", ll.streak)
		}
	}
}

func TestStreak_ResetsOnWrong(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
		{Word: "b", Translation: "2", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	// Get first correct to build streak
	idx0 := ll.practiceQueue[0]
	ll.practiceInput = ll.vocab[idx0].Translation
	ll.submitAnswer()
	if ll.streak != 1 {
		t.Fatalf("setup: expected streak 1, got %d", ll.streak)
	}

	ll.nextCard()

	// Now answer wrong
	if ll.practiceIdx < len(ll.practiceQueue) {
		ll.practiceInput = "completely wrong"
		ll.submitAnswer()
		if ll.streak != 0 {
			t.Errorf("expected streak 0 after wrong answer, got %d", ll.streak)
		}
	}
}

func TestSessionCounters(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, LastReviewed: "", Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	ll.startPractice()

	ll.practiceInput = "1"
	ll.submitAnswer()

	if ll.sessionTotal != 1 {
		t.Errorf("expected sessionTotal 1, got %d", ll.sessionTotal)
	}
	if ll.sessionRight != 1 {
		t.Errorf("expected sessionRight 1, got %d", ll.sessionRight)
	}
}

// ---------------------------------------------------------------------------
// 8. Level distribution
// ---------------------------------------------------------------------------

func TestLevelDistribution(t *testing.T) {
	vocab := sampleVocab()

	levels := [6]int{}
	for _, v := range vocab {
		if v.Level >= 1 && v.Level <= 5 {
			levels[v.Level]++
		}
	}

	// sampleVocab: Level 1=1, Level 2=1, Level 3=1, Level 4=1, Level 5=1
	for i := 1; i <= 5; i++ {
		if levels[i] != 1 {
			t.Errorf("expected 1 word at level %d, got %d", i, levels[i])
		}
	}
}

func TestLevelDistribution_MultipleAtSameLevel(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 1, Correct: 0},
		{Word: "b", Translation: "2", Language: "Spanish", Level: 1, Correct: 0},
		{Word: "c", Translation: "3", Language: "Spanish", Level: 1, Correct: 0},
		{Word: "d", Translation: "4", Language: "Spanish", Level: 3, Correct: 5},
		{Word: "e", Translation: "5", Language: "Spanish", Level: 5, Correct: 20},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	levels := [6]int{}
	for _, v := range ll.vocab {
		if v.Level >= 1 && v.Level <= 5 {
			levels[v.Level]++
		}
	}

	if levels[1] != 3 {
		t.Errorf("expected 3 words at level 1, got %d", levels[1])
	}
	if levels[3] != 1 {
		t.Errorf("expected 1 word at level 3, got %d", levels[3])
	}
	if levels[5] != 1 {
		t.Errorf("expected 1 word at level 5, got %d", levels[5])
	}
}

func TestPracticeQueueWeighting(t *testing.T) {
	// Verify that lower-level words get higher weight in the pre-dedup queue
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "beginner", Translation: "b", Language: "Spanish", Level: 1, Correct: 0},
		{Word: "mastered", Translation: "m", Language: "Spanish", Level: 5, Correct: 0},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	// Weight for level 1 = 6-1 = 5; weight for level 5 = 6-5 = 1
	// After dedup, both should appear at least once, but we verify queue length
	// is based on weights before dedup
	ll.startPractice()
	if len(ll.practiceQueue) < 2 {
		t.Errorf("expected at least 2 entries in queue, got %d", len(ll.practiceQueue))
	}
}

// ---------------------------------------------------------------------------
// 9. Edge cases
// ---------------------------------------------------------------------------

func TestEdge_EmptyVocab_LoadReturnsNil(t *testing.T) {
	root := t.TempDir()
	// No Languages directory at all

	ll := NewLanguageLearning()
	ll.Open(root)

	if ll.vocab != nil {
		t.Errorf("expected nil vocab when no file exists, got %v", ll.vocab)
	}
	if len(ll.vocab) != 0 {
		t.Errorf("expected 0 vocab entries, got %d", len(ll.vocab))
	}
}

func TestEdge_EmptyVocab_DashboardRendersGracefully(t *testing.T) {
	root := t.TempDir()

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 60)
	ll.tab = 3

	view := ll.viewDashboard(80)
	if !strings.Contains(view, "No vocabulary data yet") {
		t.Error("expected empty dashboard message")
	}
}

func TestEdge_EmptyVocab_PracticeViewShowsMessage(t *testing.T) {
	root := t.TempDir()

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)
	ll.tab = 1

	view := ll.viewPractice(80)
	if !strings.Contains(view, "No vocabulary to practice") {
		t.Error("expected 'No vocabulary to practice' message")
	}
}

func TestEdge_AllMastered_DashboardShowsMastered(t *testing.T) {
	root := t.TempDir()
	entries := []VocabEntry{
		{Word: "a", Translation: "1", Language: "Spanish", Level: 5, LastReviewed: "2026-01-01", Correct: 50},
		{Word: "b", Translation: "2", Language: "Spanish", Level: 5, LastReviewed: "2026-01-01", Correct: 50},
	}
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 60)

	view := ll.viewDashboard(80)
	if !strings.Contains(view, "All words are mastered") {
		t.Error("expected 'All words are mastered' when all words are level 5")
	}
}

func TestEdge_NewLanguageWithNoEntries(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, nil)

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	// Vocab tab shows empty message
	view := ll.viewVocabulary(80)
	if !strings.Contains(view, "No vocabulary entries yet") {
		t.Error("expected empty vocabulary message")
	}

	// Grammar tab with no files
	ll.tab = 2
	view = ll.viewGrammar(80)
	if !strings.Contains(view, "No grammar notes found") {
		t.Error("expected no grammar notes message")
	}
}

func TestEdge_LoadVocabulary_MalformedLines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Languages")
	_ = os.MkdirAll(dir, 0755)

	content := `---
type: vocabulary
---
# Vocabulary

| Word | Translation | Language | Level | Last Reviewed | Correct |
|------|-------------|----------|-------|---------------|--------|
| good | bueno | Spanish | 2 | 2026-01-01 | 3 |
| bad line with too few fields |
| another | line | here |
| fine | bien | French | 1 | 2026-01-02 | 0 |
`
	_ = os.WriteFile(filepath.Join(dir, "vocabulary.md"), []byte(content), 0644)

	ll := NewLanguageLearning()
	ll.Open(root)

	// Should only load the two valid lines (7+ pipe-separated parts)
	if len(ll.vocab) != 2 {
		t.Errorf("expected 2 valid vocab entries, got %d", len(ll.vocab))
	}
	if ll.vocab[0].Word != "good" {
		t.Errorf("expected first word 'good', got %q", ll.vocab[0].Word)
	}
	if ll.vocab[1].Word != "fine" {
		t.Errorf("expected second word 'fine', got %q", ll.vocab[1].Word)
	}
}

func TestEdge_NoGrammarDir(t *testing.T) {
	root := t.TempDir()
	// No grammar dir created

	ll := NewLanguageLearning()
	ll.vaultRoot = root
	ll.loadGrammarFiles()

	if ll.grammarFiles != nil {
		t.Errorf("expected nil grammarFiles when directory missing, got %v", ll.grammarFiles)
	}
}

func TestTabNavigation(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	if ll.tab != 0 {
		t.Fatalf("expected initial tab 0, got %d", ll.tab)
	}

	// Tab cycles through
	ll, _ = ll.Update(llSpecialKeyMsg(tea.KeyTab))
	if ll.tab != 1 {
		t.Errorf("expected tab 1 after tab, got %d", ll.tab)
	}

	ll, _ = ll.Update(llSpecialKeyMsg(tea.KeyTab))
	if ll.tab != 2 {
		t.Errorf("expected tab 2, got %d", ll.tab)
	}

	ll, _ = ll.Update(llSpecialKeyMsg(tea.KeyTab))
	if ll.tab != 3 {
		t.Errorf("expected tab 3, got %d", ll.tab)
	}

	ll, _ = ll.Update(llSpecialKeyMsg(tea.KeyTab))
	if ll.tab != 0 {
		t.Errorf("expected tab 0 after wrap, got %d", ll.tab)
	}

	// Number key navigation
	ll, _ = ll.Update(llKeyMsg("3"))
	if ll.tab != 2 {
		t.Errorf("expected tab 2 from key '3', got %d", ll.tab)
	}
}

func TestEscClosesOverlay(t *testing.T) {
	root := t.TempDir()

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	if !ll.IsActive() {
		t.Fatal("expected active after Open")
	}

	ll, _ = ll.Update(llSpecialKeyMsg(tea.KeyEsc))
	if ll.IsActive() {
		t.Error("expected inactive after Esc")
	}
}

func TestUpdateWhenInactive_IsNoop(t *testing.T) {
	ll := NewLanguageLearning()
	// Not active

	ll2, cmd := ll.Update(llKeyMsg("n"))
	if cmd != nil {
		t.Error("expected nil cmd when inactive")
	}
	if ll2.addingWord {
		t.Error("expected no state change when inactive")
	}
}

func TestContentHeight(t *testing.T) {
	ll := NewLanguageLearning()

	ll.height = 30
	if h := ll.contentHeight(); h != 16 { // 30 - 14
		t.Errorf("expected contentHeight 16, got %d", h)
	}

	ll.height = 10
	if h := ll.contentHeight(); h != 5 { // max(10-14, 5) = 5
		t.Errorf("expected contentHeight 5 (minimum), got %d", h)
	}
}

func TestTruncStr(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"abcdef", 3, "abc"},
		{"ab", 3, "ab"},
		{"a", 1, "a"},
	}

	for _, tt := range tests {
		got := truncStr(tt.input, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncStr(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
		}
	}
}

func TestHandleTextInput_Backspace(t *testing.T) {
	ll := NewLanguageLearning()

	result := ll.handleTextInput("hello", tea.KeyMsg{Type: tea.KeyBackspace})
	if result != "hell" {
		t.Errorf("expected 'hell' after backspace, got %q", result)
	}

	result = ll.handleTextInput("", tea.KeyMsg{Type: tea.KeyBackspace})
	if result != "" {
		t.Errorf("expected empty string after backspace on empty, got %q", result)
	}
}

func TestHandleTextInput_CharAppend(t *testing.T) {
	ll := NewLanguageLearning()

	result := ll.handleTextInput("hel", llKeyMsg("l"))
	if result != "hell" {
		t.Errorf("expected 'hell', got %q", result)
	}
}

func TestVocabCursorNavigation(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(100, 40)

	if ll.vocabCursor != 0 {
		t.Fatalf("expected initial cursor 0")
	}

	ll, _ = ll.updateVocabTab(llKeyMsg("j"))
	if ll.vocabCursor != 1 {
		t.Errorf("expected cursor 1, got %d", ll.vocabCursor)
	}

	ll, _ = ll.updateVocabTab(llKeyMsg("k"))
	if ll.vocabCursor != 0 {
		t.Errorf("expected cursor 0, got %d", ll.vocabCursor)
	}

	// Can't go above 0
	ll, _ = ll.updateVocabTab(llKeyMsg("k"))
	if ll.vocabCursor != 0 {
		t.Errorf("expected cursor 0 at boundary, got %d", ll.vocabCursor)
	}
}

func TestResetTabState(t *testing.T) {
	ll := NewLanguageLearning()
	ll.vocabCursor = 5
	ll.vocabScroll = 3
	ll.grammarCursor = 2
	ll.grammarScroll = 1
	ll.dashScroll = 4
	ll.grammarView = "some content"
	ll.addingWord = true
	ll.creatingNote = true
	ll.practicing = true

	ll.resetTabState()

	if ll.vocabCursor != 0 || ll.vocabScroll != 0 {
		t.Error("expected vocab cursor/scroll reset")
	}
	if ll.grammarCursor != 0 || ll.grammarScroll != 0 {
		t.Error("expected grammar cursor/scroll reset")
	}
	if ll.dashScroll != 0 {
		t.Error("expected dashScroll reset")
	}
	if ll.grammarView != "" {
		t.Error("expected grammarView cleared")
	}
	if ll.addingWord || ll.creatingNote || ll.practicing {
		t.Error("expected addingWord, creatingNote, practicing all false")
	}
}

func TestSaveAndReloadVocabulary(t *testing.T) {
	root := t.TempDir()
	entries := sampleVocab()
	makeVocabFile(t, root, entries)

	ll := NewLanguageLearning()
	ll.Open(root)

	if len(ll.vocab) != len(entries) {
		t.Fatalf("expected %d entries after load, got %d", len(entries), len(ll.vocab))
	}

	// Modify and save
	ll.vocab[0].Correct = 99
	ll.saveVocabulary()

	// Reload
	ll2 := NewLanguageLearning()
	ll2.Open(root)

	if ll2.vocab[0].Correct != 99 {
		t.Errorf("expected correct 99 after save/reload, got %d", ll2.vocab[0].Correct)
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	root := t.TempDir()
	makeVocabFile(t, root, sampleVocab())
	makeGrammarFile(t, root, "test.md", "# Test")

	ll := NewLanguageLearning()
	ll.Open(root)
	ll.SetSize(120, 40)

	// Test each tab renders without panic
	for tab := 0; tab < 4; tab++ {
		ll.tab = tab
		output := ll.View()
		if output == "" {
			t.Errorf("tab %d: expected non-empty view output", tab)
		}
	}
}
