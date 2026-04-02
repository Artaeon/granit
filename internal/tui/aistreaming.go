package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
func streamOllamaChat(baseURL, model, systemPrompt, userPrompt, tag string) <-chan tea.Msg {
	ch := make(chan tea.Msg, 64)

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

		reqBody, _ := json.Marshal(map[string]interface{}{
			"model":    model,
			"messages": messages,
			"stream":   true,
		})

		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Post(baseURL+"/api/chat", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", baseURL)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("Ollama error %d: %s", resp.StatusCode, string(body))}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
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
				ch <- streamChunkMsg{text: chunk.Message.Content, tag: tag}
			}

			if chunk.Done {
				break
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- streamDoneMsg{tag: tag, err: err}
			return
		}

		ch <- streamDoneMsg{tag: tag}
	}()

	return ch
}

// streamOllamaGenerate sends a streaming request to Ollama's /api/generate endpoint.
func streamOllamaGenerate(baseURL, model, prompt, tag string) <-chan tea.Msg {
	ch := make(chan tea.Msg, 64)

	go func() {
		defer close(ch)

		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}

		reqBody, _ := json.Marshal(map[string]interface{}{
			"model":  model,
			"prompt": prompt,
			"stream": true,
		})

		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Post(baseURL+"/api/generate", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", baseURL)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("Ollama error %d: %s", resp.StatusCode, string(body))}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk struct {
				Response string `json:"response"`
				Done     bool   `json:"done"`
			}
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}

			if chunk.Response != "" {
				ch <- streamChunkMsg{text: chunk.Response, tag: tag}
			}

			if chunk.Done {
				break
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- streamDoneMsg{tag: tag, err: err}
			return
		}

		ch <- streamDoneMsg{tag: tag}
	}()

	return ch
}

// streamOpenAI sends a streaming request to OpenAI's chat completions endpoint.
func streamOpenAI(apiKey, model, systemPrompt, userPrompt, tag string) <-chan tea.Msg {
	ch := make(chan tea.Msg, 64)

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

		reqBody, _ := json.Marshal(map[string]interface{}{
			"model":    model,
			"messages": messages,
			"stream":   true,
		})

		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(reqBody))
		if err != nil {
			ch <- streamDoneMsg{tag: tag, err: err}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			ch <- streamDoneMsg{tag: tag, err: fmt.Errorf("cannot reach OpenAI API — check your internet connection")}
			return
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
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
				ch <- streamChunkMsg{text: chunk.Choices[0].Delta.Content, tag: tag}
			}
		}

		ch <- streamDoneMsg{tag: tag}
	}()

	return ch
}

// sendToAIStreaming dispatches a streaming AI request to the configured provider.
// It returns a channel of tea.Msg values. For providers that don't support
// streaming (nous, nerve), it falls back to a single response sent as one
// chunk followed by done.
func sendToAIStreaming(ai AIConfig, systemPrompt, userPrompt, tag string) <-chan tea.Msg {
	switch ai.Provider {
	case "openai":
		model := ai.Model
		if model == "" {
			model = "gpt-4o-mini"
		}
		return streamOpenAI(ai.APIKey, model, systemPrompt, userPrompt, tag)

	case "nous":
		ch := make(chan tea.Msg, 2)
		go func() {
			defer close(ch)
			client := NewNousClient(ai.NousURL, ai.NousAPIKey)
			prompt := userPrompt
			if systemPrompt != "" {
				prompt = systemPrompt + "\n\n" + userPrompt
			}
			resp, err := client.Chat(prompt)
			if err != nil {
				ch <- streamDoneMsg{tag: tag, err: err}
				return
			}
			ch <- streamChunkMsg{text: resp, tag: tag}
			ch <- streamDoneMsg{tag: tag}
		}()
		return ch

	case "nerve":
		ch := make(chan tea.Msg, 2)
		go func() {
			defer close(ch)
			client := ai.NewNerve()
			resp, err := client.Chat(systemPrompt, userPrompt, 120*time.Second)
			if err != nil {
				ch <- streamDoneMsg{tag: tag, err: err}
				return
			}
			ch <- streamChunkMsg{text: resp, tag: tag}
			ch <- streamDoneMsg{tag: tag}
		}()
		return ch

	default: // "ollama"
		url := ai.OllamaEndpoint()
		model := ai.ModelOrDefault("qwen2.5:0.5b")
		return streamOllamaChat(url, model, systemPrompt, userPrompt, tag)
	}
}
