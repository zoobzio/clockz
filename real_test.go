package clockz

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestRealClock_Now validates that Now returns current time within reasonable bounds.
func TestRealClock_Now(t *testing.T) {
	clock := RealClock

	before := time.Now()
	actual := clock.Now()
	after := time.Now()

	if actual.Before(before) || actual.After(after) {
		t.Errorf("Now() returned %v, expected between %v and %v", actual, before, after)
	}
}

// TestRealClock_After validates After channel delivers after specified duration.
func TestRealClock_After(t *testing.T) {
	clock := RealClock
	duration := 50 * time.Millisecond

	start := time.Now()
	ch := clock.After(duration)

	select {
	case received := <-ch:
		elapsed := time.Since(start)

		// Allow for reasonable timing variance in CI environments
		if elapsed < duration || elapsed > duration+100*time.Millisecond {
			t.Errorf("After took %v, expected around %v", elapsed, duration)
		}

		// Time on channel should be approximately when it fired
		expectedTime := start.Add(duration)
		if received.Sub(expectedTime).Abs() > 50*time.Millisecond {
			t.Errorf("Received time %v, expected around %v", received, expectedTime)
		}

	case <-time.After(200 * time.Millisecond):
		t.Fatal("After channel did not deliver within timeout")
	}
}

// TestRealClock_AfterFunc validates AfterFunc executes function after duration.
func TestRealClock_AfterFunc(t *testing.T) {
	clock := RealClock
	duration := 50 * time.Millisecond

	executed := false
	var mu sync.Mutex

	start := time.Now()
	timer := clock.AfterFunc(duration, func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	})

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	wasExecuted := executed
	mu.Unlock()

	if !wasExecuted {
		t.Fatal("AfterFunc function was not executed")
	}

	elapsed := time.Since(start)
	if elapsed < duration {
		t.Errorf("Function executed too early: %v < %v", elapsed, duration)
	}

	// Timer should be valid
	if timer == nil {
		t.Fatal("AfterFunc returned nil timer")
	}
}

// TestRealClock_AfterFunc_Stop validates Timer.Stop prevents execution.
func TestRealClock_AfterFunc_Stop(t *testing.T) {
	clock := RealClock
	duration := 100 * time.Millisecond

	executed := false
	var mu sync.Mutex

	timer := clock.AfterFunc(duration, func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	})

	// Stop timer before it fires
	stopped := timer.Stop()
	if !stopped {
		t.Fatal("Stop() returned false for active timer")
	}

	// Wait longer than duration to ensure it would have fired
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	wasExecuted := executed
	mu.Unlock()

	if wasExecuted {
		t.Fatal("Function executed despite Stop()")
	}
}

// TestRealClock_NewTimer validates timer creation and basic operation.
func TestRealClock_NewTimer(t *testing.T) {
	clock := RealClock
	duration := 50 * time.Millisecond

	start := time.Now()
	timer := clock.NewTimer(duration)

	if timer == nil {
		t.Fatal("NewTimer returned nil")
	}

	select {
	case received := <-timer.C():
		elapsed := time.Since(start)

		if elapsed < duration || elapsed > duration+100*time.Millisecond {
			t.Errorf("Timer fired after %v, expected around %v", elapsed, duration)
		}

		// Time on channel should be approximately when it fired
		expectedTime := start.Add(duration)
		if received.Sub(expectedTime).Abs() > 50*time.Millisecond {
			t.Errorf("Timer sent %v, expected around %v", received, expectedTime)
		}

	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timer did not fire within timeout")
	}
}

// TestRealClock_Timer_Stop validates Timer.Stop functionality.
func TestRealClock_Timer_Stop(t *testing.T) {
	clock := RealClock
	duration := 100 * time.Millisecond

	timer := clock.NewTimer(duration)

	// Stop timer before it fires
	stopped := timer.Stop()
	if !stopped {
		t.Fatal("Stop() returned false for active timer")
	}

	// Timer should not fire
	select {
	case <-timer.C():
		t.Fatal("Timer fired despite Stop()")
	case <-time.After(150 * time.Millisecond):
		// Expected - timer was stopped
	}

	// Stopping already stopped timer should return false
	stopped = timer.Stop()
	if stopped {
		t.Error("Stop() returned true for already stopped timer")
	}
}

// TestRealClock_Timer_Reset validates Timer.Reset functionality.
func TestRealClock_Timer_Reset(t *testing.T) {
	clock := RealClock
	initialDuration := 100 * time.Millisecond
	resetDuration := 50 * time.Millisecond

	timer := clock.NewTimer(initialDuration)

	// Reset timer to shorter duration
	time.Sleep(25 * time.Millisecond) // Wait a bit before reset
	start := time.Now()
	wasActive := timer.Reset(resetDuration)
	if !wasActive {
		t.Fatal("Reset() returned false for active timer")
	}

	select {
	case <-timer.C():
		elapsed := time.Since(start)

		if elapsed < resetDuration || elapsed > resetDuration+50*time.Millisecond {
			t.Errorf("Reset timer fired after %v, expected around %v", elapsed, resetDuration)
		}

	case <-time.After(150 * time.Millisecond):
		t.Fatal("Reset timer did not fire within timeout")
	}
}

// TestRealClock_NewTicker validates ticker creation and operation.
func TestRealClock_NewTicker(t *testing.T) {
	clock := RealClock
	interval := 50 * time.Millisecond

	ticker := clock.NewTicker(interval)
	if ticker == nil {
		t.Fatal("NewTicker returned nil")
	}
	defer ticker.Stop()

	start := time.Now()
	var ticks []time.Time

	// Collect first 3 ticks
	for i := 0; i < 3; i++ {
		select {
		case tick := <-ticker.C():
			ticks = append(ticks, tick)
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("Tick %d did not arrive within timeout", i+1)
		}
	}

	totalElapsed := time.Since(start)
	expectedTotal := 3 * interval

	// Should take approximately 3 intervals
	if totalElapsed < expectedTotal || totalElapsed > expectedTotal+100*time.Millisecond {
		t.Errorf("3 ticks took %v, expected around %v", totalElapsed, expectedTotal)
	}

	// Verify tick intervals - allow for timing variance in CI environments
	for i := 1; i < len(ticks); i++ {
		tickInterval := ticks[i].Sub(ticks[i-1])
		// Allow 1ms tolerance below interval (timer precision) and 50ms above
		if tickInterval < interval-time.Millisecond || tickInterval > interval+50*time.Millisecond {
			t.Errorf("Tick interval %d was %v, expected around %v", i, tickInterval, interval)
		}
	}
}

// TestRealClock_Ticker_Stop validates Ticker.Stop functionality.
func TestRealClock_Ticker_Stop(t *testing.T) {
	clock := RealClock
	interval := 30 * time.Millisecond

	ticker := clock.NewTicker(interval)

	// Wait for at least one tick
	select {
	case <-ticker.C():
		// Got first tick
	case <-time.After(100 * time.Millisecond):
		t.Fatal("First tick did not arrive")
	}

	// Stop ticker
	ticker.Stop()

	// No more ticks should arrive
	select {
	case <-ticker.C():
		t.Fatal("Ticker sent tick after Stop()")
	case <-time.After(100 * time.Millisecond):
		// Expected - ticker was stopped
	}
}

// TestRealClock_Timer_Channel validates Timer.C() returns correct channel.
func TestRealClock_Timer_Channel(t *testing.T) {
	clock := RealClock
	duration := 50 * time.Millisecond

	timer := clock.NewTimer(duration)
	ch1 := timer.C()
	ch2 := timer.C()

	// Should return same channel
	if ch1 != ch2 {
		t.Error("Timer.C() returned different channels on multiple calls")
	}

	// Channel should eventually receive
	select {
	case <-ch1:
		// Success
	case <-time.After(150 * time.Millisecond):
		t.Fatal("Timer channel did not receive value")
	}
}

// TestRealClock_Ticker_Channel validates Ticker.C() returns correct channel.
func TestRealClock_Ticker_Channel(t *testing.T) {
	clock := RealClock
	interval := 30 * time.Millisecond

	ticker := clock.NewTicker(interval)
	defer ticker.Stop()

	ch1 := ticker.C()
	ch2 := ticker.C()

	// Should return same channel
	if ch1 != ch2 {
		t.Error("Ticker.C() returned different channels on multiple calls")
	}

	// Channel should receive ticks
	select {
	case <-ch1:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Ticker channel did not receive tick")
	}
}

// TestRealClock_InterfaceCompliance validates real clock implements Clock interface.
func TestRealClock_InterfaceCompliance(t *testing.T) {
	var _ Clock = RealClock

	// Basic smoke test - all methods should be callable
	now := RealClock.Now()
	if now.IsZero() {
		t.Error("Now() returned zero time")
	}

	after := RealClock.After(time.Nanosecond)
	if after == nil {
		t.Error("After() returned nil channel")
	}

	timer := RealClock.NewTimer(time.Nanosecond)
	if timer == nil {
		t.Error("NewTimer() returned nil")
	}

	ticker := RealClock.NewTicker(time.Millisecond)
	if ticker == nil {
		t.Error("NewTicker() returned nil")
	}
	ticker.Stop()

	afterFuncCalled := false
	var afterFuncMu sync.Mutex
	afterFuncTimer := RealClock.AfterFunc(time.Nanosecond, func() {
		afterFuncMu.Lock()
		afterFuncCalled = true
		afterFuncMu.Unlock()
	})
	if afterFuncTimer == nil {
		t.Error("AfterFunc() returned nil timer")
	}

	// Give AfterFunc time to execute
	time.Sleep(10 * time.Millisecond)
	afterFuncMu.Lock()
	wasExecuted := afterFuncCalled
	afterFuncMu.Unlock()
	if !wasExecuted {
		t.Error("AfterFunc callback was not executed")
	}
}

// TestRealClock_Sleep validates Sleep blocks for the specified duration.
func TestRealClock_Sleep(t *testing.T) {
	clock := RealClock
	duration := 50 * time.Millisecond

	start := time.Now()
	clock.Sleep(duration)
	elapsed := time.Since(start)

	// Allow for reasonable timing variance
	if elapsed < duration || elapsed > duration+50*time.Millisecond {
		t.Errorf("Sleep took %v, expected around %v", elapsed, duration)
	}
}

// TestRealClock_Since validates Since returns correct elapsed duration.
func TestRealClock_Since(t *testing.T) {
	clock := RealClock

	start := clock.Now()
	time.Sleep(50 * time.Millisecond)
	elapsed := clock.Since(start)

	// Should be approximately the sleep duration
	if elapsed < 40*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Errorf("Since returned %v, expected around 50ms", elapsed)
	}
}

// TestRealClock_Since_PastTime validates Since with past time.
func TestRealClock_Since_PastTime(t *testing.T) {
	clock := RealClock

	// Time from one hour ago
	pastTime := clock.Now().Add(-time.Hour)
	elapsed := clock.Since(pastTime)

	// Should be approximately one hour
	if elapsed < 59*time.Minute || elapsed > 61*time.Minute {
		t.Errorf("Since returned %v for time one hour ago", elapsed)
	}
}

// TestRealClock_Since_FutureTime validates Since with future time returns negative.
func TestRealClock_Since_FutureTime(t *testing.T) {
	clock := RealClock

	// Time one hour in the future
	futureTime := clock.Now().Add(time.Hour)
	elapsed := clock.Since(futureTime)

	// Should be negative (approximately negative one hour)
	if elapsed > -59*time.Minute || elapsed < -61*time.Minute {
		t.Errorf("Since returned %v for future time, expected negative duration around -1h", elapsed)
	}
}

func TestRealClock_WithTimeout_Basic(t *testing.T) {
	clock := RealClock
	parent := context.Background()

	// Create timeout context with very short timeout
	ctx, cancel := clock.WithTimeout(parent, 10*time.Millisecond)
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
	if time.Until(deadline) > 15*time.Millisecond {
		t.Fatal("deadline too far in future")
	}

	// Wait for timeout
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected context to timeout")
	}
}

func TestRealClock_WithTimeout_ManualCancel(t *testing.T) {
	clock := RealClock
	parent := context.Background()

	// Create timeout context with long timeout
	ctx, cancel := clock.WithTimeout(parent, 1*time.Hour)

	// Cancel manually immediately
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
}

func TestRealClock_WithDeadline_Basic(t *testing.T) {
	clock := RealClock
	parent := context.Background()
	deadline := time.Now().Add(10 * time.Millisecond)

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

	// Wait for timeout
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected context to timeout")
	}
}

func TestRealClock_WithDeadline_PastDeadline(t *testing.T) {
	clock := RealClock
	parent := context.Background()
	pastDeadline := time.Now().Add(-1 * time.Second) // Past deadline

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

func TestRealClock_WithTimeout_ParentAlreadyCancelled(t *testing.T) {
	clock := RealClock

	// Create already canceled parent context
	parent, parentCancel := context.WithCancel(context.Background())
	parentCancel()

	// Create timeout context with canceled parent
	ctx, cancel := clock.WithTimeout(parent, 1*time.Hour)
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

func TestRealClock_WithTimeout_ValuePropagation(t *testing.T) {
	clock := RealClock

	// Create parent context with value
	type keyType string
	const testKey keyType = "test"
	parent := context.WithValue(context.Background(), testKey, "test-value")

	// Create timeout context
	ctx, cancel := clock.WithTimeout(parent, 1*time.Hour)
	defer cancel()

	// Should propagate parent's value
	if value := ctx.Value(testKey); value != "test-value" {
		t.Fatalf("expected 'test-value', got %v", value)
	}
}

func TestRealClock_WithTimeout_ZeroTimeout(t *testing.T) {
	clock := RealClock
	parent := context.Background()

	// Create timeout context with zero timeout
	ctx, cancel := clock.WithTimeout(parent, 0)
	defer cancel()

	// Should be canceled immediately or very quickly
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(10 * time.Millisecond):
		t.Fatal("expected context to be canceled immediately")
	}
}

func TestRealClock_WithTimeout_NegativeTimeout(t *testing.T) {
	clock := RealClock
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
	case <-time.After(10 * time.Millisecond):
		t.Fatal("expected context to be canceled immediately")
	}
}
