package syncdaemon

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDebounceFlushExitsWhenConsumerReturned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan WatchEvent, 4)
	batches := Debounce(ctx, in, 10*time.Millisecond)

	// Produce a batch, drain it, then stop consuming entirely.
	in <- WatchEvent{Path: "notes/a.md"}
	in <- WatchEvent{Path: "notes/b.md"}
	select {
	case batch := <-batches:
		if len(batch) == 0 {
			t.Fatalf("unexpected empty batch: %#v", batch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("debounce did not deliver the first batch")
	}

	in <- WatchEvent{Path: "notes/c.md"}
	time.Sleep(50 * time.Millisecond) // let the timer fire into a blocking flush
	cancel()

	// The debounce goroutine must exit (close(batches)) instead of hanging on
	// the blocked flush send now that ctx is cancelled and nobody reads.
	select {
	case _, ok := <-batches:
		if ok {
			// A final batch may or may not be delivered; what matters is that
			// the channel eventually closes.
			select {
			case _, stillOpen := <-batches:
				if stillOpen {
					t.Fatal("debounce channel still open after cancel")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("debounce channel did not close after cancel")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("debounce channel did not close after cancel")
	}
}

func TestForwardDropsWatcherErrorBurstWithoutBlocking(t *testing.T) {
	// forward() delegates each error to deliverWatcherError, which must never
	// block: emit a burst of errors into an undrained cap-1 channel and assert
	// the first is delivered, the rest are dropped, and every call returns.
	errs := make(chan error, 1)
	if !deliverWatcherError(errs, errors.New("first failure")) {
		t.Fatal("first error should be delivered into the empty channel")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Undrained channel: the second error must be dropped, not block.
		if deliverWatcherError(errs, errors.New("second failure")) {
			t.Error("second error unexpectedly delivered to full channel")
		}
		if deliverWatcherError(errs, errors.New("third failure")) {
			t.Error("third error unexpectedly delivered to full channel")
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deliverWatcherError blocked on a full error channel")
	}
	if got := <-errs; got.Error() != "first failure" {
		t.Fatalf("delivered error = %v, want first failure", got)
	}
}
