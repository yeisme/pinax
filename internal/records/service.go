package records

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	SchemaVersion         = "pinax.records.v1"
	EventSchemaVersion    = "pinax.record_event.v1"
	RegistrySchemaVersion = "pinax.record_registry.v1"
)

type Service struct {
	root string
	now  func() time.Time
	mu   sync.Mutex
}

func NewService(root string) *Service {
	return &Service{root: root, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Init(ctx context.Context) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(s.recordsDir(), 0o755); err != nil {
		return err
	}
	for _, file := range []struct {
		path string
		body []byte
	}{
		{s.eventsPath(), nil},
		{s.registryPath(), mustJSON(domain.LedgerState{SchemaVersion: RegistrySchemaVersion, Records: map[string]domain.NoteRecord{}, Tombstones: map[string]domain.Tombstone{}, Version: domain.LedgerVersion{SchemaVersion: SchemaVersion}})},
		{s.tombstonesPath(), mustJSON(map[string]domain.Tombstone{})},
		{s.versionPath(), mustJSON(domain.LedgerVersion{SchemaVersion: SchemaVersion})},
	} {
		if _, err := os.Stat(file.path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(file.path, file.body, 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AppendEvent(ctx context.Context, event domain.RecordEvent) (domain.RecordEvent, error) {
	if err := ctxErr(ctx); err != nil {
		return domain.RecordEvent{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Init(ctx); err != nil {
		return domain.RecordEvent{}, err
	}
	events, err := s.readEvents()
	if err != nil {
		return domain.RecordEvent{}, err
	}
	for _, existing := range events {
		if event.IdempotencyKey != "" && existing.IdempotencyKey == event.IdempotencyKey {
			return existing, nil
		}
	}
	state, err := materialize(events)
	if err != nil {
		return domain.RecordEvent{}, err
	}
	event.SchemaVersion = EventSchemaVersion
	event.Seq = uint64(len(events) + 1)
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = fmt.Sprintf("record_evt_%06d", event.Seq)
	}
	if strings.TrimSpace(event.CreatedAt) == "" {
		event.CreatedAt = s.now().Format(time.RFC3339)
	}
	if err := applyRecordEvent(&state, event); err != nil {
		return domain.RecordEvent{}, err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return domain.RecordEvent{}, err
	}
	f, err := os.OpenFile(s.eventsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return domain.RecordEvent{}, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return domain.RecordEvent{}, err
	}
	if err := f.Close(); err != nil {
		return domain.RecordEvent{}, err
	}
	if err := s.writeState(state); err != nil {
		return domain.RecordEvent{}, err
	}
	return event, nil
}

func (s *Service) Replay(ctx context.Context) (domain.LedgerState, error) {
	if err := ctxErr(ctx); err != nil {
		return domain.LedgerState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Init(ctx); err != nil {
		return domain.LedgerState{}, err
	}
	events, err := s.readEvents()
	if err != nil {
		return domain.LedgerState{}, err
	}
	state, err := materialize(events)
	if err != nil {
		return domain.LedgerState{}, err
	}
	if err := s.writeState(state); err != nil {
		return domain.LedgerState{}, err
	}
	return state, nil
}

func materialize(events []domain.RecordEvent) (domain.LedgerState, error) {
	state := domain.LedgerState{SchemaVersion: RegistrySchemaVersion, Records: map[string]domain.NoteRecord{}, Tombstones: map[string]domain.Tombstone{}, Version: domain.LedgerVersion{SchemaVersion: SchemaVersion}}
	for _, event := range events {
		if err := applyRecordEvent(&state, event); err != nil {
			return domain.LedgerState{}, err
		}
	}
	return state, nil
}

func applyRecordEvent(state *domain.LedgerState, event domain.RecordEvent) error {
	if strings.TrimSpace(event.NoteID) == "" {
		return &domain.CommandError{Code: "record_note_id_required", Message: "record event 缺少 note_id"}
	}
	record := state.Records[event.NoteID]
	transition := func(lifecycle domain.NoteLifecycle) {
		record.NoteID = event.NoteID
		if event.Path != "" {
			record.Path = event.Path
		}
		if event.Title != "" {
			record.Title = event.Title
		}
		record.Lifecycle = lifecycle
		record.RecordVersion++
		record.LedgerSeq = event.Seq
		if event.ContentRevision.Hash != "" || event.ContentRevision.Size != 0 || event.ContentRevision.ModifiedUnix != 0 {
			record.ContentRevision = event.ContentRevision
		}
		if event.VersionEvidence.Backend != "" {
			record.VersionEvidence = event.VersionEvidence
		}
		state.Records[event.NoteID] = record
		state.Version.LastSeq = event.Seq
		state.Version.UpdatedAt = event.CreatedAt
	}
	switch event.Kind {
	case domain.RecordEventNoteCreated:
		if record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(domain.NoteLifecycleActive)
	case domain.RecordEventNoteRenamed:
		if record.NoteID == "" || record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(record.Lifecycle)
	case domain.RecordEventNoteMoved:
		if record.NoteID == "" || record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(record.Lifecycle)
	case domain.RecordEventNoteArchived:
		if record.NoteID == "" || record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(domain.NoteLifecycleArchived)
	case domain.RecordEventNoteTrashed:
		if record.NoteID == "" || record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(domain.NoteLifecycleTrashed)
	case domain.RecordEventNoteDeleted:
		if record.NoteID == "" || record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		oldPath := record.Path
		oldHash := record.ContentRevision.Hash
		transition(domain.NoteLifecycleDeleted)
		state.Tombstones[event.NoteID] = domain.Tombstone{NoteID: event.NoteID, OldPath: oldPath, OldHash: oldHash, Title: record.Title, DeletedAt: event.CreatedAt, Source: string(event.Kind), Evidence: event.Evidence}
	case domain.RecordEventNoteRestored:
		if record.NoteID == "" || record.Lifecycle != domain.NoteLifecycleDeleted && record.Lifecycle != domain.NoteLifecycleTrashed {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(domain.NoteLifecycleActive)
		delete(state.Tombstones, event.NoteID)
	case domain.RecordEventNoteMetadataUpdated:
		if record.NoteID == "" || record.Lifecycle == domain.NoteLifecycleDeleted {
			return invalidTransition(event, record.Lifecycle)
		}
		transition(record.Lifecycle)
	default:
		return &domain.CommandError{Code: "record_event_kind_invalid", Message: "record event kind 不受支持"}
	}
	return nil
}

func invalidTransition(event domain.RecordEvent, current domain.NoteLifecycle) error {
	return &domain.CommandError{Code: "record_lifecycle_invalid", Message: fmt.Sprintf("record lifecycle transition invalid: %s from %s", event.Kind, current)}
}

func (s *Service) readEvents() ([]domain.RecordEvent, error) {
	f, err := os.Open(s.eventsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	events := []domain.RecordEvent{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event domain.RecordEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, &domain.CommandError{Code: "record_event_log_corrupt", Message: fmt.Sprintf("events.jsonl 第 %d 行不是合法 JSON", lineNo)}
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Service) writeState(state domain.LedgerState) error {
	if err := writeJSON(s.registryPath(), state); err != nil {
		return err
	}
	if err := writeJSON(s.tombstonesPath(), state.Tombstones); err != nil {
		return err
	}
	return writeJSON(s.versionPath(), state.Version)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func mustJSON(value any) []byte {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(body, '\n')
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (s *Service) recordsDir() string     { return filepath.Join(s.root, ".pinax", "records") }
func (s *Service) eventsPath() string     { return filepath.Join(s.recordsDir(), "events.jsonl") }
func (s *Service) registryPath() string   { return filepath.Join(s.recordsDir(), "notes.json") }
func (s *Service) tombstonesPath() string { return filepath.Join(s.recordsDir(), "tombstones.json") }
func (s *Service) versionPath() string    { return filepath.Join(s.recordsDir(), "version.json") }
