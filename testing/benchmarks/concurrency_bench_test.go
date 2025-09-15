package benchmarks

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// BenchmarkConcurrentTimerCreation measures timer creation under concurrent load.
// This exposes mutex contention in the main clock mutex during timer setup.
func BenchmarkConcurrentTimerCreation(b *testing.B) {
	scenarios := []struct {
		name               string
		goroutines         int
		timersPerGoroutine int
	}{
		{"light_concurrent", 4, 25},
		{"medium_concurrent", 8, 50},
		{"heavy_concurrent", 16, 100},
		{"extreme_concurrent", 32, 200},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				for g := 0; g < scenario.goroutines; g++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						var timers []clockz.Timer
						for t := 0; t < scenario.timersPerGoroutine; t++ {
							// Realistic timer duration spread
							duration := time.Duration(10+t%500) * time.Millisecond
							timer := clock.NewTimer(duration)
							timers = append(timers, timer)
						}

						// Clean up timers
						for _, timer := range timers {
							timer.Stop()
						}
					}()
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkConcurrentAdvancement measures time advancement under concurrent timer access.
// This exposes the real pain point: concurrent Advance() calls with active waiters.
func BenchmarkConcurrentAdvancement(b *testing.B) {
	scenarios := []struct {
		name          string
		advancers     int
		observers     int
		initialTimers int
	}{
		{"single_advancer", 1, 4, 100},
		{"dual_advancers", 2, 4, 200},
		{"multi_advancers", 4, 8, 300},
		{"chaos_advancers", 8, 16, 500},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Pre-populate with timers to create realistic load
			var timers []clockz.Timer
			for i := 0; i < scenario.initialTimers; i++ {
				duration := time.Duration(50+i%1000) * time.Millisecond
				timers = append(timers, clock.NewTimer(duration))
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				// Goroutines advancing time (the expensive operation)
				for a := 0; a < scenario.advancers; a++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						// Multiple small advances - realistic test pattern
						for step := 0; step < 10; step++ {
							advancement := time.Duration(10+step*5) * time.Millisecond
							clock.Advance(advancement)
						}
					}()
				}

				// Goroutines reading time and checking waiters (observers)
				for o := 0; o < scenario.observers; o++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						for read := 0; read < 50; read++ {
							_ = clock.Now()
							_ = clock.HasWaiters()
							time.Sleep(1 * time.Microsecond) // Brief pause
						}
					}()
				}

				wg.Wait()
			}

			b.StopTimer()

			// Cleanup
			for _, timer := range timers {
				timer.Stop()
			}
		})
	}
}

// BenchmarkConcurrentMixedOps measures realistic concurrent usage patterns.
// This combines timer creation, advancement, context operations, and cleanup.
func BenchmarkConcurrentMixedOps(b *testing.B) {
	scenarios := []struct {
		name         string
		workers      int
		opsPerWorker int
	}{
		{"light_mixed", 4, 20},
		{"medium_mixed", 8, 40},
		{"heavy_mixed", 16, 80},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				// Worker goroutines doing mixed operations
				for w := 0; w < scenario.workers; w++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						for op := 0; op < scenario.opsPerWorker; op++ {
							switch op % 5 {
							case 0:
								// Create and immediately stop timer
								timer := clock.NewTimer(100 * time.Millisecond)
								timer.Stop()

							case 1:
								// Create context and cancel
								ctx, cancel := clock.WithTimeout(context.Background(), 200*time.Millisecond)
								cancel()
								_ = ctx.Err()

							case 2:
								// Read current time
								_ = clock.Now()
								_ = clock.HasWaiters()

							case 3:
								// Create ticker and stop
								ticker := clock.NewTicker(50 * time.Millisecond)
								ticker.Stop()

							case 4:
								// Small time advancement
								clock.Advance(10 * time.Millisecond)
							}
						}
					}()
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkRaceConditionStress attempts to trigger race conditions through timing.
// This stress tests the locking mechanisms under extreme concurrent load.
func BenchmarkRaceConditionStress(b *testing.B) {
	clock := clockz.NewFakeClock()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup

		// Maximum stress: many goroutines doing conflicting operations
		for g := 0; g < 20; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for iteration := 0; iteration < 25; iteration++ {
					switch iteration % 4 {
					case 0:
						// Timer lifecycle
						timer := clock.NewTimer(time.Duration(iteration+1) * time.Millisecond)
						if iteration%3 == 0 {
							timer.Reset(time.Duration(iteration+10) * time.Millisecond)
						}
						timer.Stop()

					case 1:
						// Context lifecycle with immediate operations
						ctx, cancel := clock.WithTimeout(context.Background(), time.Duration(iteration+5)*time.Millisecond)
						_, hasDeadline := ctx.Deadline()
						_ = hasDeadline
						cancel()

					case 2:
						// Time manipulation
						clock.Advance(time.Duration(iteration+1) * time.Millisecond)

					case 3:
						// State queries
						_ = clock.Now()
						_ = clock.HasWaiters()
					}
				}
			}()
		}

		wg.Wait()
	}
}

// BenchmarkBlockUntilReadyContention measures BlockUntilReady under concurrent load.
// This tests the coordination mechanism when multiple goroutines wait for completion.
func BenchmarkBlockUntilReadyContention(b *testing.B) {
	blockerCounts := []int{1, 4, 8, 16}

	for _, count := range blockerCounts {
		b.Run(fmt.Sprintf("blockers_%d", count), func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Set up some pending operations
			for i := 0; i < 50; i++ {
				clock.NewTimer(time.Duration(i+1) * time.Millisecond)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				// Create activity that BlockUntilReady must wait for
				clock.Advance(100 * time.Millisecond)

				// Multiple goroutines calling BlockUntilReady
				for g := 0; g < count; g++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						clock.BlockUntilReady()
					}()
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkTimerResetContention measures timer Reset operations under concurrent access.
// This tests the cost of modifying active waiters while other operations are ongoing.
func BenchmarkTimerResetContention(b *testing.B) {
	clock := clockz.NewFakeClock()

	// Create baseline set of timers
	var timers []clockz.Timer
	for i := 0; i < 100; i++ {
		timer := clock.NewTimer(time.Duration(i+100) * time.Millisecond)
		timers = append(timers, timer)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup

		// Goroutines constantly resetting timers
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(gIndex int) {
				defer wg.Done()

				for reset := 0; reset < 20; reset++ {
					timerIndex := (gIndex*20 + reset) % len(timers)
					newDuration := time.Duration(reset+50) * time.Millisecond
					timers[timerIndex].Reset(newDuration)
				}
			}(g)
		}

		// Concurrent time advancement
		go func() {
			for advance := 0; advance < 10; advance++ {
				clock.Advance(25 * time.Millisecond)
				time.Sleep(1 * time.Millisecond)
			}
		}()

		wg.Wait()
	}

	b.StopTimer()

	// Cleanup
	for _, timer := range timers {
		timer.Stop()
	}
}
