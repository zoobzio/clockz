package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// BenchmarkWaiterScaling exposes the O(n log n) sort bottleneck in setTimeLocked.
// This benchmark shows how performance degrades with waiter count.
// Critical for understanding test suite pain points with many timers.
func BenchmarkWaiterScaling(b *testing.B) {
	waiterCounts := []int{10, 50, 100, 500, 1000, 2000}

	for _, count := range waiterCounts {
		b.Run(fmt.Sprintf("waiters_%d", count), func(b *testing.B) {
			benchmarkWaitersAtScale(b, count)
		})
	}
}

func benchmarkWaitersAtScale(b *testing.B, waiterCount int) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		clock := clockz.NewFakeClock()

		// Create realistic timer distribution - not all at same time
		timers := make([]clockz.Timer, waiterCount)
		for j := 0; j < waiterCount; j++ {
			// Spread timers across realistic durations (1ms to 1s)
			duration := time.Duration(j%1000+1) * time.Millisecond
			timers[j] = clock.NewTimer(duration)
		}

		b.StartTimer()
		// This triggers the O(n log n) sort on ALL waiters
		clock.Advance(500 * time.Millisecond)
		b.StopTimer()

		// Clean up to avoid affecting next iteration
		for _, timer := range timers {
			timer.Stop()
		}
	}
}

// BenchmarkAdvanceWithManyTimers measures the real cost of time advancement
// when there are many active timers - the actual test suite pain point.
func BenchmarkAdvanceWithManyTimers(b *testing.B) {
	scenarios := []struct {
		name        string
		timerCount  int
		advancement time.Duration
		timerSpread time.Duration
	}{
		{"small_burst", 50, 100 * time.Millisecond, 50 * time.Millisecond},
		{"medium_spread", 200, 500 * time.Millisecond, 1 * time.Second},
		{"large_cluster", 500, 200 * time.Millisecond, 100 * time.Millisecond},
		{"huge_distributed", 1000, 1 * time.Second, 2 * time.Second},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Create realistic timer pattern - some will fire, some won't
			for i := 0; i < scenario.timerCount; i++ {
				// Random distribution within spread range
				offset := time.Duration(i) * scenario.timerSpread / time.Duration(scenario.timerCount)
				clock.NewTimer(offset)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// The actual operation that hurts in test suites
				clock.Advance(scenario.advancement)
			}
		})
	}
}

// BenchmarkTickerScaling tests performance with multiple active tickers.
// Tickers are worse than timers because they keep rescheduling themselves.
func BenchmarkTickerScaling(b *testing.B) {
	tickerCounts := []int{5, 10, 25, 50, 100}

	for _, count := range tickerCounts {
		b.Run(fmt.Sprintf("tickers_%d", count), func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Create tickers with realistic periods
			tickers := make([]clockz.Ticker, count)
			for i := 0; i < count; i++ {
				// Period between 10ms and 100ms - realistic test patterns
				period := time.Duration(10+i%90) * time.Millisecond
				tickers[i] = clock.NewTicker(period)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Advance enough to trigger multiple ticker firings
				clock.Advance(200 * time.Millisecond)
			}

			b.StopTimer()
			for _, ticker := range tickers {
				ticker.Stop()
			}
		})
	}
}

// BenchmarkMixedWaiterTypes tests realistic scenarios with timers, tickers, and contexts.
// This reflects actual test suite usage patterns.
func BenchmarkMixedWaiterTypes(b *testing.B) {
	scenarios := []struct {
		name        string
		timers      int
		tickers     int
		contexts    int
		advancement time.Duration
	}{
		{"light_mixed", 20, 5, 10, 100 * time.Millisecond},
		{"heavy_timers", 100, 10, 20, 200 * time.Millisecond},
		{"ticker_heavy", 30, 30, 15, 300 * time.Millisecond},
		{"context_heavy", 50, 10, 100, 150 * time.Millisecond},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			clock := clockz.NewFakeClock()

			// Set up mixed waiter scenario
			var timers []clockz.Timer
			var tickers []clockz.Ticker
			var cancels []context.CancelFunc

			// Create timers with spread durations
			for i := 0; i < scenario.timers; i++ {
				duration := time.Duration(50+i%200) * time.Millisecond
				timers = append(timers, clock.NewTimer(duration))
			}

			// Create tickers with different periods
			for i := 0; i < scenario.tickers; i++ {
				period := time.Duration(25+i%75) * time.Millisecond
				tickers = append(tickers, clock.NewTicker(period))
			}

			// Create contexts with timeout spread
			for i := 0; i < scenario.contexts; i++ {
				timeout := time.Duration(80+i%160) * time.Millisecond
				_, cancel := clock.WithTimeout(context.Background(), timeout)
				cancels = append(cancels, cancel)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// The critical operation that exposes sorting bottleneck
				clock.Advance(scenario.advancement)
			}

			b.StopTimer()

			// Cleanup
			for _, timer := range timers {
				timer.Stop()
			}
			for _, ticker := range tickers {
				ticker.Stop()
			}
			for _, cancel := range cancels {
				cancel()
			}
		})
	}
}
