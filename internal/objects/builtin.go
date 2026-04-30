package objects

// builtinTypes returns the starter type set shipped with granit. These
// are the five most common PKM object kinds and serve two purposes:
//
//  1. Out-of-the-box experience — a fresh vault gets useful gallery
//     views for People / Books / Projects / Meetings / Ideas without
//     the user having to design schemas from scratch.
//
//  2. Reference for custom types — when users write their own
//     `.granit/types/<id>.json` overrides, they can copy and adapt
//     these as a starting point.
//
// The Properties slice on each type is INTENTIONALLY short. Capacities
// and Notion both ship with property-laden defaults that overwhelm
// new users; granit instead picks 4-6 fields that cover ~80% of use
// cases, and the user adds more as needed (vault-local overrides win).
//
// Per-vault overrides at `.granit/types/<id>.json` REPLACE the
// built-in entirely — they don't merge. Otherwise a user's custom
// person type with `email`, `slack` properties would surprise them by
// also showing the built-in `email`, `phone`, `role` columns. Full
// override is the simpler mental model.
func builtinTypes() []Type {
	return []Type{
		{
			ID:              "person",
			Name:            "Person",
			Description:     "Someone you know — friend, colleague, contact",
			Icon:            "👤",
			Folder:          "People",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "name", Kind: KindText, Required: true,
					Description: "Full name as it appears in the heading"},
				{Name: "email", Kind: KindURL,
					Description: "Primary contact email"},
				{Name: "phone", Kind: KindText,
					Description: "Phone number (any format)"},
				{Name: "role", Kind: KindText,
					Description: "Job title, relationship, or role"},
				{Name: "last_contact", Kind: KindDate,
					Description: "Date of most recent meaningful contact"},
				{Name: "tags", Kind: KindTag,
					Description: "Free-form tags, no leading #"},
			},
		},
		{
			ID:              "book",
			Name:            "Book",
			Description:     "A book on your reading list (active or done)",
			Icon:            "📚",
			Folder:          "Books",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true},
				{Name: "author", Kind: KindText, Required: true},
				{Name: "status", Kind: KindSelect,
					Options: []string{"to-read", "reading", "read", "abandoned"},
					Default: "to-read"},
				{Name: "rating", Kind: KindNumber,
					Description: "Personal rating 1–5"},
				{Name: "started", Kind: KindDate},
				{Name: "finished", Kind: KindDate},
				{Name: "url", Kind: KindURL,
					Description: "Goodreads / publisher / Amazon link"},
			},
		},
		{
			ID:              "project",
			Name:            "Project",
			Description:     "A multi-task initiative with a goal and deadline",
			Icon:            "🎯",
			Folder:          "Projects",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "name", Kind: KindText, Required: true},
				{Name: "status", Kind: KindSelect,
					Options: []string{"backlog", "active", "paused", "shipped", "abandoned"},
					Default: "backlog"},
				{Name: "owner", Kind: KindLink,
					Description: "Wikilink to the responsible person"},
				{Name: "deadline", Kind: KindDate},
				{Name: "started", Kind: KindDate},
				{Name: "repo", Kind: KindText,
					Description: "Local path to the project's git repo (e.g. /home/me/Projects/foo). Hub strip + Repo Tracker pull live status from here."},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			// `goal` is a typed-object equivalent of the legacy GoalsMode
			// store. The two coexist intentionally: GoalsMode keeps
			// working for users with existing goal data; new goals can
			// be created as typed-object notes (Alt+O → 'n' → goal) and
			// participate in saved views, agents, and the dashboard.
			// Status enum mirrors GoalStatus values so a future
			// migration can map cleanly.
			ID:              "goal",
			Name:            "Goal",
			Description:     "A measurable outcome with a target date and status",
			Icon:            "🏁",
			Folder:          "Goals",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true},
				{Name: "status", Kind: KindSelect,
					Options: []string{"active", "completed", "paused", "archived"},
					Default: "active"},
				{Name: "target_date", Kind: KindDate,
					Description: "When you want to be done"},
				{Name: "priority", Kind: KindSelect,
					Options: []string{"low", "medium", "high"},
					Default: "medium"},
				{Name: "why", Kind: KindText,
					Description: "Motivation — refer back to it on hard days"},
				{Name: "started", Kind: KindDate, Default: "{today}"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "meeting",
			Name:            "Meeting",
			Description:     "Notes from a meeting, call, or 1:1",
			Icon:            "🗣️",
			Folder:          "Meetings",
			FilenamePattern: "{date} - {title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true,
					Description: "Topic / context (e.g. 'Q3 planning sync')"},
				{Name: "date", Kind: KindDate, Required: true,
					Default: "{today}"},
				{Name: "attendees", Kind: KindText,
					Description: "Comma-separated names or wikilinks"},
				{Name: "type", Kind: KindSelect,
					Options: []string{"1:1", "team", "external", "interview", "review"}},
				{Name: "follow_up", Kind: KindCheckbox,
					Description: "Has at least one follow-up action"},
			},
		},
		{
			ID:              "idea",
			Name:            "Idea",
			Description:     "A nascent concept — pre-project, pre-decision",
			Icon:            "💡",
			Folder:          "Ideas",
			FilenamePattern: "{date} - {title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true},
				{Name: "captured", Kind: KindDate,
					Default: "{today}"},
				{Name: "score", Kind: KindNumber,
					Description: "ICE / RICE / personal weight 1–10"},
				{Name: "status", Kind: KindSelect,
					Options: []string{"raw", "refining", "scheduled", "rejected"},
					Default: "raw"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "article",
			Name:            "Article",
			Description:     "Saved web article — read later, highlighted, or summarised",
			Icon:            "📰",
			Folder:          "Articles",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true},
				{Name: "url", Kind: KindURL, Required: true,
					Description: "Source URL (clickable in the gallery)"},
				{Name: "author", Kind: KindText},
				{Name: "publication", Kind: KindText,
					Description: "Site or publication name"},
				{Name: "saved", Kind: KindDate, Default: "{today}"},
				{Name: "status", Kind: KindSelect,
					Options: []string{"to-read", "reading", "read", "archived"},
					Default: "to-read"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "podcast",
			Name:            "Podcast Episode",
			Description:     "Podcast episode — show, host, key takeaways",
			Icon:            "🎙️",
			Folder:          "Podcasts",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true,
					Description: "Episode title"},
				{Name: "show", Kind: KindText, Required: true,
					Description: "Podcast / show name"},
				{Name: "host", Kind: KindText},
				{Name: "url", Kind: KindURL,
					Description: "Episode link"},
				{Name: "duration", Kind: KindText,
					Description: "Length, e.g. '1h 23m'"},
				{Name: "listened", Kind: KindDate},
				{Name: "rating", Kind: KindNumber,
					Description: "Personal rating 1-5"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "video",
			Name:            "Video",
			Description:     "YouTube / Vimeo / lecture video with timestamped notes",
			Icon:            "📺",
			Folder:          "Videos",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true},
				{Name: "channel", Kind: KindText,
					Description: "Channel / creator"},
				{Name: "url", Kind: KindURL, Required: true},
				{Name: "duration", Kind: KindText,
					Description: "Length, e.g. '32:18'"},
				{Name: "watched", Kind: KindDate},
				{Name: "rating", Kind: KindNumber,
					Description: "Personal rating 1-5"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "quote",
			Name:            "Quote",
			Description:     "A pithy quote with attribution and context",
			Icon:            "💬",
			Folder:          "Quotes",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true,
					Description: "Short identifier — first few words"},
				{Name: "author", Kind: KindText, Required: true,
					Description: "Who said / wrote it"},
				{Name: "source", Kind: KindText,
					Description: "Book / article / talk it came from"},
				{Name: "captured", Kind: KindDate, Default: "{today}"},
				{Name: "context", Kind: KindText,
					Description: "Why this matters to you"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "place",
			Name:            "Place",
			Description:     "A location — venue, city, restaurant, landmark",
			Icon:            "📍",
			Folder:          "Places",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "name", Kind: KindText, Required: true},
				{Name: "kind", Kind: KindSelect,
					Options: []string{"city", "neighbourhood", "venue", "restaurant", "landmark", "trail", "other"},
					Default: "venue"},
				{Name: "city", Kind: KindText,
					Description: "Containing city or region"},
				{Name: "country", Kind: KindText},
				{Name: "url", Kind: KindURL,
					Description: "Maps link or website"},
				{Name: "visited", Kind: KindDate},
				{Name: "rating", Kind: KindNumber,
					Description: "Personal rating 1-5"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "recipe",
			Name:            "Recipe",
			Description:     "A cooking recipe with ingredients and method",
			Icon:            "🍳",
			Folder:          "Recipes",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true},
				{Name: "cuisine", Kind: KindText,
					Description: "Italian, Thai, etc."},
				{Name: "course", Kind: KindSelect,
					Options: []string{"breakfast", "lunch", "dinner", "side", "dessert", "snack", "drink"}},
				{Name: "servings", Kind: KindNumber},
				{Name: "prep_time", Kind: KindText,
					Description: "e.g. '20 min'"},
				{Name: "cook_time", Kind: KindText,
					Description: "e.g. '45 min'"},
				{Name: "rating", Kind: KindNumber,
					Description: "Personal rating 1-5"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			// agent_run captures a completed multi-step agent invocation.
			// Each run writes one of these to Agents/<timestamp>-<preset>.md
			// so the user can browse, search, and re-open past agent runs
			// — same pattern as Deepnote notebook history. The body holds
			// the formatted transcript (Thought/Action/Observation lines).
			ID:              "agent_run",
			Name:            "Agent Run",
			Description:     "A completed multi-step AI agent invocation with transcript",
			Icon:            "🤖",
			Folder:          "Agents",
			FilenamePattern: "{title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true,
					Description: "Run title (preset + truncated goal)"},
				{Name: "preset", Kind: KindText, Required: true,
					Description: "Agent preset ID used for this run"},
				{Name: "model", Kind: KindText,
					Description: "Model name the run used (e.g. gpt-4o-mini)"},
				{Name: "goal", Kind: KindText, Required: true,
					Description: "User-entered goal for this run"},
				{Name: "status", Kind: KindSelect,
					Options: []string{"ok", "budget", "error", "cancelled"},
					Default: "ok"},
				{Name: "started", Kind: KindDate, Default: "{today}"},
				{Name: "steps", Kind: KindNumber,
					Description: "How many ReAct iterations the run took"},
				{Name: "tags", Kind: KindTag},
			},
		},
		{
			ID:              "highlight",
			Name:            "Highlight",
			Description:     "A passage you want to remember — from a book, article, or conversation",
			Icon:            "🔖",
			Folder:          "Highlights",
			FilenamePattern: "{date} - {title}",
			Properties: []Property{
				{Name: "title", Kind: KindText, Required: true,
					Description: "Short identifier"},
				{Name: "source", Kind: KindLink,
					Description: "Wikilink to the source note (book, article, etc.)"},
				{Name: "captured", Kind: KindDate, Default: "{today}"},
				{Name: "page", Kind: KindText,
					Description: "Page / location / timestamp"},
				{Name: "tags", Kind: KindTag},
			},
		},
	}
}
