package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// formatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration_Seconds(t *testing.T) {
	if got := formatDuration(45 * time.Second); got != "45s" {
		t.Errorf("expected '45s', got %q", got)
	}
}

func TestFormatDuration_Minutes(t *testing.T) {
	if got := formatDuration(5*time.Minute + 30*time.Second); got != "5m 30s" {
		t.Errorf("expected '5m 30s', got %q", got)
	}
}

func TestFormatDuration_Hours(t *testing.T) {
	if got := formatDuration(2*time.Hour + 15*time.Minute + 9*time.Second); got != "2h 15m" {
		t.Errorf("expected '2h 15m', got %q", got)
	}
}

func TestFormatDuration_TruncatesSubSecond(t *testing.T) {
	d := 45*time.Second + 750*time.Millisecond
	if got := formatDuration(d); got != "45s" {
		t.Errorf("expected '45s', got %q", got)
	}
}

func TestFormatDuration_Zero(t *testing.T) {
	if got := formatDuration(0); got != "0s" {
		t.Errorf("expected '0s', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// clockDataPath
// ---------------------------------------------------------------------------

func TestClockDataPath(t *testing.T) {
	got := clockDataPath("/tmp/myvault")
	want := filepath.Join("/tmp/myvault", ".granit", "clock.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// loadClockData / saveClockData
// ---------------------------------------------------------------------------

func TestLoadClockData_MissingFile(t *testing.T) {
	dir := t.TempDir()
	data := loadClockData(dir)
	if data.Active != nil {
		t.Error("expected no active session for empty vault")
	}
	if len(data.Sessions) != 0 {
		t.Errorf("expected zero sessions, got %d", len(data.Sessions))
	}
}

func TestSaveAndLoadClockData_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	original := clockData{
		Active: &clockSession{
			Project: "granit",
			Start:   now.Format(time.RFC3339),
		},
		Sessions: []clockSession{
			{Project: "alpha", Start: now.Add(-1 * time.Hour).Format(time.RFC3339), End: now.Format(time.RFC3339)},
		},
	}
	saveClockData(dir, original)

	loaded := loadClockData(dir)
	if loaded.Active == nil || loaded.Active.Project != "granit" {
		t.Errorf("active session lost or wrong: %+v", loaded.Active)
	}
	if len(loaded.Sessions) != 1 || loaded.Sessions[0].Project != "alpha" {
		t.Errorf("session history wrong: %+v", loaded.Sessions)
	}
}

// Regression: saveClockData must be atomic — no leftover .tmp on success.
func TestSaveClockData_AtomicNoTmp(t *testing.T) {
	dir := t.TempDir()
	saveClockData(dir, clockData{Sessions: []clockSession{{Project: "x"}}})
	if _, err := os.Stat(clockDataPath(dir) + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected no .tmp file, stat err = %v", err)
	}
}

// Regression: malformed JSON in clock.json must not crash; load returns empty.
func TestLoadClockData_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".granit"), 0755)
	_ = os.WriteFile(clockDataPath(dir), []byte("{not json"), 0644)
	data := loadClockData(dir)
	if data.Active != nil || len(data.Sessions) != 0 {
		t.Errorf("malformed JSON should produce empty data, got %+v", data)
	}
}

// ---------------------------------------------------------------------------
// todayTotalTime
// ---------------------------------------------------------------------------

func TestTodayTotalTime_Empty(t *testing.T) {
	if got := todayTotalTime(clockData{}); got != 0 {
		t.Errorf("expected 0 for empty data, got %v", got)
	}
}

func TestTodayTotalTime_OnlyTodaySessions(t *testing.T) {
	now := time.Now()
	// Skip near midnight: when now is within 1h of 00:00, "now-1h" lives on
	// yesterday and the "today" filter excludes both sessions, breaking
	// the assertion. Mirrors the skip helpers in internal/tui.
	if now.Hour() == 0 || (now.Hour() == 23 && now.Minute() >= 0) {
		t.Skip("skipping date-boundary-sensitive test near midnight")
	}
	earlier := now.Add(-1 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	data := clockData{
		Sessions: []clockSession{
			{Start: earlier.Format(time.RFC3339), End: now.Format(time.RFC3339)},
			{Start: yesterday.Add(-1 * time.Hour).Format(time.RFC3339), End: yesterday.Format(time.RFC3339)},
		},
	}
	got := todayTotalTime(data)
	want := time.Hour
	// allow a small tolerance because of RFC3339 second-precision rounding
	if got < want-2*time.Second || got > want+2*time.Second {
		t.Errorf("expected ~1h, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// saveSessionToVault
// ---------------------------------------------------------------------------

func TestSaveSessionToVault_CreatesFileWithHeader(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 4, 10, 9, 0, 0, 0, time.Local)
	end := start.Add(45 * time.Minute)
	if err := saveSessionToVault(dir, start, end, "writing", 45*time.Minute); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "Timetracking", "2026-04-10.md")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected timelog at %s, got %v", logPath, err)
	}
	got := string(data)
	if !strings.Contains(got, "type: timelog") {
		t.Error("expected frontmatter type: timelog")
	}
	if !strings.Contains(got, "| 09:00 | 09:45 | writing | 45m 00s |") {
		t.Errorf("expected session row, got:\n%s", got)
	}
}

func TestSaveSessionToVault_AppendsNotOverwrites(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 4, 10, 9, 0, 0, 0, time.Local)
	if err := saveSessionToVault(dir, start, start.Add(30*time.Minute), "alpha", 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := saveSessionToVault(dir, start.Add(1*time.Hour), start.Add(2*time.Hour), "beta", time.Hour); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Timetracking", "2026-04-10.md"))
	if !strings.Contains(string(data), "alpha") || !strings.Contains(string(data), "beta") {
		t.Errorf("expected both sessions in timelog, got:\n%s", data)
	}
	// Header should appear exactly once.
	if c := strings.Count(string(data), "type: timelog"); c != 1 {
		t.Errorf("expected exactly one frontmatter, got %d", c)
	}
}

// Verify the clock data structure marshals as expected — guards against
// accidental field tag changes that would break deserialization of older logs.
func TestClockData_JSONFieldNames(t *testing.T) {
	d := clockData{
		Active: &clockSession{Project: "p", Start: "2026-04-10T09:00:00Z"},
	}
	raw, _ := json.Marshal(d)
	for _, want := range []string{"\"active\"", "\"project\"", "\"start\""} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("expected JSON to contain %s, got %s", want, raw)
		}
	}
}
