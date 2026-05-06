// Package history provides a per-note version history backed by
// snapshot files under <vault>/.granit/history/. Every time a note
// is saved through the API, the previous on-disk content is copied
// into a timestamped snapshot file BEFORE the new content overwrites
// it. The user's request was emphatic: "make sure there is file
// history as well for rollback and nothing is ever lost!!!" — so
// the design priorities here are durability and dedup, not space.
//
// Storage layout
//
// A note at vault-relative path "projects/foo/bar.md" snapshots into
//
//	<vault>/.granit/history/projects/foo/bar.md.versions/<ts>.md
//
// where <ts> is a filename-safe ISO-8601 stamp ("2026-05-06T12-34-56.789Z",
// colons replaced with dashes since Windows file systems disallow ':').
// The ".versions" suffix on the parent directory avoids any chance of
// colliding with a future note named "bar.md/something.md" — vault
// paths can't contain ".versions" since the vault loader rejects
// directories with that suffix at scan time (we don't actually need
// a check, since notes have a .md extension and ".versions" is a
// directory marker, but the suffix is still distinctive).
//
// Dedup
//
// A naive "snapshot every save" produces enormous history bloat
// because the editor autosaves on every keystroke pause. Snapshot()
// hashes the to-be-snapshotted content (the OLD body, before the
// new write lands) and skips if that hash matches the most-recent
// existing snapshot's hash. The first snapshot of a session always
// lands; subsequent identical saves are dropped silently.
//
// Atomicity
//
// Snapshots are written via atomicio.WriteNote — same crash-safe
// rename pattern as a regular note write. A crash mid-snapshot
// leaves either no new snapshot or a complete one, never a
// truncated file. The snapshot write happens BEFORE the new note
// write, so even if the snapshot itself fails, the original note
// is still on disk untouched.
package history

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// dirSuffix is appended to the note's filename to form the history
// directory name. Picked to be distinctive enough that no real note
// path collides with it. ".versions" reads naturally to a human
// browsing the .granit directory.
const dirSuffix = ".versions"

// historyRoot is the vault-relative root for all snapshot dirs.
const historyRoot = ".granit/history"

// MaxVersionsListed caps how many versions we return from List() so
// a note that's been edited 50,000 times doesn't blow up the JSON
// payload. The UI only ever shows ~100 entries at once and lazy-
// loads older ones; raising this is fine.
const MaxVersionsListed = 500

// Snapshot represents one historical version of a note.
type Snapshot struct {
	// Timestamp is the ISO-8601 stamp from the filename, restored
	// to canonical form (with colons) so clients can parse it
	// directly.
	Timestamp time.Time `json:"timestamp"`
	// Size is the byte length of the snapshotted content, surfaced
	// for the UI's "+/- N bytes" diff badge without forcing the
	// client to fetch the snapshot body just to compute size.
	Size int64 `json:"size"`
	// Hash is the first 16 hex chars of the SHA-256 of the content,
	// so the UI can show a short fingerprint and dedup-detect.
	Hash string `json:"hash"`
}

// Snap writes a snapshot of `oldContent` into the history directory
// for `relPath`. It dedupes against the most recent existing
// snapshot — if hashes match, no new file is written and (nil, nil)
// is returned. On a successful write the resulting Snapshot is
// returned for the caller to log / surface to the WS hub.
//
// vaultRoot must be an absolute filesystem path. relPath is the
// vault-relative path of the note (forward-slash separated).
//
// `oldContent` is the body BEFORE the impending overwrite. Pass
// nil to mean "the note didn't exist before this save" — Snap then
// returns (nil, nil) without writing anything (there's no prior
// version to preserve).
func Snap(vaultRoot, relPath string, oldContent []byte) (*Snapshot, error) {
	if vaultRoot == "" || relPath == "" {
		return nil, errors.New("history: empty vaultRoot or relPath")
	}
	if oldContent == nil {
		return nil, nil
	}
	dir, err := dirFor(vaultRoot, relPath)
	if err != nil {
		return nil, err
	}
	hash := hashOf(oldContent)
	if last, err := mostRecent(dir); err == nil && last != nil && last.Hash == hash {
		// Dedup hit — same content as the most recent snapshot,
		// no point writing a duplicate.
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("history: mkdir %s: %w", dir, err)
	}
	now := time.Now().UTC()
	stamp := stampForFilename(now)
	target := filepath.Join(dir, stamp+".md")
	if err := atomicio.WriteNote(target, string(oldContent)); err != nil {
		return nil, fmt.Errorf("history: write %s: %w", target, err)
	}
	return &Snapshot{
		Timestamp: now,
		Size:      int64(len(oldContent)),
		Hash:      hash,
	}, nil
}

// List returns the snapshots for `relPath` in descending timestamp
// order (newest first). At most MaxVersionsListed entries are
// returned. Missing history directory returns an empty slice, not
// an error — a never-edited note legitimately has no history.
func List(vaultRoot, relPath string) ([]Snapshot, error) {
	dir, err := dirFor(vaultRoot, relPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Snapshot{}, nil
		}
		return nil, err
	}
	var out []Snapshot
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		stamp := strings.TrimSuffix(e.Name(), ".md")
		t, ok := parseStamp(stamp)
		if !ok {
			continue
		}
		// Read just enough to compute size + hash. The UI never
		// renders the body in the list view — it asks for the body
		// only when the user clicks "Preview" on a specific entry.
		full := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		out = append(out, Snapshot{
			Timestamp: t,
			Size:      int64(len(data)),
			Hash:      hashOf(data),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	if len(out) > MaxVersionsListed {
		out = out[:MaxVersionsListed]
	}
	return out, nil
}

// Read returns the content of a specific snapshot, identified by
// the filename-safe timestamp string the API receives from the
// client (the same stamp the List() output contains, formatted as
// the canonical ISO-8601 — we accept either form here).
//
// Returns os.ErrNotExist when the version isn't in the directory.
func Read(vaultRoot, relPath, stamp string) ([]byte, error) {
	dir, err := dirFor(vaultRoot, relPath)
	if err != nil {
		return nil, err
	}
	// Accept both canonical-ISO ("2026-05-06T12:34:56.789Z") and
	// filename-safe form ("2026-05-06T12-34-56.789Z"). The on-disk
	// form is always the latter; canonicalise.
	fnStamp := canonicalToFilename(stamp)
	full := filepath.Join(dir, fnStamp+".md")
	return os.ReadFile(full)
}

// dirFor computes the absolute path to the history directory for
// a given vault-relative note path. Rejects paths that escape the
// vault (.., absolute paths) — same defense as the rest of the
// API surface.
func dirFor(vaultRoot, relPath string) (string, error) {
	if strings.Contains(relPath, "..") || strings.HasPrefix(relPath, "/") {
		return "", errors.New("history: invalid relPath")
	}
	clean := filepath.FromSlash(relPath)
	return filepath.Join(vaultRoot, historyRoot, clean+dirSuffix), nil
}

func mostRecent(dir string) (*Snapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var bestStamp string
	var bestT time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		stamp := strings.TrimSuffix(e.Name(), ".md")
		t, ok := parseStamp(stamp)
		if !ok {
			continue
		}
		if t.After(bestT) {
			bestT = t
			bestStamp = stamp
		}
	}
	if bestStamp == "" {
		return nil, nil
	}
	full := filepath.Join(dir, bestStamp+".md")
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		Timestamp: bestT,
		Size:      int64(len(data)),
		Hash:      hashOf(data),
	}, nil
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// stampForFilename converts a UTC time to a filename-safe ISO-8601
// stamp: "2026-05-06T12-34-56.789Z" — colons replaced with dashes
// because Windows / FAT / exFAT disallow ':' in filenames. Also
// truncates to millisecond precision; nanosecond-resolution stamps
// are visually noisy and rarely useful for human review.
func stampForFilename(t time.Time) string {
	canonical := t.UTC().Format("2006-01-02T15:04:05.000Z")
	return strings.ReplaceAll(canonical, ":", "-")
}

func canonicalToFilename(s string) string {
	// If already in filename form, no-op. Otherwise replace ':'.
	if !strings.Contains(s, ":") {
		return s
	}
	// Date prefix has no colons; only the time portion does. We can
	// just blanket-replace.
	return strings.ReplaceAll(s, ":", "-")
}

func parseStamp(s string) (time.Time, bool) {
	// On-disk stamps are filename-safe ("2026-05-06T12-34-56.789Z").
	// Convert back by replacing the time-portion dashes with colons.
	// The first three '-' are date separators ('2026-05-06') and
	// must stay; everything after the 'T' position-10 is time.
	if len(s) < 11 || s[10] != 'T' {
		return time.Time{}, false
	}
	canonical := s[:11] + strings.ReplaceAll(s[11:], "-", ":")
	t, err := time.Parse("2006-01-02T15:04:05.000Z", canonical)
	if err != nil {
		// Older format without millis? Try.
		t, err = time.Parse("2006-01-02T15:04:05Z", canonical)
		if err != nil {
			return time.Time{}, false
		}
	}
	return t, true
}
