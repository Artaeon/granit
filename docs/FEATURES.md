# Granit — Feature Guide

> Comprehensive reference for every feature in Granit, the terminal-native knowledge manager.

---

> **Looking for the static site generator?** See the [`granit publish`](#granit-publish----obsidian-publish-style-folder-to-website) section under Export & Publishing, or the dedicated [Publishing guide](PUBLISH.md).

## Table of Contents

- [Core Editor](#core-editor)
- [Vault Management](#vault-management)
- [AI-Powered Features](#ai-powered-features)
- [Productivity Tools](#productivity-tools)
- [Knowledge Tools](#knowledge-tools)
- [Git Integration](#git-integration)
- [Export & Publishing](#export--publishing)
- [Extensibility](#extensibility)
- [Customization](#customization)

---

## Core Editor

### Syntax-Highlighted Markdown

Granit renders Markdown syntax with semantic colors: headings, bold, italic, inline code, code blocks, blockquotes, lists, checkboxes, wikilinks, tags, and YAML frontmatter are each styled according to the active theme.

- **Access:** Automatic in edit mode
- **Example:** Headings display in the Primary color, bold text is brightened, and code blocks use the Surface0 background.

### Language-Aware Code Highlighting

Fenced code blocks (` ```go `, ` ```python `, etc.) receive per-language coloring. Supported languages include Go, Python, JavaScript/TypeScript, Rust, Shell/Bash, Java, C/C++, Ruby, and more. Keywords, types, strings, comments, and numbers are each distinctly styled.

- **Access:** Automatic in both edit and view mode for fenced code blocks with a language tag
- **Example:** A Go code block highlights `func`, `return`, and `if` as keywords, `string` and `int` as types, and `"hello"` as a string literal.

### Wikilinks

Type `[[note name]]` to create links between notes. Granit resolves wikilinks across the entire vault, matching by filename (without the `.md` extension). Following a link opens the target note.

- **Access:** Type `[[` in the editor to trigger autocomplete; press `Enter` on a link in view mode to follow it
- **Example:** `[[Meeting Notes]]` links to `Meeting Notes.md` anywhere in the vault.

### Backlinks Panel

The right-side panel shows every note that links **to** the current note (backlinks) and every note the current note links **to** (outgoing links). Each entry is clickable to navigate.

- **Access:** `F3` to focus the backlinks panel; visible in Default, Reading, and Dashboard layouts
- **Example:** When editing `Project Alpha.md`, the backlinks panel lists every note containing `[[Project Alpha]]`.

### Live Backlink Preview

When the cursor rests on a `[[wikilink]]`, a floating popup shows a preview of the linked note's content. Scroll within the popup to read more.

- **Access:** Automatic when the cursor is positioned on a wikilink in edit mode
- **Example:** Hovering over `[[Architecture Decisions]]` shows the first several lines of that note.

### YAML Frontmatter

Notes can begin with a `---` delimited YAML frontmatter block. Granit parses and displays `tags`, `date`, `title`, and any custom fields. Frontmatter is visually distinguished from note content.

- **Access:** Automatic parsing on file open
- **Example:**
  ```yaml
  ---
  title: Weekly Review
  date: 2026-03-07
  tags: [review, weekly]
  ---
  ```

### Frontmatter Editor

A structured overlay for editing frontmatter properties. Tags display as removable pills, booleans as toggles, dates with validation, and presets for common fields.

- **Access:** Command palette > "Edit Frontmatter"
- **Example:** Add a new tag by typing in the tag input field; toggle a `draft: true` boolean with Enter.

### Rendered View Mode

Toggle between raw Markdown editing and a styled reading view. View mode renders headings with proper hierarchy, formats blockquotes, displays checkboxes, renders callouts, and resolves embedded content.

- **Access:** `Ctrl+E` to toggle
- **Example:** A `> [!warning]` callout block renders as a styled warning box in view mode.

### Vim Keybindings

Full modal editing emulation with four modes: Normal, Insert, Visual, and Command. Supports motions (`h`/`j`/`k`/`l`, `w`/`b`/`e`, `0`/`$`, `gg`/`G`), operators (`d`, `c`, `y` with motions), numeric prefixes, dot repeat, visual selection, and command-line (`:w`, `:q`, `:wq`, `:{line}`).

- **Access:** Enable via Settings (`Ctrl+,` > "Vim Mode") or Command palette > "Toggle Vim Mode"
- **Example:** In Normal mode, `5dd` deletes 5 lines; `ciw` changes the word under the cursor; `gg` jumps to the top of the file.

### Multi-Cursor Editing

Add multiple cursors to edit several locations simultaneously. Select the word under the cursor, then repeatedly add cursors at the next occurrence of that word.

- **Access:** `Ctrl+D` to select word and add cursor at next occurrence; `Ctrl+Shift+Up`/`Ctrl+Shift+Down` to add cursors above/below; `Esc` to clear
- **Example:** Select a variable name with `Ctrl+D`, press `Ctrl+D` twice more to select the next two occurrences, then type a replacement — all three change at once.

### Undo / Redo

Full edit history with unlimited undo and redo. Each keystroke is recorded as an undo step.

- **Access:** `Ctrl+U` to undo, `Ctrl+Y` to redo (or `u` / `Ctrl+R` in Vim mode)
- **Example:** Undo an accidental deletion, then redo if you change your mind.

### Find & Replace

Search within the current file with match highlighting. Replace mode supports one-at-a-time or replace-all.

- **Access:** `Ctrl+F` for find, `Ctrl+H` for find & replace
- **Example:** Search for "TODO" to jump between all TODO markers in a note; replace "2025" with "2026" throughout.

### Smart Autocomplete (Wikilinks)

Typing `[[` triggers an inline popup listing all notes in the vault, filtered by fuzzy search as you type. Each suggestion shows a content preview snippet.

- **Access:** Type `[[` in the editor
- **Example:** Type `[[mee` and the popup shows "Meeting Notes", "Meeting Template", etc.

### Collapsible Sections (Folding)

Fold and unfold headings and code blocks. Fold indicators (`▶`/`▼`) appear in the gutter. Folded sections hide their content, making it easy to navigate large notes.

- **Access:** Command palette > "Toggle Fold", "Fold All", "Unfold All"; in Vim mode: `za` (toggle), `zM` (fold all), `zR` (unfold all)
- **Example:** Fold all level-2 headings to get an overview of a long document, then unfold only the section you want to edit.

### Footnotes

Standard Markdown footnote syntax: `[^1]` references in text link to `[^1]: definition` blocks. Footnote markers are styled and definitions are accessible via lookup.

- **Access:** Automatic rendering in both edit and view mode
- **Example:** `This is a claim[^1]` with `[^1]: Source: Research paper, 2025` at the bottom.

### Auto-Close Brackets

Typing an opening bracket (`(`, `[`, `{`, `` ` ``, `"`, `'`) automatically inserts the matching closing bracket.

- **Access:** Enabled by default; toggle via Settings > "Auto Close Brackets"
- **Example:** Typing `(` produces `()` with the cursor between them.

### Line Numbers

Line numbers display in the gutter, with the active line number highlighted in the Accent color.

- **Access:** Enabled by default; toggle via Settings > "Line Numbers"
- **Example:** Line 42 highlights in peach when the cursor is on it.

### Snippet Expansion

18 built-in snippets expand short trigger words into templates. Placeholders like `{{date}}` and `{{time}}` are replaced with current values.

| Trigger | Description | Expanded Content |
|---------|-------------|------------------|
| `/date` | Today's date | `2026-03-08` |
| `/time` | Current time | `14:30` |
| `/datetime` | Date and time | `2026-03-08 14:30` |
| `/todo` | Checkbox | `- [ ] ` |
| `/done` | Checked box | `- [x] ` |
| `/h1` | Heading 1 | `# ` |
| `/h2` | Heading 2 | `## ` |
| `/h3` | Heading 3 | `### ` |
| `/link` | Wikilink | `[[]]` |
| `/code` | Code block | ` ```\n\n``` ` |
| `/table` | Markdown table | 3-column table template |
| `/meeting` | Meeting notes | Full meeting template with date |
| `/daily` | Daily note | Daily note with tasks/notes/reflection |
| `/callout` | Callout block | `> [!note]\n> ` |
| `/divider` | Horizontal rule | `---` |
| `/quote` | Block quote | `> ` |
| `/img` | Image | `![alt text](url)` |
| `/frontmatter` | YAML frontmatter | Full frontmatter block with date |

- **Access:** Type the trigger (e.g., `/meeting`) and it auto-expands. A slash menu appears showing matching snippets as you type.
- **Example:** Type `/meeting` and the full meeting template appears with today's date filled in.

### Inline AI Editor (slash-menu AI actions, Alt+/)

Six selection-aware AI actions live inside the slash menu and a dedicated AI-only menu, powered by whatever provider is configured (Ollama, OpenAI, Anthropic, Nous, Nerve):

| Trigger | Action | What it does |
|---|---|---|
| `/rewrite` or AI menu | **Rewrite** | Same meaning, clearer wording |
| `/expand` | **Expand** | Adds depth + detail (≤ 1 paragraph) |
| `/summarize` | **Summarize** | 1–3 sentence distillation |
| `/improve` | **Improve** | Word choice + flow tightening |
| `/shorten` | **Shorten** | Tighter, fewer words |
| `/fix` | **Fix Grammar** | Minimal-edit grammar/spelling pass |

- **Access (with selection):** `Alt+/` — preserves the selection so the action targets exactly the highlighted text. Mnemonic: same key as the `/` slash menu, plus Alt for AI mode.
- **Access (no selection):** Type `/` then the action name — operates on the current line. Selection is lost when `/` is typed (it's a regular keystroke), so this path is best when you're editing the line you're on.
- **Output handling:** spliced back at the originally-captured range — moving the cursor while the AI thinks doesn't write to the wrong spot. LLM preambles ("Sure! Here's…"), wrapping code fences, and surrounding quotes are stripped before insertion.

### Spell Checking

Integrated spell checking using aspell or hunspell. Misspelled words are highlighted and suggestions are available.

- **Access:** Command palette > "Spell Check"; enable persistent checking via Settings > "Spell Check"
- **Requires:** `aspell` or `hunspell` installed on the system
- **Example:** "recieve" is highlighted; selecting it shows "receive" as a correction.

### Focus / Zen Mode

A distraction-free writing environment with a centered, width-limited editor. No sidebar, no backlinks, no status bar — just you and the text.

- **Access:** `Ctrl+Z` or Command palette > "Focus Mode"
- **Example:** Enter Zen mode for a focused writing session with an 80-character-wide editor centered on screen.

### Ghost Writer

AI-powered inline writing suggestions that appear as you type. Ghost text appears dimmed ahead of the cursor; press `Tab` to accept the suggestion.

- **Access:** Enable via Settings > "Ghost Writer" or Command palette > "Ghost Writer"
- **Works with:** Ollama, OpenAI, or local fallback
- **Example:** Start typing "The main advantage of..." and Ghost Writer suggests "...this approach is its simplicity and maintainability."

### Visual Table Editor

Edit Markdown tables in a spreadsheet-like interface. Navigate between cells, add/remove rows and columns, and changes sync back to the Markdown source.

- **Access:** Command palette > "Table Editor" (cursor must be on a Markdown table)
- **Example:** Place the cursor on a `| Column |` table, open the table editor, and use arrow keys to navigate cells.

### Mermaid Diagrams

ASCII rendering of Mermaid diagram syntax in view mode. Supports flowcharts, sequence diagrams, pie charts, class diagrams, and Gantt charts.

- **Access:** Write Mermaid syntax in a ` ```mermaid ` code block and switch to view mode (`Ctrl+E`)
- **Example:**
  ````
  ```mermaid
  graph TD
    A[Start] --> B[Process]
    B --> C[End]
  ```
  ````
  Renders as an ASCII flowchart in view mode.

### Custom Diagrams

A custom diagram engine supporting 6 diagram types in ` ```diagram ` code blocks:

| Type | Description |
|------|-------------|
| `sequence` | Combo/flow sequences |
| `tree` | Decision trees |
| `movement` | Footwork/movement grids |
| `timeline` | Timeline visualizations |
| `comparison` | Comparison tables |
| `figure` | Pre-drawn technique illustrations (10 poses) |

- **Access:** Write diagram syntax in a ` ```diagram ` code block
- **Example:** A `type: sequence` diagram with labeled steps renders as an ASCII flow.

### Global Search & Replace

Find and replace text across all vault files. Preview matches per file, choose per-file or vault-wide replacement, with confirmation before applying changes.

- **Access:** Command palette > "Global Search & Replace"
- **Example:** Replace all instances of `#project` with `#projects` across the entire vault.

### Link Assistant

Scans the current note for unlinked mentions of other note titles. Shows each mention with context and offers batch-creation of `[[wikilinks]]`.

- **Access:** Command palette > "Link Assistant"
- **Example:** If you have a note called "Machine Learning" and your current note mentions "machine learning" without a link, the assistant suggests converting it to `[[Machine Learning]]`.

### Tab Management

Open multiple notes in tabs. Reorder tabs, pin frequently used notes, and close tabs.

- **Access:** `Alt+Shift+Left`/`Alt+Shift+Right` to reorder; `Alt+W` to close; Command palette > "Pin Note" / "Unpin Note"
- **Example:** Pin your `Tasks.md` tab so it always appears first.

### Note Encryption

AES-256-GCM encryption with PBKDF2 key derivation. Encrypted notes are saved as `.md.enc` files, making them safe for syncing via Git or cloud storage.

- **Access:** Command palette > "Encrypt/Decrypt Note"
- **Example:** Encrypt a journal entry before pushing to GitHub; decrypt it when you open it later.

### Language Learning

Vocabulary tracker supporting 9 languages with spaced repetition practice, grammar notes, and a dashboard with streaks and level charts.

- **Access:** Command palette > "Language Learning"
- **Example:** Add German vocabulary words, then practice with flashcard-style review sessions.

---

## Vault Management

### Multi-Vault Switcher

Switch between different vaults without restarting Granit. The vault list is managed in-app and persisted to `~/.config/granit/vaults.json`.

- **Access:** Command palette > "Switch Vault"
- **Example:** Switch from your personal notes vault to your work vault instantly.

### Vault Selector

When launching `granit` without arguments, a full-screen vault selector shows recently used vaults. Create new vaults, remove entries, or select one to open.

- **Access:** Run `granit` with no arguments
- **Example:** See "Personal Notes (last: 2026-03-07)" and "Work Wiki (last: 2026-03-05)" — select one with Enter.

### File Tree Explorer

A hierarchical file browser with folder expand/collapse, file type icons, and fuzzy search filtering. Folder collapse state persists across sessions.

- **Access:** `F1` to focus; `j`/`k` to navigate; `Enter`/`Space` to open file or toggle folder; `Left`/`Right` to collapse/expand
- **Folder operations:** `z` collapse all, `Z` expand all
- **Search:** Press `/` to enter search mode; fuzzy-matches across all filenames
- **Tab switching:** `Ctrl+Tab` / `Ctrl+Shift+Tab` to cycle between open tabs
- **Example:** Press `z` to collapse all folders, then expand only the ones you need. State is saved automatically.

### Fuzzy Search

Quick file open with fuzzy matching across all note names in the vault.

- **Access:** `Ctrl+P`
- **Example:** Type "wkrev" to match "Weekly Review 2026-03-07.md".

### Full-Text Search

Search across the content of all notes in the vault. Results show matching lines with highlighted search terms and file context.

- **Access:** Command palette > "Search Vault Contents"
- **Example:** Search for "API endpoint" to find every note mentioning it, with the matching line shown.

### Tag Browser

Browse all tags used across the vault. Select a tag to see all notes containing it.

- **Access:** `Ctrl+T`
- **Example:** Select `#project` to list every note tagged with it.

### Graph View

Visualize note connections as an ASCII graph. Notes are nodes, wikilinks are edges. See clusters, hubs, and orphan notes at a glance.

- **Access:** `Ctrl+G`
- **Example:** Your "Index" note appears as a central hub with many connections radiating outward.

### Task Manager

Comprehensive task system with 6 views:

| View | Description |
|------|-------------|
| **Today** | Tasks due today |
| **Upcoming** | Tasks due in the next 7 days |
| **All** | Every task across the vault |
| **Done** | Completed tasks |
| **Calendar** | Tasks on a calendar grid |
| **Kanban** | Drag tasks between columns (Backlog, Todo, In Progress, Done) |

Features include 5 priority levels, due dates with a date picker, dedicated `Tasks.md` storage, source file badges showing which note a task came from, and cross-vault task scanning.

- **Access:** `Ctrl+K` or Command palette > "Task Manager"
- **Example:** Create a high-priority task due tomorrow, then drag it to "In Progress" on the Kanban board.

### Calendar View

Full-featured calendar with 6 views: month, week, 3-day, 1-day, agenda, and year. The weekly view uses the full terminal width with hourly time slots, event blocks with background cards, and today's column highlighted.

- **Access:** `Ctrl+L`
- **Navigation:** `←/→` days, `↑/↓` hours (week view) or weeks (month view), `[/]` months, `w` cycle views
- **Task time-blocking:** Press `b` to assign a task to the selected time slot
- **Event creation:** Press `a` for the step-by-step wizard (title → time → duration → location → recurrence → color → description)
- **Weekly milestones:** Press `g` to create a milestone linked to an active goal
- **Goals integration:** Active goals shown as progress bar badges in the week header
- **Daily focus:** Plan My Day's top goal shown in the day header
- **ICS support:** Auto-loads `.ics` files from vault; per-file toggle in Settings > Files
- **Calendar panel:** Compact sidebar widget for cockpit/widescreen layouts, auto-loads events on startup

### Timeline View

Chronological visualization of all notes grouped by day, week, or month. Browse your note creation history on a visual timeline.

- **Access:** Command palette > "Timeline"
- **Example:** See that you created 12 notes last week, grouped by day.

### Bookmarks & Recents

Star your favorite notes and quickly access recently opened files. Two tabs: Starred and Recent.

- **Access:** `Ctrl+B` to open; Command palette > "Toggle Bookmark" to star/unstar
- **Example:** Star your "Daily Tasks" note and it appears at the top of the bookmarks overlay.

### Quick Switch

Fast file switcher showing your most recently opened notes. Faster than fuzzy search for jumping between active files.

- **Access:** `Ctrl+J`
- **Example:** Switch between the 3 notes you've been editing without searching.

### Note Outline

Heading-based document outline for the current note. Click a heading to jump to it.

- **Access:** `Ctrl+O`
- **Example:** A note with 5 headings shows them in a navigable list; select "## Implementation" to jump there.

### Workspace Layouts

Save and restore named workspace snapshots. A workspace captures which tabs are open, the current layout, and view mode settings.

- **Access:** Command palette > "Workspaces"
- **Example:** Save a "Research" workspace with 4 tabs open in Dashboard layout, then switch to a "Writing" workspace with 1 tab in Zen layout.

### Breadcrumb Navigation

A folder-path breadcrumb above the editor shows `vault > folder > note`. Browser-style back/forward navigation lets you retrace your steps. Supports pinned tabs.

- **Access:** `Alt+Left` to go back, `Alt+Right` to go forward; breadcrumb is always visible above the editor
- **Example:** Navigate from "Projects/Alpha/Design.md" — the breadcrumb shows "vault > Projects > Alpha > Design".

### Daily Notes

Create or open today's daily note with a single command. Configurable folder and template.

- **Access:** Command palette > "Daily Note"; or `granit daily` from the command line
- **Example:** Press the Daily Note command on March 8 and `2026-03-08.md` opens (or is created from template).

### Vault Statistics

Overview of vault health: total notes, total links, orphan notes, word counts, link density, and tag distribution with bar charts.

- **Access:** Command palette > "Vault Statistics"
- **Example:** "1,247 notes, 3,891 links, 42 orphan notes, 312,456 words".

### Trash (Recycle Bin)

Soft-delete notes to `.granit-trash/` with the ability to restore. Deleted notes keep their content and can be recovered.

- **Access:** Command palette > "Trash" to view and restore; delete a note and it goes to trash
- **Example:** Accidentally delete a note, open Trash, select it, and press `r` to restore.

### Folder Management

Create new folders and move files between folders from within the TUI.

- **Access:** Command palette > "New Folder" or "Move File"
- **Example:** Create a "Research" folder, then move 3 notes into it.

### File Watcher

Auto-detects external file changes (from another editor, Git pull, sync tool) and refreshes the vault.

- **Access:** Enabled by default; toggle via Settings > "Auto Refresh Vault"
- **Example:** Edit a note in VS Code and Granit immediately reflects the changes.

### Lazy Vault Loading

On-demand content reading for fast startup with large vaults (1000+ notes). Only metadata and filenames are loaded initially; file contents are read when a note is opened.

- **Access:** Automatic — no configuration needed
- **Example:** A vault with 5,000 notes opens in under a second.

### Pomodoro Timer

25-minute focus sessions with break cycles. Tracks writing statistics during each session (words written, notes edited).

- **Access:** Command palette > "Pomodoro Timer"
- **Example:** Start a 25-minute session, write 500 words, take a 5-minute break, then start another session.

### System Clipboard

Paste from the system clipboard with platform-native access.

- **Access:** `Ctrl+V`
- **Requires:** `xclip`, `xsel`, or `wl-copy` on Linux (degrades gracefully without them)

### Web Clipper

Fetch a URL, convert the web page to Markdown, and save it as a new note in the vault.

- **Access:** Command palette > "Web Clipper"
- **Example:** Paste a blog URL and Granit creates a Markdown note with the article content.

---

## AI-Powered Features

Granit includes **25+ AI features** that work with four providers:

| Provider | Description | Setup |
|----------|-------------|-------|
| **Local** | Keyword matching, stopword filtering, topic detection — no network, no API keys | Works out of the box |
| **Ollama** | Local LLM via HTTP API — recommended for privacy | Install Ollama + pull a model |
| **OpenAI** | Cloud API with GPT models | Set API key in config |
| **Nous / Nerve** | Alternative local LLM backends | See AI-GUIDE.md |

All AI features share a **production-grade reliability stack**: small-model auto-detection (0.5B–3B), automatic retry on transient errors, real HTTP cancellation via `context.Context`, hard per-request deadlines, in-flight guards for auto-save features, token-budget fit checks, empty-response fallbacks, word-boundary truncation, ghostwriter completion cache, and elapsed time display on every loading screen.

See [AI-GUIDE.md](AI-GUIDE.md) for detailed setup instructions and the full bot reference.

### 19 AI Bots (`Ctrl+R`)

Organized into 6 semantic categories. Every bot is small-model-aware (automatically detects Ollama models ≤3B and adapts prompt size, temperature, and context limits).

| Category | Bots |
|---|---|
| **SUMMARIZE** | **TL;DR** — one-sentence summary • **Summarizer** — 2-4 sentence summary • **Explain Simply** — rewrites for a 12-year-old |
| **WRITING** | **Title Suggester** — 5 alternative titles • **Writing Assistant** — readability + improvements • **Tone Adjuster** — formal/casual/concise rewrites • **Expand** — flesh out a terse note |
| **ANALYSIS** | **Question Bot** — answer questions from your vault • **Counter-Argument** — devil's advocate opposing views • **Pros & Cons** — decision-analysis list • **Action Items** — extract todos |
| **ORGANIZE** | **Auto-Tagger** — suggest tags with few-shot learning • **Link Suggester** — find related notes • **Auto-Link** — find `[[wikilinks]]` to insert • **Outline Generator** — hierarchical outline |
| **LEARNING** | **Flashcard Generator** — Q&A pairs • **Key Terms** — glossary extraction |
| **VAULT** | **MOC Generator** — Map of Content for vault • **Daily Digest** — vault activity summary (local-only) |

**Bot workflow:**
- **`Ctrl+R`** opens the overlay
- **Type to filter** the list by name or description
- **`1`–`9`** quick-pick the first nine visible bots
- **`Enter`** runs the selected bot (cursor remembers last-used)
- **Results view:** `c` copies to clipboard, `s` saves as a vault note with frontmatter, `r` re-runs the bot, `j/k g/G` scroll, `Esc` back
- **Loading screen:** shows elapsed time, animated progress bar, slow-model hints; `Esc` actually cancels the HTTP request

See [AI-GUIDE.md](AI-GUIDE.md) for the full bot reference and reliability details.

### AI Chat

Ask questions about your entire vault. The AI uses note content as context to provide relevant, informed answers.

- **Access:** Command palette > "AI Chat"
- **Example:** Ask "What are my main project deadlines?" and the AI searches your vault for deadline mentions.

### Chat with Note

AI Q&A focused on the current note. Ask questions about the content you're reading or writing.

- **Access:** Command palette > "Chat with Note"
- **Example:** On a technical note, ask "What are the trade-offs mentioned here?" and get a focused answer.

### AI Compose

Generate a full note from a topic prompt. The AI creates structured Markdown with headings, content, and frontmatter.

- **Access:** Command palette > "AI Compose Note"
- **Example:** Enter "Introduction to Kubernetes" and receive a complete note with sections, definitions, and examples.

### Ghost Writer

Inline AI writing suggestions that appear as dimmed text ahead of the cursor. Press `Tab` to accept a suggestion.

- **Access:** Enable via Settings > "Ghost Writer" or Command palette > "Ghost Writer"
- **Example:** Start writing "The key principles of..." and see "...clean architecture include separation of concerns" appear as ghost text.

### Thread Weaver

Select multiple notes and synthesize them into a new essay, summary, or combined document. The AI identifies common themes and creates a coherent narrative.

- **Access:** Command palette > "Thread Weaver"
- **Example:** Select 5 notes about "microservices" and Thread Weaver creates a comprehensive overview document.

### Semantic Search

AI-powered meaning-based vault search using embeddings. Finds notes by concept rather than exact keywords.

- **Access:** Command palette > "Semantic Search"
- **Example:** Search for "motivation techniques" and find notes about "habit formation", "goal setting", and "productivity tips" even if they don't use the exact search terms.

### Knowledge Graph AI

AI-powered analysis of your vault's link structure. Identifies clusters, hub notes, orphan notes, and suggests new connections.

- **Access:** Command palette > "Knowledge Graph AI"
- **Example:** The analysis reveals that your "Machine Learning" cluster is disconnected from your "Statistics" cluster and suggests linking them.

### Auto-Link

Scans the current note for unlinked mentions of other note titles and offers to convert them to `[[wikilinks]]`.

- **Access:** Command palette > "Auto-Link Suggestions"
- **Example:** Your note mentions "neural networks" 3 times without linking — Auto-Link offers to add `[[Neural Networks]]` links.

### Auto-Tag

Automatically suggests tags when saving a note. Analyzes content to determine relevant topics.

- **Access:** Enable via Settings > "Auto-Tag on Save"
- **Example:** Save a note about Python web frameworks and auto-tag suggests `#python`, `#web`, `#frameworks`.

### Similar Notes (TF-IDF)

Find notes with similar content using TF-IDF cosine similarity. Shows shared keywords and a similarity score.

- **Access:** Command palette > "Similar Notes"
- **Example:** On a note about "React hooks", similar notes might include "React State Management", "JavaScript Patterns", and "Frontend Architecture".

### AI Template Generator

Generate full notes from 9 template types using AI:

| Template Type | Description |
|---------------|-------------|
| Meeting | Meeting notes with agenda, attendees, action items |
| Project | Project plan with goals, milestones, risks |
| Tech Doc | Technical documentation with API, architecture, examples |
| Blog | Blog post with introduction, body, conclusion |
| Tutorial | Step-by-step tutorial with prerequisites and exercises |
| Comparison | Side-by-side comparison of options |
| Summary | Executive summary of a topic |
| Workout | Exercise routine with sets, reps, notes |
| Custom | Free-form AI generation from a prompt |

- **Access:** Command palette > "AI Template"
- **Example:** Select "Tech Doc" template, enter "REST API Design", and receive a complete technical document.

### Deep Dive Research

An AI research agent powered by Claude Code. Give it a topic and it:

1. Searches the web for current information
2. Creates 5-25 interconnected notes in a `Research/` folder
3. Generates a hub note (`_Index.md`) linking everything
4. Adds frontmatter, tags, and `[[wikilinks]]` automatically

Supports 4 profiles (General, Academic, Technical, Creative), 4 source filters (Any, Web, Docs, Papers), 3 depth levels, and 3 output formats (Zettelkasten, Outline, Study Guide).

- **Access:** Command palette > "Deep Dive Research"
- **Requires:** Claude Code installed (`claude` in PATH)
- **Example:** Research "quantum computing applications" in Academic profile and receive 15 interconnected Zettelkasten notes.

### Research Follow-Up

Go deeper on the current note's topic. Uses the note content as context for further Claude Code research.

- **Access:** Command palette > "Research Follow-Up"
- **Example:** On a note about "transformer architectures", launch a follow-up to research "attention mechanism variants".

### Vault Analyzer

AI-powered analysis of your vault's structure, identifying gaps, orphan notes, missing connections, and suggesting improvements.

- **Access:** Command palette > "Vault Analyzer"
- **Requires:** Claude Code installed
- **Example:** The analyzer identifies 15 orphan notes, suggests 8 new connections, and recommends reorganizing your "Projects" folder.

### Note Enhancer

AI-enhance the current note with additional wikilinks, better structure, deeper content, and improved formatting.

- **Access:** Command palette > "Note Enhancer"
- **Requires:** Claude Code installed
- **Example:** A sparse note about "Docker" gets enhanced with wikilinks to related notes, additional sections, and better formatting.

### Daily Digest

Generate a weekly review from recent vault activity. Summarizes what you've been working on, what changed, and what needs attention.

- **Access:** Command palette > "Daily Digest"
- **Requires:** Claude Code installed
- **Example:** Generate a digest covering the last week's 23 modified notes, 5 new notes, and 12 completed tasks.

### Vault Refactor

AI-powered suggestions to reorganize, merge, split, or retag notes. Analyzes vault structure and proposes improvements.

- **Access:** Command palette > "AI Vault Refactor"
- **Example:** The refactor suggests merging 3 overlapping notes about "authentication", splitting a 2000-word note into 4 focused notes, and adding consistent tags.

### Daily Briefing

AI-generated morning summary covering recent notes, pending tasks, upcoming deadlines, and suggested connections.

- **Access:** Command palette > "Daily Briefing"
- **Example:** "Good morning! You have 3 tasks due today, 2 notes were modified yesterday, and your 'Project Alpha' note has 5 unlinked mentions."

### Quiz Mode

Auto-generated quizzes from your notes for active recall. Multiple-choice and open-ended questions test your knowledge of note content.

- **Access:** Command palette > "Quiz Mode"
- **Example:** A quiz on your "Machine Learning" notes asks "What is the difference between supervised and unsupervised learning?"

### Flashcards

Spaced repetition study using the SM-2 algorithm. Flashcards are automatically extracted from your notes (headings become questions, content becomes answers).

- **Access:** Command palette > "Flashcards"
- **Example:** Review 20 flashcards from your "Statistics" notes with increasing intervals as you master each card.

### Learning Dashboard

Track study progress across flashcards and quizzes. View streaks, mastery levels, and study time statistics.

- **Access:** Command palette > "Learning Dashboard"
- **Example:** See that you've maintained a 14-day study streak with 85% average quiz accuracy.

### Natural Language Search

Ask questions in plain English and find relevant notes. The AI interprets your intent and searches by meaning.

- **Access:** Command palette > "Natural Language Search"
- **Example:** Search "notes about improving team productivity" and find relevant notes even if they use different terminology.

### AI Writing Coach

Detailed analysis of your writing with suggestions for clarity, structure, and style. Supports soul note persona customization for consistent voice.

- **Access:** Command palette > "Writing Coach"
- **Example:** The coach analyzes a blog draft and suggests "Consider breaking this paragraph into smaller sentences for readability."

### AI Smart Scheduler

AI-powered optimal daily schedule generation. Analyzes your tasks, priorities, and habits to create a time-blocked schedule with breaks.

- **Access:** Command palette > "AI Smart Scheduler"
- **Works with:** Ollama, OpenAI, or local fallback algorithm
- **Example:** The scheduler arranges your 8 tasks into optimal time slots with breaks, putting high-priority deep work in the morning.

---

## Productivity Tools

### Task Manager

Full-featured task management with 7 views (Today, Upcoming, All, Done, Calendar, Kanban, Eisenhower Matrix), 5 priority levels, due dates, and cross-vault scanning. Tasks are stored in `Tasks.md` and also parsed from all vault notes.

- **Access:** `Ctrl+K`
- **Subtasks:** Indent tasks with spaces to create parent-child hierarchy. Press `e` to expand/collapse.
- **Dependencies:** Add `depends:taskname` (or `depends:"multi word"`) to block a task until its dependency is done.
- **Time Estimation:** Add `~30m` or `~2h` to task text. Press `E` for quick presets. Today view shows total workload.
- **Time Tracking:** Actual logged time shown as colored badge next to estimate (green = under, red = over).
- **Reschedule:** Press `r` for quick options: tomorrow, next Monday, +1 week, +1 month, or custom date.
- **Batch Reschedule:** Press `R` in Today view to walk through all overdue tasks with quick date picks (1=tomorrow, 2=+1 week, s=skip).
- **Snooze:** Press `z` to snooze a task (1=1 hour, 2=4 hours, 3=tomorrow 9am). Snoozed tasks are hidden until the time expires.
- **Pinned Tasks:** Press `W` to pin/unpin. Pinned tasks sort to the top in all views. Persisted to `.granit/pinned-tasks.json`.
- **Task Notes:** Press `n` to add or edit a freeform note on any task. Notes persist to `.granit/task-notes.json`. Note icon shown in row.
- **Auto-Priority:** Press `A` to auto-suggest priority: +2 overdue, +1 today, +1 due ≤ 2 days, +1 blocks others, +1 in project, -1 no date.
- **Undo:** Press `u` to undo task modifications (10-deep stack). Shows remaining count: "Undone (3 more)".
- **Sort:** Press `s` to cycle: priority (default), due date, alphabetical, source note, or first tag.
- **Bulk Ops:** Press `v` to enter select mode. Space selects tasks, `x` bulk-toggles, `d` bulk-sets dates.
- **Filters:** `#` cycles tag filter, `P` cycles priority filter, `/` searches (supports `#tag` syntax), `c` clears.
- **Focus:** Press `f` to start a focus session pre-loaded with the selected task.
- **Overdue:** Today view groups tasks into OVERDUE (red) and TODAY (green) sections.
- **Custom Kanban:** Configure columns via `kanban_columns` and `kanban_column_tags` in settings.
- **Eisenhower Matrix:** Press `7` for 2×2 grid: DO (urgent+important), SCHEDULE (important), DELEGATE (urgent), ELIMINATE (neither).
- **Quick-Add Syntax:** Ctrl+T quick capture parses `@today`/`@tomorrow`/`@monday`, `!high`/`!low`, `~30m`/`~2h` from text.
- **Natural Language Dates:** Quick-add also supports `@next week`, `@next friday`, `@end of month`, `@in 3 days`, `@in 2 weeks`.
- **Task Templates:** Press `T` to save current task as a reusable template. Press `t` + number to create from template. Stored in `.granit/task-templates.json`.
- **Task Archiving:** Press `X` to move completed tasks older than 30 days from Tasks.md to `Archive/tasks-YYYY-MM.md`.
- **Help Overlay:** Press `?` for full keybinding reference organized into 5 sections.
- **Status Bar:** Red "N overdue" badge shown in status bar when overdue tasks exist.
- **Project Matching:** Tasks auto-assign to projects by folder path or tag.

### Goal Manager

Standalone goal tracking module independent of projects and habits. Goals have a full lifecycle (active → paused → completed → archived) with milestones, categories, target dates, and progress tracking.

- **Access:** Command palette > "Goals"
- **Views:** Active (all active/paused goals), By Category (grouped), Timeline (sorted by deadline), Completed (done/archived)
- **Creation:** Press `a` for 3-step wizard: title → target date → category
- **Milestones:** Press `m` to add. Enter/x toggles completion. When all milestones are done, goal auto-completes.
- **Lifecycle:** `x` = complete/reactivate, `p` = pause/resume, `A` = archive, `D` = delete permanently
- **Edit:** `e` = edit title, `E` = edit description, `n` = edit notes
- **Date Selection:** Quick-pick: 1=1mo, 2=3mo, 3=6mo, 4=1yr, 5=2yr, 6=3yr, 7=5yr, 0=no deadline
- **Category:** Pick existing by number or type a new one
- **Milestones:** `m` = add, `Enter`/`x` = toggle, `d` = delete, `!` = set due date (1wk/2wk/1mo/3mo), `J`/`K` = reorder. Auto-completes goal when all done.
- **Task Linking:** `t` creates a task from a milestone with `goal:ID` marker. Expanded view shows linked task counts.
- **Reviews:** `r` sets review frequency (weekly/monthly/quarterly) or writes a review reflection when expanded. Review log with progress snapshots.
- **Progress:** Color-coded bar + milestone count (2/5). Sparkline chart from review history (▁▂▃▄▅▆▇█).
- **Colors:** `C` assigns one of 7 theme colors (blue/red/green/yellow/mauve/pink/teal) to status icon and bar.
- **Timeframes:** Human-readable badges: "3mo left", "1y6mo left", "5d overdue"
- **Overdue:** Red `!` indicator, overdue count in stats bar
- **Storage:** `.granit/goals.json` with auto-incrementing IDs (G001, G002, etc.)
- **Help:** Press `?` for full keybinding reference

### Daily Planner

Time-blocked daily schedule from 6am to 10pm in 30-minute slots. Supports multi-hour blocks (30m to 3h). Syncs with tasks, calendar events, and habits. Shows a progress bar and supports launching focus sessions from time blocks.

- **Access:** Command palette > "Daily Planner"
- **Copy Plan:** Press `c` to copy the full day plan (schedule, tasks, habits, active goals) to clipboard
- **Export Plan:** Press `Shift+S` to save as `Plans/plan-YYYY-MM-DD.md` with frontmatter
- **Goals Integration:** Active goals with progress shown in planner and included in copy/export
- **Duration:** Press `-`/`+` when adding a block to adjust duration from 30 minutes to 3 hours.
- **Example:** Block 9:00-11:00 for a 2-hour "Deep Work: Write Report" session.

### Smart Daily Note Template

Daily notes support 16 template variables that are auto-substituted when a note is created:

- **Core:** `{{date}}`, `{{title}}`, `{{weekday}}`, `{{time}}`, `{{yesterday}}`, `{{tomorrow}}`
- **Calendar:** `{{week_number}}`, `{{month_name}}`, `{{year}}`
- **Tasks:** `{{overdue_tasks}}` (overdue task checkboxes), `{{today_tasks}}` (tasks due today), `{{carry_forward}}` (yesterday's incomplete tasks)
- **Habits:** `{{today_habits}}` (habit checkboxes from Habits/habits.md)
- **Schedule:** `{{today_schedule}}` (planner blocks for the date)
- **Stats:** `{{streak}}` (consecutive daily note streak), `{{recurring_tasks}}` (configured recurring tasks)

Custom templates can be set via `daily_note_template` in settings.

### Search Everything

Fuzzy search across all data types — notes, tasks, goals, and habits — in a single overlay.

- **Access:** `Ctrl+/` or command palette > "Search Everything"
- **Search:** Type to filter. Results ranked by fuzzy match score with start-of-string and consecutive-match bonuses.
- **Results:** Grouped by type (NOTES/TASKS/GOALS/HABITS) with colored headers and icons.
- **Navigation:** Up/down to move, Enter to open result (loads note, jumps to task, opens goals/habits).
- **Note search:** Matches title and content (shows matching line snippet).
- **Task search:** Matches cleaned task text with done/source context.
- **Goal search:** Matches goal title and milestone text with status/timeframe.

### Pomodoro Timer

25-minute focus timer with configurable work/break cycles. Tracks words written and notes edited during each session.

- **Access:** Command palette > "Pomodoro Timer"
- **Example:** Start a 25-minute pomodoro, write 300 words, take a 5-minute break.

### Focus Sessions

Guided work sessions with timers (25/45/60/90 minutes), goal setting, a built-in scratchpad, break timer, and session logs saved to `FocusSessions/`.

- **Access:** Command palette > "Focus Session"
- **Example:** Start a 45-minute session with the goal "Finish chapter 3", use the scratchpad for quick thoughts, and review your session log afterward.

### Time Tracker

Per-note and per-task time tracking with pomodoro integration. View reports showing time spent on each note or task.

- **Access:** Command palette > "Time Tracker"
- **Example:** Track 2 hours on "Research Paper" and 45 minutes on "Meeting Notes", then view a summary report.

### Recurring Tasks

Create tasks that automatically recur daily, weekly, or monthly. Manage all recurring tasks from a dedicated overlay.

- **Access:** Command palette > "Recurring Tasks"
- **Auto-Next:** When a recurring task is completed, the next instance is automatically created with the correct due date.
- **Frequencies:** daily, weekly, monthly, 3x-week (Mon→Wed→Fri). 3x-week skips weekends.

### Habit & Goal Tracker

Track daily habits with streak visualization. Set goals with milestones and progress bars. View completion statistics.

- **Access:** Command palette > "Habit Tracker"
- **Example:** Track habits like "Write 500 words", "Exercise", and "Read 30 minutes" with a 30-day streak chart.

### Daily Review

Guided end-of-day review with 5 phases: celebrate completed tasks, reschedule overdue items (tomorrow/next week/skip), review tomorrow's plan, write a reflection, and save to `Reviews/daily-YYYY-MM-DD.md`.

- **Access:** Command palette > "Daily Review"
- **Example:** Reschedule 2 overdue tasks to tomorrow, review your 5 completed tasks, and write "Good progress on the API" as your reflection.

### Weekly Review

Structured weekly reflection overlay with metrics, wins, challenges, and next week planning.

- **Access:** Command palette > "Weekly Review"

### Daily Standup Generator

Auto-generates standup reports from git commits, modified notes, and completed tasks. Saves reports to `Standups/`.

- **Access:** Command palette > "Daily Standup"

### Quick Capture

A compact floating input for quickly saving thoughts. Choose a destination: Inbox note, daily note, Tasks, or a new note. Inbox item count shown in status bar.

- **Access:** Command palette > "Quick Capture"
- **Inbox Badge:** Status bar shows a blue badge with unprocessed inbox item count.

### Project Health Dashboard

Cross-project overview showing all projects with progress bars, health indicators (On Track / At Risk / Behind), velocity tracking (milestones per week), and overdue task warnings.

- **Access:** Command palette > "Project Dashboard"

### Goal Burndown Charts

ASCII burndown chart in the project goals section showing ideal vs actual milestone pace. Displays weeks on X-axis, remaining milestones on Y-axis, and a pace indicator.

- **Shown in:** Project Mode > Goals section (when project has milestones and due date)

### Link Suggestions

The backlinks panel has a "Suggested" tab showing notes similar to the current note, powered by TF-IDF similarity analysis. Press Enter on a suggestion to insert a `[[wikilink]]`.

- **Access:** Focus backlinks panel (`F3`), press `Tab` to reach "Suggested" tab.

### Reading List

The bookmarks overlay has a "Reading" tab for tracking reading progress on notes with status (to-read / reading / completed) and 1-5 star ratings.

- **Access:** `Ctrl+B` > Tab to "Reading"
- **Keys:** `a` add note, `p` cycle status, `r` cycle rating, `d` remove

### Journal Prompts

100+ reflection prompts across 8 categories. Select a prompt and enter a guided writing mode.

- **Access:** Command palette > "Journal Prompts"
- **Example:** Get the prompt "What did you learn today that changed your perspective?" and write your reflection.

### Clipboard Manager

50-entry clipboard history with search, pin, preview, and paste. Persists across note switches.

- **Access:** Command palette > "Clipboard Manager"
- **Example:** Copy 5 code snippets from different notes, then browse the clipboard history to paste the third one.

### Floating Scratchpad

A persistent scratch area that survives across notes and sessions. Use it for temporary notes, quick calculations, or drafting text.

- **Access:** Command palette > "Scratchpad"
- **Example:** Jot down a quick idea in the scratchpad while editing a different note.

### Project Mode

Project management with 9 categories, dashboards, and note/task grouping. Organize notes and tasks by project.

- **Access:** Command palette > "Projects"
- **Example:** Create a "Website Redesign" project, assign notes and tasks to it, and view a project dashboard.

### Plan My Day (AI)

AI-optimized daily planning. Gathers tasks, calendar events, habits, projects, and yesterday's carry-forward items, then generates an optimized schedule with focus order and personalized advice. Shows clocked time tracking sessions integrated into the plan. Works with Ollama, OpenAI, or a local fallback algorithm.

- **Access:** Command palette > "Plan My Day"
- **Example:** AI analyzes your 8 tasks, 2 meetings, and 3 habits, then generates: "9:00 Deep Work — Write Report (highest priority), 10:30 Meeting — Team Standup, 11:00 Review — PR #42..."

### AI Goal Coach

Holistic AI analysis of all active goals. DEEPCOVEN evaluates velocity, identifies stalled goals, detects competing priorities, and recommends a priority order with honest feedback.

- **Access:** Goals overlay > press `I`
- **Example:** "Goal 'Learn Rust' is stalled (0 milestones in 14 days). Pause it or set a specific deadline. Focus on 'Ship v2.0' first — it's 3 days from deadline at 40%."

### AI Project Insights

DEEPCOVEN-powered project health analysis. Evaluates status (green/yellow/red), identifies risks and blockers, suggests next actions, and checks timeline feasibility.

- **Access:** Projects > Dashboard > press `I`
- **Example:** "HEALTH: Yellow — 60% tasks done but deadline is in 5 days. RISK: No progress on Phase 3 milestones. NEXT: Close the 2 blocked PRs before starting new features."

### AI Daily Review Summary

After the reflection phase of the daily review, DEEPCOVEN generates a personalized end-of-day summary: win of the day, recurring patterns, tomorrow's #1 priority, and an honest note.

- **Access:** Automatic — appears after the reflection step in Daily Review (`Alt+E`). Skips if no AI configured.
- **Example:** "WIN: Shipped the parser refactor. PATTERN: You keep rescheduling documentation tasks. TOMORROW: Write the API docs before anything else."

### AI Weekly Review Synthesis

After entering next-week priorities, DEEPCOVEN generates a weekly synthesis: week score (1-10), patterns, carry-forward items, goal alignment check, and a challenge for next week.

- **Access:** Automatic — appears after priorities step in Weekly Review. Skips if no AI configured.
- **Example:** "SCORE: 7/10 — Strong coding week but health goals neglected. PATTERN: Deep work sessions are productive; meetings fragment afternoons."

### AI Scripture Devotional

Personal AI reflection connecting the daily scripture verse to the user's active goals and current focus. Includes verse insight, today's application, prayer focus, and a concrete action item.

- **Access:** Command palette > "AI Scripture Devotional"
- **Example:** Given Proverbs 16:3 and goals "Launch product" and "Exercise daily", generates a reflection about committing plans to the Lord with specific connections to each goal.

### Clock / Time Tracking (CLI)

Clock in and out of work sessions from the terminal. Sessions are tagged with optional project names, tracked with elapsed time in the TUI status bar, and logged to `Timetracking/` as Markdown tables.

- **Access:** `granit clock in [--project "name"]`, `granit clock out`, `granit clock status`, `granit clock log [--week]`
- **Example:** `granit clock in --project "granit"` starts tracking. The status bar shows `[01:23:45 granit]`. `granit clock out` logs the session.

### Reminders

Scheduled reminders with daily, weekdays, or one-time patterns. Reminders fire as a terminal bell and status bar notification when the TUI is running.

- **Access:** `granit remind "text" --at HH:MM [--daily|--weekdays|--once]`, `granit remind list`, `granit remind remove <n>`
- **Example:** `granit remind "Stand up and stretch" --at 14:00 --weekdays` sets a weekday-only 2pm reminder.

### Today Dashboard (CLI)

Terminal dashboard showing today's tasks, overdue items, upcoming tasks (7 days), completed tasks, habit streaks, and clocked time totals. Supports `--json` output for scripting.

- **Access:** `granit today [vault-path]`
- **Example:** Run `granit today ~/Notes` to see "3 tasks due today, 1 overdue, 2h 15m clocked" at a glance.

### Daily / Weekly Review (CLI)

Generate a review summary of completed and pending tasks, habit completion, and time tracked. Save reviews to the `Reviews/` folder in your vault.

- **Access:** `granit review [path] [--week] [--markdown] [--save]`
- **Example:** `granit review ~/Notes --week --save` generates a weekly review and saves it as a Markdown note.

### CLI Task Add

Add tasks directly from the command line with due dates, priorities, and tags. Tasks are appended to `Tasks.md`.

- **Access:** `granit todo "text" [--due date] [--priority level] [--tag name]`
- **Example:** `granit todo "Ship v2.0" --due friday --priority high --tag release`

### Data Safety

Production-hardened data protection: atomic file writes (temp file + rename), automatic save on quit, SIGTERM/SIGHUP signal handling for graceful shutdown, clipboard timeout protection, and rune-safe text operations.

- **Access:** Automatic — no configuration needed
- **Example:** If the terminal is killed during a save, no data is lost because writes are atomic.

---

## Knowledge Tools

### Saved Views (smart collections, Alt+V)

Capacities-style saved queries over the typed-objects index. Define a view once, granit re-evaluates it whenever the tab is opened or the vault is refreshed.

**Eight built-in views ship with granit:**

- **Articles to Read** — `type:article` where `status != read AND status != archived`
- **Recent Highlights** — `type:highlight` sorted by capture date (desc)
- **Active Projects** — `type:project` where `status == active`
- **Active Goals** — `type:goal` where `status == active`, sorted by `target_date`
- **Overdue Goals** — `type:goal`, active, with a target_date set
- **Raw Ideas** — `type:idea` where `status == raw`
- **Top-Rated Podcasts** — `type:podcast` where `rating > 3`
- **Currently Reading** — `type:book` where `status == reading`

**Custom views** — drop a JSON file in `.granit/views/<id>.json`:

```json
{
  "id": "books-to-finish",
  "name": "Books to Finish",
  "description": "Books I'm partway through",
  "type": "book",
  "where": [
    { "property": "status", "op": "eq", "value": "reading" }
  ],
  "sort": { "property": "title", "direction": "asc" },
  "limit": 20
}
```

Operators: `eq`, `ne`, `contains`, `exists`, `missing`, `gt`, `lt`. AND-only; case-insensitive string compare; best-effort numeric parsing for `gt`/`lt`.

- **Access:** `Alt+V` or Command palette → "Saved Views". Opens in catalog-picker mode; Enter loads a view.
- **Inside a view:** `j/k` navigate · Enter opens note · `n` quick-create a new object of this view's type · `r` re-evaluate · `p` return to picker.

### Project / Goal Hub strip

When the active note is a typed-project (`type: project`) or typed-goal (`type: goal`), granit prepends a one-line summary above the editor:

```
🎯 Project: Apollo  ● active  · 7 tasks (3 done)  · Alt+N to add task
```

- Status badge with semantic colours (green/blue/yellow/dim)
- Linked task counts: for projects, tasks whose `Project` field matches the project title (auto-populated from the note's frontmatter); for goals, tasks written inside the goal note
- Target date (goals) or deadline (projects) when set
- `Alt+N` quick-add: appends `- [ ] ` at end of file with cursor positioned to type the title (no-op on regular notes)
- Hidden on regular notes / feature tabs / welcome screen

### Repo Tracker — local git projects as typed objects

Many of your `~/Projects/` folders are git repos. The Repo Tracker scans that root, lists each repository with live status (branch, dirty count, ahead/behind, last-commit age), and lets you import any of them as a typed-project note with one Enter — `repo:` is pre-populated, so the Project Hub strip immediately shows live git status when you open the note.

- **Open:** Command palette → "Repo Tracker"
- **Configure scan root:** Settings → `RepoScanRoot` (defaults to `~/Projects`)
- **Inside the tracker:** `j/k` navigate · `Enter` import (or jump to existing note when the row is already imported, marked with ✓) · `g` jump-only · `r` rescan + drop status cache · `Esc` close
- **Status badges:** green `clean` for in-sync repos · yellow `N dirty` and `↑N ↓M` for outstanding work · dim age for stale repos (`> 30d`)
- **Hub strip integration:** any `type: project` note with `repo: /path/to/repo` shows `git: branch · 3 dirty · ↑2 ↓0 · 2h` inline in the strip above the editor; cached for 30s so renders stay cheap
- **From any project note** (when `repo:` is set): `Alt+\` opens the folder in your system file manager (xdg-open / open / explorer); `Alt+'` copies the absolute path to the clipboard
- **Saved view:** "Code Projects" surfaces every project that has a `repo:` set — combine with the dashboard primary view to see your build pipeline at a glance

### Object Browser actions

- `Alt+O` opens the browser; left pane = type list (all 13 built-ins always visible, empty types dimmed); middle = filterable gallery; right (≥95 cols) = preview pane
- `n` create new object of focused type — title prompt in the footer; pre-populates frontmatter from the type's required properties + defaults (`{today}`, `{now}` substituted)
- `D` (also `Ctrl+D`, `Delete`) delete focused object — y/n confirmation; removes the underlying note file
- `Enter` open the object's note · `Tab` swap pane · `/` filter

### Smart Connections

TF-IDF content similarity finds semantically related notes. Shows shared keywords and a similarity score for each match.

- **Access:** Command palette > "Smart Connections"
- **Example:** On a note about "database indexing", smart connections finds "Query Optimization", "PostgreSQL Performance", and "B-Tree Data Structures".

### Writing Statistics

Word count tracking, 14-day activity chart, writing streaks, and top notes by length. Visualize your writing productivity over time.

- **Access:** Command palette > "Writing Statistics"
- **Example:** See that you wrote 2,500 words this week across 12 notes, with a 7-day streak.

### Mind Map View

ASCII mind map generated from note headings and wikilinks. Two modes: headings (document structure) and links (note connections).

- **Access:** Command palette > "Mind Map"
- **Example:** View a note's structure as a tree: the title at the center, headings as branches, and sub-headings as leaves.

### Dataview Queries

Query notes by frontmatter properties using an interactive query builder. Filter, sort, and display results from structured note metadata.

- **Access:** Command palette > "Dataview Query"
- **Example:** Query all notes where `type: project` and `status: active`, sorted by `date`, displayed as a table.

### Note Versioning Timeline

Git-based history per note with a visual timeline, colored diff viewer, and the ability to restore any previous version.

- **Access:** Command palette > "Note History"
- **Requires:** Git repository
- **Example:** View 15 previous versions of a note, compare any two versions with colored diffs, and restore the version from last Tuesday.

### Note Preview Popup

A floating preview of linked notes. When you select a note reference, see its content without navigating away.

- **Access:** Command palette > "Note Preview"; also available via cursor-on-wikilink preview
- **Example:** Hover over a wikilink to read the first 20 lines of the linked note.

### Vault Dashboard

Home screen showing today's tasks, recent notes, vault statistics, writing streaks, and a 7-day activity chart.

- **Access:** Command palette > "Dashboard"
- **Example:** See at a glance: 3 tasks due today, 5 notes modified yesterday, 1,247 total notes, 7-day writing streak.

---

## Git Integration

Built-in git overlay with three views:

| View | Description |
|------|-------------|
| **Status** | Modified, added, deleted, and untracked files |
| **Log** | Recent commit history with colored hashes |
| **Diff** | Syntax-highlighted diff of unstaged changes |

### Quick Actions

| Key | Action |
|-----|--------|
| `c` | Commit with message |
| `p` | Push to remote |
| `P` | Pull from remote |
| `r` | Refresh status |

### Auto-Sync

Optional automatic commit + push on save, pull on open. Enable via Settings > "Git Auto Sync".

### Per-Note Git History

View the commit history for any individual note, browse colored diffs between versions, and restore previous versions.

- **Access:** Command palette > "Git: Status & Commit" for the overlay; "Git History" for per-note history
- **Example:** View the last 20 commits affecting `Architecture.md`, see what changed in each commit, and restore the version from 3 days ago.

### CLI Sync

One-command pull (rebase), stage all changes, commit, and push. Auto-resolves conflicts by accepting the newest version. Supports `--quiet`, `--dry-run`, and `-m "message"`.

- **Access:** `granit sync [path] [-m "message"] [--dry-run]`
- **Example:** `granit sync ~/Notes -m "Evening sync"` pulls, commits, and pushes all changes.

### Vault Backup

Create timestamped zip backups of your vault. List available backups and restore from any archive.

- **Access:** `granit backup [path] [--output dir]`, `granit backup --restore file.zip`, `granit backup --list`
- **Example:** `granit backup ~/Notes` creates `notes-2026-03-12.zip`.

---

## Export & Publishing

### Export to HTML

Export the current note as a styled HTML document with CSS, syntax highlighting, and proper formatting.

- **Access:** Command palette > "Export Current Note" > HTML
- **Example:** Export your "API Documentation" note as a standalone HTML file for sharing.

### Export to Plain Text

Strip all Markdown formatting and export as plain text.

- **Access:** Command palette > "Export Current Note" > Text
- **Example:** Export a note for pasting into a plain-text email.

### Export to PDF

Export the current note as a clean, document-style PDF via pandoc + xelatex. Granit handles markdown preprocessing so wikilinks, task-marker emojis, and granit-specific syntax render legibly on a printed page.

- **Access:** Command palette > "Export Current Note" > PDF
- **Output:** `<note-basename>.pdf` next to the source note in the vault
- **Requires:** `pandoc` and a LaTeX engine (`xelatex` recommended for Unicode + system fonts)
  - Arch: `sudo pacman -S pandoc-cli texlive-basic texlive-latex texlive-fontsrecommended texlive-xetex texlive-binextra`
  - Debian/Ubuntu: `sudo apt install pandoc texlive-xetex texlive-fonts-recommended`
  - macOS: `brew install pandoc basictex`

**Typography defaults:**
- A4 page, 2.2 cm margins
- 11pt body with 1.35 line-stretch for documentation-grade rhythm
- Subtle blue link colour (accessible contrast on white paper)
- `tango` syntax-highlighting style for code blocks
- DejaVu Sans Mono for code fragments
- Auto Table of Contents for substantial notes (≥4 H2 sections AND ≥1500 words)

**Markdown preprocessing (so the PDF reads cleanly):**
- `[[Note Name]]` → italicized *Note Name* (no broken-link blue underline)
- `[[Note|Display]]` → italicized *Display*
- `- [ ]` / `- [x]` → Unicode `☐` / `☑` (renders correctly with default xelatex fonts)
- Task-marker emojis (📅, 🔼, 🔁, ⏰, ⏫, etc.) stripped — LaTeX's default fonts have no emoji glyphs and would render them as missing-glyph boxes
- YAML frontmatter `title:` / `date:` / `author:` populate a proper pandoc title block

**Friendly errors:**
- "pandoc is not installed" — clear actionable message instead of a Go exec error
- Missing LaTeX engine triggers a tailored hint with the package name to install

### Bulk HTML Export

Export all vault notes at once as HTML files, preserving the folder structure.

- **Access:** Command palette > "Export Current Note" > Bulk HTML
- **Example:** Export your entire vault as a browsable HTML archive.

### Static Site Publisher (in-vault)

Export your vault as a complete HTML website with search functionality, tag pages, and wikilink resolution. Creates a self-contained site that can be hosted anywhere.

- **Access:** Command palette > "Publish Site"
- **Example:** Publish your knowledge base as a searchable static website.

### `granit publish` -- Obsidian-Publish-style folder-to-website

A dedicated CLI subcommand that renders any folder of markdown notes to a black-and-white static site. Inspired by Obsidian Publish; designed for GitHub Pages but works on any static host (Cloudflare Pages, Netlify, S3, fleetdeck, plain VPS via rsync).

```bash
granit publish build ~/Notes/Research --output ./docs --title "Research"
granit publish preview ~/Notes/Research          # local server on :8080
granit publish init ~/Notes/Research             # generate publish.json template
```

What ships in every build:

| File | Purpose |
|------|---------|
| `index.html` | Auto note list, OR a homepage note (`--homepage README.md`), OR hero layout (`--hero`) |
| `notes/<slug>.html` | One page per note: outline panel, prev/next nav, backlinks, tags |
| `tags/index.html` + `tags/<tag>.html` | Tag pages, auto-generated from frontmatter + inline `#tags` |
| `graph.html` | Force-directed wikilink graph as inline SVG (deterministic, JS-free) |
| `impressum.html` / `datenschutz.html` | Auto-detected German legal pages with footer links (filename or `legal:` frontmatter) |
| `feed.xml` | RSS 2.0 feed with `<link rel="alternate">` auto-discovery |
| `sitemap.xml` + `robots.txt` | SEO essentials |
| `404.html` | Custom 404 page (GitHub Pages serves automatically) |
| `og/<slug>.png` | Auto-generated 1200x630 OG images (enable with `--auto-og`) |
| `style.css` | 4 KB B&W theme, mobile-responsive, light + dark via `prefers-color-scheme` |
| `search.js` + `search-index.json` | ~30 lines of vanilla JS fuzzy-filter, no framework |
| `.nojekyll` | GitHub Pages compatibility |

Per-page features:

- Wikilinks (`[[Note]]`, `[[Note|Display]]`, `[[Note#section]]`) resolved across the published set
- Backlinks panel ("Linked from")
- Per-note Contents outline (collapsible, H2-H4)
- Reading-time chip for notes over 100 words
- Open Graph + Twitter Card + JSON-LD Article schema for rich social previews
- KaTeX math rendering (`--math`, opt-in, loaded only on pages with `$math$`)
- Mermaid diagrams (`--mermaid`, opt-in, loaded only on pages with ` ```mermaid ` blocks)
- Code highlighting via chroma (B&W "bw" style)
- Cookie banner (`--cookie-banner`, opt-in, localStorage dismissal)
- "Built with Granit" footer link (suppressible via `--no-branding`)

Frontmatter directives:

```yaml
---
title: ...           # overrides H1/filename fallback
date: 2026-04-08     # shown under title; sorts the index page
tags: [a, b, c]      # array OR "a, b, c" comma string
author: Jane Doe     # per-note author
image: cover.png     # per-note og:image (relative path)
publish: false       # exclude from publish even when folder is published
noindex: true        # exclude from sitemap, emit robots noindex
legal: impressum     # render to /impressum.html (root, not under /notes/)
legal: datenschutz   # render to /datenschutz.html
---
```

GitHub Pages workflow (60 seconds to live):

```bash
cd ~/your-repo
granit publish build ~/Notes/Research --output ./docs --site-url "https://you.github.io/your-repo"
git add docs/ && git commit -m "publish notes" && git push
# Repo Settings -> Pages -> Source: Deploy from a branch / docs
```

Full reference: [docs/PUBLISH.md](PUBLISH.md).

### Blog Publisher

Publish individual notes to external platforms:

| Platform | Features |
|----------|----------|
| **Medium** | Publish as draft, public, or unlisted; automatic tag extraction from frontmatter |
| **GitHub** | Push Markdown to any repository and branch |

Tokens are saved to the global config for reuse.

- **Access:** Command palette > "Publish to Blog"
- **Example:** Publish a polished note to Medium as a draft with tags from frontmatter.

---

## Extensibility

### Plugin System

Language-agnostic plugin system using scripts with JSON manifests. Plugins can respond to hooks, add commands to the palette, and transform note content.

- **Access:** Command palette > "Plugins"
- **See:** [PLUGINS.md](PLUGINS.md) for the full development guide

### Lua Scripting

Full scripting API with access to vault operations. Scripts can read/write notes, access frontmatter, insert text at the cursor, and replace editor content.

Available API functions:

| Function | Description |
|----------|-------------|
| `granit.read_note(name)` | Read another note's content |
| `granit.write_note(name, content)` | Write or overwrite a note |
| `granit.list_notes()` | List all .md files in the vault |
| `granit.note_path` | Current note's file path |
| `granit.note_content` | Current note's content |
| `granit.vault_path` | Vault root directory |
| `granit.note_name` | Current note name (without .md) |
| `granit.frontmatter` | Table of frontmatter key-value pairs |
| `granit.date(format)` | Current date (default: YYYY-MM-DD) |
| `granit.time()` | Current time (HH:MM:SS) |
| `granit.msg(text)` | Display a status message |
| `granit.set_content(text)` | Replace editor content |
| `granit.insert(text)` | Insert text at cursor |

- **Access:** Command palette > "Lua Scripts"
- **Script locations:** `<vault>/.granit/lua/` and `~/.config/granit/lua/`

### Core Plugins Toggle

Enable or disable 16 built-in modules via Settings > Core Plugins:

Task Manager, Calendar, Canvas, Graph View, Flashcards, Quiz Mode, Pomodoro, Git Integration, Blog Publisher, AI Templates, Research Agent, Language Learning, Habit Tracker, Ghost Writer, Encryption, Spell Check.

- **Access:** `Ctrl+,` > scroll to "Core Plugins" section

### Canvas / Whiteboard

Visual 2D canvas for arranging notes, creating connections, and organizing ideas spatially. Add notes as cards, draw connections between them, and color-code groups.

- **Access:** `Ctrl+W`
- **Example:** Create a visual map of your project's architecture with notes as cards and arrows showing dependencies.

### Split Panes

View two notes side by side for comparison or reference.

- **Access:** Command palette > "Split View"
- **Example:** Compare two drafts of a document or reference one note while editing another.

### Obsidian Import

Import settings from an existing `.obsidian/` directory to ease migration.

- **Access:** Command palette > "Import Obsidian Config"
- **Example:** Migrate your Obsidian theme preference and editor settings to Granit.

---

## Customization

### 35 Built-In Themes

29 dark themes and 6 light themes. See [THEMES.md](THEMES.md) for the complete reference.

- **Access:** Settings > "Theme" or Command palette > Theme Editor
- **Example:** Switch from `catppuccin-mocha` to `tokyo-night` instantly.

### Theme Editor

Live-edit all 16 color roles with hex values, preview changes instantly, and save/export custom themes as JSON.

- **Access:** Command palette > "Theme Editor"
- **Example:** Adjust the Primary color from purple to teal and see every heading, border, and accent update in real time.

### 4 Icon Sets

| Set | Description |
|-----|-------------|
| **Unicode** | Standard Unicode symbols (default) |
| **Nerd Font** | Nerd Font glyphs (requires a Nerd Font) |
| **Emoji** | Emoji icons |
| **ASCII** | Plain ASCII characters (maximum compatibility) |

- **Access:** Settings > "Icon Theme"

### 8 Panel Layouts

| Layout | Panels | Description |
|--------|--------|-------------|
| **Default** | 3 | Sidebar + Editor + Backlinks |
| **Writer** | 2 | Sidebar + Editor |
| **Minimal** | 1 | Editor only |
| **Reading** | 2 | Editor + Backlinks (wide editor) |
| **Dashboard** | 4 | Sidebar + Editor + Outline + Backlinks |
| **Zen** | 1 | Centered distraction-free editor |
| **Taskboard** | 3 | Sidebar + Editor + Task summary |
| **Research** | 3 | Sidebar + Editor + Notes panel |

Adaptive layout degradation: terminals narrower than 80 columns automatically switch to Minimal layout; terminals narrower than 120 columns switch to Writer layout.

- **Access:** Settings > "Layout" or Command palette > "Default Layout", "Writer Layout", etc.

### Adaptive Terminal Layout

Granit automatically adjusts to the terminal size. Small terminals get simplified layouts to ensure usability on any screen size, including mobile terminals.

- **Example:** On a phone terminal (40 columns), Granit shows only the editor — no sidebar or backlinks.
