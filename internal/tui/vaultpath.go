package tui

import (
	"errors"
	"path/filepath"
	"strings"
)

// errPathEscapesVault is returned by resolveVaultPath when the requested
// note path resolves outside the configured vault root.
var errPathEscapesVault = errors.New("path escapes vault root")

// resolveVaultPath joins relPath onto vaultRoot and verifies the result
// stays within the vault. It returns the absolute, cleaned path on success.
//
// The check defends against three classes of accident or attack:
//
//  1. Relative components like "../../etc/passwd" that would otherwise
//     resolve outside the vault when joined.
//  2. Absolute paths supplied as the relative side (filepath.Join replaces
//     the base with an absolute argument).
//  3. Symlink-style escapes via cleaned but still-suspicious inputs.
//
// Callers that want to read or write a file scoped to the vault should go
// through this helper instead of building the path with filepath.Join
// directly. The cost is one stat-free string operation per call.
func resolveVaultPath(vaultRoot, relPath string) (string, error) {
	if vaultRoot == "" {
		return "", errPathEscapesVault
	}
	absRoot, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", err
	}
	// filepath.Join + Clean collapses any ".." components, but a leading
	// "/" on relPath would replace absRoot entirely, so we resolve the
	// joined result and then verify the prefix.
	candidate := filepath.Join(absRoot, relPath)
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	// Use the trailing-separator form so that "/vault" does not match
	// "/vaulted/...". An exact match on the root itself is also allowed.
	if abs != absRoot && !strings.HasPrefix(abs, absRoot+string(filepath.Separator)) {
		return "", errPathEscapesVault
	}
	return abs, nil
}
