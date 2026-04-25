package tui

import "github.com/artaeon/granit/internal/modules"

// pomodoroModule declares the Pomodoro timer feature as a registry
// module. The legacy CorePlugins flag for "pomodoro" is mirrored into
// the registry at startup, so users who explicitly disabled it in
// settings.json see the same effect after this migration.
func pomodoroModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "pomodoro",
			name: "Pomodoro Timer",
			desc: "Focus timer with writing-stat tracking and per-block task queue",
			cat:  CatTasks,
			cmds: []modules.CommandRef{
				{
					ID:       "pomodoro.open",
					Label:    "Pomodoro Timer",
					Desc:     "Focus timer with writing stats",
					Category: CatTasks,
				},
				{
					ID:       "pomodoro.now",
					Label:    "Pomodoro: Now",
					Desc:     "Start a pomodoro for the currently scheduled time block",
					Category: CatTasks,
				},
			},
		},
		actions: map[string]CommandAction{
			"pomodoro.open": CmdPomodoro,
			"pomodoro.now":  CmdPomodoroNow,
		},
	}
}
