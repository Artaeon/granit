package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/config"
)

func runInit(targetPath string) {
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Create the vault directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	// Create .granit/ subfolder
	granitDir := filepath.Join(absPath, ".granit")
	if err := os.MkdirAll(granitDir, 0755); err != nil {
		fmt.Printf("Error creating .granit directory: %v\n", err)
		os.Exit(1)
	}

	// Create .granit/plugins/ directory
	pluginsDir := filepath.Join(granitDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		fmt.Printf("Error creating plugins directory: %v\n", err)
		os.Exit(1)
	}

	// Write default per-vault config
	cfg := config.DefaultConfig()
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling config: %v\n", err)
		os.Exit(1)
	}
	cfgPath := filepath.Join(absPath, ".granit.json")
	if err := os.WriteFile(cfgPath, cfgData, 0644); err != nil {
		fmt.Printf("Error writing config: %v\n", err)
		os.Exit(1)
	}

	// Create Welcome.md
	welcomeContent := `---
title: Welcome to Granit
date: ` + todayDate() + `
tags: [getting-started]
---

# Welcome to Granit

Welcome to your new vault! Granit is a terminal-based knowledge manager that is fully compatible with Obsidian vaults.

## Getting Started

- **Create a new note**: Press ` + "`Ctrl+N`" + ` to open the template picker
- **Navigate files**: Use the sidebar (` + "`Tab`" + ` to switch panels)
- **Edit a note**: Press ` + "`Ctrl+E`" + ` to toggle between view and edit mode
- **Search notes**: Press ` + "`Ctrl+P`" + ` for quick open or ` + "`Ctrl+F`" + ` to find in file
- **Link notes**: Type ` + "`[[`" + ` to start a wikilink and see autocomplete suggestions
- **Save**: Press ` + "`Ctrl+S`" + ` to save the current note

## Features

- [[Wikilinks]] for connecting notes
- Backlink tracking and graph visualization (` + "`Ctrl+G`" + `)
- AI-powered analysis with Ollama or OpenAI (` + "`Ctrl+R`" + `)
- Canvas whiteboard (` + "`Ctrl+W`" + `)
- Calendar with agenda view (` + "`Ctrl+L`" + `)
- Command palette (` + "`Ctrl+X`" + `) for quick access to all features
- Plugin system for extending functionality

## Folder Structure

` + "```" + `
your-vault/
  .granit/           # Vault configuration directory
    plugins/         # Vault-local plugins
  .granit.json       # Per-vault settings
  templates/         # Note templates
  Welcome.md         # This file
` + "```" + `

## Next Steps

1. Create your first note with ` + "`Ctrl+N`" + `
2. Explore settings with ` + "`Ctrl+,`" + `
3. Check out the help page with ` + "`F5`" + `
4. Try the command palette with ` + "`Ctrl+X`" + `

Happy note-taking!
`
	welcomePath := filepath.Join(absPath, "Welcome.md")
	if err := os.WriteFile(welcomePath, []byte(welcomeContent), 0644); err != nil {
		fmt.Printf("Error writing Welcome.md: %v\n", err)
		os.Exit(1)
	}

	// Create templates/ folder with default templates
	templatesDir := filepath.Join(absPath, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		fmt.Printf("Error creating templates directory: %v\n", err)
		os.Exit(1)
	}

	templates := map[string]string{
		"Note.md": `---
title: {{title}}
date: {{date}}
tags: []
---

# {{title}}

`,
		"Daily Note.md": `---
date: {{date}}
type: daily
---

# {{date}}

## Tasks
- [ ]

## Notes

## Reflections

`,
		"Meeting Notes.md": `---
title: Meeting - {{title}}
date: {{date}}
type: meeting
attendees: []
tags: [meeting]
---

# Meeting: {{title}}

**Date**: {{date}}
**Attendees**:

## Agenda

1.

## Notes

## Action Items

- [ ]

## Follow-up

`,
	}

	for name, content := range templates {
		tplPath := filepath.Join(templatesDir, name)
		if err := os.WriteFile(tplPath, []byte(content), 0644); err != nil {
			fmt.Printf("Error writing template %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	// Register the vault
	vl := config.LoadVaultList()
	vl.AddVault(absPath)
	config.SaveVaultList(vl)

	// Print success message
	vaultName := filepath.Base(absPath)
	fmt.Printf("Initialized new Granit vault: %s\n", absPath)
	fmt.Println()
	fmt.Println("Created:")
	fmt.Println("  .granit/              Vault configuration directory")
	fmt.Println("  .granit/plugins/      Local plugins directory")
	fmt.Println("  .granit.json          Per-vault settings")
	fmt.Println("  Welcome.md            Getting started guide")
	fmt.Println("  templates/            Note templates")
	fmt.Println("    Note.md")
	fmt.Println("    Daily Note.md")
	fmt.Println("    Meeting Notes.md")
	fmt.Println()
	fmt.Printf("Next steps:\n")
	fmt.Printf("  granit %s             Open your vault\n", vaultName)
	fmt.Printf("  granit open %s        Same, explicit form\n", absPath)
	fmt.Printf("  granit daily %s       Create today's daily note\n", absPath)
}

func todayDate() string {
	now := time.Now()
	return fmt.Sprintf("%d-%02d-%02d", now.Year(), now.Month(), now.Day())
}
