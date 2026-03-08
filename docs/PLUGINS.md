# Granit — Plugin Development Guide

> How to create, install, and manage plugins for Granit.

---

## Table of Contents

- [Overview](#overview)
- [Plugin Directory Structure](#plugin-directory-structure)
- [plugin.json Manifest](#pluginjson-manifest)
- [Commands](#commands)
- [Hooks](#hooks)
- [Environment Variables](#environment-variables)
- [stdin/stdout Protocol](#stdinstdout-protocol)
- [Lua Scripting API](#lua-scripting-api)
- [Example Plugins](#example-plugins)
- [Plugin Manager](#plugin-manager)
- [Best Practices](#best-practices)

---

## Overview

Granit's plugin system is **language-agnostic** — plugins are executable scripts or programs with a JSON manifest. They can:

- Add commands to the command palette
- React to lifecycle hooks (save, open, create, delete)
- Read and modify note content
- Display status messages
- Insert text at the cursor

Plugins run as child processes with a 10-second timeout, ensuring they cannot hang or crash Granit.

---

## Plugin Directory Structure

Plugins are discovered from two locations:

| Location | Scope | Path |
|----------|-------|------|
| **Global** | Available in all vaults | `~/.config/granit/plugins/<plugin-name>/` |
| **Vault-local** | Available only in that vault | `<vault>/.granit/plugins/<plugin-name>/` |

Each plugin lives in its own directory and must contain a `plugin.json` manifest:

```
~/.config/granit/plugins/
└── word-count/
    ├── plugin.json          # Required: manifest
    ├── count.sh             # Script(s) referenced by manifest
    └── README.md            # Optional: documentation

<vault>/.granit/plugins/
└── custom-formatter/
    ├── plugin.json
    └── format.py
```

---

## plugin.json Manifest

The manifest describes the plugin and declares its commands and hooks.

### Full Schema

```json
{
  "name": "Plugin Name",
  "description": "What this plugin does",
  "version": "1.0.0",
  "author": "Author Name",
  "enabled": true,
  "commands": [
    {
      "label": "Command Label",
      "description": "What this command does",
      "run": "python3 script.py"
    }
  ],
  "hooks": {
    "on_save": "bash on_save.sh",
    "on_open": "",
    "on_create": "",
    "on_delete": ""
  }
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name shown in Plugin Manager |
| `description` | string | Yes | Brief description |
| `version` | string | Yes | Semantic version (e.g., `"1.0.0"`) |
| `author` | string | No | Author name |
| `enabled` | bool | Yes | Whether the plugin is active |
| `commands` | array | No | List of commands (see below) |
| `hooks` | object | No | Lifecycle hooks (see below) |

---

## Commands

Commands appear in the command palette and can be run from the Plugin Manager.

### Command Definition

```json
{
  "label": "Format Note",
  "description": "Reformat the current note with prettier",
  "run": "node format.js"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `label` | string | Command name shown in the palette |
| `description` | string | Brief description |
| `run` | string | Shell command to execute |

### Execution

When a command is run:

1. The `run` command is executed as a shell command
2. The current note's content is passed via **stdin**
3. Environment variables provide context (see below)
4. The script's **stdout** is parsed for output directives
5. The script has a **10-second timeout**

### Multiple Commands

A plugin can define multiple commands:

```json
{
  "commands": [
    {
      "label": "Count Words",
      "description": "Show word count",
      "run": "bash count.sh words"
    },
    {
      "label": "Count Characters",
      "description": "Show character count",
      "run": "bash count.sh chars"
    }
  ]
}
```

---

## Hooks

Hooks trigger automatically on note lifecycle events.

### Available Hooks

| Hook | Event | When |
|------|-------|------|
| `on_save` | Note saved | After `Ctrl+S` or auto-save |
| `on_open` | Note opened | When a note is selected in the sidebar |
| `on_create` | Note created | When `Ctrl+N` creates a new note |
| `on_delete` | Note deleted | When a note is sent to trash |

### Hook Definition

Set each hook to a shell command string, or `""` to disable:

```json
{
  "hooks": {
    "on_save": "python3 on_save.py",
    "on_open": "bash log_open.sh",
    "on_create": "",
    "on_delete": "bash archive.sh"
  }
}
```

### Hook Execution

Hooks follow the same execution model as commands:

- Current note content via **stdin**
- Environment variables for context
- stdout parsed for output directives
- 10-second timeout

---

## Environment Variables

When a plugin command or hook runs, these environment variables are set:

| Variable | Description | Example |
|----------|-------------|---------|
| `GRANIT_NOTE_PATH` | Absolute path to the current note | `/home/user/notes/Project.md` |
| `GRANIT_NOTE_NAME` | Note filename without `.md` extension | `Project` |
| `GRANIT_VAULT_PATH` | Absolute path to the vault root | `/home/user/notes` |

### Using Environment Variables

**Bash:**
```bash
#!/bin/bash
echo "MSG:Processing $GRANIT_NOTE_NAME in $GRANIT_VAULT_PATH"
```

**Python:**
```python
import os
note_path = os.environ['GRANIT_NOTE_PATH']
vault_path = os.environ['GRANIT_VAULT_PATH']
note_name = os.environ['GRANIT_NOTE_NAME']
```

**Node.js:**
```javascript
const notePath = process.env.GRANIT_NOTE_PATH;
const vaultPath = process.env.GRANIT_VAULT_PATH;
```

---

## stdin/stdout Protocol

### Input (stdin)

The current note's full content is passed to the plugin via stdin. Read it in your preferred language:

**Bash:**
```bash
content=$(cat)
```

**Python:**
```python
import sys
content = sys.stdin.read()
```

**Node.js:**
```javascript
let content = '';
process.stdin.on('data', chunk => content += chunk);
process.stdin.on('end', () => { /* process content */ });
```

### Output (stdout)

Plugin output is parsed line by line. Three directive prefixes are recognized:

| Prefix | Action | Format |
|--------|--------|--------|
| `MSG:` | Display a status message | `MSG:Your message text` |
| `CONTENT:` | Replace the editor content | `CONTENT:<base64-encoded-text>` |
| `INSERT:` | Insert text at the cursor | `INSERT:<base64-encoded-text>` |

**Important:** `CONTENT:` and `INSERT:` values must be **base64-encoded** to safely handle newlines and special characters.

### Output Examples

**Display a message:**
```bash
echo "MSG:Word count: $(wc -w <<< "$content")"
```

**Replace editor content:**
```bash
formatted=$(echo "$content" | some_formatter)
encoded=$(echo -n "$formatted" | base64 -w 0)
echo "CONTENT:$encoded"
```

**Insert text at cursor:**
```python
import base64
import sys

timestamp = "Last updated: 2026-03-08"
encoded = base64.b64encode(timestamp.encode()).decode()
print(f"INSERT:{encoded}")
```

### No Output

If a plugin produces no recognized output directives, Granit silently ignores the output. This is fine for hooks that only perform side effects (logging, file operations, etc.).

---

## Lua Scripting API

In addition to the plugin system, Granit includes a built-in Lua scripting engine powered by [GopherLua](https://github.com/yuin/gopher-lua).

### Script Locations

| Location | Scope |
|----------|-------|
| `<vault>/.granit/lua/` | Vault-local scripts |
| `~/.config/granit/lua/` | Global scripts |

Scripts are `.lua` files discovered automatically from these directories.

### Running Lua Scripts

- **Access:** Command palette > "Lua Scripts"
- Select a script from the list and press Enter

### API Reference

All API functions are available through the `granit` global table:

#### Context (Read-Only Properties)

| Property | Type | Description |
|----------|------|-------------|
| `granit.note_path` | string | Absolute path to the current note |
| `granit.note_content` | string | Full content of the current note |
| `granit.vault_path` | string | Absolute path to the vault root |
| `granit.note_name` | string | Note filename without `.md` extension |
| `granit.frontmatter` | table | Key-value table of frontmatter properties |

#### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `granit.read_note(name)` | `(string) → string, string?` | Read another note's content. Returns content or nil + error |
| `granit.write_note(name, content)` | `(string, string) → bool, string?` | Write/overwrite a note. Returns true or false + error |
| `granit.list_notes()` | `() → table` | List all `.md` files in the vault (relative paths) |
| `granit.date(format?)` | `(string?) → string` | Current date. Default format: `"2006-01-02"` (Go time format) |
| `granit.time()` | `() → string` | Current time as `"HH:MM:SS"` |
| `granit.msg(text)` | `(string) → nil` | Display a status message |
| `granit.set_content(text)` | `(string) → nil` | Replace the entire editor content |
| `granit.insert(text)` | `(string) → nil` | Insert text at the cursor position |

### Lua Script Limits

- **Timeout:** 5 seconds
- **Libraries:** All standard Lua libraries are available (string, table, math, io, os)
- **Sandboxing:** Scripts run in a fresh Lua VM for each execution

### Example Lua Script

```lua
-- word-count.lua — Display word count for the current note

local content = granit.note_content
local words = 0

for _ in content:gmatch("%S+") do
    words = words + 1
end

granit.msg("Word count: " .. words .. " words in " .. granit.note_name)
```

```lua
-- add-timestamp.lua — Insert a timestamp at the cursor

local timestamp = "Updated: " .. granit.date() .. " " .. granit.time()
granit.insert(timestamp)
```

```lua
-- list-orphans.lua — Find notes with no incoming links

local notes = granit.list_notes()
local linked = {}

-- Scan all notes for wikilinks
for _, path in ipairs(notes) do
    local content = granit.read_note(path)
    if content then
        for link in content:gmatch("%[%[(.-)%]%]") do
            linked[link .. ".md"] = true
            linked[link] = true
        end
    end
end

-- Find orphans
local orphans = {}
for _, path in ipairs(notes) do
    local name = path:match("([^/]+)$"):gsub("%.md$", "")
    if not linked[name] and not linked[path] then
        table.insert(orphans, path)
    end
end

granit.msg("Orphan notes: " .. #orphans .. " (of " .. #notes .. " total)")
```

---

## Example Plugins

### Word Counter (Bash)

```
~/.config/granit/plugins/word-count/
├── plugin.json
└── count.sh
```

**plugin.json:**
```json
{
  "name": "Word Counter",
  "description": "Count words, lines, and characters in the current note",
  "version": "1.0.0",
  "author": "Example",
  "enabled": true,
  "commands": [
    {
      "label": "Word Count",
      "description": "Show word/line/char counts",
      "run": "bash count.sh"
    }
  ],
  "hooks": {
    "on_save": "",
    "on_open": "",
    "on_create": "",
    "on_delete": ""
  }
}
```

**count.sh:**
```bash
#!/bin/bash
content=$(cat)
words=$(echo "$content" | wc -w | tr -d ' ')
lines=$(echo "$content" | wc -l | tr -d ' ')
chars=$(echo "$content" | wc -c | tr -d ' ')
echo "MSG:$GRANIT_NOTE_NAME: $words words, $lines lines, $chars chars"
```

### Auto-Frontmatter (Python)

Automatically adds missing frontmatter on save.

```
~/.config/granit/plugins/auto-frontmatter/
├── plugin.json
└── add_frontmatter.py
```

**plugin.json:**
```json
{
  "name": "Auto Frontmatter",
  "description": "Add frontmatter to notes that don't have it",
  "version": "1.0.0",
  "author": "Example",
  "enabled": true,
  "commands": [],
  "hooks": {
    "on_save": "python3 add_frontmatter.py",
    "on_open": "",
    "on_create": "",
    "on_delete": ""
  }
}
```

**add_frontmatter.py:**
```python
import sys
import os
import base64
from datetime import date

content = sys.stdin.read()
note_name = os.environ.get('GRANIT_NOTE_NAME', 'Untitled')

# Check if frontmatter already exists
if content.startswith('---'):
    # Already has frontmatter, skip
    sys.exit(0)

# Add frontmatter
frontmatter = f"""---
title: {note_name}
date: {date.today().isoformat()}
tags: []
---

"""

new_content = frontmatter + content
encoded = base64.b64encode(new_content.encode()).decode()
print(f"CONTENT:{encoded}")
print(f"MSG:Added frontmatter to {note_name}")
```

### Link Checker (Node.js)

Validates that all wikilinks point to existing notes.

**plugin.json:**
```json
{
  "name": "Link Checker",
  "description": "Find broken wikilinks in the current note",
  "version": "1.0.0",
  "author": "Example",
  "enabled": true,
  "commands": [
    {
      "label": "Check Links",
      "description": "Find broken wikilinks",
      "run": "node check_links.js"
    }
  ],
  "hooks": {
    "on_save": "",
    "on_open": "",
    "on_create": "",
    "on_delete": ""
  }
}
```

**check_links.js:**
```javascript
const fs = require('fs');
const path = require('path');

let content = '';
process.stdin.on('data', chunk => content += chunk);
process.stdin.on('end', () => {
    const vaultPath = process.env.GRANIT_VAULT_PATH;
    const linkRegex = /\[\[([^\]]+)\]\]/g;
    const broken = [];
    let match;

    while ((match = linkRegex.exec(content)) !== null) {
        const linkName = match[1] + '.md';
        const linkPath = path.join(vaultPath, linkName);
        if (!fs.existsSync(linkPath)) {
            broken.push(match[1]);
        }
    }

    if (broken.length === 0) {
        console.log('MSG:All links are valid!');
    } else {
        console.log(`MSG:Broken links: ${broken.join(', ')}`);
    }
});
```

### Save Logger (Bash Hook)

Logs every save to a file.

**plugin.json:**
```json
{
  "name": "Save Logger",
  "description": "Log every save with timestamp",
  "version": "1.0.0",
  "author": "Example",
  "enabled": true,
  "commands": [],
  "hooks": {
    "on_save": "bash log.sh",
    "on_open": "",
    "on_create": "",
    "on_delete": ""
  }
}
```

**log.sh:**
```bash
#!/bin/bash
echo "$(date -Iseconds) SAVED $GRANIT_NOTE_PATH" >> "$GRANIT_VAULT_PATH/.granit/save.log"
```

---

## Plugin Manager

The Plugin Manager overlay lets you view, enable/disable, and run plugins.

### Opening

- Command palette (`Ctrl+X`) > "Plugins"

### Controls

| Key | Action |
|-----|--------|
| `Up` / `k` | Move cursor up |
| `Down` / `j` | Move cursor down |
| `Enter` / `Space` | Toggle enabled/disabled |
| `d` | Show detail view (commands, hooks) |
| `r` | Run the first command |
| `i` | Show installable plugins |
| `Esc` / `q` | Close |

### Detail View

Pressing `d` on a plugin shows its commands, hooks, version, and author. From here you can select and run individual commands with `Enter`.

---

## Best Practices

1. **Keep scripts fast.** Plugins have a 10-second timeout. For long operations, consider writing output incrementally or running async.

2. **Use MSG: for feedback.** Always output at least a `MSG:` line so the user knows the plugin ran.

3. **Handle missing input gracefully.** stdin may be empty for new notes or when run on a note without content.

4. **Use base64 for content changes.** Always base64-encode `CONTENT:` and `INSERT:` values to avoid issues with newlines and special characters.

5. **Respect the vault structure.** If your plugin creates files, put them in sensible locations. Avoid modifying files outside the vault.

6. **Document your plugin.** Include a README.md explaining what it does, any requirements, and usage examples.

7. **Use version numbers.** Follow semantic versioning so users can track updates.

8. **Test with both empty and large notes.** Edge cases often appear with very short or very long content.
