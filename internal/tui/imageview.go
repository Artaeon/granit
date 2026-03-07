package tui

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Part 1: Terminal image renderer using half-block characters
// ---------------------------------------------------------------------------

// isTerminalImageCapable returns true if the terminal advertises truecolor
// or 256-color support via the COLORTERM environment variable.
func isTerminalImageCapable() bool {
	ct := os.Getenv("COLORTERM")
	return ct == "truecolor" || ct == "24bit"
}

// renderImageTerminal loads an image file and converts it to terminal art
// using upper-half-block characters. Each terminal row encodes two pixel
// rows: the foreground color represents the top pixel and the background
// color represents the bottom pixel.  Returns the rendered string or an error.
func renderImageTerminal(imagePath string, maxWidth, maxHeight int) (string, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return "", fmt.Errorf("image has zero dimensions")
	}

	// Each terminal row represents 2 pixel rows, so we can fit maxHeight*2 pixel rows.
	targetW := maxWidth
	targetH := maxHeight * 2

	// Scale to fit within target dimensions while preserving aspect ratio.
	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale > 1.0 {
		scale = 1.0 // don't upscale
	}

	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	// Ensure even number of pixel rows for half-block pairing.
	if dstH%2 != 0 {
		dstH++
	}

	// Nearest-neighbor downscale: sample pixels from source.
	pixels := make([][]color.Color, dstH)
	for y := 0; y < dstH; y++ {
		pixels[y] = make([]color.Color, dstW)
		srcY := bounds.Min.Y + int(float64(y)*float64(srcH)/float64(dstH))
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < dstW; x++ {
			srcX := bounds.Min.X + int(float64(x)*float64(srcW)/float64(dstW))
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			pixels[y][x] = img.At(srcX, srcY)
		}
	}

	// Render using half-block characters.
	var sb strings.Builder
	for y := 0; y < dstH; y += 2 {
		if y > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("  ") // left margin
		for x := 0; x < dstW; x++ {
			topR, topG, topB, _ := pixels[y][x].RGBA()
			var botR, botG, botB uint32
			if y+1 < dstH {
				botR, botG, botB, _ = pixels[y+1][x].RGBA()
			}
			// Convert from 16-bit to 8-bit color components.
			tr, tg, tb := topR>>8, topG>>8, topB>>8
			br, bg, bb := botR>>8, botG>>8, botB>>8

			fgColor := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", tr, tg, tb))
			bgColor := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", br, bg, bb))

			style := lipgloss.NewStyle().
				Foreground(fgColor).
				Background(bgColor)
			sb.WriteString(style.Render("\u2580")) // upper half block
		}
	}

	return sb.String(), nil
}

// resolveImagePath tries to find an image file in the vault, checking the
// root and common subdirectories.  Returns the absolute path if found.
func resolveImagePath(vaultRoot, filename string) string {
	candidates := []string{
		filepath.Join(vaultRoot, filename),
		filepath.Join(vaultRoot, "attachments", filename),
		filepath.Join(vaultRoot, "assets", filename),
		filepath.Join(vaultRoot, "images", filename),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Part 2: Image Manager Overlay
// ---------------------------------------------------------------------------

// imageEntry holds metadata about a single image found in the vault.
type imageEntry struct {
	RelPath  string // path relative to vault root
	AbsPath  string // absolute path on disk
	Width    int    // pixel width (0 if unreadable)
	Height   int    // pixel height
	FileSize int64  // bytes
}

// ImageManager is an overlay that lists all images in the vault, shows a
// terminal preview of the selected image, and lets the user insert an embed
// link or delete images.
type ImageManager struct {
	active       bool
	images       []imageEntry
	cursor       int
	scroll       int
	width        int
	height       int
	vaultRoot    string
	insertResult string // consumed-once embed text
	confirmDel   bool   // true while awaiting delete confirmation
	preview      string // cached terminal preview for selected image
	previewIdx   int    // cursor index the cached preview was built for
	statusMsg    string // ephemeral status message
}

// NewImageManager creates a new ImageManager.
func NewImageManager() ImageManager {
	return ImageManager{previewIdx: -1}
}

// SetSize updates the overlay dimensions.
func (im *ImageManager) SetSize(width, height int) {
	im.width = width
	im.height = height
}

// IsActive returns whether the overlay is visible.
func (im *ImageManager) IsActive() bool {
	return im.active
}

// Open scans the vault for image files and opens the overlay.
func (im *ImageManager) Open(vaultRoot string) {
	im.active = true
	im.vaultRoot = vaultRoot
	im.cursor = 0
	im.scroll = 0
	im.insertResult = ""
	im.confirmDel = false
	im.preview = ""
	im.previewIdx = -1
	im.statusMsg = ""
	im.scanImages()
}

// Close hides the overlay.
func (im *ImageManager) Close() {
	im.active = false
}

// GetInsertResult returns the wikilink embed text to insert, consuming it.
func (im *ImageManager) GetInsertResult() (string, bool) {
	if im.insertResult != "" {
		r := im.insertResult
		im.insertResult = ""
		return r, true
	}
	return "", false
}

// scanImages walks the vault root and common image directories looking for
// image files.
func (im *ImageManager) scanImages() {
	im.images = nil
	seen := make(map[string]bool)

	dirs := []string{
		im.vaultRoot,
		filepath.Join(im.vaultRoot, "images"),
		filepath.Join(im.vaultRoot, "assets"),
		filepath.Join(im.vaultRoot, "attachments"),
	}

	imgExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".svg": true, ".webp": true,
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if !imgExts[ext] {
				continue
			}
			absPath := filepath.Join(dir, e.Name())
			if seen[absPath] {
				continue
			}
			seen[absPath] = true

			relPath, _ := filepath.Rel(im.vaultRoot, absPath)

			info, err := e.Info()
			var fileSize int64
			if err == nil {
				fileSize = info.Size()
			}

			w, h := 0, 0
			f, err := os.Open(absPath)
			if err == nil {
				cfg, _, decErr := image.DecodeConfig(f)
				f.Close()
				if decErr == nil {
					w = cfg.Width
					h = cfg.Height
				}
			}

			im.images = append(im.images, imageEntry{
				RelPath:  relPath,
				AbsPath:  absPath,
				Width:    w,
				Height:   h,
				FileSize: fileSize,
			})
		}
	}

	// Sort alphabetically by relative path.
	sort.Slice(im.images, func(i, j int) bool {
		return im.images[i].RelPath < im.images[j].RelPath
	})
}

// buildPreview renders a terminal art preview for the currently selected
// image.  Caches the result to avoid re-rendering on every View() call.
func (im *ImageManager) buildPreview() {
	if im.previewIdx == im.cursor {
		return // already cached
	}
	im.preview = ""
	im.previewIdx = im.cursor

	if len(im.images) == 0 || im.cursor >= len(im.images) {
		return
	}

	entry := im.images[im.cursor]

	// Skip SVG/WebP (stdlib can't decode them).
	ext := strings.ToLower(filepath.Ext(entry.AbsPath))
	if ext == ".svg" || ext == ".webp" {
		im.preview = lipgloss.NewStyle().Foreground(overlay0).Italic(true).
			Render("  [preview not available for " + ext + " files]")
		return
	}

	previewW := im.width/2 - 8
	if previewW < 10 {
		previewW = 10
	}
	if previewW > 60 {
		previewW = 60
	}
	previewH := 12

	if isTerminalImageCapable() {
		rendered, err := renderImageTerminal(entry.AbsPath, previewW, previewH)
		if err == nil {
			im.preview = rendered
			return
		}
	}

	// Fallback: text placeholder
	var sb strings.Builder
	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	fileStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	boxW := previewW
	sb.WriteString("  " + borderStyle.Render("\u256D"+strings.Repeat("\u2500", boxW)+"\u256E") + "\n")
	sb.WriteString("  " + borderStyle.Render("\u2502") + " " + fileStyle.Render("IMG  "+filepath.Base(entry.AbsPath)) + "\n")
	if entry.Width > 0 {
		sb.WriteString("  " + borderStyle.Render("\u2502") + " " + dimStyle.Render(fmt.Sprintf("[%dx%d]", entry.Width, entry.Height)) + "\n")
	}
	sb.WriteString("  " + borderStyle.Render("\u2570"+strings.Repeat("\u2500", boxW)+"\u256F"))
	im.preview = sb.String()
}

// formatFileSize returns a human-readable size string.
func formatFileSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}

// Update handles keyboard input for the image manager overlay.
func (im ImageManager) Update(msg tea.Msg) (ImageManager, tea.Cmd) {
	if !im.active {
		return im, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Cancel delete confirmation on any key that isn't "y".
		if im.confirmDel {
			if msg.String() == "y" {
				im.deleteSelected()
			}
			im.confirmDel = false
			return im, nil
		}

		switch msg.String() {
		case "esc":
			im.active = false
		case "up", "k":
			if im.cursor > 0 {
				im.cursor--
				if im.cursor < im.scroll {
					im.scroll = im.cursor
				}
			}
		case "down", "j":
			if im.cursor < len(im.images)-1 {
				im.cursor++
				visH := im.listHeight()
				if im.cursor >= im.scroll+visH {
					im.scroll = im.cursor - visH + 1
				}
			}
		case "enter":
			if len(im.images) > 0 && im.cursor < len(im.images) {
				entry := im.images[im.cursor]
				name := filepath.Base(entry.RelPath)
				im.insertResult = "![[" + name + "]]"
				im.active = false
			}
		case "d":
			if len(im.images) > 0 && im.cursor < len(im.images) {
				im.confirmDel = true
			}
		case "o":
			if len(im.images) > 0 && im.cursor < len(im.images) {
				entry := im.images[im.cursor]
				// Fire and forget — open in system viewer.
				cmd := exec.Command("xdg-open", entry.AbsPath)
				cmd.Start()
			}
		}
	}

	return im, nil
}

// deleteSelected removes the currently selected image from disk and the list.
func (im *ImageManager) deleteSelected() {
	if len(im.images) == 0 || im.cursor >= len(im.images) {
		return
	}
	entry := im.images[im.cursor]
	if err := os.Remove(entry.AbsPath); err != nil {
		im.statusMsg = "Delete failed: " + err.Error()
		return
	}
	im.statusMsg = "Deleted " + filepath.Base(entry.RelPath)
	im.images = append(im.images[:im.cursor], im.images[im.cursor+1:]...)
	if im.cursor >= len(im.images) && im.cursor > 0 {
		im.cursor--
	}
	im.previewIdx = -1 // force preview rebuild
}

// listHeight returns the number of visible list rows.
func (im *ImageManager) listHeight() int {
	h := im.height - 14
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the image manager overlay.
func (im ImageManager) View() string {
	totalW := im.width * 3 / 4
	if totalW < 60 {
		totalW = 60
	}
	if totalW > 100 {
		totalW = 100
	}

	listW := totalW/2 - 2
	if listW < 28 {
		listW = 28
	}

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Image Manager")
	count := lipgloss.NewStyle().Foreground(overlay0).Render(fmt.Sprintf(" (%d)", len(im.images)))
	b.WriteString(title + count + "\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", totalW-6)) + "\n\n")

	if len(im.images) == 0 {
		b.WriteString(DimStyle.Render("  No images found in vault") + "\n")
		b.WriteString(DimStyle.Render("  Place images in root, images/, assets/, or attachments/") + "\n")
	} else {
		im.buildPreview()

		visH := im.listHeight()
		end := im.scroll + visH
		if end > len(im.images) {
			end = len(im.images)
		}

		// Build the left-side list.
		var listLines []string
		for i := im.scroll; i < end; i++ {
			entry := im.images[i]
			name := filepath.Base(entry.RelPath)
			if len(name) > listW-8 {
				name = name[:listW-11] + "..."
			}

			var detail string
			if entry.Width > 0 {
				detail = fmt.Sprintf("%dx%d", entry.Width, entry.Height)
			}
			sizeStr := formatFileSize(entry.FileSize)
			if detail != "" {
				detail += " " + sizeStr
			} else {
				detail = sizeStr
			}

			detailStyled := lipgloss.NewStyle().Foreground(overlay0).Render(detail)

			if i == im.cursor {
				accent := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(ThemeAccentBar)
				nameStyled := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(name)
				line := accent + " " + nameStyled
				listLines = append(listLines, line)
				listLines = append(listLines, "    "+detailStyled)
			} else {
				listLines = append(listLines, "  "+NormalItemStyle.Render(name))
				listLines = append(listLines, "    "+detailStyled)
			}
		}

		// Build the right-side preview.
		var previewLines []string
		if im.preview != "" {
			previewLines = strings.Split(im.preview, "\n")
		} else {
			previewLines = []string{DimStyle.Render("  [no preview]")}
		}

		// Merge list + preview side by side.
		maxLines := len(listLines)
		if len(previewLines) > maxLines {
			maxLines = len(previewLines)
		}
		separator := lipgloss.NewStyle().Foreground(surface1).Render(" \u2502 ")
		for li := 0; li < maxLines; li++ {
			left := ""
			if li < len(listLines) {
				left = listLines[li]
			}
			// Pad the left column to listW using spaces.
			left = imgPadRight(left, listW)

			right := ""
			if li < len(previewLines) {
				right = previewLines[li]
			}

			b.WriteString(left + separator + right + "\n")
		}
	}

	// Status message.
	if im.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  " + im.statusMsg))
		b.WriteString("\n")
	}

	// Confirmation prompt.
	if im.confirmDel && len(im.images) > 0 && im.cursor < len(im.images) {
		b.WriteString("\n")
		name := filepath.Base(im.images[im.cursor].RelPath)
		b.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Delete " + name + "? (y/n)"))
		b.WriteString("\n")
	}

	// Footer.
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", totalW-6)) + "\n")

	enterKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Enter")
	enterDesc := DimStyle.Render(": insert  ")
	delKey := lipgloss.NewStyle().Foreground(red).Bold(true).Render("d")
	delDesc := DimStyle.Render(": delete  ")
	openKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("o")
	openDesc := DimStyle.Render(": open  ")
	escKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Esc")
	escDesc := DimStyle.Render(": close")

	b.WriteString("  " + enterKey + enterDesc + delKey + delDesc + openKey + openDesc + escKey + escDesc)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(totalW).
		Background(mantle)

	return border.Render(b.String())
}

// imgPadRight pads a string with spaces so that its visible width reaches at
// least w characters.  This is a rough approximation — ANSI escape sequences
// make exact width calculation non-trivial, so we use lipgloss.Width.
func imgPadRight(s string, w int) string {
	visible := lipgloss.Width(s)
	if visible >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visible)
}
