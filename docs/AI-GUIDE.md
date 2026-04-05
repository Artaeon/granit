# Granit — AI Setup & Usage Guide

> Complete guide to configuring and using Granit's 25+ AI-powered features, including 19 built-in bots and a production-grade reliability stack optimized for small local models.

---

## Table of Contents

- [AI Provider Overview](#ai-provider-overview)
- [Local Provider (Zero Setup)](#local-provider-zero-setup)
- [Ollama Setup (Recommended)](#ollama-setup-recommended)
- [OpenAI Setup](#openai-setup)
- [Claude Code Setup (Deep Dive Research)](#claude-code-setup-deep-dive-research)
- [Reliability & Performance](#reliability--performance)
- [Bots — The AI Toolkit](#bots--the-ai-toolkit)
- [AI Feature Reference](#ai-feature-reference)
- [Troubleshooting](#troubleshooting)

---

## AI Provider Overview

Granit supports three AI providers. All AI features work with any provider, adapting their behavior accordingly:

| Provider | Type | Quality | Privacy | Cost | Setup |
|----------|------|---------|---------|------|-------|
| **Local** | Offline | Basic | Full privacy | Free | None required |
| **Ollama** | Local LLM | High | Full privacy | Free | Install Ollama + model |
| **OpenAI** | Cloud API | Highest | Data sent to OpenAI | Pay per use | API key required |

The provider can be set in three ways:

1. **Settings overlay:** `Ctrl+,` > "AI Provider"
2. **Config file:** Set `"ai_provider"` in `~/.config/granit/config.json`
3. **Per-vault override:** Set `"ai_provider"` in `<vault>/.granit.json`

---

## Local Provider (Zero Setup)

The default `"local"` provider works offline with no configuration. It uses:

- **Keyword extraction** with stopword filtering
- **Topic detection** via pattern matching
- **TF-IDF similarity** for related note suggestions
- **Rule-based analysis** for tagging, linking, and summarization

This provider is less sophisticated than LLM-based providers but provides useful results for:

- Auto-tagging (keyword-based)
- Link suggestions (title matching)
- Similar notes (TF-IDF cosine similarity)
- Basic summarization (extractive)

### When to Use Local

- You want zero setup and offline-only operation
- You're on a machine without GPU or limited RAM
- Privacy is paramount and you don't want any LLM running
- You just want basic AI assistance without the overhead

---

## Ollama Setup (Recommended)

Ollama runs open-source LLMs locally on your machine. This is the recommended provider for the best balance of quality, privacy, and cost.

### Built-In Setup Wizard

Granit includes an automated Ollama setup wizard:

1. Open Settings: `Ctrl+,`
2. Navigate to **">> Setup Ollama (install + model)"**
3. Press `Enter`
4. The wizard will:
   - Check if Ollama is installed
   - Install Ollama if needed (via the official install script)
   - Pull your configured model
   - Verify the connection
5. Change "AI Provider" to `ollama`

### Manual Setup

If you prefer manual installation:

```bash
# 1. Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# 2. Start the Ollama server
ollama serve

# 3. Pull a model (in a separate terminal)
ollama pull qwen2.5:0.5b
```

Then configure Granit:

```json
{
  "ai_provider": "ollama",
  "ollama_model": "qwen2.5:0.5b",
  "ollama_url": "http://localhost:11434"
}
```

### Model Recommendations by RAM

| Available RAM | Model | Parameters | Quality | Speed |
|---------------|-------|------------|---------|-------|
| **4 GB** | `qwen2.5:0.5b` | 500M | Adequate | Very fast |
| **4 GB** | `tinyllama` | 1.1B | Basic | Fast |
| **8 GB** | `qwen2.5:1.5b` | 1.5B | Good | Fast |
| **8 GB** | `phi3:mini` | 3.8B | Good | Moderate |
| **8 GB** | `gemma2:2b` | 2B | Good | Fast |
| **16 GB** | `qwen2.5:3b` | 3B | Very good | Moderate |
| **16 GB** | `phi3.5:3.8b` | 3.8B | Very good | Moderate |
| **16 GB** | `llama3.2:1b` | 1B | Good | Very fast |
| **32 GB+** | `llama3.2` | 8B | Excellent | Moderate |
| **32 GB+** | `mistral` | 7B | Excellent | Moderate |
| **32 GB+** | `gemma2` | 9B | Excellent | Slower |

**Recommendation:** Start with `qwen2.5:0.5b` (fast, works on any hardware) and upgrade to a larger model if you have the RAM. The `qwen2.5` family provides the best quality-to-size ratio for note-taking tasks.

### Available Models in Settings

When you open Settings (`Ctrl+,`) and navigate to "Ollama Model", you can select from these pre-configured options:

- `qwen2.5:0.5b`
- `qwen2.5:1.5b`
- `qwen2.5:3b`
- `phi3:mini`
- `phi3.5:3.8b`
- `gemma2:2b`
- `tinyllama`
- `llama3.2`
- `llama3.2:1b`
- `mistral`
- `gemma2`

You can also manually edit the config file to use any model available on Ollama.

### Custom Ollama URL

If Ollama runs on a different machine or port:

```json
{
  "ollama_url": "http://192.168.1.100:11434"
}
```

### Automatic Model Unloading

When Granit exits, it automatically unloads the Ollama model from memory by calling `ollama stop <model>`. This frees GPU/RAM for other applications.

### Ollama API Details

Granit communicates with Ollama via HTTP:

- **Endpoint:** `POST {ollama_url}/api/generate`
- **Request format:** `{"model": "...", "prompt": "...", "stream": false}`
- **Response format:** `{"response": "..."}`
- **Async:** All Ollama calls run as background `tea.Cmd` operations — the UI remains responsive.

---

## OpenAI Setup

Use OpenAI's cloud API for the highest quality AI responses.

### Configuration

1. Get an API key from [platform.openai.com](https://platform.openai.com/api-keys)
2. Configure in Settings (`Ctrl+,`) or config file:

```json
{
  "ai_provider": "openai",
  "openai_key": "sk-...",
  "openai_model": "gpt-4o-mini"
}
```

### Available Models

| Model | Quality | Speed | Cost |
|-------|---------|-------|------|
| `gpt-4o-mini` | Good | Fast | Low |
| `gpt-4.1-nano` | Good | Very fast | Very low |
| `gpt-4.1-mini` | Very good | Fast | Low |
| `gpt-4o` | Excellent | Moderate | Higher |

**Recommendation:** `gpt-4o-mini` provides excellent results for note-taking tasks at a low cost per request.

### OpenAI API Details

- **Endpoint:** `POST https://api.openai.com/v1/chat/completions`
- **Authentication:** Bearer token via `Authorization` header
- **Request format:** Standard OpenAI chat completions with `messages` array
- **Async:** All calls run in background; the UI stays responsive

### Security Notes

- The API key is stored in your config file (`~/.config/granit/config.json`)
- The config file permissions should be restricted: `chmod 600 ~/.config/granit/config.json`
- Note content is sent to OpenAI's servers when using their API
- For maximum privacy, use Ollama or the local provider instead

---

## Claude Code Setup (Deep Dive Research)

Claude Code is used exclusively for the research-oriented AI features. It runs as an external CLI tool — no API key configuration is needed in Granit.

### Features Requiring Claude Code

| Feature | Command Palette Entry |
|---------|----------------------|
| Deep Dive Research | "Deep Dive Research" |
| Research Follow-Up | "Research Follow-Up" |
| Vault Analyzer | "Vault Analyzer" |
| Note Enhancer | "Note Enhancer" |
| Daily Digest | "Daily Digest" |

### Installation

1. Install Claude Code following the [official documentation](https://docs.anthropic.com/en/docs/claude-code)
2. Authenticate: `claude login`
3. Verify: `which claude` should return a path

No additional configuration in Granit is needed — it simply calls the `claude` binary.

### Deep Dive Research

The flagship Claude Code feature. Given a topic, it:

1. Searches the web for current, authoritative information
2. Creates 5-25 interconnected notes in the vault's `Research/` folder
3. Generates a hub note (`_Index.md`) linking all research notes
4. Adds proper frontmatter, tags, and `[[wikilinks]]`

#### Research Profiles

| Profile | Description | Best For |
|---------|-------------|----------|
| **General** | Balanced coverage | Most topics |
| **Academic** | Scholarly sources, citations | Research papers, theory |
| **Technical** | Code examples, architecture | Programming, engineering |
| **Creative** | Diverse perspectives, analogies | Writing, brainstorming |

#### Source Filters

| Filter | Description |
|--------|-------------|
| **Any** | All available sources |
| **Web** | Web articles and blogs |
| **Docs** | Official documentation |
| **Papers** | Academic papers and research |

#### Output Formats

| Format | Description |
|--------|-------------|
| **Zettelkasten** | Atomic notes, one idea per note, densely interlinked |
| **Outline** | Hierarchical structure, main topic with subtopics |
| **Study Guide** | Includes flashcard-ready Q&A sections |

#### Depth Levels

| Level | Notes Created | Detail |
|-------|--------------|--------|
| **Quick** | 5-10 | Overview and key points |
| **Standard** | 10-15 | Comprehensive coverage |
| **Deep** | 15-25 | Exhaustive, with subtopics |

#### CLAUDE.md Integration

If your vault contains a `CLAUDE.md` file, Claude Code reads it for context about your vault structure, writing style preferences, and persona. This allows the research output to match your vault's conventions.

#### Soul Note Persona

The AI Writing Coach feature supports a "soul note" persona — a note in your vault that describes your writing voice, style preferences, and communication philosophy. When present, AI features adapt their output to match your preferred style.

#### Background Processing

Research runs in the background. A status indicator appears in the status bar showing progress. You can:

- Continue editing while research runs
- Close the research overlay — it keeps running
- Reopen the overlay to check progress
- Cancel a running research task

### Vault Analyzer

Analyzes your entire vault structure via Claude Code:

- Identifies orphan notes (no incoming or outgoing links)
- Finds structural gaps (topics that should be connected)
- Suggests new connections between notes
- Recommends folder reorganization
- Highlights duplicate or overlapping content

### Note Enhancer

Enhances the current note using Claude Code:

- Adds `[[wikilinks]]` to existing notes where appropriate
- Improves document structure (headings, sections)
- Expands thin sections with deeper content
- Fixes formatting inconsistencies

### Daily Digest

Generates a weekly review by analyzing recently modified notes via Claude Code:

- Summarizes what was created and modified
- Identifies trends and themes in your recent work
- Highlights completed tasks and milestones
- Suggests next steps and areas needing attention

---

## Reliability & Performance

Granit's AI stack is designed to be **rock-solid on slow, local, small models** (0.5B–3B parameters) as well as on large cloud models. Every AI call passes through a central reliability layer that handles:

### Small-model auto-detection

Granit automatically detects small models by name (`0.5b`, `1b`, `1.5b`, `2b`, `3b` suffixes, plus `tinyllama`, `phi3:mini`, `gemma:2b`, etc.) and adapts its behavior:

| Feature | Large model | Small model |
|---|---|---|
| `num_ctx` | 4096 | 2048 |
| `num_predict` | 1024 | 512 |
| `temperature` | 0.7 | 0.3 (more deterministic) |
| Content per prompt | Full (2000–6000 chars) | Reduced (800–2000 chars) |
| System prompts | Full instructions | Compact format-only |
| Ghostwriter debounce | 800ms | 1500ms (avoids pile-up) |

All 25+ AI features have small-model-aware prompt variants — the output format stays the same, but the instructions become terse and focused. This gives 0.5B models a real chance at producing usable output.

### Retry on transient errors

Every AI request is automatically retried **once with a 500ms backoff** when it hits a transient error (connection refused, timeout, EOF, reset). Permanent errors (bad API key, missing model, unauthorized) are not retried. Retry works transparently for both sync calls (`Chat`) and streaming requests — no configuration needed.

### Real HTTP cancellation

Pressing **Esc** during an AI loading screen now *actually* aborts the underlying HTTP request via `context.Context`, freeing the local model's CPU/GPU immediately. This is implemented across AI Chat, Plan My Day, Task Triage, and all bot overlays. No more goroutines eating resources on responses you don't want.

### Hard deadlines

Every feature has a bounded lifetime:

| Feature | Deadline |
|---|---|
| Ghostwriter completion | 15s (30s small models) |
| Auto-tag on save | 45s (90s small models) |
| Auto-link on save | 45s (90s small models) |
| Bots | 3 minutes |
| Streaming (AI chat, Plan My Day) | 5 minutes + Esc cancellation |

A hung Ollama or stalled network can't lock the UI forever.

### In-flight guards

Auto-tag and auto-link suggestions on save will **skip** if a previous request is still running. Rapid saves no longer pile up concurrent requests, which was a silent killer on slow local models.

### Token-budget fit checks

Before sending, auto-features estimate the token count of the prompt (~4 chars/token) and compare it to the model's effective context window (`num_ctx - num_predict`). Oversized prompts are skipped gracefully instead of being silently truncated by Ollama.

### Ghostwriter completion cache

A 32-entry FIFO cache stores recent ghostwriter completions keyed by the exact note context. When you backspace and retype the same content (common typing pattern), the suggestion is served instantly with **zero AI round-trip**. The cache is automatically invalidated when you switch models.

### Empty-response fallback

When a small model returns an empty or whitespace-only response (common failure mode on complex prompts), bots fall back to local analysis with a clear yellow warning instead of showing an empty result screen.

### Elapsed time display

Every AI loading screen shows elapsed seconds with a spinner. After 15s on a small model, a hint appears: *"Small models can be slow — consider a larger model for complex tasks"*. After 30s, a generic *"Taking longer than usual"* hint. You always know what's happening.

### Word-boundary truncation

All content truncation (prompt building, snippet rendering, ghostwriter vault-note previews) respects word boundaries via `truncateAtBoundary`. No mid-word cuts that confuse small models.

### Unicode correctness

Editor, git commit input, and tag parsing all correctly handle multi-byte UTF-8 characters — emoji, accented letters (café, über), CJK (日本語), etc. The auto-tagger preserves Unicode letters and digits in tags via `unicode.IsLetter`/`IsDigit`.

### Deterministic iteration

Bots that iterate over the vault notes (Question Bot, MOC Generator) sort paths before iterating so Go's random map ordering doesn't produce inconsistent results across runs.

### Multi-line YAML frontmatter

Tag extraction supports all three Obsidian-compatible formats:

```yaml
tags: [one, two, three]      # inline list
tags: one, two, three        # inline comma
tags:                        # multi-line list
  - one
  - two
  - three
```

---

## Bots — The AI Toolkit

The **Bots overlay** (`Ctrl+R`) is Granit's main AI toolkit: 19 specialized bots organized into 6 categories, all sharing the reliability stack above.

### Opening and navigating

- **`Ctrl+R`** — open the Bots overlay
- **Type to filter** — start typing to narrow the list by name or description
- **`↑`/`↓`** / **`k`/`j`** — select (wraps around at top/bottom)
- **`home`/`end`** — jump to first/last bot
- **`1`–`9`** — quick-pick the first nine visible bots
- **`Enter`** — run the selected bot
- **`Esc`** — clear filter (if set), otherwise close the overlay

The list remembers your last-used bot and positions the cursor on it the next time you open. Categories are shown as bold mauve headers when there's no filter active; once you start typing, the list collapses to a flat filtered view.

### Bot categories

| Category | Bots |
|---|---|
| **SUMMARIZE** | TL;DR, Summarizer, Explain Simply |
| **WRITING** | Title Suggester, Writing Assistant, Tone Adjuster, Expand |
| **ANALYSIS** | Question Bot, Counter-Argument, Pros & Cons, Action Items |
| **ORGANIZE** | Auto-Tagger, Link Suggester, Auto-Link, Outline Generator |
| **LEARNING** | Flashcard Generator, Key Terms |
| **VAULT** | MOC Generator, Daily Digest |

### Individual bot descriptions

#### SUMMARIZE

- **TL;DR** — one-sentence summary capturing the single most important idea of the note. Rendered bold for instant scanning.
- **Summarizer** — 2-4 sentence summary covering the key ideas and actionable points. Includes tag/folder metadata for grounding.
- **Explain Simply** — rewrites the note's content for a curious 12-year-old using everyday analogies and short sentences. Great for demystifying complex topics or sanity-checking your own understanding.

#### WRITING

- **Title Suggester** — 5 alternative titles (3-8 words, title case) based on content and existing tags.
- **Writing Assistant** — readability assessment + 3-5 specific suggestions to improve clarity, structure, or style. Flags passive voice, repetition, and missing structure.
- **Tone Adjuster** — rewrites the note in three tones: FORMAL, CASUAL, CONCISE. Preserves meaning; only changes style.
- **Expand** — flesh out a terse note with additional detail, context, and examples. Preserves the author's voice and structure.

#### ANALYSIS

- **Question Bot** — answers questions about your vault using only the provided notes. Builds context from keyword-matched notes with configurable size limits.
- **Counter-Argument** — acts as a devil's advocate, surfacing 3-5 strong opposing viewpoints to the note's main claims. Yellow CLAIMs, red COUNTERs.
- **Pros & Cons** — structured decision-analysis list with green pros and red cons. Identifies the decision being discussed and provides specific, non-generic items.
- **Action Items** — extracts explicit todos, implicit follow-ups, and deadlines from meeting notes or documents. Formats as `- [ ] Task @person by date`.

#### ORGANIZE

- **Auto-Tagger** — 3-5 tags for the current note. Prefers existing vault tags when they fit (reduces tag sprawl). Few-shot examples from similar notes.
- **Link Suggester** — 3-5 related notes from the vault, with a one-line reason for each. Works even on large vaults via ranked previews.
- **Auto-Link** — finds concepts in the current note that match existing notes and should be linked with `[[wikilinks]]`. Outputs `LINK: original text -> [[Note Name]]`.
- **Outline Generator** — hierarchical outline (## headings + bullet points) of the current note. Local fallback extracts existing headings when AI is unavailable.

#### LEARNING

- **Flashcard Generator** — 5-8 Q:/A: flashcards from the note. Questions are specific; answers are 1-2 sentences. Prioritizes rarely-obvious facts.
- **Key Terms** — glossary of 5-10 key terms with 1-sentence definitions grounded in the note's own context. Handles proper nouns and domain-specific jargon.

#### VAULT

- **MOC Generator** — creates a Map of Content for the entire vault. Groups notes into 3-6 categories with headings and `[[wikilinks]]`.
- **Daily Digest** — local-only bot that summarizes recent vault activity (orphan notes, most-linked notes, top tags). No AI required.

### Bot result actions

Once a bot finishes, the results view supports:

- **`c`** or **`y`** — copy raw response to the system clipboard
- **`s`** — save as a permanent vault note in `<vault>/Bots/<note>-<bot>-<timestamp>.md` with full YAML frontmatter (source wikilink, bot name, provider, model, `ai-generated` tag)
- **`r`** — re-run the same bot (huge workflow win when a small model gives a weird response)
- **`j`**/**`k`**, **`pgup`**/**`pgdown`**, **`ctrl+u`**/**`ctrl+d`** — scroll
- **`g`**/**`home`** — jump to top
- **`G`**/**`end`** — jump to bottom
- **`Esc`** — back to the bot list

The results header shows the model name and elapsed time (e.g. `qwen2.5:0.5b • 4.2s`) so you always know what produced the output and how long it took.

### Loading screen

While a bot is running, the loading view shows:

- The bot name + category pill
- An animated "comet" progress bar beneath the spinner
- Elapsed time in yellow (`Thinking with qwen2.5:0.5b 12s`)
- Connection info (`Connecting to Ollama at http://localhost:11434`)
- Slow-model hints after 15s / 30s
- **`Esc: cancel`** — actually aborts the HTTP request, not just the UI

---

## AI Feature Reference

### Quick Reference Table

| Feature | Access | Provider | Description |
|---------|--------|----------|-------------|
| **Bots overlay** | `Ctrl+R` | All | 19 specialized AI bots (see [Bots section](#bots--the-ai-toolkit)) |
| TL;DR | `Ctrl+R` > TL;DR | All | One-sentence summary |
| Summarizer | `Ctrl+R` > Summarizer | All | 2-4 sentence summary |
| Explain Simply | `Ctrl+R` > Explain Simply | All | Explain like I'm 12 |
| Title Suggester | `Ctrl+R` > Title Suggester | All | Propose better titles |
| Writing Assistant | `Ctrl+R` > Writing Assistant | All | Suggest writing improvements |
| Tone Adjuster | `Ctrl+R` > Tone Adjuster | All | Rewrite formal/casual/concise |
| Expand | `Ctrl+R` > Expand | All | Flesh out a terse note |
| Question Bot | `Ctrl+R` > Question Bot | All | Ask questions about your notes |
| Counter-Argument | `Ctrl+R` > Counter-Argument | All | Generate opposing viewpoints |
| Pros & Cons | `Ctrl+R` > Pros & Cons | All | Decision-analysis list |
| Action Items | `Ctrl+R` > Action Items | All | Extract todos from notes |
| Auto-Tagger | `Ctrl+R` > Auto-Tagger | All | Suggest tags for current note |
| Link Suggester | `Ctrl+R` > Link Suggester | All | Find related notes |
| Auto-Link | `Ctrl+R` > Auto-Link | All | Suggest `[[wikilinks]]` to insert |
| Outline Generator | `Ctrl+R` > Outline Generator | All | Hierarchical outline |
| Flashcard Generator | `Ctrl+R` > Flashcard Generator | All | Q&A flashcards |
| Key Terms | `Ctrl+R` > Key Terms | All | Glossary extraction |
| MOC Generator | `Ctrl+R` > MOC Generator | All | Create a Map of Content |
| Daily Digest Bot | `Ctrl+R` > Daily Digest | Local | Summarize vault activity |
| AI Chat | Command palette | All | Ask questions about your vault |
| Chat with Note | Command palette | All | Ask questions about current note |
| AI Compose | Command palette | All | Generate a note from a topic |
| Ghost Writer | Settings toggle | All | Inline writing suggestions |
| Thread Weaver | Command palette | All | Synthesize multiple notes |
| Semantic Search | Command palette | All | Meaning-based vault search |
| Knowledge Graph AI | Command palette | All | Analyze vault link structure |
| Auto-Link | Command palette | All | Find unlinked mentions |
| Auto-Tag | Settings toggle | All | Auto-suggest tags on save |
| Similar Notes | Command palette | All | TF-IDF note similarity |
| AI Templates | Command palette | All | Generate notes from templates |
| Natural Language Search | Command palette | All | "Find notes about..." |
| AI Writing Coach | Command palette | All | Writing style analysis |
| AI Smart Scheduler | Command palette | All | Optimal schedule generation |
| Vault Refactor | Command palette | All | AI reorganization suggestions |
| Daily Briefing | Command palette | All | Morning summary |
| AI Goal Coach | Goals > `I` key | All | Holistic goal analysis and priority recommendations |
| AI Project Insights | Projects > Dashboard > `I` key | All | Project health, risks, and next actions |
| AI Daily Review | Daily Review (Alt+E) | All | End-of-day summary after reflection |
| AI Weekly Review | Weekly Review | All | Week score, patterns, goal alignment |
| AI Scripture Devotional | Command palette | All | Personal verse reflection tied to goals |
| Quiz Mode | Command palette | All | Auto-generated quizzes |
| Flashcards | Command palette | All | Spaced repetition study |
| Learning Dashboard | Command palette | All | Study progress tracking |
| Deep Dive Research | Command palette | Claude Code | Web research agent |
| Research Follow-Up | Command palette | Claude Code | Deeper research on a topic |
| Vault Analyzer | Command palette | Claude Code | Vault structure analysis |
| Note Enhancer | Command palette | Claude Code | Enhance current note |
| Daily Digest | Command palette | Claude Code | Weekly review generation |

### Status Bar Indicators

When AI features are active, the status bar shows indicators:

| Indicator | Meaning |
|-----------|---------|
| Green bot icon | Ollama is the active provider |
| Blue bot icon | OpenAI is the active provider |
| Animated dots | A background AI operation is running |
| Research status | Shows topic and progress for Deep Dive Research |

---

## Troubleshooting

### Ollama Connection Issues

**Symptom:** "Error connecting to Ollama" or AI features return errors.

**Solutions:**

1. Verify Ollama is running:
   ```bash
   curl http://localhost:11434/api/version
   ```

2. Start the Ollama server:
   ```bash
   ollama serve
   ```

3. Check the configured URL matches:
   ```bash
   # In Granit settings, default is:
   # http://localhost:11434
   ```

4. Verify the model is pulled:
   ```bash
   ollama list
   # Should show your configured model
   ```

5. Pull the model if missing:
   ```bash
   ollama pull qwen2.5:0.5b
   ```

### Ollama Out of Memory

**Symptom:** Ollama crashes or produces garbage output.

**Solutions:**

1. Use a smaller model:
   ```json
   {"ollama_model": "qwen2.5:0.5b"}
   ```

2. Close other memory-intensive applications

3. Check available memory:
   ```bash
   free -h
   ```

### OpenAI API Errors

**Symptom:** "Error: 401 Unauthorized" or "Error: 429 Too Many Requests".

**Solutions:**

1. Verify your API key is correct and has credits:
   ```bash
   curl https://api.openai.com/v1/models \
     -H "Authorization: Bearer sk-your-key"
   ```

2. Check for rate limiting — wait a moment and retry

3. Verify the model name is correct:
   ```json
   {"openai_model": "gpt-4o-mini"}
   ```

### Claude Code Not Found

**Symptom:** "claude: command not found" when using Deep Dive Research.

**Solutions:**

1. Install Claude Code:
   ```bash
   # Follow: https://docs.anthropic.com/en/docs/claude-code
   ```

2. Verify installation:
   ```bash
   which claude
   claude --version
   ```

3. Authenticate:
   ```bash
   claude login
   ```

### AI Features Return Poor Results

**Solutions:**

1. Switch to a better provider:
   - Local < Ollama < OpenAI (in terms of quality)

2. Use a larger Ollama model:
   - `qwen2.5:0.5b` < `qwen2.5:1.5b` < `qwen2.5:3b` < `llama3.2`

3. Ensure note content is substantial — AI features work best with notes that have meaningful content to analyze

### Ghost Writer Not Showing Suggestions

**Solutions:**

1. Verify Ghost Writer is enabled:
   - Settings > "Ghost Writer" should be `true`

2. Verify an AI provider is configured:
   - Settings > "AI Provider" should be `ollama` or `openai` (local works but produces basic suggestions)

3. Type enough text for context — Ghost Writer needs at least a few words/sentences to generate suggestions

4. Wait a moment — suggestions appear after a brief delay to avoid overwhelming the AI with requests
