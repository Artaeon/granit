package tui

// Command-palette frecency: a small persistent log of which commands the
// user has invoked, when, and how often. Used to:
//
//   - Rank empty-query results so frequently/recently used commands surface
//     to the top instead of being buried in declaration order.
//   - Boost matching results when the user types a partial query, so
//     "Daily Note" beats "Daily Briefing" if the user actually uses the
//     former more often.
//
// Stored at ~/.config/granit/command-history.json (user-global, not per-
// vault) so a power user gets the same boosted ordering across vaults.

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// commandUsage is the per-command tally persisted to disk.
type commandUsage struct {
	LastUsed time.Time `json:"last_used"`
	Count    int       `json:"count"`
}

// CommandHistory tracks usage of each CommandAction. Methods are safe to
// call when the receiver is nil (silent no-op) so the palette can run
// without history when the on-disk file is unreadable.
type CommandHistory struct {
	// Keyed by the JSON-encoded numeric CommandAction. Strings are easier
	// to serialise and tolerate enum reordering across releases (the file
	// records an unknown int → ignored on load, no upgrade pain).
	Usage map[string]commandUsage `json:"usage"`
}

// commandHistoryPath returns the absolute path to the on-disk store.
func commandHistoryPath() string {
	return filepath.Join(config.ConfigDir(), "command-history.json")
}

// LoadCommandHistory reads the persisted history. Returns an empty (but
// non-nil) history when the file is missing or unreadable — callers
// should treat that as "no recents yet."
func LoadCommandHistory() *CommandHistory {
	h := &CommandHistory{Usage: map[string]commandUsage{}}
	data, err := os.ReadFile(commandHistoryPath())
	if err != nil {
		return h
	}
	_ = json.Unmarshal(data, h)
	if h.Usage == nil {
		h.Usage = map[string]commandUsage{}
	}
	return h
}

// Save persists the history to disk. Best-effort — any error (no config
// dir, read-only fs) is swallowed because losing the recents log on a
// single command invocation isn't worth interrupting the user.
func (h *CommandHistory) Save() {
	if h == nil {
		return
	}
	dir := config.ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteState(commandHistoryPath(), data)
}

// Record bumps usage for an action. Safe on nil receiver.
func (h *CommandHistory) Record(action CommandAction) {
	if h == nil {
		return
	}
	if h.Usage == nil {
		h.Usage = map[string]commandUsage{}
	}
	key := commandKey(action)
	u := h.Usage[key]
	u.Count++
	u.LastUsed = time.Now()
	h.Usage[key] = u
}

// FrecencyScore returns 0..1000-ish for an action — higher = more
// "frecent". Combines log(count) with an exponential recency decay
// (half-life ≈ 7 days). 0 means the command has never been used.
//
// The shape is deliberately bounded so frecency adds a *boost* on top
// of fuzzy-match scoring rather than overwhelming it; a very-popular
// command shouldn't always win when the user typed something specific.
func (h *CommandHistory) FrecencyScore(action CommandAction) int {
	if h == nil {
		return 0
	}
	u, ok := h.Usage[commandKey(action)]
	if !ok || u.Count <= 0 {
		return 0
	}
	// Recency: 1.0 today, 0.5 at 7 days, 0.25 at 14 days, asymptotic 0.
	ageDays := time.Since(u.LastUsed).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	recency := math.Pow(0.5, ageDays/7)
	// Frequency: log curve so a 100-use command isn't 100× a 1-use one.
	freq := math.Log1p(float64(u.Count))
	return int(200 * recency * freq)
}

func commandKey(action CommandAction) string {
	return strconv.Itoa(int(action))
}
