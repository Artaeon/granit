package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// reminder represents a scheduled reminder.
type reminder struct {
	Text    string `json:"text"`
	Time    string `json:"time"`              // "HH:MM"
	Repeat  string `json:"repeat"`            // "daily", "weekdays", "once"
	Enabled bool   `json:"enabled"`
	Created string `json:"created,omitempty"` // YYYY-MM-DD
}

// runRemind handles "granit remind" subcommands.
func runRemind(args []string) {
	if len(args) == 0 {
		printRemindUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		runRemindList()
	case "remove", "rm":
		if len(args) < 2 {
			exitError("Usage: granit remind remove <number>")
		}
		runRemindRemove(args[1])
	case "toggle":
		if len(args) < 2 {
			exitError("Usage: granit remind toggle <number>")
		}
		runRemindToggle(args[1])
	case "clear":
		runRemindClear()
	default:
		// Default: add a reminder
		runRemindAdd(args)
	}
}

func printRemindUsage() {
	fmt.Print(`Usage: granit remind <text> --at HH:MM [--daily|--weekdays|--once]

Subcommands:
  <text> --at HH:MM       Add a reminder
  list                     Show all reminders
  remove <number>          Remove a reminder by index
  toggle <number>          Enable/disable a reminder
  clear                    Remove all reminders

Options:
  --at HH:MM               Time to trigger (required for add)
  --daily                   Repeat every day (default)
  --weekdays                Repeat Mon-Fri only
  --once                    Fire once then auto-disable

Examples:
  granit remind "Start work" --at 07:00 --daily
  granit remind "Lunch break" --at 12:00 --weekdays
  granit remind "Team standup" --at 09:30 --weekdays
  granit remind "Review notes" --at 18:00
  granit remind list
  granit remind remove 2
`)
}

func runRemindAdd(args []string) {
	// Collect text (non-flag args)
	var textParts []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			if !strings.Contains(arg, "=") {
				i++ // skip value
			}
			continue
		}
		textParts = append(textParts, arg)
	}
	text := strings.TrimSpace(strings.Join(textParts, " "))
	if text == "" {
		exitError("Reminder text is required.\nUsage: granit remind \"Start work\" --at 07:00")
	}

	timeStr := getFlagValue("--at")
	if timeStr == "" {
		exitError("--at HH:MM is required.\nUsage: granit remind \"%s\" --at 07:00", text)
	}

	// Validate time format
	if _, err := time.Parse("15:04", timeStr); err != nil {
		exitError("Invalid time format: %q (use HH:MM, e.g. 07:00)", timeStr)
	}

	repeat := "daily"
	if hasFlag("--weekdays") {
		repeat = "weekdays"
	} else if hasFlag("--once") {
		repeat = "once"
	}

	r := reminder{
		Text:    text,
		Time:    timeStr,
		Repeat:  repeat,
		Enabled: true,
		Created: time.Now().Format("2006-01-02"),
	}

	vaultPath := resolveRemindVault()
	reminders := loadReminders(vaultPath)
	reminders = append(reminders, r)
	saveReminders(vaultPath, reminders)

	fmt.Printf("Reminder set: \"%s\" at %s (%s)\n", text, timeStr, repeat)
}

func runRemindList() {
	vaultPath := resolveRemindVault()
	reminders := loadReminders(vaultPath)

	if len(reminders) == 0 {
		fmt.Println("No reminders set. Add one with: granit remind \"text\" --at HH:MM")
		return
	}

	fmt.Println("\n  Reminders")
	fmt.Println(strings.Repeat("─", 50))
	for i, r := range reminders {
		status := "✓"
		if !r.Enabled {
			status = "✗"
		}
		fmt.Printf("  %s %d. %-25s %s  (%s)\n", status, i+1, r.Text, r.Time, r.Repeat)
	}
	fmt.Println()
}

func runRemindRemove(indexStr string) {
	idx, err := strconv.Atoi(indexStr)
	if err != nil {
		exitError("Invalid index: %s", indexStr)
	}

	vaultPath := resolveRemindVault()
	reminders := loadReminders(vaultPath)

	if idx < 1 || idx > len(reminders) {
		exitError("Index out of range: %d (have %d reminders)", idx, len(reminders))
	}

	removed := reminders[idx-1]
	reminders = append(reminders[:idx-1], reminders[idx:]...)
	saveReminders(vaultPath, reminders)

	fmt.Printf("Removed reminder: \"%s\" at %s\n", removed.Text, removed.Time)
}

func runRemindToggle(indexStr string) {
	idx, err := strconv.Atoi(indexStr)
	if err != nil {
		exitError("Invalid index: %s", indexStr)
	}

	vaultPath := resolveRemindVault()
	reminders := loadReminders(vaultPath)

	if idx < 1 || idx > len(reminders) {
		exitError("Index out of range: %d (have %d reminders)", idx, len(reminders))
	}

	reminders[idx-1].Enabled = !reminders[idx-1].Enabled
	saveReminders(vaultPath, reminders)

	state := "enabled"
	if !reminders[idx-1].Enabled {
		state = "disabled"
	}
	fmt.Printf("Reminder %d %s: \"%s\" at %s\n", idx, state, reminders[idx-1].Text, reminders[idx-1].Time)
}

func runRemindClear() {
	vaultPath := resolveRemindVault()
	saveReminders(vaultPath, []reminder{})
	fmt.Println("All reminders cleared.")
}

// ── Helpers ────────────────────────────────────────────────────────

func resolveRemindVault() string {
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

func remindersPath(vaultPath string) string {
	return filepath.Join(vaultPath, ".granit", "reminders.json")
}

func loadReminders(vaultPath string) []reminder {
	var reminders []reminder
	raw, err := os.ReadFile(remindersPath(vaultPath))
	if err != nil {
		return reminders
	}
	_ = json.Unmarshal(raw, &reminders)
	return reminders
}

func saveReminders(vaultPath string, reminders []reminder) {
	dir := filepath.Join(vaultPath, ".granit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		exitError("Error creating reminders dir: %v", err)
	}
	raw, err := json.MarshalIndent(reminders, "", "  ")
	if err != nil {
		exitError("Error saving reminders: %v", err)
	}
	// Atomic write so a crash mid-save cannot truncate reminders.json.
	path := remindersPath(vaultPath)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		_ = os.Remove(tmp)
		exitError("Error writing reminders: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		exitError("Error writing reminders: %v", err)
	}
}
