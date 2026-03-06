package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/artaeon/granit/internal/config"
)

// ---------------------------------------------------------------------------
// Manifest types
// ---------------------------------------------------------------------------

type PluginManifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Author      string         `json:"author"`
	Enabled     bool           `json:"enabled"`
	Commands    []PluginCmdDef `json:"commands"`
	Hooks       PluginHooks    `json:"hooks"`
}

type PluginCmdDef struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Run         string `json:"run"`
}

type PluginHooks struct {
	OnSave   string `json:"on_save"`
	OnOpen   string `json:"on_open"`
	OnCreate string `json:"on_create"`
	OnDelete string `json:"on_delete"`
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type Plugin struct {
	Manifest PluginManifest
	Dir      string // absolute path to plugin directory
	Source   string // "global" or "vault"
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

type pluginCmdResultMsg struct {
	pluginName string
	output     string
	err        error
	action     string // "command", "hook"
}

// pendingCmd holds a deferred plugin command to run after the overlay closes.
type pendingCmd struct {
	plugin Plugin
	cmdDef PluginCmdDef
}

// ---------------------------------------------------------------------------
// PluginManager
// ---------------------------------------------------------------------------

type PluginManager struct {
	active    bool
	width     int
	height    int
	plugins   []Plugin
	cursor    int
	scroll    int
	vaultPath string
	detail    bool   // show detail view for selected plugin
	runCursor int    // cursor within plugin commands in detail view
	message   string
	pending   *pendingCmd
}

func NewPluginManager() PluginManager {
	return PluginManager{}
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// LoadPlugins scans both the global and vault-local plugin directories and
// returns all discovered plugins.
func LoadPlugins(vaultPath string) []Plugin {
	var plugins []Plugin

	// Global plugins: ~/.config/granit/plugins/
	globalDir := filepath.Join(config.ConfigDir(), "plugins")
	plugins = append(plugins, loadPluginsFromDir(globalDir, "global")...)

	// Vault-local plugins: <vault>/.granit/plugins/
	if vaultPath != "" {
		vaultDir := filepath.Join(vaultPath, ".granit", "plugins")
		plugins = append(plugins, loadPluginsFromDir(vaultDir, "vault")...)
	}

	return plugins
}

func loadPluginsFromDir(dir, source string) []Plugin {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var plugins []Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		absDir, _ := filepath.Abs(filepath.Join(dir, entry.Name()))
		plugins = append(plugins, Plugin{
			Manifest: manifest,
			Dir:      absDir,
			Source:   source,
		})
	}
	return plugins
}

// ---------------------------------------------------------------------------
// PluginManager methods
// ---------------------------------------------------------------------------

func (pm *PluginManager) SetVaultPath(path string) {
	pm.vaultPath = path
	pm.plugins = LoadPlugins(path)
}

func (pm *PluginManager) Reload() {
	pm.plugins = LoadPlugins(pm.vaultPath)
}

func (pm *PluginManager) GetPlugins() []Plugin {
	return pm.plugins
}

func (pm *PluginManager) EnabledPlugins() []Plugin {
	var enabled []Plugin
	for _, p := range pm.plugins {
		if p.Manifest.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetPluginCommands returns Command structs suitable for inclusion in the
// command palette. They use CmdNone so the palette itself doesn't try to
// handle them — the caller matches by label.
func (pm *PluginManager) GetPluginCommands() []Command {
	var cmds []Command
	for _, p := range pm.plugins {
		if !p.Manifest.Enabled {
			continue
		}
		for _, cd := range p.Manifest.Commands {
			cmds = append(cmds, Command{
				Label:  cd.Label,
				Desc:   cd.Description,
				Action: CmdNone,
				Icon:   &IconBotChar,
			})
		}
	}
	return cmds
}

// ---------------------------------------------------------------------------
// Overlay methods
// ---------------------------------------------------------------------------

func (pm *PluginManager) IsActive() bool {
	return pm.active
}

func (pm *PluginManager) Open() {
	pm.active = true
	pm.cursor = 0
	pm.scroll = 0
	pm.detail = false
	pm.runCursor = 0
	pm.message = ""
	pm.Reload()
}

func (pm *PluginManager) Close() {
	pm.active = false
	pm.detail = false
	pm.message = ""
}

func (pm *PluginManager) SetSize(w, h int) {
	pm.width = w
	pm.height = h
}

// PendingCommand returns (and clears) any plugin command that was queued while
// the overlay was closing.  The caller uses it to fire RunPluginCommand.
func (pm *PluginManager) PendingCommand() *pendingCmd {
	p := pm.pending
	pm.pending = nil
	return p
}

func (pm PluginManager) Update(msg tea.Msg) (PluginManager, tea.Cmd) {
	if !pm.active {
		return pm, nil
	}

	switch msg := msg.(type) {
	case pluginCmdResultMsg:
		if msg.err != nil {
			pm.message = fmt.Sprintf("Plugin %s error: %v", msg.pluginName, msg.err)
		} else {
			pm.message = parsePluginMessage(msg.output, msg.pluginName)
		}
		return pm, nil

	case tea.KeyMsg:
		if pm.detail {
			return pm.updateDetail(msg)
		}
		return pm.updateList(msg)
	}

	return pm, nil
}

func (pm PluginManager) updateList(msg tea.KeyMsg) (PluginManager, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		pm.active = false
		return pm, nil
	case "up", "k":
		if pm.cursor > 0 {
			pm.cursor--
		}
	case "down", "j":
		if pm.cursor < len(pm.plugins)-1 {
			pm.cursor++
		}
	case "enter", " ":
		// Toggle enabled/disabled
		if len(pm.plugins) > 0 && pm.cursor < len(pm.plugins) {
			pm.plugins[pm.cursor].Manifest.Enabled = !pm.plugins[pm.cursor].Manifest.Enabled
			pm.saveManifest(pm.cursor)
		}
	case "d":
		// Show detail view
		if len(pm.plugins) > 0 && pm.cursor < len(pm.plugins) {
			pm.detail = true
			pm.runCursor = 0
		}
	case "r":
		// Run first command of selected plugin
		if len(pm.plugins) > 0 && pm.cursor < len(pm.plugins) {
			p := pm.plugins[pm.cursor]
			if len(p.Manifest.Commands) > 0 {
				pm.pending = &pendingCmd{
					plugin: p,
					cmdDef: p.Manifest.Commands[0],
				}
				pm.active = false
				return pm, nil
			}
		}
	}
	return pm, nil
}

func (pm PluginManager) updateDetail(msg tea.KeyMsg) (PluginManager, tea.Cmd) {
	p := pm.plugins[pm.cursor]
	cmdCount := len(p.Manifest.Commands)

	switch msg.String() {
	case "esc", "q":
		pm.detail = false
		return pm, nil
	case "up", "k":
		if pm.runCursor > 0 {
			pm.runCursor--
		}
	case "down", "j":
		if pm.runCursor < cmdCount-1 {
			pm.runCursor++
		}
	case "enter":
		if cmdCount > 0 && pm.runCursor < cmdCount {
			pm.pending = &pendingCmd{
				plugin: p,
				cmdDef: p.Manifest.Commands[pm.runCursor],
			}
			pm.active = false
			return pm, nil
		}
	}
	return pm, nil
}

// saveManifest writes the current manifest back to plugin.json.
func (pm *PluginManager) saveManifest(idx int) {
	p := pm.plugins[idx]
	data, err := json.MarshalIndent(p.Manifest, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(p.Dir, "plugin.json")
	_ = os.WriteFile(path, data, 0644)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (pm PluginManager) View() string {
	if pm.detail && len(pm.plugins) > 0 && pm.cursor < len(pm.plugins) {
		return pm.viewDetail()
	}
	return pm.viewList()
}

func (pm PluginManager) viewList() string {
	width := pm.width * 2 / 3
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSettingsChar + " Plugin Manager")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	if len(pm.plugins) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No plugins installed"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Add plugins to ~/.config/granit/plugins/"))
		b.WriteString("\n")
	} else {
		visibleItems := pm.height - 10
		if visibleItems < 4 {
			visibleItems = 4
		}

		start := pm.scroll
		if pm.cursor >= start+visibleItems {
			start = pm.cursor - visibleItems + 1
		}
		if pm.cursor < start {
			start = pm.cursor
		}
		pm.scroll = start

		end := start + visibleItems
		if end > len(pm.plugins) {
			end = len(pm.plugins)
		}

		b.WriteString("\n")
		for i := start; i < end; i++ {
			p := pm.plugins[i]
			isSelected := i == pm.cursor

			// Enabled indicator
			indicator := lipgloss.NewStyle().Foreground(red).Render("\u2717")
			if p.Manifest.Enabled {
				indicator = lipgloss.NewStyle().Foreground(green).Render("\u2713")
			}

			// Name and version
			nameStr := p.Manifest.Name
			versionStr := lipgloss.NewStyle().Foreground(overlay0).Render(" v" + p.Manifest.Version)
			authorStr := lipgloss.NewStyle().Foreground(overlay0).Render(p.Manifest.Author)

			// Build line: indicator  Name vX.X.X          author
			labelPart := indicator + " " + nameStr + versionStr
			// Pad to push author to right side
			labelLen := len(p.Manifest.Name) + len(" v"+p.Manifest.Version) + 4 // indicator + spaces
			authorPad := width - 8 - labelLen - len(p.Manifest.Author)
			if authorPad < 2 {
				authorPad = 2
			}
			line := "  " + labelPart + strings.Repeat(" ", authorPad) + authorStr

			if isSelected {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString(NormalItemStyle.Render(line))
			}
			b.WriteString("\n")

			// Description
			desc := "    " + p.Manifest.Description
			if isSelected {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(overlay0).
					Width(width - 6).
					Render(desc))
			} else {
				b.WriteString(DimStyle.Render(desc))
			}
			b.WriteString("\n")

			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Message
	if pm.message != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  " + pm.message))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: toggle  r: run command  d: details  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (pm PluginManager) viewDetail() string {
	p := pm.plugins[pm.cursor]

	width := pm.width * 2 / 3
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	// Header: icon  Name vX.X.X
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSettingsChar + " " + p.Manifest.Name + " v" + p.Manifest.Version)
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  by " + p.Manifest.Author))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	// Description
	b.WriteString(NormalItemStyle.Render("  " + p.Manifest.Description))
	b.WriteString("\n\n")

	// Commands
	if len(p.Manifest.Commands) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Commands:"))
		b.WriteString("\n")
		for i, cmd := range p.Manifest.Commands {
			pointer := "  "
			if i == pm.runCursor {
				pointer = lipgloss.NewStyle().Foreground(peach).Bold(true).Render("> ")
			}

			if i == pm.runCursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render("  " + pointer + cmd.Label))
			} else {
				b.WriteString(NormalItemStyle.Render("  " + pointer + cmd.Label))
			}
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("    " + cmd.Description))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(DimStyle.Render("  No commands defined"))
		b.WriteString("\n")
	}

	// Hooks
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Hooks:"))
	b.WriteString("\n")

	hasHooks := false
	if p.Manifest.Hooks.OnSave != "" {
		b.WriteString(DimStyle.Render("    on_save: " + p.Manifest.Hooks.OnSave))
		b.WriteString("\n")
		hasHooks = true
	}
	if p.Manifest.Hooks.OnOpen != "" {
		b.WriteString(DimStyle.Render("    on_open: " + p.Manifest.Hooks.OnOpen))
		b.WriteString("\n")
		hasHooks = true
	}
	if p.Manifest.Hooks.OnCreate != "" {
		b.WriteString(DimStyle.Render("    on_create: " + p.Manifest.Hooks.OnCreate))
		b.WriteString("\n")
		hasHooks = true
	}
	if p.Manifest.Hooks.OnDelete != "" {
		b.WriteString(DimStyle.Render("    on_delete: " + p.Manifest.Hooks.OnDelete))
		b.WriteString("\n")
		hasHooks = true
	}
	if !hasHooks {
		b.WriteString(DimStyle.Render("    (none)"))
		b.WriteString("\n")
	}

	// Source
	b.WriteString("\n")
	sourceLabel := "global"
	if p.Source == "vault" {
		sourceLabel = "vault-local"
	}
	b.WriteString(DimStyle.Render("  Source: " + sourceLabel + " (" + p.Dir + ")"))
	b.WriteString("\n")

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: run command  Esc: back"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Plugin execution
// ---------------------------------------------------------------------------

// RunPluginCommand runs a single plugin command asynchronously and returns a
// tea.Cmd that produces a pluginCmdResultMsg.
func RunPluginCommand(plugin Plugin, cmdDef PluginCmdDef, notePath, noteContent, vaultPath string) tea.Cmd {
	return func() tea.Msg {
		output, err := executePluginScript(plugin.Dir, cmdDef.Run, notePath, noteContent, vaultPath)
		return pluginCmdResultMsg{
			pluginName: plugin.Manifest.Name,
			output:     output,
			err:        err,
			action:     "command",
		}
	}
}

// RunPluginHook runs all enabled plugins' hooks for the given event name
// concurrently via tea.Batch. Valid hook names: "on_save", "on_open",
// "on_create", "on_delete".
func RunPluginHook(plugins []Plugin, hook string, notePath, noteContent, vaultPath string) tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range plugins {
		if !p.Manifest.Enabled {
			continue
		}
		script := hookScript(p.Manifest.Hooks, hook)
		if script == "" {
			continue
		}
		// Capture loop variable
		plug := p
		scr := script
		cmds = append(cmds, func() tea.Msg {
			output, err := executePluginScript(plug.Dir, scr, notePath, noteContent, vaultPath)
			return pluginCmdResultMsg{
				pluginName: plug.Manifest.Name,
				output:     output,
				err:        err,
				action:     "hook",
			}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// hookScript returns the script path for a given hook name.
func hookScript(hooks PluginHooks, name string) string {
	switch name {
	case "on_save":
		return hooks.OnSave
	case "on_open":
		return hooks.OnOpen
	case "on_create":
		return hooks.OnCreate
	case "on_delete":
		return hooks.OnDelete
	default:
		return ""
	}
}

// executePluginScript runs a plugin script with a 10-second timeout.
// The script receives environment variables and note content via stdin.
func executePluginScript(pluginDir, script, notePath, noteContent, vaultPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Resolve script path relative to plugin directory
	scriptPath := script
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(pluginDir, scriptPath)
	}

	cmd := exec.CommandContext(ctx, scriptPath)
	cmd.Dir = pluginDir

	// Environment variables
	cmd.Env = append(os.Environ(),
		"GRANIT_NOTE_PATH="+notePath,
		"GRANIT_NOTE_NAME="+filepath.Base(notePath),
		"GRANIT_VAULT_PATH="+vaultPath,
	)

	// Pipe note content via stdin
	cmd.Stdin = strings.NewReader(noteContent)

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("plugin timed out after 10 seconds")
	}
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, string(out))
	}

	return string(out), nil
}

// parsePluginMessage processes plugin output lines and returns a human-readable
// summary. Lines starting with MSG: are extracted as status messages. Lines
// starting with CONTENT: or INSERT: carry base64-encoded payloads (decoded
// here for display). Everything else is shown as-is.
func parsePluginMessage(output, pluginName string) string {
	if output == "" {
		return pluginName + ": completed (no output)"
	}

	var messages []string
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "MSG:"):
			messages = append(messages, strings.TrimPrefix(line, "MSG:"))
		case strings.HasPrefix(line, "CONTENT:"):
			payload := strings.TrimPrefix(line, "CONTENT:")
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
			if err == nil {
				messages = append(messages, fmt.Sprintf("[content replaced: %d bytes]", len(decoded)))
			} else {
				messages = append(messages, "[content: decode error]")
			}
		case strings.HasPrefix(line, "INSERT:"):
			payload := strings.TrimPrefix(line, "INSERT:")
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
			if err == nil {
				messages = append(messages, fmt.Sprintf("[insert: %d bytes]", len(decoded)))
			} else {
				messages = append(messages, "[insert: decode error]")
			}
		default:
			if line != "" {
				messages = append(messages, line)
			}
		}
	}

	if len(messages) == 0 {
		return pluginName + ": completed"
	}
	return pluginName + ": " + strings.Join(messages, " | ")
}
