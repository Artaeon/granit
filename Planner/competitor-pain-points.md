---
date: 2026-03-09
type: research
---

# User Pain Points in Note-Taking / Knowledge Management Apps

Comprehensive research from Reddit, forums, blogs, and review sites (2024-2026).
Focused on complaints about Obsidian, Logseq, and the broader PKM space.
Prioritized by frequency of mention across multiple independent sources.

---

## 1. PERFORMANCE & RESOURCE USAGE (Very Frequently Mentioned)

### Electron Bloat
- Obsidian consumes ~2 GB RAM with a single note open (multiple Electron processes)
- Neovim does the same task in 56 MB — a 35x difference
- Users on older hardware or laptops report constant sluggishness
- Obsidian 1.9.10+ introduced regressions from Electron 37 update (lag, choppiness)

### Large Vault Performance
- Vaults with 3,000+ files: noticeable search delays, 2-second debounce lag
- Vaults with 40,000 files: 4-second delay PER KEYSTROKE when typing [[wikilinks
- Graph view becomes "practically unusable" with thousands of notes
- Mobile app takes 30+ seconds to load; sometimes over 1 minute with plugins

### Startup Time
- Desktop: slow with 25+ plugins enabled
- Mobile: consistently 30+ seconds; disabling plugins drops to ~5 seconds
- Users created "Lazy Plugin Loader" and "FastStart" scripts — the problem is so bad the community builds workarounds

**GRANIT OPPORTUNITY**: Native Go binary = instant startup, tiny memory footprint. This is the single biggest architectural advantage over every Electron-based competitor. Emphasize this relentlessly.

---

## 2. PLUGIN DEPENDENCY & FRAGILITY (Very Frequently Mentioned)

### The Core Problem
- ~3,000 community plugins, but essential features depend on third-party code
- Plugin updates break other plugins; "rolling the dice" with each update
- Abandoned plugins gradually degrade — developer walks away, bugs accumulate
- Plugin conflicts: link display modifier breaks backlinks tracker, both stop working
- One user's entire brainstorming session was corrupted by plugin update + sync conflict

### Features Users Want Built-In (Not as Plugins)
- Kanban boards
- Advanced tables (WYSIWYG cell editing, merged cells, multi-line cells)
- Dataview-style queries (aggregate tasks across vault, custom note views)
- Calendar view for daily notes
- Emoji support
- Text highlighting/colored text
- Database/structured data views (board, list, gallery, timeline)
- Task management beyond simple checkboxes

### Decision Fatigue
- New users overwhelmed by 2,500+ plugins before they write their first note
- "Spent more time managing my second brain than using it"
- "Plugin rabbit holes" — weeks researching plugins instead of writing

**GRANIT OPPORTUNITY**: Every feature is built-in and maintained by the same codebase. No plugin ecosystem to manage, no breakage on updates, no abandoned third-party code. Features like kanban (canvas), calendar, templates, bookmarks, tags, find/replace, outline, and export are already native.

---

## 3. STEEP LEARNING CURVE & BAD ONBOARDING (Frequently Mentioned)

### Empty Vault Problem
- New users see a blank screen with cryptic sidebar icons
- No tutorial, no suggested workflow, no "here's how most people use this"
- Notion/Evernote provide templates and guided setup; Obsidian provides nothing
- Users try to replicate elaborate Reddit setups before writing a single note

### Confusing Terminology
- "Vault" is just a folder but sounds intimidating
- "Backlinks" confuse beginners who don't understand bidirectional linking
- Graph view looks cool with 5 notes but provides zero value until hundreds

### Configuration Before Productivity
- Users report spending days/weeks configuring before doing actual work
- "You will spend more time figuring things out and researching rather than actually using the tool"
- CSS snippets required for visual customization — demands technical knowledge
- Markdown syntax must be memorized; no intuitive formatting toolbar

**GRANIT OPPORTUNITY**: Splash screen, built-in templates (Ctrl+N), command palette with discoverable commands, sensible defaults, settings overlay with clear descriptions. Consider adding a "first run" experience that creates a sample note and shows key shortcuts.

---

## 4. SYNC & COLLABORATION (Frequently Mentioned)

### Paid Sync Complaints
- Obsidian Sync costs $4-8/month for a feature competitors include free
- "Charging for cross-device access feels like a basic feature being monetized"
- DIY sync via Dropbox/iCloud/Git causes merge conflicts and data loss
- iCloud sync specifically causes delays and corruption with Obsidian vaults
- Logseq Sync has caused "massive data loss" for multiple users

### No Real-Time Collaboration
- Obsidian had zero native collaboration until 2026 (now limited, via Sync only)
- All collaborators must purchase Sync licenses
- Teams need shared editing, permissions, single source of truth
- This is the #1 reason teams choose Notion/Google Docs over Obsidian

### Data Loss Incidents
- Files reverting to older states after sync conflicts
- Folders disappearing after restart (Live Preview editor implicated)
- Plugin + sync conflict = corrupted notes
- Logseq users report data vanishing when using third-party sync (iCloud, Dropbox)

**GRANIT OPPORTUNITY**: Git-based sync is built-in and free. Local-first means no cloud dependency. For a terminal app, git is the natural sync mechanism and users already understand it. Data loss prevention through git history/restore is already implemented.

---

## 5. MOBILE EXPERIENCE (Frequently Mentioned)

### Slow and Impractical
- 30+ second load times make quick capture impossible
- "Actually terrible for quick capture" — the most common mobile note-taking need
- Mobile Quick Capture has been on Obsidian's roadmap for years, still not shipped
- Touch interfaces feel clunky; not designed for mobile-first use
- Text appears too large on Logseq mobile; UI described as "basic"

### The Quick Capture Gap
- Users want to jot a thought in <3 seconds on their phone
- Obsidian mobile requires waiting for vault sync, app load, navigation
- Many users carry a separate app (Apple Notes, Google Keep) just for capture
- This is a massive workflow break — captured thoughts never make it back to vault

**GRANIT OPPORTUNITY**: Terminal apps can run over SSH on any device. The `granit serve` command already enables read-only web access. Consider a lightweight mobile web capture endpoint that appends to inbox/daily note.

---

## 6. TABLE EDITING (Frequently Mentioned)

### Markdown Tables Are Painful
- Only support one line of markdown per cell
- No merged cells, no multi-line content in cells
- HTML workarounds create unreadable source that requires horizontal scrolling
- Wikilinks don't work inside HTML tables
- Raw markdown editing for tables is "unusable" for complex tables

### What Users Want
- WYSIWYG table editor (edit cell by cell, like Notion/OneNote)
- Multi-line cell content (paragraphs, lists, formatted text)
- Cell merging
- Easy import of tables from external sources
- Tables as a first-class feature, not an afterthought

**GRANIT OPPORTUNITY**: The TUI editor could offer a table-aware editing mode that makes Markdown table editing less painful (auto-alignment, tab between cells, easy column add/remove).

---

## 7. GRAPH VIEW CRITICIZED AS USELESS (Moderately Mentioned)

### Common Complaints
- "Looked cool but didn't help with anything practical"
- Becomes chaotic after a few dozen notes; lines crossing everywhere
- Force-directed layout gives no meaningful spatial information
- Users who invested in linking feel betrayed when graph provides no insights
- Default view is overwhelming; needs plugins (Juggl) to be useful

### What Would Make It Useful
- Clustering by tags/folders
- Better layout algorithms (hierarchical, radial, grouped)
- Ability to embed local graph in notes
- Search within graph
- Layer controls (show/hide by type)

**GRANIT OPPORTUNITY**: Granit's graph overlay already exists. Consider making it more functional than a visualization — show clusters, allow navigation, filter by tags. A TUI graph that is actually useful for discovery would be a differentiator.

---

## 8. TASK MANAGEMENT LIMITATIONS (Moderately Mentioned)

### Obsidian's Weakness
- Not built for task management; requires Tasks plugin for basics
- Tasks plugin requires significant setup and ongoing maintenance
- No native recurring tasks, priorities, due dates, or kanban
- Can't aggregate tasks across multiple notes without Dataview plugin
- Logseq handles tasks natively and better — a key reason people switch

### What Users Want
- Tasks that surface automatically based on due date
- Recurring task support
- Priority levels
- Kanban/board view of tasks
- Tasks visible in calendar view
- Cross-note task aggregation without plugins

**GRANIT OPPORTUNITY**: Calendar already parses tasks. Consider adding a dedicated task view/overlay that aggregates all `- [ ]` items across the vault with filtering by tag, date, priority. This is a major differentiator if done natively.

---

## 9. NOT OPEN SOURCE / TRUST CONCERNS (Moderately Mentioned)

### The Complaint
- Obsidian is free but NOT open source — source code is proprietary
- Users worry about future monetization, feature removal, price increases
- "If the tool I trusted to capture ideas was actively losing ideas, it was a liability"
- Logseq's open-source nature is cited as a key reason to switch
- Joplin being "actually open-source" is a selling point

### What Users Value
- Ability to inspect code for data handling practices
- Guarantee against vendor lock-in
- Community can fork if development stalls
- Self-hosted options eliminate cloud dependency

**GRANIT OPPORTUNITY**: If Granit ever goes open source, this becomes a significant advantage. Even without it, being a local-first Go binary with plain Markdown files and git-based sync provides strong lock-in protection.

---

## 10. AI & AUTOMATION GAPS (Moderately Mentioned)

### Current State
- Obsidian has no native AI features
- Users must use external tools (NotebookLM, ChatGPT) for summarization, connection finding
- Breaking workflow to switch to AI tools defeats the purpose of a knowledge base
- Notion charges $8/month extra for AI add-on — widely criticized
- Users want AI that works on LOCAL data without sending it to the cloud

### What Users Want
- Summarize notes / meetings within the app
- Find connections across research papers
- Auto-tag notes based on content
- Writing assistance (grammar, style, expansion)
- Question answering over your own vault
- All of this running LOCALLY for privacy

**GRANIT OPPORTUNITY**: Already has 9 AI bots with Ollama (local), OpenAI, and local fallback. Auto-tagger, link suggester, summarizer, question bot, writing assistant, title suggester, action items, MOC generator, daily digest. This is a massive built-in advantage — emphasize local AI that never sends data to the cloud.

---

## 11. LACK OF NATIVE DRAWING / HANDWRITING (Occasionally Mentioned)

- No built-in drawing tools; must use external programs
- Excalidraw plugin exists but "doesn't feel good" for serious handwriting use
- Apple Pencil support inadequate compared to Apple Notes, GoodNotes
- Users who think visually feel like second-class citizens

**GRANIT OPPORTUNITY**: Limited in TUI context, but the canvas feature (Ctrl+W) provides visual 2D note arrangement. This is a pragmatic alternative for spatial thinkers.

---

## 12. PRICING MODEL CONFUSION (Occasionally Mentioned)

- Free app but Sync ($4-8/mo), Publish ($8/mo), and Commercial License are separate
- All collaborators need Sync licenses — barrier to team adoption
- Catalyst (early supporter) license gives no credit toward other tiers
- "Nickeled-and-dimed" feeling from stacking multiple payment tiers
- Personal vs. commercial use distinction feels artificial

**GRANIT OPPORTUNITY**: 100% free, no tiers, no subscriptions, no upsells.

---

## 13. LOCALIZATION & ACCESSIBILITY (Occasionally Mentioned)

- Documentation and plugins predominantly English-only
- No complete translations for many languages
- $10 USD for Sync feels exploitative in countries with weaker currencies (e.g., Brazil)
- No intuitive font size controls for individual text
- Markdown syntax requirement excludes non-technical users

---

## 14. WHAT TERMINAL/CLI USERS SPECIFICALLY WANT

From r/commandline, r/vim, r/neovim, and developer blogs:

### Core Requirements
- Plain markdown files, no proprietary format
- Editor-agnostic OR built-in vim-like editing
- Fast search without UI overhead (fzf, ripgrep integration)
- Minimal composability with GNU tools (grep, date, awk)
- Git-backed versioning and sync
- Instant startup, tiny resource footprint
- Works over SSH

### Features Wished For
- Zettelkasten / wiki-link support in terminal
- Fuzzy file switching
- Frontmatter YAML for metadata (tags, dates)
- Templates for new notes
- Daily notes / journaling workflow
- Cross-note search and backlinks
- Mobile access without a heavy app

### The Gap
- Glow: beautiful reader but no editing beyond delegating to vim/nano
- nb: comprehensive but complex CLI syntax, not a TUI
- zk: powerful CLI but no visual interface
- vim-wiki: good but requires heavy vim plugin configuration
- No single tool combines: TUI interface + wiki-links + graph + search + git + templates + AI

**GRANIT OPPORTUNITY**: Granit is literally the only tool that fills this entire gap. A native Go TUI with wiki-links, graph view, fuzzy search, templates, git integration, AI bots, multiple themes, and vim-style editing — all in a single binary. This positioning should be central to marketing.

---

## 15. LOGSEQ-SPECIFIC COMPLAINTS (For Competitive Awareness)

- Slow startup and performance with large graphs (2190+ files)
- Data loss from sync features (multiple reports)
- "Almost unusable" bug reports
- Basic UI/UX on both desktop and mobile
- Sparse documentation for advanced queries
- DB version endlessly delayed ("The endless wait for Logseq DB")
- Tasks marked "doing" vanish from diary views
- Plugin stability issues and key-binding conflicts
- Third-party sync (iCloud, Dropbox) causes data corruption

---

## SUMMARY: TOP OPPORTUNITIES FOR GRANIT

Ranked by user pain intensity and Granit's ability to address them:

1. **Performance** — Native Go binary vs. Electron. Instant startup, 50x less RAM. Lead with this.
2. **Built-in features** — No plugin dependency. Everything works out of the box, maintained together.
3. **Local AI** — Ollama integration for private, on-device AI. No cloud, no subscription.
4. **Free sync via Git** — No $8/month Obsidian Sync. Git is the natural developer sync tool.
5. **Terminal-native** — The only TUI that combines wiki-links + graph + search + templates + AI.
6. **Fast onboarding** — Templates, command palette, sensible defaults vs. empty vault paralysis.
7. **Task aggregation** — Potential to add cross-vault task views natively.
8. **Table editing** — Potential to offer better markdown table editing in TUI.
9. **Open/transparent** — Plain files, git history, no vendor lock-in, no proprietary format.
10. **Stable** — No plugin conflicts, no abandoned third-party code, no update roulette.
