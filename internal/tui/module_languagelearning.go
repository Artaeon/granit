package tui

import "github.com/artaeon/granit/internal/modules"

// languageLearningModule declares the vocabulary-tracker / grammar /
// practice-session feature. Specialized enough that disabling it for
// users who don't study a second language meaningfully cleans up the
// palette — a good case for the module-toggle UX the relaunch is
// building toward.
func languageLearningModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "language_learning",
			name: "Language Learning",
			desc: "Vocabulary tracker, practice sessions, and grammar notes",
			cat:  CatLearning,
			cmds: []modules.CommandRef{
				{
					ID:       "language_learning.open",
					Label:    "Language Learning",
					Desc:     "Vocabulary tracker, practice sessions, grammar notes",
					Category: CatLearning,
				},
			},
		},
		actions: map[string]CommandAction{
			"language_learning.open": CmdLanguageLearning,
		},
	}
}
