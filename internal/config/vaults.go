package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// VaultEntry represents a single known vault.
type VaultEntry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	LastOpen string `json:"last_open"` // ISO date
}

// VaultList holds the list of known vaults and the last-used vault path.
type VaultList struct {
	Vaults   []VaultEntry `json:"vaults"`
	LastUsed string       `json:"last_used"` // path of last opened vault
}

// vaultsPath returns the path to the vaults.json file.
func vaultsPath() string {
	return filepath.Join(ConfigDir(), "vaults.json")
}

// LoadVaultList loads the vault list from ~/.config/granit/vaults.json.
// If the file does not exist or cannot be parsed, an empty VaultList is returned.
func LoadVaultList() VaultList {
	var vl VaultList
	data, err := os.ReadFile(vaultsPath())
	if err != nil {
		return vl
	}
	if err := json.Unmarshal(data, &vl); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v (using empty vault list)\n", vaultsPath(), err)
	}
	return vl
}

// SaveVaultList saves the vault list to ~/.config/granit/vaults.json.
func SaveVaultList(vl VaultList) {
	dir := ConfigDir()
	_ = os.MkdirAll(dir, 0700)

	data, err := json.MarshalIndent(vl, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(vaultsPath(), data, 0600)
}

// AddVault adds or updates a vault entry. It sets LastOpen to the current time
// and updates LastUsed to the given path.
func (vl *VaultList) AddVault(path string) {
	now := time.Now().Format("2006-01-02")
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// Derive a name from the directory basename.
	name := filepath.Base(absPath)

	// Check if vault already exists and update it.
	for i, v := range vl.Vaults {
		if v.Path == absPath {
			vl.Vaults[i].LastOpen = now
			vl.LastUsed = absPath
			return
		}
	}

	// Add new entry.
	vl.Vaults = append(vl.Vaults, VaultEntry{
		Path:     absPath,
		Name:     name,
		LastOpen: now,
	})
	vl.LastUsed = absPath
}

// RemoveVault removes a vault entry from the list by path.
func (vl *VaultList) RemoveVault(path string) {
	for i, v := range vl.Vaults {
		if v.Path == path {
			vl.Vaults = append(vl.Vaults[:i], vl.Vaults[i+1:]...)
			if vl.LastUsed == path {
				vl.LastUsed = ""
			}
			return
		}
	}
}

// GetLastUsed returns the path of the last opened vault.
func (vl *VaultList) GetLastUsed() string {
	return vl.LastUsed
}
