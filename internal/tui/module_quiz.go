package tui

import "github.com/artaeon/granit/internal/modules"

// quizModule declares the auto-generated quiz feature. Depends on
// flashcards because the quiz reuses the same card extraction
// pipeline; disabling flashcards would leave the quiz with nothing
// to draw from.
//
// This pilot is also where the dependency-resolution path gets
// exercised end-to-end: trying to disable flashcards while quiz is
// enabled now fails with a "module X depends on it" error, and
// re-enabling quiz after disabling flashcards is refused for the
// same reason.
func quizModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "quiz_mode",
			name: "Quiz Mode",
			desc: "Auto-generated quizzes from your flashcard pool",
			cat:  CatLearning,
			deps: []string{"flashcards"},
			cmds: []modules.CommandRef{
				{
					ID:       "quiz.open",
					Label:    "Quiz Mode",
					Desc:     "Test your knowledge with auto-generated quizzes",
					Category: CatLearning,
				},
			},
		},
		actions: map[string]CommandAction{
			"quiz.open": CmdQuizMode,
		},
	}
}
