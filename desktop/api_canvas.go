package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ==================== Canvas ====================

// SaveCanvas persists canvas JSON data to .granit/canvas/<name>.json.
func (a *GranitApp) SaveCanvas(name string, data string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	if name == "" {
		return fmt.Errorf("canvas name is required")
	}
	// Sanitize name — strip path separators and ensure .json suffix
	name = filepath.Base(name)
	if !strings.HasSuffix(name, ".json") {
		name += ".json"
	}

	dir := filepath.Join(a.vaultRoot, ".granit", "canvas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create canvas directory: %w", err)
	}

	fp := filepath.Join(dir, name)
	abs, err := filepath.Abs(fp)
	if err != nil || !strings.HasPrefix(abs, filepath.Join(a.vaultRoot, ".granit", "canvas")) {
		return fmt.Errorf("invalid canvas name")
	}

	return os.WriteFile(abs, []byte(data), 0o644)
}

// GetCanvas loads canvas JSON data from .granit/canvas/<name>.json.
func (a *GranitApp) GetCanvas(name string) (string, error) {
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	if name == "" {
		return "", fmt.Errorf("canvas name is required")
	}
	name = filepath.Base(name)
	if !strings.HasSuffix(name, ".json") {
		name += ".json"
	}

	fp := filepath.Join(a.vaultRoot, ".granit", "canvas", name)
	abs, err := filepath.Abs(fp)
	if err != nil || !strings.HasPrefix(abs, filepath.Join(a.vaultRoot, ".granit", "canvas")) {
		return "", fmt.Errorf("invalid canvas name")
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("canvas not found: %s", name)
	}
	return string(data), nil
}

// ListCanvases returns the names of all saved canvases.
func (a *GranitApp) ListCanvases() ([]string, error) {
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}

	dir := filepath.Join(a.vaultRoot, ".granit", "canvas")
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist yet — return empty list, not error
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names, nil
}

// DeleteCanvas removes a canvas file from .granit/canvas/.
func (a *GranitApp) DeleteCanvas(name string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	if name == "" {
		return fmt.Errorf("canvas name is required")
	}
	name = filepath.Base(name)
	if !strings.HasSuffix(name, ".json") {
		name += ".json"
	}

	fp := filepath.Join(a.vaultRoot, ".granit", "canvas", name)
	abs, err := filepath.Abs(fp)
	if err != nil || !strings.HasPrefix(abs, filepath.Join(a.vaultRoot, ".granit", "canvas")) {
		return fmt.Errorf("invalid canvas name")
	}

	if err := os.Remove(abs); err != nil {
		if os.IsNotExist(err) {
			return nil // already gone
		}
		return err
	}
	return nil
}
