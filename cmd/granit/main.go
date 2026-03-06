package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/tui"
	"github.com/artaeon/granit/internal/vault"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "open":
		if len(os.Args) < 3 {
			fmt.Println("Usage: granit open <vault-path>")
			os.Exit(1)
		}
		runTUI(os.Args[2])

	case "scan":
		if len(os.Args) < 3 {
			fmt.Println("Usage: granit scan <vault-path>")
			os.Exit(1)
		}
		runScan(os.Args[2])

	case "daily":
		vaultPath := "."
		if len(os.Args) >= 3 {
			vaultPath = os.Args[2]
		}
		runDaily(vaultPath)

	case "version":
		fmt.Printf("Granit v%s\n", version)

	case "help":
		printUsage()

	default:
		// If argument is a path, try to open it
		if info, err := os.Stat(command); err == nil && info.IsDir() {
			runTUI(command)
		} else {
			fmt.Printf("Unknown command: %s\n", command)
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Println(`
   ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
  ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
  ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
  ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
  ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
   ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝

  Terminal Knowledge Manager — Obsidian Compatible

Usage:
  granit <vault-path>     Open a vault in the TUI
  granit open <path>      Open a vault in the TUI
  granit scan <path>      Scan vault and print stats
  granit daily [path]     Open/create today's daily note
  granit version          Print version
  granit help             Show this help

Keyboard Shortcuts (TUI):
  Ctrl+1          Focus file sidebar
  Ctrl+2          Focus editor
  Ctrl+3          Focus backlinks panel
  Ctrl+S          Save current note
  Ctrl+Q / Ctrl+C Quit
  Tab             Toggle backlinks/outgoing (in links panel)
  j/k or arrows   Navigate
  Enter           Open selected file/link
  Type to search  Fuzzy filter in sidebar
`)
}

func runTUI(vaultPath string) {
	model, err := tui.NewModel(vaultPath)
	if err != nil {
		fmt.Printf("Error opening vault: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running Granit: %v\n", err)
		os.Exit(1)
	}
}

func runScan(vaultPath string) {
	fmt.Printf("Scanning vault: %s\n", vaultPath)
	fmt.Println(strings.Repeat("─", 40))

	v, err := vault.NewVault(vaultPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	start := time.Now()
	if err := v.Scan(); err != nil {
		fmt.Printf("Error scanning: %v\n", err)
		os.Exit(1)
	}

	idx := vault.NewIndex(v)
	idx.Build()
	elapsed := time.Since(start)

	fmt.Printf("Notes found:  %d\n", v.NoteCount())
	fmt.Printf("Scan time:    %v\n", elapsed)
	fmt.Println(strings.Repeat("─", 40))

	// Print notes with their links
	for _, path := range v.SortedPaths() {
		note := v.GetNote(path)
		fmt.Printf("\n  %s\n", path)

		if len(note.Frontmatter) > 0 {
			fmt.Printf("    Frontmatter: ")
			for k, val := range note.Frontmatter {
				fmt.Printf("%s=%v ", k, val)
			}
			fmt.Println()
		}

		if len(note.Links) > 0 {
			fmt.Printf("    Links: %s\n", strings.Join(note.Links, ", "))
		}

		backlinks := idx.GetBacklinks(path)
		if len(backlinks) > 0 {
			fmt.Printf("    Backlinks: %s\n", strings.Join(backlinks, ", "))
		}
	}
}

func runDaily(vaultPath string) {
	today := time.Now().Format("2006-01-02")
	filename := today + ".md"
	dailyPath := filepath.Join(vaultPath, filename)

	if _, err := os.Stat(dailyPath); os.IsNotExist(err) {
		content := fmt.Sprintf(`---
date: %s
type: daily
---

# %s

## Tasks
- [ ]

## Notes

`, today, today)
		if err := os.WriteFile(dailyPath, []byte(content), 0644); err != nil {
			fmt.Printf("Error creating daily note: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created daily note: %s\n", dailyPath)
	} else {
		fmt.Printf("Daily note exists: %s\n", dailyPath)
	}

	// Open the vault with the daily note
	runTUI(vaultPath)
}
