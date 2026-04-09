package tui

import "testing"

func TestJournalPrompts_Open(t *testing.T) {
	jp := JournalPrompts{}
	jp.allPrompts = curatedPrompts
	jp.Open(t.TempDir())

	if !jp.active {
		t.Error("expected active after Open")
	}
	if len(jp.prompts) == 0 {
		t.Error("expected prompts loaded after Open")
	}
}

func TestJournalPrompts_FilterByCategory(t *testing.T) {
	jp := JournalPrompts{}
	jp.allPrompts = curatedPrompts

	// All category
	jp.category = 0
	jp.filterPrompts()
	allCount := len(jp.prompts)
	if allCount == 0 {
		t.Fatal("expected prompts in all category")
	}

	// Gratitude category
	jp.category = 1
	jp.filterPrompts()
	gratCount := len(jp.prompts)
	if gratCount == 0 {
		t.Fatal("expected gratitude prompts")
	}
	if gratCount >= allCount {
		t.Error("filtered category should have fewer prompts than all")
	}
	for _, p := range jp.prompts {
		if p.Category != "Gratitude" {
			t.Errorf("expected all prompts to be Gratitude, got %q", p.Category)
		}
	}
}

func TestJournalPrompts_AllCategoriesHavePrompts(t *testing.T) {
	jp := JournalPrompts{}
	jp.allPrompts = curatedPrompts

	for i := 1; i < len(journalCategories); i++ {
		jp.category = i
		jp.filterPrompts()
		if len(jp.prompts) == 0 {
			t.Errorf("category %q has 0 prompts", journalCategories[i])
		}
	}
}

func TestJournalPrompts_CuratedPromptsNotEmpty(t *testing.T) {
	if len(curatedPrompts) == 0 {
		t.Fatal("curatedPrompts should not be empty")
	}
	for i, p := range curatedPrompts {
		if p.Text == "" {
			t.Errorf("prompt %d has empty text", i)
		}
		if p.Category == "" {
			t.Errorf("prompt %d has empty category", i)
		}
	}
}
