package tui

import (
	"strings"
	"time"
)

// sheetTemplate is a starter pack for a brand-new spreadsheet.
// Each template carries:
//   - a stable ID for keyboard activation by digit
//   - a one-line user-facing description (shown in the picker)
//   - a default filename pattern, with {date} expanded to today
//   - the actual seed rows (header + a handful of example rows)
//
// Templates are intentionally small (10-15 sample rows) so a new
// user can SEE the structure immediately and either keep, edit,
// or delete the example rows. Bigger seeds would feel like junk
// data the user has to clean up.
//
// All numbers are written without locale-specific grouping so
// the parser handles them whether the user's vault is set up
// with US ($1,234.50) or DE (1.234,50 €) conventions — the
// display formatter re-renders them to match the data shape.
type sheetTemplate struct {
	ID          string
	Name        string
	Description string
	Suggested   string
	Rows        [][]string
}

// expandFilename replaces {date} / {month} / {year} placeholders
// with today's values.
func (t sheetTemplate) expandFilename(now time.Time) string {
	return svExpandPlaceholders(t.Suggested, now)
}

// expandedRows returns the template's row data with {date} etc.
// substituted in cell values (Invoice etc.) so the user lands
// on a sheet with today's date already filled in.
func (t sheetTemplate) expandedRows(now time.Time) [][]string {
	out := make([][]string, len(t.Rows))
	for i, row := range t.Rows {
		nr := make([]string, len(row))
		for j, cell := range row {
			nr[j] = svExpandPlaceholders(cell, now)
		}
		out[i] = nr
	}
	return out
}

func svExpandPlaceholders(s string, now time.Time) string {
	if !strings.Contains(s, "{") {
		return s
	}
	s = strings.ReplaceAll(s, "{date}", now.Format("2006-01-02"))
	s = strings.ReplaceAll(s, "{month}", now.Format("2006-01"))
	s = strings.ReplaceAll(s, "{year}", now.Format("2006"))
	return s
}

// allSheetTemplates returns the built-in template list. New
// templates can be added freely — the picker auto-numbers them.
// Order matters: most-useful templates first so the digit keys
// (1-9) land on the most common picks.
func allSheetTemplates() []sheetTemplate {
	return []sheetTemplate{
		{
			ID:          "blank",
			Name:        "Blank Sheet",
			Description: "Empty 4-column sheet — start from scratch",
			Suggested:   "Untitled-{date}",
			Rows: [][]string{
				{"A", "B", "C", "D"},
				{"", "", "", ""},
				{"", "", "", ""},
			},
		},
		{
			ID:          "monthly-budget",
			Name:        "Monthly Budget",
			Description: "Income vs expenses by category, with planned/actual diff",
			Suggested:   "Budget-{month}",
			Rows: [][]string{
				{"Category", "Type", "Planned", "Actual", "Notes"},
				{"Salary", "Income", "5000.00", "5000.00", ""},
				{"Side Income", "Income", "500.00", "0.00", ""},
				{"Investments", "Income", "100.00", "85.00", "Dividends"},
				{"", "", "", "", ""},
				{"Rent / Mortgage", "Expense", "1500.00", "1500.00", ""},
				{"Groceries", "Expense", "600.00", "642.30", ""},
				{"Utilities", "Expense", "180.00", "165.00", "Gas + electric"},
				{"Internet & Phone", "Expense", "75.00", "75.00", ""},
				{"Transport", "Expense", "200.00", "180.00", ""},
				{"Subscriptions", "Expense", "50.00", "62.99", "Netflix, Spotify"},
				{"Dining Out", "Expense", "200.00", "240.00", ""},
				{"Entertainment", "Expense", "100.00", "85.00", ""},
				{"Health & Fitness", "Expense", "80.00", "80.00", "Gym"},
				{"Savings", "Goal", "800.00", "800.00", "Emergency fund"},
			},
		},
		{
			ID:          "expense-tracker",
			Name:        "Daily Expense Tracker",
			Description: "Per-transaction log with date, category, amount, payment method",
			Suggested:   "Expenses-{month}",
			Rows: [][]string{
				{"Date", "Category", "Description", "Amount", "Payment", "Notes"},
				{"2026-04-01", "Groceries", "Weekly shop", "67.40", "Card", ""},
				{"2026-04-02", "Coffee", "Morning latte", "4.50", "Cash", ""},
				{"2026-04-03", "Transport", "Bus pass", "30.00", "Card", "Monthly"},
				{"2026-04-05", "Dining", "Lunch with team", "22.00", "Card", "Work"},
				{"2026-04-06", "Subscriptions", "Spotify", "9.99", "Card", "Auto-renew"},
				{"2026-04-08", "Health", "Pharmacy", "12.50", "Cash", ""},
				{"2026-04-10", "Entertainment", "Movie", "15.00", "Card", ""},
			},
		},
		{
			ID:          "task-tracker",
			Name:        "Task / Project Tracker",
			Description: "Task list with status, priority, owner, due date, est hours",
			Suggested:   "Tasks-{date}",
			Rows: [][]string{
				{"Task", "Status", "Priority", "Owner", "Due", "Est Hours", "Notes"},
				{"Define MVP scope", "Done", "High", "You", "2026-04-15", "4", ""},
				{"Set up project repo", "Done", "Medium", "You", "2026-04-16", "1", ""},
				{"Build login flow", "In Progress", "High", "You", "2026-04-30", "8", ""},
				{"Database schema design", "Todo", "High", "You", "2026-05-02", "6", ""},
				{"Write API tests", "Todo", "Medium", "You", "2026-05-05", "10", ""},
				{"Deploy to staging", "Blocked", "Medium", "Ops", "2026-05-10", "3", "Waiting on infra"},
				{"User feedback round 1", "Todo", "Low", "You", "2026-05-15", "4", ""},
			},
		},
		{
			ID:          "habit-tracker",
			Name:        "Weekly Habit Tracker",
			Description: "Habits × weekdays with checkboxes, plus streak count",
			Suggested:   "Habits-{date}",
			Rows: [][]string{
				{"Habit", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun", "Streak"},
				{"Morning meditation", "x", "x", "x", "", "", "", "", "3"},
				{"Workout", "x", "", "x", "", "x", "", "", "0"},
				{"Read 20 minutes", "x", "x", "x", "x", "", "", "", "4"},
				{"No social media before noon", "x", "x", "", "", "", "", "", "0"},
				{"Cook at home", "x", "x", "x", "", "x", "", "", "0"},
				{"Journal", "", "x", "x", "x", "", "", "", "0"},
				{"In bed by 23:00", "x", "x", "x", "x", "", "", "", "0"},
			},
		},
		{
			ID:          "time-log",
			Name:        "Time Log / Timesheet",
			Description: "Daily time entries with project, task, hours billable/non",
			Suggested:   "Timelog-{month}",
			Rows: [][]string{
				{"Date", "Project", "Task", "Hours", "Billable", "Rate", "Total", "Notes"},
				{"2026-04-01", "Granit", "TUI redesign", "3.5", "yes", "85.00", "297.50", ""},
				{"2026-04-01", "Internal", "Email", "0.5", "no", "0.00", "0.00", ""},
				{"2026-04-02", "Granit", "Sheet view", "5.0", "yes", "85.00", "425.00", ""},
				{"2026-04-03", "Client A", "Onboarding call", "1.0", "yes", "120.00", "120.00", ""},
				{"2026-04-04", "Client A", "Architecture doc", "4.0", "yes", "120.00", "480.00", ""},
				{"2026-04-05", "Internal", "Admin", "1.0", "no", "0.00", "0.00", ""},
			},
		},
		{
			ID:          "sales-pipeline",
			Name:        "Sales Pipeline",
			Description: "Deals with stage, value, probability, weighted forecast",
			Suggested:   "Pipeline-{month}",
			Rows: [][]string{
				{"Deal", "Stage", "Value", "Probability", "Weighted", "Owner", "Close Date", "Next Action"},
				{"Acme Corp — Annual Plan", "Negotiation", "12000.00", "70%", "8400.00", "You", "2026-04-30", "Send revised quote"},
				{"Beta Studios — Pilot", "Discovery", "3000.00", "30%", "900.00", "You", "2026-05-15", "Schedule demo"},
				{"Cygnus Ltd — Enterprise", "Proposal", "45000.00", "50%", "22500.00", "You", "2026-06-01", "Follow up on RFP"},
				{"Delta Co — Renewal", "Won", "8500.00", "100%", "8500.00", "You", "2026-04-12", ""},
				{"Echo Group — Lead", "Lead", "2000.00", "10%", "200.00", "You", "2026-07-01", "Cold email"},
			},
		},
		{
			ID:          "invoice",
			Name:        "Simple Invoice",
			Description: "Invoice line items with quantity × rate = total",
			Suggested:   "Invoice-{date}",
			Rows: [][]string{
				{"Invoice #", "INV-2026-001", "", "", ""},
				{"Date", "{date}", "", "", ""},
				{"Client", "Acme Corp", "", "", ""},
				{"Due", "Net 30", "", "", ""},
				{"", "", "", "", ""},
				{"Item", "Description", "Quantity", "Rate", "Amount"},
				{"Consulting", "Architecture review", "8", "120.00", "960.00"},
				{"Development", "TUI feature work", "16", "85.00", "1360.00"},
				{"Support", "Q2 retainer", "1", "500.00", "500.00"},
				{"", "", "", "", ""},
				{"", "", "", "Subtotal", "2820.00"},
				{"", "", "", "Tax (20%)", "564.00"},
				{"", "", "", "Total Due", "3384.00"},
			},
		},
		{
			ID:          "workout",
			Name:        "Workout Log",
			Description: "Exercises with sets × reps × weight, weekly volume",
			Suggested:   "Workout-{month}",
			Rows: [][]string{
				{"Date", "Exercise", "Sets", "Reps", "Weight", "Volume", "RPE", "Notes"},
				{"2026-04-01", "Squat", "5", "5", "100.0", "2500.0", "8", ""},
				{"2026-04-01", "Bench Press", "5", "5", "75.0", "1875.0", "7", ""},
				{"2026-04-01", "Row", "3", "8", "60.0", "1440.0", "7", ""},
				{"2026-04-03", "Deadlift", "3", "5", "120.0", "1800.0", "8", ""},
				{"2026-04-03", "Overhead Press", "5", "5", "45.0", "1125.0", "8", ""},
				{"2026-04-03", "Pull-Up", "3", "8", "0.0", "0.0", "9", "Bodyweight"},
				{"2026-04-05", "Squat", "5", "5", "102.5", "2562.5", "8", ""},
			},
		},
		{
			ID:          "reading-list",
			Name:        "Reading List",
			Description: "Books with status, rating, started/finished, notes",
			Suggested:   "Reading-{year}",
			Rows: [][]string{
				{"Title", "Author", "Status", "Rating", "Started", "Finished", "Notes"},
				{"Atomic Habits", "James Clear", "Read", "5", "2026-01-10", "2026-02-04", "Re-read intro"},
				{"Designing Data-Intensive Applications", "Kleppmann", "Reading", "", "2026-03-01", "", "Up to ch.7"},
				{"The Pragmatic Programmer", "Hunt & Thomas", "To Read", "", "", "", ""},
				{"Range", "David Epstein", "Read", "4", "2026-02-10", "2026-02-25", ""},
				{"Crafting Interpreters", "Robert Nystrom", "To Read", "", "", "", "Free online"},
			},
		},
	}
}
