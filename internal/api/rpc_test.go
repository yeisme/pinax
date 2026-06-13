package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestLocalRPCProjectBoardNoteAndProjectItemPlan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "Research", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	created, err := svc.ProjectItemAdd(ctx, app.ProjectItemRequest{VaultPath: root, Project: "research", Title: "RPC Item", Column: "next", Body: "secret body"})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	itemID := created.Facts["item_id"]
	noteRef := "note" + strings.TrimPrefix(itemID, "item")
	rpc := NewRPCDispatcher(svc, root)

	board, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.ProjectBoard.Show", Params: map[string]any{"project": "research", "note_display": "card"}})
	if err != nil || board.Command != "project.board.show" || board.Facts["project"] != "research" {
		t.Fatalf("board rpc projection=%#v err=%v", board, err)
	}
	note, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.Note.Read", Params: map[string]any{"ref": noteRef, "display": "card"}})
	if err != nil || note.Command != "note.show" || strings.Contains(note.Facts["display"], "body") {
		t.Fatalf("note rpc projection=%#v err=%v", note, err)
	}
	defaultNote, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.Note.Read", Params: map[string]any{"ref": noteRef}})
	if err != nil || defaultNote.Command != "note.show" || defaultNote.Facts["display"] != "card" {
		t.Fatalf("default note rpc projection=%#v err=%v", defaultNote, err)
	}
	if data, ok := defaultNote.Data.(map[string]any); !ok || data["body"] != nil {
		t.Fatalf("default note rpc exposed full body: %#v", defaultNote.Data)
	}
	plan, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.ProjectItem.Plan", Params: map[string]any{"action": "archive", "item_id": itemID, "yes": true}})
	if err == nil || plan.Error == nil || plan.Error.Code != "snapshot_required" {
		t.Fatalf("item plan should require snapshot: projection=%#v err=%v", plan, err)
	}
	plan, err = rpc.Call(ctx, RPCRequest{Method: "Pinax.ProjectItem.Plan", Params: map[string]any{"action": "move", "item_id": itemID, "column": "done"}})
	if err == nil || plan.Error == nil || plan.Error.Code != "approval_required" {
		t.Fatalf("move done plan should require approval: projection=%#v err=%v", plan, err)
	}
	plan, err = rpc.Call(ctx, RPCRequest{Method: "Pinax.ProjectItem.Plan", Params: map[string]any{"action": "move", "item_id": itemID, "column": "done", "yes": true}})
	if err == nil || plan.Error == nil || plan.Error.Code != "snapshot_required" {
		t.Fatalf("move done plan should require snapshot: projection=%#v err=%v", plan, err)
	}
}

func TestLocalRPCNoteListSupportsCLIQueryFilters(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAPIFixture(t, filepath.Join(root, "notes", "research-go.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_research_go\ntitle: Research Go\nstatus: active\nkind: reference\ntags: [research, go]\nproject: work\ngroup: work\npriority: high\n---\n\nbody\n")
	writeAPIFixture(t, filepath.Join(root, "notes", "other.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_other\ntitle: Other\nstatus: archived\nkind: reference\ntags: [personal]\n---\n\nbody\n")
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	rpc := NewRPCDispatcher(svc, root)

	projection, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.Note.List", Params: map[string]any{"tags": []any{"research", "go"}, "project": "work", "group": "work", "status": "active", "limit": 10, "properties": []any{"priority"}, "strict_properties": true}})
	if err != nil || projection.Command != "note.list" || projection.Facts["notes"] != "1" {
		t.Fatalf("note list rpc projection=%#v err=%v", projection, err)
	}
	body, err := json.Marshal(projection.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	if !strings.Contains(string(body), "Research Go") || strings.Contains(string(body), "Other") {
		t.Fatalf("note list rpc data = %s", body)
	}
}

func TestLocalRPCFolderRoutesAndWriteGate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	readonly := NewRPCDispatcher(svc, root)
	list, err := readonly.Call(ctx, RPCRequest{Method: "Pinax.Folder.List", Params: map[string]any{"include_empty": true}})
	if err != nil || list.Command != "folder.list" {
		t.Fatalf("folder list rpc projection=%#v err=%v", list, err)
	}
	blocked, err := readonly.Call(ctx, RPCRequest{Method: "Pinax.Folder.Create", Params: map[string]any{"path": "spaces/rpc", "purpose": "notes", "yes": true}})
	if err == nil || blocked.Error == nil || blocked.Error.Code != "write_disabled" {
		t.Fatalf("readonly folder create should fail with write_disabled: projection=%#v err=%v", blocked, err)
	}

	writer := NewRPCDispatcherWithOptions(svc, root, DispatcherOptions{AllowWrite: true})
	approval, err := writer.Call(ctx, RPCRequest{Method: "Pinax.Folder.Create", Params: map[string]any{"path": "spaces/rpc", "purpose": "notes"}})
	if err == nil || approval.Error == nil || approval.Error.Code != "approval_required" {
		t.Fatalf("folder create without approval should fail: projection=%#v err=%v", approval, err)
	}
	created, err := writer.Call(ctx, RPCRequest{Method: "Pinax.Folder.Create", Params: map[string]any{"path": "spaces/rpc", "purpose": "notes", "yes": true}})
	if err != nil || created.Command != "folder.create" || created.Facts["folder_path"] != "spaces/rpc" {
		t.Fatalf("folder create rpc projection=%#v err=%v", created, err)
	}
	renamed, err := writer.Call(ctx, RPCRequest{Method: "Pinax.Folder.Rename", Params: map[string]any{"path": "spaces/rpc", "target_path": "spaces/rpc-renamed", "yes": true}})
	if err == nil || renamed.Error == nil || renamed.Error.Code != "snapshot_required" {
		t.Fatalf("folder rename without snapshot should fail: projection=%#v err=%v", renamed, err)
	}
	if _, err := svc.VersionSnapshot(ctx, app.SnapshotRequest{VaultPath: root, Message: "folder rpc"}); err != nil {
		t.Fatalf("version snapshot: %v", err)
	}
	renamed, err = writer.Call(ctx, RPCRequest{Method: "Pinax.Folder.Rename", Params: map[string]any{"path": "spaces/rpc", "target_path": "spaces/rpc-renamed", "yes": true}})
	if err != nil || renamed.Command != "folder.rename" || renamed.Facts["target_path"] != "spaces/rpc-renamed" {
		t.Fatalf("folder rename rpc projection=%#v err=%v", renamed, err)
	}
}
func TestLocalRPCCreateDryRunDoesNotWriteNotes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	rpc := NewRPCDispatcherWithOptions(svc, root, DispatcherOptions{AllowWrite: true})

	projection, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.Inbox.Capture", Params: map[string]any{"title": "Preview Only", "dry_run": true}})
	if err != nil || projection.Command != "inbox.capture" || projection.Facts["planned_path"] == "" {
		t.Fatalf("dry-run capture projection=%#v err=%v", projection, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, projection.Facts["planned_path"])); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run capture wrote note file: stat err=%v", statErr)
	}

	projection, err = rpc.Call(ctx, RPCRequest{Method: "Pinax.Draft.Create", Params: map[string]any{"title": "Draft Preview", "dry_run": true}})
	if err != nil || projection.Command != "draft.create" || projection.Facts["planned_path"] == "" {
		t.Fatalf("dry-run draft projection=%#v err=%v", projection, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, projection.Facts["planned_path"])); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run draft wrote note file: stat err=%v", statErr)
	}
}

func TestLocalRPCSyncPushPullUsesWriteGateAndService(t *testing.T) {
	ctx := context.Background()
	objectRoot := t.TempDir()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o700); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"), []byte("# Alpha\n\nrpc body\n"), 0o600); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if _, err := svc.CloudLogin(ctx, app.CloudLoginRequest{VaultPath: root, Endpoint: "file://" + objectRoot, WorkspaceID: "personal", DeviceID: "rpc", SecretRef: "env://PINAX_TEST_SECRET"}); err != nil {
		t.Fatalf("cloud login: %v", err)
	}
	readonly := NewRPCDispatcher(svc, root)
	blocked, err := readonly.Call(ctx, RPCRequest{Method: "Pinax.Sync.Push", Params: map[string]any{"target": "cloud", "yes": true}})
	if err == nil || blocked.Error == nil || blocked.Error.Code != "write_disabled" {
		t.Fatalf("readonly sync push should fail with write_disabled: projection=%#v err=%v", blocked, err)
	}
	writer := NewRPCDispatcherWithOptions(svc, root, DispatcherOptions{AllowWrite: true})
	approval, err := writer.Call(ctx, RPCRequest{Method: "Pinax.Sync.Push", Params: map[string]any{"target": "cloud"}})
	if err == nil || approval.Error == nil || approval.Error.Code != "approval_required" {
		t.Fatalf("sync push without yes should require approval: projection=%#v err=%v", approval, err)
	}
	pushed, err := writer.Call(ctx, RPCRequest{Method: "Pinax.Sync.Push", Params: map[string]any{"target": "cloud", "yes": true}})
	if err != nil || pushed.Command != "sync.push" || pushed.Facts["remote_write"] != "true" {
		t.Fatalf("sync push rpc projection=%#v err=%v", pushed, err)
	}
	pulled, err := writer.Call(ctx, RPCRequest{Method: "Pinax.Sync.Pull", Params: map[string]any{"target": "cloud", "yes": true}})
	if err != nil || pulled.Command != "sync.pull" || pulled.Facts["remote_write"] != "false" {
		t.Fatalf("sync pull rpc projection=%#v err=%v", pulled, err)
	}
}

func TestLocalRPCRoutesMatchRegistry(t *testing.T) {
	ctx := context.Background()
	root, svc, itemID, noteRef := newAPITestVault(t, ctx)
	// Add inbox and draft fixtures
	writeAPIFixture(t, filepath.Join(root, "inbox", "inbox-rpc-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_inbox_rpc_1\ntitle: RPC Inbox\nstatus: inbox\nkind: inbox\n---\n\ninbox rpc\n")
	writeAPIFixture(t, filepath.Join(root, "drafts", "draft-rpc-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_draft_rpc_1\ntitle: RPC Draft\nstatus: draft\nkind: draft\n---\n\ndraft rpc\n")
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	rpc := NewRPCDispatcher(svc, root)

	fixtures := map[string]RPCRequest{
		"rpc.project.board.show": {Method: "Pinax.ProjectBoard.Show", Params: map[string]any{"project": "research", "note_display": "card"}},
		"rpc.note.read":          {Method: "Pinax.Note.Read", Params: map[string]any{"ref": noteRef, "display": "card"}},
		"rpc.note.list":          {Method: "Pinax.Note.List", Params: map[string]any{"status": "active"}},
		"rpc.project.item.plan":  {Method: "Pinax.ProjectItem.Plan", Params: map[string]any{"item_id": itemID, "action": "archive"}},
		"rpc.folder.list":        {Method: "Pinax.Folder.List", Params: map[string]any{}},
		"rpc.folder.show":        {Method: "Pinax.Folder.Show", Params: map[string]any{"path": "research"}},
		"rpc.folder.create":      {Method: "Pinax.Folder.Create", Params: map[string]any{"path": "rpc-created", "yes": true}},
		"rpc.folder.rename":      {Method: "Pinax.Folder.Rename", Params: map[string]any{"path": "research", "target_path": "research-renamed", "yes": true}},
		"rpc.folder.move":        {Method: "Pinax.Folder.Move", Params: map[string]any{"path": "research", "target_parent": "api-target", "yes": true}},
		"rpc.folder.delete":      {Method: "Pinax.Folder.Delete", Params: map[string]any{"path": "research", "empty_only": true, "yes": true}},
		"rpc.folder.adopt":       {Method: "Pinax.Folder.Adopt", Params: map[string]any{"path": "research", "purpose": "notes", "yes": true}},
		"rpc.folder.repair":      {Method: "Pinax.Folder.RepairPlan", Params: map[string]any{}},
		// Inbox RPC fixtures
		"rpc.inbox.list":    {Method: "Pinax.Inbox.List", Params: map[string]any{}},
		"rpc.inbox.show":    {Method: "Pinax.Inbox.Show", Params: map[string]any{"ref": "note_inbox_rpc_1"}},
		"rpc.inbox.capture": {Method: "Pinax.Inbox.Capture", Params: map[string]any{"title": "RPC Capture", "yes": true}},
		"rpc.inbox.promote": {Method: "Pinax.Inbox.Promote", Params: map[string]any{"ref": "note_inbox_rpc_1", "to": "active"}},
		"rpc.inbox.discard": {Method: "Pinax.Inbox.Discard", Params: map[string]any{"ref": "note_inbox_rpc_1"}},
		// Draft RPC fixtures
		"rpc.draft.list":    {Method: "Pinax.Draft.List", Params: map[string]any{}},
		"rpc.draft.show":    {Method: "Pinax.Draft.Show", Params: map[string]any{"ref": "note_draft_rpc_1"}},
		"rpc.draft.create":  {Method: "Pinax.Draft.Create", Params: map[string]any{"title": "RPC Draft Create", "yes": true}},
		"rpc.draft.promote": {Method: "Pinax.Draft.Promote", Params: map[string]any{"ref": "note_draft_rpc_1"}},
		"rpc.draft.archive": {Method: "Pinax.Draft.Archive", Params: map[string]any{"ref": "note_draft_rpc_1"}},
		"rpc.draft.discard": {Method: "Pinax.Draft.Discard", Params: map[string]any{"ref": "note_draft_rpc_1"}},
		// Sync RPC fixtures
		"rpc.sync.push": {Method: "Pinax.Sync.Push", Params: map[string]any{"target": "cloud", "yes": true}},
		"rpc.sync.pull": {Method: "Pinax.Sync.Pull", Params: map[string]any{"target": "cloud", "yes": true}},
	}

	for _, route := range app.RemoteRoutes() {
		if route.Surface != "rpc" {
			continue
		}
		req, ok := fixtures[route.RouteID]
		if !ok {
			t.Fatalf("missing representative RPC params for route %s", route.RouteID)
		}
		if req.Method != route.RPCMethod {
			t.Fatalf("fixture method for %s = %s, want registry method %s", route.RouteID, req.Method, route.RPCMethod)
		}
		projection, _ := rpc.Call(ctx, req)
		if projection.Error != nil && projection.Error.Code == "rpc_method_not_found" {
			t.Fatalf("registered RPC route %s returned rpc_method_not_found", route.RouteID)
		}
		if projection.Command != route.Command {
			t.Fatalf("registered RPC route %s command = %s, want %s", route.RouteID, projection.Command, route.Command)
		}
	}
}

func TestLocalRPCUnknownMethodReturnsStableProjection(t *testing.T) {
	ctx := context.Background()
	root, svc, _, _ := newAPITestVault(t, ctx)
	rpc := NewRPCDispatcher(svc, root)

	projection, err := rpc.Call(ctx, RPCRequest{Method: "Pinax.Unknown"})
	if err == nil || projection.Error == nil || projection.Error.Code != "rpc_method_not_found" {
		t.Fatalf("unknown rpc should fail with rpc_method_not_found: projection=%#v err=%v", projection, err)
	}
	if !strings.Contains(projection.Error.Hint, "pinax api routes") {
		t.Fatalf("unknown rpc hint should mention pinax api routes: %#v", projection.Error)
	}
}

func TestLocalRPCRemoteWriteGateResponsesStayRedacted(t *testing.T) {
	ctx := context.Background()
	root, svc, itemID, _ := newAPITestVault(t, ctx)
	rpc := NewRPCDispatcher(svc, root)

	projection, _ := rpc.Call(ctx, RPCRequest{Method: "Pinax.ProjectItem.Plan", Params: map[string]any{"item_id": itemID, "action": "archive", "token": "secret-token", "authorization": "Bearer hidden"}})
	body, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	assertNoSecretLeak(t, string(body))
}

func TestLocalRPCAndRESTCapabilityMetadataStaysAligned(t *testing.T) {
	routesByCapability := map[string][]string{}
	routes := app.RemoteRoutes()
	for _, route := range routes {
		routesByCapability[route.CapabilityID] = append(routesByCapability[route.CapabilityID], route.Surface)
	}

	for capabilityID, surfaces := range routesByCapability {
		if !containsSurface(surfaces, "rest") || !containsSurface(surfaces, "rpc") {
			continue
		}
		var restRoute, rpcRoute *testingRemoteRoute
		for _, route := range routes {
			if route.CapabilityID != capabilityID {
				continue
			}
			copy := testingRemoteRoute{Command: route.Command, SchemaVersion: route.SchemaVersion, Readonly: route.Readonly, BodyAllowed: route.BodyAllowed, ApprovalRequired: route.ApprovalRequired, SnapshotRequired: route.SnapshotRequired}
			if route.Surface == "rest" {
				restRoute = &copy
			}
			if route.Surface == "rpc" {
				rpcRoute = &copy
			}
		}
		if restRoute == nil || rpcRoute == nil || *restRoute != *rpcRoute {
			t.Fatalf("capability %s REST/RPC metadata drift: rest=%#v rpc=%#v", capabilityID, restRoute, rpcRoute)
		}
	}
}

type testingRemoteRoute struct {
	Command          string
	SchemaVersion    string
	Readonly         bool
	BodyAllowed      bool
	ApprovalRequired bool
	SnapshotRequired bool
}

func containsSurface(surfaces []string, want string) bool {
	for _, surface := range surfaces {
		if surface == want {
			return true
		}
	}
	return false
}
