package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// runTodo handles "granit todo <text>" — adds a task to Tasks.md with optional metadata.
func runTodo(args []string) {
	vaultPath := resolveTodoVault(args)
	targetFile := "Tasks.md"
	if f := getFlagValue("--file"); f != "" {
		targetFile = f
	} else if f := getFlagValue("-f"); f != "" {
		targetFile = f
	}
	targetPath := filepath.Join(vaultPath, targetFile)

	// Collect task text from positional args
	text := collectTodoText(args)
	if text == "" {
		fmt.Println(`Usage: granit todo "Buy groceries" [--due tomorrow] [--priority high] [--tag shopping]

Options:
  --due <date>        Due date: "today", "tomorrow", "monday", or YYYY-MM-DD
  --priority <level>  Priority: highest, high, medium, low
  --tag <name>        Add a tag (repeatable)
  --file/-f <name>    Target file (default: Tasks.md)
  --vault/-v <path>   Vault path

Examples:
  granit todo "Buy milk" --due tomorrow --priority high
  granit todo "Review PR" --tag work --due 2026-03-15
  granit todo "Quick idea"`)
		os.Exit(1)
	}

	// Build the task line
	taskLine := buildTaskLine(text)

	// Ensure the file exists
	ensureTodoFile(targetPath)

	// Append the task
	existing, err := os.ReadFile(targetPath)
	if err != nil {
		exitError("Error reading %s: %v", targetFile, err)
	}

	newContent := string(existing)
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += taskLine + "\n"

	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
		_ = os.Remove(tmpPath)
		exitError("Error writing file: %v", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		exitError("Error saving file: %v", err)
	}

	rel, _ := filepath.Rel(vaultPath, targetPath)
	if rel == "" {
		rel = targetFile
	}
	fmt.Printf("Added to %s: %s\n", rel, taskLine)
}

// resolveTodoVault determines the vault path from flags or defaults.
func resolveTodoVault(args []string) string {
	if v := getFlagValue("--vault"); v != "" {
		return v
	}
	if v := getFlagValue("-v"); v != "" {
		return v
	}
	if envVault := os.Getenv("GRANIT_VAULT"); envVault != "" {
		return envVault
	}
	if last := config.LoadVaultList().LastUsed; last != "" {
		return last
	}
	return "."
}

// collectTodoText extracts the task text from positional arguments, skipping flags.
func collectTodoText(args []string) string {
	var parts []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			// Skip flag and its value
			if !strings.Contains(arg, "=") {
				i++ // skip value
			}
			continue
		}
		parts = append(parts, arg)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// buildTaskLine creates a markdown task line with emoji metadata markers.
func buildTaskLine(text string) string {
	line := "- [ ] " + text

	// Add priority emoji
	if p := getFlagValue("--priority"); p != "" {
		switch strings.ToLower(p) {
		case "highest":
			line += " 🔺"
		case "high":
			line += " ⏫"
		case "medium":
			line += " 🔼"
		case "low":
			line += " 🔽"
		}
	}

	// Add due date
	if d := getFlagValue("--due"); d != "" {
		date := parseDueDate(d)
		if date != "" {
			line += " 📅 " + date
		}
	}

	// Add tags
	for i, arg := range os.Args {
		if arg == "--tag" && i+1 < len(os.Args) {
			tag := os.Args[i+1]
			if !strings.HasPrefix(tag, "#") {
				tag = "#" + tag
			}
			line += " " + tag
		}
	}

	return line
}

// parseDueDate converts human-readable dates to YYYY-MM-DD format.
func parseDueDate(input string) string {
	today := time.Now()
	switch strings.ToLower(input) {
	case "today":
		return today.Format("2006-01-02")
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format("2006-01-02")
	case "yesterday":
		return today.AddDate(0, 0, -1).Format("2006-01-02")
	case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		return nextWeekday(input).Format("2006-01-02")
	default:
		// Try parsing as YYYY-MM-DD
		if _, err := time.Parse("2006-01-02", input); err == nil {
			return input
		}
		// Try parsing as MM-DD
		if t, err := time.Parse("01-02", input); err == nil {
			result := time.Date(today.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())
			if result.Before(today) {
				result = result.AddDate(1, 0, 0)
			}
			return result.Format("2006-01-02")
		}
		fmt.Fprintf(os.Stderr, "Warning: could not parse date %q, skipping\n", input)
		return ""
	}
}

// nextWeekday returns the next occurrence of the given weekday.
func nextWeekday(name string) time.Time {
	target := map[string]time.Weekday{
		"monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday,
		"sunday": time.Sunday,
	}
	day, ok := target[strings.ToLower(name)]
	if !ok {
		return time.Now()
	}
	now := time.Now()
	daysUntil := int(day-now.Weekday()+7) % 7
	if daysUntil == 0 {
		daysUntil = 7
	}
	return now.AddDate(0, 0, daysUntil)
}

// ensureTodoFile creates the Tasks.md file with proper header if it doesn't exist.
func ensureTodoFile(targetPath string) {
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		return
	}
	today := time.Now().Format("2006-01-02")
	baseName := strings.TrimSuffix(filepath.Base(targetPath), ".md")
	header := fmt.Sprintf("---\ntitle: %s\ndate: %s\ntype: tasks\ntags: [tasks]\n---\n\n# %s\n", baseName, today, baseName)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		exitError("Error creating directory: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(header), 0644); err != nil {
		exitError("Error creating %s: %v", filepath.Base(targetPath), err)
	}
}
