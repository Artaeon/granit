// Package serveapi — handlers for AI-driven margin-annotation
// suggestions on notes.
//
// Surface: POST /api/v1/ai/annotate-note { notePath } → returns a
// list of proposed margin annotations the user can accept,
// reject, or edit before they land in the annotations store.
//
// Why this is its own file (not handlers_ai_features.go): the
// other Tier 1 features all return "snapshot in, markdown out".
// This one returns structured JSON (line + anchor + text + color)
// because the proposals feed directly into the annotations
// schema. Mixing the parsing + per-row validation into the
// generic features file would muddy that file's clean shape.
package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/aiprefs"
)

// annotateNoteSystemPrompt is the marginalia-tradition system
// prompt — questions, counter-arguments, "this matters" markers.
// We deliberately steer away from generic praise / summary because
// those are different surfaces; the value of margin notes is the
// quality of *re-reading* the user does later, and pleasant
// summaries don't reward re-reading.
const annotateNoteSystemPrompt = `You will receive the body of a user's note, with each line numbered.
Propose 3 to 5 margin annotations the user might want to add — the kind of thing a careful re-reader would write in the margin of a printed page.

Hard rules:
  (1) Each annotation MUST anchor to a SPECIFIC numbered line. Pick the line where the claim, definition, or assertion lives — NOT a header line.
  (2) Each annotation must be one of:
        - a sharp question that probes an assumption ("does this hold for X?")
        - a counter-argument that names a contrary view ("but Smith argues…")
        - a connection to something specific ("ties to the section on Y above")
        - a "this matters" marker that names what's load-bearing ("this is the whole argument in one line")
  (3) AVOID: generic praise, restatement of what the line already says, vague suggestions to "expand". The user can write those without help.
  (4) Each annotation under 25 words. Tone is the user thinking with themselves at midnight, not a polished review.
  (5) Color tag picks ONE of: yellow (questions), blue (connections / context), green ("this matters"), pink (counter-arguments).
  (6) Output STRICT JSON ONLY — no markdown fences, no preamble. Schema:
{"annotations":[{"lineNum":<1-indexed>,"anchorText":"<first 60 chars of that line>","text":"<your annotation>","color":"yellow|blue|green|pink"}]}

If the note is too short or too thin to annotate honestly, return {"annotations":[]} — better empty than padded.`

// AnnotateProposal is the wire shape of one suggested annotation.
// Mirrors annotations.Annotation (subset) so the frontend can
// pass it nearly verbatim into the create endpoint after the
// user reviews + accepts.
type AnnotateProposal struct {
	LineNum    int    `json:"lineNum"`
	AnchorText string `json:"anchorText"`
	Text       string `json:"text"`
	Color      string `json:"color"`
}

func (s *Server) handleAISuggestAnnotations(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NotePath string `json:"notePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.NotePath) == "" {
		writeError(w, http.StatusBadRequest, "notePath required")
		return
	}
	// Resolve the note via the vault — same path-safety rules
	// the rest of the notes endpoints use. EnsureLoaded so the
	// content is hydrated (ScanFast leaves Content empty per the
	// vault contract).
	note := s.cfg.Vault.GetNote(body.NotePath)
	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	s.cfg.Vault.EnsureLoaded(body.NotePath)
	noteBody := note.Content
	if noteBody == "" {
		writeJSON(w, http.StatusOK, map[string]any{"annotations": []AnnotateProposal{}})
		return
	}
	// Build the numbered-line user prompt. We cap at ~600 lines
	// (typical note: well under) so the prompt stays bounded;
	// notes that overshoot get the head + tail of the body, which
	// gives the model the framing + the conclusion to anchor
	// suggestions to without burning a 100k context.
	numbered := numberLines(noteBody, 600)
	user := "Note body (one line per row, prefixed with line number):\n\n" + numbered

	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureAnnotateNote,
		annotateNoteSystemPrompt, user)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Parse the structured reply. Models occasionally wrap in
	// fences; strip defensively. If parsing fails, return the
	// raw text + a warning so the UI can show the user what
	// came back instead of silently giving up.
	cleaned := strings.TrimSpace(out)
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}
	var parsed struct {
		Annotations []AnnotateProposal `json:"annotations"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"annotations": []AnnotateProposal{},
			"raw":         out,
			"warning":     "Model didn't return parseable JSON; showing raw response.",
		})
		return
	}
	// Validate each proposal: line must exist in the body, color
	// must be one of the four palette values, anchor + text must
	// be non-empty. Drop bad rows silently — better to ship 3
	// good suggestions than 5 with one broken row.
	lineCount := strings.Count(noteBody, "\n") + 1
	valid := make([]AnnotateProposal, 0, len(parsed.Annotations))
	for _, p := range parsed.Annotations {
		if p.LineNum < 1 || p.LineNum > lineCount {
			continue
		}
		if strings.TrimSpace(p.Text) == "" {
			continue
		}
		switch p.Color {
		case "yellow", "blue", "green", "pink":
			// ok
		default:
			p.Color = "yellow"
		}
		// Re-snapshot AnchorText from the actual line, regardless
		// of what the model returned — guarantees consistency
		// with what the editor will see when the user accepts.
		if p.AnchorText == "" {
			p.AnchorText = anchorAtLine(noteBody, p.LineNum)
		}
		valid = append(valid, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"annotations": valid})
}

// numberLines prefixes each line with its 1-indexed number. The
// `cap` arg is a head/tail truncation: notes longer than `cap`
// emit head/2 + tail/2 with a "[…]" gap so the model sees the
// framing + conclusion of long pieces without overflowing context.
func numberLines(body string, cap int) string {
	lines := strings.Split(body, "\n")
	if len(lines) <= cap {
		return formatNumbered(lines, 1)
	}
	head := lines[:cap/2]
	tail := lines[len(lines)-cap/2:]
	tailStartLine := len(lines) - cap/2 + 1
	out := formatNumbered(head, 1)
	out += fmt.Sprintf("\n[…lines %d–%d elided…]\n", len(head)+1, tailStartLine-1)
	out += formatNumbered(tail, tailStartLine)
	return out
}

func formatNumbered(lines []string, start int) string {
	var b strings.Builder
	for i, ln := range lines {
		fmt.Fprintf(&b, "%d: %s\n", start+i, ln)
	}
	return b.String()
}

// anchorAtLine returns the first 60 chars of the 1-indexed line.
// Used by the validator when the model didn't return an anchor —
// guarantees the saved annotation references the right text.
func anchorAtLine(body string, lineNum int) string {
	lines := strings.Split(body, "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return ""
	}
	s := strings.TrimSpace(lines[lineNum-1])
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

