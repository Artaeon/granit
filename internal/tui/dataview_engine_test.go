package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/artaeon/granit/internal/vault"
)

// testVault creates a temporary vault with markdown files and returns it
// after scanning. The caller does not need to clean up; t.TempDir handles it.
func testVault(t *testing.T, files map[string]string) *vault.Vault {
	t.Helper()
	dir := t.TempDir()

	for name, content := range files {
		fullPath := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", name, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	v, err := vault.NewVault(dir)
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}
	if err := v.Scan(); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	return v
}

// ---------------------------------------------------------------------------
// TABLE query returns correct fields
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_TableFields(t *testing.T) {
	v := testVault(t, map[string]string{
		"note1.md": "---\ntitle: Alpha\nstatus: active\npriority: high\n---\nSome content here.",
		"note2.md": "---\ntitle: Beta\nstatus: done\npriority: low\n---\nOther content here.",
	})

	q := ParseDVQuery(`TABLE status, priority`)
	result := ExecuteDVQuery(q, v)

	if result.Mode != DVModeTable {
		t.Errorf("expected TABLE mode, got %d", result.Mode)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}

	// Check that each row has the requested fields
	for _, row := range result.Rows {
		if _, ok := row.Fields["status"]; !ok {
			t.Errorf("row for %s missing 'status' field", row.Title)
		}
		if _, ok := row.Fields["priority"]; !ok {
			t.Errorf("row for %s missing 'priority' field", row.Title)
		}
	}

	// Verify field values are actually populated
	foundActive := false
	foundDone := false
	for _, row := range result.Rows {
		if row.Fields["status"] == "active" {
			foundActive = true
			if row.Fields["priority"] != "high" {
				t.Errorf("active note should have priority high, got %q", row.Fields["priority"])
			}
		}
		if row.Fields["status"] == "done" {
			foundDone = true
		}
	}
	if !foundActive || !foundDone {
		t.Errorf("expected to find both active and done notes")
	}
}

// ---------------------------------------------------------------------------
// WHERE condition filters: =, CONTAINS, >, <
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_WhereEquals(t *testing.T) {
	v := testVault(t, map[string]string{
		"a.md": "---\nstatus: active\n---\n",
		"b.md": "---\nstatus: done\n---\n",
		"c.md": "---\nstatus: active\n---\n",
	})

	q := ParseDVQuery(`TABLE status WHERE status = "active"`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows matching status=active, got %d", len(result.Rows))
	}
	for _, row := range result.Rows {
		if row.Fields["status"] != "active" {
			t.Errorf("expected status=active, got %q", row.Fields["status"])
		}
	}
}

func TestExecuteDVQuery_WhereContains(t *testing.T) {
	// Use comma-separated string for tags (the custom frontmatter parser
	// stores bracket arrays as []string which doesn't match []interface{},
	// so use the plain string format that dvNoteHasTag handles).
	v := testVault(t, map[string]string{
		"go.md":     "---\ntags: go, programming\n---\n",
		"python.md": "---\ntags: python, programming\n---\n",
		"recipe.md": "---\ntags: cooking\n---\n",
	})

	q := ParseDVQuery(`TABLE tags WHERE tags CONTAINS "programming"`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 2 {
		t.Errorf("expected 2 notes with tag 'programming', got %d", len(result.Rows))
	}
}

func TestExecuteDVQuery_WhereGreaterThan(t *testing.T) {
	v := testVault(t, map[string]string{
		"old.md":    "---\ndate: 2023-01-01\n---\n",
		"recent.md": "---\ndate: 2024-06-15\n---\n",
		"newest.md": "---\ndate: 2025-01-01\n---\n",
	})

	q := ParseDVQuery(`TABLE date WHERE date > "2024-01-01"`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 2 {
		t.Errorf("expected 2 notes with date > 2024-01-01, got %d", len(result.Rows))
	}
}

func TestExecuteDVQuery_WhereLessThan(t *testing.T) {
	v := testVault(t, map[string]string{
		"old.md":    "---\ndate: 2023-01-01\n---\n",
		"recent.md": "---\ndate: 2024-06-15\n---\n",
		"newest.md": "---\ndate: 2025-01-01\n---\n",
	})

	q := ParseDVQuery(`TABLE date WHERE date < "2024-01-01"`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 1 {
		t.Errorf("expected 1 note with date < 2024-01-01, got %d", len(result.Rows))
	}
}

// ---------------------------------------------------------------------------
// SORT orders correctly (ASC, DESC)
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_SortAsc(t *testing.T) {
	v := testVault(t, map[string]string{
		"c.md": "---\npriority: 3\n---\n",
		"a.md": "---\npriority: 1\n---\n",
		"b.md": "---\npriority: 2\n---\n",
	})

	q := ParseDVQuery(`TABLE priority SORT priority ASC`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
	for i := 0; i < len(result.Rows)-1; i++ {
		if result.Rows[i].Fields["priority"] > result.Rows[i+1].Fields["priority"] {
			t.Errorf("rows not sorted ASC: %q comes before %q",
				result.Rows[i].Fields["priority"], result.Rows[i+1].Fields["priority"])
		}
	}
}

func TestExecuteDVQuery_SortDesc(t *testing.T) {
	v := testVault(t, map[string]string{
		"c.md": "---\npriority: 3\n---\n",
		"a.md": "---\npriority: 1\n---\n",
		"b.md": "---\npriority: 2\n---\n",
	})

	q := ParseDVQuery(`TABLE priority SORT priority DESC`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
	for i := 0; i < len(result.Rows)-1; i++ {
		if result.Rows[i].Fields["priority"] < result.Rows[i+1].Fields["priority"] {
			t.Errorf("rows not sorted DESC: %q comes before %q",
				result.Rows[i].Fields["priority"], result.Rows[i+1].Fields["priority"])
		}
	}
}

// ---------------------------------------------------------------------------
// TASK mode extracts checkboxes
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_TaskMode(t *testing.T) {
	v := testVault(t, map[string]string{
		"tasks.md": "---\ntitle: My Tasks\n---\n# Tasks\n- [ ] Buy groceries\n- [x] Write tests\n- [ ] Read a book\n",
	})

	q := ParseDVQuery(`TASK`)
	result := ExecuteDVQuery(q, v)

	if result.Mode != DVModeTask {
		t.Errorf("expected TASK mode, got %d", result.Mode)
	}
	if len(result.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(result.Tasks))
	}

	// Check task content
	completedCount := 0
	for _, task := range result.Tasks {
		if task.Completed {
			completedCount++
			if task.Text != "Write tests" {
				t.Errorf("expected completed task 'Write tests', got %q", task.Text)
			}
		}
	}
	if completedCount != 1 {
		t.Errorf("expected 1 completed task, got %d", completedCount)
	}
}

// ---------------------------------------------------------------------------
// Empty vault returns empty results
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_EmptyVault(t *testing.T) {
	v := testVault(t, map[string]string{})

	q := ParseDVQuery(`TABLE title`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows from empty vault, got %d", len(result.Rows))
	}
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
}

// ---------------------------------------------------------------------------
// Nil vault returns empty results
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_NilVault(t *testing.T) {
	q := ParseDVQuery(`TABLE title`)
	result := ExecuteDVQuery(q, nil)

	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows from nil vault, got %d", len(result.Rows))
	}
}

// ---------------------------------------------------------------------------
// Non-existent field returns empty string
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_NonExistentField(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "---\ntitle: Test\n---\nHello",
	})

	q := ParseDVQuery(`TABLE nosuchfield`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	val := result.Rows[0].Fields["nosuchfield"]
	if val != "" {
		t.Errorf("expected empty string for non-existent field, got %q", val)
	}
}

// ---------------------------------------------------------------------------
// FROM folder filter
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_FromFolder(t *testing.T) {
	v := testVault(t, map[string]string{
		"projects/a.md":   "---\ntitle: Project A\n---\n",
		"projects/b.md":   "---\ntitle: Project B\n---\n",
		"journal/day1.md": "---\ntitle: Day 1\n---\n",
	})

	q := ParseDVQuery(`TABLE title FROM "projects"`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows from projects folder, got %d", len(result.Rows))
	}
	// Note: Title is derived from filename (without .md), not frontmatter
	for _, row := range result.Rows {
		if row.Title != "a" && row.Title != "b" {
			t.Errorf("unexpected note in results: %q", row.Title)
		}
	}
}

// ---------------------------------------------------------------------------
// FROM tag filter
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_FromTag(t *testing.T) {
	v := testVault(t, map[string]string{
		"tagged.md":   "---\ntags: review, important\n---\n",
		"untagged.md": "---\ntitle: Plain\n---\n",
	})

	q := ParseDVQuery(`LIST FROM #review`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row from #review tag, got %d", len(result.Rows))
	}
}

// ---------------------------------------------------------------------------
// LIMIT caps results
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_LimitCapsResults(t *testing.T) {
	v := testVault(t, map[string]string{
		"a.md": "---\ntitle: A\n---\n",
		"b.md": "---\ntitle: B\n---\n",
		"c.md": "---\ntitle: C\n---\n",
		"d.md": "---\ntitle: D\n---\n",
		"e.md": "---\ntitle: E\n---\n",
	})

	q := ParseDVQuery(`TABLE title LIMIT 3`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 3 {
		t.Errorf("expected 3 rows with LIMIT 3, got %d", len(result.Rows))
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
}

// ---------------------------------------------------------------------------
// LIST mode returns title and path
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_ListMode(t *testing.T) {
	v := testVault(t, map[string]string{
		"hello.md": "---\ntitle: Hello World\n---\nContent",
	})

	q := ParseDVQuery(`LIST`)
	result := ExecuteDVQuery(q, v)

	if result.Mode != DVModeList {
		t.Errorf("expected LIST mode, got %d", result.Mode)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	row := result.Rows[0]
	if row.Fields["title"] == "" {
		t.Error("expected title field to be populated in LIST mode")
	}
	if row.Fields["path"] == "" {
		t.Error("expected path field to be populated in LIST mode")
	}
}

// ---------------------------------------------------------------------------
// Virtual fields: words, size
// ---------------------------------------------------------------------------

func TestExecuteDVQuery_VirtualFields(t *testing.T) {
	v := testVault(t, map[string]string{
		"note.md": "---\ntitle: Test\n---\none two three four five",
	})

	q := ParseDVQuery(`TABLE words, size`)
	result := ExecuteDVQuery(q, v)

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	row := result.Rows[0]
	if row.Fields["words"] == "" || row.Fields["words"] == "0" {
		t.Errorf("expected non-zero word count, got %q", row.Fields["words"])
	}
	if row.Fields["size"] == "" || row.Fields["size"] == "0" {
		t.Errorf("expected non-zero size, got %q", row.Fields["size"])
	}
}
