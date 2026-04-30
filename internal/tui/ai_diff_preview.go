package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Inline AI diff preview
// ---------------------------------------------------------------------------
//
// Sits between aiEditDoneMsg and the editor splice: when the model's
// reply lands, instead of writing immediately we show the user a small
// "before / after" overlay. y/Enter accepts, n/Esc discards. This is
// the Deepnote-style "see what AI proposed before committing" pattern.
//
// Power users can opt out via the AIAutoApplyEdits config flag —
// flipped on, the preview is skipped and edits land immediately.

// AIDiffPreview holds the active proposal. Empty value = no proposal,
// the rest of the UI behaves as before.
type AIDiffPreview struct {
	OverlayBase

	action  aiEditAction
	original string
	proposed string

	// Range to splice when accepted — captured at dispatch time so a
	// cursor move during the AI roundtrip can't redirect the write.
	startLine, startCol, endLine, endCol int
	hadSelection                         bool
}

// NewAIDiffPreview returns an empty, inactive preview.
func NewAIDiffPreview() AIDiffPreview { return AIDiffPreview{} }

// Open arms the preview with a finished AI proposal. Stays inactive
// (no-op) when proposed text is empty — caller should surface the
// "empty response" warning separately.
func (p *AIDiffPreview) Open(action aiEditAction, original, proposed string,
	sl, sc, el, ec int, hadSelection bool) {
	if strings.TrimSpace(proposed) == "" {
		return
	}
	p.Activate()
	p.action = action
	p.original = original
	p.proposed = proposed
	p.startLine, p.startCol = sl, sc
	p.endLine, p.endCol = el, ec
	p.hadSelection = hadSelection
}

// Range returns the captured target range. Used by the host accept-path
// to call editor.ReplaceRange(...).
func (p *AIDiffPreview) Range() (int, int, int, int) {
	return p.startLine, p.startCol, p.endLine, p.endCol
}

// Output returns the proposed replacement text.
func (p *AIDiffPreview) Output() string { return p.proposed }

// Action returns the AI action label (used for status message text).
func (p *AIDiffPreview) Action() aiEditAction { return p.action }

// Reset clears the preview. Called on accept and on discard.
func (p *AIDiffPreview) Reset() {
	p.Close()
	p.original = ""
	p.proposed = ""
	p.action = ""
}

// View renders the preview as a centred panel: header (action label),
// before / after blocks, footer hints (y accept · n discard).
func (p *AIDiffPreview) View() string {
	if !p.IsActive() {
		return ""
	}
	width := p.Width()
	if width <= 0 {
		width = 80
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 4
	if innerW < 30 {
		innerW = 30
	}

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	headerStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)
	beforeStyle := lipgloss.NewStyle().Foreground(red)
	afterStyle := lipgloss.NewStyle().Foreground(green)
	hintStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	dimRule := lipgloss.NewStyle().Foreground(surface1)

	var b strings.Builder
	label := "AI " + aiActionLabel(p.action)
	b.WriteString(titleStyle.Render(label))
	b.WriteString("\n")
	b.WriteString(dimRule.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	b.WriteString(headerStyle.Render("BEFORE"))
	b.WriteString("\n")
	b.WriteString(beforeStyle.Render(softWrap(p.original, innerW, "− ")))
	b.WriteString("\n\n")

	b.WriteString(headerStyle.Render("AFTER"))
	b.WriteString("\n")
	b.WriteString(afterStyle.Render(softWrap(p.proposed, innerW, "+ ")))
	b.WriteString("\n\n")

	b.WriteString(dimRule.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("y / Enter — accept · n / Esc — discard · r — retry (regenerate)"))

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(surface1).
		Padding(0, 2).
		Width(width)
	return box.Render(b.String())
}

// extractEditorRange pulls the text from editor.content covering
// [(startLine,startCol), (endLine,endCol)) — same semantics as
// Editor.ReplaceRange's argument range. Used to capture the BEFORE
// snapshot for the diff preview (the editor's selection state may
// have already cleared by the time the AI roundtrip returns).
func extractEditorRange(e *Editor, sl, sc, el, ec int) string {
	if e == nil || len(e.content) == 0 {
		return ""
	}
	clamp := func(line int) int {
		if line < 0 {
			return 0
		}
		if line >= len(e.content) {
			return len(e.content) - 1
		}
		return line
	}
	sl = clamp(sl)
	el = clamp(el)
	if el < sl || (el == sl && ec < sc) {
		sl, sc, el, ec = el, ec, sl, sc
	}
	if sc < 0 {
		sc = 0
	}
	if sc > len(e.content[sl]) {
		sc = len(e.content[sl])
	}
	if ec < 0 {
		ec = 0
	}
	if ec > len(e.content[el]) {
		ec = len(e.content[el])
	}
	if sl == el {
		return e.content[sl][sc:ec]
	}
	parts := []string{e.content[sl][sc:]}
	for i := sl + 1; i < el; i++ {
		parts = append(parts, e.content[i])
	}
	parts = append(parts, e.content[el][:ec])
	return strings.Join(parts, "\n")
}

// softWrap word-wraps `s` into lines no longer than width-len(prefix),
// prefixing each line with `prefix` (used for the "− " / "+ " gutter
// markers). Falls back to literal break-at-width when the input has
// long token without spaces.
func softWrap(s string, width int, prefix string) string {
	avail := width - len(prefix)
	if avail < 10 {
		avail = 10
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			out = append(out, prefix)
			continue
		}
		// Build wrapped sub-lines word by word.
		words := strings.Fields(line)
		if len(words) == 0 {
			out = append(out, prefix+line)
			continue
		}
		cur := prefix
		curLen := 0
		for _, w := range words {
			candidateLen := curLen + len(w) + 1 // +1 for the space we'd add
			if curLen == 0 {
				candidateLen = len(w)
			}
			if candidateLen > avail && curLen > 0 {
				out = append(out, cur)
				cur = prefix + w
				curLen = len(w)
				continue
			}
			if curLen == 0 {
				cur += w
				curLen = len(w)
			} else {
				cur += " " + w
				curLen = candidateLen
			}
		}
		if curLen > 0 {
			out = append(out, cur)
		}
		// Cap at 8 lines per BEFORE/AFTER block so a huge selection
		// doesn't push the hint footer off-screen.
		if len(out) > 8 {
			out = out[:8]
			out = append(out, prefix+lipgloss.NewStyle().Foreground(overlay0).Render("... (truncated)"))
		}
	}
	return strings.Join(out, "\n")
}
