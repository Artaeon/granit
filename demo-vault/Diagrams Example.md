---
title: Diagrams Example
date: 2026-03-08
tags: [diagrams, mermaid, reference, showcase]
---

# Mermaid Diagram Examples

Granit supports rendering Mermaid diagrams in preview mode. This note showcases the most common diagram types. See [[Getting Started]] for how to toggle between edit and preview modes.

## Flowchart

A decision flow for choosing a database technology (related to [[Research/Graph Databases]]):

```mermaid
flowchart TD
    A[Need a Database?] --> B{What type of data?}
    B -->|Structured, relational| C[PostgreSQL]
    B -->|Documents, flexible schema| D[MongoDB]
    B -->|Highly connected| E{Scale?}
    B -->|Key-Value, cache| F[Redis]
    E -->|Small/Medium| G[SurrealDB]
    E -->|Large/Enterprise| H[Neo4j]
    C --> I{Need full-text search?}
    I -->|Yes| J[Add Elasticsearch]
    I -->|No| K[PostgreSQL is enough]
    G --> L[Embed in application]
    H --> M[Dedicated server]
```

## Sequence Diagram

The authentication flow being rebuilt in [[Projects/Web App Redesign]]:

```mermaid
sequenceDiagram
    participant U as User
    participant C as Client App
    participant A as Auth Service
    participant D as Database
    participant T as Token Store

    U->>C: Enter credentials
    C->>A: POST /auth/login
    A->>D: Validate credentials
    D-->>A: User record
    A->>A: Generate JWT + Refresh Token
    A->>T: Store refresh token
    A-->>C: {access_token, refresh_token}
    C->>C: Store tokens
    C-->>U: Redirect to dashboard

    Note over C,A: Later, token expires...

    C->>A: POST /auth/refresh
    A->>T: Validate refresh token
    T-->>A: Token valid
    A->>A: Generate new JWT
    A-->>C: {access_token}
```

## Class Diagram

Granit's core data model:

```mermaid
classDiagram
    class Vault {
        +string Path
        +string Name
        +[]Note Notes
        +Config Config
        +Scan()
        +Search(query) []Note
    }

    class Note {
        +string Path
        +string Title
        +string Content
        +Frontmatter map
        +time Created
        +time Modified
        +GetTags() []string
        +GetLinks() []string
    }

    class Config {
        +string Theme
        +string Layout
        +string AIProvider
        +string OllamaModel
        +bool VimMode
        +Load()
        +Save()
    }

    class Index {
        +map Notes
        +map Tags
        +map Links
        +Build(vault)
        +Search(query) []Result
    }

    Vault "1" --> "*" Note : contains
    Vault "1" --> "1" Config : uses
    Vault "1" --> "1" Index : maintains
    Index --> Note : indexes
```

## State Diagram

Editor mode transitions:

```mermaid
stateDiagram-v2
    [*] --> Normal : Open note
    Normal --> Insert : i, a, o
    Insert --> Normal : Esc
    Normal --> Visual : v
    Visual --> Normal : Esc
    Normal --> Command : :
    Command --> Normal : Enter/Esc
    Normal --> Search : /
    Search --> Normal : Enter/Esc

    state Normal {
        [*] --> Navigation
        Navigation --> Editing : d, c, y
        Editing --> Navigation : Complete
    }
```

## Pie Chart

Time distribution across this week's activities:

```mermaid
pie title Weekly Time Distribution
    "Coding" : 35
    "Code Review" : 15
    "Meetings" : 10
    "Research & Reading" : 15
    "Documentation" : 10
    "Testing" : 10
    "Planning" : 5
```

## Gantt Chart

Sprint timeline (see [[Tasks]] for details):

```mermaid
gantt
    title Sprint 2026-W10
    dateFormat  YYYY-MM-DD
    axisFormat  %a %d

    section Web App
    Auth flow migration       :active, auth, 2026-03-08, 3d
    Component library         :crit, comp, 2026-03-08, 5d
    E2E test setup           :e2e, after auth, 3d

    section CLI Tool
    Export module             :export, 2026-03-09, 4d
    Link checker             :after export, 3d

    section Infrastructure
    Staging credentials       :crit, staging, 2026-03-09, 1d
    CI/CD pipeline            :cicd, after staging, 4d

    section Research
    SurrealDB PoC             :poc, 2026-03-10, 5d
```

## Entity Relationship Diagram

Database schema for the web app:

```mermaid
erDiagram
    USER ||--o{ NOTE : creates
    USER ||--o{ VAULT : owns
    VAULT ||--|{ NOTE : contains
    NOTE ||--o{ TAG : has
    NOTE ||--o{ LINK : "links to"
    NOTE ||--o{ TASK : contains
    LINK }o--|| NOTE : "points to"

    USER {
        uuid id PK
        string email
        string name
        timestamp created_at
    }

    VAULT {
        uuid id PK
        uuid user_id FK
        string name
        string path
    }

    NOTE {
        uuid id PK
        uuid vault_id FK
        string title
        text content
        jsonb frontmatter
        timestamp created_at
        timestamp modified_at
    }

    TAG {
        uuid id PK
        string name
    }

    TASK {
        uuid id PK
        uuid note_id FK
        string text
        boolean done
        date due_date
        string priority
    }
```

## Tips for Writing Diagrams

1. Keep diagrams focused — one concept per diagram
2. Use descriptive labels, not single letters
3. Flowcharts work best for decision processes
4. Sequence diagrams are ideal for API interactions
5. Use `crit` in Gantt charts to highlight critical path items

---

*Related: [[Welcome]] | [[Meetings/Architecture Review]] | [[Projects/Web App Redesign]]*
