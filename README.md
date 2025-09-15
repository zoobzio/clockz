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

## Quick Start

```go
import "github.com/zoobzio/clockz"

// Production: use real time
service := &Service{clock: clockz.RealClock}

// Testing: control time precisely
clock := clockz.NewFakeClockAt(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
clock.Advance(5 * time.Minute) // Jump forward in time
```

## Why clockz?

- **Deterministic Testing**: Eliminate time-based test flakiness
- **Zero Dependencies**: Just the standard library
- **Thread-Safe**: All operations safe for concurrent use
- **Context Integration**: Built-in timeout and deadline support
- **Simple API**: Familiar interface matching `time` package

## Installation

```bash
go get github.com/zoobzio/clockz
```

Requirements: Go 1.21+

## Core Concepts

### Clock Selection Guide

**Use RealClock when:**
- Running in production
- Integrating with external systems
- Measuring actual performance

**Use FakeClock when:**
- Writing unit tests
- Testing timeout behavior
- Simulating time-dependent scenarios
- Eliminating test flakiness

### Dependency Injection Pattern

```go
type Service struct {
    clock clockz.Clock // Inject clock dependency
}

func NewService(clock clockz.Clock) *Service {
    return &Service{clock: clock}
}

func (s *Service) ProcessWithRetry() error {
    for attempt := 0; attempt < 3; attempt++ {
        if err := s.process(); err == nil {
            return nil
        }
        
        // Use injected clock for delays
        s.clock.Sleep(time.Second * time.Duration(1<<attempt))
    }
    return errors.New("max retries exceeded")
}

// Production
service := NewService(clockz.RealClock)

// Testing - runs instantly
func TestProcessWithRetry(t *testing.T) {
    clock := clockz.NewFakeClockAt(time.Now())
    service := NewService(clock)
    
    go service.ProcessWithRetry()
    
    // Control time advancement
    clock.Advance(1 * time.Second)  // First retry
    clock.Advance(2 * time.Second)  // Second retry
}
```

## Basic Usage Examples

### Timer Management

```go
// Create and use timers
timer := clock.NewTimer(5 * time.Second)
defer timer.Stop()

select {
case <-timer.C():
    fmt.Println("Timer expired")
case <-ctx.Done():
    fmt.Println("Context cancelled")
}

// Reset timer for reuse
if timer.Reset(10 * time.Second) {
    fmt.Println("Timer was active")
}
```

### Ticker Operations

```go
// Create ticker for periodic events
ticker := clock.NewTicker(time.Second)
defer ticker.Stop()

for i := 0; i < 5; i++ {
    <-ticker.C()
    fmt.Printf("Tick %d at %v\n", i+1, clock.Now())
}
```

### Context with Timeouts

```go
// Create timeout context
ctx, cancel := clock.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Create deadline context
deadline := clock.Now().Add(1 * time.Hour)
ctx, cancel = clock.WithDeadline(context.Background(), deadline)
defer cancel()

// Use in operations
select {
case result := <-doWork():
    return result, nil
case <-ctx.Done():
    return nil, ctx.Err()
}
```

### Time Measurements

```go
// Measure elapsed time
start := clock.Now()
processData()
elapsed := clock.Since(start)
fmt.Printf("Processing took %v\n", elapsed)

// Sleep for duration
clock.Sleep(100 * time.Millisecond)

// Wait with channel
select {
case <-clock.After(5 * time.Second):
    fmt.Println("Timeout reached")
case result := <-operation():
    fmt.Printf("Got result: %v\n", result)
}
```

## Testing Patterns

### Basic Time Control

```go
func TestTimeoutBehavior(t *testing.T) {
    clock := clockz.NewFakeClockAt(time.Now())
    
    done := make(chan bool)
    go func() {
        <-clock.After(5 * time.Minute)
        done <- true
    }()
    
    // Verify nothing happens before timeout
    clock.Advance(4 * time.Minute)
    select {
    case <-done:
        t.Fatal("Should not timeout yet")
    default:
        // Expected
    }
    
    // Trigger timeout
    clock.Advance(1 * time.Minute)
    select {
    case <-done:
        // Success
    case <-time.After(100 * time.Millisecond):
        t.Fatal("Should have timed out")
    }
}
```

### Testing Concurrent Operations

```go
func TestConcurrentTimers(t *testing.T) {
    clock := clockz.NewFakeClockAt(time.Now())
    results := make(chan int, 3)
    
    // Start multiple timers
    go func() {
        <-clock.After(1 * time.Second)
        results <- 1
    }()
    
    go func() {
        <-clock.After(2 * time.Second)
        results <- 2
    }()
    
    go func() {
        <-clock.After(3 * time.Second)
        results <- 3
    }()
    
    // Advance time and verify order
    clock.Advance(2 * time.Second)
    
    if val := <-results; val != 1 {
        t.Errorf("Expected 1, got %d", val)
    }
    if val := <-results; val != 2 {
        t.Errorf("Expected 2, got %d", val)
    }
    
    clock.Advance(1 * time.Second)
    if val := <-results; val != 3 {
        t.Errorf("Expected 3, got %d", val)
    }
}
```

### Context Timeout Testing

```go
func TestContextDeadline(t *testing.T) {
    clock := clockz.NewFakeClockAt(time.Now())
    
    ctx, cancel := clock.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    done := make(chan error)
    go func() {
        <-ctx.Done()
        done <- ctx.Err()
    }()
    
    // Advance to trigger timeout
    clock.Advance(5 * time.Second)
    
    err := <-done
    if err != context.DeadlineExceeded {
        t.Errorf("Expected DeadlineExceeded, got %v", err)
    }
}
```

## Advanced Patterns

### Retry with Exponential Backoff

```go
func RetryWithBackoff(clock clockz.Clock, operation func() error) error {
    backoff := 100 * time.Millisecond
    maxBackoff := 30 * time.Second
    
    for attempt := 0; attempt < 5; attempt++ {
        if err := operation(); err == nil {
            return nil
        }
        
        if attempt < 4 { // Don't sleep after last attempt
            clock.Sleep(backoff)
            backoff *= 2
            if backoff > maxBackoff {
                backoff = maxBackoff
            }
        }
    }
    
    return errors.New("operation failed after 5 attempts")
}
```

### Rate Limiting

```go
type RateLimiter struct {
    clock    clockz.Clock
    ticker   clockz.Ticker
    tokens   chan struct{}
}

func NewRateLimiter(clock clockz.Clock, rps int) *RateLimiter {
    rl := &RateLimiter{
        clock:  clock,
        ticker: clock.NewTicker(time.Second / time.Duration(rps)),
        tokens: make(chan struct{}, rps),
    }
    
    // Fill initial tokens
    for i := 0; i < rps; i++ {
        rl.tokens <- struct{}{}
    }
    
    // Refill tokens
    go func() {
        for range rl.ticker.C() {
            select {
            case rl.tokens <- struct{}{}:
            default: // Bucket full
            }
        }
    }()
    
    return rl
}

func (rl *RateLimiter) Wait() {
    <-rl.tokens
}
```

### Deadline Management

```go
func ProcessBatch(clock clockz.Clock, items []Item, deadline time.Time) error {
    for _, item := range items {
        if clock.Now().After(deadline) {
            return fmt.Errorf("deadline exceeded, processed %d/%d items", i, len(items))
        }
        
        remaining := deadline.Sub(clock.Now())
        ctx, cancel := clock.WithTimeout(context.Background(), remaining)
        
        err := processItem(ctx, item)
        cancel()
        
        if err != nil {
            return fmt.Errorf("failed to process item %d: %w", i, err)
        }
    }
    
    return nil
}
```

## Complete API Reference

### Clock Interface

The main interface that both RealClock and FakeClock implement:

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

#### Clock.Now() time.Time
Returns the current time according to the clock.
- **RealClock**: Returns `time.Now()`
- **FakeClock**: Returns the fake clock's current time

#### Clock.After(d time.Duration) <-chan time.Time
Returns a channel that sends the current time after duration d.
- **RealClock**: Wraps `time.After(d)`
- **FakeClock**: Fires when fake time advances past target
- Channel has buffer of 1 to prevent blocking

#### Clock.AfterFunc(d time.Duration, f func()) Timer
Executes function f after duration d.
- **RealClock**: Wraps `time.AfterFunc(d, f)`
- **FakeClock**: Executes synchronously when time advances
- Returns Timer that can be stopped or reset
- Function runs in its own goroutine (RealClock) or synchronously (FakeClock)

#### Clock.NewTimer(d time.Duration) Timer
Creates a new Timer that fires after duration d.
- **RealClock**: Wraps `time.NewTimer(d)`
- **FakeClock**: Creates timer that respects fake time
- Timer can be stopped or reset
- Use `timer.C()` to access the channel

#### Clock.NewTicker(d time.Duration) Ticker
Creates a new Ticker that fires repeatedly every duration d.
- **RealClock**: Wraps `time.NewTicker(d)`
- **FakeClock**: Creates ticker that respects fake time
- Must call `ticker.Stop()` to release resources
- Panics if d <= 0

#### Clock.Sleep(d time.Duration)
Blocks the calling goroutine for duration d.
- **RealClock**: Wraps `time.Sleep(d)`
- **FakeClock**: Blocks until fake time advances by d
- Implemented as `<-clock.After(d)`

#### Clock.Since(t time.Time) time.Duration
Returns time elapsed since t.
- **RealClock**: Equivalent to `time.Since(t)`
- **FakeClock**: Returns `fakeClock.Now().Sub(t)`
- Convenience method for `clock.Now().Sub(t)`

#### Clock.WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc)
Creates a context that cancels after timeout duration.
- **RealClock**: Wraps `context.WithTimeout(ctx, timeout)`
- **FakeClock**: Cancels when fake time advances past deadline
- Returns context and cancel function
- Cancel function must be called to release resources

#### Clock.WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc)
Creates a context that cancels at deadline time.
- **RealClock**: Wraps `context.WithDeadline(ctx, deadline)`
- **FakeClock**: Cancels when fake time reaches deadline
- Returns immediately cancelled context if deadline already passed
- Cancel function must be called to release resources

### Timer Interface

```go
type Timer interface {
    Stop() bool
    Reset(d time.Duration) bool
    C() <-chan time.Time
}
```

#### Timer.Stop() bool
Stops the timer.
- Returns true if timer was stopped before firing
- Returns false if timer already fired or was stopped
- After Stop(), timer will not fire
- Safe to call multiple times

#### Timer.Reset(d time.Duration) bool
Resets the timer to fire after duration d.
- Returns true if timer was active before reset
- Returns false if timer had expired or was stopped
- Timer must be stopped or expired before Reset
- Resets from current clock time, not original start time

#### Timer.C() <-chan time.Time
Returns the channel on which the timer sends its time.
- Channel has buffer of 1
- Receives exactly once when timer fires
- Channel is not closed after firing

### Ticker Interface

```go
type Ticker interface {
    Stop()
    C() <-chan time.Time
}
```

#### Ticker.Stop()
Stops the ticker.
- No more ticks will be sent after Stop
- Does not close the channel
- Required to release ticker resources

#### Ticker.C() <-chan time.Time
Returns the channel on which ticks are delivered.
- Channel has buffer of 1
- Sends time at regular intervals
- May drop ticks if receiver is slow

### RealClock Implementation

Global instance that delegates to the standard `time` package:

```go
var RealClock Clock = &realClock{}
```

Usage:
```go
// Use the global instance directly
now := clockz.RealClock.Now()
timer := clockz.RealClock.NewTimer(5 * time.Second)
```

Characteristics:
- Thread-safe
- Zero overhead (direct delegation)
- Production ready
- No additional methods beyond Clock interface

### FakeClock Implementation

Provides complete control over time for testing:

```go
// Constructors
func NewFakeClock() *FakeClock
func NewFakeClockAt(t time.Time) *FakeClock

// Additional methods beyond Clock interface
func (f *FakeClock) Advance(d time.Duration)
func (f *FakeClock) SetTime(t time.Time)
func (f *FakeClock) HasWaiters() bool
func (f *FakeClock) BlockUntilReady()
```

#### NewFakeClock() *FakeClock
Creates a fake clock initialized to `time.Now()`.
- Captures current real time as starting point
- Time does not advance automatically

#### NewFakeClockAt(t time.Time) *FakeClock
Creates a fake clock initialized to specific time t.
- Useful for deterministic test scenarios
- Common pattern: `NewFakeClockAt(time.Unix(0, 0))`

#### FakeClock.Advance(d time.Duration)
Advances the fake clock's time by duration d.
- Triggers all timers scheduled before new time
- Executes AfterFunc callbacks synchronously
- Updates all tickers
- Triggers context timeouts/deadlines
- Thread-safe

#### FakeClock.SetTime(t time.Time)
Sets the fake clock to specific time t.
- Can move time forward or backward
- Forward movement triggers timers/contexts
- Backward movement does not untrigger fired timers
- Thread-safe

#### FakeClock.HasWaiters() bool
Returns true if there are active timers, tickers, or contexts.
- Useful for test assertions
- Includes: timers, tickers, AfterFunc callbacks, contexts
- Thread-safe

#### FakeClock.BlockUntilReady()
Blocks until all pending timer operations are delivered.
- Ensures timer channel sends are processed
- Waits for context timeout processing (1ms sleep)
- Non-blocking sends: skips if receiver not ready
- Useful for test synchronization
- Thread-safe

### Context Implementation Details

The fake clock implements custom context handling for deterministic timeout testing:

**Parent Context Monitoring:**
- Starts goroutine to watch parent cancellation
- Propagates parent cancellation to child
- Cleans up on cancellation

**Timeout Behavior:**
- Contexts cancel precisely when fake time reaches deadline
- No real time delays in tests
- Proper error codes (DeadlineExceeded vs Canceled)

**Resource Management:**
- Cancel functions clean up waiters
- Goroutines terminated on context cancellation
- No resource leaks

### Thread Safety

All clockz operations are thread-safe:

**RealClock:**
- Delegates to thread-safe `time` package functions
- No additional synchronization needed

**FakeClock:**
- Uses `sync.RWMutex` for time access
- Separate `contextMu` for context operations
- Safe for concurrent timer/ticker creation
- Safe for concurrent time advancement

### Performance Characteristics

**RealClock:**
- Zero overhead: direct delegation to `time` package
- No allocations beyond `time` package needs
- Suitable for production use

**FakeClock:**
- O(n) time advancement where n = number of waiters
- Timers sorted by target time
- Synchronous AfterFunc execution
- Minimal allocations for timer tracking

## Testing clockz Itself

The library includes comprehensive tests demonstrating usage patterns:

```bash
# Run all tests
make test

# Run with race detection
make test-race

# Run with coverage
make test-coverage

# Run benchmarks
make bench
```

Key test files to examine:
- `fake_test.go`: Fake clock behavior tests
- `integration_test.go`: Real-world usage patterns
- `example_test.go`: Runnable documentation examples

## Common Pitfalls and Solutions

### Pitfall: Timer Channel Blocking

**Problem:** Timer channels have buffer of 1 and may block if not consumed.

**Solution:** Always consume timer channel or use Stop():
```go
timer := clock.NewTimer(5 * time.Second)
defer timer.Stop() // Prevents resource leak

select {
case <-timer.C():
    // Handle timeout
case <-done:
    // Handle completion
}
```

### Pitfall: Ticker Resource Leak

**Problem:** Tickers must be stopped to release resources.

**Solution:** Always defer Stop():
```go
ticker := clock.NewTicker(time.Second)
defer ticker.Stop() // Critical for cleanup
```

### Pitfall: Context Cancel Function Leak

**Problem:** Not calling cancel function leaks resources.

**Solution:** Always defer cancel():
```go
ctx, cancel := clock.WithTimeout(ctx, 30*time.Second)
defer cancel() // Always call, even on success path
```

### Pitfall: Fake Clock Synchronization

**Problem:** Test operations may not be ready when time advances.

**Solution:** Use BlockUntilReady() or proper goroutine synchronization:
```go
go func() {
    <-clock.After(5 * time.Second)
    done <- true
}()

clock.BlockUntilReady() // Ensure timer is set up
clock.Advance(5 * time.Second)
```

### Pitfall: AfterFunc Execution Differences

**Problem:** FakeClock executes AfterFunc synchronously, RealClock asynchronously.

**Solution:** Don't rely on execution context:
```go
// Works with both implementations
var mu sync.Mutex
clock.AfterFunc(time.Second, func() {
    mu.Lock()
    defer mu.Unlock()
    // Thread-safe operation
})
```

## Migration Guide

### From time.After to clock.After

```go
// Before
select {
case <-time.After(5 * time.Second):
    handleTimeout()
}

// After
select {
case <-clock.After(5 * time.Second):
    handleTimeout()
}
```

### From time.NewTimer to clock.NewTimer

```go
// Before
timer := time.NewTimer(duration)
<-timer.C

// After  
timer := clock.NewTimer(duration)
<-timer.C() // Note: C is a method, not field
```

### From context.WithTimeout to clock.WithTimeout

```go
// Before
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)

// After
ctx, cancel := clock.WithTimeout(ctx, 30*time.Second)
```

## Contributing

Contributions welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
# Setup
git clone https://github.com/zoobzio/clockz
cd clockz

# Development
make test           # Run tests
make test-race      # Test with race detection
make lint           # Run linters
make bench          # Run benchmarks

# Before committing
make ci             # Run all checks
```

## License

MIT License - see [LICENSE](LICENSE) file for details.