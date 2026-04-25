package tui

import "github.com/artaeon/granit/internal/modules"

// habitsModule declares the Habit Tracker feature. First module to
// own a keybind (alt+b) — the registry-driven dispatcher in
// app_update.go now resolves alt+b through this declaration instead
// of the deleted legacy switch case.
func habitsModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "habit_tracker",
			name: "Habit Tracker",
			desc: "Daily habits, goals, streaks, and progress tracking",
			cat:  CatLearning,
			cmds: []modules.CommandRef{
				{
					ID:       "habits.open",
					Label:    "Habit Tracker",
					Desc:     "Daily habits, goals, streaks, and progress tracking",
					Shortcut: "Alt+B",
					Category: CatLearning,
				},
			},
			keys: []modules.Keybind{
				{Key: "alt+b", CommandID: "habits.open"},
			},
		},
		actions: map[string]CommandAction{
			"habits.open": CmdHabitTracker,
		},
	}
}
