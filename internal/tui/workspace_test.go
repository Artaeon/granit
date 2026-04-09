package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspace_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	w := &Workspace{configDir: dir}

	w.layouts = []WorkspaceLayout{
		{Name: "focus", ActiveNote: "tasks.md", Layout: "writer"},
		{Name: "review", ActiveNote: "journal.md", Layout: "reading"},
	}
	w.saveWorkspaces()

	// Verify file
	data, err := os.ReadFile(filepath.Join(dir, "workspaces.json"))
	if err != nil {
		t.Fatalf("workspaces.json not created: %v", err)
	}
	var loaded []WorkspaceLayout
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 layouts, got %d", len(loaded))
	}

	// Reload
	w2 := &Workspace{configDir: dir}
	w2.loadWorkspaces()
	if len(w2.layouts) != 2 {
		t.Fatalf("expected 2 layouts after load, got %d", len(w2.layouts))
	}
	if w2.layouts[0].Name != "focus" {
		t.Errorf("expected 'focus', got %q", w2.layouts[0].Name)
	}
}

func TestWorkspace_LoadMissingFile(t *testing.T) {
	w := &Workspace{configDir: t.TempDir()}
	w.loadWorkspaces()
	if w.layouts != nil {
		t.Errorf("expected nil for missing file, got %v", w.layouts)
	}
}

func TestWorkspace_SaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "config")
	w := &Workspace{configDir: dir}
	w.layouts = []WorkspaceLayout{{Name: "test"}}
	w.saveWorkspaces()

	if _, err := os.Stat(filepath.Join(dir, "workspaces.json")); os.IsNotExist(err) {
		t.Error("save should create nested directories")
	}
}
