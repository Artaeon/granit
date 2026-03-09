package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// resolveTargetFile
// ---------------------------------------------------------------------------

func TestResolveTargetFile_Default(t *testing.T) {
	// Save and restore os.Args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "hello"}
	got := resolveTargetFile()
	if got != "inbox.md" {
		t.Errorf("expected default 'inbox.md', got %q", got)
	}
}

func TestResolveTargetFile_WithFileFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "--file", "tasks.md", "hello"}
	got := resolveTargetFile()
	if got != "tasks.md" {
		t.Errorf("expected 'tasks.md', got %q", got)
	}
}

func TestResolveTargetFile_WithShortFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "-f", "notes.md", "hello"}
	got := resolveTargetFile()
	if got != "notes.md" {
		t.Errorf("expected 'notes.md', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ensureTargetFile
// ---------------------------------------------------------------------------

func TestEnsureTargetFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "inbox.md")

	ensureTargetFile(target)

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "---") {
		t.Error("expected frontmatter delimiters in new file")
	}
	if !strings.Contains(s, "title: inbox") {
		t.Error("expected title derived from filename")
	}
	if !strings.Contains(s, "type: inbox") {
		t.Error("expected type: inbox in frontmatter")
	}
}

func TestEnsureTargetFile_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.md")
	original := "# My existing note\n"
	os.WriteFile(target, []byte(original), 0644)

	ensureTargetFile(target)

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != original {
		t.Errorf("ensureTargetFile should not overwrite existing file")
	}
}

func TestEnsureTargetFile_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "deep", "note.md")

	ensureTargetFile(target)

	if _, err := os.Stat(target); os.IsNotExist(err) {
		t.Error("expected file to be created with parent directories")
	}
}

// ---------------------------------------------------------------------------
// appendCapture
// ---------------------------------------------------------------------------

func TestAppendCapture_AppendsTimestampedEntry(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "inbox.md")

	appendCapture(dir, target, "Buy milk")

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "Buy milk") {
		t.Error("expected captured text in file")
	}
	if !strings.Contains(s, "**") {
		t.Error("expected bold timestamp markers in entry")
	}
}

func TestAppendCapture_AppendsToExistingContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "inbox.md")
	os.WriteFile(target, []byte("# Inbox\n"), 0644)

	appendCapture(dir, target, "First item")
	appendCapture(dir, target, "Second item")

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "# Inbox") {
		t.Error("expected original content preserved")
	}
	if !strings.Contains(s, "First item") {
		t.Error("expected first captured item")
	}
	if !strings.Contains(s, "Second item") {
		t.Error("expected second captured item")
	}
}

// ---------------------------------------------------------------------------
// getCapturePositionalArgs
// ---------------------------------------------------------------------------

func TestGetCapturePositionalArgs_SimpleText(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "hello", "world"}
	args := getCapturePositionalArgs()

	if len(args) != 2 || args[0] != "hello" || args[1] != "world" {
		t.Errorf("expected [hello world], got %v", args)
	}
}

func TestGetCapturePositionalArgs_SkipsFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "--file", "tasks.md", "Buy", "milk"}
	args := getCapturePositionalArgs()

	if len(args) != 2 || args[0] != "Buy" || args[1] != "milk" {
		t.Errorf("expected [Buy milk], got %v", args)
	}
}

func TestGetCapturePositionalArgs_SkipsEqualsFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "--file=tasks.md", "hello"}
	args := getCapturePositionalArgs()

	if len(args) != 1 || args[0] != "hello" {
		t.Errorf("expected [hello], got %v", args)
	}
}

func TestGetCapturePositionalArgs_NoArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture"}
	args := getCapturePositionalArgs()

	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestGetCapturePositionalArgs_SkipsShortFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"granit", "capture", "-f", "tasks.md", "-v", "/vault", "text"}
	args := getCapturePositionalArgs()

	if len(args) != 1 || args[0] != "text" {
		t.Errorf("expected [text], got %v", args)
	}
}

// ---------------------------------------------------------------------------
// resolveCaptureVault
// ---------------------------------------------------------------------------

func TestResolveCaptureVault_FallbackToDot(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Clear env var
	origEnv := os.Getenv("GRANIT_VAULT")
	os.Unsetenv("GRANIT_VAULT")
	defer func() {
		if origEnv != "" {
			os.Setenv("GRANIT_VAULT", origEnv)
		}
	}()

	os.Args = []string{"granit", "capture", "text"}

	got := resolveCaptureVault()
	// Without any flags, env, or last-used vault, it should fall back to
	// either "." or the last used vault. We just check it doesn't panic
	// and returns a non-empty string.
	if got == "" {
		t.Error("expected non-empty vault path")
	}
}

func TestResolveCaptureVault_FromEnv(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	origEnv := os.Getenv("GRANIT_VAULT")
	os.Setenv("GRANIT_VAULT", "/tmp/myvault")
	defer func() {
		if origEnv != "" {
			os.Setenv("GRANIT_VAULT", origEnv)
		} else {
			os.Unsetenv("GRANIT_VAULT")
		}
	}()

	os.Args = []string{"granit", "capture", "text"}

	got := resolveCaptureVault()
	if got != "/tmp/myvault" {
		t.Errorf("expected '/tmp/myvault' from env, got %q", got)
	}
}

func TestResolveCaptureVault_FlagOverridesEnv(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	origEnv := os.Getenv("GRANIT_VAULT")
	os.Setenv("GRANIT_VAULT", "/tmp/envvault")
	defer func() {
		if origEnv != "" {
			os.Setenv("GRANIT_VAULT", origEnv)
		} else {
			os.Unsetenv("GRANIT_VAULT")
		}
	}()

	os.Args = []string{"granit", "capture", "--vault", "/tmp/flagvault", "text"}

	got := resolveCaptureVault()
	if got != "/tmp/flagvault" {
		t.Errorf("expected '/tmp/flagvault' from flag, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// isTerminal
// ---------------------------------------------------------------------------

func TestIsTerminal_InTestEnvironment(t *testing.T) {
	// In test environment, stdin is typically not a terminal (it's a pipe).
	// We just verify the function doesn't panic.
	_ = isTerminal()
}
