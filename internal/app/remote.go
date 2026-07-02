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
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "workbench.activity.list", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "activity.list", Readonly: true, BodyAllowed: false, UIGroup: "workbench.activity", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax activity list --vault <vault> --json", RequestSchema: "pinax.activity.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"invalid_activity_time"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "workbench.activity.show", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "activity.show", Readonly: true, BodyAllowed: false, UIGroup: "workbench.activity", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax activity show <event-id> --vault <vault> --json", RequestSchema: "pinax.activity.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"activity_event_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "monitor.list", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "monitor.runs", Readonly: true, BodyAllowed: false, UIGroup: "workbench.monitor", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax monitor runs --vault <vault> --json", RequestSchema: "pinax.monitor.list.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"invalid_activity_time"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "monitor.show", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "monitor.show", Readonly: true, BodyAllowed: false, UIGroup: "workbench.monitor", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax monitor show <run-id> --vault <vault> --json", RequestSchema: "pinax.monitor.show.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"monitor_run_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "monitor.summary", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "monitor.summary", Readonly: true, BodyAllowed: false, UIGroup: "workbench.monitor", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax monitor summary --vault <vault> --json", RequestSchema: "pinax.monitor.summary.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"invalid_activity_time"}},
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
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "memory.list", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "memory.list", Readonly: true, BodyAllowed: false, UIGroup: "agent.memory", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax memory list --vault <vault> --json", RequestSchema: "pinax.memory.list.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "memory.capture", Surfaces: []string{"cli", "rest", "rpc"}, Command: "memory.capture", Readonly: false, BodyAllowed: true, ApprovalRequired: true, UIGroup: "agent.memory", BodyExposureDefault: "explicit", WriteGate: "approval", CopyCommand: "pinax memory capture --type fact --subject <subject> --object <object> --vault <vault> --json", RequestSchema: "pinax.memory.capture.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "memory_record_invalid"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "memory.recall", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "memory.recall", Readonly: true, BodyAllowed: false, UIGroup: "agent.memory", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax memory recall <query> --vault <vault> --json", RequestSchema: "pinax.memory.recall.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "memory.context", Surfaces: []string{"cli", "rest", "rpc", "mcp", "dashboard"}, Command: "memory.context", Readonly: true, BodyAllowed: false, UIGroup: "agent.memory", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax memory context <task> --vault <vault> --limit 12 --agent", RequestSchema: "pinax.memory.context.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "memory.stats", Surfaces: []string{"cli", "rest", "rpc", "dashboard"}, Command: "memory.stats", Readonly: true, BodyAllowed: false, UIGroup: "agent.memory", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax memory stats --vault <vault> --json", RequestSchema: "pinax.memory.stats.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "brain.context.bundle", Surfaces: []string{"cli"}, Command: "brain.context", Readonly: true, BodyAllowed: false, UIGroup: "agent.brain", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax brain context <task> --vault <vault> --json", LocalOnlyReason: "future-contract", RequestSchema: "pinax.agent_brain.context_bundle.request.v1", ResponseSchema: "pinax.agent_brain.context_bundle.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "brain.answer.preview", Surfaces: []string{"cli"}, Command: "brain.answer", Readonly: true, BodyAllowed: false, UIGroup: "agent.brain", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax brain answer <question> --vault <vault> --json", LocalOnlyReason: "future-contract", RequestSchema: "pinax.agent_brain.answer.request.v1", ResponseSchema: "pinax.agent_brain.answer.v1", Errors: []string{"provider_not_configured", "insufficient_evidence"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "brain.maintenance.plan", Surfaces: []string{"cli"}, Command: "brain.maintain", Readonly: true, BodyAllowed: false, UIGroup: "agent.brain", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax brain maintain --vault <vault> --dry-run --json", LocalOnlyReason: "future-contract", RequestSchema: "pinax.agent_brain.maintenance_plan.request.v1", ResponseSchema: "pinax.agent_brain.maintenance_plan.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "brain.sources.list", Surfaces: []string{"cli"}, Command: "brain.sources", Readonly: true, BodyAllowed: false, UIGroup: "agent.brain", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax brain sources <question> --vault <vault> --json", LocalOnlyReason: "future-contract", RequestSchema: "pinax.agent_brain.sources.request.v1", ResponseSchema: "pinax.agent_brain.sources.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "brain.provider.cost_status", Surfaces: []string{"cli"}, Command: "brain.provider.status", Readonly: true, BodyAllowed: false, UIGroup: "agent.brain", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax brain provider status --vault <vault> --json", LocalOnlyReason: "future-contract", RequestSchema: "pinax.agent_brain.provider_status.request.v1", ResponseSchema: "pinax.agent_brain.provider_status.v1"},
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
		// CLI proof-loop release core capabilities. These are the agent-safe loop
		// commands that own writes and diagnostics locally. They have no REST/RPC
		// route by design — agents discover them as metadata and copy the CLI next
		// command. OpenAPI export must not fabricate HTTP paths for these.
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "vault.init", Surfaces: []string{"cli"}, Command: "vault.init", Readonly: false, BodyAllowed: false, ApprovalRequired: false, UIGroup: "vault.bootstrap", BodyExposureDefault: "none", WriteGate: "write", CopyCommand: "pinax init vault --title <title> --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.vault.init.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_already_initialized", "invalid_vault_path"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "vault.validate", Surfaces: []string{"cli"}, Command: "vault.validate", Readonly: true, BodyAllowed: false, UIGroup: "vault.bootstrap", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax vault validate --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.vault.validate.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "vault.stats", Surfaces: []string{"cli"}, Command: "vault.stats", Readonly: true, BodyAllowed: false, UIGroup: "vault.bootstrap", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax vault stats --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.vault.stats.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "vault.doctor", Surfaces: []string{"cli"}, Command: "vault.doctor", Readonly: true, BodyAllowed: false, UIGroup: "vault.health", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax vault doctor --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.vault.doctor.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "asset.doctor", Surfaces: []string{"cli"}, Command: "asset.doctor", Readonly: true, BodyAllowed: false, UIGroup: "vault.health", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax asset doctor --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.asset.doctor.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "note.add", Surfaces: []string{"cli"}, Command: "note.add", Readonly: false, BodyAllowed: true, ApprovalRequired: false, UIGroup: "editor.note", BodyExposureDefault: "explicit", WriteGate: "write", CopyCommand: "pinax note add <title> --body <body> --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.note.add.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"note_path_conflict", "invalid_note_body"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "journal.daily.append", Surfaces: []string{"cli"}, Command: "daily.append", Readonly: false, BodyAllowed: true, ApprovalRequired: false, UIGroup: "editor.note", BodyExposureDefault: "explicit", WriteGate: "write", CopyCommand: "pinax journal daily append --body <body> --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.journal.daily.append.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "import.markdown", Surfaces: []string{"cli"}, Command: "import.markdown", Readonly: false, BodyAllowed: false, ApprovalRequired: true, UIGroup: "editor.note", BodyExposureDefault: "none", WriteGate: "approval", CopyCommand: "pinax import markdown <path> --vault <vault> --dry-run --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.import.markdown.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "unsafe_import_path"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "index.refresh", Surfaces: []string{"cli"}, Command: "index.refresh", Readonly: false, BodyAllowed: false, ApprovalRequired: false, UIGroup: "search.view", BodyExposureDefault: "none", WriteGate: "write", CopyCommand: "pinax index refresh --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.index.refresh.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "note.search", Surfaces: []string{"cli", "mcp"}, Command: "note.search", Readonly: true, BodyAllowed: false, UIGroup: "search.view", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax search <query> --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.note.search.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "kb.context", Surfaces: []string{"cli", "mcp"}, Command: "kb.context", Readonly: true, BodyAllowed: false, UIGroup: "provider.status", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax kb context <task> --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.kb.context.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"kb_provider_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "proof.loop.run", Surfaces: []string{"cli"}, Command: "proof.loop.run", Readonly: true, BodyAllowed: false, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax proof loop run --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.proof.loop.run.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "index_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "repair.plan", Surfaces: []string{"cli", "mcp"}, Command: "repair.plan", Readonly: true, BodyAllowed: false, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax repair plan --vault <vault> --save --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.repair.plan.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "plan_save_failed"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "organize.plan", Surfaces: []string{"cli", "mcp"}, Command: "organize.plan", Readonly: true, BodyAllowed: false, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax organize plan --vault <vault> --save --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.organize.plan.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "plan_save_failed"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "repair.apply", Surfaces: []string{"cli"}, Command: "repair.apply", Readonly: false, BodyAllowed: false, ApprovalRequired: true, SnapshotRequired: true, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "approval_and_snapshot", CopyCommand: "pinax repair apply --vault <vault> --plan <plan-id> --yes --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.repair.apply.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "snapshot_required", "plan_not_found", "plan_stale"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "organize.apply", Surfaces: []string{"cli"}, Command: "organize.apply", Readonly: false, BodyAllowed: false, ApprovalRequired: true, SnapshotRequired: true, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "approval_and_snapshot", CopyCommand: "pinax organize apply --vault <vault> --plan <plan-id> --yes --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.organize.apply.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"write_disabled", "approval_required", "snapshot_required", "plan_not_found", "plan_stale"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "version.snapshot", Surfaces: []string{"cli"}, Command: "version.snapshot", Readonly: false, BodyAllowed: false, ApprovalRequired: false, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "write", CopyCommand: "pinax version snapshot --vault <vault> --message <message> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.version.snapshot.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"vault_not_initialized", "version_backend_unavailable"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "version.restore", Surfaces: []string{"cli"}, Command: "version.restore", Readonly: true, BodyAllowed: false, UIGroup: "proof.gate", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax version restore <path> --revision HEAD --plan --vault <vault> --json", LocalOnlyReason: "cli-proof-loop", RequestSchema: "pinax.version.restore.request.v1", ResponseSchema: "pinax.projection.v1", Errors: []string{"revision_not_found", "path_not_found"}},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "api.routes", Surfaces: []string{"cli"}, Command: "api.routes", Readonly: true, BodyAllowed: false, UIGroup: "workbench.status", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax api routes --vault <vault> --json", LocalOnlyReason: "cli-discovery", RequestSchema: "pinax.api.routes.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "api.schema.export", Surfaces: []string{"cli"}, Command: "api.schema.export", Readonly: true, BodyAllowed: false, UIGroup: "workbench.status", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax api schema export --format openapi --vault <vault> --json", LocalOnlyReason: "cli-discovery", RequestSchema: "pinax.api.schema.export.request.v1", ResponseSchema: "pinax.projection.v1"},
		{SchemaVersion: domain.RemoteCapabilitySchemaVersion, ID: "mcp.serve", Surfaces: []string{"cli"}, Command: "mcp.serve", Readonly: true, BodyAllowed: false, UIGroup: "workbench.status", BodyExposureDefault: "none", WriteGate: "readonly", CopyCommand: "pinax mcp serve --vault <vault>", LocalOnlyReason: "cli-discovery", RequestSchema: "pinax.mcp.serve.request.v1", ResponseSchema: "pinax.projection.v1"},
	}
	return decorateRemoteCapabilities(caps)
}

// releaseCoreCapabilityIDs lists the capability IDs that form the release proof
// loop: vault bootstrap, capture, retrieve, diagnose, plan, apply safely, and
// discover. They are the stable agent-facing surface; every other capability is
// secondary, preview, or advanced.
func releaseCoreCapabilityIDs() map[string]bool {
	return map[string]bool{
		// Vault bootstrap / discover.
		"vault.init":        true,
		"vault.validate":    true,
		"vault.stats":       true,
		"vault.doctor":      true,
		"api.routes":        true,
		"api.schema.export": true,
		"mcp.serve":         true,
		// Capture.
		"note.add":             true,
		"inbox.capture":        true,
		"journal.daily.append": true,
		"import.markdown":      true,
		// Retrieve.
		"index.refresh":        true,
		"note.search":          true,
		"note.read":            true,
		"note.list":            true,
		"memory.context":       true,
		"memory.list":          true,
		"memory.recall":        true,
		"kb.context":           true,
		"graph.summary":        true,
		"database.view.render": true,
		"folder.list":          true,
		"folder.show":          true,
		"inbox.list":           true,
		"inbox.show":           true,
		// Diagnose / plan.
		"proof.loop.run":    true,
		"asset.doctor":      true,
		"repair.plan":       true,
		"organize.plan":     true,
		"folder.repair":     true,
		"project.item.plan": true,
		// Apply safely.
		"version.snapshot": true,
		"version.restore":  true,
		"repair.apply":     true,
		"organize.apply":   true,
	}
}

func decorateRemoteCapabilities(caps []domain.RemoteCapability) []domain.RemoteCapability {
	releaseCore := releaseCoreCapabilityIDs()
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
		if releaseCore[cap.ID] {
			cap.ReleaseCore = true
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
	case strings.HasPrefix(id, "memory."):
		return "agent.memory"
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
		remoteRoute("rest.workbench.activity.list", "rest", "GET", "/v1/workbench/activity", "", byID["workbench.activity.list"]),
		remoteRoute("rest.workbench.activity.show", "rest", "GET", "/v1/workbench/activity/{event_id}", "", byID["workbench.activity.show"]),
		remoteRoute("rest.monitor.list", "rest", "GET", "/v1/monitor/runs", "", byID["monitor.list"]),
		remoteRoute("rest.monitor.show", "rest", "GET", "/v1/monitor/runs/{run_id}", "", byID["monitor.show"]),
		remoteRoute("rest.monitor.summary", "rest", "GET", "/v1/monitor/summary", "", byID["monitor.summary"]),
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
		remoteRoute("rest.memory.list", "rest", "GET", "/v1/memory", "", byID["memory.list"]),
		remoteRoute("rest.memory.capture", "rest", "POST", "/v1/memory:capture", "", byID["memory.capture"]),
		remoteRoute("rest.memory.recall", "rest", "GET", "/v1/memory:recall", "", byID["memory.recall"]),
		remoteRoute("rest.memory.context", "rest", "GET", "/v1/memory:context", "", byID["memory.context"]),
		remoteRoute("rest.memory.stats", "rest", "GET", "/v1/memory:stats", "", byID["memory.stats"]),
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
		remoteRoute("rpc.workbench.activity.list", "rpc", "CALL", "", "Pinax.Workbench.Activity.List", byID["workbench.activity.list"]),
		remoteRoute("rpc.workbench.activity.show", "rpc", "CALL", "", "Pinax.Workbench.Activity.Show", byID["workbench.activity.show"]),
		remoteRoute("rpc.monitor.list", "rpc", "CALL", "", "Pinax.Monitor.List", byID["monitor.list"]),
		remoteRoute("rpc.monitor.show", "rpc", "CALL", "", "Pinax.Monitor.Show", byID["monitor.show"]),
		remoteRoute("rpc.monitor.summary", "rpc", "CALL", "", "Pinax.Monitor.Summary", byID["monitor.summary"]),
		remoteRoute("rpc.project.board.show", "rpc", "CALL", "", "Pinax.ProjectBoard.Show", byID["project.board.show"]),
		remoteRoute("rpc.project.subproject.list", "rpc", "CALL", "", "Pinax.Project.Subproject.List", byID["project.subproject.list"]),
		remoteRoute("rpc.project.subproject.show", "rpc", "CALL", "", "Pinax.Project.Subproject.Show", byID["project.subproject.show"]),
		remoteRoute("rpc.project.subproject.create", "rpc", "CALL", "", "Pinax.Project.Subproject.Create", byID["project.subproject.create"]),
		remoteRoute("rpc.note.read", "rpc", "CALL", "", "Pinax.Note.Read", byID["note.read"]),
		remoteRoute("rpc.note.list", "rpc", "CALL", "", "Pinax.Note.List", byID["note.list"]),
		remoteRoute("rpc.database.view.render", "rpc", "CALL", "", "Pinax.DatabaseView.Render", byID["database.view.render"]),
		remoteRoute("rpc.task.adopt.plan", "rpc", "CALL", "", "Pinax.Task.AdoptPlan", byID["task.adopt.plan"]),
		remoteRoute("rpc.graph.summary", "rpc", "CALL", "", "Pinax.Graph.Summary", byID["graph.summary"]),
		remoteRoute("rpc.memory.list", "rpc", "CALL", "", "Pinax.Memory.List", byID["memory.list"]),
		remoteRoute("rpc.memory.capture", "rpc", "CALL", "", "Pinax.Memory.Capture", byID["memory.capture"]),
		remoteRoute("rpc.memory.recall", "rpc", "CALL", "", "Pinax.Memory.Recall", byID["memory.recall"]),
		remoteRoute("rpc.memory.context", "rpc", "CALL", "", "Pinax.Memory.Context", byID["memory.context"]),
		remoteRoute("rpc.memory.stats", "rpc", "CALL", "", "Pinax.Memory.Stats", byID["memory.stats"]),
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
	return domain.RemoteRoute{RouteID: routeID, Surface: surface, Method: method, Path: path, RPCMethod: rpcMethod, Command: cap.Command, CapabilityID: cap.ID, SchemaVersion: cap.SchemaVersion, ReleaseCore: cap.ReleaseCore, Readonly: cap.Readonly, BodyAllowed: cap.BodyAllowed, ApprovalRequired: cap.ApprovalRequired, SnapshotRequired: cap.SnapshotRequired, UIGroup: cap.UIGroup, BodyExposureDefault: cap.BodyExposureDefault, WriteGate: cap.WriteGate, CopyCommand: cap.CopyCommand, LocalOnlyReason: cap.LocalOnlyReason, Errors: cap.Errors}
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
	capabilities := RemoteCapabilities()
	releaseCoreCount := 0
	for _, cap := range capabilities {
		if cap.ReleaseCore {
			releaseCoreCount++
		}
	}
	projection := domain.NewProjection("api.routes", "API capabilities listed.")
	projection.Facts["routes"] = fmt.Sprint(len(routes))
	projection.Facts["capabilities"] = fmt.Sprint(len(capabilities))
	projection.Facts["release_core"] = fmt.Sprint(releaseCoreCount)
	projection.Facts["schema_version"] = domain.RemoteCapabilitySchemaVersion
	for _, route := range routes {
		endpoint := route.Path
		if endpoint == "" {
			endpoint = route.RPCMethod
		}
		projection.Evidence = append(projection.Evidence, fmt.Sprintf("%s %s -> %s", route.Method, endpoint, route.Command))
	}
	projection.Actions = []domain.Action{{Name: "schema", Command: fmt.Sprintf("pinax api schema export --format openapi --vault %s --json", shellQuote(req.VaultPath))}}
	projection.Data = map[string]any{"routes": routes, "capabilities": capabilities}
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
				"x-pinax-release-core":      route.ReleaseCore,
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
