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
	Root        string
	Notes       map[string]*Note // keyed by relative path
	SearchIndex *SearchIndex     // full-text search index
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
	err := filepath.Walk(v.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
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

		relPath, err := filepath.Rel(v.Root, path)
		if err != nil {
			return nil // skip this file
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files (locked, permission denied)
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
	if err != nil {
		return err
	}

	// Build or rebuild the full-text search index
	if v.SearchIndex == nil {
		v.SearchIndex = NewSearchIndex()
	}
	v.SearchIndex.Build(v)

	return nil
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

		relPath, err := filepath.Rel(v.Root, path)
		if err != nil {
			return nil // skip this file
		}
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

// EnsureLoaded reads the file content and parses frontmatter and links for
// the note at relPath, if it hasn't been loaded yet. Returns false if the
// note does not exist in the vault or if reading fails.
func (v *Vault) EnsureLoaded(relPath string) bool {
	note, exists := v.Notes[relPath]
	if !exists {
		return false
	}
	if note.loaded {
		return true
	}

	content, err := os.ReadFile(note.Path)
	if err != nil {
		return false
	}

	note.Content = string(content)
	note.Frontmatter = ParseFrontmatter(note.Content)
	note.Links = ParseWikiLinks(note.Content)
	note.loaded = true
	return true
}

func (v *Vault) GetNote(relPath string) *Note {
	v.EnsureLoaded(relPath)
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
