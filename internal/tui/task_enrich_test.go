package tui

import (
	"testing"

	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/vault"
)

func mkModel() *Model {
	v := &vault.Vault{Notes: map[string]*vault.Note{}}
	return &Model{vault: v}
}

func addNote(v *vault.Vault, path, content string, fm map[string]interface{}) {
	v.Notes[path] = &vault.Note{
		Path: path, Content: content, Frontmatter: fm,
	}
}

func TestEnrichTasksWithProjects_TypedProjectNote(t *testing.T) {
	m := mkModel()
	addNote(m.vault, "Projects/Apollo.md", "tasks here",
		map[string]interface{}{"type": "project", "title": "Apollo Mission"})

	// Build the typed-objects index from that note.
	reg := objects.NewRegistry()
	b := objects.NewBuilder(reg)
	b.Add("Projects/Apollo.md", "Apollo Mission", map[string]string{
		"type": "project", "title": "Apollo Mission",
	})
	m.objectsIndex = b.Finalize()

	tasks := []Task{
		{Text: "Build the rocket", NotePath: "Projects/Apollo.md"},
		{Text: "Standalone task", NotePath: "Inbox.md"},
	}

	out := m.enrichTasksWithProjects(tasks)
	if out[0].Project != "Apollo Mission" {
		t.Errorf("typed-project note didn't populate Project: got %q", out[0].Project)
	}
	if out[1].Project != "" {
		t.Errorf("non-project note shouldn't get a Project: got %q", out[1].Project)
	}
}

func TestEnrichTasksWithProjects_FrontmatterProjectKey(t *testing.T) {
	m := mkModel()
	addNote(m.vault, "Inbox.md", "tasks here",
		map[string]interface{}{"project": "Q3 Roadmap"})
	m.objectsIndex = objects.NewIndex()

	tasks := []Task{{Text: "Plan something", NotePath: "Inbox.md"}}
	out := m.enrichTasksWithProjects(tasks)
	if out[0].Project != "Q3 Roadmap" {
		t.Errorf("project: frontmatter didn't populate: got %q", out[0].Project)
	}
}

func TestEnrichTasksWithProjects_DoesNotOverwriteExplicit(t *testing.T) {
	m := mkModel()
	addNote(m.vault, "Inbox.md", "tasks",
		map[string]interface{}{"project": "From Frontmatter"})
	m.objectsIndex = objects.NewIndex()

	tasks := []Task{
		{Text: "x", NotePath: "Inbox.md", Project: "Already Set"},
	}
	out := m.enrichTasksWithProjects(tasks)
	if out[0].Project != "Already Set" {
		t.Errorf("enrichment overwrote explicit Project: got %q", out[0].Project)
	}
}

func TestEnrichTasksWithProjects_NoIndexNoCrash(t *testing.T) {
	m := mkModel()
	addNote(m.vault, "Inbox.md", "x", nil)
	tasks := []Task{{Text: "x", NotePath: "Inbox.md"}}
	out := m.enrichTasksWithProjects(tasks)
	if len(out) != 1 || out[0].Project != "" {
		t.Errorf("no-index path should pass through cleanly: %+v", out)
	}
}
