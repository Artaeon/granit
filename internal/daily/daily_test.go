package daily

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Folder != "" {
		t.Errorf("expected empty folder, got %q", cfg.Folder)
	}
	if cfg.Template == "" {
		t.Error("expected non-empty template")
	}
	if !strings.Contains(cfg.Template, "{{date}}") {
		t.Error("template should contain {{date}} placeholder")
	}
}

func TestGetDailyPath_NoFolder(t *testing.T) {
	cfg := DailyConfig{Folder: ""}
	path := GetDailyPath("/vault", cfg)
	today := time.Now().Format("2006-01-02")

	expected := filepath.Join("/vault", today+".md")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestGetDailyPath_WithFolder(t *testing.T) {
	cfg := DailyConfig{Folder: "Jots"}
	path := GetDailyPath("/vault", cfg)
	today := time.Now().Format("2006-01-02")

	expected := filepath.Join("/vault", "Jots", today+".md")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestEnsureDaily_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()

	path, created, err := EnsureDaily(dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for new daily note")
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	// Verify file contents
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content := string(data)
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(content, today) {
		t.Error("daily note should contain today's date")
	}
	if strings.Contains(content, "{{date}}") {
		t.Error("{{date}} placeholder should be replaced")
	}
}

func TestEnsureDaily_ExistingNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()

	// Create first
	path, _, err := EnsureDaily(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Write custom content
	_ = os.WriteFile(path, []byte("custom content"), 0644)

	// Ensure again — should NOT overwrite
	_, created, err := EnsureDaily(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("should not create when file already exists")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "custom content" {
		t.Error("existing file should not be overwritten")
	}
}

func TestEnsureDaily_WithSubfolder(t *testing.T) {
	dir := t.TempDir()
	cfg := DailyConfig{
		Folder:   "Daily",
		Template: "# {{date}}\n",
	}

	path, created, err := EnsureDaily(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("expected new file")
	}
	if !strings.Contains(path, "Daily") {
		t.Errorf("expected path to contain 'Daily', got %q", path)
	}

	// Verify subfolder was created
	if _, err := os.Stat(filepath.Join(dir, "Daily")); os.IsNotExist(err) {
		t.Error("subfolder should be created")
	}
}

func TestReplaceAll(t *testing.T) {
	result := replaceAll("hello {{name}}, welcome {{name}}", "{{name}}", "World")
	if result != "hello World, welcome World" {
		t.Errorf("expected replacements, got %q", result)
	}
}

func TestReplaceAll_NoMatch(t *testing.T) {
	result := replaceAll("hello world", "{{none}}", "X")
	if result != "hello world" {
		t.Errorf("expected unchanged, got %q", result)
	}
}
