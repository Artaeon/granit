package agents

import "context"

// LLM is the minimal interface the agent runtime needs from a
// language model. Implementations live elsewhere (tui/agentllm.go
// for the production bridge to granit's existing AIConfig; tests
// install MockLLM to drive deterministic transcripts).
//
// We deliberately keep this surface tiny — Complete in, completion
// out — so the interface can be satisfied by any backend (Ollama,
// OpenAI, Claude, a local llama.cpp binary) without either side
// growing capability methods. Tool calling is encoded in the
// prompt + parsed from the completion, NOT a separate mechanism.
type LLM interface {
	// Complete sends the prompt to the model and returns its full
	// response. ctx carries cancellation; implementations MUST
	// honour ctx.Done() to keep the agent loop interruptible.
	//
	// Errors should describe the failure in user-actionable
	// terms ("Ollama is not running on localhost:11434") rather
	// than leaking SDK-internal stack traces — they're surfaced
	// directly in the agent transcript.
	Complete(ctx context.Context, prompt string) (string, error)
}

// MockLLM is a deterministic LLM for tests: returns successive
// canned responses from Responses on each Complete call. Records
// every prompt it received in Prompts so tests can assert on the
// agent's prompt construction without coupling to its internal
// template format.
//
// When Responses is exhausted, returns "" so the agent terminates
// (and the test fails clearly with "exhausted" in the transcript).
type MockLLM struct {
	Responses []string
	Prompts   []string
	calls     int
}

// Complete returns the next canned response, or "" when exhausted.
func (m *MockLLM) Complete(_ context.Context, prompt string) (string, error) {
	m.Prompts = append(m.Prompts, prompt)
	if m.calls >= len(m.Responses) {
		m.calls++
		return "", nil
	}
	resp := m.Responses[m.calls]
	m.calls++
	return resp, nil
}

// LLMFunc adapts a plain function to the LLM interface — useful
// when you don't want to define a struct for a single test or
// inline shim.
type LLMFunc func(ctx context.Context, prompt string) (string, error)

// Complete calls the underlying func.
func (f LLMFunc) Complete(ctx context.Context, prompt string) (string, error) {
	return f(ctx, prompt)
}
