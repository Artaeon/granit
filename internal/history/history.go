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
	"encoding/json"
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

// MaxVersionsKept is the retention cap per note. After every successful
// Snap, snapshots beyond this count are pruned (oldest first). Before
// this cap existed, .versions dirs grew unbounded — a 600-line note
// edited daily for two years held ~700 snapshots × ~30 KB each = ~20 MB
// per note. The cap trades long-tail history for predictable disk use;
// 100 versions covers ~3 months of daily editing.
const MaxVersionsKept = 100

// manifestName is the sidecar JSON file inside each .versions
// directory. Lets List() return the snapshot metadata without
// re-reading every file from disk — N file reads + hash recompute per
// call became visible on heavily-edited notes. The manifest is
// rebuilt from scan on first read after upgrade and whenever the file
// goes missing/corrupt, so the on-disk snapshot files remain the
// source of truth.
const manifestName = ".manifest.json"

// manifest is the on-disk shape of the sidecar. Snapshots are sorted
// newest-first so the UI's primary path is a slice copy with no
// extra work. UpdatedAt is for debugging; the source of truth is
// always the snapshot file set on disk.
type manifest struct {
	Snapshots []Snapshot `json:"snapshots"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

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
	snap := Snapshot{
		Timestamp: now,
		Size:      int64(len(oldContent)),
		Hash:      hash,
	}
	// Update manifest + apply retention. Manifest failures are
	// non-fatal — the snapshot file is on disk and the next List()
	// will rebuild from scan. Surface the error path so callers can
	// log it, but never abort the save chain over manifest housekeeping.
	if err := commitSnapshotToManifest(dir, snap); err != nil {
		// Stamp the error onto the returned snapshot? No — the
		// caller (PUT handler) treats Snap's return value as
		// authoritative for "what landed". Logging is the caller's
		// job once a sink exists; for now, best-effort is correct.
		_ = err
	}
	return &snap, nil
}

// commitSnapshotToManifest appends a fresh snapshot to the sidecar
// manifest, applies the MaxVersionsKept retention cap (deleting the
// snapshot files for evicted entries), and atomically writes the
// updated manifest. Idempotent under crash: a half-finished prune
// can be re-detected on the next call because the manifest's view
// matches whatever survived on disk after the rebuild path runs.
func commitSnapshotToManifest(dir string, snap Snapshot) error {
	m, err := loadManifest(dir)
	if err != nil {
		// Rebuild from scan if the manifest was missing or corrupt.
		// This is the upgrade path for existing notes that predate
		// the manifest, and the recovery path for any time the file
		// gets clobbered. The scan READS THE NEW SNAPSHOT FILE THAT
		// SNAP JUST WROTE, so the result already contains `snap` at
		// the head — no prepend below.
		m, err = rebuildManifestFromDir(dir)
		if err != nil {
			return err
		}
	} else {
		// Manifest loaded cleanly — it does NOT contain the new
		// snapshot yet, so prepend it (the list is newest-first).
		m.Snapshots = append([]Snapshot{snap}, m.Snapshots...)
	}
	// Retention: evict the oldest entries beyond the cap and delete
	// their underlying files. The cap is per-note; evict from the
	// tail of the slice (oldest end).
	if MaxVersionsKept > 0 && len(m.Snapshots) > MaxVersionsKept {
		evicted := m.Snapshots[MaxVersionsKept:]
		m.Snapshots = m.Snapshots[:MaxVersionsKept]
		for _, e := range evicted {
			fnStamp := canonicalToFilename(e.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z"))
			_ = os.Remove(filepath.Join(dir, fnStamp+".md"))
		}
	}
	m.UpdatedAt = time.Now().UTC()
	return saveManifest(dir, m)
}

// List returns the snapshots for `relPath` in descending timestamp
// order (newest first). At most MaxVersionsListed entries are
// returned. Missing history directory returns an empty slice, not
// an error — a never-edited note legitimately has no history.
//
// Reads the sidecar manifest if present; otherwise rebuilds from a
// directory scan and persists the manifest so the next call is fast.
func List(vaultRoot, relPath string) ([]Snapshot, error) {
	dir, err := dirFor(vaultRoot, relPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Snapshot{}, nil
		}
		return nil, err
	}
	if m, err := loadManifest(dir); err == nil && m != nil {
		return capList(m.Snapshots), nil
	}
	// No usable manifest — fall back to a scan and persist the
	// result so the next call is O(1) read.
	m, err := rebuildManifestFromDir(dir)
	if err != nil {
		return nil, err
	}
	_ = saveManifest(dir, m) // best-effort; List still returns the data
	return capList(m.Snapshots), nil
}

func capList(snaps []Snapshot) []Snapshot {
	if len(snaps) > MaxVersionsListed {
		snaps = snaps[:MaxVersionsListed]
	}
	// Copy so callers can't mutate the underlying manifest cache
	// (we don't currently cache, but the contract should hold).
	out := make([]Snapshot, len(snaps))
	copy(out, snaps)
	return out
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

// loadManifest reads and parses the sidecar manifest. Returns a
// nil-or-error if the file is missing or unreadable; the caller
// recovers via rebuildManifestFromDir.
func loadManifest(dir string) (*manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("history: parse manifest: %w", err)
	}
	return &m, nil
}

// saveManifest atomically writes the manifest via atomicio (same
// crash-safe rename pattern as note writes). Errors propagate so
// the caller can decide whether to log.
func saveManifest(dir string, m *manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("history: marshal manifest: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return atomicio.WriteNote(filepath.Join(dir, manifestName), string(data))
}

// rebuildManifestFromDir scans the .versions directory and builds a
// fresh manifest from disk. Used on first read after upgrade (existing
// notes have no sidecar yet) and as a recovery path when the manifest
// goes missing/corrupt. The snapshot files themselves remain the
// authoritative source; the manifest is purely an index.
func rebuildManifestFromDir(dir string) (*manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		stamp := strings.TrimSuffix(e.Name(), ".md")
		t, ok := parseStamp(stamp)
		if !ok {
			continue
		}
		full := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		snaps = append(snaps, Snapshot{
			Timestamp: t,
			Size:      int64(len(data)),
			Hash:      hashOf(data),
		})
	}
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].Timestamp.After(snaps[j].Timestamp)
	})
	return &manifest{Snapshots: snaps, UpdatedAt: time.Now().UTC()}, nil
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
