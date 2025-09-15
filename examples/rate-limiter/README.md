# Rate Limiter Testing - How FlowAPI Lost Their Weekend

## The Friday Deploy Crisis

**Date**: November 8, 2024  
**Company**: FlowAPI (API Gateway Startup)  
**Impact**: 47 flaky test failures, 3 engineers debugging all weekend  
**Root Cause**: Non-deterministic rate limiter tests using time.Sleep()

## The Story

FlowAPI built a sophisticated rate limiting system for their API gateway. Multiple algorithms: sliding window, token bucket, fixed window. Everything worked perfectly in production.

The tests? Different story.

### The Timeline of Pain

**Friday 4:00 PM**: Final commit before release  
**Friday 4:15 PM**: CI pipeline fails - 3 rate limiter tests  
**Friday 4:30 PM**: Re-run CI - different tests fail  
**Friday 5:00 PM**: Added more time.Sleep() - still flaky  
**Friday 6:00 PM**: Increased sleeps to 100ms - now takes 5 minutes to run  
**Friday 8:00 PM**: Tests pass on Mac, fail on Linux CI  
**Friday 11:00 PM**: Emergency "skip these tests" commit  
**Saturday 9:00 AM**: Customer reports rate limiter not working  
**Saturday 2:00 PM**: Rolled back release  

### The Problem

Their original test code:

```go
func TestRateLimiter_SlidingWindow(t *testing.T) {
    limiter := NewSlidingWindowLimiter(10, time.Second)
    
    // Send 10 requests - should all pass
    for i := 0; i < 10; i++ {
        if !limiter.Allow() {
            t.Fatalf("Request %d should have been allowed", i)
        }
    }
    
    // 11th request should fail
    if limiter.Allow() {
        t.Fatal("11th request should have been blocked")
    }
    
    // Wait for window to slide
    time.Sleep(1100 * time.Millisecond) // The curse begins
    
    // Should allow again
    if !limiter.Allow() {
        t.Fatal("Request should be allowed after window slides")
    }
}
```

**Why it failed randomly:**
- CI runners under load: Sleep(1100ms) actually slept 900ms
- Time drift between limiter.Now() and test's time.Sleep()
- Goroutine scheduling delays
- System clock adjustments during virtualization

### The Investigation

Three engineers. One weekend. Hundreds of CI runs.

**What they tried:**
1. Increased sleep to 2 seconds → Tests now take 10 minutes
2. Added retry logic → Hides real bugs
3. Mocked time.Now() → Missed the async ticker logic
4. Disabled tests on CI → Shipped broken code

**What they discovered:**
- Sleep duration varies by ±20% on loaded CI runners
- Mac timer precision: 1ms, Linux CI: 10-100ms
- Container throttling affects time.Sleep accuracy
- Rate limiter used time.Ticker internally - couldn't mock properly

### The Solution

They found clockz on Monday morning:

```go
func TestRateLimiter_SlidingWindow_Deterministic(t *testing.T) {
    clock := clockz.NewFakeClock()
    limiter := NewSlidingWindowLimiter(10, time.Second, clock)
    
    // Send 10 requests - should all pass
    for i := 0; i < 10; i++ {
        if !limiter.Allow() {
            t.Fatalf("Request %d should have been allowed", i)
        }
    }
    
    // 11th request should fail
    if !limiter.Allow() {
        t.Fatal("11th request should have been blocked")
    }
    
    // Advance time precisely - no sleep, no flake
    clock.Advance(time.Second)
    
    // Should allow again - guaranteed
    if !limiter.Allow() {
        t.Fatal("Request should be allowed after window slides")
    }
}
```

### The Results

**Before clockz:**
- Test duration: 5-10 minutes (all the sleeps)
- Flake rate: 15% on CI
- Developer confidence: "Just re-run it"
- Weekend debugging sessions: Monthly

**After clockz:**
- Test duration: 50ms (instant time control)
- Flake rate: 0%
- Developer confidence: "Tests caught a real race condition!"
- Weekend debugging sessions: None

### The Lessons

1. **Time.Sleep() in tests is a code smell** - It's non-deterministic by design
2. **CI environments are hostile to timing** - Different OS, load, virtualization
3. **Deterministic time = Deterministic tests** - Control time, control outcomes
4. **Fast tests get run more** - 50ms vs 5 minutes changes everything

## Try It Yourself

Run the examples to see the difference:

```bash
# See the flaky original version
go test -run TestRateLimiter_Flaky -count=100

# See the deterministic clockz version  
go test -run TestRateLimiter_Deterministic -count=100
```

One will eventually fail. The other never will.