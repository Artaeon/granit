// Package atomicio provides crash-safe file writes via the
// write-to-temp-and-rename pattern. The rename is atomic on POSIX
// filesystems, so a crash mid-write leaves either the old contents
// or the new contents on disk but never a truncated file.
//
// Beyond crash-safety the helpers also provide:
//
//   - O_EXCL on the temp file — concurrent writers fail loudly
//     instead of overwriting each other's temp files
//   - O_NOFOLLOW on the temp open — a malicious or stale symlink
//     at <path>.tmp.<...> can't redirect the write
//   - Per-call PID + nanosecond suffix on the temp name — no two
//     concurrent writers ever collide on the temp path
//   - Mode-preservation on overwrite — when path already exists,
//     the existing perm bits are reapplied after rename so
//     `chmod 600 secrets.md` doesn't get silently downgraded by
//     a checkbox toggle
//
// Two convenience wrappers cover the two perm modes granit uses
// for new files:
//
//   - WriteNote — 0o644, for user-editable vault notes (markdown)
//   - WriteState — 0o600, for personal state under .granit/ that
//     shouldn't be world-readable on shared machines
//
// New code should call these instead of os.WriteFile directly so
// the safety properties stay uniform across the codebase.
package atomicio

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

// tempCounter ensures the temp suffix is unique even within the
// same nanosecond on platforms with coarse clock resolution.
var tempCounter atomic.Uint64

// WriteNote writes content to a vault note atomically. New files
// get 0o644 (world-readable, owner-writable). Existing files keep
// their current mode bits — `chmod 600 secrets.md` survives a
// granit edit.
func WriteNote(path, content string) error {
	return WriteWithPerm(path, []byte(content), 0o644)
}

// WriteState writes data to an internal state file atomically.
// New files get 0o600 (owner-only). Use for .granit/* state.
// Existing files keep their current mode (including any
// hand-tightened to 0o400).
func WriteState(path string, data []byte) error {
	return WriteWithPerm(path, data, 0o600)
}

// WriteWithPerm is the underlying primitive. perm applies only
// when the destination doesn't already exist — overwriting an
// existing file preserves its current mode bits.
//
// Steps:
//  1. Stat the destination (if any) to capture existing perms
//  2. Open a uniquely-named temp file with O_EXCL|O_NOFOLLOW so
//     symlink races and concurrent writers can't redirect the IO
//  3. Write + close
//  4. If the destination existed, Chmod the temp to its old mode
//  5. Rename over the destination (atomic on POSIX same-fs)
//  6. On any error, best-effort remove the temp
func WriteWithPerm(path string, data []byte, perm os.FileMode) error {
	finalPerm := perm
	if info, err := os.Lstat(path); err == nil {
		// Existing file: keep its mode (covers hardening like
		// 0o400). Symlinks at the destination get rejected — we
		// don't want to write through a symlink to who-knows-where.
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("atomicio: refusing to write through symlink at %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("atomicio: refusing to write to non-regular file at %s", path)
		}
		finalPerm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("atomicio: stat %s: %w", path, err)
	}

	tmp := tempName(path)
	// O_EXCL: fail if tmp already exists (concurrent writer or
	// stale temp from a prior crash). O_NOFOLLOW: refuse to follow
	// a symlink masquerading as our temp.
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL | syscall.O_NOFOLLOW
	f, err := os.OpenFile(tmp, flags, finalPerm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Chmod after close in case the umask masked perm bits during
	// open. No-op when finalPerm matches what OpenFile produced.
	if err := os.Chmod(tmp, finalPerm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// tempName returns a path-adjacent temp name unique to this
// process, this nanosecond, and a per-call counter. Living next to
// the destination keeps the rename on the same filesystem so it
// stays atomic.
func tempName(path string) string {
	n := tempCounter.Add(1)
	return path + ".tmp." +
		strconv.Itoa(os.Getpid()) + "." +
		strconv.FormatInt(time.Now().UnixNano(), 36) + "." +
		strconv.FormatUint(n, 36)
}
