package lectionary

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// State is the on-disk envelope under <vault>/.granit/lectionary-state.json.
// Tracks which plans the user has "started" and when. A plan is just a
// catalogue entry (lectionary.go) until it appears here — at that point
// "day N of the plan" becomes computable from StartedAt + today.
//
// Schema is intentionally minimal: no completion ticks, no per-day
// done/skip flags. Whether the user actually read day 47 is tracked by
// the existing biblereading streak (which fires on any chapter open).
// Mixing per-day completion in here would duplicate that surface and
// invite the question "what's the source of truth for 'I read today'"
// — there's already one answer, and it's biblereading.
type State struct {
	Active []ActivePlan `json:"active"`
}

// ActivePlan = "the user started plan X on day Y". Time is RFC3339
// (json.Marshal default for time.Time), serialised at second precision.
// dayOfPlan(now) is computed as int(now.Sub(StartedAt).Hours()/24)+1 —
// see DayOfPlan below.
type ActivePlan struct {
	PlanID    string    `json:"plan_id"`
	StartedAt time.Time `json:"started_at"`
}

// stateMu serialises read-modify-write on the state file. StartPlan /
// StopPlan both load → mutate → save, and a user opening the /scripture/plans
// page in two tabs could race the same way bible-reading does. Same
// idiom as biblereading.logMu.
var stateMu sync.Mutex

func statePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "lectionary-state.json")
}

// LoadState returns the current state, or an empty State{} when the
// file doesn't exist (a fresh vault has no active plans — that's a
// valid state, not an error). Hard parse errors do propagate so the
// caller can decide whether to surface or fall back.
func LoadState(vaultRoot string) (*State, error) {
	raw, err := os.ReadFile(statePath(vaultRoot))
	if errors.Is(err, fs.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveState writes the state atomically via atomicio.WriteState (0o600,
// crash-safe rename). Ensures the .granit/ directory exists since a
// brand-new vault won't have it yet — same dance biblereading.Save uses.
func SaveState(vaultRoot string, s *State) error {
	if s == nil {
		s = &State{}
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(statePath(vaultRoot), raw)
}

// StartPlan marks the given plan as active, starting NOW. If the plan
// is already active, its StartedAt is REPLACED — this matches a user's
// expectation that hitting "start" again means "restart from today",
// not "no-op, you already started." A restart use case: the user fell
// off the wagon two weeks in and wants to begin again on day 1.
//
// Validates that planID exists in the catalogue. Unknown ids return
// an error rather than silently writing a useless entry to disk.
func StartPlan(vaultRoot, planID string) error {
	if _, ok := Get(planID); !ok {
		return errors.New("lectionary: unknown plan id: " + planID)
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	s, err := LoadState(vaultRoot)
	if err != nil {
		return err
	}
	now := time.Now()
	for i := range s.Active {
		if s.Active[i].PlanID == planID {
			s.Active[i].StartedAt = now
			return SaveState(vaultRoot, s)
		}
	}
	s.Active = append(s.Active, ActivePlan{PlanID: planID, StartedAt: now})
	return SaveState(vaultRoot, s)
}

// StopPlan removes an active plan entry. Idempotent — stopping an
// already-stopped plan is a no-op success, not an error (matches the
// REST DELETE convention where 204-on-already-gone keeps clients
// simple).
func StopPlan(vaultRoot, planID string) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	s, err := LoadState(vaultRoot)
	if err != nil {
		return err
	}
	out := s.Active[:0]
	for _, a := range s.Active {
		if a.PlanID == planID {
			continue
		}
		out = append(out, a)
	}
	s.Active = out
	return SaveState(vaultRoot, s)
}

// DayOfPlan computes the 1-indexed day of the plan for the given
// reference time. StartedAt's calendar date is "day 1"; the next
// calendar day is day 2; etc.
//
// We compare LOCAL calendar dates, not raw .Hours()/24, so a user who
// started at 23:50 doesn't tick into "day 2" at 00:00 the same night —
// they tick at midnight of the FOLLOWING day, which is what "day N of
// the plan" means in everyday speech. Pure function; safe to call on
// every request without touching disk.
func DayOfPlan(active ActivePlan, when time.Time) int {
	start := startOfLocalDay(active.StartedAt)
	now := startOfLocalDay(when)
	diff := now.Sub(start)
	if diff < 0 {
		return 1
	}
	return int(diff.Hours()/24) + 1
}

func startOfLocalDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
