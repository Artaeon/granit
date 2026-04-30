package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/objects"
)

func TestRenderTranscriptMarkdown_IncludesGoalAndAnswer(t *testing.T) {
	preset := agents.Preset{ID: "p", Name: "Demo", Description: "desc"}
	tr := agents.Transcript{
		Goal:        "Investigate granit hub strip",
		FinalAnswer: "It works.",
		StartedAt:   time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
		EndedAt:     time.Date(2026, 4, 30, 10, 0, 5, 0, time.UTC),
		StoppedBy:   "answer",
		Steps: []agents.Step{
			{
				Number:  1,
				Thought: "Look up hub strip code",
				ToolCall: &agents.ToolCall{
					Tool: "search_vault",
					Args: map[string]string{"q": "hub strip"},
				},
				ToolResult: &agents.ToolResult{Output: "found 3 notes"},
			},
			{Number: 2, FinalAnswer: "It works."},
		},
	}
	body := renderTranscriptMarkdown(preset, tr, tr.Goal)
	for _, want := range []string{
		"# Demo — agent run",
		"**Goal:** Investigate granit hub strip",
		"## Answer",
		"It works.",
		"### Step 1",
		"search_vault",
		"hub strip",
		"found 3 notes",
		"### Step 2",
		"**Final answer reached.**",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\nfull body:\n%s", want, body)
		}
	}
}

func TestRenderTranscriptMarkdown_TruncatesLongObservations(t *testing.T) {
	preset := agents.Preset{ID: "p", Name: "P"}
	huge := strings.Repeat("X", 5000)
	tr := agents.Transcript{
		Goal: "g",
		Steps: []agents.Step{
			{Number: 1, ToolCall: &agents.ToolCall{Tool: "x"},
				ToolResult: &agents.ToolResult{Output: huge}},
		},
	}
	body := renderTranscriptMarkdown(preset, tr, "g")
	if !strings.Contains(body, "(truncated)") {
		t.Error("expected long observation to be truncated")
	}
	// Body shouldn't be 5kB+ when truncated.
	if len(body) > 2000 {
		t.Errorf("truncation didn't shrink body: %d bytes", len(body))
	}
}

func TestQueuePersist_BuildsAgentRunNote(t *testing.T) {
	a := AgentRunner{}
	a.registry = objects.NewRegistry()
	a.presets = []agents.Preset{
		{ID: "research-synthesizer", Name: "Research Synthesizer",
			Description: "research"},
	}
	a.cursor = 0
	a.startedAt = time.Now()
	a.aiConfig.Model = "qwen2.5:0.5b"

	tr := agents.Transcript{
		Goal:        "Test goal",
		FinalAnswer: "Done.",
		StartedAt:   a.startedAt,
		EndedAt:     a.startedAt.Add(time.Second),
		StoppedBy:   "answer",
	}
	a.queuePersist(tr)

	relPath, content, ok := a.GetPersistRequest()
	if !ok {
		t.Fatal("expected persist request after queue")
	}
	if !strings.HasPrefix(relPath, "Agents/") {
		t.Errorf("path should be in Agents/, got %q", relPath)
	}
	if !strings.Contains(relPath, "research-synthesizer") {
		t.Errorf("path should encode preset id, got %q", relPath)
	}
	for _, want := range []string{
		"type: agent_run",
		"preset: research-synthesizer",
		"goal: Test goal",
		"status: ok",
		"# Research Synthesizer — agent run",
		"## Answer",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q", want)
		}
	}
	// Consumed-once.
	if _, _, ok := a.GetPersistRequest(); ok {
		t.Error("persist request should be consumed-once")
	}
}

func TestQueuePersist_BudgetStatusPropagates(t *testing.T) {
	a := AgentRunner{}
	a.registry = objects.NewRegistry()
	a.presets = []agents.Preset{{ID: "p", Name: "P", Description: "x"}}
	a.cursor = 0
	tr := agents.Transcript{Goal: "g", StoppedBy: "budget"}
	a.queuePersist(tr)
	_, content, ok := a.GetPersistRequest()
	if !ok {
		t.Fatal("expected persist after queue")
	}
	if !strings.Contains(content, "status: budget") {
		t.Errorf("expected status: budget, got:\n%s", content)
	}
}
