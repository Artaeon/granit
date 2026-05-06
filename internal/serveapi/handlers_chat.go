package serveapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/config"
)

// chatMessage mirrors agentruntime.ChatMessage with JSON tags for the
// wire shape. Kept local so the wire schema doesn't drift if the
// runtime renames its fields.
type chatMessage struct {
	Role    string `json:"role"`    // system | user | assistant
	Content string `json:"content"` // raw text, no markdown rendering
}

type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	// Optional: notePath asks the server to attach the named note's
	// body as a system message, so the LLM has the user's vault context
	// without the user pasting it manually. Cheap on the server side
	// (we already have the note loaded); saves the round-trip a
	// "fetch and paste" UX would require.
	NotePath string `json:"notePath,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// handleChat is a single-turn helper around agentruntime.Chatter — the
// caller supplies the conversation history each time, the server reads
// the AI config, calls the model, returns the next assistant message.
//
// We deliberately don't store conversations server-side. The web's
// /chat page keeps history in localStorage; if the user wants
// long-term retention they save the conversation as a note. Stateless
// server keeps the auth model simple — anyone with a session token can
// chat, but no chat data leaks across users.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages required")
		return
	}

	cfg := config.LoadForVault(s.cfg.Vault.Root)
	llm, err := agentruntime.NewLLM(cfg)
	if err != nil {
		// Misconfiguration → 400 not 500. The user has to fix
		// config.json, not us.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Pre-flight ping: surfaces a clean 400 within ~5s when the
	// provider is unreachable or the model isn't pulled. Chat is
	// fast, so the upside is mostly consistency with /agents/run +
	// a clearer error than the upstream provider's raw failure
	// (which we'd otherwise return as 502 below).
	if hint := preflightLLM(llm); hint != "" {
		writeError(w, http.StatusBadRequest, hint)
		return
	}
	chatter, ok := llm.(agentruntime.Chatter)
	if !ok {
		// Should never happen given our two implementations, but
		// future LLMs (e.g. a tool-calling-only backend) might not
		// support multi-turn chat. Surface clearly.
		writeError(w, http.StatusInternalServerError, "configured LLM does not support chat")
		return
	}

	// Prepend the system context: a short preamble that frames the
	// model as a vault assistant + (optionally) the named note's body.
	// This lives server-side so all clients get consistent context
	// without each web/mobile/CLI client having to encode it.
	prefix := defaultSystemMessages(s, body.NotePath)

	wire := make([]agentruntime.ChatMessage, 0, len(prefix)+len(body.Messages))
	for _, m := range prefix {
		wire = append(wire, agentruntime.ChatMessage{Role: m.Role, Content: m.Content})
	}
	for _, m := range body.Messages {
		role := strings.TrimSpace(m.Role)
		if role != "system" && role != "user" && role != "assistant" {
			role = "user"
		}
		wire = append(wire, agentruntime.ChatMessage{Role: role, Content: m.Content})
	}

	// Bound LLM calls so a hung backend can't tie up a request
	// indefinitely. 90s is plenty for chat — agent runs use 5min
	// because they may make several model calls per run.
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	reply, err := chatter.Chat(ctx, wire)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chatResponse{
		Message: chatMessage{Role: "assistant", Content: reply},
	})
}

// handleChatStream is the SSE-streaming sibling of handleChat. Same
// request shape; response is text/event-stream where each event's
// data is `{"chunk":"…"}`. A final `event: done` marks the end of
// the response. Errors are surfaced as `event: error` so the client
// can distinguish them from normal chunks without parsing the body.
//
// Falls back to a buffered single chunk when the configured LLM
// doesn't implement ChatStreamer — keeps the endpoint usable
// across providers without forking the client logic.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages required")
		return
	}

	cfg := config.LoadForVault(s.cfg.Vault.Root)
	llm, err := agentruntime.NewLLM(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if hint := preflightLLM(llm); hint != "" {
		writeError(w, http.StatusBadRequest, hint)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported by transport")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Disable proxy buffering — most reverse proxies (Traefik, Nginx)
	// buffer small responses and the stream chokes until enough bytes
	// pile up, defeating the whole point. X-Accel-Buffering is the
	// nginx-style hint; both honour it in modern builds.
	w.Header().Set("X-Accel-Buffering", "no")

	// Build the wire messages with the same system-prefix shape as
	// the buffered handler so streaming and non-streaming paths
	// behave identically apart from delivery.
	prefix := defaultSystemMessages(s, body.NotePath)
	wire := make([]agentruntime.ChatMessage, 0, len(prefix)+len(body.Messages))
	for _, m := range prefix {
		wire = append(wire, agentruntime.ChatMessage{Role: m.Role, Content: m.Content})
	}
	for _, m := range body.Messages {
		role := strings.TrimSpace(m.Role)
		if role != "system" && role != "user" && role != "assistant" {
			role = "user"
		}
		wire = append(wire, agentruntime.ChatMessage{Role: role, Content: m.Content})
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	send := func(event, data string) {
		if event != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", event)
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	streamer, ok := llm.(agentruntime.ChatStreamer)
	if !ok {
		// Buffered fallback for providers that don't implement
		// streaming. Send the full reply as a single chunk so the
		// client's progressive-render path still works (no special
		// case needed for legacy providers).
		chatter, ok := llm.(agentruntime.Chatter)
		if !ok {
			send("error", `{"message":"configured LLM does not support chat"}`)
			return
		}
		reply, err := chatter.Chat(ctx, wire)
		if err != nil {
			send("error", mustJSON(map[string]string{"message": err.Error()}))
			return
		}
		send("", mustJSON(map[string]string{"chunk": reply}))
		send("done", "{}")
		return
	}

	err = streamer.ChatStream(ctx, wire, func(chunk string) {
		send("", mustJSON(map[string]string{"chunk": chunk}))
	})
	if err != nil {
		// ctx.Err() means the client disconnected — no point trying
		// to write back to a broken pipe. Other errors get a final
		// event so the client surfaces a clean message.
		if ctx.Err() == nil {
			send("error", mustJSON(map[string]string{"message": err.Error()}))
		}
		return
	}
	send("done", "{}")
}

// mustJSON marshals a value that we know is JSON-safe (string-keyed
// maps with string values). Fallback to a literal "{}" rather than
// panicking — a bad chunk shouldn't take the whole stream down.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// defaultSystemMessages builds the system-prompt prefix for every chat.
// Always includes a short framing message; conditionally attaches the
// referenced note's body so the model can answer questions about it
// without the user copy-pasting.
func defaultSystemMessages(s *Server, notePath string) []chatMessage {
	out := []chatMessage{{
		Role: "system",
		Content: "You are a helpful assistant inside the user's personal knowledge vault (granit). " +
			"The user may ask about their notes, tasks, projects, or general questions. " +
			"Keep replies concise unless they ask for detail. Use markdown when it helps. " +
			"Do not invent vault content — if you weren't given a note's text, say so.",
	}}
	if notePath = strings.TrimSpace(notePath); notePath == "" {
		return out
	}
	// vault.GetNote calls EnsureLoaded internally, so the body is
	// guaranteed populated when GetNote returns non-nil. (The
	// previous version called GetNote → returned early on nil →
	// EnsureLoaded → re-GetNote, which never reached the load if the
	// first lookup missed; effectively a no-op on lazy notes.)
	n := s.cfg.Vault.GetNote(notePath)
	if n == nil || strings.TrimSpace(n.Content) == "" {
		return out
	}
	body := n.Content
	// Cap at ~10k chars so the chat doesn't blow the model's context
	// on a giant note. The user can always paste excerpts if they
	// need more.
	const maxAttach = 10000
	if len(body) > maxAttach {
		body = body[:maxAttach] + "\n\n[truncated; user can paste more if needed]"
	}
	out = append(out, chatMessage{
		Role:    "system",
		Content: fmt.Sprintf("The user is viewing this note (path: %s). Refer to it when relevant:\n\n%s", n.RelPath, body),
	})
	return out
}
