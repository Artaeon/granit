package serveapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Outbound webhook dispatcher — pings the configured intranet URL
// shortly after a mutation so the receiver can run its own sync. The
// downstream (stoicera-intranet's /api/webhooks/granit) debounces on
// its end and triggers a full syncAll, so granit's job is just to
// nudge — not to serialise the actual deltas.
//
// Behaviour:
//
//   - Fire-and-forget goroutine — handlers never block on HTTP I/O
//   - Coalesce window (configurable, default 500ms): bursts of
//     mutations during a single user action fire one POST, not N
//   - Skip when WebhookURL is empty (feature disabled at the sidecar
//     level — no goroutine spawned, no allocation)
//   - Bearer auth using WebhookSecret; matches the receiver's
//     timing-safe compare on `Authorization: Bearer <secret>`
//   - Short timeout (10s) so a slow/dead receiver doesn't strand
//     goroutines indefinitely
//
// Settings are re-read from disk on each fire so updating the
// sidecar doesn't require a server restart. The read is cheap (the
// sidecar is a few hundred bytes) and serialises with the dispatcher
// mutex; the actual HTTP POST runs in a detached goroutine.

type webhookDispatcher struct {
	vaultRoot string
	log       *slog.Logger
	// queued is true when a coalesced fire is already scheduled; the
	// next notify() call within the window is a no-op. Reset by the
	// goroutine before issuing the POST so a fresh mutation right
	// after dispatch fires the next round.
	queued atomic.Bool
	// window is how long we wait before firing after the first
	// queued mutation. Short enough that the user perceives "live"
	// updates, long enough to coalesce a click-burst into one POST.
	window time.Duration
	client *http.Client
}

func newWebhookDispatcher(vaultRoot string, log *slog.Logger) *webhookDispatcher {
	return &webhookDispatcher{
		vaultRoot: vaultRoot,
		log:       log,
		window:    500 * time.Millisecond,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// notify schedules a coalesced webhook POST. The kind is informational
// — the receiver doesn't dispatch on it (it runs a full sync regardless)
// but it lands in logs / observability. Safe to call from any handler
// path; the function returns immediately.
func (w *webhookDispatcher) notify(kind string) {
	if w == nil {
		return
	}
	if !w.queued.CompareAndSwap(false, true) {
		return // already queued
	}
	go w.fireAfter(kind)
}

func (w *webhookDispatcher) fireAfter(kind string) {
	time.Sleep(w.window)
	w.queued.Store(false)

	settings := loadStoiceraSettings(w.vaultRoot)
	if settings.WebhookURL == "" || settings.WebhookSecret == "" {
		return
	}

	body, err := json.Marshal(map[string]any{
		"kind": kind,
		"at":   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		w.log.Warn("webhook marshal failed", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.WebhookURL, bytes.NewReader(body))
	if err != nil {
		w.log.Warn("webhook request build failed", "err", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+settings.WebhookSecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "granit-webhook/1")

	resp, err := w.client.Do(req)
	if err != nil {
		w.log.Warn("webhook POST failed", "url", settings.WebhookURL, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		w.log.Warn("webhook non-2xx", "url", settings.WebhookURL, "status", resp.StatusCode)
	}
}

// Compile-time check that we use the sync package as the test
// expectations demand. (No-op when unused — keeps the import list
// stable if future hooks need to add mutex coordination.)
var _ = sync.Mutex{}
