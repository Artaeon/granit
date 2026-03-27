package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GetJournalNotes returns the N most recent daily notes (YYYY-MM-DD.md format).
func (a *GranitApp) GetJournalNotes(count int) []NoteDetail {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return nil
	}

	type dated struct {
		date time.Time
		path string
	}

	var dailies []dated
	for relPath := range a.vault.Notes {
		base := strings.TrimSuffix(filepath.Base(relPath), ".md")
		t, err := time.Parse("2006-01-02", base)
		if err != nil {
			continue
		}
		dailies = append(dailies, dated{date: t, path: relPath})
	}

	sort.Slice(dailies, func(i, j int) bool {
		return dailies[i].date.After(dailies[j].date)
	})

	if count > 0 && len(dailies) > count {
		dailies = dailies[:count]
	}

	results := make([]NoteDetail, 0, len(dailies))
	for _, d := range dailies {
		note := a.vault.Notes[d.path]
		if note == nil {
			continue
		}
		backlinks := a.index.GetBacklinks(d.path)
		wordCount := len(strings.Fields(note.Content))
		results = append(results, NoteDetail{
			RelPath:     note.RelPath,
			Title:       note.Title,
			Content:     note.Content,
			Frontmatter: note.Frontmatter,
			Links:       note.Links,
			Backlinks:   backlinks,
			ModTime:     note.ModTime.Format(time.RFC3339),
			WordCount:   wordCount,
		})
	}
	return results
}

// EnsureJournalNote creates a daily note for the given date (YYYY-MM-DD) if it
// doesn't already exist, then returns the note detail.
func (a *GranitApp) EnsureJournalNote(date string) (*NoteDetail, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	// Validate the date format
	_, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
	}

	relPath := date + ".md"

	// Check if it already exists
	if note := a.vault.GetNote(relPath); note != nil {
		backlinks := a.index.GetBacklinks(relPath)
		wordCount := len(strings.Fields(note.Content))
		return &NoteDetail{
			RelPath:     note.RelPath,
			Title:       note.Title,
			Content:     note.Content,
			Frontmatter: note.Frontmatter,
			Links:       note.Links,
			Backlinks:   backlinks,
			ModTime:     note.ModTime.Format(time.RFC3339),
			WordCount:   wordCount,
		}, nil
	}

	// Create it
	content := fmt.Sprintf("- \n")
	absPath := filepath.Join(a.vaultRoot, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, err
	}
	if err := atomicWriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	if err := a.vault.Scan(); err != nil {
		return nil, err
	}
	a.index.Build()

	if a.vault.SearchIndex != nil {
		a.vault.SearchIndex.Update(relPath, content)
	}

	// Use vault.GetNote directly instead of a.GetNote to avoid deadlock
	// (we already hold the write lock).
	note := a.vault.GetNote(relPath)
	if note == nil {
		return nil, fmt.Errorf("note not found after creation: %s", relPath)
	}
	backlinks := a.index.GetBacklinks(relPath)
	wordCount := len(strings.Fields(note.Content))
	return &NoteDetail{
		RelPath:     note.RelPath,
		Title:       note.Title,
		Content:     note.Content,
		Frontmatter: note.Frontmatter,
		Links:       note.Links,
		Backlinks:   backlinks,
		ModTime:     note.ModTime.Format(time.RFC3339),
		WordCount:   wordCount,
	}, nil
}
