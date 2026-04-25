package tasks

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// staticScan returns a fixed slice of NoteContents on every call.
// stubScan lets tests mutate the returned slice between Reload calls.
type stubScan struct {
	mu    sync.Mutex
	notes []NoteContent
}

func (s *stubScan) set(notes []NoteContent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = notes
}

func (s *stubScan) fn() []NoteContent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]NoteContent, len(s.notes))
	copy(out, s.notes)
	return out
}

func TestStore_LoadFreshVault_IngestsAllTasks(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{
		{Path: "Tasks.md", Content: "# Tasks\n- [ ] alpha\n- [ ] beta\n"},
		{Path: "2026-04-25.md", Content: "- [ ] jot capture\n"},
	})

	store, err := Load(vault, scan.fn)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	all := store.All()
	if len(all) != 3 {
		t.Fatalf("got %d tasks, want 3 — %+v", len(all), all)
	}
	for _, task := range all {
		if task.ID == "" {
			t.Errorf("task missing ID: %+v", task)
		}
		if task.CreatedAt.IsZero() {
			t.Errorf("task missing CreatedAt: %+v", task)
		}
	}
}

func TestStore_LoadFreshVault_PersistsSidecar(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{
		{Path: "Tasks.md", Content: "- [ ] persist me\n"},
	})

	store1, err := Load(vault, scan.fn)
	if err != nil {
		t.Fatal(err)
	}
	id := store1.All()[0].ID

	// Second Load with the same vault should hit the sidecar and
	// reuse the ID.
	store2, err := Load(vault, scan.fn)
	if err != nil {
		t.Fatal(err)
	}
	if store2.All()[0].ID != id {
		t.Errorf("ID changed across Load: %q vs %q", id, store2.All()[0].ID)
	}
}

func TestStore_Reload_AppliesEdits(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] before\n"}})
	store, err := Load(vault, scan.fn)
	if err != nil {
		t.Fatal(err)
	}
	id := store.All()[0].ID

	// External edit: same line, done toggled.
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [x] before\n"}})
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("len: %d", len(all))
	}
	if all[0].ID != id {
		t.Errorf("ID changed across edit: %q vs %q", id, all[0].ID)
	}
	if !all[0].Done {
		t.Error("Done should be true after edit")
	}
	if all[0].CompletedAt == nil {
		t.Error("CompletedAt should be set when Done flips true")
	}
}

func TestStore_Reload_HandlesDeletion(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] going away\n- [ ] staying\n"}})
	store, err := Load(vault, scan.fn)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.All()) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(store.All()))
	}

	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] staying\n"}})
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	if len(store.All()) != 1 {
		t.Fatalf("want 1 task after delete, got %d", len(store.All()))
	}
	if store.All()[0].Text != "staying" {
		t.Errorf("wrong survivor: %q", store.All()[0].Text)
	}
}

func TestStore_GetByID_ReturnsKnownTask(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] x\n"}})
	store, _ := Load(vault, scan.fn)
	id := store.All()[0].ID

	got, ok := store.GetByID(id)
	if !ok {
		t.Fatal("GetByID returned !ok for known ID")
	}
	if got.ID != id {
		t.Errorf("wrong task: %q", got.ID)
	}
}

func TestStore_GetByID_UnknownReturnsNotOk(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	store, _ := Load(vault, scan.fn)
	if _, ok := store.GetByID("nope"); ok {
		t.Error("GetByID should return !ok for unknown ID")
	}
}

func TestStore_GetByAnchor_ResolvesByLocation(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "\n\n- [ ] anchored\n"}})
	store, _ := Load(vault, scan.fn)

	got, ok := store.GetByAnchor("Tasks.md", 3)
	if !ok {
		t.Fatal("GetByAnchor missed line 3")
	}
	if got.Text != "anchored" {
		t.Errorf("wrong task: %q", got.Text)
	}
}

func TestStore_Filter_AppliesPredicate(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{
		Path:    "Tasks.md",
		Content: "- [ ] todo\n- [x] done\n- [ ] another\n",
	}})
	store, _ := Load(vault, scan.fn)

	open := store.Filter(func(t Task) bool { return !t.Done })
	if len(open) != 2 {
		t.Errorf("want 2 open, got %d", len(open))
	}
	done := store.Filter(func(t Task) bool { return t.Done })
	if len(done) != 1 {
		t.Errorf("want 1 done, got %d", len(done))
	}
}

func TestStore_Subscribe_FiresOnReload(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] x\n"}})
	store, _ := Load(vault, scan.fn)

	var fired int32
	done := make(chan Event, 1)
	unsub := store.Subscribe(func(ev Event) {
		atomic.AddInt32(&fired, 1)
		select {
		case done <- ev:
		default:
		}
	})
	defer unsub()

	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] x\n- [ ] y\n"}})
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-done:
		if ev.Kind != EventReloaded {
			t.Errorf("wrong event kind: %q", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not fire within 2s")
	}
}

func TestStore_Subscribe_UnsubscribeStopsCallbacks(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] x\n"}})
	store, _ := Load(vault, scan.fn)

	var fired int32
	unsub := store.Subscribe(func(ev Event) { atomic.AddInt32(&fired, 1) })
	unsub()

	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // give the would-be goroutine a chance
	if atomic.LoadInt32(&fired) != 0 {
		t.Errorf("unsubscribed callback fired %d times", fired)
	}
}

// TestStore_ConcurrentReloadAndRead exercises the lock discipline.
// Run with -race to catch any data race.
func TestStore_ConcurrentReloadAndRead(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] x\n"}})
	store, _ := Load(vault, scan.fn)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Reader goroutines
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = store.All()
					_, _ = store.GetByID("nope")
				}
			}
		}()
	}

	// Reloader goroutine — bumps content each round
	wg.Add(1)
	go func() {
		defer wg.Done()
		for round := 0; round < 50; round++ {
			scan.set([]NoteContent{{
				Path:    "Tasks.md",
				Content: "- [ ] x\n- [ ] round" + string(rune('0'+round%10)) + "\n",
			}})
			if err := store.Reload(); err != nil {
				t.Errorf("reload: %v", err)
				return
			}
		}
	}()

	// Let it run a bit, then stop readers
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestStore_ConsumeSaveError_ReturnsAndClears(t *testing.T) {
	vault := t.TempDir()
	scan := &stubScan{}
	scan.set([]NoteContent{{Path: "Tasks.md", Content: "- [ ] x\n"}})
	store, _ := Load(vault, scan.fn)

	// No errors yet
	if err := store.ConsumeSaveError(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// Inject a save error directly (no public API for this in
	// production — testing the consume side only).
	store.mu.Lock()
	store.saveErr = errStub("disk full")
	store.mu.Unlock()

	got := store.ConsumeSaveError()
	if got == nil || got.Error() != "disk full" {
		t.Errorf("expected disk full, got %v", got)
	}
	// Second call should return nil.
	if err := store.ConsumeSaveError(); err != nil {
		t.Errorf("expected nil after consume, got %v", err)
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }
