package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// researchResultMsg carries the output of a Claude Code research run.
type researchResultMsg struct {
	output    string
	err       error
	filesHint []string // files Claude created (parsed from output)
}

// researchPhase tracks the current stage of research.
type researchPhase int

const (
	researchInput researchPhase = iota
	researchRunning
	researchDone
	researchError
)

// researchMode identifies which agent mode is active.
type researchMode int

const (
	modeResearch      researchMode = iota // Deep Dive Research
	modeFollowUp                          // Follow-Up Research
	modeVaultAnalyzer                     // Vault Analyzer
	modeNoteEnhancer                      // Note Enhancer
	modeDailyDigest                       // Daily Digest Generator
)

// ResearchAgent is an overlay that invokes Claude Code CLI to research a topic
// and generate structured notes in the vault.
type ResearchAgent struct {
	active    bool
	width     int
	height    int
	phase     researchPhase
	topic     string
	output    string
	errorMsg  string
	scroll    int
	vaultRoot string

	// Options
	depth        int // 0=quick, 1=standard, 2=deep
	format       int // 0=zettelkasten, 1=outline, 2=study
	profile      int // 0=general, 1=academic, 2=technical, 3=creative
	sourceFilter int // 0=any, 1=web, 2=docs, 3=papers

	// Selection
	focusField int // 0=topic, 1=depth, 2=format, 3=profile, 4=source, 5=run button

	// Created files
	createdFiles []string
	selectedFile int

	// Elapsed time
	startTime time.Time
	elapsed   string

	// Background state — research keeps running even when overlay is closed
	running    bool
	cancelFunc context.CancelFunc // cancel running research process

	// Follow-up mode — build upon existing research
	followUp        bool
	followUpContext string
	followUpSource  string // original note path (relative)

	// Agent mode
	mode researchMode

	// Vault Analyzer state
	vaultNoteList []string // list of all note relative paths

	// Note Enhancer state
	enhanceNotePath    string   // relative path of note to enhance
	enhanceNoteContent string   // current content of note to enhance
	enhanceVaultNames  []string // all vault note names for wikilink suggestions

	// Daily Digest state
	recentNotes map[string]string // path → content for recently modified notes
}

// NewResearchAgent creates a new research agent overlay.
func NewResearchAgent() ResearchAgent {
	return ResearchAgent{
		depth:  1,
		format: 0,
	}
}

// IsActive returns whether the overlay is visible.
func (r ResearchAgent) IsActive() bool {
	return r.active
}

// IsRunning returns whether a research task is in progress (even if overlay is closed).
func (r ResearchAgent) IsRunning() bool {
	return r.running
}

// StatusText returns a short status string for the status bar.
func (r ResearchAgent) StatusText() string {
	if !r.running {
		return ""
	}
	var prefix string
	switch r.mode {
	case modeVaultAnalyzer:
		prefix = "Analyzing"
	case modeNoteEnhancer:
		prefix = "Enhancing"
	case modeDailyDigest:
		prefix = "Digest"
	default:
		prefix = ""
	}
	topic := r.topic
	if len(topic) > 20 {
		topic = topic[:20] + "…"
	}
	dots := int(time.Since(r.startTime).Seconds()) % 4
	label := topic
	if prefix != "" {
		label = prefix + ": " + topic
		if len(label) > 28 {
			label = label[:28] + "…"
		}
	}
	return IconBotChar + " " + label + strings.Repeat(".", dots+1)
}

// Reopen shows the overlay again (e.g. to check on running/completed research).
func (r *ResearchAgent) Reopen() {
	r.active = true
}

// SetSize updates dimensions.
func (r *ResearchAgent) SetSize(w, h int) {
	r.width = w
	r.height = h
}

// Open activates the research overlay.
func (r *ResearchAgent) Open(vaultRoot string) {
	r.active = true
	r.phase = researchInput
	r.topic = ""
	r.output = ""
	r.errorMsg = ""
	r.scroll = 0
	r.vaultRoot = vaultRoot
	r.focusField = 0
	r.createdFiles = nil
	r.selectedFile = 0
	r.elapsed = ""
	r.followUp = false
	r.followUpContext = ""
	r.followUpSource = ""
	r.mode = modeResearch
	r.vaultNoteList = nil
	r.enhanceNotePath = ""
	r.enhanceNoteContent = ""
	r.enhanceVaultNames = nil
	r.recentNotes = nil
}

// OpenFollowUp activates the research overlay in follow-up mode, pre-filling
// the topic from the note title and storing existing content as context.
func (r *ResearchAgent) OpenFollowUp(vaultRoot, notePath, noteContent string) {
	r.active = true
	r.phase = researchInput
	r.output = ""
	r.errorMsg = ""
	r.scroll = 0
	r.vaultRoot = vaultRoot
	r.focusField = 0
	r.createdFiles = nil
	r.selectedFile = 0
	r.elapsed = ""

	// Follow-up state
	r.followUp = true
	r.followUpContext = noteContent
	r.followUpSource = notePath
	r.mode = modeFollowUp

	// Clear other mode state
	r.vaultNoteList = nil
	r.enhanceNotePath = ""
	r.enhanceNoteContent = ""
	r.enhanceVaultNames = nil
	r.recentNotes = nil

	// Pre-fill topic from note filename (strip .md extension)
	name := strings.TrimSuffix(filepath.Base(notePath), ".md")
	r.topic = name
}

// OpenVaultAnalyzer activates the research overlay in vault analyzer mode.
// noteList should contain relative paths of all notes in the vault.
func (r *ResearchAgent) OpenVaultAnalyzer(vaultRoot string, noteList []string) {
	r.active = true
	r.phase = researchInput
	r.topic = "Vault Analysis"
	r.output = ""
	r.errorMsg = ""
	r.scroll = 0
	r.vaultRoot = vaultRoot
	r.focusField = 3 // focus on run button since no topic input needed
	r.createdFiles = nil
	r.selectedFile = 0
	r.elapsed = ""
	r.followUp = false
	r.followUpContext = ""
	r.followUpSource = ""
	r.mode = modeVaultAnalyzer
	r.depth = 1
	r.format = 0
	r.vaultNoteList = noteList
	r.enhanceNotePath = ""
	r.enhanceNoteContent = ""
	r.enhanceVaultNames = nil
	r.recentNotes = nil
}

// OpenNoteEnhancer activates the research overlay in note enhancer mode.
// notePath is the relative path, noteContent is current content, vaultNoteNames
// lists all note names for wikilink suggestions.
func (r *ResearchAgent) OpenNoteEnhancer(vaultRoot, notePath, noteContent string, vaultNoteNames []string) {
	r.active = true
	r.phase = researchInput
	r.output = ""
	r.errorMsg = ""
	r.scroll = 0
	r.vaultRoot = vaultRoot
	r.focusField = 3 // focus on run button
	r.createdFiles = nil
	r.selectedFile = 0
	r.elapsed = ""
	r.followUp = false
	r.followUpContext = ""
	r.followUpSource = ""
	r.mode = modeNoteEnhancer
	r.depth = 1
	r.format = 0
	r.enhanceNotePath = notePath
	r.enhanceNoteContent = noteContent
	r.enhanceVaultNames = vaultNoteNames
	r.vaultNoteList = nil
	r.recentNotes = nil

	// Pre-fill topic from note filename
	name := strings.TrimSuffix(filepath.Base(notePath), ".md")
	r.topic = name
}

// OpenDailyDigest activates the research overlay in daily digest mode.
// recentNotes maps relative path → content for notes modified in the last 7 days.
func (r *ResearchAgent) OpenDailyDigest(vaultRoot string, recentNotes map[string]string) {
	r.active = true
	r.phase = researchInput
	r.topic = "Weekly Review"
	r.output = ""
	r.errorMsg = ""
	r.scroll = 0
	r.vaultRoot = vaultRoot
	r.focusField = 3 // focus on run button
	r.createdFiles = nil
	r.selectedFile = 0
	r.elapsed = ""
	r.followUp = false
	r.followUpContext = ""
	r.followUpSource = ""
	r.mode = modeDailyDigest
	r.depth = 1
	r.format = 0
	r.vaultNoteList = nil
	r.enhanceNotePath = ""
	r.enhanceNoteContent = ""
	r.enhanceVaultNames = nil
	r.recentNotes = recentNotes
}

// Close hides the overlay.
func (r *ResearchAgent) Close() {
	r.active = false
}

// GetSelectedFile returns the file the user selected from results.
func (r *ResearchAgent) GetSelectedFile() (string, bool) {
	if r.phase == researchDone && len(r.createdFiles) > 0 && r.selectedFile < len(r.createdFiles) {
		path := r.createdFiles[r.selectedFile]
		return path, true
	}
	return "", false
}

// findClaude locates the claude CLI binary.
func findClaude() string {
	paths := []string{
		"claude",
		"/usr/local/bin/claude",
		"/usr/bin/claude",
	}
	if home, err := exec.Command("sh", "-c", "echo $HOME").Output(); err == nil {
		h := strings.TrimSpace(string(home))
		paths = append([]string{
			filepath.Join(h, ".local", "bin", "claude"),
			filepath.Join(h, ".claude", "local", "claude"),
		}, paths...)
	}
	for _, p := range paths {
		if fullPath, err := exec.LookPath(p); err == nil {
			return fullPath
		}
	}
	return ""
}

// loadProjectContext reads CLAUDE.md and .claude/settings.json from the vault
// root to provide Claude Code with project-specific context.
func loadProjectContext(vaultRoot string) string {
	var ctx strings.Builder

	// Check for CLAUDE.md in vault root
	claudeMdPath := filepath.Join(vaultRoot, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMdPath); err == nil && len(data) > 0 {
		ctx.WriteString("\n\nPROJECT CONTEXT (from CLAUDE.md):\n")
		ctx.WriteString(string(data))
		ctx.WriteString("\n")
	}

	// Check for .claude/settings.json
	claudeSettingsPath := filepath.Join(vaultRoot, ".claude", "settings.json")
	if data, err := os.ReadFile(claudeSettingsPath); err == nil && len(data) > 0 {
		ctx.WriteString("\n\nCLAUDE SETTINGS:\n")
		ctx.WriteString(string(data))
		ctx.WriteString("\n")
	}

	return ctx.String()
}

// loadSoulNote reads .granit/soul-note.md from the vault to shape the tone
// and style of research output.
func loadSoulNote(vaultRoot string) string {
	soulNotePath := filepath.Join(vaultRoot, ".granit", "soul-note.md")
	data, err := os.ReadFile(soulNotePath)
	if err != nil || len(data) == 0 {
		return ""
	}
	return "\n\nWrite in the following style and voice:\n" + string(data) + "\n"
}

// runResearch launches claude code to research a topic and create notes.
func (r *ResearchAgent) runResearch() tea.Cmd {
	topic := r.topic
	vaultRoot := r.vaultRoot
	depth := r.depth
	format := r.format
	profile := r.profile
	sourceFilter := r.sourceFilter
	startTime := r.startTime
	followUp := r.followUp
	followUpContext := r.followUpContext
	followUpSource := r.followUpSource
	mode := r.mode
	vaultNoteList := r.vaultNoteList
	enhanceNotePath := r.enhanceNotePath
	enhanceNoteContent := r.enhanceNoteContent
	enhanceVaultNames := r.enhanceVaultNames
	recentNotes := r.recentNotes

	// Create a context with 10-minute timeout for the research process
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	r.cancelFunc = cancel

	return func() tea.Msg {
		defer cancel()

		claudePath := findClaude()
		if claudePath == "" {
			return researchResultMsg{
				err: fmt.Errorf("claude CLI not found - install Claude Code first"),
			}
		}

		var prompt string
		switch mode {
		case modeFollowUp:
			prompt = buildFollowUpPrompt(topic, vaultRoot, depth, format, followUpContext, followUpSource)
		case modeVaultAnalyzer:
			prompt = buildVaultAnalyzerPrompt(vaultRoot, vaultNoteList, depth)
		case modeNoteEnhancer:
			prompt = buildNoteEnhancerPrompt(vaultRoot, enhanceNotePath, enhanceNoteContent, enhanceVaultNames)
		case modeDailyDigest:
			prompt = buildDailyDigestPrompt(vaultRoot, recentNotes)
		default:
			if followUp {
				prompt = buildFollowUpPrompt(topic, vaultRoot, depth, format, followUpContext, followUpSource)
			} else {
				prompt = buildResearchPrompt(topic, vaultRoot, depth, format, profile, sourceFilter)
			}
		}

		cmd := exec.CommandContext(ctx, claudePath,
			"-p", prompt,
			"--output-format", "text",
			"--allowedTools", "Bash(find:*,ls:*,cat:*) Read Write WebSearch WebFetch Glob Grep",
			"--add-dir", vaultRoot,
		)

		cmd.Env = append(cmd.Environ(), "CLAUDECODE=")

		output, err := cmd.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			return researchResultMsg{
				output: string(output),
				err:    fmt.Errorf("research timed out after 10 minutes"),
			}
		}
		if ctx.Err() == context.Canceled {
			return researchResultMsg{
				output: string(output),
				err:    fmt.Errorf("research cancelled by user"),
			}
		}
		if err != nil {
			return researchResultMsg{
				output: string(output),
				err:    fmt.Errorf("claude exited with error: %w", err),
			}
		}

		files := parseCreatedFiles(string(output), vaultRoot)

		// Append to Research Log if this was a research or follow-up run
		if mode == modeResearch || mode == modeFollowUp {
			appendResearchLog(vaultRoot, topic, depth, format, profile, sourceFilter, len(files), time.Since(startTime))
		}

		return researchResultMsg{
			output:    string(output),
			filesHint: files,
		}
	}
}

// buildResearchPrompt creates the prompt for Claude Code.
func buildResearchPrompt(topic, vaultRoot string, depth, format, profile, sourceFilter int) string {
	today := time.Now().Format("2006-01-02")

	safeTopic := strings.ReplaceAll(topic, "/", "-")
	safeTopic = strings.ReplaceAll(safeTopic, "\\", "-")
	if len(safeTopic) > 50 {
		safeTopic = safeTopic[:50]
	}
	folder := filepath.Join(vaultRoot, "Research", fmt.Sprintf("%s %s", safeTopic, today))

	var noteCount string
	switch depth {
	case 0:
		noteCount = "5-8 concise notes"
	case 1:
		noteCount = "10-15 detailed notes"
	case 2:
		noteCount = "15-25 comprehensive notes covering every aspect"
	}

	var formatInstr string
	switch format {
	case 0:
		formatInstr = `Use Zettelkasten-style atomic notes. Each note should cover ONE concept or idea.
Use descriptive titles. Link notes extensively with [[wikilinks]].
Create a hub/MOC (Map of Content) note as _Index.md that links to all other notes.`
	case 1:
		formatInstr = `Use a hierarchical outline structure. Create a main overview note as _Index.md,
then create sub-topic notes organized by category. Use [[wikilinks]] to connect them.
Each note can cover broader topics than Zettelkasten but should still be focused.`
	case 2:
		formatInstr = `Create study-oriented notes optimized for learning. Each note should include:
- Clear explanations with examples
- Key takeaways in bullet points
- Flashcard-ready Q&A pairs formatted as "Q: question" / "A: answer" blocks
- Practice exercises or thought experiments where applicable
Create a hub note as _Index.md with a suggested study order.
Use [[wikilinks]] extensively.`
	}

	// Profile-specific instructions
	var profileInstr string
	switch profile {
	case 1: // Academic
		profileInstr = `RESEARCH PROFILE — Academic:
- Use formal academic writing style throughout
- Emphasize peer-reviewed sources and scholarly references
- Include a bibliography/references section in each note
- Cite sources using author-date format (e.g., Smith, 2024)
- Discuss methodology, limitations, and research gaps
- Prioritize precision and nuance over simplicity`
	case 2: // Technical
		profileInstr = `RESEARCH PROFILE — Technical:
- Include code examples and implementation details where relevant
- Use architecture diagrams described in Markdown (ASCII or Mermaid-style)
- Cover APIs, libraries, frameworks, and tooling
- Discuss performance considerations and trade-offs
- Include configuration snippets and practical setup steps
- Prioritize actionable, hands-on content`
	case 3: // Creative
		profileInstr = `RESEARCH PROFILE — Creative:
- Use brainstorming and exploratory writing style
- Present alternative perspectives and unconventional angles
- Use mind-map style organization with branching ideas
- Include thought experiments and "what if" scenarios
- Draw connections across different domains and disciplines
- Prioritize divergent thinking and idea generation`
	default: // General
		profileInstr = `RESEARCH PROFILE — General:
- Use a balanced, well-rounded research approach
- Mix breadth and depth appropriately
- Write in clear, accessible language
- Include both overview and detailed analysis`
	}

	// Source filter instructions
	var sourceInstr string
	switch sourceFilter {
	case 1: // Web
		sourceInstr = "SOURCE FOCUS: Search the general web for up-to-date articles, blog posts, tutorials, and news."
	case 2: // Docs
		sourceInstr = "SOURCE FOCUS: Prioritize official documentation, MDN, language references, API docs, and authoritative technical sources."
	case 3: // Papers
		sourceInstr = "SOURCE FOCUS: Prioritize academic papers, arxiv preprints, research publications, and scholarly articles. Include DOIs where available."
	default: // Any
		sourceInstr = "SOURCE FOCUS: Use any reliable sources — web articles, documentation, papers, books, etc."
	}

	prompt := fmt.Sprintf(`You are a research assistant creating structured knowledge notes.

TOPIC: %s

%s

%s

INSTRUCTIONS:
1. Research this topic thoroughly using web search to find current, accurate information.
2. Create %s in the folder: %s
3. Create the folder if it doesn't exist.
4. %s

FORMAT for each note:
- Start with YAML frontmatter: ---\ndate: %s\ntype: research\ntags: [research, <relevant-tags>]\nsource: <url-if-applicable>\n---
- Use Markdown with proper headings (## for sections)
- Include [[wikilinks]] to other notes you create (use just the filename without .md)
- Be thorough, accurate, and cite sources where possible

IMPORTANT:
- Create ALL files using the Write tool
- The _Index.md hub note should be created LAST and link to everything
- Each filename should be descriptive (e.g., "Concept - Neural Networks.md")
- Do NOT create files outside the research folder
- After creating all files, list the files you created`, topic, profileInstr, sourceInstr, noteCount, folder, formatInstr, today)

	// Append project context from CLAUDE.md and .claude/settings.json
	prompt += loadProjectContext(vaultRoot)

	// Append soul note for tone/style guidance
	prompt += loadSoulNote(vaultRoot)

	prompt += "\n\nSTART RESEARCHING NOW."
	return prompt
}

// buildFollowUpPrompt creates the prompt for follow-up research that builds
// upon existing notes.
func buildFollowUpPrompt(topic, vaultRoot string, depth, format int, existingContent, sourcePath string) string {
	today := time.Now().Format("2006-01-02")

	// Determine the folder — reuse the existing Research folder from the source note
	folder := filepath.Join(vaultRoot, filepath.Dir(sourcePath))

	var noteCount string
	switch depth {
	case 0:
		noteCount = "3-5 concise follow-up notes"
	case 1:
		noteCount = "5-10 detailed follow-up notes"
	case 2:
		noteCount = "10-15 comprehensive follow-up notes covering every sub-aspect"
	}

	var formatInstr string
	switch format {
	case 0:
		formatInstr = `Use Zettelkasten-style atomic notes. Each note should cover ONE concept or idea.
Use descriptive titles. Link notes extensively with [[wikilinks]].
Update the existing _Index.md hub note to include the new notes alongside existing entries.`
	case 1:
		formatInstr = `Use a hierarchical outline structure. Create sub-topic notes organized by category.
Use [[wikilinks]] to connect them. Update the existing _Index.md to include new notes.`
	case 2:
		formatInstr = `Create study-oriented notes optimized for learning. Each note should include:
- Clear explanations with examples
- Key takeaways in bullet points
- Flashcard-ready Q&A pairs formatted as "Q: question" / "A: answer" blocks
- Practice exercises or thought experiments where applicable
Update the existing _Index.md with the new notes and a suggested study order.
Use [[wikilinks]] extensively.`
	}

	// Truncate existing content if it's very long to stay within prompt limits
	contextSnippet := existingContent
	if len(contextSnippet) > 8000 {
		contextSnippet = contextSnippet[:8000] + "\n\n[... content truncated for brevity ...]"
	}

	prompt := fmt.Sprintf(`You are a research assistant performing FOLLOW-UP research to go deeper on an existing topic.

TOPIC: %s

EXISTING RESEARCH (from %s):
---
%s
---

INSTRUCTIONS:
1. Read the existing research above carefully to understand what has already been covered.
2. Research this topic MORE DEEPLY using web search — focus on:
   - Sub-topics not yet covered in the existing notes
   - Recent developments or updates since the existing research
   - Deeper technical details, edge cases, or advanced concepts
   - Related topics, connections, and cross-disciplinary insights
   - Counterarguments, criticisms, or alternative perspectives
3. Create %s in the folder: %s
4. Do NOT duplicate content that already exists in the folder — check existing files first.
5. %s

FORMAT for each note:
- Start with YAML frontmatter: ---\ndate: %s\ntype: research\ntags: [research, follow-up, <relevant-tags>]\nsource: <url-if-applicable>\n---
- Use Markdown with proper headings (## for sections)
- Include [[wikilinks]] to other notes (both new and existing ones)
- Be thorough, accurate, and cite sources where possible
- Write in a clear, educational style

IMPORTANT:
- Create ALL new files using the Write tool
- Read the existing _Index.md first, then UPDATE it to include links to your new notes (keep all existing links intact)
- Each new filename should be descriptive (e.g., "Concept - Advanced Neural Architectures.md")
- Do NOT create files outside the research folder
- Do NOT overwrite existing notes — only create NEW ones and update _Index.md
- After creating all files, list the new files you created`, topic, sourcePath, contextSnippet, noteCount, folder, formatInstr, today)

	// Append project context from CLAUDE.md and .claude/settings.json
	prompt += loadProjectContext(vaultRoot)

	// Append soul note for tone/style guidance
	prompt += loadSoulNote(vaultRoot)

	prompt += "\n\nSTART FOLLOW-UP RESEARCH NOW."
	return prompt
}

// buildVaultAnalyzerPrompt creates the prompt for vault analysis mode.
// Claude Code reads the entire vault, generates a MOC, gaps report, and suggestions.
func buildVaultAnalyzerPrompt(vaultRoot string, noteList []string, depth int) string {
	today := time.Now().Format("2006-01-02")
	folder := filepath.Join(vaultRoot, "Vault Analysis "+today)

	// Build the note listing for the prompt
	var noteListStr string
	if len(noteList) > 0 {
		// Include all note paths so Claude knows what exists
		listed := noteList
		if len(listed) > 500 {
			listed = listed[:500]
			noteListStr = strings.Join(listed, "\n") + fmt.Sprintf("\n... and %d more notes", len(noteList)-500)
		} else {
			noteListStr = strings.Join(listed, "\n")
		}
	} else {
		noteListStr = "(use Glob and Read tools to discover notes)"
	}

	var depthInstr string
	switch depth {
	case 0:
		depthInstr = "Do a quick scan — focus on note titles and folder structure. Read a sample of 10-20 notes."
	case 1:
		depthInstr = "Do a thorough analysis — read all notes (or a substantial sample of 50+) to understand content and connections."
	case 2:
		depthInstr = "Do a comprehensive deep analysis — read EVERY note in the vault, analyze all content, links, and tags in detail."
	}

	prompt := fmt.Sprintf(`You are a vault analysis assistant for a knowledge management system.

VAULT ROOT: %s

KNOWN NOTES IN VAULT:
%s

INSTRUCTIONS:
1. %s
2. Use the Glob tool to find all .md files: Glob("**/*.md") in the vault directory.
3. Use the Read tool to read note contents and understand their topics, links, and structure.
4. Create the analysis output folder: %s

ANALYSIS TASKS — Create these files:

A) **_Map of Content.md** — A comprehensive MOC (Map of Content):
   - Group all notes by topic/theme
   - Create sections with headings for each topic area
   - List notes under each topic with [[wikilinks]]
   - Show connections between topic areas
   - Include a "Quick Navigation" section at the top

B) **_Gaps Report.md** — Identify knowledge gaps:
   - Topics that are mentioned or linked but don't have their own notes
   - Broken [[wikilinks]] that point to non-existent notes
   - Topic areas that seem underdeveloped compared to others
   - Notes that are orphaned (no links to or from other notes)
   - Missing connections between related notes

C) **_Suggestions.md** — Actionable suggestions:
   - Specific new notes to create (with suggested titles and brief outlines)
   - Existing notes that should be linked together but aren't
   - Notes that could be split into more atomic pieces
   - Notes that could be merged
   - Suggested folder reorganization if applicable
   - Tags that could improve discoverability

FORMAT for each analysis note:
- Start with YAML frontmatter: ---\ndate: %s\ntype: vault-analysis\ntags: [vault-analysis, meta]\n---
- Use Markdown with proper headings
- Include [[wikilinks]] to actual vault notes (use just the filename without .md)
- Be specific and actionable

IMPORTANT:
- Create ALL files using the Write tool
- Create the output folder if it doesn't exist
- Do NOT modify any existing vault notes
- After creating all files, list the files you created`, vaultRoot, noteListStr, depthInstr, folder, today)

	// Append project context from CLAUDE.md and .claude/settings.json
	prompt += loadProjectContext(vaultRoot)

	// Append soul note for tone/style guidance
	prompt += loadSoulNote(vaultRoot)

	prompt += "\n\nSTART VAULT ANALYSIS NOW."
	return prompt
}

// buildNoteEnhancerPrompt creates the prompt for note enhancement mode.
// Claude Code enhances a specific note with additional research and wikilinks.
func buildNoteEnhancerPrompt(vaultRoot, notePath, noteContent string, vaultNoteNames []string) string {
	today := time.Now().Format("2006-01-02")
	fullPath := filepath.Join(vaultRoot, notePath)
	backupPath := fullPath + ".backup-" + today

	// Build list of existing vault notes for wikilink suggestions
	var noteNamesStr string
	if len(vaultNoteNames) > 0 {
		names := vaultNoteNames
		if len(names) > 300 {
			names = names[:300]
			noteNamesStr = strings.Join(names, "\n") + fmt.Sprintf("\n... and %d more", len(vaultNoteNames)-300)
		} else {
			noteNamesStr = strings.Join(names, "\n")
		}
	} else {
		noteNamesStr = "(use Glob to discover notes)"
	}

	// Truncate very long note content
	content := noteContent
	if len(content) > 12000 {
		content = content[:12000] + "\n\n[... content truncated for brevity ...]"
	}

	prompt := fmt.Sprintf(`You are a note enhancement assistant for a knowledge management system.

NOTE TO ENHANCE: %s
VAULT ROOT: %s

CURRENT NOTE CONTENT:
---
%s
---

EXISTING VAULT NOTES (for wikilink suggestions):
%s

INSTRUCTIONS:
1. First, create a backup of the original note by writing the current content to: %s
2. Then, enhance the note by:
   a) Research the topic using WebSearch to find current, accurate information
   b) Expand on existing content — add missing sections, fill in details, provide deeper explanations
   c) Add relevant [[wikilinks]] to existing vault notes where topics overlap (use filenames without .md)
   d) Add sources and citations in a "## Sources" section at the bottom
   e) Preserve the existing YAML frontmatter but add/update the "updated" field to today's date (%s)
   f) Keep the original author's voice and style — enhance, don't rewrite from scratch
   g) Add new sections that are relevant but missing from the original
3. Write the enhanced version back to the original file: %s

ENHANCEMENT GUIDELINES:
- Keep ALL original content — only add to it, reorganize, or clarify
- Add [[wikilinks]] naturally within the text where other vault notes are relevant
- New sections should flow naturally with the existing content
- If the note has frontmatter tags, add relevant new tags
- Include a "## See Also" section at the end with related vault notes
- Add blockquotes for important definitions or key concepts
- Use proper Markdown formatting (headings, lists, bold/italic)

IMPORTANT:
- Create the backup file FIRST using Write tool
- Then write the enhanced version to the original path using Write tool
- Do NOT create any other files
- After writing, list what was changed (backup path and enhanced file path)`, notePath, vaultRoot, content, noteNamesStr, backupPath, today, fullPath)

	// Append project context from CLAUDE.md and .claude/settings.json
	prompt += loadProjectContext(vaultRoot)

	// Append soul note for tone/style guidance
	prompt += loadSoulNote(vaultRoot)

	prompt += "\n\nSTART ENHANCING NOW."
	return prompt
}

// buildDailyDigestPrompt creates the prompt for daily digest / weekly review mode.
// Claude Code analyzes recently modified notes and generates a weekly review.
func buildDailyDigestPrompt(vaultRoot string, recentNotes map[string]string) string {
	today := time.Now().Format("2006-01-02")
	reviewPath := filepath.Join(vaultRoot, fmt.Sprintf("Weekly Review %s.md", today))

	// Build summaries of recent notes
	var recentSummaries strings.Builder
	noteCount := 0
	for path, content := range recentNotes {
		noteCount++
		// Truncate long notes to keep prompt manageable
		snippet := content
		if len(snippet) > 2000 {
			snippet = snippet[:2000] + "\n[... truncated ...]"
		}
		recentSummaries.WriteString(fmt.Sprintf("\n### %s\n```\n%s\n```\n", path, snippet))

		// Safety limit for very large vaults
		if noteCount >= 50 {
			recentSummaries.WriteString(fmt.Sprintf("\n... and %d more recently modified notes (use Glob+Read to access them)\n", len(recentNotes)-50))
			break
		}
	}

	recentContent := recentSummaries.String()
	if recentContent == "" {
		recentContent = "(No recently modified notes found — use Glob to find .md files and check their modification times)"
	}

	prompt := fmt.Sprintf(`You are a weekly review assistant for a knowledge management system.

VAULT ROOT: %s
TODAY'S DATE: %s

RECENTLY MODIFIED NOTES (last 7 days):
%s

INSTRUCTIONS:
1. Analyze the recently modified notes above carefully.
2. If the note list seems incomplete, use Glob("**/*.md") and Read to find recently modified files.
3. Create a comprehensive weekly review note at: %s

THE WEEKLY REVIEW NOTE SHOULD CONTAIN:

## Activity Summary
- How many notes were modified/created this week
- Which areas/topics saw the most activity
- Brief description of what was worked on

## Key Themes
- Identify the main themes or topics across this week's notes
- Show how different notes connect to each other
- Highlight any emerging patterns or threads

## Connections Discovered
- Notes that are related but might not be linked yet
- Cross-topic connections and insights
- Suggest [[wikilinks]] between related recent notes

## Follow-Up Tasks
- Action items extracted from recent notes (look for TODO, FIXME, task lists)
- Topics that need further research or development
- Notes that feel incomplete and should be expanded
- Questions raised in recent notes that haven't been answered

## This Week at a Glance
- A brief narrative summary of the week's knowledge work
- What was learned, what was created, what evolved

FORMAT:
- Start with YAML frontmatter: ---\ndate: %s\ntype: weekly-review\ntags: [weekly-review, meta, digest]\n---
- Use Markdown with clear headings and bullet points
- Include [[wikilinks]] to all referenced notes (use filenames without .md)
- Be specific — reference actual note content, not generic observations
- Keep it actionable and useful

IMPORTANT:
- Create the review note using the Write tool
- Do NOT modify any existing notes
- If a weekly review for today already exists, overwrite it
- After creating the file, list the file path

START GENERATING WEEKLY REVIEW NOW.`, vaultRoot, today, recentContent, reviewPath, today)

	// Append project context from CLAUDE.md and .claude/settings.json
	prompt += loadProjectContext(vaultRoot)

	// Append soul note for tone/style guidance
	prompt += loadSoulNote(vaultRoot)

	return prompt
}

// appendResearchLog writes a summary line to Research Log.md in the vault root.
func appendResearchLog(vaultRoot, topic string, depth, format, profile, sourceFilter, noteCount int, elapsed time.Duration) {
	logPath := filepath.Join(vaultRoot, "Research Log.md")

	depthNames := []string{"quick", "standard", "deep"}
	formatNames := []string{"zettelkasten", "outline", "study"}
	profileNames := []string{"general", "academic", "technical", "creative"}

	depthStr := "standard"
	if depth >= 0 && depth < len(depthNames) {
		depthStr = depthNames[depth]
	}
	formatStr := "zettelkasten"
	if format >= 0 && format < len(formatNames) {
		formatStr = formatNames[format]
	}
	profileStr := "general"
	if profile >= 0 && profile < len(profileNames) {
		profileStr = profileNames[profile]
	}

	date := time.Now().Format("2006-01-02 15:04")
	elapsedStr := elapsed.Truncate(time.Second).String()
	line := fmt.Sprintf("- [%s] **%s** — %s, %s, %s — %d notes created — %s\n",
		date, topic, depthStr, formatStr, profileStr, noteCount, elapsedStr)

	// Create the file with a header if it doesn't exist
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		header := "---\ndate: " + time.Now().Format("2006-01-02") + "\ntype: research-log\ntags: [research, log, meta]\n---\n\n# Research Log\n\n"
		_ = os.WriteFile(logPath, []byte(header+line), 0644)
		return
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(line)
}

// parseCreatedFiles extracts file paths from Claude's output.
func parseCreatedFiles(output, vaultRoot string) []string {
	var files []string
	seen := make(map[string]bool)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".md") || strings.Contains(line, ".md ") || strings.Contains(line, ".md)") {
			parts := strings.Fields(line)
			for _, p := range parts {
				p = strings.Trim(p, "`,*[]()-\"'")
				if strings.HasSuffix(p, ".md") && strings.Contains(p, "/") {
					if strings.HasPrefix(p, vaultRoot) {
						rel, _ := filepath.Rel(vaultRoot, p)
						if rel != "" && !seen[rel] {
							files = append(files, rel)
							seen[rel] = true
						}
					}
				}
			}
		}
	}
	return files
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key events for the research overlay.
func (r ResearchAgent) Update(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch r.phase {
	case researchInput:
		return r.updateInput(msg)
	case researchRunning:
		if msg.String() == "esc" {
			// Cancel the running research process
			if r.cancelFunc != nil {
				r.cancelFunc()
				r.cancelFunc = nil
			}
			r.phase = researchInput
			r.running = false
			return r, nil
		}
		return r, nil
	case researchDone:
		return r.updateDone(msg)
	case researchError:
		if msg.String() == "esc" || msg.String() == "enter" {
			r.phase = researchInput
			return r, nil
		}
		return r, nil
	}
	return r, nil
}

func (r ResearchAgent) updateInput(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch r.mode {
	case modeVaultAnalyzer:
		return r.updateInputVaultAnalyzer(msg)
	case modeNoteEnhancer:
		return r.updateInputNoteEnhancer(msg)
	case modeDailyDigest:
		return r.updateInputDailyDigest(msg)
	default:
		return r.updateInputResearch(msg)
	}
}

// updateInputResearch handles input for Deep Dive and Follow-Up modes.
// Focus fields: 0=topic, 1=depth, 2=format, 3=profile, 4=source, 5=run button
func (r ResearchAgent) updateInputResearch(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc":
		r.active = false
		return r, nil
	case "tab":
		r.focusField = (r.focusField + 1) % 6
		return r, nil
	case "shift+tab":
		r.focusField = (r.focusField + 5) % 6
		return r, nil
	case "enter":
		if r.focusField == 5 && r.topic != "" {
			r.phase = researchRunning
			r.running = true
			r.startTime = time.Now()
			return r, tea.Batch(r.runResearch(), r.tickElapsed())
		}
		if r.focusField < 5 {
			r.focusField++
		}
		return r, nil
	case "left":
		if r.focusField == 1 && r.depth > 0 {
			r.depth--
		} else if r.focusField == 2 && r.format > 0 {
			r.format--
		} else if r.focusField == 3 && r.profile > 0 {
			r.profile--
		} else if r.focusField == 4 && r.sourceFilter > 0 {
			r.sourceFilter--
		}
		return r, nil
	case "right":
		if r.focusField == 1 && r.depth < 2 {
			r.depth++
		} else if r.focusField == 2 && r.format < 2 {
			r.format++
		} else if r.focusField == 3 && r.profile < 3 {
			r.profile++
		} else if r.focusField == 4 && r.sourceFilter < 3 {
			r.sourceFilter++
		}
		return r, nil
	case "backspace":
		if r.focusField == 0 && len(r.topic) > 0 {
			r.topic = r.topic[:len(r.topic)-1]
		}
		return r, nil
	default:
		if r.focusField == 0 {
			ch := msg.String()
			if len(ch) == 1 && ch[0] >= 32 {
				r.topic += ch
			} else if ch == "space" {
				r.topic += " "
			}
		}
		return r, nil
	}
}

// updateInputVaultAnalyzer handles input for vault analyzer mode.
// Fields: 1=depth, 3=run button
func (r ResearchAgent) updateInputVaultAnalyzer(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc":
		r.active = false
		return r, nil
	case "tab":
		if r.focusField == 1 {
			r.focusField = 3
		} else {
			r.focusField = 1
		}
		return r, nil
	case "shift+tab":
		if r.focusField == 3 {
			r.focusField = 1
		} else {
			r.focusField = 3
		}
		return r, nil
	case "enter":
		if r.focusField == 3 {
			r.phase = researchRunning
			r.running = true
			r.startTime = time.Now()
			return r, tea.Batch(r.runResearch(), r.tickElapsed())
		}
		r.focusField = 3
		return r, nil
	case "left":
		if r.focusField == 1 && r.depth > 0 {
			r.depth--
		}
		return r, nil
	case "right":
		if r.focusField == 1 && r.depth < 2 {
			r.depth++
		}
		return r, nil
	}
	return r, nil
}

// updateInputNoteEnhancer handles input for note enhancer mode.
// Only has the run button (field 3).
func (r ResearchAgent) updateInputNoteEnhancer(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc":
		r.active = false
		return r, nil
	case "enter":
		r.phase = researchRunning
		r.running = true
		r.startTime = time.Now()
		return r, tea.Batch(r.runResearch(), r.tickElapsed())
	}
	return r, nil
}

// updateInputDailyDigest handles input for daily digest mode.
// Only has the run button (field 3).
func (r ResearchAgent) updateInputDailyDigest(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc":
		r.active = false
		return r, nil
	case "enter":
		r.phase = researchRunning
		r.running = true
		r.startTime = time.Now()
		return r, tea.Batch(r.runResearch(), r.tickElapsed())
	}
	return r, nil
}

type researchTickMsg struct{}

func (r *ResearchAgent) tickElapsed() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return researchTickMsg{}
	})
}

func (r ResearchAgent) updateDone(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		r.active = false
		return r, nil
	case "up", "k":
		if r.selectedFile > 0 {
			r.selectedFile--
		}
		return r, nil
	case "down", "j":
		if r.selectedFile < len(r.createdFiles)-1 {
			r.selectedFile++
		}
		return r, nil
	case "enter":
		r.active = false
		return r, nil
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (r ResearchAgent) overlayWidth() int {
	w := r.width * 2 / 3
	if w < 56 {
		w = 56
	}
	if w > 80 {
		w = 80
	}
	return w
}

// View renders the research overlay.
func (r ResearchAgent) View() string {
	w := r.overlayWidth()
	innerW := w - 6 // padding + border

	var body string
	switch r.phase {
	case researchInput:
		body = r.viewInput(innerW)
	case researchRunning:
		body = r.viewRunning(innerW)
	case researchDone:
		body = r.viewDone(innerW)
	case researchError:
		body = r.viewError(innerW)
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Background(mantle).
		Padding(1, 2).
		Width(w)

	return border.Render(body)
}

// ---------------------------------------------------------------------------
// Input view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewInput(innerW int) string {
	switch r.mode {
	case modeVaultAnalyzer:
		return r.viewInputVaultAnalyzer(innerW)
	case modeNoteEnhancer:
		return r.viewInputNoteEnhancer(innerW)
	case modeDailyDigest:
		return r.viewInputDailyDigest(innerW)
	default:
		return r.viewInputResearch(innerW)
	}
}

// viewInputResearch renders the input form for Deep Dive and Follow-Up modes.
func (r ResearchAgent) viewInputResearch(innerW int) string {
	var b strings.Builder

	// Title — changes based on mode
	if r.followUp {
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
			Render("  Follow-Up Research"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).
			Render("  Deep Dive Research"))
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Powered by Claude Code"))
	b.WriteString("\n\n")

	// ── Follow-up context preview ──
	if r.followUp && r.followUpContext != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lavender).Bold(true).
			Render("  Source Note"))
		b.WriteString("\n")
		srcName := strings.TrimSuffix(filepath.Base(r.followUpSource), ".md")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Render(srcName))
		b.WriteString("\n")

		// Show a preview of the context (first few lines, excluding frontmatter)
		preview := r.contextPreview(innerW - 6)
		if preview != "" {
			previewBox := lipgloss.NewStyle().
				Foreground(overlay0).
				Width(innerW - 4).
				Render(preview)
			b.WriteString("  " + previewBox)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// ── Topic ──
	label := r.fieldLabel("Topic", 0)
	b.WriteString(label + "\n")

	topicText := r.topic
	if r.focusField == 0 {
		topicText += "█"
	}
	if topicText == "" {
		topicText = DimStyle.Render("Type a research topic...")
	}
	inputW := innerW - 4
	if inputW < 20 {
		inputW = 20
	}
	inputBox := lipgloss.NewStyle().
		Background(surface0).
		Foreground(text).
		Width(inputW).
		Padding(0, 1).
		Render(topicText)
	b.WriteString("  " + inputBox + "\n\n")

	// ── Depth ──
	depthLabels := []string{"Quick", "Standard", "Deep"}
	depthDescs := []string{"5-8 notes", "10-15 notes", "15-25 notes"}
	b.WriteString(r.fieldLabel("Depth", 1) + "\n")
	b.WriteString(r.renderRadio(depthLabels, depthDescs, r.depth, r.focusField == 1, innerW))
	b.WriteString("\n\n")

	// ── Format ──
	fmtLabels := []string{"Zettelkasten", "Outline", "Study Guide"}
	fmtDescs := []string{"atomic notes", "hierarchical", "with flashcards"}
	b.WriteString(r.fieldLabel("Format", 2) + "\n")
	b.WriteString(r.renderRadio(fmtLabels, fmtDescs, r.format, r.focusField == 2, innerW))
	b.WriteString("\n\n")

	// ── Profile ──
	profileLabels := []string{"General", "Academic", "Technical", "Creative"}
	profileDescs := []string{"balanced", "citations", "code+arch", "brainstorm"}
	b.WriteString(r.fieldLabel("Profile", 3) + "\n")
	b.WriteString(r.renderRadio(profileLabels, profileDescs, r.profile, r.focusField == 3, innerW))
	b.WriteString("\n\n")

	// ── Source ──
	sourceLabels := []string{"Any", "Web", "Docs", "Papers"}
	sourceDescs := []string{"no filter", "general web", "official docs", "academic"}
	b.WriteString(r.fieldLabel("Source", 4) + "\n")
	b.WriteString(r.renderRadio(sourceLabels, sourceDescs, r.sourceFilter, r.focusField == 4, innerW))
	b.WriteString("\n\n")

	// ── Button ──
	if r.topic != "" {
		btnColor := surface0
		btnFg := text
		if r.focusField == 5 {
			btnColor = green
			btnFg = mantle
		}
		btnLabel := " Start Research "
		if r.followUp {
			btnLabel = " Start Follow-Up "
		}
		btn := lipgloss.NewStyle().
			Background(btnColor).
			Foreground(btnFg).
			Bold(r.focusField == 5).
			Padding(0, 3).
			Render(btnLabel)
		b.WriteString("  " + btn)
	} else {
		b.WriteString("  " + DimStyle.Render("Enter a topic to begin"))
	}
	b.WriteString("\n\n")

	// ── Help ──
	b.WriteString(DimStyle.Render("  Tab switch  ←→ option  Enter confirm  Esc close"))

	return b.String()
}

// viewInputVaultAnalyzer renders the input form for vault analyzer mode.
func (r ResearchAgent) viewInputVaultAnalyzer(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(teal).Bold(true).
		Render("  Vault Analyzer"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Powered by Claude Code"))
	b.WriteString("\n\n")

	// ── Description ──
	descStyle := lipgloss.NewStyle().Foreground(lavender)
	b.WriteString(descStyle.Render("  Analyze your entire vault to generate:"))
	b.WriteString("\n")
	itemStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Map of Content linking notes by topic"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Gaps report for missing topics"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Suggestions for new notes & links"))
	b.WriteString("\n\n")

	// ── Vault info ──
	noteCountStr := fmt.Sprintf("%d notes", len(r.vaultNoteList))
	b.WriteString(lipgloss.NewStyle().Foreground(lavender).Bold(true).
		Render("  Vault"))
	b.WriteString("\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Render(noteCountStr+" in "+filepath.Base(r.vaultRoot)))
	b.WriteString("\n\n")

	// ── Depth (reuse field index 1) ──
	depthLabels := []string{"Quick", "Standard", "Deep"}
	depthDescs := []string{"sample scan", "thorough", "every note"}
	b.WriteString(r.fieldLabel("Analysis Depth", 1) + "\n")
	b.WriteString(r.renderRadio(depthLabels, depthDescs, r.depth, r.focusField == 1, innerW))
	b.WriteString("\n\n")

	// ── Button ──
	btnColor := surface0
	btnFg := text
	if r.focusField == 3 {
		btnColor = green
		btnFg = mantle
	}
	btn := lipgloss.NewStyle().
		Background(btnColor).
		Foreground(btnFg).
		Bold(r.focusField == 3).
		Padding(0, 3).
		Render(" Analyze Vault ")
	b.WriteString("  " + btn)
	b.WriteString("\n\n")

	// ── Help ──
	b.WriteString(DimStyle.Render("  Tab switch  ←→ depth  Enter confirm  Esc close"))

	return b.String()
}

// viewInputNoteEnhancer renders the input form for note enhancer mode.
func (r ResearchAgent) viewInputNoteEnhancer(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(pink).Bold(true).
		Render("  Note Enhancer"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Powered by Claude Code"))
	b.WriteString("\n\n")

	// ── Description ──
	descStyle := lipgloss.NewStyle().Foreground(lavender)
	b.WriteString(descStyle.Render("  Enhance your note with:"))
	b.WriteString("\n")
	itemStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Web research to expand content"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Missing sections and details"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Wikilinks to existing vault notes"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Sources and citations"))
	b.WriteString("\n\n")

	// ── Target note ──
	b.WriteString(lipgloss.NewStyle().Foreground(lavender).Bold(true).
		Render("  Target Note"))
	b.WriteString("\n")
	noteName := strings.TrimSuffix(filepath.Base(r.enhanceNotePath), ".md")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Render(noteName))
	b.WriteString("\n")

	// Preview of current content
	preview := r.enhancerPreview(innerW - 6)
	if preview != "" {
		previewBox := lipgloss.NewStyle().
			Foreground(overlay0).
			Width(innerW - 4).
			Render(preview)
		b.WriteString("  " + previewBox)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// ── Info ──
	infoStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(infoStyle.Render("  A backup will be created before changes."))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf("  %d vault notes available for linking.", len(r.enhanceVaultNames))))
	b.WriteString("\n\n")

	// ── Button ──
	btnColor := surface0
	btnFg := text
	if r.focusField == 3 {
		btnColor = green
		btnFg = mantle
	}
	btn := lipgloss.NewStyle().
		Background(btnColor).
		Foreground(btnFg).
		Bold(r.focusField == 3).
		Padding(0, 3).
		Render(" Enhance Note ")
	b.WriteString("  " + btn)
	b.WriteString("\n\n")

	// ── Help ──
	b.WriteString(DimStyle.Render("  Enter confirm  Esc close"))

	return b.String()
}

// viewInputDailyDigest renders the input form for daily digest mode.
func (r ResearchAgent) viewInputDailyDigest(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).
		Render("  Daily Digest Generator"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Powered by Claude Code"))
	b.WriteString("\n\n")

	// ── Description ──
	descStyle := lipgloss.NewStyle().Foreground(lavender)
	b.WriteString(descStyle.Render("  Generate a weekly review from recent activity:"))
	b.WriteString("\n")
	itemStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Summary of recent changes"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Connections between modified notes"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Suggested follow-up tasks"))
	b.WriteString("\n")
	b.WriteString(itemStyle.Render("  " + ThemeAccentBar + " Weekly review note"))
	b.WriteString("\n\n")

	// ── Recent activity ──
	b.WriteString(lipgloss.NewStyle().Foreground(lavender).Bold(true).
		Render("  Recent Activity"))
	b.WriteString("\n")
	noteCount := len(r.recentNotes)
	if noteCount > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).
			Render(fmt.Sprintf("%d notes modified in the last 7 days", noteCount)))
		b.WriteString("\n")

		// Show a few recent note names
		shown := 0
		for path := range r.recentNotes {
			if shown >= 5 {
				remaining := noteCount - shown
				if remaining > 0 {
					b.WriteString(DimStyle.Render(fmt.Sprintf("    ... and %d more", remaining)))
					b.WriteString("\n")
				}
				break
			}
			name := strings.TrimSuffix(filepath.Base(path), ".md")
			if len(name) > innerW-8 {
				name = name[:innerW-9] + "…"
			}
			b.WriteString(DimStyle.Render("    " + name))
			b.WriteString("\n")
			shown++
		}
	} else {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(overlay0).
			Render("No recently modified notes found"))
		b.WriteString("\n")
		b.WriteString("  " + DimStyle.Render("Claude will scan the vault for recent changes"))
	}
	b.WriteString("\n")

	// ── Button ──
	btnColor := surface0
	btnFg := text
	if r.focusField == 3 {
		btnColor = green
		btnFg = mantle
	}
	btn := lipgloss.NewStyle().
		Background(btnColor).
		Foreground(btnFg).
		Bold(r.focusField == 3).
		Padding(0, 3).
		Render(" Generate Digest ")
	b.WriteString("  " + btn)
	b.WriteString("\n\n")

	// ── Help ──
	b.WriteString(DimStyle.Render("  Enter confirm  Esc close"))

	return b.String()
}

// enhancerPreview returns a short preview of the note to be enhanced.
func (r ResearchAgent) enhancerPreview(maxWidth int) string {
	content := r.enhanceNoteContent
	if content == "" {
		return ""
	}

	// Strip YAML frontmatter
	if strings.HasPrefix(content, "---") {
		if end := strings.Index(content[3:], "---"); end >= 0 {
			content = strings.TrimSpace(content[end+6:])
		}
	}

	lines := strings.Split(content, "\n")
	var preview []string
	maxLines := 4
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > maxWidth {
			trimmed = trimmed[:maxWidth-1] + "…"
		}
		preview = append(preview, "  "+trimmed)
		if len(preview) >= maxLines {
			break
		}
	}
	if len(preview) == 0 {
		return ""
	}
	result := strings.Join(preview, "\n")
	if len(lines) > maxLines {
		result += "\n  ..."
	}
	return result
}

// contextPreview returns a short preview of the follow-up source note content,
// skipping YAML frontmatter and showing the first few meaningful lines.
func (r ResearchAgent) contextPreview(maxWidth int) string {
	content := r.followUpContext
	if content == "" {
		return ""
	}

	// Strip YAML frontmatter
	if strings.HasPrefix(content, "---") {
		if end := strings.Index(content[3:], "---"); end >= 0 {
			content = strings.TrimSpace(content[end+6:])
		}
	}

	lines := strings.Split(content, "\n")
	var preview []string
	maxLines := 4
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Truncate long lines
		if len(trimmed) > maxWidth {
			trimmed = trimmed[:maxWidth-1] + "…"
		}
		preview = append(preview, "  "+trimmed)
		if len(preview) >= maxLines {
			break
		}
	}
	if len(preview) == 0 {
		return ""
	}
	result := strings.Join(preview, "\n")
	if len(lines) > maxLines {
		result += "\n  ..."
	}
	return result
}

// fieldLabel returns a styled label, highlighted when focused.
func (r ResearchAgent) fieldLabel(name string, idx int) string {
	style := DimStyle
	if r.focusField == idx {
		style = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}
	return style.Render("  " + name)
}

// renderRadio renders a horizontal radio-button selector.
func (r ResearchAgent) renderRadio(labels, descs []string, selected int, focused bool, maxW int) string {
	var parts []string
	for i, label := range labels {
		desc := ""
		if i < len(descs) {
			desc = " " + descs[i]
		}
		full := label + desc

		if i == selected {
			bg := mauve
			if focused {
				bg = peach
			}
			parts = append(parts, lipgloss.NewStyle().
				Foreground(mantle).
				Background(bg).
				Padding(0, 1).
				Render(full))
		} else {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(overlay0).
				Padding(0, 1).
				Render(full))
		}
	}
	return "  " + strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Running view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewRunning(innerW int) string {
	var b strings.Builder

	// Mode-specific title and description
	var titleText, titleIcon, workDesc, timeDesc string
	var titleColor lipgloss.Color
	switch r.mode {
	case modeVaultAnalyzer:
		titleText = "Analyzing Vault..."
		titleIcon = " "
		titleColor = teal
		workDesc = "Reading and analyzing vault notes."
		timeDesc = "This takes 1-5 min depending on vault size."
	case modeNoteEnhancer:
		titleText = "Enhancing Note..."
		titleIcon = " "
		titleColor = pink
		workDesc = "Researching and enhancing your note."
		timeDesc = "This takes 1-3 min."
	case modeDailyDigest:
		titleText = "Generating Digest..."
		titleIcon = " "
		titleColor = yellow
		workDesc = "Analyzing recent activity."
		timeDesc = "This takes 1-2 min."
	default:
		titleText = "Researching..."
		titleIcon = " "
		titleColor = mauve
		workDesc = "Searching the web and creating notes."
		timeDesc = "This takes 1-3 min depending on depth."
	}

	b.WriteString(lipgloss.NewStyle().Foreground(titleColor).Bold(true).
		Render(titleIcon + titleText))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Topic
	b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
		Render("  " + r.topic))
	b.WriteString("\n\n")

	// Animated dots
	dots := int(time.Since(r.startTime).Seconds()) % 4
	dotStr := strings.Repeat(".", dots+1)
	spinner := lipgloss.NewStyle().Foreground(green).Bold(true).Render(dotStr)
	b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Render("Claude is working") + spinner)
	b.WriteString("\n\n")

	// Progress info
	if r.elapsed != "" {
		b.WriteString("  " + DimStyle.Render("Elapsed: "+r.elapsed))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Description
	infoStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(infoStyle.Render("  " + workDesc))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render("  " + timeDesc))
	b.WriteString("\n\n")

	b.WriteString(DimStyle.Render("  Esc cancel"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Done view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewDone(innerW int) string {
	var b strings.Builder

	// Mode-specific completion title
	var doneTitle string
	switch r.mode {
	case modeVaultAnalyzer:
		doneTitle = "  Vault Analysis Complete"
	case modeNoteEnhancer:
		doneTitle = "  Note Enhanced"
	case modeDailyDigest:
		doneTitle = "  Digest Generated"
	default:
		doneTitle = "  Research Complete"
	}

	b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).
		Render(doneTitle))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Summary
	b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
		Render("  " + r.topic))
	b.WriteString("\n")
	if r.elapsed != "" {
		b.WriteString(DimStyle.Render("  Completed in " + r.elapsed))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(r.createdFiles) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lavender).
			Render(fmt.Sprintf("  %d notes created:", len(r.createdFiles))))
		b.WriteString("\n\n")

		maxVisible := r.height/2 - 10
		if maxVisible < 5 {
			maxVisible = 5
		}
		if maxVisible > len(r.createdFiles) {
			maxVisible = len(r.createdFiles)
		}

		start := 0
		if r.selectedFile >= start+maxVisible {
			start = r.selectedFile - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(r.createdFiles) {
			end = len(r.createdFiles)
		}

		nameStyle := lipgloss.NewStyle().Foreground(text)
		selectedStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		pathStyle := lipgloss.NewStyle().Foreground(overlay0)

		for i := start; i < end; i++ {
			name := strings.TrimSuffix(filepath.Base(r.createdFiles[i]), ".md")
			dir := filepath.Dir(r.createdFiles[i])

			// Truncate long names
			maxName := innerW - 8
			if maxName < 20 {
				maxName = 20
			}
			if len(name) > maxName {
				name = name[:maxName-1] + "…"
			}

			if i == r.selectedFile {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Render(ThemeAccentBar) + " ")
				b.WriteString(selectedStyle.Render(name))
				b.WriteString("\n")
				// Show path for selected item
				b.WriteString("      " + pathStyle.Render(dir))
				b.WriteString("\n")
			} else {
				b.WriteString("    ")
				b.WriteString(nameStyle.Render(name))
				b.WriteString("\n")
			}
		}

		if len(r.createdFiles) > maxVisible {
			b.WriteString(DimStyle.Render(fmt.Sprintf("\n  %d/%d shown", maxVisible, len(r.createdFiles))))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(DimStyle.Render("  Notes created in your vault."))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Refresh to see them."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter open  j/k navigate  Esc close"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Error view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewError(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).
		Render("  Research Failed"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Error message — wrap to innerW
	errMsg := r.errorMsg
	if len(errMsg) > innerW-4 {
		// Simple word wrap
		words := strings.Fields(errMsg)
		var lines []string
		cur := ""
		for _, w := range words {
			if len(cur)+len(w)+1 > innerW-4 {
				lines = append(lines, cur)
				cur = w
			} else {
				if cur != "" {
					cur += " "
				}
				cur += w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		for _, l := range lines {
			b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  " + l))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  " + errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if r.output != "" {
		b.WriteString(DimStyle.Render("  Output:"))
		b.WriteString("\n")
		lines := strings.Split(r.output, "\n")
		maxLines := 8
		for i, line := range lines {
			if i >= maxLines {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ... %d more lines", len(lines)-i)))
				b.WriteString("\n")
				break
			}
			if len(line) > innerW-4 {
				line = line[:innerW-4]
			}
			b.WriteString(DimStyle.Render("  " + line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/Esc back"))

	return b.String()
}
