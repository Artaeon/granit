package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// projectsStatePath returns the canonical path of projects.json for a vault.
func projectsStatePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "projects.json")
}

// LoadProjects reads the project list from <vault>/.granit/projects.json.
// Returns nil for both a missing file and a corrupt file — callers always
// check len() and treat zero projects as the empty state.
func LoadProjects(vaultRoot string) []Project {
	data, err := os.ReadFile(projectsStatePath(vaultRoot))
	if err != nil {
		return nil
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil
	}
	return projects
}

// SaveProjects writes the project list to <vault>/.granit/projects.json
// using an atomic tmp+rename so a crash mid-write cannot truncate the
// store. The .granit directory is created if missing.
func SaveProjects(vaultRoot string, projects []Project) error {
	path := projectsStatePath(vaultRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteState(path, data)
}
