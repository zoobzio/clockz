package main

import (
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestSimpleTimer tests basic timer functionality
func TestSimpleTimer(t *testing.T) {
	clock := clockz.NewFakeClock()
	
	fired := false
	clock.AfterFunc(50*time.Millisecond, func() {
		fired = true
	})
	
	// Shouldn't fire yet
	clock.Advance(49 * time.Millisecond)
	if fired {
		t.Fatal("Fired too early")
	}
	
	// Should fire now
	clock.Advance(1 * time.Millisecond)
	if !fired {
		t.Fatal("Should have fired")
	}
}

// TestSimpleBuffer tests simplified buffer logic
func TestSimpleBuffer(t *testing.T) {
	clock := clockz.NewFakeClock()
	buffer := NewEventBufferWithClock(100, 50*time.Millisecond, clock)
	
	// Add events
	for i := 0; i < 50; i++ {
		buffer.Add(Event{ID: i})
	}
	
	// Start receiver before advancing time
	done := make(chan []Event, 1)
	go func() {
		events := buffer.WaitForFlush()
		if events != nil {
			done <- events
		}
	}()
	
	// Let goroutine start
	time.Sleep(10 * time.Millisecond)
	
	// Trigger flush
	clock.Advance(50 * time.Millisecond)
	
	// Get result
	select {
	case events := <-done:
		if len(events) != 50 {
			t.Fatalf("Expected 50 events, got %d", len(events))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for flush")
	}
	
	buffer.Close()
}