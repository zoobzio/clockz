package benchmarks

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// BenchmarkContextCreation measures the overhead of creating timeout contexts.
// This exposes the cost of context waiter registration and goroutine spawning.
func BenchmarkContextCreation(b *testing.B) {
	contextCounts := []int{1, 10, 50, 100, 500}

	for _, count := range contextCounts {
		b.Run(fmt.Sprintf("contexts_%d", count), func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var cancels []context.CancelFunc

				// Create many contexts in burst - realistic test pattern
				for j := 0; j < count; j++ {
					timeout := time.Duration(100+j%400) * time.Millisecond
					_, cancel := clock.WithTimeout(context.Background(), timeout)
					cancels = append(cancels, cancel)
				}

				// Clean up to avoid affecting next iteration
				for _, cancel := range cancels {
					cancel()
				}
			}
		})
	}
}

// BenchmarkContextTimeout measures the cost of context timeout processing.
// This exposes the overhead in setTimeLocked when handling context waiters.
func BenchmarkContextTimeout(b *testing.B) {
	scenarios := []struct {
		name          string
		contextCount  int
		timeoutSpread time.Duration
		advancement   time.Duration
	}{
		{"quick_timeouts", 50, 50 * time.Millisecond, 100 * time.Millisecond},
		{"spread_timeouts", 100, 200 * time.Millisecond, 150 * time.Millisecond},
		{"delayed_timeouts", 200, 500 * time.Millisecond, 300 * time.Millisecond},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Create contexts with spread timeouts
			var cancels []context.CancelFunc

			for i := 0; i < scenario.contextCount; i++ {
				// Spread timeouts across range
				offset := time.Duration(i) * scenario.timeoutSpread / time.Duration(scenario.contextCount)
				_, cancel := clock.WithTimeout(context.Background(), offset)
				cancels = append(cancels, cancel)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// This triggers context waiter processing in setTimeLocked
				clock.Advance(scenario.advancement)
			}

			b.StopTimer()

			// Cleanup
			for _, cancel := range cancels {
				cancel()
			}
		})
	}
}

// BenchmarkContextCancellation measures the overhead of manual context cancellation.
// This tests the removeContextWaiter performance and mutex contention.
func BenchmarkContextCancellation(b *testing.B) {
	contextCounts := []int{10, 50, 100, 500, 1000}

	for _, count := range contextCounts {
		b.Run(fmt.Sprintf("cancel_%d", count), func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var cancels []context.CancelFunc

				// Create contexts
				for j := 0; j < count; j++ {
					timeout := time.Duration(1+j) * time.Second // Long timeouts
					_, cancel := clock.WithTimeout(context.Background(), timeout)
					cancels = append(cancels, cancel)
				}

				b.StartTimer()
				// Cancel all contexts - tests removeContextWaiter performance
				for _, cancel := range cancels {
					cancel()
				}
				b.StopTimer()
			}
		})
	}
}

// BenchmarkConcurrentContextOps measures context operations under concurrent access.
// This exposes mutex contention between main mutex and context mutex.
func BenchmarkConcurrentContextOps(b *testing.B) {
	scenarios := []struct {
		name            string
		goroutines      int
		opsPerGoroutine int
	}{
		{"light_concurrent", 4, 25},
		{"medium_concurrent", 8, 50},
		{"heavy_concurrent", 16, 100},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				// Start concurrent goroutines doing context operations
				for g := 0; g < scenario.goroutines; g++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						for op := 0; op < scenario.opsPerGoroutine; op++ {
							// Mix of operations that stress context mutex
							timeout := time.Duration(100+op%200) * time.Millisecond
							ctx, cancel := clock.WithTimeout(context.Background(), timeout)

							// Immediate cancellation creates removeContextWaiter calls
							cancel()

							// Check context state
							_ = ctx.Err()
						}
					}()
				}

				// While contexts are being created/canceled, advance time
				go func() {
					for j := 0; j < 10; j++ {
						clock.Advance(10 * time.Millisecond)
						time.Sleep(1 * time.Millisecond) // Brief pause
					}
				}()

				wg.Wait()
			}
		})
	}
}

// BenchmarkContextDeadlineChecking measures the cost of deadline evaluation.
// Tests the performance of checking if contexts should timeout.
func BenchmarkContextDeadlineChecking(b *testing.B) {
	clock := clockz.NewFakeClock()

	// Create mix of contexts - some expired, some not
	var contexts []context.Context
	var cancels []context.CancelFunc

	for i := 0; i < 200; i++ {
		var timeout time.Duration
		if i%3 == 0 {
			// Some contexts already expired
			timeout = time.Duration(-50+i%100) * time.Millisecond
		} else {
			// Some contexts expire in future
			timeout = time.Duration(100+i%500) * time.Millisecond
		}

		ctx, cancel := clock.WithTimeout(context.Background(), timeout)
		contexts = append(contexts, ctx)
		cancels = append(cancels, cancel)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Check all context deadlines and errors
		for _, ctx := range contexts {
			_, hasDeadline := ctx.Deadline()
			_ = hasDeadline
			_ = ctx.Err()
		}
	}

	b.StopTimer()

	// Cleanup
	for _, cancel := range cancels {
		cancel()
	}
}

// BenchmarkNestedContexts measures the performance of nested context hierarchies.
// This tests parent cancellation propagation and goroutine cleanup.
func BenchmarkNestedContexts(b *testing.B) {
	depths := []int{2, 5, 10, 20}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			clock := clockz.NewFakeClock()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create nested context chain
				parent := context.Background()
				var cancels []context.CancelFunc

				for level := 0; level < depth; level++ {
					timeout := time.Duration(100*(level+1)) * time.Millisecond
					ctx, cancel := clock.WithTimeout(parent, timeout)
					cancels = append(cancels, cancel)
					parent = ctx
				}

				// Cancel root - should propagate to all children
				if len(cancels) > 0 {
					cancels[0]() // Cancel first (root) context
				}

				// Cleanup remaining
				for _, cancel := range cancels[1:] {
					cancel()
				}
			}
		})
	}
}
