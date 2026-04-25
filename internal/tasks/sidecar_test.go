package tasks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempVault(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func sampleSidecar() sidecarFile {
	now := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	return sidecarFile{
		Schema:    sidecarSchemaVersion,
		UpdatedAt: now,
		Tasks: []sidecarTask{
			{
				ID:          "01HX0000000000000000000001",
				Fingerprint: "abc123",
				Anchor:      sidecarAnchor{File: "Tasks.md", Line: 3, Indent: 0},
				NormText:    "ship phase 2",
				Triage:      TriageScheduled,
				Origin:      OriginManual,
				CreatedAt:   now,
			},
		},
		Tombstones: []sidecarTombstone{
			{ID: "01HX0000000000000000000099", Fingerprint: "dead", RemovedAt: now},
		},
	}
}

func TestSidecar_RoundTrip(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)

	want := sampleSidecar()
	if err := saveSidecar(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, result := loadSidecar(path)
	if result != LoadOK {
		t.Fatalf("load result: got %v want %v", result, LoadOK)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != want.Tasks[0].ID {
		t.Errorf("tasks: got %+v want %+v", got.Tasks, want.Tasks)
	}
	if got.Tasks[0].Triage != TriageScheduled {
		t.Errorf("triage: got %q want %q", got.Tasks[0].Triage, TriageScheduled)
	}
	if len(got.Tombstones) != 1 || got.Tombstones[0].ID != want.Tombstones[0].ID {
		t.Errorf("tombstones: got %+v want %+v", got.Tombstones, want.Tombstones)
	}
}

func TestSidecar_SaveCreatesGranitDir(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)

	if err := saveSidecar(path, sampleSidecar()); err != nil {
		t.Fatalf("save: %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("granit dir missing: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("expected directory")
	}
}

func TestSidecar_StampsSchemaAndUpdatedAt(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)

	// Caller passes an unstamped value — saveSidecar should fill them in.
	zero := sidecarFile{Tasks: []sidecarTask{{ID: "x", Fingerprint: "y", Anchor: sidecarAnchor{File: "a", Line: 1}}}}
	if err := saveSidecar(path, zero); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var got sidecarFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != sidecarSchemaVersion {
		t.Errorf("schema not stamped: got %d want %d", got.Schema, sidecarSchemaVersion)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updated_at not stamped")
	}
}

func TestSidecar_LoadMissingFileReportsMissing(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)
	got, result := loadSidecar(path)
	if result != LoadMissing {
		t.Errorf("got %v want %v", result, LoadMissing)
	}
	if got.Schema != sidecarSchemaVersion {
		t.Errorf("missing should return zero-value with current schema, got %d", got.Schema)
	}
}

func TestSidecar_CorruptFileBackedUpAndReportsCorrupt(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, result := loadSidecar(path)
	if result != LoadCorrupt {
		t.Errorf("got %v want %v", result, LoadCorrupt)
	}

	// Original should have been moved to .v1.bak
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original still exists: %v", err)
	}
	if _, err := os.Stat(path + ".v1.bak"); err != nil {
		t.Errorf("backup missing: %v", err)
	}
}

func TestSidecar_FutureSchemaBackedUpAndReportsFutureSchema(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	future := sidecarFile{Schema: sidecarSchemaVersion + 99, UpdatedAt: time.Now()}
	data, _ := json.Marshal(future)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, result := loadSidecar(path)
	if result != LoadFutureSchema {
		t.Errorf("got %v want %v", result, LoadFutureSchema)
	}
	if _, err := os.Stat(path + ".v1.bak"); err != nil {
		t.Errorf("backup missing: %v", err)
	}
}

func TestSidecar_BackupsIncrementWhenPrior(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Pre-existing v1 backup
	if err := os.WriteFile(path+".v1.bak", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	loadSidecar(path)
	if _, err := os.Stat(path + ".v2.bak"); err != nil {
		t.Errorf("expected .v2.bak to be created when .v1.bak exists: %v", err)
	}
}

func TestSidecar_PreVersionedFileTreatedAsCurrent(t *testing.T) {
	vault := tempVault(t)
	path := SidecarPath(vault)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Schema field omitted — older internal preview build.
	preVer := struct {
		Tasks []sidecarTask `json:"tasks"`
	}{
		Tasks: []sidecarTask{{ID: "x", Fingerprint: "y", Anchor: sidecarAnchor{File: "a", Line: 1}}},
	}
	data, _ := json.Marshal(preVer)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, result := loadSidecar(path)
	if result != LoadOK {
		t.Errorf("expected LoadOK for pre-versioned, got %v", result)
	}
	if got.Schema != sidecarSchemaVersion {
		t.Errorf("schema should be normalized to current: got %d", got.Schema)
	}
	if len(got.Tasks) != 1 {
		t.Errorf("tasks lost: got %v", got.Tasks)
	}
}

func TestPruneTombstones_DropsExpired(t *testing.T) {
	now := time.Now()
	tomb := []sidecarTombstone{
		{ID: "a", RemovedAt: now.Add(-1 * time.Hour)},               // fresh
		{ID: "b", RemovedAt: now.Add(-tombstoneTTL - time.Hour)},    // expired
		{ID: "c", RemovedAt: now.Add(-tombstoneTTL / 2)},            // halfway
		{ID: "d", RemovedAt: now.Add(-tombstoneTTL - 24*time.Hour)}, // expired
	}
	got := pruneTombstones(tomb, now)
	if len(got) != 2 {
		t.Fatalf("len: got %d want 2 (%+v)", len(got), got)
	}
	keepIDs := got[0].ID + "," + got[1].ID
	if !strings.Contains(keepIDs, "a") || !strings.Contains(keepIDs, "c") {
		t.Errorf("wrong survivors: %v", got)
	}
}

func TestPruneTombstones_EmptyIsNoOp(t *testing.T) {
	got := pruneTombstones(nil, time.Now())
	if len(got) != 0 {
		t.Errorf("nil input should stay empty, got %v", got)
	}
}

func TestNewID_ProducesUniqueULIDs(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewID()
		if len(id) != 26 {
			t.Fatalf("ULID length: got %d want 26 (id=%q)", len(id), id)
		}
		if seen[id] {
			t.Fatalf("collision at iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewID_TimeSortable(t *testing.T) {
	a := NewID()
	time.Sleep(2 * time.Millisecond)
	b := NewID()
	if a >= b {
		t.Errorf("ULIDs should sort by time: a=%q >= b=%q", a, b)
	}
}
