package daily

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type DailyConfig struct {
	Folder   string // subfolder for daily notes, e.g. "daily/"
	Template string // template for new daily notes
}

func DefaultConfig() DailyConfig {
	return DailyConfig{
		Folder:   "",
		Template: defaultTemplate(),
	}
}

func defaultTemplate() string {
	return `---
date: {{date}}
type: daily
---

# {{date}}

## Tasks
- [ ]

## Notes

`
}

func GetDailyPath(vaultRoot string, cfg DailyConfig) string {
	today := time.Now().Format("2006-01-02")
	filename := today + ".md"
	if cfg.Folder != "" {
		return filepath.Join(vaultRoot, cfg.Folder, filename)
	}
	return filepath.Join(vaultRoot, filename)
}

func EnsureDaily(vaultRoot string, cfg DailyConfig) (string, bool, error) {
	path := GetDailyPath(vaultRoot, cfg)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create directory: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		return path, false, nil // already exists
	}

	today := time.Now().Format("2006-01-02")
	content := cfg.Template
	content = replaceAll(content, "{{date}}", today)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", false, fmt.Errorf("failed to create daily note: %w", err)
	}

	return path, true, nil
}

func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}
