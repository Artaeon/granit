# Typed Objects

Granit's typed-object system is a thin schema layer over markdown
notes that lets you treat certain notes as instances of a *Type*
(Person, Book, Project, Meeting, Idea — or anything you define).
Inspired by Capacities, but file-first: every object is just a
markdown note with a `type:` line in its YAML frontmatter.

> Open the **Object Browser** with `Alt+O` or via the command palette
> (`Ctrl+X` → "Object Browser").

---

## Table of contents

- [Why typed objects](#why-typed-objects)
- [Quick start](#quick-start)
- [Built-in types](#built-in-types)
- [Frontmatter conventions](#frontmatter-conventions)
- [Object Browser](#object-browser)
- [Custom types](#custom-types)
- [Property kinds reference](#property-kinds-reference)
- [Roadmap](#roadmap)

---

## Why typed objects

Most PKM apps make you choose between **structure** (Notion, Capacities,
Tana — every entry has a typed schema) and **portability** (Obsidian,
Logseq — plain markdown, git-friendly, no lock-in). Granit gives you
both:

- Storage stays plain markdown. Every typed note is still just a `.md`
  file you can open in any editor.
- Frontmatter `type: person` carries the type association — visible,
  editable by hand, no proprietary database.
- Schemas live in `.granit/types/<id>.json` per vault, with built-in
  defaults shipped in granit itself.
- Untyped notes still work normally. The type system is opt-in,
  per-note.

The payoff: you get **galleries** (all your books as a sortable grid),
**type-aware mentions** (eventually `@person:Sebastian` autocomplete),
and **typed queries** (`type=person where last_contact > 30d`) — without
giving up the "notes are files in a folder" promise.

---

## Quick start

1. Open any note (or create one) and add a frontmatter `type:`:

   ```markdown
   ---
   type: person
   name: Sebastian Becker
   email: s@example.com
   role: Co-founder
   last_contact: 2026-04-22
   ---

   # Sebastian Becker

   Notes from our last call: …
   ```

2. Press `Alt+O` (or run "Object Browser" from `Ctrl+X`).
3. The **People** type appears in the left pane with a count badge;
   the right pane shows Sebastian as a row in a sortable grid.
4. Press `j`/`k` to navigate, `Enter` to open the note in the editor,
   `/` to filter within the type, `Tab` to swap pane focus.

That's it. No database, no plugin install, no config — granit ships
with five starter types and the registry merges any vault-local
overrides on startup.

---

## Built-in types

Five types ship with granit out of the box. They're intentionally
short on properties — Capacities and Notion both ship with
property-laden defaults that overwhelm new users; granit picks 4-6
fields that cover ~80% of use cases and lets you add more.

| ID | Icon | What it represents | Required fields |
|---|---|---|---|
| `person` | 👤 | Someone you know — friend, colleague, contact | `name` |
| `book` | 📚 | A book on your reading list (active or done) | `title`, `author` |
| `project` | 🎯 | A multi-task initiative with a goal and deadline | `name` |
| `meeting` | 🗣️ | Notes from a meeting, call, or 1:1 | `title`, `date` |
| `idea` | 💡 | A nascent concept — pre-project, pre-decision | `title` |
| `article` | 📰 | Saved web article — read-later, highlighted, summarised | `title`, `url` |
| `podcast` | 🎙️ | Podcast episode — show, host, key takeaways | `title`, `show` |
| `video` | 📺 | YouTube / Vimeo / lecture video with timestamped notes | `title`, `url` |
| `quote` | 💬 | A pithy quote with attribution and context | `title`, `author` |
| `place` | 📍 | A location — venue, city, restaurant, landmark | `name` |
| `recipe` | 🍳 | A cooking recipe with ingredients and method | `title` |
| `highlight` | 🔖 | A passage from a book, article, or conversation worth remembering | `title` |

Each type also declares a default **folder** and **filename pattern**
(e.g. people land in `People/{title}.md`, meetings in
`Meetings/{date} - {title}.md`). The Object Browser's "create new"
flow (Phase 2) will use these to scaffold new instances.

---

## Frontmatter conventions

A typed note's frontmatter is just YAML:

```yaml
---
type: book              # required — the type ID
title: Atomic Habits    # promoted to the gallery's Title column
author: James Clear     # arbitrary property (key matches type schema)
status: read
rating: 5
started: 2026-01-15
finished: 2026-02-04
---
```

Rules:

- The `type:` field is **case-sensitive** and must match a registered
  type ID exactly. Unknown types still load (the note shows up under
  its declared type ID) but appear in a "⚠ N notes reference unknown
  types" warning at the top of the Object Browser.
- The `title:` field is promoted to the gallery's first column. When
  absent, granit falls back to the first H1 in the body, then to the
  filename without extension.
- Other keys are matched against the type's declared properties for
  formatting (a `KindCheckbox` property renders `true` as `✓`, etc.).
  Unknown keys are ignored by the gallery but kept on disk — granit
  never strips frontmatter.
- The schema is a **hint**, not a wall. Required fields show a visual
  warning when empty but the note still loads. Type errors don't
  block builds.

---

## Sidebar Types view

The file explorer (left sidebar) cycles between two modes with the
**`m`** key when focused:

```
EXPLORER  files  127 files                EXPLORER  types  64 objects
┌─────────────                            ┌──────────────────
│ > Archive/                              │ ★ Pinned          1
│ > Books/                                │   ● Bob Jones
│ > Inbox/                                │ 📚  Book           3
│ > Jots/                                 │     Atomic Habits
│ > Notes/                                │     ...
                                          │ 👤  Person        12
                                          │     Alice Smith
                                          │   ● Bob Jones
```

- **Mode chip** in the header tells you which view you're in
- **`j` / `k`** auto-skip type headers so navigation feels continuous
- **`Enter`** on an object loads the note in the editor (same as Files mode)
- **`/`** filters within types — matches object titles; type headers
  with no surviving objects are hidden
- **`b`** pins / unpins the typed object under the cursor — pinned
  objects appear in the dedicated `★ Pinned` section at the top of
  the Types view AND get a yellow ★ next to their row
- **`●`** marks the currently-open note in the editor
- **Empty types are hidden** — `Meeting (0)` would be visual dead
  weight, so types with no instances stay out of the list

The Sidebar Types view shares the same registry/index as the
Object Browser (`Alt+O`); a typed note pinned via `b` in either
surface appears in both. Pin state persists in
`.granit/sidebar-pinned.json`.

---

## Object Browser

```
┌─ 📦 Objects ──────  17 typed objects across 3 types ─────────┐
│                                                              │
│  TYPES                  │  📚  Book                          │
│                         │  Filter: (press / to search)       │
│  📚  Book           7   │                                    │
│ ▶👤  Person        12   │  Title          Author       Stat… │
│  🗣️  Meeting        4   │  Atomic Habits  James Clear  read  │
│                         │  Designing Da…  Kleppmann    read… │
│                         │  ...                               │
│                                                              │
│ ──────────────────────────────────────────────────────────── │
│  j/k:nav  Tab:swap pane  Enter:open  /:filter  Esc:close     │
└──────────────────────────────────────────────────────────────┘
```

**Keyboard:**

| Key | Action |
|---|---|
| `j` / `k` (or arrows) | Move cursor in the focused pane |
| `Tab` / `Shift+Tab` | Swap focus between type list and grid |
| `Enter` (on type) | Focus the grid for that type |
| `Enter` (on object) | Open the note in the editor pane (closes the browser tab) |
| `/` | Focus the filter input — typing narrows the grid by title or any property value |
| `Esc` (first press) | Clear the active filter |
| `Esc` (second press) | Close the Object Browser tab |

**Search:** the filter is case-insensitive and matches against the
note's title AND any property value, so typing "vienna" finds
people whose `city: Vienna` matches even when "Vienna" never appears
in the title.

**Empty types are hidden** from the type list to keep it dense — a
registered-but-empty type doesn't take a row.

---

## Custom types

Vault-local type definitions live in `<vault>/.granit/types/<id>.json`
and **replace** any built-in with the same ID (full override, not deep
merge — see `internal/objects/builtin.go` for the rationale).

Minimal custom type:

```json
{
  "id": "snippet",
  "name": "Code Snippet",
  "description": "A reusable code fragment",
  "icon": "✂️",
  "folder": "Snippets",
  "filenamePattern": "{title}",
  "properties": [
    { "name": "title", "kind": "text", "required": true },
    { "name": "language", "kind": "select",
      "options": ["go", "python", "rust", "ts", "shell"] },
    { "name": "tags", "kind": "tag" }
  ]
}
```

Save as `.granit/types/snippet.json`, restart granit, and the new
type appears in the Object Browser as soon as you tag a note with
`type: snippet`.

**Note:** the filename basename must match the embedded `id` field
(case-insensitive). Mismatches are skipped with a warning so a typo'd
filename doesn't silently shadow a different type by coincidence.

### Overriding a built-in

Save a `.granit/types/person.json` to fully replace the built-in
Person schema. Common reasons: adding fields specific to your
workflow (Slack handle, GitHub username, last-1:1-date), changing
the default folder, or shrinking the property list.

---

## Property kinds reference

| Kind | Stored as | Render |
|---|---|---|
| `text` | string (default) | passthrough |
| `number` | string parsed as float | passthrough |
| `date` | `YYYY-MM-DD` string | passthrough (no timezone games) |
| `url` | string | passthrough; future: clickable in TUI |
| `tag` | string (single tag, no `#`) | passthrough |
| `checkbox` | `true`/`false`/`yes`/`no`/`1`/`0` | `✓` / `✗` |
| `link` | wikilink target or relative path | passthrough; future: clickable |
| `select` | one of declared `options` | passthrough |

A property declared with no `kind` defaults to `text`. The `Required`
flag affects rendering only — a missing required field shows a warning
but doesn't fail validation.

---

## Roadmap

Phase 1 (this doc) ships the foundation: type registry, vault index,
Object Browser, Capacities-style galleries.

**Phase 2** adds the agent runtime — Deepnote-style multi-step AI
agents that can query the typed-object index. "Find every person I
haven't talked to in 30 days" becomes a real operation, not a manual
filter.

**Phase 3** wires types into mentions and queries:

- `@person:Sebastian` autocomplete in the editor
- `@book:Atomic Habits` for typed cross-references
- Daily Hub widget: "people you haven't talked to in 30 days"
- Daily Hub widget: "books you started but didn't finish"
- Inline queries: ` ```query type=book where rating >= 4 ` rendered as
  a live table inside any note

The architecture is structured so Phase 2 doesn't require Phase 1
changes — agents reuse the registry/index this phase shipped.
