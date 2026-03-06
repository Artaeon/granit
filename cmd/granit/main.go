package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/tui"
	"github.com/artaeon/granit/internal/vault"
)

// Set by goreleaser ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		// No arguments: show vault selector or open last-used vault
		vl := config.LoadVaultList()
		if len(vl.Vaults) == 0 {
			// No known vaults: use current directory
			runTUI(".")
		} else {
			// Show vault selector
			vs := tui.NewVaultSelector()
			p := tea.NewProgram(vs, tea.WithAltScreen())
			finalModel, err := p.Run()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			vsModel, ok := finalModel.(tui.VaultSelector)
			if !ok {
				return
			}
			vsPtr := &vsModel
			if !vsPtr.IsDone() {
				// User quit without selecting
				return
			}
			selected := vsPtr.SelectedVault()
			if selected == "" {
				return
			}
			runTUI(selected)
		}
		return
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
		fmt.Printf("Granit v%s (%s, %s)\n", version, commit, date)

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
	fmt.Print(`
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
  Tab / Shift+Tab Cycle between panels
  F1              Focus file sidebar
  F2              Focus editor
  F3              Focus backlinks panel
  F4              Rename current note
  F5              Show keyboard shortcuts
  Ctrl+P          Quick open (fuzzy search)
  Ctrl+N          Create new note
  Ctrl+S          Save current note
  Ctrl+E          Toggle view/edit mode
  Ctrl+F          Find in file
  Ctrl+H          Find & replace
  Ctrl+J          Quick switch files
  Ctrl+O          Note outline
  Ctrl+B          Bookmarks & recent
  Ctrl+Z          Focus / zen mode
  Ctrl+G          Show note graph
  Ctrl+T          Browse tags
  Ctrl+W          Canvas / whiteboard
  Ctrl+L          Calendar (month/week/agenda)
  Ctrl+R          AI bots (Ollama / local)
  Ctrl+X          Command palette
  Ctrl+,          Settings
  Ctrl+Q / Ctrl+C Quit
  Esc             Return to sidebar / close overlay
  j/k or arrows   Navigate
  Enter           Open selected file/link
  Type to search  Fuzzy filter in sidebar
  PgUp / PgDown   Scroll
`)
}

func runTUI(vaultPath string) {
	// Register this vault in the vault list
	vl := config.LoadVaultList()
	vl.AddVault(vaultPath)
	config.SaveVaultList(vl)

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
