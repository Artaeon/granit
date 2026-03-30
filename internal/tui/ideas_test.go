package tui

import "testing"

func TestIdeaScore(t *testing.T) {
	tests := []struct {
		impact, effort int
		wantAbove      float64
	}{
		{5, 1, 4.0},  // high impact, low effort = great
		{1, 5, 0.1},  // low impact, high effort = poor
		{3, 3, 0.9},  // balanced
		{0, 0, -0.1}, // no rating = 0
	}
	for _, tc := range tests {
		idea := Idea{Impact: tc.impact, Effort: tc.effort}
		score := idea.Score()
		if score <= tc.wantAbove {
			t.Errorf("Score(impact=%d, effort=%d) = %.2f, want > %.2f", tc.impact, tc.effort, score, tc.wantAbove)
		}
	}
}

func TestIdeasBoardNextID(t *testing.T) {
	ib := &IdeasBoard{
		ideas: []Idea{{ID: "I001"}, {ID: "I003"}},
	}
	got := ib.nextID()
	if got != "I004" {
		t.Errorf("nextID() = %q, want %q", got, "I004")
	}
}

func TestIdeasBoardNextID_Empty(t *testing.T) {
	ib := &IdeasBoard{}
	got := ib.nextID()
	if got != "I001" {
		t.Errorf("nextID() = %q, want %q", got, "I001")
	}
}

func TestIdeasInStage(t *testing.T) {
	ib := &IdeasBoard{
		ideas: []Idea{
			{ID: "I001", Stage: IdeaInbox},
			{ID: "I002", Stage: IdeaExploring},
			{ID: "I003", Stage: IdeaInbox},
			{ID: "I004", Stage: IdeaDone},
		},
	}
	inbox := ib.ideasInStage(IdeaInbox)
	if len(inbox) != 2 {
		t.Errorf("inbox count = %d, want 2", len(inbox))
	}
	done := ib.ideasInStage(IdeaDone)
	if len(done) != 1 {
		t.Errorf("done count = %d, want 1", len(done))
	}
}
