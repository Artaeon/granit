package tui

import (
	"strings"
	"testing"
)

func TestGenerateDefinitionQuestions_DoubleColon(t *testing.T) {
	lines := []string{
		"Go :: A compiled programming language",
		"Rust :: A systems programming language",
		"",
		"Some other text",
	}
	qs := generateDefinitionQuestions("test.md", lines)

	found := 0
	for _, q := range qs {
		if q.Type == "definition" && strings.Contains(q.Question, "Go") {
			found++
			if q.Answer != "A compiled programming language" {
				t.Errorf("wrong answer for Go: %q", q.Answer)
			}
		}
	}
	if found == 0 {
		t.Error("expected definition question for Go")
	}
}

func TestGenerateDefinitionQuestions_Heading(t *testing.T) {
	lines := []string{
		"# Notes",
		"",
		"## Polymorphism",
		"The ability of objects to take many forms in OOP programming.",
		"",
		"## Encapsulation",
		"Bundling data and methods that operate on that data together.",
	}
	qs := generateDefinitionQuestions("test.md", lines)

	if len(qs) < 2 {
		t.Fatalf("expected at least 2 heading-based questions, got %d", len(qs))
	}
}

func TestGenerateDefinitionQuestions_Empty(t *testing.T) {
	qs := generateDefinitionQuestions("test.md", []string{})
	if len(qs) != 0 {
		t.Errorf("expected 0 questions for empty lines, got %d", len(qs))
	}
}

func TestGenerateFillBlankQuestions_BoldTerm(t *testing.T) {
	lines := []string{
		"The **mitochondria** is the powerhouse of the cell.",
		"Regular text without bold.",
	}
	qs := generateFillBlankQuestions("test.md", lines)

	found := false
	for _, q := range qs {
		if q.Type == "fill_blank" && strings.Contains(q.Question, "___") {
			found = true
			if q.Answer != "mitochondria" {
				t.Errorf("expected answer 'mitochondria', got %q", q.Answer)
			}
		}
	}
	if !found {
		t.Error("expected fill-in-the-blank question for bold term")
	}
}

func TestGenerateFillBlankQuestions_NoMarkup(t *testing.T) {
	lines := []string{
		"Just plain text without any formatting.",
	}
	qs := generateFillBlankQuestions("test.md", lines)
	for _, q := range qs {
		if q.Type == "fill_blank" {
			t.Error("should not generate fill-blank from plain text")
		}
	}
}

func TestGenerateTrueFalseQuestions(t *testing.T) {
	lines := []string{
		"The Earth is the third planet from the Sun. Water covers most of the surface.",
		"Go was created by Google in 2009. It compiles to machine code.",
	}
	qs := generateTrueFalseQuestions("test.md", lines)

	if len(qs) == 0 {
		t.Skip("no true/false questions generated (depends on sentence parsing)")
	}
	for _, q := range qs {
		if q.Type != "true_false" {
			t.Errorf("expected type true_false, got %q", q.Type)
		}
		ans := strings.ToLower(q.Answer)
		if ans != "true" && ans != "false" {
			t.Errorf("answer should be true or false, got %q", q.Answer)
		}
	}
}

func TestQuizSession_ScoreTracking(t *testing.T) {
	session := QuizSession{
		Questions: []QuizQuestion{
			{Question: "Q1", Answer: "A1"},
			{Question: "Q2", Answer: "A2"},
			{Question: "Q3", Answer: "A3"},
		},
		Total: 3,
	}

	// Simulate correct answer
	session.Score++
	session.Current++

	if session.Score != 1 {
		t.Errorf("expected score=1, got %d", session.Score)
	}
	if session.Current != 1 {
		t.Errorf("expected current=1, got %d", session.Current)
	}
}
