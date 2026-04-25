package tasks

import (
	"fmt"
	"sync"
	"time"
)

// ScanFunc returns a snapshot of every parseable note in the vault.
// Callers (typically tui.Model) wire it to vault.Notes; the store
// calls it on Load and on Reload. Decoupling from internal/vault
// keeps the dependency direction clean — tasks → atomicio + stdlib
// only.
type ScanFunc func() []NoteContent

// TaskStore is the canonical task layer. Reads from vault notes via
// the injected ScanFunc, writes through the markdown notes and the
// .granit/tasks-meta.json sidecar. Stable IDs survive markdown
// edits via the reconciliation algorithm in reconcile.go.
//
// Goroutine-safe. Read methods take an RLock and copy data out;
// write methods take the write lock, mutate, persist, then release
// before notifying subscribers.
type TaskStore struct {
	vaultRoot string
	scan      ScanFunc

	mu       sync.RWMutex
	tasks    map[string]*Task   // ID → task
	byAnchor map[anchorKey]string // (file, line) → ID
	byFP     map[string][]string  // fingerprint → []ID
	sidecar  sidecarFile          // last loaded/saved sidecar
	subs     []subscription
	saveErr  error
	nextSubID int
}

type anchorKey struct {
	File string
	Line int
}

type subscription struct {
	id int
	fn func(Event)
}

// Load opens (or creates) a task store rooted at vaultRoot. Calls
// scan() once to ingest current state. If no sidecar exists, runs
// first-ingestion mode: every parsed task gets a fresh ULID, a
// timestamp from now, and triage=inbox. If a sidecar exists, runs
// the 6-pass reconciliation to glue stable IDs to the current
// markdown.
//
// Never returns nil store on error — corrupt sidecars are backed up
// and treated as missing so the app can boot. The error return is
// reserved for catastrophic failures (the scan func panics, etc.)
// that we genuinely can't recover from.
func Load(vaultRoot string, scan ScanFunc) (*TaskStore, error) {
	if scan == nil {
		return nil, fmt.Errorf("tasks: Load requires a non-nil ScanFunc")
	}
	s := &TaskStore{
		vaultRoot: vaultRoot,
		scan:      scan,
		tasks:     make(map[string]*Task),
		byAnchor:  make(map[anchorKey]string),
		byFP:      make(map[string][]string),
	}
	side, _ := loadSidecar(SidecarPath(vaultRoot))
	s.sidecar = side
	if err := s.reloadLocked(); err != nil {
		return s, err // store is still usable; caller may surface the error
	}
	return s, nil
}

// All returns a snapshot of every known task in stable ULID order
// (ULIDs are time-sortable, so this is also creation order).
func (s *TaskStore) All() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, *t)
	}
	sortByID(out)
	return out
}

// Filter returns tasks for which pred returns true. Same snapshot
// semantics as All — safe to hold the result across mutations.
func (s *TaskStore) Filter(pred func(Task) bool) []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Task
	for _, t := range s.tasks {
		if pred(*t) {
			out = append(out, *t)
		}
	}
	sortByID(out)
	return out
}

// GetByID returns the task with the given ULID, if known.
func (s *TaskStore) GetByID(id string) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *t, true
}

// GetByAnchor looks up a task by its current source location.
// Useful for code paths that haven't been migrated to ID-based
// identity yet.
func (s *TaskStore) GetByAnchor(file string, line int) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byAnchor[anchorKey{file, line}]
	if !ok {
		return Task{}, false
	}
	return *s.tasks[id], true
}

// Reload re-runs scan(), reconciles IDs against the existing
// sidecar, persists the new state, and notifies subscribers. Safe
// to call concurrently with reads — readers see the prior snapshot
// until Reload completes.
func (s *TaskStore) Reload() error {
	s.mu.Lock()
	if err := s.reloadLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.notify(Event{Kind: EventReloaded})
	return nil
}

// reloadLocked is the lock-held implementation of Reload, also
// used by Load for the initial ingestion. Caller must hold the
// write lock.
func (s *TaskStore) reloadLocked() error {
	notes := s.scan()
	parsed := ParseNotes(notes)
	reconciled := reconcile(reconcileInput{
		parsed:     parsed,
		sidecar:    s.sidecar,
		now:        time.Now().UTC(),
		newID:      NewID,
	})

	s.tasks = make(map[string]*Task, len(reconciled.tasks))
	s.byAnchor = make(map[anchorKey]string, len(reconciled.tasks))
	s.byFP = make(map[string][]string, len(reconciled.tasks))
	for i := range reconciled.tasks {
		t := &reconciled.tasks[i]
		s.tasks[t.ID] = t
		s.byAnchor[anchorKey{t.NotePath, t.LineNum}] = t.ID
		s.byFP[Fingerprint(t.Text)] = append(s.byFP[Fingerprint(t.Text)], t.ID)
	}
	s.sidecar = reconciled.sidecar
	if err := saveSidecar(SidecarPath(s.vaultRoot), s.sidecar); err != nil {
		s.saveErr = err
		return err
	}
	return nil
}

// Subscribe registers a callback invoked after each store change
// is committed. Returns an unsubscribe function. Callbacks fire on
// a background goroutine after the store has released its lock so
// subscribers can safely call back into the store without
// deadlocking.
func (s *TaskStore) Subscribe(fn func(Event)) (unsubscribe func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSubID++
	id := s.nextSubID
	s.subs = append(s.subs, subscription{id: id, fn: fn})
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, sub := range s.subs {
			if sub.id == id {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
	}
}

// notify fires Subscribe callbacks. Caller must NOT hold the lock.
// Each callback runs in a goroutine so a slow subscriber can't
// block reconciliation.
func (s *TaskStore) notify(ev Event) {
	s.mu.RLock()
	subs := make([]subscription, len(s.subs))
	copy(subs, s.subs)
	s.mu.RUnlock()
	for _, sub := range subs {
		go sub.fn(ev)
	}
}

// ConsumeSaveError returns and clears the most recent persistence
// error, if any. Mirrors the existing pattern in tui (pomodoro,
// clockin) so the Model can surface store errors in the status bar
// without holding a permanent reference.
func (s *TaskStore) ConsumeSaveError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.saveErr
	s.saveErr = nil
	return err
}

// SidecarPath exposes the persistence path for diagnostic UIs.
func (s *TaskStore) SidecarPath() string {
	return SidecarPath(s.vaultRoot)
}

// sortByID sorts tasks in-place by ULID. Stable ULID order ≈
// stable creation order, which is the most useful default for the
// triage queue (oldest first) and avoids surprising shuffles when
// a task gets re-anchored.
func sortByID(tasks []Task) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j-1].ID > tasks[j].ID; j-- {
			tasks[j-1], tasks[j] = tasks[j], tasks[j-1]
		}
	}
}
