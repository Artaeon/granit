package tui

import "testing"

func TestNewFoldState(t *testing.T) {
	fs := NewFoldState()
	if fs.folds == nil {
		t.Fatal("NewFoldState should return a FoldState with a non-nil folds map")
	}
	if len(fs.folds) != 0 {
		t.Fatalf("NewFoldState should return an empty folds map, got %d entries", len(fs.folds))
	}
}

func TestToggleFold_Heading(t *testing.T) {
	content := []string{
		"## Section A",   // 0
		"some text",      // 1
		"more text",      // 2
		"## Section B",   // 3
		"sub text",       // 4
	}

	fs := NewFoldState()
	fs.ToggleFold(0, content)

	end, ok := fs.GetFoldEnd(0)
	if !ok {
		t.Fatal("expected fold at line 0 to exist")
	}
	// h2 folds until the next h2 at line 3, so end should be line 2
	if end != 2 {
		t.Fatalf("expected fold end at 2, got %d", end)
	}
}

func TestToggleFold_Unfold(t *testing.T) {
	content := []string{
		"# Title",
		"body",
		"more body",
	}

	fs := NewFoldState()
	fs.ToggleFold(0, content)

	_, ok := fs.GetFoldEnd(0)
	if !ok {
		t.Fatal("expected fold to exist after first toggle")
	}

	// Toggle again should unfold
	fs.ToggleFold(0, content)
	_, ok = fs.GetFoldEnd(0)
	if ok {
		t.Fatal("expected fold to be removed after second toggle")
	}
}

func TestToggleFold_OutOfBounds(t *testing.T) {
	content := []string{"# Title", "body"}
	fs := NewFoldState()

	// Should not panic or add a fold
	fs.ToggleFold(-1, content)
	fs.ToggleFold(5, content)
	if len(fs.folds) != 0 {
		t.Fatal("out-of-bounds toggle should not create a fold")
	}
}

func TestToggleFold_NonFoldableLine(t *testing.T) {
	content := []string{"just text", "more text"}
	fs := NewFoldState()
	fs.ToggleFold(0, content)
	if len(fs.folds) != 0 {
		t.Fatal("toggling a non-heading, non-fence line should not create a fold")
	}
}

func TestToggleFold_CodeFence(t *testing.T) {
	content := []string{
		"```go",          // 0
		"func main() {}", // 1
		"```",            // 2
		"after fence",    // 3
	}

	fs := NewFoldState()
	fs.ToggleFold(0, content)

	end, ok := fs.GetFoldEnd(0)
	if !ok {
		t.Fatal("expected fold at code fence line 0")
	}
	if end != 2 {
		t.Fatalf("expected code fence fold end at 2, got %d", end)
	}
}

func TestIsFolded(t *testing.T) {
	content := []string{
		"## Section A",  // 0
		"line 1",        // 1
		"line 2",        // 2
		"## Section B",  // 3
		"line 3",        // 4
	}

	fs := NewFoldState()
	fs.ToggleFold(0, content)
	// h2 at line 0 folds until next h2 at line 3 => fold covers lines 1-2

	tests := []struct {
		name   string
		line   int
		folded bool
	}{
		{"fold trigger line is not folded", 0, false},
		{"first line inside fold", 1, true},
		{"second line inside fold", 2, true},
		{"line outside fold (next heading)", 3, false},
		{"line after fold region", 4, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fs.IsFolded(tc.line)
			if got != tc.folded {
				t.Errorf("IsFolded(%d) = %v, want %v", tc.line, got, tc.folded)
			}
		})
	}
}

func TestFoldAll(t *testing.T) {
	content := []string{
		"# Heading 1",    // 0
		"content a",      // 1
		"## Heading 2",   // 2
		"content b",      // 3
		"```python",      // 4
		"print('hi')",    // 5
		"```",            // 6
		"### Heading 3",  // 7
		"content c",      // 8
	}

	fs := NewFoldState()
	fs.FoldAll(content)

	// The h1 at line 0 should fold lines 1 through (at least) up to but not
	// including h2. Check that folded lines are hidden.
	if !fs.IsFolded(1) {
		t.Error("line 1 should be folded by FoldAll")
	}

	// After FoldAll, multiple folds should exist (h1 encompasses the rest
	// because FoldAll uses skip to avoid nested folds that are already covered).
	if len(fs.folds) == 0 {
		t.Fatal("FoldAll should create at least one fold")
	}
}

func TestUnfoldAll(t *testing.T) {
	content := []string{
		"# Heading",
		"body",
		"## Sub",
		"sub body",
	}

	fs := NewFoldState()
	fs.FoldAll(content)

	if len(fs.folds) == 0 {
		t.Fatal("expected folds after FoldAll")
	}

	fs.UnfoldAll()
	if len(fs.folds) != 0 {
		t.Fatalf("UnfoldAll should clear all folds, got %d", len(fs.folds))
	}

	// No line should be folded
	for i := range content {
		if fs.IsFolded(i) {
			t.Errorf("line %d should not be folded after UnfoldAll", i)
		}
	}
}

func TestGetFoldEnd(t *testing.T) {
	content := []string{
		"## Section",  // 0
		"text a",      // 1
		"text b",      // 2
	}

	fs := NewFoldState()

	// Before folding
	_, ok := fs.GetFoldEnd(0)
	if ok {
		t.Fatal("GetFoldEnd should return false before folding")
	}

	fs.ToggleFold(0, content)

	end, ok := fs.GetFoldEnd(0)
	if !ok {
		t.Fatal("GetFoldEnd should return true after folding")
	}
	if end != 2 {
		t.Fatalf("expected fold end at 2, got %d", end)
	}
}

func TestGetFoldIndicator(t *testing.T) {
	content := []string{
		"# Title",     // 0
		"body text",   // 1
		"```",         // 2
		"code",        // 3
		"```",         // 4
		"plain line",  // 5
	}

	fs := NewFoldState()

	t.Run("expanded heading", func(t *testing.T) {
		ind := fs.GetFoldIndicator(0, content)
		if ind != "▼" {
			t.Errorf("expected down arrow for expanded heading, got %q", ind)
		}
	})

	t.Run("normal line", func(t *testing.T) {
		ind := fs.GetFoldIndicator(1, content)
		if ind != "" {
			t.Errorf("expected empty for normal line, got %q", ind)
		}
	})

	t.Run("expanded code fence", func(t *testing.T) {
		ind := fs.GetFoldIndicator(2, content)
		if ind != "▼" {
			t.Errorf("expected down arrow for expanded code fence, got %q", ind)
		}
	})

	t.Run("folded heading", func(t *testing.T) {
		fs.ToggleFold(0, content)
		ind := fs.GetFoldIndicator(0, content)
		if ind != "▶" {
			t.Errorf("expected right arrow for folded heading, got %q", ind)
		}
	})

	t.Run("folded code fence", func(t *testing.T) {
		fs.ToggleFold(2, content)
		ind := fs.GetFoldIndicator(2, content)
		if ind != "▶" {
			t.Errorf("expected right arrow for folded fence, got %q", ind)
		}
	})

	t.Run("out of bounds", func(t *testing.T) {
		ind := fs.GetFoldIndicator(-1, content)
		if ind != "" {
			t.Errorf("expected empty for out of bounds, got %q", ind)
		}
		ind = fs.GetFoldIndicator(100, content)
		if ind != "" {
			t.Errorf("expected empty for out of bounds, got %q", ind)
		}
	})

	t.Run("plain line always empty", func(t *testing.T) {
		ind := fs.GetFoldIndicator(5, content)
		if ind != "" {
			t.Errorf("expected empty for plain line, got %q", ind)
		}
	})
}

func TestHeadingLevel(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"# Title", 1},
		{"## Section", 2},
		{"### Sub", 3},
		{"#### Deep", 4},
		{"##### Deeper", 5},
		{"###### Deepest", 6},
		{"####### TooDeep", 0},
		{"#NoSpace", 0},
		{"not a heading", 0},
		{"", 0},
		{"#", 0},
		{"## ", 2},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := headingLevel(tc.input)
			if got != tc.want {
				t.Errorf("headingLevel(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestHeadingFoldEnd(t *testing.T) {
	t.Run("stops at equal level heading", func(t *testing.T) {
		content := []string{
			"## A",     // 0
			"text",     // 1
			"## B",     // 2
			"text b",   // 3
		}
		end := headingFoldEnd(content, 0, 2)
		if end != 1 {
			t.Errorf("expected 1, got %d", end)
		}
	})

	t.Run("stops at higher level heading", func(t *testing.T) {
		content := []string{
			"### Sub",  // 0
			"text",     // 1
			"## Parent", // 2
			"text p",   // 3
		}
		end := headingFoldEnd(content, 0, 3)
		if end != 1 {
			t.Errorf("expected 1, got %d", end)
		}
	})

	t.Run("includes lower level headings", func(t *testing.T) {
		content := []string{
			"## Section",    // 0
			"text",          // 1
			"### Subsection", // 2
			"sub text",      // 3
		}
		end := headingFoldEnd(content, 0, 2)
		// No equal-or-higher heading follows, so fold to end of document
		if end != 3 {
			t.Errorf("expected 3, got %d", end)
		}
	})

	t.Run("folds to end of document", func(t *testing.T) {
		content := []string{
			"# Title",     // 0
			"line 1",      // 1
			"line 2",      // 2
		}
		end := headingFoldEnd(content, 0, 1)
		if end != 2 {
			t.Errorf("expected 2, got %d", end)
		}
	})

	t.Run("nothing to fold returns start", func(t *testing.T) {
		content := []string{
			"## A",  // 0
			"## B",  // 1
		}
		end := headingFoldEnd(content, 0, 2)
		if end != 0 {
			t.Errorf("expected 0 (nothing to fold), got %d", end)
		}
	})
}

func TestCodeFenceFoldEnd(t *testing.T) {
	t.Run("finds closing fence", func(t *testing.T) {
		content := []string{
			"```",    // 0
			"code",   // 1
			"more",   // 2
			"```",    // 3
			"after",  // 4
		}
		end := codeFenceFoldEnd(content, 0)
		if end != 3 {
			t.Errorf("expected 3, got %d", end)
		}
	})

	t.Run("no closing fence returns start", func(t *testing.T) {
		content := []string{
			"```go",   // 0
			"code",    // 1
			"more",    // 2
		}
		end := codeFenceFoldEnd(content, 0)
		if end != 0 {
			t.Errorf("expected 0 when no closing fence, got %d", end)
		}
	})

	t.Run("closing fence with language tag", func(t *testing.T) {
		content := []string{
			"```python",  // 0
			"x = 1",      // 1
			"```",        // 2
		}
		end := codeFenceFoldEnd(content, 0)
		if end != 2 {
			t.Errorf("expected 2, got %d", end)
		}
	})

	t.Run("fence with whitespace", func(t *testing.T) {
		content := []string{
			"  ```",   // 0
			"code",    // 1
			"  ```",   // 2
		}
		end := codeFenceFoldEnd(content, 0)
		if end != 2 {
			t.Errorf("expected 2, got %d", end)
		}
	})
}
