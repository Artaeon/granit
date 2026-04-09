package tui

import "testing"

func newTestFileTree(files []string) FileTree {
	ft := NewFileTree()
	ft.SetFiles(files)
	ft.SetSize(40, 20)
	ft.SetFocused(true)
	return ft
}

func TestFileTree_SetFiles_BuildsTree(t *testing.T) {
	ft := newTestFileTree([]string{
		"notes/hello.md",
		"notes/world.md",
		"tasks/todo.md",
	})
	if ft.root == nil {
		t.Fatal("expected root to be set")
	}
	// Root should have 2 children: notes/ and tasks/
	if len(ft.root.Children) != 2 {
		t.Errorf("expected 2 root children (notes, tasks), got %d", len(ft.root.Children))
	}
}

func TestFileTree_Selected_File(t *testing.T) {
	ft := newTestFileTree([]string{
		"notes/hello.md",
		"notes/world.md",
	})
	// Move cursor to a file (first visible item is a folder)
	for i, v := range ft.visible {
		if !v.IsDir {
			ft.cursor = i
			break
		}
	}
	sel := ft.Selected()
	if sel == "" {
		t.Error("expected a file to be selected")
	}
}

func TestFileTree_Selected_EmptyTree(t *testing.T) {
	ft := newTestFileTree([]string{})
	if ft.Selected() != "" {
		t.Error("expected empty selected for empty tree")
	}
}

func TestFileTree_Selected_Directory(t *testing.T) {
	ft := newTestFileTree([]string{"folder/file.md"})
	// First visible item should be the folder
	if len(ft.visible) > 0 && ft.visible[0].IsDir {
		ft.cursor = 0
		if ft.Selected() != "" {
			t.Error("selecting a directory should return empty string")
		}
	}
}

func TestFileTree_SetFilesPreservesExpansion(t *testing.T) {
	ft := newTestFileTree([]string{
		"notes/a.md",
		"tasks/b.md",
	})
	// Count visible before and after re-setting files
	countBefore := len(ft.visible)

	ft.SetFiles([]string{
		"notes/a.md",
		"notes/c.md",
		"tasks/b.md",
	})

	// Should still have the same folders expanded
	if len(ft.visible) < countBefore {
		t.Error("expansion state should be preserved after SetFiles")
	}
}

func TestFileTree_NodeFileCount(t *testing.T) {
	node := &TreeNode{
		IsDir: true,
		Children: []*TreeNode{
			{IsDir: false},
			{IsDir: false},
			{IsDir: true, Children: []*TreeNode{
				{IsDir: false},
			}},
		},
	}
	if count := node.fileCount(); count != 3 {
		t.Errorf("expected 3 files, got %d", count)
	}
}

func TestFileTree_NodeFileCount_Empty(t *testing.T) {
	node := &TreeNode{IsDir: true}
	if count := node.fileCount(); count != 0 {
		t.Errorf("expected 0 files, got %d", count)
	}
}
