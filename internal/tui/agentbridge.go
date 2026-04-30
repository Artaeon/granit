package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/vault"
)

// agentBridge implements internal/agents.VaultReader and VaultWriter
// against granit's existing vault + object index + task list. Lives
// in the tui package because the agents package can't import vault
// (the import direction goes one way: tui → agents).
//
// Constructed per agent run by the model so it always sees the
// freshest snapshot — agents are not long-lived background workers,
// they execute one goal at a time on demand.
type agentBridge struct {
	vault    *vault.Vault
	registry *objects.Registry
	index    *objects.Index
	tasks    func() []agents.TaskRecord
	writer   func(rel, content string) (string, error)
	appender func(line string) (string, error)
}

func (b *agentBridge) VaultRoot() string { return b.vault.Root }

func (b *agentBridge) NoteContent(rel string) (string, bool) {
	n := b.vault.GetNote(rel)
	if n == nil {
		return "", false
	}
	return n.Content, true
}

// SearchVault runs a substring scan across every note's content
// (case-insensitive). For a vault with thousands of notes this
// would be slow; for typical PKM scale (hundreds to a few thousand)
// it's fine and avoids dragging the search-index gob into the
// agents path.
func (b *agentBridge) SearchVault(query string, limit int) []agents.SearchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var hits []agents.SearchHit
	for _, n := range b.vault.Notes {
		body := n.Content
		idx := strings.Index(strings.ToLower(body), q)
		if idx < 0 {
			continue
		}
		// Snippet: 60 chars before and after the first match,
		// with newlines collapsed to spaces so the LLM sees
		// readable context.
		start := idx - 60
		if start < 0 {
			start = 0
		}
		end := idx + len(q) + 60
		if end > len(body) {
			end = len(body)
		}
		snippet := strings.ReplaceAll(body[start:end], "\n", " ")
		snippet = strings.TrimSpace(snippet)
		hits = append(hits, agents.SearchHit{Path: n.RelPath, Snippet: snippet})
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

func (b *agentBridge) ObjectIndex() *objects.Index       { return b.index }
func (b *agentBridge) ObjectRegistry() *objects.Registry { return b.registry }
func (b *agentBridge) TaskList() []agents.TaskRecord {
	if b.tasks == nil {
		return nil
	}
	return b.tasks()
}

func (b *agentBridge) WriteNote(rel, content string) (string, error) {
	if b.writer == nil {
		return "", errContextWriterNil
	}
	return b.writer(rel, content)
}

func (b *agentBridge) AppendTaskLine(line string) (string, error) {
	if b.appender == nil {
		return "", errContextAppenderNil
	}
	return b.appender(line)
}

// agentLLM bridges granit's existing AIConfig (Ollama / OpenAI /
// Nous / Nerve) into the agent runtime's LLM interface. Each
// Complete call goes through the same provider switch the bots use.
//
// We deliberately don't share state with bots.go — agents have
// their own system prompts and shouldn't accidentally inherit a
// bot's persona.
type agentLLM struct {
	cfg AIConfig
}

func (l *agentLLM) Complete(ctx context.Context, prompt string) (string, error) {
	// ChatCtx already supports a system prompt + user prompt split,
	// but our agent prompt builder bakes both into one string for
	// simplicity. Pass an empty system prompt and the assembled
	// agent prompt as user input — the providers all handle this
	// shape uniformly.
	resp, err := l.cfg.ChatCtx(ctx, "", prompt)
	if err != nil {
		return "", translateProviderError(l.cfg, err)
	}
	return resp, nil
}

// translateProviderError converts low-level network / provider
// errors into actionable user-facing messages. Without this, the
// agent transcript shows raw Go errors like "Post
// http://127.0.0.1:11434/api/chat: dial tcp 127.0.0.1:11434:
// connect: connection refused" — accurate but unhelpful for a user
// who just wants to know "is Ollama running?".
//
// Common cases:
//
//   - Connection refused → likely Ollama isn't running. The hint
//     directs them to start the daemon.
//   - 404 with model name → model not pulled yet. Hint with the
//     `ollama pull <model>` command for the configured model.
//   - 401/403 on cloud providers → API key invalid. Hint at
//     Settings (Ctrl+,) → AI section.
//
// On unrecognised errors the original message passes through so
// edge cases aren't swallowed.
func translateProviderError(cfg AIConfig, err error) error {
	msg := err.Error()
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch {
	case strings.Contains(msg, "connection refused"):
		switch provider {
		case "ollama", "":
			return fmt.Errorf("Ollama isn't running. Start it with `ollama serve` (or restart the system service), then retry. Original: %v", err)
		case "nous":
			return fmt.Errorf("Nous server not reachable at %s. Start the local AI server, then retry. Original: %v", cfg.NousURL, err)
		}
		return fmt.Errorf("AI provider %q not reachable. Original: %v", cfg.Provider, err)
	case strings.Contains(msg, "model") && strings.Contains(msg, "not found"):
		// Ollama returns "model 'X' not found, try pulling it first".
		return fmt.Errorf("Model %q is not pulled. Run `ollama pull %s` and retry. Original: %v",
			cfg.Model, cfg.Model, err)
	case strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") ||
		strings.Contains(msg, "403") || strings.Contains(msg, "invalid_api_key"):
		return fmt.Errorf("API key for %q is invalid or missing. Open Settings (Ctrl+,) → AI to fix. Original: %v",
			cfg.Provider, err)
	case strings.Contains(msg, "context deadline exceeded"):
		return fmt.Errorf("AI request timed out. The model may be too large for your hardware, or the agent is making complex calls. Try a smaller model or a simpler goal. Original: %v", err)
	}
	return err
}

// agentTaskBridge converts granit's []Task to []agents.TaskRecord.
// Lives here (not in agents) because tasks.Task carries TUI-specific
// fields the agents package doesn't need to know about.
func agentTaskBridge(tasks []Task) []agents.TaskRecord {
	out := make([]agents.TaskRecord, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, agents.TaskRecord{
			Text:     tmCleanText(t.Text),
			NotePath: t.NotePath,
			Done:     t.Done,
			DueDate:  t.DueDate,
			Priority: t.Priority,
			Tags:     append([]string(nil), t.Tags...),
		})
	}
	return out
}

// errContextWriterNil / errContextAppenderNil are sentinel errors
// for the (nil-writer-but-write-tool-registered) path. In normal
// use the bridge always has its writers wired; these guard against
// programmer error in test/agent factory code.
type bridgeErr string

func (e bridgeErr) Error() string { return string(e) }

const (
	errContextWriterNil   bridgeErr = "agent bridge: WriteNote called but writer is nil"
	errContextAppenderNil bridgeErr = "agent bridge: AppendTaskLine called but appender is nil"
)
