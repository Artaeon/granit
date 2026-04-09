package tui

import "testing"

func TestMindMap_BuildHeadingsTree(t *testing.T) {
	mm := &MindMap{notePath: "notes/test.md"}
	content := `# Main Title

## Section A

Some text here.

### Subsection A1

More text.

## Section B

Final text.
`
	mm.buildHeadingsTree(content)

	if mm.root == nil {
		t.Fatal("expected root node")
	}
	if mm.root.Label != "test" {
		t.Errorf("expected root label 'test', got %q", mm.root.Label)
	}
	// Root should have H1 as child, which has ## headings as grandchildren
	if len(mm.root.Children) < 1 {
		t.Fatalf("expected at least 1 child (Main Title), got %d", len(mm.root.Children))
	}
	h1 := mm.root.Children[0]
	if h1.Label != "Main Title" {
		t.Errorf("expected first child 'Main Title', got %q", h1.Label)
	}
	if len(h1.Children) < 2 {
		t.Fatalf("expected H1 to have at least 2 children (Section A, B), got %d", len(h1.Children))
	}
}

func TestMindMap_BuildHeadingsTree_SkipsCodeBlocks(t *testing.T) {
	mm := &MindMap{notePath: "test.md"}
	content := "# Title\n\n```\n## Not a heading\n```\n\n## Real heading\n"
	mm.buildHeadingsTree(content)

	if mm.root == nil {
		t.Fatal("expected root")
	}
	// Only "Real heading" should be a child, not "Not a heading"
	for _, child := range mm.root.Children {
		if child.Label == "Not a heading" {
			t.Error("heading inside code block should be skipped")
		}
	}
}

func TestMindMap_BuildHeadingsTree_Empty(t *testing.T) {
	mm := &MindMap{notePath: "empty.md"}
	mm.buildHeadingsTree("")

	if mm.root == nil {
		t.Fatal("expected root even for empty content")
	}
	if len(mm.root.Children) != 0 {
		t.Errorf("expected 0 children for empty content, got %d", len(mm.root.Children))
	}
}

func TestMmFindParent(t *testing.T) {
	root := &mindMapNode{Label: "root", Depth: 0}
	section := &mindMapNode{Label: "section", Depth: 1}

	stack := make([]*mindMapNode, 7)
	stack[0] = root
	stack[1] = section

	// Finding parent for level 2 (subsection) should return section
	parent := mmFindParent(stack, 2)
	if parent != section {
		t.Errorf("expected section as parent for level 2, got %q", parent.Label)
	}

	// Finding parent for level 1 should return root
	parent = mmFindParent(stack, 1)
	if parent != root {
		t.Errorf("expected root as parent for level 1, got %q", parent.Label)
	}
}

func TestMmDeepestNode(t *testing.T) {
	root := &mindMapNode{Label: "root"}
	deep := &mindMapNode{Label: "deep"}

	stack := make([]*mindMapNode, 7)
	stack[0] = root
	stack[3] = deep

	result := mmDeepestNode(stack)
	if result != deep {
		t.Errorf("expected deepest node 'deep', got %q", result.Label)
	}
}

func TestMmDeepestNode_OnlyRoot(t *testing.T) {
	root := &mindMapNode{Label: "root"}
	stack := make([]*mindMapNode, 7)
	stack[0] = root

	result := mmDeepestNode(stack)
	if result != root {
		t.Errorf("expected root when no deeper nodes, got %q", result.Label)
	}
}
