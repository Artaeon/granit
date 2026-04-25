package tasks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// freshStore writes the given files into a temp vault and returns a
// loaded store + the vault path so tests can assert on disk state.
func freshStore(t *testing.T, files map[string]string) (*TaskStore, string) {
	t.Helper()
	vault := t.TempDir()
	for path, content := range files {
		full := filepath.Join(vault, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scan := func() []NoteContent {
		var out []NoteContent
		for path := range files {
			data, _ := os.ReadFile(filepath.Join(vault, path))
			out = append(out, NoteContent{Path: path, Content: string(data)})
		}
		return out
	}
	store, err := Load(vault, scan)
	if err != nil {
		t.Fatal(err)
	}
	return store, vault
}

func TestUpdateMeta_TriagePersistsToSidecar(t *testing.T) {
	store, vault := freshStore(t, map[string]string{
		"Tasks.md": "- [ ] triage me\n",
	})
	id := store.All()[0].ID

	if err := store.Triage(id, TriageScheduled); err != nil {
		t.Fatal(err)
	}

	// In memory
	got, _ := store.GetByID(id)
	if got.Triage != TriageScheduled {
		t.Errorf("in-memory: got %q want scheduled", got.Triage)
	}
	if got.LastTriagedAt == nil {
		t.Errorf("LastTriagedAt should be set")
	}

	// On disk
	data, _ := os.ReadFile(filepath.Join(vault, ".granit", "tasks-meta.json"))
	if !contains(string(data), `"triage": "scheduled"`) {
		t.Errorf("sidecar missing triage state:\n%s", data)
	}
}

func TestUpdateMeta_DoesNotTouchMarkdown(t *testing.T) {
	original := "- [ ] keep me intact\n"
	store, vault := freshStore(t, map[string]string{"Tasks.md": original})
	id := store.All()[0].ID

	if err := store.Triage(id, TriageScheduled); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if string(got) != original {
		t.Errorf("markdown should be unchanged after triage, got %q", got)
	}
}

func TestSchedule_SetsScheduledStartAndDuration(t *testing.T) {
	store, _ := freshStore(t, map[string]string{"Tasks.md": "- [ ] schedule me\n"})
	id := store.All()[0].ID
	when := time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC)
	if err := store.Schedule(id, when, 90*time.Minute); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetByID(id)
	if got.ScheduledStart == nil || !got.ScheduledStart.Equal(when) {
		t.Errorf("ScheduledStart: got %v want %v", got.ScheduledStart, when)
	}
	if got.Duration != 90*time.Minute {
		t.Errorf("Duration: got %v want 90m", got.Duration)
	}
}

func TestUpdateLine_FlipsCheckboxAndSyncsDone(t *testing.T) {
	store, vault := freshStore(t, map[string]string{
		"Tasks.md": "- [ ] complete me\n",
	})
	id := store.All()[0].ID

	if err := store.UpdateLine(id, toggleCheckbox(true)); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetByID(id)
	if !got.Done {
		t.Error("Done should be true after toggle")
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set when Done flips true")
	}

	disk, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if !contains(string(disk), "- [x] complete me") {
		t.Errorf("markdown not updated: %q", disk)
	}
}

func TestComplete_SetsDoneAndTriageDone(t *testing.T) {
	store, _ := freshStore(t, map[string]string{
		"Tasks.md": "- [ ] finish writing this test\n",
	})
	id := store.All()[0].ID

	if err := store.Complete(id); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetByID(id)
	if !got.Done {
		t.Error("Done should be true")
	}
	if got.Triage != TriageDone {
		t.Errorf("Triage: got %q want done", got.Triage)
	}
}

func TestUpdateLine_PreservesSidecarMetadata(t *testing.T) {
	store, _ := freshStore(t, map[string]string{
		"Tasks.md": "- [ ] keep my triage\n",
	})
	id := store.All()[0].ID
	scheduled := time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC)
	if err := store.Schedule(id, scheduled, 60*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Triage(id, TriageScheduled); err != nil {
		t.Fatal(err)
	}
	// Now edit the markdown line.
	if err := store.UpdateLine(id, func(line string) string {
		return line + " 📅 2026-04-30"
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetByID(id)
	if got.Triage != TriageScheduled {
		t.Errorf("Triage lost across UpdateLine: %q", got.Triage)
	}
	if got.ScheduledStart == nil || !got.ScheduledStart.Equal(scheduled) {
		t.Errorf("ScheduledStart lost: %v", got.ScheduledStart)
	}
	if got.DueDate != "2026-04-30" {
		t.Errorf("DueDate not picked up from markdown edit: %q", got.DueDate)
	}
}

func TestUpdateLine_DeletingCheckboxRemovesTask(t *testing.T) {
	store, _ := freshStore(t, map[string]string{
		"Tasks.md": "- [ ] gone soon\n",
	})
	id := store.All()[0].ID

	// Transform that strips the checkbox so the line no longer
	// parses as a task.
	if err := store.UpdateLine(id, func(line string) string {
		return "just plain text"
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.GetByID(id); ok {
		t.Error("task should be gone after checkbox stripped")
	}
}

func TestCreate_AppendsToTasksMd(t *testing.T) {
	store, vault := freshStore(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] existing\n",
	})

	got, err := store.Create("brand new", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" {
		t.Error("Create should return task with ID set")
	}
	if got.Text != "brand new" {
		t.Errorf("Text: got %q want 'brand new'", got.Text)
	}
	if got.Triage != TriageInbox {
		t.Errorf("Triage: got %q want inbox", got.Triage)
	}
	if got.Origin != OriginManual {
		t.Errorf("Origin: got %q want manual", got.Origin)
	}

	disk, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if !contains(string(disk), "- [ ] brand new") {
		t.Errorf("markdown missing new task: %q", disk)
	}
	if !contains(string(disk), "- [ ] existing") {
		t.Errorf("markdown lost existing task: %q", disk)
	}

	// Store should know about both tasks now.
	if len(store.All()) != 2 {
		t.Errorf("store has %d tasks, want 2", len(store.All()))
	}
}

func TestCreate_SeedsTasksMdHeader(t *testing.T) {
	store, vault := freshStore(t, map[string]string{}) // empty vault
	if _, err := store.Create("first task", CreateOpts{}); err != nil {
		t.Fatal(err)
	}
	disk, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if !contains(string(disk), "# Tasks") {
		t.Errorf("missing # Tasks header in fresh file: %q", disk)
	}
	if !contains(string(disk), "- [ ] first task") {
		t.Errorf("missing the new task: %q", disk)
	}
}

func TestCreate_RespectsCustomFile(t *testing.T) {
	store, vault := freshStore(t, map[string]string{
		"Tasks.md":      "- [ ] root\n",
		"Projects/A.md": "# Project A\n",
	})
	got, err := store.Create("project task", CreateOpts{
		File:      "Projects/A.md",
		Origin:    OriginProjectImport,
		ProjectID: "proj_a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.NotePath != "Projects/A.md" {
		t.Errorf("NotePath: got %q", got.NotePath)
	}
	if got.Origin != OriginProjectImport {
		t.Errorf("Origin: got %q want project_import", got.Origin)
	}
	if got.ProjectID != "proj_a" {
		t.Errorf("ProjectID: got %q", got.ProjectID)
	}
	disk, _ := os.ReadFile(filepath.Join(vault, "Projects/A.md"))
	if !contains(string(disk), "- [ ] project task") {
		t.Errorf("project file missing task: %q", disk)
	}
}

func TestCreate_RejectsEmptyText(t *testing.T) {
	store, _ := freshStore(t, nil)
	if _, err := store.Create("   ", CreateOpts{}); err == nil {
		t.Error("expected error on whitespace-only text")
	}
}

func TestDelete_RemovesLineAndTombstones(t *testing.T) {
	store, vault := freshStore(t, map[string]string{
		"Tasks.md": "- [ ] alpha\n- [ ] beta\n- [ ] gamma\n",
	})
	betaID := ""
	for _, task := range store.All() {
		if task.Text == "beta" {
			betaID = task.ID
		}
	}
	if betaID == "" {
		t.Fatal("beta task missing")
	}

	if err := store.Delete(betaID); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.GetByID(betaID); ok {
		t.Error("beta should be gone from store")
	}
	disk, _ := os.ReadFile(filepath.Join(vault, "Tasks.md"))
	if contains(string(disk), "- [ ] beta") {
		t.Errorf("beta still in markdown: %q", disk)
	}
	if !contains(string(disk), "- [ ] alpha") || !contains(string(disk), "- [ ] gamma") {
		t.Errorf("siblings damaged: %q", disk)
	}

	// Tombstone exists so a re-add via git pull would revive the ID.
	side, _ := loadSidecar(SidecarPath(vault))
	found := false
	for _, tomb := range side.Tombstones {
		if tomb.ID == betaID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tombstone for %q, got %+v", betaID, side.Tombstones)
	}
}

func TestUpdateLine_UnknownIDReturnsErrNotFound(t *testing.T) {
	store, _ := freshStore(t, map[string]string{"Tasks.md": "- [ ] x\n"})
	err := store.UpdateLine("never-existed", toggleCheckbox(true))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete_UnknownIDReturnsErrNotFound(t *testing.T) {
	store, _ := freshStore(t, map[string]string{"Tasks.md": "- [ ] x\n"})
	if err := store.Delete("never-existed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && stringsIndex(haystack, needle) >= 0
}

// avoid importing strings just for this — cheap manual loop
func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
