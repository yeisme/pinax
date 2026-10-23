package api

import (
	"net/http"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func (s *Server) ensureSyncWriteAllowed(w http.ResponseWriter, r *http.Request, command string) bool {
	if !s.allowWrite {
		writeProjectionStatus(w, domain.NewErrorProjection(command, &domain.CommandError{
			Code:    "write_disabled",
			Message: "Remote sync writes are not enabled",
			Hint:    "Start pinax api serve with --allow-write",
		}), http.StatusForbidden)
		return false
	}
	if !boolQuery(r.URL.Query(), "yes") {
		writeProjectionStatus(w, domain.NewErrorProjection(command, &domain.CommandError{
			Code:    "approval_required",
			Message: "Sync writes require explicit confirmation",
			Hint:    "Review sync diff first, then repeat with yes=true",
		}), http.StatusBadRequest)
		return false
	}
	return true
}

func syncRequestFromQuery(r *http.Request) app.SyncRequest {
	q := r.URL.Query()
	target := strings.TrimSpace(q.Get("target"))
	if target == "" {
		target = "capsa"
	}
	return app.SyncRequest{
		VaultPath:      "",
		Target:         target,
		Yes:            boolQuery(q, "yes"),
		DryRun:         boolQuery(q, "dry_run"),
		BaseRevision:   q.Get("base_revision"),
		RemoteRevision: q.Get("remote_revision"),
		ContentDiff:    boolQuery(q, "content_diff"),
	}
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.status", &domain.CommandError{Code: "method_not_allowed", Message: "Sync status only supports GET"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.SyncStatus(r.Context(), app.SyncStatusRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) handleSyncDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.diff", &domain.CommandError{Code: "method_not_allowed", Message: "Sync diff only supports POST"}), http.StatusMethodNotAllowed)
		return
	}
	req := syncRequestFromQuery(r)
	req.VaultPath = s.vault
	req.DryRun = true
	projection, err := s.service.SyncDiff(r.Context(), req)
	writeProjection(w, projection, err)
}

func (s *Server) handleSyncPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.push", &domain.CommandError{Code: "method_not_allowed", Message: "Sync push only supports POST"}), http.StatusMethodNotAllowed)
		return
	}
	if !s.ensureSyncWriteAllowed(w, r, "sync.push") {
		return
	}
	req := syncRequestFromQuery(r)
	req.VaultPath, req.Yes = s.vault, true
	req.Operation = syncHTTPIdentity(r, "rest.sync."+strings.TrimPrefix(r.URL.Path, "/v1/sync:"))
	projection, err := s.service.SyncPush(r.Context(), req)
	writeProjection(w, projection, err)
}

func (s *Server) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.pull", &domain.CommandError{Code: "method_not_allowed", Message: "Sync pull only supports POST"}), http.StatusMethodNotAllowed)
		return
	}
	if !s.ensureSyncWriteAllowed(w, r, "sync.pull") {
		return
	}
	req := syncRequestFromQuery(r)
	req.VaultPath, req.Yes = s.vault, true
	req.Operation = syncHTTPIdentity(r, "rest.sync."+strings.TrimPrefix(r.URL.Path, "/v1/sync:"))
	projection, err := s.service.SyncPull(r.Context(), req)
	writeProjection(w, projection, err)
}

func (s *Server) handleSyncConflictsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.conflicts.list", &domain.CommandError{Code: "method_not_allowed", Message: "Conflict listing only supports GET"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.SyncConflictsList(r.Context(), app.SyncConflictsListRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) handleSyncConflictsDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.conflicts.diff", &domain.CommandError{Code: "method_not_allowed", Message: "Conflict diff only supports GET"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.SyncConflictsDiff(r.Context(), app.SyncConflictFileRequest{VaultPath: s.vault, File: r.URL.Query().Get("file")})
	writeProjection(w, projection, err)
}

func (s *Server) handleSyncConflictsResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("sync.conflicts.resolve", &domain.CommandError{Code: "method_not_allowed", Message: "Conflict resolution only supports POST"}), http.StatusMethodNotAllowed)
		return
	}
	if !s.ensureSyncWriteAllowed(w, r, "sync.conflicts.resolve") {
		return
	}
	q := r.URL.Query()
	projection, err := s.service.SyncConflictsResolve(r.Context(), app.SyncConflictResolveRequest{
		Operation:  syncHTTPIdentity(r, "rest.sync.conflicts.resolve"),
		VaultPath:  s.vault,
		File:       q.Get("file"),
		KeepLocal:  boolQuery(q, "keep_local"),
		KeepRemote: boolQuery(q, "keep_remote"),
		MergedPath: q.Get("merged_path"),
		Yes:        true,
	})
	writeProjection(w, projection, err)
}

func syncHTTPIdentity(r *http.Request, binding string) app.RemoteOperationIdentity {
	return app.RemoteOperationIdentity{OperationID: r.Header.Get("X-Pinax-Operation-ID"), IdempotencyKey: r.Header.Get("Idempotency-Key"), BindingID: binding, Access: operationAccessFromRequest(r)}
}
