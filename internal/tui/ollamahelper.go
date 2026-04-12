package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
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

// ollamaProc tracks the ollama serve process started by granit so we can
// stop it cleanly on exit and avoid zombie processes.
var (
	ollamaProc   *os.Process
	ollamaProcMu sync.Mutex
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

	// Check if our model is available.
	// Match exactly, or with a ":" or "-" suffix (e.g. "qwen2.5:0.5b" matches
	// "qwen2.5:0.5b-instruct"). This avoids false positives like "phi" matching
	// "phi3" when the user configured just "phi".
	for _, m := range result.Models {
		if m.Name == model {
			setOllamaState(ollamaReady)
			return fmt.Sprintf("Ready (%s)", model), true
		}
		if strings.HasPrefix(m.Name, model) && len(m.Name) > len(model) {
			next := m.Name[len(model)]
			if next == ':' || next == '-' {
				setOllamaState(ollamaReady)
				return fmt.Sprintf("Ready (%s)", model), true
			}
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
	resp, err := ollamaPullClient.Post(baseURL+"/api/pull", "application/json", bytes.NewReader(reqBody))
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

// OllamaStartServer starts "ollama serve" in the background and waits until
// the server is reachable. The process is tracked so it can be stopped
// cleanly on exit via OllamaStopServer. Returns a user-friendly status
// message and whether the server is now running.
func OllamaStartServer(baseURL string) (string, bool) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Already running?
	if resp, err := ollamaCheckClient.Get(baseURL + "/api/tags"); err == nil {
		resp.Body.Close()
		return "Ollama is already running", true
	}

	// Check binary exists
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		return "Ollama not installed. Use Setup Ollama first.", false
	}

	cmd := exec.Command(ollamaPath, "serve")
	// Put the process in its own process group so we can kill it cleanly
	// without affecting granit itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("Failed to start ollama: %v", err), false
	}

	ollamaProcMu.Lock()
	ollamaProc = cmd.Process
	ollamaProcMu.Unlock()

	// Reap the child in the background to prevent zombies.
	go func() {
		_ = cmd.Wait()
		ollamaProcMu.Lock()
		if ollamaProc != nil && ollamaProc.Pid == cmd.Process.Pid {
			ollamaProc = nil
		}
		ollamaProcMu.Unlock()
	}()

	// Wait for the server to become reachable (up to 8 seconds).
	for i := 0; i < 16; i++ {
		time.Sleep(500 * time.Millisecond)
		if resp, err := ollamaCheckClient.Get(baseURL + "/api/tags"); err == nil {
			resp.Body.Close()
			return "Ollama started", true
		}
	}

	return "Ollama started but not yet responding — try again shortly", false
}

// OllamaStopServer stops the ollama serve process that was started by
// OllamaStartServer. It sends SIGTERM so ollama can shut down gracefully.
// This is a no-op if we didn't start the process.
func OllamaStopServer() {
	ollamaProcMu.Lock()
	proc := ollamaProc
	ollamaProc = nil
	ollamaProcMu.Unlock()

	if proc == nil {
		return
	}
	// Kill the entire process group to catch any children ollama may have spawned.
	_ = syscall.Kill(-proc.Pid, syscall.SIGTERM)
}

// OllamaWeStartedServer returns true if granit launched the ollama serve process.
func OllamaWeStartedServer() bool {
	ollamaProcMu.Lock()
	defer ollamaProcMu.Unlock()
	return ollamaProc != nil
}

// OllamaEnsureModel checks availability and auto-pulls if needed.
// Returns a status message suitable for display in a toast.
func OllamaEnsureModel(baseURL, model string) string {
	msg, ready := OllamaCheck(baseURL, model)
	if ready {
		return msg
	}

	// Atomically check state and transition to pulling to avoid TOCTOU race.
	ollamaMu.Lock()
	if ollamaState == ollamaNoServer {
		ollamaMu.Unlock()
		return msg // can't auto-pull without server
	}
	ollamaState = ollamaPulling
	ollamaMu.Unlock()
	if err := OllamaPullModel(baseURL, model); err != nil {
		return fmt.Sprintf("Auto-pull failed: %v", err)
	}

	return fmt.Sprintf("Model %s pulled successfully", model)
}
