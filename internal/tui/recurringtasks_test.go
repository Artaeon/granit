package tui

import (
	"testing"
	"time"
)

func TestRecurringTask_IsDue_Daily(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{Text: "Meditate", Frequency: "daily", Enabled: true}
	today := time.Date(2026, 4, 9, 8, 0, 0, 0, time.Local)

	if !rt.isDue(task, today) {
		t.Error("daily task should be due")
	}
}

func TestRecurringTask_IsDue_DailyAlreadyCreated(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{
		Text: "Meditate", Frequency: "daily", Enabled: true,
		LastCreated: "2026-04-09",
	}
	today := time.Date(2026, 4, 9, 8, 0, 0, 0, time.Local)

	if rt.isDue(task, today) {
		t.Error("daily task already created today should NOT be due")
	}
}

func TestRecurringTask_IsDue_Weekly_CorrectDay(t *testing.T) {
	rt := &RecurringTasks{}
	// Wednesday = 3
	task := &RecurringTask{
		Text: "Weekly review", Frequency: "weekly", DayOfWeek: 3, Enabled: true,
	}
	wednesday := time.Date(2026, 4, 8, 8, 0, 0, 0, time.Local) // Wednesday

	if !rt.isDue(task, wednesday) {
		t.Errorf("weekly task should be due on Wednesday (weekday=%d)", wednesday.Weekday())
	}
}

func TestRecurringTask_IsDue_Weekly_WrongDay(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{
		Text: "Weekly review", Frequency: "weekly", DayOfWeek: 3, Enabled: true,
	}
	thursday := time.Date(2026, 4, 9, 8, 0, 0, 0, time.Local) // Thursday

	if rt.isDue(task, thursday) {
		t.Error("weekly task should NOT be due on Thursday when set for Wednesday")
	}
}

func TestRecurringTask_IsDue_Monthly_CorrectDay(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{
		Text: "Pay rent", Frequency: "monthly", DayOfMonth: 1, Enabled: true,
	}
	firstOfMonth := time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local)

	if !rt.isDue(task, firstOfMonth) {
		t.Error("monthly task should be due on the 1st")
	}
}

func TestRecurringTask_IsDue_Monthly_WrongDay(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{
		Text: "Pay rent", Frequency: "monthly", DayOfMonth: 1, Enabled: true,
	}
	secondOfMonth := time.Date(2026, 5, 2, 8, 0, 0, 0, time.Local)

	if rt.isDue(task, secondOfMonth) {
		t.Error("monthly task should NOT be due on the 2nd when set for 1st")
	}
}

func TestRecurringTask_IsDue_UnknownFrequency(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{
		Text: "Test", Frequency: "biweekly", Enabled: true,
	}
	today := time.Date(2026, 4, 9, 8, 0, 0, 0, time.Local)

	if rt.isDue(task, today) {
		t.Error("unknown frequency should not be due")
	}
}

func TestRecurringTask_IsDue_Disabled(t *testing.T) {
	rt := &RecurringTasks{}
	task := &RecurringTask{
		Text: "Disabled", Frequency: "daily", Enabled: false,
	}
	today := time.Date(2026, 4, 9, 8, 0, 0, 0, time.Local)

	// isDue doesn't check Enabled — that's done in checkAndCreate.
	// So isDue should still return true for disabled tasks.
	if !rt.isDue(task, today) {
		t.Error("isDue should return true regardless of Enabled flag")
	}
}
