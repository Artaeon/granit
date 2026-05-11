package aimemory

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestAdd_HappyPath(t *testing.T) {
	dir := t.TempDir()
	f, err := Add(dir, "User is vegetarian", []string{"diet"})
	if err != nil {
		t.Fatal(err)
	}
	if f.ID == "" || f.CreatedAt == "" || f.UpdatedAt == "" {
		t.Errorf("expected id + timestamps populated: %+v", f)
	}
	if f.Content != "User is vegetarian" {
		t.Errorf("content round-trip failed: %q", f.Content)
	}
	if len(f.Tags) != 1 || f.Tags[0] != "diet" {
		t.Errorf("tags = %v, want [diet]", f.Tags)
	}
}

func TestAdd_RejectsEmptyContent(t *testing.T) {
	dir := t.TempDir()
	cases := []string{"", "   ", "\t\n"}
	for _, c := range cases {
		if _, err := Add(dir, c, nil); err == nil {
			t.Errorf("expected error for empty content %q", c)
		}
	}
}

func TestAdd_RejectsOversizedContent(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("x", MaxContentLen+1)
	if _, err := Add(dir, long, nil); err == nil {
		t.Errorf("expected error for content over %d chars", MaxContentLen)
	}
}

func TestAdd_DedupesOnByteEqualContent(t *testing.T) {
	// Real scenario: the assistant proposes a "remember-this" action
	// chip; the user clicks it, then later clicks again from a regen.
	// Both clicks fire Add with byte-identical content. The store
	// should return the existing fact instead of duplicating.
	dir := t.TempDir()
	a, _ := Add(dir, "User's wife is Anna", nil)
	b, err := Add(dir, "User's wife is Anna", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Errorf("duplicate Add must return existing fact (a=%s b=%s)", a.ID, b.ID)
	}
	snap, _ := Snapshot(dir)
	if len(snap) != 1 {
		t.Errorf("expected 1 fact after dedupe, got %d", len(snap))
	}
}

func TestAdd_NormalizesTags(t *testing.T) {
	dir := t.TempDir()
	// Whitespace + case + duplicates should all collapse.
	f, _ := Add(dir, "x", []string{"  Family ", "FAMILY", "", "health"})
	if len(f.Tags) != 2 {
		t.Fatalf("expected 2 tags after normalize, got %v", f.Tags)
	}
	if f.Tags[0] != "family" || f.Tags[1] != "health" {
		t.Errorf("tag normalize order/case wrong: %v", f.Tags)
	}
}

func TestAdd_EnforcesMaxFacts(t *testing.T) {
	// Stress the cap so a future regression that removed it would
	// silently let the store grow unbounded. We don't add MaxFacts+1
	// real entries (too slow); instead, plant a store right at the
	// limit and assert Add refuses.
	dir := t.TempDir()
	s := Store{Version: 1}
	for i := 0; i < MaxFacts; i++ {
		s.Facts = append(s.Facts, Fact{
			ID:        fmt.Sprintf("seed-%d", i),
			Content:   fmt.Sprintf("seed fact %d", i),
			CreatedAt: "2026-01-01T00:00:00Z",
		})
	}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(dir, "overflow", nil); err == nil {
		t.Errorf("expected store-full error at %d facts", MaxFacts)
	}
}

func TestPatch_UpdatesContentAndTags(t *testing.T) {
	dir := t.TempDir()
	a, _ := Add(dir, "initial", []string{"a"})
	patched, err := Patch(dir, a.ID, "updated", []string{"b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Content != "updated" {
		t.Errorf("content didn't update: %+v", patched)
	}
	if len(patched.Tags) != 2 || patched.Tags[0] != "b" || patched.Tags[1] != "c" {
		t.Errorf("tags didn't update: %v", patched.Tags)
	}
	if patched.UpdatedAt == a.UpdatedAt {
		t.Errorf("UpdatedAt must advance on patch")
	}
}

func TestPatch_UnknownID(t *testing.T) {
	dir := t.TempDir()
	if _, err := Patch(dir, "missing", "x", nil); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete_Idempotent(t *testing.T) {
	dir := t.TempDir()
	a, _ := Add(dir, "x", nil)
	if err := Delete(dir, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := Delete(dir, a.ID); err != nil {
		t.Errorf("second Delete should be no-op, got %v", err)
	}
	if err := Delete(dir, "never-existed"); err != nil {
		t.Errorf("delete of unknown id should be no-op, got %v", err)
	}
}

func TestSnapshot_ReturnsNilOnEmpty(t *testing.T) {
	dir := t.TempDir()
	snap, err := Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on fresh vault, got %d facts", len(snap))
	}
}

func TestConcurrentAddsAllPersist(t *testing.T) {
	// Same load-modify-write race pattern as annotations + books
	// sidecars. The action-chip flow can fire several Add calls
	// from a single thread re-render; without the mutex, the second
	// writer's pre-modify read misses the first writer's commit and
	// loses an entry. 30 parallel adds covers the worst real-world
	// burst with healthy headroom.
	dir := t.TempDir()
	const N = 30
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			if _, err := Add(dir, fmt.Sprintf("fact-%d", idx), nil); err != nil {
				t.Errorf("concurrent Add %d failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
	snap, _ := Snapshot(dir)
	if len(snap) != N {
		t.Errorf("expected %d facts after concurrent adds; got %d (lost writes)", N, len(snap))
	}
}
