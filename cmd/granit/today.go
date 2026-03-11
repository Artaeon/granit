package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// todayTask mirrors the task structure for CLI display.
type todayTask struct {
	Text     string
	Done     bool
	DueDate  string
	Priority int
	NotePath string
	Tags     []string
}

var (
	todayTaskRe    = regexp.MustCompile(`^(\s*- \[)([ xX])(\] .+)`)
	todayDueDateRe = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)
	todayPrioRe4   = regexp.MustCompile(`\x{1F53A}`)
	todayPrioRe3   = regexp.MustCompile(`\x{23EB}`)
	todayPrioRe2   = regexp.MustCompile(`\x{1F53C}`)
	todayPrioRe1   = regexp.MustCompile(`\x{1F53D}`)
	todayTagRe     = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
)

// runToday handles "granit today [vault-path]" — prints a terminal dashboard.
func runToday(args []string) {
	vaultPath := resolveTodayVault(args)

	v, err := vault.NewVault(vaultPath)
	if err != nil {
		exitError("Error opening vault: %v", err)
	}
	if err := v.Scan(); err != nil {
		exitError("Error scanning vault: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	weekday := time.Now().Weekday().String()

	jsonOut := hasFlag("--json")

	// Parse all tasks
	allTasks := parseTodayTasks(v)

	// Categorize
	var overdue, todayTasks, upcoming []todayTask
	var completed int
	for _, t := range allTasks {
		if t.Done {
			if t.DueDate == today {
				completed++
			}
			continue
		}
		if t.DueDate == "" {
			// No due date — show under today if in today's daily note
			baseName := strings.TrimSuffix(filepath.Base(t.NotePath), ".md")
			if baseName == today {
				todayTasks = append(todayTasks, t)
			}
			continue
		}
		if t.DueDate < today {
			overdue = append(overdue, t)
		} else if t.DueDate == today {
			todayTasks = append(todayTasks, t)
		} else if t.DueDate <= addDays(today, 7) {
			upcoming = append(upcoming, t)
		}
	}

	// Sort by priority (highest first)
	sortByPriority(overdue)
	sortByPriority(todayTasks)
	sortByPriority(upcoming)

	// Load habits
	habits, todayHabits := loadTodayHabits(vaultPath, today)

	if jsonOut {
		printTodayJSON(today, weekday, overdue, todayTasks, upcoming, completed, habits, todayHabits)
		return
	}

	// Print dashboard
	totalToday := len(todayTasks) + completed
	fmt.Printf("\n  %s — %s\n", today, weekday)
	fmt.Printf("  %d task(s) today, %d completed, %d overdue\n", totalToday, completed, len(overdue))
	fmt.Println(strings.Repeat("─", 50))

	if len(overdue) > 0 {
		fmt.Println("\n  ⚠ Overdue")
		for _, t := range overdue {
			fmt.Printf("    %s %s (📅 %s)\n", taskCheckbox(t.Done), t.Text, t.DueDate)
		}
	}

	if len(todayTasks) > 0 {
		fmt.Println("\n  📋 Today")
		for _, t := range todayTasks {
			prio := priorityLabel(t.Priority)
			extra := ""
			if prio != "" {
				extra = " " + prio
			}
			fmt.Printf("    %s %s%s\n", taskCheckbox(t.Done), t.Text, extra)
		}
	} else if completed == 0 {
		fmt.Println("\n  📋 No tasks for today")
	}

	if completed > 0 {
		fmt.Printf("\n  ✓ %d task(s) completed today\n", completed)
	}

	if len(upcoming) > 0 {
		fmt.Println("\n  📅 Upcoming (7 days)")
		for _, t := range upcoming {
			fmt.Printf("    %s %s (📅 %s)\n", taskCheckbox(t.Done), t.Text, t.DueDate)
		}
	}

	if len(habits) > 0 {
		fmt.Println("\n  🔁 Habits")
		for _, h := range habits {
			done := "☐"
			for _, c := range todayHabits {
				if c == h.name {
					done = "☑"
					break
				}
			}
			streak := ""
			if h.streak > 0 {
				streak = fmt.Sprintf(" (streak: %d days)", h.streak)
			}
			fmt.Printf("    %s %s%s\n", done, h.name, streak)
		}
	}

	fmt.Println()
}

// resolveTodayVault determines the vault path from args/env/last-used.
func resolveTodayVault(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	if envVault := os.Getenv("GRANIT_VAULT"); envVault != "" {
		return envVault
	}
	if last := config.LoadVaultList().LastUsed; last != "" {
		return last
	}
	return "."
}

// parseTodayTasks extracts all tasks from the vault.
func parseTodayTasks(v *vault.Vault) []todayTask {
	var tasks []todayTask
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		if note.Content == "" {
			continue
		}
		lines := strings.Split(note.Content, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [") {
				continue
			}
			m := todayTaskRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			done := m[2] == "x" || m[2] == "X"
			text := m[3][2:]

			t := todayTask{
				Text:     cleanTaskText(text),
				Done:     done,
				NotePath: note.RelPath,
			}

			if dm := todayDueDateRe.FindStringSubmatch(text); dm != nil {
				t.DueDate = dm[1]
			}
			if todayPrioRe4.MatchString(text) {
				t.Priority = 4
			} else if todayPrioRe3.MatchString(text) {
				t.Priority = 3
			} else if todayPrioRe2.MatchString(text) {
				t.Priority = 2
			} else if todayPrioRe1.MatchString(text) {
				t.Priority = 1
			}
			for _, tm := range todayTagRe.FindAllStringSubmatch(text, -1) {
				t.Tags = append(t.Tags, tm[1])
			}

			tasks = append(tasks, t)
		}
	}
	return tasks
}

// cleanTaskText strips emoji markers for cleaner display.
func cleanTaskText(text string) string {
	text = todayDueDateRe.ReplaceAllString(text, "")
	text = todayPrioRe4.ReplaceAllString(text, "")
	text = todayPrioRe3.ReplaceAllString(text, "")
	text = todayPrioRe2.ReplaceAllString(text, "")
	text = todayPrioRe1.ReplaceAllString(text, "")
	text = regexp.MustCompile(`⏰\s*\d{2}:\d{2}-\d{2}:\d{2}`).ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	return text
}

type todayHabit struct {
	name   string
	streak int
}

// loadTodayHabits reads habits from the vault's Habits/habits.md file.
func loadTodayHabits(vaultPath, today string) ([]todayHabit, []string) {
	habitsFile := filepath.Join(vaultPath, "Habits", "habits.md")
	data, err := os.ReadFile(habitsFile)
	if err != nil {
		return nil, nil
	}

	var habits []todayHabit
	var todayCompleted []string
	content := string(data)
	lines := strings.Split(content, "\n")

	section := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Habits" {
			section = "habits"
			continue
		}
		if trimmed == "## Log" {
			section = "log"
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			section = ""
			continue
		}
		if !strings.HasPrefix(trimmed, "|") || strings.Contains(trimmed, "---") {
			continue
		}
		cells := parseTableCells(trimmed)
		if section == "habits" && len(cells) >= 3 && cells[0] != "Habit" {
			streak, _ := strconv.Atoi(strings.TrimSpace(cells[2]))
			habits = append(habits, todayHabit{
				name:   strings.TrimSpace(cells[0]),
				streak: streak,
			})
		}
		if section == "log" && len(cells) >= 2 && cells[0] != "Date" {
			if strings.TrimSpace(cells[0]) == today {
				for _, c := range strings.Split(cells[1], ",") {
					c = strings.TrimSpace(c)
					if c != "" {
						todayCompleted = append(todayCompleted, c)
					}
				}
			}
		}
	}
	return habits, todayCompleted
}

// parseTableCells splits a markdown table row into cells.
func parseTableCells(line string) []string {
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cells = append(cells, p)
		}
	}
	return cells
}

func taskCheckbox(done bool) string {
	if done {
		return "☑"
	}
	return "☐"
}

func priorityLabel(p int) string {
	switch p {
	case 4:
		return "🔺"
	case 3:
		return "⏫"
	case 2:
		return "🔼"
	case 1:
		return "🔽"
	default:
		return ""
	}
}

func sortByPriority(tasks []todayTask) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Priority > tasks[j].Priority
	})
}

func addDays(dateStr string, days int) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

func printTodayJSON(today, weekday string, overdue, todayTasks, upcoming []todayTask, completed int, habits []todayHabit, todayHabits []string) {
	fmt.Println("{")
	fmt.Printf("  \"date\": %q,\n", today)
	fmt.Printf("  \"weekday\": %q,\n", weekday)
	fmt.Printf("  \"completed\": %d,\n", completed)

	printTaskArray := func(name string, tasks []todayTask, last bool) {
		fmt.Printf("  %q: [", name)
		if len(tasks) == 0 {
			fmt.Print("]")
		} else {
			fmt.Println()
			for i, t := range tasks {
				comma := ","
				if i == len(tasks)-1 {
					comma = ""
				}
				fmt.Printf("    {\"text\": %q, \"priority\": %d, \"due\": %q, \"note\": %q}%s\n",
					t.Text, t.Priority, t.DueDate, t.NotePath, comma)
			}
			fmt.Print("  ]")
		}
		if !last {
			fmt.Println(",")
		} else {
			fmt.Println()
		}
	}

	printTaskArray("overdue", overdue, false)
	printTaskArray("today", todayTasks, false)
	printTaskArray("upcoming", upcoming, len(habits) == 0)

	if len(habits) > 0 {
		fmt.Println("  \"habits\": [")
		for i, h := range habits {
			done := false
			for _, c := range todayHabits {
				if c == h.name {
					done = true
					break
				}
			}
			comma := ","
			if i == len(habits)-1 {
				comma = ""
			}
			fmt.Printf("    {\"name\": %q, \"streak\": %d, \"done_today\": %v}%s\n",
				h.name, h.streak, done, comma)
		}
		fmt.Println("  ]")
	}

	fmt.Println("}")
}
