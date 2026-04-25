// Package atomicio provides crash-safe file writes via the
// write-to-temp-and-rename pattern. The rename is atomic on POSIX
// filesystems, so a crash mid-write leaves either the old contents
// or the new contents on disk but never a truncated file.
//
// Two convenience wrappers cover the two perm modes granit uses:
//
//   - WriteNote — 0o644, for user-editable vault notes (markdown).
//   - WriteState — 0o600, for personal state under .granit/ that
//     shouldn't be world-readable on shared machines.
//
// New code should call these instead of os.WriteFile directly so
// the crash-safety property is uniform across the codebase.
package atomicio

import "os"

// WriteNote writes content to a vault note atomically with 0o644
// perms (world-readable, owner-writable — matches the user's
// expectation when they `cat` a note from another tool).
func WriteNote(path, content string) error {
	return WriteWithPerm(path, []byte(content), 0o644)
}

// WriteState writes data to an internal state file atomically with
// 0o600 perms (owner-only). Use for files under .granit/ that
// contain personal data (sessions, goals, projects, kanban state,
// task metadata, embeddings, etc.).
func WriteState(path string, data []byte) error {
	return WriteWithPerm(path, data, 0o600)
}

// WriteWithPerm is the underlying primitive. Writes data to
// path+".tmp" and renames over path. On any error the temp file is
// best-effort removed.
func WriteWithPerm(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
