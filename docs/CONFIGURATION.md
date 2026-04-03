# Granit — Configuration Reference

> Complete reference for all configuration options, file locations, and example setups.

---

## Table of Contents

- [Configuration Files](#configuration-files)
- [Global vs Per-Vault Config](#global-vs-per-vault-config)
- [All Configuration Options](#all-configuration-options)
- [Core Plugins](#core-plugins)
- [Vault List Management](#vault-list-management)
- [Blog Publisher Tokens](#blog-publisher-tokens)
- [AI Provider Configuration](#ai-provider-configuration)
- [Environment Variables](#environment-variables)
- [Example Configurations](#example-configurations)

---

## Configuration Files

| File | Path | Purpose |
|------|------|---------|
| Global config | `~/.config/granit/config.json` | Default settings for all vaults |
| Per-vault config | `<vault>/.granit.json` | Vault-specific overrides |
| Vault list | `~/.config/granit/vaults.json` | Registry of known vaults |
| Global plugins | `~/.config/granit/plugins/` | Plugins available in all vaults |
| Global Lua scripts | `~/.config/granit/lua/` | Lua scripts available in all vaults |
| Custom themes | `~/.config/granit/themes/` | User-created theme JSON files |
| Vault plugins | `<vault>/.granit/plugins/` | Vault-local plugins |
| Vault Lua scripts | `<vault>/.granit/lua/` | Vault-local Lua scripts |

All configuration files are JSON formatted and are created automatically when settings are changed.

---

## Global vs Per-Vault Config

Granit uses a **layered configuration** system:

1. **Defaults** — hardcoded in the application
2. **Global config** (`~/.config/granit/config.json`) — overrides defaults
3. **Per-vault config** (`<vault>/.granit.json`) — overrides global

This means you can set your preferred theme, AI provider, and editor settings globally, then override specific settings for individual vaults. For example, you might use `catppuccin-mocha` globally but `github-light` for a specific work vault.

### How Layering Works

When loading configuration for a vault:

```
DefaultConfig() → Merge global config.json → Merge vault .granit.json
```

Fields not present in a config file retain their previous value. This means a per-vault config only needs to contain the fields you want to override:

```json
// <vault>/.granit.json — only overrides theme and auto-save
{
  "theme": "github-light",
  "auto_save": true
}
```

All other settings (AI provider, vim mode, etc.) come from the global config or defaults.

---

## All Configuration Options

### Editor Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `editor.tab_size` | int | `4` | Number of spaces per tab |
| `editor.insert_tabs` | bool | `false` | Use tab characters instead of spaces |
| `editor.auto_indent` | bool | `true` | Auto-indent new lines |

### Appearance

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `theme` | string | `"catppuccin-mocha"` | Color theme name (38 built-in + custom) |
| `icon_theme` | string | `"unicode"` | Icon set: `"unicode"`, `"nerd"`, `"emoji"`, `"ascii"` |
| `layout` | string | `"default"` | Panel layout (13 options): `"default"`, `"writer"`, `"reading"`, `"dashboard"`, `"zen"`, `"cockpit"`, `"stacked"`, `"cornell"`, `"focus"`, `"preview"`, `"presenter"`, `"kanban"`, `"widescreen"` |
| `sidebar_position` | string | `"left"` | Sidebar placement: `"left"` or `"right"` |
| `show_icons` | bool | `true` | Show file type icons in sidebar |
| `compact_mode` | bool | `false` | Reduce padding and spacing |
| `show_splash` | bool | `true` | Show animated splash screen on startup |
| `show_help` | bool | `true` | Show help bar at the bottom |

### Editor Behavior

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `line_numbers` | bool | `true` | Show line numbers in the editor |
| `word_wrap` | bool | `false` | Wrap long lines |
| `auto_save` | bool | `false` | Automatically save on focus change |
| `auto_close_brackets` | bool | `true` | Auto-insert closing brackets |
| `highlight_current_line` | bool | `true` | Highlight the active cursor line |
| `default_view_mode` | bool | `false` | Open notes in view mode by default |
| `vim_mode` | bool | `false` | Enable Vim-style modal editing |
| `confirm_delete` | bool | `true` | Ask for confirmation before deleting notes |
| `spell_check` | bool | `false` | Enable spell checking (requires aspell/hunspell) |

### Sidebar & Search

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `sort_by` | string | `"name"` | File sorting: `"name"`, `"modified"`, `"created"` |
| `show_hidden_files` | bool | `false` | Show dotfiles in the sidebar |
| `search_content_by_default` | bool | `true` | Search note contents (not just filenames) |
| `max_search_results` | int | `50` | Maximum search results to display |

### Daily Notes

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `daily_notes_folder` | string | `""` | Subfolder for daily notes (empty = vault root) |
| `daily_note_template` | string | `""` | Template to use for new daily notes |

### AI / Bots

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ai_provider` | string | `"local"` | AI backend: `"local"`, `"ollama"`, `"openai"` |
| `ollama_model` | string | `"qwen2.5:0.5b"` | Ollama model name |
| `ollama_url` | string | `"http://localhost:11434"` | Ollama server URL |
| `openai_key` | string | `""` | OpenAI API key |
| `openai_model` | string | `"gpt-4o-mini"` | OpenAI model: `"gpt-4o-mini"`, `"gpt-4o"`, `"gpt-4.1-mini"`, `"gpt-4.1-nano"` |
| `background_bots` | bool | `false` | Auto-analyze notes on save |
| `auto_tag` | bool | `false` | Auto-suggest tags when saving |
| `ghost_writer` | bool | `false` | Enable inline AI writing suggestions |

### Git

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `git_auto_sync` | bool | `false` | Auto commit+push on save, pull on open |
| `auto_refresh` | bool | `true` | Auto-detect external file changes |

### Blog Publisher

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `medium_token` | string | `""` | Medium API integration token |
| `github_token` | string | `""` | GitHub personal access token |
| `github_repo` | string | `""` | GitHub repository (e.g., `"user/blog"`) |
| `github_branch` | string | `""` | GitHub branch to publish to (e.g., `"main"`) |

---

## Core Plugins

Granit includes 16 toggleable core plugins. Each can be enabled or disabled without affecting other features.

The `core_plugins` config field is a map of plugin names to boolean values:

```json
{
  "core_plugins": {
    "task_manager": true,
    "calendar": true,
    "canvas": true,
    "graph_view": true,
    "flashcards": true,
    "quiz_mode": true,
    "pomodoro": true,
    "git_integration": true,
    "blog_publisher": true,
    "ai_templates": true,
    "research_agent": true,
    "language_learning": true,
    "habit_tracker": true,
    "ghost_writer": true,
    "encryption": true,
    "spell_check": true
  }
}
```

| Plugin | Default | Feature Controlled |
|--------|---------|-------------------|
| `task_manager` | `true` | Task Manager, Kanban board |
| `calendar` | `true` | Calendar view (month/week/agenda) |
| `canvas` | `true` | Visual canvas / whiteboard |
| `graph_view` | `true` | Note graph visualization |
| `flashcards` | `true` | Spaced repetition flashcards |
| `quiz_mode` | `true` | Auto-generated quizzes |
| `pomodoro` | `true` | Pomodoro focus timer |
| `git_integration` | `true` | Git overlay (status/log/diff/commit) |
| `blog_publisher` | `true` | Medium and GitHub blog publishing |
| `ai_templates` | `true` | AI template note generator |
| `research_agent` | `true` | Claude Code research agent |
| `language_learning` | `true` | Vocabulary tracker and practice |
| `habit_tracker` | `true` | Habit and goal tracking |
| `ghost_writer` | `true` | Inline AI writing suggestions |
| `encryption` | `true` | AES-256-GCM note encryption |
| `spell_check` | `true` | aspell/hunspell spell checking |

Plugins not present in the map default to enabled.

Toggle core plugins via Settings (`Ctrl+,`) — scroll to the "Core Plugins" section.

---

## Vault List Management

The vault list (`~/.config/granit/vaults.json`) tracks all known vaults:

```json
{
  "vaults": [
    {
      "path": "/home/user/notes",
      "name": "notes",
      "last_open": "2026-03-08"
    },
    {
      "path": "/home/user/work-wiki",
      "name": "work-wiki",
      "last_open": "2026-03-07"
    }
  ],
  "last_used": "/home/user/notes"
}
```

### Automatic Management

- Vaults are **automatically registered** when opened with `granit <path>`
- The `last_open` date and `last_used` path are updated on each open
- The vault name is derived from the directory basename

### Manual Management

- **View vaults:** `granit list`
- **Remove a vault:** Use the vault selector (run `granit` with no args) and delete an entry
- **Edit directly:** Modify `~/.config/granit/vaults.json` in any text editor

---

## Blog Publisher Tokens

Blog publisher tokens are stored in the global config file. They persist across sessions so you only need to enter them once.

### Medium

1. Generate an integration token at [medium.com/me/settings](https://medium.com/me/settings)
2. Set in config or enter when prompted:

```json
{
  "medium_token": "your-medium-integration-token"
}
```

### GitHub

1. Generate a personal access token at [github.com/settings/tokens](https://github.com/settings/tokens) with `repo` scope
2. Set in config:

```json
{
  "github_token": "ghp_your-token",
  "github_repo": "username/blog-repo",
  "github_branch": "main"
}
```

**Security note:** These tokens are stored in plaintext in `~/.config/granit/config.json`. Ensure the file has restrictive permissions:

```bash
chmod 600 ~/.config/granit/config.json
```

---

## AI Provider Configuration

### Local (Default)

No configuration needed. Works offline.

```json
{
  "ai_provider": "local"
}
```

### Ollama

```json
{
  "ai_provider": "ollama",
  "ollama_model": "qwen2.5:0.5b",
  "ollama_url": "http://localhost:11434"
}
```

### OpenAI

```json
{
  "ai_provider": "openai",
  "openai_key": "sk-your-api-key",
  "openai_model": "gpt-4o-mini"
}
```

### Per-Vault AI Override

Use a different AI provider for a specific vault:

```json
// <vault>/.granit.json
{
  "ai_provider": "openai",
  "openai_model": "gpt-4o"
}
```

---

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `GRANIT_VAULT` | Default vault path when no argument given | `/home/user/notes` |
| `EDITOR` | External editor for shell-out operations | `vim` |
| `HOME` | User home directory (config location) | `/home/user` |
| `GOPATH` | Go workspace (for `make install`) | `/home/user/go` |

---

## Example Configurations

### Minimal Writer

Focused writing setup with no distractions:

```json
{
  "theme": "rose-pine",
  "layout": "zen",
  "show_splash": false,
  "show_help": false,
  "line_numbers": false,
  "auto_save": true,
  "ghost_writer": true,
  "ai_provider": "ollama",
  "ollama_model": "qwen2.5:1.5b"
}
```

### Developer / Zettelkasten

Full-featured setup for knowledge management:

```json
{
  "theme": "tokyo-night",
  "layout": "default",
  "vim_mode": true,
  "auto_close_brackets": true,
  "highlight_current_line": true,
  "auto_tag": true,
  "git_auto_sync": true,
  "ai_provider": "ollama",
  "ollama_model": "qwen2.5:3b",
  "icon_theme": "nerd",
  "sort_by": "modified"
}
```

### Team Wiki (OpenAI)

Shared vault with cloud AI and auto-sync:

```json
{
  "theme": "github-dark",
  "layout": "dashboard",
  "auto_save": true,
  "git_auto_sync": true,
  "ai_provider": "openai",
  "openai_key": "sk-...",
  "openai_model": "gpt-4o-mini",
  "background_bots": true,
  "auto_tag": true,
  "search_content_by_default": true
}
```

### Light Theme, Minimal Distractions

```json
{
  "theme": "catppuccin-latte",
  "layout": "writer",
  "icon_theme": "ascii",
  "compact_mode": true,
  "show_splash": false,
  "word_wrap": true,
  "default_view_mode": true
}
```

### Research-Focused

```json
{
  "theme": "kanagawa",
  "layout": "research",
  "ai_provider": "ollama",
  "ollama_model": "llama3.2",
  "auto_tag": true,
  "ghost_writer": true,
  "daily_notes_folder": "Journal",
  "sort_by": "modified"
}
```

### Mobile / Small Terminal

```json
{
  "theme": "min-light",
  "layout": "minimal",
  "icon_theme": "ascii",
  "compact_mode": true,
  "show_splash": false,
  "show_help": false,
  "line_numbers": false
}
```

---

## Viewing Current Configuration

Use the CLI to inspect the active configuration:

```bash
granit config
```

Output:

```
Granit Configuration
──────────────────────────────────────────────
  Global config:   /home/user/.config/granit/config.json
  Config dir:      /home/user/.config/granit
  Vaults file:     /home/user/.config/granit/vaults.json
  Plugins dir:     /home/user/.config/granit/plugins

  Global config:   [exists]
  Vaults file:     [exists]

Current settings:
──────────────────────────────────────────────
  {
    "editor": {
      "tab_size": 4,
      "insert_tabs": false,
      "auto_indent": true
    },
    "theme": "catppuccin-mocha",
    ...
  }
```

---

## Resetting Configuration

To reset to defaults, delete the config file:

```bash
rm ~/.config/granit/config.json
```

Granit will recreate it with default values the next time settings are changed.

To reset a per-vault config:

```bash
rm <vault>/.granit.json
```
