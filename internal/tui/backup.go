package tui

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BackupEntry represents a single vault backup archive.
type BackupEntry struct {
	Name    string
	Path    string
	Size    int64
	Created time.Time
}

// Backup provides an overlay for creating, restoring, and managing vault
// backup archives stored as timestamped zip files in .granit/backups/.
type Backup struct {
	active        bool
	width         int
	height        int
	vaultPath     string
	backups       []BackupEntry
	cursor        int
	scroll        int
	confirming    bool
	confirmAction string
	message       string
	autoMode      string // "none", "on_save", "daily"
	maxBackups    int
}

// NewBackup creates a Backup overlay with sensible defaults.
func NewBackup() Backup {
	return Backup{
		autoMode:   "none",
		maxBackups: 10,
	}
}

// IsActive returns whether the backup overlay is currently visible.
func (b Backup) IsActive() bool {
	return b.active
}

// Open activates the overlay and scans the backups directory.
func (b *Backup) Open(vaultPath string) {
	b.active = true
	b.vaultPath = vaultPath
	b.cursor = 0
	b.scroll = 0
	b.confirming = false
	b.confirmAction = ""
	b.message = ""
	b.scanBackups()
}

// Close hides the backup overlay.
func (b *Backup) Close() {
	b.active = false
}

// SetSize updates the available dimensions for the overlay.
func (b *Backup) SetSize(w, h int) {
	b.width = w
	b.height = h
}

// backupDir returns the absolute path to the .granit/backups/ directory.
func (b *Backup) backupDir() string {
	return filepath.Join(b.vaultPath, ".granit", "backups")
}

// scanBackups reads the backup directory and populates the backups slice,
// sorted by creation time (newest first).
func (b *Backup) scanBackups() {
	b.backups = nil
	dir := b.backupDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		created := info.ModTime()

		// Try to parse timestamp from filename: backup_YYYY-MM-DD_HHMMSS.zip
		if ts, err := time.Parse("backup_2006-01-02_150405.zip", entry.Name()); err == nil {
			created = ts
		}

		b.backups = append(b.backups, BackupEntry{
			Name:    entry.Name(),
			Path:    fullPath,
			Size:    info.Size(),
			Created: created,
		})
	}

	sort.Slice(b.backups, func(i, j int) bool {
		return b.backups[i].Created.After(b.backups[j].Created)
	})
}

// Update handles keyboard input for the backup overlay.
func (b Backup) Update(msg tea.Msg) (Backup, tea.Cmd) {
	if !b.active {
		return b, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If showing a result message, any key dismisses it.
		if b.message != "" && !b.confirming {
			switch msg.String() {
			case "esc", "enter", "q":
				b.message = ""
				b.scanBackups()
			}
			return b, nil
		}

		// Confirmation dialog.
		if b.confirming {
			switch msg.String() {
			case "y":
				b.confirming = false
				switch b.confirmAction {
				case "restore":
					if b.cursor >= 0 && b.cursor < len(b.backups) {
						entry := b.backups[b.cursor]
						if err := RestoreBackup(b.vaultPath, entry.Path); err != nil {
							b.message = "Error: " + err.Error()
						} else {
							b.message = "Restored from " + entry.Name
						}
					}
				case "delete":
					if b.cursor >= 0 && b.cursor < len(b.backups) {
						entry := b.backups[b.cursor]
						if err := DeleteBackup(entry.Path); err != nil {
							b.message = "Error: " + err.Error()
						} else {
							b.message = "Deleted " + entry.Name
							b.scanBackups()
							if b.cursor >= len(b.backups) && b.cursor > 0 {
								b.cursor--
							}
						}
					}
				}
				b.confirmAction = ""
			case "n", "esc":
				b.confirming = false
				b.confirmAction = ""
			}
			return b, nil
		}

		// Normal mode.
		switch msg.String() {
		case "esc":
			b.active = false
		case "up", "k":
			if b.cursor > 0 {
				b.cursor--
				if b.cursor < b.scroll {
					b.scroll = b.cursor
				}
			}
		case "down", "j":
			if b.cursor < len(b.backups)-1 {
				b.cursor++
				visH := b.visibleHeight()
				if b.cursor >= b.scroll+visH {
					b.scroll = b.cursor - visH + 1
				}
			}
		case "c":
			if err := CreateBackup(b.vaultPath); err != nil {
				b.message = "Error: " + err.Error()
			} else {
				b.message = "Backup created successfully"
				PruneBackups(b.vaultPath, b.maxBackups)
				b.scanBackups()
			}
		case "r":
			if len(b.backups) > 0 && b.cursor < len(b.backups) {
				b.confirming = true
				b.confirmAction = "restore"
			}
		case "d":
			if len(b.backups) > 0 && b.cursor < len(b.backups) {
				b.confirming = true
				b.confirmAction = "delete"
			}
		case "a":
			// Cycle auto-backup mode: none -> on_save -> daily -> none
			switch b.autoMode {
			case "none":
				b.autoMode = "on_save"
			case "on_save":
				b.autoMode = "daily"
			case "daily":
				b.autoMode = "none"
			default:
				b.autoMode = "none"
			}
		}
	}
	return b, nil
}

// visibleHeight returns how many backup entries fit in the visible area.
func (b *Backup) visibleHeight() int {
	h := b.height - 14
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the backup overlay.
func (b Backup) View() string {
	width := b.width / 2
	if width < 55 {
		width = 55
	}
	if width > 75 {
		width = 75
	}

	var s strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSaveChar + " Vault Backups")
	count := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf(" (%d)", len(b.backups)))
	s.WriteString(title + count)
	s.WriteString("\n")
	s.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	s.WriteString("\n")

	// Auto-backup mode indicator
	modeLabel := lipgloss.NewStyle().Foreground(text).Render("  Auto-backup: ")
	var modeValue string
	switch b.autoMode {
	case "on_save":
		modeValue = lipgloss.NewStyle().Foreground(green).Bold(true).Render("on save")
	case "daily":
		modeValue = lipgloss.NewStyle().Foreground(blue).Bold(true).Render("daily")
	default:
		modeValue = lipgloss.NewStyle().Foreground(overlay0).Render("disabled")
	}
	s.WriteString(modeLabel + modeValue)
	s.WriteString("  ")
	s.WriteString(lipgloss.NewStyle().Foreground(surface2).Render(fmt.Sprintf("(max: %d)", b.maxBackups)))
	s.WriteString("\n\n")

	// Confirmation dialog
	if b.confirming {
		var prompt string
		promptStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		switch b.confirmAction {
		case "restore":
			prompt = "  Restore will overwrite current vault files. Continue?"
			promptStyle = promptStyle.Foreground(red)
		case "delete":
			prompt = "  Permanently delete this backup?"
		}
		s.WriteString(promptStyle.Render(prompt))
		s.WriteString("\n\n")
		yKey := lipgloss.NewStyle().Foreground(green).Bold(true).Render("y")
		nKey := lipgloss.NewStyle().Foreground(red).Bold(true).Render("n")
		s.WriteString("  " + yKey + DimStyle.Render(": yes  ") + nKey + DimStyle.Render(": no"))
		s.WriteString("\n")
	} else if b.message != "" {
		// Result message
		msgStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		if strings.HasPrefix(b.message, "Error:") {
			msgStyle = lipgloss.NewStyle().Foreground(red).Bold(true)
		}
		s.WriteString(msgStyle.Render("  " + b.message))
		s.WriteString("\n\n")
		s.WriteString(DimStyle.Render("  Press any key to continue"))
		s.WriteString("\n")
	} else if len(b.backups) == 0 {
		// Empty state
		s.WriteString(DimStyle.Render("  No backups yet"))
		s.WriteString("\n")
		s.WriteString(DimStyle.Render("  Press c to create your first backup"))
		s.WriteString("\n")
	} else {
		// Backup list
		visH := b.visibleHeight()
		end := b.scroll + visH
		if end > len(b.backups) {
			end = len(b.backups)
		}

		archiveIcon := lipgloss.NewStyle().Foreground(blue).Render(IconSaveChar)
		dimTimeStyle := lipgloss.NewStyle().Foreground(overlay0)
		sizeStyle := lipgloss.NewStyle().Foreground(surface2)

		for i := b.scroll; i < end; i++ {
			entry := b.backups[i]
			name := strings.TrimSuffix(entry.Name, ".zip")
			ago := timeAgo(entry.Created)
			size := backupFormatSize(entry.Size)

			detail := dimTimeStyle.Render(ago) + sizeStyle.Render("  "+size)

			if i == b.cursor {
				line := "  " + archiveIcon + " " + name + "  " + detail
				s.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				s.WriteString("  " + archiveIcon + " " + NormalItemStyle.Render(name) + "  " + detail)
			}
			if i < end-1 {
				s.WriteString("\n")
			}
		}
		s.WriteString("\n")
	}

	// Help bar
	s.WriteString("\n")
	s.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	s.WriteString("\n")

	cKey := lipgloss.NewStyle().Foreground(green).Bold(true).Render("c")
	cDesc := DimStyle.Render(": create  ")
	rKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("r")
	rDesc := DimStyle.Render(": restore  ")
	dKey := lipgloss.NewStyle().Foreground(red).Bold(true).Render("d")
	dDesc := DimStyle.Render(": delete  ")
	aKey := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("a")
	aDesc := DimStyle.Render(": auto  ")
	escKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Esc")
	escDesc := DimStyle.Render(": close")

	s.WriteString("  " + cKey + cDesc + rKey + rDesc + dKey + dDesc + aKey + aDesc + escKey + escDesc)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(s.String())
}

// ---------------------------------------------------------------------------
// Backup operations
// ---------------------------------------------------------------------------

// CreateBackup creates a timestamped zip archive of the vault, including all
// .md files and the .granit/ config directory but skipping .git/,
// .granit/backups/, and .granit-trash/.
func CreateBackup(vaultPath string) error {
	backupDir := filepath.Join(vaultPath, ".granit", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	stamp := time.Now().Format("2006-01-02_150405")
	zipName := fmt.Sprintf("backup_%s.zip", stamp)
	zipPath := filepath.Join(backupDir, zipName)

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	err = filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		relPath, relErr := filepath.Rel(vaultPath, path)
		if relErr != nil {
			return nil
		}

		// Skip directories and files we don't want.
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == ".granit-trash" {
				return filepath.SkipDir
			}
			// Skip .granit/backups/ but allow other .granit/ contents.
			if relPath == filepath.Join(".granit", "backups") {
				return filepath.SkipDir
			}
			return nil
		}

		// Include .md files and anything inside .granit/ (except backups).
		isMd := strings.HasSuffix(path, ".md")
		isGranitConfig := strings.HasPrefix(relPath, ".granit"+string(filepath.Separator)) || relPath == ".granit.json"
		if !isMd && !isGranitConfig {
			return nil
		}

		header, headerErr := zip.FileInfoHeader(info)
		if headerErr != nil {
			return nil
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		writer, createErr := w.CreateHeader(header)
		if createErr != nil {
			return createErr
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		defer f.Close()

		_, copyErr := io.Copy(writer, f)
		return copyErr
	})

	if err != nil {
		// Clean up partial zip on error.
		os.Remove(zipPath)
		return fmt.Errorf("walk vault: %w", err)
	}

	return nil
}

// RestoreBackup extracts a zip archive over the vault directory, overwriting
// existing files. This is a destructive operation and should be called only
// after user confirmation.
func RestoreBackup(vaultPath string, backupPath string) error {
	r, err := zip.OpenReader(backupPath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		destPath := filepath.Join(vaultPath, f.Name)

		// Prevent path traversal attacks.
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(vaultPath)+string(filepath.Separator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			continue
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteBackup removes a single backup archive from disk.
func DeleteBackup(path string) error {
	return os.Remove(path)
}

// PruneBackups deletes the oldest backups when the total count exceeds
// maxCount. Backups are sorted by modification time and the oldest are
// removed first.
func PruneBackups(vaultPath string, maxCount int) {
	if maxCount <= 0 {
		return
	}

	backupDir := filepath.Join(vaultPath, ".granit", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	// Collect only .zip files with their info.
	type backupFile struct {
		path    string
		modTime time.Time
	}
	var files []backupFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, backupFile{
			path:    filepath.Join(backupDir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(files) <= maxCount {
		return
	}

	// Sort oldest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	// Remove the oldest entries that exceed the limit.
	toRemove := len(files) - maxCount
	for i := 0; i < toRemove; i++ {
		os.Remove(files[i].path)
	}
}

// backupFormatSize returns a human-readable file size string.
func backupFormatSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1fGB", float64(size)/(1024*1024*1024))
	}
}
