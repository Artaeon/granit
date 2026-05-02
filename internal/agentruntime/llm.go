// Package agentruntime wires granit's existing internal/agents loop into a
// runnable shape callable from anywhere — TUI, web server, scheduled jobs.
//
// The agents package itself is deliberately thin: it knows the ReAct loop,
// the tool registry, and the LLM interface, but doesn't know about granit's
// vault, config, or how to talk to OpenAI/Ollama. agentruntime supplies all
// three so a caller just needs:
//
//	llm := agentruntime.NewLLM(cfg)            // OpenAI/Ollama/Anthropic per cfg
//	bridge := agentruntime.NewBridge(vault, …) // tools see the live vault
//	runner := agentruntime.New(llm, bridge, opts)
//	transcript, _ := runner.Run(ctx, preset, goal)
//
// No TUI dependency — the web server and any future scheduled-jobs daemon
// can use the same runtime.
package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/config"
)

// ChatMessage is a single turn in a chat conversation. Used by Chat()
// for multi-turn conversations where the agents.LLM single-shot
// Complete() interface isn't enough.
type ChatMessage struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// Chatter is the multi-turn-aware extension of agents.LLM. NewLLM's
// returns implement both — Complete for the agent loop, Chat for the
// /chat endpoint. We keep them separate because the agent loop
// doesn't need conversation history (it bakes everything into one
// prompt) and we don't want to pay the prompt-building cost twice.
type Chatter interface {
	Chat(ctx context.Context, messages []ChatMessage) (string, error)
}

// Usage is the token tally from one LLM call. Captured by the OpenAI
// implementation (Ollama doesn't bill, so its usage is always zero).
// Cost is computed from a price table at the runtime layer, not here.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	Model            string // model that produced these tokens
}

// Metered is implemented by LLMs that report token usage after each
// call. The runtime polls LastUsage() between iterations to drive
// budget enforcement; LLMs without this interface (Ollama) get a free
// pass — no budget tracking, no cost reporting.
type Metered interface {
	LastUsage() Usage
}

// NewLLM constructs an LLM bound to the user's granit config. Provider
// selection mirrors what the TUI does so a working `granit tui` setup
// just works on the web side too — same key, same model, same provider.
//
// "openai" hits the cloud API; "ollama"/"local"/empty go to
// localhost:11434 by default. Unknown providers fall back to OpenAI when
// a key is set, otherwise return an error so misconfiguration surfaces
// as soon as an agent actually runs (rather than silently doing nothing).
//
// TODO(anthropic): port the Messages-API client from
// internal/tui/aiconfig.go (~lines 288-500) and add a `case "anthropic":`
// branch keyed on cfg.AnthropicKey + cfg.AnthropicModel. Until then the
// settings UI hides the Anthropic option to avoid silent fallthrough
// to OpenAI/Ollama for users who pick "anthropic" expecting Claude.
func NewLLM(cfg config.Config) (agents.LLM, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.AIProvider))
	switch provider {
	case "openai":
		if cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("openai: no API key set (config.json: openai_key)")
		}
		model := cfg.OpenAIModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		return &openAILLM{apiKey: cfg.OpenAIKey, model: model}, nil
	case "ollama", "local", "":
		url := strings.TrimRight(cfg.OllamaURL, "/")
		if url == "" {
			url = "http://localhost:11434"
		}
		model := cfg.OllamaModel
		if model == "" {
			model = "llama3.2"
		}
		return &ollamaLLM{url: url, model: model}, nil
	}
	// Unknown provider — defer to OpenAI if a key happens to be set,
	// otherwise refuse so the user sees the misconfiguration.
	if cfg.OpenAIKey != "" {
		model := cfg.OpenAIModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		return &openAILLM{apiKey: cfg.OpenAIKey, model: model}, nil
	}
	return nil, fmt.Errorf("unsupported ai_provider %q (set ai_provider in config.json to openai or ollama)", cfg.AIProvider)
}

// ----- OpenAI -----

type openAILLM struct {
	apiKey string
	model  string
	// lastUsage tracks the token count from the most recent call.
	// Single-writer (the goroutine inside an agent run) so a plain
	// field is safe — no mutex needed.
	lastUsage Usage
}

func (l *openAILLM) Complete(ctx context.Context, prompt string) (string, error) {
	return l.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}})
}

// LastUsage returns the token tally from the most recent Chat/Complete.
// Zero values when no call has happened yet. The Model field always
// reflects the configured model so cost lookups stay deterministic
// even if the API silently routes to a different snapshot.
func (l *openAILLM) LastUsage() Usage { return l.lastUsage }

// Chat sends a multi-turn conversation. OpenAI's chat-completions
// endpoint takes the messages array directly so this is just a
// re-shape of our ChatMessage slice.
func (l *openAILLM) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	wire := make([]map[string]string, 0, len(messages))
	for _, m := range messages {
		wire = append(wire, map[string]string{"role": m.Role, "content": m.Content})
	}
	body, _ := json.Marshal(map[string]any{
		"model":    l.model,
		"messages": wire,
	})
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Content-Type", "application/json")

	cl := &http.Client{Timeout: 120 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("openai: API key rejected (HTTP %d). Check config.json's openai_key", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openai: %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("openai: parse: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices returned")
	}
	// Cache token usage so the runtime can poll between iterations.
	// We deliberately store the configured model (l.model) rather than
	// any "actual model" field the response might carry — for cost
	// calculation we want the user's settings, not the API's silent
	// routing decision.
	l.lastUsage = Usage{
		PromptTokens:     out.Usage.PromptTokens,
		CompletionTokens: out.Usage.CompletionTokens,
		Model:            l.model,
	}
	return out.Choices[0].Message.Content, nil
}

// ----- Ollama -----

type ollamaLLM struct {
	url   string
	model string
}

func (l *ollamaLLM) Complete(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  l.model,
		"prompt": prompt,
		"stream": false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", l.url+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	cl := &http.Client{Timeout: 5 * time.Minute} // local models can be slow
	resp, err := cl.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama at %s: %w (is `ollama serve` running?)", l.url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("ollama: model %q not pulled. Run: ollama pull %s", l.model, l.model)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ollama: %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("ollama: parse: %w", err)
	}
	return out.Response, nil
}

// Chat uses Ollama's /api/chat endpoint (which accepts the OpenAI-
// shaped messages array directly). Supported on Ollama 0.1.14+.
func (l *ollamaLLM) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	wire := make([]map[string]string, 0, len(messages))
	for _, m := range messages {
		wire = append(wire, map[string]string{"role": m.Role, "content": m.Content})
	}
	body, _ := json.Marshal(map[string]any{
		"model":    l.model,
		"messages": wire,
		"stream":   false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", l.url+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama at %s: %w (is `ollama serve` running?)", l.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ollama: %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("ollama: parse: %w", err)
	}
	return out.Message.Content, nil
}
