package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type APIRequest struct {
	VaultPath string
	Format    string
	WriteMode string
}

func RemoteCapabilities() []domain.RemoteCapability {
	caps := []domain.RemoteCapability{
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "workbench.status", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "workbench.status", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.workbench.status.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "config.path", Surfaces: []string{"cli", "dashboard"}, Command: "config.path", Readonly: true, BodyAllowed: false, UIGroup: "settings.control", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax config path --vault <vault> --json", RequestSchema: "pinax.config.path.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "config.get", Surfaces: []string{"cli", "dashboard"}, Command: "config.get", Readonly: true, BodyAllowed: false, UIGroup: "settings.control", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax config get <key> --vault <vault> --json", RequestSchema: "pinax.config.get.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"config_key_unknown"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "config.doctor", Surfaces: []string{"cli", "dashboard"}, Command: "config.doctor", Readonly: true, BodyAllowed: false, UIGroup: "settings.control", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax config doctor --vault <vault> --json", RequestSchema: "pinax.config.doctor.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"config_error"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "config.set", Surfaces: []string{"cli", "dashboard"}, Command: "config.set", Readonly: false, BodyAllowed: false, UIGroup: "settings.control", BodyExposureDefault: "none", WriteGate: "explicit_scope", CopyCommand: "pinax config set <key> <value> --scope user --vault <vault> --json", RequestSchema: "pinax.config.set.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"config_scope_required", "config_secret_rejected", "config_invalid"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "config.unset", Surfaces: []string{"cli", "dashboard"}, Command: "config.unset", Readonly: false, BodyAllowed: false, UIGroup: "settings.control", BodyExposureDefault: "none", WriteGate: "explicit_scope", CopyCommand: "pinax config unset <key> --scope user --vault <vault> --json", RequestSchema: "pinax.config.unset.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"config_scope_required", "config_key_unknown"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.list", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "project.list", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.project.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"project_registry_invalid"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.show", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "project.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.project.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"project_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.board.show", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "project.board.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.project_board.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"project_not_found", "invalid_note_display", "index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.subproject.list", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "project.subproject.list", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.project_subproject.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"project_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.subproject.show", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "project.subproject.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.project_subproject.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"project_not_found", "subproject_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.subproject.create", Surfaces: []string{"cli", "rest", "rpc"}, Command: "project.subproject.create", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.project_subproject.create.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "project_not_found", "invalid_subproject_slug", "reserved_subproject_slug"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "note.read", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "note.show", Readonly: true, BodyAllowed: true, RequestSchema: "pinax.note.read.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"note_not_found", "invalid_note_display"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "note.list", Surfaces: []string{"cli", "rpc", "mcp"}, Command: "note.list", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.note.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"property_not_found", "invalid_date"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.item.show", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "project.item.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.project_item.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"project_item_not_found", "project_item_unmanaged"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "project.item.plan", Surfaces: []string{"cli", "rest", "rpc"}, Command: "project.item.plan", Readonly: true, BodyAllowed: false, ApprovalRequired: true, SnapshotRequired: true, RequestSchema: "pinax.project_item.plan.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"approval_required", "snapshot_required"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "task.adopt.plan", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "task.adopt", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.task.adopt_plan.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"argument_required", "task_not_found", "task_adopt_unsupported"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "database.view.render", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "database.view.render", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.database_view.render.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"view_not_found", "database_view_result_unavailable", "calendar_field_required"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "graph.summary", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "graph.summary", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.graph.summary.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "canvas.layout.metadata", Surfaces: []string{"dashboard"}, Command: "canvas.layout.metadata", Readonly: true, BodyAllowed: false, UIGroup: "canvas.view", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax api routes --vault <vault> --json", LocalOnlyReason: "future-client-only", RequestSchema: "pinax.canvas.layout_metadata.request.v1", ResponseSchema: "pinax.canvas.layout_metadata.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.list", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.list", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.folder.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"invalid_folder_purpose"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.show", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.folder.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"folder_not_found", "unsafe_folder_path"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.create", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.create", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.folder.create.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "unsafe_folder_path", "invalid_folder_purpose", "folder_path_conflict"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.rename", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.rename", Readonly: false, BodyAllowed: false, ApprovalRequired: true, SnapshotRequired: true, RequestSchema: "pinax.folder.rename.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "unsafe_folder_path", "folder_not_found", "folder_path_conflict", "invalid_folder_target"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.move", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.move", Readonly: false, BodyAllowed: false, ApprovalRequired: true, SnapshotRequired: true, RequestSchema: "pinax.folder.move.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "unsafe_folder_path", "folder_not_found", "folder_path_conflict", "invalid_folder_target"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.delete", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.delete", Readonly: false, BodyAllowed: false, ApprovalRequired: true, SnapshotRequired: true, RequestSchema: "pinax.folder.delete.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "unsafe_folder_path", "folder_not_found", "folder_not_empty", "empty_only_required"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.adopt", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.adopt", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.folder.adopt.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "unsafe_folder_path", "folder_not_found", "invalid_folder_purpose"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "folder.repair", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "folder.repair", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.folder.repair.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"plan_required"}},
		// Inbox capabilities
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "inbox.list", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "inbox.list", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.inbox.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "inbox.show", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "inbox.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.inbox.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"note_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "inbox.capture", Surfaces: []string{"cli", "rest", "rpc"}, Command: "inbox.capture", Readonly: false, BodyAllowed: true, ApprovalRequired: true, RequestSchema: "pinax.inbox.capture.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "inbox.promote", Surfaces: []string{"cli", "rest", "rpc"}, Command: "inbox.promote", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.inbox.promote.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "note_not_found", "invalid_lifecycle_transition", "note_path_conflict"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "inbox.discard", Surfaces: []string{"cli", "rest", "rpc"}, Command: "inbox.discard", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.inbox.discard.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "note_not_found", "invalid_lifecycle_transition"}},
		// Draft capabilities
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "draft.list", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "draft.list", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.draft.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "draft.show", Surfaces: []string{"cli", "rest", "rpc", "mcp"}, Command: "draft.show", Readonly: true, BodyAllowed: false, RequestSchema: "pinax.draft.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"note_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "draft.create", Surfaces: []string{"cli", "rest", "rpc"}, Command: "draft.create", Readonly: false, BodyAllowed: true, ApprovalRequired: true, RequestSchema: "pinax.draft.create.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "draft.promote", Surfaces: []string{"cli", "rest", "rpc"}, Command: "draft.promote", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.draft.promote.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "note_not_found", "invalid_lifecycle_transition"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "draft.archive", Surfaces: []string{"cli", "rest", "rpc"}, Command: "draft.archive", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.draft.archive.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "note_not_found", "invalid_lifecycle_transition"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "draft.discard", Surfaces: []string{"cli", "rest", "rpc"}, Command: "draft.discard", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.draft.discard.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "note_not_found", "invalid_lifecycle_transition"}},
		// Sync capabilities
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "sync.push", Surfaces: []string{"cli", "rpc"}, Command: "sync.push", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.sync.push.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "cloud_not_configured", "revision_conflict"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "sync.pull", Surfaces: []string{"cli", "rpc"}, Command: "sync.pull", Readonly: false, BodyAllowed: false, ApprovalRequired: true, RequestSchema: "pinax.sync.pull.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "cloud_not_configured", "revision_conflict"}},
	}
	return decorateRemoteCapabilities(caps)
}

func decorateRemoteCapabilities(caps []domain.RemoteCapability) []domain.RemoteCapability {
	for i := range caps {
		cap := &caps[i]
		if cap.UIGroup == "" {
			cap.UIGroup = webUIGroupForCommand(cap.Command, cap.ID)
		}
		if cap.BodyExposureDefault == "" {
			cap.BodyExposureDefault = bodyExposureDefault(*cap)
		}
		if cap.WriteGate == "" {
			cap.WriteGate = writeGateForCapability(*cap)
		}
		if cap.CopyCommand == "" {
			cap.CopyCommand = copyCommandForCapability(*cap)
		}
	}
	return caps
}

func webUIGroupForCommand(command, id string) string {
	switch {
	case strings.HasPrefix(id, "database."):
		return "search.view"
	case strings.HasPrefix(id, "graph."):
		return "graph.view"
	case strings.HasPrefix(id, "project.item.plan"):
		return "proof.gate"
	case strings.HasPrefix(id, "project.board") || strings.HasPrefix(id, "project.item") || strings.HasPrefix(id, "task."):
		return "board.view"
	case strings.HasPrefix(id, "project."):
		return "workbench.status"
	case strings.HasPrefix(id, "sync."):
		return "settings.control"
	case strings.HasPrefix(id, "folder.") || strings.HasPrefix(id, "inbox.") || strings.HasPrefix(id, "draft.") || strings.HasPrefix(id, "note."):
		return "editor.note"
	case strings.Contains(command, "provider") || strings.HasPrefix(id, "kb."):
		return "provider.status"
	default:
		return "workbench.status"
	}
}

func bodyExposureDefault(cap domain.RemoteCapability) string {
	if cap.BodyAllowed {
		return "explicit"
	}
	return "none"
}

func writeGateForCapability(cap domain.RemoteCapability) string {
	if cap.ApprovalRequired && cap.SnapshotRequired {
		return "approval_and_snapshot"
	}
	if cap.ApprovalRequired {
		return "approval"
	}
	if cap.Readonly {
		return "readonly"
	}
	return "write"
}

func copyCommandForCapability(cap domain.RemoteCapability) string {
	parts := strings.Split(cap.Command, ".")
	return "pinax " + strings.Join(parts, " ") + " --vault <vault> --json"
}

func RemoteRoutes() []domain.RemoteRoute {
	capabilities := RemoteCapabilities()
	byID := map[string]domain.RemoteCapability{}
	for _, cap := range capabilities {
		byID[cap.ID] = cap
	}
	return []domain.RemoteRoute{
		remoteRoute("rest.workbench.status", "rest", "GET", "/v1/workbench/status", "", byID["workbench.status"]),
		remoteRoute("rest.project.list", "rest", "GET", "/v1/projects", "", byID["project.list"]),
		remoteRoute("rest.project.show", "rest", "GET", "/v1/projects/{project}", "", byID["project.show"]),
		remoteRoute("rest.project.board.show", "rest", "GET", "/v1/projects/{slug}/board", "", byID["project.board.show"]),
		remoteRoute("rest.project.subproject.list", "rest", "GET", "/v1/projects/{project}/subprojects", "", byID["project.subproject.list"]),
		remoteRoute("rest.project.subproject.show", "rest", "GET", "/v1/projects/{project}/subprojects/{subproject}", "", byID["project.subproject.show"]),
		remoteRoute("rest.project.subproject.create", "rest", "POST", "/v1/projects/{project}/subprojects", "", byID["project.subproject.create"]),
		remoteRoute("rest.note.read", "rest", "GET", "/v1/notes/{ref}", "", byID["note.read"]),
		remoteRoute("rest.project.item.show", "rest", "GET", "/v1/project-items/{ref}", "", byID["project.item.show"]),
		remoteRoute("rest.project.item.plan", "rest", "POST", "/v1/project-items/{ref}:{action}", "", byID["project.item.plan"]),
		remoteRoute("rest.task.adopt.plan", "rest", "POST", "/v1/tasks/{item}:adopt-plan", "", byID["task.adopt.plan"]),
		remoteRoute("rest.database.view.render", "rest", "GET", "/v1/database/views/{name}:render", "", byID["database.view.render"]),
		remoteRoute("rest.graph.summary", "rest", "GET", "/v1/graph/summary", "", byID["graph.summary"]),
		remoteRoute("rest.folder.list", "rest", "GET", "/v1/folders", "", byID["folder.list"]),
		remoteRoute("rest.folder.show", "rest", "GET", "/v1/folders/{path}", "", byID["folder.show"]),
		remoteRoute("rest.folder.create", "rest", "POST", "/v1/folders", "", byID["folder.create"]),
		remoteRoute("rest.folder.rename", "rest", "POST", "/v1/folders/{path}:rename", "", byID["folder.rename"]),
		remoteRoute("rest.folder.move", "rest", "POST", "/v1/folders/{path}:move", "", byID["folder.move"]),
		remoteRoute("rest.folder.delete", "rest", "POST", "/v1/folders/{path}:delete", "", byID["folder.delete"]),
		remoteRoute("rest.folder.adopt", "rest", "POST", "/v1/folders/{path}:adopt", "", byID["folder.adopt"]),
		remoteRoute("rest.folder.repair", "rest", "POST", "/v1/folders:repair-plan", "", byID["folder.repair"]),
		// Inbox REST routes
		remoteRoute("rest.inbox.list", "rest", "GET", "/v1/inbox", "", byID["inbox.list"]),
		remoteRoute("rest.inbox.show", "rest", "GET", "/v1/inbox/{ref}", "", byID["inbox.show"]),
		remoteRoute("rest.inbox.capture", "rest", "POST", "/v1/inbox:capture", "", byID["inbox.capture"]),
		remoteRoute("rest.inbox.promote", "rest", "POST", "/v1/inbox/{ref}:promote", "", byID["inbox.promote"]),
		remoteRoute("rest.inbox.discard", "rest", "POST", "/v1/inbox/{ref}:discard", "", byID["inbox.discard"]),
		// Draft REST routes
		remoteRoute("rest.draft.list", "rest", "GET", "/v1/drafts", "", byID["draft.list"]),
		remoteRoute("rest.draft.show", "rest", "GET", "/v1/drafts/{ref}", "", byID["draft.show"]),
		remoteRoute("rest.draft.create", "rest", "POST", "/v1/drafts", "", byID["draft.create"]),
		remoteRoute("rest.draft.promote", "rest", "POST", "/v1/drafts/{ref}:promote", "", byID["draft.promote"]),
		remoteRoute("rest.draft.archive", "rest", "POST", "/v1/drafts/{ref}:archive", "", byID["draft.archive"]),
		remoteRoute("rest.draft.discard", "rest", "POST", "/v1/drafts/{ref}:discard", "", byID["draft.discard"]),
		// RPC routes
		remoteRoute("rpc.workbench.status", "rpc", "CALL", "", "Pinax.Workbench.Status", byID["workbench.status"]),
		remoteRoute("rpc.project.board.show", "rpc", "CALL", "", "Pinax.ProjectBoard.Show", byID["project.board.show"]),
		remoteRoute("rpc.project.subproject.list", "rpc", "CALL", "", "Pinax.Project.Subproject.List", byID["project.subproject.list"]),
		remoteRoute("rpc.project.subproject.show", "rpc", "CALL", "", "Pinax.Project.Subproject.Show", byID["project.subproject.show"]),
		remoteRoute("rpc.project.subproject.create", "rpc", "CALL", "", "Pinax.Project.Subproject.Create", byID["project.subproject.create"]),
		remoteRoute("rpc.note.read", "rpc", "CALL", "", "Pinax.Note.Read", byID["note.read"]),
		remoteRoute("rpc.note.list", "rpc", "CALL", "", "Pinax.Note.List", byID["note.list"]),
		remoteRoute("rpc.database.view.render", "rpc", "CALL", "", "Pinax.DatabaseView.Render", byID["database.view.render"]),
		remoteRoute("rpc.task.adopt.plan", "rpc", "CALL", "", "Pinax.Task.AdoptPlan", byID["task.adopt.plan"]),
		remoteRoute("rpc.graph.summary", "rpc", "CALL", "", "Pinax.Graph.Summary", byID["graph.summary"]),
		remoteRoute("rpc.project.item.plan", "rpc", "CALL", "", "Pinax.ProjectItem.Plan", byID["project.item.plan"]),
		remoteRoute("rpc.folder.list", "rpc", "CALL", "", "Pinax.Folder.List", byID["folder.list"]),
		remoteRoute("rpc.folder.show", "rpc", "CALL", "", "Pinax.Folder.Show", byID["folder.show"]),
		remoteRoute("rpc.folder.create", "rpc", "CALL", "", "Pinax.Folder.Create", byID["folder.create"]),
		remoteRoute("rpc.folder.rename", "rpc", "CALL", "", "Pinax.Folder.Rename", byID["folder.rename"]),
		remoteRoute("rpc.folder.move", "rpc", "CALL", "", "Pinax.Folder.Move", byID["folder.move"]),
		remoteRoute("rpc.folder.delete", "rpc", "CALL", "", "Pinax.Folder.Delete", byID["folder.delete"]),
		remoteRoute("rpc.folder.adopt", "rpc", "CALL", "", "Pinax.Folder.Adopt", byID["folder.adopt"]),
		remoteRoute("rpc.folder.repair", "rpc", "CALL", "", "Pinax.Folder.RepairPlan", byID["folder.repair"]),
		// Inbox RPC routes
		remoteRoute("rpc.inbox.list", "rpc", "CALL", "", "Pinax.Inbox.List", byID["inbox.list"]),
		remoteRoute("rpc.inbox.show", "rpc", "CALL", "", "Pinax.Inbox.Show", byID["inbox.show"]),
		remoteRoute("rpc.inbox.capture", "rpc", "CALL", "", "Pinax.Inbox.Capture", byID["inbox.capture"]),
		remoteRoute("rpc.inbox.promote", "rpc", "CALL", "", "Pinax.Inbox.Promote", byID["inbox.promote"]),
		remoteRoute("rpc.inbox.discard", "rpc", "CALL", "", "Pinax.Inbox.Discard", byID["inbox.discard"]),
		// Draft RPC routes
		remoteRoute("rpc.draft.list", "rpc", "CALL", "", "Pinax.Draft.List", byID["draft.list"]),
		remoteRoute("rpc.draft.show", "rpc", "CALL", "", "Pinax.Draft.Show", byID["draft.show"]),
		remoteRoute("rpc.draft.create", "rpc", "CALL", "", "Pinax.Draft.Create", byID["draft.create"]),
		remoteRoute("rpc.draft.promote", "rpc", "CALL", "", "Pinax.Draft.Promote", byID["draft.promote"]),
		remoteRoute("rpc.draft.archive", "rpc", "CALL", "", "Pinax.Draft.Archive", byID["draft.archive"]),
		remoteRoute("rpc.draft.discard", "rpc", "CALL", "", "Pinax.Draft.Discard", byID["draft.discard"]),
		remoteRoute("rpc.sync.push", "rpc", "CALL", "", "Pinax.Sync.Push", byID["sync.push"]),
		remoteRoute("rpc.sync.pull", "rpc", "CALL", "", "Pinax.Sync.Pull", byID["sync.pull"]),
	}
}

func FindRemoteRPCMethod(method string) (domain.RemoteRoute, bool) {
	for _, route := range RemoteRoutes() {
		if route.Surface == "rpc" && route.RPCMethod == method {
			return route, true
		}
	}
	return domain.RemoteRoute{}, false
}

func remoteRoute(routeID, surface, method, path, rpcMethod string, cap domain.RemoteCapability) domain.RemoteRoute {
	return domain.RemoteRoute{RouteID: routeID, Surface: surface, Method: method, Path: path, RPCMethod: rpcMethod, Command: cap.Command, CapabilityID: cap.ID, SchemaVersion: cap.SchemaVersion, Readonly: cap.Readonly, BodyAllowed: cap.BodyAllowed, ApprovalRequired: cap.ApprovalRequired, SnapshotRequired: cap.SnapshotRequired, UIGroup: cap.UIGroup, BodyExposureDefault: cap.BodyExposureDefault, WriteGate: cap.WriteGate, CopyCommand: cap.CopyCommand, LocalOnlyReason: cap.LocalOnlyReason, Errors: cap.Errors}
}

func (s *Service) WorkbenchStatus(ctx context.Context, req APIRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("workbench.status", err), err
	}
	indexProjection, err := s.IndexStatus(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		return errorProjection("workbench.status", err), err
	}
	writeMode := strings.TrimSpace(req.WriteMode)
	if writeMode == "" {
		writeMode = "local_cli"
	}
	projection := domain.NewProjection("workbench.status", "Workbench status read.")
	projection.Facts["ui_group"] = "workbench.status"
	projection.Facts["vault_root"] = root
	projection.Facts["index_status"] = indexProjection.Facts["index_status"]
	projection.Facts["write_mode"] = writeMode
	projection.Facts["body_exposure_default"] = "none"
	projection.Facts["profile_status"] = "not_inspected"
	projection.Facts["token_status"] = "not_inspected"
	projection.Data = map[string]any{"workbench": map[string]any{"vault_root": root, "index_status": indexProjection.Facts["index_status"], "write_mode": writeMode, "body_exposure_default": "none", "profile_status": "not_inspected", "token_status": "not_inspected"}}
	if indexProjection.Status != "success" || indexProjection.Facts["index_status"] != "fresh" {
		projection.Status = "partial"
		projection.Actions = []domain.Action{{Name: "index_refresh", Command: fmt.Sprintf("pinax index refresh --vault %s --json", shellQuote(root))}}
	}
	projection.Evidence = indexProjection.Evidence
	return projection, nil
}

func (s *Service) APIRoutes(_ context.Context, req APIRequest) (domain.Projection, error) {
	routes := RemoteRoutes()
	projection := domain.NewProjection("api.routes", "API capabilities listed.")
	projection.Facts["routes"] = fmt.Sprint(len(routes))
	projection.Facts["schema_version"] = domain.RemoteCapabilitySchemaVersion
	for _, route := range routes {
		endpoint := route.Path
		if endpoint == "" {
			endpoint = route.RPCMethod
		}
		projection.Evidence = append(projection.Evidence, fmt.Sprintf("%s %s -> %s", route.Method, endpoint, route.Command))
	}
	projection.Actions = []domain.Action{{Name: "schema", Command: fmt.Sprintf("pinax api schema export --format openapi --vault %s --json", shellQuote(req.VaultPath))}}
	projection.Data = map[string]any{"routes": routes, "capabilities": RemoteCapabilities()}
	return projection, nil
}

func (s *Service) APISchemaExport(_ context.Context, req APIRequest) (domain.Projection, error) {
	format := req.Format
	if format == "" {
		format = "openapi"
	}
	if format != "openapi" {
		err := &domain.CommandError{Code: "unsupported_api_schema_format", Message: "api schema export currently only supports openapi", Hint: "Use --format openapi"}
		return domain.NewErrorProjection("api.schema.export", err), err
	}
	routes := RemoteRoutes()
	schema := map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Pinax Local API", "version": "v1"}, "paths": map[string]any{}}
	paths := schema["paths"].(map[string]any)
	for _, route := range routes {
		if route.Surface == "rest" && route.Path != "" {
			pathItem, ok := paths[route.Path].(map[string]any)
			if !ok {
				pathItem = map[string]any{}
				paths[route.Path] = pathItem
			}
			pathItem[strings.ToLower(route.Method)] = map[string]any{
				"operationId":               route.RouteID,
				"x-pinax-command":           route.Command,
				"x-pinax-capability":        route.CapabilityID,
				"x-pinax-readonly":          route.Readonly,
				"x-pinax-body-allowed":      route.BodyAllowed,
				"x-pinax-approval-required": route.ApprovalRequired,
				"x-pinax-snapshot-required": route.SnapshotRequired,
				"x-pinax-ui-group":          route.UIGroup,
				"x-pinax-body-exposure":     route.BodyExposureDefault,
				"x-pinax-write-gate":        route.WriteGate,
			}
		}
	}
	projection := domain.NewProjection("api.schema.export", "API schema exported.")
	projection.Facts["format"] = format
	projection.Facts["routes"] = fmt.Sprint(len(routes))
	projection.Data = map[string]any{"schema": schema, "routes": routes}
	return projection, nil
}

func (s *Service) GraphSummaryProjection(ctx context.Context, vaultPath string) (domain.Projection, error) {
	summary, err := s.GraphSummary(ctx, vaultPath)
	if err != nil {
		return errorProjection("graph.summary", err), err
	}
	projection := domain.NewProjection("graph.summary", "Vault link graph summary generated.")
	projection.Facts["engine"] = summary.Engine
	projection.Facts["index_status"] = summary.IndexStatus
	projection.Facts["total_notes"] = fmt.Sprint(summary.TotalNotes)
	projection.Facts["total_links"] = fmt.Sprint(summary.TotalLinks)
	projection.Facts["resolved"] = fmt.Sprint(summary.Resolved)
	projection.Facts["broken"] = fmt.Sprint(summary.Broken)
	projection.Facts["ambiguous"] = fmt.Sprint(summary.Ambiguous)
	projection.Facts["orphans"] = fmt.Sprint(summary.Orphans)
	projection.Data = summary
	projection.Actions = summary.NextActions
	if summary.Broken > 0 || summary.Ambiguous > 0 || summary.Orphans > 0 {
		projection.Status = "partial"
	}
	return projection, nil
}
