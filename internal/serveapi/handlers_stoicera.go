package serveapi

import (
	cryptorand "crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/tasks"
)

// ---------------------------------------------------------------------------
// Stoicera intranet integration — exposed at /api/v1/integrations/stoicera/*
// ---------------------------------------------------------------------------
//
// Read-only API surface that the stoicera-intranet web app calls to sync
// the user's projects / tasks / goals belonging to a specific venture.
// Authenticated via a Bearer token distinct from the granit session
// token, so the intranet doesn't get blanket vault access.
//
// All endpoints follow the same gate:
//
//   - Feature disabled → 404 (not 401) so the existence of this
//     integration leaks no info if granit sits behind a reverse proxy
//   - Feature enabled, token missing/wrong → 401
//   - Feature enabled, token valid → request flows through
//
// Settings live alongside other vault-side sidecars:
//   <vault>/.granit/stoicera-integration.json
// Mirrors the autocommit / push-subscriptions pattern so we don't have
// to plumb user-level config (TUI's ~/.config/granit/config.json)
// through the server.

// stoiceraSettings is the on-disk shape of the integration config.
// Empty Token + Enabled=false is the natural default — caller must
// explicitly enable + (re)generate.
type stoiceraSettings struct {
	Enabled      bool   `json:"enabled"`
	Token        string `json:"token,omitempty"`
	VentureName  string `json:"venture_name,omitempty"`
}

func stoiceraSettingsPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "stoicera-integration.json")
}

// loadStoiceraSettings reads the sidecar. Missing file or unreadable
// → zero value (disabled). Errors are squashed: never want to
// accidentally enable the feature because of a parse hiccup.
func loadStoiceraSettings(vaultRoot string) stoiceraSettings {
	data, err := os.ReadFile(stoiceraSettingsPath(vaultRoot))
	if err != nil {
		return stoiceraSettings{}
	}
	var s stoiceraSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return stoiceraSettings{}
	}
	return s
}

func saveStoiceraSettings(vaultRoot string, s stoiceraSettings) error {
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// 0o600 — only the running user reads the token. Token is high-
	// sensitivity (full read access to project/task/goal data within
	// the configured venture), so file mode matters.
	return os.WriteFile(stoiceraSettingsPath(vaultRoot), data, 0o600)
}

// generateStoiceraToken returns a fresh 32-char hex token (128 bits
// of entropy). Same convention as config.GenerateIntegrationToken but
// lives in serveapi so the regenerate-on-save handler doesn't import
// the TUI's config package.
func generateStoiceraToken() (string, error) {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// requireStoiceraToken is the middleware for /api/v1/integrations/stoicera/*.
// Resolves disabled → 404 (silently); missing/bad token → 401.
//
// Uses subtle.ConstantTimeCompare to avoid leaking timing info that
// could narrow the token search space.
func (s *Server) requireStoiceraToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := loadStoiceraSettings(s.cfg.Vault.Root)
		if !settings.Enabled || strings.TrimSpace(settings.Token) == "" {
			// Make disabled indistinguishable from "endpoint never
			// existed" — opt-in features should not leak their
			// existence behind a reverse proxy.
			http.NotFound(w, r)
			return
		}
		got := bearerFromHeader(r)
		if got == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(settings.Token)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Settings endpoints — the granit UI reads / writes the config
// ---------------------------------------------------------------------------

// handleGetStoiceraSettings returns the current settings to the
// granit UI. The granit UI authenticates via the regular session
// token (requireToken), NOT via the integration token — those are
// separate things.
//
// The token field is masked except for the first 6 chars so it
// renders inline without splattering the secret across the screen;
// the UI offers a "copy to clipboard" affordance for the full value.
func (s *Server) handleGetStoiceraSettings(w http.ResponseWriter, r *http.Request) {
	cur := loadStoiceraSettings(s.cfg.Vault.Root)
	masked := ""
	if cur.Token != "" {
		if len(cur.Token) > 8 {
			masked = cur.Token[:6] + "…" + cur.Token[len(cur.Token)-2:]
		} else {
			masked = "set"
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":      cur.Enabled,
		"venture_name": cur.VentureName,
		"token_masked": masked,
		"has_token":    cur.Token != "",
	})
}

// handleGetStoiceraToken returns the full unmasked token so the UI
// can "copy to clipboard" on demand. Separate endpoint from the
// settings GET so the token doesn't ride in every status poll.
func (s *Server) handleGetStoiceraToken(w http.ResponseWriter, r *http.Request) {
	cur := loadStoiceraSettings(s.cfg.Vault.Root)
	if cur.Token == "" {
		writeError(w, http.StatusNotFound, "no token set")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": cur.Token})
}

// stoiceraSettingsPatch is the request body for PATCH /settings —
// fields are optional pointers so the UI can patch one at a time.
// regenerate=true generates a fresh token (only valid when enabled).
type stoiceraSettingsPatch struct {
	Enabled     *bool   `json:"enabled,omitempty"`
	VentureName *string `json:"venture_name,omitempty"`
	Regenerate  bool    `json:"regenerate,omitempty"`
}

// handlePatchStoiceraSettings updates the on-disk settings. Generates
// the token on first-enable (so the user gets a token immediately)
// and on Regenerate. Empty venture name closes the door — the
// integration handlers refuse to serve when VentureName is blank.
func (s *Server) handlePatchStoiceraSettings(w http.ResponseWriter, r *http.Request) {
	var p stoiceraSettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur := loadStoiceraSettings(s.cfg.Vault.Root)
	if p.Enabled != nil {
		cur.Enabled = *p.Enabled
	}
	if p.VentureName != nil {
		cur.VentureName = strings.TrimSpace(*p.VentureName)
	}
	// First-time enable without an existing token: generate one so
	// the UI has something to show / copy immediately.
	if cur.Enabled && cur.Token == "" {
		tok, err := generateStoiceraToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token generation failed: "+err.Error())
			return
		}
		cur.Token = tok
	}
	if p.Regenerate && cur.Enabled {
		tok, err := generateStoiceraToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token generation failed: "+err.Error())
			return
		}
		cur.Token = tok
	}
	if err := saveStoiceraSettings(s.cfg.Vault.Root, cur); err != nil {
		writeError(w, http.StatusInternalServerError, "save failed: "+err.Error())
		return
	}
	// Echo the new state via the same shape as GET so the UI can
	// hydrate without a separate roundtrip.
	s.handleGetStoiceraSettings(w, r)
}

// ---------------------------------------------------------------------------
// Integration endpoints — what the stoicera-intranet app actually calls
// ---------------------------------------------------------------------------

// ventureFilter returns the trimmed VentureName from settings, or
// empty string when the integration shouldn't return any data. When
// empty, callers should write `{ "items": [] }` rather than the full
// listing — empty venture name means "don't share anything yet."
func (s *Server) ventureFilter() string {
	return strings.TrimSpace(loadStoiceraSettings(s.cfg.Vault.Root).VentureName)
}

// matchesVenture is a case-insensitive equality on the venture name.
// Granit's free-text Venture field is hand-typed so "Stoicera" and
// "stoicera" should be treated as the same.
func matchesVenture(field, target string) bool {
	return strings.EqualFold(strings.TrimSpace(field), target)
}

// handleStoiceraSummary returns aggregate counts for the intranet
// dashboard: how many active projects, open tasks, active goals
// belong to the configured venture. The intranet wants this on its
// homepage; the full lists are separate endpoints for paginated
// fetch.
func (s *Server) handleStoiceraSummary(w http.ResponseWriter, r *http.Request) {
	venture := s.ventureFilter()
	out := map[string]int{
		"projects_active": 0,
		"projects_total":  0,
		"tasks_open":      0,
		"tasks_done":      0,
		"goals_active":    0,
		"goals_total":     0,
	}
	if venture == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}

	if projs, err := granitmeta.ReadProjects(s.cfg.Vault.Root); err == nil {
		for _, p := range projs {
			if !matchesVenture(p.Venture, venture) {
				continue
			}
			out["projects_total"]++
			status := strings.ToLower(strings.TrimSpace(p.Status))
			if status == "" || status == "active" {
				out["projects_active"]++
			}
		}
	}

	for _, t := range s.allTasksForVenture(venture) {
		if t.Done {
			out["tasks_done"]++
		} else {
			out["tasks_open"]++
		}
	}

	for _, g := range goalsForVenture(s.cfg.Vault.Root, venture) {
		out["goals_total"]++
		st := strings.ToLower(strings.TrimSpace(string(g.Status)))
		if st == "" || st == "active" {
			out["goals_active"]++
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// goalsForVenture returns all goals whose Venture matches (case-
// insensitive). Wraps goals.LoadAll + the venture filter so callers
// don't repeat the boilerplate.
func goalsForVenture(vaultRoot, venture string) []goals.Goal {
	all := goals.LoadAll(vaultRoot)
	if venture == "" {
		return nil
	}
	out := make([]goals.Goal, 0, len(all))
	for _, g := range all {
		if matchesVenture(g.Venture, venture) {
			out = append(out, g)
		}
	}
	return out
}

// handleStoiceraListProjects returns the venture's projects. Read-
// only — the intranet sends changes to granit by writing notes
// directly via the granit auth path; the integration token isn't
// for mutation.
func (s *Server) handleStoiceraListProjects(w http.ResponseWriter, r *http.Request) {
	venture := s.ventureFilter()
	out := []map[string]interface{}{}
	if venture == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": out, "venture": ""})
		return
	}
	if projs, err := granitmeta.ReadProjects(s.cfg.Vault.Root); err == nil {
		for _, p := range projs {
			if !matchesVenture(p.Venture, venture) {
				continue
			}
			out = append(out, map[string]interface{}{
				"name":        p.Name,
				"status":      p.Status,
				"description": p.Description,
				"next_action": p.NextAction,
				"color":       p.Color,
				"created_at":  p.CreatedAt,
				"updated_at":  p.UpdatedAt,
				"venture":     p.Venture,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": out, "venture": venture})
}

// handleStoiceraListTasks returns the venture's open tasks. Filters
// via the task's Project field — if a task lives in a note whose
// project belongs to the venture, surface it.
func (s *Server) handleStoiceraListTasks(w http.ResponseWriter, r *http.Request) {
	venture := s.ventureFilter()
	out := []map[string]interface{}{}
	if venture == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": out, "venture": ""})
		return
	}
	for _, t := range s.allTasksForVenture(venture) {
		out = append(out, map[string]interface{}{
			"id":              t.ID,
			"text":            t.Text,
			"done":            t.Done,
			"due_date":        t.DueDate,
			"scheduled_start": t.ScheduledStart,
			"priority":        t.Priority,
			"tags":            t.Tags,
			"project":         t.Project,
			"note_path":       t.NotePath,
			"created_at":      t.CreatedAt,
			"completed_at":    t.CompletedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": out, "venture": venture})
}

// handleStoiceraListGoals returns the venture's goals (active + paused;
// completed/archived elided by default since the intranet wants the
// "what's in flight" view).
func (s *Server) handleStoiceraListGoals(w http.ResponseWriter, r *http.Request) {
	venture := s.ventureFilter()
	out := []map[string]interface{}{}
	if venture == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": out, "venture": ""})
		return
	}
	for _, g := range goalsForVenture(s.cfg.Vault.Root, venture) {
		st := strings.ToLower(strings.TrimSpace(string(g.Status)))
		if st == "completed" || st == "archived" {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":          g.ID,
			"title":       g.Title,
			"status":      g.Status,
			"description": g.Description,
			"target_date": g.TargetDate,
			"category":    g.Category,
			"project":     g.Project,
			"venture":     g.Venture,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": out, "venture": venture})
}

// handleStoiceraGetProject returns one project's full detail. Mounted
// on /projects/{name} so the intranet can deep-link.
func (s *Server) handleStoiceraGetProject(w http.ResponseWriter, r *http.Request) {
	venture := s.ventureFilter()
	if venture == "" {
		http.NotFound(w, r)
		return
	}
	name := chi.URLParam(r, "name")
	projs, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load projects: "+err.Error())
		return
	}
	for _, p := range projs {
		if p.Name != name {
			continue
		}
		if !matchesVenture(p.Venture, venture) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, p)
		return
	}
	http.NotFound(w, r)
}

// allTasksForVenture collects tasks belonging to any project in the
// given venture. Uses the same projectMatches helper the /projects
// list endpoints use — that way the Stoicera integration sees the
// same task counts the granit UI shows. projectMatches considers
// BOTH Folder-prefix match (task lives under Projects/Apollo/...)
// AND Project name match on Task.Project (free-text marker in the
// markdown line).
func (s *Server) allTasksForVenture(venture string) []tasks.Task {
	if s.cfg.TaskStore == nil || venture == "" {
		return nil
	}
	projs, err := granitmeta.ReadProjects(s.cfg.Vault.Root)
	if err != nil {
		return nil
	}
	keep := []granitmeta.Project{}
	for _, p := range projs {
		if matchesVenture(p.Venture, venture) {
			keep = append(keep, p)
		}
	}
	if len(keep) == 0 {
		return nil
	}
	all := s.cfg.TaskStore.All()
	out := []tasks.Task{}
	for _, t := range all {
		for _, p := range keep {
			if projectMatches(p, t) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}
