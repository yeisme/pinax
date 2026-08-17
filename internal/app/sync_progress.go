package app

// SyncEvent is the provider-neutral progress projection shared by one-shot
// sync commands and the daemon bridge. Paths are already subject to the
// command's path policy before they reach a sink.
type SyncEvent struct {
	Type           string            `json:"type"`
	Phase          string            `json:"phase"`
	Direction      string            `json:"direction,omitempty"`
	RunID          string            `json:"run_id,omitempty"`
	Completed      int               `json:"completed,omitempty"`
	Total          int               `json:"total,omitempty"`
	BytesCompleted int64             `json:"bytes_completed,omitempty"`
	BytesTotal     int64             `json:"bytes_total,omitempty"`
	Operation      string            `json:"operation,omitempty"`
	ChangeCode     string            `json:"change_code,omitempty"`
	Path           string            `json:"path,omitempty"`
	PathHash       string            `json:"path_hash,omitempty"`
	Status         string            `json:"status,omitempty"`
	RevisionID     string            `json:"revision_id,omitempty"`
	RemoteWrite    bool              `json:"remote_write"`
	LocalWrite     bool              `json:"local_write"`
	Facts          map[string]string `json:"facts,omitempty"`
}

type SyncEventSink func(SyncEvent)

func emitSyncEvent(sink SyncEventSink, event SyncEvent) {
	if sink != nil {
		sink(event)
	}
}
