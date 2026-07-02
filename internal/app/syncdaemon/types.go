package syncdaemon

import "time"

const (
	StateSchemaVersion = "pinax.sync_daemon.state.v1"
	EventSchemaVersion = "pinax.sync_daemon.event.v1"
	StatusStopped      = "stopped"
	StatusRunning      = "running"
	StatusDegraded     = "degraded"
	StatusStopping     = "stopping"
	StatusConflict     = "conflict_required"
)

type DetectionMode string

const (
	DetectionWatch DetectionMode = "watch"
	DetectionScan  DetectionMode = "scan"
)

type RunRequest struct {
	VaultPath    string
	Target       string
	Yes          bool
	Once         bool
	PollInterval time.Duration
	SyncTimeout  time.Duration
	Mode         DetectionMode
}

type DaemonState struct {
	SchemaVersion  string `json:"schema_version"`
	Status         string `json:"status"`
	Target         string `json:"target"`
	PID            int    `json:"pid,omitempty"`
	DetectionMode  string `json:"detection_mode"`
	LocalDirty     bool   `json:"local_dirty"`
	LocalHash      string `json:"local_hash,omitempty"`
	RemoteRevision string `json:"remote_revision,omitempty"`
	LastPollAt     string `json:"last_poll_at,omitempty"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	LastErrorCode  string `json:"last_error_code,omitempty"`
	NextRetryAt    string `json:"next_retry_at,omitempty"`
	Message        string `json:"message,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

type SyncDaemonEvent struct {
	SchemaVersion  string         `json:"schema_version"`
	Seq            int            `json:"seq,omitempty"`
	Type           string         `json:"type"`
	Status         string         `json:"status,omitempty"`
	Target         string         `json:"target,omitempty"`
	Path           string         `json:"path,omitempty"`
	ErrorCode      string         `json:"error_code,omitempty"`
	Message        string         `json:"message,omitempty"`
	Facts          map[string]any `json:"facts,omitempty"`
	CycleID        string         `json:"cycle_id,omitempty"`
	Trigger        string         `json:"trigger,omitempty"`
	Direction      string         `json:"direction,omitempty"`
	DurationMS     int64          `json:"duration_ms,omitempty"`
	LocalDirty     bool           `json:"local_dirty,omitempty"`
	RemoteRevision string         `json:"remote_revision,omitempty"`
	RevisionID     string         `json:"revision_id,omitempty"`
	SyncRunID      string         `json:"sync_run_id,omitempty"`
	RemoteWrite    bool           `json:"remote_write,omitempty"`
	LocalWrite     bool           `json:"local_write,omitempty"`
	CreatedAt      string         `json:"created_at"`
}

func NewState(target string, pid int, mode DetectionMode, status string) DaemonState {
	now := time.Now().UTC().Format(time.RFC3339)
	if mode == "" {
		mode = DetectionWatch
	}
	return DaemonState{SchemaVersion: StateSchemaVersion, Status: status, Target: target, PID: pid, DetectionMode: string(mode), StartedAt: now, UpdatedAt: now}
}

func NewEvent(eventType, status, target string) SyncDaemonEvent {
	return SyncDaemonEvent{SchemaVersion: EventSchemaVersion, Type: eventType, Status: status, Target: target, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
}
