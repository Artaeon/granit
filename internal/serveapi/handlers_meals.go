package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/meals"
	"github.com/artaeon/granit/internal/wshub"
)

// Meals — lightweight per-day eating plan. Source of truth is the
// daily note's `## Meals` section; the meals package owns the parse +
// render. This file is the HTTP layer: it resolves the daily-note
// path for a given date, reads the section, merges with the user's
// default slot list, and writes back atomically on PATCH.
//
// Why this lives here and not in a TUI-shared package: the feature
// is web-first, and the TUI can pick it up later by reading the same
// markdown. Habits did the same migration — the package-level parser
// is the contract.

// mealsGetResponse is the wire shape for GET /api/v1/meals?date=...
// — already merged-with-defaults so the client renders exactly what's
// returned without re-applying defaults itself.
type mealsGetResponse struct {
	Date  string       `json:"date"`
	Slots []meals.Slot `json:"slots"`
	Done  int          `json:"done"`
	Total int          `json:"total"`
}

// mealsPatchBody upserts a single slot identified by (Time, Name).
// Name is optional — only needed to disambiguate when two slots share
// a time (rare). Done / Text are pointers so the patcher can tell
// "explicitly set false / empty string" apart from "field omitted".
type mealsPatchBody struct {
	Date string  `json:"date,omitempty"`
	Time string  `json:"time"`
	Name string  `json:"name,omitempty"`
	Done *bool   `json:"done,omitempty"`
	Text *string `json:"text,omitempty"`
}

// handleListMeals returns the rendered slot list for a date. Defaults
// to today when ?date is missing or blank; rejects malformed dates so
// the client can surface a clear error.
func (s *Server) handleListMeals(w http.ResponseWriter, r *http.Request) {
	dateISO := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateISO == "" {
		dateISO = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", dateISO); err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	body := s.readDailyBody(dateISO)
	slots := meals.MergeWithDefaults(meals.Parse(body), meals.DefaultSlots())
	done, total := meals.Aggregate(slots)
	writeJSON(w, http.StatusOK, mealsGetResponse{
		Date:  dateISO,
		Slots: slots,
		Done:  done,
		Total: total,
	})
}

// handlePatchMeals upserts a single slot in the daily note's Meals
// section. Creates the daily note + section if missing so the user's
// first tick of the day Just Works without a preliminary save call.
func (s *Server) handlePatchMeals(w http.ResponseWriter, r *http.Request) {
	var b mealsPatchBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	b.Time = strings.TrimSpace(b.Time)
	if b.Time == "" {
		writeError(w, http.StatusBadRequest, "time required")
		return
	}
	// Normalise + validate the HH:MM time. Accept "8:00" or "08:00",
	// reject anything else. Without normalisation a client sending
	// "8:00" would not match the canonical "08:00" default and we'd
	// silently append a duplicate slot — confusing on the dashboard
	// and impossible for the user to clean up from the widget.
	//
	// Explicit split-and-parse (instead of time.Parse) because Go's
	// time layout matching is subtly lenient about leading zeros in
	// different ways depending on whether you use "15" (24h) or "3"
	// (12h) — explicit int-parse is unambiguous and rejects garbage
	// like "23:5" or "24:00" cleanly.
	parts := strings.Split(b.Time, ":")
	if len(parts) == 2 {
		hh, herr := strconv.Atoi(parts[0])
		mm, merr := strconv.Atoi(parts[1])
		if herr == nil && merr == nil && hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59 && len(parts[1]) == 2 {
			b.Time = fmt.Sprintf("%02d:%02d", hh, mm)
		} else {
			writeError(w, http.StatusBadRequest, "time must be HH:MM (24h)")
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "time must be HH:MM (24h)")
		return
	}
	if b.Done == nil && b.Text == nil {
		writeError(w, http.StatusBadRequest, "nothing to patch (need done and/or text)")
		return
	}

	dateISO := strings.TrimSpace(b.Date)
	if dateISO == "" {
		dateISO = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", dateISO); err != nil {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}

	// Resolve / create the daily note. Today uses EnsureDaily (creates
	// from the template if needed); past dates resolve to the
	// conventional path and stub an empty file when missing — same
	// shape examen uses.
	cfg := s.dailyConfigFor()
	dailyPath, err := s.resolveMealsDaily(cfg, dateISO)
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

	// If the client omitted Name (calendar click sends time+date only
	// because the rendered title contains "— text" suffix), resolve
	// it from the parsed slot at that time, or fall back to the
	// matching default. Without this, ApplyPatch's append-missing
	// path would create a Slot with Name="" which renders as
	// "- [x] 12:30  — sandwich" (note the double space) and Parse
	// reads it back with garbage. Found via concurrent PATCH stress
	// test.
	if b.Name == "" {
		for _, s := range meals.Parse(raw) {
			if s.Time == b.Time {
				b.Name = s.Name
				break
			}
		}
	}
	if b.Name == "" {
		for _, d := range meals.DefaultSlots() {
			if d.Time == b.Time {
				b.Name = d.Name
				break
			}
		}
	}
	if b.Name == "" {
		// Time isn't a default and isn't materialised — refuse
		// rather than create an anonymous row. The client should
		// pass a name for custom slots.
		writeError(w, http.StatusBadRequest, "name required for non-default time")
		return
	}

	// Patch against the *parsed* slots only (not merged-with-defaults).
	// ApplyPatch's append-missing path materialises just the targeted
	// slot, so a single tick writes one row instead of stamping all
	// three defaults into the daily note. This matters most for past-
	// day back-fills ("I had lunch yesterday") where stamping empty
	// Breakfast/Dinner ghost rows would be surprising. For today the
	// behaviour is identical from the user's POV — the GET response
	// still merges in defaults, so the widget renders the full list.
	parsed := meals.Parse(raw)
	updated, changed := meals.ApplyPatch(parsed, b.Time, b.Name, b.Done, b.Text)
	if !changed {
		// Idempotent no-op — return the merged view so the client can
		// reconcile against canonical data (defaults included).
		merged := meals.MergeWithDefaults(parsed, meals.DefaultSlots())
		done, total := meals.Aggregate(merged)
		writeJSON(w, http.StatusOK, mealsGetResponse{
			Date:  dateISO,
			Slots: merged,
			Done:  done,
			Total: total,
		})
		return
	}

	// WriteSection rewrites the Meals section line-by-line so the
	// user's free-form notes inside the section (e.g. a journal line
	// about a meal) survive the round-trip. The old RenderSection +
	// upsertNamedSection path discarded non-meal lines silently — a
	// data-loss bug we don't tolerate for a daily-driver tool.
	rewritten := meals.WriteSection(raw, updated)
	if err := atomicio.WriteNote(dailyPath, rewritten); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Refresh in-memory state so subsequent reads see the new content.
	// Meal rows aren't tasks (the parser's section-skip excludes them),
	// but ScanFast keeps the snapshot fresh for habits/tasks adjacent
	// to the section we just edited.
	s.rescanMu.Lock()
	_ = s.cfg.Vault.ScanFast()
	_ = s.cfg.TaskStore.Reload()
	s.rescanMu.Unlock()

	// Broadcast so other tabs/devices re-fetch. The daily note's
	// relative path is the most precise signal — listeners that care
	// only about today already filter on it.
	rel, err := filepath.Rel(s.cfg.Vault.Root, dailyPath)
	if err != nil {
		rel = dailyPath
	}
	s.hub.Broadcast(wshub.Event{Type: "note.changed", Path: filepath.ToSlash(rel)})

	// Response mirrors the GET shape: merge the freshly-written slots
	// with the user's defaults so the client redraws the full row
	// list (not just the one we touched).
	merged := meals.MergeWithDefaults(updated, meals.DefaultSlots())
	done, total := meals.Aggregate(merged)
	writeJSON(w, http.StatusOK, mealsGetResponse{
		Date:  dateISO,
		Slots: merged,
		Done:  done,
		Total: total,
	})
}

// dailyPathFor returns the absolute path of the daily note for a
// given date. Matches daily.GetDailyPath's logic exactly: when
// cfg.Folder is empty the note lives at the vault root (the granit
// default), otherwise inside the configured subfolder. Hard-coding
// "Daily" as a fallback was the bug that made PATCHes invisible to
// subsequent GETs on default vaults — EnsureDaily wrote at the
// root while GET tried to read from /Daily.
func (s *Server) dailyPathFor(cfg daily.DailyConfig, dateISO string) string {
	filename := dateISO + ".md"
	folder := strings.TrimRight(cfg.Folder, "/")
	if folder == "" {
		return filepath.Join(s.cfg.Vault.Root, filename)
	}
	return filepath.Join(s.cfg.Vault.Root, folder, filename)
}

// readDailyBody reads the daily note for a given date and returns its
// raw markdown body. Missing file = empty string — callers treat that
// as "no meals yet" (defaults will fill in).
func (s *Server) readDailyBody(dateISO string) string {
	abs := s.dailyPathFor(s.dailyConfigFor(), dateISO)
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	return string(data)
}

// resolveMealsDaily picks the daily-note absolute path for the target
// date. Today defaults to daily.EnsureDaily (creates from template);
// other dates resolve conventionally and stub an empty file when
// missing so the upsert has something to write into.
func (s *Server) resolveMealsDaily(cfg daily.DailyConfig, dateISO string) (string, error) {
	today := time.Now().Format("2006-01-02")
	if dateISO == today {
		path, _, err := daily.EnsureDaily(s.cfg.Vault.Root, cfg)
		return path, err
	}
	abs := s.dailyPathFor(cfg, dateISO)
	if _, statErr := os.Stat(abs); statErr != nil {
		if dir := filepath.Dir(abs); dir != s.cfg.Vault.Root {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
		}
		if err := os.WriteFile(abs, []byte{}, 0o644); err != nil {
			return "", err
		}
	}
	return abs, nil
}
