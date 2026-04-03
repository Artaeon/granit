package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ollamaStatusMsg is sent after the startup Ollama check completes.
type ollamaStatusMsg struct {
	text  string
	ready bool
}

// ollamaStatus tracks the state of the Ollama connection and model.
type ollamaStatus int

const (
	ollamaUnknown    ollamaStatus = iota
	ollamaReady                   // server running, model available
	ollamaNoServer                // server not running
	ollamaPulling                 // model being pulled
	ollamaNoModel                 // server running but model not available
)

// ollamaState caches the connection state to avoid repeated checks.
var (
	ollamaState = ollamaUnknown
	ollamaMu    sync.RWMutex
)

// Shared HTTP clients for Ollama operations.
var (
	ollamaCheckClient = &http.Client{Timeout: 2 * time.Second}
	ollamaPullClient  = &http.Client{Timeout: 300 * time.Second}
)

// setOllamaState updates the cached state under the write lock.
func setOllamaState(s ollamaStatus) {
	ollamaMu.Lock()
	ollamaState = s
	ollamaMu.Unlock()
}

// OllamaCheck tests if Ollama is running and the configured model is available.
// Returns a user-friendly status message and whether AI is ready to use.
func OllamaCheck(baseURL, model string) (string, bool) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "qwen2.5:0.5b"
	}

	// Check if server is running
	resp, err := ollamaCheckClient.Get(baseURL + "/api/tags")
	if err != nil {
		setOllamaState(ollamaNoServer)
		return "Ollama not running. Install: curl -fsSL https://ollama.ai/install.sh | sh", false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		setOllamaState(ollamaNoServer)
		return "Ollama not responding", false
	}

	// Parse available models
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		setOllamaState(ollamaNoServer)
		return "Ollama response error", false
	}

	// Check if our model is available
	for _, m := range result.Models {
		// Match "qwen2.5:0.5b" or "qwen2.5:0.5b-instruct" etc.
		if m.Name == model || strings.HasPrefix(m.Name, model) {
			setOllamaState(ollamaReady)
			return fmt.Sprintf("Ready (%s)", model), true
		}
	}

	setOllamaState(ollamaNoModel)
	return fmt.Sprintf("Model %s not found. Run: ollama pull %s", model, model), false
}

// OllamaPullModel sends a pull request to Ollama to download the model.
// This is non-blocking — Ollama downloads in the background.
func OllamaPullModel(baseURL, model string) error {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "qwen2.5:0.5b"
	}

	reqBody, err := json.Marshal(map[string]interface{}{"name": model, "stream": false})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	resp, err := ollamaPullClient.Post(baseURL+"/api/pull", "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("pull returned status %d", resp.StatusCode)
	}

	setOllamaState(ollamaReady)
	return nil
}

// OllamaIsReady returns true if the last check found Ollama ready.
func OllamaIsReady() bool {
	ollamaMu.RLock()
	defer ollamaMu.RUnlock()
	return ollamaState == ollamaReady
}

// OllamaEnsureModel checks availability and auto-pulls if needed.
// Returns a status message suitable for display in a toast.
func OllamaEnsureModel(baseURL, model string) string {
	msg, ready := OllamaCheck(baseURL, model)
	if ready {
		return msg
	}

	ollamaMu.RLock()
	noServer := ollamaState == ollamaNoServer
	ollamaMu.RUnlock()
	if noServer {
		return msg // can't auto-pull without server
	}

	// Server running but model missing — try to pull
	setOllamaState(ollamaPulling)
	if err := OllamaPullModel(baseURL, model); err != nil {
		return fmt.Sprintf("Auto-pull failed: %v", err)
	}

	return fmt.Sprintf("Model %s pulled successfully", model)
}
