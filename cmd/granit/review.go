package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// runReview handles "granit review [vault-path]" — generates a daily or weekly summary.
func runReview(args []string) {
	vaultPath := resolveReviewVault(args)

	v, err := vault.NewVault(vaultPath)
	if err != nil {
		exitError("Error opening vault: %v", err)
	}
	if err := v.Scan(); err != nil {
		exitError("Error scanning vault: %v", err)
	}

	weekly := hasFlag("--week") || hasFlag("-w")
	markdown := hasFlag("--markdown") || hasFlag("--md")
	saveFile := hasFlag("--save")

	now := time.Now()
	today := now.Format("2006-01-02")
	weekday := now.Weekday().String()

	allTasks := parseTodayTasks(v)

	if weekly {
		generateWeeklyReview(v, allTasks, now, markdown, saveFile, vaultPath)
	} else {
		generateDailyReview(v, allTasks, today, weekday, markdown, saveFile, vaultPath)
	}
}

// resolveReviewVault determines the vault path from args/env/last-used.
func resolveReviewVault(args []string) string {
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

func generateDailyReview(v *vault.Vault, allTasks []todayTask, today, weekday string, markdown, save bool, vaultPath string) {
	// Categorize tasks
	var completed, inProgress, planned []todayTask
	var overdue []todayTask

	for _, t := range allTasks {
		if t.Done && t.DueDate == today {
			completed = append(completed, t)
			continue
		}
		if t.Done {
			// Check if from today's daily note
			baseName := strings.TrimSuffix(filepath.Base(t.NotePath), ".md")
			if baseName == today {
				completed = append(completed, t)
			}
			continue
		}
		if t.DueDate != "" && t.DueDate < today {
			overdue = append(overdue, t)
			continue
		}
		if t.DueDate == today {
			inProgress = append(inProgress, t)
			continue
		}
		// Tasks from today's daily note without due date
		baseName := strings.TrimSuffix(filepath.Base(t.NotePath), ".md")
		if baseName == today {
			planned = append(planned, t)
		}
	}

	// Count notes modified today
	notesModified := countNotesModifiedOn(v, today)

	if markdown {
		printDailyReviewMarkdown(today, weekday, completed, inProgress, planned, overdue, notesModified)
	} else {
		printDailyReviewPlain(today, weekday, completed, inProgress, planned, overdue, notesModified)
	}

	if save {
		content := captureDailyReviewMarkdown(today, weekday, completed, inProgress, planned, overdue, notesModified)
		savePath := filepath.Join(vaultPath, fmt.Sprintf("Reviews/daily-%s.md", today))
		saveReviewFile(savePath, content)
	}
}

func generateWeeklyReview(v *vault.Vault, allTasks []todayTask, now time.Time, markdown, save bool, vaultPath string) {
	// Calculate week range (Monday to Sunday)
	weekStart := now
	for weekStart.Weekday() != time.Monday {
		weekStart = weekStart.AddDate(0, 0, -1)
	}
	weekEnd := weekStart.AddDate(0, 0, 6)

	startStr := weekStart.Format("2006-01-02")
	endStr := weekEnd.Format("2006-01-02")
	_, weekNum := now.ISOWeek()

	// Categorize tasks for the week
	var completedThisWeek, pendingThisWeek, overdueItems []todayTask

	for _, t := range allTasks {
		if t.Done && t.DueDate >= startStr && t.DueDate <= endStr {
			completedThisWeek = append(completedThisWeek, t)
			continue
		}
		if t.Done {
			baseName := strings.TrimSuffix(filepath.Base(t.NotePath), ".md")
			if baseName >= startStr && baseName <= endStr {
				completedThisWeek = append(completedThisWeek, t)
			}
			continue
		}
		if t.DueDate != "" && t.DueDate < startStr {
			overdueItems = append(overdueItems, t)
			continue
		}
		if t.DueDate >= startStr && t.DueDate <= endStr {
			pendingThisWeek = append(pendingThisWeek, t)
		}
	}

	// Notes created/modified this week
	notesThisWeek := countNotesModifiedBetween(v, startStr, endStr)

	// Tags used this week
	tagCounts := countTagsInRange(allTasks, startStr, endStr)

	if markdown {
		printWeeklyReviewMarkdown(weekNum, startStr, endStr, completedThisWeek, pendingThisWeek, overdueItems, notesThisWeek, tagCounts)
	} else {
		printWeeklyReviewPlain(weekNum, startStr, endStr, completedThisWeek, pendingThisWeek, overdueItems, notesThisWeek, tagCounts)
	}

	if save {
		content := captureWeeklyReviewMarkdown(weekNum, startStr, endStr, completedThisWeek, pendingThisWeek, overdueItems, notesThisWeek, tagCounts)
		savePath := filepath.Join(vaultPath, fmt.Sprintf("Reviews/week-%d-%s.md", weekNum, startStr))
		saveReviewFile(savePath, content)
	}
}

// ── Plain text output ──────────────────────────────────────────────

func printDailyReviewPlain(today, weekday string, completed, inProgress, planned, overdue []todayTask, notesModified int) {
	fmt.Printf("\n  Daily Review — %s (%s)\n", today, weekday)
	fmt.Println(strings.Repeat("─", 50))

	fmt.Printf("  Completed: %d | In progress: %d | Planned: %d | Overdue: %d\n", len(completed), len(inProgress), len(planned), len(overdue))
	fmt.Printf("  Notes modified: %d\n", notesModified)
	fmt.Println(strings.Repeat("─", 50))

	if len(completed) > 0 {
		fmt.Println("\n  ✓ Completed")
		for _, t := range completed {
			fmt.Printf("    ☑ %s\n", t.Text)
		}
	}

	if len(inProgress) > 0 {
		fmt.Println("\n  → In Progress")
		for _, t := range inProgress {
			fmt.Printf("    ☐ %s\n", t.Text)
		}
	}

	if len(planned) > 0 {
		fmt.Println("\n  📋 Planned")
		for _, t := range planned {
			fmt.Printf("    ☐ %s\n", t.Text)
		}
	}

	if len(overdue) > 0 {
		fmt.Println("\n  ⚠ Overdue")
		for _, t := range overdue {
			fmt.Printf("    ☐ %s (📅 %s)\n", t.Text, t.DueDate)
		}
	}

	fmt.Println()
}

func printWeeklyReviewPlain(weekNum int, startStr, endStr string, completed, pending, overdue []todayTask, notesCount int, tagCounts map[string]int) {
	fmt.Printf("\n  Weekly Review — Week %d (%s → %s)\n", weekNum, startStr, endStr)
	fmt.Println(strings.Repeat("─", 50))

	fmt.Printf("  Completed: %d | Pending: %d | Overdue: %d\n", len(completed), len(pending), len(overdue))
	fmt.Printf("  Notes modified: %d\n", notesCount)
	fmt.Println(strings.Repeat("─", 50))

	if len(completed) > 0 {
		fmt.Println("\n  ✓ Completed this week")
		for _, t := range completed {
			fmt.Printf("    ☑ %s\n", t.Text)
		}
	}

	if len(pending) > 0 {
		fmt.Println("\n  → Still pending")
		for _, t := range pending {
			fmt.Printf("    ☐ %s (📅 %s)\n", t.Text, t.DueDate)
		}
	}

	if len(overdue) > 0 {
		fmt.Println("\n  ⚠ Overdue (from before this week)")
		for _, t := range overdue {
			fmt.Printf("    ☐ %s (📅 %s)\n", t.Text, t.DueDate)
		}
	}

	if len(tagCounts) > 0 {
		fmt.Println("\n  🏷  Top areas")
		top := topTags(tagCounts, 5)
		for _, kv := range top {
			fmt.Printf("    #%-15s %d task(s)\n", kv.key, kv.val)
		}
	}

	fmt.Println()
}

// ── Markdown output (--markdown/--md) ──────────────────────────────

func printDailyReviewMarkdown(today, weekday string, completed, inProgress, planned, overdue []todayTask, notesModified int) {
	fmt.Print(captureDailyReviewMarkdown(today, weekday, completed, inProgress, planned, overdue, notesModified))
}

func captureDailyReviewMarkdown(today, weekday string, completed, inProgress, planned, overdue []todayTask, notesModified int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Daily Review — %s (%s)\n\n", today, weekday))
	b.WriteString(fmt.Sprintf("**Completed:** %d | **In progress:** %d | **Planned:** %d | **Overdue:** %d\n", len(completed), len(inProgress), len(planned), len(overdue)))
	b.WriteString(fmt.Sprintf("**Notes modified:** %d\n\n", notesModified))

	if len(completed) > 0 {
		b.WriteString("## Completed\n")
		for _, t := range completed {
			b.WriteString(fmt.Sprintf("- [x] %s\n", t.Text))
		}
		b.WriteString("\n")
	}

	if len(inProgress) > 0 {
		b.WriteString("## In Progress\n")
		for _, t := range inProgress {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", t.Text))
		}
		b.WriteString("\n")
	}

	if len(planned) > 0 {
		b.WriteString("## Planned\n")
		for _, t := range planned {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", t.Text))
		}
		b.WriteString("\n")
	}

	if len(overdue) > 0 {
		b.WriteString("## Overdue\n")
		for _, t := range overdue {
			b.WriteString(fmt.Sprintf("- [ ] %s (📅 %s)\n", t.Text, t.DueDate))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func printWeeklyReviewMarkdown(weekNum int, startStr, endStr string, completed, pending, overdue []todayTask, notesCount int, tagCounts map[string]int) {
	fmt.Print(captureWeeklyReviewMarkdown(weekNum, startStr, endStr, completed, pending, overdue, notesCount, tagCounts))
}

func captureWeeklyReviewMarkdown(weekNum int, startStr, endStr string, completed, pending, overdue []todayTask, notesCount int, tagCounts map[string]int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Weekly Review — Week %d (%s → %s)\n\n", weekNum, startStr, endStr))
	b.WriteString(fmt.Sprintf("**Completed:** %d | **Pending:** %d | **Overdue:** %d\n", len(completed), len(pending), len(overdue)))
	b.WriteString(fmt.Sprintf("**Notes modified:** %d\n\n", notesCount))

	if len(completed) > 0 {
		b.WriteString("## Completed this week\n")
		for _, t := range completed {
			b.WriteString(fmt.Sprintf("- [x] %s\n", t.Text))
		}
		b.WriteString("\n")
	}

	if len(pending) > 0 {
		b.WriteString("## Still pending\n")
		for _, t := range pending {
			b.WriteString(fmt.Sprintf("- [ ] %s (📅 %s)\n", t.Text, t.DueDate))
		}
		b.WriteString("\n")
	}

	if len(overdue) > 0 {
		b.WriteString("## Overdue\n")
		for _, t := range overdue {
			b.WriteString(fmt.Sprintf("- [ ] %s (📅 %s)\n", t.Text, t.DueDate))
		}
		b.WriteString("\n")
	}

	if len(tagCounts) > 0 {
		b.WriteString("## Top areas\n")
		top := topTags(tagCounts, 5)
		for _, kv := range top {
			b.WriteString(fmt.Sprintf("- **#%s** — %d task(s)\n", kv.key, kv.val))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// ── Helpers ────────────────────────────────────────────────────────

func countNotesModifiedOn(v *vault.Vault, dateStr string) int {
	count := 0
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		if note.ModTime.Year() == date.Year() && note.ModTime.YearDay() == date.YearDay() {
			count++
		}
	}
	return count
}

func countNotesModifiedBetween(v *vault.Vault, startStr, endStr string) int {
	count := 0
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		modDate := note.ModTime.Format("2006-01-02")
		if modDate >= startStr && modDate <= endStr {
			count++
		}
	}
	return count
}

func countTagsInRange(tasks []todayTask, startStr, endStr string) map[string]int {
	counts := make(map[string]int)
	for _, t := range tasks {
		inRange := false
		if t.DueDate >= startStr && t.DueDate <= endStr {
			inRange = true
		}
		baseName := strings.TrimSuffix(filepath.Base(t.NotePath), ".md")
		if baseName >= startStr && baseName <= endStr {
			inRange = true
		}
		if inRange {
			for _, tag := range t.Tags {
				counts[tag]++
			}
		}
	}
	return counts
}

type tagCount struct {
	key string
	val int
}

func topTags(counts map[string]int, limit int) []tagCount {
	var sorted []tagCount
	for k, v := range counts {
		sorted = append(sorted, tagCount{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].val > sorted[j].val
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

func saveReviewFile(path, content string) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create Reviews directory: %v\n", err)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save review: %v\n", err)
		return
	}
	fmt.Printf("Saved review to %s\n", path)
}
