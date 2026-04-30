package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ollamaModelOptions returns the dropdown choices for the
// "Ollama Model" setting. Tries to query the local Ollama daemon's
// /api/tags endpoint for installed models so the user picks from
// what's actually available; falls back to a curated starter list
// when the daemon isn't running or the query fails.
//
// `current` is included in the result even if Ollama isn't running
// — otherwise switching to a model the user knows they have would
// be impossible until they start the daemon.
func ollamaModelOptions(ollamaURL, current string) []string {
	fallback := []string{
		"qwen2.5:0.5b", "qwen2.5:1.5b", "qwen2.5:3b", "qwen2.5:7b",
		"llama3.1", "llama3.1:8b", "llama3.2", "llama3.2:1b", "llama3.2:3b",
		"phi3:mini", "phi3.5:3.8b", "gemma2:2b", "gemma2",
		"mistral", "mistral-nemo", "tinyllama",
	}
	url := ollamaURL
	if url == "" {
		url = "http://localhost:11434"
	}
	// Cap the request at 800ms — settings render shouldn't block
	// noticeably when Ollama is down.
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(url, "/")+"/api/tags", nil)
	if err != nil {
		return mergeIncludeCurrent(fallback, current)
	}
	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return mergeIncludeCurrent(fallback, current)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return mergeIncludeCurrent(fallback, current)
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return mergeIncludeCurrent(fallback, current)
	}
	if len(body.Models) == 0 {
		return mergeIncludeCurrent(fallback, current)
	}
	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return mergeIncludeCurrent(names, current)
}

// mergeIncludeCurrent guarantees `current` is in the option list so
// the dropdown always opens on a valid selection. Order: current
// first if not already in `base`, then the rest preserving order.
func mergeIncludeCurrent(base []string, current string) []string {
	current = strings.TrimSpace(current)
	if current == "" {
		return base
	}
	for _, b := range base {
		if b == current {
			return base
		}
	}
	return append([]string{current}, base...)
}

// runProviderTest fires a tiny prompt at the currently-configured
// provider and surfaces the result via setupStatus. Used by the
// ">> Test AI Provider" action button.
//
// The prompt is the literal "ping" — most models reply within one
// or two tokens; latency is dominated by the round-trip + first
// token. With Ollama on a warm model this returns in <1s; cold
// model load can take 5-30s depending on size. We cap at 30s.
func (s *Settings) runProviderTest() tea.Cmd {
	cfg := AIConfig{
		Provider:      s.config.AIProvider,
		OllamaURL:     s.config.OllamaURL,
		APIKey:        s.config.OpenAIKey,
		AnthropicKey:  s.config.AnthropicKey,
		NousURL:       s.config.NousURL,
		NousAPIKey:    s.config.NousAPIKey,
		NerveBinary:   s.config.NerveBinary,
		NerveModel:    s.config.NerveModel,
		NerveProvider: s.config.NerveProvider,
	}
	switch cfg.Provider {
	case "ollama", "local", "":
		cfg.Model = s.config.OllamaModel
	case "openai":
		cfg.Model = s.config.OpenAIModel
	case "anthropic", "claude":
		cfg.Model = s.config.AnthropicModel
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := cfg.ChatCtx(ctx,
			"You are a connection-test target. Reply with the single word: ok.",
			"ping")
		if err != nil {
			return providerTestMsg{success: false, message: translateProviderError(cfg, err).Error()}
		}
		return providerTestMsg{success: true, message: fmt.Sprintf("✓ %s + %s reachable", cfg.Provider, cfg.Model)}
	}
}

// providerTestMsg is the test result delivered back to Settings.Update.
// Mirrors ollamaStartMsg's shape for symmetry — Settings already has
// the rendering branch for that, we just route through it.
type providerTestMsg struct {
	success bool
	message string
}
