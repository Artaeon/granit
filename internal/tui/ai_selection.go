package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Inline AI selection edits
// ---------------------------------------------------------------------------
//
// The slash menu and the Alt+A shortcut surface a small set of "do something
// with this text" actions: rewrite, expand, summarize, improve, shorten, fix.
// This file owns the prompt registry, the dispatcher, and the result message
// that the editor handler consumes to splice the AI's output back into the
// note.
//
// Design notes
// ------------
//   - The runtime is *stateless* per request: the action ID, the source text,
//     and the byte range to replace are captured at dispatch time and round-
//     tripped through the message so the handler doesn't need to remember
//     in-flight state.
//   - Range round-trip matters because the user may move the cursor or even
//     start a new selection while the AI is thinking. The message tells the
//     handler exactly what to replace, eliminating "where did the cursor go"
//     races.
//   - We deliberately use the same translateProviderError helper as the
//     agent runtime so the failure surface is consistent between agents and
//     inline edits.

// aiEditAction enumerates the supported inline-edit actions. Defined as a
// string-typed constant set so callers can pass through item.Action verbatim
// (e.g. "ai:rewrite") and we resolve it here.
type aiEditAction string

const (
	aiActionRewrite   aiEditAction = "ai:rewrite"
	aiActionExpand    aiEditAction = "ai:expand"
	aiActionSummarize aiEditAction = "ai:summarize"
	aiActionImprove   aiEditAction = "ai:improve"
	aiActionShorten   aiEditAction = "ai:shorten"
	aiActionFix       aiEditAction = "ai:fix"
)

// aiEditPrompts holds the user-prompt template for each action. The template
// is appended after a leading instruction line and the source text is
// embedded under a fenced header so the model can't confuse it with prose.
var aiEditPrompts = map[aiEditAction]string{
	aiActionRewrite:   "Rewrite the text below to be clearer and more direct. Keep the same meaning, length, and tone.",
	aiActionExpand:    "Expand the text below with more detail and depth. Keep the same tone and voice. Add at most one paragraph of new content.",
	aiActionSummarize: "Summarize the text below in 1–3 sentences. Capture the key points only.",
	aiActionImprove:   "Improve the text below — better word choice, smoother flow, fewer redundancies. Keep the same meaning and approximate length.",
	aiActionShorten:   "Make the text below shorter and tighter. Preserve the key information; cut filler.",
	aiActionFix:       "Fix grammar, spelling, and punctuation in the text below. Do not rewrite — make minimal changes only.",
}

// aiEditSystemPrompt is shared across all actions. It's strict on purpose:
// small models love to add "Sure! Here's the rewritten text:" preambles and
// fence the output in code blocks.
const aiEditSystemPrompt = "You are a text-editing assistant inside a note-taking app. " +
	"Output ONLY the edited text — no preamble, no explanation, no labels, " +
	"no quotes around the output, no code fences. " +
	"Preserve the original markdown formatting (headings, lists, links). " +
	"If the input is empty, output an empty string."

// aiEditDoneMsg is delivered to the main Update loop when an inline edit
// finishes (success or failure). The handler uses (startLine, startCol,
// endLine, endCol) to splice the result in, regardless of where the cursor
// has wandered to in the meantime.
type aiEditDoneMsg struct {
	action aiEditAction
	output string
	err    error

	// The range to replace, captured at dispatch time.
	startLine int
	startCol  int
	endLine   int
	endCol    int

	// hadSelection indicates whether the dispatch was selection-based. When
	// false, we replaced a "current line" range and the handler may want to
	// preserve trailing whitespace differently.
	hadSelection bool
}

// runAIEdit returns a tea.Cmd that performs an inline edit. It captures the
// AI config, action, source text, and target range so the result message
// contains everything the handler needs to splice back in.
//
// The deadline is generous (60s) — Ollama on a cold large model can take
// 20-40s to respond, and we'd rather wait than time out a working request.
// Cloud providers usually return in under 5s.
func runAIEdit(cfg AIConfig, action aiEditAction, source string,
	startLine, startCol, endLine, endCol int, hadSelection bool) tea.Cmd {

	return func() tea.Msg {
		template, ok := aiEditPrompts[action]
		if !ok {
			return aiEditDoneMsg{
				action: action,
				err:    errors.New("unknown AI action: " + string(action)),
				startLine: startLine, startCol: startCol,
				endLine: endLine, endCol: endCol,
				hadSelection: hadSelection,
			}
		}
		userPrompt := template + "\n\n---\n" + source + "\n---"

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		raw, err := cfg.ChatCtx(ctx, aiEditSystemPrompt, userPrompt)
		if err != nil {
			return aiEditDoneMsg{
				action: action,
				err:    translateProviderError(cfg, err),
				startLine: startLine, startCol: startCol,
				endLine: endLine, endCol: endCol,
				hadSelection: hadSelection,
			}
		}

		out := cleanAIEditOutput(raw)
		return aiEditDoneMsg{
			action: action, output: out,
			startLine: startLine, startCol: startCol,
			endLine: endLine, endCol: endCol,
			hadSelection: hadSelection,
		}
	}
}

// cleanAIEditOutput strips common preamble, code fences, and surrounding
// quotes that small models are prone to adding despite the system prompt.
// Conservative on purpose — we'd rather leave one stray character than
// accidentally chop off the user's actual content.
func cleanAIEditOutput(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// Strip common preambles like "Sure! Here's the rewritten text:" — only
	// when followed by a newline or colon, so we don't strip legitimate prose.
	for _, prefix := range []string{
		"Sure!", "Sure,", "Here's the", "Here is the", "Here's a", "Here is a",
		"Rewritten:", "Improved:", "Summary:", "Expanded:", "Shortened:", "Fixed:",
	} {
		if strings.HasPrefix(s, prefix) {
			if idx := strings.Index(s, "\n"); idx >= 0 && idx < 80 {
				s = strings.TrimSpace(s[idx+1:])
				break
			}
		}
	}

	// Strip surrounding code fences. Only if the whole output is a single
	// fenced block — partial fences in the middle of structured output are
	// legitimate.
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") {
		// Drop opening fence + optional language tag.
		if nl := strings.Index(s, "\n"); nl > 0 {
			s = s[nl+1:]
		}
		// Drop closing fence.
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	// Strip a single matching pair of straight or smart quotes wrapping the
	// entire output.
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}

	return s
}

// dispatchAIEdit packages up the editor's current selection (or current line
// fallback), captures the AI config, and returns a tea.Cmd that runs the
// edit. Sets m.aiEditPending so overlapping requests are rejected and the
// caller can render a status hint.
//
// preferSelection=true means the menu was opened in modeAI (selection
// preserved), so we use selection if present. preferSelection=false means
// the menu was opened by typing "/" — selection was cleared by that
// keystroke, so we always fall back to the current line.
func (m *Model) dispatchAIEdit(action aiEditAction, preferSelection bool) tea.Cmd {
	if m.aiEditPending {
		m.statusbar.SetMessage("AI: already running")
		return m.clearMessageAfter(2 * time.Second)
	}

	var (
		source                                    string
		startLine, startCol, endLine, endCol int
		hadSelection                              bool
	)

	if preferSelection && m.editor.HasSelection() {
		source = m.editor.GetSelectedText()
		startLine, startCol, endLine, endCol = m.editor.SelectionRange()
		hadSelection = true
	} else {
		startLine, startCol, endLine, endCol = m.editor.CurrentLineRange()
		if startLine < len(m.editor.content) {
			source = m.editor.content[startLine]
		}
	}

	if strings.TrimSpace(source) == "" {
		m.statusbar.SetWarning("AI: nothing to edit (select text or place cursor on a non-empty line)")
		return m.clearMessageAfter(4 * time.Second)
	}

	cfg := m.aiConfig()
	cfg.Model = m.getAIModel()

	m.aiEditPending = true
	m.statusbar.SetMessage("AI " + aiActionLabel(action) + "...")

	return runAIEdit(cfg, action, source, startLine, startCol, endLine, endCol, hadSelection)
}

// aiActionLabel returns a short user-facing label for status messages.
func aiActionLabel(a aiEditAction) string {
	switch a {
	case aiActionRewrite:
		return "rewriting"
	case aiActionExpand:
		return "expanding"
	case aiActionSummarize:
		return "summarizing"
	case aiActionImprove:
		return "improving"
	case aiActionShorten:
		return "shortening"
	case aiActionFix:
		return "fixing grammar"
	default:
		return "thinking"
	}
}
