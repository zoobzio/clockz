package benchmarks

import (
	"context"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// BenchmarkMemoryLeakDetection measures memory usage over sustained operations.
// This exposes whether waiters, contexts, or goroutines leak during normal usage.
func BenchmarkMemoryLeakDetection(b *testing.B) {
	scenarios := []struct {
		name       string
		cycles     int
		operations int
	}{
		{"short_cycles", 100, 10},
		{"medium_cycles", 200, 25},
		{"long_cycles", 500, 50},
		{"extended_cycles", 1000, 100},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Baseline memory measurement
			runtime.GC()
			var baselineStats runtime.MemStats
			runtime.ReadMemStats(&baselineStats)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate sustained test suite operation
				for cycle := 0; cycle < scenario.cycles; cycle++ {
					var timers []clockz.Timer
					var cancels []context.CancelFunc

					// Create operations
					for op := 0; op < scenario.operations; op++ {
						// Mix of timer operations
						timer := clock.NewTimer(time.Duration(op+50) * time.Millisecond)
						timers = append(timers, timer)

						// Mix of context operations
						_, cancel := clock.WithTimeout(context.Background(), time.Duration(op+100)*time.Millisecond)
						cancels = append(cancels, cancel)
					}

					// Advance time to trigger some operations
					clock.Advance(75 * time.Millisecond)
					clock.BlockUntilReady()

					// Cleanup operations
					for _, timer := range timers {
						timer.Stop()
					}
					for _, cancel := range cancels {
						cancel()
					}

					// Force GC periodically to detect leaks
					if cycle%50 == 0 {
						runtime.GC()
					}
				}
			}

			b.StopTimer()

			// Final memory measurement
			runtime.GC()
			var finalStats runtime.MemStats
			runtime.ReadMemStats(&finalStats)

			// Report memory growth (should be minimal for no leaks)
			memoryGrowth := finalStats.Alloc - baselineStats.Alloc
			b.ReportMetric(float64(memoryGrowth), "bytes_growth")
		})
	}
}

// BenchmarkAllocationPatterns measures allocation behavior under realistic usage.
// This helps identify allocation hotspots and memory pressure patterns.
func BenchmarkAllocationPatterns(b *testing.B) {
	allocationScenarios := []struct {
		name         string
		timerCount   int
		contextCount int
		advancement  time.Duration
	}{
		{"light_allocation", 20, 10, 50 * time.Millisecond},
		{"medium_allocation", 100, 50, 200 * time.Millisecond},
		{"heavy_allocation", 500, 250, 500 * time.Millisecond},
		{"extreme_allocation", 1000, 500, 1 * time.Second},
	}

	for _, scenario := range allocationScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var timers []clockz.Timer
				var cancels []context.CancelFunc

				// Allocation burst - realistic test pattern
				for j := 0; j < scenario.timerCount; j++ {
					duration := time.Duration(j%500+10) * time.Millisecond
					timer := clock.NewTimer(duration)
					timers = append(timers, timer)
				}

				for j := 0; j < scenario.contextCount; j++ {
					timeout := time.Duration(j%300+50) * time.Millisecond
					_, cancel := clock.WithTimeout(context.Background(), timeout)
					cancels = append(cancels, cancel)
				}

				// Trigger operations (causes allocations in waiter processing)
				clock.Advance(scenario.advancement)
				clock.BlockUntilReady()

				// Cleanup
				for _, timer := range timers {
					timer.Stop()
				}
				for _, cancel := range cancels {
					cancel()
				}
			}
		})
	}
}

// BenchmarkGCPressure measures garbage collection impact under sustained load.
// This exposes how waiter slicing and context cleanup affects GC behavior.
func BenchmarkGCPressure(b *testing.B) {
	clock := clockz.NewFakeClock()

	b.ReportAllocs()

	// Disable GC during measurement to see allocation pressure
	gcPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(gcPercent)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create heavy load that stresses slice management
		var allTimers []clockz.Timer
		var allCancels []context.CancelFunc

		// Create many waiters in waves
		for wave := 0; wave < 10; wave++ {
			var waveCancels []context.CancelFunc

			// Each wave creates many waiters
			for item := 0; item < 100; item++ {
				// Timers with different expiration times
				timer := clock.NewTimer(time.Duration(wave*50+item) * time.Millisecond)
				allTimers = append(allTimers, timer)

				// Contexts with different timeouts
				_, cancel := clock.WithTimeout(context.Background(), time.Duration(wave*100+item)*time.Millisecond)
				waveCancels = append(waveCancels, cancel)
				allCancels = append(allCancels, cancel)
			}

			// Advance time to expire some waiters (triggers slice reallocation)
			clock.Advance(time.Duration(wave*25+50) * time.Millisecond)

			// Cancel some contexts randomly (triggers slice modification)
			for idx, cancel := range waveCancels {
				if idx%3 == 0 {
					cancel()
				}
			}
		}

		// Final advancement to clean up remaining waiters
		clock.Advance(1 * time.Second)
		clock.BlockUntilReady()

		// Cleanup all remaining
		for _, timer := range allTimers {
			timer.Stop()
		}
		for _, cancel := range allCancels {
			cancel()
		}
	}
}

// BenchmarkWaiterSliceGrowth measures the cost of waiter slice management.
// This exposes the overhead of slice growth and shrinkage during waiter lifecycle.
func BenchmarkWaiterSliceGrowth(b *testing.B) {
	growthPatterns := []struct {
		name    string
		maxSize int
		pattern string // "linear", "burst", "oscillate"
	}{
		{"linear_growth", 200, "linear"},
		{"burst_growth", 500, "burst"},
		{"oscillating_growth", 300, "oscillate"},
	}

	for _, pattern := range growthPatterns {
		b.Run(pattern.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var timers []clockz.Timer

				switch pattern.pattern {
				case "linear":
					// Steady growth
					for size := 10; size <= pattern.maxSize; size += 10 {
						timer := clock.NewTimer(time.Duration(size) * time.Millisecond)
						timers = append(timers, timer)

						if size%50 == 0 {
							// Occasionally trigger waiter processing
							clock.Advance(25 * time.Millisecond)
						}
					}

				case "burst":
					// Burst creation followed by cleanup
					for burst := 0; burst < 5; burst++ {
						var burstTimers []clockz.Timer

						// Create burst of timers
						for j := 0; j < pattern.maxSize/5; j++ {
							timer := clock.NewTimer(time.Duration(j+10) * time.Millisecond)
							burstTimers = append(burstTimers, timer)
							timers = append(timers, timer)
						}

						// Advance time to expire some
						clock.Advance(50 * time.Millisecond)

						// Stop some timers (modifies waiter slice)
						for idx, timer := range burstTimers {
							if idx%2 == 0 {
								timer.Stop()
							}
						}
					}

				case "oscillate":
					// Oscillating growth/shrinkage
					for cycle := 0; cycle < 10; cycle++ {
						var cycleTimers []clockz.Timer

						// Growth phase
						cycleSize := pattern.maxSize / 10
						for j := 0; j < cycleSize; j++ {
							timer := clock.NewTimer(time.Duration(j+cycle*100) * time.Millisecond)
							cycleTimers = append(cycleTimers, timer)
							timers = append(timers, timer)
						}

						// Advance to expire some
						clock.Advance(time.Duration(cycle*20+30) * time.Millisecond)

						// Shrinkage phase - stop half the timers
						for idx, timer := range cycleTimers {
							if idx < len(cycleTimers)/2 {
								timer.Stop()
							}
						}
					}
				}

				// Final cleanup
				for _, timer := range timers {
					timer.Stop()
				}
			}
		})
	}
}

// BenchmarkLongRunningStability measures performance degradation over time.
// This exposes any memory or performance leaks during extended operation.
func BenchmarkLongRunningStability(b *testing.B) {
	clock := clockz.NewFakeClock()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate long-running test suite with periodic operations
		for hour := 0; hour < 24; hour++ { // Simulate 24 "hours" of operation
			for minute := 0; minute < 60; minute += 5 { // Every 5 "minutes"
				// Create some timers
				var timers []clockz.Timer
				for t := 0; t < 10; t++ {
					duration := time.Duration(t*100+minute*10) * time.Millisecond
					timer := clock.NewTimer(duration)
					timers = append(timers, timer)
				}

				// Create some contexts
				var cancels []context.CancelFunc
				for c := 0; c < 5; c++ {
					timeout := time.Duration(c*200+minute*20) * time.Millisecond
					_, cancel := clock.WithTimeout(context.Background(), timeout)
					cancels = append(cancels, cancel)
				}

				// Advance time
				clock.Advance(time.Duration(minute+1) * time.Minute)
				clock.BlockUntilReady()

				// Cleanup
				for _, timer := range timers {
					timer.Stop()
				}
				for _, cancel := range cancels {
					cancel()
				}

				// Periodic GC to detect accumulation
				if minute%20 == 0 {
					runtime.GC()
				}
			}
		}
	}
}
