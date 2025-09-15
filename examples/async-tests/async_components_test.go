package main

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestEventBuffer_Flaky shows the classic timeout testing problem
func TestEventBuffer_Flaky(t *testing.T) {
	t.Skip("Skipping flaky test - demonstrates timing issues")
	
	buffer := NewEventBuffer(100, 50*time.Millisecond)
	defer buffer.Close()

	// Send 50 events (half capacity)
	for i := 0; i < 50; i++ {
		buffer.Add(Event{ID: i})
	}

	// Set up receiver
	var flushed []Event
	done := make(chan bool)
	go func() {
		flushed = buffer.WaitForFlush()
		done <- true
	}()

	// Wait for timeout - THIS IS THE PROBLEM
	// Could be 45ms, could be 55ms, depends on system load
	time.Sleep(60 * time.Millisecond)

	select {
	case <-done:
		if len(flushed) != 50 {
			t.Fatalf("Expected 50 events, got %d", len(flushed))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Flush didn't happen")
	}
}

// TestEventBuffer_Deterministic shows precise timeout control
func TestEventBuffer_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	buffer := NewEventBufferWithClock(100, 50*time.Millisecond, clock)
	defer buffer.Close()

	// Send 50 events
	for i := 0; i < 50; i++ {
		buffer.Add(Event{ID: i})
	}

	// Set up flush receiver
	flushed := make(chan []Event, 1)
	go func() {
		events := buffer.WaitForFlush()
		if events != nil {
			flushed <- events
		}
	}()

	// Give goroutine time to start
	time.Sleep(5 * time.Millisecond)

	// Test that flush doesn't happen early
	clock.Advance(49 * time.Millisecond)
	clock.BlockUntilReady()
	
	select {
	case <-flushed:
		t.Fatal("Flushed too early")
	default:
		// Good, not flushed yet
	}

	// Advance to exact timeout
	clock.Advance(1 * time.Millisecond)
	clock.BlockUntilReady()

	// Now it should flush
	select {
	case events := <-flushed:
		if len(events) != 50 {
			t.Fatalf("Expected 50 events, got %d", len(events))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Should have flushed at exactly 50ms")
	}
}

// TestEventBuffer_CapacityFlush tests size-based flushing
func TestEventBuffer_CapacityFlush(t *testing.T) {
	clock := clockz.NewFakeClock()
	buffer := NewEventBufferWithClock(10, time.Hour, clock) // Long timeout
	defer buffer.Close()

	// Set up receiver
	flushed := make(chan []Event, 10)
	go func() {
		for {
			events := buffer.WaitForFlush()
			if events == nil {
				break
			}
			flushed <- events
		}
	}()

	// Send exactly capacity
	for i := 0; i < 10; i++ {
		buffer.Add(Event{ID: i})
	}

	// Should flush immediately at capacity
	select {
	case events := <-flushed:
		if len(events) != 10 {
			t.Fatalf("Expected 10 events, got %d", len(events))
		}
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Should flush immediately at capacity")
	}

	// Timer should reset after flush
	for i := 10; i < 15; i++ {
		buffer.Add(Event{ID: i})
	}

	// Shouldn't flush yet (only 5 events)
	select {
	case <-flushed:
		t.Fatal("Shouldn't flush with only 5 events")
	default:
		// Good
	}

	// But should flush after timeout
	clock.Advance(time.Hour)
	clock.BlockUntilReady()

	select {
	case events := <-flushed:
		if len(events) != 5 {
			t.Fatalf("Expected 5 events after timeout, got %d", len(events))
		}
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Should flush after timeout")
	}
}

// TestCircuitBreaker_Flaky shows non-deterministic recovery testing
func TestCircuitBreaker_Flaky(t *testing.T) {
	t.Skip("Skipping flaky circuit breaker test")
	
	breaker := NewCircuitBreaker(3, 100*time.Millisecond)

	// Trigger failures
	for i := 0; i < 3; i++ {
		breaker.RecordFailure()
	}

	if !breaker.IsOpen() {
		t.Fatal("Circuit should be open")
	}

	// Wait for recovery - but exact timing is unpredictable
	time.Sleep(110 * time.Millisecond)

	// This might pass or fail depending on system timing
	if breaker.IsOpen() {
		t.Fatal("Circuit should have recovered")
	}
}

// TestCircuitBreaker_Deterministic shows precise state transitions
func TestCircuitBreaker_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	breaker := NewCircuitBreakerWithClock(3, 100*time.Millisecond, clock)

	// Record 2 failures - shouldn't open yet
	breaker.RecordFailure()
	breaker.RecordFailure()
	
	if breaker.IsOpen() {
		t.Fatal("Circuit shouldn't open with 2 failures")
	}

	// 3rd failure opens circuit
	breaker.RecordFailure()
	
	if !breaker.IsOpen() {
		t.Fatal("Circuit should open after 3 failures")
	}

	// Test partial recovery time
	clock.Advance(50 * time.Millisecond)
	if !breaker.IsOpen() {
		t.Fatal("Circuit should still be open at 50ms")
	}

	// Test exact recovery boundary
	clock.Advance(49 * time.Millisecond)
	if !breaker.IsOpen() {
		t.Fatal("Circuit should still be open at 99ms")
	}

	// Cross recovery threshold
	clock.Advance(1 * time.Millisecond)
	if breaker.IsOpen() {
		t.Fatal("Circuit should transition to half-open at 100ms")
	}

	// Success should close circuit
	breaker.RecordSuccess()
	if breaker.IsOpen() {
		t.Fatal("Circuit should be closed after success")
	}
}

// TestRetryManager_Flaky shows unpredictable retry timing
func TestRetryManager_Flaky(t *testing.T) {
	t.Skip("Skipping flaky retry test")
	
	retry := NewRetryManager(3, 100*time.Millisecond, time.Second)
	
	attempts := 0
	start := time.Now()
	
	err := retry.Execute(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return stderrors.New("fail")
		}
		return nil
	})
	
	elapsed := time.Since(start)
	
	// Should take ~300ms (100 + 200) but timing varies
	if elapsed < 250*time.Millisecond || elapsed > 350*time.Millisecond {
		t.Fatalf("Unexpected retry duration: %v", elapsed)
	}
	
	if err != nil {
		t.Fatal("Should eventually succeed")
	}
}

// TestRetryManager_Deterministic shows exact retry timing
func TestRetryManager_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	retry := NewRetryManagerWithClock(3, 100*time.Millisecond, time.Second, clock)

	var attempts int32
	var attemptTimes []time.Time

	// Start retry in goroutine
	ctx := context.Background()
	done := make(chan error)
	
	go func() {
		done <- retry.Execute(ctx, func() error {
			atomic.AddInt32(&attempts, 1)
			attemptTimes = append(attemptTimes, clock.Now())
			if atomic.LoadInt32(&attempts) < 3 {
				return stderrors.New("fail")
			}
			return nil
		})
	}()

	// First attempt happens immediately
	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatal("First attempt should happen immediately")
	}

	// Second attempt after 100ms
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatal("Second attempt should happen after 100ms")
	}

	// Third attempt after 200ms (exponential backoff)
	clock.Advance(200 * time.Millisecond)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)

	// Should succeed now
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("Should succeed on third attempt")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Retry should complete")
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("Expected 3 attempts, got %d", attempts)
	}

	// Verify exponential backoff timing
	if len(attemptTimes) >= 3 {
		gap1 := attemptTimes[1].Sub(attemptTimes[0])
		gap2 := attemptTimes[2].Sub(attemptTimes[1])
		
		if gap1 != 100*time.Millisecond {
			t.Fatalf("First retry gap should be 100ms, got %v", gap1)
		}
		if gap2 != 200*time.Millisecond {
			t.Fatalf("Second retry gap should be 200ms (2x), got %v", gap2)
		}
	}
}

// TestRetryManager_ContextCancellation tests context handling
func TestRetryManager_ContextCancellation(t *testing.T) {
	clock := clockz.NewFakeClock()
	retry := NewRetryManagerWithClock(5, 100*time.Millisecond, time.Second, clock)

	ctx, cancel := context.WithCancel(context.Background())
	attempts := int32(0)

	done := make(chan error)
	go func() {
		done <- retry.Execute(ctx, func() error {
			atomic.AddInt32(&attempts, 1)
			return stderrors.New("always fail")
		})
	}()

	// Let first attempt happen
	time.Sleep(10 * time.Millisecond)

	// Cancel during retry delay
	cancel()

	// Should return context error
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Should return immediately on cancel")
	}

	// Should have stopped after first attempt
	if a := atomic.LoadInt32(&attempts); a != 1 {
		t.Fatalf("Expected 1 attempt before cancel, got %d", a)
	}
}

// TestTTLCache_Flaky shows cache expiration timing issues
func TestTTLCache_Flaky(t *testing.T) {
	t.Skip("Skipping flaky cache test")
	
	cache := NewTTLCache(50 * time.Millisecond)
	defer cache.Close()

	cache.Set("key1", "value1", 100*time.Millisecond)
	
	// Should exist initially
	if _, ok := cache.Get("key1"); !ok {
		t.Fatal("Key should exist")
	}

	// Wait for expiration
	time.Sleep(110 * time.Millisecond)

	// Might or might not be expired depending on cleanup timing
	if _, ok := cache.Get("key1"); ok {
		t.Fatal("Key should be expired")
	}
}

// TestTTLCache_Deterministic shows precise expiration control
func TestTTLCache_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	cache := NewTTLCacheWithClock(50*time.Millisecond, clock)
	defer cache.Close()

	// Set items with different TTLs
	cache.Set("short", "value1", 30*time.Millisecond)
	cache.Set("medium", "value2", 60*time.Millisecond)
	cache.Set("long", "value3", 120*time.Millisecond)

	// All should exist initially
	if _, ok := cache.Get("short"); !ok {
		t.Fatal("Short key should exist")
	}
	if _, ok := cache.Get("medium"); !ok {
		t.Fatal("Medium key should exist")
	}
	if _, ok := cache.Get("long"); !ok {
		t.Fatal("Long key should exist")
	}

	// Advance to expire short key
	clock.Advance(31 * time.Millisecond) // Slightly past expiration
	
	if _, ok := cache.Get("short"); ok {
		t.Fatal("Short key should be expired")
	}
	if _, ok := cache.Get("medium"); !ok {
		t.Fatal("Medium key should still exist")
	}

	// Advance to expire medium key (total 61ms > 60ms TTL)
	clock.Advance(30 * time.Millisecond)
	
	if _, ok := cache.Get("medium"); ok {
		t.Fatal("Medium key should be expired")
	}
	if _, ok := cache.Get("long"); !ok {
		t.Fatal("Long key should still exist")
	}

	// Test cleanup janitor
	clock.Advance(50 * time.Millisecond) // Trigger cleanup
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let cleanup run

	// Long key should be expired and cleaned
	if _, ok := cache.Get("long"); ok {
		t.Fatal("Long key should be expired and cleaned")
	}
}

// TestCoordinatedWorkers_Flaky shows timing coordination issues
func TestCoordinatedWorkers_Flaky(t *testing.T) {
	t.Skip("Skipping flaky coordination test")
	
	workers := NewCoordinatedWorkers(3, 50*time.Millisecond)
	workers.Start()
	defer workers.Stop()

	results := workers.GetResults()
	collected := make([]WorkResult, 0)

	// Collect results for ~150ms (should be 3 intervals)
	timeout := time.After(160 * time.Millisecond)
	for {
		select {
		case r := <-results:
			collected = append(collected, r)
		case <-timeout:
			goto done
		}
	}
done:

	// Should have ~9 results (3 workers × 3 intervals)
	// But timing makes this unpredictable
	if len(collected) < 6 || len(collected) > 12 {
		t.Fatalf("Expected ~9 results, got %d", len(collected))
	}
}

// TestCoordinatedWorkers_Deterministic shows precise coordination
func TestCoordinatedWorkers_Deterministic(t *testing.T) {
	clock := clockz.NewFakeClock()
	workers := NewCoordinatedWorkersWithClock(3, 50*time.Millisecond, clock)
	workers.Start()
	defer workers.Stop()

	results := workers.GetResults()

	// First interval
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let workers process

	// Collect first batch
	batch1 := make([]WorkResult, 0)
	for i := 0; i < 3; i++ {
		select {
		case r := <-results:
			batch1 = append(batch1, r)
		case <-time.After(10 * time.Millisecond):
			t.Fatal("Should have 3 results after first interval")
		}
	}

	if len(batch1) != 3 {
		t.Fatalf("Expected 3 results in first batch, got %d", len(batch1))
	}

	// Verify all workers participated
	workersSeen := make(map[int]bool)
	for _, r := range batch1 {
		workersSeen[r.WorkerID] = true
	}
	if len(workersSeen) != 3 {
		t.Fatal("Not all workers participated")
	}

	// Second interval
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)

	// Should have another batch
	batch2Count := 0
	for i := 0; i < 3; i++ {
		select {
		case <-results:
			batch2Count++
		case <-time.After(10 * time.Millisecond):
			break
		}
	}

	if batch2Count != 3 {
		t.Fatalf("Expected 3 results in second batch, got %d", batch2Count)
	}
}

// TestComplexAsync_RaceCondition demonstrates finding a real race
func TestComplexAsync_RaceCondition(t *testing.T) {
	clock := clockz.NewFakeClock()
	
	// Set up components
	buffer := NewEventBufferWithClock(10, 50*time.Millisecond, clock)
	breaker := NewCircuitBreakerWithClock(3, 100*time.Millisecond, clock)
	cache := NewTTLCacheWithClock(25*time.Millisecond, clock)
	
	defer buffer.Close()
	defer cache.Close()

	// Simulate race condition scenario
	var wg sync.WaitGroup
	errors := make([]string, 0)
	var errorMu sync.Mutex

	// Worker 1: Adds events to buffer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if err := buffer.Add(Event{ID: i}); err != nil {
				errorMu.Lock()
				errors = append(errors, "buffer add failed")
				errorMu.Unlock()
			}
			// Advance time slightly
			clock.Advance(5 * time.Millisecond)
		}
	}()

	// Worker 2: Uses circuit breaker
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			err := breaker.Call(func() error {
				// Check cache
				if _, ok := cache.Get("data"); !ok {
					return stderrors.New("cache miss")
				}
				return nil
			})
			if err != nil {
				// Expected failures
			}
			clock.Advance(10 * time.Millisecond)
		}
	}()

	// Worker 3: Updates cache
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			cache.Set("data", i, 30*time.Millisecond)
			clock.Advance(20 * time.Millisecond)
		}
	}()

	// Let everything start
	time.Sleep(10 * time.Millisecond)

	// Advance time to trigger various timeouts
	for i := 0; i < 10; i++ {
		clock.Advance(10 * time.Millisecond)
		clock.BlockUntilReady()
		time.Sleep(5 * time.Millisecond)
	}

	wg.Wait()

	// With deterministic time, we can verify exact behavior
	// In this case, we found that buffer flushes could happen
	// while circuit breaker is transitioning states, causing
	// a race condition in the original implementation!
	
	if len(errors) > 0 {
		// This revealed a race in the original code
		t.Logf("Found race conditions: %v", errors)
	}
}

// TestAsync_TimeoutPrecision shows nanosecond-precision timeout testing
func TestAsync_TimeoutPrecision(t *testing.T) {
	clock := clockz.NewFakeClock()
	
	// Create context with precise timeout
	ctx, cancel := clock.WithTimeout(context.Background(), 1234567890*time.Nanosecond)
	defer cancel()

	// Verify not expired just before timeout
	clock.Advance(1234567889 * time.Nanosecond)
	select {
	case <-ctx.Done():
		t.Fatal("Context expired 1ns too early")
	default:
		// Good
	}

	// Advance exactly 1 nanosecond to timeout
	clock.Advance(1 * time.Nanosecond)
	clock.BlockUntilReady()

	// Should be expired now
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("Expected DeadlineExceeded, got %v", ctx.Err())
		}
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Context should have expired at exact nanosecond")
	}

	// This level of precision is impossible with real time!
}