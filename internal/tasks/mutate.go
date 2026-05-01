package tasks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// ErrNotFound is returned by mutating methods when the given ID is
// not in the store. Callers can errors.Is(err, ErrNotFound) without
// inspecting the error string.
var ErrNotFound = errors.New("tasks: not found")

// UpdateMeta mutates sidecar-only fields (Triage, ScheduledStart,
// Duration, ProjectID, GoalID, Notes, etc.) without touching the
// markdown line. The mutator receives a copy of the current Task —
// modify it in place and return; the store applies the changes
// under the write lock.
//
// Markdown-derived fields (Text, Done, DueDate, Priority, etc.)
// can be set in the mutator, but UpdateMeta will NOT rewrite the
// markdown to reflect them — use UpdateLine for that.
func (s *TaskStore) UpdateMeta(id string, mut func(*Task)) error {
	s.mu.Lock()
	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	old := *t
	mut(t)
	s.upsertSidecarTaskLocked(t)
	if err := s.persistSidecarLocked(); err != nil {
		// Roll back the in-memory mutation to keep store and disk
		// coherent — caller can retry without seeing partial state.
		*t = old
		s.upsertSidecarTaskLocked(t)
		s.mu.Unlock()
		return err
	}
	new := *t
	s.mu.Unlock()
	s.notify(Event{Kind: EventUpdated, ID: id, Old: &old, New: &new})
	return nil
}

// UpdateLine rewrites the markdown line for the given task. The
// transform receives the current line (including the "- [ ]"
// prefix) and returns the replacement. The store re-parses the new
// line, updates the in-memory Task accordingly, persists the
// markdown file atomically, and refreshes the sidecar.
//
// Use for done-toggle, due-date set, priority cycle, text edits,
// add/remove tag — anything the user could type into the line by
// hand.
//
// Subscriber callbacks fire after the lock is released so a
// callback that synchronously calls back into a write method
// won't deadlock on its own write lock.
func (s *TaskStore) UpdateLine(id string, transform func(line string) string) error {
	s.mu.Lock()
	// Note: do NOT `defer Unlock` here. We need to release the
	// lock explicitly before calling notify so subscribers don't
	// observe the store mid-mutation and don't risk deadlocking
	// on a re-entrant write.

	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	notePath := t.NotePath
	lineNum := t.LineNum

	abs, err := resolveInVault(s.vaultRoot, notePath)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("tasks: %s: %w", notePath, err)
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("tasks: read %s: %w", notePath, err)
	}
	lines := strings.Split(string(content), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		s.mu.Unlock()
		return fmt.Errorf("tasks: line %d out of range in %s (len=%d)", lineNum, notePath, len(lines))
	}
	oldLine := lines[lineNum-1]
	newLine := transform(oldLine)
	if newLine == oldLine {
		s.mu.Unlock()
		return nil // no-op
	}
	lines[lineNum-1] = newLine
	if err := atomicio.WriteNote(abs, strings.Join(lines, "\n")); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("tasks: write %s: %w", notePath, err)
	}

	// Re-parse the new line via the parser's full machinery so
	// fields like Tags / Priority / DueDate stay consistent with
	// what a fresh ParseNotes would produce.
	reparsed := parseNote(NoteContent{Path: notePath, Content: strings.Join(lines, "\n")})
	var newTask *Task
	for i := range reparsed {
		if reparsed[i].LineNum == lineNum {
			newTask = &reparsed[i]
			break
		}
	}
	if newTask == nil {
		// User's transform made the line stop matching the task
		// regex (deleted the checkbox, etc.) — treat as a delete.
		old := *t
		s.deleteInMemoryLocked(id, time.Now().UTC())
		if err := s.persistSidecarLocked(); err != nil {
			s.mu.Unlock()
			return err
		}
		s.mu.Unlock()
		s.notify(Event{Kind: EventDeleted, ID: id, Old: &old})
		return nil
	}

	old := *t
	// Preserve sidecar-only fields across the line edit.
	newTask.ID = id
	newTask.Triage = t.Triage
	newTask.ScheduledStart = t.ScheduledStart
	newTask.Duration = t.Duration
	newTask.ProjectID = t.ProjectID
	if newTask.GoalID == "" {
		newTask.GoalID = t.GoalID
	}
	newTask.Origin = t.Origin
	newTask.CreatedAt = t.CreatedAt
	newTask.LastTriagedAt = t.LastTriagedAt
	newTask.Notes = t.Notes
	// Done state may have changed via the transform — keep
	// CompletedAt in sync.
	switch {
	case newTask.Done && t.CompletedAt == nil:
		now := time.Now().UTC()
		newTask.CompletedAt = &now
	case !newTask.Done:
		newTask.CompletedAt = nil
	default:
		newTask.CompletedAt = t.CompletedAt
	}

	// Reindex: delete old anchor + fp, install new ones.
	delete(s.byAnchor, anchorKey{old.NotePath, old.LineNum})
	s.removeFPRefLocked(Fingerprint(old.Text), id)
	s.tasks[id] = newTask
	s.byAnchor[anchorKey{newTask.NotePath, newTask.LineNum}] = id
	s.byFP[Fingerprint(newTask.Text)] = append(s.byFP[Fingerprint(newTask.Text)], id)
	s.upsertSidecarTaskLocked(newTask)

	if err := s.persistSidecarLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.notify(Event{Kind: EventUpdated, ID: id, Old: &old, New: newTask})
	return nil
}

// Triage sets the planning-loop state for a task. Pure sidecar
// edit — markdown unchanged. Updates LastTriagedAt automatically.
func (s *TaskStore) Triage(id string, state TriageState) error {
	now := time.Now().UTC()
	return s.UpdateMeta(id, func(t *Task) {
		t.Triage = state
		t.LastTriagedAt = &now
	})
}

// Schedule places a task on the calendar. Pure sidecar edit —
// markdown unchanged. Setting state to TriageScheduled is the
// caller's responsibility (Triage() can be called separately).
func (s *TaskStore) Schedule(id string, start time.Time, dur time.Duration) error {
	return s.UpdateMeta(id, func(t *Task) {
		t.ScheduledStart = &start
		t.Duration = dur
	})
}

// Complete marks a task done. Updates the markdown checkbox AND
// the sidecar — composes UpdateLine + UpdateMeta so callers don't
// have to think about it.
func (s *TaskStore) Complete(id string) error {
	if err := s.UpdateLine(id, toggleCheckbox(true)); err != nil {
		return err
	}
	return s.UpdateMeta(id, func(t *Task) {
		t.Triage = TriageDone
	})
}

// Create appends a new task to the destination file (Tasks.md by
// default), assigns a fresh ULID, persists the sidecar, and
// returns the new Task with all fields populated.
//
// The text argument should be the bare task body without the
// checkbox prefix — Create wraps it as "- [ ] {text}". To create
// an already-done task, set Done in opts (not yet supported in
// Phase 2; use UpdateLine after Create for now).
func (s *TaskStore) Create(text string, opts CreateOpts) (Task, error) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return Task{}, errors.New("tasks: Create requires non-empty text")
	}
	// Single-line invariant: a task is one markdown line. If
	// the caller passes a string with embedded newlines we'd
	// silently write multiple task lines (or break a markdown
	// fenced block) — reject explicitly so future widgets,
	// Lua plugins, or AI auto-capture can't accidentally
	// inject hidden tasks via a multiline buffer.
	if strings.ContainsAny(cleaned, "\n\r") {
		return Task{}, errors.New("tasks: Create text must be a single line — newlines not allowed")
	}
	// Normalize: if the caller pre-wrapped it, accept as-is.
	taskLine := cleaned
	if !strings.HasPrefix(strings.TrimLeft(cleaned, " \t"), "- [") {
		taskLine = "- [ ] " + cleaned
	}

	dest := opts.File
	if dest == "" {
		dest = "Tasks.md"
	}

	s.mu.Lock()
	// Explicit Unlock + sync notify (no defer). See UpdateLine.

	abs, err := resolveInVault(s.vaultRoot, dest)
	if err != nil {
		s.mu.Unlock()
		return Task{}, fmt.Errorf("tasks: %s: %w", dest, err)
	}
	if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
		s.mu.Unlock()
		return Task{}, fmt.Errorf("tasks: mkdir %s: %w", filepath.Dir(dest), mkErr)
	}
	existing, err := os.ReadFile(abs)
	if err != nil && !os.IsNotExist(err) {
		s.mu.Unlock()
		return Task{}, fmt.Errorf("tasks: read %s: %w", dest, err)
	}
	// If a target Section heading is specified and present, insert the new
	// task line directly after the heading (and any single blank line that
	// follows). Otherwise fall back to the historical "append at end"
	// behavior. Either path yields a single atomic file write.
	var newContent string
	insertedLine := 0
	if opts.Section != "" {
		if c, ln, ok := insertUnderSection(string(existing), opts.Section, taskLine); ok {
			newContent = c
			insertedLine = ln
		}
	}
	if newContent == "" {
		var buf strings.Builder
		if len(existing) == 0 && dest == "Tasks.md" {
			buf.WriteString("# Tasks\n\n")
		} else {
			buf.Write(existing)
			if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
				buf.WriteByte('\n')
			}
		}
		buf.WriteString(taskLine)
		buf.WriteByte('\n')
		newContent = buf.String()
		// Inserted line is the last line of newContent. We compute it after
		// parsing below if needed; for the append path, "last task" works.
	}
	if err := atomicio.WriteNote(abs, newContent); err != nil {
		s.mu.Unlock()
		return Task{}, fmt.Errorf("tasks: write %s: %w", dest, err)
	}

	// Re-parse the destination file and find the just-inserted task. If we
	// inserted into a section we know the exact line; otherwise it's the
	// last task line in the file (append path).
	reparsed := parseNote(NoteContent{Path: dest, Content: newContent})
	if len(reparsed) == 0 {
		s.mu.Unlock()
		return Task{}, errors.New("tasks: Create wrote a line that didn't parse as a task — check the input format")
	}
	var t Task
	if insertedLine > 0 {
		found := false
		for _, p := range reparsed {
			if p.LineNum == insertedLine {
				t = p
				found = true
				break
			}
		}
		if !found {
			t = reparsed[len(reparsed)-1]
		}
	} else {
		t = reparsed[len(reparsed)-1]
	}
	t.ID = NewID()

	if opts.Origin != "" {
		t.Origin = opts.Origin
	} else if isDailyNoteFile(dest) {
		t.Origin = OriginJot
	} else {
		t.Origin = OriginManual
	}
	if opts.Triage != "" {
		t.Triage = opts.Triage
	} else {
		t.Triage = TriageInbox
	}
	if opts.ProjectID != "" {
		t.ProjectID = opts.ProjectID
	}
	if opts.GoalID != "" {
		t.GoalID = opts.GoalID
	}
	t.CreatedAt = time.Now().UTC()

	// Install in indices.
	s.tasks[t.ID] = &t
	s.byAnchor[anchorKey{t.NotePath, t.LineNum}] = t.ID
	s.byFP[Fingerprint(t.Text)] = append(s.byFP[Fingerprint(t.Text)], t.ID)
	s.upsertSidecarTaskLocked(&t)

	if err := s.persistSidecarLocked(); err != nil {
		// Roll back in-memory state to stay coherent with disk.
		delete(s.tasks, t.ID)
		delete(s.byAnchor, anchorKey{t.NotePath, t.LineNum})
		s.removeFPRefLocked(Fingerprint(t.Text), t.ID)
		s.mu.Unlock()
		return Task{}, err
	}
	s.mu.Unlock()
	s.notify(Event{Kind: EventCreated, ID: t.ID, New: &t})
	return t, nil
}

// Delete removes a task line from its source file and tombstones
// the ID so a future re-introduction (via git pull) can revive it
// instead of getting a fresh ID.
func (s *TaskStore) Delete(id string) error {
	s.mu.Lock()
	// Explicit Unlock + sync notify (no defer). See UpdateLine.

	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	notePath := t.NotePath
	lineNum := t.LineNum

	abs, err := resolveInVault(s.vaultRoot, notePath)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("tasks: %s: %w", notePath, err)
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("tasks: read %s: %w", notePath, err)
	}
	lines := strings.Split(string(content), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		s.mu.Unlock()
		return fmt.Errorf("tasks: line %d out of range in %s", lineNum, notePath)
	}
	// Drop the line; preserve trailing newline shape.
	lines = append(lines[:lineNum-1], lines[lineNum:]...)
	if err := atomicio.WriteNote(abs, strings.Join(lines, "\n")); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("tasks: write %s: %w", notePath, err)
	}

	old := *t
	s.deleteInMemoryLocked(id, time.Now().UTC())
	if err := s.persistSidecarLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.notify(Event{Kind: EventDeleted, ID: id, Old: &old})
	return nil
}

// ── lock-held helpers ─────────────────────────────────────────

// deleteInMemoryLocked removes a task from in-memory indices and
// adds a tombstone. Caller holds the write lock.
func (s *TaskStore) deleteInMemoryLocked(id string, now time.Time) {
	t, ok := s.tasks[id]
	if !ok {
		return
	}
	delete(s.tasks, id)
	delete(s.byAnchor, anchorKey{t.NotePath, t.LineNum})
	s.removeFPRefLocked(Fingerprint(t.Text), id)
	// Tombstone in sidecar.
	s.sidecar.Tombstones = append(s.sidecar.Tombstones, sidecarTombstone{
		ID:          id,
		Fingerprint: Fingerprint(t.Text),
		NormText:    NormalizeTaskText(t.Text),
		RemovedAt:   now,
	})
	// Remove from sidecar.Tasks.
	for i := range s.sidecar.Tasks {
		if s.sidecar.Tasks[i].ID == id {
			s.sidecar.Tasks = append(s.sidecar.Tasks[:i], s.sidecar.Tasks[i+1:]...)
			break
		}
	}
}

// upsertSidecarTaskLocked sets the sidecar entry for the given
// task to match its current in-memory state. Caller holds the
// write lock.
func (s *TaskStore) upsertSidecarTaskLocked(t *Task) {
	st := sidecarTask{
		ID:              t.ID,
		Fingerprint:     Fingerprint(t.Text),
		Anchor:          sidecarAnchor{File: t.NotePath, Line: t.LineNum, Indent: t.Indent},
		NormText:        NormalizeTaskText(t.Text),
		Triage:          t.Triage,
		ScheduledStart:  t.ScheduledStart,
		DurationMinutes: int(t.Duration / time.Minute),
		ProjectID:       t.ProjectID,
		GoalID:          t.GoalID,
		Origin:          t.Origin,
		CreatedAt:       t.CreatedAt,
		LastTriagedAt:   t.LastTriagedAt,
		CompletedAt:     t.CompletedAt,
		Notes:           t.Notes,
	}
	for i := range s.sidecar.Tasks {
		if s.sidecar.Tasks[i].ID == t.ID {
			s.sidecar.Tasks[i] = st
			return
		}
	}
	s.sidecar.Tasks = append(s.sidecar.Tasks, st)
}

// removeFPRefLocked drops one ID from the byFP slice for the given
// fingerprint. Caller holds the write lock.
func (s *TaskStore) removeFPRefLocked(fp, id string) {
	ids := s.byFP[fp]
	for i, x := range ids {
		if x == id {
			s.byFP[fp] = append(ids[:i], ids[i+1:]...)
			if len(s.byFP[fp]) == 0 {
				delete(s.byFP, fp)
			}
			return
		}
	}
}

// persistSidecarLocked writes the sidecar to disk. Caller holds
// the write lock. Save errors are also recorded in s.saveErr for
// the ConsumeSaveError() reporting path.
func (s *TaskStore) persistSidecarLocked() error {
	if err := saveSidecar(SidecarPath(s.vaultRoot), s.sidecar); err != nil {
		s.saveErr = err
		return err
	}
	return nil
}

// toggleCheckbox returns an UpdateLine transform that flips the
// checkbox to the given done state. Idempotent — calling
// toggleCheckbox(true) on an already-done line is a no-op.
func toggleCheckbox(done bool) func(string) string {
	return func(line string) string {
		// Find the "[ ]" or "[x]" / "[X]" and replace.
		idx := strings.Index(line, "[")
		if idx < 0 || idx+3 > len(line) || line[idx+2] != ']' {
			return line
		}
		ch := byte(' ')
		if done {
			ch = 'x'
		}
		return line[:idx+1] + string(ch) + line[idx+2:]
	}
}

// insertUnderSection finds a markdown heading whose trimmed text matches
// `section` (e.g. "## Tasks", "### Habits") and inserts `taskLine` directly
// after it. Returns the new content, the 1-indexed line number of the
// inserted line, and ok=true on success. Returns ok=false if the section
// isn't present so the caller can fall back to append-at-end.
//
// Heading match is case-sensitive on the heading text and tolerates an
// optional blank line right after the heading (the typical markdown form).
func insertUnderSection(content, section, taskLine string) (string, int, bool) {
	want := strings.TrimSpace(section)
	if want == "" {
		return "", 0, false
	}
	lines := strings.Split(content, "\n")
	hit := -1
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t\r") == want {
			hit = i
			break
		}
	}
	if hit < 0 {
		return "", 0, false
	}
	insertAt := hit + 1
	// Skip a single blank line directly after the heading so the new task
	// doesn't sit awkwardly right against the header underline.
	if insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
		insertAt++
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:insertAt]...)
	out = append(out, taskLine)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n"), insertAt + 1, true
}
