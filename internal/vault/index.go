package vault

import (
	"path/filepath"
	"strings"
)

type Index struct {
	vault     *Vault
	Backlinks map[string][]string // target note -> list of source notes
}

func NewIndex(v *Vault) *Index {
	return &Index{
		vault:     v,
		Backlinks: make(map[string][]string),
	}
}

func (idx *Index) Build() {
	idx.Backlinks = make(map[string][]string)

	for srcPath, note := range idx.vault.Notes {
		for _, link := range note.Links {
			targetPath := idx.resolveLink(link)
			if targetPath != "" {
				idx.Backlinks[targetPath] = append(idx.Backlinks[targetPath], srcPath)
			}
		}
	}

	// Populate backlinks on each note
	for notePath, note := range idx.vault.Notes {
		if backlinks, ok := idx.Backlinks[notePath]; ok {
			note.Backlinks = backlinks
		}
	}
}

func (idx *Index) resolveLink(link string) string {
	// Try exact match first (with .md extension)
	if !strings.HasSuffix(link, ".md") {
		link = link + ".md"
	}

	// Direct path match
	if _, exists := idx.vault.Notes[link]; exists {
		return link
	}

	// Search by filename only (Obsidian's shortest-path resolution)
	baseName := filepath.Base(link)
	for notePath := range idx.vault.Notes {
		if filepath.Base(notePath) == baseName {
			return notePath
		}
	}

	return ""
}

func (idx *Index) GetBacklinks(relPath string) []string {
	return idx.Backlinks[relPath]
}

func (idx *Index) GetOutgoingLinks(relPath string) []string {
	note := idx.vault.GetNote(relPath)
	if note == nil {
		return nil
	}
	return note.Links
}

// ResolveLink resolves a wikilink name to a vault-relative path.
func (idx *Index) ResolveLink(link string) string {
	return idx.resolveLink(link)
}
