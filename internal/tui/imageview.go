package tui

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Terminal image renderer using half-block characters
// ---------------------------------------------------------------------------

func isTerminalImageCapable() bool {
	ct := os.Getenv("COLORTERM")
	return ct == "truecolor" || ct == "24bit"
}

func renderImageTerminal(imagePath string, maxWidth, maxHeight int) (string, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

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

	targetW := maxWidth
	targetH := maxHeight * 2

	scaleX := float64(targetW) / float64(srcW)
	scaleY := float64(targetH) / float64(srcH)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale > 1.0 {
		scale = 1.0
	}

	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	if dstH%2 != 0 {
		dstH++
	}

	// Bilinear interpolation for smoother downscaling
	pixels := make([][]color.Color, dstH)
	for y := 0; y < dstH; y++ {
		pixels[y] = make([]color.Color, dstW)
		srcYf := float64(y) * float64(srcH) / float64(dstH)
		for x := 0; x < dstW; x++ {
			srcXf := float64(x) * float64(srcW) / float64(dstW)
			pixels[y][x] = bilinearSample(img, bounds, srcXf, srcYf)
		}
	}

	var sb strings.Builder
	for y := 0; y < dstH; y += 2 {
		if y > 0 {
			sb.WriteString("\n")
		}
		for x := 0; x < dstW; x++ {
			topR, topG, topB, _ := pixels[y][x].RGBA()
			var botR, botG, botB uint32
			if y+1 < dstH {
				botR, botG, botB, _ = pixels[y+1][x].RGBA()
			}
			tr, tg, tb := topR>>8, topG>>8, topB>>8
			br, bg, bb := botR>>8, botG>>8, botB>>8

			fgColor := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", tr, tg, tb))
			bgColor := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", br, bg, bb))

			style := lipgloss.NewStyle().
				Foreground(fgColor).
				Background(bgColor)
			sb.WriteString(style.Render("\u2580"))
		}
	}

	return sb.String(), nil
}

// bilinearSample performs bilinear interpolation at fractional source coordinates.
func bilinearSample(img image.Image, bounds image.Rectangle, srcX, srcY float64) color.Color {
	x0 := int(srcX) + bounds.Min.X
	y0 := int(srcY) + bounds.Min.Y
	x1 := x0 + 1
	y1 := y0 + 1

	// Clamp to bounds
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if y0 < bounds.Min.Y {
		y0 = bounds.Min.Y
	}
	if x1 >= bounds.Max.X {
		x1 = bounds.Max.X - 1
	}
	if y1 >= bounds.Max.Y {
		y1 = bounds.Max.Y - 1
	}

	fx := srcX - float64(int(srcX))
	fy := srcY - float64(int(srcY))

	r00, g00, b00, a00 := img.At(x0, y0).RGBA()
	r10, g10, b10, a10 := img.At(x1, y0).RGBA()
	r01, g01, b01, a01 := img.At(x0, y1).RGBA()
	r11, g11, b11, a11 := img.At(x1, y1).RGBA()

	lerp := func(v00, v10, v01, v11 uint32) uint8 {
		top := float64(v00)*(1-fx) + float64(v10)*fx
		bot := float64(v01)*(1-fx) + float64(v11)*fx
		return uint8((top*(1-fy) + bot*fy) / 256)
	}

	return color.RGBA{
		R: lerp(r00, r10, r01, r11),
		G: lerp(g00, g10, g01, g11),
		B: lerp(b00, b10, b01, b11),
		A: lerp(a00, a10, a01, a11),
	}
}

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
// Image Manager Overlay
// ---------------------------------------------------------------------------

type imageEntry struct {
	RelPath  string
	AbsPath  string
	Width    int
	Height   int
	FileSize int64
}

type ImageManager struct {
	active       bool
	images       []imageEntry
	cursor       int
	scroll       int
	width        int
	height       int
	vaultRoot    string
	insertResult string
	confirmDel   bool
	preview      string
	previewIdx   int
	statusMsg    string

	// Import mode
	importing bool
	importBuf string
}

func NewImageManager() ImageManager {
	return ImageManager{previewIdx: -1}
}

func (im *ImageManager) SetSize(width, height int) {
	im.width = width
	im.height = height
}

func (im *ImageManager) IsActive() bool {
	return im.active
}

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
	im.importing = false
	im.importBuf = ""
	im.scanImages()
}

func (im *ImageManager) Close() {
	im.active = false
}

func (im *ImageManager) GetInsertResult() (string, bool) {
	if im.insertResult != "" {
		r := im.insertResult
		im.insertResult = ""
		return r, true
	}
	return "", false
}

func (im *ImageManager) scanImages() {
	im.images = nil
	seen := make(map[string]bool)

	imgExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".svg": true, ".webp": true,
		".bmp": true,
	}

	_ = filepath.Walk(im.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == ".granit-trash" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !imgExts[ext] {
			return nil
		}
		absPath, _ := filepath.Abs(path)
		if seen[absPath] {
			return nil
		}
		seen[absPath] = true

		relPath, _ := filepath.Rel(im.vaultRoot, absPath)

		w, h := 0, 0
		if ext != ".svg" && ext != ".webp" {
			f, ferr := os.Open(absPath)
			if ferr == nil {
				cfg, _, decErr := image.DecodeConfig(f)
				_ = f.Close()
				if decErr == nil {
					w = cfg.Width
					h = cfg.Height
				}
			}
		}

		im.images = append(im.images, imageEntry{
			RelPath:  relPath,
			AbsPath:  absPath,
			Width:    w,
			Height:   h,
			FileSize: info.Size(),
		})
		return nil
	})

	sort.Slice(im.images, func(i, j int) bool {
		return im.images[i].RelPath < im.images[j].RelPath
	})
}

func (im *ImageManager) buildPreview() {
	if im.previewIdx == im.cursor {
		return
	}
	im.preview = ""
	im.previewIdx = im.cursor

	if len(im.images) == 0 || im.cursor >= len(im.images) {
		return
	}

	entry := im.images[im.cursor]
	ext := strings.ToLower(filepath.Ext(entry.AbsPath))
	if ext == ".svg" || ext == ".webp" {
		im.preview = lipgloss.NewStyle().Foreground(overlay0).Italic(true).
			Render("  Preview not available for " + ext)
		return
	}

	previewW := im.innerWidth() - 4
	if previewW < 10 {
		previewW = 10
	}
	if previewW > 80 {
		previewW = 80
	}
	previewH := im.previewHeight()

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

	boxW := 40
	sb.WriteString("  " + borderStyle.Render("\u256D"+strings.Repeat("\u2500", boxW)+"\u256E") + "\n")
	sb.WriteString("  " + borderStyle.Render("\u2502") + " " + fileStyle.Render(filepath.Base(entry.AbsPath)) + "\n")
	if entry.Width > 0 {
		sb.WriteString("  " + borderStyle.Render("\u2502") + " " + dimStyle.Render(fmt.Sprintf("%dx%d  %s", entry.Width, entry.Height, formatFileSize(entry.FileSize))) + "\n")
	}
	sb.WriteString("  " + borderStyle.Render("\u2570"+strings.Repeat("\u2500", boxW)+"\u256F"))
	im.preview = sb.String()
}

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

func (im *ImageManager) innerWidth() int {
	w := im.width*3/4 - 6
	if w < 54 {
		w = 54
	}
	if w > 110 {
		w = 110
	}
	return w
}

func (im *ImageManager) listHeight() int {
	// Reserve space for: title(2) + separator(1) + info(1) + separator(1) + preview area + separator(1) + help(1) + padding
	h := im.height/2 - 6
	if h < 4 {
		h = 4
	}
	if h > 20 {
		h = 20
	}
	return h
}

func (im *ImageManager) previewHeight() int {
	h := im.height/2 - 6
	if h < 6 {
		h = 6
	}
	if h > 24 {
		h = 24
	}
	return h
}

func (im ImageManager) Update(msg tea.Msg) (ImageManager, tea.Cmd) {
	if !im.active {
		return im, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Import mode — typing a file path
		if im.importing {
			switch key {
			case "esc":
				im.importing = false
				im.importBuf = ""
			case "enter":
				im.doImport()
			case "backspace":
				if len(im.importBuf) > 0 {
					im.importBuf = im.importBuf[:len(im.importBuf)-1]
				}
			default:
				if len(key) == 1 && key[0] >= 32 {
					im.importBuf += key
				} else if key == " " {
					im.importBuf += " "
				}
			}
			return im, nil
		}

		// Delete confirmation
		if im.confirmDel {
			if key == "y" {
				im.deleteSelected()
			}
			im.confirmDel = false
			return im, nil
		}

		switch key {
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
				openFileExternal(entry.AbsPath)
			}
		case "i":
			im.importing = true
			im.importBuf = ""
			im.statusMsg = ""
		case "c":
			// Copy embed link to status (visual feedback)
			if len(im.images) > 0 && im.cursor < len(im.images) {
				name := filepath.Base(im.images[im.cursor].RelPath)
				im.statusMsg = "Copied: ![[" + name + "]]"
			}
		}
	}

	return im, nil
}

func (im *ImageManager) doImport() {
	srcPath := strings.TrimSpace(im.importBuf)
	im.importing = false
	im.importBuf = ""

	if srcPath == "" {
		return
	}

	// Expand ~ to home dir
	if strings.HasPrefix(srcPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			srcPath = filepath.Join(home, srcPath[2:])
		}
	}

	// Check source exists
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		im.statusMsg = "File not found: " + srcPath
		return
	}
	if srcInfo.IsDir() {
		im.statusMsg = "Cannot import a directory"
		return
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(srcPath))
	validExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".svg": true, ".webp": true, ".bmp": true,
	}
	if !validExts[ext] {
		im.statusMsg = "Not an image: " + ext
		return
	}

	// Ensure attachments dir exists
	destDir := filepath.Join(im.vaultRoot, "attachments")
	_ = os.MkdirAll(destDir, 0755)

	destPath := filepath.Join(destDir, filepath.Base(srcPath))

	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		im.statusMsg = "Already exists: " + filepath.Base(srcPath)
		return
	}

	// Copy file
	src, err := os.Open(srcPath)
	if err != nil {
		im.statusMsg = "Cannot open: " + err.Error()
		return
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(destPath)
	if err != nil {
		im.statusMsg = "Cannot create: " + err.Error()
		return
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		im.statusMsg = "Copy failed: " + err.Error()
		return
	}

	im.statusMsg = "Imported: " + filepath.Base(srcPath)
	im.scanImages()
	im.previewIdx = -1

	// Move cursor to imported file
	for idx, entry := range im.images {
		if entry.AbsPath == destPath {
			im.cursor = idx
			break
		}
	}
}

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
	im.previewIdx = -1
}

// openFileExternal opens a file in the system's default application.
func openFileExternal(path string) {
	for _, opener := range []string{"xdg-open", "open"} {
		// Check if opener exists by trying to resolve it
		if _, err := os.Stat("/usr/bin/" + opener); err == nil {
			p, err := os.StartProcess("/usr/bin/"+opener, []string{opener, path}, &os.ProcAttr{
				Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			})
			if err == nil {
				_ = p.Release()
				return
			}
		}
	}
}

func (im ImageManager) View() string {
	innerW := im.innerWidth()
	totalW := innerW + 6

	var b strings.Builder

	// ── Title ──
	titleStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(titleStyle.Render("  Image Manager"))
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d images", len(im.images))))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerW-2)))
	b.WriteString("\n")

	if len(im.images) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("  No images found in vault."))
		b.WriteString("\n\n")
		b.WriteString(emptyStyle.Render("  Place images in your vault root, or in"))
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("  attachments/, assets/, or images/ folders."))
		b.WriteString("\n\n")
		b.WriteString(emptyStyle.Render("  Press ") + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("i") + emptyStyle.Render(" to import an image."))
		b.WriteString("\n")
	} else {
		im.buildPreview()

		// ── Image List ──
		visH := im.listHeight()
		end := im.scroll + visH
		if end > len(im.images) {
			end = len(im.images)
		}

		nameW := innerW/2 - 4
		if nameW < 20 {
			nameW = 20
		}

		for i := im.scroll; i < end; i++ {
			entry := im.images[i]
			name := entry.RelPath
			if len(name) > nameW {
				name = "..." + name[len(name)-nameW+3:]
			}

			// Right-align info
			var info string
			if entry.Width > 0 {
				info = fmt.Sprintf("%dx%d  %s", entry.Width, entry.Height, formatFileSize(entry.FileSize))
			} else {
				info = formatFileSize(entry.FileSize)
			}

			if i == im.cursor {
				accentBar := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(ThemeAccentBar)
				nameStyled := lipgloss.NewStyle().
					Background(surface0).
					Foreground(blue).
					Bold(true).
					Render(name)
				infoStyled := lipgloss.NewStyle().
					Background(surface0).
					Foreground(overlay0).
					Render(info)
				// Build full line with padding
				leftPart := accentBar + " " + nameStyled
				rightPart := infoStyled + " "
				gap := innerW - lipgloss.Width(leftPart) - lipgloss.Width(rightPart) - 1
				if gap < 1 {
					gap = 1
				}
				padded := lipgloss.NewStyle().Background(surface0).Render(strings.Repeat(" ", gap))
				b.WriteString(leftPart + padded + rightPart)
			} else {
				nameStyled := lipgloss.NewStyle().Foreground(text).Render(name)
				infoStyled := lipgloss.NewStyle().Foreground(overlay0).Render(info)
				leftPart := "  " + nameStyled
				rightPart := infoStyled + " "
				gap := innerW - lipgloss.Width(leftPart) - lipgloss.Width(rightPart) - 1
				if gap < 1 {
					gap = 1
				}
				b.WriteString(leftPart + strings.Repeat(" ", gap) + rightPart)
			}
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(im.images) > visH {
			scrollInfo := fmt.Sprintf("  %d-%d of %d", im.scroll+1, end, len(im.images))
			b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(scrollInfo))
			b.WriteString("\n")
		}

		// ── Separator ──
		b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerW-2)))
		b.WriteString("\n")

		// ── Selected image info ──
		if im.cursor < len(im.images) {
			entry := im.images[im.cursor]
			labelStyle := lipgloss.NewStyle().Foreground(subtext1)
			valStyle := lipgloss.NewStyle().Foreground(text)
			b.WriteString(labelStyle.Render("  File: ") + valStyle.Render(filepath.Base(entry.RelPath)))
			if entry.Width > 0 {
				b.WriteString(labelStyle.Render("  Size: ") + valStyle.Render(fmt.Sprintf("%dx%d", entry.Width, entry.Height)))
			}
			b.WriteString(labelStyle.Render("  Disk: ") + valStyle.Render(formatFileSize(entry.FileSize)))
			embedText := "![[" + filepath.Base(entry.RelPath) + "]]"
			b.WriteString(labelStyle.Render("  Embed: ") + lipgloss.NewStyle().Foreground(green).Render(embedText))
			b.WriteString("\n")
		}

		// ── Preview ──
		if im.preview != "" {
			b.WriteString("\n")
			previewLines := strings.Split(im.preview, "\n")
			for _, line := range previewLines {
				b.WriteString("  " + line + "\n")
			}
		}
	}

	// ── Import mode ──
	if im.importing {
		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		inputStyle := lipgloss.NewStyle().Background(surface0).Foreground(text)
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("│")
		b.WriteString(promptStyle.Render("  Import from: "))
		b.WriteString(inputStyle.Render(im.importBuf+cursor) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  Enter absolute path or ~/... path to image file") + "\n")
	}

	// ── Status message ──
	if im.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  " + im.statusMsg) + "\n")
	}

	// ── Delete confirmation ──
	if im.confirmDel && len(im.images) > 0 && im.cursor < len(im.images) {
		b.WriteString("\n")
		name := filepath.Base(im.images[im.cursor].RelPath)
		b.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Delete " + name + "? (y/n)") + "\n")
	}

	// ── Help bar ──
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerW-2)) + "\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter", "insert"}, {"i", "import"}, {"o", "open"}, {"d", "delete"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(totalW)

	return border.Render(b.String())
}
