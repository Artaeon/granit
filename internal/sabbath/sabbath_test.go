package sabbath

import (
	"testing"
	"time"
)

func TestActiveAt_ManualFlag(t *testing.T) {
	loc := time.UTC
	at := time.Date(2026, 5, 16, 14, 0, 0, 0, loc) // Sat 14:00
	s := State{ActiveOn: "2026-05-16", Schedule: DefaultSchedule()}
	if !s.IsActiveAt(at) {
		t.Fatal("manual flag matching today should be active")
	}
	s.ActiveOn = "2026-05-15"
	if s.IsActiveAt(at) {
		t.Fatal("manual flag for yesterday should not be active today")
	}
}

func TestActiveAt_ScheduleMidnightToMidnight(t *testing.T) {
	loc := time.UTC
	sched := Schedule{Enabled: true, DayOfWeek: int(time.Saturday), StartHour: 0, StartMinute: 0, DurationMinutes: 1440}
	s := State{Schedule: sched}
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"sat 00:00", time.Date(2026, 5, 16, 0, 0, 0, 0, loc), true},
		{"sat 12:00", time.Date(2026, 5, 16, 12, 0, 0, 0, loc), true},
		{"sat 23:59", time.Date(2026, 5, 16, 23, 59, 0, 0, loc), true},
		{"sun 00:00", time.Date(2026, 5, 17, 0, 0, 0, 0, loc), false},
		{"fri 23:59", time.Date(2026, 5, 15, 23, 59, 0, 0, loc), false},
	}
	for _, c := range cases {
		if got := s.IsActiveAt(c.at); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestActiveAt_ScheduleSundownToSundown(t *testing.T) {
	loc := time.UTC
	// Friday 18:00 → Saturday 18:00.
	sched := Schedule{Enabled: true, DayOfWeek: int(time.Friday), StartHour: 18, StartMinute: 0, DurationMinutes: 1440}
	s := State{Schedule: sched}
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"fri 17:59", time.Date(2026, 5, 15, 17, 59, 0, 0, loc), false},
		{"fri 18:00", time.Date(2026, 5, 15, 18, 0, 0, 0, loc), true},
		{"fri 23:30", time.Date(2026, 5, 15, 23, 30, 0, 0, loc), true},
		{"sat 00:30", time.Date(2026, 5, 16, 0, 30, 0, 0, loc), true},
		{"sat 17:59", time.Date(2026, 5, 16, 17, 59, 0, 0, loc), true},
		{"sat 18:00", time.Date(2026, 5, 16, 18, 0, 0, 0, loc), false},
		{"sun 12:00", time.Date(2026, 5, 17, 12, 0, 0, 0, loc), false},
	}
	for _, c := range cases {
		if got := s.IsActiveAt(c.at); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestActiveAt_ScheduleDisabled(t *testing.T) {
	s := State{Schedule: Schedule{Enabled: false, DayOfWeek: int(time.Saturday), StartHour: 0, DurationMinutes: 1440}}
	at := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	if s.IsActiveAt(at) {
		t.Fatal("disabled schedule should not activate even on the chosen day")
	}
}

func TestLoad_PreScheduleSidecar(t *testing.T) {
	// Older sidecars only had {active_on}. Loading one should
	// produce DefaultSchedule with Enabled=false so the missing
	// field doesn't accidentally turn sabbath on.
	dir := t.TempDir()
	if err := Save(dir, State{ActiveOn: "2026-05-16"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schedule.Enabled {
		t.Fatal("default schedule should be disabled")
	}
	if loaded.Schedule.DurationMinutes != 1440 {
		t.Fatalf("default duration should be 1440, got %d", loaded.Schedule.DurationMinutes)
	}
}
