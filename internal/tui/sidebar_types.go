package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

// SidebarMode controls the structure shown in the sidebar pane.
// Files mode is the historical default — folder/file tree pulled
// from disk. Types mode shows typed-objects from the registry,
// grouped by Type, with each typed note appearing under its Type
// heading. Untyped notes don't appear in Types mode (use Files mode
// to find them).
//
// The mode is per-session — not persisted across launches — because
// most users settle on one and the cycle key is one keystroke away
// when they want the other.
type SidebarMode int

const (
	// ModeFiles renders the folder/file tree (existing default).
	ModeFiles SidebarMode = iota
	// ModeTypes renders typed objects grouped by Type.
	ModeTypes
)

// String returns a human label for the mode, used in the sidebar
// header so the user can tell at a glance which mode they're in.
func (m SidebarMode) String() string {
	switch m {
	case ModeFiles:
		return "files"
	case ModeTypes:
		return "types"
	}
	return "?"
}

// SetMode changes the sidebar's render mode and resets cursor /
// scroll because the row index space differs between modes.
// Called by the cycle keybinding ('m') and by future mode-switch
// command-palette entries.
func (s *Sidebar) SetMode(m SidebarMode) {
	s.mode = m
	s.cursor = 0
	s.scroll = 0
	// Search filter is mode-specific; clearing it on switch
	// avoids the "I typed 'alice' in Files but Types still shows
	// the filtered set" surprise.
	s.search = ""
	s.searching = false
	s.applyFilter()
	s.rebuildTypeRows()
}

// CycleMode advances to the next mode. Wired to 'm' when the
// sidebar is focused. The status message confirms the change so
// the user gets feedback even when the new mode happens to render
// identically to the old (e.g. tiny vault with one type).
func (s *Sidebar) CycleMode() {
	next := SidebarMode((int(s.mode) + 1) % 2)
	s.SetMode(next)
	s.statusMsg = "Sidebar mode: " + next.String()
}

// SetTypedObjects wires the typed-objects registry + index into
// the sidebar so Types mode has data to render. Called by the
// Model on startup and after every vault refresh — Sidebar holds
// only a reference, never the source of truth.
func (s *Sidebar) SetTypedObjects(reg *objects.Registry, idx *objects.Index) {
	s.objectsRegistry = reg
	s.objectsIndex = idx
	s.rebuildTypeRows()
}

// typeRow is one displayable row in Types mode. Header rows have
// Header=true and represent a Type (renders as bold with count
// badge); object rows render the note title indented under their
// Type. We pre-flatten this list so cursor navigation is uniform
// (j/k step one row regardless of header vs. object).
type typeRow struct {
	Header bool   // true: Type header; false: object row
	TypeID string // both header and object rows carry this for filtering
	Title  string // object title (header rows: ignored)
	Path   string // object's NotePath (header rows: empty)
	Icon   string // type icon for headers
	Count  int    // count badge for headers
	Pinned bool   // object row only — drives the ★ marker
}

// rebuildTypeRows flattens the registry+index into the linear
// typeRow slice the Types-mode renderer iterates. Called whenever
// the index changes (initial load, vault refresh, save).
//
// Order:
//   1. PINNED section — typed objects the user starred with `b`,
//      sorted alphabetically. Mirrors the Files-mode PINNED
//      section so the muscle memory transfers.
//   2. Type sections — types in registry order; within each type,
//      objects sorted by title. Empty types are skipped.
//
// A pinned object STILL appears under its Type heading too —
// users said the dual placement felt natural in the Files-mode
// equivalent (the file shows up pinned AND in its folder).
func (s *Sidebar) rebuildTypeRows() {
	s.typeRows = nil
	if s.objectsRegistry == nil || s.objectsIndex == nil {
		return
	}

	// PINNED section — only typed objects (the pinned map can
	// also contain regular file paths from Files mode; we filter
	// for paths that resolve to a typed object).
	var pinnedObjs []*objects.Object
	for path := range s.pinned {
		if obj := s.objectsIndex.ByPath(path); obj != nil {
			pinnedObjs = append(pinnedObjs, obj)
		}
	}
	if len(pinnedObjs) > 0 {
		sort.SliceStable(pinnedObjs, func(i, j int) bool {
			return strings.ToLower(pinnedObjs[i].Title) < strings.ToLower(pinnedObjs[j].Title)
		})
		s.typeRows = append(s.typeRows, typeRow{
			Header: true,
			TypeID: typeRowPinnedHeaderID,
			Title:  "Pinned",
			Icon:   "★",
			Count:  len(pinnedObjs),
		})
		for _, o := range pinnedObjs {
			s.typeRows = append(s.typeRows, typeRow{
				Header: false,
				TypeID: o.TypeID, // keep type id so Enter still routes correctly
				Title:  o.Title,
				Path:   o.NotePath,
				Pinned: true, // marks this row for ★ rendering
			})
		}
	}

	for _, t := range s.objectsRegistry.All() {
		objs := s.objectsIndex.ByType(t.ID)
		if len(objs) == 0 {
			continue
		}
		// Header
		s.typeRows = append(s.typeRows, typeRow{
			Header: true,
			TypeID: t.ID,
			Title:  t.Name,
			Icon:   t.Icon,
			Count:  len(objs),
		})
		// Objects (already sorted by Title in the index, but
		// re-sort defensively since the index is a shared
		// reference and a future change could relax that).
		sorted := append([]*objects.Object(nil), objs...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return strings.ToLower(sorted[i].Title) < strings.ToLower(sorted[j].Title)
		})
		for _, o := range sorted {
			s.typeRows = append(s.typeRows, typeRow{
				Header: false,
				TypeID: t.ID,
				Title:  o.Title,
				Path:   o.NotePath,
				Pinned: s.pinned[o.NotePath],
			})
		}
	}
}

// typeRowPinnedHeaderID is the synthetic TypeID used for the
// PINNED section's header row. Chosen to never collide with a
// real type ID (registry IDs are validated as snake_case slugs,
// the colon prefix here is illegal there).
const typeRowPinnedHeaderID = ":pinned"

// filteredTypeRows applies the active search filter to typeRows,
// keeping a Type's header visible when at least one of its
// objects matches. Empty search returns the full list unfiltered.
//
// Match is case-insensitive substring on object titles. Type
// names are NOT matched on so a search for "person" doesn't
// surface every Person note — which is what you'd want via
// query_objects, not via free-text filter.
func (s *Sidebar) filteredTypeRows() []typeRow {
	q := strings.ToLower(strings.TrimSpace(s.search))
	if q == "" {
		return s.typeRows
	}
	// Two-pass: first find which TypeIDs have matching objects,
	// then emit headers + matched objects in order.
	matchByType := map[string][]typeRow{}
	for _, r := range s.typeRows {
		if r.Header {
			continue
		}
		if strings.Contains(strings.ToLower(r.Title), q) {
			matchByType[r.TypeID] = append(matchByType[r.TypeID], r)
		}
	}
	var out []typeRow
	for _, r := range s.typeRows {
		if !r.Header {
			continue
		}
		if matches, ok := matchByType[r.TypeID]; ok {
			// Replace the header's Count with the filtered
			// count so the badge reflects what the user sees.
			h := r
			h.Count = len(matches)
			out = append(out, h)
			out = append(out, matches...)
		}
	}
	return out
}

// renderTypesView produces the body of Types mode (everything
// below the header + search bar). Mirrors the structure of the
// Files-mode renderer but with type-grouping headers + indented
// object rows. The active note (if any) gets a left-margin marker
// so the user always sees where they are even mid-scroll.
func (s *Sidebar) renderTypesView(contentWidth int) string {
	rows := s.filteredTypeRows()

	if len(rows) == 0 {
		empty := "  No typed notes yet."
		if s.objectsIndex == nil || s.objectsIndex.Total() == 0 {
			empty += "\n  Add `type: person` to a note's frontmatter."
		} else if s.search != "" {
			empty = "  No matches for: " + s.search
		}
		return DimStyle.Render(empty)
	}

	visH := s.height - 4
	if visH < 1 {
		visH = 1
	}

	// Clamp cursor + scroll. Headers count as rows for
	// navigation purposes — the Enter handler distinguishes.
	if s.cursor >= len(rows) {
		s.cursor = len(rows) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= s.scroll+visH {
		s.scroll = s.cursor - visH + 1
	}
	if s.cursor < s.scroll {
		s.scroll = s.cursor
	}
	end := s.scroll + visH
	if end > len(rows) {
		end = len(rows)
	}

	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(surface2)
	objStyle := lipgloss.NewStyle().Foreground(text)
	for i := s.scroll; i < end; i++ {
		r := rows[i]
		isCursor := i == s.cursor && s.focused
		if r.Header {
			icon := r.Icon
			if icon == "" {
				icon = "•"
			}
			// Build the header row with the count badge right-aligned.
			// Truncate the title to fit so we never wrap into a second
			// row inside the narrow sidebar pane.
			badge := fmt.Sprintf("  %d", r.Count)
			titleAvail := contentWidth - 4 - lipgloss.Width(icon) - lipgloss.Width(badge)
			if titleAvail < 4 {
				titleAvail = 4
			}
			title := TruncateDisplay(r.Title, titleAvail)
			line := fmt.Sprintf("  %s  %s", icon, title)
			line = PadRight(line, contentWidth-lipgloss.Width(badge)) + badge
			line = TruncateDisplay(line, contentWidth)
			line = PadRight(line, contentWidth)
			if isCursor {
				out := lipgloss.NewStyle().
					Background(surface0).Foreground(peach).Bold(true).
					Render(line)
				b.WriteString(out)
			} else {
				// Render header (mauve bold) with the badge dimmed.
				// Splitting at the badge boundary keeps the count
				// readable but understated.
				headerLen := lipgloss.Width(line) - lipgloss.Width(badge)
				if headerLen < 0 {
					headerLen = 0
				}
				if headerLen > len(line) {
					headerLen = len(line)
				}
				out := headerStyle.Render(line[:headerLen]) + countStyle.Render(line[headerLen:])
				b.WriteString(out)
			}
		} else {
			activeMarker := "    "
			if r.Path == s.activeNote {
				activeMarker = "  ●"
			}
			star := " "
			if r.Pinned {
				star = "★"
			}
			// Truncate the title to whatever fits after marker + star
			// + spacer. This is the fix for "many lines and cut-off
			// names": before, long titles wrapped because lipgloss
			// Width() defaults to wrap when content exceeds the box.
			titleAvail := contentWidth - lipgloss.Width(activeMarker) - lipgloss.Width(star) - 1
			if titleAvail < 4 {
				titleAvail = 4
			}
			title := TruncateDisplay(r.Title, titleAvail)
			line := fmt.Sprintf("%s%s %s", activeMarker, star, title)
			line = PadRight(line, contentWidth)
			rendered := objStyle.Render(line)
			if r.Pinned {
				// Yellow star prefix so pinned items are scannable.
				// Replace the literal "★" inside the padded line —
				// the offset depends on activeMarker length but
				// strings.Replace handles that for us.
				styled := strings.Replace(line, "★",
					lipgloss.NewStyle().Foreground(yellow).Render("★"), 1)
				rendered = objStyle.Render(styled)
			}
			if isCursor {
				rendered = lipgloss.NewStyle().
					Background(surface0).Foreground(peach).Bold(true).
					Render(line)
			}
			b.WriteString(rendered)
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	if len(rows) > visH {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(fmt.Sprintf("  %d/%d", s.cursor+1, len(rows))))
	}
	return b.String()
}

// selectedTypeRow returns the row at the cursor in Types mode, or
// (zero, false) when the cursor is out of range. Used by the
// Update Enter handler to decide whether to load a note (object
// row) or no-op (header row).
func (s *Sidebar) selectedTypeRow() (typeRow, bool) {
	rows := s.filteredTypeRows()
	if s.cursor < 0 || s.cursor >= len(rows) {
		return typeRow{}, false
	}
	return rows[s.cursor], true
}

// updateTypesMode is the Sidebar.Update branch active when
// mode=ModeTypes. Distinct from the file-tree branch because the
// row model is flat (header + object rows interleaved) — file-tree
// expansion / pinning don't apply.
func (s Sidebar) updateTypesMode(msg tea.Msg) (Sidebar, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	rows := s.filteredTypeRows()
	visH := s.height - 4
	if visH < 1 {
		visH = 1
	}
	switch km.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
			// Skip headers when moving up by k — pressing j on a
			// type header puts you on the first object, so the
			// inverse should land on the previous type's last
			// object, not the header above it. Cleaner navigation.
			for s.cursor > 0 && rows[s.cursor].Header {
				s.cursor--
			}
			if s.cursor < s.scroll {
				s.scroll = s.cursor
			}
		}
	case "down", "j":
		if s.cursor < len(rows)-1 {
			s.cursor++
			for s.cursor < len(rows)-1 && rows[s.cursor].Header {
				s.cursor++
			}
			if s.cursor >= s.scroll+visH {
				s.scroll = s.cursor - visH + 1
			}
		}
	case "pgup", "ctrl+u":
		s.cursor -= visH / 2
		if s.cursor < 0 {
			s.cursor = 0
		}
		if s.cursor < s.scroll {
			s.scroll = s.cursor
		}
	case "pgdown", "ctrl+d":
		s.cursor += visH / 2
		if s.cursor >= len(rows) {
			s.cursor = len(rows) - 1
		}
		if s.cursor < 0 {
			s.cursor = 0
		}
		if s.cursor >= s.scroll+visH {
			s.scroll = s.cursor - visH + 1
		}
	case "home":
		s.cursor = 0
		s.scroll = 0
	case "end":
		if len(rows) > 0 {
			s.cursor = len(rows) - 1
			if s.cursor >= s.scroll+visH {
				s.scroll = s.cursor - visH + 1
			}
		}
	case "/":
		s.searching = true
	case "backspace":
		if s.searching && len(s.search) > 0 {
			s.search = TrimLastRune(s.search)
			s.cursor = 0
			s.scroll = 0
		}
	case "esc":
		if s.searching {
			s.searching = false
			s.search = ""
			s.cursor = 0
			s.scroll = 0
		}
	case "enter":
		// On an object row, the host's outer routing reads
		// Selected() and loadNotes — this Update doesn't need to
		// emit anything special. Just commit the search to keep
		// the filter visible.
		s.searching = false
	case "b":
		// Pin / unpin the typed object under cursor. Mirrors
		// the Files-mode `b` behaviour but operates on the row
		// model's Path. Header rows ignore (no path to pin).
		if r, ok := s.selectedTypeRow(); ok && !r.Header && r.Path != "" {
			if s.pinned == nil {
				s.pinned = make(map[string]bool)
			}
			if s.pinned[r.Path] {
				delete(s.pinned, r.Path)
				s.statusMsg = "Unpinned " + r.Title
			} else {
				s.pinned[r.Path] = true
				s.statusMsg = "Pinned " + r.Title
			}
			s.savePinned()
			s.fileTree.SetPinned(s.pinned)
			s.rebuildTypeRows()
		}
	default:
		if s.searching && len(km.Runes) > 0 {
			s.search += string(km.Runes)
			s.cursor = 0
			s.scroll = 0
		}
	}
	return s, nil
}

