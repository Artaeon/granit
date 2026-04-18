package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// ── Save / Load round-trip ──

func TestSearchIndex_SaveAndLoad_RoundTrips(t *testing.T) {
	src := NewSearchIndex()

	// Manually populate so we can compare an exact snapshot — calling
	// Build would couple the test to tokenizer behaviour.
	src.invertedIndex["hello"] = map[string]bool{"a.md": true, "b.md": true}
	src.invertedIndex["world"] = map[string]bool{"a.md": true}
	src.positions["hello"] = map[string][]int{"a.md": {0, 2}, "b.md": {1}}
	src.positions["world"] = map[string][]int{"a.md": {0}}
	src.docWordCount["a.md"] = 5
	src.docWordCount["b.md"] = 3
	src.docLines["a.md"] = []string{"hello world", "foo", "hello again"}
	src.docLines["b.md"] = []string{"foo", "hello"}
	src.totalDocs = 2
	src.ready = true

	path := filepath.Join(t.TempDir(), "idx.gob")
	if err := src.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dst := NewSearchIndex()
	if err := dst.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !dst.IsReady() {
		t.Error("loaded index should be ready")
	}
	if dst.totalDocs != 2 {
		t.Errorf("totalDocs = %d, want 2", dst.totalDocs)
	}
	if !dst.invertedIndex["hello"]["a.md"] {
		t.Errorf("inverted index lost mapping hello → a.md")
	}
	if got := dst.positions["hello"]["a.md"]; len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("positions lost: %+v", got)
	}
}

func TestSearchIndex_Load_MissingFileIsNoOp(t *testing.T) {
	si := NewSearchIndex()
	err := si.Load(filepath.Join(t.TempDir(), "nonexistent.gob"))
	if err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if si.IsReady() {
		t.Error("missing file should leave index un-ready")
	}
}

func TestSearchIndex_Load_CorruptFileDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.gob")
	if err := os.WriteFile(path, []byte("not a real gob"), 0o644); err != nil {
		t.Fatal(err)
	}
	si := NewSearchIndex()
	err := si.Load(path)
	if err == nil {
		t.Error("corrupt file should return an error")
	}
	if si.IsReady() {
		t.Error("corrupt file should not leave the index ready")
	}
}

func TestSearchIndex_Save_AtomicReplaceOfExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idx.gob")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	si := NewSearchIndex()
	si.invertedIndex["x"] = map[string]bool{"a.md": true}
	si.totalDocs = 1
	si.ready = true
	if err := si.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Sanity: file should now be a valid snapshot, not the stale bytes.
	dst := NewSearchIndex()
	if err := dst.Load(path); err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if !dst.invertedIndex["x"]["a.md"] {
		t.Error("Save did not overwrite the stale file with new data")
	}
}
