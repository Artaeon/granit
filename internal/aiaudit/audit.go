// Package aiaudit records every outbound AI request to an append-
// only log so the user can inspect what data left their device.
// Stored at <vault>/.granit/ai-audit.jsonl — JSONL so a corrupted
// last line doesn't poison the rest of the file. One line per
// request.
//
// The log records:
//   - Timestamp (when the request fired)
//   - Feature ID (which granit surface initiated it)
//   - Provider (ollama / openai / anthropic / "")
//   - Model (gpt-4o-mini / claude-haiku-4-5 / etc.)
//   - Prompt size in bytes (NOT the prompt itself — privacy)
//   - Prompt hash (SHA-256 first-12-hex; lets the user spot
//     "this same prompt fired 6 times today")
//   - Redactions applied (count per rule type)
//   - Response size in bytes (or 0 on failure)
//   - Error string (if any)
//
// Crucially, the prompt and response BODIES are not stored. A
// good-faith user might want to inspect those — they can re-run
// the feature with debug logging, or look at the source markdown
// the prompt was assembled from. The audit log is for "what
// metadata is moving around", not "show me what I asked the AI."
package aiaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one logged request.
type Entry struct {
	Timestamp        time.Time `json:"timestamp"`
	Feature          string    `json:"feature"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model,omitempty"`
	PromptSizeBytes  int       `json:"prompt_size_bytes"`
	PromptHash       string    `json:"prompt_hash,omitempty"`
	RedactionsByRule []Stat    `json:"redactions,omitempty"`
	ResponseSizeBytes int      `json:"response_size_bytes,omitempty"`
	Error            string    `json:"error,omitempty"`
}

// Stat mirrors airedact.Stat but kept here as a separate type so
// this package doesn't import the redact package (circular dep
// risk if redact ever wants to log something). Same JSON shape.
type Stat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Logger appends entries to the log file under a mutex so two
// concurrent requests don't interleave a half-written line.
type Logger struct {
	vaultRoot string
	mu        sync.Mutex
}

func New(vaultRoot string) *Logger {
	return &Logger{vaultRoot: vaultRoot}
}

func (l *Logger) path() string {
	return filepath.Join(l.vaultRoot, ".granit", "ai-audit.jsonl")
}

// Append writes one entry as a JSON line. Sets Timestamp + hashes
// the prompt (without storing it) before writing. Returns the
// entry as written (with the populated fields) so the caller can
// surface the same shape to the UI.
func (l *Logger) Append(e Entry, promptText string) (Entry, error) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if promptText != "" {
		e.PromptSizeBytes = len(promptText)
		sum := sha256.Sum256([]byte(promptText))
		e.PromptHash = hex.EncodeToString(sum[:])[:12]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	dir := filepath.Dir(l.path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return e, err
	}
	f, err := os.OpenFile(l.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return e, err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return e, err
	}
	return e, nil
}

// List returns the most recent N entries, newest first. Reads the
// whole file each time — fine for a personal-scale log. Cap N to
// keep the wire payload bounded.
func (l *Logger) List(limit int) ([]Entry, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := os.ReadFile(l.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}
	var entries []Entry
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var e Entry
		if err := dec.Decode(&e); err != nil {
			// Skip bad lines — JSONL is forgiving by design.
			break
		}
		entries = append(entries, e)
	}
	// Reverse for newest-first.
	out := make([]Entry, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		out = append(out, entries[i])
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Clear removes the audit log file entirely. Wired to a settings
// "Clear AI history" button — the equivalent of GDPR right-to-
// erasure for the on-device portion.
func (l *Logger) Clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	err := os.Remove(l.path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

