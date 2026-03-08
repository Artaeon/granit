package main

import (
	"encoding/json"
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

	case "list":
		if hasFlag("--vaults") {
			runListVaults()
		} else {
			runListNotes()
		}

	case "search":
		runSearch()

	case "capture":
		runCapture()

	case "query":
		runQuery()

	case "plugin":
		runPlugin()

	case "config":
		runConfig()

	case "man":
		fmt.Print(generateManPage())

	case "version", "--version", "-v":
		fmt.Printf("Granit v%s (%s, %s)\n", version, commit, date)

	case "help", "--help", "-h":
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
	fmt.Printf(`
   ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗
  ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝
  ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║
  ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║
  ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║
   ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝

  Terminal Knowledge Manager — Obsidian Compatible
  Version %s

`, version)

	fmt.Print(`USAGE
  granit                        Launch vault selector (or open current dir)
  granit <vault-path>           Open a vault directly
  granit <command> [arguments]

CORE COMMANDS
  open <path>                   Open a vault in the TUI
  daily [path]                  Open or create today's daily note
  help, --help, -h              Show this help message
  version, --version, -v        Print version information

VAULT MANAGEMENT
  scan <path>                   Scan a vault and print statistics (--json)
  list [path]                   List vault notes (--json, --paths, --tags)
  list --vaults                 List all known vaults
  config                        Show configuration paths and current values

PLUGIN MANAGEMENT
  plugin list                   List all installed plugins (--json)
  plugin install <path>         Install a plugin from a local directory
  plugin remove <name>          Remove an installed plugin
  plugin enable <name>          Enable a plugin
  plugin disable <name>         Disable a plugin
  plugin info <name>            Show detailed plugin information (--json)
  plugin create <name>          Scaffold a new plugin template (--dir=<path>)

SEARCH & QUERY
  search <query> [path]         Search vault content (--json, --regex)
  query '<expression>' [path]   Query notes by metadata (--json, --table)
  capture <text>                Quick-capture to Inbox.md (--to, --daily, --stdin)

ADVANCED
  man                           Output roff-formatted man page (pipe to man -l -)

EXAMPLES
  granit ~/notes                Open the vault at ~/notes
  granit open ~/knowledge       Same as above, explicit form
  granit daily                  Create/open today's daily note in current dir
  granit daily ~/notes          Create/open daily note in ~/notes
  granit scan ~/notes           Print vault statistics and link graph
  granit scan ~/notes --json    Output vault stats as JSON
  granit list ~/notes           List all notes as a table
  granit list ~/notes --json    Output notes as JSON array
  granit list ~/notes --paths   Output note paths (one per line, for piping)
  granit list ~/notes --tags    List all unique tags in the vault
  granit search "TODO" ~/notes  Search vault content (grep-like output)
  granit search --regex "#+\s" ~/notes  Search with regex
  granit query 'tag:project'    Find notes with a specific tag
  granit query 'tag:project AND status:active' --table
  granit capture "Buy milk"     Append to Inbox.md
  granit capture --daily "Met with team"  Append to today's daily note
  echo "idea" | granit capture --stdin    Capture from stdin
  granit list --vaults          Show registered vaults with last-opened dates
  granit config                 Display active configuration
  granit man | man -l -         View the full manual page

ENVIRONMENT VARIABLES
  GRANIT_VAULT                  Default vault path (used when no path given)
  EDITOR                        Preferred external editor for shell-out

CONFIGURATION FILES
  ~/.config/granit/config.json  Global settings (theme, keybindings, AI, etc.)
  ~/.config/granit/vaults.json  Known vault registry
  <vault>/.granit.json          Per-vault setting overrides
  ~/.config/granit/plugins/     Global plugin directory
  <vault>/.granit/plugins/      Vault-local plugin directory

KEYBOARD SHORTCUTS (TUI)

  Navigation
    Tab / Shift+Tab             Cycle between panels
    F1                          Focus file sidebar
    F2                          Focus editor
    F3                          Focus backlinks panel
    j/k or Up/Down              Navigate lists
    Enter                       Open selected file or follow link
    PgUp / PgDn                 Scroll page up/down
    Type to search              Fuzzy filter in sidebar

  Editing
    Ctrl+S                      Save current note
    Ctrl+E                      Toggle view/edit mode
    Ctrl+U                      Undo
    Ctrl+Y                      Redo
    Ctrl+F                      Find in file
    Ctrl+H                      Find and replace
    Ctrl+D                      Multi-cursor: select word / next occurrence
    Ctrl+Shift+Up/Down          Add cursor above/below

  Overlays
    Ctrl+P                      Quick open (fuzzy file search)
    Ctrl+N                      Create new note (template picker)
    Ctrl+J                      Quick switch files
    Ctrl+O                      Note outline (headings)
    Ctrl+B                      Bookmarks and recent notes
    Ctrl+G                      Note graph visualization
    Ctrl+T                      Browse tags
    Ctrl+W                      Canvas / whiteboard
    Ctrl+L                      Calendar (month/week/agenda)
    Ctrl+X                      Command palette
    Ctrl+,                      Settings
    F4                          Rename current note
    F5                          Show keyboard shortcuts
    Ctrl+Z                      Focus / zen mode
    Esc                         Close overlay / return to sidebar

  AI Bots
    Ctrl+R                      Open AI bots panel
                                Providers: Ollama, OpenAI, local fallback
                                9 bots: tagger, linker, summarizer, Q&A,
                                writing assistant, titles, action items,
                                MOC generator, daily digest

  Application
    Ctrl+Q                      Quit
    Ctrl+C                      Quit
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

	if hasFlag("--json") {
		runScanJSON(v, idx, elapsed)
		return
	}

	// Default: human-readable output
	fmt.Printf("Scanning vault: %s\n", vaultPath)
	fmt.Println(strings.Repeat("─", 40))
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

// ScanStats is the JSON representation for scan --json output.
type ScanStats struct {
	VaultPath  string     `json:"vault_path"`
	NoteCount  int        `json:"note_count"`
	TotalWords int        `json:"total_words"`
	TotalLinks int        `json:"total_links"`
	TotalTags  int        `json:"total_tags"`
	ScanTimeMs int64      `json:"scan_time_ms"`
	Notes      []ScanNote `json:"notes"`
}

// ScanNote is a single note entry in the scan JSON.
type ScanNote struct {
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Words     int      `json:"words"`
	Links     []string `json:"links,omitempty"`
	Backlinks []string `json:"backlinks,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Modified  string   `json:"modified"`
}

func runScanJSON(v *vault.Vault, idx *vault.Index, elapsed time.Duration) {
	compact := hasFlag("--compact")

	stats := ScanStats{
		VaultPath:  v.Root,
		NoteCount:  v.NoteCount(),
		ScanTimeMs: elapsed.Milliseconds(),
	}

	allTags := make(map[string]bool)
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		tags := extractTags(note)
		words := countWords(note.Content)
		backlinks := idx.GetBacklinks(p)

		stats.TotalWords += words
		stats.TotalLinks += len(note.Links)
		for _, t := range tags {
			allTags[t] = true
		}

		stats.Notes = append(stats.Notes, ScanNote{
			Path:      note.RelPath,
			Title:     note.Title,
			Words:     words,
			Links:     note.Links,
			Backlinks: backlinks,
			Tags:      tags,
			Modified:  note.ModTime.Format("2006-01-02"),
		})
	}
	stats.TotalTags = len(allTags)

	var data []byte
	var jsonErr error
	if compact {
		data, jsonErr = json.Marshal(stats)
	} else {
		data, jsonErr = json.MarshalIndent(stats, "", "  ")
	}
	if jsonErr != nil {
		exitError("Error marshaling JSON: %v", jsonErr)
	}
	fmt.Println(string(data))
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

func runListVaults() {
	vl := config.LoadVaultList()
	if len(vl.Vaults) == 0 {
		fmt.Println("No known vaults. Open a directory with 'granit <path>' to register it.")
		return
	}

	fmt.Println("Known vaults:")
	fmt.Println(strings.Repeat("─", 60))
	for _, v := range vl.Vaults {
		marker := "  "
		if v.Path == vl.LastUsed {
			marker = "* "
		}
		fmt.Printf("%s%-30s  %s  (last: %s)\n", marker, v.Name, v.Path, v.LastOpen)
	}
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("  %d vault(s) registered (* = last used)\n", len(vl.Vaults))
}

func runConfig() {
	cfg := config.Load()

	fmt.Println("Granit Configuration")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Global config:   %s\n", config.ConfigPath())
	fmt.Printf("  Config dir:      %s\n", config.ConfigDir())
	fmt.Printf("  Vaults file:     %s\n", filepath.Join(config.ConfigDir(), "vaults.json"))
	fmt.Printf("  Plugins dir:     %s\n", filepath.Join(config.ConfigDir(), "plugins"))
	fmt.Println()

	// Check which files actually exist
	globalExists := "missing"
	if _, err := os.Stat(config.ConfigPath()); err == nil {
		globalExists = "exists"
	}
	vaultsExists := "missing"
	vaultsPath := filepath.Join(config.ConfigDir(), "vaults.json")
	if _, err := os.Stat(vaultsPath); err == nil {
		vaultsExists = "exists"
	}
	fmt.Printf("  Global config:   [%s]\n", globalExists)
	fmt.Printf("  Vaults file:     [%s]\n", vaultsExists)
	fmt.Println()

	fmt.Println("Current settings:")
	fmt.Println(strings.Repeat("─", 50))

	data, err := json.MarshalIndent(cfg, "  ", "  ")
	if err != nil {
		fmt.Printf("  Error marshaling config: %v\n", err)
		return
	}
	fmt.Printf("  %s\n", string(data))
}
