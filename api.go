package clockz

import (
	"context"
	"time"
)

// Clock provides an interface for time operations, enabling
// both real and mock implementations for testing.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// After waits for the duration to elapse and then sends the current time
	// on the returned channel.
	After(d time.Duration) <-chan time.Time

	// AfterFunc waits for the duration to elapse and then executes f
	// in its own goroutine. It returns a Timer that can be used to
	// cancel the call using its Stop method.
	AfterFunc(d time.Duration, f func()) Timer

	// NewTimer creates a new Timer that will send the current time
	// on its channel after at least duration d.
	NewTimer(d time.Duration) Timer

	// NewTicker returns a new Ticker containing a channel that will
	// send the time with a period specified by the duration argument.
	NewTicker(d time.Duration) Ticker

	// Sleep blocks for the specified duration.
	// Implemented as simple blocking on After() channel.
	Sleep(d time.Duration)

	// Since returns the time elapsed since t.
	// Equivalent to Now().Sub(t).
	Since(t time.Time) time.Duration

	// WithTimeout returns a context that cancels after the specified duration.
	// Equivalent to context.WithTimeout but uses clock time source.
	WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc)

	// WithDeadline returns a context that cancels at the specified deadline.
	// Equivalent to context.WithDeadline but uses clock time source.
	WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc)
}

// Timer represents a single event timer.
type Timer interface {
	// Stop prevents the Timer from firing.
	// It returns true if the call stops the timer, false if the timer
	// has already expired or been stopped.
	Stop() bool

	// Reset changes the timer to expire after duration d.
	// It returns true if the timer had been active, false if
	// the timer had expired or been stopped.
	Reset(d time.Duration) bool

	// C returns the channel on which the time will be sent.
	C() <-chan time.Time
}

// Ticker holds a channel that delivers ticks of a clock at intervals.
type Ticker interface {
	// Stop turns off a ticker. After Stop, no more ticks will be sent.
	Stop()

	// C returns the channel on which the ticks are delivered.
	C() <-chan time.Time
}
