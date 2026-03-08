---
title: Getting Started with Granit
date: 2026-03-08
tags: [tutorial, keybindings, onboarding]
---

# Getting Started with Granit

Welcome to Granit! This note covers the essentials to get you productive quickly. For a full overview of this vault, see [[Welcome]].

## Essential Keybindings

### Navigation
| Key | Action |
|-----|--------|
| `Ctrl+J` | Quick file switcher |
| `Ctrl+O` | Note outline (heading jump) |
| `Ctrl+B` | Bookmarks (starred & recent) |
| `Ctrl+L` | Calendar view |
| `Ctrl+X` | Command palette |
| `Tab` | Cycle focus between panels |

### Editing
| Key | Action |
|-----|--------|
| `Ctrl+S` | Save current note |
| `Ctrl+U` | Undo |
| `Ctrl+Y` | Redo |
| `Ctrl+D` | Select word / next occurrence |
| `Ctrl+F` | Find in file |
| `Ctrl+H` | Find and replace |
| `Ctrl+N` | New note from template |

### Multi-Cursor
| Key | Action |
|-----|--------|
| `Ctrl+D` | Add cursor at next match |
| `Ctrl+Shift+Up` | Add cursor above |
| `Ctrl+Shift+Down` | Add cursor below |
| `Esc` | Clear all extra cursors |

### Views & Modes
| Key | Action |
|-----|--------|
| `Ctrl+Z` | Focus (zen) mode |
| `Ctrl+W` | Canvas view |
| `Ctrl+,` | Settings |
| `Ctrl+G` | Graph view |
| `Ctrl+R` | AI bots panel |

## Tips for New Users

1. **Start with daily notes.** Press `Ctrl+N` and select the daily note template, or check out [[Daily/2026-03-08]] for an example.

2. **Link everything.** Type `[[` to trigger autocomplete and connect notes. The more links you create, the more useful the [[MOC - Knowledge Management|graph view]] becomes.

3. **Use tags in frontmatter.** Every note should have a `tags` array in its YAML header. This powers the tag explorer and AI auto-tagger.

4. **Try the AI bots.** Press `Ctrl+R` to access summarizer, auto-tagger, and more. Works with Ollama, OpenAI, or a local fallback.

5. **Explore the command palette.** `Ctrl+X` gives you access to 33+ commands. It supports fuzzy search — just type a few characters.

## Layouts

Granit supports three layouts that adapt to your terminal size:

- **Default** (3 panels) — Sidebar + Editor + Backlinks. Best at 120+ columns.
- **Writer** (2 panels) — Sidebar + Editor. Good for focused writing at 80-120 columns.
- **Minimal** (1 panel) — Editor only. Automatically used below 80 columns, or toggle with focus mode.

## What to Explore Next

- [[Tasks]] — See how task management works with priorities and due dates
- [[Diagrams Example]] — Check out Mermaid diagram rendering
- [[Books/Designing Data-Intensive Applications]] — Example of structured book notes
- [[Projects/Web App Redesign]] — A project note with full task tracking

> **Pro tip:** Use `Ctrl+G` to open the graph view and see how all notes in this vault are connected.
