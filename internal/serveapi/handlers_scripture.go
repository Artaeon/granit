package serveapi

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/scripture"
)

// handleListScriptures returns the user's full verse set (built-in
// defaults if .granit/scriptures.md is absent or empty). The web's
// quiz mode pulls from this list directly so it can sample without
// replacement and track per-verse accuracy locally.
func (s *Server) handleListScriptures(w http.ResponseWriter, r *http.Request) {
	all := scripture.Load(s.cfg.Vault.Root)
	writeJSON(w, http.StatusOK, map[string]any{
		"scriptures": all,
		"total":      len(all),
	})
}

// handleDailyScripture returns the deterministic verse-of-the-day. Same
// rotation function as the TUI uses, so a phone and a laptop on the
// same vault see the same verse on the same day. Idempotent on every
// request within a calendar day.
func (s *Server) handleDailyScripture(w http.ResponseWriter, r *http.Request) {
	today := scripture.Daily(s.cfg.Vault.Root)
	writeJSON(w, http.StatusOK, today)
}

// handleRandomScripture returns one verse chosen uniformly at random.
// Used by the "another one" button on the scripture page when the
// user wants more than the one daily verse. Optional ?seed=<int> makes
// it deterministic for testing.
func (s *Server) handleRandomScripture(w http.ResponseWriter, r *http.Request) {
	all := scripture.Load(s.cfg.Vault.Root)
	if len(all) == 0 {
		writeJSON(w, http.StatusOK, scripture.Defaults()[0])
		return
	}
	if seedStr := r.URL.Query().Get("seed"); seedStr != "" {
		if seed, err := strconv.ParseInt(seedStr, 10, 64); err == nil {
			rng := rand.New(rand.NewSource(seed))
			writeJSON(w, http.StatusOK, all[rng.Intn(len(all))])
			return
		}
	}
	writeJSON(w, http.StatusOK, all[rand.Intn(len(all))])
}

// handleSaveDevotional creates a Devotionals/{date}-{slug}.md note
// pre-seeded with the verse text + an optional reflection block. The
// "reflect on this" button on the scripture page POSTs here; the
// server returns the new note path so the UI can navigate the user
// straight into editing.
//
// We intentionally don't run the AI here — a separate /agents/run
// call with a "devotional" preset can fill in a generated reflection.
// Keeping the two endpoints orthogonal makes it cheap to skip the AI
// step when the user just wants a blank reflection page.
type devotionalRequest struct {
	Verse      string `json:"verse"`      // text of the verse being reflected on
	Source     string `json:"source"`     // citation, e.g. "Proverbs 3:5-6"
	Reflection string `json:"reflection"` // optional pre-filled body
}

func (s *Server) handleCreateDevotional(w http.ResponseWriter, r *http.Request) {
	var body devotionalRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Verse) == "" {
		writeError(w, http.StatusBadRequest, "verse required")
		return
	}

	stamp := time.Now().Format("2006-01-02")
	slug := slugifyForDevotional(body.Source)
	if slug == "" {
		slug = "reflection"
	}
	rel := fmt.Sprintf("Devotionals/%s-%s.md", stamp, slug)

	note := buildDevotionalNote(body, stamp)
	abs := s.cfg.Vault.Root + "/" + rel
	if err := atomicio.WriteNote(abs, note); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"path":  rel,
		"title": stamp + " — " + firstNonEmpty(body.Source, "reflection"),
	})
}

func buildDevotionalNote(req devotionalRequest, stamp string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: devotional\n")
	fmt.Fprintf(&b, "date: %s\n", stamp)
	if src := strings.TrimSpace(req.Source); src != "" {
		fmt.Fprintf(&b, "source: %q\n", src)
	}
	b.WriteString("tags: [devotional]\n")
	b.WriteString("---\n\n")

	if src := strings.TrimSpace(req.Source); src != "" {
		fmt.Fprintf(&b, "# %s\n\n", src)
	} else {
		b.WriteString("# Reflection\n\n")
	}

	// Verse rendered as a blockquote — markdown-standard, looks right
	// in both the editor and the preview pane.
	for _, line := range strings.Split(strings.TrimSpace(req.Verse), "\n") {
		fmt.Fprintf(&b, "> %s\n", strings.TrimSpace(line))
	}
	b.WriteString("\n")

	if r := strings.TrimSpace(req.Reflection); r != "" {
		b.WriteString("## Reflection\n\n")
		b.WriteString(r)
		b.WriteString("\n")
	} else {
		b.WriteString("## Reflection\n\n")
		b.WriteString("_what does this verse mean for me today?_\n")
	}
	return b.String()
}

// slugifyForDevotional turns "Proverbs 3:5-6" into "proverbs-3-5-6"
// for the filename. Lowercase, ascii-only, hyphens between tokens.
// Plenty good enough for filesystem identifiers; we don't try to be
// clever about Unicode.
func slugifyForDevotional(s string) string {
	var b strings.Builder
	prevDash := true
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
			prevDash = false
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
