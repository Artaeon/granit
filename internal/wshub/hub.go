// Package wshub is a small WebSocket fan-out hub that relays vault events to
// browser clients connected via the granit web server.
package wshub

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Event struct {
	Type string `json:"type"`           // hello | note.changed | note.removed | task.changed | vault.rescanned
	Path string `json:"path,omitempty"` // vault-relative
	ID   string `json:"id,omitempty"`   // task ID (for task.changed)
}

type Hub struct {
	log *slog.Logger
	mu  sync.Mutex
	cs  map[*subscriber]struct{}
}

type subscriber struct {
	conn  *websocket.Conn
	queue chan Event
}

func New(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log, cs: map[*subscriber]struct{}{}}
}

// Subscribe runs until the client disconnects. Each subscriber has a small
// buffered queue; if the client falls behind, oldest messages are dropped.
func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) {
	s := &subscriber{conn: conn, queue: make(chan Event, 32)}
	h.mu.Lock()
	h.cs[s] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.cs, s)
		h.mu.Unlock()
	}()

	readErr := make(chan error, 1)
	go func() {
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	_ = writeEvent(ctx, conn, Event{Type: "hello"})

	tk := time.NewTicker(30 * time.Second)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readErr:
			if err != nil {
				return
			}
		case ev := <-s.queue:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := writeEvent(wctx, conn, ev)
			cancel()
			if err != nil {
				h.log.Debug("ws write failed", "err", err)
				return
			}
		case <-tk.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (h *Hub) Broadcast(ev Event) {
	h.mu.Lock()
	subs := make([]*subscriber, 0, len(h.cs))
	for s := range h.cs {
		subs = append(subs, s)
	}
	h.mu.Unlock()

	for _, s := range subs {
		select {
		case s.queue <- ev:
		default:
			select {
			case <-s.queue:
			default:
			}
			select {
			case s.queue <- ev:
			default:
			}
		}
	}
}

func (h *Hub) Connections() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.cs)
}

func writeEvent(ctx context.Context, conn *websocket.Conn, ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}
