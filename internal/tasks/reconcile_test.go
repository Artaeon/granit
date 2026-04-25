package tasks

import (
	"strconv"
	"testing"
	"time"
)

// idGen yields deterministic monotonic IDs for tests so we can
// assert "the new task got id N1, then N2" without ULID noise.
type idGen struct{ n int }

func (g *idGen) next() string {
	g.n++
	return "ID-" + strconv.Itoa(g.n)
}

func mkTask(file string, line int, text string) Task {
	return Task{NotePath: file, LineNum: line, Text: text}
}

func mkSidecarTask(id, file string, line int, text string, triage TriageState) sidecarTask {
	return sidecarTask{
		ID:          id,
		Fingerprint: Fingerprint(text),
		Anchor:      sidecarAnchor{File: file, Line: line},
		NormText:    NormalizeTaskText(text),
		Triage:      triage,
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestReconcile_FreshIngestion_AssignsIDsAndDefaults(t *testing.T) {
	g := &idGen{}
	in := reconcileInput{
		parsed: []Task{
			mkTask("Tasks.md", 1, "- [ ] alpha"),
			mkTask("2026-04-25.md", 3, "- [ ] beta jotted today"),
		},
		sidecar: sidecarFile{},
		now:     time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC),
		newID:   g.next,
	}
	out := reconcile(in)

	if len(out.tasks) != 2 {
		t.Fatalf("tasks: got %d want 2", len(out.tasks))
	}
	if len(out.created) != 2 {
		t.Errorf("expected 2 created IDs, got %v", out.created)
	}

	// First-ingestion defaults: triage=inbox, origin=manual or jot,
	// CreatedAt set, CompletedAt nil.
	for _, task := range out.tasks {
		if task.Triage != TriageInbox {
			t.Errorf("task %s triage: got %q want inbox", task.ID, task.Triage)
		}
		if task.CreatedAt.IsZero() {
			t.Errorf("task %s CreatedAt unset", task.ID)
		}
		if task.CompletedAt != nil {
			t.Errorf("task %s should have nil CompletedAt", task.ID)
		}
	}

	// Daily-note task should be tagged as jot origin.
	for _, task := range out.tasks {
		switch task.NotePath {
		case "2026-04-25.md":
			if task.Origin != OriginJot {
				t.Errorf("daily-note task: origin = %q, want jot", task.Origin)
			}
		case "Tasks.md":
			if task.Origin != OriginManual {
				t.Errorf("Tasks.md task: origin = %q, want manual", task.Origin)
			}
		}
	}
}

func TestReconcile_Pass1_ExactMatchPreservesID(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("KEEP-ME", "Tasks.md", 5, "- [ ] ship phase 2", TriageScheduled),
		},
	}
	in := reconcileInput{
		// Same file, same line, same text — strongest possible match.
		parsed:  []Task{mkTask("Tasks.md", 5, "- [ ] ship phase 2")},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if len(out.tasks) != 1 || out.tasks[0].ID != "KEEP-ME" {
		t.Fatalf("ID lost: got %+v", out.tasks)
	}
	if out.tasks[0].Triage != TriageScheduled {
		t.Errorf("triage state lost: %q", out.tasks[0].Triage)
	}
	if len(out.created) != 0 {
		t.Errorf("should not have created any IDs: %v", out.created)
	}
}

func TestReconcile_Pass1_SurvivesDoneToggleAndMetadataEdit(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("KEEP", "Tasks.md", 1, "- [ ] write design doc", TriageScheduled),
		},
	}
	// Same line, but: done flipped, due-date added, priority bumped.
	in := reconcileInput{
		parsed: []Task{
			mkTask("Tasks.md", 1, "- [x] write design doc 📅 2026-04-30 ⏫"),
		},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID != "KEEP" {
		t.Fatalf("ID changed under metadata edit: %s", out.tasks[0].ID)
	}
}

func TestReconcile_Pass2_LineMoveWithinFile(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("KEEP", "Tasks.md", 5, "- [ ] alpha", TriageInbox),
		},
	}
	// Same file, same text, line moved 5 → 30.
	in := reconcileInput{
		parsed:  []Task{mkTask("Tasks.md", 30, "- [ ] alpha")},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID != "KEEP" {
		t.Fatalf("ID lost on line move: %+v", out.tasks)
	}
	if out.tasks[0].LineNum != 30 {
		t.Errorf("anchor not refreshed: line=%d want 30", out.tasks[0].LineNum)
	}
	// Drift event should be recorded.
	if len(out.updated) != 1 || out.updated[0] != "KEEP" {
		t.Errorf("expected drift event, got %v", out.updated)
	}
}

func TestReconcile_Pass3_CrossFileMove(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("MOVED", "2026-04-20.md", 3, "- [ ] follow up with client", TriageInbox),
		},
	}
	// Task migrated to Tasks.md; old file no longer contains it.
	in := reconcileInput{
		parsed:  []Task{mkTask("Tasks.md", 1, "- [ ] follow up with client")},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID != "MOVED" {
		t.Fatalf("ID lost on cross-file move: %+v", out.tasks)
	}
	if out.tasks[0].NotePath != "Tasks.md" {
		t.Errorf("anchor not refreshed: file=%s", out.tasks[0].NotePath)
	}
}

func TestReconcile_Pass4_AmbiguousFingerprintByProximity(t *testing.T) {
	g := &idGen{}
	// Two sidecar entries with the SAME text in the SAME file.
	// The reconciler should pair each parsed line to the closest
	// sidecar entry by line number.
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("CLOSER",  "Tasks.md", 10, "- [ ] same", TriageInbox),
			mkSidecarTask("FARTHER", "Tasks.md", 50, "- [ ] same", TriageScheduled),
		},
	}
	in := reconcileInput{
		parsed: []Task{
			mkTask("Tasks.md", 12, "- [ ] same"), // closest to line 10
			mkTask("Tasks.md", 48, "- [ ] same"), // closest to line 50
		},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	idAtLine := map[int]string{}
	for _, t := range out.tasks {
		idAtLine[t.LineNum] = t.ID
	}
	if idAtLine[12] != "CLOSER" {
		t.Errorf("line 12 expected CLOSER, got %q", idAtLine[12])
	}
	if idAtLine[48] != "FARTHER" {
		t.Errorf("line 48 expected FARTHER, got %q", idAtLine[48])
	}
}

func TestReconcile_Pass5_FuzzyMatchOnReworded(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("REWORD", "Tasks.md", 5,
				"- [ ] ship the unified task store design doc by friday",
				TriageScheduled),
		},
	}
	// User reworded — single word swap in a longer task. Similarity
	// stays above 0.85 (small fraction of total chars), within 10
	// lines, so Pass 5 should reuse the ID.
	in := reconcileInput{
		parsed: []Task{
			mkTask("Tasks.md", 6,
				"- [ ] ship the unified task store design plan by friday"),
		},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID != "REWORD" {
		t.Fatalf("fuzzy fallback failed: %+v", out.tasks)
	}
	if out.tasks[0].Triage != TriageScheduled {
		t.Errorf("triage lost across fuzzy: %q", out.tasks[0].Triage)
	}
}

func TestReconcile_Pass5_RejectsBelowThreshold(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("OLD", "Tasks.md", 5, "- [ ] ship phase 2 design", TriageInbox),
		},
	}
	// Edit too aggressive — adds " doc" to a 19-char task, which
	// is ~21% change → ratio ~0.83 → below the 0.85 gate. We'd
	// rather mint a new ID and tombstone OLD than silently
	// re-attribute.
	in := reconcileInput{
		parsed:  []Task{mkTask("Tasks.md", 6, "- [ ] ship phase 2 design doc")},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID == "OLD" {
		t.Errorf("fuzzy match should have been rejected by threshold")
	}
	if len(out.deleted) != 1 || out.deleted[0] != "OLD" {
		t.Errorf("OLD should have been tombstoned, got deleted=%v", out.deleted)
	}
}

func TestReconcile_Pass5_GatedByLineDistance(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("OLD", "Tasks.md", 5, "- [ ] write a doc about phase 2", TriageInbox),
		},
	}
	// Wording is similar but line moved by 50 — outside the
	// fuzzy gate (≤ 10). Should mint a new ID and tombstone OLD.
	in := reconcileInput{
		parsed:  []Task{mkTask("Tasks.md", 80, "- [ ] write a doc about phase 2 deliverable")},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID == "OLD" {
		t.Errorf("fuzzy should have been gated by line distance, got %s", out.tasks[0].ID)
	}
}

func TestReconcile_Pass6_MintsNewIDForTrulyNewLine(t *testing.T) {
	g := &idGen{}
	in := reconcileInput{
		parsed:  []Task{mkTask("Tasks.md", 1, "- [ ] brand new task")},
		sidecar: sidecarFile{},
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID != "ID-1" {
		t.Errorf("expected ID-1, got %s", out.tasks[0].ID)
	}
	if len(out.created) != 1 {
		t.Errorf("expected 1 created ID, got %v", out.created)
	}
}

func TestReconcile_DeletedTaskBecomesTombstone(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tasks: []sidecarTask{
			mkSidecarTask("GONE", "Tasks.md", 1, "- [ ] deleted thing", TriageInbox),
		},
	}
	// No parsed lines — task was removed from markdown.
	in := reconcileInput{
		parsed:  nil,
		sidecar: side,
		now:     time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
		newID:   g.next,
	}
	out := reconcile(in)
	if len(out.tasks) != 0 {
		t.Errorf("expected no tasks, got %v", out.tasks)
	}
	if len(out.deleted) != 1 || out.deleted[0] != "GONE" {
		t.Errorf("expected GONE in deleted, got %v", out.deleted)
	}
	if len(out.sidecar.Tombstones) != 1 || out.sidecar.Tombstones[0].ID != "GONE" {
		t.Errorf("tombstone missing: %+v", out.sidecar.Tombstones)
	}
}

func TestReconcile_TombstoneRevival_ReusesOriginalID(t *testing.T) {
	g := &idGen{}
	side := sidecarFile{
		Tombstones: []sidecarTombstone{
			{
				ID:          "REVIVED",
				Fingerprint: Fingerprint("- [ ] back from the dead"),
				NormText:    NormalizeTaskText("- [ ] back from the dead"),
				RemovedAt:   time.Now().Add(-time.Hour),
			},
		},
	}
	in := reconcileInput{
		parsed:  []Task{mkTask("Tasks.md", 1, "- [ ] back from the dead")},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].ID != "REVIVED" {
		t.Fatalf("expected REVIVED, got %s", out.tasks[0].ID)
	}
	// Tombstone should be cleared.
	for _, tomb := range out.sidecar.Tombstones {
		if tomb.ID == "REVIVED" {
			t.Errorf("tombstone still present after revival: %+v", out.sidecar.Tombstones)
		}
	}
}

func TestReconcile_DonePopulatesCompletedAt(t *testing.T) {
	g := &idGen{}
	now := time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC)
	in := reconcileInput{
		parsed: []Task{{
			NotePath: "Tasks.md", LineNum: 1,
			Text: "- [x] done already", Done: true,
		}},
		sidecar: sidecarFile{},
		now:     now,
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].CompletedAt == nil || !out.tasks[0].CompletedAt.Equal(now) {
		t.Errorf("CompletedAt: got %v want %v", out.tasks[0].CompletedAt, now)
	}
}

func TestReconcile_UncompletingClearsCompletedAt(t *testing.T) {
	g := &idGen{}
	completed := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	side := sidecarFile{
		Tasks: []sidecarTask{
			{
				ID:          "T",
				Fingerprint: Fingerprint("- [ ] toggle me"),
				Anchor:      sidecarAnchor{File: "Tasks.md", Line: 1},
				NormText:    NormalizeTaskText("- [ ] toggle me"),
				Triage:      TriageDone,
				CompletedAt: &completed,
			},
		},
	}
	in := reconcileInput{
		parsed: []Task{
			{NotePath: "Tasks.md", LineNum: 1, Text: "- [ ] toggle me", Done: false},
		},
		sidecar: side,
		now:     time.Now(),
		newID:   g.next,
	}
	out := reconcile(in)
	if out.tasks[0].CompletedAt != nil {
		t.Errorf("CompletedAt should clear when task uncompleted, got %v", out.tasks[0].CompletedAt)
	}
}
