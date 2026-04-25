package tui

import "github.com/artaeon/granit/internal/modules"

// taskManagerModule declares the cross-vault Task Manager. Owns
// ctrl+k — routing it through the registry (instead of the legacy
// switch case in app_update.go) means the gate check actually fires
// for keyboard activation, and the project/time-tracker enrichment
// the CmdTaskManager case does after Open() now runs in both code
// paths instead of only via the palette.
//
// Future migrations should fold task_triage, recurring_tasks, and
// kanban into this module's settings rather than spinning them off
// as separate modules — per the relaunch decision, Tasks is one
// module customized via its own settings.
func taskManagerModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "task_manager",
			name: "Task Manager",
			desc: "Cross-vault task triage, scheduling, and execution surface",
			cat:  CatTasks,
			cmds: []modules.CommandRef{
				{
					ID:       "task_manager.open",
					Label:    "Task Manager",
					Desc:     "View, manage, and plan all tasks across vault",
					Shortcut: "Ctrl+K",
					Category: CatTasks,
				},
			},
			keys: []modules.Keybind{
				{Key: "ctrl+k", CommandID: "task_manager.open"},
			},
		},
		actions: map[string]CommandAction{
			"task_manager.open": CmdTaskManager,
		},
	}
}
