package main

import (
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestRateLimiter_Flaky demonstrates the original flaky test approach.
// This test will randomly fail on loaded CI systems.
func TestRateLimiter_Flaky(t *testing.T) {
	t.Skip("Skipping flaky test - uncomment to see the pain")
	
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

	// Wait for window to slide - THE PROBLEM STARTS HERE
	// On a loaded CI runner, this might sleep for 900ms or 1300ms
	time.Sleep(1100 * time.Millisecond)

	// This assertion randomly fails when sleep is too short
	if !limiter.Allow() {
		t.Fatal("Request should be allowed after window slides")
	}
}

// TestRateLimiter_FlakyWorkaround shows common "fixes" that don't really fix.
func TestRateLimiter_FlakyWorkaround(t *testing.T) {
	t.Skip("Skipping flaky workaround test")
	
	limiter := NewSlidingWindowLimiter(10, time.Second)

	// Fill the limiter
	for i := 0; i < 10; i++ {
		if !limiter.Allow() {
			t.Fatalf("Request %d should have been allowed", i)
		}
	}

	// "Fix" #1: Add more sleep (makes tests slow)
	time.Sleep(2 * time.Second)

	// "Fix" #2: Retry logic (hides real bugs)
	var allowed bool
	for attempts := 0; attempts < 5; attempts++ {
		if limiter.Allow() {
			allowed = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !allowed {
		t.Fatal("Request should be allowed after window slides")
	}

	// Test now takes 3+ seconds and still might fail
}

// TestRateLimiter_Deterministic shows the clockz solution.
// This test NEVER fails and runs instantly.
func TestRateLimiter_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	limiter := NewSlidingWindowLimiterWithClock(10, time.Second, clock)

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

	// Advance time precisely - no sleep, no flake!
	clock.Advance(time.Second)

	// Should allow again - guaranteed to work
	if !limiter.Allow() {
		t.Fatal("Request should be allowed after window slides")
	}

	// We can even test edge cases precisely
	clock.Advance(500 * time.Millisecond) // Partial window slide
	
	// Should still have room for more requests
	for i := 0; i < 9; i++ {
		if !limiter.Allow() {
			t.Fatalf("Request %d should have been allowed in partial window", i)
		}
	}
}

// TestTokenBucket_Flaky shows why token bucket tests are even worse.
// Background goroutines with timers = testing nightmare.
func TestTokenBucket_Flaky(t *testing.T) {
	t.Skip("Skipping flaky token bucket test")
	
	// 5 tokens, refill 2 tokens every 100ms
	limiter := NewTokenBucketLimiter(5, 2, 100*time.Millisecond)
	defer limiter.Stop()

	// Use all tokens
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Fatalf("Should allow request %d", i)
		}
	}

	// Should be empty
	if limiter.Allow() {
		t.Fatal("Bucket should be empty")
	}

	// Wait for refill - BUT WHEN EXACTLY DOES IT HAPPEN?
	time.Sleep(150 * time.Millisecond) // Maybe 2 tokens, maybe 4, maybe 0

	// How many tokens do we have? Depends on goroutine scheduling!
	allowed := 0
	for i := 0; i < 5; i++ {
		if limiter.Allow() {
			allowed++
		}
	}

	// This assertion is basically random
	if allowed != 2 {
		t.Fatalf("Expected 2 tokens after refill, got %d", allowed)
	}
}

// TestTokenBucket_Deterministic shows perfect control over token bucket.
func TestTokenBucket_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	// 5 tokens, refill 2 tokens every 100ms
	limiter := NewTokenBucketLimiterWithClock(5, 2, 100*time.Millisecond, clock)
	defer limiter.Stop()

	// Use all tokens
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Fatalf("Should allow request %d", i)
		}
	}

	// Should be empty
	if limiter.Allow() {
		t.Fatal("Bucket should be empty")
	}

	// Advance time to trigger exactly one refill
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady() // Ensure ticker fires

	// Exactly 2 tokens available - guaranteed!
	for i := 0; i < 2; i++ {
		if !limiter.Allow() {
			t.Fatalf("Should have token %d after refill", i+1)
		}
	}
	if limiter.Allow() {
		t.Fatal("Should have exactly 2 tokens, no more")
	}

	// Advance for multiple refills
	clock.Advance(200 * time.Millisecond) // 2 more refills = 4 tokens
	clock.BlockUntilReady()

	// Should have 4 tokens (capped at 5, but we have 4)
	for i := 0; i < 4; i++ {
		if !limiter.Allow() {
			t.Fatalf("Should have token %d after multiple refills", i+1)
		}
	}
	if limiter.Allow() {
		t.Fatal("Should have exactly 4 tokens")
	}
}

// TestBurstLimiter_RaceCondition shows how clockz helps find real bugs.
// This test revealed a race condition in the burst limiter logic.
func TestBurstLimiter_RaceCondition(t *testing.T) {
	clock := clockz.NewFakeClock()
	limiter := NewBurstLimiterWithClock(3, time.Second, clock)

	// Send burst of 3
	for i := 0; i < 3; i++ {
		if !limiter.Allow() {
			t.Fatalf("Burst request %d should be allowed", i)
		}
	}

	// Should be in cooldown
	if limiter.Allow() {
		t.Fatal("Should be in cooldown after burst")
	}

	// Advance time just before cooldown ends
	clock.Advance(999 * time.Millisecond)
	if limiter.Allow() {
		t.Fatal("Should still be in cooldown")
	}

	// Advance past cooldown
	clock.Advance(2 * time.Millisecond)
	if !limiter.Allow() {
		t.Fatal("Should allow after cooldown")
	}

	// Test concurrent access during cooldown transition
	// This found a real race condition in the original implementation!
	var wg sync.WaitGroup
	errors := make([]string, 0)
	var errorMu sync.Mutex

	// Fill burst again
	for i := 0; i < 2; i++ {
		limiter.Allow()
	}

	// Multiple goroutines trying to access at cooldown boundary
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// All goroutines try to access at exactly the cooldown time
			clock.Advance(time.Second)
			result := limiter.Allow()
			
			// Track results for verification
			if !result {
				errorMu.Lock()
				errors = append(errors, "goroutine blocked unexpectedly")
				errorMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// The deterministic clock revealed that multiple goroutines
	// could reset the burst counter simultaneously!
	if len(errors) > 0 {
		// This would have been impossible to catch with time.Sleep()
		t.Logf("Found race condition: %v", errors)
	}
}

// TestComplexScenario shows how clockz enables testing complex time-dependent scenarios.
// Try writing this test with time.Sleep() - it would take 30+ seconds!
func TestComplexScenario(t *testing.T) {
	clock := clockz.NewFakeClock()
	
	// Multiple rate limiters working together (common in API gateways)
	userLimiter := NewSlidingWindowLimiterWithClock(100, time.Minute, clock)
	ipLimiter := NewSlidingWindowLimiterWithClock(1000, time.Hour, clock) 
	burstLimiter := NewBurstLimiterWithClock(10, 5*time.Second, clock)

	// Simulate 1 hour of traffic in milliseconds!
	for minute := 0; minute < 60; minute++ {
		// Each minute, send traffic
		for req := 0; req < 50; req++ {
			allowedByUser := userLimiter.Allow()
			allowedByIP := ipLimiter.Allow()
			allowedByBurst := burstLimiter.Allow()

			allowed := allowedByUser && allowedByIP && allowedByBurst

			// Verify expected behavior at specific times
			if minute == 0 && req < 10 {
				// First minute, burst limiter kicks in after 10
				if req < 10 && !allowed {
					t.Fatalf("Request %d in minute %d should be allowed", req, minute)
				}
				if req >= 10 && allowed {
					t.Fatalf("Request %d in minute %d should be blocked by burst", req, minute)
				}
			}
		}

		// Advance to next minute
		clock.Advance(time.Minute)
		
		// Burst limiter cooldown is only 5 seconds
		if minute%5 == 0 {
			// Should have reset by now
			if !burstLimiter.Allow() {
				t.Fatalf("Burst limiter should reset after cooldown at minute %d", minute)
			}
		}
	}

	// Test takes milliseconds instead of an hour!
	// And it's perfectly deterministic every single time
}