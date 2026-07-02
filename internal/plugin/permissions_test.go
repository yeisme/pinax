package plugin

import "testing"

func TestPermissionEngineDeniesActionPlanWithoutGrant(t *testing.T) {
	plugin := RegistryPlugin{ID: "planner", PermissionGrants: []PermissionGrant{{Permission: "projection.read", Capability: "plan"}}}
	result := ResultEnvelope{SchemaVersion: PluginResultSchema, Status: "success", ActionPlan: map[string]any{"actions": []any{map[string]any{"kind": "note.update"}}}}
	if err := ValidateActionPlanBoundary(plugin, result); RunnerErrorCode(err) != "plugin_permission_denied" {
		t.Fatalf("action plan without grant err = %v", err)
	}
	plugin.PermissionGrants = append(plugin.PermissionGrants, PermissionGrant{Permission: "action_plan.write", Capability: "plan"})
	if err := ValidateActionPlanBoundary(plugin, result); err != nil {
		t.Fatalf("action plan with grant: %v", err)
	}
}

func TestPermissionGrantValidationRejectsUnsafePermission(t *testing.T) {
	if err := ValidatePermissionName("env.SECRET_TOKEN"); RunnerErrorCode(err) != "plugin_permission_invalid" {
		t.Fatalf("permission validation err = %v", err)
	}
	if err := ValidatePermissionName("projection.read"); err != nil {
		t.Fatalf("projection.read should be valid: %v", err)
	}
}
