# Clockz Examples - Developer War Stories

## Real Problems, Real Solutions

These aren't tutorials. They're war stories from the trenches of async testing.

Each example follows the same pattern:
1. **The Crisis** - What went wrong, when, and how bad it got
2. **The Investigation** - Hours/days/weeks of debugging
3. **The Solution** - How clockz fixed it
4. **The Results** - Before vs after metrics
5. **The Lessons** - What we learned the hard way

## The Stories

### 🚦 [Rate Limiter Testing](./rate-limiter/)
**The Friday Deploy Crisis**
- Company: FlowAPI
- Impact: 47 flaky tests, 3 engineers, 1 weekend
- Problem: Rate limiter tests using time.Sleep()
- Solution: Deterministic time control with clockz

*"The tests passed on Mac, failed on Linux CI. We added more sleep. Now they take 5 minutes and still fail."*

### ⏰ [Scheduled Jobs](./scheduled-jobs/)
**The 3 AM Debug Session**
- Company: DataSync Inc
- Impact: 6 hours debugging, tests only fail at midnight
- Problem: Scheduler tests assuming real-time behavior
- Solution: Control time instead of waiting for it

*"The test literally only passed if you ran it at midnight. We stayed up until 3 AM to figure that out."*

### 🔄 [Async Components](./async-tests/)
**The Concurrency Catastrophe**
- Company: StreamFlow
- Impact: 2 weeks hunting phantom bugs
- Problem: Testing concurrent timers with sleep and prayers
- Solution: Deterministic async behavior with clockz

*"1 in 50 test runs failed. Different test each time. Adding logs made the bug disappear."*

## The Pattern

Every flaky test follows the same pattern:

```go
// The Original Sin
func TestSomethingAsync(t *testing.T) {
    component := NewComponent()
    component.DoAsync()
    
    time.Sleep(100 * time.Millisecond) // 🙏 Please be enough
    
    // Sometimes passes, sometimes fails
    assert.Equal(t, expected, component.State())
}

// The Solution
func TestSomethingDeterministic(t *testing.T) {
    clock := clockz.NewFakeClock()
    component := NewComponentWithClock(clock)
    component.DoAsync()
    
    clock.Advance(100 * time.Millisecond) // ✅ Exactly 100ms
    
    // Always passes
    assert.Equal(t, expected, component.State())
}
```

## The Numbers

Aggregate metrics from all examples:

**Before clockz:**
- Test duration: 2-10 minutes
- Flake rate: 2-15%
- Debug time: Hours to weeks
- CI confidence: "Just re-run it"

**After clockz:**
- Test duration: 50ms-3s
- Flake rate: 0%
- Debug time: Tests catch bugs immediately
- CI confidence: "Tests saved us from production issues"

## Common Problems Solved

1. **Timer Coordination** - Multiple timers need precise sequencing
2. **Timeout Testing** - Verify behavior at exact timeout boundaries
3. **Rate Limiting** - Test limits without waiting for time windows
4. **Scheduled Jobs** - Test cron jobs without waiting for midnight
5. **Retry Logic** - Verify exponential backoff without the wait
6. **Circuit Breakers** - Test state transitions precisely
7. **Cache Expiration** - Control TTL behavior deterministically
8. **Event Buffering** - Test time-based flushing instantly

## How to Run

Each example includes both flaky and deterministic versions:

```bash
# See the pain (flaky tests)
cd rate-limiter
go test -run Flaky -count=100

# See the solution (deterministic tests)
go test -run Deterministic -count=1000

# Run all examples
for dir in */; do
    echo "Testing $dir"
    (cd "$dir" && go test -v)
done
```

## The Universal Truth

> "Every time.Sleep() in a test is a bug waiting to happen."

These examples prove it. Learn from our pain. Use clockz.

## Contributing

Have your own testing war story? We'd love to hear it:
1. Follow the pattern: Crisis → Investigation → Solution → Results → Lessons
2. Include both flaky and fixed versions
3. Add real metrics (before/after)
4. Share the pain points that resonate with developers

Remember: We're not writing documentation. We're sharing scars.