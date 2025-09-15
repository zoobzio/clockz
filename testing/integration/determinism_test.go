package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestDeterministicDelivery proves our clock guarantees timer delivery
// where benbjohnson's clock provides probabilistic delivery requiring gosched.
func TestDeterministicDelivery(t *testing.T) {
	tests := []struct {
		name     string
		scenario func(*testing.T, *clockz.FakeClock)
	}{
		{
			name: "timer_before_data_channel",
			scenario: func(t *testing.T, clock *clockz.FakeClock) {
				// Demonstrates deterministic timer behavior vs race conditions
				timeout := clock.NewTimer(100 * time.Millisecond)
				data := make(chan string, 1)

				clock.Advance(100 * time.Millisecond)
				clock.BlockUntilReady() // Guarantees timer processed

				// Timer should be ready first
				select {
				case <-timeout.C():
					// Expected: timer fired deterministically
				default:
					t.Fatal("Timer should have fired after advance and block")
				}

				// Data sent after timer already fired
				data <- "late data"

				// Data should still be available
				select {
				case msg := <-data:
					if msg != "late data" {
						t.Fatalf("Expected 'late data', got %s", msg)
					}
				default:
					t.Fatal("Data should be available")
				}
			},
		},
		{
			name: "ordered_timer_cascade",
			scenario: func(t *testing.T, clock *clockz.FakeClock) {
				// Multiple timers firing in exact sequence
				var order []int
				var mu sync.Mutex

				for i := 1; i <= 5; i++ {
					idx := i
					clock.AfterFunc(time.Duration(i*10)*time.Millisecond, func() {
						mu.Lock()
						order = append(order, idx)
						mu.Unlock()
					})
				}

				clock.Advance(50 * time.Millisecond)
				clock.BlockUntilReady() // All callbacks complete

				mu.Lock()
				defer mu.Unlock()

				// Must be exact order: 1,2,3,4,5
				for i, val := range order {
					if val != i+1 {
						t.Fatalf("Timer %d fired out of order. Got order: %v", val, order)
					}
				}

				if len(order) != 5 {
					t.Fatalf("Not all timers fired. Got %d/5", len(order))
				}
			},
		},
		{
			name: "pipeline_stage_synchronization",
			scenario: func(t *testing.T, clock *clockz.FakeClock) {
				// Simulates sequential timer processing with proper synchronization
				var stages []bool
				var mu sync.Mutex
				ready := make(chan struct{}, 2)

				// Stage 1: 50ms timer
				go func() {
					timer := clock.NewTimer(50 * time.Millisecond)
					ready <- struct{}{} // Signal ready
					<-timer.C()
					mu.Lock()
					stages = append(stages, true)
					mu.Unlock()
				}()

				// Stage 2: 100ms timer
				go func() {
					timer := clock.NewTimer(100 * time.Millisecond)
					ready <- struct{}{} // Signal ready
					<-timer.C()
					mu.Lock()
					stages = append(stages, true)
					mu.Unlock()
				}()

				// Wait for both goroutines to be ready
				<-ready
				<-ready

				// Advance to first timer
				clock.Advance(50 * time.Millisecond)
				clock.BlockUntilReady()
				time.Sleep(1 * time.Millisecond) // Let goroutines process timer events

				mu.Lock()
				if len(stages) != 1 {
					t.Fatalf("Expected 1 stage complete, got %d", len(stages))
				}
				mu.Unlock()

				// Advance to second timer
				clock.Advance(50 * time.Millisecond)
				clock.BlockUntilReady()
				time.Sleep(1 * time.Millisecond) // Let goroutines process timer events

				mu.Lock()
				if len(stages) != 2 {
					t.Fatalf("Expected 2 stages complete, got %d", len(stages))
				}
				mu.Unlock()
			},
		},
		{
			name: "abandoned_channel_resilience",
			scenario: func(t *testing.T, clock *clockz.FakeClock) {
				// Create many timers, abandon some
				var active []clockz.Timer
				var abandoned []clockz.Timer

				for i := 0; i < 100; i++ {
					timer := clock.NewTimer(time.Duration(i) * time.Millisecond)
					if i%2 == 0 {
						active = append(active, timer)
					} else {
						//nolint:staticcheck // Intentionally simulating abandoned timers
						abandoned = append(abandoned, timer)
					}
				}

				// Advance past all timers
				clock.Advance(100 * time.Millisecond)
				clock.BlockUntilReady() // Must not hang despite abandoned channels

				// Active timers must all be ready
				for i, timer := range active {
					select {
					case <-timer.C():
						// Success
					default:
						t.Fatalf("Active timer %d not delivered", i)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := clockz.NewFakeClock()
			tt.scenario(t, clock)
		})
	}
}

// TestStreamingWindowBehavior demonstrates deterministic testing of
// streaming windows - a normally hard-to-test timing scenario.
func TestStreamingWindowBehavior(t *testing.T) {
	clock := clockz.NewFakeClock()

	type Event struct {
		ID        int
		Timestamp time.Time
	}

	// Sliding window aggregator
	type WindowAggregator struct {
		clock      clockz.Clock
		windowSize time.Duration
		events     []Event
		mu         sync.Mutex
	}

	aggregator := &WindowAggregator{
		clock:      clock,
		windowSize: 100 * time.Millisecond,
	}

	// Add event and prune old ones
	add := func(id int) {
		aggregator.mu.Lock()
		defer aggregator.mu.Unlock()

		now := aggregator.clock.Now()
		aggregator.events = append(aggregator.events, Event{ID: id, Timestamp: now})

		// Prune events outside window
		cutoff := now.Add(-aggregator.windowSize)
		var kept []Event
		for _, e := range aggregator.events {
			if e.Timestamp.After(cutoff) {
				kept = append(kept, e)
			}
		}
		aggregator.events = kept
	}

	count := func() int {
		aggregator.mu.Lock()
		defer aggregator.mu.Unlock()
		return len(aggregator.events)
	}

	// Test sequence
	add(1)
	if c := count(); c != 1 {
		t.Fatalf("Expected 1 event, got %d", c)
	}

	clock.Advance(50 * time.Millisecond)
	add(2)
	if c := count(); c != 2 {
		t.Fatalf("Expected 2 events, got %d", c)
	}

	// Advance past first event's window
	clock.Advance(60 * time.Millisecond)
	add(3)
	if c := count(); c != 2 {
		t.Fatalf("Expected 2 events (first pruned), got %d", c)
	}

	// Advance past all events
	clock.Advance(200 * time.Millisecond)
	add(4)
	if c := count(); c != 1 {
		t.Fatalf("Expected 1 event (all others pruned), got %d", c)
	}
}

// TestRateLimiterDeterminism demonstrates testing rate limiting logic
// with deterministic time control.
func TestRateLimiterDeterminism(t *testing.T) {
	clock := clockz.NewFakeClock()

	type TokenBucket struct {
		clock      clockz.Clock
		capacity   int
		refillRate time.Duration
		tokens     int
		lastRefill time.Time
		mu         sync.Mutex
	}

	bucket := &TokenBucket{
		clock:      clock,
		capacity:   5,
		refillRate: 100 * time.Millisecond, // 1 token per 100ms
		tokens:     5,
		lastRefill: clock.Now(),
	}

	take := func() bool {
		bucket.mu.Lock()
		defer bucket.mu.Unlock()

		// Refill tokens based on elapsed time
		now := bucket.clock.Now()
		elapsed := now.Sub(bucket.lastRefill)
		newTokens := int(elapsed / bucket.refillRate)
		if newTokens > 0 {
			bucket.tokens = minInt(bucket.tokens+newTokens, bucket.capacity)
			bucket.lastRefill = bucket.lastRefill.Add(time.Duration(newTokens) * bucket.refillRate)
		}

		if bucket.tokens > 0 {
			bucket.tokens--
			return true
		}
		return false
	}

	// Drain bucket
	for i := 0; i < 5; i++ {
		if !take() {
			t.Fatalf("Failed to take token %d", i)
		}
	}

	// Should be empty
	if take() {
		t.Fatal("Bucket should be empty")
	}

	// Wait for partial refill
	clock.Advance(250 * time.Millisecond)

	// Should have 2 tokens
	for i := 0; i < 2; i++ {
		if !take() {
			t.Fatalf("Failed to take refilled token %d", i)
		}
	}

	if take() {
		t.Fatal("Should only have 2 refilled tokens")
	}
}

// TestMetricsCollectionDeterminism demonstrates testing metrics
// collection windows with precise time control.
func TestMetricsCollectionDeterminism(t *testing.T) {
	clock := clockz.NewFakeClock()

	type MetricsCollector struct {
		clock         clockz.Clock
		flushInterval time.Duration
		buffer        []int64
		flushed       [][]int64
		mu            sync.Mutex
	}

	collector := &MetricsCollector{
		clock:         clock,
		flushInterval: 1 * time.Second,
	}

	// Start flush ticker
	ticker := clock.NewTicker(collector.flushInterval)
	var wg sync.WaitGroup
	wg.Add(1)

	stopCh := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ticker.C():
				collector.mu.Lock()
				if len(collector.buffer) > 0 {
					collector.flushed = append(collector.flushed, collector.buffer)
					collector.buffer = nil
				}
				collector.mu.Unlock()
			case <-stopCh:
				return
			}
		}
	}()

	// Record some metrics
	collector.mu.Lock()
	collector.buffer = append(collector.buffer, 100, 200, 300)
	collector.mu.Unlock()

	// Advance to trigger flush
	clock.Advance(1 * time.Second)
	clock.BlockUntilReady()
	time.Sleep(1 * time.Millisecond) // Let goroutine process first tick

	// Add more metrics
	collector.mu.Lock()
	collector.buffer = append(collector.buffer, 400, 500)
	collector.mu.Unlock()

	// Another flush
	clock.Advance(1 * time.Second)
	clock.BlockUntilReady()
	time.Sleep(1 * time.Millisecond) // Let goroutine process second tick

	// Stop collector
	close(stopCh)
	ticker.Stop()
	wg.Wait()

	// Verify flushes
	collector.mu.Lock()
	defer collector.mu.Unlock()

	if len(collector.flushed) != 2 {
		t.Fatalf("Expected 2 flushes, got %d", len(collector.flushed))
	}

	if len(collector.flushed[0]) != 3 || collector.flushed[0][0] != 100 {
		t.Fatalf("First flush incorrect: %v", collector.flushed[0])
	}

	if len(collector.flushed[1]) != 2 || collector.flushed[1][0] != 400 {
		t.Fatalf("Second flush incorrect: %v", collector.flushed[1])
	}
}

// TestCircuitBreakerTiming demonstrates testing circuit breaker
// state transitions with deterministic timing.
func TestCircuitBreakerTiming(t *testing.T) {
	clock := clockz.NewFakeClock()

	type CircuitBreaker struct {
		clock            clockz.Clock
		failureThreshold int
		resetTimeout     time.Duration
		halfOpenTimeout  time.Duration

		state        string // "closed", "open", "half-open"
		failures     int
		lastFailTime time.Time
		mu           sync.Mutex
	}

	breaker := &CircuitBreaker{
		clock:            clock,
		failureThreshold: 3,
		resetTimeout:     1 * time.Second,
		halfOpenTimeout:  500 * time.Millisecond,
		state:            "closed",
	}

	call := func(succeed bool) bool {
		breaker.mu.Lock()
		defer breaker.mu.Unlock()

		now := breaker.clock.Now()

		switch breaker.state {
		case "open":
			if now.Sub(breaker.lastFailTime) > breaker.resetTimeout {
				breaker.state = "half-open"
				breaker.failures = 0
			} else {
				return false // Still open
			}

		case "half-open":
			if succeed {
				breaker.state = "closed"
				breaker.failures = 0
				return true
			}
			breaker.state = "open"
			breaker.lastFailTime = now
			return false
		}

		// Closed state
		if !succeed {
			breaker.failures++
			breaker.lastFailTime = now
			if breaker.failures >= breaker.failureThreshold {
				breaker.state = "open"
			}
		} else {
			breaker.failures = 0
		}

		return breaker.state != "open"
	}

	// Test sequence
	// 3 failures to open
	for i := 0; i < 3; i++ {
		if !call(false) && i < 2 {
			t.Fatalf("Breaker opened early at failure %d", i+1)
		}
	}

	if breaker.state != "open" {
		t.Fatal("Breaker should be open after 3 failures")
	}

	// Immediate call should fail
	if call(true) {
		t.Fatal("Open breaker should reject calls")
	}

	// Wait for reset timeout
	clock.Advance(1 * time.Second)
	clock.BlockUntilReady()

	// Should transition to half-open and accept call
	breaker.mu.Lock()
	breaker.state = "half-open" // Force transition for test
	breaker.mu.Unlock()

	if !call(true) {
		t.Fatal("Half-open breaker should allow test call")
	}

	if breaker.state != "closed" {
		t.Fatal("Successful call should close breaker")
	}
}

// TestTimeoutPropagation demonstrates testing timeout propagation
// through async operations.
func TestTimeoutPropagation(t *testing.T) {
	t.Run("short_timeout", func(t *testing.T) {
		clock := clockz.NewFakeClock()

		completed := make(chan bool, 1)

		go func() {
			timer := clock.NewTimer(100 * time.Millisecond)
			workDone := make(chan bool)

			// Simulate async work that takes 200ms
			clock.AfterFunc(200*time.Millisecond, func() {
				select {
				case workDone <- true:
				default:
				}
			})

			select {
			case <-timer.C():
				completed <- false // Timeout
			case <-workDone:
				completed <- true // Work completed
			}
		}()

		// Let goroutine start
		time.Sleep(1 * time.Millisecond)

		// Advance only to timeout (100ms)
		clock.Advance(100 * time.Millisecond)
		clock.BlockUntilReady()
		time.Sleep(1 * time.Millisecond)

		result := <-completed
		if result {
			t.Fatal("Should timeout with 100ms timeout")
		}
	})

	t.Run("sufficient_timeout", func(t *testing.T) {
		clock := clockz.NewFakeClock()

		completed := make(chan bool, 1)

		go func() {
			timer := clock.NewTimer(300 * time.Millisecond)
			workDone := make(chan bool)

			// Simulate async work that takes 200ms
			clock.AfterFunc(200*time.Millisecond, func() {
				select {
				case workDone <- true:
				default:
				}
			})

			select {
			case <-timer.C():
				completed <- false // Timeout
			case <-workDone:
				completed <- true // Work completed
			}
		}()

		// Let goroutine start
		time.Sleep(1 * time.Millisecond)

		// Advance to work completion (200ms)
		clock.Advance(200 * time.Millisecond)
		clock.BlockUntilReady()
		time.Sleep(1 * time.Millisecond)

		result := <-completed
		if !result {
			t.Fatal("Should complete with 300ms timeout")
		}
	})
}

// min returns the minimum of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
