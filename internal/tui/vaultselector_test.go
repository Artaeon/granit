package tui

import (
	"testing"
	"time"
)

func TestFormatLastOpen_Empty(t *testing.T) {
	if got := formatLastOpen(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFormatLastOpen_Today(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	got := formatLastOpen(today)
	if got != "opened today" {
		t.Errorf("expected 'opened today', got %q", got)
	}
}

func TestFormatLastOpen_Yesterday(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	got := formatLastOpen(yesterday)
	if got != "opened yesterday" {
		t.Errorf("expected 'opened yesterday', got %q", got)
	}
}

func TestFormatLastOpen_DaysAgo(t *testing.T) {
	date := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	got := formatLastOpen(date)
	if got != "opened 3 days ago" {
		t.Errorf("expected 'opened 3 days ago', got %q", got)
	}
}

func TestFormatLastOpen_InvalidDate(t *testing.T) {
	got := formatLastOpen("not-a-date")
	if got != "not-a-date" {
		t.Errorf("expected raw string for invalid date, got %q", got)
	}
}

func TestVaultSelector_NewDefaults(t *testing.T) {
	vs := NewVaultSelector()
	if vs.IsDone() {
		t.Error("new selector should not be done")
	}
	if vs.SelectedVault() != "" {
		t.Error("no vault should be selected initially")
	}
}
