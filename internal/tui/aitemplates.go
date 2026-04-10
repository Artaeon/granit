package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// aiTemplateResultMsg — carries the AI-generated content back to Update
// ---------------------------------------------------------------------------

type aiTemplateResultMsg struct {
	content string
	err     error
}

// ---------------------------------------------------------------------------
// Loading tick
// ---------------------------------------------------------------------------

type aiTemplateTickMsg struct{}

func aiTemplateTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return aiTemplateTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Template type descriptors
// ---------------------------------------------------------------------------

type aiTemplateType struct {
	name string
	icon string
	desc string
}

var aiTemplateTypes = []aiTemplateType{
	{name: "Meeting Notes", icon: IconCalendarChar, desc: "Agenda, attendees, action items"},
	{name: "Project Plan", icon: IconOutlineChar, desc: "Goals, timeline, milestones, tasks"},
	{name: "Technical Doc", icon: IconFileChar, desc: "Architecture, API, implementation details"},
	{name: "Blog Post", icon: IconEditChar, desc: "Introduction, sections, conclusion"},
	{name: "Tutorial / How-To", icon: IconBookmarkChar, desc: "Step-by-step guide with examples"},
	{name: "Comparison / Analysis", icon: IconSearchChar, desc: "Side-by-side evaluation of options"},
	{name: "Book/Article Summary", icon: IconTagChar, desc: "Key ideas, quotes, takeaways"},
	{name: "Training / Workout Plan", icon: IconGraphChar, desc: "Exercises, sets, schedule"},
	{name: "Custom", icon: IconNewChar, desc: "Write your own prompt"},
}

// ---------------------------------------------------------------------------
// UI states
// ---------------------------------------------------------------------------

const (
	aitStateTypeSelect = iota // picking a template type
	aitStateTopicInput        // entering the topic
	aitStateGenerating        // waiting for AI
	aitStatePreview           // showing generated content
)

// ---------------------------------------------------------------------------
// AITemplates overlay
// ---------------------------------------------------------------------------

// AITemplates is an overlay that lets users describe what kind of note they
// want, picks a template type, enters a topic, and generates the full note
// content via Ollama, OpenAI, or a local fallback.
type AITemplates struct {
	active bool
	width  int
	height int

	state  int
	cursor int
	scroll int

	// Selected template type index
	selectedType int

	// User inputs
	topicInput  string
	customInput string // used when template type is "Custom"

	// AI config (set via Open)
	ai AIConfig

	// Generated content
	generatedContent string
	generatedTitle   string

	// Loading animation
	loadingTick  int
	loadingStart time.Time

	// Error
	errMsg string

	// Consumed-once result
	resultTitle   string
	resultContent string
	resultReady   bool
}

// NewAITemplates creates a new AITemplates overlay in its default state.
func NewAITemplates() AITemplates {
	return AITemplates{}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

// IsActive returns true when the overlay is visible.
func (a AITemplates) IsActive() bool { return a.active }

// Open activates the overlay and stores the AI configuration.
func (a *AITemplates) OpenWithAI(cfg AIConfig) {
	a.active = true
	a.state = aitStateTypeSelect
	a.cursor = 0
	a.scroll = 0
	a.selectedType = 0
	a.topicInput = ""
	a.customInput = ""
	a.generatedContent = ""
	a.generatedTitle = ""
	a.loadingTick = 0
	a.loadingStart = time.Time{}
	a.errMsg = ""
	a.resultReady = false
	a.resultTitle = ""
	a.resultContent = ""

	a.ai = cfg
	if a.ai.Provider == "" {
		a.ai.Provider = "local"
	}
	if a.ai.Model == "" {
		a.ai.Model = "llama3.2"
	}
	if a.ai.OllamaURL == "" {
		a.ai.OllamaURL = "http://localhost:11434"
	}
}

// Close hides the overlay.
func (a *AITemplates) Close() {
	a.active = false
}

// SetSize updates the available dimensions.
func (a *AITemplates) SetSize(w, h int) {
	a.width = w
	a.height = h
}

// GetResult returns the generated title and content once, then clears the
// result so it is not consumed twice.
func (a *AITemplates) GetResult() (title string, content string, ok bool) {
	if !a.resultReady {
		return "", "", false
	}
	t := a.resultTitle
	c := a.resultContent
	a.resultReady = false
	a.resultTitle = ""
	a.resultContent = ""
	return t, c, true
}

// ---------------------------------------------------------------------------
// Prompt construction
// ---------------------------------------------------------------------------

func (a *AITemplates) buildPrompt() string {
	typeName := aiTemplateTypes[a.selectedType].name
	topic := a.topicInput

	if typeName == "Custom" {
		// For custom, the customInput IS the prompt
		return fmt.Sprintf(`Generate a well-structured markdown note based on the following instruction.

Instruction: %s
Topic: %s

Include:
1) YAML frontmatter with title, date (use today's date in YYYY-MM-DD format), tags (relevant to the content), and type fields.
2) Clear headings (##, ###) that organize the content logically.
3) Bullet points, numbered lists, or tables where appropriate.
4) Substantive content — not just placeholders, but real useful information.
5) Keep it informative, well-organized, and ready to use as a note.`, a.customInput, topic)
	}

	return fmt.Sprintf(`Generate a well-structured markdown note of type "%s" about the following topic.

Topic: %s

Include:
1) YAML frontmatter with:
   - title: a clear title based on the topic
   - date: today's date in YYYY-MM-DD format
   - tags: relevant tags as a YAML list
   - type: %s
2) Clear headings (##, ###) appropriate for a %s document.
3) Substantive content — not just placeholders, but real useful information and structure.
4) Use bullet points, numbered lists, code blocks, or tables where appropriate.
5) Make it ready to use as a note — fill in details relevant to the topic.

Generate ONLY the markdown content (starting with the --- frontmatter). No preamble or explanation.`,
		typeName, topic, strings.ToLower(strings.ReplaceAll(typeName, " ", "-")),
		strings.ToLower(typeName))
}

// ---------------------------------------------------------------------------
// Local fallback — generates a structured template skeleton
// ---------------------------------------------------------------------------

func (a *AITemplates) generateLocalFallback() string {
	topic := a.topicInput
	if topic == "" {
		topic = "Untitled"
	}
	today := time.Now().Format("2006-01-02")
	switch a.selectedType {
	case 0: // Meeting Notes
		return fmt.Sprintf(`---
title: "Meeting Notes — %s"
date: %s
tags: [meeting, %s]
type: meeting
---

# Meeting Notes — %s

## Meeting Details
- **Date:** %s
- **Time:**
- **Location:**
- **Facilitator:**

## Attendees
- [ ]
- [ ]

## Agenda
1. %s — overview and discussion
2. Open items from previous meetings
3. New business

## Discussion Notes

### %s
- Key points discussed:
  -
  -
- Decisions made:
  -

### Open Discussion
-

## Action Items
- [ ] Follow up on %s — **Owner:**  — **Due:**
- [ ] Prepare materials for next meeting — **Owner:**  — **Due:**
- [ ] Share meeting notes with team — **Owner:**  — **Due:**

## Next Meeting
- **Date:**
- **Agenda Preview:**
`, topic, today, strings.ToLower(strings.Fields(topic)[0]), topic,
			today, topic, topic, topic)

	case 1: // Project Plan
		return fmt.Sprintf(`---
title: "Project Plan — %s"
date: %s
tags: [project, planning, %s]
type: project
---

# Project Plan — %s

## Overview
This project plan outlines the scope, goals, timeline, and deliverables for %s.

## Goals
- [ ] Define clear objectives and success criteria
- [ ] Establish timeline and milestones
- [ ] Identify resources and dependencies

## Scope
### In Scope
-
-

### Out of Scope
-
-

## Timeline
| Phase | Description | Start | End | Status |
|-------|-------------|-------|-----|--------|
| Phase 1 | Research & Planning | %s | | Not Started |
| Phase 2 | Implementation | | | Not Started |
| Phase 3 | Testing & Review | | | Not Started |
| Phase 4 | Launch | | | Not Started |

## Tasks
### Phase 1 — Research & Planning
- [ ] Research %s requirements
- [ ] Define success metrics
- [ ] Create detailed specification

### Phase 2 — Implementation
- [ ] Set up environment
- [ ] Build core features
- [ ] Integrate components

### Phase 3 — Testing & Review
- [ ] Write tests
- [ ] Conduct review
- [ ] Address feedback

## Resources
- **Team:**
- **Tools:**
- **Budget:**

## Risks
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| | Medium | High | |

## Notes

`, topic, today, strings.ToLower(strings.Fields(topic)[0]), topic, topic,
			today, topic)

	case 2: // Technical Doc
		return fmt.Sprintf(`---
title: "Technical Documentation — %s"
date: %s
tags: [technical, documentation, %s]
type: technical-doc
---

# %s

## Overview
A technical overview of %s, covering architecture, key components, and implementation details.

## Architecture

### System Components
- **Component 1:**
- **Component 2:**
- **Component 3:**

### Data Flow
1.
2.
3.

## Implementation Details

### Prerequisites
-
-

### Setup
` + "```" + `bash
# Installation steps
` + "```" + `

### Configuration
` + "```" + `yaml
# Key configuration options
` + "```" + `

### API Reference

#### Endpoint / Function 1
- **Description:**
- **Parameters:**
- **Returns:**

#### Endpoint / Function 2
- **Description:**
- **Parameters:**
- **Returns:**

## Usage Examples

` + "```" + `
# Example usage
` + "```" + `

## Troubleshooting
| Issue | Cause | Solution |
|-------|-------|----------|
| | | |

## References
-
-
`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic)

	case 3: // Blog Post
		return fmt.Sprintf(`---
title: "%s"
date: %s
tags: [blog, %s]
type: blog
draft: true
---

# %s

## Introduction
An exploration of %s — why it matters and what you need to know.

## Background
Before diving in, it helps to understand the context around %s.

-
-

## Main Content

### Key Point 1
Discuss the first major aspect of %s.

-
-

### Key Point 2
Explore the second dimension.

-
-

### Key Point 3
Address the third angle.

-
-

## Practical Takeaways
1.
2.
3.

## Conclusion
Summarize the key insights about %s and suggest next steps for the reader.

## Further Reading
-
-
`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic, topic, topic, topic)

	case 4: // Tutorial / How-To
		return fmt.Sprintf(`---
title: "How-To — %s"
date: %s
tags: [tutorial, how-to, %s]
type: tutorial
---

# How-To: %s

## What You Will Learn
By the end of this guide, you will be able to:
-
-
-

## Prerequisites
-
-

## Step 1 — Getting Started
Begin by setting up your environment for %s.

` + "```" + `
# Setup commands or initial steps
` + "```" + `

## Step 2 — Core Implementation
Now implement the main functionality.

` + "```" + `
# Core steps
` + "```" + `

## Step 3 — Configuration and Customization
Adjust settings to match your needs.

- **Option A:**
- **Option B:**

## Step 4 — Testing and Verification
Verify everything works as expected.

` + "```" + `
# Verification steps
` + "```" + `

## Common Issues
| Problem | Solution |
|---------|----------|
| | |

## Tips and Best Practices
-
-
-

## Summary
You have now learned the basics of %s. For more advanced usage, consider exploring:
-
-
`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic, topic)

	case 5: // Comparison / Analysis
		return fmt.Sprintf(`---
title: "Comparison — %s"
date: %s
tags: [comparison, analysis, %s]
type: comparison
---

# %s

## Overview
A structured comparison and analysis of %s to help make an informed decision.

## Criteria
The following criteria are used for evaluation:
1. **Feature Set** — What capabilities does each option provide?
2. **Performance** — How well does each option perform?
3. **Ease of Use** — How accessible is each option?
4. **Cost** — What is the total cost of ownership?
5. **Community & Support** — What resources are available?

## Comparison Table
| Criteria | Option A | Option B |
|----------|----------|----------|
| Feature Set | | |
| Performance | | |
| Ease of Use | | |
| Cost | | |
| Community | | |

## Detailed Analysis

### Option A
**Strengths:**
-
-

**Weaknesses:**
-
-

### Option B
**Strengths:**
-
-

**Weaknesses:**
-
-

## Use Case Recommendations
- **Choose Option A if:**
- **Choose Option B if:**

## Verdict
Based on the analysis of %s, the recommendation is:

>

## References
-
-
`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic, topic)

	case 6: // Book/Article Summary
		return fmt.Sprintf(`---
title: "Summary — %s"
date: %s
tags: [summary, book-notes, %s]
type: book-summary
---

# Summary: %s

## Metadata
- **Author:**
- **Published:**
- **Genre/Category:**
- **Rating:** /5

## Overview
A concise summary of %s — the central thesis and what it aims to accomplish.

## Key Ideas

### Idea 1
-

### Idea 2
-

### Idea 3
-

## Notable Quotes
> "Quote 1"

> "Quote 2"

## Chapter/Section Notes

### Chapter 1
-

### Chapter 2
-

## Personal Reflections
- What resonated with me:
  -
- What I disagree with:
  -
- How I can apply this:
  -

## Action Items
- [ ]
- [ ]

## Related Reading
-
-
`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic)

	case 7: // Training / Workout Plan
		return fmt.Sprintf(`---
title: "Training Plan — %s"
date: %s
tags: [training, workout, %s]
type: workout
---

# Training Plan: %s

## Goals
- **Primary Goal:**
- **Timeline:**
- **Frequency:**

## Schedule Overview
| Day | Focus | Duration |
|-----|-------|----------|
| Monday | | |
| Tuesday | Rest / Active Recovery | |
| Wednesday | | |
| Thursday | | |
| Friday | | |
| Saturday | | |
| Sunday | Rest | |

## Warm-Up Routine
1. Dynamic stretching — 5 min
2. Light cardio — 5 min
3. Mobility work — 5 min

## Workout Details

### Session A — %s (Focus 1)
| Exercise | Sets | Reps | Rest |
|----------|------|------|------|
| | 3 | 10-12 | 60s |
| | 3 | 10-12 | 60s |
| | 3 | 8-10 | 90s |
| | 3 | 12-15 | 45s |

### Session B — %s (Focus 2)
| Exercise | Sets | Reps | Rest |
|----------|------|------|------|
| | 3 | 10-12 | 60s |
| | 3 | 10-12 | 60s |
| | 3 | 8-10 | 90s |
| | 3 | 12-15 | 45s |

## Cool-Down
1. Static stretching — 5 min
2. Foam rolling — 5 min

## Nutrition Notes
- **Pre-workout:**
- **Post-workout:**
- **Daily protein target:**

## Progress Tracking
| Week | Notes | Adjustments |
|------|-------|-------------|
| Week 1 | | |
| Week 2 | | |
| Week 3 | | |
| Week 4 | Deload | |

## Notes

`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic, topic)

	default: // Custom or fallback
		return fmt.Sprintf(`---
title: "%s"
date: %s
tags: [%s]
type: note
---

# %s

## Overview
%s

## Details

### Section 1
-
-

### Section 2
-
-

### Section 3
-
-

## Key Points
1.
2.
3.

## Notes

## References
-
`, topic, today, strings.ToLower(strings.Fields(topic)[0]),
			topic, topic)
	}
}

// ---------------------------------------------------------------------------
// AI generation via tea.Cmd
// ---------------------------------------------------------------------------

func (a *AITemplates) generateContent() tea.Cmd {
	systemPrompt := "You are a note-taking assistant. Generate well-structured markdown notes with YAML frontmatter. Be thorough and informative."
	userPrompt := a.buildPrompt()
	ai := a.ai

	return func() tea.Msg {
		resp, err := ai.Chat(systemPrompt, userPrompt)
		return aiTemplateResultMsg{content: resp, err: err}
	}
}
// ---------------------------------------------------------------------------
// Title derivation
// ---------------------------------------------------------------------------

func aiTemplateTitleFromTopic(topic string) string {
	title := strings.TrimSpace(topic)
	if len(title) > 60 {
		title = title[:60]
	}
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	title = replacer.Replace(title)
	if title == "" {
		title = "Untitled"
	}
	return title
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (a AITemplates) Update(msg tea.Msg) (AITemplates, tea.Cmd) {
	if !a.active {
		return a, nil
	}

	switch msg := msg.(type) {

	case aiTemplateResultMsg:
		if a.state != aitStateGenerating {
			return a, nil
		}
		if msg.err != nil {
			// Fallback to local generation on AI error
			a.errMsg = msg.err.Error()
			a.generatedContent = a.generateLocalFallback()
			a.generatedTitle = aiTemplateTitleFromTopic(a.topicInput)
			a.state = aitStatePreview
			a.scroll = 0
			return a, nil
		}
		a.generatedContent = msg.content
		a.generatedTitle = aiTemplateTitleFromTopic(a.topicInput)
		a.state = aitStatePreview
		a.scroll = 0
		a.errMsg = ""
		return a, nil

	case aiTemplateTickMsg:
		if a.state == aitStateGenerating {
			a.loadingTick++
			return a, aiTemplateTickCmd()
		}
		return a, nil

	case tea.KeyMsg:
		switch a.state {
		case aitStateTypeSelect:
			return a.updateTypeSelect(msg)
		case aitStateTopicInput:
			return a.updateTopicInput(msg)
		case aitStateGenerating:
			if msg.String() == "esc" {
				a.state = aitStateTypeSelect
				return a, nil
			}
		case aitStatePreview:
			return a.updatePreview(msg)
		}
	}

	return a, nil
}

func (a AITemplates) updateTypeSelect(msg tea.KeyMsg) (AITemplates, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.active = false
	case "up", "k":
		if a.cursor > 0 {
			a.cursor--
			if a.cursor < a.scroll {
				a.scroll = a.cursor
			}
		}
	case "down", "j":
		if a.cursor < len(aiTemplateTypes)-1 {
			a.cursor++
			visH := a.height - 12
			if visH < 1 {
				visH = 1
			}
			if a.cursor >= a.scroll+visH {
				a.scroll = a.cursor - visH + 1
			}
		}
	case "enter":
		if a.cursor >= 0 && a.cursor < len(aiTemplateTypes) {
			a.selectedType = a.cursor
			a.state = aitStateTopicInput
			a.topicInput = ""
			a.customInput = ""
			a.errMsg = ""
		}
	}
	return a, nil
}

func (a AITemplates) updateTopicInput(msg tea.KeyMsg) (AITemplates, tea.Cmd) {
	isCustom := aiTemplateTypes[a.selectedType].name == "Custom"

	switch msg.String() {
	case "esc":
		a.state = aitStateTypeSelect
		a.errMsg = ""
	case "enter":
		if strings.TrimSpace(a.topicInput) == "" {
			return a, nil
		}
		if isCustom && strings.TrimSpace(a.customInput) == "" {
			return a, nil
		}
		// Start generation
		if a.ai.Provider == "local" {
			a.generatedContent = a.generateLocalFallback()
			a.generatedTitle = aiTemplateTitleFromTopic(a.topicInput)
			a.state = aitStatePreview
			a.scroll = 0
			return a, nil
		}
		a.state = aitStateGenerating
		a.loadingTick = 0
		a.loadingStart = time.Now()
		a.errMsg = ""
		return a, tea.Batch(a.generateContent(), aiTemplateTickCmd())
	case "backspace":
		if len(a.topicInput) > 0 {
			a.topicInput = TrimLastRune(a.topicInput)
		}
	case "ctrl+u":
		a.topicInput = ""
	case "tab":
		// For custom type, tab switches focus — but to keep things simple
		// we use a single input and the custom prompt is the topic itself.
		// No-op.
	default:
		if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
			a.topicInput += msg.String()
		}
	}
	return a, nil
}

func (a AITemplates) updatePreview(msg tea.KeyMsg) (AITemplates, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.state = aitStateTopicInput
		a.scroll = 0
	case "enter":
		// Accept — store result
		a.resultTitle = a.generatedTitle
		a.resultContent = a.generatedContent
		a.resultReady = true
		a.active = false
		return a, nil
	case "r":
		// Regenerate
		if a.ai.Provider == "local" {
			a.generatedContent = a.generateLocalFallback()
			a.scroll = 0
			return a, nil
		}
		a.state = aitStateGenerating
		a.loadingTick = 0
		a.loadingStart = time.Now()
		a.generatedContent = ""
		a.errMsg = ""
		return a, tea.Batch(a.generateContent(), aiTemplateTickCmd())
	case "up", "k":
		if a.scroll > 0 {
			a.scroll--
		}
	case "down", "j":
		lines := strings.Count(a.generatedContent, "\n") + 1
		maxScroll := lines - (a.height - 16)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if a.scroll < maxScroll {
			a.scroll++
		}
	}
	return a, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (a AITemplates) View() string {
	width := a.width / 2
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	var b strings.Builder

	switch a.state {
	case aitStateTypeSelect:
		b.WriteString(a.viewTypeSelect(width))
	case aitStateTopicInput:
		b.WriteString(a.viewTopicInput(width))
	case aitStateGenerating:
		b.WriteString(a.viewGenerating(width))
	case aitStatePreview:
		b.WriteString(a.viewPreview(width))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (a AITemplates) viewTypeSelect(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconNewChar + " AI Template Generator")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	b.WriteString(NormalItemStyle.Render("  Choose a template type:"))
	b.WriteString("\n\n")

	iconColors := []lipgloss.Color{peach, blue, green, sapphire, yellow, red, lavender, teal, mauve}

	visH := a.height - 14
	if visH < 5 {
		visH = 5
	}
	end := a.scroll + visH
	if end > len(aiTemplateTypes) {
		end = len(aiTemplateTypes)
	}

	for i := a.scroll; i < end; i++ {
		tmpl := aiTemplateTypes[i]
		colorIdx := i % len(iconColors)
		iconStyle := lipgloss.NewStyle().Foreground(iconColors[colorIdx])
		icon := iconStyle.Render(tmpl.icon)

		if i == a.cursor {
			line := fmt.Sprintf("  %s %s", icon, tmpl.name)
			desc := DimStyle.Render(" — " + tmpl.desc)
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(width - 6).
				Render(line) + desc)
		} else {
			b.WriteString(fmt.Sprintf("  %s %s", icon, NormalItemStyle.Render(tmpl.name)))
			b.WriteString(DimStyle.Render(" — " + tmpl.desc))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	providerBadge := a.providerBadge()
	b.WriteString(DimStyle.Render("  Enter: select  Esc: close") + "  " + providerBadge)

	return b.String()
}

func (a AITemplates) viewTopicInput(width int) string {
	var b strings.Builder

	typeName := aiTemplateTypes[a.selectedType].name

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconNewChar + " AI Template Generator")
	b.WriteString(title)
	b.WriteString("\n")

	typeLabel := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(typeName)
	b.WriteString("  " + DimStyle.Render("Type: ") + typeLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	if typeName == "Custom" {
		b.WriteString(NormalItemStyle.Render("  Enter the topic and your custom instructions:"))
	} else {
		b.WriteString(NormalItemStyle.Render("  Enter the topic for your " + strings.ToLower(typeName) + ":"))
	}
	b.WriteString("\n\n")

	promptStyle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true)
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(width - 12)

	b.WriteString("  " + promptStyle.Render("> "))
	displayTopic := a.topicInput + "\u2588"
	b.WriteString(inputStyle.Render(displayTopic))
	b.WriteString("\n\n")

	if a.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red)
		b.WriteString("  " + errStyle.Render("Error: "+a.errMsg))
		b.WriteString("\n\n")
	}

	// Show examples based on type
	b.WriteString(DimStyle.Render("  Examples:"))
	b.WriteString("\n")

	exampleStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	examples := a.examplesForType()
	for _, ex := range examples {
		b.WriteString("    " + exampleStyle.Render("\""+ex+"\""))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: generate  Esc: back"))

	return b.String()
}

func (a AITemplates) viewGenerating(width int) string {
	var b strings.Builder

	typeName := aiTemplateTypes[a.selectedType].name

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconNewChar + " AI Template Generator")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	spinner := []string{"\u280b", "\u2819", "\u2838", "\u2834", "\u2826", "\u2807"}
	frame := spinner[a.loadingTick%len(spinner)]

	elapsed := time.Since(a.loadingStart).Truncate(time.Second)
	loadStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString("  " + loadStyle.Render(frame+" Generating "+strings.ToLower(typeName)+"...") + DimStyle.Render(fmt.Sprintf(" %s", elapsed)))
	b.WriteString("\n\n")

	promptDisplay := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	b.WriteString("  " + DimStyle.Render("Topic: ") + promptDisplay.Render(a.topicInput))
	b.WriteString("\n\n")

	providerDisplay := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString("  " + providerDisplay.Render("Provider: "+a.ai.Provider+"  Model: "+a.ai.Model))

	return b.String()
}

func (a AITemplates) viewPreview(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  Note Generated")
	b.WriteString(title)
	b.WriteString("\n")

	filenameStyle := lipgloss.NewStyle().Foreground(peach)
	b.WriteString("  " + DimStyle.Render("File: ") + filenameStyle.Render(a.generatedTitle+".md"))
	b.WriteString("\n")

	if a.errMsg != "" {
		warnStyle := lipgloss.NewStyle().Foreground(yellow).Italic(true)
		b.WriteString("  " + warnStyle.Render("(AI unavailable, using local fallback)"))
		b.WriteString("\n")
	}

	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	// Render preview with basic syntax highlighting
	contentLines := strings.Split(a.generatedContent, "\n")

	visibleHeight := a.height - 16
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := a.scroll
	if start > len(contentLines) {
		start = len(contentLines)
	}
	end := start + visibleHeight
	if end > len(contentLines) {
		end = len(contentLines)
	}

	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	for i := start; i < end; i++ {
		line := contentLines[i]
		rendered := aiTemplateHighlightLine(line, contentWidth)
		b.WriteString("  " + rendered)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(contentLines) {
		b.WriteString("\n")
		scrollInfo := lipgloss.NewStyle().Foreground(overlay0)
		remaining := len(contentLines) - end
		b.WriteString("  " + scrollInfo.Render(fmt.Sprintf("... %d more lines (j/k to scroll)", remaining)))
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: accept  r: regenerate  Esc: back"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (a AITemplates) providerBadge() string {
	switch a.ai.Provider {
	case "ollama":
		return lipgloss.NewStyle().Foreground(green).Render("[ollama]")
	case "openai":
		return lipgloss.NewStyle().Foreground(blue).Render("[openai]")
	case "nous":
		return lipgloss.NewStyle().Foreground(green).Render("[nous]")
	default:
		return lipgloss.NewStyle().Foreground(overlay0).Render("[local]")
	}
}

func (a AITemplates) examplesForType() []string {
	switch a.selectedType {
	case 0: // Meeting Notes
		return []string{
			"Q1 product roadmap review",
			"Sprint retrospective — Team Alpha",
			"Client onboarding kickoff",
		}
	case 1: // Project Plan
		return []string{
			"Mobile app redesign",
			"Data pipeline migration",
			"Company blog launch",
		}
	case 2: // Technical Doc
		return []string{
			"REST API authentication flow",
			"Kubernetes deployment architecture",
			"Database schema design for e-commerce",
		}
	case 3: // Blog Post
		return []string{
			"Why Rust is great for CLI tools",
			"Remote work productivity tips",
			"Introduction to Zettelkasten method",
		}
	case 4: // Tutorial / How-To
		return []string{
			"Setting up Neovim with LSP",
			"Docker multi-stage builds",
			"Building a REST API in Go",
		}
	case 5: // Comparison / Analysis
		return []string{
			"React vs Vue vs Svelte",
			"PostgreSQL vs MySQL for web apps",
			"Obsidian vs Notion vs Logseq",
		}
	case 6: // Book/Article Summary
		return []string{
			"Thinking, Fast and Slow by Daniel Kahneman",
			"The Pragmatic Programmer by Hunt & Thomas",
			"Atomic Habits by James Clear",
		}
	case 7: // Training / Workout Plan
		return []string{
			"Boxing fundamentals for beginners",
			"5x5 strength training program",
			"Half marathon preparation 12-week plan",
		}
	case 8: // Custom
		return []string{
			"Create a recipe collection for meal prep",
			"Design a personal finance tracking system",
			"Build a reading list with progress tracking",
		}
	}
	return []string{"Your topic here"}
}

// ---------------------------------------------------------------------------
// Syntax highlighting for preview
// ---------------------------------------------------------------------------

func aiTemplateHighlightLine(line string, maxWidth int) string {
	if maxWidth > 0 && len(line) > maxWidth {
		line = line[:maxWidth]
	}

	headingStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	boldStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	wikilinkStyle := lipgloss.NewStyle().Foreground(blue).Underline(true)
	codeStyle := lipgloss.NewStyle().Foreground(yellow)
	frontmatterStyle := lipgloss.NewStyle().Foreground(overlay0)
	normalStyle := lipgloss.NewStyle().Foreground(text)

	trimmed := strings.TrimSpace(line)

	// Frontmatter delimiters
	if trimmed == "---" {
		return frontmatterStyle.Render(line)
	}

	// Frontmatter key-value lines (inside frontmatter block)
	if strings.Contains(line, ":") && !strings.HasPrefix(trimmed, "#") &&
		!strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "|") {
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		if len(key) > 0 && len(key) < 20 && !strings.Contains(key, " ") {
			keyStyle := lipgloss.NewStyle().Foreground(blue)
			valStyle := lipgloss.NewStyle().Foreground(text)
			return keyStyle.Render(parts[0]+":") + valStyle.Render(parts[1])
		}
	}

	// Headings
	if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") ||
		strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
		return headingStyle.Render(line)
	}

	// Code block markers
	if strings.HasPrefix(trimmed, "```") {
		return codeStyle.Render(line)
	}

	// Table rows
	if strings.HasPrefix(trimmed, "|") {
		return lipgloss.NewStyle().Foreground(overlay2).Render(line)
	}

	// Checkbox items
	if strings.Contains(trimmed, "- [ ]") || strings.Contains(trimmed, "- [x]") {
		checkStyle := lipgloss.NewStyle().Foreground(green)
		return checkStyle.Render(line)
	}

	// Process inline elements
	result := aiTemplateHighlightInline(line, normalStyle, boldStyle, wikilinkStyle, codeStyle)
	return result
}

func aiTemplateHighlightInline(line string, normal, bold, wikilink, code lipgloss.Style) string {
	var b strings.Builder
	i := 0
	n := len(line)

	for i < n {
		// Wikilinks [[...]]
		if i+1 < n && line[i] == '[' && line[i+1] == '[' {
			end := strings.Index(line[i+2:], "]]")
			if end >= 0 {
				linkText := line[i : i+2+end+2]
				b.WriteString(wikilink.Render(linkText))
				i = i + 2 + end + 2
				continue
			}
		}

		// Inline code `...`
		if line[i] == '`' {
			end := strings.Index(line[i+1:], "`")
			if end >= 0 {
				codeText := line[i : i+1+end+1]
				b.WriteString(code.Render(codeText))
				i = i + 1 + end + 1
				continue
			}
		}

		// Bold **...**
		if i+1 < n && line[i] == '*' && line[i+1] == '*' {
			end := strings.Index(line[i+2:], "**")
			if end >= 0 {
				boldText := line[i+2 : i+2+end]
				b.WriteString(bold.Render("**" + boldText + "**"))
				i = i + 2 + end + 2
				continue
			}
		}

		b.WriteString(normal.Render(string(line[i])))
		i++
	}

	return b.String()
}
