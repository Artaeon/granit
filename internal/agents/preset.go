package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Preset is a serialisable agent definition that bundles persona,
// tool selection, and write-access opt-in. Built-in presets are
// declared in code (defaultPresets); vault-local overrides live at
// `<vault>/.granit/agents/<id>.json` and replace built-ins by ID.
//
// The JSON shape is hand-editable on purpose — users should be able
// to write a custom agent without touching Go. Field names match
// what a user would expect from reading docs/AGENTS.md.
type Preset struct {
	// ID is the stable handle used for vault-local override
	// filenames and command-palette routing. Lower-snake case.
	ID string `json:"id"`

	// Name is the human-friendly label shown in the runner's
	// preset picker.
	Name string `json:"name"`

	// Description is the one-line summary under the name.
	Description string `json:"description"`

	// SystemPrompt is the persistent persona block prepended to
	// every iteration. Empty falls through to the built-in
	// generic helper preamble.
	SystemPrompt string `json:"systemPrompt"`

	// Tools is the explicit allow-list of tool names this preset
	// can use. Empty means "every read tool" — the safe default.
	// Listing tools also drives what the LLM sees in the system
	// prompt, so a preset that only needs search_vault doesn't
	// distract the model with a 9-tool catalog.
	Tools []string `json:"tools,omitempty"`

	// IncludeWrite, when true, registers the package's three
	// write tools (write_note, create_task, create_object)
	// alongside the Tools allow-list. Only takes effect when the
	// runtime caller has supplied an Approve callback — without
	// that, agent construction fails the safety gate.
	IncludeWrite bool `json:"includeWrite,omitempty"`

	// MaxSteps overrides the runtime's default step budget for
	// this preset. Useful when an agent's expected work is large
	// enough that the default 8-step cap is too tight (research
	// synthesis across 20 notes), or small enough that 8 is
	// wasteful (single-shot triage). Zero falls through to the
	// runtime default.
	MaxSteps int `json:"maxSteps,omitempty"`

	// Model overrides the AI model used for THIS preset's runs,
	// independent of the user's global Settings choice. Pattern:
	// fast cheap models for simple multi-step routing (Inbox
	// Triager picks a tag — qwen2.5:0.5b is fine), bigger smarter
	// models for synthesis (Research Synthesizer benefits from
	// llama3.1:8b or gpt-4o-mini). Empty falls through to the
	// global model. Provider is NEVER overridden — the preset
	// rides the user's configured provider, just with a
	// different model name on it.
	Model string `json:"model,omitempty"`
}

// Validate reports a clear error when a Preset is missing fields
// that the runtime requires. Empty Tools is valid (means "all read
// tools"); empty SystemPrompt is valid (means "use generic
// preamble"). The hard requirements are an ID + Name + Description
// — those are user-facing labels that have no sensible fallback.
func (p Preset) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("preset ID is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("preset %q: Name is required", p.ID)
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("preset %q: Description is required", p.ID)
	}
	for _, name := range p.Tools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("preset %q: empty tool name in Tools list", p.ID)
		}
	}
	return nil
}

// PresetCatalog is the merged view of built-in + vault-local
// presets. Built-ins ship in code (callers register them at
// startup); vault-local overrides at `.granit/agents/<id>.json`
// REPLACE the built-in with the same ID rather than merging.
//
// Same rationale as the Type registry's full-override semantics:
// merge semantics on user-edited JSON make for surprising
// behaviour ("which fields win?"), full override is the simpler
// mental model.
type PresetCatalog struct {
	presets map[string]Preset
}

// BuiltinPresets returns the list of presets shipped with granit. The
// TUI's AgentRunner and the web's /agents page both consume this so a
// new preset only has to be declared once. Vault-local overrides at
// .granit/agents/<id>.json take precedence — see PresetCatalog.LoadVaultDir.
//
// Each preset is a starter for a common PKM workflow: research synthesis,
// project review, inbox triage. Add more by appending here; the JSON-on-
// disk override path remains the user's escape hatch.
func BuiltinPresets() []Preset {
	return []Preset{
		{
			ID:          "research-synthesizer",
			Name:        "Research Synthesizer",
			Description: "Given a topic, finds related notes and summarises patterns + open questions.",
			SystemPrompt: "You are a careful research synthesiser. The user gives you a topic. " +
				"Use search_vault and read_note to gather related notes from the vault, " +
				"then synthesise a structured answer covering: (1) what the notes say, " +
				"(2) recurring themes, (3) open questions or contradictions. " +
				"Do not invent facts not present in the notes. Cite note paths when claiming something.",
			Tools: []string{"search_vault", "read_note", "list_notes", "get_today"},
		},
		{
			ID:          "project-manager",
			Name:        "Project Manager",
			Description: "Reviews a project: status, blockers, related tasks, recent activity.",
			SystemPrompt: "You are a project manager assistant. The user names a project (typically by " +
				"its title or a substring). Find the matching project object via query_objects with " +
				"type=project; if there are multiple matches, ask the user which one to review (Final " +
				"Answer with a list). Otherwise: read the project note, find its open tasks via " +
				"query_tasks, and produce a structured report covering (1) current status, " +
				"(2) blockers / waiting-on, (3) next concrete actions. Cite tasks and note paths. " +
				"Do not invent dates — call get_today first if you need to reason about overdue.",
			Tools: []string{"query_objects", "read_note", "query_tasks", "search_vault", "get_today"},
		},
		{
			ID:          "devotional",
			Name:        "Devotional Reflection",
			Description: "Reads a verse, writes a 200-300 word reflection grounded in the text and connected to today's life.",
			SystemPrompt: "You are a thoughtful devotional writer. The user gives you a verse and its citation. " +
				"Steps:\n" +
				"1. Call get_today to anchor when 'today' is.\n" +
				"2. Optionally call read_note on Jots/{today}.md to glimpse what's on the user's mind today (skip if it doesn't exist or is empty).\n" +
				"3. Write a focused 200–300 word reflection on the verse. Structure:\n" +
				"   - One sentence framing what the verse is saying in plain language.\n" +
				"   - One paragraph drawing out the main spiritual or practical insight, grounded IN the text — quote or paraphrase the verse, don't drift into generic platitudes.\n" +
				"   - One paragraph connecting it to a specific posture, choice, or attention the user could hold today. Concrete, not abstract.\n" +
				"   - One closing sentence — a question or prayer, not a summary.\n" +
				"4. Write your reflection via write_note to Devotionals/{today}-{slug}.md, where {slug} is the citation lowercased with non-alphanumerics replaced by hyphens (e.g. 'proverbs-3-5-6'). The note should be valid markdown with frontmatter (type: devotional, date: today, source: \"the citation\", tags: [devotional]) followed by an H1 of the citation, the verse as a blockquote, and ## Reflection containing your text.\n" +
				"5. Final answer: a 1-sentence summary of the reflection's core insight + the path you wrote.\n" +
				"Do not invent details about the user's life. Stay tight on the verse.",
			Tools:        []string{"get_today", "read_note", "write_note"},
			IncludeWrite: true,
		},
		{
			ID:          "plan-my-day",
			Name:        "Plan my day",
			Description: "Reads today's calendar, open tasks, and project next-actions; writes a time-blocked schedule to today's daily note.",
			SystemPrompt: "You build a focused day plan. Steps:\n" +
				"1. Call get_today to anchor the date.\n" +
				"2. Call query_tasks with status=open to see what's pending. Prioritise: P1 (high) first, items due today or overdue next, then quick wins (≤30 min) you can knock out between meetings.\n" +
				"3. Call query_objects with type=project to see active projects; their next_action field is the user's chosen next concrete step.\n" +
				"4. Read today's daily note (Jots/{today}.md) with read_note to see any plans the user already wrote — DO NOT clobber them.\n" +
				"5. Produce a time-blocked schedule between 09:00 and 18:00 with these rules:\n" +
				"   - 25–60 min focus blocks separated by 5–10 min breaks\n" +
				"   - Lunch 12:30–13:15\n" +
				"   - At most ONE 90-minute deep-work block per morning\n" +
				"   - Don't double-book existing calendar events (assume they're already in the daily note)\n" +
				"6. Write the plan via write_note to Jots/{today}.md, APPENDING a section titled '## Plan' (preserve any existing content above). Format each block as:\n" +
				"   - HH:MM–HH:MM — task or focus theme\n" +
				"7. Final answer: brief 2-3 sentence summary of the plan + which P1 you picked first.\n" +
				"Cite task IDs or note paths when you reference them so the user can drill in.",
			Tools:        []string{"get_today", "query_tasks", "query_objects", "read_note", "write_note"},
			IncludeWrite: true,
		},
		{
			ID:          "summarize-day",
			Name:        "Summarize today",
			Description: "Reads today's daily note + completed tasks and appends a ## Summary section recapping what happened.",
			SystemPrompt: "You write a tight end-of-day summary appended to the user's daily note. Steps:\n" +
				"1. Call get_today to anchor the date.\n" +
				"2. Call read_note on Jots/{today}.md — this is the source of truth for what the user logged today (notes, tasks marked done, plan blocks, jots).\n" +
				"3. Call query_tasks with status=done to see tasks completed today (filter out anything not completed today by checking the date in the task context — if you can't tell, leave it out).\n" +
				"4. Synthesise a 4–8 line ## Summary section:\n" +
				"   - One opening sentence: the day's overall arc (productive / scattered / blocked / etc.) — be honest, not generic.\n" +
				"   - Bulleted list (3–6 bullets) of concrete things done: tasks shipped, decisions made, notes written. Cite task ids or note paths.\n" +
				"   - One closing line: anything worth remembering for tomorrow (a lesson, a leftover, a tone-shift).\n" +
				"5. Write back via write_note to Jots/{today}.md, APPENDING the new ## Summary section to whatever the note already contains. Do NOT delete or rewrite existing content.\n" +
				"6. Final answer: 1-sentence recap of the day.\n" +
				"Be specific. \"Made progress on the migration\" is useless; \"shipped #134 (auth refresh) and unblocked the QA queue\" is good. If the note is empty / sparse, say so plainly rather than padding.",
			Tools:        []string{"get_today", "read_note", "query_tasks", "write_note"},
			IncludeWrite: true,
			MaxSteps:     8,
		},
		{
			ID:          "reflect-on-day",
			Name:        "Reflect on today",
			Description: "Reads today's daily note and writes a ## Reflection — a thoughtful, honest 150–250 word check-in.",
			SystemPrompt: "You are a thoughtful journaling coach helping the user reflect at end of day. Steps:\n" +
				"1. Call get_today to anchor the date.\n" +
				"2. Call read_note on Jots/{today}.md to see what the user actually logged: scripture, goal, tasks, habits, thoughts, plan, summary if present.\n" +
				"3. Optionally call read_note on yesterday's daily (Jots/{yesterday}.md) for one-day continuity context — skip if the file doesn't exist.\n" +
				"4. Write a 150–250 word ## Reflection grounded in what's actually in the note. Structure freely but cover:\n" +
				"   - What seems to have gone well (and why, based on the entries).\n" +
				"   - Where there was friction, fatigue, or drift.\n" +
				"   - One question or noticing for tomorrow — open, not prescriptive.\n" +
				"5. Write back via write_note to Jots/{today}.md, APPENDING ## Reflection to existing content. Do NOT delete or rewrite anything else.\n" +
				"6. Final answer: 1 short sentence pointing to the core noticing.\n" +
				"Tone: warm, observational, second-person (\"you\"). Do not invent feelings the user did not write. If the note is too sparse to reflect on honestly, say so in one sentence and write a 3-line ## Reflection inviting the user to journal more — short and gentle.",
			Tools:        []string{"get_today", "read_note", "write_note"},
			IncludeWrite: true,
			MaxSteps:     6,
		},
		{
			ID:          "deep-research",
			Name:        "Deep Research",
			Description: "Multi-step research run: gathers vault notes on a topic, synthesises a structured brief, writes it to Research/.",
			SystemPrompt: "You are a research analyst running on the server with a budget cap. The user gives you a topic " +
				"(e.g. \"AI in 2026 SaaS niches\"). Your job is to produce a structured, well-cited research brief and " +
				"persist it to the vault. Work in phases — DO NOT try to write the brief before you've gathered material.\n\n" +
				"Phase 1 — Scope (1 step):\n" +
				"  - Restate the topic in your own words and list 3–5 sub-questions you'll investigate.\n\n" +
				"Phase 2 — Gather (3–8 steps):\n" +
				"  - Use search_vault with several distinct queries that map onto your sub-questions. Don't just rephrase the same query — vary terms.\n" +
				"  - Use list_notes on relevant folders (e.g. Research/, Inbox/, Reading/) when search comes up thin.\n" +
				"  - Read each promising hit with read_note. Quote (briefly) the lines that matter; track the path.\n" +
				"  - If the vault is sparse on the topic, that's a finding — note it explicitly rather than padding.\n\n" +
				"Phase 3 — Synthesise (2–4 steps):\n" +
				"  - Identify recurring themes, contradictions, and gaps across what you read.\n" +
				"  - For each sub-question, draft a 1–3 sentence answer grounded in the cited notes.\n" +
				"  - Flag what's NOT in the vault — open questions the user would need to research externally.\n\n" +
				"Phase 4 — Write (1 step):\n" +
				"  - Call get_today to get the current date.\n" +
				"  - Write the brief via write_note to Research/{today}-{slug}.md, where {slug} is the topic " +
				"lowercased with non-alphanumerics replaced by hyphens (e.g. 'ai-2026-saas-niches').\n" +
				"  - Frontmatter: type: research, date: today, topic: \"the topic\", tags: [research].\n" +
				"  - Body structure:\n" +
				"    # {Topic}\n" +
				"    ## Scope — restated topic + sub-questions\n" +
				"    ## Findings — one ### subsection per sub-question, each with cited claims (link or quote note paths)\n" +
				"    ## Themes — recurring patterns across sources\n" +
				"    ## Open questions — what the vault doesn't answer\n" +
				"    ## Sources — bullet list of every note path you read\n\n" +
				"Phase 5 — Final answer:\n" +
				"  - 3–4 sentence summary of the brief's core insight + the path you wrote.\n\n" +
				"Rules: cite note paths inline ([path/to/note.md]) for every non-trivial claim. Do not invent " +
				"sources or facts not present in the vault. If a claim feels like prior knowledge rather than " +
				"vault content, mark it explicitly as '(prior knowledge)' so the reader knows it isn't sourced. " +
				"Stop early and write what you have if you're approaching the budget — a short well-cited brief " +
				"beats a long fabricated one.",
			Tools:        []string{"search_vault", "list_notes", "read_note", "query_objects", "query_tasks", "get_today", "write_note"},
			IncludeWrite: true,
			MaxSteps:     20,
		},
		{
			ID:          "inbox-triager",
			Name:        "Inbox Triager",
			Description: "Reviews recent captures and proposes next-action tasks (with confirmation).",
			SystemPrompt: "You triage an inbox of captured notes. Use list_notes on the 'Inbox' folder " +
				"(or whatever folder the user names) to enumerate recent captures. For each capture, " +
				"read it briefly with read_note, then propose ONE concrete next-action task via " +
				"create_task — phrase the task so it's actionable in <30 minutes. Always include a " +
				"due date (call get_today if needed) and a relevant tag. Do not create a task for " +
				"items that are already complete, irrelevant, or duplicates of existing tasks. " +
				"Stop after 5 captures and produce a Final Answer summarising what you did.",
			Tools:        []string{"list_notes", "read_note", "query_tasks", "create_task", "get_today"},
			IncludeWrite: true,
		},
		{
			ID:          "weekly-review-draft",
			Name:        "Weekly review draft",
			Description: "Drafts a weekly review by reading the last 7 days of jots + completed tasks. Saves the draft to Reviews/<week>.md so the /review page picks it up. Refuses to overwrite an existing review.",
			SystemPrompt: "You draft a weekly review for the user. The five canonical questions live as ## headings " +
				"in the saved markdown so the /review page parses your output back into its form.\n\n" +
				"Procedure:\n" +
				"1. Call get_today to anchor the date. Compute today and the previous 6 days (so a 7-day " +
				"   window ending today).\n" +
				"2. Compute the ISO week ID for today (year-Www, e.g. 2026-W18). The output file is " +
				"   Reviews/<week>.md. The user told you their preferred ISO week format in the goal — " +
				"   trust it if supplied.\n" +
				"3. Try read_note on Reviews/<week>.md FIRST. If it exists with non-empty answers, STOP — " +
				"   do not overwrite the user's own work. Final Answer: tell them the review already " +
				"   exists and ask them to delete it first if they want a fresh AI draft.\n" +
				"4. Otherwise, gather signal:\n" +
				"   - read_note on Jots/<each-of-the-7-dates>.md (the daily-note folder may be 'Jots' or " +
				"     custom — try Jots first; skip 404s silently).\n" +
				"   - query_tasks with status=done to see what shipped this week. Filter to tasks whose " +
				"     date context fits the 7-day window; if you can't tell, leave them out rather than " +
				"     guessing.\n" +
				"5. The user's vision/season-focus is in the goal text the page sent you — quote it " +
				"   verbatim in the 'Vision check' answer.\n" +
				"6. Write a markdown body with EXACTLY these five ## headings, in this order, each followed " +
				"   by 2-5 lines of honest, evidence-grounded prose drawn from the jots and tasks you read:\n" +
				"     ## Vision check\n" +
				"     ## Wins\n" +
				"     ## Setbacks\n" +
				"     ## People\n" +
				"     ## Next week's one thing\n" +
				"   Cite specifics (dates, task ids, note paths). Write in second person ('you'), warm, " +
				"   not flattering. If a section has no real signal, write one short honest line " +
				"   ('Nothing recorded this week — worth noticing.') rather than padding.\n" +
				"7. write_note to Reviews/<week>.md with the body above PLUS frontmatter at the top:\n" +
				"     ---\n" +
				"     type: weekly-review\n" +
				"     week_iso: <week>\n" +
				"     ---\n" +
				"   Without the frontmatter the /review page won't recognise it as a structured review.\n" +
				"8. Final Answer: 1 sentence — 'Drafted Reviews/<week>.md, open /review to read and edit.'\n\n" +
				"Hard rule: never invent feelings, decisions, or events that aren't in the jots/tasks/notes you " +
				"read. A short, true draft beats a long fabricated one.",
			Tools:        []string{"get_today", "read_note", "list_notes", "query_tasks", "write_note"},
			IncludeWrite: true,
			MaxSteps:     14,
		},
	}
}

// NewPresetCatalog returns a catalog seeded with the given
// built-in presets. The TUI passes its hardcoded list at
// startup; tests can pass an empty list to exercise edge cases.
func NewPresetCatalog(builtins []Preset) *PresetCatalog {
	c := &PresetCatalog{presets: map[string]Preset{}}
	for _, p := range builtins {
		// Built-in misconfiguration is a programmer error, not a
		// runtime fault — skip silently rather than panic so a
		// bad built-in doesn't take down the whole runner.
		if err := p.Validate(); err == nil {
			c.presets[p.ID] = p
		}
	}
	return c
}

// LoadVaultDir scans `<vaultRoot>/.granit/agents/` for `*.json`
// files and overlays them onto the catalog. Same rules as the
// type registry: filename basename must match the embedded ID
// (case-insensitive), each file is validated independently,
// per-file errors are returned together so the caller can render
// them all at once.
//
// Returns (loadedCount, errors) — errors is nil-slice when no
// problems occurred. The catalog mutates in place.
func (c *PresetCatalog) LoadVaultDir(vaultRoot string) (int, []error) {
	if vaultRoot == "" {
		return 0, nil
	}
	dir := filepath.Join(vaultRoot, ".granit", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, []error{fmt.Errorf("read %s: %w", dir, err)}
	}
	loaded := 0
	var errs []error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		var p Preset
		if err := json.Unmarshal(data, &p); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		expectedID := strings.TrimSuffix(e.Name(), ".json")
		if !strings.EqualFold(expectedID, p.ID) {
			errs = append(errs, fmt.Errorf("%s: filename %q does not match embedded id %q", e.Name(), expectedID, p.ID))
			continue
		}
		if err := p.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		c.presets[p.ID] = p
		loaded++
	}
	return loaded, errs
}

// ByID returns the preset with the given ID, or (zero, false) when
// none exists. Used by the runner to look up the preset the user
// picked from the list.
func (c *PresetCatalog) ByID(id string) (Preset, bool) {
	p, ok := c.presets[id]
	return p, ok
}

// All returns every preset in stable ID order so the picker
// renders deterministically across vault rebuilds.
func (c *PresetCatalog) All() []Preset {
	ids := make([]string, 0, len(c.presets))
	for id := range c.presets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Preset, len(ids))
	for i, id := range ids {
		out[i] = c.presets[id]
	}
	return out
}

// Len reports the catalog size.
func (c *PresetCatalog) Len() int { return len(c.presets) }

// SavePreset writes a preset to `<vaultRoot>/.granit/agents/<id>.json`,
// creating the directory if needed. Validates first; an invalid
// preset is not written. Used by future "save as preset" UI flows.
func SavePreset(vaultRoot string, p Preset) error {
	if err := p.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(vaultRoot, ".granit", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(dir, p.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// BuildRegistryForPreset constructs a Registry containing the tools
// the preset opts into. allReadTools and allWriteTools are the
// caller-supplied factories — the agents package doesn't know about
// VaultReader/VaultWriter directly, so the TUI hands in pre-built
// tools and we filter by the preset's Tools allow-list.
//
// When preset.Tools is empty, ALL provided readTools are registered
// (the "no allow-list = all reads" convention). preset.IncludeWrite
// adds writeTools regardless of the Tools allow-list.
func BuildRegistryForPreset(preset Preset, readTools, writeTools []Tool) (*Registry, error) {
	r := NewRegistry()
	wantedRead := map[string]bool{}
	if len(preset.Tools) == 0 {
		// No allow-list: all read tools.
		for _, t := range readTools {
			wantedRead[t.Name()] = true
		}
	} else {
		for _, name := range preset.Tools {
			wantedRead[strings.TrimSpace(name)] = true
		}
	}
	for _, t := range readTools {
		if t.Kind() != KindRead {
			continue // safety: a "read" factory that returns a write tool stays out
		}
		if wantedRead[t.Name()] {
			if err := r.Register(t); err != nil {
				return nil, err
			}
		}
	}
	if preset.IncludeWrite {
		for _, t := range writeTools {
			if t.Kind() != KindWrite {
				continue
			}
			if err := r.Register(t); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}
