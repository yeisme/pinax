package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

type Server struct {
	service      *app.Service
	vault        string
	allowWrite   bool
	authMode     AuthMode
	tokenStore   TokenStore
	auditLogger  *AuditLogger
	logger       *zap.Logger
	exposeGroups []string
	hideGroups   []string
	tempSecret   string
}

type ServerOptions struct {
	AllowWrite   bool
	AuthMode     AuthMode
	TokenFile    string
	ExposeGroups []string
	HideGroups   []string
	Logger       *zap.Logger
}

type HTTPRPCRequest struct {
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

const maxRPCBodyBytes int64 = 1 << 20

func NewServer(service *app.Service, vault string) *Server {
	return NewServerWithOptions(service, vault, ServerOptions{})
}

func NewServerWithOptions(service *app.Service, vault string, options ServerOptions) *Server {
	s := &Server{
		service:      service,
		vault:        vault,
		allowWrite:   options.AllowWrite,
		authMode:     options.AuthMode,
		exposeGroups: options.ExposeGroups,
		hideGroups:   options.HideGroups,
		logger:       options.Logger,
	}
	switch options.AuthMode {
	case AuthModeTemp:
		s.authMode = AuthModeTemp
		store := NewMemoryTokenStore()
		rec, secret := GenerateTokenRecord("temp", map[TokenScope]ScopeTarget{
			ScopeRead:  {},
			ScopeWrite: {},
		}, "", "auto")
		_ = store.Create(rec)
		s.tokenStore = store
		s.tempSecret = secret
	case AuthModeTokenFile:
		store, err := NewFileTokenStore(options.TokenFile)
		if err != nil {
			s.tokenStore = NewMemoryTokenStore()
			s.authMode = AuthModeTemp
		} else {
			s.tokenStore = store
		}
	case AuthModeNone:
		s.tokenStore = nil
	default:
		// Zero value: no auth middleware (used by tests via NewServer)
		s.authMode = 0
		s.tokenStore = nil
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	routes := []struct {
		pattern string
		handler http.HandlerFunc
		group   string
	}{
		{"/", s.handleRoot, "capabilities"},
		{"/v1/capabilities", s.handleCapabilities, "capabilities"},
		{"/v1/folders:repair-plan", s.handleFolderRepairPlan, "folders"},
		{"/v1/folders", s.handleFolders, "folders"},
		{"/v1/folders/", s.handleFolders, "folders"},
		{"/v1/inbox:capture", s.handleInboxCapture, "inbox"},
		{"/v1/inbox", s.handleInboxList, "inbox"},
		{"/v1/inbox/", s.handleInboxItem, "inbox"},
		{"/v1/drafts", s.handleDrafts, "drafts"},
		{"/v1/drafts/", s.handleDraftItem, "drafts"},
		{"/v1/notes/", s.handleNotes, "notes"},
		{"/v1/project-items/", s.handleProjectItems, "projects"},
		{"/v1/projects/", s.handleProjects, "projects"},
	}
	mux.HandleFunc("/v1/rpc", s.handleRPC)

	for _, route := range routes {
		if !s.isGroupExposed(route.group) {
			continue
		}
		mux.HandleFunc(route.pattern, route.handler)
	}

	return s.accessLogMiddleware(s.authMiddleware(s.cacheMiddleware(mux)))
}

func (s *Server) isGroupExposed(group string) bool {
	if len(s.exposeGroups) > 0 {
		for _, g := range s.exposeGroups {
			if g == group {
				return true
			}
		}
		return false
	}
	for _, g := range s.hideGroups {
		if g == group {
			return false
		}
	}
	return true
}

// writeAudit logs an audit entry if the audit logger is configured.
func (s *Server) writeAudit(tokenID, method, path, scope, group string, status int) {
	if s.auditLogger == nil {
		return
	}
	_ = s.auditLogger.Log(AuditEntry{
		TokenID: tokenID,
		Method:  method,
		Path:    path,
		Scope:   scope,
		Group:   group,
		Status:  status,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleRouteNotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.root", &domain.CommandError{Code: "method_not_allowed", Message: "API root only supports GET"}), http.StatusMethodNotAllowed)
		return
	}
	projection := domain.NewProjection("api.root", "Pinax local API is running.")
	projection.Facts["capabilities_url"] = "/v1/capabilities"
	projection.Facts["routes_command"] = "pinax api routes --vault <vault> --json"
	projection.Actions = []domain.Action{{Name: "list-routes", Command: "pinax api routes --vault <vault> --json"}}
	projection.Data = map[string]any{
		"service":          "pinax.local_api",
		"capabilities_url": "/v1/capabilities",
		"routes_command":   "pinax api routes --vault <vault> --json",
		"schema_command":   "pinax api schema export --format openapi --vault <vault> --json",
	}
	writeProjectionStatus(w, projection, http.StatusOK)
}

func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	if s.logger == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &auditStatusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		group := lookupRouteGroup(r.URL.Path)
		fields := []zap.Field{
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(start)),
		}
		if group != "" {
			fields = append(fields, zap.String("group", group))
		}
		s.logger.Info("api.request", fields...)
	})
}

func (s *Server) handleFolderRepairPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.RepairFolders(r.Context(), app.FolderRepairRequest{VaultPath: s.vault, Plan: true})
	writeProjection(w, projection, err)
}

func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/folders" || r.URL.Path == "/v1/folders/" {
		s.handleFolderCollection(w, r)
		return
	}
	ref := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/folders/"), "/")
	pathPart, action, _ := strings.Cut(ref, ":")
	folderPath, err := url.PathUnescape(pathPart)
	if err != nil {
		writeProjectionStatus(w, domain.NewErrorProjection("folder.show", &domain.CommandError{Code: "unsafe_folder_path", Message: "folder path cannot be resolved"}), http.StatusBadRequest)
		return
	}
	if action == "" && r.Method == http.MethodGet {
		projection, err := s.service.ShowFolder(r.Context(), app.FolderRequest{VaultPath: s.vault, Path: folderPath})
		writeProjection(w, projection, err)
		return
	}
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	command := folderActionCommand(action)
	if command == "" {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
		return
	}
	if !s.ensureFolderWriteAllowed(w, r, command) {
		return
	}
	query := r.URL.Query()
	var projection domain.Projection
	var callErr error
	switch action {
	case "rename":
		projection, callErr = s.service.RenameFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, TargetPath: query.Get("target_path"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes"), RequireSnapshot: true})
	case "move":
		projection, callErr = s.service.MoveFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, TargetParent: query.Get("target_parent"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes"), RequireSnapshot: true})
	case "delete":
		projection, callErr = s.service.DeleteFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, EmptyOnly: boolQuery(query, "empty_only"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes"), RequireSnapshot: true})
	case "adopt":
		projection, callErr = s.service.AdoptFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: folderPath, Purpose: query.Get("purpose"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes")})
	}
	writeProjection(w, projection, callErr)
}

func (s *Server) handleFolderCollection(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	switch r.Method {
	case http.MethodGet:
		projection, err := s.service.ListFolders(r.Context(), app.FolderListRequest{VaultPath: s.vault, Purpose: query.Get("purpose"), IncludeEmpty: boolQuery(query, "include_empty"), Depth: intQuery(query, "depth")})
		writeProjection(w, projection, err)
	case http.MethodPost:
		if !s.ensureFolderWriteAllowed(w, r, "folder.create") {
			return
		}
		projection, err := s.service.CreateFolder(r.Context(), app.FolderOperationRequest{VaultPath: s.vault, Path: query.Get("path"), Purpose: query.Get("purpose"), DryRun: boolQuery(query, "dry_run"), Yes: boolQuery(query, "yes")})
		writeProjection(w, projection, err)
	default:
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
	}
}

func (s *Server) ensureFolderWriteAllowed(w http.ResponseWriter, r *http.Request, command string) bool {
	if !s.allowWrite {
		err := &domain.CommandError{Code: "write_disabled", Message: "API server is currently read-only", Hint: "Start with pinax api serve --allow-write and retry"}
		writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusForbidden)
		return false
	}
	query := r.URL.Query()
	if !boolQuery(query, "dry_run") && !boolQuery(query, "yes") {
		err := &domain.CommandError{Code: "approval_required", Message: "Remote folder writes require yes=true", Hint: "Preview with dry_run=true first, then append yes=true to confirm"}
		writeProjectionStatus(w, domain.NewErrorProjection(command, err), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.APIRoutes(r.Context(), app.APIRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	ref := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/notes/"), "/")
	display := r.URL.Query().Get("display")
	if display == "" {
		display = "card"
	}
	projection, err := s.service.ShowNoteProjection(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref, Display: display})
	writeProjection(w, projection, err)
}

func (s *Server) handleProjectItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	target := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/project-items/"), "/")
	action := ""
	ref := target
	if before, after, ok := strings.Cut(target, ":"); ok {
		ref = before
		action = after
	}
	if action == "" {
		action = r.URL.Query().Get("action")
	}
	projection, err := s.service.ProjectItemPlan(r.Context(), app.ProjectItemRequest{VaultPath: s.vault, ItemID: ref, Action: action, Column: r.URL.Query().Get("column"), Yes: r.URL.Query().Get("yes") == "true"})
	writeProjection(w, projection, err)
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
	if !strings.HasSuffix(path, "/board") {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
		return
	}
	slug := strings.Trim(strings.TrimSuffix(path, "/board"), "/")
	display := r.URL.Query().Get("note_display")
	if display == "" {
		display = "card"
	}
	projection, err := s.service.ProjectBoardShow(r.Context(), app.ProjectBoardRequest{VaultPath: s.vault, Project: slug, NoteDisplay: display})
	writeProjection(w, projection, err)
}

func (s *Server) handleInboxList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.InboxList(r.Context(), app.VaultRequest{VaultPath: s.vault})
	writeProjection(w, projection, err)
}

func (s *Server) handleInboxCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	if !s.allowWrite {
		writeProjectionStatus(w, domain.NewErrorProjection("inbox.capture", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
		return
	}
	if !boolQuery(r.URL.Query(), "yes") {
		writeProjectionStatus(w, domain.NewErrorProjection("inbox.capture", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
		return
	}
	projection, err := s.service.InboxCapture(r.Context(), app.CreateNoteRequest{VaultPath: s.vault, Title: r.URL.Query().Get("title"), Body: r.URL.Query().Get("body")})
	writeProjection(w, projection, err)
}

func (s *Server) handleInboxItem(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimPrefix(r.URL.Path, "/v1/inbox/")
	action := ""
	if idx := strings.Index(ref, ":"); idx >= 0 {
		action = ref[idx+1:]
		ref = ref[:idx]
	}
	if action == "promote" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.promote", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.promote", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		to := r.URL.Query().Get("to")
		if to == "" {
			to = "active"
		}
		projection, err := s.service.InboxPromote(r.Context(), app.InboxPromoteRequest{VaultPath: s.vault, NoteRef: ref, To: to, Group: r.URL.Query().Get("group"), Folder: r.URL.Query().Get("folder"), Kind: r.URL.Query().Get("kind"), Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	if action == "discard" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.discard", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("inbox.discard", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.InboxDiscard(r.Context(), app.NoteMutationRequest{VaultPath: s.vault, NoteRef: ref, Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	// Default: show inbox note
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.InboxShow(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref})
	writeProjection(w, projection, err)
}

func (s *Server) handleDrafts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		projection, err := s.service.DraftList(r.Context(), app.VaultRequest{VaultPath: s.vault})
		writeProjection(w, projection, err)
		return
	}
	if r.Method == http.MethodPost {
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.create", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.create", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.DraftCreate(r.Context(), app.CreateNoteRequest{VaultPath: s.vault, Title: r.URL.Query().Get("title"), Body: r.URL.Query().Get("body")})
		writeProjection(w, projection, err)
		return
	}
	writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
}

func (s *Server) handleDraftItem(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimPrefix(r.URL.Path, "/v1/drafts/")
	action := ""
	if idx := strings.Index(ref, ":"); idx >= 0 {
		action = ref[idx+1:]
		ref = ref[:idx]
	}
	if action == "promote" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.promote", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.promote", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}
		projection, err := s.service.DraftPromote(r.Context(), app.DraftPromoteRequest{VaultPath: s.vault, NoteRef: ref, Status: status, Folder: r.URL.Query().Get("folder"), Kind: r.URL.Query().Get("kind"), Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	if action == "archive" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.archive", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.archive", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.DraftArchive(r.Context(), app.NoteMutationRequest{VaultPath: s.vault, NoteRef: ref, Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	if action == "discard" {
		if r.Method != http.MethodPost {
			writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
			return
		}
		if !s.allowWrite {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.discard", &domain.CommandError{Code: "write_disabled", Message: "Remote writes are not enabled", Hint: "Use pinax api serve --allow-write"}), http.StatusForbidden)
			return
		}
		if !boolQuery(r.URL.Query(), "yes") {
			writeProjectionStatus(w, domain.NewErrorProjection("draft.discard", &domain.CommandError{Code: "approval_required", Message: "Write confirmation is required", Hint: "Pass yes=true to confirm"}), http.StatusBadRequest)
			return
		}
		projection, err := s.service.DraftDiscard(r.Context(), app.NoteMutationRequest{VaultPath: s.vault, NoteRef: ref, Yes: true, DryRun: boolQuery(r.URL.Query(), "dry_run")})
		writeProjection(w, projection, err)
		return
	}
	// Default: show draft note
	if r.Method != http.MethodGet {
		writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "method_not_allowed", Message: "API route does not support this HTTP method"}), http.StatusMethodNotAllowed)
		return
	}
	projection, err := s.service.DraftShow(r.Context(), app.ShowNoteRequest{VaultPath: s.vault, NoteRef: ref})
	writeProjection(w, projection, err)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != http.MethodPost {
		projection := domain.NewErrorProjection("api.rpc", &domain.CommandError{Code: "method_not_allowed", Message: "RPC endpoint only supports POST"})
		writeProjectionStatus(w, projection, http.StatusMethodNotAllowed)
		s.logRPCRequest(start, HTTPRPCRequest{}, domain.RemoteRoute{}, http.StatusMethodNotAllowed, projection)
		return
	}

	var req HTTPRPCRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes))
	if err := decoder.Decode(&req); err != nil {
		projection := domain.NewErrorProjection("api.rpc", &domain.CommandError{Code: "invalid_rpc_request", Message: "RPC request body must be valid JSON"})
		writeProjectionStatus(w, projection, http.StatusBadRequest)
		s.logRPCRequest(start, req, domain.RemoteRoute{}, http.StatusBadRequest, projection)
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		projection := domain.NewErrorProjection("api.rpc", &domain.CommandError{Code: "rpc_method_required", Message: "RPC method is required"})
		writeProjectionStatus(w, projection, http.StatusBadRequest)
		s.logRPCRequest(start, req, domain.RemoteRoute{}, http.StatusBadRequest, projection)
		return
	}

	route, ok := app.FindRemoteRPCMethod(req.Method)
	if !ok {
		err := &domain.CommandError{Code: "rpc_method_not_found", Message: "RPC method not found", Hint: fmt.Sprintf("Check whether pinax api routes includes %s", req.Method)}
		projection := domain.NewErrorProjection("api.rpc", err)
		writeProjectionStatus(w, projection, http.StatusNotFound)
		s.logRPCRequest(start, req, domain.RemoteRoute{}, http.StatusNotFound, projection)
		return
	}
	group := rpcRouteGroup(route)
	if !s.isGroupExposed(group) {
		projection := domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"})
		writeProjectionStatus(w, projection, http.StatusNotFound)
		s.logRPCRequest(start, req, route, http.StatusNotFound, projection)
		return
	}
	if !route.Readonly {
		if !s.allowWrite {
			err := &domain.CommandError{Code: "write_disabled", Message: "API server is currently read-only", Hint: "Start with pinax api serve --allow-write and retry"}
			projection := domain.NewErrorProjection(route.Command, err)
			writeProjectionStatus(w, projection, http.StatusForbidden)
			s.logRPCRequest(start, req, route, http.StatusForbidden, projection)
			return
		}
		if !boolParam(req.Params, "dry_run") && !boolParam(req.Params, "yes") {
			err := &domain.CommandError{Code: "approval_required", Message: "Remote RPC writes require yes=true", Hint: "Preview with dry_run=true first, then include yes=true to confirm"}
			projection := domain.NewErrorProjection(route.Command, err)
			writeProjectionStatus(w, projection, http.StatusBadRequest)
			s.logRPCRequest(start, req, route, http.StatusBadRequest, projection)
			return
		}
	}

	projection, err := NewRPCDispatcherWithOptions(s.service, s.vault, DispatcherOptions{AllowWrite: s.allowWrite}).Call(r.Context(), RPCRequest{Method: req.Method, Params: req.Params})
	status := projectionHTTPStatus(projection, err)
	writeProjectionStatus(w, projection, status)
	s.logRPCRequest(start, req, route, status, projection)
}

func (s *Server) logRPCRequest(start time.Time, req HTTPRPCRequest, route domain.RemoteRoute, status int, projection domain.Projection) {
	if s.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("rpc_method", req.Method),
		zap.Int("status", status),
		zap.Duration("duration", time.Since(start)),
	}
	if req.ID != "" {
		fields = append(fields, zap.String("rpc_id", req.ID))
	}
	if route.Command != "" {
		fields = append(fields,
			zap.String("command", route.Command),
			zap.String("group", rpcRouteGroup(route)),
			zap.Bool("readonly", route.Readonly),
		)
	} else if projection.Command != "" {
		fields = append(fields, zap.String("command", projection.Command))
	}
	if projection.Error != nil {
		fields = append(fields, zap.String("error_code", projection.Error.Code))
	}
	s.logger.Info("api.rpc", fields...)
}

func projectionHTTPStatus(projection domain.Projection, err error) int {
	if err == nil {
		return http.StatusOK
	}
	if projection.Error == nil {
		return http.StatusBadRequest
	}
	switch projection.Error.Code {
	case "write_disabled", "insufficient_scope":
		return http.StatusForbidden
	case "rpc_method_not_found", "route_not_found", "note_not_found", "folder_not_found":
		return http.StatusNotFound
	case "revision_conflict", "lock_held", "folder_path_conflict", "note_path_conflict":
		return http.StatusConflict
	case "backend_unavailable", "transport_unavailable", "cloud_backend_unavailable":
		return http.StatusServiceUnavailable
	case "internal_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

func rpcRouteGroup(route domain.RemoteRoute) string {
	parts := strings.Split(route.CapabilityID, ".")
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "project":
		return "projects"
	case "folder":
		return "folders"
	case "note":
		return "notes"
	case "draft":
		return "drafts"
	default:
		return parts[0]
	}
}

func (s *Server) handleRouteNotFound(w http.ResponseWriter, r *http.Request) {
	writeProjectionStatus(w, domain.NewErrorProjection("api.route", &domain.CommandError{Code: "route_not_found", Message: "API route not found"}), http.StatusNotFound)
}

func writeProjection(w http.ResponseWriter, projection domain.Projection, err error) {
	writeProjectionStatus(w, projection, projectionHTTPStatus(projection, err))
}

func writeProjectionStatus(w http.ResponseWriter, projection domain.Projection, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	projection.Mode = "json"
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(projection)
}

func ListenAndServe(ctx context.Context, service *app.Service, vault string, port int, logf func(string, ...any), options ...ServerOptions) error {
	serverOptions := ServerOptions{}
	if len(options) > 0 {
		serverOptions = options[0]
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := NewServerWithOptions(service, vault, serverOptions)

	// Init audit logger
	if srv.authMode != AuthModeNone {
		auditPath := filepath.Join(vault, ".pinax", "events", "api-audit.jsonl")
		auditLogger, auditErr := NewAuditLogger(auditPath)
		if auditErr == nil {
			srv.auditLogger = auditLogger
			defer func() { _ = auditLogger.Close() }()
		}
	}

	apiURL := fmt.Sprintf("http://%s", listener.Addr().String())
	if srv.logger != nil {
		srv.logger.Info("pinax api ready", zap.String("url", apiURL), zap.String("vault", vault), zap.Bool("allow_write", srv.allowWrite), zap.String("auth_mode", authModeLabel(srv.authMode)))
		if srv.tempSecret != "" {
			srv.logger.Warn("pinax api temp token issued", zap.String("token", srv.tempSecret), zap.String("scope", "read,write"))
		}
	} else if logf != nil {
		logf("Pinax local API: %s", apiURL)
		if srv.tempSecret != "" {
			logf("Temp token: %s", srv.tempSecret)
		}
	}

	httpServer := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	err = httpServer.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func authModeLabel(mode AuthMode) string {
	switch mode {
	case AuthModeTemp:
		return "temp-token"
	case AuthModeTokenFile:
		return "token-file"
	case AuthModeNone:
		return "none"
	default:
		return "unset"
	}
}

func folderActionCommand(action string) string {
	switch action {
	case "rename", "move", "delete", "adopt":
		return "folder." + action
	default:
		return ""
	}
}

func boolQuery(values url.Values, key string) bool {
	value := strings.ToLower(strings.TrimSpace(values.Get(key)))
	return value == "true" || value == "1" || value == "yes"
}

func intQuery(values url.Values, key string) int {
	value := strings.TrimSpace(values.Get(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
