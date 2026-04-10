package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// remindersPath
// ---------------------------------------------------------------------------

func TestRemindersPath(t *testing.T) {
	got := remindersPath("/tmp/myvault")
	want := filepath.Join("/tmp/myvault", ".granit", "reminders.json")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// loadReminders / saveReminders round-trip
// ---------------------------------------------------------------------------

func TestLoadReminders_MissingFile(t *testing.T) {
	vault := t.TempDir()
	got := loadReminders(vault)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got))
	}
}

func TestSaveAndLoadReminders_RoundTrip(t *testing.T) {
	vault := t.TempDir()
	original := []reminder{
		{Text: "Drink water", Time: "10:00", Repeat: "daily", Enabled: true},
		{Text: "Stand up", Time: "14:30", Repeat: "weekdays", Enabled: true, Created: "2026-04-10"},
		{Text: "Snooze", Time: "22:00", Repeat: "once", Enabled: false},
	}
	saveReminders(vault, original)

	loaded := loadReminders(vault)
	if len(loaded) != 3 {
		t.Fatalf("expected 3 reminders, got %d", len(loaded))
	}
	if loaded[0].Text != "Drink water" || loaded[0].Time != "10:00" || !loaded[0].Enabled {
		t.Errorf("first reminder lost fields: %+v", loaded[0])
	}
	if loaded[1].Created != "2026-04-10" {
		t.Errorf("created date lost: %q", loaded[1].Created)
	}
	if loaded[2].Enabled {
		t.Errorf("disabled state lost: %+v", loaded[2])
	}
}

// Regression: saveReminders must be atomic (commit e384a34) — no
// leftover .tmp file on success.
func TestSaveReminders_AtomicNoTmp(t *testing.T) {
	vault := t.TempDir()
	saveReminders(vault, []reminder{{Text: "x", Time: "00:00", Repeat: "once", Enabled: true}})

	if _, err := os.Stat(remindersPath(vault) + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected no .tmp file after save, stat err = %v", err)
	}
}

func TestSaveReminders_OverwritesPrevious(t *testing.T) {
	vault := t.TempDir()
	saveReminders(vault, []reminder{{Text: "old", Time: "09:00", Repeat: "daily", Enabled: true}})
	saveReminders(vault, []reminder{{Text: "new", Time: "10:00", Repeat: "daily", Enabled: true}})

	loaded := loadReminders(vault)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 reminder after overwrite, got %d", len(loaded))
	}
	if loaded[0].Text != "new" {
		t.Errorf("expected 'new', got %q", loaded[0].Text)
	}
}

func TestSaveReminders_EmptyList(t *testing.T) {
	vault := t.TempDir()
	saveReminders(vault, []reminder{})
	loaded := loadReminders(vault)
	if len(loaded) != 0 {
		t.Errorf("expected 0 reminders, got %d", len(loaded))
	}
}

// Regression: malformed reminders.json must not crash; load returns empty.
func TestLoadReminders_MalformedJSON(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, ".granit")
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(remindersPath(vault), []byte("{not json"), 0644)

	loaded := loadReminders(vault)
	if len(loaded) != 0 {
		t.Errorf("expected empty result for malformed JSON, got %d", len(loaded))
	}
}

// JSON field tags must remain stable so older reminders.json files still load.
func TestReminder_JSONFieldNames(t *testing.T) {
	vault := t.TempDir()
	r := reminder{Text: "x", Time: "10:00", Repeat: "daily", Enabled: true, Created: "2026-04-10"}
	saveReminders(vault, []reminder{r})

	raw, _ := os.ReadFile(remindersPath(vault))
	for _, want := range []string{"\"text\"", "\"time\"", "\"repeat\"", "\"enabled\"", "\"created\""} {
		if !contains(string(raw), want) {
			t.Errorf("expected JSON to contain %s, got %s", want, raw)
		}
	}
}

// helper used by the field-name test (avoids depending on strings import here)
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
