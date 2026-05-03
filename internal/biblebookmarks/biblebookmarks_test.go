package biblebookmarks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// roundTrip ensures every field a writer puts in survives a SaveAll
// → LoadAll cycle, including the Note field that was added late.
// Catches "I forgot a json tag" regressions before the web hits them.
func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	in := []Bookmark{{
		ID:        "01abc",
		BookCode:  "JHN",
		Book:      "John",
		Chapter:   3,
		VerseFrom: 16,
		VerseTo:   17,
		Reference: "John 3:16-17",
		Text:      "For God so loved the world…",
		Note:      "favorite",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	if err := SaveAll(dir, in); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	got := LoadAll(dir)
	if len(got) != 1 {
		t.Fatalf("len got=%d want 1", len(got))
	}
	if !reflect.DeepEqual(got[0], in[0]) {
		t.Errorf("round-trip mismatch:\n got = %+v\n want = %+v", got[0], in[0])
	}
}

// SaveAll for a missing-then-empty case must produce `[]`, not
// `null`, so the web's JSON parser unwraps to an empty array. A
// regression here would reintroduce the null-on-empty pitfall the
// deadlines / goals files already paper over.
func TestSaveEmptyProducesArray(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAll(dir, nil); err != nil {
		t.Fatalf("SaveAll nil: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".granit", "bible-bookmarks.json"))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if got := string(data); got != "[]" {
		t.Errorf("empty save = %q, want %q", got, "[]")
	}
}

func TestSortNewestFirst(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	in := []Bookmark{
		{ID: "old", CreatedAt: t1},
		{ID: "newest", CreatedAt: t3},
		{ID: "mid", CreatedAt: t2},
	}
	out := SortNewestFirst(in)
	want := []string{"newest", "mid", "old"}
	for i, b := range out {
		if b.ID != want[i] {
			t.Errorf("sort[%d]=%q want %q", i, b.ID, want[i])
		}
	}
}

