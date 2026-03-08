---
title: Graph Databases
date: 2026-02-18
tags: [research, databases, graph, backend]
status: draft
---

# Graph Databases

Research notes on graph database technologies for potential use in the backend of [[Projects/Web App Redesign]] and for powering the knowledge graph in Granit.

## Why Graph Databases?

Traditional relational databases struggle with highly connected data. When your queries involve multiple joins across relationship-heavy schemas, performance degrades. Graph databases model relationships as first-class citizens.

**Use cases:**
- Social networks and recommendation engines
- Knowledge graphs and ontologies
- Fraud detection and network analysis
- Dependency resolution (package managers, build systems)
- **Note linking** — exactly what Granit's graph view does

## Technology Comparison

| Feature | Neo4j | ArangoDB | DGraph | SurrealDB |
|---------|-------|----------|--------|-----------|
| Query Language | Cypher | AQL | GraphQL+ | SurrealQL |
| License | GPL / Commercial | Apache 2.0 | Apache 2.0 | BSL 1.1 |
| Multi-model | No | Yes (doc+graph+KV) | No | Yes (doc+graph) |
| Clustering | Enterprise only | Yes | Yes | Yes |
| Embeddable | No | No | Yes (badger) | Yes |
| Written In | Java | C++ | Go | Rust |
| Maturity | Very High | High | Medium | Low |

## Query Examples

### Neo4j (Cypher)

```cypher
// Find all notes linked from a given note, up to 3 hops
MATCH path = (start:Note {title: "Welcome"})-[:LINKS_TO*1..3]->(related:Note)
RETURN related.title, length(path) as depth
ORDER BY depth

// Find notes that share the most tags
MATCH (n1:Note)-[:HAS_TAG]->(t:Tag)<-[:HAS_TAG]-(n2:Note)
WHERE n1 <> n2
RETURN n1.title, n2.title, COUNT(t) as shared_tags
ORDER BY shared_tags DESC
LIMIT 10
```

### ArangoDB (AQL)

```aql
// Shortest path between two notes
FOR v, e IN OUTBOUND SHORTEST_PATH
  'notes/welcome' TO 'notes/ml-basics'
  GRAPH 'knowledge_graph'
  RETURN v.title

// Find orphan notes (no incoming links)
FOR note IN notes
  LET incoming = (
    FOR v IN 1..1 INBOUND note links
    RETURN v
  )
  FILTER LENGTH(incoming) == 0
  RETURN note.title
```

## Graph Data Modeling for a Knowledge Base

```
(:Note {title, content, created, modified})
  -[:LINKS_TO]->(:Note)
  -[:HAS_TAG]->(:Tag {name})
  -[:IN_FOLDER]->(:Folder {path})
  -[:HAS_TASK]->(:Task {text, done, due, priority})
```

> This schema maps directly to Granit's vault model. Each note is a node, wikilinks are `LINKS_TO` edges, and tags from frontmatter become `HAS_TAG` relationships.

## Performance Considerations

For a personal knowledge base (hundreds to low thousands of notes), **any** of these databases would be overkill. Granit currently uses an in-memory index built at startup, which is perfectly adequate.

However, for a hosted/collaborative version, a proper graph database would enable:
- Real-time collaborative graph exploration
- Cross-vault link suggestions
- Community knowledge discovery

## Embedding Options

For a local-first tool like Granit, embedding is key. Two promising options:

1. **DGraph with Badger** — Go-native, embeds as a library
2. **SurrealDB** — Single binary, Rust-based, very fast startup

Both would avoid the need for a separate database server.

## Related Notes

- [[Research/Machine Learning Basics]] — Graph Neural Networks combine ML with graph structures
- [[Books/Designing Data-Intensive Applications]] — Chapter 2 covers graph data models
- [[Meetings/Architecture Review]] — We discussed graph DB options for the backend
- [[Projects/Web App Redesign]] — Potential consumer of graph queries for the dashboard
