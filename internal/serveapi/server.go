// Package serveapi is the HTTP/JSON+WebSocket server granit ships for the
// web frontend. It wraps granit's existing vault, tasks, and daily packages
// rather than reimplementing them, so the web app and TUI share the same
// data model.
package serveapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
	"github.com/artaeon/granit/internal/wshub"
)

type Config struct {
	Vault     *vault.Vault
	TaskStore *tasks.TaskStore
	Daily     daily.DailyConfig
	Token     string
	Dev       bool
	Logger    *slog.Logger
}

type Server struct {
	cfg      Config
	hub      *wshub.Hub
	watcher  *watcher
	search   *vault.SearchIndex
	auth     *authState
	rescanMu sync.Mutex
	mu       sync.Mutex
	syncer   *Syncer

	// activeTimer is the currently-running clock-in session, if any.
	// Server-side state (one timer per server, since one server hosts
	// one vault); guarded by timerMu. Survives only as long as the
	// process — granit web restart drops the timer.
	timerMu     sync.Mutex
	activeTimer *activeTimer

	// Recurring-task scheduling. Per-Server (not package-level) so
	// multiple servers in one process — tests, future multi-vault —
	// don't share state. recurringMu serialises the "create today's
	// due tasks" pass; recurringRanFor caches the YYYY-MM-DD of the
	// most recent successful run so 100 hits to /recurring on the
	// same day fire the work once.
	recurringMu     sync.Mutex
	recurringRanFor string
}

// activeTimer is the in-memory shape of a running timer. We keep it
// local to the serveapi package — the TUI uses its own struct for the
// same purpose (it has a UI loop to drive). The shared package's
// timetracker.Active is the duck-type.
type activeTimer struct {
	NotePath  string
	TaskText  string
	TaskID    string
	StartTime time.Time
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Vault == nil {
		return nil, fmt.Errorf("serveapi: vault is required")
	}
	if cfg.TaskStore == nil {
		return nil, fmt.Errorf("serveapi: taskstore is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	w, err := newWatcher(cfg.Vault.Root, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("serveapi: watcher: %w", err)
	}
	auth, err := newAuthState(cfg.Vault.Root)
	if err != nil {
		return nil, fmt.Errorf("serveapi: auth: %w", err)
	}
	s := &Server{
		cfg:     cfg,
		hub:     wshub.New(cfg.Logger),
		watcher: w,
		search:  vault.NewSearchIndex(),
		auth:    auth,
	}
	// Build the search index in the background — could take a moment on
	// large vaults, and the API doesn't need to wait for it.
	go func() {
		s.search.Build(cfg.Vault)
		cfg.Logger.Info("search index built")
	}()
	go s.runWatcher()
	return s, nil
}

func (s *Server) Close() error {
	if s.watcher != nil {
		return s.watcher.Close()
	}
	return nil
}

// Handler returns the http.Handler for the API + embedded SPA.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Cap every request body at 4 MiB. Notes can be large but rarely
	// over a megabyte; legitimate API writes (config patches, agent
	// goals, devotional reflections) are far smaller. A bigger payload
	// is almost certainly a buggy client or an attempt to exhaust
	// server memory — fail fast at the read instead of silently
	// streaming gigabytes into a json.Decoder.
	r.Use(maxBodyBytes(4 << 20))

	if s.cfg.Dev {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type", "If-Match"},
			ExposedHeaders:   []string{"ETag"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	r.Get("/api/v1/health", s.handleHealth)
	r.Get("/api/v1/ws", s.handleWS)

	// Auth: public endpoints (status / setup-on-first-launch / login).
	// Setup is a no-op once a password exists; login is rate-limited via
	// a per-failure delay rather than a bucket — single-user is fine.
	r.Get("/api/v1/auth/status", s.handleAuthStatus)
	r.Post("/api/v1/auth/setup", s.handleAuthSetup)
	r.Post("/api/v1/auth/login", s.handleAuthLogin)

	r.Group(func(r chi.Router) {
		r.Use(s.requireToken)
		r.Post("/api/v1/auth/logout", s.handleAuthLogout)
		r.Post("/api/v1/auth/change-password", s.handleAuthChangePassword)
		r.Post("/api/v1/auth/revoke-all", s.handleAuthRevokeAll)
		r.Get("/api/v1/vault", s.handleVault)
		r.Get("/api/v1/notes", s.handleListNotes)
		r.Post("/api/v1/notes", s.handleCreateNote)
		r.Get("/api/v1/notes/*", s.handleGetNote)
		r.Put("/api/v1/notes/*", s.handlePutNote)
		r.Delete("/api/v1/notes/*", s.handleDeleteNote)
		// Rename / move a note. POST so the body carries from+to —
		// chi doesn't have a clean "rename" verb shape.
		r.Post("/api/v1/notes/rename", s.handleRenameNote)
		r.Get("/api/v1/links/*", s.handleGetLinks)

		r.Get("/api/v1/tasks", s.handleListTasks)
		r.Post("/api/v1/tasks", s.handleCreateTask)
		r.Delete("/api/v1/tasks/{id}", s.handleDeleteTask)
		r.Get("/api/v1/tasks/{id}", s.handleGetTask)
		r.Patch("/api/v1/tasks/{id}", s.handlePatchTask)

		// Literal path registered first so chi matches it before the
		// {date} wildcard branch (otherwise "context" would be parsed
		// as a date and 400 from handleGetDaily's parser).
		r.Get("/api/v1/daily/context", s.handleDailyContext)
		r.Get("/api/v1/daily/{date}", s.handleGetDaily)
		r.Get("/api/v1/jots", s.handleListJots)
		r.Get("/api/v1/calendar", s.handleCalendar)
		r.Get("/api/v1/calendar/sources", s.handleListCalendarSources)
		r.Patch("/api/v1/calendar/sources", s.handlePatchCalendarSources)

		r.Get("/api/v1/projects", s.handleListProjects)
		r.Post("/api/v1/projects", s.handleCreateProject)
		r.Get("/api/v1/projects/{name}", s.handleGetProject)
		r.Patch("/api/v1/projects/{name}", s.handlePatchProject)
		r.Delete("/api/v1/projects/{name}", s.handleDeleteProject)

		r.Get("/api/v1/events", s.handleListEvents)
		r.Post("/api/v1/events", s.handleCreateEvent)
		r.Patch("/api/v1/events/{id}", s.handlePatchEvent)
		r.Delete("/api/v1/events/{id}", s.handleDeleteEvent)

		r.Get("/api/v1/goals", s.handleListGoals)
		r.Post("/api/v1/goals", s.handleCreateGoal)
		r.Patch("/api/v1/goals/{id}", s.handlePatchGoal)
		r.Delete("/api/v1/goals/{id}", s.handleDeleteGoal)
		r.Post("/api/v1/goals/{id}/milestones", s.handleAddMilestone)
		r.Patch("/api/v1/goals/{id}/milestones/{idx}", s.handlePatchMilestone)
		r.Delete("/api/v1/goals/{id}/milestones/{idx}", s.handleDeleteMilestone)
		r.Post("/api/v1/goals/{id}/review", s.handleLogReview)

		r.Get("/api/v1/types", s.handleListTypes)
		r.Get("/api/v1/types/{id}/objects", s.handleListTypeObjects)
		r.Get("/api/v1/tags", s.handleListTags)

		r.Get("/api/v1/pinned", s.handleListPinned)
		r.Patch("/api/v1/pinned", s.handlePatchPinned)

		r.Get("/api/v1/habits", s.handleListHabits)
		// Per-date habit toggle. Mark a habit done/undone for ANY day
		// (not just today) — drives the click-on-past-day-dot
		// interaction on the habits heatmap.
		r.Post("/api/v1/habits/toggle", s.handleToggleHabit)

		r.Get("/api/v1/search", s.handleSearch)

		r.Get("/api/v1/templates", s.handleListTemplates)
		r.Post("/api/v1/notes/from-template", s.handleFromTemplate)

		r.Get("/api/v1/stats", s.handleStats)

		r.Post("/api/v1/morning/save", s.handleSaveMorning)

		r.Get("/api/v1/sync", s.handleSyncStatus)
		r.Post("/api/v1/sync", s.handleSyncTrigger)

		// Settings — curated view of the granit config.json the TUI also
		// reads, so changes made on /settings show up in the TUI on next
		// launch and vice-versa.
		r.Get("/api/v1/config", s.handleGetConfig)
		r.Patch("/api/v1/config", s.handlePatchConfig)
		// Curated OpenAI model picker — refreshed against
		// developers.openai.com/api/docs/pricing periodically. Exposed
		// so the settings page can render a dropdown of recommended
		// models instead of a free-form text input where the user has
		// to know exact IDs.
		r.Get("/api/v1/config/openai-models", s.handleListOpenAIModels)

		// Vault binary file passthrough — used by the markdown preview
		// to inline images via `![[image.png]]`. Markdown files have
		// their own JSON endpoint and are refused here.
		r.Get("/api/v1/files/*", s.handleGetFile)

		// Recurring tasks — same .granit/recurring.json file the TUI's
		// recurringtasks overlay edits. Server fires due rules at
		// midnight + on every list/mutate.
		r.Get("/api/v1/recurring", s.handleListRecurring)
		r.Put("/api/v1/recurring", s.handlePutRecurring)

		// Time tracking — clock-in/out + session history. Persists to
		// .granit/timetracker.json (same file the TUI's clock-in
		// overlay writes).
		r.Get("/api/v1/timetracker", s.handleListTimetracker)
		r.Post("/api/v1/timetracker/start", s.handleClockIn)
		r.Post("/api/v1/timetracker/stop", s.handleClockOut)

		r.Get("/api/v1/dashboard", s.handleGetDashboard)
		r.Put("/api/v1/dashboard", s.handlePutDashboard)

		// Agents — read-only catalog + run history. Reuses internal/agents
		// and the vault index, so this stays in lockstep with what the
		// TUI's AgentRunner sees.
		r.Get("/api/v1/agents/presets", s.handleListAgentPresets)
		r.Get("/api/v1/agents/runs", s.handleListAgentRuns)
		r.Post("/api/v1/agents/run", s.handleRunAgent)
		// Synchronous wrapper around plan-my-day that ALSO post-
		// processes the agent's `## Plan` block and writes
		// scheduledStart back to matched tasks. See handlers_plan_day_schedule.go.
		r.Post("/api/v1/agents/plan-day-schedule", s.handlePlanDaySchedule)

		// Multi-turn chat — single-shot helper around agentruntime.Chatter.
		// Stateless on the server; the web persists history client-side.
		r.Post("/api/v1/chat", s.handleChat)

		// Scripture / devotional — verse of the day, full set, "another
		// one" random pick, and a one-shot devotional-note creator.
		r.Get("/api/v1/scripture", s.handleListScriptures)
		r.Get("/api/v1/scripture/today", s.handleDailyScripture)
		r.Get("/api/v1/scripture/random", s.handleRandomScripture)
		r.Post("/api/v1/devotionals", s.handleCreateDevotional)

		// Bible — full embedded WEB (World English Bible, public domain)
		// as a reader + random-passage source. Backed by
		// internal/scripture/bible (loaded once from a go:embed JSON).
		r.Get("/api/v1/bible/books", s.handleBibleBooks)
		r.Get("/api/v1/bible/random", s.handleBibleRandom)
		r.Get("/api/v1/bible/search", s.handleBibleSearch)
		r.Get("/api/v1/bible/{book}/{chapter}", s.handleBibleChapter)

		// Devices — authState.Sessions exposed for management.
		r.Get("/api/v1/devices", s.handleListDevices)
		r.Delete("/api/v1/devices/{id}", s.handleRevokeDevice)
	})

	// SPA fallback — last resort
	assets := Assets()
	fileSrv := http.FileServer(assets)
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		f, err := assets.Open(req.URL.Path)
		if err != nil {
			req2 := req.Clone(req.Context())
			req2.URL.Path = "/"
			fileSrv.ServeHTTP(w, req2)
			return
		}
		f.Close()
		fileSrv.ServeHTTP(w, req)
	})

	return r
}

// runWatcher receives debounced fs events, rescans the vault, reloads the
// task store, and broadcasts to WS subscribers.
func (s *Server) runWatcher() {
	for ev := range s.watcher.Events() {
		rel, err := filepath.Rel(s.cfg.Vault.Root, ev.Path)
		if err != nil {
			continue
		}
		relSlash := filepath.ToSlash(rel)
		if !strings.HasSuffix(strings.ToLower(relSlash), ".md") {
			continue
		}

		s.rescanMu.Lock()
		_ = s.cfg.Vault.ScanFast()
		_ = s.cfg.TaskStore.Reload()
		// Incrementally update the search index. Remove on delete; reindex
		// on create/write/rename so the body changes propagate.
		if ev.Kind == fsRemove {
			s.search.Remove(relSlash)
		} else {
			if n := s.cfg.Vault.GetNote(relSlash); n != nil {
				s.cfg.Vault.EnsureLoaded(relSlash)
				s.search.Update(relSlash, n.Content)
			}
		}
		s.rescanMu.Unlock()

		t := "note.changed"
		if ev.Kind == fsRemove {
			t = "note.removed"
		}
		s.hub.Broadcast(wshub.Event{Type: t, Path: relSlash})
	}
}

// helpers ---------------------------------------------------------------

func (s *Server) etagFor(modTime time.Time, size int64) string {
	return fmt.Sprintf(`W/"%d-%d"`, modTime.UnixNano(), size)
}

// Run is a convenience: builds the handler and runs ListenAndServe. The
// caller is responsible for calling Close on shutdown.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// Fire recurring tasks at boot + every midnight while the server
	// runs. ctx cancellation stops the loop cleanly.
	s.startRecurringLoop(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// maxBodyBytes wraps every request's body with http.MaxBytesReader so a
// pathological POST can't stream gigabytes into a json.Decoder. The
// limit is generous (notes can be big, but the cap covers everything
// the web actually writes) and the failure is a clean 400 from the
// json decode call rather than OOM-ing the process.
func maxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}
