package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zoobzio/clockz"
)

// Event represents a data event in the system
type Event struct {
	ID        int
	Timestamp time.Time
	Data      interface{}
}

// EventBuffer batches events and flushes on size or timeout
type EventBuffer struct {
	clock        clockz.Clock
	capacity     int
	flushTimeout time.Duration
	buffer       []Event
	mu           sync.Mutex
	flushChan    chan []Event
	timer        clockz.Timer
	closed       int32
}

// NewEventBuffer creates buffer with real clock
func NewEventBuffer(capacity int, timeout time.Duration) *EventBuffer {
	return NewEventBufferWithClock(capacity, timeout, clockz.RealClock)
}

// NewEventBufferWithClock creates buffer with custom clock
func NewEventBufferWithClock(capacity int, timeout time.Duration, clock clockz.Clock) *EventBuffer {
	eb := &EventBuffer{
		clock:        clock,
		capacity:     capacity,
		flushTimeout: timeout,
		buffer:       make([]Event, 0, capacity),
		flushChan:    make(chan []Event, 1),
	}
	eb.resetTimer()
	return eb
}

// Add adds an event to the buffer
func (eb *EventBuffer) Add(event Event) error {
	if atomic.LoadInt32(&eb.closed) == 1 {
		return errors.New("buffer closed")
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	event.Timestamp = eb.clock.Now()
	eb.buffer = append(eb.buffer, event)

	// Flush if at capacity
	if len(eb.buffer) >= eb.capacity {
		eb.flushLocked()
	}

	return nil
}

// WaitForFlush blocks until next flush
func (eb *EventBuffer) WaitForFlush() []Event {
	return <-eb.flushChan
}

// flushLocked flushes buffer (must hold lock)
func (eb *EventBuffer) flushLocked() {
	if len(eb.buffer) == 0 {
		return
	}

	// Copy buffer for sending
	events := make([]Event, len(eb.buffer))
	copy(events, eb.buffer)

	// Reset buffer
	eb.buffer = eb.buffer[:0]

	// Send to flush channel (non-blocking)
	select {
	case eb.flushChan <- events:
	default:
		// Channel full, drop flush
	}

	// Reset timer
	eb.resetTimer()
}

// resetTimer resets the flush timer
func (eb *EventBuffer) resetTimer() {
	if eb.timer != nil {
		eb.timer.Stop()
	}
	eb.timer = eb.clock.AfterFunc(eb.flushTimeout, func() {
		// Run flush in separate goroutine to avoid deadlock
		// when using FakeClock (AfterFunc runs synchronously)
		go func() {
			eb.mu.Lock()
			defer eb.mu.Unlock()
			eb.flushLocked()
		}()
	})
}

// Close closes the buffer
func (eb *EventBuffer) Close() {
	atomic.StoreInt32(&eb.closed, 1)
	eb.mu.Lock()
	if eb.timer != nil {
		eb.timer.Stop()
	}
	eb.flushLocked()
	eb.mu.Unlock()
	close(eb.flushChan)
}

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
	clock           clockz.Clock
	failureThreshold int
	recoveryTimeout time.Duration
	failures        int
	lastFailureTime time.Time
	state           string // "closed", "open", "half-open"
	mu              sync.RWMutex
}

// NewCircuitBreaker creates breaker with real clock
func NewCircuitBreaker(threshold int, recovery time.Duration) *CircuitBreaker {
	return NewCircuitBreakerWithClock(threshold, recovery, clockz.RealClock)
}

// NewCircuitBreakerWithClock creates breaker with custom clock
func NewCircuitBreakerWithClock(threshold int, recovery time.Duration, clock clockz.Clock) *CircuitBreaker {
	return &CircuitBreaker{
		clock:           clock,
		failureThreshold: threshold,
		recoveryTimeout: recovery,
		state:           "closed",
	}
}

// RecordFailure records a failure
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = cb.clock.Now()

	if cb.failures >= cb.failureThreshold {
		cb.state = "open"
	}
}

// RecordSuccess records a success
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == "half-open" {
		cb.state = "closed"
		cb.failures = 0
	}
}

// IsOpen checks if circuit is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == "closed" {
		return false
	}

	// Check if recovery time has passed
	if cb.state == "open" {
		if cb.clock.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
			// Transition to half-open for testing
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = "half-open"
			cb.mu.Unlock()
			cb.mu.RLock()
			return false
		}
	}

	return cb.state == "open"
}

// Call executes function with circuit breaker
func (cb *CircuitBreaker) Call(fn func() error) error {
	if cb.IsOpen() {
		return errors.New("circuit breaker open")
	}

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// RetryManager handles retry logic with backoff
type RetryManager struct {
	clock       clockz.Clock
	maxRetries  int
	baseDelay   time.Duration
	maxDelay    time.Duration
	multiplier  float64
}

// NewRetryManager creates retry manager with real clock
func NewRetryManager(maxRetries int, baseDelay, maxDelay time.Duration) *RetryManager {
	return NewRetryManagerWithClock(maxRetries, baseDelay, maxDelay, clockz.RealClock)
}

// NewRetryManagerWithClock creates retry manager with custom clock
func NewRetryManagerWithClock(maxRetries int, baseDelay, maxDelay time.Duration, clock clockz.Clock) *RetryManager {
	return &RetryManager{
		clock:      clock,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		maxDelay:   maxDelay,
		multiplier: 2.0,
	}
}

// Execute runs function with retries
func (rm *RetryManager) Execute(ctx context.Context, fn func() error) error {
	var lastErr error
	delay := rm.baseDelay

	for attempt := 0; attempt <= rm.maxRetries; attempt++ {
		// Check context
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try the function
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		// Don't sleep after last attempt
		if attempt < rm.maxRetries {
			// Calculate next delay
			if delay > rm.maxDelay {
				delay = rm.maxDelay
			}

			// Sleep with context
			timer := rm.clock.NewTimer(delay)
			select {
			case <-timer.C():
				// Continue to next attempt
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}

			// Exponential backoff
			delay = time.Duration(float64(delay) * rm.multiplier)
		}
	}

	return lastErr
}

// TTLCache is a cache with time-based expiration
type TTLCache struct {
	clock   clockz.Clock
	data    map[string]cacheEntry
	mu      sync.RWMutex
	janitor *cacheJanitor
}

type cacheEntry struct {
	value      interface{}
	expiration time.Time
}

type cacheJanitor struct {
	interval time.Duration
	stop     chan struct{}
}

// NewTTLCache creates cache with real clock
func NewTTLCache(cleanupInterval time.Duration) *TTLCache {
	return NewTTLCacheWithClock(cleanupInterval, clockz.RealClock)
}

// NewTTLCacheWithClock creates cache with custom clock
func NewTTLCacheWithClock(cleanupInterval time.Duration, clock clockz.Clock) *TTLCache {
	c := &TTLCache{
		clock: clock,
		data:  make(map[string]cacheEntry),
	}

	// Start cleanup goroutine
	if cleanupInterval > 0 {
		janitor := &cacheJanitor{
			interval: cleanupInterval,
			stop:     make(chan struct{}),
		}
		c.janitor = janitor
		go c.cleanupLoop(janitor)
	}

	return c
}

// Set adds item with TTL
func (c *TTLCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = cacheEntry{
		value:      value,
		expiration: c.clock.Now().Add(ttl),
	}
}

// Get retrieves item if not expired
func (c *TTLCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}

	if c.clock.Now().After(entry.expiration) {
		// Expired
		return nil, false
	}

	return entry.value, true
}

// cleanupLoop removes expired entries
func (c *TTLCache) cleanupLoop(j *cacheJanitor) {
	ticker := c.clock.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C():
			c.deleteExpired()
		case <-j.stop:
			return
		}
	}
}

// deleteExpired removes all expired entries
func (c *TTLCache) deleteExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock.Now()
	for key, entry := range c.data {
		if now.After(entry.expiration) {
			delete(c.data, key)
		}
	}
}

// Close stops the cache janitor
func (c *TTLCache) Close() {
	if c.janitor != nil {
		close(c.janitor.stop)
	}
}

// CoordinatedWorkers demonstrates timer coordination
type CoordinatedWorkers struct {
	clock        clockz.Clock
	workers      int
	workInterval time.Duration
	coordinator  chan int
	results      chan WorkResult
	wg           sync.WaitGroup
	stop         chan struct{}
}

type WorkResult struct {
	WorkerID  int
	Timestamp time.Time
	Success   bool
}

// NewCoordinatedWorkers creates workers with real clock
func NewCoordinatedWorkers(workers int, interval time.Duration) *CoordinatedWorkers {
	return NewCoordinatedWorkersWithClock(workers, interval, clockz.RealClock)
}

// NewCoordinatedWorkersWithClock creates workers with custom clock
func NewCoordinatedWorkersWithClock(workers int, interval time.Duration, clock clockz.Clock) *CoordinatedWorkers {
	return &CoordinatedWorkers{
		clock:        clock,
		workers:      workers,
		workInterval: interval,
		coordinator:  make(chan int, workers),
		results:      make(chan WorkResult, workers*10),
		stop:         make(chan struct{}),
	}
}

// Start begins coordinated work
func (cw *CoordinatedWorkers) Start() {
	// Start workers
	for i := 0; i < cw.workers; i++ {
		cw.wg.Add(1)
		go cw.worker(i)
	}

	// Start coordinator
	cw.wg.Add(1)
	go cw.coordinatorLoop()
}

// worker performs work when coordinated
func (cw *CoordinatedWorkers) worker(id int) {
	defer cw.wg.Done()

	for {
		select {
		case <-cw.coordinator:
			// Simulate work
			result := WorkResult{
				WorkerID:  id,
				Timestamp: cw.clock.Now(),
				Success:   true,
			}
			select {
			case cw.results <- result:
			default:
				// Results channel full
			}
		case <-cw.stop:
			return
		}
	}
}

// coordinatorLoop triggers workers periodically
func (cw *CoordinatedWorkers) coordinatorLoop() {
	defer cw.wg.Done()

	ticker := cw.clock.NewTicker(cw.workInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C():
			// Trigger all workers
			for i := 0; i < cw.workers; i++ {
				select {
				case cw.coordinator <- i:
				default:
					// Worker busy
				}
			}
		case <-cw.stop:
			return
		}
	}
}

// GetResults returns the results channel
func (cw *CoordinatedWorkers) GetResults() <-chan WorkResult {
	return cw.results
}

// Stop stops all workers
func (cw *CoordinatedWorkers) Stop() {
	close(cw.stop)
	cw.wg.Wait()
}