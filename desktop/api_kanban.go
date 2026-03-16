package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ==================== Kanban ====================

// GetKanban loads the kanban board JSON from .granit/kanban.json.
func (a *GranitApp) GetKanban() (string, error) {
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	fp := filepath.Join(a.vaultRoot, ".granit", "kanban.json")
	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return "{}", nil // empty board
		}
		return "", err
	}
	return string(data), nil
}

// SaveKanban persists the kanban board JSON to .granit/kanban.json.
func (a *GranitApp) SaveKanban(data string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	dir := filepath.Join(a.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create .granit directory: %w", err)
	}
	fp := filepath.Join(dir, "kanban.json")
	return os.WriteFile(fp, []byte(data), 0o644)
}

// ==================== Tasks ====================

// TaskItem represents a single task extracted from a vault note.
type TaskItem struct {
	Text     string `json:"text"`
	Done     bool   `json:"done"`
	NotePath string `json:"notePath"`
	LineNum  int    `json:"lineNum"`
}

// GetAllTasks scans all vault notes for checkbox task lines (- [ ] and - [x])
// and returns them with their source note path, line number, text, and status.
func (a *GranitApp) GetAllTasks() ([]TaskItem, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	var tasks []TaskItem
	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}
		lines := strings.Split(note.Content, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
				tasks = append(tasks, TaskItem{
					Text:     strings.TrimSpace(trimmed[6:]),
					Done:     true,
					NotePath: p,
					LineNum:  i + 1, // 1-based
				})
			} else if strings.HasPrefix(trimmed, "- [ ] ") {
				tasks = append(tasks, TaskItem{
					Text:     strings.TrimSpace(trimmed[6:]),
					Done:     false,
					NotePath: p,
					LineNum:  i + 1,
				})
			}
		}
	}
	return tasks, nil
}
