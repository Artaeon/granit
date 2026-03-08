package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/config"
)

type pluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Enabled     bool   `json:"enabled"`
}

func runPlugin(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: granit plugin <list|install|remove> [name]")
		os.Exit(1)
	}

	subcommand := args[0]
	rest := args[1:]

	switch subcommand {
	case "list":
		pluginList()
	case "install":
		if len(rest) < 1 {
			fmt.Println("Usage: granit plugin install <path>")
			os.Exit(1)
		}
		pluginInstall(rest[0])
	case "remove":
		if len(rest) < 1 {
			fmt.Println("Usage: granit plugin remove <name>")
			os.Exit(1)
		}
		pluginRemove(rest[0])
	default:
		fmt.Printf("Unknown plugin subcommand: %s\n", subcommand)
		fmt.Println("Usage: granit plugin <list|install|remove> [name]")
		os.Exit(1)
	}
}

func pluginList() {
	pluginsDir := filepath.Join(config.ConfigDir(), "plugins")

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No plugins installed.")
			fmt.Printf("Plugin directory: %s\n", pluginsDir)
			return
		}
		fmt.Printf("Error reading plugins directory: %v\n", err)
		os.Exit(1)
	}

	var plugins []pluginInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginsDir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest pluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		plugins = append(plugins, pluginInfo{
			dir:      entry.Name(),
			manifest: manifest,
		})
	}

	if len(plugins) == 0 {
		fmt.Println("No plugins installed.")
		fmt.Printf("Plugin directory: %s\n", pluginsDir)
		return
	}

	fmt.Println("Installed plugins:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("  %-20s %-10s %-10s %s\n", "NAME", "VERSION", "STATUS", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 70))

	for _, p := range plugins {
		status := "disabled"
		if p.manifest.Enabled {
			status = "enabled"
		}
		desc := p.manifest.Description
		if len(desc) > 30 {
			desc = desc[:27] + "..."
		}
		fmt.Printf("  %-20s %-10s %-10s %s\n",
			p.manifest.Name,
			p.manifest.Version,
			status,
			desc,
		)
	}
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("  %d plugin(s) installed\n", len(plugins))
	fmt.Printf("  Plugin directory: %s\n", pluginsDir)
}

type pluginInfo struct {
	dir      string
	manifest pluginManifest
}

func pluginInstall(sourcePath string) {
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Verify source is a directory
	info, err := os.Stat(absSource)
	if err != nil || !info.IsDir() {
		fmt.Printf("Error: %s is not a valid directory\n", absSource)
		os.Exit(1)
	}

	// Verify plugin.json exists
	manifestPath := filepath.Join(absSource, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Printf("Error: no plugin.json found in %s\n", absSource)
		os.Exit(1)
	}

	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		fmt.Printf("Error: invalid plugin.json: %v\n", err)
		os.Exit(1)
	}

	if manifest.Name == "" {
		fmt.Println("Error: plugin.json must have a 'name' field")
		os.Exit(1)
	}

	// Determine destination
	pluginsDir := filepath.Join(config.ConfigDir(), "plugins")
	destDir := filepath.Join(pluginsDir, manifest.Name)

	// Check if already installed
	if _, err := os.Stat(destDir); err == nil {
		fmt.Printf("Plugin %q is already installed at %s\n", manifest.Name, destDir)
		fmt.Println("Remove it first with: granit plugin remove " + manifest.Name)
		os.Exit(1)
	}

	// Create plugins directory
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		fmt.Printf("Error creating plugins directory: %v\n", err)
		os.Exit(1)
	}

	// Copy the plugin directory
	if err := copyDir(absSource, destDir); err != nil {
		fmt.Printf("Error installing plugin: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Installed plugin: %s v%s\n", manifest.Name, manifest.Version)
	if manifest.Description != "" {
		fmt.Printf("  %s\n", manifest.Description)
	}
	if manifest.Author != "" {
		fmt.Printf("  Author: %s\n", manifest.Author)
	}
	fmt.Printf("  Location: %s\n", destDir)
}

func pluginRemove(name string) {
	pluginsDir := filepath.Join(config.ConfigDir(), "plugins")
	pluginDir := filepath.Join(pluginsDir, name)

	// Verify plugin exists
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		fmt.Printf("Error: plugin %q is not installed\n", name)
		fmt.Printf("Use 'granit plugin list' to see installed plugins.\n")
		os.Exit(1)
	}

	// Read manifest for display
	manifestPath := filepath.Join(pluginDir, "plugin.json")
	var manifest pluginManifest
	if data, err := os.ReadFile(manifestPath); err == nil {
		json.Unmarshal(data, &manifest)
	}

	// Remove the directory
	if err := os.RemoveAll(pluginDir); err != nil {
		fmt.Printf("Error removing plugin: %v\n", err)
		os.Exit(1)
	}

	displayName := name
	if manifest.Name != "" {
		displayName = manifest.Name
	}
	fmt.Printf("Removed plugin: %s\n", displayName)
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
