package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

type RPCRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type RPCDispatcher struct {
	service    *app.Service
	vault      string
	allowWrite bool
}

type DispatcherOptions struct {
	AllowWrite bool
}

func NewRPCDispatcher(service *app.Service, vault string) *RPCDispatcher {
	return NewRPCDispatcherWithOptions(service, vault, DispatcherOptions{})
}

func NewRPCDispatcherWithOptions(service *app.Service, vault string, options DispatcherOptions) *RPCDispatcher {
	return &RPCDispatcher{service: service, vault: vault, allowWrite: options.AllowWrite}
}

func (d *RPCDispatcher) Call(ctx context.Context, req RPCRequest) (domain.Projection, error) {
	switch req.Method {
	case "Pinax.Workbench.Status":
		writeMode := "remote_readonly"
		if d.allowWrite {
			writeMode = "remote_allow_write"
		}
		projection, err := d.service.WorkbenchStatus(ctx, app.APIRequest{VaultPath: d.vault, WriteMode: writeMode})
		projection.Mode = "json"
		return projection, err
	case "Pinax.ProjectBoard.Show":
		projection, err := d.service.ProjectBoardShow(ctx, app.ProjectBoardRequest{VaultPath: d.vault, Project: stringParam(req.Params, "project"), Subproject: stringParam(req.Params, "subproject"), NoteDisplay: stringParam(req.Params, "note_display")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Project.Subproject.List":
		projection, err := d.service.ProjectSubprojectList(ctx, app.ProjectWorkspaceRequest{VaultPath: d.vault, Project: stringParam(req.Params, "project")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Project.Subproject.Show":
		projection, err := d.service.ProjectSubprojectShow(ctx, app.ProjectWorkspaceRequest{VaultPath: d.vault, Project: stringParam(req.Params, "project"), Subproject: stringParam(req.Params, "subproject")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Project.Subproject.Create":
		if projection, err := d.ensureWriteAllowed("project.subproject.create", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.ProjectSubprojectCreate(ctx, app.ProjectWorkspaceRequest{VaultPath: d.vault, Project: stringParam(req.Params, "project"), Subproject: stringParam(req.Params, "subproject"), Title: stringParam(req.Params, "title"), Template: stringParam(req.Params, "template"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Note.List":
		projection, err := d.service.ListNotesQuery(ctx, app.NoteListRequest{VaultPath: d.vault, Tags: stringSliceParam(req.Params, "tags"), Project: stringParam(req.Params, "project"), Group: stringParam(req.Params, "group"), Folder: stringParam(req.Params, "folder"), Kind: stringParam(req.Params, "kind"), Status: stringParam(req.Params, "status"), CreatedAfter: stringParam(req.Params, "created_after"), UpdatedBefore: stringParam(req.Params, "updated_before"), Recent: boolParam(req.Params, "recent"), Limit: intParam(req.Params, "limit"), Sort: stringParam(req.Params, "sort"), PathPrefix: stringParam(req.Params, "path_prefix"), Properties: stringSliceParam(req.Params, "properties"), StrictProperties: boolParam(req.Params, "strict_properties")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Note.Read":
		display := stringParam(req.Params, "display")
		if strings.TrimSpace(display) == "" {
			display = "card"
		}
		projection, err := d.service.ShowNoteProjection(ctx, app.ShowNoteRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref"), Display: display})
		projection.Mode = "json"
		return projection, err
	case "Pinax.DatabaseView.Render":
		projection, err := d.service.RenderDatabaseView(ctx, app.ViewRequest{VaultPath: d.vault, Name: stringParam(req.Params, "name")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Task.AdoptPlan":
		projection, err := d.service.TaskAdopt(ctx, app.TaskAdoptRequest{VaultPath: d.vault, ItemID: stringParam(req.Params, "item_id"), Yes: false})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Graph.Summary":
		projection, err := d.service.GraphSummaryProjection(ctx, d.vault)
		projection.Mode = "json"
		return projection, err
	case "Pinax.ProjectItem.Plan":
		projection, err := d.service.ProjectItemPlan(ctx, app.ProjectItemRequest{VaultPath: d.vault, ItemID: stringParam(req.Params, "item_id"), Action: stringParam(req.Params, "action"), Column: stringParam(req.Params, "column"), Yes: boolParam(req.Params, "yes")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.List":
		projection, err := d.service.ListFolders(ctx, app.FolderListRequest{VaultPath: d.vault, Purpose: stringParam(req.Params, "purpose"), Under: stringParam(req.Params, "under"), IncludeEmpty: boolParam(req.Params, "include_empty"), Depth: intParam(req.Params, "depth")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.Show":
		projection, err := d.service.ShowFolder(ctx, app.FolderRequest{VaultPath: d.vault, Path: stringParam(req.Params, "path")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.Create":
		if projection, err := d.ensureWriteAllowed("folder.create", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.CreateFolder(ctx, app.FolderOperationRequest{VaultPath: d.vault, Path: stringParam(req.Params, "path"), Purpose: stringParam(req.Params, "purpose"), DryRun: boolParam(req.Params, "dry_run"), Yes: boolParam(req.Params, "yes")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.Rename":
		if projection, err := d.ensureWriteAllowed("folder.rename", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.RenameFolder(ctx, app.FolderOperationRequest{VaultPath: d.vault, Path: stringParam(req.Params, "path"), TargetPath: stringParam(req.Params, "target_path"), DryRun: boolParam(req.Params, "dry_run"), Yes: boolParam(req.Params, "yes"), RequireSnapshot: true})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.Move":
		if projection, err := d.ensureWriteAllowed("folder.move", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.MoveFolder(ctx, app.FolderOperationRequest{VaultPath: d.vault, Path: stringParam(req.Params, "path"), TargetParent: stringParam(req.Params, "target_parent"), DryRun: boolParam(req.Params, "dry_run"), Yes: boolParam(req.Params, "yes"), RequireSnapshot: true})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.Delete":
		if projection, err := d.ensureWriteAllowed("folder.delete", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.DeleteFolder(ctx, app.FolderOperationRequest{VaultPath: d.vault, Path: stringParam(req.Params, "path"), EmptyOnly: boolParam(req.Params, "empty_only"), DryRun: boolParam(req.Params, "dry_run"), Yes: boolParam(req.Params, "yes"), RequireSnapshot: true})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.Adopt":
		if projection, err := d.ensureWriteAllowed("folder.adopt", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.AdoptFolder(ctx, app.FolderOperationRequest{VaultPath: d.vault, Path: stringParam(req.Params, "path"), Purpose: stringParam(req.Params, "purpose"), DryRun: boolParam(req.Params, "dry_run"), Yes: boolParam(req.Params, "yes")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Folder.RepairPlan":
		projection, err := d.service.RepairFolders(ctx, app.FolderRepairRequest{VaultPath: d.vault, Plan: true})
		projection.Mode = "json"
		return projection, err
	// Inbox RPC methods
	case "Pinax.Inbox.List":
		projection, err := d.service.InboxList(ctx, app.VaultRequest{VaultPath: d.vault})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Inbox.Show":
		projection, err := d.service.InboxShow(ctx, app.ShowNoteRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Inbox.Capture":
		if projection, err := d.ensureWriteAllowed("inbox.capture", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.InboxCapture(ctx, app.CreateNoteRequest{VaultPath: d.vault, Title: stringParam(req.Params, "title"), Body: stringParam(req.Params, "body"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Inbox.Promote":
		if projection, err := d.ensureWriteAllowed("inbox.promote", req.Params); err != nil {
			return projection, err
		}
		to := stringParam(req.Params, "to")
		if to == "" {
			to = "active"
		}
		projection, err := d.service.InboxPromote(ctx, app.InboxPromoteRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref"), To: to, Group: stringParam(req.Params, "group"), Folder: stringParam(req.Params, "folder"), Kind: stringParam(req.Params, "kind"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Inbox.Discard":
		if projection, err := d.ensureWriteAllowed("inbox.discard", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.InboxDiscard(ctx, app.NoteMutationRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	// Draft RPC methods
	case "Pinax.Draft.List":
		projection, err := d.service.DraftList(ctx, app.VaultRequest{VaultPath: d.vault})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Draft.Show":
		projection, err := d.service.DraftShow(ctx, app.ShowNoteRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Draft.Create":
		if projection, err := d.ensureWriteAllowed("draft.create", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.DraftCreate(ctx, app.CreateNoteRequest{VaultPath: d.vault, Title: stringParam(req.Params, "title"), Body: stringParam(req.Params, "body"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Draft.Promote":
		if projection, err := d.ensureWriteAllowed("draft.promote", req.Params); err != nil {
			return projection, err
		}
		status := stringParam(req.Params, "status")
		if status == "" {
			status = "active"
		}
		projection, err := d.service.DraftPromote(ctx, app.DraftPromoteRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref"), Status: status, Folder: stringParam(req.Params, "folder"), Kind: stringParam(req.Params, "kind"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Draft.Archive":
		if projection, err := d.ensureWriteAllowed("draft.archive", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.DraftArchive(ctx, app.NoteMutationRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Draft.Discard":
		if projection, err := d.ensureWriteAllowed("draft.discard", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.DraftDiscard(ctx, app.NoteMutationRequest{VaultPath: d.vault, NoteRef: stringParam(req.Params, "ref"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Sync.Push":
		if projection, err := d.ensureWriteAllowed("sync.push", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.SyncPush(ctx, app.SyncRequest{VaultPath: d.vault, Target: stringParam(req.Params, "target"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run"), BaseRevision: stringParam(req.Params, "base_revision"), RemoteRevision: stringParam(req.Params, "remote_revision")})
		projection.Mode = "json"
		return projection, err
	case "Pinax.Sync.Pull":
		if projection, err := d.ensureWriteAllowed("sync.pull", req.Params); err != nil {
			return projection, err
		}
		projection, err := d.service.SyncPull(ctx, app.SyncRequest{VaultPath: d.vault, Target: stringParam(req.Params, "target"), Yes: boolParam(req.Params, "yes"), DryRun: boolParam(req.Params, "dry_run"), BaseRevision: stringParam(req.Params, "base_revision"), RemoteRevision: stringParam(req.Params, "remote_revision")})
		projection.Mode = "json"
		return projection, err
	default:
		err := &domain.CommandError{Code: "rpc_method_not_found", Message: "RPC method not found", Hint: fmt.Sprintf("Check whether pinax api routes includes %s", req.Method)}
		projection := domain.NewErrorProjection("api.rpc", err)
		projection.Mode = "json"
		return projection, err
	}
}

func (d *RPCDispatcher) ensureWriteAllowed(command string, params map[string]any) (domain.Projection, error) {
	if !d.allowWrite {
		err := &domain.CommandError{Code: "write_disabled", Message: "RPC dispatcher is currently read-only", Hint: "Start the API server in allow-write mode and retry"}
		projection := domain.NewErrorProjection(command, err)
		projection.Mode = "json"
		return projection, err
	}
	if !boolParam(params, "dry_run") && !boolParam(params, "yes") {
		err := &domain.CommandError{Code: "approval_required", Message: "Remote folder writes require yes=true", Hint: "Preview with dry_run=true first, then append yes=true to confirm"}
		projection := domain.NewErrorProjection(command, err)
		projection.Mode = "json"
		return projection, err
	}
	return domain.Projection{}, nil
}

func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if value, ok := params[key].(string); ok {
		return value
	}
	return ""
}

func boolParam(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	value, ok := params[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true"
	default:
		return false
	}
}

func stringSliceParam(params map[string]any, key string) []string {
	if params == nil {
		return nil
	}
	value, ok := params[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || text == "" {
				continue
			}
			items = append(items, text)
		}
		return items
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func intParam(params map[string]any, key string) int {
	if params == nil {
		return 0
	}
	value, ok := params[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(typed, "%d", &parsed)
		return parsed
	default:
		return 0
	}
}
