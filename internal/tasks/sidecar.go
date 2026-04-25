package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// sidecarSchemaVersion is bumped only when the on-disk format
// changes in a non-additive way. Loaders for older versions stay in
// the codebase indefinitely; loaders for newer-than-known versions
// back up the file and trigger first-ingestion.
const sidecarSchemaVersion = 1

// sidecarFile is the JSON shape of .granit/tasks-meta.json. Public
// to the package only — callers go through Load/Save.
type sidecarFile struct {
	Schema    int                `json:"schema"`
	UpdatedAt time.Time          `json:"updated_at"`
	Tasks     []sidecarTask      `json:"tasks"`
	Tombstones []sidecarTombstone `json:"tombstones,omitempty"`
}

// sidecarTask is the per-task metadata persisted to disk. Every
// field that can't be reconstructed from markdown lives here.
//
// Anchor records where the task was last seen so reconciliation can
// walk the file and confirm or update the location. Fingerprint is
// the FNV hash of the normalized text — see fingerprint.go.
type sidecarTask struct {
	ID             string         `json:"id"`
	Fingerprint    string         `json:"fingerprint"`
	Anchor         sidecarAnchor  `json:"anchor"`
	NormText       string         `json:"norm_text"`
	Triage         TriageState    `json:"triage,omitempty"`
	ScheduledStart *time.Time     `json:"scheduled_start,omitempty"`
	DurationMinutes int            `json:"duration_minutes,omitempty"`
	ProjectID      string         `json:"project_id,omitempty"`
	GoalID         string         `json:"goal_id,omitempty"`
	Origin         Origin         `json:"origin,omitempty"`
	CreatedAt      time.Time      `json:"created_at,omitempty"`
	LastTriagedAt  *time.Time     `json:"last_triaged_at,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	Notes          string         `json:"notes,omitempty"`
}

type sidecarAnchor struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Indent int    `json:"indent,omitempty"`
}

// sidecarTombstone keeps deleted IDs around for ~30 days so a
// `git pull` that re-introduces a task line that was deleted
// locally can revive the original ID instead of minting a new one
// (which would lose triage/schedule state).
type sidecarTombstone struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint"`
	NormText    string    `json:"norm_text,omitempty"`
	RemovedAt   time.Time `json:"removed_at"`
}

const tombstoneTTL = 30 * 24 * time.Hour

// SidecarPath returns the canonical path for the sidecar inside a
// vault. Exposed so tests and migration tools can reference it
// without duplicating the path literal.
func SidecarPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "tasks-meta.json")
}

// LoadResult reports what happened when loading a sidecar. The
// store uses this to decide whether to run first-ingestion.
type LoadResult int

const (
	LoadOK            LoadResult = iota // sidecar parsed cleanly
	LoadMissing                         // file does not exist → first-ingestion
	LoadCorrupt                         // file exists but unparseable → backed up + first-ingestion
	LoadFutureSchema                    // schema > known → backed up + first-ingestion
	LoadIOError                         // EACCES / EIO / ENOTDIR — surface to user, do not destroy
)

// loadSidecar reads and parses the sidecar at the given path. On
// any non-OK result the file is backed up to <path>.v{n}.bak (when
// it existed at all) so the user can recover by hand if first
// ingestion picks the wrong identities.
//
// Permission/IO errors propagate as LoadIOError (caller decides
// whether to refuse launch or fall through to first-ingestion).
// Parse errors and future-schema versions are backed up and the
// caller falls back to first-ingestion. Tasks with paths that
// escape the vault are silently dropped — they cannot be reached
// by any code path anyway, and a hand-edited or git-merged sidecar
// shouldn't crash the launch.
func loadSidecar(path, vaultRoot string) (sidecarFile, LoadResult) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sidecarFile{Schema: sidecarSchemaVersion}, LoadMissing
		}
		// Real I/O error (EACCES, EIO, ENOTDIR, etc.) — distinct
		// from "the file is corrupt." Don't back up the file (it
		// may be perfectly valid behind a transient mount issue);
		// caller surfaces this and decides recovery policy.
		return sidecarFile{Schema: sidecarSchemaVersion}, LoadIOError
	}
	var s sidecarFile
	if err := json.Unmarshal(data, &s); err != nil {
		_ = backupSidecar(path)
		return sidecarFile{Schema: sidecarSchemaVersion}, LoadCorrupt
	}
	if s.Schema > sidecarSchemaVersion {
		_ = backupSidecar(path)
		return sidecarFile{Schema: sidecarSchemaVersion}, LoadFutureSchema
	}
	if s.Schema == 0 {
		// Pre-versioned file from an internal preview build.
		// Treat as v1 — the field set is identical so unmarshaling
		// already succeeded.
		s.Schema = sidecarSchemaVersion
	}
	// Drop tasks whose anchor escapes the vault — could be a
	// hand-edited file or a git merge from a malicious branch.
	// Keeping them would let any subsequent UpdateLine/Delete on
	// that ID write outside the vault.
	if vaultRoot != "" {
		filtered := s.Tasks[:0]
		for _, t := range s.Tasks {
			if _, err := resolveInVault(vaultRoot, t.Anchor.File); err == nil {
				filtered = append(filtered, t)
			}
		}
		s.Tasks = filtered
	}
	return s, LoadOK
}

// saveSidecar writes the sidecar atomically. Prunes tombstones
// older than tombstoneTTL before write. Always stamps UpdatedAt
// and Schema so the file is self-describing.
func saveSidecar(path string, s sidecarFile) error {
	s.Schema = sidecarSchemaVersion
	s.UpdatedAt = time.Now().UTC()
	s.Tombstones = pruneTombstones(s.Tombstones, time.Now())
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}

// pruneTombstones drops entries older than tombstoneTTL. Exposed
// at package level for tests; called from saveSidecar.
func pruneTombstones(tomb []sidecarTombstone, now time.Time) []sidecarTombstone {
	if len(tomb) == 0 {
		return tomb
	}
	cutoff := now.Add(-tombstoneTTL)
	out := tomb[:0]
	for _, t := range tomb {
		if t.RemovedAt.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// backupSidecar moves a corrupt or future-schema file aside so the
// user can recover by hand. Best-effort: if the rename fails
// (perms, disk full), we proceed anyway with first-ingestion
// rather than refusing to launch.
func backupSidecar(path string) error {
	for n := 1; n < 100; n++ {
		dst := fmt.Sprintf("%s.v%d.bak", path, n)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			return os.Rename(path, dst)
		}
	}
	// Pathological case: 100 backups already exist. Overwrite the
	// last one rather than refusing to back up.
	return os.Rename(path, path+".v99.bak")
}
