---
title: Go Programming
tags: [programming, golang, technical]
created: 2025-08-15
---

# Go Programming

Notes on Go development patterns and idioms.

## Why Go

- **Simple syntax** — Easy to read and maintain
- **Fast compilation** — Near-instant builds
- **Built-in concurrency** — Goroutines and channels
- **Static binary** — Single file deployment
- **Great tooling** — `go fmt`, `go vet`, `go test`

## Patterns

### Error Handling

```go
result, err := doSomething()
if err != nil {
    return fmt.Errorf("context: %w", err)
}
```

### Interface Composition

```go
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

type ReadWriter interface {
    Reader
    Writer
}
```

### Table-Driven Tests

```go
tests := []struct {
    name     string
    input    string
    expected int
}{
    {"empty", "", 0},
    {"single", "a", 1},
    {"multi", "abc", 3},
}
```

## Projects Built with Go

- [[Architecture|Granit]] — This very app!
- Docker, Kubernetes, Terraform
- Hugo, CockroachDB, Prometheus

## Resources

- [Go by Example](https://gobyexample.com)
- [Effective Go](https://go.dev/doc/effective_go)

See also: [[Architecture]], [[Features]]
