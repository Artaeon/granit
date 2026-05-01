package serveapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/daily"
)

// MorningSaveBody is the payload from the web wizard. All fields are
// optional; send only what the user filled in.
type MorningSaveBody struct {
	Scripture struct {
		Text   string `json:"text"`
		Source string `json:"source"`
	} `json:"scripture"`
	Goal     string   `json:"goal"`
	Tasks    []string `json:"tasks"`    // existing task texts the user committed to
	Habits   []string `json:"habits"`   // habit names to track today
	Thoughts string   `json:"thoughts"` // free-form morning reflection
}

// handleSaveMorning composes a "## Daily Plan" block matching granit's TUI
// morningroutine.go output, then upserts it into today's daily note.
//   - Section header: "## Daily Plan — Monday, January 2, 2026"
//   - Scripture as a blockquote
//   - "### Today's Goal" with bolded goal
//   - "### Tasks" as plain bullets (not checkboxes, intentionally — granit's
//     TUI reasons that turning these into "- [ ]" duplicates the original
//     task lines in TaskStore.All())
//   - "### Habits" as checkboxes (these ARE habit task lines, picked up by
//     the habits feature)
//   - "### Thoughts" as a paragraph
//
// If today's note already has a "## Daily Plan" block from a previous run,
// it's replaced. The rest of the daily note is preserved.
func (s *Server) handleSaveMorning(w http.ResponseWriter, r *http.Request) {
	var b MorningSaveBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// Ensure today's daily exists (granit's daily.EnsureDaily handles missing).
	cfg := s.dailyConfigFor()
	dailyPath, _, err := daily.EnsureDaily(s.cfg.Vault.Root, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("daily: %v", err))
		return
	}

	rawBytes, err := os.ReadFile(dailyPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	raw := string(rawBytes)

	plan := buildDailyPlan(b, time.Now())
	updated := upsertDailyPlan(raw, plan)

	if err := atomicio.WriteNote(dailyPath, updated); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Refresh in-memory state so subsequent reads see the new content.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()

	rel, err := filepath.Rel(s.cfg.Vault.Root, dailyPath)
	if err != nil {
		rel = dailyPath
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":  filepath.ToSlash(rel),
		"saved": true,
	})
}

// buildDailyPlan formats the section. Mirrors morningroutine.go's
// buildDailyPlanMarkdown but trimmed to the fields the web wizard collects.
func buildDailyPlan(b MorningSaveBody, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Daily Plan — %s\n\n", now.Format("Monday, January 2, 2006")))

	if t := strings.TrimSpace(b.Scripture.Text); t != "" {
		src := strings.TrimSpace(b.Scripture.Source)
		if src != "" {
			sb.WriteString(fmt.Sprintf("> *%q* — %s\n\n", t, src))
		} else {
			sb.WriteString(fmt.Sprintf("> *%q*\n\n", t))
		}
	}

	if g := strings.TrimSpace(b.Goal); g != "" {
		sb.WriteString("### Today's Goal\n\n")
		sb.WriteString(fmt.Sprintf("**%s**\n\n", g))
	}

	if len(b.Tasks) > 0 {
		sb.WriteString("### Tasks\n\n")
		for _, t := range b.Tasks {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			sb.WriteString("- ")
			sb.WriteString(t)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(b.Habits) > 0 {
		sb.WriteString("### Habits\n\n")
		for _, h := range b.Habits {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			sb.WriteString("- [ ] ")
			sb.WriteString(h)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if t := strings.TrimSpace(b.Thoughts); t != "" {
		sb.WriteString("### Thoughts\n\n")
		sb.WriteString(t)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// upsertDailyPlan inserts (or replaces) the "## Daily Plan" section in raw,
// preserving everything else. The section ends at the next "## " heading
// (or end-of-file).
func upsertDailyPlan(raw, plan string) string {
	const marker = "## Daily Plan"
	idx := strings.Index(raw, marker)
	if idx < 0 {
		// Append; ensure single blank line separator.
		raw = strings.TrimRight(raw, "\n")
		if raw != "" {
			raw += "\n\n"
		}
		return raw + plan
	}
	// Find the end of the existing section: the next "\n## " (top-level
	// heading) or EOF.
	rest := raw[idx:]
	end := -1
	for i := 0; i < len(rest); {
		nl := strings.IndexByte(rest[i:], '\n')
		var line string
		if nl < 0 {
			line = rest[i:]
			i = len(rest)
		} else {
			line = rest[i : i+nl+1]
			i += nl + 1
		}
		// Skip the first heading line (it's our section header)
		if i-len(line) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimRight(line, "\n"), "## ") &&
			!strings.HasPrefix(strings.TrimRight(line, "\n"), "### ") {
			end = i - len(line)
			break
		}
	}
	before := raw[:idx]
	if end < 0 {
		// Section runs to EOF.
		return strings.TrimRight(before, "\n") + "\n\n" + plan
	}
	after := rest[end:]
	return strings.TrimRight(before, "\n") + "\n\n" + plan + after
}

// silence linter if no other consumer
var _ = errors.New
