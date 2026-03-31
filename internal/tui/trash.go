package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TrashItem represents a single file that was moved to the trash.
type TrashItem struct {
	OrigPath  string    `json:"orig_path"`  // original relative path
	TrashPath string    `json:"trash_path"` // path in .granit-trash/
	DeletedAt time.Time `json:"deleted_at"`
}

// Trash provides a recycle-bin overlay for deleted notes.
type Trash struct {
	active    bool
	items     []TrashItem
	cursor    int
	scroll    int
	width     int
	height    int
	vaultRoot string
	result    string // restore action result path
	doRestore bool
	doPurge   bool
}

// NewTrash creates a new Trash component for the given vault root.
func NewTrash(vaultRoot string) Trash {
	return Trash{
		vaultRoot: vaultRoot,
	}
}

// SetSize updates the available dimensions for the overlay.
func (t *Trash) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// IsActive returns whether the trash overlay is currently visible.
func (t *Trash) IsActive() bool {
	return t.active
}

// Open scans the .granit-trash/ folder and populates the item list sorted
// by deletion date (newest first).
func (t *Trash) Open() {
	t.active = true
	t.cursor = 0
	t.scroll = 0
	t.result = ""
	t.doRestore = false
	t.doPurge = false
	t.scanTrash()
}

// Close hides the trash overlay.
func (t *Trash) Close() {
	t.active = false
}

// trashDir returns the absolute path to .granit-trash/.
func (t *Trash) trashDir() string {
	return filepath.Join(t.vaultRoot, ".granit-trash")
}

// scanTrash reads all .json sidecar files in .granit-trash/ and builds the
// items slice, sorted newest-first.
func (t *Trash) scanTrash() {
	t.items = nil
	dir := t.trashDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var item TrashItem
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}

		// Only include items whose trashed file still exists.
		if _, err := os.Stat(filepath.Join(dir, item.TrashPath)); err == nil {
			t.items = append(t.items, item)
		}
	}

	// Sort newest first.
	sort.Slice(t.items, func(i, j int) bool {
		return t.items[i].DeletedAt.After(t.items[j].DeletedAt)
	})
}

// MoveToTrash moves a vault file to .granit-trash/ and writes a JSON sidecar
// with the original path and deletion timestamp.
func (t *Trash) MoveToTrash(relPath string) error {
	dir := t.trashDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	srcPath := filepath.Join(t.vaultRoot, relPath)

	// Build a unique trash filename based on timestamp + original base name.
	stamp := time.Now().UnixNano()
	baseName := filepath.Base(relPath)
	trashName := fmt.Sprintf("%d_%s", stamp, baseName)

	// Move the file.
	dstPath := filepath.Join(dir, trashName)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return err
	}
	if err := os.Remove(srcPath); err != nil {
		// Best-effort: file was already copied into trash.
		_ = err
	}

	// Write JSON sidecar.
	item := TrashItem{
		OrigPath:  relPath,
		TrashPath: trashName,
		DeletedAt: time.Now(),
	}
	sidecar, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	sidecarPath := filepath.Join(dir, trashName+".json")
	return os.WriteFile(sidecarPath, sidecar, 0644)
}

// RestoreFile moves the currently selected trash item back to its original
// location and returns the original relative path. Returns "" if nothing
// was restored.
func (t *Trash) RestoreFile() string {
	if len(t.items) == 0 || t.cursor >= len(t.items) {
		return ""
	}

	item := t.items[t.cursor]
	dir := t.trashDir()

	srcPath := filepath.Join(dir, item.TrashPath)
	dstPath := filepath.Join(t.vaultRoot, item.OrigPath)

	// Ensure destination directory exists.
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return ""
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return ""
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return ""
	}

	// Clean up trash copy and sidecar.
	_ = os.Remove(srcPath)
	_ = os.Remove(filepath.Join(dir, item.TrashPath+".json"))

	t.result = item.OrigPath
	t.doRestore = true

	// Remove from list and fix cursor.
	t.items = append(t.items[:t.cursor], t.items[t.cursor+1:]...)
	if len(t.items) == 0 {
		t.cursor = 0
	} else if t.cursor >= len(t.items) {
		t.cursor = len(t.items) - 1
	}

	return t.result
}

// ShouldRestore checks and resets the restore flag.
func (t *Trash) ShouldRestore() bool {
	if t.doRestore {
		t.doRestore = false
		return true
	}
	return false
}

// PurgeSelected permanently deletes the selected trash item.
func (t *Trash) PurgeSelected() {
	if len(t.items) == 0 || t.cursor >= len(t.items) {
		return
	}

	item := t.items[t.cursor]
	dir := t.trashDir()

	_ = os.Remove(filepath.Join(dir, item.TrashPath))
	_ = os.Remove(filepath.Join(dir, item.TrashPath+".json"))

	t.items = append(t.items[:t.cursor], t.items[t.cursor+1:]...)
	if len(t.items) == 0 {
		t.cursor = 0
	} else if t.cursor >= len(t.items) {
		t.cursor = len(t.items) - 1
	}
	t.doPurge = true
}

// Update handles keyboard input for the trash overlay.
func (t Trash) Update(msg tea.Msg) (Trash, tea.Cmd) {
	if !t.active {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			t.active = false
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
				if t.cursor < t.scroll {
					t.scroll = t.cursor
				}
			}
		case "down", "j":
			if t.cursor < len(t.items)-1 {
				t.cursor++
				visH := t.height - 10
				if visH < 1 {
					visH = 1
				}
				if t.cursor >= t.scroll+visH {
					t.scroll = t.cursor - visH + 1
				}
			}
		case "enter", "r":
			t.RestoreFile()
		case "d":
			t.PurgeSelected()
		}
	}
	return t, nil
}

// timeAgo returns a human-readable relative time string.
func timeAgo(then time.Time) string {
	d := time.Since(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// fileSize returns a human-readable file size for a path inside the trash dir.
func (t *Trash) fileSize(trashPath string) string {
	info, err := os.Stat(filepath.Join(t.trashDir(), trashPath))
	if err != nil {
		return ""
	}
	size := info.Size()
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}

// View renders the trash overlay.
func (t Trash) View() string {
	width := t.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(red).
		Bold(true).
		Render("  Trash")
	count := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf(" (%d)", len(t.items)))
	b.WriteString(title + count)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	if len(t.items) == 0 {
		b.WriteString(DimStyle.Render("  Trash is empty"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Deleted notes will appear here"))
	} else {
		visH := t.height - 10
		if visH < 5 {
			visH = 5
		}
		end := t.scroll + visH
		if end > len(t.items) {
			end = len(t.items)
		}

		trashIcon := lipgloss.NewStyle().Foreground(red).Render("\uf014 ")
		dimTimeStyle := lipgloss.NewStyle().Foreground(overlay0)
		sizeStyle := lipgloss.NewStyle().Foreground(surface2)

		for i := t.scroll; i < end; i++ {
			item := t.items[i]
			name := strings.TrimSuffix(filepath.Base(item.OrigPath), ".md")
			ago := timeAgo(item.DeletedAt)
			size := t.fileSize(item.TrashPath)

			detail := dimTimeStyle.Render(ago)
			if size != "" {
				detail += sizeStyle.Render("  " + size)
			}

			if i == t.cursor {
				line := "  " + trashIcon + " " + name + "  " + detail
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString("  " + trashIcon + " " + NormalItemStyle.Render(name) + "  " + detail)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter/r", "restore"}, {"d", "delete forever"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
