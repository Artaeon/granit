package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/serveapi"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// runWeb implements `granit web [--addr :8787] [--dev] [vault-path]`.
//
// Boots the JSON+WebSocket API and serves the embedded SvelteKit SPA. Uses
// granit's existing vault, tasks, and daily packages directly so the web
// frontend and the TUI operate on the same data.
func runWeb(args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	addr := fs.String("addr", defaultWebAddr(), "listen address")
	dev := fs.Bool("dev", false, "enable dev CORS for the Vite dev server")
	sync := fs.Bool("sync", false, "git auto-sync (pull + commit + push on a tick)")
	syncEvery := fs.Duration("sync-interval", 60*time.Second, "interval between auto-sync runs (min 10s)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	vaultPath := "."
	if fs.NArg() >= 1 {
		vaultPath = fs.Arg(0)
	}
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolving vault path:", err)
		os.Exit(1)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "vault path is not a directory:", abs)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	v, err := vault.NewVault(abs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "opening vault:", err)
		os.Exit(1)
	}
	if err := v.Scan(); err != nil {
		fmt.Fprintln(os.Stderr, "scanning vault:", err)
		os.Exit(1)
	}
	logger.Info("vault opened", "path", v.Root, "notes", v.NoteCount())

	store, err := tasks.Load(v.Root, func() []tasks.NoteContent {
		notes := v.SnapshotNotes()
		out := make([]tasks.NoteContent, 0, len(notes))
		for _, n := range notes {
			out = append(out, tasks.NoteContent{Path: n.RelPath, Content: n.Content})
		}
		return out
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "loading task store:", err)
		os.Exit(1)
	}

	tok, err := serveapi.LoadOrCreateToken(v.Root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth token:", err)
		os.Exit(1)
	}
	tokenPath := filepath.Join(v.Root, ".granit", "everything-token")
	authPath := filepath.Join(v.Root, ".granit", "web-auth.json")
	hasPassword := false
	if data, err := os.ReadFile(authPath); err == nil {
		hasPassword = len(data) > 30 // any non-empty password_hash field
	}
	if hasPassword {
		fmt.Fprintf(os.Stderr, "\n  Open the web UI and log in with your password.\n")
		fmt.Fprintf(os.Stderr, "  CLI scripts can use the legacy token: %s\n  (stored in %s)\n\n", tok, tokenPath)
	} else {
		fmt.Fprintf(os.Stderr, "\n  First launch — open the web UI to set your password.\n")
		fmt.Fprintf(os.Stderr, "  Until you do, the legacy bearer token is: %s\n  (stored in %s)\n\n", tok, tokenPath)
	}

	cfg := config.LoadForVault(v.Root)
	dailyCfg := daily.DailyConfig{Folder: cfg.DailyNotesFolder, Template: cfg.DailyNoteTemplate}
	if dailyCfg.Template == "" {
		dailyCfg.Template = daily.DefaultConfig().Template
	}

	srv, err := serveapi.NewServer(serveapi.Config{
		Vault:     v,
		TaskStore: store,
		Daily:     dailyCfg,
		Token:     tok,
		Dev:       *dev,
		Logger:    logger,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "starting server:", err)
		os.Exit(1)
	}
	defer srv.Close()

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *sync {
		syncer := serveapi.NewSyncer(v.Root, *syncEvery, logger)
		srv.SetSyncer(syncer)
		go syncer.Run(ctx)
	}

	go func() {
		logger.Info("granit web listening", "addr", *addr, "dev", *dev, "sync", *sync)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

func defaultWebAddr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8787"
}
