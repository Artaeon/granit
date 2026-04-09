package tui

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SM-2 ReviewCard
// ---------------------------------------------------------------------------

func TestReviewCard_FailResets(t *testing.T) {
	card := &Flashcard{Interval: 10, EaseFactor: 2.5, Reps: 5}
	ReviewCard(card, 1) // quality 1 = fail

	if card.Interval != 1 {
		t.Errorf("expected interval reset to 1, got %f", card.Interval)
	}
	if card.Reps != 0 {
		t.Errorf("expected reps reset to 0, got %d", card.Reps)
	}
	if card.Lapses != 1 {
		t.Errorf("expected 1 lapse, got %d", card.Lapses)
	}
}

func TestReviewCard_HardIncreasesSlowly(t *testing.T) {
	card := &Flashcard{Interval: 5, EaseFactor: 2.5, Reps: 3}
	ReviewCard(card, 3) // quality 3 = hard

	// Interval should increase by factor 1.2
	expected := 5.0 * 1.2
	if card.Interval < expected-0.01 || card.Interval > expected+0.01 {
		t.Errorf("expected interval ~%f, got %f", expected, card.Interval)
	}
	if card.Reps != 4 {
		t.Errorf("expected reps=4, got %d", card.Reps)
	}
}

func TestReviewCard_GoodUsesEaseFactor(t *testing.T) {
	card := &Flashcard{Interval: 5, EaseFactor: 2.5, Reps: 3}
	ReviewCard(card, 4) // quality 4 = good

	// Interval should increase by ease factor
	if card.Interval < 12 || card.Interval > 13 {
		t.Errorf("expected interval ~12.5, got %f", card.Interval)
	}
}

func TestReviewCard_EasyBonusMultiplier(t *testing.T) {
	card := &Flashcard{Interval: 5, EaseFactor: 2.5, Reps: 3}
	ReviewCard(card, 5) // quality 5 = easy

	// Interval should increase by ease factor * 1.3
	expected := 5.0 * 2.5 * 1.3
	if card.Interval < expected-1 || card.Interval > expected+1 {
		t.Errorf("expected interval ~%f, got %f", expected, card.Interval)
	}
}

func TestReviewCard_EaseFactorFloor(t *testing.T) {
	card := &Flashcard{Interval: 1, EaseFactor: 1.3, Reps: 0}
	// Multiple failures should not drop ease factor below 1.3
	for i := 0; i < 10; i++ {
		ReviewCard(card, 0)
	}
	if card.EaseFactor < 1.3 {
		t.Errorf("ease factor should not drop below 1.3, got %f", card.EaseFactor)
	}
}

func TestReviewCard_QualityClamped(t *testing.T) {
	card := &Flashcard{Interval: 1, EaseFactor: 2.5}
	// Quality out of range should be clamped
	ReviewCard(card, -5)
	if card.Reps != 0 {
		t.Error("negative quality should be treated as fail")
	}

	card2 := &Flashcard{Interval: 1, EaseFactor: 2.5}
	ReviewCard(card2, 100)
	if card2.Reps != 1 {
		t.Error("quality > 5 should be clamped to 5 (easy)")
	}
}

func TestReviewCard_DueIsSet(t *testing.T) {
	card := &Flashcard{Interval: 1, EaseFactor: 2.5}
	before := time.Now()
	ReviewCard(card, 4)
	if card.Due.Before(before) {
		t.Error("due date should be in the future")
	}
}

func TestReviewCard_ZeroIntervalGuard(t *testing.T) {
	card := &Flashcard{Interval: 0, EaseFactor: 2.5}
	ReviewCard(card, 4) // good
	if card.Interval < 1 {
		t.Errorf("interval should be at least 1, got %f", card.Interval)
	}
}

// ---------------------------------------------------------------------------
// ExtractCards
// ---------------------------------------------------------------------------

func TestExtractCards_QAPairs(t *testing.T) {
	content := `# Study Notes

Q: What is Go?
A: A programming language by Google

Q: What is Rust?
A: A systems programming language
`
	cards := ExtractCards("notes/study.md", content)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if cards[0].Question != "What is Go?" {
		t.Errorf("expected 'What is Go?', got %q", cards[0].Question)
	}
	if cards[0].Answer != "A programming language by Google" {
		t.Errorf("wrong answer: %q", cards[0].Answer)
	}
}

func TestExtractCards_HeadingSections(t *testing.T) {
	content := `# Notes

## What is TDD?

Test-driven development means writing tests before code.

## What is BDD?

Behavior-driven development focuses on user behavior.
`
	cards := ExtractCards("notes/dev.md", content)
	if len(cards) < 2 {
		t.Fatalf("expected at least 2 cards from headings, got %d", len(cards))
	}
}

func TestExtractCards_EmptyContent(t *testing.T) {
	cards := ExtractCards("empty.md", "")
	if len(cards) != 0 {
		t.Errorf("expected 0 cards for empty content, got %d", len(cards))
	}
}

func TestExtractCards_QWithoutA(t *testing.T) {
	content := "Q: Orphaned question without answer\n"
	cards := ExtractCards("test.md", content)
	// Should not create a card without an answer
	for _, c := range cards {
		if c.Question == "Orphaned question without answer" {
			t.Error("should not create card without answer")
		}
	}
}

func TestExtractCards_CaseInsensitiveQA(t *testing.T) {
	content := "q: lowercase question\na: lowercase answer\n"
	cards := ExtractCards("test.md", content)
	found := false
	for _, c := range cards {
		if c.Question == "lowercase question" {
			found = true
		}
	}
	if !found {
		t.Error("should parse lowercase q:/a: pairs")
	}
}

func TestExtractCards_DefaultEaseFactor(t *testing.T) {
	content := "Q: Test\nA: Answer\n"
	cards := ExtractCards("test.md", content)
	if len(cards) != 1 {
		t.Fatal("expected 1 card")
	}
	if cards[0].EaseFactor != 2.5 {
		t.Errorf("expected default ease factor 2.5, got %f", cards[0].EaseFactor)
	}
}

func TestCardID_Deterministic(t *testing.T) {
	id1 := cardID("question", "source")
	id2 := cardID("question", "source")
	if id1 != id2 {
		t.Error("cardID should be deterministic")
	}
}

func TestCardID_Unique(t *testing.T) {
	id1 := cardID("question1", "source")
	id2 := cardID("question2", "source")
	if id1 == id2 {
		t.Error("different questions should produce different IDs")
	}
}
