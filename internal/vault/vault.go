package vault

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Note struct {
	Path        string
	RelPath     string
	Title       string
	Frontmatter map[string]interface{}
	Links       []string // outgoing [[wikilinks]]
	Backlinks   []string // notes linking to this one (populated by index)
	Content     string
	ModTime     time.Time
	Size        int64 // file size in bytes (available without loading content)
	loaded      bool  // whether content, frontmatter, and links have been parsed
}

type Vault struct {
	Root  string
	Notes map[string]*Note // keyed by relative path
}

func NewVault(root string) (*Vault, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Vault{
		Root:  absRoot,
		Notes: make(map[string]*Note),
	}, nil
}

func (v *Vault) Scan() error {
	v.Notes = make(map[string]*Note)
	return filepath.Walk(v.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden directories (.obsidian, .git, etc.)
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}

		relPath, _ := filepath.Rel(v.Root, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		note := &Note{
			Path:    path,
			RelPath: relPath,
			Title:   strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())),
			Content: string(content),
			ModTime: info.ModTime(),
			Size:    info.Size(),
			loaded:  true,
		}

		note.Frontmatter = ParseFrontmatter(note.Content)
		note.Links = ParseWikiLinks(note.Content)

		v.Notes[relPath] = note
		return nil
	})
}

// ScanFast collects only file paths, mod times, and sizes without reading
// any file content. Notes are created with loaded=false; their content,
// frontmatter, and links are parsed lazily when first accessed via GetNote.
func (v *Vault) ScanFast() error {
	v.Notes = make(map[string]*Note)
	return filepath.Walk(v.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}

		relPath, _ := filepath.Rel(v.Root, path)
		note := &Note{
			Path:    path,
			RelPath: relPath,
			Title:   strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())),
			ModTime: info.ModTime(),
			Size:    info.Size(),
			loaded:  false,
		}
		v.Notes[relPath] = note
		return nil
	})
}

func (v *Vault) GetNote(relPath string) *Note {
	return v.Notes[relPath]
}

func (v *Vault) NoteCount() int {
	return len(v.Notes)
}

func (v *Vault) SortedPaths() []string {
	paths := make([]string, 0, len(v.Notes))
	for p := range v.Notes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
