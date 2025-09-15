package clockz

import (
	"context"
	"time"
)

// RealClock is the default Clock implementation using the standard time package.
var RealClock Clock = &realClock{}

// realClock implements Clock using the standard time package.
type realClock struct{}

// Now returns the current time.
func (realClock) Now() time.Time {
	return time.Now()
}

// After waits for the duration to elapse and then sends the current time.
func (realClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// AfterFunc waits for the duration to elapse and then executes f.
func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return &realTimer{timer: time.AfterFunc(d, f)}
}

// NewTimer creates a new Timer.
func (realClock) NewTimer(d time.Duration) Timer {
	return &realTimer{timer: time.NewTimer(d)}
}

// NewTicker returns a new Ticker.
func (realClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

// Sleep blocks for the specified duration.
func (c realClock) Sleep(d time.Duration) {
	<-c.After(d)
}

// Since returns the time elapsed since t.
func (c realClock) Since(t time.Time) time.Duration {
	return c.Now().Sub(t)
}

// WithTimeout returns a context that cancels after the specified duration.
// Delegates to standard library for real clock implementation.
func (realClock) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// WithDeadline returns a context that cancels at the specified deadline.
// Delegates to standard library for real clock implementation.
func (realClock) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, deadline)
}

// realTimer wraps time.Timer to implement the Timer interface.
type realTimer struct {
	timer *time.Timer
}

// Stop prevents the Timer from firing.
func (t *realTimer) Stop() bool {
	return t.timer.Stop()
}

// Reset changes the timer to expire after duration d.
func (t *realTimer) Reset(d time.Duration) bool {
	return t.timer.Reset(d)
}

// C returns the channel on which the time will be sent.
func (t *realTimer) C() <-chan time.Time {
	return t.timer.C
}

// realTicker wraps time.Ticker to implement the Ticker interface.
type realTicker struct {
	ticker *time.Ticker
}

// Stop turns off the ticker.
func (t *realTicker) Stop() {
	t.ticker.Stop()
}

// C returns the channel on which the ticks are delivered.
func (t *realTicker) C() <-chan time.Time {
	return t.ticker.C
}
