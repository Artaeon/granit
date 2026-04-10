package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
)

// clockSession represents a single clock-in/out session.
type clockSession struct {
	Start   string `json:"start"`             // RFC3339
	End     string `json:"end,omitempty"`      // RFC3339 (empty if active)
	Project string `json:"project,omitempty"`
}

// clockData is the persistent storage for clock sessions.
type clockData struct {
	Active   *clockSession  `json:"active,omitempty"`
	Sessions []clockSession `json:"sessions"`
}

// runClock handles "granit clock <subcommand>".
func runClock(args []string) {
	if len(args) == 0 {
		printClockUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "in":
		runClockIn(args[1:])
	case "out":
		runClockOut(args[1:])
	case "status":
		runClockStatus()
	case "log":
		runClockLog(args[1:])
	default:
		fmt.Printf("Unknown clock subcommand: %s\n", args[0])
		printClockUsage()
		os.Exit(1)
	}
}

func printClockUsage() {
	fmt.Print(`Usage: granit clock <subcommand>

Subcommands:
  in [--project "name"]    Clock in (start a work session)
  out                      Clock out (end the active session)
  status                   Show current session status
  log [--week]             Show today's time log (or --week for weekly)

Examples:
  granit clock in --project "Go study"
  granit clock in
  granit clock status
  granit clock out
  granit clock log
  granit clock log --week
`)
}

func runClockIn(args []string) {
	vaultPath := resolveClockVault()
	data := loadClockData(vaultPath)

	if data.Active != nil {
		start, _ := time.Parse(time.RFC3339, data.Active.Start)
		elapsed := time.Since(start).Truncate(time.Second)
		project := data.Active.Project
		if project == "" {
			project = "(no project)"
		}
		fmt.Printf("Already clocked in: %s — %s (%s)\n", project, elapsed, start.Format("15:04"))
		fmt.Println("Clock out first with: granit clock out")
		return
	}

	project := ""
	for i, arg := range args {
		if (arg == "--project" || arg == "-p") && i+1 < len(args) {
			project = args[i+1]
		}
	}

	session := &clockSession{
		Start:   time.Now().Format(time.RFC3339),
		Project: project,
	}
	data.Active = session
	saveClockData(vaultPath, data)

	label := "work"
	if project != "" {
		label = project
	}
	fmt.Printf("Clocked in: %s at %s\n", label, time.Now().Format("15:04"))
}

func runClockOut(args []string) {
	vaultPath := resolveClockVault()
	data := loadClockData(vaultPath)

	if data.Active == nil {
		fmt.Println("Not clocked in. Start with: granit clock in")
		return
	}

	// Complete the session
	data.Active.End = time.Now().Format(time.RFC3339)
	start, _ := time.Parse(time.RFC3339, data.Active.Start)
	elapsed := time.Since(start).Truncate(time.Second)

	// Move to history
	data.Sessions = append(data.Sessions, *data.Active)
	label := data.Active.Project
	if label == "" {
		label = "work"
	}
	data.Active = nil
	saveClockData(vaultPath, data)

	// Also save to vault timetracking note
	if err := saveSessionToVault(vaultPath, start, time.Now(), label, elapsed); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update timelog: %v\n", err)
	}

	fmt.Printf("Clocked out: %s — %s\n", label, formatDuration(elapsed))
}

func runClockStatus() {
	vaultPath := resolveClockVault()
	data := loadClockData(vaultPath)

	if data.Active == nil {
		fmt.Println("Not clocked in.")

		// Show today's total
		todayTotal := todayTotalTime(data)
		if todayTotal > 0 {
			fmt.Printf("Today's total: %s\n", formatDuration(todayTotal))
		}
		return
	}

	start, _ := time.Parse(time.RFC3339, data.Active.Start)
	elapsed := time.Since(start).Truncate(time.Second)
	project := data.Active.Project
	if project == "" {
		project = "(no project)"
	}

	fmt.Printf("  Clocked in since %s\n", start.Format("15:04"))
	fmt.Printf("  Project:  %s\n", project)
	fmt.Printf("  Elapsed:  %s\n", formatDuration(elapsed))

	todayTotal := todayTotalTime(data) + elapsed
	fmt.Printf("  Today:    %s\n", formatDuration(todayTotal))
}

func runClockLog(args []string) {
	vaultPath := resolveClockVault()
	data := loadClockData(vaultPath)

	weekly := false
	for _, arg := range args {
		if arg == "--week" || arg == "-w" {
			weekly = true
		}
	}

	today := time.Now().Format("2006-01-02")

	if weekly {
		printWeeklyLog(data)
		return
	}

	// Daily log
	weekday := time.Now().Weekday().String()
	fmt.Printf("\n  Time Log — %s (%s)\n", today, weekday)
	fmt.Println(strings.Repeat("─", 50))

	var totalDuration time.Duration
	for _, s := range data.Sessions {
		start, _ := time.Parse(time.RFC3339, s.Start)
		if start.Format("2006-01-02") != today {
			continue
		}
		end, _ := time.Parse(time.RFC3339, s.End)
		dur := end.Sub(start)
		totalDuration += dur
		project := s.Project
		if project == "" {
			project = "work"
		}
		fmt.Printf("  %s → %s   %-20s %s\n",
			start.Format("15:04"), end.Format("15:04"), project, formatDuration(dur))
	}

	// Show active session
	if data.Active != nil {
		start, _ := time.Parse(time.RFC3339, data.Active.Start)
		if start.Format("2006-01-02") == today {
			elapsed := time.Since(start).Truncate(time.Second)
			totalDuration += elapsed
			project := data.Active.Project
			if project == "" {
				project = "work"
			}
			fmt.Printf("  %s → ...     %-20s %s (active)\n",
				start.Format("15:04"), project, formatDuration(elapsed))
		}
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Total: %s\n\n", formatDuration(totalDuration))
}

func printWeeklyLog(data clockData) {
	now := time.Now()
	weekStart := now
	for weekStart.Weekday() != time.Monday {
		weekStart = weekStart.AddDate(0, 0, -1)
	}

	_, weekNum := now.ISOWeek()
	fmt.Printf("\n  Weekly Time Log — Week %d (%s → %s)\n",
		weekNum, weekStart.Format("2006-01-02"), now.Format("2006-01-02"))
	fmt.Println(strings.Repeat("─", 50))

	// Group by day
	dayTotals := make(map[string]time.Duration)
	dayProjects := make(map[string]map[string]time.Duration)

	for _, s := range data.Sessions {
		start, _ := time.Parse(time.RFC3339, s.Start)
		dayStr := start.Format("2006-01-02")
		if dayStr < weekStart.Format("2006-01-02") {
			continue
		}
		end, _ := time.Parse(time.RFC3339, s.End)
		dur := end.Sub(start)
		dayTotals[dayStr] += dur

		project := s.Project
		if project == "" {
			project = "work"
		}
		if dayProjects[dayStr] == nil {
			dayProjects[dayStr] = make(map[string]time.Duration)
		}
		dayProjects[dayStr][project] += dur
	}

	var weekTotal time.Duration
	for d := weekStart; !d.After(now); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format("2006-01-02")
		total := dayTotals[dayStr]
		if total == 0 {
			continue
		}
		weekTotal += total
		weekdayName := d.Weekday().String()[:3]
		fmt.Printf("  %s %s  %s", weekdayName, dayStr, formatDuration(total))

		// Show project breakdown
		projects := dayProjects[dayStr]
		if len(projects) > 0 {
			var parts []string
			for p, dur := range projects {
				parts = append(parts, fmt.Sprintf("%s: %s", p, formatDuration(dur)))
			}
			fmt.Printf("  (%s)", strings.Join(parts, ", "))
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Week total: %s\n\n", formatDuration(weekTotal))
}

// ── Helpers ────────────────────────────────────────────────────────

func resolveClockVault() string {
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

func clockDataPath(vaultPath string) string {
	return filepath.Join(vaultPath, ".granit", "clock.json")
}

func loadClockData(vaultPath string) clockData {
	var data clockData
	raw, err := os.ReadFile(clockDataPath(vaultPath))
	if err != nil {
		return data
	}
	_ = json.Unmarshal(raw, &data)
	return data
}

func saveClockData(vaultPath string, data clockData) {
	dir := filepath.Join(vaultPath, ".granit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		exitError("Error creating clock dir: %v", err)
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		exitError("Error saving clock data: %v", err)
	}
	// Atomic write so a crash mid-save cannot truncate clock.json and lose
	// the user's entire clock-in history.
	path := clockDataPath(vaultPath)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		_ = os.Remove(tmp)
		exitError("Error writing clock data: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		exitError("Error writing clock data: %v", err)
	}
}

func todayTotalTime(data clockData) time.Duration {
	today := time.Now().Format("2006-01-02")
	var total time.Duration
	for _, s := range data.Sessions {
		start, _ := time.Parse(time.RFC3339, s.Start)
		if start.Format("2006-01-02") != today {
			continue
		}
		end, _ := time.Parse(time.RFC3339, s.End)
		total += end.Sub(start)
	}
	return total
}

func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// saveSessionToVault appends a session row to the daily timelog markdown
// file. Returns an error so the caller can surface failures rather than
// silently dropping the user's clocked-out session.
func saveSessionToVault(vaultPath string, start, end time.Time, project string, elapsed time.Duration) error {
	dir := filepath.Join(vaultPath, "Timetracking")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create timelog dir: %w", err)
	}
	dateStr := start.Format("2006-01-02")
	filePath := filepath.Join(dir, dateStr+".md")

	// Create file with header if it doesn't exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		weekday := start.Weekday().String()
		header := fmt.Sprintf("---\ntitle: Time Log %s\ndate: %s\ntype: timelog\ntags: [timelog]\n---\n\n# Time Log — %s (%s)\n\n| Start | End | Project | Duration |\n|-------|-----|---------|----------|\n",
			dateStr, dateStr, dateStr, weekday)
		if err := os.WriteFile(filePath, []byte(header), 0644); err != nil {
			return fmt.Errorf("write timelog header: %w", err)
		}
	}

	// Append the session row
	row := fmt.Sprintf("| %s | %s | %s | %s |\n",
		start.Format("15:04"), end.Format("15:04"), project, formatDuration(elapsed))

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open timelog: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(row); err != nil {
		return fmt.Errorf("append timelog row: %w", err)
	}
	return nil
}
