package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/artaeon/granit/internal/objects"
)

func TestDashboard_SetTypedObjects_PopulatesCounts(t *testing.T) {
	tmp := t.TempDir()
	mustWrite := func(p string) {
		full := filepath.Join(tmp, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("# x"), 0o644)
	}
	mustWrite("People/Alice.md")
	mustWrite("People/Bob.md")
	mustWrite("Books/A.md")

	reg := objects.NewRegistryEmpty()
	_ = reg.Set(objects.Type{ID: "person", Name: "Person", Icon: "👤"})
	_ = reg.Set(objects.Type{ID: "book", Name: "Book", Icon: "📖"})
	bld := objects.NewBuilder(reg)
	bld.Add("People/Alice.md", "Alice", map[string]string{"type": "person"})
	bld.Add("People/Bob.md", "Bob", map[string]string{"type": "person"})
	bld.Add("Books/A.md", "Atomic", map[string]string{"type": "book"})
	idx := bld.Finalize()

	d := &Dashboard{}
	d.SetTypedObjects(reg, idx, nil, "", tmp)

	if d.objTotal != 3 {
		t.Errorf("expected total=3, got %d", d.objTotal)
	}
	// Sorted by count desc — person (2) before book (1).
	if len(d.objCountsByType) != 2 {
		t.Fatalf("expected 2 type rows, got %d", len(d.objCountsByType))
	}
	if d.objCountsByType[0].ID != "person" || d.objCountsByType[0].Count != 2 {
		t.Errorf("first row should be person count=2, got %+v", d.objCountsByType[0])
	}
	if len(d.objRecent) != 3 {
		t.Errorf("expected 3 recent, got %d", len(d.objRecent))
	}
}

func TestDashboard_SetTypedObjects_PrimaryView(t *testing.T) {
	tmp := t.TempDir()
	full := filepath.Join(tmp, "Articles", "X.md")
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	_ = os.WriteFile(full, []byte("# x"), 0o644)

	reg := objects.NewRegistryEmpty()
	_ = reg.Set(objects.Type{ID: "article", Name: "Article"})
	bld := objects.NewBuilder(reg)
	bld.Add("Articles/X.md", "X", map[string]string{"type": "article", "status": "to-read"})
	idx := bld.Finalize()

	cat := objects.NewViewCatalog([]objects.View{
		{ID: "articles-to-read", Name: "Articles to Read", Description: "...",
			Type: "article",
			Where: []objects.ViewClause{
				{Property: "status", Op: objects.ViewOpEq, Value: "to-read"},
			}},
	})

	d := &Dashboard{}
	d.SetTypedObjects(reg, idx, cat, "articles-to-read", tmp)
	if d.primaryView == nil || d.primaryView.ID != "articles-to-read" {
		t.Fatalf("primary view not set: %+v", d.primaryView)
	}
	if len(d.primaryViewObjs) != 1 || d.primaryViewObjs[0].Title != "X" {
		t.Errorf("primary view didn't resolve: got %+v", d.primaryViewObjs)
	}
}

func TestDashboard_SetTypedObjects_NilSafe(t *testing.T) {
	d := &Dashboard{}
	d.SetTypedObjects(nil, nil, nil, "", "")
	if d.objTotal != 0 || len(d.objCountsByType) != 0 || len(d.objRecent) != 0 {
		t.Errorf("nil inputs should leave fields zero")
	}
}

func TestDailyJot_SetTypedObjects_PicksUpToday(t *testing.T) {
	tmp := t.TempDir()
	todayPath := filepath.Join(tmp, "Today.md")
	_ = os.WriteFile(todayPath, []byte("# x"), 0o644)
	// Set its mtime to "now" explicitly (file was just created so this
	// is already the case, but be explicit for the test).
	now := time.Now()
	_ = os.Chtimes(todayPath, now, now)

	yesterdayPath := filepath.Join(tmp, "Yesterday.md")
	_ = os.WriteFile(yesterdayPath, []byte("# y"), 0o644)
	yest := now.AddDate(0, 0, -1)
	_ = os.Chtimes(yesterdayPath, yest, yest)

	reg := objects.NewRegistryEmpty()
	_ = reg.Set(objects.Type{ID: "idea", Name: "Idea", Icon: "💡"})
	bld := objects.NewBuilder(reg)
	bld.Add("Today.md", "Today's Idea", map[string]string{"type": "idea"})
	bld.Add("Yesterday.md", "Yesterday's Idea", map[string]string{"type": "idea"})
	idx := bld.Finalize()

	dj := &DailyJot{}
	dj.SetTypedObjects(reg, idx, tmp)

	if dj.todayObjectCount != 1 {
		t.Errorf("expected 1 today, got %d", dj.todayObjectCount)
	}
	if len(dj.todayObjects) != 1 || dj.todayObjects[0].Title != "Today's Idea" {
		t.Errorf("wrong recent: %+v", dj.todayObjects)
	}
}

func TestDailyJot_SetTypedObjects_NilSafe(t *testing.T) {
	dj := &DailyJot{}
	dj.SetTypedObjects(nil, nil, "")
	if dj.todayObjectCount != 0 || len(dj.todayObjects) != 0 {
		t.Errorf("nil inputs should not populate")
	}
}
