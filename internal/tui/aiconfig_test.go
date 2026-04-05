package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestIsSmallModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		// Parameter-count suffixes
		{"qwen2.5:0.5b", true},
		{"qwen2.5:1.5b", true},
		{"llama3.2:1b", true},
		{"llama3.2:3b", true},
		{"gemma:2b", true},
		{"phi3:mini", true},
		{"tinyllama", true},
		{"gemma2:2b", true},
		// Named variants
		{"phi3.5:mini", true},
		// Not small
		{"llama3.1:8b", false},
		{"qwen2.5:7b", false},
		{"gpt-4o-mini", false},
		{"gpt-4o", false},
		{"mistral:7b", false},
		{"", false},
		// Uppercase handling
		{"Qwen2.5:0.5B", true},
		{"LLAMA3.2:1B", true},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			c := AIConfig{Model: tt.model}
			if got := c.IsSmallModel(); got != tt.want {
				t.Errorf("IsSmallModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestMaxPromptContext(t *testing.T) {
	small := AIConfig{Model: "qwen2.5:0.5b"}
	large := AIConfig{Model: "llama3.1:8b"}
	if small.MaxPromptContext() >= large.MaxPromptContext() {
		t.Errorf("small model context (%d) should be less than large (%d)",
			small.MaxPromptContext(), large.MaxPromptContext())
	}
}

func TestOllamaOptions(t *testing.T) {
	small := AIConfig{Model: "qwen2.5:0.5b"}
	large := AIConfig{Model: "llama3.1:8b"}

	smallOpts := small.ollamaOptions()
	largeOpts := large.ollamaOptions()

	// Small model should have smaller num_ctx and num_predict
	if smallOpts["num_ctx"].(int) >= largeOpts["num_ctx"].(int) {
		t.Errorf("small num_ctx (%v) should be less than large (%v)",
			smallOpts["num_ctx"], largeOpts["num_ctx"])
	}
	if smallOpts["num_predict"].(int) >= largeOpts["num_predict"].(int) {
		t.Errorf("small num_predict (%v) should be less than large (%v)",
			smallOpts["num_predict"], largeOpts["num_predict"])
	}
	// Small model should have lower temperature (more deterministic)
	if smallOpts["temperature"].(float64) >= largeOpts["temperature"].(float64) {
		t.Errorf("small temperature (%v) should be less than large (%v)",
			smallOpts["temperature"], largeOpts["temperature"])
	}
}

func TestOllamaOptionsShort(t *testing.T) {
	c := AIConfig{Model: "llama3.1:8b"}
	normal := c.ollamaOptions()
	short := c.ollamaOptionsShort()
	if short["num_predict"].(int) >= normal["num_predict"].(int) {
		t.Errorf("short num_predict (%v) should be less than normal (%v)",
			short["num_predict"], normal["num_predict"])
	}
}

func TestTruncateAtBoundary(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"no truncation needed", "hello world", 20, "hello world"},
		{"exact length", "hello", 5, "hello"},
		{"word boundary", "the quick brown fox jumps", 15, "the quick"},
		{"paragraph break preferred", "para one\n\npara two\n\npara three", 15, "para one"},
		{"newline preferred over space", "line one\nline two three four", 15, "line one"},
		{"no boundary found returns raw cut", "supercalifragilistic", 5, "super"},
		{"empty string", "", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateAtBoundary(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateAtBoundary(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTruncateAtBoundaryNeverExceeds(t *testing.T) {
	// Property: result must never exceed maxLen.
	inputs := []string{
		"hello world",
		"the quick brown fox jumps over the lazy dog",
		"single-very-long-word-with-no-spaces-at-all",
		"a b c d e f g h i j k l m n o p",
		"multi\nline\ntext\nhere",
	}
	for _, input := range inputs {
		for _, maxLen := range []int{0, 1, 5, 10, 20, 100} {
			got := truncateAtBoundary(input, maxLen)
			if len(got) > maxLen && len(got) > len(input) {
				t.Errorf("truncateAtBoundary(%q, %d) returned %q (len %d) > maxLen", input, maxLen, got, len(got))
			}
			if maxLen >= len(input) && got != input {
				t.Errorf("truncateAtBoundary(%q, %d) = %q, want unchanged", input, maxLen, got)
			}
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"hello world", 3},
		{strings.Repeat("x", 400), 100},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("len=%d", len(tt.input)), func(t *testing.T) {
			if got := EstimateTokens(tt.input); got != tt.want {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestPromptFitsContext(t *testing.T) {
	c := AIConfig{Model: "qwen2.5:0.5b"} // small, num_ctx=2048, num_predict=512
	// Available budget: 2048 - 512 = 1536 tokens ≈ 6144 chars

	// Tiny prompt should fit.
	fits, _, _ := c.PromptFitsContext("sys", "hi")
	if !fits {
		t.Error("tiny prompt should fit")
	}

	// Huge prompt should not fit.
	huge := strings.Repeat("x", 20000)
	fits, used, available := c.PromptFitsContext("sys", huge)
	if fits {
		t.Errorf("huge prompt should not fit, used=%d available=%d", used, available)
	}
	if used <= available {
		t.Errorf("used (%d) should exceed available (%d) for non-fitting prompt", used, available)
	}
}

func TestIsTransientAIError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"timeout", errors.New("context deadline exceeded: i/o timeout"), true},
		{"eof", errors.New("unexpected EOF"), true},
		{"reset", errors.New("connection reset by peer"), true},
		{"cannot connect", errors.New("cannot connect to Ollama"), true},
		// Permanent configuration errors should NOT retry
		{"model not found", errors.New("model qwen not found"), false},
		{"api key", errors.New("invalid api key"), false},
		{"unauthorized", errors.New("401 unauthorized"), false},
		{"not configured", errors.New("openai not configured"), false},
		// Generic error — don't retry by default
		{"generic", errors.New("something went wrong"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientAIError(tt.err); got != tt.want {
				t.Errorf("isTransientAIError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestModelOrDefault(t *testing.T) {
	c := AIConfig{Model: "custom"}
	if got := c.ModelOrDefault("default"); got != "custom" {
		t.Errorf("ModelOrDefault with set Model = %q, want %q", got, "custom")
	}
	c2 := AIConfig{}
	if got := c2.ModelOrDefault("default"); got != "default" {
		t.Errorf("ModelOrDefault with empty Model = %q, want %q", got, "default")
	}
}

func TestOllamaEndpoint(t *testing.T) {
	c := AIConfig{OllamaURL: "http://custom:1234"}
	if got := c.OllamaEndpoint(); got != "http://custom:1234" {
		t.Errorf("OllamaEndpoint = %q, want custom URL", got)
	}
	c2 := AIConfig{}
	if got := c2.OllamaEndpoint(); got != "http://localhost:11434" {
		t.Errorf("OllamaEndpoint empty = %q, want default", got)
	}
}
