---
title: CLI Tool
date: 2026-02-20
tags: [project, golang, cli, active]
status: in-progress
priority: medium
---

# CLI Tool — `vaultctl`

A command-line utility for managing vault operations outside of Granit. Think of it as the `git` to Granit's GUI — quick operations without leaving your terminal workflow.

## Motivation

Sometimes you want to:
- Quickly create a note from a shell script
- Search vault content from a CI/CD pipeline
- Export notes in bulk for publishing
- Automate tagging and linking as a cron job

The [[Projects/Web App Redesign]] project could also use this for automated documentation generation.

## Feature List

- [x] `vaultctl new <title>` — Create a new note with frontmatter
- [x] `vaultctl search <query>` — Full-text fuzzy search
- [x] `vaultctl list --tag <tag>` — List notes by tag
- [ ] `vaultctl export --format html|pdf` — Bulk export
- [ ] `vaultctl link-check` — Find broken wikilinks
- [ ] `vaultctl graph --output dot` — Export link graph as DOT format
- [ ] `vaultctl stats` — Vault statistics (note count, word count, orphans)
- [ ] `vaultctl daily` — Create/open today's daily note

## Architecture

```
vaultctl/
  cmd/
    root.go        // cobra root command
    new.go         // note creation
    search.go      // full-text search
    list.go        // listing and filtering
    export.go      // export engine
    graph.go       // link graph operations
  internal/
    parser/        // markdown + frontmatter parsing
    index/         // search index (bleve)
    linker/        // wikilink resolution
  go.mod
  go.sum
```

The search index uses [Bleve](https://github.com/blevesearch/bleve) for full-text search with support for fuzzy matching, phrase queries, and faceted search by tag.

## Code Example

```go
package main

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
    Use:   "vaultctl",
    Short: "CLI tool for vault management",
    Long:  "A command-line utility for managing your knowledge vault outside of Granit.",
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

## Integration Points

- Uses the same vault parser as Granit (see `internal/vault/parser.go`)
- Reads `.granit.json` for vault configuration
- Respects `.granit-trash/` for delete operations
- Shares the tag extraction logic described in [[Research/Machine Learning Basics]] for auto-tagging

## Related Notes

- [[Meetings/Architecture Review]] — Discussed the CLI tool's scope
- [[Ideas/Side Projects]] — Originally started as a side project idea
- [[Tasks]] — Current task backlog includes CLI tool milestones
