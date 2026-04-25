package tui

import "github.com/artaeon/granit/internal/modules"

// blogPublisherModule declares the Medium/GitHub blog publishing
// feature. No keybind, no dependencies, single command. Settings
// (medium token, github token/repo/branch) stay on the global
// Config struct for now — module-owned settings ship in a later
// phase when we move per-module config out of cfg.
func blogPublisherModule() builtinRegistration {
	return builtinRegistration{
		mod: &builtinModule{
			id:   "blog_publisher",
			name: "Blog Publisher",
			desc: "Publish notes to Medium or a GitHub blog",
			cat:  CatPublish,
			cmds: []modules.CommandRef{
				{
					ID:       "blog_publisher.publish",
					Label:    "Publish to Blog",
					Desc:     "Publish note to Medium or GitHub blog",
					Category: CatPublish,
				},
			},
		},
		actions: map[string]CommandAction{
			"blog_publisher.publish": CmdBlogPublish,
		},
	}
}
