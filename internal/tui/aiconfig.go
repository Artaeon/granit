package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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

// Chat sends a synchronous chat request to the configured AI provider.
// It returns the response text or an error. This is the shared entry
// point — all AI features should use this instead of making HTTP calls
// directly.
func (c AIConfig) Chat(systemPrompt, userPrompt string) (string, error) {
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
		return c.chatOllama(systemPrompt, userPrompt)
	}
}

// chatOllama sends a non-streaming chat request to Ollama's /api/chat endpoint.
func (c AIConfig) chatOllama(systemPrompt, userPrompt string) (string, error) {
	url := c.OllamaEndpoint()
	model := c.ModelOrDefault("qwen2.5:1.5b")

	messages := []chatMessage{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		messages = append([]chatMessage{{Role: "system", Content: systemPrompt}}, messages...)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Post(url+"/api/chat", "application/json", bytes.NewReader(reqBody))
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

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
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
