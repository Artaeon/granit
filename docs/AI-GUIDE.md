# Granit — AI Setup & Usage Guide

> Complete guide to configuring and using Granit's 25+ AI-powered features.

---

## Table of Contents

- [AI Provider Overview](#ai-provider-overview)
- [Local Provider (Zero Setup)](#local-provider-zero-setup)
- [Ollama Setup (Recommended)](#ollama-setup-recommended)
- [OpenAI Setup](#openai-setup)
- [Claude Code Setup (Deep Dive Research)](#claude-code-setup-deep-dive-research)
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

## AI Feature Reference

### Quick Reference Table

| Feature | Access | Provider | Description |
|---------|--------|----------|-------------|
| Auto-Tagger | `Ctrl+R` > Auto-Tagger | All | Suggest tags for the current note |
| Link Suggester | `Ctrl+R` > Link Suggester | All | Suggest notes to link to |
| Summarizer | `Ctrl+R` > Summarizer | All | Generate a summary |
| Question Bot | `Ctrl+R` > Question Bot | All | Generate study questions |
| Writing Assistant | `Ctrl+R` > Writing Assistant | All | Suggest writing improvements |
| Title Suggester | `Ctrl+R` > Title Suggester | All | Propose better titles |
| Action Items | `Ctrl+R` > Action Items | All | Extract action items |
| MOC Generator | `Ctrl+R` > MOC Generator | All | Create a Map of Content |
| Daily Digest Bot | `Ctrl+R` > Daily Digest | All | Summarize recent activity |
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
