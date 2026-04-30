package publish

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// generateOGImage rasterizes a 1200×630 PNG og:image for a single
// note. Black-on-white aesthetic, no graphics — just the note title in
// large bold text plus the site title in smaller regular text. Uses
// the embedded `golang.org/x/image/font/gofont` typeface so no external
// font file is needed.
//
// Output size targets the social-media sweet spot (1200×630 px,
// roughly 1.91:1) — Twitter/Facebook/LinkedIn all crop or letterbox
// other ratios.
//
// Title text wraps automatically across lines when measured width
// would exceed the safe inset. Site title sits at the top-left at a
// smaller weight/size; note title fills the centre.
//
// Returns the relative output path (e.g. "og/<slug>.png") so the
// builder can wire it into og:image meta.
func generateOGImage(outputDir, slug, noteTitle, siteTitle string) (string, error) {
	const (
		W       = 1200
		H       = 630
		margin  = 64
		titlePt = 70
		sitePt  = 28
	)

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	// White background.
	bg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			img.Set(x, y, bg)
		}
	}
	// 1px border on all four sides — gives the card a crisp framed
	// look at small thumbnail sizes where minimal designs otherwise
	// look like blank squares.
	borderCol := color.RGBA{R: 17, G: 17, B: 17, A: 255}
	for x := 0; x < W; x++ {
		img.Set(x, 0, borderCol)
		img.Set(x, H-1, borderCol)
	}
	for y := 0; y < H; y++ {
		img.Set(0, y, borderCol)
		img.Set(W-1, y, borderCol)
	}

	siteFace, err := newFace(goregular.TTF, sitePt)
	if err != nil {
		return "", fmt.Errorf("og: site face: %w", err)
	}
	titleFace, err := newFace(gobold.TTF, titlePt)
	if err != nil {
		return "", fmt.Errorf("og: title face: %w", err)
	}

	// Site title — top-left, muted (#666 — same as the CSS muted var).
	mutedCol := color.RGBA{R: 102, G: 102, B: 102, A: 255}
	drawText(img, siteFace, mutedCol, margin, margin+sitePt, siteTitle)

	// Note title — wrapped to fit. Vertical-centre by computing total
	// wrapped height and offsetting from the top inset.
	titleColor := color.RGBA{R: 17, G: 17, B: 17, A: 255}
	maxLineWidth := W - 2*margin
	lines := wrapText(titleFace, noteTitle, maxLineWidth)
	// Cap to 4 lines — anything longer becomes "..." on the 4th line.
	if len(lines) > 4 {
		lines = lines[:4]
		// Append ellipsis if the last line + "…" still fits.
		last := lines[3] + " …"
		if measureWidth(titleFace, last) <= maxLineWidth {
			lines[3] = last
		} else {
			lines[3] = strings.TrimRight(lines[3], " ") + "…"
		}
	}
	lineHeight := titlePt + 14
	totalHeight := lineHeight * len(lines)
	startY := (H+sitePt+margin)/2 - totalHeight/2 + titlePt
	for i, line := range lines {
		drawText(img, titleFace, titleColor, margin, startY+i*lineHeight, line)
	}

	// "Built with Granit" credit at bottom-right.
	creditPt := 20.0
	creditFace, err := newFace(goregular.TTF, creditPt)
	if err != nil {
		return "", fmt.Errorf("og: credit face: %w", err)
	}
	credit := "Built with Granit"
	cw := measureWidth(creditFace, credit)
	drawText(img, creditFace, mutedCol, W-margin-cw, H-margin, credit)

	// Output to og/<slug>.png inside the site root. og/ is the
	// conventional Hugo / Eleventy folder; reusing keeps the
	// expectations consistent.
	dir := filepath.Join(outputDir, "og")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("og: mkdir: %w", err)
	}
	outPath := filepath.Join(dir, slug+".png")
	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("og: create: %w", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", fmt.Errorf("og: encode: %w", err)
	}
	return "og/" + slug + ".png", nil
}

// newFace loads a TTF byte slice and returns a font.Face at the given
// point size, 96 DPI, and standard hinting. Wraps the boilerplate so
// generateOGImage stays focused on the layout logic.
func newFace(ttf []byte, sizePt float64) (font.Face, error) {
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    sizePt,
		DPI:     96,
		Hinting: font.HintingFull,
	})
}

// drawText renders a string onto img at (x, y) using the given face.
// y is the baseline coordinate, matching the convention of the
// font.Drawer API.
func drawText(img *image.RGBA, face font.Face, col color.Color, x, y int, s string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

// measureWidth returns the advance width of s in pixels using face.
func measureWidth(face font.Face, s string) int {
	advance := font.MeasureString(face, s)
	return advance.Round()
}

// wrapText breaks s into lines that each fit within maxWidth pixels at
// the given face. Word-aware — splits only on spaces, never inside a
// word. If a single word is wider than maxWidth (rare for normal note
// titles) it gets its own line and overflows; we don't hyphenate.
func wrapText(face font.Face, s string, maxWidth int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}
	var lines []string
	var cur string
	for _, w := range words {
		candidate := w
		if cur != "" {
			candidate = cur + " " + w
		}
		if measureWidth(face, candidate) <= maxWidth {
			cur = candidate
			continue
		}
		// Doesn't fit; flush current and start new.
		if cur != "" {
			lines = append(lines, cur)
		}
		cur = w
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
