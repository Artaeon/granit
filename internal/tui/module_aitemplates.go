package tui

import "github.com/artaeon/granit/internal/modules"

// aiTemplatesModule declares the AI-driven note-from-template
// generator. Distinct from the static templates feature — this one
// hands the template + topic to the AI provider and pastes the
// generated body.
func aiTemplatesModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "ai_templates",
			name: "AI Templates",
			desc: "Generate a full note from a template type and topic via AI",
			cat:  CatAI,
			cmds: []modules.CommandRef{
				{
					ID:       "ai_templates.open",
					Label:    "AI Template",
					Desc:     "Generate a full note from a template type + topic with AI",
					Category: CatAI,
				},
			},
		},
		actions: map[string]CommandAction{
			"ai_templates.open": CmdAITemplate,
		},
	}
}
