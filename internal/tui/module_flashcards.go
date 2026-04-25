package tui

import "github.com/artaeon/granit/internal/modules"

// flashcardsModule declares the spaced-repetition flashcards feature.
// Self-contained: no dependencies, single command, no keybind.
// Quiz mode (next pilot) declares a DependsOn this module.
func flashcardsModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "flashcards",
			name: "Flashcards",
			desc: "Spaced-repetition study cards mined from your notes",
			cat:  CatLearning,
			cmds: []modules.CommandRef{
				{
					ID:       "flashcards.open",
					Label:    "Flashcards",
					Desc:     "Spaced repetition study from your notes",
					Category: CatLearning,
				},
			},
		},
		actions: map[string]CommandAction{
			"flashcards.open": CmdFlashcards,
		},
	}
}
