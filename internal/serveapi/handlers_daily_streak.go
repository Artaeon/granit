package serveapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/daily"
)

// handleDailyStreak returns the current + longest consecutive-day
// streak for the user's daily notes. Status-bar feedback for the
// "did I write today?" question without the user having to scroll
// the Jots feed.
//
// Scopes the discovery to whatever DailyNotesFolder is in vault
// config — same regex shape Jots uses — so a user who later changes
// the folder picks up automatically.
func (s *Server) handleDailyStreak(w http.ResponseWriter, r *http.Request) {
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	folder := strings.Trim(cfg.DailyNotesFolder, "/")
	re := jotPathRegex(folder)

	dates := make([]string, 0, 64)
	for _, n := range s.cfg.Vault.SnapshotNotes() {
		m := re.FindStringSubmatch(n.RelPath)
		if m == nil {
			continue
		}
		dates = append(dates, m[1])
	}
	// time.Now().Local() so "today" matches the user's wall clock —
	// the daily-note filename uses local date semantics (see
	// daily.GetDailyPath). Using UTC here would produce a wrong
	// boundary for users east of UTC after midnight local time.
	today := time.Now().Local().Format("2006-01-02")
	writeJSON(w, http.StatusOK, daily.ComputeStreak(dates, today))
}
