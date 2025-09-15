package clockz

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestFakeClock_DirectSynchronization(t *testing.T) {
	t.Run("single timer delivery", func(t *testing.T) {
		clock := NewFakeClock()
		timer := clock.NewTimer(100 * time.Millisecond)

		// Advance and synchronize
		clock.Advance(100 * time.Millisecond)
		clock.BlockUntilReady()

		// Timer should be ready immediately
		select {
		case <-timer.C():
			// Success - timer delivered
		default:
			t.Fatal("Timer not delivered after BlockUntilReady()")
		}
	})

	t.Run("multiple timer delivery", func(t *testing.T) {
		clock := NewFakeClock()
		timer1 := clock.NewTimer(50 * time.Millisecond)
		timer2 := clock.NewTimer(100 * time.Millisecond)

		clock.Advance(100 * time.Millisecond)
		clock.BlockUntilReady()

		// Both timers should be ready
		select {
		case <-timer1.C():
		default:
			t.Fatal("Timer1 not delivered")
		}

		select {
		case <-timer2.C():
		default:
			t.Fatal("Timer2 not delivered")
		}
	})

	t.Run("ticker multiple ticks", func(t *testing.T) {
		clock := NewFakeClock()
		ticker := clock.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		// Advance step by step to ensure ticks are processed properly
		var ticks []time.Time

		// First tick
		clock.Advance(50 * time.Millisecond)
		clock.BlockUntilReady()
		select {
		case tick := <-ticker.C():
			ticks = append(ticks, tick)
		default:
			t.Fatal("First tick not delivered")
		}

		// Second tick
		clock.Advance(50 * time.Millisecond)
		clock.BlockUntilReady()
		select {
		case tick := <-ticker.C():
			ticks = append(ticks, tick)
		default:
			t.Fatal("Second tick not delivered")
		}

		// Third tick
		clock.Advance(50 * time.Millisecond)
		clock.BlockUntilReady()
		select {
		case tick := <-ticker.C():
			ticks = append(ticks, tick)
		default:
			t.Fatal("Third tick not delivered")
		}

		if len(ticks) != 3 {
			t.Fatalf("Expected 3 ticks, got %d", len(ticks))
		}
	})
}

func TestFakeClock_ResourceSafety(t *testing.T) {
	t.Run("many timers no resource exhaustion", func(t *testing.T) {
		clock := NewFakeClock()

		// Create substantial number of timers
		const timerCount = 1000
		var timers []Timer
		for i := 0; i < timerCount; i++ {
			timers = append(timers, clock.NewTimer(time.Duration(i)*time.Microsecond))
		}

		// This should not cause resource exhaustion
		start := time.Now()
		clock.Advance(time.Millisecond)
		clock.BlockUntilReady()

		if time.Since(start) > time.Second {
			t.Fatal("Processing took too long - resource issue")
		}

		// Verify all timers delivered
		delivered := 0
		for _, timer := range timers {
			select {
			case <-timer.C():
				delivered++
			default:
				// Expected for some timers not yet expired
			}
		}

		if delivered == 0 {
			t.Fatal("No timers delivered")
		}
	})

	t.Run("abandoned channel handling", func(t *testing.T) {
		clock := NewFakeClock()
		timer := clock.NewTimer(10 * time.Millisecond)

		// Advance but don't read from timer channel
		clock.Advance(10 * time.Millisecond)

		// BlockUntilReady should not hang
		done := make(chan bool, 1)
		go func() {
			clock.BlockUntilReady()
			done <- true
		}()

		select {
		case <-done:
			// Success - no hang
		case <-time.After(2 * time.Second):
			t.Fatal("BlockUntilReady() hung on abandoned channel")
		}

		// Timer value should still be deliverable
		select {
		case <-timer.C():
			// Success - value preserved despite abandonment
		default:
			t.Error("Timer value lost")
		}
	})
}

func TestFakeClock_ConcurrentAccess(t *testing.T) {
	clock := NewFakeClock()

	// Multiple goroutines creating and using timers
	const workers = 50
	var wg sync.WaitGroup
	errors := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			timer := clock.NewTimer(time.Duration(id) * time.Microsecond)

			// Each worker advances by different amounts
			clock.Advance(time.Duration(id+1) * time.Microsecond)
			clock.BlockUntilReady()

			// Verify timer behavior
			select {
			case <-timer.C():
				// Success
			case <-time.After(time.Second):
				errors <- fmt.Errorf("worker %d: timer timeout", id)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any worker errors
	for err := range errors {
		t.Error(err)
	}
}

func TestFakeClock_NoRaceCondition(t *testing.T) {
	// Stress test the exact throttle test sequence
	for run := 0; run < 100; run++ {
		t.Run(fmt.Sprintf("iteration_%d", run), func(t *testing.T) {
			clock := NewFakeClock()
			timer := clock.NewTimer(50 * time.Millisecond)

			// The exact sequence that was racing
			clock.Advance(50 * time.Millisecond)
			clock.BlockUntilReady() // Must guarantee timer processed

			// This must never hang or race
			select {
			case <-timer.C():
				// Success
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Run %d: Timer not delivered - race condition", run)
			}
		})
	}
}

func TestFakeClock_AfterFunc_StillWorks(t *testing.T) {
	clock := NewFakeClock()
	executed := false

	clock.AfterFunc(100*time.Millisecond, func() {
		executed = true
	})

	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()

	if !executed {
		t.Fatal("AfterFunc not executed after BlockUntilReady")
	}
}

func BenchmarkFakeClock_DirectSynchronization(b *testing.B) {
	clock := NewFakeClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := clock.NewTimer(time.Microsecond)
		clock.Advance(time.Microsecond)
		clock.BlockUntilReady()
		<-timer.C()
	}
}

// Compare with baseline (before fix).
func BenchmarkFakeClock_Baseline(b *testing.B) {
	clock := NewFakeClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := clock.NewTimer(time.Microsecond)
		clock.Advance(time.Microsecond)
		// No BlockUntilReady - use non-blocking select to avoid deadlock
		select {
		case <-timer.C():
			// Timer delivered successfully
		default:
			// Timer not ready - baseline behavior without synchronization
			b.Logf("Timer not ready on iteration %d", i)
		}
	}
}

// TestFakeClock_Sleep validates Sleep blocks until time advances.
func TestFakeClock_Sleep(t *testing.T) {
	clock := NewFakeClock()
	duration := 100 * time.Millisecond

	// Sleep should block until clock advances
	sleepDone := make(chan bool)
	go func() {
		clock.Sleep(duration)
		close(sleepDone)
	}()

	// Sleep should block initially
	select {
	case <-sleepDone:
		t.Fatal("Sleep returned before clock advanced")
	case <-time.After(50 * time.Millisecond):
		// Expected - sleep is blocked
	}

	// Advance clock by duration
	clock.Advance(duration)
	clock.BlockUntilReady()

	// Sleep should now complete
	select {
	case <-sleepDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Sleep did not complete after clock advance")
	}
}

// TestFakeClock_Since validates Since returns correct elapsed duration.
func TestFakeClock_Since(t *testing.T) {
	startTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClockAt(startTime)

	// Record start time
	start := clock.Now()

	// Advance clock
	advancement := 5 * time.Minute
	clock.Advance(advancement)

	// Since should return the advancement
	elapsed := clock.Since(start)
	if elapsed != advancement {
		t.Errorf("Since returned %v, expected %v", elapsed, advancement)
	}
}

// TestFakeClock_Since_PastTime validates Since with time before clock start.
func TestFakeClock_Since_PastTime(t *testing.T) {
	startTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClockAt(startTime)

	// Time one hour before clock start
	pastTime := startTime.Add(-time.Hour)

	elapsed := clock.Since(pastTime)
	expected := time.Hour

	if elapsed != expected {
		t.Errorf("Since returned %v, expected %v", elapsed, expected)
	}
}

// TestFakeClock_Since_FutureTime validates Since with future time returns negative.
func TestFakeClock_Since_FutureTime(t *testing.T) {
	startTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClockAt(startTime)

	// Time one hour in the future
	futureTime := startTime.Add(time.Hour)

	elapsed := clock.Since(futureTime)
	expected := -time.Hour

	if elapsed != expected {
		t.Errorf("Since returned %v, expected %v", elapsed, expected)
	}
}

// TestFakeClock_Sleep_PartialAdvancement validates Sleep behavior with partial time advancement.
func TestFakeClock_Sleep_PartialAdvancement(t *testing.T) {
	clock := NewFakeClock()
	duration := 100 * time.Millisecond

	sleepDone := make(chan bool)
	go func() {
		clock.Sleep(duration)
		close(sleepDone)
	}()

	// Give Sleep time to register its After() call
	time.Sleep(10 * time.Millisecond)

	// Advance by half the duration
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady()

	// Sleep should still be blocked
	select {
	case <-sleepDone:
		t.Fatal("Sleep completed with partial advancement")
	case <-time.After(50 * time.Millisecond):
		// Expected - sleep still blocked
	}

	// Advance by remaining duration
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady()

	// Sleep should now complete
	select {
	case <-sleepDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Sleep did not complete after full advancement")
	}
}

func TestFakeClock_WithTimeout_Basic(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create timeout context
	ctx, cancel := clock.WithTimeout(parent, 5*time.Second)
	defer cancel()

	// Should not be canceled initially
	select {
	case <-ctx.Done():
		t.Fatal("context canceled prematurely")
	default:
	}

	// Should have correct deadline
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		t.Fatal("expected deadline to be set")
	}
	expectedDeadline := time.Unix(1005, 0)
	if !deadline.Equal(expectedDeadline) {
		t.Fatalf("expected deadline %v, got %v", expectedDeadline, deadline)
	}

	// Advance time past deadline
	clock.Advance(6 * time.Second)
	clock.BlockUntilReady()

	// Should be canceled with deadline exceeded
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled")
	}
}

func TestFakeClock_WithTimeout_ManualCancel(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create timeout context
	ctx, cancel := clock.WithTimeout(parent, 5*time.Second)

	// Cancel manually before timeout
	cancel()

	// Should be canceled with Canceled error
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.Canceled {
			t.Fatalf("expected Canceled, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled")
	}

	// Advancing time should not affect already canceled context
	clock.Advance(10 * time.Second)
	clock.BlockUntilReady()

	// Error should still be Canceled, not DeadlineExceeded
	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("expected Canceled after time advance, got %v", err)
	}
}

func TestFakeClock_WithDeadline_Basic(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()
	deadline := time.Unix(1003, 0)

	// Create deadline context
	ctx, cancel := clock.WithDeadline(parent, deadline)
	defer cancel()

	// Should not be canceled initially
	select {
	case <-ctx.Done():
		t.Fatal("context canceled prematurely")
	default:
	}

	// Should have correct deadline
	ctxDeadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		t.Fatal("expected deadline to be set")
	}
	if !ctxDeadline.Equal(deadline) {
		t.Fatalf("expected deadline %v, got %v", deadline, ctxDeadline)
	}

	// Advance time to deadline
	clock.SetTime(deadline)
	clock.BlockUntilReady()

	// Should be canceled with deadline exceeded
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled")
	}
}

func TestFakeClock_WithDeadline_PastDeadline(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()
	pastDeadline := time.Unix(999, 0) // Before current time

	// Create deadline context with past deadline
	ctx, cancel := clock.WithDeadline(parent, pastDeadline)
	defer cancel()

	// Should be canceled immediately
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled immediately")
	}
}

func TestFakeClock_WithTimeout_ParentAlreadyCancelled(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))

	// Create already canceled parent context
	parent, parentCancel := context.WithCancel(context.Background())
	parentCancel()

	// Create timeout context with canceled parent
	ctx, cancel := clock.WithTimeout(parent, 5*time.Second)
	defer cancel()

	// Should inherit parent's cancellation
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.Canceled {
			t.Fatalf("expected Canceled from parent, got %v", err)
		}
	default:
		t.Fatal("expected context to inherit parent cancellation")
	}
}

func TestFakeClock_WithTimeout_ValuePropagation(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))

	// Create parent context with value
	type keyType string
	const testKey keyType = "test"
	parent := context.WithValue(context.Background(), testKey, "test-value")

	// Create timeout context
	ctx, cancel := clock.WithTimeout(parent, 5*time.Second)
	defer cancel()

	// Should propagate parent's value
	if value := ctx.Value(testKey); value != "test-value" {
		t.Fatalf("expected 'test-value', got %v", value)
	}
}

func TestFakeClock_WithTimeout_ZeroTimeout(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create timeout context with zero timeout
	ctx, cancel := clock.WithTimeout(parent, 0)
	defer cancel()

	// Should be canceled immediately
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled immediately")
	}
}

func TestFakeClock_WithTimeout_NegativeTimeout(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create timeout context with negative timeout
	ctx, cancel := clock.WithTimeout(parent, -1*time.Second)
	defer cancel()

	// Should be canceled immediately
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled immediately")
	}
}

func TestFakeClock_WithTimeout_ConcurrentOperations(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	var wg sync.WaitGroup
	results := make([]error, 10)

	// Create multiple timeout contexts concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := clock.WithTimeout(parent, 2*time.Second)
			defer cancel()

			// Wait for cancellation
			<-ctx.Done()
			results[idx] = ctx.Err()
		}(i)
	}

	// Advance time to trigger all timeouts
	time.Sleep(10 * time.Millisecond) // Allow goroutines to start
	clock.Advance(3 * time.Second)
	clock.BlockUntilReady()

	wg.Wait()

	// All contexts should have timed out
	for i, err := range results {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("context %d: expected DeadlineExceeded, got %v", i, err)
		}
	}
}

func TestFakeClock_WithTimeout_ConcurrentCancelAndTimeout(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create timeout context
	ctx, cancel := clock.WithTimeout(parent, 1*time.Second)

	var wg sync.WaitGroup
	wg.Add(2)

	// Start goroutine to cancel manually
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	// Start goroutine to advance time
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		clock.Advance(2 * time.Second)
		clock.BlockUntilReady()
	}()

	wg.Wait()

	// Context should be canceled (either Canceled or DeadlineExceeded is valid)
	select {
	case <-ctx.Done():
		err := ctx.Err()
		if err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("expected Canceled or DeadlineExceeded, got %v", err)
		}
	default:
		t.Fatal("expected context to be canceled")
	}
}

func TestFakeClock_WithTimeout_HasWaiters(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Initially no waiters
	if clock.HasWaiters() {
		t.Fatal("expected no waiters initially")
	}

	// Create timeout context
	_, cancel := clock.WithTimeout(parent, 5*time.Second)
	defer cancel()

	// Should have waiters now
	if !clock.HasWaiters() {
		t.Fatal("expected waiters after creating timeout context")
	}

	// Cancel manually
	cancel()

	// Brief wait for cleanup
	time.Sleep(1 * time.Millisecond)

	// Should have no waiters after cancellation
	if clock.HasWaiters() {
		t.Fatal("expected no waiters after manual cancellation")
	}
}

func TestFakeClock_WithTimeout_ResourceCleanup(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create many timeout contexts and cancel them manually
	for i := 0; i < 100; i++ {
		ctx, cancel := clock.WithTimeout(parent, 1*time.Hour)
		cancel()
		_ = ctx // Use ctx to prevent compiler optimization
	}

	// Brief wait for cleanup
	time.Sleep(10 * time.Millisecond)

	// Should have no waiters after all manual cancellations
	if clock.HasWaiters() {
		t.Fatal("expected no waiters after mass cancellation")
	}

	// Create contexts and let them timeout
	cancels := make([]context.CancelFunc, 10)
	for i := 0; i < 10; i++ {
		_, cancel := clock.WithTimeout(parent, 1*time.Second)
		cancels[i] = cancel
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	// Advance time to trigger timeouts
	clock.Advance(2 * time.Second)
	clock.BlockUntilReady()

	// Should have no waiters after timeouts
	if clock.HasWaiters() {
		t.Fatal("expected no waiters after timeout cleanup")
	}
}

func TestFakeClock_WithTimeout_MultipleDeadlines(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))
	parent := context.Background()

	// Create contexts with different deadlines
	ctx1, cancel1 := clock.WithTimeout(parent, 1*time.Second)
	defer cancel1()
	ctx2, cancel2 := clock.WithTimeout(parent, 2*time.Second)
	defer cancel2()
	ctx3, cancel3 := clock.WithTimeout(parent, 3*time.Second)
	defer cancel3()

	// Advance to first deadline
	clock.Advance(1 * time.Second)
	clock.BlockUntilReady()

	// Only first context should be canceled
	select {
	case <-ctx1.Done():
		if ctx1.Err() != context.DeadlineExceeded {
			t.Fatalf("ctx1: expected DeadlineExceeded, got %v", ctx1.Err())
		}
	default:
		t.Fatal("expected ctx1 to be canceled")
	}

	select {
	case <-ctx2.Done():
		t.Fatal("ctx2 should not be canceled yet")
	default:
	}

	select {
	case <-ctx3.Done():
		t.Fatal("ctx3 should not be canceled yet")
	default:
	}

	// Advance to second deadline
	clock.Advance(1 * time.Second) // Total: 2 seconds
	clock.BlockUntilReady()

	// Second context should now be canceled
	select {
	case <-ctx2.Done():
		if ctx2.Err() != context.DeadlineExceeded {
			t.Fatalf("ctx2: expected DeadlineExceeded, got %v", ctx2.Err())
		}
	default:
		t.Fatal("expected ctx2 to be canceled")
	}

	select {
	case <-ctx3.Done():
		t.Fatal("ctx3 should not be canceled yet")
	default:
	}
}

func TestFakeClock_WithTimeout_ParentCancellationPropagation(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))

	// Create parent context with cancel function
	parent, parentCancel := context.WithCancel(context.Background())

	// Create child timeout context with long timeout
	child, childCancel := clock.WithTimeout(parent, 1*time.Hour)
	defer childCancel()

	// Child should not be canceled initially
	select {
	case <-child.Done():
		t.Fatal("child context canceled prematurely")
	default:
	}

	// Cancel parent context
	parentCancel()

	// Brief wait for parent cancellation to propagate
	time.Sleep(10 * time.Millisecond)

	// Child should now be canceled due to parent cancellation
	select {
	case <-child.Done():
		if err := child.Err(); err != context.Canceled {
			t.Fatalf("expected Canceled from parent, got %v", err)
		}
	default:
		t.Fatal("expected child context to be canceled when parent cancels")
	}

	// Advancing clock should not change the cancellation reason
	clock.Advance(2 * time.Hour)
	clock.BlockUntilReady()

	// Error should still be from parent cancellation, not deadline
	if err := child.Err(); err != context.Canceled {
		t.Fatalf("expected Canceled after time advance, got %v", err)
	}
}

func TestFakeClock_WithTimeout_ParentCancellationRace(t *testing.T) {
	clock := NewFakeClockAt(time.Unix(1000, 0))

	// Test multiple parent cancellations happening concurrently
	for i := 0; i < 10; i++ {
		// Create fresh parent for each iteration
		parent, parentCancel := context.WithCancel(context.Background())
		child, childCancel := clock.WithTimeout(parent, 1*time.Hour)

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: Cancel parent
		go func() {
			defer wg.Done()
			parentCancel()
		}()

		// Goroutine 2: Check child cancellation
		var childErr error
		go func() {
			defer wg.Done()
			<-child.Done()
			childErr = child.Err()
		}()

		wg.Wait()

		// Child should be canceled with parent's error
		if childErr != context.Canceled {
			t.Fatalf("iteration %d: expected Canceled, got %v", i, childErr)
		}

		childCancel()
	}
}
