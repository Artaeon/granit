package agentruntime

import (
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/config"
)

// NewLLM picks the right backend per provider. We don't exercise the
// network — that's a contract test for the implementation, not the
// factory. Just check we get a non-nil LLM with the right concrete
// type for each branch.
func TestNewLLM_ProviderSwitch(t *testing.T) {
	cases := []struct {
		name     string
		cfg      config.Config
		wantType string
		wantErr  string // substring match; "" = no error expected
	}{
		{"openai with key",
			config.Config{AIProvider: "openai", OpenAIKey: "sk-test", OpenAIModel: "gpt-4o-mini"},
			"openAILLM", ""},
		{"openai without key errors",
			config.Config{AIProvider: "openai"},
			"", "no API key"},
		{"openai default model",
			config.Config{AIProvider: "openai", OpenAIKey: "sk-test"},
			"openAILLM", ""},
		{"ollama default",
			config.Config{AIProvider: "ollama"},
			"ollamaLLM", ""},
		{"local alias for ollama",
			config.Config{AIProvider: "local"},
			"ollamaLLM", ""},
		{"empty provider falls through to ollama",
			config.Config{},
			"ollamaLLM", ""},
		{"unknown with openai key falls back to openai",
			config.Config{AIProvider: "weird-provider", OpenAIKey: "sk-test"},
			"openAILLM", ""},
		{"unknown without keys errors",
			config.Config{AIProvider: "weird-provider"},
			"", "unsupported ai_provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			llm, err := NewLLM(tc.cfg)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotType := concreteName(llm)
			if gotType != tc.wantType {
				t.Errorf("type = %s, want %s", gotType, tc.wantType)
			}
		})
	}
}

// concreteName extracts the non-pointer type name of v for assertion.
// Uses reflection-free string surgery against fmt's "%T" output to
// avoid pulling in reflect for one test.
func concreteName(v any) string {
	s := stringTypeOf(v)
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// stringTypeOf returns "*pkg.Type" without importing reflect.
func stringTypeOf(v any) string {
	type interfacer interface{ Complete(any, any) (string, error) }
	_ = interfacer(nil) // unused; just keeps imports clean
	// fmt.Sprintf("%T") is the standard way; avoid pulling fmt though.
	switch v.(type) {
	case *openAILLM:
		return "openAILLM"
	case *ollamaLLM:
		return "ollamaLLM"
	default:
		return "unknown"
	}
}
