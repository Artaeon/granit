package prayer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []Intention{{
		ID:         "01a",
		Text:       "Wisdom for the launch decision",
		Category:   "Self",
		Status:     string(StatusPraying),
		StartedAt:  "2026-05-01",
		Notes:      "Seeking clarity",
		CreatedAt:  now,
		UpdatedAt:  now,
	}}
	if err := SaveAll(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadAll(dir)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch:\n got=%+v\n want=%+v", got, in)
	}
}

func TestSaveEmptyProducesArray(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAll(dir, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".granit", "prayer", "intentions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Errorf("empty save = %q, want []", string(data))
	}
}

func TestSortForDisplay(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	in := []Intention{
		{ID: "p-old", Status: string(StatusPraying), UpdatedAt: t1},
		{ID: "ans-old", Status: string(StatusAnswered), AnsweredAt: "2026-01-01"},
		{ID: "p-new", Status: string(StatusPraying), UpdatedAt: t3},
		{ID: "arc", Status: string(StatusArchived), UpdatedAt: t2},
		{ID: "ans-new", Status: string(StatusAnswered), AnsweredAt: "2026-03-01"},
	}
	out := SortForDisplay(in)
	want := []string{"p-new", "p-old", "ans-new", "ans-old", "arc"}
	for i, x := range out {
		if x.ID != want[i] {
			t.Errorf("sort[%d]=%q want %q", i, x.ID, want[i])
		}
	}
}

func TestNormalizeStatus(t *testing.T) {
	if NormalizeStatus("") != string(StatusPraying) {
		t.Error("empty should default to praying")
	}
	if NormalizeStatus("typo") != string(StatusPraying) {
		t.Error("unknown should default to praying")
	}
	if NormalizeStatus("answered") != string(StatusAnswered) {
		t.Error("answered should round-trip")
	}
}
