package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// NousClient — HTTP client for the local Nous AI server
// ---------------------------------------------------------------------------

// NousClient communicates with a local Nous server.
type NousClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewNousClient creates a NousClient configured for the given endpoint.
func NewNousClient(url, apiKey string) *NousClient {
	if url == "" {
		url = "http://localhost:3333"
	}
	return &NousClient{
		baseURL: url,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// ---------------------------------------------------------------------------
// Request / response types
// ---------------------------------------------------------------------------

type nousChatRequest struct {
	Message string `json:"message"`
}

type nousChatResponse struct {
	Answer     string `json:"answer"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

type nousHealthResponse struct {
	Status string `json:"status"`
}

type nousStatusResponse struct {
	Version     string `json:"version,omitempty"`
	Model       string `json:"model,omitempty"`
	Uptime      string `json:"uptime,omitempty"`
	ToolCount   int    `json:"tool_count,omitempty"`
	Percepts    int    `json:"percepts,omitempty"`
	Goals       int    `json:"goals,omitempty"`
	QueuedJobs  int    `json:"queued_jobs,omitempty"`
	RunningJobs int    `json:"running_jobs,omitempty"`
}

// ---------------------------------------------------------------------------
// API methods
// ---------------------------------------------------------------------------

// Chat sends a message and returns the response text.
func (nc *NousClient) Chat(message string) (string, error) {
	return nc.ChatCtx(context.Background(), message)
}

// ChatCtx is like Chat but cancels the underlying HTTP request when ctx
// is done. Callers with a deadline (e.g. ghost writer) can use this to
// drop slow requests instead of waiting for the client's own timeout.
func (nc *NousClient) ChatCtx(ctx context.Context, message string) (string, error) {
	reqBody := nousChatRequest{
		Message: message,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", nc.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	nc.setHeaders(req)

	resp, err := nc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot connect to Nous at %s — is it running?", nc.baseURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("nous error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp nousChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}

	return chatResp.Answer, nil
}

// ChatWithContext sends a message with vault context for grounded responses.
func (nc *NousClient) ChatWithContext(message string, context string) (string, error) {
	prompt := message
	if context != "" {
		prompt = fmt.Sprintf("Context from the user's notes:\n%s\n\nQuestion: %s", context, message)
	}
	return nc.Chat(prompt)
}

// IngestNote teaches Nous about a note by sending it via the remember command.
func (nc *NousClient) IngestNote(path, content string) error {
	// Use Nous's /remember command to store note content as a fact
	snippet := content
	if len(snippet) > 500 {
		snippet = snippet[:500]
	}
	msg := fmt.Sprintf("/remember %s %s", path, snippet)
	_, err := nc.Chat(msg)
	return err
}

// IngestVault indexes vault notes into Nous's knowledge system via remember commands.
// Only indexes notes with substantial content (>50 chars) to avoid noise.
func (nc *NousClient) IngestVault(notes map[string]*vault.Note) (int, error) {
	count := 0
	for path, note := range notes {
		if note == nil || len(note.Content) < 50 {
			continue
		}
		if err := nc.IngestNote(path, note.Content); err != nil {
			log.Printf("nous: ingest failed for %s: %v", path, err)
			continue
		}
		count++
	}
	return count, nil
}

// TestConnection checks if Nous server is reachable.
func (nc *NousClient) TestConnection() error {
	req, err := http.NewRequest("GET", nc.baseURL+"/api/health", nil)
	if err != nil {
		return err
	}
	nc.setHeaders(req)

	resp, err := nc.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to Nous at %s — is it running?", nc.baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("nous health check failed (status %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var health nousHealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return err
	}

	if health.Status != "ok" {
		return fmt.Errorf("nous health status: %s", health.Status)
	}
	return nil
}

// GetStatus returns Nous server status info.
func (nc *NousClient) GetStatus() (string, error) {
	req, err := http.NewRequest("GET", nc.baseURL+"/api/status", nil)
	if err != nil {
		return "", err
	}
	nc.setHeaders(req)

	resp, err := nc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot connect to Nous at %s — is it running?", nc.baseURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("nous status error (status %d): %s", resp.StatusCode, string(body))
	}

	var status nousStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		// Return raw body if it doesn't match expected structure
		return string(body), nil
	}

	result := "Nous: connected"
	if status.Version != "" {
		result += fmt.Sprintf("\nVersion: %s", status.Version)
	}
	if status.Model != "" {
		result += fmt.Sprintf("\nModel: %s", status.Model)
	}
	if status.Uptime != "" {
		result += fmt.Sprintf("\nUptime: %s", status.Uptime)
	}
	if status.ToolCount > 0 {
		result += fmt.Sprintf("\nTools: %d", status.ToolCount)
	}
	return result, nil
}

// setHeaders adds common headers (Content-Type, Authorization) to a request.
func (nc *NousClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if nc.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+nc.apiKey)
	}
}
