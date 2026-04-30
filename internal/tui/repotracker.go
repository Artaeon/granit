package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/repos"
)

// ---------------------------------------------------------------------------
// RepoTracker — local-repo browser + import surface
// ---------------------------------------------------------------------------
//
// Scans a configured root (typically ~/Projects/) for git repositories,
// shows each with live status (branch, dirty, ahead/behind, age), and
// lets the user import any of them as typed-project notes. Pressing
// Enter on a row writes Projects/<repo-name>.md with type:project and
// repo:<absolute-path> pre-filled, then opens it in the editor.
//
// Why a dedicated surface instead of just the Object Browser?
//   - Discovery: brand-new users won't think to add `repo:` manually.
//     The tracker surfaces every repo on disk so the "I have N
//     projects" knowledge is immediate.
//   - Status visibility: rows render git state at a glance (dirty
//     count, last commit), so the tracker doubles as a local
//     "everything I'm working on" board.
//   - Import flow: one keystroke maps a repo to a typed-project note
//     with consistent path conventions; manual creation forces the
//     user to remember the exact path string.

// repoTrackerRow is one entry in the scan: the absolute repo path,
// its display name (folder basename), the live git Status, and a
// flag indicating whether granit has already imported this repo
// (i.e. there's a typed-project note with `repo:` matching this path).
type repoTrackerRow struct {
	Path         string
	Name         string
	Status       repos.Status
	StatusErr    error
	Imported     bool
	ImportedPath string // vault-relative path of the existing project note
}

// RepoTracker is the editor-tab surface for tracking local git repos.
type RepoTracker struct {
	OverlayBase

	// Scan inputs.
	scanRoot string

	// Typed-objects context — used to detect already-imported repos
	// so we don't surface a duplicate-create option.
	registry *objects.Registry
	index    *objects.Index

	// Scan results, sorted by name.
	rows []repoTrackerRow

	// Cursor + scroll state.
	cursor int
	scroll int

	// Scan-time message ("Scanning…", "No git repos found…").
	statusMsg string

	// Consumed-once import request: vault path + content + abs path
	// to the repo. Mirrors ObjectBrowser's createReq pattern.
	importPath    string
	importContent string
	importReq     bool

	// Consumed-once jump request — when the user presses 'g' on a
	// row we want to open the existing project note (if imported).
	jumpPath string
	jumpReq  bool

	// Consumed-once status message — populated by 'o' / 'c' actions
	// (open folder externally, copy path to clipboard) so the host
	// can surface the result on the global status bar. Cleared on
	// read.
	pendingStatus string
}

// NewRepoTracker returns a fresh, inactive surface.
func NewRepoTracker() RepoTracker { return RepoTracker{} }

// Open scans scanRoot for git repositories and activates the surface.
// Errors during the scan are surfaced via statusMsg so the user sees
// "no such directory" instead of a blank tab.
func (r *RepoTracker) Open(scanRoot string, reg *objects.Registry, idx *objects.Index) {
	r.Activate()
	r.scanRoot = scanRoot
	r.registry = reg
	r.index = idx
	r.cursor = 0
	r.scroll = 0
	r.rows = nil
	r.statusMsg = ""

	if strings.TrimSpace(scanRoot) == "" {
		r.statusMsg = "Set RepoScanRoot in Settings (Ctrl+,) to point at your projects folder"
		return
	}
	expanded, err := expandHome(scanRoot)
	if err != nil {
		r.statusMsg = "Bad scan root: " + err.Error()
		return
	}
	if info, err := os.Stat(expanded); err != nil || !info.IsDir() {
		r.statusMsg = "Scan root does not exist or is not a directory: " + expanded
		return
	}
	r.scanRoot = expanded
	r.scan()
	if len(r.rows) == 0 {
		r.statusMsg = "No git repositories found under " + expanded
	}
}

// scan walks one level deep under scanRoot, collecting any direct
// subdirectory that contains a .git entry. Bounded depth keeps the
// scan fast (no descent into node_modules, vendor, etc.) and matches
// the typical "Projects/foo/" layout. Use rg/find externally for
// deeper hierarchies.
func (r *RepoTracker) scan() {
	entries, err := os.ReadDir(r.scanRoot)
	if err != nil {
		r.statusMsg = "Read scan root failed: " + err.Error()
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	imported := r.indexedRepos()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(r.scanRoot, e.Name())
		gitPath := filepath.Join(full, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			continue
		}
		s, err := repos.StatusOf(full)
		row := repoTrackerRow{
			Path: full, Name: e.Name(),
			Status: s, StatusErr: err,
		}
		if existing, ok := imported[full]; ok {
			row.Imported = true
			row.ImportedPath = existing
		}
		r.rows = append(r.rows, row)
	}
}

// indexedRepos returns absoluteRepoPath → vaultRelativeNotePath for
// every typed-project note that has a `repo:` property set. Used to
// mark already-imported rows in the scan results.
func (r *RepoTracker) indexedRepos() map[string]string {
	out := map[string]string{}
	if r.index == nil {
		return out
	}
	for _, obj := range r.index.ByType("project") {
		repoPath := strings.TrimSpace(obj.PropertyValue("repo"))
		if repoPath == "" {
			continue
		}
		expanded, err := expandHome(repoPath)
		if err != nil {
			expanded = repoPath
		}
		// Resolve to absolute path for matching — frontmatter values
		// can be either absolute or ~-prefixed.
		if abs, err := filepath.Abs(expanded); err == nil {
			out[abs] = obj.NotePath
		} else {
			out[expanded] = obj.NotePath
		}
	}
	return out
}

// expandHome turns "~" or "~/foo" into an absolute path. Leaves other
// inputs unchanged. Saves users from having to write absolute paths
// in their config — `~/Projects` is the natural way to express it.
func expandHome(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// GetImportRequest is the consumed-once accessor mirroring the
// Object Browser's GetCreateRequest. Returns (vault-relative path,
// content, ok). The host writes the file, refreshes the vault and
// objects index, and opens the new note.
func (r *RepoTracker) GetImportRequest() (string, string, bool) {
	if !r.importReq {
		return "", "", false
	}
	p, c := r.importPath, r.importContent
	r.importReq = false
	r.importPath = ""
	r.importContent = ""
	return p, c, true
}

// GetJumpRequest returns the existing project note path the user
// pressed 'g' on. Used when the row is already imported.
func (r *RepoTracker) GetJumpRequest() (string, bool) {
	if !r.jumpReq {
		return "", false
	}
	p := r.jumpPath
	r.jumpReq = false
	r.jumpPath = ""
	return p, true
}

// ConsumePendingStatus returns the message queued by the most recent
// folder-action (open / copy-path), then clears it. Host pushes the
// returned string into the status bar.
func (r *RepoTracker) ConsumePendingStatus() string {
	msg := r.pendingStatus
	r.pendingStatus = ""
	return msg
}

// Update handles a single key. Mirrors the Object Browser's surface
// where it makes sense (j/k nav, Enter, Esc, `r` refresh).
func (r *RepoTracker) Update(msg tea.Msg) (RepoTracker, tea.Cmd) {
	if !r.active {
		return *r, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *r, nil
	}
	switch keyMsg.String() {
	case "esc", "q":
		r.active = false
	case "j", "down":
		if r.cursor < len(r.rows)-1 {
			r.cursor++
		}
	case "k", "up":
		if r.cursor > 0 {
			r.cursor--
		}
	case "g":
		// Jump to the project note for an already-imported row;
		// otherwise no-op (only meaningful when imported).
		if r.cursor < len(r.rows) {
			row := r.rows[r.cursor]
			if row.Imported && row.ImportedPath != "" {
				r.jumpPath = row.ImportedPath
				r.jumpReq = true
				r.active = false
			}
		}
	case "G":
		r.cursor = len(r.rows) - 1
		if r.cursor < 0 {
			r.cursor = 0
		}
	case "r":
		// Refresh: re-scan + re-cache statuses. Drops the cache for
		// every visible row so dirty counts the user just changed
		// reflect on the next render.
		clearRepoStatusCache()
		r.rows = nil
		r.scan()
	case "o":
		// Open the focused repo's folder in the system file manager
		// (xdg-open / open / explorer). Doesn't close the tracker —
		// the user often wants to bounce between rows.
		if r.cursor < len(r.rows) {
			row := r.rows[r.cursor]
			if err := repos.OpenFolder(row.Path); err != nil {
				r.pendingStatus = "Open folder failed: " + err.Error()
			} else {
				r.pendingStatus = "Opened " + row.Path
			}
		}
	case "c":
		// Copy the focused repo's absolute path to the system
		// clipboard. Useful for `cd $(xclip -o)` or pasting into
		// other tools (terminals, editors, browsers).
		if r.cursor < len(r.rows) {
			row := r.rows[r.cursor]
			if err := ClipboardCopy(row.Path); err != nil {
				r.pendingStatus = "Clipboard copy failed: " + err.Error()
			} else {
				r.pendingStatus = "Copied " + row.Path + " to clipboard"
			}
		}
	case "enter":
		// Import the focused repo as a typed-project note. Already-
		// imported rows jump to the existing note instead of creating
		// a duplicate.
		if r.cursor >= len(r.rows) {
			break
		}
		row := r.rows[r.cursor]
		if row.Imported && row.ImportedPath != "" {
			r.jumpPath = row.ImportedPath
			r.jumpReq = true
			r.active = false
			break
		}
		if r.registry == nil {
			break
		}
		t, ok := r.registry.ByID("project")
		if !ok {
			break
		}
		// Build the note: type, title, repo, status default. Title
		// = repo folder name; user can rename later.
		extras := map[string]string{
			"repo":   row.Path,
			"status": "active",
		}
		path := objects.PathFor(t, row.Name)
		content := objects.BuildFrontmatter(t, row.Name, extras) +
			"# " + row.Name + "\n\n" +
			"_Imported from `" + row.Path + "`._\n"
		r.importPath = path
		r.importContent = content
		r.importReq = true
		r.active = false
	}
	return *r, nil
}

// clearRepoStatusCache wipes the package-level cache used by the hub
// strip and the tracker so a manual refresh always pulls fresh data.
// Defined here (not in projectgoalhub.go) because the tracker is the
// only caller — keeping the symbol close to its use.
func clearRepoStatusCache() {
	repoStatusCacheMu.Lock()
	repoStatusCache = map[string]repoStatusEntry{}
	repoStatusCacheMu.Unlock()
}

// View renders the tracker. One row per repo with status badges,
// imported flag, and a footer with keybinds.
func (r *RepoTracker) View() string {
	width := r.Width()
	if width <= 0 {
		width = 80
	}
	height := r.Height()
	if height <= 0 {
		height = 24
	}

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	subtitleStyle := lipgloss.NewStyle().Foreground(overlay0)
	hintStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)

	var b strings.Builder
	b.WriteString(" ")
	b.WriteString(titleStyle.Render(fmt.Sprintf("📂 Repo Tracker (%d)", len(r.rows))))
	b.WriteString("\n ")
	if r.scanRoot != "" {
		b.WriteString(subtitleStyle.Render("scanning " + r.scanRoot))
	} else {
		b.WriteString(subtitleStyle.Render("no scan root configured"))
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width-2))
	b.WriteString("\n")

	if r.statusMsg != "" {
		b.WriteString(" ")
		b.WriteString(hintStyle.Render(r.statusMsg))
		b.WriteString("\n")
	}

	if len(r.rows) == 0 {
		b.WriteString("\n  ")
		b.WriteString(hintStyle.Render("Set RepoScanRoot in Settings (Ctrl+,) and press 'r' to rescan."))
		return b.String()
	}

	// Layout: name (24) · status badges (flex) · last commit (right)
	listHeight := height - 7
	if listHeight < 5 {
		listHeight = 5
	}
	if r.cursor >= r.scroll+listHeight {
		r.scroll = r.cursor - listHeight + 1
	}
	if r.cursor < r.scroll {
		r.scroll = r.cursor
	}
	end := r.scroll + listHeight
	if end > len(r.rows) {
		end = len(r.rows)
	}

	rowStyle := lipgloss.NewStyle().Foreground(text)
	selStyle := lipgloss.NewStyle().Background(surface0).Foreground(text).Bold(true)
	for i := r.scroll; i < end; i++ {
		row := r.rows[i]
		line := r.formatRow(row, width-4)
		if i == r.cursor {
			b.WriteString(selStyle.Render(" " + line))
		} else {
			b.WriteString(rowStyle.Render(" " + line))
		}
		b.WriteString("\n")
	}

	// Footer keybinds.
	b.WriteString("\n ")
	b.WriteString(hintStyle.Render(
		"j/k nav · Enter import/open · g jump · o open folder · c copy path · r refresh · Esc close"))
	return b.String()
}

// formatRow builds a single row string. Width budget is split:
//
//   name (28) · imported-badge (3) · branch+dirty (flex) · age (right)
func (r *RepoTracker) formatRow(row repoTrackerRow, w int) string {
	nameW := 28
	if w < 60 {
		nameW = 18
	}
	ageW := 8
	statusW := w - nameW - ageW - 6
	if statusW < 12 {
		statusW = 12
	}

	importedBadge := "  "
	if row.Imported {
		importedBadge = lipgloss.NewStyle().Foreground(green).Render("✓ ")
	}

	name := TruncateDisplay(row.Name, nameW)
	name = PadRight(name, nameW)

	var statusStr, ageStr string
	switch {
	case row.StatusErr != nil:
		statusStr = lipgloss.NewStyle().Foreground(yellow).Render("git: error")
	case !row.Status.IsRepo:
		statusStr = lipgloss.NewStyle().Foreground(overlay0).Render("(not a repo)")
	default:
		parts := []string{row.Status.Branch}
		if row.Status.Dirty > 0 {
			parts = append(parts,
				lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d dirty", row.Status.Dirty)))
		}
		if row.Status.Ahead > 0 || row.Status.Behind > 0 {
			parts = append(parts,
				lipgloss.NewStyle().Foreground(yellow).Render(
					fmt.Sprintf("↑%d ↓%d", row.Status.Ahead, row.Status.Behind)))
		}
		if row.Status.IsClean() {
			parts = append(parts, lipgloss.NewStyle().Foreground(green).Render("clean"))
		}
		statusStr = strings.Join(parts, " · ")
		if !row.Status.LastCommit.IsZero() {
			ageStr = hubAgeString(row.Status.AgeSinceLastCommit())
		}
	}
	statusStr = TruncateDisplay(statusStr, statusW)
	statusStr = PadRight(statusStr, statusW)
	ageStr = PadRight(ageStr, ageW)

	return importedBadge + name + "  " + statusStr + "  " +
		lipgloss.NewStyle().Foreground(overlay0).Render(ageStr)
}
