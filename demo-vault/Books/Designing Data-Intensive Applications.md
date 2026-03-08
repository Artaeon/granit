---
title: "Designing Data-Intensive Applications"
date: 2026-02-05
tags: [book, databases, distributed-systems, architecture]
author: Martin Kleppmann
rating: 5
status: reading
---

# Designing Data-Intensive Applications

**Author:** Martin Kleppmann | **Published:** 2017 | **Pages:** 616
**My Rating:** 5/5 — Essential reading for anyone building data systems

## Why This Book Matters

This is the single best book on data systems I have read. It bridges the gap between academic distributed systems literature and practical engineering. Every chapter is dense with insight.

> "Data is at the center of many challenges in system design today." — Kleppmann

## Part I: Foundations of Data Systems

### Chapter 1 — Reliability, Scalability, Maintainability

The three pillars of good data system design:

- **Reliability** — The system works correctly even when things go wrong (hardware faults, software bugs, human errors)
- **Scalability** — The system handles growth gracefully (data volume, traffic, complexity)
- **Maintainability** — The system is easy to work on over time (operability, simplicity, evolvability)

*Key insight:* Design for the common case, handle the edge cases gracefully. Never assume things won't fail.

### Chapter 2 — Data Models and Query Languages

Comparison of relational, document, and graph models. This chapter directly informed our [[Research/Graph Databases]] evaluation.

**Graph models** are ideal when relationships between entities are as important as the entities themselves — exactly the case for a knowledge base like Granit.

### Chapter 3 — Storage and Retrieval

Two main families of storage engines:
1. **Log-structured** (LSM trees, SSTables) — Optimized for writes
2. **Page-oriented** (B-trees) — Optimized for reads

| Engine Type | Write | Read | Space | Use Case |
|-------------|-------|------|-------|----------|
| LSM Tree | Fast | Slower | Compact | Write-heavy workloads |
| B-Tree | Slower | Fast | More overhead | Read-heavy, transactional |

## Part II: Distributed Data

### Chapter 5 — Replication

Three replication strategies:
1. **Single-leader** — One writable replica, others are read-only
2. **Multi-leader** — Multiple writable replicas (complex conflict resolution)
3. **Leaderless** — Any replica accepts writes (Dynamo-style)

*Key insight:* Eventual consistency is not a single model — there is a spectrum of consistency guarantees (read-after-write, monotonic reads, consistent prefix reads).

### Chapter 6 — Partitioning

How to split data across multiple nodes:
- **Key range partitioning** — Good for range queries, risk of hot spots
- **Hash partitioning** — Even distribution, but no range queries
- **Compound strategies** — Hash the first part of the key, range on the rest

### Chapter 7 — Transactions

ACID is not as straightforward as it sounds:
- **Atomicity** — All or nothing (not about concurrency!)
- **Consistency** — Application-level invariants hold
- **Isolation** — Concurrent transactions don't interfere
- **Durability** — Committed data survives crashes

## Key Takeaways

1. There is no one-size-fits-all database. Choose based on access patterns.
2. Distributed systems introduce fundamental tradeoffs (CAP is an oversimplification).
3. Schema-on-read vs. schema-on-write is a spectrum, not a binary.
4. Understand your consistency requirements before choosing replication.
5. Batch processing and stream processing are converging.

## How This Applies to Our Work

- The [[Projects/Web App Redesign]] backend needs to handle real-time updates — Chapter 11 on stream processing is directly relevant
- [[Research/Graph Databases]] evaluation was guided by Chapter 2's comparison framework
- The [[Meetings/Architecture Review]] decisions on data layer align with Chapter 3's storage engine analysis

## Quotes I Highlighted

> "A system is only as reliable as its least reliable component."

> "The limits of my language mean the limits of my world." (quoting Wittgenstein, on query language design)

> "Complexity is accidental if it is not inherent in the problem but arises from the implementation."

## Related

- [[Books/The Pragmatic Programmer]] — Complementary reading on software engineering practices
- [[Research/Machine Learning Basics]] — ML pipelines are data-intensive applications
- [[MOC - Knowledge Management]] — This book informs how we think about data in PKM tools
