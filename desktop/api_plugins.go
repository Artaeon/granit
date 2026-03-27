package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// ==================== Plugin System ====================

// PluginInfoDTO is the JSON-serializable representation of an installed plugin.
type PluginInfoDTO struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Enabled     bool     `json:"enabled"`
	Commands    []string `json:"commands"`
	Hooks       []string `json:"hooks"`
	Path        string   `json:"path"`
}

// pluginManifestDesktop mirrors the TUI PluginManifest for JSON deserialization.
type pluginManifestDesktop struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Version     string                  `json:"version"`
	Author      string                  `json:"author"`
	Enabled     bool                    `json:"enabled"`
	Commands    []pluginCmdDefDesktop   `json:"commands"`
	Hooks       pluginHooksDesktop      `json:"hooks"`
}

type pluginCmdDefDesktop struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Run         string `json:"run"`
}

type pluginHooksDesktop struct {
	OnSave   string `json:"on_save"`
	OnOpen   string `json:"on_open"`
	OnCreate string `json:"on_create"`
	OnDelete string `json:"on_delete"`
}

// GetPlugins discovers plugins in both the global (~/.config/granit/plugins/)
// and vault-local (<vault>/.granit/plugins/) directories and returns their
// manifest information.
func (a *GranitApp) GetPlugins() ([]PluginInfoDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var plugins []PluginInfoDTO

	// Global plugins
	globalDir := filepath.Join(config.ConfigDir(), "plugins")
	plugins = append(plugins, discoverPlugins(globalDir)...)

	// Vault-local plugins
	if a.vaultRoot != "" {
		vaultDir := filepath.Join(a.vaultRoot, ".granit", "plugins")
		plugins = append(plugins, discoverPlugins(vaultDir)...)
	}

	return plugins, nil
}

// discoverPlugins scans a directory for plugin subdirectories containing
// plugin.json manifests and returns PluginInfoDTO entries.
func discoverPlugins(dir string) []PluginInfoDTO {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var plugins []PluginInfoDTO
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest pluginManifestDesktop
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		absDir, _ := filepath.Abs(filepath.Join(dir, entry.Name()))

		// Collect command labels
		var commands []string
		for _, cmd := range manifest.Commands {
			commands = append(commands, cmd.Label)
		}

		// Collect active hooks
		var hooks []string
		if manifest.Hooks.OnSave != "" {
			hooks = append(hooks, "on_save")
		}
		if manifest.Hooks.OnOpen != "" {
			hooks = append(hooks, "on_open")
		}
		if manifest.Hooks.OnCreate != "" {
			hooks = append(hooks, "on_create")
		}
		if manifest.Hooks.OnDelete != "" {
			hooks = append(hooks, "on_delete")
		}

		plugins = append(plugins, PluginInfoDTO{
			Name:        manifest.Name,
			Description: manifest.Description,
			Version:     manifest.Version,
			Author:      manifest.Author,
			Enabled:     manifest.Enabled,
			Commands:    commands,
			Hooks:       hooks,
			Path:        absDir,
		})
	}
	return plugins
}

// TogglePlugin enables or disables a plugin by name. It searches both global
// and vault-local plugin directories and toggles the enabled flag in the
// plugin.json manifest.
func (a *GranitApp) TogglePlugin(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	dirs := []string{filepath.Join(config.ConfigDir(), "plugins")}
	if a.vaultRoot != "" {
		dirs = append(dirs, filepath.Join(a.vaultRoot, ".granit", "plugins"))
	}

	for _, dir := range dirs {
		manifestPath := filepath.Join(dir, name, "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest pluginManifestDesktop
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		manifest.Enabled = !manifest.Enabled

		updated, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal manifest: %w", err)
		}
		if err := atomicWriteFile(manifestPath, updated, 0600); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
		return nil
	}

	return fmt.Errorf("plugin not found: %s", name)
}

// RunPluginCommand executes a named command from a plugin's manifest.
// It locates the plugin by name, finds the command by label, and runs the
// associated script with a 10-second timeout. Environment variables
// GRANIT_VAULT_PATH, GRANIT_NOTE_PATH, and GRANIT_NOTE_NAME are set.
func (a *GranitApp) RunPluginCommand(name string, command string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	dirs := []string{filepath.Join(config.ConfigDir(), "plugins")}
	if a.vaultRoot != "" {
		dirs = append(dirs, filepath.Join(a.vaultRoot, ".granit", "plugins"))
	}

	for _, dir := range dirs {
		pluginDir := filepath.Join(dir, name)
		manifestPath := filepath.Join(pluginDir, "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest pluginManifestDesktop
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		// Find the command by label
		var scriptPath string
		for _, cmd := range manifest.Commands {
			if cmd.Label == command {
				scriptPath = cmd.Run
				break
			}
		}
		if scriptPath == "" {
			return "", fmt.Errorf("command %q not found in plugin %q", command, name)
		}

		// Resolve script path relative to plugin directory
		if !filepath.IsAbs(scriptPath) {
			scriptPath = filepath.Join(pluginDir, scriptPath)
		}

		// Validate script stays within plugin directory
		absPlugin, err := filepath.Abs(pluginDir)
		if err != nil {
			return "", fmt.Errorf("resolve plugin dir: %w", err)
		}
		absScript, err := filepath.Abs(scriptPath)
		if err != nil {
			return "", fmt.Errorf("resolve script path: %w", err)
		}
		if !strings.HasPrefix(absScript, absPlugin+string(filepath.Separator)) {
			return "", fmt.Errorf("script path escapes plugin directory")
		}

		// Verify script exists
		info, err := os.Stat(absScript)
		if err != nil {
			return "", fmt.Errorf("script not found: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("script is not a regular file")
		}

		// Execute with 10-second timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, absScript)
		cmd.Dir = pluginDir
		cmd.Env = append(os.Environ(),
			"GRANIT_VAULT_PATH="+a.vaultRoot,
			"GRANIT_NOTE_PATH=",
			"GRANIT_NOTE_NAME=",
		)

		out, err := cmd.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("plugin timed out after 10 seconds")
		}
		if err != nil {
			return "", fmt.Errorf("%v: %s", err, string(out))
		}

		// Parse output for MSG: lines
		return parsePluginOutput(string(out), name), nil
	}

	return "", fmt.Errorf("plugin not found: %s", name)
}

// parsePluginOutput extracts MSG: lines from plugin output, falling back
// to the raw output if no MSG: lines are found.
func parsePluginOutput(output, pluginName string) string {
	if output == "" {
		return pluginName + ": completed (no output)"
	}

	var messages []string
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MSG:") {
			messages = append(messages, strings.TrimPrefix(line, "MSG:"))
		}
	}

	if len(messages) == 0 {
		return strings.TrimSpace(output)
	}
	return strings.Join(messages, " | ")
}
