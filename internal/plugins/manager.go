package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PluginInfo describes a discovered or installable plugin.
type PluginInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Enabled     bool     `json:"enabled"`
	Commands    []string `json:"commands"`
	Hooks       []string `json:"hooks"`
	Path        string   `json:"-"`
}

// pluginManifest mirrors the on-disk plugin.json structure.
type pluginManifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Author      string         `json:"author"`
	Enabled     bool           `json:"enabled"`
	Commands    []pluginCmdDef `json:"commands"`
	Hooks       pluginHooks    `json:"hooks"`
}

type pluginCmdDef struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Run         string `json:"run"`
}

type pluginHooks struct {
	OnSave   string `json:"on_save"`
	OnOpen   string `json:"on_open"`
	OnCreate string `json:"on_create"`
	OnDelete string `json:"on_delete"`
}

// ListPlugins returns all plugins found in the global plugins directory
// (<configDir>/plugins/).
func ListPlugins(configDir string) ([]PluginInfo, error) {
	pluginsDir := filepath.Join(configDir, "plugins")

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}

	var plugins []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsDir, entry.Name())
		info, err := ValidatePlugin(dir)
		if err != nil {
			continue // skip invalid plugins
		}
		plugins = append(plugins, *info)
	}

	return plugins, nil
}

// InstallPlugin copies a plugin from source (a local directory path) into the
// global plugins directory. The source must contain a valid plugin.json.
func InstallPlugin(source string, configDir string) error {
	// Validate the source
	info, err := ValidatePlugin(source)
	if err != nil {
		return fmt.Errorf("invalid plugin at %q: %w", source, err)
	}

	pluginsDir := filepath.Join(configDir, "plugins")
	destDir := filepath.Join(pluginsDir, info.Name)

	// Check for duplicate
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("plugin %q is already installed", info.Name)
	}

	// Create the destination directory
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	// Copy all files from source to destination
	entries, err := os.ReadDir(source)
	if err != nil {
		// Clean up on failure
		_ = os.RemoveAll(destDir)
		return fmt.Errorf("read source dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // only copy top-level files
		}
		srcFile := filepath.Join(source, entry.Name())
		dstFile := filepath.Join(destDir, entry.Name())

		if err := copyFile(srcFile, dstFile); err != nil {
			_ = os.RemoveAll(destDir)
			return fmt.Errorf("copy %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// RemovePlugin deletes a plugin directory from the global plugins directory.
func RemovePlugin(name string, configDir string) error {
	pluginsDir := filepath.Join(configDir, "plugins")
	pluginDir := filepath.Join(pluginsDir, name)

	// Verify it exists
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin %q is not installed", name)
	}

	// Verify it has a valid manifest (safety check to avoid deleting random dirs)
	if _, err := ValidatePlugin(pluginDir); err != nil {
		return fmt.Errorf("plugin %q has invalid manifest, refusing to remove: %w", name, err)
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("remove plugin: %w", err)
	}

	return nil
}

// EnablePlugin sets the enabled field to true in the plugin's manifest.
func EnablePlugin(name string, configDir string) error {
	return setPluginEnabled(name, configDir, true)
}

// DisablePlugin sets the enabled field to false in the plugin's manifest.
func DisablePlugin(name string, configDir string) error {
	return setPluginEnabled(name, configDir, false)
}

// ValidatePlugin reads and validates a plugin.json in the given directory.
// Returns a PluginInfo on success.
func ValidatePlugin(path string) (*PluginInfo, error) {
	manifestPath := filepath.Join(path, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m pluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Validate required fields
	if m.Name == "" {
		return nil, fmt.Errorf("missing required field: name")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("missing required field: version")
	}

	// Extract command labels
	var commands []string
	for _, cmd := range m.Commands {
		commands = append(commands, cmd.Label)
	}

	// Extract active hooks
	var hooks []string
	if m.Hooks.OnSave != "" {
		hooks = append(hooks, "on_save")
	}
	if m.Hooks.OnOpen != "" {
		hooks = append(hooks, "on_open")
	}
	if m.Hooks.OnCreate != "" {
		hooks = append(hooks, "on_create")
	}
	if m.Hooks.OnDelete != "" {
		hooks = append(hooks, "on_delete")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	return &PluginInfo{
		Name:        m.Name,
		Description: m.Description,
		Version:     m.Version,
		Author:      m.Author,
		Enabled:     m.Enabled,
		Commands:    commands,
		Hooks:       hooks,
		Path:        absPath,
	}, nil
}

// ScaffoldPlugin creates a new plugin directory with a template manifest,
// script, and README.
func ScaffoldPlugin(name string, dir string) (string, error) {
	pluginDir := filepath.Join(dir, name)

	if _, err := os.Stat(pluginDir); err == nil {
		return "", fmt.Errorf("directory %q already exists", pluginDir)
	}

	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return "", fmt.Errorf("create plugin dir: %w", err)
	}

	// Write plugin.json
	manifest := pluginManifest{
		Name:        name,
		Description: "A custom Granit plugin",
		Version:     "0.1.0",
		Author:      "Your Name",
		Enabled:     true,
		Commands: []pluginCmdDef{
			{
				Label:       titleCase(name),
				Description: "Run " + name,
				Run:         "main.sh",
			},
		},
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), manifestData, 0600); err != nil {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// Write main.sh
	script := `#!/bin/bash
# Plugin: ` + name + `
# Environment variables available:
#   GRANIT_NOTE_PATH  — full path to the current note
#   GRANIT_NOTE_NAME  — filename of the current note
#   GRANIT_VAULT_PATH — path to the vault root
#
# Note content is passed via stdin.
# Output format:
#   MSG:<message>                — display a status message
#   CONTENT:<base64>             — replace note content (base64-encoded)
#   INSERT:<base64>              — insert text at cursor (base64-encoded)

# Example: count words in the current note
WORDS=$(wc -w < /dev/stdin)
echo "MSG:` + name + `: $WORDS words"
`

	if err := os.WriteFile(filepath.Join(pluginDir, "main.sh"), []byte(script), 0700); err != nil {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("write script: %w", err)
	}

	// Write README.md
	readme := `# ` + name + `

A custom plugin for Granit.

## Installation

Copy this directory to ` + "`~/.config/granit/plugins/`" + ` or use:

` + "```" + `
granit plugin install .
` + "```" + `

## Configuration

Edit ` + "`plugin.json`" + ` to configure commands and hooks.

### Commands

Commands appear in the Granit command palette and can be run from the
plugin manager overlay.

### Hooks

Available hooks:
- ` + "`on_save`" + `   — runs when a note is saved
- ` + "`on_open`" + `   — runs when a note is opened
- ` + "`on_create`" + ` — runs when a new note is created
- ` + "`on_delete`" + ` — runs when a note is deleted

### Script Protocol

Scripts receive these environment variables:
- ` + "`GRANIT_NOTE_PATH`" + `  — full path to the current note
- ` + "`GRANIT_NOTE_NAME`" + `  — filename of the current note
- ` + "`GRANIT_VAULT_PATH`" + ` — path to the vault root

Note content is passed via stdin. Scripts have a 10-second timeout.

Output lines:
- ` + "`MSG:<text>`" + `          — show a status message
- ` + "`CONTENT:<base64>`" + `    — replace the note content (base64-encoded)
- ` + "`INSERT:<base64>`" + `     — insert text at cursor position (base64-encoded)
`

	if err := os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte(readme), 0600); err != nil {
		_ = os.RemoveAll(pluginDir)
		return "", fmt.Errorf("write README: %w", err)
	}

	return pluginDir, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setPluginEnabled(name string, configDir string, enabled bool) error {
	pluginsDir := filepath.Join(configDir, "plugins")
	pluginDir := filepath.Join(pluginsDir, name)

	manifestPath := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plugin %q is not installed", name)
		}
		return fmt.Errorf("read manifest: %w", err)
	}

	var m pluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	m.Enabled = enabled

	updated, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, updated, 0600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// titleCase converts a hyphen-separated name to title case.
func titleCase(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
