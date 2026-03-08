package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 1. Initialization
// ---------------------------------------------------------------------------

func TestNewWritingCoach_Defaults(t *testing.T) {
	wc := NewWritingCoach()
	if wc.IsActive() {
		t.Error("new WritingCoach should not be active")
	}
	if wc.phase != 0 {
		t.Errorf("expected phase 0, got %d", wc.phase)
	}
	if wc.analysisMode != 0 {
		t.Errorf("expected default analysisMode 0, got %d", wc.analysisMode)
	}
	if wc.feedback != nil {
		t.Error("feedback should be nil on init")
	}
}

func TestWritingCoach_Open_ActivatesAndSetsDefaults(t *testing.T) {
	wc := NewWritingCoach()
	vault := t.TempDir()

	wc.Open(vault, "Hello world.", "notes/test.md", "local", "", "", "", "")

	if !wc.IsActive() {
		t.Error("Open should activate the coach")
	}
	if wc.phase != 0 {
		t.Errorf("expected phase 0 (setup), got %d", wc.phase)
	}
	if wc.analysisMode != 3 {
		t.Errorf("expected analysisMode 3 (full review), got %d", wc.analysisMode)
	}
	if wc.noteContent != "Hello world." {
		t.Errorf("noteContent mismatch: %q", wc.noteContent)
	}
	if wc.notePath != "notes/test.md" {
		t.Errorf("notePath mismatch: %q", wc.notePath)
	}
	if wc.cursor != 0 || wc.scroll != 0 {
		t.Error("cursor and scroll should be 0 after Open")
	}
	if wc.hasResult {
		t.Error("hasResult should be false after Open")
	}
}

func TestWritingCoach_SetSize(t *testing.T) {
	wc := NewWritingCoach()
	wc.SetSize(120, 40)
	if wc.width != 120 || wc.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", wc.width, wc.height)
	}
}

func TestWritingCoach_GetSuggestion_ConsumedOnce(t *testing.T) {
	wc := NewWritingCoach()
	wc.selectedSuggestion = "Rewrite this sentence."

	s, ok := wc.GetSuggestion()
	if !ok || s != "Rewrite this sentence." {
		t.Errorf("first GetSuggestion: got %q, %v", s, ok)
	}

	s2, ok2 := wc.GetSuggestion()
	if ok2 || s2 != "" {
		t.Error("second GetSuggestion should return empty/false")
	}
}

// ---------------------------------------------------------------------------
// 2. Local fallback analysis
// ---------------------------------------------------------------------------

// --- Long sentence detection ---

func TestLocalAnalysis_LongSentenceDetection(t *testing.T) {
	wc := NewWritingCoach()
	// Build a sentence with >40 words
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	longSentence := strings.Join(words, " ") + "."

	wc.noteContent = longSentence
	wc.analysisMode = 0 // clarity
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if fb.Category == "clarity" && strings.Contains(fb.Issue, "45 words") {
			found = true
			if fb.Severity != 2 {
				t.Errorf("expected severity 2 (moderate), got %d", fb.Severity)
			}
		}
	}
	if !found {
		t.Error("expected long sentence feedback for 45-word sentence")
	}
}

func TestLocalAnalysis_NormalSentenceNoLongWarning(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = "This is a short sentence. And so is this one."
	wc.analysisMode = 0
	wc.runLocalAnalysis()

	for _, fb := range wc.feedback {
		if strings.Contains(fb.Issue, "words, which may be hard") {
			t.Error("should not flag normal-length sentences")
		}
	}
}

// --- Passive voice detection ---

func TestLocalAnalysis_PassiveVoiceDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool // expect passive voice feedback
	}{
		{
			name:    "was + past participle",
			content: "The ball was kicked across the field.",
			want:    true,
		},
		{
			name:    "were + past participle",
			content: "The documents were shredded by the team.",
			want:    true,
		},
		{
			name:    "is + past participle",
			content: "The report is written every week by the manager.",
			want:    true,
		},
		{
			name:    "been + past participle",
			content: "The task has been completed successfully by everyone involved in the project.",
			want:    true,
		},
		{
			name:    "active voice - no flag",
			content: "The team completed the project successfully and delivered it on time.",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wc := NewWritingCoach()
			wc.noteContent = tt.content
			wc.analysisMode = 0 // clarity
			wc.runLocalAnalysis()

			found := false
			for _, fb := range wc.feedback {
				if strings.Contains(fb.Issue, "Possible passive voice") {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("passive voice detection: got found=%v, want %v", found, tt.want)
			}
		})
	}
}

// --- Readability / structure scoring ---

func TestLocalAnalysis_LongParagraphDetection(t *testing.T) {
	wc := NewWritingCoach()
	// Build a paragraph with >200 words
	words := make([]string, 210)
	for i := range words {
		words[i] = "text"
	}
	longParagraph := strings.Join(words, " ") + "."

	wc.noteContent = longParagraph
	wc.analysisMode = 1 // structure
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if fb.Category == "structure" && strings.Contains(fb.Issue, "210 words") {
			found = true
			if fb.Severity != 2 {
				t.Errorf("expected severity 2, got %d", fb.Severity)
			}
		}
	}
	if !found {
		t.Error("expected long paragraph feedback for >200 words")
	}
}

func TestLocalAnalysis_NoHeadingsLongNote(t *testing.T) {
	wc := NewWritingCoach()
	// 550 words without any headings
	words := make([]string, 550)
	for i := range words {
		words[i] = "content"
	}
	wc.noteContent = strings.Join(words, " ") + "."
	wc.analysisMode = 1 // structure
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if fb.Category == "structure" && strings.Contains(fb.Issue, "No headings found") {
			found = true
			if fb.Severity != 3 {
				t.Errorf("expected severity 3 (major), got %d", fb.Severity)
			}
		}
	}
	if !found {
		t.Error("expected 'No headings found' feedback for 550-word note without headings")
	}
}

func TestLocalAnalysis_MissingHeadingsGap(t *testing.T) {
	wc := NewWritingCoach()
	// First heading, then >500 words without another heading
	words := make([]string, 510)
	for i := range words {
		words[i] = "filler"
	}
	wc.noteContent = "# Introduction\n\n" + strings.Join(words, " ") + "."
	wc.analysisMode = 1 // structure
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if fb.Category == "structure" && strings.Contains(fb.Issue, "words since the last heading") {
			found = true
		}
	}
	if !found {
		t.Error("expected feedback for >500 words gap between headings")
	}
}

func TestLocalAnalysis_AdverbDensity(t *testing.T) {
	wc := NewWritingCoach()
	// Create content with high adverb density (>4%).
	// We need adverb count / total words > 0.04, with total > 50.
	wc.noteContent = "She quickly ran to the store and happily bought items for the house. " +
		"He slowly walked back home and carefully placed them on the table for dinner. " +
		"They eagerly started the work and gracefully finished the task in the afternoon. " +
		"We boldly went forward and steadily made good progress on the big project for the team."
	wc.analysisMode = 2 // style
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if fb.Category == "style" && strings.Contains(fb.Issue, "adverb density") {
			found = true
			if fb.Severity != 1 {
				t.Errorf("expected severity 1, got %d", fb.Severity)
			}
		}
	}
	if !found {
		t.Error("expected adverb density feedback")
	}
}

func TestLocalAnalysis_WeakOpeners(t *testing.T) {
	wc := NewWritingCoach()
	// More than 1/3 of sentences start with weak openers
	wc.noteContent = "There is a problem here. " +
		"It seems like a big deal. " +
		"This is really important. " +
		"There are many reasons why. " +
		"It looks like rain today. " +
		"This needs to be fixed soon."
	wc.analysisMode = 2 // style
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if fb.Category == "style" && strings.Contains(fb.Issue, "weak openers") {
			found = true
		}
	}
	if !found {
		t.Error("expected weak openers feedback")
	}
}

// ---------------------------------------------------------------------------
// 3. Analysis mode selection
// ---------------------------------------------------------------------------

func TestLocalAnalysis_ClarityModeOnly(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = buildLongMixedContent()
	wc.analysisMode = 0 // clarity only
	wc.runLocalAnalysis()

	for _, fb := range wc.feedback {
		if fb.Category == "structure" {
			t.Error("clarity mode should not produce structure feedback")
		}
		// style feedback should not appear either (adverbs, weak openers)
		if fb.Category == "style" && fb.Issue != "No significant issues detected." {
			t.Error("clarity mode should not produce style feedback")
		}
	}
}

func TestLocalAnalysis_StructureModeOnly(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = buildLongMixedContent()
	wc.analysisMode = 1 // structure only
	wc.runLocalAnalysis()

	for _, fb := range wc.feedback {
		if fb.Category == "clarity" {
			t.Error("structure mode should not produce clarity feedback")
		}
		if fb.Category == "style" && fb.Issue != "No significant issues detected." {
			t.Error("structure mode should not produce style feedback")
		}
	}
}

func TestLocalAnalysis_StyleModeOnly(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = buildLongMixedContent()
	wc.analysisMode = 2 // style only
	wc.runLocalAnalysis()

	for _, fb := range wc.feedback {
		if fb.Category == "clarity" {
			t.Error("style mode should not produce clarity feedback")
		}
		if fb.Category == "structure" {
			t.Error("style mode should not produce structure feedback")
		}
	}
}

func TestLocalAnalysis_FullReviewIncludesAll(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = buildLongMixedContent()
	wc.analysisMode = 3 // full review
	wc.runLocalAnalysis()

	categories := make(map[string]bool)
	for _, fb := range wc.feedback {
		categories[fb.Category] = true
	}

	// Full review should have at least clarity and structure findings
	// because buildLongMixedContent has long sentences and no headings
	if !categories["clarity"] {
		t.Error("full review should include clarity feedback")
	}
	if !categories["structure"] {
		t.Error("full review should include structure feedback")
	}
}

func TestModeLabel(t *testing.T) {
	wc := NewWritingCoach()

	tests := []struct {
		mode int
		want string
	}{
		{0, "Clarity"},
		{1, "Structure"},
		{2, "Style"},
		{3, "Full Review"},
		{99, "Full Review"}, // default
	}

	for _, tt := range tests {
		wc.analysisMode = tt.mode
		got := wc.modeLabel()
		if got != tt.want {
			t.Errorf("modeLabel(%d) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Severity coding of feedback
// ---------------------------------------------------------------------------

func TestCoachSeverityLabel(t *testing.T) {
	tests := []struct {
		sev  int
		want string
	}{
		{1, "minor"},
		{2, "moderate"},
		{3, "major"},
		{0, "moderate"},  // default
		{99, "moderate"}, // default
	}
	for _, tt := range tests {
		got := coachSeverityLabel(tt.sev)
		if got != tt.want {
			t.Errorf("coachSeverityLabel(%d) = %q, want %q", tt.sev, got, tt.want)
		}
	}
}

func TestCoachSeverityColor(t *testing.T) {
	if coachSeverityColor(1) != green {
		t.Error("severity 1 should be green")
	}
	if coachSeverityColor(2) != yellow {
		t.Error("severity 2 should be yellow")
	}
	if coachSeverityColor(3) != red {
		t.Error("severity 3 should be red")
	}
	if coachSeverityColor(0) != yellow {
		t.Error("severity 0 (unknown) should default to yellow")
	}
}

func TestCoachCategoryIcon(t *testing.T) {
	cats := map[string]string{
		"clarity":   IconBotChar,
		"structure": IconEditChar,
		"style":     IconEditChar,
		"grammar":   IconEditChar,
		"tone":      IconBotChar,
		"unknown":   IconBotChar, // default
	}
	for cat, wantIcon := range cats {
		got := coachCategoryIcon(cat)
		if got != wantIcon {
			t.Errorf("coachCategoryIcon(%q) = %q, want %q", cat, got, wantIcon)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. Soul note persona support
// ---------------------------------------------------------------------------

func TestWritingCoach_Open_LoadsSoulNote(t *testing.T) {
	vault := t.TempDir()
	soulDir := filepath.Join(vault, ".granit")
	os.MkdirAll(soulDir, 0755)
	soulContent := "You are a cheerful writing mentor."
	os.WriteFile(filepath.Join(soulDir, "soul-note.md"), []byte(soulContent), 0644)

	wc := NewWritingCoach()
	wc.Open(vault, "Test content.", "test.md", "local", "", "", "", "")

	if !wc.hasSoulNote {
		t.Error("expected hasSoulNote=true when soul-note.md exists")
	}
	if wc.soulNote != soulContent {
		t.Errorf("soulNote = %q, want %q", wc.soulNote, soulContent)
	}
}

func TestWritingCoach_Open_NoSoulNote(t *testing.T) {
	vault := t.TempDir()

	wc := NewWritingCoach()
	wc.Open(vault, "Test content.", "test.md", "local", "", "", "", "")

	if wc.hasSoulNote {
		t.Error("expected hasSoulNote=false when soul-note.md doesn't exist")
	}
	if wc.soulNote != "" {
		t.Errorf("soulNote should be empty, got %q", wc.soulNote)
	}
}

func TestWritingCoach_BuildPrompt_IncludesSoulNote(t *testing.T) {
	wc := NewWritingCoach()
	wc.hasSoulNote = true
	wc.soulNote = "Be concise and direct."
	wc.noteContent = "Some text to analyze."
	wc.analysisMode = 3

	prompt := wc.buildPrompt()

	if !strings.Contains(prompt, "Be concise and direct.") {
		t.Error("prompt should include soul note persona")
	}
	if !strings.Contains(prompt, "Adopt this persona") {
		t.Error("prompt should instruct AI to adopt persona")
	}
}

func TestWritingCoach_BuildPrompt_ExcludesSoulNoteWhenDisabled(t *testing.T) {
	wc := NewWritingCoach()
	wc.hasSoulNote = false
	wc.soulNote = ""
	wc.noteContent = "Some text to analyze."
	wc.analysisMode = 3

	prompt := wc.buildPrompt()

	if strings.Contains(prompt, "Adopt this persona") {
		t.Error("prompt should not include persona instruction when soul note is disabled")
	}
}

func TestWritingCoach_BuildPrompt_TruncatesLongSoulNote(t *testing.T) {
	wc := NewWritingCoach()
	wc.hasSoulNote = true
	wc.soulNote = strings.Repeat("x", 2000) // >1500
	wc.noteContent = "Some text."
	wc.analysisMode = 0

	prompt := wc.buildPrompt()

	// The soul note should be truncated to 1500 chars
	if strings.Contains(prompt, strings.Repeat("x", 1501)) {
		t.Error("soul note should be truncated to 1500 chars in prompt")
	}
	if !strings.Contains(prompt, strings.Repeat("x", 1500)) {
		t.Error("prompt should contain the first 1500 chars of soul note")
	}
}

func TestWritingCoach_BuildPrompt_TruncatesLongContent(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = strings.Repeat("y", 5000) // >4000
	wc.analysisMode = 3

	prompt := wc.buildPrompt()

	if strings.Contains(prompt, strings.Repeat("y", 4001)) {
		t.Error("note content should be truncated to 4000 chars in prompt")
	}
}

func TestWritingCoach_BuildPrompt_ModeDescriptions(t *testing.T) {
	modes := []struct {
		mode int
		want string
	}{
		{0, "clarity"},
		{1, "structure"},
		{2, "style"},
		{3, "all aspects"},
	}

	for _, tt := range modes {
		wc := NewWritingCoach()
		wc.noteContent = "Test."
		wc.analysisMode = tt.mode

		prompt := wc.buildPrompt()
		if !strings.Contains(prompt, tt.want) {
			t.Errorf("mode %d: prompt should contain %q", tt.mode, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Edge cases
// ---------------------------------------------------------------------------

func TestLocalAnalysis_EmptyNote(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = ""
	wc.analysisMode = 3 // full review
	wc.runLocalAnalysis()

	// Should produce at least the encouraging "no issues" message
	if len(wc.feedback) == 0 {
		t.Error("expected at least one feedback item (encouragement) for empty note")
	}
	if wc.feedback[0].Issue != "No significant issues detected." {
		t.Errorf("unexpected issue for empty note: %q", wc.feedback[0].Issue)
	}
}

func TestLocalAnalysis_VeryShortNote(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = "Hello."
	wc.analysisMode = 3
	wc.runLocalAnalysis()

	// Very short note should not crash and should produce encouraging message
	if len(wc.feedback) == 0 {
		t.Fatal("expected feedback for very short note")
	}
	// A single word sentence "Hello." won't even be detected as a sentence
	// (coachSplitSentences requires >1 word), so no clarity issues
	for _, fb := range wc.feedback {
		if strings.Contains(fb.Issue, "words, which may be hard") {
			t.Error("should not flag very short content as having long sentences")
		}
	}
}

func TestLocalAnalysis_OnlyHeadings(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = "# Heading 1\n\n## Heading 2\n\n### Heading 3\n"
	wc.analysisMode = 3
	wc.runLocalAnalysis()

	// Headings-only note should not crash
	if len(wc.feedback) == 0 {
		t.Fatal("expected at least one feedback item")
	}
	// Should not flag structure issues — there ARE headings, just no body
	for _, fb := range wc.feedback {
		if strings.Contains(fb.Issue, "No headings found") {
			t.Error("should not flag 'No headings' when headings exist")
		}
	}
}

func TestLocalAnalysis_CodeBlocksExcluded(t *testing.T) {
	wc := NewWritingCoach()
	// Long code block should not be analyzed as prose
	longCode := strings.Repeat("word ", 50)
	wc.noteContent = "Some intro.\n\n```\n" + longCode + "\n```\n\nShort conclusion."
	wc.analysisMode = 0 // clarity
	wc.runLocalAnalysis()

	for _, fb := range wc.feedback {
		if strings.Contains(fb.Issue, "50 words") {
			t.Error("code block content should be excluded from sentence analysis")
		}
	}
}

func TestLocalAnalysis_RepeatedWords(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = "The implementation works great for everyone. The implementation is solid and reliable."
	wc.analysisMode = 0 // clarity
	wc.runLocalAnalysis()

	found := false
	for _, fb := range wc.feedback {
		if strings.Contains(fb.Issue, "repeated in consecutive") {
			found = true
		}
	}
	if !found {
		t.Error("expected feedback about repeated word 'implementation' in consecutive sentences")
	}
}

// ---------------------------------------------------------------------------
// parseCoachResponse tests
// ---------------------------------------------------------------------------

func TestParseCoachResponse_ValidLines(t *testing.T) {
	response := "clarity | 2 | Sentence is unclear | Rewrite for clarity | line 5\n" +
		"grammar | 1 | Missing comma | Add a comma after 'however' | line 12\n" +
		"style | 3 | Inconsistent tone | Maintain formal voice throughout | general\n"

	items := parseCoachResponse(response)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	if items[0].Category != "clarity" || items[0].Severity != 2 {
		t.Errorf("item 0: category=%q severity=%d", items[0].Category, items[0].Severity)
	}
	if items[1].Category != "grammar" || items[1].Severity != 1 {
		t.Errorf("item 1: category=%q severity=%d", items[1].Category, items[1].Severity)
	}
	if items[2].Category != "style" || items[2].Severity != 3 {
		t.Errorf("item 2: category=%q severity=%d", items[2].Category, items[2].Severity)
	}
	if items[2].LineRef != "general" {
		t.Errorf("item 2 lineRef=%q, want 'general'", items[2].LineRef)
	}
}

func TestParseCoachResponse_InvalidCategory(t *testing.T) {
	response := "foobar | 2 | Some issue | Some suggestion | line 1\n"
	items := parseCoachResponse(response)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	// Invalid categories default to "style"
	if items[0].Category != "style" {
		t.Errorf("invalid category should default to 'style', got %q", items[0].Category)
	}
}

func TestParseCoachResponse_EmptyIssue(t *testing.T) {
	response := "clarity | 2 |  | Some suggestion | line 1\n"
	items := parseCoachResponse(response)
	if len(items) != 0 {
		t.Errorf("empty issue should be skipped, got %d items", len(items))
	}
}

func TestParseCoachResponse_TooFewParts(t *testing.T) {
	response := "clarity | 2 | only three parts\n"
	items := parseCoachResponse(response)
	if len(items) != 0 {
		t.Errorf("lines with <4 parts should be skipped, got %d items", len(items))
	}
}

func TestParseCoachResponse_EmptyAndBlankLines(t *testing.T) {
	response := "\n\n  \nclarity | 1 | Issue | Suggestion | general\n  \n"
	items := parseCoachResponse(response)
	if len(items) != 1 {
		t.Fatalf("expected 1 item (skipping blank lines), got %d", len(items))
	}
}

func TestParseCoachResponse_FourPartLine(t *testing.T) {
	// 4-part line (no line ref)
	response := "tone | 1 | Informal tone | Consider formal language\n"
	items := parseCoachResponse(response)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].LineRef != "" {
		t.Errorf("expected empty lineRef, got %q", items[0].LineRef)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestCoachSplitSentences(t *testing.T) {
	text := "First sentence here. Second sentence there! Third one right?"
	sents := coachSplitSentences(text)
	if len(sents) != 3 {
		t.Errorf("expected 3 sentences, got %d: %v", len(sents), sents)
	}
}

func TestCoachSplitSentences_SkipsHeadings(t *testing.T) {
	text := "# My Heading\n\nThis is the body text."
	sents := coachSplitSentences(text)
	for _, s := range sents {
		if strings.Contains(s, "Heading") {
			t.Error("headings should be excluded from sentence splitting")
		}
	}
}

func TestCoachSplitSentences_SkipsCodeBlocks(t *testing.T) {
	text := "Intro sentence here.\n\n```\ncode block content here.\n```\n\nAfter code sentence."
	sents := coachSplitSentences(text)
	for _, s := range sents {
		if strings.Contains(s, "code block content") {
			t.Error("code block content should be excluded")
		}
	}
}

func TestCoachSplitSentences_SkipsSingleWordSentences(t *testing.T) {
	text := "Hello. This is valid."
	sents := coachSplitSentences(text)
	for _, s := range sents {
		if s == "Hello." {
			t.Error("single-word sentences should be filtered out")
		}
	}
}

func TestRemoveCodeBlocks(t *testing.T) {
	text := "Before code.\n\n```go\nfmt.Println(\"hello\")\n```\n\nAfter code."
	result := removeCodeBlocks(text)
	if strings.Contains(result, "Println") {
		t.Error("code block content should be removed")
	}
	if !strings.Contains(result, "Before code.") || !strings.Contains(result, "After code.") {
		t.Error("non-code content should be preserved")
	}
}

func TestSplitCoachParagraphs(t *testing.T) {
	text := "Paragraph one.\n\nParagraph two.\n\n```\ncode\n```\n\nParagraph three."
	paras := splitCoachParagraphs(text)
	// code-block-starting paragraphs should be excluded
	for _, p := range paras {
		if strings.HasPrefix(p, "```") {
			t.Error("code block paragraphs should be excluded")
		}
	}
	if len(paras) < 2 {
		t.Errorf("expected at least 2 paragraphs, got %d", len(paras))
	}
}

func TestStripCoachPunctuation(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello.", "hello"},
		{"(world)", "world"},
		{"**bold**", "bold"},
		{"clean", "clean"},
		{"", ""},
		{"...", ""},
	}
	for _, tt := range tests {
		got := stripCoachPunctuation(tt.in)
		if got != tt.want {
			t.Errorf("stripCoachPunctuation(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLooksLikeCoachPastParticiple(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"kicked", true},
		{"written", true},
		{"shown", true},
		{"kept", true},
		{"run", false},   // too short
		{"go", false},    // too short
		{"happy", false}, // no matching suffix
		{"the", false},   // too short
	}
	for _, tt := range tests {
		got := looksLikeCoachPastParticiple(tt.word)
		if got != tt.want {
			t.Errorf("looksLikeCoachPastParticiple(%q) = %v, want %v", tt.word, got, tt.want)
		}
	}
}

func TestTruncateCoach(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"hello", 0, "hello"}, // maxLen 0 returns as-is
	}
	for _, tt := range tests {
		got := truncateCoach(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateCoach(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func TestCoachSignificantWords(t *testing.T) {
	sentence := "The quick brown fox jumps over the lazy dog."
	words := coachSignificantWords(sentence)

	// "the" (len 3), "fox" (len 3), "dog" (len 3) should be excluded (len <= 4)
	if words["the"] || words["fox"] || words["dog"] {
		t.Error("short words should be excluded")
	}
	// "quick" (len 5), "brown" (len 5), "jumps" (len 5) should be included
	if !words["quick"] || !words["brown"] || !words["jumps"] {
		t.Error("words with len > 4 should be included")
	}
}

func TestWritingCoach_WordCount(t *testing.T) {
	wc := NewWritingCoach()
	wc.noteContent = "one two three four five"
	if wc.wordCount() != 5 {
		t.Errorf("wordCount() = %d, want 5", wc.wordCount())
	}

	wc.noteContent = ""
	if wc.wordCount() != 0 {
		t.Errorf("wordCount() on empty = %d, want 0", wc.wordCount())
	}
}

func TestNoteTitle(t *testing.T) {
	wc := NewWritingCoach()
	wc.notePath = "folder/subfolder/My Note.md"
	if wc.noteTitle() != "My Note" {
		t.Errorf("noteTitle() = %q, want 'My Note'", wc.noteTitle())
	}

	wc.notePath = "plain.md"
	if wc.noteTitle() != "plain" {
		t.Errorf("noteTitle() = %q, want 'plain'", wc.noteTitle())
	}
}

func TestOverlayWidth(t *testing.T) {
	wc := NewWritingCoach()

	// Small terminal
	wc.width = 60
	w := wc.overlayWidth()
	if w < 60 {
		t.Errorf("overlayWidth should be at least 60, got %d", w)
	}

	// Large terminal
	wc.width = 200
	w = wc.overlayWidth()
	if w > 100 {
		t.Errorf("overlayWidth should cap at 100, got %d", w)
	}

	// Normal terminal
	wc.width = 120
	w = wc.overlayWidth()
	expected := 120 * 2 / 3
	if w != expected {
		t.Errorf("overlayWidth = %d, want %d", w, expected)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildLongMixedContent produces content that triggers findings across
// all analysis modes: long sentences (clarity), no headings with >500 words
// (structure), and high adverb density (style).
func buildLongMixedContent() string {
	var sb strings.Builder

	// Long sentence for clarity detection (>40 words)
	longWords := make([]string, 45)
	for i := range longWords {
		longWords[i] = "something"
	}
	sb.WriteString(strings.Join(longWords, " "))
	sb.WriteString(". ")

	// Pad to >500 words for structure detection (no headings)
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf("Filler sentence number %d is here. ", i))
	}

	// Adverbs for style detection
	sb.WriteString("She quickly ran. He slowly walked. They happily worked. We carefully planned. ")
	sb.WriteString("It gradually improved. She frequently visited. He rarely complained. ")

	return sb.String()
}
