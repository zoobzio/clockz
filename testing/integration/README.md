# Integration Tests

Tests validating clockz's core value proposition: deterministic time control for reliable testing.

## Test Coverage

### determinism_test.go
Validates that FakeClock provides completely deterministic behavior across complex timing scenarios:

- **Timer Cascade**: Timers fire in exact sequence regardless of creation order
- **Pipeline Synchronization**: Multi-stage processing with precise timing control
- **Abandoned Channels**: Resilience when timer channels aren't consumed
- **Streaming Windows**: Complex time-windowing aggregation logic

Each test contrasts flaky real-time behavior with deterministic fake clock behavior.

### parent_cancellation_test.go
Ensures proper context cancellation propagation between parent and child contexts:

- **Parent cancels before timeout**: Child inherits cancellation
- **Parent cancels after timeout**: Timeout takes precedence
- **Cleanup verification**: No resource leaks after cancellation

Critical for maintaining Go's context contract when using fake clocks.

## Running Tests

```bash
# Run all integration tests
go test -v

# Run specific test file
go test -v -run TestDeterministic

# Run with race detection
go test -v -race
```

## Key Validation

These tests prove clockz's fundamental guarantee: replacing `time.Sleep()` and real timers with deterministic alternatives eliminates test flakiness while maintaining correct behavior.