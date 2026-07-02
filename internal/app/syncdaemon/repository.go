package syncdaemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/redaction"
)

type Repository struct{ Root string }

func NewRepository(root string) Repository { return Repository{Root: root} }

func (r Repository) Dir() string { return filepath.Join(r.Root, ".pinax", "sync-daemon") }

func (r Repository) StatePath() string { return filepath.Join(r.Dir(), "daemon.json") }

func (r Repository) EventsPath() string { return filepath.Join(r.Dir(), "events.jsonl") }

func (r Repository) StopPath() string { return filepath.Join(r.Dir(), "stop.request") }

func (r Repository) WriteState(state DaemonState) error {
	if state.SchemaVersion == "" {
		state.SchemaVersion = StateSchemaVersion
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Message = redactDaemonMessage(state.Message)
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.StatePath(), append(payload, '\n'), 0o600)
}

func (r Repository) ReadState() (DaemonState, error) {
	payload, err := os.ReadFile(r.StatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DaemonState{SchemaVersion: StateSchemaVersion, Status: StatusStopped, DetectionMode: string(DetectionWatch), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
		}
		return DaemonState{}, err
	}
	var state DaemonState
	if err := json.Unmarshal(payload, &state); err != nil {
		return DaemonState{}, err
	}
	if state.SchemaVersion == "" {
		state.SchemaVersion = StateSchemaVersion
	}
	return state, nil
}

func PrepareEvent(event SyncDaemonEvent) SyncDaemonEvent {
	if event.SchemaVersion == "" {
		event.SchemaVersion = EventSchemaVersion
	}
	event.Message = redactDaemonMessage(event.Message)
	event.Path = SafeEventPath(event.Path)
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return event
}

func (r Repository) AppendEvent(event SyncDaemonEvent) error {
	event = PrepareEvent(event)
	if event.Seq == 0 {
		if existing, err := r.ReadEvents(0); err == nil {
			event.Seq = len(existing) + 1
		}
	}
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(r.EventsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return json.NewEncoder(file).Encode(event)
}

func (r Repository) ReadEvents(limit int) ([]SyncDaemonEvent, error) {
	file, err := os.Open(r.EventsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	events := []SyncDaemonEvent{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event SyncDaemonEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func (r Repository) RequestStop() error {
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(r.StopPath(), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}

func (r Repository) StopRequested() bool {
	_, err := os.Stat(r.StopPath())
	return err == nil
}

func (r Repository) ClearStopRequest() { _ = os.Remove(r.StopPath()) }

func IgnoreRuntimePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "." || clean == "" {
		return false
	}
	for _, prefix := range []string{".git/", ".pinax/", "temp/", "dist/"} {
		if strings.HasPrefix(clean, prefix) || clean == strings.TrimSuffix(prefix, "/") {
			return true
		}
	}
	return strings.Contains(clean, "/.pinax/") || strings.Contains(clean, "/.git/") || strings.Contains(clean, ".tmp")
}

func SafeEventPath(path string) string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "." || clean == "" || IgnoreRuntimePath(clean) {
		return ""
	}
	if filepath.IsAbs(clean) {
		return filepath.Base(clean)
	}
	return clean
}

var daemonSensitivePattern = regexp.MustCompile(`(?i)Authorization|Bearer|raw_provider_payload|provider_payload|raw_prompt|hidden_prompt|system_prompt`)

func redactDaemonMessage(message string) string {
	message = redaction.Cloud(message)
	return daemonSensitivePattern.ReplaceAllString(message, "[REDACTED]")
}
