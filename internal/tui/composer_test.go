package tui

import "testing"

func TestComposer_NewDefaults(t *testing.T) {
	c := NewComposer()
	if c.IsActive() {
		t.Error("new composer should be inactive")
	}
}

func TestComposer_OpenClose(t *testing.T) {
	c := NewComposer()
	c.Open()

	if !c.IsActive() {
		t.Error("expected active after Open")
	}
	if c.mode != composerModeInput {
		t.Error("expected input mode after Open")
	}

	c.Close()
	if c.IsActive() {
		t.Error("expected inactive after Close")
	}
}

func TestComposer_OpenResetsState(t *testing.T) {
	c := NewComposer()
	c.prompt = "leftover"
	c.generatedContent = "old content"
	c.loading = true
	c.errMsg = "old error"

	c.Open()

	if c.prompt != "" {
		t.Error("prompt should be reset")
	}
	if c.generatedContent != "" {
		t.Error("generated content should be reset")
	}
	if c.loading {
		t.Error("loading should be reset")
	}
	if c.errMsg != "" {
		t.Error("error message should be reset")
	}
}

func TestComposer_GetResult_NotReady(t *testing.T) {
	c := NewComposer()
	_, _, ok := c.GetResult()
	if ok {
		t.Error("expected no result when not ready")
	}
}

func TestComposer_GetResult_ConsumedOnce(t *testing.T) {
	c := NewComposer()
	c.resultReady = true
	c.resultTitle = "Test Note"
	c.resultContent = "# Test Note\n\nContent."

	title, content, ok := c.GetResult()
	if !ok {
		t.Fatal("expected result ready")
	}
	if title != "Test Note" {
		t.Errorf("expected title 'Test Note', got %q", title)
	}
	if content != "# Test Note\n\nContent." {
		t.Errorf("unexpected content: %q", content)
	}

	// Second call should return not ready
	_, _, ok2 := c.GetResult()
	if ok2 {
		t.Error("result should be consumed after first GetResult")
	}
}

func TestComposer_SetExistingNotes(t *testing.T) {
	c := NewComposer()
	c.SetExistingNotes([]string{"a.md", "b.md"})
	if len(c.existingNotes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(c.existingNotes))
	}
}

func TestComposer_SetNoteContents(t *testing.T) {
	c := NewComposer()
	c.SetNoteContents(map[string]string{"a.md": "content"})
	if len(c.noteContents) != 1 {
		t.Errorf("expected 1 entry, got %d", len(c.noteContents))
	}
}
