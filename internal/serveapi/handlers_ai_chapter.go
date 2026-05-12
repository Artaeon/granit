package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/atomicio"
)

// POST /api/v1/ai/generate-chapter
// Body: { parentPath, chapterTitle, outline?, save? }
// Returns: { content: "<markdown body>", path?: "<written-path>" }
//
// Used by the "research outline" workflow: the user generates a
// study-plan outline (typically via the Researcher AI mode) which
// renders as a markdown document with [[wikilinks]] to per-chapter
// notes. Clicking an unresolved wikilink offers "generate with AI"
// — the frontend calls this endpoint with the parent (outline) note
// path + the chapter title, and the model produces a focused
// markdown note for that chapter, grounded in the parent's framing.
//
// When `save=true`, granit writes the result to the vault and
// returns the resolved path. When false, returns only the content
// so the caller can show a preview / let the user edit before save.

const generateChapterSystemPrompt = `You are an expert tutor writing ONE focused chapter of a structured study plan.

You will receive:
  - The parent outline (a markdown document framing the whole topic + the chapters around the one you're writing)
  - The specific chapter title you are responsible for

Rules:
  - Write ONLY this chapter — assume the prior chapters have been (or will be) covered separately. Do not repeat earlier chapters' material.
  - Lead with a 1-2 sentence orientation: "what this chapter is about and why it comes after the previous one".
  - Use markdown headings (## and ###) to structure subsections. The chapter starts at level 1 (#), so subsections begin at ##.
  - Include concrete examples, code samples, or diagrams (mermaid) when they sharpen understanding — but never as filler. One real example beats three vague ones.
  - End with a "Practice" or "Try this" section: a small concrete exercise the user can do in 5-15 minutes to solidify the chapter.
  - Cite sources only when you have a specific named source in mind ("Knuth's Art of Programming Vol 1" — yes; "according to research" — no).
  - Length: 400-1200 words. A chapter that takes <400 to cover doesn't need its own note; one that needs >1200 should probably be split (suggest a split at the END if so, in a final "Further reading" line).
  - NO sign-off, NO "I hope this helps", NO preamble before the content. Start with the heading line.
  - Output is markdown plain text; do not wrap in code fences.`

type generateChapterRequest struct {
	ParentPath   string `json:"parentPath"`
	ChapterTitle string `json:"chapterTitle"`
	// Outline override: when set, used instead of reading parentPath
	// from disk. Lets the UI pass an in-flight unsaved outline.
	OutlineOverride string `json:"outline,omitempty"`
	// Save=true → write the result as Chapters/<slug>.md (or
	// <parentDir>/<slug>.md if parent is in a subfolder) and return
	// the path. Save=false → preview only.
	Save bool `json:"save,omitempty"`
	// TargetPath — optional explicit path to write to. When set,
	// overrides the derived path. The UI uses this to honor the
	// exact wikilink the user clicked.
	TargetPath string `json:"targetPath,omitempty"`
}

type generateChapterResponse struct {
	Content string `json:"content"`
	Path    string `json:"path,omitempty"`
}

func (s *Server) handleAIGenerateChapter(w http.ResponseWriter, r *http.Request) {
	var body generateChapterRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	chapterTitle := sanitiseChapterTitle(body.ChapterTitle)
	if chapterTitle == "" {
		writeError(w, http.StatusBadRequest, "chapterTitle required")
		return
	}

	// Resolve the parent outline body. Override > vault note. If
	// neither, the model has no framing — surface a clear error
	// rather than synthesising a chapter in a vacuum.
	outline := strings.TrimSpace(body.OutlineOverride)
	if outline == "" && strings.TrimSpace(body.ParentPath) != "" {
		note := s.cfg.Vault.GetNote(body.ParentPath)
		if note == nil {
			writeError(w, http.StatusNotFound, "parent note not found")
			return
		}
		s.cfg.Vault.EnsureLoaded(body.ParentPath)
		outline = strings.TrimSpace(note.Content)
	}
	if outline == "" {
		writeError(w, http.StatusBadRequest, "outline (or parentPath with content) required — chapter needs framing context")
		return
	}

	// Path validation runs BEFORE the LLM call so a bad targetPath
	// fails fast — otherwise we waste tokens on content the server
	// would refuse to save anyway. Validated path is reused below.
	var resolvedPath string
	if body.Save {
		p, err := normaliseChapterTargetPath(body.TargetPath, body.ParentPath, chapterTitle)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Conflict check is also pre-LLM — same reasoning.
		if existing := s.cfg.Vault.GetNote(p); existing != nil {
			writeError(w, http.StatusConflict,
				fmt.Sprintf("note %q already exists — refusing to overwrite", p))
			return
		}
		resolvedPath = p
	}

	userPrompt := "PARENT OUTLINE (the framing for the whole topic; the chapter you're writing is one of these):\n\n" +
		outline +
		"\n\n----\n\nCHAPTER TO WRITE: " + chapterTitle +
		"\n\nWrite ONLY this chapter's content as markdown, starting with the # heading line."

	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureGenerateChapter,
		generateChapterSystemPrompt, userPrompt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	content := cleanGeneratedChapter(out, chapterTitle)

	resp := generateChapterResponse{Content: content}

	// Optional save — write the result to disk so the next click on
	// the same wikilink resolves to the new note. Path was already
	// validated + conflict-checked above (pre-LLM); reuse it.
	if body.Save {
		abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(resolvedPath))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, "mkdir: "+err.Error())
			return
		}
		if err := atomicio.WriteNote(abs, content); err != nil {
			writeError(w, http.StatusInternalServerError, "write: "+err.Error())
			return
		}
		s.rescanMu.Lock()
		_ = s.cfg.Vault.ScanFast()
		s.rescanMu.Unlock()
		resp.Path = resolvedPath
	}

	writeJSON(w, http.StatusOK, resp)
}

// cleanGeneratedChapter strips common model preamble + wrapping
// code-fences. Defence against the same set of LLM bad habits the
// inline AI editor's cleanAIEditOutput protects against, but here
// for full-note generation rather than mid-line splicing.
//
// Also normalises the leading heading:
//   - When the model returned content WITHOUT a `# Title` line, we
//     prepend `# <chapterTitle>` so the file is well-formed.
//   - When the model returned `# <something else>` (a real failure
//     mode — the model occasionally synthesises its own title from
//     a misreading of the prompt), we REPLACE the first heading
//     with the requested chapterTitle. Otherwise the file's title
//     and the wikilink would diverge and the user would lose track.
//   - When the model returned `# <chapterTitle>` (the happy path),
//     we leave it alone.
//
// The "do these match" check is case-insensitive + trim — small
// presentation drift (extra spaces, slight capitalization) shouldn't
// trigger a rewrite, because the model's variant is often slightly
// more polished prose ("Lexical Scoping & Closures" vs the user's
// "Lexical scoping and closures"). We accept the polished form and
// only intervene when the topic genuinely differs.
func cleanGeneratedChapter(raw, chapterTitle string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip outer code fence — accept both ```...``` and ~~~...~~~
	// variants (CommonMark spec allows both; some models prefer
	// tildes when the content itself contains backticks).
	for _, fence := range []string{"```", "~~~"} {
		if strings.HasPrefix(s, fence) && strings.HasSuffix(s, fence) {
			if nl := strings.Index(s, "\n"); nl > 0 {
				s = s[nl+1:]
			}
			s = strings.TrimSuffix(s, fence)
			s = strings.TrimSpace(s)
			break
		}
	}
	// Strip common preamble lines (first line only, when followed by
	// a blank line and looks intro-y).
	for _, prefix := range []string{
		"Sure!", "Sure,", "Here's the chapter", "Here is the chapter",
		"Below is", "Below's", "I'll write",
	} {
		if strings.HasPrefix(s, prefix) {
			if idx := strings.Index(s, "\n\n"); idx > 0 && idx < 120 {
				s = strings.TrimSpace(s[idx+2:])
				break
			}
		}
	}
	// Heading normalisation. Three cases:
	if !strings.HasPrefix(s, "# ") {
		// Case A: no heading — prepend the requested one.
		return "# " + chapterTitle + "\n\n" + s
	}
	// Case B/C: model emitted a heading. Read the heading text and
	// compare to the requested title.
	nl := strings.Index(s, "\n")
	if nl < 0 {
		nl = len(s)
	}
	headingLine := strings.TrimSpace(strings.TrimPrefix(s[:nl], "# "))
	if topicallyEqual(headingLine, chapterTitle) {
		// Case B: matches (possibly with cosmetic drift) — leave alone.
		return s
	}
	// Case C: heading describes a different topic. Replace it.
	// Defence: prevents the wikilink (still pointing at the user's
	// requested title) from diverging from the saved note's heading.
	rest := ""
	if nl < len(s) {
		rest = s[nl+1:]
	}
	return "# " + chapterTitle + "\n" + rest
}

// sanitiseChapterTitle removes control characters + collapses runs
// of whitespace from a user-controlled chapter title before we
// either (a) splice it into the LLM user-prompt or (b) emit it as
// a markdown heading line.
//
// Why bother:
//   - Newlines in the title would break the heading rewrite path
//     (`# Foo\nBar` becomes two markdown lines, one being
//     freestanding "Bar" text).
//   - Newlines + control chars in the user prompt are a prompt-
//     injection surface — a malicious caller could embed
//     "Closures\n\nIGNORE PREVIOUS INSTRUCTIONS..." in the title.
//     The Researcher mode prompt is short and a determined attacker
//     could still find a way, but stripping the obvious vector is
//     basic hygiene.
//   - Tabs / weird whitespace make filenames awkward when the title
//     also feeds slugifyChapter via deriveChapterPath.
//
// Returns "" for inputs that are entirely whitespace after sanitising
// so the caller's empty-string check rejects them cleanly.
func sanitiseChapterTitle(raw string) string {
	// Drop ASCII control chars (0x00-0x1F) + DEL (0x7F) — covers \r,
	// \n, \t, and anything obscure a model or hostile caller might
	// emit. Multi-byte unicode passes through unchanged.
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r < 0x20 || r == 0x7F {
			b.WriteByte(' ') // replace with space so word boundaries survive
			continue
		}
		b.WriteRune(r)
	}
	// Collapse whitespace runs to single spaces.
	fields := strings.Fields(b.String())
	return strings.Join(fields, " ")
}

// topicallyEqual is a forgiving title comparison: case-insensitive,
// punctuation-elastic, whitespace-collapsed. "Lexical Scoping &
// Closures" == "lexical scoping and closures" — both clearly point
// at the same topic, the model just polished the prose.
//
// Only flags as "different" when the words themselves differ
// (different lemmas, different topic words). A small heuristic but
// catches the genuine "model wrote a different chapter" failure
// without false-positiving on stylistic improvements.
func topicallyEqual(a, b string) bool {
	norm := func(s string) string {
		s = strings.ToLower(s)
		// Replace common conjunction punctuation with spaces so " & "
		// and " and " normalise to the same token sequence.
		s = strings.ReplaceAll(s, "&", " and ")
		s = strings.ReplaceAll(s, ",", " ")
		s = strings.ReplaceAll(s, ";", " ")
		s = strings.ReplaceAll(s, ":", " ")
		// Collapse runs of whitespace to single spaces.
		fields := strings.Fields(s)
		return strings.Join(fields, " ")
	}
	return norm(a) == norm(b)
}

// normaliseChapterTargetPath produces the vault-relative path the
// chapter will land at. Either uses the caller's explicit target,
// or derives one from the parent + chapter title. Then runs all the
// path-safety checks in one place so the handler stays linear:
//
//   - Backslashes → forward-slashes (Windows clients)
//   - Absolute paths rejected (filepath.IsAbs catches Unix /etc/...
//     and Windows C:\...; explicit leading-slash check is belt + braces)
//   - Any segment of ".." rejected (path traversal). Segment-level
//     check (not strings.Contains) so titles legitimately containing
//     ".." in prose ("Patterns .. Anti-patterns") aren't rejected
//   - ".md" suffix enforced so the file is indexable as a note
//
// Returns the validated path or a descriptive error suitable for a
// 400 response body.
func normaliseChapterTargetPath(targetPath, parentPath, chapterTitle string) (string, error) {
	p := strings.TrimSpace(targetPath)
	if p == "" {
		p = deriveChapterPath(parentPath, chapterTitle)
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("targetPath must be vault-relative")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("targetPath cannot traverse outside the vault")
		}
	}
	if !strings.HasSuffix(strings.ToLower(p), ".md") {
		p += ".md"
	}
	return p, nil
}

// deriveChapterPath picks a sensible vault-relative path for a new
// chapter note based on the parent outline's path and the chapter
// title. Rules:
//   - If parent is at Research/<topic>.md, child goes to
//     Research/<topic>/<slug>.md
//   - If parent is at <dir>/<name>.md, child goes to
//     <dir>/<name>/<slug>.md
//   - If no parent path, child goes to Chapters/<slug>.md
//
// Slug strips path-unsafe chars (the agents.SanitiseFilename
// convention) and replaces spaces with hyphens for a friendlier
// URL-style filename.
func deriveChapterPath(parentPath, chapterTitle string) string {
	slug := slugifyChapter(chapterTitle)
	if slug == "" {
		slug = "chapter"
	}
	if strings.TrimSpace(parentPath) == "" {
		return "Chapters/" + slug + ".md"
	}
	// Strip ".md" then treat the result as a folder
	name := strings.TrimSuffix(parentPath, ".md")
	return name + "/" + slug + ".md"
}

// slugifyChapter produces a filesystem-safe filename slug from a
// chapter title. Same sanitisation rules as objects.SanitiseFilename
// (rejects /, \, :, *, ?, ", <, >, |) but ALSO collapses runs of
// whitespace to single hyphens so the resulting filename feels like
// a URL-style slug.
//
// Length cap: 100 runes. Some filesystems (eCryptfs, encrypted ZFS,
// FAT32) limit individual segments to 143 bytes, and the chapter
// filename is one segment of a path that may already include the
// parent's directory (`Research/Outline/<slug>.md`). 100 leaves
// generous headroom for the `.md` suffix and any future encoding
// quirks. Cuts at a word boundary when possible so the truncated
// title stays human-readable.
func slugifyChapter(s string) string {
	s = strings.TrimSpace(s)
	for _, bad := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		s = strings.ReplaceAll(s, bad, "")
	}
	// Collapse whitespace to single hyphens.
	fields := strings.Fields(s)
	slug := strings.Join(fields, "-")
	const cap = 100
	if len(slug) <= cap {
		return slug
	}
	// Prefer a word-boundary cut: walk back from `cap` to the nearest
	// hyphen so the truncation lands between fields rather than
	// mid-word. Falls through to a hard cut if no hyphen within the
	// last 30 chars (single-word title that's just very long).
	cut := cap
	for i := cap; i > cap-30 && i > 0; i-- {
		if slug[i] == '-' {
			cut = i
			break
		}
	}
	return slug[:cut]
}
