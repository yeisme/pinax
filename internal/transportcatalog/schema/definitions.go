package schema

const (
	actionV1               = "pinax.action.v1"
	projectionWarningV1    = "pinax.projection_warning.v1"
	capabilityDefinitionV1 = "pinax.capability_definition.v1"
	transportBindingV1     = "pinax.transport_binding.v1"
	manifestCapabilityV1   = "pinax.manifest_capability.v1"
)

var defaultRegistry = mustRegistry(defaultDefinitions())

func mustRegistry(definitions []Definition) *Registry {
	registry, err := New(definitions...)
	if err != nil {
		panic(err)
	}
	return registry
}

func defaultDefinitions() []Definition {
	definitions := coreDefinitions()
	definitions = append(definitions, mcpRequestDefinitions()...)
	return definitions
}

func coreDefinitions() []Definition {
	return []Definition{
		define(actionV1, closedObject(map[string]any{
			"name":    stringValue(),
			"command": stringValue(),
		}, "name", "command")),
		define(ErrorV1, closedObject(map[string]any{
			"code":    stringValue(),
			"message": stringValue(),
			"hint":    stringValue(),
		}, "code", "message")),
		define(projectionWarningV1, closedObject(map[string]any{
			"code":    stringValue(),
			"message": stringValue(),
			"hint":    stringValue(),
		}, "code", "message")),
		define(ProjectionV1, closedObject(map[string]any{
			"spec_version": stringValue(),
			"mode":         stringValue(),
			"command":      stringValue(),
			"status":       stringValue(),
			"summary":      stringValue(),
			"facts":        stringMapValue(),
			"actions":      arrayValue(refValue(actionV1)),
			"evidence":     arrayValue(stringValue()),
			"data":         map[string]any{"description": "Capability-specific bounded payload."},
			"warnings":     arrayValue(refValue(projectionWarningV1)),
			"error":        refValue(ErrorV1),
		}, "spec_version", "mode", "command", "status")),
		define(ReadinessV1, closedObject(map[string]any{
			"status":        enumValue("ready", "degraded", "blocked", "not_configured", "not_applicable"),
			"maturity":      enumValue("exploratory", "first-support", "mature"),
			"blockers":      arrayValue(stringValue()),
			"next_actions":  arrayValue(stringValue()),
			"evidence_refs": arrayValue(stringValue()),
		}, "status", "maturity")),
		define(ConnectionReadinessV1, closedObject(map[string]any{
			"schema_version": map[string]any{"type": "string", "const": ConnectionReadinessV1},
			"overall":        refValue(ReadinessV1),
			"layers": closedObject(map[string]any{
				"contract":          refValue(ReadinessV1),
				"transport":         refValue(ReadinessV1),
				"auth":              refValue(ReadinessV1),
				"owner":             refValue(ReadinessV1),
				"mutation_recovery": refValue(ReadinessV1),
				"production":        refValue(ReadinessV1),
			}, "contract", "transport", "auth", "owner", "mutation_recovery", "production"),
		}, "schema_version", "overall", "layers")),
		define(capabilityDefinitionV1, closedObject(capabilityProperties(),
			"id", "command", "release_core", "readonly", "body_allowed", "approval_required", "snapshot_required", "request_schema", "response_schema", "stability")),
		define(transportBindingV1, closedObject(map[string]any{
			"id":              stringValue(),
			"capability_id":   stringValue(),
			"transport":       enumValue("cli", "rest", "rpc", "mcp_tool", "mcp_resource", "dashboard"),
			"availability":    enumValue("available", "planned", "blocked", "future_owner"),
			"backing_ref":     stringValue(),
			"method":          stringValue(),
			"path":            stringValue(),
			"protocol_name":   stringValue(),
			"readonly":        boolValue(),
			"write_gate":      stringValue(),
			"request_schema":  stringValue(),
			"response_schema": stringValue(),
			"blockers":        arrayValue(stringValue()),
			"readiness":       refValue(ReadinessV1),
		}, "id", "capability_id", "transport", "availability", "readonly", "request_schema", "response_schema")),
		define(manifestCapabilityV1, closedObject(manifestCapabilityProperties(),
			"id", "command", "release_core", "readonly", "body_allowed", "approval_required", "snapshot_required", "request_schema", "response_schema", "stability")),
		define(TransportManifestV1, closedObject(map[string]any{
			"schema_version": map[string]any{"type": "string", "const": TransportManifestV1},
			"digest":         map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			"capabilities":   arrayValue(refValue(manifestCapabilityV1)),
		}, "schema_version", "digest", "capabilities")),
		define(OperationV1, closedObject(map[string]any{
			"schema_version":     map[string]any{"type": "string", "const": OperationV1},
			"operation_id":       stringValue(),
			"capability_id":      stringValue(),
			"binding_id":         stringValue(),
			"idempotency_key":    stringValue(),
			"request_digest":     stringValue(),
			"status":             enumValue("accepted", "applying", "succeeded", "failed", "reconcile_required"),
			"retryable":          boolValue(),
			"replay_safe":        boolValue(),
			"reconcile_required": boolValue(),
			"required_action":    stringValue(),
			"result":             map[string]any{"description": "Bounded operation result."},
			"receipt":            map[string]any{"description": "Bounded recovery receipt."},
			"receipt_ref":        stringValue(),
			"resource_ref":       stringValue(),
			"revision_before":    stringValue(),
			"revision_after":     stringValue(),
			"error":              refValue(ErrorV1),
			"created_at":         dateTimeValue(),
			"updated_at":         dateTimeValue(),
			"accepted_at":        dateTimeValue(),
			"applying_at":        dateTimeValue(),
			"completed_at":       dateTimeValue(),
		}, "schema_version", "operation_id", "capability_id", "binding_id", "status", "retryable", "replay_safe", "reconcile_required", "created_at", "updated_at", "accepted_at")),
		define(MCPToolResultV1, closedObject(map[string]any{
			"schema_version": map[string]any{"type": "string", "const": MCPToolResultV1},
			"status":         stringValue(),
			"summary":        stringValue(),
			"command":        stringValue(),
			"data":           openObject(),
		}, "schema_version", "status", "summary", "data")),
		define(TransportManifestReq, closedObject(nil)),
		define(ConnectionReadinessReq, closedObject(nil)),
	}
}

func mcpRequestDefinitions() []Definition {
	brainProperties := map[string]any{
		"question":      stringValue(),
		"task":          stringValue(),
		"body_exposure": enumValue("bounded_projection", "full", "none"),
	}
	noteProperties := map[string]any{
		"note_ref": stringValue(),
		"note_id":  stringValue(),
		"path":     stringValue(),
		"display":  enumValue("card", "metadata", "body"),
	}
	return []Definition{
		request("pinax.note.search.request.v1", map[string]any{"query": stringValue()}, "query"),
		request("pinax.agent_brain.context_bundle.request.v1", brainProperties),
		request("pinax.agent_brain.answer.request.v1", brainProperties),
		request("pinax.agent_brain.sources.request.v1", brainProperties),
		request("pinax.agent_brain.maintenance_plan.request.v1", brainProperties),
		request("pinax.query.run.request.v1", map[string]any{"sql": stringValue()}, "sql"),
		request("pinax.database_view.show.request.v1", map[string]any{"name": stringValue()}, "name"),
		request("pinax.database_view.render.request.v1", map[string]any{"name": stringValue()}, "name"),
		request("pinax.note.read.request.v1", noteProperties),
		request("pinax.note.links.request.v1", noteProperties),
		request("pinax.note.backlinks.request.v1", noteProperties),
		request("pinax.note.context.request.v1", noteProperties),
		request("pinax.graph.summary.request.v1", nil),
		request("pinax.project_board.show.request.v1", map[string]any{"project": stringValue(), "slug": stringValue()}),
		request("pinax.task.adopt_plan.request.v1", map[string]any{"item_id": stringValue(), "item": stringValue()}),
		request("pinax.organize.plan.request.v1", nil),
		request("pinax.version.snapshot_plan.request.v1", nil),
		request("pinax.agent.context.request.v1", map[string]any{
			"workspace": map[string]any{"type": "string", "default": "default"},
			"entities":  stringArrayValue(),
		}),
		request("pinax.agent.memory.recall.request.v1", map[string]any{
			"workspace": map[string]any{"type": "string", "default": "default"},
			"kinds":     stringArrayValue(),
		}),
		request("pinax.agent.handoff.read.request.v1", map[string]any{
			"workspace": map[string]any{"type": "string", "default": "default"},
		}),
		// sync job status 只读投影（pinax-local-async-substrate-v1 §2.4）：
		// MCP 消费者按 run_id 重放事件流结论，不触碰执行器状态。
		request("pinax.sync.logs.status.request.v1", map[string]any{
			"run_id": stringValue(),
		}, "run_id"),
	}
}

func define(id string, document map[string]any) Definition {
	return Definition{ID: id, Schema: document}
}

func request(id string, properties map[string]any, required ...string) Definition {
	return define(id, closedObject(properties, required...))
}

func closedObject(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	document := map[string]any{
		"type":                 "object",
		"properties":           cloneValue(properties),
		"additionalProperties": false,
	}
	if len(required) > 0 {
		document["required"] = append([]string(nil), required...)
	}
	return document
}

func openObject() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true}
}

func stringMapValue() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": stringValue()}
}

func stringValue() map[string]any {
	return map[string]any{"type": "string"}
}

func dateTimeValue() map[string]any {
	return map[string]any{"type": "string", "format": "date-time"}
}

func boolValue() map[string]any {
	return map[string]any{"type": "boolean"}
}

func enumValue(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": append([]string(nil), values...)}
}

func arrayValue(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func stringArrayValue() map[string]any {
	return arrayValue(stringValue())
}

func refValue(id string) map[string]any {
	return map[string]any{"$ref": Ref(id)}
}

func capabilityProperties() map[string]any {
	return map[string]any{
		"id":                    stringValue(),
		"command":               stringValue(),
		"release_core":          boolValue(),
		"readonly":              boolValue(),
		"body_allowed":          boolValue(),
		"approval_required":     boolValue(),
		"snapshot_required":     boolValue(),
		"ui_group":              stringValue(),
		"body_exposure_default": stringValue(),
		"write_gate":            stringValue(),
		"copy_command":          stringValue(),
		"local_only_reason":     stringValue(),
		"request_schema":        stringValue(),
		"response_schema":       stringValue(),
		"errors":                arrayValue(stringValue()),
		"stability":             enumValue("experimental", "preview", "stable", "deprecated", "legacy"),
	}
}

func manifestCapabilityProperties() map[string]any {
	properties := capabilityProperties()
	properties["declared_surfaces"] = arrayValue(stringValue())
	properties["available_surfaces"] = arrayValue(stringValue())
	properties["bindings"] = arrayValue(refValue(transportBindingV1))
	return properties
}
