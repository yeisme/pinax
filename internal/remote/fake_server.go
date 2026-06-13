package remote

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

type FakeServer struct {
	URL string

	server *httptest.Server
	mu     sync.Mutex
	vaults map[string]*fakeWorkspace
}

type fakeWorkspace struct {
	Revision string
	Manifest map[string]any
	Blobs    map[string][]byte
}

type FakeManifestPutRequest struct {
	BaseRevision string         `json:"base_revision"`
	Manifest     map[string]any `json:"manifest"`
}

type FakeManifestResponse struct {
	Revision string         `json:"revision"`
	Manifest map[string]any `json:"manifest,omitempty"`
}

type FakeBlobCheckRequest struct {
	BlobIDs []string `json:"blob_ids"`
}

type FakeBlobCheckResponse struct {
	MissingBlobIDs []string `json:"missing_blob_ids"`
}

type FakeRevisionResponse struct {
	RevisionID     string `json:"revision_id"`
	ManifestBlobID string `json:"manifest_blob_id"`
}

type FakeRevisionCommitRequest struct {
	BaseRevision   string   `json:"base_revision"`
	RevisionID     string   `json:"revision_id,omitempty"`
	ManifestBlobID string   `json:"manifest_blob_id"`
	BlobIDs        []string `json:"blob_ids,omitempty"`
	DeviceID       string   `json:"device_id,omitempty"`
}

type FakeRevisionCommitResponse struct {
	RevisionID     string `json:"revision_id"`
	ManifestBlobID string `json:"manifest_blob_id"`
}

type FakeContractErrorResponse struct {
	Error FakeContractError `json:"error"`
}

type FakeContractError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type FakeErrorResponse struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	CurrentRevision string `json:"current_revision,omitempty"`
}

func NewFakeServer() *FakeServer {
	fake := &FakeServer{vaults: map[string]*fakeWorkspace{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", fake.handleHealth)
	mux.HandleFunc("/v1/workspaces/", fake.handleWorkspace)
	server := httptest.NewServer(mux)
	fake.server = server
	fake.URL = server.URL
	return fake
}

func (s *FakeServer) Close() {
	if s != nil && s.server != nil {
		s.server.Close()
	}
}

func (s *FakeServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeFakeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *FakeServer) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, rest, ok := parseFakeWorkspacePath(r.URL.Path)
	if !ok {
		writeFakeJSON(w, http.StatusNotFound, FakeErrorResponse{Code: "NOT_FOUND", Message: "unknown fake cloud path"})
		return
	}
	switch {
	case rest == "manifest" && r.Method == http.MethodGet:
		s.handleGetManifest(w, workspaceID)
	case rest == "manifest" && r.Method == http.MethodPost:
		s.handlePutManifest(w, r, workspaceID)
	case rest == "revision" && r.Method == http.MethodGet:
		s.handleGetRevision(w, workspaceID)
	case rest == "blobs:batchCheck" && r.Method == http.MethodPost:
		s.handleBatchCheckBlobs(w, r, workspaceID)
	case rest == "revisions:commit" && r.Method == http.MethodPost:
		s.handleCommitRevision(w, r, workspaceID)
	case strings.HasPrefix(rest, "blobs/") && r.Method == http.MethodPut:
		s.handlePutBlob(w, r, workspaceID, strings.TrimPrefix(rest, "blobs/"))
	case strings.HasPrefix(rest, "blobs/") && r.Method == http.MethodGet:
		s.handleGetBlob(w, workspaceID, strings.TrimPrefix(rest, "blobs/"))
	default:
		writeFakeJSON(w, http.StatusMethodNotAllowed, FakeErrorResponse{Code: "METHOD_NOT_ALLOWED", Message: "unsupported fake cloud operation"})
	}
}

func (s *FakeServer) handleGetManifest(w http.ResponseWriter, workspaceID string) {
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	response := FakeManifestResponse{Revision: workspace.Revision, Manifest: workspace.Manifest}
	s.mu.Unlock()
	writeFakeJSON(w, http.StatusOK, response)
}

func (s *FakeServer) handleGetRevision(w http.ResponseWriter, workspaceID string) {
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	response := FakeRevisionResponse{RevisionID: workspace.Revision, ManifestBlobID: fakeManifestBlobID(workspace.Manifest)}
	s.mu.Unlock()
	writeFakeJSON(w, http.StatusOK, response)
}

func (s *FakeServer) handleBatchCheckBlobs(w http.ResponseWriter, r *http.Request, workspaceID string) {
	var req FakeBlobCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFakeContractError(w, http.StatusBadRequest, "invalid_request", err.Error(), false)
		return
	}
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	missing := make([]string, 0, len(req.BlobIDs))
	for _, blobID := range req.BlobIDs {
		if _, ok := workspace.Blobs[blobID]; !ok {
			missing = append(missing, blobID)
		}
	}
	s.mu.Unlock()
	writeFakeJSON(w, http.StatusOK, FakeBlobCheckResponse{MissingBlobIDs: missing})
}

func (s *FakeServer) handleCommitRevision(w http.ResponseWriter, r *http.Request, workspaceID string) {
	var req FakeRevisionCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFakeContractError(w, http.StatusBadRequest, "invalid_request", err.Error(), false)
		return
	}
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	if req.BaseRevision != workspace.Revision {
		current := workspace.Revision
		s.mu.Unlock()
		_ = current
		writeFakeContractError(w, http.StatusConflict, "revision_conflict", "base revision mismatch", true)
		return
	}
	for _, blobID := range req.BlobIDs {
		if _, ok := workspace.Blobs[blobID]; !ok {
			s.mu.Unlock()
			writeFakeContractError(w, http.StatusNotFound, "blob_not_found", "commit references missing blob", false)
			return
		}
	}
	workspace.Manifest = map[string]any{"manifest_blob_id": req.ManifestBlobID, "blob_ids": append([]string(nil), req.BlobIDs...), "device_id": req.DeviceID}
	workspace.Revision = nextFakeRevision(workspace.Revision)
	response := FakeRevisionCommitResponse{RevisionID: workspace.Revision, ManifestBlobID: req.ManifestBlobID}
	s.mu.Unlock()
	writeFakeJSON(w, http.StatusOK, response)
}

func (s *FakeServer) handlePutManifest(w http.ResponseWriter, r *http.Request, workspaceID string) {
	var req FakeManifestPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFakeJSON(w, http.StatusBadRequest, FakeErrorResponse{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	if req.BaseRevision != workspace.Revision {
		current := workspace.Revision
		s.mu.Unlock()
		writeFakeJSON(w, http.StatusConflict, FakeErrorResponse{Code: "REVISION_CONFLICT", Message: "base revision mismatch", CurrentRevision: current})
		return
	}
	workspace.Manifest = req.Manifest
	workspace.Revision = nextFakeRevision(workspace.Revision)
	response := FakeManifestResponse{Revision: workspace.Revision, Manifest: workspace.Manifest}
	s.mu.Unlock()
	writeFakeJSON(w, http.StatusOK, response)
}

func (s *FakeServer) handlePutBlob(w http.ResponseWriter, r *http.Request, workspaceID, blobID string) {
	if blobID == "" {
		writeFakeJSON(w, http.StatusBadRequest, FakeErrorResponse{Code: "BAD_REQUEST", Message: "blob id required"})
		return
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeFakeJSON(w, http.StatusBadRequest, FakeErrorResponse{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	workspace.Blobs[blobID] = append([]byte(nil), b...)
	s.mu.Unlock()
	writeFakeJSON(w, http.StatusOK, map[string]string{"status": "stored", "blob_id": blobID})
}

func (s *FakeServer) handleGetBlob(w http.ResponseWriter, workspaceID, blobID string) {
	s.mu.Lock()
	workspace := s.workspaceLocked(workspaceID)
	b, ok := workspace.Blobs[blobID]
	if ok {
		b = append([]byte(nil), b...)
	}
	s.mu.Unlock()
	if !ok {
		writeFakeJSON(w, http.StatusNotFound, FakeErrorResponse{Code: "BLOB_NOT_FOUND", Message: "blob not found"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *FakeServer) workspaceLocked(workspaceID string) *fakeWorkspace {
	workspace, ok := s.vaults[workspaceID]
	if !ok {
		workspace = &fakeWorkspace{Revision: "", Blobs: map[string][]byte{}, Manifest: map[string]any{}}
		s.vaults[workspaceID] = workspace
	}
	return workspace
}

func parseFakeWorkspacePath(path string) (string, string, bool) {
	path = strings.TrimPrefix(path, "/v1/workspaces/")
	if path == "" {
		return "", "", false
	}
	workspaceID, rest, ok := strings.Cut(path, "/")
	return workspaceID, rest, ok && workspaceID != "" && rest != ""
}

func fakeManifestBlobID(manifest map[string]any) string {
	if manifest == nil {
		return ""
	}
	if value, ok := manifest["manifest_blob_id"].(string); ok {
		return value
	}
	return ""
}

func nextFakeRevision(current string) string {
	var n int
	if _, err := fmt.Sscanf(current, "rev_%d", &n); err != nil {
		return "rev_1"
	}
	return fmt.Sprintf("rev_%d", n+1)
}

func writeFakeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeFakeContractError(w http.ResponseWriter, status int, code, message string, retryable bool) {
	writeFakeJSON(w, status, FakeContractErrorResponse{Error: FakeContractError{Code: code, Message: message, Retryable: retryable}})
}
