# clockz

[![CI Status](https://github.com/zoobzio/clockz/workflows/CI/badge.svg)](https://github.com/zoobzio/clockz/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/clockz/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/clockz)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/clockz)](https://goreportcard.com/report/github.com/zoobzio/clockz)
[![CodeQL](https://github.com/zoobzio/clockz/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/clockz/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/clockz.svg)](https://pkg.go.dev/github.com/zoobzio/clockz)
[![License](https://img.shields.io/github/license/zoobzio/clockz)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod-go-version/zoobzio/clockz)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/clockz)](https://github.com/zoobzio/clockz/releases)

Type-safe clock abstractions for Go with zero dependencies.

Build time-dependent code that's deterministic to test and simple to reason about.

## Deterministic Time

```go
func TestRetryBackoff(t *testing.T) {
    // Create a clock frozen at a specific moment
    clock := clockz.NewFakeClockAt(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
    service := NewService(clock)

    go service.RetryWithBackoff()

    // Advance through retry delays instantly
    clock.Advance(1 * time.Second)  // First retry
    clock.Advance(2 * time.Second)  // Second retry
    clock.Advance(4 * time.Second)  // Third retry

    // Test completes in milliseconds, not seconds
}
```

No sleeps. No flaky timeouts. Time moves when you say so.

## Installation

```bash
go get github.com/zoobzio/clockz
```

Requires Go 1.24+

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/zoobzio/clockz"
)

type Service struct {
    clock clockz.Clock
}

func (s *Service) ProcessWithTimeout(ctx context.Context) error {
    ctx, cancel := s.clock.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    timer := s.clock.NewTimer(5 * time.Second)
    defer timer.Stop()

    select {
    case t := <-timer.C():
        fmt.Printf("Processing at %v\n", t)
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func main() {
    // Production: real time
    service := &Service{clock: clockz.RealClock}
    if err := service.ProcessWithTimeout(context.Background()); err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Capabilities

| Feature | Description | Docs |
|---------|-------------|------|
| `RealClock` | Production clock wrapping `time` package | [API Reference](docs/5.reference/1.api.md) |
| `FakeClock` | Test clock with manual time control | [API Reference](docs/5.reference/1.api.md) |
| Timers & Tickers | Create, stop, reset timing primitives | [Concepts](docs/2.learn/2.concepts.md) |
| Context Integration | `WithTimeout` and `WithDeadline` support | [Concepts](docs/2.learn/2.concepts.md) |
| Time Advancement | `Advance()` and `SetTime()` for tests | [Testing Guide](docs/3.guides/1.testing.md) |

## Why clockz?

- **Deterministic tests** — Eliminate time-based flakiness entirely
- **Zero dependencies** — Only the standard library
- **Thread-safe** — All operations safe for concurrent use
- **Familiar API** — Mirrors the `time` package interface
- **Context-aware** — Built-in timeout and deadline support

## Documentation

**Learn**
- [Quick Start](docs/2.learn/1.quickstart.md) — Get up and running
- [Core Concepts](docs/2.learn/2.concepts.md) — Clock selection, dependency injection

**Guides**
- [Testing Patterns](docs/3.guides/1.testing.md) — Strategies for time-dependent tests

**Cookbook**
- [Common Patterns](docs/4.cookbook/1.patterns.md) — Retry, rate limiting, deadlines

**Reference**
- [API Reference](docs/5.reference/1.api.md) — Complete interface documentation

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
make help      # Available commands
make check     # Run tests and lint
```

## License

MIT — see [LICENSE](LICENSE)
