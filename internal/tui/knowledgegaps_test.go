package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Helpers: create temporary vaults with markdown files
// ---------------------------------------------------------------------------

// kgTestFile describes a markdown file to create in a temp vault.
type kgTestFile struct {
	relPath string
	content string
	modTime time.Time // zero means use current time
}

// kgCreateVault creates a temp directory with the given markdown files and
// returns the vault root path.
func kgCreateVault(t *testing.T, files []kgTestFile) string {
	t.Helper()
	root := t.TempDir()
	now := time.Now()

	for _, f := range files {
		fullPath := filepath.Join(root, f.relPath)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", fullPath, err)
		}
		modTime := f.modTime
		if modTime.IsZero() {
			modTime = now
		}
		if err := os.Chtimes(fullPath, modTime, modTime); err != nil {
			t.Fatalf("failed to set mtime for %s: %v", fullPath, err)
		}
	}
	return root
}

// ---------------------------------------------------------------------------
// 1. Initialization
// ---------------------------------------------------------------------------

func TestNewKnowledgeGaps(t *testing.T) {
	kg := NewKnowledgeGaps()

	if kg.IsActive() {
		t.Error("new KnowledgeGaps should not be active")
	}
	if kg.tab != 0 {
		t.Errorf("expected initial tab 0, got %d", kg.tab)
	}
	if kg.cursor != 0 {
		t.Errorf("expected initial cursor 0, got %d", kg.cursor)
	}
	if kg.wantJump {
		t.Error("wantJump should be false initially")
	}
}

func TestKnowledgeGaps_Open(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# Hello\nSome content here with machine learning topics."},
		{relPath: "note2.md", content: "# World\nMore content about machine learning and neural networks."},
		{relPath: "note3.md", content: "# Cooking\nRecipes for italian pasta and tomato sauce."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	if !kg.IsActive() {
		t.Error("expected KnowledgeGaps to be active after Open")
	}
	if kg.vaultRoot != root {
		t.Errorf("expected vaultRoot %q, got %q", root, kg.vaultRoot)
	}
	if kg.tab != 0 {
		t.Errorf("expected tab reset to 0, got %d", kg.tab)
	}
	if kg.cursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", kg.cursor)
	}
	if kg.scroll != 0 {
		t.Errorf("expected scroll reset to 0, got %d", kg.scroll)
	}
}

func TestKnowledgeGaps_SetSize(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.SetSize(120, 40)

	if kg.width != 120 {
		t.Errorf("expected width 120, got %d", kg.width)
	}
	if kg.height != 40 {
		t.Errorf("expected height 40, got %d", kg.height)
	}
}

func TestKnowledgeGaps_GetSelectedNote(t *testing.T) {
	kg := NewKnowledgeGaps()

	// No selection initially
	path, ok := kg.GetSelectedNote()
	if ok {
		t.Error("expected no selection initially")
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}

	// Set a selection
	kg.selectedNote = "test/note.md"
	kg.wantJump = true

	path, ok = kg.GetSelectedNote()
	if !ok {
		t.Error("expected selection to be available")
	}
	if path != "test/note.md" {
		t.Errorf("expected path %q, got %q", "test/note.md", path)
	}

	// Should be consumed
	path2, ok2 := kg.GetSelectedNote()
	if ok2 {
		t.Error("selection should be consumed after first read")
	}
	if path2 != "" {
		t.Errorf("expected empty path after consumption, got %q", path2)
	}
}

// ---------------------------------------------------------------------------
// 2. Topic Coverage Analysis (TF-IDF based)
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_AnalyzeTopics_OrphanTag(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# ML\nMachine learning #ai #data-science"},
		{relPath: "note2.md", content: "# DL\nDeep learning #ai"},
		{relPath: "note3.md", content: "# Cooking\nPasta recipe #cooking"},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	// #cooking appears in only 1 note => orphan tag finding
	// #data-science appears in only 1 note => orphan tag finding
	foundCooking := false
	foundDataScience := false
	for _, f := range kg.topicFindings {
		if strings.Contains(f.Title, "cooking") {
			foundCooking = true
			if f.Severity != 1 {
				t.Errorf("orphan tag should have severity 1, got %d", f.Severity)
			}
		}
		if strings.Contains(f.Title, "data-science") {
			foundDataScience = true
		}
	}

	if !foundCooking {
		t.Error("expected orphan tag finding for #cooking")
	}
	if !foundDataScience {
		t.Error("expected orphan tag finding for #data-science")
	}
}

func TestKnowledgeGaps_AnalyzeTopics_NoOrphanForSharedTag(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# ML\nMachine learning #shared-tag"},
		{relPath: "note2.md", content: "# DL\nDeep learning #shared-tag"},
		{relPath: "note3.md", content: "# AI\nArtificial intelligence #shared-tag"},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.topicFindings {
		if strings.Contains(f.Title, "shared-tag") {
			t.Error("shared tag (used 3 times) should not be an orphan tag finding")
		}
	}
}

func TestKnowledgeGaps_AnalyzeTopics_SortedBySeverity(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# ML\nMachine learning #ai #unique-tag1"},
		{relPath: "note2.md", content: "# DL\nDeep learning #ai #unique-tag2"},
		{relPath: "note3.md", content: "# NLP\nNatural language processing #unique-tag3"},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for i := 1; i < len(kg.topicFindings); i++ {
		if kg.topicFindings[i].Severity > kg.topicFindings[i-1].Severity {
			t.Errorf("topic findings not sorted by severity descending: [%d]=%d > [%d]=%d",
				i, kg.topicFindings[i].Severity, i-1, kg.topicFindings[i-1].Severity)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Stale Knowledge Detection (file modification time based)
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_AnalyzeStale_RecentNotesNotStale(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "recent1.md", content: "# Recent\nThis is recent content."},
		{relPath: "recent2.md", content: "# Also Recent\nAnother recent note."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	if len(kg.staleFindings) != 0 {
		t.Errorf("expected 0 stale findings for recent notes, got %d", len(kg.staleFindings))
	}
}

func TestKnowledgeGaps_AnalyzeStale_OldNoteWithBacklinks(t *testing.T) {
	oldTime := time.Now().Add(-120 * 24 * time.Hour) // 120 days ago

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "hub.md", content: "# Hub\nImportant hub note.", modTime: oldTime},
		{relPath: "note1.md", content: "# Note1\nLinks to [[hub]]."},
		{relPath: "note2.md", content: "# Note2\nAlso links to [[hub]]."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundStaleHub := false
	for _, f := range kg.staleFindings {
		if strings.Contains(f.Title, "hub") && f.Severity == 3 {
			foundStaleHub = true
			if !strings.Contains(f.Description, "incoming links") {
				t.Error("stale hub finding should mention incoming links")
			}
		}
	}

	if !foundStaleHub {
		t.Error("expected high-severity stale hub finding for old note with backlinks")
	}
}

func TestKnowledgeGaps_AnalyzeStale_OldNoteWithTodo(t *testing.T) {
	oldTime := time.Now().Add(-100 * 24 * time.Hour) // 100 days ago

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "todo-note.md", content: "# Tasks\n- [ ] TODO: finish this", modTime: oldTime},
		{relPath: "other.md", content: "# Other\nJust a filler note."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundStaleTodo := false
	for _, f := range kg.staleFindings {
		if strings.Contains(f.Title, "todo-note") && f.Severity == 2 {
			foundStaleTodo = true
			if !strings.Contains(f.Description, "TODO") {
				t.Error("stale TODO finding should mention TODO markers")
			}
		}
	}

	if !foundStaleTodo {
		t.Error("expected medium-severity stale TODO finding for old note with TODOs")
	}
}

func TestKnowledgeGaps_AnalyzeStale_VeryOldNote(t *testing.T) {
	veryOldTime := time.Now().Add(-200 * 24 * time.Hour) // 200 days ago

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "ancient.md", content: "# Ancient\nVery old content from long ago.", modTime: veryOldTime},
		{relPath: "other.md", content: "# Other\nFiller note."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundVeryOld := false
	for _, f := range kg.staleFindings {
		if strings.Contains(f.Title, "ancient") && f.Severity == 1 {
			foundVeryOld = true
		}
	}

	if !foundVeryOld {
		t.Error("expected low-severity finding for very old note (>180 days)")
	}
}

func TestKnowledgeGaps_AnalyzeStale_SortedBySeverity(t *testing.T) {
	veryOldTime := time.Now().Add(-200 * 24 * time.Hour)
	oldTime := time.Now().Add(-120 * 24 * time.Hour)

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "hub.md", content: "# Hub\nHub note.", modTime: oldTime},
		{relPath: "note1.md", content: "# Note1\nLinks to [[hub]]."},
		{relPath: "note2.md", content: "# Note2\nAlso links to [[hub]]."},
		{relPath: "ancient.md", content: "# Ancient\nVery old.", modTime: veryOldTime},
		{relPath: "todo.md", content: "# Todo\n- [ ] Fix thing.", modTime: oldTime},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for i := 1; i < len(kg.staleFindings); i++ {
		if kg.staleFindings[i].Severity > kg.staleFindings[i-1].Severity {
			t.Errorf("stale findings not sorted by severity: [%d]=%d > [%d]=%d",
				i, kg.staleFindings[i].Severity, i-1, kg.staleFindings[i-1].Severity)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Missing Links Detection (potential wikilinks not yet created)
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_AnalyzeMissingLinks_SimilarUnlinked(t *testing.T) {
	// Create two very similar notes that don't link to each other
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "ml-basics.md", content: "# Machine Learning\nMachine learning algorithms neural networks deep learning training models optimization gradient descent backpropagation"},
		{relPath: "ml-advanced.md", content: "# Advanced ML\nMachine learning algorithms neural networks deep learning advanced models training optimization gradient"},
		{relPath: "cooking.md", content: "# Cooking\nRecipes pasta italian food tomato sauce garlic olive oil basil oregano"},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	// The two ML notes should appear as a missing link candidate
	foundMLPair := false
	for _, f := range kg.missingLinkFindings {
		if (strings.Contains(f.Title, "ml-basics") || strings.Contains(f.Title, "Machine Learning")) &&
			(strings.Contains(f.Title, "ml-advanced") || strings.Contains(f.Title, "Advanced ML")) {
			foundMLPair = true
			if len(f.Keywords) == 0 {
				t.Error("missing link finding should have shared keywords")
			}
		}
	}

	if !foundMLPair {
		t.Logf("Missing link findings: %+v", kg.missingLinkFindings)
		t.Error("expected missing link finding between the two similar ML notes")
	}
}

func TestKnowledgeGaps_AnalyzeMissingLinks_AlreadyLinked(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "ml-basics.md", content: "# Machine Learning\nMachine learning algorithms neural networks deep learning. See also [[ml-advanced]]."},
		{relPath: "ml-advanced.md", content: "# Advanced ML\nMachine learning algorithms neural networks deep learning advanced models."},
		{relPath: "filler.md", content: "# Filler\nSome unrelated filler content here."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	// Already linked pair should not appear
	for _, f := range kg.missingLinkFindings {
		isBothML := (strings.Contains(f.NotePath, "ml-basics") && strings.Contains(f.NotePath2, "ml-advanced")) ||
			(strings.Contains(f.NotePath, "ml-advanced") && strings.Contains(f.NotePath2, "ml-basics"))
		if isBothML {
			t.Error("already linked notes should not appear in missing links findings")
		}
	}
}

func TestKnowledgeGaps_AnalyzeMissingLinks_SeverityLevels(t *testing.T) {
	kg := NewKnowledgeGaps()

	// Directly test that the severity assignment logic is correct
	// by checking findings that would be generated
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "a.md", content: "# A\nmachine learning algorithms neural networks deep learning training models optimization gradient descent backpropagation regularization epochs batch size learning rate"},
		{relPath: "b.md", content: "# B\nmachine learning algorithms neural networks deep learning training models optimization gradient descent backpropagation regularization epochs batch size learning rate"},
		{relPath: "c.md", content: "# C\nrecipes pasta italian cooking tomato sauce garlic bread olive oil"},
	})

	kg.Open(root)

	for _, f := range kg.missingLinkFindings {
		if f.Severity < 1 || f.Severity > 3 {
			t.Errorf("severity should be 1-3, got %d for %q", f.Severity, f.Title)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. Orphan Note Detection (notes with no incoming links)
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_AnalyzeOrphans_NoteWithNoLinks(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "connected1.md", content: "# Connected\nThis links to [[connected2]]."},
		{relPath: "connected2.md", content: "# Connected2\nThis links to [[connected1]]."},
		{relPath: "orphan.md", content: "# Orphan\nThis note has no links at all."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundOrphan := false
	for _, f := range kg.orphanFindings {
		if f.Title == "orphan" {
			foundOrphan = true
			if f.Type != "orphan" {
				t.Errorf("expected type 'orphan', got %q", f.Type)
			}
		}
	}

	if !foundOrphan {
		t.Logf("orphan findings: %+v", kg.orphanFindings)
		t.Error("expected orphan finding for note with no links")
	}
}

func TestKnowledgeGaps_AnalyzeOrphans_ShortOrphanHighSeverity(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "stub.md", content: "# Stub\nShort."},
		{relPath: "filler.md", content: "# Filler\nSome content."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.orphanFindings {
		if f.Title == "stub" {
			// Short orphan (< 100 words) should be severity 3
			if f.Severity != 3 {
				t.Errorf("short orphan should have severity 3, got %d", f.Severity)
			}
			return
		}
	}
	t.Error("expected orphan finding for stub note")
}

func TestKnowledgeGaps_AnalyzeOrphans_LongOrphanMediumSeverity(t *testing.T) {
	// Create a long orphan note (>100 words)
	longContent := "# Long Orphan\n" + strings.Repeat("word ", 150)

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "long-orphan.md", content: longContent},
		{relPath: "filler.md", content: "# Filler\nSome filler content."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.orphanFindings {
		if f.Title == "long-orphan" {
			if f.Severity != 2 {
				t.Errorf("long orphan (>100 words) should have severity 2, got %d", f.Severity)
			}
			return
		}
	}
	t.Logf("orphan findings: %+v", kg.orphanFindings)
	t.Error("expected orphan finding for long orphan note")
}

func TestKnowledgeGaps_AnalyzeOrphans_LinkedNoteNotOrphan(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "source.md", content: "# Source\nSee [[target]] for details."},
		{relPath: "target.md", content: "# Target\nThis is linked to."},
		{relPath: "filler.md", content: "# Filler\nSome content."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.orphanFindings {
		if f.Title == "source" {
			t.Error("source note (has outgoing links) should not be an orphan")
		}
		if f.Title == "target" {
			t.Error("target note (has incoming links) should not be an orphan")
		}
	}
}

func TestKnowledgeGaps_AnalyzeOrphans_SortedBySeverity(t *testing.T) {
	longContent := "# Long\n" + strings.Repeat("word ", 150)

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "stub1.md", content: "# Stub1\nShort."},
		{relPath: "stub2.md", content: "# Stub2\nAlso short."},
		{relPath: "long.md", content: longContent},
		{relPath: "filler.md", content: "# Filler\nSome content."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for i := 1; i < len(kg.orphanFindings); i++ {
		if kg.orphanFindings[i].Severity > kg.orphanFindings[i-1].Severity {
			t.Errorf("orphan findings not sorted by severity: [%d]=%d > [%d]=%d",
				i, kg.orphanFindings[i].Severity, i-1, kg.orphanFindings[i-1].Severity)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Structure Suggestions
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_AnalyzeStructure_LargeFolder(t *testing.T) {
	files := make([]kgTestFile, 22)
	for i := 0; i < 22; i++ {
		files[i] = kgTestFile{
			relPath: fmt.Sprintf("bigfolder/note%02d.md", i),
			content: fmt.Sprintf("# Note %d\nSome content for note %d.", i, i),
		}
	}

	root := kgCreateVault(t, files)
	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundLargeFolder := false
	for _, f := range kg.structureFindings {
		if strings.Contains(f.Title, "bigfolder") && strings.Contains(f.Title, "22 notes") {
			foundLargeFolder = true
			if f.Severity != 2 {
				t.Errorf("large folder finding should have severity 2, got %d", f.Severity)
			}
		}
	}

	if !foundLargeFolder {
		t.Error("expected structure finding for folder with >20 notes")
	}
}

func TestKnowledgeGaps_AnalyzeStructure_VaultRootDisplay(t *testing.T) {
	// Create >20 notes in vault root (folder = ".")
	files := make([]kgTestFile, 22)
	for i := 0; i < 22; i++ {
		files[i] = kgTestFile{
			relPath: fmt.Sprintf("note%02d.md", i),
			content: fmt.Sprintf("# Note %d\nContent.", i),
		}
	}

	root := kgCreateVault(t, files)
	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundVaultRoot := false
	for _, f := range kg.structureFindings {
		if strings.Contains(f.Title, "vault root") {
			foundVaultRoot = true
		}
	}

	if !foundVaultRoot {
		t.Error("expected 'vault root' display name for root folder with >20 notes")
	}
}

func TestKnowledgeGaps_AnalyzeStructure_LongNote(t *testing.T) {
	longContent := "# Very Long Note\n" + strings.Repeat("word ", 3100)

	root := kgCreateVault(t, []kgTestFile{
		{relPath: "long.md", content: longContent},
		{relPath: "short.md", content: "# Short\nBrief content."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundLongNote := false
	for _, f := range kg.structureFindings {
		if strings.Contains(f.Title, "Long note") || strings.Contains(f.Title, "Very Long Note") {
			foundLongNote = true
			if f.Severity != 2 {
				t.Errorf("long note finding should have severity 2, got %d", f.Severity)
			}
		}
	}

	if !foundLongNote {
		t.Error("expected structure finding for note with >3000 words")
	}
}

func TestKnowledgeGaps_AnalyzeStructure_RelatedDifferentFolders(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "folderA/note1.md", content: "# Note1\n#tag1 #tag2 #tag3 #tag4"},
		{relPath: "folderB/note2.md", content: "# Note2\n#tag1 #tag2 #tag3 #tag4"},
		{relPath: "filler.md", content: "# Filler\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	foundRelated := false
	for _, f := range kg.structureFindings {
		if strings.Contains(f.Title, "Related notes in different folders") {
			foundRelated = true
			if len(f.Keywords) < 3 {
				t.Errorf("expected at least 3 shared tags, got %d", len(f.Keywords))
			}
		}
	}

	if !foundRelated {
		t.Error("expected structure finding for related notes in different folders sharing 3+ tags")
	}
}

func TestKnowledgeGaps_AnalyzeStructure_SmallFolderNoFinding(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "folder/note1.md", content: "# Note1\nContent."},
		{relPath: "folder/note2.md", content: "# Note2\nContent."},
		{relPath: "filler.md", content: "# Filler\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.structureFindings {
		if strings.Contains(f.Title, "Large folder") {
			t.Error("folder with 2 notes should not be flagged as large")
		}
	}
}

// ---------------------------------------------------------------------------
// 7. Severity Icon Assignment
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_SeverityIcon(t *testing.T) {
	kg := NewKnowledgeGaps()

	tests := []struct {
		severity int
		wantChar string // The underlying character before styling
	}{
		{0, "○"},
		{1, "●"},
		{2, "●"},
		{3, "●"},
	}

	for _, tc := range tests {
		icon := kg.severityIcon(tc.severity)
		// The icon is styled with lipgloss, but should contain the expected character
		if !strings.Contains(icon, tc.wantChar) {
			t.Errorf("severityIcon(%d) = %q, want to contain %q", tc.severity, icon, tc.wantChar)
		}
	}
}

func TestKnowledgeGaps_SeverityIcon_Distinct(t *testing.T) {
	kg := NewKnowledgeGaps()

	icon0 := kg.severityIcon(0)
	icon1 := kg.severityIcon(1)

	// Info (0) uses "○", others use "●"
	if icon0 == icon1 {
		t.Error("severity 0 and 1 icons should be visually distinct")
	}
}

// ---------------------------------------------------------------------------
// 8. Tab Switching Between 5 Analysis Views
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_TabSwitching_Forward(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)
	kg.SetSize(120, 40)

	if kg.tab != 0 {
		t.Fatalf("expected initial tab 0, got %d", kg.tab)
	}

	// Tab forward through all 5 tabs
	for i := 1; i <= 5; i++ {
		kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyTab})
		expectedTab := i % 5
		if kg.tab != expectedTab {
			t.Errorf("after %d tab presses, expected tab %d, got %d", i, expectedTab, kg.tab)
		}
	}
}

func TestKnowledgeGaps_TabSwitching_Backward(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)
	kg.SetSize(120, 40)

	// Shift+tab from tab 0 should go to tab 4
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if kg.tab != 4 {
		t.Errorf("shift+tab from 0 should go to 4, got %d", kg.tab)
	}

	// Shift+tab again should go to tab 3
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if kg.tab != 3 {
		t.Errorf("shift+tab from 4 should go to 3, got %d", kg.tab)
	}
}

func TestKnowledgeGaps_TabSwitching_NumberKeys(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)
	kg.SetSize(120, 40)

	numberKeyTests := []struct {
		key     string
		wantTab int
	}{
		{"1", 0},
		{"2", 1},
		{"3", 2},
		{"4", 3},
		{"5", 4},
	}

	for _, tc := range numberKeyTests {
		kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		if kg.tab != tc.wantTab {
			t.Errorf("pressing %q should set tab to %d, got %d", tc.key, tc.wantTab, kg.tab)
		}
	}
}

func TestKnowledgeGaps_TabSwitching_ResetsCursorAndScroll(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)
	kg.SetSize(120, 40)

	// Manually set cursor and scroll to non-zero values
	kg.cursor = 5
	kg.scroll = 3

	// Switch tab
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyTab})

	if kg.cursor != 0 {
		t.Errorf("tab switch should reset cursor to 0, got %d", kg.cursor)
	}
	if kg.scroll != 0 {
		t.Errorf("tab switch should reset scroll to 0, got %d", kg.scroll)
	}
}

func TestKnowledgeGaps_CurrentFindings(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.topicFindings = []GapFinding{{Title: "topic"}}
	kg.staleFindings = []GapFinding{{Title: "stale"}}
	kg.missingLinkFindings = []GapFinding{{Title: "missing"}}
	kg.orphanFindings = []GapFinding{{Title: "orphan"}}
	kg.structureFindings = []GapFinding{{Title: "structure"}}

	tests := []struct {
		tab       int
		wantTitle string
	}{
		{0, "topic"},
		{1, "stale"},
		{2, "missing"},
		{3, "orphan"},
		{4, "structure"},
	}

	for _, tc := range tests {
		kg.tab = tc.tab
		findings := kg.currentFindings()
		if len(findings) != 1 {
			t.Errorf("tab %d: expected 1 finding, got %d", tc.tab, len(findings))
			continue
		}
		if findings[0].Title != tc.wantTitle {
			t.Errorf("tab %d: expected title %q, got %q", tc.tab, tc.wantTitle, findings[0].Title)
		}
	}

	// Invalid tab
	kg.tab = 99
	if findings := kg.currentFindings(); findings != nil {
		t.Error("invalid tab should return nil findings")
	}
}

func TestKnowledgeGaps_Update_EscCloses(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if kg.IsActive() {
		t.Error("esc should close the overlay")
	}
}

func TestKnowledgeGaps_Update_QCloses(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if kg.IsActive() {
		t.Error("q should close the overlay")
	}
}

func TestKnowledgeGaps_Update_InactiveNoOp(t *testing.T) {
	kg := NewKnowledgeGaps()
	// Not active - updates should be no-ops
	kgBefore := kg
	kg, cmd := kg.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		t.Error("inactive overlay should return nil cmd")
	}
	if kg.tab != kgBefore.tab {
		t.Error("inactive overlay should not change tab")
	}
}

func TestKnowledgeGaps_Update_CursorNavigation(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.active = true
	kg.SetSize(120, 40)
	kg.topicFindings = []GapFinding{
		{Title: "Finding 1"},
		{Title: "Finding 2"},
		{Title: "Finding 3"},
	}
	kg.tab = 0

	// Move down
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyDown})
	if kg.cursor != 1 {
		t.Errorf("down should move cursor to 1, got %d", kg.cursor)
	}

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyDown})
	if kg.cursor != 2 {
		t.Errorf("down should move cursor to 2, got %d", kg.cursor)
	}

	// Should not go past the end
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyDown})
	if kg.cursor != 2 {
		t.Errorf("cursor should not exceed list length, got %d", kg.cursor)
	}

	// Move up
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyUp})
	if kg.cursor != 1 {
		t.Errorf("up should move cursor to 1, got %d", kg.cursor)
	}

	// j/k also work
	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if kg.cursor != 2 {
		t.Errorf("j should move cursor down to 2, got %d", kg.cursor)
	}

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if kg.cursor != 1 {
		t.Errorf("k should move cursor up to 1, got %d", kg.cursor)
	}
}

func TestKnowledgeGaps_Update_CursorDoesNotGoBelowZero(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.active = true
	kg.SetSize(120, 40)
	kg.topicFindings = []GapFinding{{Title: "Only"}}
	kg.tab = 0
	kg.cursor = 0

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyUp})
	if kg.cursor != 0 {
		t.Errorf("cursor should not go below 0, got %d", kg.cursor)
	}
}

func TestKnowledgeGaps_Update_EnterSelectsNote(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.active = true
	kg.SetSize(120, 40)
	kg.orphanFindings = []GapFinding{
		{Title: "Orphan", NotePath: "orphan.md"},
	}
	kg.tab = 3

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyEnter})

	path, ok := kg.GetSelectedNote()
	if !ok {
		t.Error("enter should select a note")
	}
	if path != "orphan.md" {
		t.Errorf("expected selected note %q, got %q", "orphan.md", path)
	}
	if kg.IsActive() {
		t.Error("overlay should close after selecting a note")
	}
}

func TestKnowledgeGaps_Update_EnterNoopWithEmptyNotePath(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.active = true
	kg.SetSize(120, 40)
	kg.structureFindings = []GapFinding{
		{Title: "Large folder", NotePath: ""},
	}
	kg.tab = 4

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyEnter})

	_, ok := kg.GetSelectedNote()
	if ok {
		t.Error("enter should not select when NotePath is empty")
	}
	if !kg.IsActive() {
		t.Error("overlay should stay active when no note was selected")
	}
}

func TestKnowledgeGaps_Update_RefreshReanalysis(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# A\nContent."},
		{relPath: "note2.md", content: "# B\nContent."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)
	kg.SetSize(120, 40)
	kg.cursor = 5
	kg.scroll = 3

	kg, _ = kg.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	if kg.cursor != 0 {
		t.Errorf("refresh should reset cursor to 0, got %d", kg.cursor)
	}
	if kg.scroll != 0 {
		t.Errorf("refresh should reset scroll to 0, got %d", kg.scroll)
	}
}

// ---------------------------------------------------------------------------
// 9. Edge Cases
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_EmptyVault(t *testing.T) {
	root := kgCreateVault(t, nil)

	kg := NewKnowledgeGaps()
	kg.Open(root)

	if len(kg.topicFindings) != 0 {
		t.Errorf("empty vault: expected 0 topic findings, got %d", len(kg.topicFindings))
	}
	if len(kg.staleFindings) != 0 {
		t.Errorf("empty vault: expected 0 stale findings, got %d", len(kg.staleFindings))
	}
	if len(kg.missingLinkFindings) != 0 {
		t.Errorf("empty vault: expected 0 missing link findings, got %d", len(kg.missingLinkFindings))
	}
	if len(kg.orphanFindings) != 0 {
		t.Errorf("empty vault: expected 0 orphan findings, got %d", len(kg.orphanFindings))
	}
	if len(kg.structureFindings) != 0 {
		t.Errorf("empty vault: expected 0 structure findings, got %d", len(kg.structureFindings))
	}
}

func TestKnowledgeGaps_SingleNote(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "only.md", content: "# Only Note\nThis is the only note in the vault."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	// With < 2 notes, analyze() returns early with no findings
	totalFindings := len(kg.topicFindings) + len(kg.staleFindings) +
		len(kg.missingLinkFindings) + len(kg.orphanFindings) + len(kg.structureFindings)

	if totalFindings != 0 {
		t.Errorf("single note vault: expected 0 total findings, got %d", totalFindings)
	}
}

func TestKnowledgeGaps_AllNotesWellConnected(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# Note1\nSee [[note2]] and [[note3]]. #shared-tag"},
		{relPath: "note2.md", content: "# Note2\nSee [[note1]] and [[note3]]. #shared-tag"},
		{relPath: "note3.md", content: "# Note3\nSee [[note1]] and [[note2]]. #shared-tag"},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	// No orphans expected since all notes link to each other
	if len(kg.orphanFindings) != 0 {
		t.Errorf("fully connected vault: expected 0 orphan findings, got %d", len(kg.orphanFindings))
		for _, f := range kg.orphanFindings {
			t.Logf("  orphan: %s", f.Title)
		}
	}
}

func TestKnowledgeGaps_DotFoldersSkipped(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "visible.md", content: "# Visible\nNormal note."},
		{relPath: ".hidden/secret.md", content: "# Secret\nHidden note."},
		{relPath: "also-visible.md", content: "# Also Visible\nAnother note."},
	})

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.orphanFindings {
		if strings.Contains(f.Title, "Secret") || strings.Contains(f.NotePath, ".hidden") {
			t.Error("notes in dot-folders should be skipped")
		}
	}
}

func TestKnowledgeGaps_NonMdFilesSkipped(t *testing.T) {
	root := kgCreateVault(t, []kgTestFile{
		{relPath: "note1.md", content: "# Note1\nContent."},
		{relPath: "note2.md", content: "# Note2\nContent."},
	})
	// Also create a .txt file
	txtPath := filepath.Join(root, "readme.txt")
	_ = os.WriteFile(txtPath, []byte("This is not markdown"), 0o644)

	kg := NewKnowledgeGaps()
	kg.Open(root)

	for _, f := range kg.orphanFindings {
		if strings.Contains(f.Title, "readme") {
			t.Error("non-.md files should be skipped")
		}
	}
}

func TestKnowledgeGaps_VisibleHeight(t *testing.T) {
	kg := NewKnowledgeGaps()

	// Normal height
	kg.SetSize(120, 50)
	h := kg.visibleHeight()
	if h < 5 {
		t.Errorf("visibleHeight should be at least 5, got %d", h)
	}

	// Very small height should clamp to 5
	kg.SetSize(120, 10)
	h = kg.visibleHeight()
	if h != 5 {
		t.Errorf("very small height should clamp visibleHeight to 5, got %d", h)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestKgExtractTags(t *testing.T) {
	t.Run("inline tags", func(t *testing.T) {
		tags := kgExtractTags("Some text #golang #testing more text")
		found := make(map[string]bool)
		for _, tag := range tags {
			found[tag] = true
		}
		if !found["golang"] {
			t.Error("expected tag 'golang'")
		}
		if !found["testing"] {
			t.Error("expected tag 'testing'")
		}
	})

	t.Run("frontmatter tags", func(t *testing.T) {
		content := "---\ntags: [ml, ai, data]\n---\n# Note\nContent."
		tags := kgExtractTags(content)
		found := make(map[string]bool)
		for _, tag := range tags {
			found[tag] = true
		}
		for _, want := range []string{"ml", "ai", "data"} {
			if !found[want] {
				t.Errorf("expected frontmatter tag %q, got tags: %v", want, tags)
			}
		}
	})

	t.Run("skips false positives", func(t *testing.T) {
		tags := kgExtractTags("Some #heading and #a short")
		for _, tag := range tags {
			if tag == "heading" || tag == "a" {
				t.Errorf("should skip false positive tag %q", tag)
			}
		}
	})

	t.Run("sorted output", func(t *testing.T) {
		tags := kgExtractTags("#zebra #alpha #middle")
		for i := 1; i < len(tags); i++ {
			if tags[i] < tags[i-1] {
				t.Errorf("tags not sorted: %v", tags)
				break
			}
		}
	})
}

func TestKgExtractHeadings(t *testing.T) {
	content := "# Title\nSome text\n## Section\nMore text\n### Subsection"
	headings := kgExtractHeadings(content)

	if len(headings) != 3 {
		t.Fatalf("expected 3 headings, got %d: %v", len(headings), headings)
	}
	if headings[0] != "Title" {
		t.Errorf("expected heading 'Title', got %q", headings[0])
	}
	if headings[1] != "Section" {
		t.Errorf("expected heading 'Section', got %q", headings[1])
	}
	if headings[2] != "Subsection" {
		t.Errorf("expected heading 'Subsection', got %q", headings[2])
	}
}

func TestKgExtractWikilinks(t *testing.T) {
	t.Run("basic links", func(t *testing.T) {
		links := kgExtractWikilinks("See [[note1]] and [[note2]].")
		if len(links) != 2 {
			t.Fatalf("expected 2 links, got %d: %v", len(links), links)
		}
	})

	t.Run("aliased links", func(t *testing.T) {
		links := kgExtractWikilinks("See [[note1|display text]].")
		if len(links) != 1 {
			t.Fatalf("expected 1 link, got %d", len(links))
		}
		if links[0] != "note1" {
			t.Errorf("expected link target 'note1', got %q", links[0])
		}
	})

	t.Run("deduplicated", func(t *testing.T) {
		links := kgExtractWikilinks("See [[note1]] and also [[note1]].")
		if len(links) != 1 {
			t.Errorf("expected 1 unique link, got %d: %v", len(links), links)
		}
	})

	t.Run("no links", func(t *testing.T) {
		links := kgExtractWikilinks("No links here.")
		if len(links) != 0 {
			t.Errorf("expected 0 links, got %d", len(links))
		}
	})
}

func TestKgResolveLink(t *testing.T) {
	notes := []kgNoteInfo{
		{relPath: "folder/note1.md", name: "note1"},
		{relPath: "note2.md", name: "note2"},
	}

	t.Run("direct match", func(t *testing.T) {
		result := kgResolveLink("note2", notes)
		if result != "note2.md" {
			t.Errorf("expected 'note2.md', got %q", result)
		}
	})

	t.Run("basename match", func(t *testing.T) {
		result := kgResolveLink("note1", notes)
		if result != "folder/note1.md" {
			t.Errorf("expected 'folder/note1.md', got %q", result)
		}
	})

	t.Run("no match", func(t *testing.T) {
		result := kgResolveLink("nonexistent", notes)
		if result != "" {
			t.Errorf("expected empty string for no match, got %q", result)
		}
	})
}

func TestKgPairKey(t *testing.T) {
	// Order-independent
	key1 := kgPairKey("a.md", "b.md")
	key2 := kgPairKey("b.md", "a.md")
	if key1 != key2 {
		t.Errorf("pair key should be order-independent: %q != %q", key1, key2)
	}

	// Contains both paths
	if !strings.Contains(key1, "a.md") || !strings.Contains(key1, "b.md") {
		t.Errorf("pair key should contain both paths: %q", key1)
	}
}

func TestKgFindPathByName(t *testing.T) {
	notes := []kgNoteInfo{
		{relPath: "folder/note1.md", name: "note1"},
		{relPath: "note2.md", name: "note2"},
	}

	if path := kgFindPathByName(notes, "note1"); path != "folder/note1.md" {
		t.Errorf("expected 'folder/note1.md', got %q", path)
	}
	if path := kgFindPathByName(notes, "nonexistent"); path != "" {
		t.Errorf("expected empty for nonexistent, got %q", path)
	}
}

func TestKgSharedTags(t *testing.T) {
	a := []string{"go", "testing", "code"}
	b := []string{"testing", "code", "review"}

	shared := kgSharedTags(a, b)
	if len(shared) != 2 {
		t.Fatalf("expected 2 shared tags, got %d: %v", len(shared), shared)
	}

	sharedSet := make(map[string]bool)
	for _, s := range shared {
		sharedSet[s] = true
	}
	if !sharedSet["testing"] || !sharedSet["code"] {
		t.Errorf("expected shared tags [testing, code], got %v", shared)
	}
}

func TestKgSharedTags_NoOverlap(t *testing.T) {
	shared := kgSharedTags([]string{"a", "b"}, []string{"c", "d"})
	if len(shared) != 0 {
		t.Errorf("expected 0 shared tags, got %d: %v", len(shared), shared)
	}
}

func TestKgTopTerms(t *testing.T) {
	vec := map[string]float64{
		"machine":  0.5,
		"learning": 0.8,
		"deep":     0.3,
		"neural":   0.9,
	}

	top := kgTopTerms(vec, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2 top terms, got %d: %v", len(top), top)
	}
	// Top 2 by weight should be "neural" (0.9) and "learning" (0.8)
	if top[0] != "neural" {
		t.Errorf("expected top term 'neural', got %q", top[0])
	}
	if top[1] != "learning" {
		t.Errorf("expected second term 'learning', got %q", top[1])
	}
}

func TestKgTopTerms_FewerThanN(t *testing.T) {
	vec := map[string]float64{"only": 1.0}
	top := kgTopTerms(vec, 5)
	if len(top) != 1 {
		t.Errorf("expected 1 term when fewer exist, got %d", len(top))
	}
}

func TestKgTopTerms_ZeroWeightsSkipped(t *testing.T) {
	vec := map[string]float64{
		"zero": 0.0,
		"pos":  1.0,
	}
	top := kgTopTerms(vec, 5)
	for _, term := range top {
		if term == "zero" {
			t.Error("zero-weight terms should be excluded")
		}
	}
}

// ---------------------------------------------------------------------------
// View rendering smoke test
// ---------------------------------------------------------------------------

func TestKnowledgeGaps_View_SmokTest(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.active = true
	kg.SetSize(120, 40)
	kg.topicFindings = []GapFinding{
		{Type: "topic", Severity: 2, Title: "Test finding", Description: "Test description"},
	}

	view := kg.View()
	if view == "" {
		t.Error("View should produce non-empty output")
	}
	if !strings.Contains(view, "Knowledge Gaps") {
		t.Error("View should contain title text")
	}
	if !strings.Contains(view, "Test finding") {
		t.Error("View should contain finding title")
	}
}

func TestKnowledgeGaps_View_EmptyFindings(t *testing.T) {
	kg := NewKnowledgeGaps()
	kg.active = true
	kg.SetSize(120, 40)

	view := kg.View()
	if !strings.Contains(view, "No findings") {
		t.Error("empty tab should display 'No findings' message")
	}
}
