package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ==================== Note History ====================

// NoteVersionDTO represents a single commit from the git history of a note.
type NoteVersionDTO struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Author  string `json:"author"`
}

// GetNoteHistory returns the git commit history for a specific note file.
// It runs `git log --follow` to track the file across renames.
func (a *GranitApp) GetNoteHistory(relPath string) ([]NoteVersionDTO, error) {
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}

	out, err := a.runGit("log", "--follow",
		"--format=%H|%ad|%s|%an",
		"--date=short",
		"--", relPath,
	)
	if err != nil {
		return nil, fmt.Errorf("git log failed: %s", out)
	}

	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var versions []NoteVersionDTO
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		versions = append(versions, NoteVersionDTO{
			Hash:    parts[0],
			Date:    parts[1],
			Message: parts[2],
			Author:  parts[3],
		})
	}

	return versions, nil
}

// GetNoteAtVersion returns the content of a note at a specific git commit.
// It runs `git show <hash>:<path>` to retrieve the historical content.
func (a *GranitApp) GetNoteAtVersion(relPath string, commitHash string) (string, error) {
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}

	// Sanitize commit hash to prevent injection
	for _, c := range commitHash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", fmt.Errorf("invalid commit hash")
		}
	}

	out, err := a.runGit("show", commitHash+":"+relPath)
	if err != nil {
		return "", fmt.Errorf("git show failed: %s", out)
	}

	return out, nil
}

// GetNoteDiff returns the diff between the current version of a note and a
// specific historical commit.
func (a *GranitApp) GetNoteDiff(relPath string, commitHash string) (string, error) {
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}

	// Sanitize commit hash
	for _, c := range commitHash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", fmt.Errorf("invalid commit hash")
		}
	}

	out, err := a.runGit("diff", commitHash, "HEAD", "--", relPath)
	if err != nil {
		// If HEAD doesn't exist yet, try diffing against empty tree
		out2, err2 := a.runGit("diff", commitHash, "--", relPath)
		if err2 != nil {
			return "", fmt.Errorf("git diff failed: %s", out)
		}
		return out2, nil
	}

	if strings.TrimSpace(out) == "" {
		return "(no differences)", nil
	}

	return out, nil
}

// RestoreNoteVersion restores a note to a specific historical version by
// retrieving the content at the given commit hash and writing it to disk.
func (a *GranitApp) RestoreNoteVersion(relPath string, commitHash string) error {
	content, err := a.GetNoteAtVersion(relPath, commitHash)
	if err != nil {
		return err
	}

	return a.SaveNote(relPath, content)
}

// ==================== Workspace Manager ====================

// SaveWorkspace saves a workspace configuration as a JSON file in
// <vault>/.granit/workspaces/<name>.json.
func (a *GranitApp) SaveWorkspace(name string, data string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}

	// Sanitize name
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}

	wsDir := filepath.Join(a.vaultRoot, ".granit", "workspaces")
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		return fmt.Errorf("create workspaces dir: %w", err)
	}

	// Validate that the provided data is valid JSON
	var check interface{}
	if err := json.Unmarshal([]byte(data), &check); err != nil {
		return fmt.Errorf("invalid workspace data: %w", err)
	}

	wsPath := filepath.Join(wsDir, name+".json")

	// Validate path stays within vault
	abs, err := filepath.Abs(wsPath)
	if err != nil || !strings.HasPrefix(abs, a.vaultRoot) {
		return fmt.Errorf("invalid workspace name")
	}

	return os.WriteFile(wsPath, []byte(data), 0644)
}

// LoadWorkspace reads a saved workspace JSON from disk.
func (a *GranitApp) LoadWorkspace(name string) (string, error) {
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}

	wsPath := filepath.Join(a.vaultRoot, ".granit", "workspaces", name+".json")

	// Validate path stays within vault
	abs, err := filepath.Abs(wsPath)
	if err != nil || !strings.HasPrefix(abs, a.vaultRoot) {
		return "", fmt.Errorf("invalid workspace name")
	}

	data, err := os.ReadFile(wsPath)
	if err != nil {
		return "", fmt.Errorf("workspace not found: %s", name)
	}

	return string(data), nil
}

// ListWorkspaces returns the names of all saved workspaces.
func (a *GranitApp) ListWorkspaces() ([]string, error) {
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}

	wsDir := filepath.Join(a.vaultRoot, ".granit", "workspaces")
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		return nil, nil // No workspaces directory yet is not an error
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, strings.TrimSuffix(entry.Name(), ".json"))
		}
	}

	sort.Strings(names)
	return names, nil
}

// DeleteWorkspace removes a saved workspace file.
func (a *GranitApp) DeleteWorkspace(name string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}

	wsPath := filepath.Join(a.vaultRoot, ".granit", "workspaces", name+".json")

	// Validate path stays within vault
	abs, err := filepath.Abs(wsPath)
	if err != nil || !strings.HasPrefix(abs, a.vaultRoot) {
		return fmt.Errorf("invalid workspace name")
	}

	if err := os.Remove(wsPath); err != nil {
		return fmt.Errorf("could not delete workspace: %w", err)
	}

	return nil
}

// RenameWorkspace renames a saved workspace file.
func (a *GranitApp) RenameWorkspace(oldName string, newName string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}

	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}

	wsDir := filepath.Join(a.vaultRoot, ".granit", "workspaces")
	oldPath := filepath.Join(wsDir, oldName+".json")
	newPath := filepath.Join(wsDir, newName+".json")

	// Validate paths stay within vault
	absOld, err := filepath.Abs(oldPath)
	if err != nil || !strings.HasPrefix(absOld, a.vaultRoot) {
		return fmt.Errorf("invalid workspace name")
	}
	absNew, err := filepath.Abs(newPath)
	if err != nil || !strings.HasPrefix(absNew, a.vaultRoot) {
		return fmt.Errorf("invalid workspace name")
	}

	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("workspace %q already exists", newName)
	}

	return os.Rename(oldPath, newPath)
}

// ==================== Backup System ====================

// CreateBackup creates a tar.gz backup of the entire vault (excluding the
// .granit/backups directory itself) and stores it in <vault>/.granit/backups/.
// Returns the backup filename on success.
func (a *GranitApp) CreateBackup() (string, error) {
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}

	backupDir := filepath.Join(a.vaultRoot, ".granit", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_150405")
	vaultName := filepath.Base(a.vaultRoot)
	backupName := fmt.Sprintf("%s_%s.tar.gz", vaultName, timestamp)
	backupPath := filepath.Join(backupDir, backupName)

	outFile, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	backupDirRel := filepath.Join(".granit", "backups")

	err = filepath.Walk(a.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		relPath, err := filepath.Rel(a.vaultRoot, path)
		if err != nil {
			return nil
		}

		// Skip the backup directory itself
		if strings.HasPrefix(relPath, backupDirRel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden directories (but not .granit)
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && info.Name() != ".granit" {
			return filepath.SkipDir
		}

		// Skip the root directory entry
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		_, err = io.Copy(tarWriter, f)
		return err
	})

	if err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("backup failed: %w", err)
	}

	return backupName, nil
}

// BackupInfoDTO represents a backup file with metadata.
type BackupInfoDTO struct {
	Name string `json:"name"`
	Date string `json:"date"`
	Size int64  `json:"size"`
}

// ListBackups returns all backup files in <vault>/.granit/backups/,
// sorted by date (newest first).
func (a *GranitApp) ListBackups() ([]BackupInfoDTO, error) {
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}

	backupDir := filepath.Join(a.vaultRoot, ".granit", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, nil // No backups directory yet is not an error
	}

	var backups []BackupInfoDTO
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfoDTO{
			Name: entry.Name(),
			Date: info.ModTime().Format(time.RFC3339),
			Size: info.Size(),
		})
	}

	// Sort newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Date > backups[j].Date
	})

	return backups, nil
}

// DeleteBackup removes a backup file.
func (a *GranitApp) DeleteBackup(name string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}

	backupPath := filepath.Join(a.vaultRoot, ".granit", "backups", name)

	// Validate path stays within vault
	abs, err := filepath.Abs(backupPath)
	if err != nil || !strings.HasPrefix(abs, a.vaultRoot) {
		return fmt.Errorf("invalid backup name")
	}

	if !strings.HasSuffix(name, ".tar.gz") {
		return fmt.Errorf("invalid backup file")
	}

	return os.Remove(backupPath)
}
