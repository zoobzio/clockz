# clockz

[![CI Status](https://github.com/zoobzio/clockz/workflows/CI/badge.svg)](https://github.com/zoobzio/clockz/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/clockz/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/clockz)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/clockz)](https://goreportcard.com/report/github.com/zoobzio/clockz)
[![CodeQL](https://github.com/zoobzio/clockz/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/clockz/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/clockz.svg)](https://pkg.go.dev/github.com/zoobzio/clockz)
[![License](https://img.shields.io/github/license/zoobzio/clockz)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/clockz)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/clockz)](https://github.com/zoobzio/clockz/releases)

Type-safe clock abstractions for Go with zero dependencies.

Build reliable applications with deterministic time handling that are easy to test and reason about.

## The Power of Abstractions

At its core, clockz is built on a simple, clean interface:

```go
type Clock interface {
    Now() time.Time
    After(d time.Duration) <-chan time.Time
    AfterFunc(d time.Duration, f func()) Timer
    NewTimer(d time.Duration) Timer
    NewTicker(d time.Duration) Ticker
    Sleep(d time.Duration)
    Since(t time.Time) time.Duration
    WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc)
    WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc)
}
```

**Any type that implements this interface can be used as a clock.** This means you can:
- Use the real clock implementation for production
- Use fake clock implementations for deterministic testing
- Mix and match both approaches seamlessly

```go
// Production code - uses real time
func processWithDeadline(clock clockz.Clock, deadline time.Time) error {
    if clock.Now().After(deadline) {
        return errors.New("deadline already passed")
    }
    
    duration := deadline.Sub(clock.Now())
    timer := clock.NewTimer(duration)
    defer timer.Stop()
    
    select {
    case <-timer.C():
        return errors.New("operation timed out")
    case <-doWork():
        return nil
    }
}

// Test code - uses fake time for deterministic testing
func TestProcessWithDeadline(t *testing.T) {
    clock := clockz.NewFakeClockAt(time.Now())
    deadline := clock.Now().Add(5 * time.Minute)
    
    // Simulate timeout
    clock.Advance(6 * time.Minute)
    
    err := processWithDeadline(clock, deadline)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "deadline already passed")
}
```

## Quick Start

While you can implement `Clock` directly, clockz provides convenient implementations:

```go
// Real clock for production
realClock := clockz.RealClock
now := realClock.Now()

// Fake clock for testing
fakeClock := clockz.NewFakeClockAt(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
testTime := fakeClock.Now() // Always returns 2024-01-01 12:00:00 UTC

// Advance time in tests
fakeClock.Advance(1 * time.Hour)
laterTime := fakeClock.Now() // Returns 2024-01-01 13:00:00 UTC
```

## Why clockz?

- **Type-safe**: Full compile-time type checking with Go generics
- **Deterministic testing**: Fake clocks eliminate time-based test flakiness
- **Zero dependencies**: Just standard library
- **Thread-safe**: All operations are safe for concurrent use
- **Context integration**: Built-in support for timeouts and cancellation
- **Production ready**: Battle-tested patterns for reliable time handling
- **Simple**: Minimal API surface, easy to understand and use

## Installation

```bash
go get github.com/zoobzio/clockz
```

Requirements: Go 1.21+ (for generics)

## Quick Example

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

func (s *Service) ProcessWithTimeout(ctx context.Context, timeout time.Duration) error {
    // Create a timeout context using our clock
    timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    // Start processing
    start := s.clock.Now()
    
    select {
    case <-timeoutCtx.Done():
        return fmt.Errorf("operation timed out after %v", s.clock.Since(start))
    case <-s.doWork():
        fmt.Printf("Completed in %v\n", s.clock.Since(start))
        return nil
    }
}

func (s *Service) doWork() <-chan struct{} {
    done := make(chan struct{})
    // Simulate work with a timer from our clock
    s.clock.AfterFunc(100*time.Millisecond, func() {
        close(done)
    })
    return done
}

func main() {
    // Production: use real clock
    service := &Service{clock: clockz.RealClock}
    
    ctx := context.Background()
    err := service.ProcessWithTimeout(ctx, 200*time.Millisecond)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Core Concepts

**The Clock Interface**: Everything in clockz implements `Clock`. You can:
- Use the real clock for production code
- Use fake clocks for deterministic testing
- Create custom clock implementations for specific needs

**Real Clock**:
- Wraps the standard `time` package
- Provides the same functionality with a testable interface
- Safe for concurrent use

**Fake Clock**:
- Deterministic time for testing
- Manually advance time with `Advance()` method
- All timers and tickers respect the fake time
- Thread-safe operations

**Context Integration**:
- `WithTimeout()` creates timeout contexts using clock time
- `WithDeadline()` creates deadline contexts using clock time  
- Works seamlessly with standard context patterns
- Enables deterministic timeout testing with fake clocks

**Additional Methods**:
- `Sleep()` blocks for a duration using the clock's time source
- `After()` returns a channel that receives after a duration
- `Since()` calculates time elapsed since a given time
- All methods respect the clock implementation (real vs fake)

## Method Examples

**Context Methods**:
```go
// WithTimeout creates a context that cancels after duration
ctx, cancel := clock.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// WithDeadline creates a context that cancels at specific time
deadline := clock.Now().Add(1 * time.Minute)
ctx, cancel = clock.WithDeadline(context.Background(), deadline)
defer cancel()
```

**Blocking and Timing**:
```go
// Sleep blocks for duration (respects fake time)
clock.Sleep(100 * time.Millisecond)

// After returns channel that receives after duration  
select {
case <-clock.After(time.Second):
    fmt.Println("Timer fired")
case <-ctx.Done():
    fmt.Println("Context cancelled")
}

// Since calculates elapsed time
start := clock.Now()
// ... do work ...
elapsed := clock.Since(start)
```

## Testing Example

Here's how clockz makes time-dependent code testable:

```go
// Production code that depends on time
func RetryWithBackoff(clock clockz.Clock, fn func() error) error {
    backoff := time.Second
    for i := 0; i < 3; i++ {
        if err := fn(); err == nil {
            return nil
        }
        
        timer := clock.NewTimer(backoff)
        <-timer.C()
        backoff *= 2
    }
    return errors.New("max retries exceeded")
}

// Test that runs instantly but tests real behavior
func TestRetryWithBackoff(t *testing.T) {
    clock := clockz.NewFakeClockAt(time.Now())
    attempts := 0
    
    // Function that fails twice, then succeeds
    fn := func() error {
        attempts++
        if attempts < 3 {
            return errors.New("temporary failure")
        }
        return nil
    }
    
    // Run retry in goroutine
    done := make(chan error, 1)
    go func() {
        done <- RetryWithBackoff(clock, fn)
    }()
    
    // Advance time to trigger retries
    clock.Advance(time.Second)    // First retry
    clock.Advance(2 * time.Second) // Second retry
    
    // Verify success
    err := <-done
    assert.NoError(t, err)
    assert.Equal(t, 3, attempts)
}
```

## Performance

clockz is designed for minimal overhead:

- Real clock operations delegate directly to `time` package
- Fake clock operations are simple atomic operations
- No reflection or runtime type assertions
- Optimized for common paths

Run benchmarks:
```bash
make bench
```

## Contributing

Contributions welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
# Run tests
make test

# Run linter
make lint

# Run benchmarks
make bench
```

## License

MIT License - see [LICENSE](LICENSE) file for details.