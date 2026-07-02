package plugin

func ValidatePermissionName(permission string) error {
	switch permission {
	case "projection.read", "note.body.read", "action_plan.write", "temp.write", "network", "env.read":
		return nil
	default:
		return &RunnerError{Code: "plugin_permission_invalid", Message: "Plugin permission is not allowed"}
	}
}

func HasPermission(plugin RegistryPlugin, permission, capability string) bool {
	for _, grant := range plugin.PermissionGrants {
		if grant.Permission != permission {
			continue
		}
		if grant.Capability == "" || grant.Capability == capability {
			return true
		}
	}
	return false
}

func ValidateRunPermission(plugin RegistryPlugin, capability string) error {
	if HasPermission(plugin, "projection.read", capability) {
		return nil
	}
	return &RunnerError{Code: "plugin_permission_denied", Message: "Plugin capability requires projection.read grant"}
}

func ValidateActionPlanBoundary(plugin RegistryPlugin, result ResultEnvelope) error {
	if result.ActionPlan == nil {
		return nil
	}
	if HasAnyPermission(plugin, "action_plan.write") {
		return nil
	}
	return &RunnerError{Code: "plugin_permission_denied", Message: "Plugin action plans require action_plan.write grant"}
}

func HasAnyPermission(plugin RegistryPlugin, permission string) bool {
	for _, grant := range plugin.PermissionGrants {
		if grant.Permission == permission {
			return true
		}
	}
	return false
}
