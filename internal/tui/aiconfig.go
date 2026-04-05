package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// aiHTTPClient is the shared HTTP client for AI chat requests (Ollama, OpenAI).
// Reusing a single client enables connection pooling and avoids per-request overhead.
var aiHTTPClient = &http.Client{Timeout: 3 * time.Minute}

// chatMessage is used for building chat API request bodies.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIConfig holds the configuration for AI provider access.
// Embedded by all AI-consuming components to avoid field duplication.
type AIConfig struct {
	Provider      string
	Model         string
	OllamaURL     string
	APIKey        string
	NousURL       string
	NousAPIKey    string
	NerveBinary   string
	NerveModel    string
	NerveProvider string
}

// SetFromConfig populates the AIConfig from the app's config values.
func (c *AIConfig) SetFromConfig(provider, model, ollamaURL, apiKey, nousURL, nousAPIKey, nerveBinary, nerveModel, nerveProvider string) {
	c.Provider = provider
	c.Model = model
	c.OllamaURL = ollamaURL
	c.APIKey = apiKey
	c.NousURL = nousURL
	c.NousAPIKey = nousAPIKey
	c.NerveBinary = nerveBinary
	c.NerveModel = nerveModel
	c.NerveProvider = nerveProvider
}

// NewNerve creates a NerveClient from this config.
func (c AIConfig) NewNerve() *NerveClient {
	return NewNerveClient(c.NerveBinary, c.NerveModel, c.NerveProvider)
}

// NewNous creates a NousClient from this config.
func (c AIConfig) NewNous() *NousClient {
	return NewNousClient(c.NousURL, c.NousAPIKey)
}

// OllamaEndpoint returns the Ollama URL with a default fallback.
func (c AIConfig) OllamaEndpoint() string {
	if c.OllamaURL != "" {
		return c.OllamaURL
	}
	return "http://localhost:11434"
}

// ModelOrDefault returns the model name with a fallback.
func (c AIConfig) ModelOrDefault(fallback string) string {
	if c.Model != "" {
		return c.Model
	}
	return fallback
}

// IsSmallModel returns true if the configured model is a small local model
// (roughly ≤3B parameters) that benefits from shorter prompts and less context.
func (c AIConfig) IsSmallModel() bool {
	m := strings.ToLower(c.ModelOrDefault(""))
	// Match parameter-count suffixes (0.5b, 1b, 1.5b, 2b, 3b, etc.)
	smallPatterns := []string{
		"0.5b", "0.6b", "1b", "1.5b", "2b", "3b",
		":0.5b", ":0.6b", ":1b", ":1.5b", ":2b", ":3b",
	}
	for _, s := range smallPatterns {
		if strings.Contains(m, s) {
			return true
		}
	}
	// Match known small model families by name
	smallNames := []string{"tinyllama", "phi3:mini", "phi3.5:mini", "gemma:2b", "gemma2:2b"}
	for _, name := range smallNames {
		if strings.Contains(m, name) {
			return true
		}
	}
	return false
}

// MaxPromptContext returns the maximum number of characters to include as
// context in AI prompts. Small models get less context to stay within their
// effective window and produce faster responses.
func (c AIConfig) MaxPromptContext() int {
	if c.IsSmallModel() {
		return 1500
	}
	return 4000
}

// EstimateTokens returns a rough token count for the given text. A character-
// based heuristic (~4 chars per token) is used — good enough for budgeting,
// not for precise billing. Works across English, code, and most Latin scripts.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}

// PromptFitsContext returns true if the given system+user prompt is likely
// to fit in the model's context window, leaving room for the response.
// Small models benefit most from this check since they silently truncate.
func (c AIConfig) PromptFitsContext(systemPrompt, userPrompt string) (bool, int, int) {
	used := EstimateTokens(systemPrompt) + EstimateTokens(userPrompt)
	opts := c.ollamaOptions()
	budget := 4096
	if v, ok := opts["num_ctx"].(int); ok {
		budget = v
	}
	// Reserve headroom for the response.
	predict := 1024
	if v, ok := opts["num_predict"].(int); ok {
		predict = v
	}
	available := budget - predict
	return used <= available, used, available
}

// truncateAtBoundary truncates text to maxLen characters at the nearest word
// or sentence boundary, avoiding mid-word cuts that confuse small models.
func truncateAtBoundary(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	// Try to find a paragraph break first
	if idx := strings.LastIndex(text[:maxLen], "\n\n"); idx > maxLen/2 {
		return text[:idx]
	}
	// Fall back to last newline
	if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
		return text[:idx]
	}
	// Fall back to last space
	if idx := strings.LastIndex(text[:maxLen], " "); idx > maxLen/2 {
		return text[:idx]
	}
	return text[:maxLen]
}

// ollamaOptions returns Ollama-specific options tuned for the model size.
// Small models get a tighter context window and lower temperature for more
// focused, deterministic responses.
func (c AIConfig) ollamaOptions() map[string]interface{} {
	if c.IsSmallModel() {
		return map[string]interface{}{
			"num_ctx":     2048,
			"num_predict": 512,
			"temperature": 0.3,
		}
	}
	return map[string]interface{}{
		"num_ctx":     4096,
		"num_predict": 1024,
		"temperature": 0.7,
	}
}

// ollamaOptionsShort returns options suited for very short completions
// (ghost writer, tag suggestions). Caps num_predict aggressively so the
// model returns quickly instead of generating a long continuation.
func (c AIConfig) ollamaOptionsShort() map[string]interface{} {
	opts := c.ollamaOptions()
	opts["num_predict"] = 64
	return opts
}

// isTransientAIError returns true if the error looks like a transient
// connection issue that could succeed on retry (as opposed to a permanent
// configuration problem like a missing model or bad API key).
func isTransientAIError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Do not retry configuration errors.
	if strings.Contains(msg, "not found") ||
		strings.Contains(msg, "api key") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "not configured") {
		return false
	}
	return strings.Contains(msg, "cannot connect") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "reset") ||
		strings.Contains(msg, "temporarily")
}

// Chat sends a synchronous chat request to the configured AI provider.
// It returns the response text or an error. This is the shared entry
// point — all AI features should use this instead of making HTTP calls
// directly. Transient errors are retried once with a short backoff.
func (c AIConfig) Chat(systemPrompt, userPrompt string) (string, error) {
	resp, err := c.chatOnce(systemPrompt, userPrompt)
	if err != nil && isTransientAIError(err) {
		time.Sleep(500 * time.Millisecond)
		resp, err = c.chatOnce(systemPrompt, userPrompt)
	}
	return resp, err
}

// ChatCtx is like Chat but honors a context with optional deadline.
// For Ollama, the HTTP request is aborted when ctx expires, freeing the
// local model. Non-HTTP providers fall through to the legacy Chat path.
func (c AIConfig) ChatCtx(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.Provider == "ollama" || c.Provider == "local" || c.Provider == "" {
		resp, err := c.chatOllamaCtx(ctx, systemPrompt, userPrompt, c.ollamaOptions())
		if err != nil && isTransientAIError(err) && ctx.Err() == nil {
			time.Sleep(500 * time.Millisecond)
			resp, err = c.chatOllamaCtx(ctx, systemPrompt, userPrompt, c.ollamaOptions())
		}
		return resp, err
	}
	return c.Chat(systemPrompt, userPrompt)
}

// chatOnce performs a single chat request without retries.
func (c AIConfig) chatOnce(systemPrompt, userPrompt string) (string, error) {
	switch c.Provider {
	case "openai":
		return c.chatOpenAI(systemPrompt, userPrompt)
	case "nous":
		client := c.NewNous()
		prompt := userPrompt
		if systemPrompt != "" {
			prompt = systemPrompt + "\n\n" + userPrompt
		}
		return client.Chat(prompt)
	case "nerve":
		client := c.NewNerve()
		return client.Chat(systemPrompt, userPrompt, 120*time.Second)
	default: // "ollama", "local"
		return c.chatOllamaWithOptions(systemPrompt, userPrompt, c.ollamaOptions())
	}
}

// ChatShort is like Chat but requests a very short response. Used by
// ghostwriter, auto-tagger, and other features where the expected output
// is a single line or short list.
func (c AIConfig) ChatShort(systemPrompt, userPrompt string) (string, error) {
	return c.ChatShortCtx(context.Background(), systemPrompt, userPrompt)
}

// ChatShortCtx is like ChatShort but honors a context with optional deadline.
// If the context has a deadline, the HTTP request is aborted when it expires.
func (c AIConfig) ChatShortCtx(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := c.chatShortOnce(ctx, systemPrompt, userPrompt)
	if err != nil && isTransientAIError(err) && ctx.Err() == nil {
		time.Sleep(500 * time.Millisecond)
		resp, err = c.chatShortOnce(ctx, systemPrompt, userPrompt)
	}
	return resp, err
}

func (c AIConfig) chatShortOnce(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.Provider == "ollama" || c.Provider == "local" || c.Provider == "" {
		return c.chatOllamaCtx(ctx, systemPrompt, userPrompt, c.ollamaOptionsShort())
	}
	// Other providers don't expose an easy token limit — fall through.
	return c.chatOnce(systemPrompt, userPrompt)
}

// chatOllamaCtx is the context-aware variant of chatOllamaWithOptions. The
// HTTP request is cancelled when ctx is done, freeing the local model.
func (c AIConfig) chatOllamaCtx(ctx context.Context, systemPrompt, userPrompt string, options map[string]interface{}) (string, error) {
	url := c.OllamaEndpoint()
	model := c.ModelOrDefault("qwen2.5:0.5b")

	messages := []chatMessage{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		messages = append([]chatMessage{{Role: "system", Content: systemPrompt}}, messages...)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
		"options":  options,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", url)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("model %q not found — run: ollama pull %s", model, model)
		}
		return "", fmt.Errorf("Ollama error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if chatResp.Error != "" {
		return "", fmt.Errorf("Ollama: %s", chatResp.Error)
	}
	return chatResp.Message.Content, nil
}

// chatOllama sends a non-streaming chat request to Ollama's /api/chat endpoint.
func (c AIConfig) chatOllama(systemPrompt, userPrompt string) (string, error) {
	return c.chatOllamaWithOptions(systemPrompt, userPrompt, c.ollamaOptions())
}

// chatOllamaWithOptions is like chatOllama but uses caller-supplied options
// (e.g. for short-form completions with a smaller num_predict).
func (c AIConfig) chatOllamaWithOptions(systemPrompt, userPrompt string, options map[string]interface{}) (string, error) {
	url := c.OllamaEndpoint()
	model := c.ModelOrDefault("qwen2.5:0.5b")

	messages := []chatMessage{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		messages = append([]chatMessage{{Role: "system", Content: systemPrompt}}, messages...)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
		"options":  options,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := aiHTTPClient.Post(url+"/api/chat", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", url)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("model %q not found — run: ollama pull %s", model, model)
		}
		return "", fmt.Errorf("Ollama error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if chatResp.Error != "" {
		return "", fmt.Errorf("Ollama: %s", chatResp.Error)
	}
	return chatResp.Message.Content, nil
}

// chatOpenAI sends a non-streaming chat request to OpenAI's API.
func (c AIConfig) chatOpenAI(systemPrompt, userPrompt string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("OpenAI API key not configured — set it in Settings (Ctrl+,)")
	}
	model := c.ModelOrDefault("gpt-4o-mini")

	messages := []chatMessage{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		messages = append([]chatMessage{{Role: "system", Content: systemPrompt}}, messages...)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": messages,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach OpenAI API — check your internet connection")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI error %d: %s", resp.StatusCode, string(body))
	}

	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned no choices")
	}
	return openaiResp.Choices[0].Message.Content, nil
}
