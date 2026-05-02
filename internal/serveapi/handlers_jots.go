package serveapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// Jots — Amplenote-style infinite-scroll feed of every daily note in the
// vault, newest first. The /jots web view paginates through this endpoint
// rather than fetching N daily notes individually so the round-trip cost
// stays at one request per page (default 20) regardless of how many years
// of dailies the user has accumulated.
//
// Implementation notes
// ────────────────────
//   - We pull the daily folder from config.LoadForVault on every request.
//     This keeps Jots in lockstep with the user's settings page (and the
//     TUI) without having to wire a refresh hook through Server. The
//     read is cheap — a JSON file in .granit/.
//   - Empty days are skipped, not placeholder-rendered. Amplenote shows
//     placeholders too but they clutter the feed for users who don't write
//     every day. The web's <input type="date"> provides a fast jump-to-day
//     for missing dates.

type jotEntry struct {
	Date        string                 `json:"date"`
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	ModTime     time.Time              `json:"modTime"`
	Size        int64                  `json:"size"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body"`
	OpenTasks   int                    `json:"openTasks"`
}

// jotPathRegex returns a compiled regex that matches a daily-note relative
// path for the given folder. Empty folder = vault root. Group 1 captures
// the YYYY-MM-DD date.
func jotPathRegex(folder string) *regexp.Regexp {
	folder = strings.Trim(folder, "/")
	if folder == "" {
		return regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
	}
	return regexp.MustCompile(`^` + regexp.QuoteMeta(folder) + `/(\d{4}-\d{2}-\d{2})\.md$`)
}

func (s *Server) handleListJots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	before := strings.TrimSpace(q.Get("before"))
	limit := 20
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > 50 {
		limit = 50
	}

	// Refresh the daily folder from the vault config on each request so
	// the user can change "Daily Notes Folder" in /settings and see
	// Jots respect it without a server restart.
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	folder := strings.Trim(cfg.DailyNotesFolder, "/")
	re := jotPathRegex(folder)

	// 1. Walk vault snapshot, collect daily notes.
	type candidate struct {
		date string
		note *vault.Note
	}
	cands := make([]candidate, 0, 64)
	for _, n := range s.cfg.Vault.SnapshotNotes() {
		m := re.FindStringSubmatch(n.RelPath)
		if m == nil {
			continue
		}
		cands = append(cands, candidate{date: m[1], note: n})
	}

	// 2. Sort newest-first by date.
	sort.Slice(cands, func(i, j int) bool { return cands[i].date > cands[j].date })

	// 3. Apply the `before` cursor — exclusive — so the next page picks
	//    up cleanly from where the previous one ended.
	if before != "" {
		filtered := make([]candidate, 0, len(cands))
		for _, c := range cands {
			if c.date < before {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
	}

	// 4. Bucket open tasks by note path once; per-jot indexing is then O(1).
	openByPath := map[string]int{}
	for _, t := range s.cfg.TaskStore.All() {
		if t.Done {
			continue
		}
		openByPath[t.NotePath]++
	}

	// 5. Page slice.
	hasMore := len(cands) > limit
	if hasMore {
		cands = cands[:limit]
	}

	// 6. Build the response, loading content for each candidate.
	out := make([]jotEntry, 0, len(cands))
	for _, c := range cands {
		s.cfg.Vault.EnsureLoaded(c.note.RelPath)
		out = append(out, jotEntry{
			Date:        c.date,
			Path:        c.note.RelPath,
			Title:       c.note.Title,
			ModTime:     c.note.ModTime,
			Size:        c.note.Size,
			Frontmatter: c.note.Frontmatter,
			Body:        stripFrontmatterBody(c.note.Content),
			OpenTasks:   openByPath[c.note.RelPath],
		})
	}

	var nextBefore *string
	if hasMore && len(out) > 0 {
		earliest := out[len(out)-1].Date
		nextBefore = &earliest
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jots":       out,
		"nextBefore": nextBefore,
		"hasMore":    hasMore,
	})
}
