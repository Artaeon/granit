package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/plugins"
)

func runPlugin() {
	if len(os.Args) < 3 {
		printPluginUsage()
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "list":
		runPluginList()
	case "install":
		runPluginInstall()
	case "remove":
		runPluginRemove()
	case "enable":
		runPluginEnable()
	case "disable":
		runPluginDisable()
	case "info":
		runPluginInfo()
	case "create":
		runPluginCreate()
	default:
		fmt.Printf("Unknown plugin subcommand: %s\n", subcommand)
		printPluginUsage()
		os.Exit(1)
	}
}

func printPluginUsage() {
	fmt.Print(`Usage: granit plugin <subcommand> [arguments]

Subcommands:
  list                    List all installed plugins
  install <path>          Install a plugin from a local directory
  remove <name>           Remove an installed plugin
  enable <name>           Enable a plugin
  disable <name>          Disable a plugin
  info <name>             Show detailed plugin information
  create <name>           Scaffold a new plugin template

Examples:
  granit plugin list
  granit plugin install ./my-plugin
  granit plugin remove word-count
  granit plugin enable word-count
  granit plugin disable word-count
  granit plugin info word-count
  granit plugin create my-new-plugin
`)
}

func runPluginList() {
	configDir := config.ConfigDir()
	pluginList, err := plugins.ListPlugins(configDir)
	if err != nil {
		exitError("Error listing plugins: %v", err)
	}

	if hasFlag("--json") {
		data, err := json.MarshalIndent(pluginList, "", "  ")
		if err != nil {
			exitError("Error marshaling JSON: %v", err)
		}
		fmt.Println(string(data))
		return
	}

	if len(pluginList) == 0 {
		fmt.Println("No plugins installed.")
		fmt.Printf("  Plugin directory: %s/plugins/\n", configDir)
		fmt.Println("  Use 'granit plugin create <name>' to scaffold a new plugin.")
		return
	}

	fmt.Println("Installed plugins:")
	fmt.Println(strings.Repeat("\u2500", 60))

	for _, p := range pluginList {
		status := "\u2717 disabled"
		if p.Enabled {
			status = "\u2713 enabled"
		}

		fmt.Printf("  %-20s v%-10s %s\n", p.Name, p.Version, status)
		if p.Description != "" {
			fmt.Printf("    %s\n", p.Description)
		}
	}

	fmt.Println(strings.Repeat("\u2500", 60))
	fmt.Printf("  %d plugin(s) installed\n", len(pluginList))
}

func runPluginInstall() {
	if len(os.Args) < 4 {
		exitError("Usage: granit plugin install <path>")
	}

	source := os.Args[3]
	configDir := config.ConfigDir()

	if err := plugins.InstallPlugin(source, configDir); err != nil {
		exitError("Error installing plugin: %v", err)
	}

	// Read back the installed plugin info for display
	info, _ := plugins.ValidatePlugin(source)
	if info != nil {
		fmt.Printf("Installed plugin: %s v%s\n", info.Name, info.Version)
	} else {
		fmt.Println("Plugin installed successfully.")
	}
}

func runPluginRemove() {
	if len(os.Args) < 4 {
		exitError("Usage: granit plugin remove <name>")
	}

	name := os.Args[3]
	configDir := config.ConfigDir()

	if err := plugins.RemovePlugin(name, configDir); err != nil {
		exitError("Error removing plugin: %v", err)
	}

	fmt.Printf("Removed plugin: %s\n", name)
}

func runPluginEnable() {
	if len(os.Args) < 4 {
		exitError("Usage: granit plugin enable <name>")
	}

	name := os.Args[3]
	configDir := config.ConfigDir()

	if err := plugins.EnablePlugin(name, configDir); err != nil {
		exitError("Error enabling plugin: %v", err)
	}

	fmt.Printf("Enabled plugin: %s\n", name)
}

func runPluginDisable() {
	if len(os.Args) < 4 {
		exitError("Usage: granit plugin disable <name>")
	}

	name := os.Args[3]
	configDir := config.ConfigDir()

	if err := plugins.DisablePlugin(name, configDir); err != nil {
		exitError("Error disabling plugin: %v", err)
	}

	fmt.Printf("Disabled plugin: %s\n", name)
}

func runPluginInfo() {
	if len(os.Args) < 4 {
		exitError("Usage: granit plugin info <name>")
	}

	name := os.Args[3]
	configDir := config.ConfigDir()

	pluginList, err := plugins.ListPlugins(configDir)
	if err != nil {
		exitError("Error listing plugins: %v", err)
	}

	var found *plugins.PluginInfo
	for i := range pluginList {
		if pluginList[i].Name == name {
			found = &pluginList[i]
			break
		}
	}

	if found == nil {
		exitError("Plugin %q not found. Use 'granit plugin list' to see installed plugins.", name)
	}

	if hasFlag("--json") {
		data, err := json.MarshalIndent(found, "", "  ")
		if err != nil {
			exitError("Error marshaling JSON: %v", err)
		}
		fmt.Println(string(data))
		return
	}

	status := "disabled"
	if found.Enabled {
		status = "enabled"
	}

	fmt.Printf("Plugin: %s\n", found.Name)
	fmt.Println(strings.Repeat("\u2500", 40))
	fmt.Printf("  Version:     %s\n", found.Version)
	fmt.Printf("  Author:      %s\n", found.Author)
	fmt.Printf("  Status:      %s\n", status)
	fmt.Printf("  Description: %s\n", found.Description)
	fmt.Printf("  Path:        %s\n", found.Path)

	if len(found.Commands) > 0 {
		fmt.Printf("  Commands:    %s\n", strings.Join(found.Commands, ", "))
	} else {
		fmt.Printf("  Commands:    (none)\n")
	}

	if len(found.Hooks) > 0 {
		fmt.Printf("  Hooks:       %s\n", strings.Join(found.Hooks, ", "))
	} else {
		fmt.Printf("  Hooks:       (none)\n")
	}
}

func runPluginCreate() {
	if len(os.Args) < 4 {
		exitError("Usage: granit plugin create <name>")
	}

	name := os.Args[3]

	// Determine output directory: use current directory by default,
	// or --dir=<path> flag if provided.
	dir := "."
	if d := getFlagValue("--dir"); d != "" {
		dir = d
	}

	pluginDir, err := plugins.ScaffoldPlugin(name, dir)
	if err != nil {
		exitError("Error creating plugin: %v", err)
	}

	fmt.Printf("Created plugin scaffold: %s\n", pluginDir)
	fmt.Println()
	fmt.Println("Files created:")
	fmt.Printf("  %s/plugin.json   — plugin manifest\n", pluginDir)
	fmt.Printf("  %s/main.sh       — main script template\n", pluginDir)
	fmt.Printf("  %s/README.md     — development guide\n", pluginDir)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit main.sh with your plugin logic")
	fmt.Println("  2. Update plugin.json with description and commands")
	fmt.Printf("  3. Install with: granit plugin install %s\n", pluginDir)
}
