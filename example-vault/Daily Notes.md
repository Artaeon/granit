---
title: Daily Notes
tags: [docs, daily]
created: 2024-01-01
---

# Daily Notes

Granit supports daily notes, just like Obsidian.

## Usage

Run from the command line:
```bash
granit daily ~/my-vault
```

This will:
1. Create a file named `YYYY-MM-DD.md` in your vault
2. Open the vault with the daily note loaded

## Template

Daily notes use a default template with:
- YAML frontmatter with the date
- A heading with today's date
- A tasks section
- A notes section

## Customization

You can customize the daily notes folder and template in your vault configuration.

Related: [[Features]], [[Welcome]]
