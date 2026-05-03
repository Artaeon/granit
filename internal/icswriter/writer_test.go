package icswriter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildRRULE_Cases covers the FREQ/INTERVAL/COUNT/UNTIL/BYDAY
// combinations the create-event form emits. Each case asserts the
// canonical-order serialization that BuildRRULE promises.
func TestBuildRRULE_Cases(t *testing.T) {
	tests := []struct {
		name string
		in   RRULEOptions
		want string
	}{
		{"empty", RRULEOptions{}, ""},
		{"daily", RRULEOptions{Freq: "DAILY"}, "FREQ=DAILY"},
		{"daily-interval", RRULEOptions{Freq: "DAILY", Interval: 2}, "FREQ=DAILY;INTERVAL=2"},
		{"daily-count", RRULEOptions{Freq: "DAILY", Count: 5}, "FREQ=DAILY;COUNT=5"},
		{
			"weekly-byday",
			RRULEOptions{Freq: "WEEKLY", ByDay: []string{"mo", "we", "fr"}},
			"FREQ=WEEKLY;BYDAY=FR,MO,WE",
		},
		{
			"weekly-interval-until",
			RRULEOptions{
				Freq:     "WEEKLY",
				Interval: 2,
				Until:    time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
				ByDay:    []string{"TU", "TH"},
			},
			"FREQ=WEEKLY;INTERVAL=2;UNTIL=20261231T000000Z;BYDAY=TH,TU",
		},
		{
			"count-wins-over-until",
			RRULEOptions{
				Freq:  "MONTHLY",
				Count: 3,
				Until: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			},
			"FREQ=MONTHLY;COUNT=3",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildRRULE(tc.in)
			if got != tc.want {
				t.Fatalf("BuildRRULE(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeText(t *testing.T) {
	in := `He said, "hi"; line two
new line\back`
	got := EscapeText(in)
	want := `He said\, "hi"\; line two\nnew line\\back`
	if got != want {
		t.Fatalf("EscapeText:\n got: %q\nwant: %q", got, want)
	}
}

// TestWriteFile_LineFolding verifies long lines get the CRLF+space
// continuation that 5545 §3.1 mandates. We fabricate a >75-byte
// summary and confirm the on-disk file contains the fold.
func TestWriteFile_LineFolding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fold.ics")
	long := strings.Repeat("x", 200)
	err := WriteFile(path, CalendarMeta{Name: "fold"}, []Event{{
		UID:     "fold-1",
		Summary: long,
		Start:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "\r\n ") {
		t.Errorf("expected continuation line (CRLF+space) in folded output")
	}
}

// TestWriteFile_AllLinesEndCRLF asserts every line in the produced file
// ends with CRLF — the canonical 5545 line terminator. A bare LF will
// trip strict parsers (looking at you, iOS Calendar).
func TestWriteFile_AllLinesEndCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf.ics")
	if err := WriteFile(path, CalendarMeta{Name: "crlf"}, []Event{{
		UID:     "crlf-1",
		Summary: "Hi",
		Start:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	// Every LF in the file should be preceded by CR.
	s := string(raw)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			t.Fatalf("bare LF at offset %d", i)
		}
	}
}

// TestWriteFile_ContainsHeader asserts the VCALENDAR header carries
// PRODID + the friendly NAME / X-WR-CALNAME we promise.
func TestWriteFile_ContainsHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hdr.ics")
	meta := CalendarMeta{
		ProdID:      "-//test//1.0//EN",
		Name:        "Faith",
		DisplayName: "Faith Calendar",
	}
	if err := WriteFile(path, meta, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	s := string(raw)
	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//test//1.0//EN",
		"CALSCALE:GREGORIAN",
		"NAME:Faith",
		"X-WR-CALNAME:Faith Calendar",
		"END:VCALENDAR",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected header to contain %q\n--- got ---\n%s", want, s)
		}
	}
}
