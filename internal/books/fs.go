package books

import (
	"os"

	"github.com/artaeon/granit/internal/atomicio"
)

// fs.go — small filesystem wrappers used by the discovery import
// path. They live here rather than reaching into atomicio directly
// from discover.go so the wider package can pivot to a different
// IO strategy (e.g. streaming write, content-hash dedup) without
// touching the discovery code.

func mkdirAll(p string) error { return os.MkdirAll(p, 0o755) }

// writeFileAtomic is a thin wrapper over atomicio.WriteWithPerm.
// EPUBs are user-facing content, so 0o644 (matching note files,
// not 0o600 sidecar state) lets the user open them in Calibre /
// Kindle desktop / their file manager without permission shuffling.
func writeFileAtomic(p string, data []byte) error {
	return atomicio.WriteWithPerm(p, data, 0o644)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func removeFile(p string) { _ = os.Remove(p) }
