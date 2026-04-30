package publish

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// copyAssets walks the source folder for non-markdown files (images,
// PDFs, plain attachments) and copies them to the output directory
// preserving the relative path. Markdown files are ALREADY rendered
// elsewhere; this routine handles everything else a note might
// reference.
//
// What gets copied:
//   - Common image extensions: .png .jpg .jpeg .gif .webp .svg .avif
//   - PDFs and plain text attachments: .pdf .txt
//   - Audio/video: .mp3 .mp4 .webm .ogg
//   - Anything else the user dropped beside their notes (.zip etc.)
//
// What gets skipped:
//   - .md / .markdown files (rendered separately)
//   - Hidden directories (.git, .granit, .obsidian)
//   - Temp/lock files (~$..., .DS_Store)
//
// Returns the count of files copied — used in the Result summary so
// the user sees "20 notes + 47 assets" after a build.
func copyAssets(srcDir, outDir string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".granit" || name == ".obsidian" || name == "node_modules" || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		base := d.Name()
		if base == ".DS_Store" || strings.HasPrefix(base, "~$") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := copyFile(path, dst); err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
		count++
		return nil
	})
	return count, err
}

// copyFile writes src to dst with permissions 0o644. Uses streaming I/O
// so large attachments (PDFs, video files) don't load into memory.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// reMDImage matches markdown image syntax with a relative-path src.
// Group 1 = alt text, group 2 = path. We deliberately don't match
// absolute URLs (http/https/data:) since those need no rewriting.
var reMDImage = regexp.MustCompile(`!\[([^\]]*)\]\(((?:\./)?[^)\s]+)\)`)

// rewriteImagePaths takes a note's body, source-path, and the output
// path of its rendered HTML, and rewrites markdown image references so
// they resolve correctly from that output URL. A regular note renders
// to notes/<slug>.html (one level deep), so a relative image path
// `./diagram.png` in the source becomes `../diagram.png` in HTML.
// Legal pages render at the SITE ROOT (/impressum.html), so the same
// reference becomes `diagram.png` (no `../` prefix) — without this
// awareness, images on legal pages would 404 because they'd point
// one level too high.
//
// Skips:
//   - Absolute URLs (http://, https://, //, data:)
//   - Already-rooted paths starting with /
//
// noteRel is the source-relative path of the note (so we know what
// "current directory" the relative image path resolves against).
// outputPath is the publish-time URL of the rendered page, used to
// compute the depth-correction prefix.
func rewriteImagePaths(body, noteRel, outputPath string) string {
	noteDir := filepath.Dir(noteRel)
	if noteDir == "." {
		noteDir = ""
	}
	// depth = number of "/" in outputPath = how many ../ to walk up
	// from the rendered page to reach the site root.
	depth := strings.Count(outputPath, "/")
	prefix := strings.Repeat("../", depth)
	return reMDImage.ReplaceAllStringFunc(body, func(m string) string {
		sub := reMDImage.FindStringSubmatch(m)
		alt, src := sub[1], sub[2]
		if isAbsoluteURL(src) || strings.HasPrefix(src, "/") {
			return m
		}
		resolved := filepath.ToSlash(filepath.Clean(filepath.Join(noteDir, src)))
		return fmt.Sprintf("![%s](%s%s)", alt, prefix, resolved)
	})
}

func isAbsoluteURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "//") ||
		strings.HasPrefix(s, "data:") ||
		strings.HasPrefix(s, "mailto:")
}
