package index

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrStaleIndexEpoch = errors.New("stale index epoch")

type IndexEventKind string

const (
	IndexEventNoteChanged      IndexEventKind = "note_changed"
	IndexEventNoteMoved        IndexEventKind = "note_moved"
	IndexEventNoteDeleted      IndexEventKind = "note_deleted"
	IndexEventRebuildRequested IndexEventKind = "rebuild_requested"
)

type IndexEvent struct {
	Seq         uint64         `json:"seq"`
	Epoch       uint64         `json:"epoch"`
	Kind        IndexEventKind `json:"kind"`
	OldPath     string         `json:"old_path,omitempty"`
	Path        string         `json:"path,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	EmittedAt   time.Time      `json:"emitted_at"`
}

type IndexWriteBatch struct {
	Epoch  uint64
	Commit func(context.Context) error
}

type IndexCoordinatorOptions struct {
	QueueSize int
	Epoch     uint64
	Now       func() time.Time
}

type IndexCoordinator struct {
	events  chan IndexEvent
	now     func() time.Time
	writer  sync.Mutex
	seq     atomic.Uint64
	epoch   atomic.Uint64
	queued  atomic.Int64
	parsed  atomic.Int64
	indexed atomic.Int64
	failed  atomic.Int64
}

func NewIndexCoordinator(opts IndexCoordinatorOptions) *IndexCoordinator {
	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = 64
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	c := &IndexCoordinator{events: make(chan IndexEvent, queueSize), now: now}
	c.epoch.Store(opts.Epoch)
	return c
}

func (c *IndexCoordinator) Emit(ctx context.Context, event IndexEvent) (IndexEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	event.Seq = c.seq.Add(1)
	event.Epoch = c.epoch.Load()
	event.EmittedAt = c.now()
	select {
	case <-ctx.Done():
		c.failed.Add(1)
		return event, ctx.Err()
	case c.events <- event:
		c.queued.Add(1)
		return event, nil
	}
}

func (c *IndexCoordinator) ProcessReady(ctx context.Context, handle func(context.Context, IndexEvent) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		c.failed.Add(1)
		return err
	}
	events := c.drainCoalesced()
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			c.failed.Add(1)
			return err
		}
		c.parsed.Add(1)
		if handle != nil {
			if err := handle(ctx, event); err != nil {
				c.failed.Add(1)
				return err
			}
		}
		c.indexed.Add(1)
	}
	return nil
}

func (c *IndexCoordinator) RuntimeFacts() map[string]string {
	return map[string]string{
		"queued":  strconv.FormatInt(c.queued.Load(), 10),
		"parsed":  strconv.FormatInt(c.parsed.Load(), 10),
		"indexed": strconv.FormatInt(c.indexed.Load(), 10),
		"failed":  strconv.FormatInt(c.failed.Load(), 10),
		"epoch":   strconv.FormatUint(c.epoch.Load(), 10),
	}
}

func (c *IndexCoordinator) BeginEpoch() uint64 {
	return c.epoch.Add(1)
}

func (c *IndexCoordinator) CommitWriteBatch(ctx context.Context, batch IndexWriteBatch) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		c.failed.Add(1)
		return err
	}
	if batch.Epoch != c.epoch.Load() {
		return ErrStaleIndexEpoch
	}
	c.writer.Lock()
	defer c.writer.Unlock()
	if err := ctx.Err(); err != nil {
		c.failed.Add(1)
		return err
	}
	if batch.Epoch != c.epoch.Load() {
		return ErrStaleIndexEpoch
	}
	if batch.Commit == nil {
		c.indexed.Add(1)
		return nil
	}
	if err := batch.Commit(ctx); err != nil {
		c.failed.Add(1)
		return err
	}
	c.indexed.Add(1)
	return nil
}

func (c *IndexCoordinator) drainCoalesced() []IndexEvent {
	latest := map[string]IndexEvent{}
	for {
		select {
		case event := <-c.events:
			c.queued.Add(-1)
			latest[indexEventCoalesceKey(event)] = event
		default:
			if len(latest) == 0 {
				return nil
			}
			events := make([]IndexEvent, 0, len(latest))
			for _, event := range latest {
				events = append(events, event)
			}
			sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
			return events
		}
	}
}

func indexEventCoalesceKey(event IndexEvent) string {
	switch event.Kind {
	case IndexEventRebuildRequested:
		return string(IndexEventRebuildRequested)
	case IndexEventNoteMoved:
		return string(event.Kind) + ":" + strings.TrimSpace(event.OldPath) + "->" + strings.TrimSpace(event.Path)
	default:
		path := strings.TrimSpace(event.Path)
		if path == "" {
			path = strings.TrimSpace(event.OldPath)
		}
		if path == "" {
			path = fmt.Sprint(event.Seq)
		}
		return string(event.Kind) + ":" + path
	}
}
