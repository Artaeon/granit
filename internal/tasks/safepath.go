package tasks

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrEscapesVault is returned when a relative path would resolve
// outside the vault root after cleaning. Caller should treat this
// as a fatal validation error — never silently fall back to writing
// outside the vault.
var ErrEscapesVault = errors.New("tasks: path escapes vault root")

// resolveInVault joins rel onto root and asserts that the result is
// still under root. Rejects:
//
//   - absolute paths (rel starts with "/")
//   - any "../" component, even ones that would cancel out — the
//     paranoid form is easier to reason about than "compute the net
//     effect, allow if zero"
//   - paths that contain a filesystem-separator-prefixed escape after
//     filepath.Clean (the canonical containment check)
//
// All three checks are necessary: rejecting "../" alone misses a
// hand-crafted absolute path; the prefix check after Clean misses
// "..\\.." on Windows; rejecting absolute paths alone misses
// "harmless/../../escape".
//
// Inputs come from sidecar JSON (which can be hand-edited or
// git-merged from any branch) and from CreateOpts.File (which a
// future Lua plugin or AI-capture path could control). Both are
// untrusted by design — every Join in the tasks package goes
// through here.
func resolveInVault(root, rel string) (string, error) {
	if rel == "" {
		return "", ErrEscapesVault
	}
	if filepath.IsAbs(rel) {
		return "", ErrEscapesVault
	}
	// Walk the cleaned path components looking for any "..".
	// filepath.Clean collapses "a/b/../c" to "a/c" so we look at the
	// raw input split on both separators (Windows + Unix).
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return "", ErrEscapesVault
		}
	}
	abs := filepath.Clean(filepath.Join(root, rel))
	rootClean := filepath.Clean(root)
	// Containment: abs must equal rootClean OR start with rootClean
	// followed by a separator. The second clause catches root-prefix
	// neighbours like "/vault" vs "/vault-attacker".
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		return "", ErrEscapesVault
	}
	return abs, nil
}
