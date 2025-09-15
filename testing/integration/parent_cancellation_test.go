package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestParentCancellationContract verifies the context contract.
// "When parent context is canceled, all child contexts must be canceled"
//
//nolint:godot // Multi-line comment with period on first line
func TestParentCancellationContract(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Unix(1000, 0))

	// Create parent context that we can cancel
	parent, parentCancel := context.WithCancel(context.Background())

	// Create child timeout context with distant deadline
	child, childCancel := clock.WithTimeout(parent, 1*time.Hour)
	defer childCancel()

	// Verify initial state
	select {
	case <-parent.Done():
		t.Fatal("parent canceled prematurely")
	default:
	}

	select {
	case <-child.Done():
		t.Fatal("child canceled prematurely")
	default:
	}

	// Cancel parent
	parentCancel()

	// Parent should be canceled immediately
	select {
	case <-parent.Done():
		if parent.Err() != context.Canceled {
			t.Fatalf("parent: expected Canceled, got %v", parent.Err())
		}
	default:
		t.Fatal("parent not canceled after parentCancel()")
	}

	// Child MUST be canceled due to parent cancellation
	// This is the critical context contract
	select {
	case <-child.Done():
		// Child must inherit parent's cancellation error
		if child.Err() != context.Canceled {
			t.Fatalf("child: expected Canceled from parent, got %v", child.Err())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("CONTEXT CONTRACT VIOLATION: child not canceled when parent canceled")
	}
}

// TestParentCancellationBeforeTimeout verifies parent cancellation.
// takes precedence over timeout
//
//nolint:godot // Multi-line comment with period on first line
func TestParentCancellationBeforeTimeout(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Unix(1000, 0))

	parent, parentCancel := context.WithCancel(context.Background())
	child, childCancel := clock.WithTimeout(parent, 5*time.Second)
	defer childCancel()

	// Cancel parent before timeout
	parentCancel()

	// Wait for propagation
	time.Sleep(10 * time.Millisecond)

	// Child should have parent's error, not DeadlineExceeded
	select {
	case <-child.Done():
		if child.Err() != context.Canceled {
			t.Fatalf("expected Canceled, got %v", child.Err())
		}
	default:
		t.Fatal("child not canceled after parent cancellation")
	}

	// Advancing past timeout shouldn't change error
	clock.Advance(10 * time.Second)
	clock.BlockUntilReady()

	if child.Err() != context.Canceled {
		t.Fatalf("error changed after timeout: got %v", child.Err())
	}
}

// TestParentCancellationConcurrency tests concurrent parent cancellations.
func TestParentCancellationConcurrency(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Unix(1000, 0))

	const numContexts = 50
	parents := make([]context.Context, numContexts)
	parentCancels := make([]context.CancelFunc, numContexts)
	children := make([]context.Context, numContexts)
	childCancels := make([]context.CancelFunc, numContexts)

	// Create parent-child pairs
	for i := 0; i < numContexts; i++ {
		parents[i], parentCancels[i] = context.WithCancel(context.Background())
		children[i], childCancels[i] = clock.WithTimeout(parents[i], 1*time.Hour)
		//nolint:gocritic // deferInLoop: Each context needs specific cleanup
		defer childCancels[i]()
	}

	// Cancel all parents concurrently
	var wg sync.WaitGroup
	for i := 0; i < numContexts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			parentCancels[idx]()
		}(i)
	}
	wg.Wait()

	// Brief wait for propagation
	time.Sleep(50 * time.Millisecond)

	// Verify all children are canceled
	for i := 0; i < numContexts; i++ {
		select {
		case <-children[i].Done():
			if children[i].Err() != context.Canceled {
				t.Errorf("child %d: expected Canceled, got %v", i, children[i].Err())
			}
		default:
			t.Errorf("child %d: not canceled after parent cancellation", i)
		}
	}
}

// TestParentCancellationCleanup verifies proper resource cleanup.
func TestParentCancellationCleanup(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Unix(1000, 0))

	// Create and cancel many parent-child pairs
	for i := 0; i < 100; i++ {
		parent, parentCancel := context.WithCancel(context.Background())
		child, childCancel := clock.WithTimeout(parent, 1*time.Hour)

		// Cancel parent
		parentCancel()

		// Wait for child cancellation
		select {
		case <-child.Done():
			// Expected
		case <-time.After(10 * time.Millisecond):
			t.Fatalf("iteration %d: child not canceled", i)
		}

		childCancel() // Cleanup
	}

	// Brief wait for all cleanup
	time.Sleep(10 * time.Millisecond)

	// Clock should have no waiters
	if clock.HasWaiters() {
		t.Fatal("clock has waiters after parent cancellation cleanup")
	}
}

// TestNestedContextCancellation tests propagation through nested contexts.
func TestNestedContextCancellation(t *testing.T) {
	clock := clockz.NewFakeClockAt(time.Unix(1000, 0))

	// Create context chain: root -> parent -> child1 -> child2
	root, rootCancel := context.WithCancel(context.Background())
	parent, parentCancel := clock.WithTimeout(root, 1*time.Hour)
	child1, child1Cancel := clock.WithTimeout(parent, 30*time.Minute)
	child2, child2Cancel := clock.WithTimeout(child1, 10*time.Minute)

	defer rootCancel()
	defer parentCancel()
	defer child1Cancel()
	defer child2Cancel()

	// Cancel root
	rootCancel()

	// Wait for propagation through chain
	time.Sleep(50 * time.Millisecond)

	// All contexts in chain should be canceled
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{"root", root},
		{"parent", parent},
		{"child1", child1},
		{"child2", child2},
	}

	for _, tc := range contexts {
		select {
		case <-tc.ctx.Done():
			if tc.ctx.Err() != context.Canceled {
				t.Errorf("%s: expected Canceled, got %v", tc.name, tc.ctx.Err())
			}
		default:
			t.Errorf("%s: not canceled after root cancellation", tc.name)
		}
	}
}

// TestParentCancellationRaceWithTimeout tests race between parent cancel and timeout.
func TestParentCancellationRaceWithTimeout(t *testing.T) {
	for i := 0; i < 20; i++ {
		clock := clockz.NewFakeClockAt(time.Unix(1000, 0))
		parent, parentCancel := context.WithCancel(context.Background())
		child, childCancel := clock.WithTimeout(parent, 100*time.Millisecond)

		var wg sync.WaitGroup
		wg.Add(2)

		// Race: parent cancellation
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(i) * time.Millisecond) // Vary timing
			parentCancel()
		}()

		// Race: timeout
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(20-i) * time.Millisecond) // Inverse timing
			clock.Advance(200 * time.Millisecond)
			clock.BlockUntilReady()
		}()

		wg.Wait()

		// Child must be canceled (either reason is valid in a race)
		select {
		case <-child.Done():
			err := child.Err()
			if err != context.Canceled && err != context.DeadlineExceeded {
				t.Fatalf("iteration %d: unexpected error %v", i, err)
			}
		default:
			t.Fatalf("iteration %d: child not canceled", i)
		}

		childCancel()
	}
}
