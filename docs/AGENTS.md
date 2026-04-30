# Agents

Granit's multi-step AI agent runtime — a Deepnote-inspired ReAct
loop where an LLM picks tools from a registered catalog, observes
their output, and iterates until it produces a final answer.

> Open the agent runner with `Alt+A` or via the command palette
> (`Ctrl+X` → "Run Agent").

---

## Table of contents

- [Why agents](#why-agents)
- [How it differs from bots](#how-it-differs-from-bots)
- [Quick start](#quick-start)
- [Built-in agent presets](#built-in-agent-presets)
- [Tool catalog](#tool-catalog)
- [Safety](#safety)
- [Architecture](#architecture)
- [Writing custom agents](#writing-custom-agents)
- [Roadmap](#roadmap)

---

## Why agents

Granit's existing 19 bots (`Ctrl+X` → "AI Bots") are **single-shot**:
one prompt, one completion. They're great for "summarise this note"
or "suggest tags" — anything where the LLM has all the context it
needs in the prompt.

Agents differ along three axes:

1. **Tools** — the LLM can call back into granit (read notes, query
   the typed-objects index, search the vault, create tasks).
2. **Multi-step** — the agent runs a `Thought` → `Action` →
   `Observation` loop until it has enough information to answer.
3. **State** — each loop iteration sees the full transcript of
   prior steps.

The combined effect: questions that bots can't answer ("what's
blocking the Stoicera launch right now?") become tractable, because
the agent can decompose the question, gather evidence, and
synthesise.

---

## How it differs from bots

| | Bots (`Ctrl+X` → AI Bots) | Agents (`Alt+A`) |
|---|---|---|
| Steps | 1 prompt → 1 response | N iterations (default cap: 8) |
| Context | Pre-baked into the prompt | Gathered live via tools |
| Vault access | Read-only via prompt-stuffing | Read + (gated) write via tools |
| Use case | Summarise, suggest tags, expand bullet | Synthesise across notes, propose follow-ups, query typed objects |
| Cost | 1 LLM call | 3-8 LLM calls per run |
| LLM requirement | Any | Any (text-based ReAct works on small Ollama models) |

Both stay in granit. Use bots when the answer fits in the prompt;
use agents when you need to **find** the answer first.

---

## Quick start

1. Press `Alt+A` (or run "Run Agent" from `Ctrl+X`).
2. Pick an agent preset (e.g. **Research Synthesizer**).
3. Type a question — e.g. *"What patterns recur in my Stoicera
   research notes?"*
4. Press Enter. Watch the live transcript stream:

   ```
   Goal: What patterns recur in my Stoicera research notes?
   [1] thought: I'll search for Stoicera notes first.
   [1] → search_vault(query="Stoicera", limit="10")
   [1] ← 1. Research/Stoicera Master Report.md
          2. Research/Stoicera/Boring Businesses.md
          ...
   [2] thought: Let me read the master report.
   [2] → read_note(path="Research/Stoicera Master Report.md")
   [2] ← (rendered note body)
   [3] thought: I have what I need.
   [3] answer
   ```

5. Press `n` to start a new run, `Esc` to close.

The agent runs entirely against your local LLM (Ollama by default).
Nothing leaves your machine unless you've configured an OpenAI key.

---

## Built-in agent presets

Three presets ship in code; vault-local definitions at
`<vault>/.granit/agents/<id>.json` add to or replace them.

### Research Synthesizer

> Given a topic, finds related notes and summarises patterns +
> open questions.

Tools: `search_vault`, `read_note`, `list_notes`, `get_today`. Read-only.

Best for:
- "Compare findings across these notes"
- "What recurring themes appear in my X research?"
- "Summarise everything I know about Y"

### Project Manager

> Reviews a project: status, blockers, related tasks, recent
> activity.

Tools: `query_objects`, `read_note`, `query_tasks`, `search_vault`,
`get_today`. Read-only.

Best for:
- "Where are we on the Stoicera launch?"
- "What's blocking the trippin deployment?"
- "Status of all active projects"

The agent looks up project objects via `query_objects(type=project)`,
reads each, and pulls related tasks via `query_tasks` to find
blockers.

### Inbox Triager

> Reviews recent captures and proposes next-action tasks.

Tools: `list_notes`, `read_note`, `query_tasks`, `create_task`,
`get_today`. **Includes write — uses `create_task`.**

Best for:
- "Triage my inbox"
- "Turn my recent jots into next-actions"
- "Review the captures from this week"

Caps at 5 captures per run by default to prevent runaway
task-creation. Each proposed task is logged in the live transcript
before it's created — press `Esc` if you see something you don't
want.

---

## Tool catalog

Each agent preset gets a curated subset of these tools. The LLM
sees them rendered into the system prompt with arg schemas, so
it picks correctly without us hardcoding tool names in templates.

### Read tools

| Tool | Purpose | Args |
|---|---|---|
| `read_note` | Fetch the body of a markdown note | `path` (req), `max_chars` |
| `list_notes` | Enumerate notes under a folder | `folder`, `limit` |
| `search_vault` | Find notes mentioning a query | `query` (req), `limit` |
| `query_objects` | Filter typed objects by type + exact-match `key=value` | `type`, `where`, `limit` |
| `query_tasks` | Filter tasks by status, due window, priority | `status`, `due`, `min_priority`, `limit` |
| `get_today` | Return today's date in ISO form | — |

> **`where` clause syntax:** comma-separated `key=value` pairs only.
> No comparison operators (`<`, `>`, `!=`) — the agent should fetch
> the full set with `type=` and reason about ranges in its Thought
> step. Example: `where='status=read,rating=5'` works;
> `where='last_contact<2026-04-01'` does NOT and silently drops
> the filter.

### Write tools (Phase 3)

Phase 2 ships these implementations but doesn't expose them to the
default presets. Phase 3 adds presets that opt in.

| Tool | Purpose | Args |
|---|---|---|
| `write_note` | Create or overwrite a markdown note | `path` (req), `content` (req), `overwrite` |
| `create_task` | Append a task to Tasks.md | `text` (req), `due`, `priority`, `tag` |
| `create_object` | Create a typed-object note (frontmatter assembled) | `type` (req), `title` (req), `properties`, `body` |

Every Write tool runs through the runtime's `Approve` callback
before executing. The TUI surfaces a confirmation prompt in
interactive mode; CLI/CI use can pass an auto-approver.

---

## Safety

Three layers protect your vault:

1. **Read/Write split.** Tools declare `KindRead` or `KindWrite`. A
   registry with no Write tools cannot mutate disk regardless of
   what the LLM proposes.

2. **Path containment.** Every tool that takes a path validates it
   stays inside the vault root, refusing `..` escapes and absolute
   paths. Defence in depth — both the `VaultReader`/`VaultWriter`
   implementation and the tool itself check.

3. **Approve callback.** Write tools require an `Options.Approve`
   callback at agent construction. The runtime rejects the session
   at start if Write tools are registered without one — fail-fast
   on the contract, not silent.

### Token / iteration budget

`Options.MaxSteps` (default 8) caps the loop so a hallucinating
model can't run forever. Once hit, the runtime emits a budget event
and returns the partial transcript. The user can inspect what was
gathered and start a new run with a refined goal.

### Cancellation

Press `Esc` during a run to cancel. The agent honours `ctx.Done()`
on the next LLM call boundary; pending tool runs complete (they're
typically sub-millisecond).

### Audit trail

Every Run returns a full `Transcript` with each Step's Thought,
ToolCall, and Observation. The TUI displays it live; programmatic
callers can persist it for compliance.

---

## Architecture

```
internal/agents/
├── tool.go         # Tool interface, Registry, ToolCall, ToolResult
├── tools_read.go   # 6 read tools (read_note, search_vault, etc.)
├── tools_write.go  # 3 write tools (write_note, create_task, create_object)
├── llm.go          # LLM interface, MockLLM for tests, LLMFunc adapter
└── agent.go        # Agent, Options, Run loop, Thought/Action parser
```

The package is **pure** — it has no dependency on the TUI, on
specific LLM providers, or on granit's vault implementation.
Wiring lives in `internal/tui/agentbridge.go` (adapts vault +
typed-objects index) and `internal/tui/agentrunner.go` (the
overlay).

### ReAct loop

Each iteration the runtime:

1. Builds the prompt: persistent system block + goal + transcript so far
2. Calls the LLM
3. Parses the response into Thought + (Action OR Final Answer)
4. If Final Answer → done, return transcript
5. If Action → validate against the registry, run the tool, append the observation
6. If neither → emit a recovery observation telling the LLM to use the right format
7. Loop until step budget or final answer

Parser is line-walking (not regex) because Go's RE2 lacks
lookaheads, and small models produce inconsistent capitalisation
(`Thought:` vs `THOUGHT:` vs `thought:`) that lookahead-free
regexes struggle with.

### Wire format

Tools are described in plain text in the system prompt:

```
## read_note — read
Read the markdown body of a note in the vault.
Args:
  path (required) — Vault-relative path
  max_chars — Truncate threshold; default 6000
```

Calls come back as:

```
Thought: I should read Alice's note.
Action: read_note
Args:
  path: People/Alice.md
  max_chars: 4000
```

This format works on any LLM — including 0.5B parameter local
models that fail at JSON-Schema-style descriptions. A future phase
can layer Anthropic-style structured tool calling on top for
capable models.

---

## Writing custom agents

Drop a JSON file at `<vault>/.granit/agents/<id>.json` and granit
loads it on the next agent-runner open. No recompile required.

```json
{
  "id": "person-reconnect",
  "name": "Person Reconnect",
  "description": "Find people I haven't talked to recently and propose follow-ups.",
  "systemPrompt": "You are a relationship hygienist. Use query_objects with type=person to find every Person object, look at their last_contact property, and produce a list of the 5 most-overdue contacts with a one-line context summary for each. Use get_today to compute 'overdue' relative to today. Do not invent contact dates that aren't on the note.",
  "tools": ["query_objects", "read_note", "get_today"],
  "includeWrite": false,
  "maxSteps": 6
}
```

Schema:

| Field | Required | Purpose |
|---|---|---|
| `id` | yes | Stable handle; must match the filename basename |
| `name` | yes | Human label in the picker |
| `description` | yes | One-line summary under the name |
| `systemPrompt` | no | Persona block prepended to every iteration; empty falls through to a generic helper preamble |
| `tools` | no | Allow-list of tool names; empty means "all read tools" |
| `includeWrite` | no | `true` registers `write_note`, `create_task`, `create_object` (gated by Approve) |
| `maxSteps` | no | Override the default 8-step loop budget; zero falls through |
| `model` | no | Per-preset model override (e.g. `llama3.1:8b`, `gpt-4o-mini`, `claude-haiku-4-5`). Provider is NEVER overridden — preset uses the user's configured provider with this model name. Empty falls through to the global Settings choice. Pattern: fast small models for triage; bigger smarter models for synthesis. |

### Supported providers

Agents inherit whatever provider you configure in Settings (Ctrl+,) → AI:

- **Ollama** (local) — default; the model dropdown live-queries `/api/tags` so you only see what's actually installed
- **OpenAI** — `gpt-4o`, `gpt-4o-mini`, `gpt-4.1` family, `o1-mini`, `o3-mini`
- **Anthropic / Claude** — `claude-haiku-4-5` (default), `claude-sonnet-4-6`, `claude-opus-4-7`; uses the Messages API directly with a separate `AnthropicKey` so it can sit alongside an OpenAI key
- **Nous** and **Nerve** — alternative endpoints for self-hosted/agent-CLI setups

Hit the `>> Test AI Provider` button in Settings to fire a `ping` against
your current configuration; the same error translator the agent runtime
uses surfaces actionable hints ("Ollama isn't running", "API key is
invalid", "model not found — `ollama pull <model>`") instead of raw
network errors.

**Override semantics:** a vault-local preset with the same `id` as
a built-in REPLACES the built-in entirely (same rationale as the
Type registry — full override is the simpler mental model).

**Filename = id:** the file basename must match the `id` field
(case-insensitive). Mismatches are skipped with a warning.

**Loading errors:** per-file errors are surfaced as a status line
in the runner's preset picker — the runner still loads the rest
so a single bad file doesn't lock you out of the feature.

---

## Roadmap

**Phase 3 — Type-aware mentions + agents-over-objects**

- `@person:Sebastian` autocomplete in the editor (typed mentions)
- New presets that use the typed-objects work from Phase 1:
  - **Project Manager** — "what's blocking project X?"
  - **Inbox Triager** — reviews captures, suggests triage state
  - **Person Reconnect** — "who haven't I talked to in 30 days?"
- Vault-local agent definitions (`.granit/agents/*.json`)
- Daily Hub widgets fed by agent output

**Phase 4 — Structured tool calling**

When the configured LLM supports JSON-mode or function-calling
(modern Ollama, OpenAI, Anthropic), bypass the text-based ReAct
parser and use the structured protocol — fewer parsing failures,
tighter loops.

**Phase 5 — Agent composition**

One agent calls another as a sub-tool. "Summarise findings then
draft a follow-up note" routes through Research Synthesizer (read)
followed by Note Writer (write) without the user re-typing.

---

## Tips for writing good goals

- **Be specific about scope.** "Summarise my Stoicera research"
  beats "summarise my notes" — the agent uses the topic in its
  search query.
- **Mention sources.** "Look at the People folder" or "in
  Research/" gives the agent a starting point and avoids it
  searching the whole vault.
- **State the output shape.** "List 3 themes and 2 open questions"
  gets a structured answer; "tell me about my notes" gets a vague
  ramble.
- **Don't require math.** Small models are unreliable at
  arithmetic. If you need counting, the answer will be wrong as
  often as it's right.
- **Verify quotes.** Agents can hallucinate; spot-check any
  claimed quote against the cited note before sharing externally.
