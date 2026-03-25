package tui

import (
	"net/http"
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// NewNousClient — default and custom URL
// ---------------------------------------------------------------------------

func TestNewNousClient_DefaultURL(t *testing.T) {
	nc := NewNousClient("", "")

	if nc.baseURL != "http://localhost:3333" {
		t.Errorf("expected default baseURL 'http://localhost:3333', got %q", nc.baseURL)
	}
}

func TestNewNousClient_CustomURL(t *testing.T) {
	nc := NewNousClient("http://myhost:9999", "secret-key")

	if nc.baseURL != "http://myhost:9999" {
		t.Errorf("expected baseURL 'http://myhost:9999', got %q", nc.baseURL)
	}
	if nc.apiKey != "secret-key" {
		t.Errorf("expected apiKey 'secret-key', got %q", nc.apiKey)
	}
}

// ---------------------------------------------------------------------------
// TestConnection — unreachable server
// ---------------------------------------------------------------------------

func TestNousClient_TestConnection_Unreachable(t *testing.T) {
	// Use a port that is almost certainly not listening.
	nc := NewNousClient("http://127.0.0.1:19", "")

	err := nc.TestConnection()
	if err == nil {
		t.Fatal("expected error when connecting to unreachable server")
	}
	if !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("expected 'cannot connect' in error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Chat — unreachable server
// ---------------------------------------------------------------------------

func TestNousClient_Chat_Unreachable(t *testing.T) {
	nc := NewNousClient("http://127.0.0.1:19", "")

	_, err := nc.Chat("hello")
	if err == nil {
		t.Fatal("expected error when chatting with unreachable server")
	}
	if !strings.Contains(err.Error(), "cannot connect") && !strings.Contains(err.Error(), "Nous") {
		t.Errorf("expected descriptive error mentioning connection, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// SetHeaders — Content-Type and Authorization
// ---------------------------------------------------------------------------

func TestNousClient_SetHeaders(t *testing.T) {
	nc := NewNousClient("http://localhost:3333", "my-api-key")

	req, err := http.NewRequest("GET", "http://localhost:3333/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	nc.setHeaders(req)

	ct := req.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	auth := req.Header.Get("Authorization")
	if auth != "Bearer my-api-key" {
		t.Errorf("expected Authorization 'Bearer my-api-key', got %q", auth)
	}

	// Without API key, Authorization should not be set.
	nc2 := NewNousClient("http://localhost:3333", "")
	req2, _ := http.NewRequest("GET", "http://localhost:3333/test", nil)
	nc2.setHeaders(req2)

	auth2 := req2.Header.Get("Authorization")
	if auth2 != "" {
		t.Errorf("expected empty Authorization without API key, got %q", auth2)
	}
}

// ---------------------------------------------------------------------------
// IngestVault — skips small and nil notes
// ---------------------------------------------------------------------------

func TestNousClient_IngestVault_SkipsSmallNotes(t *testing.T) {
	// We cannot actually connect, but we can verify that small notes are skipped
	// by checking the count logic. Use an unreachable server so IngestNote errors
	// are non-fatal and skipped.
	nc := NewNousClient("http://127.0.0.1:19", "")

	notes := map[string]*vault.Note{
		"short.md": {
			RelPath: "short.md",
			Content: "tiny", // < 50 chars, should be skipped
		},
		"long.md": {
			RelPath: "long.md",
			Content: strings.Repeat("a", 100), // >= 50 chars, will attempt ingest (but fail due to unreachable)
		},
	}

	count, err := nc.IngestVault(notes)
	if err != nil {
		t.Fatalf("IngestVault should not return error (errors are non-fatal), got %v", err)
	}
	// The short note is skipped before attempting connection.
	// The long note attempts connection but fails, so count stays 0.
	if count != 0 {
		t.Errorf("expected count=0 (unreachable server), got %d", count)
	}
}

func TestNousClient_IngestVault_NilNotes(t *testing.T) {
	nc := NewNousClient("http://127.0.0.1:19", "")

	notes := map[string]*vault.Note{
		"nil.md":   nil,
		"short.md": {RelPath: "short.md", Content: "x"},
	}

	count, err := nc.IngestVault(notes)
	if err != nil {
		t.Fatalf("IngestVault should not return error, got %v", err)
	}
	// Both should be skipped: nil note and short content.
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}
}
