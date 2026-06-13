package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditLogger_CreateAndLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	entry := AuditEntry{
		TokenID: "pt_abc123",
		Method:  "GET",
		Path:    "/v1/notes/note-001",
		Scope:   "read",
		Group:   "notes",
		Status:  200,
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Close to flush
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read file and verify
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit file: %v", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected at least one line")
	}

	var got AuditEntry
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("parse audit entry: %v", err)
	}

	if got.TokenID != "pt_abc123" {
		t.Fatalf("expected token_id=pt_abc123, got %s", got.TokenID)
	}
	if got.Method != "GET" {
		t.Fatalf("expected method=GET, got %s", got.Method)
	}
	if got.Timestamp == "" {
		t.Fatal("expected auto-filled timestamp")
	}
	if got.Status != 200 {
		t.Fatalf("expected status=200, got %d", got.Status)
	}
}

func TestAuditLogger_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := logger.Log(AuditEntry{
			TokenID: "pt_test",
			Method:  "GET",
			Path:    "/v1/capabilities",
			Status:  200,
		}); err != nil {
			t.Fatalf("Log %d: %v", i, err)
		}
	}
	_ = logger.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1
	if lines != 5 {
		t.Fatalf("expected 5 lines, got %d", lines)
	}
}

func TestAuditLogger_CustomTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, _ := NewAuditLogger(path)

	customTS := "2026-01-01T00:00:00Z"
	_ = logger.Log(AuditEntry{Timestamp: customTS, Method: "GET", Path: "/", Status: 200})
	_ = logger.Close()

	data, _ := os.ReadFile(path)
	var got AuditEntry
	_ = json.Unmarshal([]byte(strings.Split(string(data), "\n")[0]), &got)
	if got.Timestamp != customTS {
		t.Fatalf("expected custom timestamp preserved, got %s", got.Timestamp)
	}
}

func TestAuditLogger_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "audit.jsonl")
	logger, err := NewAuditLogger(path)
	if err != nil {
		t.Fatalf("NewAuditLogger with nested dirs: %v", err)
	}
	_ = logger.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestServer_WriteAudit_NilLogger(t *testing.T) {
	s := &Server{auditLogger: nil}
	// Should not panic
	s.writeAudit("id", "GET", "/v1/test", "read", "test", 200)
}

func TestAuthMiddlewareAuditsFinalHandlerStatus(t *testing.T) {
	s, secret := newAuthTestServer(t, AuthModeTemp)
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := NewAuditLogger(path)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	s.auditLogger = logger

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inbox:capture?title=Blocked&yes=true", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected handler to reject write on readonly server, got %d: %s", res.Code, res.Body.String())
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close audit logger: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit: %v", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected audit entry")
	}
	var got AuditEntry
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("parse audit entry: %v", err)
	}
	if got.Status != http.StatusForbidden {
		t.Fatalf("audit status = %d, want final handler status %d", got.Status, http.StatusForbidden)
	}
}
