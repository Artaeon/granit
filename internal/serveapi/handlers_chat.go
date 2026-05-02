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
	n := s.cfg.Vault.GetNote(notePath)
	if n == nil {
		return out
	}
	s.cfg.Vault.EnsureLoaded(notePath)
	n = s.cfg.Vault.GetNote(notePath)
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
