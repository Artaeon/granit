---
title: "The Pragmatic Programmer"
date: 2026-01-20
tags: [book, software-engineering, career, practices]
author: David Thomas, Andrew Hunt
rating: 5
status: completed
---

# The Pragmatic Programmer

**Authors:** David Thomas & Andrew Hunt | **Edition:** 20th Anniversary (2019) | **Pages:** 352
**My Rating:** 5/5 — A career-defining book for software developers

## Overview

This book is less about specific technologies and more about the *mindset* of a good developer. It is organized as a collection of tips (100 total) covering everything from coding practices to career management.

## Key Tips I Keep Coming Back To

### Tip 1: Care About Your Craft
> "There is no point in developing software unless you care about doing it well."

This is the foundation. If you don't care, nothing else in the book matters. It is why I spend evenings working on [[Projects/Web App Redesign]] — because building quality software is intrinsically rewarding.

### Tip 7: Invest Regularly in Your Knowledge Portfolio

Treat your knowledge like a financial portfolio:
- **Diversify** — Learn different technologies and domains
- **Manage risk** — Balance stable skills with bleeding-edge experiments
- **Buy low, sell high** — Learn emerging tech before it becomes mainstream
- **Review and rebalance** — Regularly assess what you know and what you need to learn

This is exactly what the [[Research/Machine Learning Basics]] and [[Research/Graph Databases]] notes represent — ongoing investments.

### Tip 11: DRY — Don't Repeat Yourself
Not just about code duplication. DRY applies to:
- Knowledge representation
- Documentation
- Data schemas
- Build processes

> "Every piece of knowledge must have a single, unambiguous, authoritative representation within a system."

### Tip 17: Eliminate Effects Between Unrelated Things (Orthogonality)

Components should be independent. Changing one should not affect others. In Granit's architecture, this is reflected in how each TUI component (`sidebar.go`, `editor.go`, `backlinks.go`) is self-contained.

### Tip 27: Don't Outrun Your Headlights

Take small steps. Get feedback. Adjust. This applies to:
- **Coding** — Write a little, test a little, refactor a little
- **Design** — Build prototypes, not cathedrals
- **Projects** — Deliver incrementally, not in a big bang

The [[Projects/Web App Redesign]] follows this principle with its incremental micro-frontend migration approach.

### Tip 68: Build End-to-End, Not Top-Down or Bottom-Up

Build a thin slice through all layers first (a "tracer bullet"). This validates the architecture before you invest heavily in any single layer.

```
❌ Build all UI → Build all API → Build all DB
✅ Build one feature: UI + API + DB → Next feature: UI + API + DB
```

## The "Broken Windows" Theory

One of the most powerful metaphors in the book:

> Don't leave "broken windows" (bad designs, wrong decisions, poor code) unrepaired. Fix each one as soon as it is discovered.

Neglect accelerates decay. A single broken window signals that nobody cares, which invites more breakage. This applies to codebases, documentation, and even vault organization.

## Practical Exercises I Found Useful

1. **Learn a new language every year** — Even if you don't use it professionally. It expands your thinking.
2. **Read a technical book every month** — See [[Books/Designing Data-Intensive Applications]] for this month's pick.
3. **Participate in code reviews** — Both giving and receiving feedback improves your craft.
4. **Automate everything you do more than twice** — Hence [[Projects/CLI Tool]] for vault operations.

## Comparison with Other Books

| Book | Focus | Best For |
|------|-------|----------|
| The Pragmatic Programmer | Mindset & practices | All developers |
| Clean Code (Martin) | Code quality | Day-to-day coding |
| DDIA (Kleppmann) | Data systems | Backend/infra engineers |
| Design Patterns (GoF) | OOP patterns | Architecture decisions |
| Refactoring (Fowler) | Code improvement | Maintenance work |

## Quotes

> "You can't write perfect software. Did that hurt? It shouldn't. Accept it as an axiom of life."

> "An investment in knowledge always pays the best interest." (quoting Benjamin Franklin)

> "Don't assume it — prove it."

## Related Notes

- [[Books/Designing Data-Intensive Applications]] — Complementary deep-dive on data systems
- [[Ideas/Blog Post Ideas]] — Several post ideas inspired by this book
- [[MOC - Knowledge Management]] — PKM is a pragmatic programmer's tool
- [[Getting Started]] — Granit itself follows many tips from this book
