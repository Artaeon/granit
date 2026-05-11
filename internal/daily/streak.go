package daily

import (
	"sort"
	"time"
)

// Streak reports how many consecutive calendar days the user has
// kept a daily-note habit. Two numbers worth showing:
//
//   - Current — the live run ending today (or yesterday, if the
//     user hasn't written today yet — today is "in progress",
//     not "broken"). When the day before yesterday is missing,
//     current resets to 0.
//   - Longest — the historical best, scanned across the full
//     dataset. Anchors progress against your own peak.
//
// LastDate is the most recent calendar day with a daily note
// (≤ today; future-dated drafts don't count). TodayLogged is the
// status-bar indicator that hits the "✓" state — distinct from
// the streak number, which lets a user with a 30-day run see at a
// glance whether today still owes them an entry.
type Streak struct {
	Current     int    `json:"current"`
	Longest     int    `json:"longest"`
	LastDate    string `json:"lastDate,omitempty"`
	TodayLogged bool   `json:"todayLogged"`
}

// ComputeStreak derives the current + longest run from a set of
// YYYY-MM-DD daily-note dates. Pure, dependency-free; the handler
// supplies `today` so tests can pin a deterministic reference and
// production passes time.Now().UTC().Format("2006-01-02").
//
// "Current" is forgiving: if today isn't logged but yesterday is,
// the streak still counts (today is in-progress, not broken). This
// matches every habit tracker that doesn't want to punish the user
// at midnight before they've had coffee.
func ComputeStreak(dates []string, today string) Streak {
	if len(dates) == 0 {
		return Streak{}
	}
	// Dedupe + drop future-dated entries. A user dating a draft
	// tomorrow shouldn't extend the streak — the streak is about
	// honoured habit days, not aspirational ones.
	seen := make(map[string]struct{}, len(dates))
	cleaned := make([]string, 0, len(dates))
	for _, d := range dates {
		if d == "" || d > today {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		cleaned = append(cleaned, d)
	}
	if len(cleaned) == 0 {
		return Streak{}
	}
	sort.Strings(cleaned)
	// LastDate is the chronologically latest entry on file.
	lastDate := cleaned[len(cleaned)-1]
	todayLogged := false
	if _, ok := seen[today]; ok {
		todayLogged = true
	}

	// Longest streak: scan the sorted slice, count consecutive
	// adjacencies (next == prev + 1 day). One O(N) pass with a
	// running counter is enough — no need to allocate sets.
	longest := 1
	run := 1
	for i := 1; i < len(cleaned); i++ {
		if nextDay(cleaned[i-1]) == cleaned[i] {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 1
		}
	}

	// Current streak: anchor at today (if logged) or yesterday (if
	// the user hasn't gotten to today yet). Walk back as long as
	// each previous calendar day is in the set.
	anchor := today
	if !todayLogged {
		anchor = prevDay(today)
		if _, ok := seen[anchor]; !ok {
			return Streak{Current: 0, Longest: longest, LastDate: lastDate, TodayLogged: false}
		}
	}
	current := 0
	for d := anchor; ; d = prevDay(d) {
		if _, ok := seen[d]; !ok {
			break
		}
		current++
	}

	return Streak{Current: current, Longest: longest, LastDate: lastDate, TodayLogged: todayLogged}
}

// nextDay / prevDay walk one calendar day relative to a
// YYYY-MM-DD string. Use time.Parse so month/year boundaries
// (Feb 28 → Mar 1, Dec 31 → Jan 1) come for free instead of being
// open-coded with mod arithmetic that gets leap-year wrong.
func nextDay(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func prevDay(d string) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}
