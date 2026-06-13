package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp string `json:"ts"`
	TokenID   string `json:"token_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Scope     string `json:"scope,omitempty"`
	Group     string `json:"group,omitempty"`
	Status    int    `json:"status"`
}

// AuditLogger writes audit entries to a JSONL file.
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// NewAuditLogger creates a new audit logger writing to the given path.
func NewAuditLogger(path string) (*AuditLogger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &AuditLogger{file: f, enc: json.NewEncoder(f)}, nil
}

// Log writes an audit entry to the log file.
func (l *AuditLogger) Log(entry AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if err := l.enc.Encode(entry); err != nil {
		return err
	}
	return l.file.Sync()
}

// Close closes the audit log file.
func (l *AuditLogger) Close() error {
	return l.file.Close()
}
