package repos

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// OpenFolder launches the system file manager / OS handler at `path`.
// Cross-platform — uses `xdg-open` on Linux/BSD, `open` on macOS,
// `explorer` on Windows. Returns an error when:
//
//   - path doesn't exist (caught early so we don't fork blindly),
//   - the OS-specific opener can't be found on PATH,
//   - the opener returns non-zero (handler missing, no DISPLAY, etc.).
//
// Silently fire-and-forgets — we don't wait for the GUI app to exit
// because that would block the TUI. The opener's own stderr is
// discarded; any user-visible failure surfaces via the returned error.
func OpenFolder(path string) error {
	if path == "" {
		return fmt.Errorf("open folder: empty path")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open folder: %w", err)
	}

	cmd, err := openCommand(path)
	if err != nil {
		return err
	}
	// Detach: the file manager should outlive granit's process,
	// and we don't want zombies.
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open folder: %w", err)
	}
	// Reap the child without blocking the TUI render thread.
	go func() { _ = cmd.Wait() }()
	return nil
}

// openCommand returns the platform-appropriate exec.Cmd for opening
// a path. Split out so the platform branching is small + testable.
func openCommand(path string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path), nil
	case "windows":
		// `explorer` is the user-facing handler; uses Win32
		// shell association. Pass path as-is — explorer handles
		// the spaces / unicode.
		return exec.Command("explorer", path), nil
	default:
		// Linux / *BSD. xdg-open is the freedesktop standard;
		// users without it (very rare) will get a clear error
		// rather than a silent miss.
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return nil, fmt.Errorf("open folder: xdg-open not found on PATH (install xdg-utils)")
		}
		return exec.Command("xdg-open", path), nil
	}
}
