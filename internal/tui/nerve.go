package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// NerveClient wraps calls to the nerve binary for AI chat completions.
type NerveClient struct {
	Binary   string // path to nerve binary (default: "nerve")
	Model    string // optional model override
	Provider string // optional nerve-internal provider (e.g. "ollama", "claude_code")
}

// NewNerveClient creates a client with the given config.
func NewNerveClient(binary, model, provider string) *NerveClient {
	if binary == "" {
		binary = "nerve"
	}
	return &NerveClient{Binary: binary, Model: model, Provider: provider}
}

// Chat sends a prompt to nerve and returns the response text.
// It passes the prompt via stdin in non-interactive mode.
func (nc *NerveClient) Chat(systemPrompt, userPrompt string, timeout time.Duration) (string, error) {
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"--stdin", "-n"}
	if nc.Provider != "" {
		args = append(args, "--provider", nc.Provider)
	}
	if nc.Model != "" {
		args = append(args, "--model", nc.Model)
	}

	// Combine system prompt and user prompt
	input := userPrompt
	if systemPrompt != "" {
		input = "SYSTEM: " + systemPrompt + "\n\nUSER: " + userPrompt
	}

	cmd := exec.CommandContext(ctx, nc.Binary, args...)
	cmd.Stdin = bytes.NewReader([]byte(input))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("nerve: %s", stderr.String())
		}
		return "", fmt.Errorf("nerve: %w", err)
	}

	return stdout.String(), nil
}

