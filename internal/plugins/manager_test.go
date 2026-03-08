package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// helper: create a minimal valid plugin in dir/<name>/ with the given manifest.
func createTestPlugin(t *testing.T, dir, name string, manifest pluginManifest) string {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins", name)
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	// Write a dummy script so the plugin directory has content
	script := "#!/bin/bash\necho 'hello'\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "main.sh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return pluginDir
}

func validManifest(name string) pluginManifest {
	return pluginManifest{
		Name:        name,
		Description: "A test plugin",
		Version:     "1.0.0",
		Author:      "Test",
		Enabled:     true,
		Commands: []pluginCmdDef{
			{Label: "Run Test", Description: "runs test", Run: "main.sh"},
		},
	}
}

// ---------------------------------------------------------------------------
// ListPlugins
// ---------------------------------------------------------------------------

func TestListPlugins_Empty(t *testing.T) {
	dir := t.TempDir()

	// No plugins directory at all
	list, err := ListPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(list))
	}
}

func TestListPlugins_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0700); err != nil {
		t.Fatal(err)
	}

	list, err := ListPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 plugins, got %d", len(list))
	}
}

func TestListPlugins_WithPlugins(t *testing.T) {
	dir := t.TempDir()
	createTestPlugin(t, dir, "alpha", validManifest("alpha"))
	createTestPlugin(t, dir, "beta", validManifest("beta"))

	list, err := ListPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(list))
	}

	names := make(map[string]bool)
	for _, p := range list {
		names[p.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Fatalf("expected alpha and beta, got %v", names)
	}
}

func TestListPlugins_SkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	createTestPlugin(t, dir, "good", validManifest("good"))

	// Create invalid plugin (bad JSON)
	badDir := filepath.Join(dir, "plugins", "bad")
	if err := os.MkdirAll(badDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "plugin.json"), []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}

	list, err := ListPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 plugin (skipping invalid), got %d", len(list))
	}
	if list[0].Name != "good" {
		t.Fatalf("expected 'good', got %q", list[0].Name)
	}
}

// ---------------------------------------------------------------------------
// InstallPlugin
// ---------------------------------------------------------------------------

func TestInstallPlugin_Valid(t *testing.T) {
	configDir := t.TempDir()
	sourceDir := t.TempDir()

	// Create a valid plugin source
	m := validManifest("test-plugin")
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(sourceDir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "main.sh"), []byte("#!/bin/bash\necho hi\n"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := InstallPlugin(sourceDir, configDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify installed
	installed := filepath.Join(configDir, "plugins", "test-plugin", "plugin.json")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("plugin.json not found at expected location: %v", err)
	}

	// Verify script was copied
	scriptPath := filepath.Join(configDir, "plugins", "test-plugin", "main.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("main.sh not found at expected location: %v", err)
	}
}

func TestInstallPlugin_InvalidSource(t *testing.T) {
	configDir := t.TempDir()
	sourceDir := t.TempDir()

	// No plugin.json
	err := InstallPlugin(sourceDir, configDir)
	if err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
}

func TestInstallPlugin_Duplicate(t *testing.T) {
	configDir := t.TempDir()
	sourceDir := t.TempDir()

	m := validManifest("dup-plugin")
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(sourceDir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	// Install once
	if err := InstallPlugin(sourceDir, configDir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	// Install again — should fail
	err := InstallPlugin(sourceDir, configDir)
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
}

// ---------------------------------------------------------------------------
// RemovePlugin
// ---------------------------------------------------------------------------

func TestRemovePlugin_Exists(t *testing.T) {
	dir := t.TempDir()
	createTestPlugin(t, dir, "removeme", validManifest("removeme"))

	if err := RemovePlugin("removeme", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify removed
	removedDir := filepath.Join(dir, "plugins", "removeme")
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Fatal("plugin directory still exists after removal")
	}
}

func TestRemovePlugin_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0700); err != nil {
		t.Fatal(err)
	}

	err := RemovePlugin("nonexistent", dir)
	if err == nil {
		t.Fatal("expected error for non-existent plugin, got nil")
	}
}

// ---------------------------------------------------------------------------
// EnablePlugin / DisablePlugin
// ---------------------------------------------------------------------------

func TestEnablePlugin(t *testing.T) {
	dir := t.TempDir()
	m := validManifest("toggle-test")
	m.Enabled = false
	createTestPlugin(t, dir, "toggle-test", m)

	if err := EnablePlugin("toggle-test", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify enabled
	info, err := ValidatePlugin(filepath.Join(dir, "plugins", "toggle-test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Enabled {
		t.Fatal("expected plugin to be enabled")
	}
}

func TestDisablePlugin(t *testing.T) {
	dir := t.TempDir()
	m := validManifest("toggle-test2")
	m.Enabled = true
	createTestPlugin(t, dir, "toggle-test2", m)

	if err := DisablePlugin("toggle-test2", dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := ValidatePlugin(filepath.Join(dir, "plugins", "toggle-test2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Enabled {
		t.Fatal("expected plugin to be disabled")
	}
}

func TestEnablePlugin_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0700); err != nil {
		t.Fatal(err)
	}

	err := EnablePlugin("ghost", dir)
	if err == nil {
		t.Fatal("expected error for non-existent plugin, got nil")
	}
}

func TestDisablePlugin_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0700); err != nil {
		t.Fatal(err)
	}

	err := DisablePlugin("ghost", dir)
	if err == nil {
		t.Fatal("expected error for non-existent plugin, got nil")
	}
}

// ---------------------------------------------------------------------------
// ValidatePlugin
// ---------------------------------------------------------------------------

func TestValidatePlugin_Valid(t *testing.T) {
	dir := t.TempDir()
	m := pluginManifest{
		Name:        "valid-test",
		Description: "A valid plugin",
		Version:     "2.0.0",
		Author:      "Tester",
		Enabled:     true,
		Commands: []pluginCmdDef{
			{Label: "Do Thing", Description: "does thing", Run: "main.sh"},
		},
		Hooks: pluginHooks{
			OnSave:   "save.sh",
			OnCreate: "create.sh",
		},
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	info, err := ValidatePlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "valid-test" {
		t.Fatalf("expected name 'valid-test', got %q", info.Name)
	}
	if info.Version != "2.0.0" {
		t.Fatalf("expected version '2.0.0', got %q", info.Version)
	}
	if info.Author != "Tester" {
		t.Fatalf("expected author 'Tester', got %q", info.Author)
	}
	if !info.Enabled {
		t.Fatal("expected enabled=true")
	}
	if len(info.Commands) != 1 || info.Commands[0] != "Do Thing" {
		t.Fatalf("unexpected commands: %v", info.Commands)
	}
	if len(info.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d: %v", len(info.Hooks), info.Hooks)
	}
}

func TestValidatePlugin_MissingName(t *testing.T) {
	dir := t.TempDir()
	m := pluginManifest{
		Version: "1.0.0",
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ValidatePlugin(dir)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestValidatePlugin_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	m := pluginManifest{
		Name: "no-version",
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ValidatePlugin(dir)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestValidatePlugin_BadJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte("{bad json!}"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ValidatePlugin(dir)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
}

func TestValidatePlugin_NoManifest(t *testing.T) {
	dir := t.TempDir()

	_, err := ValidatePlugin(dir)
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
}

// ---------------------------------------------------------------------------
// ScaffoldPlugin
// ---------------------------------------------------------------------------

func TestScaffoldPlugin(t *testing.T) {
	dir := t.TempDir()

	pluginDir, err := ScaffoldPlugin("my-test-plugin", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(pluginDir); err != nil {
		t.Fatalf("plugin directory not created: %v", err)
	}

	// Verify plugin.json exists and is valid
	info, err := ValidatePlugin(pluginDir)
	if err != nil {
		t.Fatalf("scaffold produced invalid plugin: %v", err)
	}
	if info.Name != "my-test-plugin" {
		t.Fatalf("expected name 'my-test-plugin', got %q", info.Name)
	}
	if info.Version != "0.1.0" {
		t.Fatalf("expected version '0.1.0', got %q", info.Version)
	}
	if len(info.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(info.Commands))
	}

	// Verify main.sh exists and is executable
	scriptPath := filepath.Join(pluginDir, "main.sh")
	scriptInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("main.sh not found: %v", err)
	}
	if scriptInfo.Mode()&0100 == 0 {
		t.Fatal("main.sh is not executable")
	}

	// Verify README.md exists
	readmePath := filepath.Join(pluginDir, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("README.md not found: %v", err)
	}
}

func TestScaffoldPlugin_AlreadyExists(t *testing.T) {
	dir := t.TempDir()

	if _, err := ScaffoldPlugin("existing", dir); err != nil {
		t.Fatalf("first scaffold failed: %v", err)
	}

	_, err := ScaffoldPlugin("existing", dir)
	if err == nil {
		t.Fatal("expected error for existing directory, got nil")
	}
}

func TestScaffoldPlugin_TitleCase(t *testing.T) {
	// Verify the titleCase helper used in scaffold
	tests := []struct {
		input, expected string
	}{
		{"word-count", "Word Count"},
		{"my-cool-plugin", "My Cool Plugin"},
		{"single", "Single"},
		{"a-b", "A B"},
	}
	for _, tc := range tests {
		got := titleCase(tc.input)
		if got != tc.expected {
			t.Errorf("titleCase(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
