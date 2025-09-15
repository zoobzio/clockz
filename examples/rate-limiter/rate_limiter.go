package main

import (
	"sync"
	"time"

	"github.com/zoobzio/clockz"
)

// SlidingWindowLimiter implements a sliding window rate limiter.
// This is a common pattern in API gateways and web services.
type SlidingWindowLimiter struct {
	clock    clockz.Clock
	limit    int
	window   time.Duration
	requests []time.Time
	mu       sync.Mutex
}

// NewSlidingWindowLimiter creates a rate limiter with the real clock.
// This is what production code uses.
func NewSlidingWindowLimiter(limit int, window time.Duration) *SlidingWindowLimiter {
	return NewSlidingWindowLimiterWithClock(limit, window, clockz.RealClock)
}

// NewSlidingWindowLimiterWithClock creates a rate limiter with a custom clock.
// This is what tests use for deterministic behavior.
func NewSlidingWindowLimiterWithClock(limit int, window time.Duration, clock clockz.Clock) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		clock:    clock,
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0, limit),
	}
}

// Allow checks if a request is allowed under the rate limit.
func (l *SlidingWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()
	windowStart := now.Add(-l.window)

	// Remove requests outside the window
	validRequests := make([]time.Time, 0, len(l.requests))
	for _, reqTime := range l.requests {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}
	l.requests = validRequests

	// Check if we're at the limit
	if len(l.requests) >= l.limit {
		return false
	}

	// Allow and record the request
	l.requests = append(l.requests, now)
	return true
}

// TokenBucketLimiter implements a token bucket rate limiter.
// Tokens are added at a fixed rate and consumed by requests.
type TokenBucketLimiter struct {
	clock        clockz.Clock
	capacity     int
	refillRate   int           // tokens per interval
	refillPeriod time.Duration // interval duration
	tokens       int
	lastRefill   time.Time
	ticker       clockz.Ticker
	mu           sync.Mutex
	done         chan struct{}
}

// NewTokenBucketLimiter creates a token bucket limiter with real clock.
func NewTokenBucketLimiter(capacity, refillRate int, refillPeriod time.Duration) *TokenBucketLimiter {
	return NewTokenBucketLimiterWithClock(capacity, refillRate, refillPeriod, clockz.RealClock)
}

// NewTokenBucketLimiterWithClock creates a token bucket limiter with custom clock.
func NewTokenBucketLimiterWithClock(capacity, refillRate int, refillPeriod time.Duration, clock clockz.Clock) *TokenBucketLimiter {
	l := &TokenBucketLimiter{
		clock:        clock,
		capacity:     capacity,
		refillRate:   refillRate,
		refillPeriod: refillPeriod,
		tokens:       capacity, // Start with full bucket
		lastRefill:   clock.Now(),
		done:         make(chan struct{}),
	}

	// Start background refill goroutine
	l.ticker = clock.NewTicker(refillPeriod)
	go l.refillLoop()

	return l
}

// refillLoop adds tokens at the configured rate.
func (l *TokenBucketLimiter) refillLoop() {
	for {
		select {
		case <-l.ticker.C():
			l.mu.Lock()
			l.tokens += l.refillRate
			if l.tokens > l.capacity {
				l.tokens = l.capacity
			}
			l.mu.Unlock()
		case <-l.done:
			l.ticker.Stop()
			return
		}
	}
}

// Allow tries to consume a token.
func (l *TokenBucketLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.tokens > 0 {
		l.tokens--
		return true
	}
	return false
}

// Stop cleanly shuts down the token bucket limiter.
func (l *TokenBucketLimiter) Stop() {
	close(l.done)
}

// BurstLimiter allows bursts up to a limit, then enforces a cooldown.
// Common in APIs that allow occasional bursts of activity.
type BurstLimiter struct {
	clock         clockz.Clock
	burstSize     int
	cooldownTime  time.Duration
	currentBurst  int
	cooldownUntil time.Time
	mu            sync.Mutex
}

// NewBurstLimiter creates a burst limiter with real clock.
func NewBurstLimiter(burstSize int, cooldown time.Duration) *BurstLimiter {
	return NewBurstLimiterWithClock(burstSize, cooldown, clockz.RealClock)
}

// NewBurstLimiterWithClock creates a burst limiter with custom clock.
func NewBurstLimiterWithClock(burstSize int, cooldown time.Duration, clock clockz.Clock) *BurstLimiter {
	return &BurstLimiter{
		clock:        clock,
		burstSize:    burstSize,
		cooldownTime: cooldown,
	}
}

// Allow checks if request is allowed within burst or after cooldown.
func (l *BurstLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()

	// Check if we're in cooldown
	if now.Before(l.cooldownUntil) {
		return false
	}

	// Check if we can continue the burst
	if l.currentBurst < l.burstSize {
		l.currentBurst++
		
		// If we hit the limit, start cooldown
		if l.currentBurst == l.burstSize {
			l.cooldownUntil = now.Add(l.cooldownTime)
		}
		return true
	}

	// Cooldown has passed, reset burst counter
	l.currentBurst = 1
	l.cooldownUntil = time.Time{}
	return true
}