package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// streamChunkMsg carries a single token/chunk from a streaming AI response.
type streamChunkMsg struct {
	text string
	tag  string // identifies which overlay this chunk belongs to
}

// streamDoneMsg signals that streaming is complete.
type streamDoneMsg struct {
	tag string
	err error
}

// streamCmd returns a tea.Cmd that reads the next message from a streaming
// channel. When the channel closes, it returns a streamDoneMsg.
func streamCmd(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// streamOllamaChat sends a streaming request to Ollama's /api/chat endpoint.
// It returns a channel that yields streamChunkMsg and a final streamDoneMsg.
// If ctx is cancelled, the HTTP request is aborted and the channel closes.
func streamOllamaChat(ctx context.Context, baseURL, model, systemPrompt, userPrompt, tag string, options map[string]interface{}) <-chan tea.Msg {
	ch := make(chan tea.Msg, 64)
	if ctx == nil {
		ctx = context.Background()
	}

	go func() {
		defer close(ch)

		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}

		type message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}

		messages := []message{{Role: "user", Content: userPrompt}}
		if systemPrompt != "" {
			messages = append([]message{{Role: "system", Content: systemPrompt}}, messages...)
		}

		req := map[string]interface{}{
			"model":    model,
			"messages": messages,
			"stream":   true,
		}
		if len(options) > 0 {
			req["options"] = options
		}
		reqBody, err := json.Marshal(req)
		if err != nil {
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("failed to build request: %w", err)}
			return
		}

		// Streams use the shared aiStreamingClient (no client-level timeout)
		// and rely on ctx for cancellation. Small models still get a tighter
		// per-stream deadline because their reduced context window means a
		// successful response should arrive promptly — if it doesn't, the
		// model is likely stuck and we want to bail.
		streamCtx := ctx
		if numCtx, ok := options["num_ctx"]; ok {
			if v, ok := numCtx.(int); ok && v <= 2048 {
				var cancelStream context.CancelFunc
				streamCtx, cancelStream = context.WithTimeout(ctx, 90*time.Second)
				defer cancelStream()
			}
		}
		// Retry the initial connection once on transient failures — small
		// HTTP hiccups shouldn't kill a streaming request before it starts.
		doPost := func() (*http.Response, error) {
			req, reqErr := http.NewRequestWithContext(streamCtx, "POST", baseURL+"/api/chat", bytes.NewReader(reqBody))
			if reqErr != nil {
				return nil, reqErr
			}
			req.Header.Set("Content-Type", "application/json")
			return aiStreamingClient.Do(req)
		}
		var resp *http.Response
		resp, err = doPost()
		if err != nil && ctx.Err() == nil {
			time.Sleep(500 * time.Millisecond)
			resp, err = doPost()
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				// Silently return — user cancelled.
				return
			}
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", baseURL)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("ollama error %d (could not read body)", resp.StatusCode)}
			} else {
				ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))}
			}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}

			if chunk.Message.Content != "" {
				// Non-blocking send: if consumer has stopped reading
				// (e.g. user cancelled) we stop streaming promptly.
				select {
				case ch <- streamChunkMsg{text: chunk.Message.Content, tag: tag}:
				case <-ctx.Done():
					return
				}
			}

			if chunk.Done {
				break
			}
		}

		if err := scanner.Err(); err != nil {
			// If the context was cancelled, the error is from the aborted
			// connection — don't surface it as a failure to the user.
			if ctx.Err() != nil {
				return
			}
			ch <- streamDoneMsg{tag: tag, err: err}
			return
		}

		ch <- streamDoneMsg{tag: tag}
	}()

	return ch
}

// streamOpenAI sends a streaming request to OpenAI's chat completions endpoint.
// If ctx is cancelled, the HTTP request is aborted.
func streamOpenAI(ctx context.Context, apiKey, model, systemPrompt, userPrompt, tag string) <-chan tea.Msg {
	ch := make(chan tea.Msg, 64)
	if ctx == nil {
		ctx = context.Background()
	}

	go func() {
		defer close(ch)

		type message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}

		messages := []message{{Role: "user", Content: userPrompt}}
		if systemPrompt != "" {
			messages = append([]message{{Role: "system", Content: systemPrompt}}, messages...)
		}

		reqBody, err := json.Marshal(map[string]interface{}{
			"model":    model,
			"messages": messages,
			"stream":   true,
		})
		if err != nil {
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("failed to build request: %w", err)}
			return
		}

		doPost := func() (*http.Response, error) {
			req, reqErr := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(reqBody))
			if reqErr != nil {
				return nil, reqErr
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+apiKey)
			return aiStreamingClient.Do(req)
		}
		var resp *http.Response
		resp, err = doPost()
		if err != nil && ctx.Err() == nil {
			time.Sleep(500 * time.Millisecond)
			resp, err = doPost()
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("cannot reach OpenAI API — check your internet connection")}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("OpenAI error %d (could not read body)", resp.StatusCode)}
				return
			}
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("OpenAI error %d: %s", resp.StatusCode, string(body))}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case ch <- streamChunkMsg{text: chunk.Choices[0].Delta.Content, tag: tag}:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if ctx.Err() != nil {
				return
			}
			ch <- streamDoneMsg{tag: tag, err: err}
			return
		}

		ch <- streamDoneMsg{tag: tag}
	}()

	return ch
}

// sendToAIStreamingCtx dispatches a streaming AI request and returns a cancel function
// the caller can invoke to abort an in-flight request. For HTTP providers
// (ollama, openai) this aborts the actual network request, freeing the local
// model resources immediately.
func sendToAIStreamingCtx(parent context.Context, ai AIConfig, systemPrompt, userPrompt, tag string) (<-chan tea.Msg, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	switch ai.Provider {
	case "openai":
		model := ai.Model
		if model == "" {
			model = "gpt-4o-mini"
		}
		return streamOpenAI(ctx, ai.APIKey, model, systemPrompt, userPrompt, tag), cancel

	case "nous":
		ch := make(chan tea.Msg, 2)
		go func() {
			defer close(ch)
			client := NewNousClient(ai.NousURL, ai.NousAPIKey)
			prompt := userPrompt
			if systemPrompt != "" {
				prompt = systemPrompt + "\n\n" + userPrompt
			}
			resp, err := client.ChatCtx(ctx, prompt)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				ch <- streamDoneMsg{tag: tag, err: err}
				return
			}
			ch <- streamChunkMsg{text: resp, tag: tag}
			ch <- streamDoneMsg{tag: tag}
		}()
		return ch, cancel

	case "nerve":
		ch := make(chan tea.Msg, 2)
		go func() {
			defer close(ch)
			client := ai.NewNerve()
			resp, err := client.ChatCtx(ctx, systemPrompt, userPrompt)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				ch <- streamDoneMsg{tag: tag, err: err}
				return
			}
			ch <- streamChunkMsg{text: resp, tag: tag}
			ch <- streamDoneMsg{tag: tag}
		}()
		return ch, cancel

	default: // "ollama"
		url := ai.OllamaEndpoint()
		model := ai.ModelOrDefault("qwen2.5:0.5b")
		return streamOllamaChat(ctx, url, model, systemPrompt, userPrompt, tag, ai.ollamaOptions()), cancel
	}
}

// renderStreamPreview renders a tail window of streaming AI output into the
// given builder. Used by overlays to show live progress during AI generation.
func renderStreamPreview(b *strings.Builder, preview string, height, width int) {
	if preview == "" {
		return
	}
	b.WriteString(DimStyle.Render("  ── AI output ──"))
	b.WriteString("\n")
	maxLines := height - 16
	if maxLines < 4 {
		maxLines = 4
	}
	lines := strings.Split(preview, "\n")
	start := 0
	if len(lines) > maxLines {
		start = len(lines) - maxLines
	}
	lineStyle := lipgloss.NewStyle().Foreground(overlay0)
	maxW := width - 8
	for _, line := range lines[start:] {
		if len(line) > maxW {
			line = line[:maxW]
		}
		b.WriteString("  " + lineStyle.Render(line))
		b.WriteString("\n")
	}
}
