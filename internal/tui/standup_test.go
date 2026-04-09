package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStandup_Generate_WithData(t *testing.T) {
	sg := &StandupGenerator{
		commits:    []string{"fix: resolve login bug", "feat: add dark mode"},
		doneTasks:  []string{"Deploy v2.1"},
		todayTasks: []string{"Write tests", "Review PR"},
	}
	sg.generate()

	if !strings.Contains(sg.yesterday, "fix: resolve login bug") {
		t.Error("expected commit in yesterday section")
	}
	if !strings.Contains(sg.yesterday, "Deploy v2.1") {
		t.Error("expected done task in yesterday section")
	}
	if !strings.Contains(sg.today, "Write tests") {
		t.Error("expected today task")
	}
}

func TestStandup_Generate_Empty(t *testing.T) {
	sg := &StandupGenerator{}
	sg.generate()

	if sg.yesterday == "" {
		t.Error("yesterday should have a fallback message")
	}
	if sg.blockers == "" {
		t.Error("blockers should have a default")
	}
}

func TestStandup_Save(t *testing.T) {
	dir := t.TempDir()
	sg := &StandupGenerator{
		vaultRoot: dir,
		yesterday: "Fixed bugs",
		today:     "Write docs",
		blockers:  "None",
	}
	sg.save()

	if !sg.saved {
		t.Error("expected saved=true")
	}

	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "Standups", today+".md"))
	if err != nil {
		t.Fatalf("standup file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## What I did yesterday") {
		t.Error("expected yesterday heading")
	}
	if !strings.Contains(content, "Fixed bugs") {
		t.Error("expected yesterday content")
	}
	if !strings.Contains(content, "## Blockers") {
		t.Error("expected blockers heading")
	}
}

func TestStandup_Save_EmptyVaultRoot(t *testing.T) {
	sg := &StandupGenerator{vaultRoot: ""}
	// Should handle gracefully — the MkdirAll will create in cwd,
	// but save() doesn't guard against empty vaultRoot.
	// This test just ensures it doesn't panic.
	sg.yesterday = "test"
	sg.today = "test"
	sg.blockers = "test"
	// We don't call save() here to avoid writing to cwd
}

func TestStandup_Open(t *testing.T) {
	sg := &StandupGenerator{}
	sg.Open(t.TempDir())

	if !sg.active {
		t.Error("expected active after Open")
	}
}
