package vault

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noteNames is a pool of realistic note titles used to generate wikilinks.
var noteNames = []string{
	"Getting Started", "Project Ideas", "Meeting Notes", "Daily Journal",
	"Book Review", "Research Plan", "Architecture Overview", "API Design",
	"Team Retrospective", "Sprint Planning", "Bug Tracker", "Feature Roadmap",
	"User Stories", "Deployment Guide", "Testing Strategy", "Code Review",
	"Performance Tuning", "Security Audit", "Data Migration", "Release Notes",
	"Onboarding Guide", "Style Guide", "Design System", "Component Library",
	"Database Schema", "Network Topology", "CI Pipeline", "Monitoring Setup",
	"Incident Report", "Post Mortem", "Decision Record", "Technical Debt",
	"Refactoring Plan", "Dependency Update", "License Compliance", "Accessibility",
	"Internationalization", "Analytics Dashboard", "Customer Feedback", "Roadmap Q1",
}

// generateNoteContent creates realistic markdown content with frontmatter,
// wikilinks, and body text. The rng parameter controls randomness so
// benchmarks remain reproducible within a run.
func generateNoteContent(rng *rand.Rand, title string, linkCount int) string {
	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	sb.WriteString("title: " + title + "\n")
	tags := []string{"project", "notes", "ideas", "research", "dev", "design", "ops"}
	numTags := 1 + rng.Intn(3)
	sb.WriteString("tags: [")
	for i := 0; i < numTags; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(tags[rng.Intn(len(tags))])
	}
	sb.WriteString("]\n")
	sb.WriteString("date: 2026-03-08\n")
	sb.WriteString("---\n\n")

	// Heading
	sb.WriteString("# " + title + "\n\n")

	// Body paragraphs with wikilinks
	paragraphs := []string{
		"This document outlines the key concepts and strategies we have been exploring over the past several weeks. The team has made significant progress in multiple areas.",
		"We should consider the implications of the recent changes on our overall architecture. Performance testing has revealed several bottlenecks that need to be addressed before the next release.",
		"The integration tests are passing consistently now, which gives us confidence to proceed with the deployment. However, we still need to review the edge cases identified during the last code review session.",
		"Documentation needs to be updated to reflect the new API endpoints. The current version is outdated and may confuse new team members who are onboarding this quarter.",
		"Looking ahead, we plan to refactor the authentication module to support OAuth 2.0. This will require coordination with the security team and a thorough review of our token management approach.",
	}

	for i, para := range paragraphs {
		sb.WriteString(para)
		if i < linkCount && i < len(noteNames) {
			sb.WriteString(" See [[" + noteNames[rng.Intn(len(noteNames))] + "]] for more details.")
		}
		sb.WriteString("\n\n")
	}

	// Sub-heading
	sb.WriteString("## Next Steps\n\n")
	sb.WriteString("- Review the current implementation\n")
	sb.WriteString("- Update the test suite\n")
	sb.WriteString("- Schedule a follow-up meeting\n")

	return sb.String()
}

// createTempVault populates a temporary directory with n markdown files and
// returns the directory path.
func createTempVault(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	rng := rand.New(rand.NewSource(42))

	// Create a couple of subdirectories for realism.
	subdirs := []string{"", "projects", "journal", "research"}
	for _, sub := range subdirs[1:] {
		_ = os.MkdirAll(filepath.Join(dir, sub), 0755)
	}

	for i := 0; i < n; i++ {
		subdir := subdirs[i%len(subdirs)]
		filename := fmt.Sprintf("note-%04d.md", i)
		title := fmt.Sprintf("Note %d", i)
		content := generateNoteContent(rng, title, 2+rng.Intn(4))
		path := filepath.Join(dir, subdir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			b.Fatalf("failed to create test file: %v", err)
		}
	}
	return dir
}

func BenchmarkScan(b *testing.B) {
	for _, size := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("notes=%d", size), func(b *testing.B) {
			dir := createTempVault(b, size)
			v, err := NewVault(dir)
			if err != nil {
				b.Fatalf("NewVault: %v", err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := v.Scan(); err != nil {
					b.Fatalf("Scan: %v", err)
				}
			}
		})
	}
}

func BenchmarkGetNote(b *testing.B) {
	dir := createTempVault(b, 500)
	v, err := NewVault(dir)
	if err != nil {
		b.Fatalf("NewVault: %v", err)
	}
	if err := v.ScanFast(); err != nil {
		b.Fatalf("ScanFast: %v", err)
	}
	paths := v.SortedPaths()
	target := paths[len(paths)/2] // pick a note in the middle

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		note := v.GetNote(target)
		if note == nil {
			b.Fatal("GetNote returned nil")
		}
	}
}

func BenchmarkSortedPaths(b *testing.B) {
	dir := createTempVault(b, 500)
	v, err := NewVault(dir)
	if err != nil {
		b.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		b.Fatalf("Scan: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.SortedPaths()
	}
}

func BenchmarkParseWikiLinks(b *testing.B) {
	shortContent := "See [[Note A]] and [[Note B]] for details."
	mediumContent := strings.Repeat("This is a paragraph referencing [[Project Ideas]] and [[Meeting Notes]]. ", 20)
	longContent := strings.Repeat("Deep dive into [[Architecture Overview]], [[API Design]], [[Testing Strategy]], and [[Performance Tuning]]. Plus some filler text to make this longer. ", 50)

	cases := []struct {
		name    string
		content string
	}{
		{"short", shortContent},
		{"medium", mediumContent},
		{"long", longContent},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = ParseWikiLinks(tc.content)
			}
		})
	}
}

func BenchmarkParseFrontmatter(b *testing.B) {
	content := `---
title: Benchmark Note
tags: [go, testing, performance]
date: 2026-03-08
author: tester
category: engineering
status: draft
---

# Benchmark Note

This is the body of the note.
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ParseFrontmatter(content)
	}
}

func BenchmarkStripFrontmatter(b *testing.B) {
	content := `---
title: Benchmark Note
tags: [go, testing, performance]
date: 2026-03-08
---

# Benchmark Note

This is the body that should remain after stripping frontmatter.
More content follows to make the body realistic.
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = StripFrontmatter(content)
	}
}

func BenchmarkIndexBuild(b *testing.B) {
	for _, size := range []int{100, 500} {
		b.Run(fmt.Sprintf("notes=%d", size), func(b *testing.B) {
			dir := createTempVault(b, size)
			v, err := NewVault(dir)
			if err != nil {
				b.Fatalf("NewVault: %v", err)
			}
			if err := v.Scan(); err != nil {
				b.Fatalf("Scan: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := NewIndex(v)
				idx.Build()
			}
		})
	}
}

func BenchmarkIndexGetBacklinks(b *testing.B) {
	dir := createTempVault(b, 500)
	v, err := NewVault(dir)
	if err != nil {
		b.Fatalf("NewVault: %v", err)
	}
	if err := v.Scan(); err != nil {
		b.Fatalf("Scan: %v", err)
	}
	idx := NewIndex(v)
	idx.Build()

	paths := v.SortedPaths()
	target := paths[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.GetBacklinks(target)
	}
}
