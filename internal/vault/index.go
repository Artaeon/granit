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

	// Pre-build basename→path map for O(1) basename lookups
	basenameMap := make(map[string]string, len(idx.vault.Notes))
	for notePath := range idx.vault.Notes {
		base := filepath.Base(notePath)
		basenameMap[base] = notePath
		// Also map without extension for wikilink resolution
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		if _, exists := basenameMap[noExt]; !exists {
			basenameMap[noExt] = notePath
		}
	}

	for srcPath, note := range idx.vault.Notes {
		for _, link := range note.Links {
			targetPath := idx.resolveLinkWithMap(link, basenameMap)
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

func (idx *Index) resolveLinkWithMap(link string, basenameMap map[string]string) string {
	// Strip heading anchor (e.g. "note#heading" -> "note") before resolving.
	if hashIdx := strings.Index(link, "#"); hashIdx >= 0 {
		link = link[:hashIdx]
	}
	if link == "" {
		return ""
	}

	// Try exact match first (with .md extension)
	if !strings.HasSuffix(link, ".md") {
		link = link + ".md"
	}

	// Direct path match
	if _, exists := idx.vault.Notes[link]; exists {
		return link
	}

	// Basename lookup via map (O(1) instead of O(n))
	baseName := filepath.Base(link)
	if path, ok := basenameMap[baseName]; ok {
		return path
	}

	return ""
}

func (idx *Index) resolveLink(link string) string {
	// Build a temporary basename map for single-call usage
	basenameMap := make(map[string]string, len(idx.vault.Notes))
	for notePath := range idx.vault.Notes {
		base := filepath.Base(notePath)
		basenameMap[base] = notePath
	}
	return idx.resolveLinkWithMap(link, basenameMap)
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
