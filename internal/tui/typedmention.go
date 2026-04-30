package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

// TypedMentionPicker is the modal overlay that lets the user pick a
// typed object to mention in the active editor buffer. Triggered via
// command palette ("Insert Typed Mention") or the keybind `Alt+@`.
//
// On Enter, the picker emits a wikilink `[[Title]]` insertion request
// the host model fulfils through Editor.InsertText. Compatible with
// the existing wikilink renderer — no new syntax to teach the
// renderer or the publish pipeline.
//
// Filter: type "person:" to scope to people, "person:alice" to filter
// by both type AND name. Bare query (no colon) does fuzzy match
// across every typed object's title and type name.
type TypedMentionPicker struct {
	OverlayBase

	registry *objects.Registry
	index    *objects.Index

	query   string
	cursor  int
	matches []typedMentionHit

	// Consumed-once: when the user picks an entry, mentionInsert
	// holds the wikilink string to insert; the host reads it via
	// ConsumeInsert() right after Update returns.
	mentionInsert string
}

// typedMentionHit is one row in the picker — what the user sees and
// what we paste on confirm.
type typedMentionHit struct {
	TypeID   string // e.g. "person"
	TypeName string // e.g. "Person"
	Icon     string
	Title    string
	NotePath string
}

// NewTypedMentionPicker returns a fresh picker. The host calls Open
// with the live registry+index immediately before showing it.
func NewTypedMentionPicker() TypedMentionPicker {
	return TypedMentionPicker{}
}

// Open activates the picker against the current vault state.
func (p *TypedMentionPicker) Open(reg *objects.Registry, idx *objects.Index) {
	p.Activate()
	p.registry = reg
	p.index = idx
	p.query = ""
	p.cursor = 0
	p.mentionInsert = ""
	p.refreshMatches()
}

// ConsumeInsert returns the wikilink to insert and clears it. The
// host model reads this once per Update tick after the picker's
// Update; subsequent calls return ("", false).
func (p *TypedMentionPicker) ConsumeInsert() (string, bool) {
	if p.mentionInsert == "" {
		return "", false
	}
	out := p.mentionInsert
	p.mentionInsert = ""
	return out, true
}

// refreshMatches rebuilds the visible match list from the current
// query. Triggered after every keystroke that changes the query and
// after Open.
func (p *TypedMentionPicker) refreshMatches() {
	p.matches = nil
	if p.registry == nil || p.index == nil {
		return
	}

	// Parse query for an optional `typeID:` prefix.
	wantType := ""
	titleQ := strings.TrimSpace(p.query)
	if idx := strings.Index(titleQ, ":"); idx > 0 {
		wantType = strings.ToLower(strings.TrimSpace(titleQ[:idx]))
		titleQ = strings.TrimSpace(titleQ[idx+1:])
	}
	titleQLower := strings.ToLower(titleQ)

	// Walk every type in registry order, gather objects, score.
	for _, t := range p.registry.All() {
		if wantType != "" && !strings.HasPrefix(strings.ToLower(t.ID), wantType) {
			continue
		}
		for _, o := range p.index.ByType(t.ID) {
			if titleQLower != "" {
				if !strings.Contains(strings.ToLower(o.Title), titleQLower) {
					continue
				}
			}
			p.matches = append(p.matches, typedMentionHit{
				TypeID:   t.ID,
				TypeName: t.Name,
				Icon:     t.Icon,
				Title:    o.Title,
				NotePath: o.NotePath,
			})
		}
	}
	// Stable order: prefix matches on title first, then substring
	// matches, both tie-broken by title alpha.
	sort.SliceStable(p.matches, func(i, j int) bool {
		ai := strings.HasPrefix(strings.ToLower(p.matches[i].Title), titleQLower)
		aj := strings.HasPrefix(strings.ToLower(p.matches[j].Title), titleQLower)
		if ai != aj {
			return ai
		}
		return strings.ToLower(p.matches[i].Title) < strings.ToLower(p.matches[j].Title)
	})
	if p.cursor >= len(p.matches) {
		p.cursor = max0(len(p.matches) - 1)
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

// Update handles a single tea.KeyMsg. Returns the (possibly mutated)
// picker plus a tea.Cmd. The picker doesn't emit any tea.Cmd today
// — host reads ConsumeInsert + IsActive after every Update.
func (p TypedMentionPicker) Update(msg tea.Msg) (TypedMentionPicker, tea.Cmd) {
	if !p.active {
		return p, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}
	switch km.String() {
	case "esc":
		p.active = false
		return p, nil
	case "enter":
		if p.cursor >= 0 && p.cursor < len(p.matches) {
			h := p.matches[p.cursor]
			// Inserts as a standard wikilink for compatibility
			// with the existing renderer + publish pipeline.
			// Future Phase 5 may emit the typed-mention syntax
			// `@type:Title` if the rest of the renderer learns
			// to display it specially.
			p.mentionInsert = "[[" + h.Title + "]]"
			p.active = false
		}
		return p, nil
	case "up", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, nil
	case "down", "ctrl+j":
		if p.cursor < len(p.matches)-1 {
			p.cursor++
		}
		return p, nil
	case "backspace":
		if len(p.query) > 0 {
			p.query = TrimLastRune(p.query)
			p.cursor = 0
			p.refreshMatches()
		}
		return p, nil
	default:
		if len(km.Runes) > 0 {
			p.query += string(km.Runes)
			p.cursor = 0
			p.refreshMatches()
		}
		return p, nil
	}
}

// View renders the picker as a compact centered modal.
func (p TypedMentionPicker) View() string {
	if !p.active {
		return ""
	}
	w := p.width / 2
	if w < 50 {
		w = 50
	}
	if w > 80 {
		w = 80
	}

	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  @ Insert Typed Mention"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", w-4)))
	b.WriteString("\n")

	// Search input row.
	queryStyle := lipgloss.NewStyle().Foreground(text)
	cursor := lipgloss.NewStyle().Foreground(peach).Render("█")
	b.WriteString("  ")
	if p.query == "" {
		b.WriteString(DimStyle.Render("Type to filter — try `person:` to scope"))
	} else {
		b.WriteString(queryStyle.Render(p.query))
	}
	b.WriteString(cursor)
	b.WriteString("\n\n")

	// Result list (cap to ~10 visible rows).
	if len(p.matches) == 0 {
		b.WriteString(DimStyle.Render("  (no matches)"))
	} else {
		visible := 10
		if len(p.matches) < visible {
			visible = len(p.matches)
		}
		start := 0
		if p.cursor >= visible {
			start = p.cursor - visible + 1
		}
		end := start + visible
		if end > len(p.matches) {
			end = len(p.matches)
			start = end - visible
			if start < 0 {
				start = 0
			}
		}
		for i := start; i < end; i++ {
			h := p.matches[i]
			icon := h.Icon
			if icon == "" {
				icon = "•"
			}
			line := fmt.Sprintf("  %s  %s", icon, h.Title)
			typeBadge := lipgloss.NewStyle().Foreground(overlay0).Render(
				fmt.Sprintf("  %s", h.TypeName))
			full := line + typeBadge
			if i == p.cursor {
				full = lipgloss.NewStyle().
					Background(surface0).Foreground(peach).Bold(true).
					Width(w - 4).Render(line + typeBadge)
			}
			b.WriteString(full)
			if i < end-1 {
				b.WriteString("\n")
			}
		}
		if len(p.matches) > visible {
			b.WriteString("\n")
			b.WriteString(DimStyle.Render(fmt.Sprintf("  %d/%d", p.cursor+1, len(p.matches))))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  ↑/↓ navigate · Enter insert [[link]] · Esc cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(w).
		Background(mantle)
	return border.Render(b.String())
}
