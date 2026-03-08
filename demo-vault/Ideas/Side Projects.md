---
title: Side Project Ideas
date: 2026-02-28
tags: [ideas, projects, hacking, personal]
---

# Side Project Ideas

Weekend hacking ideas and experiments. Some of these may grow into real projects — [[Projects/CLI Tool]] started as an entry on this list.

## Active Ideas

### 1. Markdown-to-Slides Generator
Convert a single markdown file into a presentation. Each `## Heading` becomes a slide. Support for speaker notes via `> Note:` blockquotes.

**Tech:** Go + Glamour for terminal rendering, or WASM for browser export
**Effort:** Medium (2-3 weekends)
**Inspiration:** Marp, Slidev, but terminal-native

### 2. AI-Powered Note Clustering
Use embeddings from Ollama to automatically group similar notes and suggest folder structure. Could integrate directly into Granit.

**Tech:** Go + Ollama embedding API + K-means clustering
**Effort:** High (needs embedding model research)
**Related:** [[Research/Machine Learning Basics]] for clustering algorithms

### 3. Terminal Dashboard for Knowledge Stats
A `btop`-style dashboard showing vault health:
- Notes created per week (bar chart)
- Most connected notes (by link count)
- Orphan note detection
- Tag frequency distribution
- Writing streak tracker

**Tech:** Go + Bubble Tea + Lip Gloss charts
**Effort:** Medium

### 4. Bidirectional Sync with Git
Real-time sync between multiple machines using Git as the transport layer. Auto-commit on save, auto-pull on focus, conflict resolution via 3-way merge.

**Tech:** Go + go-git library
**Effort:** High (conflict resolution is hard)
**Risk:** Merge conflicts with binary files in the vault

## Backlog Ideas

- [ ] **RSS-to-Note importer** — Subscribe to feeds, save articles as markdown notes with tags
- [ ] **Spaced repetition plugin** — Flashcards extracted from notes with `?` markers
- [ ] **Voice-to-note** — Whisper transcription piped into a new note
- [ ] **Vault diff viewer** — Visual diff between note versions using Git history
- [ ] **CLI bookmark manager** — Save URLs with tags, search, export to markdown

## Evaluation Criteria

Before starting a project, ask:
1. Will I actually use this daily? (Must be YES)
2. Can I build an MVP in one weekend?
3. Does it teach me something new?
4. Does it complement the Granit ecosystem?

> "The best side project is one you finish." — from [[Books/The Pragmatic Programmer]]

## Completed Side Projects

- [x] **Granit** — Started as a "weekend Obsidian alternative," now a real project
- [x] **vaultctl** — Now [[Projects/CLI Tool]], promoted from side project

---

*Related: [[Ideas/Blog Post Ideas]] | [[Research/Machine Learning Basics]] | [[Projects/CLI Tool]]*
