package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestScheduler_FlakeyMidnight shows the infamous midnight bug
func TestScheduler_FlakeyMidnight(t *testing.T) {
	t.Skip("Skipping flaky midnight test - only passes at midnight!")
	
	scheduler := NewScheduler()
	
	jobRan := false
	scheduler.AddDaily("00:00", func() error {
		jobRan = true
		return nil
	})
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// This test literally only passes if run at midnight!
	time.Sleep(2 * time.Second)
	
	if !jobRan {
		// Fails 23 hours and 59 minutes per day
		t.Fatal("Daily job should have run")
	}
}

// TestScheduler_DeterministicDaily shows how to properly test scheduled jobs
func TestScheduler_DeterministicDaily(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Date(2024, 10, 3, 23, 59, 0, 0, time.UTC))
	scheduler := NewSchedulerWithClock(clock)
	
	var jobRuns int32
	scheduler.AddDaily("00:00", func() error {
		atomic.AddInt32(&jobRuns, 1)
		return nil
	})
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// Advance to just before midnight
	clock.Advance(59 * time.Second)
	clock.BlockUntilReady()
	
	// Job shouldn't have run yet
	if atomic.LoadInt32(&jobRuns) != 0 {
		t.Fatal("Job ran too early")
	}
	
	// Advance to midnight
	clock.Advance(1 * time.Second)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let job execute
	
	// Job should run exactly once
	if runs := atomic.LoadInt32(&jobRuns); runs != 1 {
		t.Fatalf("Expected 1 run at midnight, got %d", runs)
	}
	
	// Advance another day
	clock.Advance(24 * time.Hour)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let job execute
	
	// Should run again
	if runs := atomic.LoadInt32(&jobRuns); runs != 2 {
		t.Fatalf("Expected 2 runs after 24 hours, got %d", runs)
	}
}

// TestScheduler_FlakyRetry shows non-deterministic retry testing
func TestScheduler_FlakyRetry(t *testing.T) {
	t.Skip("Skipping flaky retry test")
	
	scheduler := NewScheduler()
	attempts := 0
	
	scheduler.AddJobWithRetry("retry-job", 
		IntervalSchedule{Interval: 10 * time.Second},
		func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		}, 3, time.Second)
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// Wait for retries... but timing is unpredictable
	// 1s + 2s + 4s = 7s minimum, but could be more
	time.Sleep(8 * time.Second)
	
	// This often fails due to scheduling delays
	if attempts != 3 {
		t.Fatalf("Expected 3 attempts, got %d", attempts)
	}
}

// TestScheduler_DeterministicRetry shows precise retry testing
func TestScheduler_DeterministicRetry(t *testing.T) {
	clock := clockz.NewFakeClock()
	scheduler := NewSchedulerWithClock(clock)
	
	var attempts int32
	attemptTimes := make([]time.Time, 0)
	
	scheduler.AddJobWithRetry("retry-job",
		IntervalSchedule{Interval: time.Minute},
		func() error {
			attempt := atomic.AddInt32(&attempts, 1)
			attemptTimes = append(attemptTimes, clock.Now())
			
			if attempt < 3 {
				return errors.New("temporary failure")
			}
			return nil
		}, 3, time.Second)
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// Trigger first run immediately
	clock.Advance(time.Minute)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let goroutine start
	
	// First attempt should fail
	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatal("First attempt should have executed")
	}
	
	// Advance for first retry (1 second backoff)
	clock.Advance(time.Second)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	// Second attempt should fail
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatal("Second attempt should have executed")
	}
	
	// Advance for second retry (2 second backoff - exponential)
	clock.Advance(2 * time.Second)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	// Third attempt should succeed
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatal("Third attempt should have executed")
	}
	
	// Verify exponential backoff timing
	if len(attemptTimes) >= 3 {
		gap1 := attemptTimes[1].Sub(attemptTimes[0])
		gap2 := attemptTimes[2].Sub(attemptTimes[1])
		
		if gap1 != time.Second {
			t.Fatalf("First retry gap should be 1s, got %v", gap1)
		}
		if gap2 != 2*time.Second {
			t.Fatalf("Second retry gap should be 2s, got %v", gap2)
		}
	}
}

// TestScheduler_CronExpression tests cron-like scheduling
func TestScheduler_CronExpression(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Date(2024, 10, 3, 14, 29, 0, 0, time.UTC))
	scheduler := NewSchedulerWithClock(clock)
	
	var runs int32
	
	// Every minute at :30 seconds
	schedule := CronSchedule{Minute: 30, Hour: -1, Day: -1}
	scheduler.AddJob("cron-job", schedule, func() error {
		atomic.AddInt32(&runs, 1)
		return nil
	})
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// Advance to 14:30
	clock.Advance(time.Minute)
	clock.BlockUntilReady()
	
	if atomic.LoadInt32(&runs) != 1 {
		t.Fatal("Job should run at :30")
	}
	
	// Advance to 14:31 - shouldn't run
	clock.Advance(time.Minute)
	clock.BlockUntilReady()
	
	if atomic.LoadInt32(&runs) != 1 {
		t.Fatal("Job shouldn't run at :31")
	}
	
	// Advance to 15:30 - should run again
	clock.Advance(59 * time.Minute)
	clock.BlockUntilReady()
	
	if atomic.LoadInt32(&runs) != 2 {
		t.Fatal("Job should run again at :30")
	}
}

// TestScheduler_MaintenanceWindow tests jobs respecting maintenance
func TestScheduler_MaintenanceWindow(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Date(2024, 10, 3, 11, 0, 0, 0, time.UTC))
	scheduler := NewSchedulerWithMaintenance(clock)
	
	// Add maintenance window from 12:00 to 13:00
	maintenanceStart := time.Date(2024, 10, 3, 12, 0, 0, 0, time.UTC)
	maintenanceEnd := time.Date(2024, 10, 3, 13, 0, 0, 0, time.UTC)
	scheduler.AddMaintenanceWindow(maintenanceStart, maintenanceEnd)
	
	var runs int32
	scheduler.AddInterval(30*time.Minute, func() error {
		// Check if we're in maintenance
		if scheduler.IsInMaintenance() {
			return errors.New("in maintenance")
		}
		atomic.AddInt32(&runs, 1)
		return nil
	})
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// First run at 11:30 - before maintenance
	clock.Advance(30 * time.Minute)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	if atomic.LoadInt32(&runs) != 1 {
		t.Fatal("Job should run before maintenance")
	}
	
	// Second attempt at 12:00 - during maintenance
	clock.Advance(30 * time.Minute)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	if atomic.LoadInt32(&runs) != 1 {
		t.Fatal("Job shouldn't run during maintenance")
	}
	
	// Third attempt at 12:30 - still during maintenance
	clock.Advance(30 * time.Minute)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	if atomic.LoadInt32(&runs) != 1 {
		t.Fatal("Job shouldn't run during maintenance")
	}
	
	// Fourth attempt at 13:00 - after maintenance
	clock.Advance(30 * time.Minute)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	if atomic.LoadInt32(&runs) != 2 {
		t.Fatal("Job should run after maintenance")
	}
}

// TestScheduler_JobChain tests dependent job execution
func TestScheduler_JobChain(t *testing.T) {
	clock := clockz.NewFakeClock()
	scheduler := NewSchedulerWithClock(clock)
	
	var executionOrder []string
	
	chain := NewJobChain(scheduler).
		Then(func() error {
			executionOrder = append(executionOrder, "extract")
			return nil
		}).
		Then(func() error {
			executionOrder = append(executionOrder, "transform")
			return nil
		}).
		Then(func() error {
			executionOrder = append(executionOrder, "load")
			return nil
		})
	
	// Schedule the chain
	scheduler.AddInterval(time.Hour, func() error {
		return chain.Run()
	})
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// Trigger chain execution
	clock.Advance(time.Hour)
	clock.BlockUntilReady()
	time.Sleep(10 * time.Millisecond)
	
	// Verify execution order
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 jobs, got %d", len(executionOrder))
	}
	
	if executionOrder[0] != "extract" || executionOrder[1] != "transform" || executionOrder[2] != "load" {
		t.Fatalf("Wrong execution order: %v", executionOrder)
	}
}

// TestScheduler_ChainWithFailure tests chain failure handling
func TestScheduler_ChainWithFailure(t *testing.T) {
	clock := clockz.NewFakeClock()
	scheduler := NewSchedulerWithClock(clock)
	
	var executionOrder []string
	var errorStep int
	
	chain := NewJobChain(scheduler).
		Then(func() error {
			executionOrder = append(executionOrder, "step1")
			return nil
		}).
		Then(func() error {
			executionOrder = append(executionOrder, "step2")
			return errors.New("step2 failed")
		}).
		Then(func() error {
			executionOrder = append(executionOrder, "step3")
			return nil
		}).
		OnError(func(step int, err error) {
			errorStep = step
		})
	
	err := chain.Run()
	
	// Should fail at step 2
	if err == nil {
		t.Fatal("Chain should have failed")
	}
	
	// Only first two steps should execute
	if len(executionOrder) != 2 {
		t.Fatalf("Expected 2 steps executed, got %d", len(executionOrder))
	}
	
	// Error handler should be called with step 1 (0-indexed)
	if errorStep != 1 {
		t.Fatalf("Expected error at step 1, got %d", errorStep)
	}
}

// TestScheduler_ComplexScenario tests a realistic ETL pipeline
func TestScheduler_ComplexScenario(t *testing.T) {
	// Start at 11 PM
	clock := clockz.NewFakeClockAt(time.Date(2024, 10, 3, 23, 0, 0, 0, time.UTC))
	scheduler := NewSchedulerWithMaintenance(clock)
	
	// Maintenance window 2-3 AM
	scheduler.AddMaintenanceWindow(
		time.Date(2024, 10, 4, 2, 0, 0, 0, time.UTC),
		time.Date(2024, 10, 4, 3, 0, 0, 0, time.UTC),
	)
	
	var events []string
	
	// Hourly health check
	scheduler.AddInterval(time.Hour, func() error {
		if scheduler.IsInMaintenance() {
			events = append(events, "health-check-skipped")
			return nil
		}
		events = append(events, "health-check")
		return nil
	})
	
	// Daily ETL at midnight
	scheduler.AddDaily("00:00", func() error {
		if scheduler.IsInMaintenance() {
			events = append(events, "etl-skipped")
			return nil
		}
		
		// Simulate ETL chain
		chain := NewJobChain(scheduler.Scheduler).
			Then(func() error {
				events = append(events, "etl-extract")
				return nil
			}).
			Then(func() error {
				events = append(events, "etl-transform")
				return nil
			}).
			Then(func() error {
				events = append(events, "etl-load")
				return nil
			})
		
		return chain.Run()
	})
	
	// Cleanup job with retries every 6 hours
	var cleanupAttempts int
	scheduler.AddJobWithRetry("cleanup",
		IntervalSchedule{Interval: 6 * time.Hour},
		func() error {
			cleanupAttempts++
			events = append(events, "cleanup-attempt")
			if cleanupAttempts == 1 {
				return errors.New("cleanup failed")
			}
			events = append(events, "cleanup-success")
			return nil
		}, 2, time.Second)
	
	scheduler.Start()
	defer scheduler.Stop()
	
	// Simulate 8 hours of operations
	for hour := 0; hour < 8; hour++ {
		clock.Advance(time.Hour)
		clock.BlockUntilReady()
		time.Sleep(10 * time.Millisecond) // Let jobs execute
		
		// For retry handling
		if hour == 5 { // 6 hours for cleanup trigger
			clock.Advance(time.Second) // Retry delay
			clock.BlockUntilReady()
			time.Sleep(10 * time.Millisecond)
		}
	}
	
	// Verify the sequence of events
	expectedPatterns := []string{
		"health-check",        // 11 PM -> midnight
		"etl-extract",         // Midnight ETL
		"etl-transform",
		"etl-load",
		"health-check",        // 1 AM
		"health-check-skipped", // 2 AM (maintenance)
		"health-check",        // 3 AM (after maintenance)
		"cleanup-attempt",     // 5 AM (6 hours from start)
		"cleanup-attempt",     // Retry after 1 second
		"cleanup-success",     // Retry succeeds
	}
	
	// Check that expected events occurred
	for _, expected := range expectedPatterns {
		found := false
		for _, event := range events {
			if event == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected event %q not found in %v", expected, events)
		}
	}
	
	// Test completes in milliseconds instead of 8 hours!
}