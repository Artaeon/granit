package tui

import (
	"errors"
	"testing"
)

// TestConsumeSaveErrorContract pins the shared behaviour of every type
// that gained a ConsumeSaveError method in the error-surfacing pass:
// reading it returns the stored error, clears the slot, and the next
// read returns nil. A nil receiver is a silent no-op so the host Model
// can call it defensively. Covers both overlays and plain stores
// (EventStore) that persist user data.
func TestConsumeSaveErrorContract(t *testing.T) {
	testErr := errors.New("disk full")

	cases := []struct {
		name      string
		setErr    func() (consume func() error, nilConsume func() error)
	}{
		{
			name: "HabitTracker",
			setErr: func() (func() error, func() error) {
				h := HabitTracker{lastSaveErr: testErr}
				var nilH *HabitTracker
				return h.ConsumeSaveError, nilH.ConsumeSaveError
			},
		},
		{
			name: "DailyPlanner",
			setErr: func() (func() error, func() error) {
				dp := DailyPlanner{lastSaveErr: testErr}
				var nilDP *DailyPlanner
				return dp.ConsumeSaveError, nilDP.ConsumeSaveError
			},
		},
		{
			name: "TaskManager",
			setErr: func() (func() error, func() error) {
				tm := TaskManager{lastSaveErr: testErr}
				var nilTM *TaskManager
				return tm.ConsumeSaveError, nilTM.ConsumeSaveError
			},
		},
		{
			name: "ClockIn",
			setErr: func() (func() error, func() error) {
				c := ClockIn{lastSaveErr: testErr}
				var nilC *ClockIn
				return c.ConsumeSaveError, nilC.ConsumeSaveError
			},
		},
		{
			name: "Pomodoro",
			setErr: func() (func() error, func() error) {
				p := Pomodoro{lastSaveErr: testErr}
				var nilP *Pomodoro
				return p.ConsumeSaveError, nilP.ConsumeSaveError
			},
		},
		{
			name: "FocusSession",
			setErr: func() (func() error, func() error) {
				fs := FocusSession{lastSaveErr: testErr}
				var nilFS *FocusSession
				return fs.ConsumeSaveError, nilFS.ConsumeSaveError
			},
		},
		{
			name: "DailyReview",
			setErr: func() (func() error, func() error) {
				dr := DailyReview{lastSaveErr: testErr}
				var nilDR *DailyReview
				return dr.ConsumeSaveError, nilDR.ConsumeSaveError
			},
		},
		{
			name: "WeeklyReview",
			setErr: func() (func() error, func() error) {
				wr := WeeklyReview{lastSaveErr: testErr}
				var nilWR *WeeklyReview
				return wr.ConsumeSaveError, nilWR.ConsumeSaveError
			},
		},
		{
			name: "LanguageLearning",
			setErr: func() (func() error, func() error) {
				ll := LanguageLearning{lastSaveErr: testErr}
				var nilLL *LanguageLearning
				return ll.ConsumeSaveError, nilLL.ConsumeSaveError
			},
		},
		{
			name: "EventStore",
			setErr: func() (func() error, func() error) {
				es := EventStore{lastSaveErr: testErr}
				var nilES *EventStore
				return es.ConsumeSaveError, nilES.ConsumeSaveError
			},
		},
		{
			name: "Bookmarks",
			setErr: func() (func() error, func() error) {
				bm := Bookmarks{lastSaveErr: testErr}
				var nilBM *Bookmarks
				return bm.ConsumeSaveError, nilBM.ConsumeSaveError
			},
		},
		{
			name: "Kanban",
			setErr: func() (func() error, func() error) {
				kb := Kanban{lastSaveErr: testErr}
				var nilKB *Kanban
				return kb.ConsumeSaveError, nilKB.ConsumeSaveError
			},
		},
		{
			name: "IdeasBoard",
			setErr: func() (func() error, func() error) {
				ib := IdeasBoard{lastSaveErr: testErr}
				var nilIB *IdeasBoard
				return ib.ConsumeSaveError, nilIB.ConsumeSaveError
			},
		},
		{
			name: "ReadingList",
			setErr: func() (func() error, func() error) {
				rl := ReadingList{lastSaveErr: testErr}
				var nilRL *ReadingList
				return rl.ConsumeSaveError, nilRL.ConsumeSaveError
			},
		},
		{
			name: "Scratchpad",
			setErr: func() (func() error, func() error) {
				sp := Scratchpad{lastSaveErr: testErr}
				var nilSP *Scratchpad
				return sp.ConsumeSaveError, nilSP.ConsumeSaveError
			},
		},
		{
			name: "RecurringTasks",
			setErr: func() (func() error, func() error) {
				rt := RecurringTasks{lastSaveErr: testErr}
				var nilRT *RecurringTasks
				return rt.ConsumeSaveError, nilRT.ConsumeSaveError
			},
		},
		{
			name: "TimeTracker",
			setErr: func() (func() error, func() error) {
				tt := TimeTracker{lastSaveErr: testErr}
				var nilTT *TimeTracker
				return tt.ConsumeSaveError, nilTT.ConsumeSaveError
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			consume, nilConsume := tc.setErr()

			// First call returns the stored error.
			if got := consume(); !errors.Is(got, testErr) {
				t.Errorf("first consume: want %v, got %v", testErr, got)
			}
			// Second call returns nil — the error was consumed once.
			if got := consume(); got != nil {
				t.Errorf("second consume: want nil, got %v", got)
			}
			// Nil receiver is a silent no-op so hosts can call defensively
			// before the overlay is initialised.
			if got := nilConsume(); got != nil {
				t.Errorf("nil consume: want nil, got %v", got)
			}
		})
	}
}
