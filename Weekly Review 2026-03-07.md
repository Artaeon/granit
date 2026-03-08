---
date: 2026-03-07
type: weekly-review
tags: [weekly-review, meta, digest]
---

# Weekly Review — March 7, 2026

## Activity Summary

**~25 notes modified or created** this week across the vault. The breakdown:

- **8 new research notes** — A comprehensive Bitcoin deep-dive created on March 7, organized under `Research/Bitcoin 2026-03-07/` with a dedicated [[Research/Bitcoin 2026-03-07/_Index|Map of Content]]
- **3 diagram showcases** — Technique breakdowns for [[Diagram Showcase - Boxing Combos|boxing]], [[Diagram Showcase - Muay Thai|Muay Thai]], and [[Diagram Showcase - BJJ|BJJ]] using Granit's custom diagram engine
- **5 test/scratch notes** — Quick test files ([[testtest]], [[Thisisatest]], [[testtetbdgfb]], [[sebastiantest]], [[testetstdhdf]]) likely created while developing or demoing features
- **Project documentation updates** — [[CHANGELOG]], [[CONTRIBUTING]], and [[README]] all saw revisions reflecting a wave of new Granit features
- **Example vault maintenance** — Several example-vault notes touched, plus a daily note for [[example-vault/2026-03-06|2026-03-06]]
- **1 miscellaneous note** — [[Machine Learning basics]]

**Most active areas:** Bitcoin research (by far), Granit project development, combat sports diagrams.

## Key Themes

### 1. Bitcoin Research Sprint
The dominant activity this week was a thorough Bitcoin research project spanning 7 interconnected notes plus an index. Topics covered:
- Fundamentals: [[Research/Bitcoin 2026-03-07/Bitcoin - Overview and History|origins and history]], [[Research/Bitcoin 2026-03-07/Bitcoin - How It Works|technical architecture (UTXO, blockchain)]]
- Infrastructure: [[Research/Bitcoin 2026-03-07/Bitcoin - Mining and Consensus|mining and proof of work]], [[Research/Bitcoin 2026-03-07/Bitcoin - Lightning Network and Scaling|Lightning Network scaling]]
- Economics: [[Research/Bitcoin 2026-03-07/Bitcoin - Halving Cycles and Supply|halving cycles and fixed supply]], [[Research/Bitcoin 2026-03-07/Bitcoin - Price History and Market|price history and ETF market]]
- Policy: [[Research/Bitcoin 2026-03-07/Bitcoin - Regulation and Legal Status|U.S. Strategic Bitcoin Reserve, spot ETFs, global regulation]]

Key data points captured: BTC at ~$67,550 (down 46.7% from Oct 2025 ATH of $126,296), $147B in ETF AUM, 23 nation-states holding BTC, and the debate over whether the four-year halving cycle is dead.

### 2. Granit Feature Explosion
The [[CHANGELOG]] reveals a massive batch of unreleased features:
- **Task manager rewrite** with 6 views, priority levels, and cross-vault scanning
- **Blog publisher** to Medium and GitHub
- **Custom diagram engine** with 6 diagram types (sequence, tree, movement, timeline, comparison, figure)
- **AI template generator** with 9 template types
- **Global search & replace** across all vault files
- **Enhanced research agent** with Vault Analyzer, Note Enhancer, and Daily Digest modes

### 3. Combat Sports Diagrams as Feature Showcase
The three diagram showcase notes ([[Diagram Showcase - Boxing Combos|Boxing]], [[Diagram Showcase - Muay Thai|Muay Thai]], [[Diagram Showcase - BJJ|BJJ]]) serve double duty — real training notes and demonstrations of the new diagram engine. They cover figure poses, sequence combos, decision trees, movement grids, timelines, and comparison tables.

### 4. Testing and Iteration
Multiple scratch notes suggest active development and testing of Granit features (template creation, blog publishing, note creation flows).

## Connections Discovered

### Cross-topic links worth making
- [[Machine Learning basics]] contains a task due today (`testtask 🔺 📅 2026-03-07`) — this note likely needs attention or was used for task manager testing
- [[testtetbdgfb]] is tagged `[golang, programming]` and mentions "Large language models" — could connect to [[example-vault/Go Programming|Go Programming]] or evolve into a note about LLM implementation in Go
- [[Thisisatest]] references publishing on Medium — this was likely a test of the new **blog publisher** feature documented in [[CHANGELOG]]
- The Bitcoin research notes are well-interlinked internally but have no connections to the rest of the vault. Consider linking [[Research/Bitcoin 2026-03-07/_Index|Bitcoin MOC]] from a broader "Research" index or from notes on economics/technology topics
- [[example-vault/Writing Workflow|Writing Workflow]] mentions a "static site publisher" — the [[CHANGELOG]] now lists a **blog publisher** to Medium/GitHub, which could be referenced there as the realized version of that workflow step

### Structural patterns
- The Bitcoin research follows a clean MOC (Map of Content) pattern with `_Index.md` — this same structure could be applied to future research topics
- The diagram showcases demonstrate a content pattern (sport/discipline breakdown) that could extend to other domains (e.g., music theory, programming patterns, cooking techniques)

## Follow-Up Tasks

### Action items found in notes
- [ ] **Task due today** — `testtask 🔺` in [[Machine Learning basics]] is due 2026-03-07 with highest priority — resolve or reschedule
- [ ] **Word count goals** — marked as `(planned)` in [[example-vault/Writing Workflow|Writing Workflow]], still unchecked

### Cleanup needed
- [ ] **Purge test notes** — 5 scratch files with little to no content should be reviewed and deleted if no longer needed: `testtest.md`, `Thisisatest.md`, `testtetbdgfb.md`, `sebastiantest.md`, `testetstdhdf.md`
- [ ] **Fix "Machine Learning basics" note** — The title says "Machine Learning" but the content is a basic computer skills course. Either rename it to match the content or replace the content with actual ML material. Also lacks proper YAML frontmatter (uses plain text headers instead)
- [ ] **Fix tag formatting** — [[testtetbdgfb]] has a malformed tag array with a newline inside `[golang\n, programming]`

### Research to expand
- [ ] The Bitcoin halving cycle analysis raises the question: "Is the four-year cycle dead?" — Wall Street targets ($143K-$170K for 2026) could be tracked in a follow-up note as the year progresses
- [ ] Lightning Network adoption data (300% YoY growth, $1B monthly volume) deserves periodic updates
- [ ] The BITCOIN Act (S.954) is still in committee as of March 2026 — worth tracking for legislative progress

### Notes to develop further
- [ ] [[example-vault/2026-03-06]] — Daily note created but left completely empty
- [ ] The diagram showcases could benefit from a unified index note linking all three combat sports
- [ ] Consider creating a "Research" folder-level index linking to the Bitcoin MOC and future research topics

## This Week at a Glance

This was a **research-heavy and feature-rich week**. The centerpiece was a comprehensive Bitcoin deep-dive — 8 interconnected notes covering everything from the UTXO model to the U.S. Strategic Bitcoin Reserve, all dated March 7 and structured around a clean Map of Content. The research captures Bitcoin at an interesting inflection point: post-ATH correction, institutional ETF dominance, and the first post-halving year to finish red.

On the development side, Granit accumulated a significant batch of unreleased features — most notably a full task manager rewrite, blog publisher, custom diagram engine, and AI template generator. The three combat sports diagram showcases (boxing, Muay Thai, BJJ) serve as both real training reference material and compelling demos of the diagram engine's capabilities.

Several test notes were created throughout the week, likely as part of feature testing and demos. These should be cleaned up now that testing is complete. The vault would also benefit from connecting the Bitcoin research island to the broader note graph, and from addressing the mislabeled "Machine Learning basics" note.
