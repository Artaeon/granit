package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/vault"
)

// ---------- helpers ----------

// makeVaultAndIndex builds an in-memory Vault and Index from a map of
// relPath -> content strings, suitable for graph tests.
func makeVaultAndIndex(notes map[string]string) (*vault.Vault, *vault.Index) {
	v := &vault.Vault{
		Root:  "/tmp/testvault",
		Notes: make(map[string]*vault.Note),
	}
	for path, content := range notes {
		links := vault.ParseWikiLinks(content)
		v.Notes[path] = &vault.Note{
			Path:    "/tmp/testvault/" + path,
			RelPath: path,
			Title:   path,
			Content: content,
			Links:   links,
		}
	}
	idx := vault.NewIndex(v)
	idx.Build()
	return v, idx
}

func graphKeyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		if len(s) == 1 {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// ---------- NewGraphView ----------

func TestGraphNewGraphView(t *testing.T) {
	v, idx := makeVaultAndIndex(nil)
	g := NewGraphView(v, idx)
	if g.active {
		t.Fatal("expected inactive after construction")
	}
	if g.depth != 1 {
		t.Fatalf("expected depth=1, got %d", g.depth)
	}
	if len(g.nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(g.nodes))
	}
}

// ---------- Open / Close / IsActive ----------

func TestGraphOpenCloseIsActive(t *testing.T) {
	v, idx := makeVaultAndIndex(map[string]string{
		"note.md": "hello",
	})
	g := NewGraphView(v, idx)

	if g.IsActive() {
		t.Fatal("should start inactive")
	}

	g.Open("note.md")
	if !g.IsActive() {
		t.Fatal("should be active after Open")
	}

	g.Close()
	if g.IsActive() {
		t.Fatal("should be inactive after Close")
	}
}

// ---------- SetSize ----------

func TestGraphSetSize(t *testing.T) {
	v, idx := makeVaultAndIndex(nil)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	if g.width != 120 || g.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", g.width, g.height)
	}
}

// ---------- Graph building — nodes from notes ----------

func TestGraphBuildGlobalNodes(t *testing.T) {
	notes := map[string]string{
		"alpha.md": "text",
		"beta.md":  "text",
		"gamma.md": "text",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	if len(g.nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.nodes))
	}

	// Verify all note names are present
	names := map[string]bool{}
	for _, n := range g.nodes {
		names[n.name] = true
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !names[expected] {
			t.Errorf("missing node %q", expected)
		}
	}
}

// ---------- Node connections — wikilinks create edges ----------

func TestGraphNodeConnections(t *testing.T) {
	notes := map[string]string{
		"alpha.md": "link to [[beta]]",
		"beta.md":  "link to [[gamma]]",
		"gamma.md": "no links here",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	// Global graph sorted by total connections descending.
	// beta has 1 incoming (from alpha) + 1 outgoing (to gamma) = 2
	// alpha has 0 incoming + 1 outgoing = 1
	// gamma has 1 incoming + 0 outgoing = 1
	// beta should be first
	if g.nodes[0].name != "beta" {
		t.Errorf("expected most connected node first (beta), got %q", g.nodes[0].name)
	}
	if g.nodes[0].incoming != 1 || g.nodes[0].outgoing != 1 {
		t.Errorf("beta: expected incoming=1 outgoing=1, got incoming=%d outgoing=%d",
			g.nodes[0].incoming, g.nodes[0].outgoing)
	}
}

// ---------- Navigation — cursor movement through nodes ----------

func TestGraphNavigation(t *testing.T) {
	notes := map[string]string{
		"a.md": "text",
		"b.md": "text",
		"c.md": "text",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	if g.cursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", g.cursor)
	}

	// Move down
	g, _ = g.Update(graphKeyMsg("down"))
	if g.cursor != 1 {
		t.Fatalf("expected cursor=1 after down, got %d", g.cursor)
	}

	// Move down again
	g, _ = g.Update(graphKeyMsg("j"))
	if g.cursor != 2 {
		t.Fatalf("expected cursor=2 after j, got %d", g.cursor)
	}

	// Move up
	g, _ = g.Update(graphKeyMsg("up"))
	if g.cursor != 1 {
		t.Fatalf("expected cursor=1 after up, got %d", g.cursor)
	}

	// Move up with k
	g, _ = g.Update(graphKeyMsg("k"))
	if g.cursor != 0 {
		t.Fatalf("expected cursor=0 after k, got %d", g.cursor)
	}
}

// ---------- Enter selects a note ----------

func TestGraphEnterSelectsNote(t *testing.T) {
	notes := map[string]string{
		"a.md": "text",
		"b.md": "text",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	// Move down then press enter
	g, _ = g.Update(graphKeyMsg("down"))
	g, _ = g.Update(graphKeyMsg("enter"))

	if g.IsActive() {
		t.Fatal("graph should close after enter")
	}

	selected := g.SelectedNote()
	if selected == "" {
		t.Fatal("expected a note to be selected")
	}
}

// ---------- Esc closes the graph ----------

func TestGraphEscCloses(t *testing.T) {
	v, idx := makeVaultAndIndex(map[string]string{"a.md": "text"})
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	g, _ = g.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if g.IsActive() {
		t.Fatal("graph should close on esc")
	}
}

// ---------- Mode switching — global vs local graph ----------

func TestGraphModeSwitching(t *testing.T) {
	notes := map[string]string{
		"center.md":  "link to [[nearby]]",
		"nearby.md":  "link to [[faraway]]",
		"faraway.md": "no links",
		"orphan.md":  "isolated note",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("center.md")

	// Starts in global mode
	if g.localMode {
		t.Fatal("should start in global mode")
	}
	if len(g.nodes) != 4 {
		t.Fatalf("global mode should have 4 nodes, got %d", len(g.nodes))
	}

	// Tab -> local mode
	g, _ = g.Update(graphKeyMsg("tab"))
	if !g.localMode {
		t.Fatal("should be in local mode after tab")
	}

	// In local mode with depth=1, only center + direct neighbors
	// center links to nearby; center has no backlinks; so local = center + nearby
	localNames := map[string]bool{}
	for _, n := range g.nodes {
		localNames[n.name] = true
	}
	if !localNames["center"] {
		t.Error("local graph should include center note")
	}
	if !localNames["nearby"] {
		t.Error("local graph should include directly linked note")
	}
	if localNames["orphan"] {
		t.Error("local graph should NOT include unconnected orphan")
	}

	// Tab back -> global mode
	g, _ = g.Update(graphKeyMsg("tab"))
	if g.localMode {
		t.Fatal("should be back in global mode after second tab")
	}
}

// ---------- Depth switching in local mode ----------

func TestGraphDepthSwitching(t *testing.T) {
	notes := map[string]string{
		"center.md":  "link to [[hop1]]",
		"hop1.md":    "link to [[hop2]]",
		"hop2.md":    "no further links",
		"distant.md": "not connected",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("center.md")

	// Switch to local mode
	g, _ = g.Update(graphKeyMsg("tab"))
	if !g.localMode {
		t.Fatal("should be local mode")
	}

	// At depth=1, only center + hop1
	depth1Names := map[string]bool{}
	for _, n := range g.nodes {
		depth1Names[n.name] = true
	}
	if depth1Names["hop2"] {
		t.Error("hop2 should NOT appear at depth=1")
	}

	// Press "2" to expand to depth=2
	g, _ = g.Update(graphKeyMsg("2"))
	if g.depth != 2 {
		t.Fatalf("expected depth=2, got %d", g.depth)
	}
	depth2Names := map[string]bool{}
	for _, n := range g.nodes {
		depth2Names[n.name] = true
	}
	if !depth2Names["hop2"] {
		t.Error("hop2 should appear at depth=2")
	}
	if depth2Names["distant"] {
		t.Error("distant should NOT appear even at depth=2")
	}

	// Press "1" to reduce back to depth=1
	g, _ = g.Update(graphKeyMsg("1"))
	if g.depth != 1 {
		t.Fatalf("expected depth=1, got %d", g.depth)
	}
}

// ---------- Empty vault — no nodes, no crash ----------

func TestGraphEmptyVault(t *testing.T) {
	v, idx := makeVaultAndIndex(nil)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	if len(g.nodes) != 0 {
		t.Fatalf("expected 0 nodes for empty vault, got %d", len(g.nodes))
	}

	// Navigate should not crash
	g, _ = g.Update(graphKeyMsg("down"))
	g, _ = g.Update(graphKeyMsg("up"))
	g, _ = g.Update(graphKeyMsg("enter"))

	// View should not crash
	_ = g.View()
}

// ---------- Single note — one node, no edges ----------

func TestGraphSingleNote(t *testing.T) {
	notes := map[string]string{
		"solo.md": "just a lonely note",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	if len(g.nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.nodes))
	}
	if g.nodes[0].incoming != 0 || g.nodes[0].outgoing != 0 {
		t.Errorf("single orphan should have 0 connections, got in=%d out=%d",
			g.nodes[0].incoming, g.nodes[0].outgoing)
	}
}

// ---------- Self-referencing note — link to self handled ----------

func TestGraphSelfReferencingNote(t *testing.T) {
	notes := map[string]string{
		"self.md": "I link to [[self]]",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("self.md")

	// Should not crash and should have 1 node
	if len(g.nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.nodes))
	}
	// self links to itself: outgoing=1 (self), incoming=1 (self)
	if g.nodes[0].outgoing != 1 {
		t.Errorf("expected outgoing=1, got %d", g.nodes[0].outgoing)
	}
	if g.nodes[0].incoming != 1 {
		t.Errorf("expected incoming=1, got %d", g.nodes[0].incoming)
	}

	// Local mode with self-referencing note
	g, _ = g.Update(graphKeyMsg("tab"))
	if len(g.nodes) != 1 {
		t.Fatalf("local mode: expected 1 node, got %d", len(g.nodes))
	}
}

// ---------- Cursor bounds — cursor stays valid after graph rebuild ----------

func TestGraphCursorBoundsAfterRebuild(t *testing.T) {
	notes := map[string]string{
		"a.md": "link to [[b]]",
		"b.md": "link to [[a]]",
		"c.md": "no links",
		"d.md": "no links",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("a.md")

	// Move cursor to end
	for i := 0; i < 10; i++ {
		g, _ = g.Update(graphKeyMsg("down"))
	}
	// Cursor should be clamped to last node
	if g.cursor >= len(g.nodes) {
		t.Fatalf("cursor %d should be < %d", g.cursor, len(g.nodes))
	}

	// Switch to local mode (fewer nodes) — cursor should be clamped
	g, _ = g.Update(graphKeyMsg("tab"))
	if g.cursor >= len(g.nodes) && len(g.nodes) > 0 {
		t.Fatalf("cursor %d should be < %d after mode switch", g.cursor, len(g.nodes))
	}
}

// ---------- Cursor does not go below 0 ----------

func TestGraphCursorDoesNotGoNegative(t *testing.T) {
	v, idx := makeVaultAndIndex(map[string]string{"a.md": "text"})
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	g, _ = g.Update(graphKeyMsg("up"))
	if g.cursor != 0 {
		t.Fatalf("cursor should not go below 0, got %d", g.cursor)
	}
}

// ---------- View does not panic ----------

func TestGraphViewDoesNotPanic(t *testing.T) {
	notes := map[string]string{
		"a.md": "link to [[b]] and [[c]]",
		"b.md": "link to [[a]]",
		"c.md": "orphan note",
	}
	v, idx := makeVaultAndIndex(notes)
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("a.md")

	// Global view
	out := g.View()
	if out == "" {
		t.Fatal("View should return non-empty string")
	}

	// Local view
	g, _ = g.Update(graphKeyMsg("tab"))
	out = g.View()
	if out == "" {
		t.Fatal("local View should return non-empty string")
	}
}

// ---------- SelectedNote clears after retrieval ----------

func TestGraphSelectedNoteClears(t *testing.T) {
	v, idx := makeVaultAndIndex(map[string]string{"a.md": "text"})
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("")

	g, _ = g.Update(graphKeyMsg("enter"))
	first := g.SelectedNote()
	second := g.SelectedNote()
	if first == "" {
		t.Fatal("first call should return selected note")
	}
	if second != "" {
		t.Fatalf("second call should return empty, got %q", second)
	}
}

// ---------- Tab to local mode without valid center note ----------

func TestGraphTabLocalModeInvalidCenter(t *testing.T) {
	v, idx := makeVaultAndIndex(map[string]string{"a.md": "text"})
	g := NewGraphView(v, idx)
	g.SetSize(120, 40)
	g.Open("") // empty center note

	// Trying to switch to local mode with no center note should not switch
	g, _ = g.Update(graphKeyMsg("tab"))
	if g.localMode {
		t.Fatal("should not switch to local mode without valid center note")
	}
}

// ---------- SetCurrentNote ----------

func TestGraphSetCurrentNote(t *testing.T) {
	v, idx := makeVaultAndIndex(map[string]string{"a.md": "text"})
	g := NewGraphView(v, idx)
	g.SetCurrentNote("a.md")
	if g.centerNote != "a.md" {
		t.Fatalf("expected centerNote=a.md, got %q", g.centerNote)
	}
}
