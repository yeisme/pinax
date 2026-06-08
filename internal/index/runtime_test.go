package index

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndexEventAssignsSequenceEpochAndTimestamp(t *testing.T) {
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{QueueSize: 2, Epoch: 7, Now: func() time.Time { return now }})

	event, err := coordinator.Emit(context.Background(), IndexEvent{Kind: IndexEventNoteChanged, Path: "notes/a.md", ContentHash: "h1"})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	if event.Seq != 1 || event.Epoch != 7 || !event.EmittedAt.Equal(now) {
		t.Fatalf("event sequencing = %#v", event)
	}
}

func TestIndexCoordinatorCoalescesDuplicateEvents(t *testing.T) {
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{QueueSize: 8, Epoch: 3})
	for _, hash := range []string{"old", "new"} {
		if _, err := coordinator.Emit(context.Background(), IndexEvent{Kind: IndexEventNoteChanged, Path: "notes/a.md", ContentHash: hash}); err != nil {
			t.Fatalf("emit %s: %v", hash, err)
		}
	}

	processed := []IndexEvent{}
	if err := coordinator.ProcessReady(context.Background(), func(_ context.Context, event IndexEvent) error {
		processed = append(processed, event)
		return nil
	}); err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(processed) != 1 || processed[0].ContentHash != "new" {
		t.Fatalf("processed events = %#v", processed)
	}
	facts := coordinator.RuntimeFacts()
	if facts["queued"] != "0" || facts["parsed"] != "1" || facts["indexed"] != "1" || facts["failed"] != "0" || facts["epoch"] != "3" {
		t.Fatalf("runtime facts = %#v", facts)
	}
}

func TestIndexCoordinatorBoundedQueueHonorsContext(t *testing.T) {
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{QueueSize: 1})
	if _, err := coordinator.Emit(context.Background(), IndexEvent{Kind: IndexEventNoteChanged, Path: "notes/a.md"}); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.Emit(ctx, IndexEvent{Kind: IndexEventNoteChanged, Path: "notes/b.md"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("second emit err = %v", err)
	}

	facts := coordinator.RuntimeFacts()
	if facts["queued"] != "1" || facts["failed"] != "1" {
		t.Fatalf("runtime facts = %#v", facts)
	}
}

func TestIndexCoordinatorContextCancellationStopsProcessing(t *testing.T) {
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{QueueSize: 2})
	if _, err := coordinator.Emit(context.Background(), IndexEvent{Kind: IndexEventRebuildRequested}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := coordinator.ProcessReady(ctx, func(context.Context, IndexEvent) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("process err = %v", err)
	}
}

func TestDiscardStaleResult(t *testing.T) {
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{Epoch: 1})
	coordinator.BeginEpoch()
	called := false
	err := coordinator.CommitWriteBatch(context.Background(), IndexWriteBatch{Epoch: 1, Commit: func(context.Context) error {
		called = true
		return nil
	}})
	if !errors.Is(err, ErrStaleIndexEpoch) {
		t.Fatalf("commit err = %v", err)
	}
	if called {
		t.Fatalf("stale batch commit was called")
	}
}

func TestSingleWriter(t *testing.T) {
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{Epoch: 4})
	var active int64
	var maxActive int64
	var commits int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := coordinator.CommitWriteBatch(context.Background(), IndexWriteBatch{Epoch: 4, Commit: func(context.Context) error {
				current := atomic.AddInt64(&active, 1)
				for {
					old := atomic.LoadInt64(&maxActive)
					if current <= old || atomic.CompareAndSwapInt64(&maxActive, old, current) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				atomic.AddInt64(&active, -1)
				atomic.AddInt64(&commits, 1)
				return nil
			}})
			if err != nil {
				t.Errorf("commit: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if commits != 16 || maxActive != 1 {
		t.Fatalf("single writer violated: commits=%d maxActive=%d", commits, maxActive)
	}
}

func TestConcurrentIncremental(t *testing.T) {
	coordinator := NewIndexCoordinator(IndexCoordinatorOptions{Epoch: 9})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = coordinator.CommitWriteBatch(context.Background(), IndexWriteBatch{Epoch: 9, Commit: func(context.Context) error { return nil }})
		}()
	}
	wg.Wait()
	if facts := coordinator.RuntimeFacts(); facts["failed"] != "0" {
		t.Fatalf("runtime facts = %#v", facts)
	}
}
