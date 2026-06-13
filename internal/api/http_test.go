package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func TestLocalAPIProjectBoardMatchesProjectionEnvelope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "Research", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	writeAPIFixture(t, filepath.Join(root, "research", "task.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_task\ntitle: Board Task\nproject: research\nkind: task\nstatus: active\n---\n\nsecret body should not appear as body field\n")
	server := NewServer(svc, root)

	capRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(capRes, httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil))
	if capRes.Code != http.StatusOK || !strings.Contains(capRes.Body.String(), "project.board.show") {
		t.Fatalf("capabilities response: status=%d body=%s", capRes.Code, capRes.Body.String())
	}
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/projects/research/board?note_display=card", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("board status = %d body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("board json invalid: %v\n%s", err, res.Body.String())
	}
	if payload["command"] != "project.board.show" || !strings.Contains(res.Body.String(), `"project":"research"`) || strings.Contains(res.Body.String(), `"body"`) {
		t.Fatalf("board payload = %s", res.Body.String())
	}
	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/projects/research/board", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("write-like method status = %d", res.Code)
	}
}

func TestLocalAPINoteReadAndProjectItemWritePlan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "Research", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := svc.ProjectItemAdd(ctx, app.ProjectItemRequest{VaultPath: root, Project: "research", Title: "API Item", Column: "next", Body: "secret body"})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	itemID := created.Facts["item_id"]
	noteRef := "note" + strings.TrimPrefix(itemID, "item")
	server := NewServer(svc, root)

	noteRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(noteRes, httptest.NewRequest(http.MethodGet, "/v1/notes/"+noteRef+"?display=card", nil))
	if noteRes.Code != http.StatusOK || !strings.Contains(noteRes.Body.String(), `"command":"note.show"`) || strings.Contains(noteRes.Body.String(), `"body"`) {
		t.Fatalf("note read response: status=%d body=%s", noteRes.Code, noteRes.Body.String())
	}

	planRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(planRes, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":archive", nil))
	if planRes.Code != http.StatusBadRequest || !strings.Contains(planRes.Body.String(), `"code":"approval_required"`) {
		t.Fatalf("archive plan approval response: status=%d body=%s", planRes.Code, planRes.Body.String())
	}

	planRes = httptest.NewRecorder()
	server.Handler().ServeHTTP(planRes, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":archive?yes=true", nil))
	if planRes.Code != http.StatusBadRequest || !strings.Contains(planRes.Body.String(), `"code":"snapshot_required"`) || !strings.Contains(planRes.Body.String(), "pinax version snapshot") {
		t.Fatalf("archive plan snapshot response: status=%d body=%s", planRes.Code, planRes.Body.String())
	}

	planRes = httptest.NewRecorder()
	server.Handler().ServeHTTP(planRes, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":move?column=done", nil))
	if planRes.Code != http.StatusBadRequest || !strings.Contains(planRes.Body.String(), `"code":"approval_required"`) {
		t.Fatalf("move done plan approval response: status=%d body=%s", planRes.Code, planRes.Body.String())
	}
	planRes = httptest.NewRecorder()
	server.Handler().ServeHTTP(planRes, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":move?column=done&yes=true", nil))
	if planRes.Code != http.StatusBadRequest || !strings.Contains(planRes.Body.String(), `"code":"snapshot_required"`) || !strings.Contains(planRes.Body.String(), "pinax version snapshot") {
		t.Fatalf("move done plan snapshot response: status=%d body=%s", planRes.Code, planRes.Body.String())
	}
}

func TestLocalAPIFolderRoutesAndWriteGate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	readonly := NewServer(svc, root)

	listRes := httptest.NewRecorder()
	readonly.Handler().ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/v1/folders?include_empty=true", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), `"command":"folder.list"`) {
		t.Fatalf("folder list response: status=%d body=%s", listRes.Code, listRes.Body.String())
	}

	blocked := httptest.NewRecorder()
	readonly.Handler().ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/v1/folders?path=spaces/api&purpose=notes&yes=true", nil))
	assertRESTErrorProjection(t, blocked, http.StatusForbidden, "write_disabled")
	if fileExistsForAPITest(filepath.Join(root, "spaces", "api")) {
		t.Fatalf("readonly folder create modified vault")
	}

	writer := NewServerWithOptions(svc, root, ServerOptions{AllowWrite: true})
	approval := httptest.NewRecorder()
	writer.Handler().ServeHTTP(approval, httptest.NewRequest(http.MethodPost, "/v1/folders?path=spaces/api&purpose=notes", nil))
	assertRESTErrorProjection(t, approval, http.StatusBadRequest, "approval_required")

	createRes := httptest.NewRecorder()
	writer.Handler().ServeHTTP(createRes, httptest.NewRequest(http.MethodPost, "/v1/folders?path=spaces/api&purpose=notes&yes=true", nil))
	if createRes.Code != http.StatusOK || !strings.Contains(createRes.Body.String(), `"command":"folder.create"`) || !fileExistsForAPITest(filepath.Join(root, "spaces", "api")) {
		t.Fatalf("folder create response: status=%d body=%s", createRes.Code, createRes.Body.String())
	}

	showRes := httptest.NewRecorder()
	writer.Handler().ServeHTTP(showRes, httptest.NewRequest(http.MethodGet, "/v1/folders/spaces/api", nil))
	if showRes.Code != http.StatusOK || !strings.Contains(showRes.Body.String(), `"command":"folder.show"`) || !strings.Contains(showRes.Body.String(), "spaces/api") {
		t.Fatalf("folder show response: status=%d body=%s", showRes.Code, showRes.Body.String())
	}

	renameRes := httptest.NewRecorder()
	writer.Handler().ServeHTTP(renameRes, httptest.NewRequest(http.MethodPost, "/v1/folders/spaces/api:rename?target_path=spaces/api-renamed&yes=true", nil))
	assertRESTErrorProjection(t, renameRes, http.StatusBadRequest, "snapshot_required")
	if fileExistsForAPITest(filepath.Join(root, "spaces", "api-renamed")) {
		t.Fatalf("folder rename without snapshot modified vault")
	}
	if _, err := svc.VersionSnapshot(ctx, app.SnapshotRequest{VaultPath: root, Message: "folder api"}); err != nil {
		t.Fatalf("version snapshot: %v", err)
	}
	renameRes = httptest.NewRecorder()
	writer.Handler().ServeHTTP(renameRes, httptest.NewRequest(http.MethodPost, "/v1/folders/spaces/api:rename?target_path=spaces/api-renamed&yes=true", nil))
	if renameRes.Code != http.StatusOK || !strings.Contains(renameRes.Body.String(), `"command":"folder.rename"`) || !fileExistsForAPITest(filepath.Join(root, "spaces", "api-renamed")) {
		t.Fatalf("folder rename response: status=%d body=%s", renameRes.Code, renameRes.Body.String())
	}
}

func TestProjectionHTTPStatusMapsStableErrorCodes(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{code: "approval_required", want: http.StatusBadRequest},
		{code: "revision_conflict", want: http.StatusConflict},
		{code: "backend_unavailable", want: http.StatusServiceUnavailable},
		{code: "transport_unavailable", want: http.StatusServiceUnavailable},
		{code: "internal_error", want: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			projection := domain.NewErrorProjection("api.test", &domain.CommandError{Code: tc.code, Message: tc.code})
			if got := projectionHTTPStatus(projection, projection.Error); got != tc.want {
				t.Fatalf("status for %s = %d, want %d", tc.code, got, tc.want)
			}
		})
	}
}

func TestLocalAPIRPCTransportDispatchesProjectionEnvelope(t *testing.T) {
	ctx := context.Background()
	root, svc, _, _ := newAPITestVault(t, ctx)
	server := NewServer(svc, root)

	body := bytes.NewBufferString(`{"id":"req-1","method":"Pinax.Folder.List","params":{"include_empty":true}}`)
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/rpc", body))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"command":"folder.list"`) {
		t.Fatalf("rpc response: status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestLocalAPIRPCErrorsAndWriteGateUseProjectionEnvelope(t *testing.T) {
	ctx := context.Background()
	root, svc, _, _ := newAPITestVault(t, ctx)
	readonly := NewServer(svc, root)

	invalid := httptest.NewRecorder()
	readonly.Handler().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{`)))
	assertRESTErrorProjection(t, invalid, http.StatusBadRequest, "invalid_rpc_request")

	unknown := httptest.NewRecorder()
	readonly.Handler().ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Unknown"}`)))
	assertRESTErrorProjection(t, unknown, http.StatusNotFound, "rpc_method_not_found")

	method := httptest.NewRecorder()
	readonly.Handler().ServeHTTP(method, httptest.NewRequest(http.MethodGet, "/v1/rpc", nil))
	assertRESTErrorProjection(t, method, http.StatusMethodNotAllowed, "method_not_allowed")

	before := snapshotFileTree(t, root)
	writeDisabled := httptest.NewRecorder()
	readonly.Handler().ServeHTTP(writeDisabled, httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.Create","params":{"path":"rpc-write","yes":true,"token":"secret-token"}}`)))
	assertRESTErrorProjection(t, writeDisabled, http.StatusForbidden, "write_disabled")
	assertNoSecretLeak(t, writeDisabled.Body.String())

	writer := NewServerWithOptions(svc, root, ServerOptions{AllowWrite: true})
	approval := httptest.NewRecorder()
	writer.Handler().ServeHTTP(approval, httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.Create","params":{"path":"rpc-write"}}`)))
	assertRESTErrorProjection(t, approval, http.StatusBadRequest, "approval_required")

	inboxApproval := httptest.NewRecorder()
	writer.Handler().ServeHTTP(inboxApproval, httptest.NewRequest(http.MethodPost, "/v1/inbox:capture?title=Inbox&body=Body", nil))
	assertRESTErrorProjection(t, inboxApproval, http.StatusBadRequest, "approval_required")

	noteMissing := httptest.NewRecorder()
	writer.Handler().ServeHTTP(noteMissing, httptest.NewRequest(http.MethodGet, "/v1/notes/missing-note", nil))
	assertRESTErrorProjection(t, noteMissing, http.StatusNotFound, "note_not_found")
	if fmt.Sprint(snapshotFileTree(t, root)) != fmt.Sprint(before) {
		t.Fatalf("RPC write gates modified vault")
	}

	created := httptest.NewRecorder()
	writer.Handler().ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.Create","params":{"path":"rpc-write","yes":true}}`)))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"command":"folder.create"`) || !fileExistsForAPITest(filepath.Join(root, "rpc-write")) {
		t.Fatalf("allow-write RPC create response: status=%d body=%s", created.Code, created.Body.String())
	}
}

func TestLocalAPIRPCAuthScopeAndHiddenGroupUseMethodMetadata(t *testing.T) {
	ctx := context.Background()
	root, svc, _, _ := newAPITestVault(t, ctx)
	store := NewMemoryTokenStore()
	readRec, readSecret := GenerateTokenRecord("folders-read", map[TokenScope]ScopeTarget{ScopeRead: {Groups: []string{"folders"}}}, "", "test")
	_ = store.Create(readRec)
	writeRec, writeSecret := GenerateTokenRecord("folders-write", map[TokenScope]ScopeTarget{ScopeWrite: {Groups: []string{"folders"}}}, "", "test")
	_ = store.Create(writeRec)
	server := &Server{service: svc, vault: root, allowWrite: true, authMode: AuthModeTemp, tokenStore: store}

	readReq := httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.List","params":{"include_empty":true}}`))
	readReq.Header.Set("Authorization", "Bearer "+readSecret)
	readRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(readRes, readReq)
	if readRes.Code != http.StatusOK || !strings.Contains(readRes.Body.String(), `"command":"folder.list"`) {
		t.Fatalf("read scoped RPC response: status=%d body=%s", readRes.Code, readRes.Body.String())
	}

	writeReq := httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.Create","params":{"path":"scoped-write","yes":true}}`))
	writeReq.Header.Set("Authorization", "Bearer "+readSecret)
	writeRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(writeRes, writeReq)
	assertRESTErrorProjection(t, writeRes, http.StatusForbidden, "insufficient_scope")

	writeReq = httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.Create","params":{"path":"scoped-write","yes":true}}`))
	writeReq.Header.Set("Authorization", "Bearer "+writeSecret)
	writeRes = httptest.NewRecorder()
	server.Handler().ServeHTTP(writeRes, writeReq)
	if writeRes.Code != http.StatusOK || !fileExistsForAPITest(filepath.Join(root, "scoped-write")) {
		t.Fatalf("write scoped RPC response: status=%d body=%s", writeRes.Code, writeRes.Body.String())
	}

	hidden := NewServerWithOptions(svc, root, ServerOptions{HideGroups: []string{"folders"}})
	hiddenRes := httptest.NewRecorder()
	hidden.Handler().ServeHTTP(hiddenRes, httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"method":"Pinax.Folder.List"}`)))
	assertRESTErrorProjection(t, hiddenRes, http.StatusNotFound, "route_not_found")
}

func TestLocalRESTRoutesMatchRegistry(t *testing.T) {
	ctx := context.Background()
	root, svc, itemID, noteRef := newAPITestVault(t, ctx)
	// Add inbox fixture
	writeAPIFixture(t, filepath.Join(root, "inbox", "inbox-note-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_inbox_api_1\ntitle: API Inbox Item\nstatus: inbox\nkind: inbox\n---\n\ninbox body\n")
	// Add draft fixture
	writeAPIFixture(t, filepath.Join(root, "drafts", "draft-note-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_draft_api_1\ntitle: API Draft Item\nstatus: draft\nkind: draft\n---\n\ndraft body\n")
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	server := NewServer(svc, root)

	fixtures := map[string]struct {
		method      string
		path        string
		wantStatus  int
		wantCommand string
	}{
		"rest.project.board.show": {method: http.MethodGet, path: "/v1/projects/research/board?note_display=card", wantStatus: http.StatusOK, wantCommand: "project.board.show"},
		"rest.note.read":          {method: http.MethodGet, path: "/v1/notes/" + noteRef + "?display=card", wantStatus: http.StatusOK, wantCommand: "note.show"},
		"rest.project.item.plan":  {method: http.MethodPost, path: "/v1/project-items/" + itemID + ":archive", wantStatus: http.StatusBadRequest, wantCommand: "project.item.plan"},
		"rest.folder.list":        {method: http.MethodGet, path: "/v1/folders", wantStatus: http.StatusOK, wantCommand: "folder.list"},
		"rest.folder.show":        {method: http.MethodGet, path: "/v1/folders/research", wantStatus: http.StatusOK, wantCommand: "folder.show"},
		"rest.folder.create":      {method: http.MethodPost, path: "/v1/folders?path=api-created&yes=true", wantStatus: http.StatusForbidden, wantCommand: "folder.create"},
		"rest.folder.rename":      {method: http.MethodPost, path: "/v1/folders/research:rename?target_path=research-renamed&yes=true", wantStatus: http.StatusForbidden, wantCommand: "folder.rename"},
		"rest.folder.move":        {method: http.MethodPost, path: "/v1/folders/research:move?target_parent=api-target&yes=true", wantStatus: http.StatusForbidden, wantCommand: "folder.move"},
		"rest.folder.delete":      {method: http.MethodPost, path: "/v1/folders/research:delete?empty_only=true&yes=true", wantStatus: http.StatusForbidden, wantCommand: "folder.delete"},
		"rest.folder.adopt":       {method: http.MethodPost, path: "/v1/folders/research:adopt?purpose=notes&yes=true", wantStatus: http.StatusForbidden, wantCommand: "folder.adopt"},
		"rest.folder.repair":      {method: http.MethodPost, path: "/v1/folders:repair-plan", wantStatus: http.StatusOK, wantCommand: "folder.repair"},
		// Inbox REST fixtures
		"rest.inbox.list":    {method: http.MethodGet, path: "/v1/inbox", wantStatus: http.StatusOK, wantCommand: "inbox.list"},
		"rest.inbox.show":    {method: http.MethodGet, path: "/v1/inbox/note_inbox_api_1", wantStatus: http.StatusOK, wantCommand: "inbox.show"},
		"rest.inbox.capture": {method: http.MethodPost, path: "/v1/inbox:capture", wantStatus: http.StatusForbidden, wantCommand: "inbox.capture"},
		"rest.inbox.promote": {method: http.MethodPost, path: "/v1/inbox/note_inbox_api_1:promote", wantStatus: http.StatusForbidden, wantCommand: "inbox.promote"},
		"rest.inbox.discard": {method: http.MethodPost, path: "/v1/inbox/note_inbox_api_1:discard", wantStatus: http.StatusForbidden, wantCommand: "inbox.discard"},
		// Draft REST fixtures
		"rest.draft.list":    {method: http.MethodGet, path: "/v1/drafts", wantStatus: http.StatusOK, wantCommand: "draft.list"},
		"rest.draft.show":    {method: http.MethodGet, path: "/v1/drafts/note_draft_api_1", wantStatus: http.StatusOK, wantCommand: "draft.show"},
		"rest.draft.create":  {method: http.MethodPost, path: "/v1/drafts", wantStatus: http.StatusForbidden, wantCommand: "draft.create"},
		"rest.draft.promote": {method: http.MethodPost, path: "/v1/drafts/note_draft_api_1:promote", wantStatus: http.StatusForbidden, wantCommand: "draft.promote"},
		"rest.draft.archive": {method: http.MethodPost, path: "/v1/drafts/note_draft_api_1:archive", wantStatus: http.StatusForbidden, wantCommand: "draft.archive"},
		"rest.draft.discard": {method: http.MethodPost, path: "/v1/drafts/note_draft_api_1:discard", wantStatus: http.StatusForbidden, wantCommand: "draft.discard"},
	}

	for _, route := range app.RemoteRoutes() {
		if route.Surface != "rest" {
			continue
		}
		fixture, ok := fixtures[route.RouteID]
		if !ok {
			t.Fatalf("missing representative REST fixture for route %s", route.RouteID)
		}
		if fixture.method != route.Method {
			t.Fatalf("fixture method for %s = %s, want registry method %s", route.RouteID, fixture.method, route.Method)
		}
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, httptest.NewRequest(fixture.method, fixture.path, nil))
		if res.Code != fixture.wantStatus {
			t.Fatalf("route %s status = %d, want %d; body=%s", route.RouteID, res.Code, fixture.wantStatus, res.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("route %s returned invalid JSON: %v\n%s", route.RouteID, err, res.Body.String())
		}
		if payload["command"] != fixture.wantCommand {
			t.Fatalf("route %s command = %#v, want %s; body=%s", route.RouteID, payload["command"], fixture.wantCommand, res.Body.String())
		}
	}
}

func TestLocalAPIRootReturnsDiscoveryProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("root status = %d, body=%s", res.Code, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("root returned invalid JSON: %v\n%s", err, res.Body.String())
	}
	if payload["command"] != "api.root" || payload["status"] != "success" {
		t.Fatalf("root projection = %#v", payload)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok || data["capabilities_url"] != "/v1/capabilities" || data["routes_command"] != "pinax api routes --vault <vault> --json" {
		t.Fatalf("root discovery data = %#v", payload["data"])
	}
}

func TestLocalAPIRequestLoggerUsesZapAndRedactsSecrets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		MessageKey:     "msg",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	})
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.DebugLevel))
	server := NewServerWithOptions(svc, root, ServerOptions{AuthMode: AuthModeNone, Logger: logger})

	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities?token=secret-token", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.RemoteAddr = "127.0.0.1:12345"
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d body=%s", res.Code, res.Body.String())
	}
	logs := buf.String()
	for _, want := range []string{"api.request", `"method":"GET"`, `"path":"/v1/capabilities"`, `"status":200`, `"duration"`} {
		if !strings.Contains(logs, want) {
			t.Fatalf("request logs missing %q: %s", want, logs)
		}
	}
	for _, forbidden := range []string{"secret-token", "Authorization", "Bearer"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("request logs leaked %q: %s", forbidden, logs)
		}
	}
}

func TestLocalAPIRPCRequestLoggerIncludesOperationFields(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	})
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.DebugLevel))
	server := NewServerWithOptions(svc, root, ServerOptions{AllowWrite: true, AuthMode: AuthModeNone, Logger: logger})

	req := httptest.NewRequest(http.MethodPost, "/v1/rpc", strings.NewReader(`{"id":"call-1","method":"Pinax.Folder.Create","params":{"path":"rpc-logs","purpose":"notes"}}`))
	req.RemoteAddr = "127.0.0.1:12345"
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	assertRESTErrorProjection(t, res, http.StatusBadRequest, "approval_required")

	logs := buf.String()
	for _, want := range []string{`"msg":"api.rpc"`, `"rpc_method":"Pinax.Folder.Create"`, `"rpc_id":"call-1"`, `"command":"folder.create"`, `"group":"folders"`, `"readonly":false`, `"status":400`, `"error_code":"approval_required"`} {
		if !strings.Contains(logs, want) {
			t.Fatalf("rpc logs missing %q: %s", want, logs)
		}
	}
	for _, forbidden := range []string{"rpc-logs", "notes", "params"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("rpc logs leaked request detail %q: %s", forbidden, logs)
		}
	}
}

func TestLocalRESTMethodAndRouteErrorsUseProjectionEnvelope(t *testing.T) {
	ctx := context.Background()
	root, svc, itemID, _ := newAPITestVault(t, ctx)
	server := NewServer(svc, root)

	methodRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(methodRes, httptest.NewRequest(http.MethodGet, "/v1/project-items/"+itemID+":archive", nil))
	assertRESTErrorProjection(t, methodRes, http.StatusMethodNotAllowed, "method_not_allowed")

	routeRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(routeRes, httptest.NewRequest(http.MethodGet, "/v1/unknown", nil))
	assertRESTErrorProjection(t, routeRes, http.StatusNotFound, "route_not_found")
}

func TestLocalRESTRemoteWriteGatesDoNotModifyVaultAndStayRedacted(t *testing.T) {
	ctx := context.Background()
	root, svc, itemID, _ := newAPITestVault(t, ctx)
	server := NewServer(svc, root)
	before := snapshotFileTree(t, root)

	archiveApproval := httptest.NewRecorder()
	server.Handler().ServeHTTP(archiveApproval, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":archive?token=secret-token&webhook=https://hooks.example.invalid/leak", nil))
	assertRESTErrorProjection(t, archiveApproval, http.StatusBadRequest, "approval_required")
	assertNoSecretLeak(t, archiveApproval.Body.String())

	archiveSnapshot := httptest.NewRecorder()
	server.Handler().ServeHTTP(archiveSnapshot, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":archive?yes=true", nil))
	assertRESTErrorProjection(t, archiveSnapshot, http.StatusBadRequest, "snapshot_required")
	if !strings.Contains(archiveSnapshot.Body.String(), "pinax version snapshot") {
		t.Fatalf("snapshot gate should include runnable snapshot action or hint: %s", archiveSnapshot.Body.String())
	}
	assertNoSecretLeak(t, archiveSnapshot.Body.String())

	moveApproval := httptest.NewRecorder()
	server.Handler().ServeHTTP(moveApproval, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":move?column=done", nil))
	assertRESTErrorProjection(t, moveApproval, http.StatusBadRequest, "approval_required")

	moveSnapshot := httptest.NewRecorder()
	server.Handler().ServeHTTP(moveSnapshot, httptest.NewRequest(http.MethodPost, "/v1/project-items/"+itemID+":move?column=done&yes=true", nil))
	assertRESTErrorProjection(t, moveSnapshot, http.StatusBadRequest, "snapshot_required")
	if !strings.Contains(moveSnapshot.Body.String(), "pinax version snapshot") {
		t.Fatalf("move snapshot gate should include runnable snapshot action or hint: %s", moveSnapshot.Body.String())
	}

	after := snapshotFileTree(t, root)
	if fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("remote write gates modified vault files: before=%#v after=%#v", before, after)
	}
}

func newAPITestVault(t *testing.T, ctx context.Context) (string, *app.Service, string, string) {
	t.Helper()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "Research", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := svc.ProjectItemAdd(ctx, app.ProjectItemRequest{VaultPath: root, Project: "research", Title: "API Item", Column: "next", Body: "secret body"})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	itemID := created.Facts["item_id"]
	noteRef := "note" + strings.TrimPrefix(itemID, "item")
	return root, svc, itemID, noteRef
}

func assertRESTErrorProjection(t *testing.T, res *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if res.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, wantStatus, res.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error response is not JSON projection: %v\n%s", err, res.Body.String())
	}
	errorObject, ok := payload["error"].(map[string]any)
	if !ok || errorObject["code"] != wantCode || payload["status"] != "failed" {
		t.Fatalf("error projection code/status mismatch: body=%s", res.Body.String())
	}
}

func snapshotFileTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[rel] = fmt.Sprintf("%x", sha256.Sum256(content))
		return nil
	}); err != nil {
		t.Fatalf("snapshot file tree: %v", err)
	}
	return snapshot
}

func assertNoSecretLeak(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{"secret-token", "Authorization", "Cookie", "hooks.example.invalid", "raw provider payload", "hidden prompt"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked forbidden content %q: %s", forbidden, body)
		}
	}
}

func fileExistsForAPITest(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeAPIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// --- Auth integration tests ---

func TestAuthIntegration_TempTokenFullFlow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "AuthTest"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	opts := ServerOptions{AuthMode: AuthModeTemp}
	s := NewServerWithOptions(svc, root, opts)
	secret := s.tempSecret
	if secret == "" {
		t.Fatal("expected temp secret to be generated")
	}
	handler := s.Handler()

	// Without token: 401
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", res.Code)
	}

	// With valid token: 200
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthIntegration_FileTokenStoreFullFlow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "AuthTest"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	// Create a file token store with a token
	tokenPath := filepath.Join(root, ".pinax", "tokens", "tokens.json")
	fileStore, err := NewFileTokenStore(tokenPath)
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}
	rec, secret := GenerateTokenRecord("integration", map[TokenScope]ScopeTarget{
		ScopeRead:  {},
		ScopeWrite: {},
	}, "", "test")
	if err := fileStore.Create(rec); err != nil {
		t.Fatalf("Create token: %v", err)
	}

	opts := ServerOptions{AuthMode: AuthModeTokenFile, TokenFile: tokenPath}
	s := NewServerWithOptions(svc, root, opts)
	handler := s.Handler()

	// Without token: 401
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", res.Code)
	}

	// With valid file token: 200
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with file token, got %d: %s", res.Code, res.Body.String())
	}
}

func TestAuthIntegration_ScopedTokenCannotAccessOtherGroups(t *testing.T) {
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("read-only", map[TokenScope]ScopeTarget{
		ScopeRead: {Groups: []string{"capabilities"}},
	}, "", "test")
	_ = store.Create(rec)

	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	s := &Server{service: svc, vault: root, authMode: AuthModeTemp, tokenStore: store}
	handler := s.Handler()

	// Can access capabilities (read + capabilities group)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for capabilities, got %d: %s", res.Code, res.Body.String())
	}

	// Cannot access folders (read + folders group not in scope)
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/folders", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for folders, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "insufficient_scope") {
		t.Fatalf("expected insufficient_scope, got: %s", res.Body.String())
	}
}

func TestAuthIntegration_ExpiredTokenRejected(t *testing.T) {
	store := NewMemoryTokenStore()
	rec, secret := GenerateTokenRecord("expired", map[TokenScope]ScopeTarget{
		ScopeRead: {},
	}, time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339), "test")
	_ = store.Create(rec)

	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	s := &Server{service: svc, vault: root, authMode: AuthModeTemp, tokenStore: store}
	handler := s.Handler()

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "token_expired") {
		t.Fatalf("expected token_expired, got: %s", res.Body.String())
	}
}
