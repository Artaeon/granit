package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnap_NilOldContentSkips(t *testing.T) {
	dir := t.TempDir()
	got, err := Snap(dir, "foo.md", nil)
	if err != nil {
		t.Fatalf("nil oldContent should not error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil snapshot, got %+v", got)
	}
}

func TestSnap_FirstSnapshotWritesFile(t *testing.T) {
	dir := t.TempDir()
	snap, err := Snap(dir, "foo.md", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Size != 5 {
		t.Errorf("size: got %d want 5", snap.Size)
	}
	histDir := filepath.Join(dir, ".granit/history/foo.md.versions")
	entries, err := os.ReadDir(histDir)
	if err != nil {
		t.Fatalf("history dir not created: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 snapshot file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".md") {
		t.Errorf("expected .md suffix, got %s", entries[0].Name())
	}
}

func TestSnap_DedupsIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Snap(dir, "foo.md", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	// Sleep enough that a NEW timestamp would differ — proves the
	// skip is content-driven, not just same-clock.
	time.Sleep(10 * time.Millisecond)
	snap, err := Snap(dir, "foo.md", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if snap != nil {
		t.Errorf("expected dedup (nil snapshot), got %+v", snap)
	}
	histDir := filepath.Join(dir, ".granit/history/foo.md.versions")
	entries, _ := os.ReadDir(histDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after dedup, got %d", len(entries))
	}
}

func TestSnap_NewContentWritesNewSnapshot(t *testing.T) {
	dir := t.TempDir()
	if _, err := Snap(dir, "foo.md", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := Snap(dir, "foo.md", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	histDir := filepath.Join(dir, ".granit/history/foo.md.versions")
	entries, _ := os.ReadDir(histDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 snapshot entries, got %d", len(entries))
	}
}

func TestSnap_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	if _, err := Snap(dir, "../escape.md", []byte("x")); err == nil {
		t.Error("expected error on '..' path")
	}
	if _, err := Snap(dir, "/abs.md", []byte("x")); err == nil {
		t.Error("expected error on absolute path")
	}
}

func TestSnap_NestedPath(t *testing.T) {
	dir := t.TempDir()
	if _, err := Snap(dir, "projects/foo/bar.md", []byte("nested")); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(dir, ".granit/history/projects/foo/bar.md.versions")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("nested history dir not created: %v", err)
	}
}

func TestList_EmptyForUnknownPath(t *testing.T) {
	dir := t.TempDir()
	versions, err := List(dir, "no-such.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Errorf("expected empty, got %d", len(versions))
	}
}

func TestList_ReturnsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	// Three writes with deliberate gaps so timestamps differ at
	// millisecond granularity.
	_, _ = Snap(dir, "foo.md", []byte("a"))
	time.Sleep(2 * time.Millisecond)
	_, _ = Snap(dir, "foo.md", []byte("b"))
	time.Sleep(2 * time.Millisecond)
	_, _ = Snap(dir, "foo.md", []byte("c"))

	versions, err := List(dir, "foo.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	for i := 0; i < len(versions)-1; i++ {
		if versions[i].Timestamp.Before(versions[i+1].Timestamp) {
			t.Errorf("not sorted desc at index %d", i)
		}
	}
}

func TestRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := []byte("the original v1 content")
	if _, err := Snap(dir, "foo.md", original); err != nil {
		t.Fatal(err)
	}
	versions, _ := List(dir, "foo.md")
	if len(versions) != 1 {
		t.Fatalf("expected 1 version")
	}
	stamp := versions[0].Timestamp.UTC().Format("2006-01-02T15:04:05.000Z")
	body, err := Read(dir, "foo.md", stamp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != string(original) {
		t.Errorf("round trip: got %q want %q", body, original)
	}
}

func TestRead_AcceptsFilenameSafeForm(t *testing.T) {
	dir := t.TempDir()
	if _, err := Snap(dir, "foo.md", []byte("x")); err != nil {
		t.Fatal(err)
	}
	versions, _ := List(dir, "foo.md")
	stampSafe := stampForFilename(versions[0].Timestamp)
	body, err := Read(dir, "foo.md", stampSafe)
	if err != nil {
		t.Fatalf("filename-safe form should be accepted: %v", err)
	}
	if string(body) != "x" {
		t.Errorf("got %q want %q", body, "x")
	}
}

func TestRead_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Read(dir, "ghost.md", "2026-05-06T12:34:56.789Z")
	if !os.IsNotExist(err) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}
