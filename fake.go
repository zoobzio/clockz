// Package clockz provides a fake clock implementation for deterministic testing.
package clockz

import (
	"context"
	"sort"
	"sync"
	"time"
)

// FakeClock implements Clock for testing purposes.
// It allows manual control of time progression.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type FakeClock struct {
	mu             sync.RWMutex
	time           time.Time
	waiters        []*waiter
	pendingSends   []pendingSend    // Track timer channel deliveries
	contextWaiters []*contextWaiter // Track context timeout waiters
	contextMu      sync.Mutex       // Separate mutex for context operations
}

// pendingSend represents a timer value ready to send.
type pendingSend struct {
	ch    chan time.Time
	value time.Time
}

// waiter represents a timer or ticker waiting for a specific time.
type waiter struct {
	targetTime time.Time
	destChan   chan time.Time
	afterFunc  func()
	period     time.Duration // For tickers
	active     bool
}

// contextWaiter represents a context timeout waiting for a specific time.
type contextWaiter struct {
	deadline time.Time
	cancel   context.CancelFunc
	active   bool
}

// fakeTimeoutContext implements context.Context for fake clock timeouts.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type fakeTimeoutContext struct {
	context.Context                    // Parent context
	clock           *FakeClock         // Time source
	deadline        time.Time          // Cancellation deadline
	done            chan struct{}      // Cancellation signal (buffered like stdlib)
	err             error              // Cancellation cause
	cancelFunc      context.CancelFunc // Manual cancellation function
	once            sync.Once          // Protects done channel closure
	waiter          *contextWaiter     // Integration with fake clock
}

// NewFakeClock creates a new FakeClock set to time.Now().
func NewFakeClock() *FakeClock {
	return &FakeClock{
		time: time.Now(),
	}
}

// NewFakeClockAt creates a new FakeClock set to the given time.
func NewFakeClockAt(t time.Time) *FakeClock {
	return &FakeClock{
		time: t,
	}
}

// Now returns the current time of the fake clock.
func (f *FakeClock) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.time
}

// After waits for the duration to elapse and then sends the current time.
func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	targetTime := f.time.Add(d)

	w := &waiter{
		targetTime: targetTime,
		destChan:   ch,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return ch
}

// AfterFunc waits for the duration to elapse and then executes f.
func (f *FakeClock) AfterFunc(d time.Duration, fn func()) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()

	targetTime := f.time.Add(d)
	w := &waiter{
		targetTime: targetTime,
		afterFunc:  fn,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return &fakeTimer{
		clock:  f,
		waiter: w,
	}
}

// NewTimer creates a new Timer.
func (f *FakeClock) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	targetTime := f.time.Add(d)

	w := &waiter{
		targetTime: targetTime,
		destChan:   ch,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return &fakeTimer{
		clock:  f,
		waiter: w,
	}
}

// NewTicker returns a new Ticker.
func (f *FakeClock) NewTicker(d time.Duration) Ticker {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	targetTime := f.time.Add(d)

	w := &waiter{
		targetTime: targetTime,
		destChan:   ch,
		period:     d,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return &fakeTicker{
		clock:  f,
		waiter: w,
	}
}

// Sleep blocks for the specified duration.
func (f *FakeClock) Sleep(d time.Duration) {
	<-f.After(d)
}

// Since returns the time elapsed since t.
func (f *FakeClock) Since(t time.Time) time.Duration {
	return f.Now().Sub(t)
}

// Advance advances the fake clock by the given duration.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setTimeLocked(f.time.Add(d))
}

// SetTime sets the fake clock to the given time.
func (f *FakeClock) SetTime(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setTimeLocked(t)
}

// HasWaiters returns true if there are any waiters (timers or contexts).
func (f *FakeClock) HasWaiters() bool {
	f.mu.RLock()
	hasTimerWaiters := false
	for _, w := range f.waiters {
		if w.active {
			hasTimerWaiters = true
			break
		}
	}
	f.mu.RUnlock()

	f.contextMu.Lock()
	hasContextWaiters := len(f.contextWaiters) > 0
	f.contextMu.Unlock()

	return hasTimerWaiters || hasContextWaiters
}

// queueTimerSend adds a timer value to the pending sends queue.
// Caller must hold f.mu lock.
func (f *FakeClock) queueTimerSend(ch chan time.Time, value time.Time) {
	f.pendingSends = append(f.pendingSends, pendingSend{
		ch:    ch,
		value: value,
	})
}

// BlockUntilReady blocks until all pending operations have completed.
// This includes both AfterFunc callbacks, timer channel deliveries, and context operations.
//
// Timer channel deliveries are processed using non-blocking sends that
// preserve the original semantics - abandoned channels are skipped without
// hanging the call.
//
// Use this method to ensure deterministic timing in tests:
//
//	clock.Advance(duration)
//	clock.BlockUntilReady()  // Guarantees all timers processed
//	// Now safe to check timer channels or send additional data
func (f *FakeClock) BlockUntilReady() {
	// AfterFunc callbacks now execute synchronously, no need to wait

	// Process pending timer sends
	f.mu.Lock()
	sends := make([]pendingSend, len(f.pendingSends))
	copy(sends, f.pendingSends)
	f.pendingSends = nil // Clear pending sends
	f.mu.Unlock()

	// Complete all pending sends with abandonment detection
	for _, send := range sends {
		select {
		case send.ch <- send.value:
			// Successfully delivered
		default:
			// Channel full or no receiver - skip (preserves non-blocking semantics)
		}
	}

	// Check if any context waiters are pending (for synchronization)
	f.contextMu.Lock()
	contextReady := len(f.contextWaiters) > 0
	f.contextMu.Unlock()

	// Allow small delay for context operations to complete if any are pending
	if contextReady {
		time.Sleep(1 * time.Millisecond)
	}
}

// WithTimeout returns a context that cancels after the specified duration.
// Uses fake clock time source for deterministic testing.
func (f *FakeClock) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return f.WithDeadline(ctx, f.Now().Add(timeout))
}

// WithDeadline returns a context that cancels at the specified deadline.
// Uses fake clock time source for deterministic testing.
func (f *FakeClock) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	// Check if parent context is already canceled
	select {
	case <-ctx.Done():
		// Parent already canceled, return canceled context immediately
		done := make(chan struct{})
		close(done) // Close immediately since parent is canceled
		return &fakeTimeoutContext{
			Context:  ctx,
			clock:    f,
			deadline: deadline,
			done:     done,
			err:      ctx.Err(),
		}, func() {}
	default:
	}

	// Create new timeout context
	timeoutCtx := &fakeTimeoutContext{
		Context:  ctx,
		clock:    f,
		deadline: deadline,
		done:     make(chan struct{}),
	}

	// Create context waiter for timeout handling
	waiter := &contextWaiter{
		deadline: deadline,
		active:   true,
	}
	timeoutCtx.waiter = waiter

	// Set up timeout cancellation function
	waiter.cancel = func() {
		timeoutCtx.timeoutCancel()
	}

	// Register waiter with fake clock if deadline is in future
	if deadline.After(f.Now()) {
		f.contextMu.Lock()
		f.contextWaiters = append(f.contextWaiters, waiter)
		f.contextMu.Unlock()
	} else {
		// Deadline already passed, cancel immediately
		timeoutCtx.timeoutCancel()
	}

	// Create manual cancel function
	cancel := func() {
		timeoutCtx.cancel()
	}
	timeoutCtx.cancelFunc = cancel

	// Start goroutine to monitor parent context cancellation
	// This ensures parent cancellation propagates to child contexts
	go func() {
		select {
		case <-ctx.Done():
			// Parent canceled, propagate cancellation
			timeoutCtx.parentCancel()
		case <-timeoutCtx.done:
			// Our context canceled first, nothing to do
		}
	}()

	return timeoutCtx, cancel
}

// removeContextWaiter removes a context waiter from the slice.
// Caller must hold contextMu lock.
func (f *FakeClock) removeContextWaiter(target *contextWaiter) {
	for i, w := range f.contextWaiters {
		if w == target {
			// Remove by swapping with last element
			f.contextWaiters[i] = f.contextWaiters[len(f.contextWaiters)-1]
			f.contextWaiters = f.contextWaiters[:len(f.contextWaiters)-1]
			return
		}
	}
}

// setTimeLocked sets the time and triggers any waiters. Caller must hold f.mu.
func (f *FakeClock) setTimeLocked(t time.Time) {
	if t.Before(f.time) {
		panic("cannot move fake clock backwards")
	}

	f.time = t

	// Process timer waiters - sort by deadline for deterministic order
	activeWaiters := make([]*waiter, 0, len(f.waiters))
	for _, w := range f.waiters {
		if w.active {
			activeWaiters = append(activeWaiters, w)
		}
	}

	// Sort waiters by target time to ensure deterministic processing
	sort.Slice(activeWaiters, func(i, j int) bool {
		return activeWaiters[i].targetTime.Before(activeWaiters[j].targetTime)
	})

	newWaiters := make([]*waiter, 0, len(f.waiters))
	for _, w := range activeWaiters {
		if !w.targetTime.After(t) {
			// Time has passed, trigger the waiter
			if w.destChan != nil {
				f.queueTimerSend(w.destChan, t)
			}

			if w.afterFunc != nil {
				// Execute callback synchronously for deterministic ordering
				// Since waiters are sorted by deadline, callbacks execute in order
				w.afterFunc()
			}

			// Handle tickers
			if w.period > 0 {
				w.targetTime = w.targetTime.Add(w.period)
				for !w.targetTime.After(t) {
					f.queueTimerSend(w.destChan, w.targetTime)
					w.targetTime = w.targetTime.Add(w.period)
				}
				newWaiters = append(newWaiters, w)
			}
		} else {
			// Not ready yet
			newWaiters = append(newWaiters, w)
		}
	}
	f.waiters = newWaiters

	// Process context waiters - collect expired waiters first to avoid lock/cancel race
	var toCancel []*contextWaiter
	f.contextMu.Lock()
	newContextWaiters := make([]*contextWaiter, 0, len(f.contextWaiters))
	for _, w := range f.contextWaiters {
		if w.active && !t.Before(w.deadline) {
			// Deadline reached, mark for cancellation
			w.active = false
			toCancel = append(toCancel, w)
		} else if w.active {
			// Still active and not expired, keep it
			newContextWaiters = append(newContextWaiters, w)
		}
		// Skip inactive waiters (already canceled)
	}
	f.contextWaiters = newContextWaiters
	f.contextMu.Unlock()

	// Cancel expired waiters outside the lock to prevent deadlock
	for _, w := range toCancel {
		w.cancel()
	}
}

// fakeTimer implements Timer.
type fakeTimer struct {
	clock  *FakeClock
	waiter *waiter
}

// Stop prevents the Timer from firing.
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	active := t.waiter.active
	t.waiter.active = false
	return active
}

// Reset changes the timer to expire after duration d.
func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	active := t.waiter.active
	t.waiter.active = true
	t.waiter.targetTime = t.clock.time.Add(d)

	return active
}

// C returns the channel on which the time will be sent.
func (t *fakeTimer) C() <-chan time.Time {
	return t.waiter.destChan
}

// fakeTicker implements Ticker.
type fakeTicker struct {
	clock  *FakeClock
	waiter *waiter
}

// Stop turns off the ticker.
func (t *fakeTicker) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	t.waiter.active = false
}

// C returns the channel on which the ticks are delivered.
func (t *fakeTicker) C() <-chan time.Time {
	return t.waiter.destChan
}

// Done returns a channel that is closed when the timeout context is canceled.
// Implements context.Context interface.
func (ctx *fakeTimeoutContext) Done() <-chan struct{} {
	return ctx.done
}

// Err returns the error that caused the timeout context to be canceled.
// Returns nil until the context is canceled.
// Implements context.Context interface.
func (ctx *fakeTimeoutContext) Err() error {
	select {
	case <-ctx.done:
		return ctx.err
	default:
		return nil
	}
}

// Deadline returns the deadline time and whether a deadline is set.
// Implements context.Context interface.
func (ctx *fakeTimeoutContext) Deadline() (time.Time, bool) {
	return ctx.deadline, !ctx.deadline.IsZero()
}

// Value returns the value associated with the key from the parent context.
// Implements context.Context interface by delegating to parent.
func (ctx *fakeTimeoutContext) Value(key interface{}) interface{} {
	return ctx.Context.Value(key)
}

// cancel performs manual cancellation of the timeout context.
// Thread-safe using sync.Once to prevent double-close panics.
func (ctx *fakeTimeoutContext) cancel() {
	ctx.once.Do(func() {
		// Remove waiter immediately to prevent cleanup issues
		ctx.clock.contextMu.Lock()
		ctx.clock.removeContextWaiter(ctx.waiter)
		ctx.clock.contextMu.Unlock()

		// Set error and close done channel atomically
		ctx.err = context.Canceled
		close(ctx.done)
	})
}

// timeoutCancel performs timeout-triggered cancellation of the context.
// Called by fake clock when deadline is reached.
// Thread-safe using sync.Once to prevent double-close panics.
func (ctx *fakeTimeoutContext) timeoutCancel() {
	ctx.once.Do(func() {
		// Set timeout error and close done channel atomically
		ctx.err = context.DeadlineExceeded
		close(ctx.done)
	})
}

// parentCancel performs parent-triggered cancellation of the context.
// Called when parent context is canceled.
// Thread-safe using sync.Once to prevent double-close panics.
func (ctx *fakeTimeoutContext) parentCancel() {
	ctx.once.Do(func() {
		// Remove waiter immediately to prevent cleanup issues
		if ctx.waiter != nil {
			ctx.clock.contextMu.Lock()
			ctx.clock.removeContextWaiter(ctx.waiter)
			ctx.clock.contextMu.Unlock()
		}

		// Set parent error and close done channel atomically
		ctx.err = ctx.Context.Err()
		close(ctx.done)
	})
}
