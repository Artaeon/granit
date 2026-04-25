package tui

import "github.com/artaeon/granit/internal/modules"

// researchAgentModule declares the Claude-Code-backed deep-research
// feature. Two command surfaces: open from scratch (any topic) and
// follow-up on the current note. The follow-up command is gated by
// the same module flag in app_commands.go even though the legacy
// CorePluginEnabled check only covered the entry point — registry
// migration unifies both surfaces under the single research_agent
// toggle.
func researchAgentModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "research_agent",
			name: "Research Agent",
			desc: "AI research agent that drafts notes from any topic via Claude Code",
			cat:  CatAI,
			cmds: []modules.CommandRef{
				{
					ID:       "research_agent.open",
					Label:    "Deep Dive Research",
					Desc:     "AI research agent — create notes from any topic via Claude Code",
					Category: CatAI,
				},
				{
					ID:       "research_agent.followup",
					Label:    "Research Follow-Up",
					Desc:     "Go deeper on current note's topic via Claude Code",
					Category: CatAI,
				},
			},
		},
		actions: map[string]CommandAction{
			"research_agent.open":     CmdResearchAgent,
			"research_agent.followup": CmdResearchFollowUp,
		},
	}
}
