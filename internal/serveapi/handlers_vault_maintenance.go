package serveapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/aiaudit"
	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/airedact"
	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/history"
	"github.com/artaeon/granit/internal/sabbath"
	"github.com/artaeon/granit/internal/textutil"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

// Vault maintenance — two AI-assisted curation surfaces under
// /api/v1/maintenance/*:
//
//   - Weekly digest: scans the last N days of touched notes and
//     streams proposals (merge clusters / retitles / missing tags)
//     as the model finds them.
//   - Orphan rescue: deterministic vault scan for notes with zero
//     incoming wikilinks (or stale-modtime in the future), with an
//     opt-in AI step that suggests 2-3 backlink candidates per
//     orphan.
//
// Both pour into one apply endpoint that mutates one note at a
// time. Cross-note merges are deliberately refused — combining
// notes is editor work, not a one-click operation, and the user
// loses too much if an AI-driven concat goes wrong.

// ─── Wire shapes ─────────────────────────────────────────────────

// maintenanceSuggestion is the discriminated union the LLM emits
// one-per-line and the apply endpoint accepts. JSON tags use the
// kind name as the discriminator and the rest of the fields are
// kind-specific. The frontend models it as a TypeScript union with
// the same shape.
type maintenanceSuggestion struct {
	Kind string `json:"kind"`

	// merge: 2+ notes that look like one topic. The applier refuses
	// these because cross-note merges need editorial judgement.
	Notes  []string `json:"notes,omitempty"`
	Reason string   `json:"reason,omitempty"`

	// retitle: single note → new title (filename rename).
	Note            string `json:"note,omitempty"`
	CurrentTitle    string `json:"currentTitle,omitempty"`
	SuggestedTitle  string `json:"suggestedTitle,omitempty"`

	// missing-tags: single note + list of tag names to add.
	SuggestedTags []string `json:"suggestedTags,omitempty"`

	// add-backlink: inserts a [[wikilink]] from fromNotePath →
	// toNotePath. AnchorText is the link label; if empty the
	// target's title is used.
	FromNotePath string `json:"fromNotePath,omitempty"`
	ToNotePath   string `json:"toNotePath,omitempty"`
	AnchorText   string `json:"anchorText,omitempty"`
}

type weeklyDigestRequest struct {
	LookbackDays   int `json:"lookbackDays,omitempty"`
	MaxSuggestions int `json:"maxSuggestions,omitempty"`
}

type orphanNote struct {
	Path               string                  `json:"path"`
	Title              string                  `json:"title"`
	ModTime            time.Time               `json:"modTime"`
	SuggestedBacklinks []backlinkSuggestion    `json:"suggestedBacklinks,omitempty"`
}

type backlinkSuggestion struct {
	From    string `json:"from"`
	Excerpt string `json:"excerpt,omitempty"`
}

// ─── Weekly digest (SSE) ─────────────────────────────────────────

const weeklyDigestSystemPrompt = `You audit the user's vault for hygiene. You will receive a JSON list of recent notes (path, title, mod time, current tags, excerpt).
Emit ONE suggestion per line as a bare JSON object — no prose, no markdown fences, no array wrapper. NDJSON.

Each line must be one of these shapes:

{"kind":"merge","notes":["path1.md","path2.md"],"reason":"<<15 words"}
{"kind":"retitle","note":"path.md","currentTitle":"...","suggestedTitle":"...","reason":"<<15 words"}
{"kind":"missing-tags","note":"path.md","suggestedTags":["tag-one","tag-two"],"reason":"<<15 words"}

Rules:
- Quality over quantity. Skip a note rather than fabricate a suggestion.
- "merge" only for clusters of 2-3 notes that genuinely cover the same topic.
- "retitle" only when the current title is materially worse than the suggestion (vague, dated, autogen). Keep the .md extension off the title field.
- "missing-tags" suggests 2-4 tags, lowercase, hyphenated. Skip tags already present.
- Never invent paths. Use paths exactly as supplied.
- Emit at most %d suggestions total. Stop early if nothing else fits.`

func (s *Server) handleMaintenanceWeeklyDigest(w http.ResponseWriter, r *http.Request) {
	var body weeklyDigestRequest
	// Body is optional — POST with no JSON should still work. Decode
	// errors are not fatal; we just take defaults.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body.LookbackDays <= 0 {
		body.LookbackDays = 7
	}
	if body.LookbackDays > 90 {
		body.LookbackDays = 90 // cap to keep the prompt bounded
	}
	if body.MaxSuggestions <= 0 {
		body.MaxSuggestions = 10
	}
	if body.MaxSuggestions > 50 {
		body.MaxSuggestions = 50
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported by transport")
		return
	}

	// Same posture as other Tier 1 AI features — Sabbath + consent
	// + redaction gates before any model is built. Refusal sent as
	// an SSE error event so the client surfaces it in the same
	// channel as runtime failures.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(event, data string) {
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	if sabbath.IsActiveNow(s.cfg.Vault.Root) {
		send("error", mustJSON(map[string]string{"message": "AI features are paused during Sabbath — exit Sabbath mode to use them"}))
		return
	}
	prefs, _ := aiprefs.Load(s.cfg.Vault.Root)
	fcfg, fok := prefs.Features[aiprefs.FeatureMaintenanceDigest]
	if !fok || !fcfg.Enabled {
		send("error", mustJSON(map[string]string{"message": "feature \"maintenance_digest\" is disabled in AI preferences"}))
		return
	}

	// Build the candidate pool — notes touched in the last
	// LookbackDays days. Capped at 60 entries so the prompt stays
	// bounded; sorted newest-first so freshly-edited notes carry
	// the most signal.
	type digestEntry struct {
		Path    string    `json:"path"`
		Title   string    `json:"title"`
		ModTime time.Time `json:"modTime"`
		Tags    []string  `json:"tags,omitempty"`
		Excerpt string    `json:"excerpt,omitempty"`
	}
	cutoff := time.Now().AddDate(0, 0, -body.LookbackDays)
	notes := s.cfg.Vault.SnapshotNotes()
	pool := make([]digestEntry, 0)
	for _, n := range notes {
		if n.ModTime.Before(cutoff) {
			continue
		}
		s.cfg.Vault.EnsureLoaded(n.RelPath)
		excerpt := textutil.TruncateRunes(strings.TrimSpace(stripFrontmatterBody(n.Content)), 280)
		pool = append(pool, digestEntry{
			Path:    n.RelPath,
			Title:   n.Title,
			ModTime: n.ModTime,
			Tags:    tagsFor(n),
			Excerpt: excerpt,
		})
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].ModTime.After(pool[j].ModTime) })
	if len(pool) > 60 {
		pool = pool[:60]
	}
	if len(pool) == 0 {
		send("done", mustJSON(map[string]int{"count": 0}))
		return
	}

	cfgFile := resolveLLMConfig(s.cfg.Vault.Root, fcfg.Provider, prefs.DefaultProvider)
	llm, err := agentruntime.NewLLM(cfgFile)
	if err != nil {
		s.recordAuditFailure(aiprefs.FeatureMaintenanceDigest, cfgFile, "", nil, err)
		send("error", mustJSON(map[string]string{"message": err.Error()}))
		return
	}
	if hint := preflightLLM(llm); hint != "" {
		send("error", mustJSON(map[string]string{"message": hint}))
		return
	}

	systemPrompt := fmt.Sprintf(weeklyDigestSystemPrompt, body.MaxSuggestions)
	poolJSON, _ := json.Marshal(pool)
	userPrompt := fmt.Sprintf("Notes touched in the last %d days (sorted newest first):\n\n```json\n%s\n```\n\nEmit up to %d hygiene suggestions, one JSON object per line.",
		body.LookbackDays, string(poolJSON), body.MaxSuggestions)

	// Apply redaction to the user prompt (excerpts can carry PII)
	// before it ever touches the model. Mirrors runAIFeature's
	// posture: stats live with the audit entry, originals never.
	finalPrompt := userPrompt
	var stats []airedact.Stat
	if prefs.RedactionEnabled {
		finalPrompt, stats = airedact.RedactWithStats(userPrompt, airedact.DefaultRules())
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	messages := []agentruntime.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: finalPrompt},
	}

	// Stream collector — we accumulate text chunks and emit one
	// SSE "suggestion" event per parseable NDJSON line. Lines that
	// don't parse are skipped silently; we'd rather lose a stray
	// half-line than abort the whole stream on the model's
	// occasional malformed object.
	var (
		buf       strings.Builder
		emitted   int
		bytesOut  int
		runErr    error
	)
	emit := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		// Tolerate the occasional opening ```json fence even though
		// the system prompt forbids it. Strip and continue.
		if strings.HasPrefix(line, "```") {
			return
		}
		var sg maintenanceSuggestion
		if err := json.Unmarshal([]byte(line), &sg); err != nil {
			return
		}
		switch sg.Kind {
		case "merge":
			if len(sg.Notes) < 2 {
				return
			}
		case "retitle":
			if sg.Note == "" || strings.TrimSpace(sg.SuggestedTitle) == "" {
				return
			}
		case "missing-tags":
			if sg.Note == "" || len(sg.SuggestedTags) == 0 {
				return
			}
		default:
			return // unknown kind
		}
		emitted++
		data, _ := json.Marshal(sg)
		send("suggestion", string(data))
		if emitted >= body.MaxSuggestions {
			cancel() // signal the upstream to stop generating
		}
	}
	flushLines := func(final bool) {
		text := buf.String()
		for {
			nl := strings.IndexByte(text, '\n')
			if nl < 0 {
				break
			}
			line := text[:nl]
			text = text[nl+1:]
			emit(line)
		}
		if final && strings.TrimSpace(text) != "" {
			emit(text)
			text = ""
		}
		buf.Reset()
		buf.WriteString(text)
	}

	// Audit fires once at the end regardless of streaming vs.
	// buffered path. Same shape as auditChat / runAIFeature so the
	// log stays uniform.
	defer func() {
		if s.aiAudit == nil {
			return
		}
		entry := aiaudit.Entry{
			Feature:           string(aiprefs.FeatureMaintenanceDigest),
			Provider:          cfgFile.AIProvider,
			Model:             effectiveModel(cfgFile),
			ResponseSizeBytes: bytesOut,
		}
		if metered, ok := llm.(agentruntime.Metered); ok {
			usage := metered.LastUsage()
			entry.PromptTokens = usage.PromptTokens
			entry.CompletionTokens = usage.CompletionTokens
			if cost := agentruntime.CostMicroCents(usage); cost >= 0 {
				entry.CostMicroCents = cost
			}
		}
		if runErr != nil {
			entry.Error = runErr.Error()
		}
		if len(stats) > 0 {
			entry.RedactionsByRule = make([]aiaudit.Stat, len(stats))
			for i, st := range stats {
				entry.RedactionsByRule[i] = aiaudit.Stat{Name: st.Name, Count: st.Count}
			}
		}
		_, _ = s.aiAudit.Append(entry, finalPrompt)
	}()

	if streamer, ok := llm.(agentruntime.ChatStreamer); ok {
		runErr = streamer.ChatStream(ctx, messages, func(chunk string) {
			bytesOut += len(chunk)
			buf.WriteString(chunk)
			flushLines(false)
		})
	} else if chatter, ok := llm.(agentruntime.Chatter); ok {
		var reply string
		reply, runErr = chatter.Chat(ctx, messages)
		bytesOut = len(reply)
		buf.WriteString(reply)
	} else {
		runErr = fmt.Errorf("configured LLM does not support chat")
	}
	flushLines(true)

	if runErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) && emitted >= body.MaxSuggestions {
			// Clean cap-stop, not a real error.
			runErr = nil
			send("done", mustJSON(map[string]int{"count": emitted}))
			return
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			runErr = fmt.Errorf("cancelled by user")
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			runErr = fmt.Errorf("timed out")
		}
		send("error", mustJSON(map[string]string{"message": runErr.Error()}))
		return
	}
	send("done", mustJSON(map[string]int{"count": emitted}))
}

// ─── Orphan rescue ───────────────────────────────────────────────

const orphanSuggestSystemPrompt = `You are helping the user reconnect "orphan" notes (notes with zero incoming wikilinks) to the rest of their vault.
You will receive one orphan note (path + title + excerpt) and a JSON pool of candidate other notes.
Return STRICTLY a JSON array (no prose, no fences) of up to 3 backlink suggestions:
[{"from":"other-note-path.md","excerpt":"<<25 words from that note that justifies the link"}]

Rules:
- Pick candidates whose subject genuinely relates to the orphan. Quality over quantity. 0 is fine.
- "from" MUST be a path from the supplied candidate pool. Never invent.
- "excerpt" is a short quote (verbatim or paraphrase under 25 words) from the candidate note showing why it'd link here.
- Skip candidates that already link to the orphan.`

func (s *Server) handleMaintenanceOrphans(w http.ResponseWriter, r *http.Request) {
	suggest := r.URL.Query().Get("suggest") == "1"

	// Deterministic scan — notes whose Backlinks field is empty.
	// The index is rebuilt on every Vault.Scan(); for the fast-scan
	// path we read backlinks lazily. SnapshotNotes is safe to
	// iterate without holding the rescan mutex.
	notes := s.cfg.Vault.SnapshotNotes()
	type rec struct {
		path    string
		title   string
		modTime time.Time
	}
	var orphans []rec
	for _, n := range notes {
		if len(n.Backlinks) > 0 {
			continue
		}
		// Skip daily notes, reviews, devotionals — the recurring
		// dated surfaces are expected to be terminal leaves and
		// orphan-rescue would generate spurious work. Match the
		// folder prefixes the rest of granit treats as templated.
		p := strings.ToLower(n.RelPath)
		if strings.HasPrefix(p, "dailies/") ||
			strings.HasPrefix(p, "reviews/") ||
			strings.HasPrefix(p, "devotionals/") ||
			strings.HasPrefix(p, "examen/") ||
			strings.HasPrefix(p, "templates/") ||
			strings.HasPrefix(p, ".granit/") {
			continue
		}
		orphans = append(orphans, rec{path: n.RelPath, title: n.Title, modTime: n.ModTime})
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].modTime.After(orphans[j].modTime) })
	// Cap the response — a fresh vault can have hundreds of orphans
	// and we'd rather surface the most-recent batch than ship a
	// 5MB JSON payload.
	const maxOrphans = 200
	if len(orphans) > maxOrphans {
		orphans = orphans[:maxOrphans]
	}

	out := make([]orphanNote, 0, len(orphans))
	for _, o := range orphans {
		entry := orphanNote{
			Path:    o.path,
			Title:   o.title,
			ModTime: o.modTime,
		}
		if suggest {
			candidates, err := s.suggestOrphanBacklinks(r.Context(), o.path)
			if err == nil {
				entry.SuggestedBacklinks = candidates
			}
			// Errors swallowed per-orphan — one orphan's failed
			// AI call shouldn't sink the whole list. The audit log
			// in runAIFeature already captures the failure.
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"orphans": out})
}

// suggestOrphanBacklinks runs one AI feature call to pick 2-3 candidate
// backlink sources for a single orphan note. Returns an empty slice on
// any failure — the orphans endpoint surfaces "no suggestions" rather
// than an error, since per-orphan AI calls are best-effort.
func (s *Server) suggestOrphanBacklinks(ctx context.Context, orphanPath string) ([]backlinkSuggestion, error) {
	orphan := s.cfg.Vault.GetNote(orphanPath)
	if orphan == nil {
		return nil, fmt.Errorf("orphan not found")
	}
	// EnsureLoaded BEFORE reading Content — fast-scan vaults populate
	// only metadata up front, so a missing EnsureLoaded ships an
	// empty excerpt to the model and burns a token round trip for
	// nothing.
	s.cfg.Vault.EnsureLoaded(orphanPath)
	excerpt := textutil.TruncateRunes(strings.TrimSpace(stripFrontmatterBody(orphan.Content)), 800)

	// Candidate pool: top 40 notes by mod time, excluding the
	// orphan itself + notes that already link to it (defensive —
	// the orphan filter upstream should have caught this).
	type cand struct {
		Path    string `json:"path"`
		Title   string `json:"title"`
		Excerpt string `json:"excerpt"`
	}
	notes := s.cfg.Vault.SnapshotNotes()
	type sortRec struct {
		n   *vault.Note
		mod time.Time
	}
	all := make([]sortRec, 0, len(notes))
	already := make(map[string]bool, len(orphan.Backlinks))
	for _, b := range orphan.Backlinks {
		already[b] = true
	}
	for _, n := range notes {
		if n.RelPath == orphanPath || already[n.RelPath] {
			continue
		}
		all = append(all, sortRec{n: n, mod: n.ModTime})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mod.After(all[j].mod) })
	if len(all) > 40 {
		all = all[:40]
	}
	pool := make([]cand, 0, len(all))
	for _, rec := range all {
		s.cfg.Vault.EnsureLoaded(rec.n.RelPath)
		ex := textutil.TruncateRunes(strings.TrimSpace(stripFrontmatterBody(rec.n.Content)), 220)
		pool = append(pool, cand{Path: rec.n.RelPath, Title: rec.n.Title, Excerpt: ex})
	}
	if len(pool) == 0 {
		return nil, nil
	}
	poolJSON, _ := json.Marshal(pool)
	userPrompt := fmt.Sprintf(
		"Orphan note:\npath: %s\ntitle: %s\nexcerpt:\n%s\n\nCandidate pool:\n```json\n%s\n```",
		orphan.RelPath, orphan.Title, excerpt, string(poolJSON))

	out, err := s.runAIFeature(ctx, aiprefs.FeatureMaintenanceOrphans, orphanSuggestSystemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}
	cleaned := stripJSONFences(out)
	var parsed []backlinkSuggestion
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, err
	}
	// Defensive filter: drop entries whose `from` isn't in the
	// candidate pool. Models occasionally invent paths even when
	// told not to.
	allowed := make(map[string]struct{}, len(pool))
	for _, c := range pool {
		allowed[c.Path] = struct{}{}
	}
	filtered := parsed[:0]
	for _, p := range parsed {
		if _, ok := allowed[p.From]; !ok {
			continue
		}
		filtered = append(filtered, p)
		if len(filtered) >= 3 {
			break
		}
	}
	return filtered, nil
}

// ─── Apply suggestion ────────────────────────────────────────────

// handleMaintenanceApplySuggestion accepts one suggestion (from
// either feed) and mutates the vault accordingly.
//
// Safety posture:
//   - "merge" returns 501 Not Implemented. Cross-note merges need
//     editorial judgement — silently concat+delete would lose data
//     and confuse the file history surface. Surfaced with a "next"
//     hint so the client can route the user to a manual combine
//     flow if it has one.
//   - "retitle" reuses the rename path (file move + annotations
//     rewrite + history snap).
//   - "missing-tags" merges into the existing frontmatter `tags`
//     array, preserving every existing tag.
//   - "add-backlink" appends `[[anchor]]` to a `## Related` block
//     in the source note, creating the block if it doesn't exist.
//     Deliberately dumb — the client tried to make it clever, the
//     server keeps it predictable.
func (s *Server) handleMaintenanceApplySuggestion(w http.ResponseWriter, r *http.Request) {
	var sg maintenanceSuggestion
	if err := json.NewDecoder(r.Body).Decode(&sg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	switch sg.Kind {
	case "merge":
		// Refuse — surface the longest as the "open this one" hint
		// so the client can show "open <longest> and paste in the
		// others manually". 200 with a next field rather than 501
		// because the caller still wants the suggestion dismissed
		// from the UI without a red error toast.
		longest := ""
		var longestLen int
		for _, p := range sg.Notes {
			if n := s.cfg.Vault.GetNote(p); n != nil {
				if l := len(n.Content); l > longestLen {
					longestLen = l
					longest = p
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":     true,
			"next":   "merge-prep",
			"target": longest,
		})
		return

	case "retitle":
		if err := s.applyRetitle(sg.Note, sg.SuggestedTitle); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return

	case "missing-tags":
		if err := s.applyMissingTags(sg.Note, sg.SuggestedTags); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return

	case "add-backlink":
		if err := s.applyAddBacklink(sg.FromNotePath, sg.ToNotePath, sg.AnchorText); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeError(w, http.StatusBadRequest, "unknown suggestion kind: "+sg.Kind)
}

// applyRetitle renames the file so its basename matches the new title.
// The folder + extension are preserved. Mirrors handleRenameNote's
// safety checks so the maintenance apply surface can't be used as a
// path-traversal vector.
func (s *Server) applyRetitle(notePath, newTitle string) error {
	if notePath == "" || strings.Contains(notePath, "..") || strings.HasPrefix(notePath, "/") {
		return fmt.Errorf("invalid path")
	}
	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return fmt.Errorf("empty title")
	}
	// Sanitise — the title becomes a filename. Strip path separators
	// and control characters; replace runs of whitespace with a
	// single space.
	clean := strings.Builder{}
	prevSpace := false
	for _, r := range newTitle {
		if r == '/' || r == '\\' || r < 0x20 {
			continue
		}
		if r == ' ' || r == '\t' {
			if !prevSpace {
				clean.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		clean.WriteRune(r)
	}
	newBase := strings.TrimSpace(clean.String())
	if newBase == "" {
		return fmt.Errorf("title becomes empty after sanitisation")
	}

	dir := filepath.Dir(notePath)
	to := newBase + ".md"
	if dir != "" && dir != "." {
		to = filepath.ToSlash(filepath.Join(dir, newBase+".md"))
	}
	if to == notePath {
		return nil // no-op
	}

	root := filepath.Clean(s.cfg.Vault.Root)
	fromAbs := filepath.Clean(filepath.Join(root, filepath.FromSlash(notePath)))
	toAbs := filepath.Clean(filepath.Join(root, filepath.FromSlash(to)))
	for _, p := range []string{fromAbs, toAbs} {
		if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
			return fmt.Errorf("path escapes vault")
		}
	}
	if _, err := os.Stat(fromAbs); err != nil {
		return fmt.Errorf("source not found")
	}
	if _, err := os.Stat(toAbs); err == nil {
		return fmt.Errorf("destination exists: %s", to)
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return err
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return err
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.search.Remove(notePath)
	if n := s.cfg.Vault.GetNote(to); n != nil {
		s.cfg.Vault.EnsureLoaded(to)
		s.search.Update(to, n.Content)
	}
	s.rescanMu.Unlock()
	s.hub.Broadcast(wshub.Event{Type: "note.removed", Path: notePath})
	s.hub.Broadcast(wshub.Event{Type: "note.changed", Path: to})
	return nil
}

// applyMissingTags merges the suggested tags into the note's
// frontmatter, preserving existing tags. Tags are normalised to
// lowercase and hyphenated form before merge so a model that emits
// "Foo Bar" doesn't create a near-duplicate of an existing "foo-bar".
func (s *Server) applyMissingTags(notePath string, newTags []string) error {
	if notePath == "" || strings.Contains(notePath, "..") || strings.HasPrefix(notePath, "/") {
		return fmt.Errorf("invalid path")
	}
	n := s.cfg.Vault.GetNote(notePath)
	if n == nil {
		return fmt.Errorf("note not found")
	}
	s.cfg.Vault.EnsureLoaded(notePath)

	// Existing tags — read from frontmatter only (inline #tags are
	// part of the body and we don't mutate body text here).
	fm := map[string]interface{}{}
	for k, v := range n.Frontmatter {
		fm[k] = v
	}
	existing := []string{}
	switch v := fm["tags"].(type) {
	case []interface{}:
		for _, t := range v {
			if s, ok := t.(string); ok {
				existing = append(existing, s)
			}
		}
	case string:
		for _, t := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			if t != "" {
				existing = append(existing, t)
			}
		}
	}
	have := make(map[string]bool, len(existing))
	for _, t := range existing {
		have[normaliseTag(t)] = true
	}
	merged := append([]string{}, existing...)
	for _, t := range newTags {
		nt := normaliseTag(t)
		if nt == "" || have[nt] {
			continue
		}
		have[nt] = true
		merged = append(merged, nt)
	}
	if len(merged) == len(existing) {
		return nil // nothing to add
	}
	asIface := make([]interface{}, len(merged))
	for i, t := range merged {
		asIface[i] = t
	}
	fm["tags"] = asIface

	body := stripFrontmatterBody(n.Content)
	content, err := serializeNote(fm, body)
	if err != nil {
		return err
	}
	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(notePath))
	// History snapshot before overwrite, same as PUT /notes/*.
	if prior, rerr := os.ReadFile(abs); rerr == nil {
		_, _ = history.Snap(s.cfg.Vault.Root, notePath, prior)
	}
	if err := atomicio.WriteNote(abs, content); err != nil {
		return err
	}
	if s.autocommit != nil {
		s.autocommit.Notify(notePath)
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	if nn := s.cfg.Vault.GetNote(notePath); nn != nil {
		s.cfg.Vault.EnsureLoaded(notePath)
		s.search.Update(notePath, nn.Content)
	}
	s.rescanMu.Unlock()
	s.hub.Broadcast(wshub.Event{Type: "note.changed", Path: notePath})
	return nil
}

// applyAddBacklink appends a `[[wikilink]]` to a `## Related` section
// at the end of fromNotePath, creating the section if it doesn't
// exist. The anchor text defaults to the target note's title.
//
// Deliberately conservative: we don't try to "insert near the
// excerpt" because (a) the model's excerpt is paraphrase-ish so
// finding it in the source is unreliable, and (b) a dumb append
// has predictable, reviewable behaviour, which matters more than
// surgical placement for a backlink whose purpose is graph
// connectivity, not prose flow.
func (s *Server) applyAddBacklink(fromPath, toPath, anchor string) error {
	for _, p := range []string{fromPath, toPath} {
		if p == "" || strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
			return fmt.Errorf("invalid path")
		}
	}
	src := s.cfg.Vault.GetNote(fromPath)
	if src == nil {
		return fmt.Errorf("source note not found: %s", fromPath)
	}
	dst := s.cfg.Vault.GetNote(toPath)
	if dst == nil {
		return fmt.Errorf("target note not found: %s", toPath)
	}
	s.cfg.Vault.EnsureLoaded(fromPath)

	label := strings.TrimSpace(anchor)
	if label == "" {
		label = dst.Title
	}
	// Wikilink target: use the basename without extension if the
	// vault tooling resolves by basename uniquely; otherwise fall
	// back to the full path. Cheap heuristic — if no other note
	// shares the basename we use the short form so the link reads
	// the same as a hand-typed one.
	base := strings.TrimSuffix(filepath.Base(toPath), filepath.Ext(toPath))
	target := base
	collision := false
	for path := range s.cfg.Vault.SnapshotNotes() {
		if path == toPath {
			continue
		}
		if strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) == base {
			collision = true
			break
		}
	}
	if collision {
		target = strings.TrimSuffix(toPath, filepath.Ext(toPath))
	}
	var link string
	if label == base || label == target {
		link = fmt.Sprintf("[[%s]]", target)
	} else {
		link = fmt.Sprintf("[[%s|%s]]", target, label)
	}

	// Idempotency — if the link already appears anywhere in the
	// source's body, do nothing rather than write a second copy.
	body := stripFrontmatterBody(src.Content)
	if strings.Contains(body, "[["+target+"]]") || strings.Contains(body, "[["+target+"|") {
		return nil
	}

	// Append into a `## Related` block. Walk the body line-by-line
	// to find the existing section; append the bullet under it.
	// Otherwise add the section at the end.
	hasRelated := false
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "## ") &&
			strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "## ")), "Related") {
			hasRelated = true
			break
		}
	}
	var newBody string
	bullet := "- " + link
	if hasRelated {
		// Find the `## Related` line and insert the bullet right
		// after it (before any next H2 / EOF).
		lines := strings.Split(body, "\n")
		inserted := false
		out := make([]string, 0, len(lines)+1)
		for i, line := range lines {
			out = append(out, line)
			if inserted {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "## ") &&
				strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "## ")), "Related") {
				// Skip blank lines that immediately follow the
				// heading, then insert the bullet so it lands
				// inside the section, not after the spacer.
				j := i + 1
				for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
					out = append(out, lines[j])
					j++
				}
				// Re-walk: we've now appended lines beyond i;
				// drop them and reconstruct so we can splice
				// the bullet at j.
				out = out[:i+1]
				for k := i + 1; k < j; k++ {
					out = append(out, lines[k])
				}
				out = append(out, bullet)
				for k := j; k < len(lines); k++ {
					out = append(out, lines[k])
				}
				inserted = true
				break
			}
		}
		newBody = strings.Join(out, "\n")
	} else {
		// Append a fresh section. Trim trailing whitespace so we
		// don't double-blank-line the file.
		newBody = strings.TrimRight(body, "\n") + "\n\n## Related\n\n" + bullet + "\n"
	}

	content, err := serializeNote(src.Frontmatter, newBody)
	if err != nil {
		return err
	}
	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(fromPath))
	if prior, rerr := os.ReadFile(abs); rerr == nil {
		_, _ = history.Snap(s.cfg.Vault.Root, fromPath, prior)
	}
	if err := atomicio.WriteNote(abs, content); err != nil {
		return err
	}
	if s.autocommit != nil {
		s.autocommit.Notify(fromPath)
	}
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	if nn := s.cfg.Vault.GetNote(fromPath); nn != nil {
		s.cfg.Vault.EnsureLoaded(fromPath)
		s.search.Update(fromPath, nn.Content)
	}
	s.rescanMu.Unlock()
	s.hub.Broadcast(wshub.Event{Type: "note.changed", Path: fromPath})
	return nil
}

// normaliseTag converts free-form input like "Project Mgmt" to
// "project-mgmt" so we don't create near-duplicate frontmatter
// tags. Mirrors the convention the rest of granit uses for
// existing tags.
func normaliseTag(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "#")
	t = strings.ToLower(t)
	var b strings.Builder
	prevDash := false
	for _, r := range t {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '\t':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

