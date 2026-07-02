package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	pluginruntime "github.com/yeisme/pinax/internal/plugin"
)

type PluginRequest struct {
	VaultPath  string
	Path       string
	PluginID   string
	Capability string
	Permission string
	Scope      string
	DryRun     bool
	Yes        bool
}

func (s *Service) PluginValidate(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	result, err := pluginruntime.ValidateManifestPath(req.Path)
	if err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection("plugin.validate", cmdErr), cmdErr
	}
	projection := domain.NewProjection("plugin.validate", "Plugin manifest is valid.")
	projection.Facts["plugin_id"] = result.Manifest.ID
	projection.Facts["version"] = result.Manifest.Version
	projection.Facts["runtime"] = string(result.Manifest.Runtime.Kind)
	projection.Facts["capabilities"] = fmt.Sprint(result.CapabilityCount)
	projection.Facts["permission_summary"] = result.PermissionSummary
	projection.Facts["digest_status"] = "sha256"
	projection.Facts["write_status"] = fmt.Sprint(result.WriteStatus)
	projection.Data = map[string]any{
		"plugin": map[string]any{
			"id":                   result.Manifest.ID,
			"name":                 result.Manifest.Name,
			"version":              result.Manifest.Version,
			"runtime":              result.Manifest.Runtime.Kind,
			"capability_count":     result.CapabilityCount,
			"permission_summary":   result.PermissionSummary,
			"manifest_sha256":      result.Digest,
			"write_status":         result.WriteStatus,
			"schema_version":       result.Manifest.SchemaVersion,
			"manifest_path_status": "resolved",
		},
	}
	projection.Actions = []domain.Action{{Name: "install", Command: "pinax plugin install <plugin-path> --scope vault --vault <vault> --json"}}
	return projection, nil
}

func (s *Service) PluginInstall(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("plugin.install", err), err
	}
	store := pluginruntime.Store{Root: root}
	installed, err := store.Install(req.Path, pluginruntime.ScopeOrDefault(req.Scope))
	if err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection("plugin.install", cmdErr), cmdErr
	}
	projection := pluginStateProjection("plugin.install", installed, "Plugin installed disabled.")
	projection.Evidence = pluginruntime.RegistryAssetPaths()
	projection.Actions = []domain.Action{{Name: "enable", Command: fmt.Sprintf("pinax plugin enable %s --vault <vault> --yes --json", shellQuote(installed.ID))}}
	return projection, nil
}

func (s *Service) PluginList(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	registry, err := loadPluginRegistry(req, "plugin.list")
	if err != nil {
		return errorProjection("plugin.list", err), err
	}
	projection := domain.NewProjection("plugin.list", "Plugins listed.")
	projection.Facts["plugins"] = fmt.Sprint(len(registry.Plugins))
	projection.Facts["enabled"] = fmt.Sprint(pluginruntime.EnabledCount(registry))
	projection.Data = map[string]any{"plugins": registry.Plugins}
	if len(registry.Plugins) > 0 {
		projection.Actions = []domain.Action{{Name: "inspect", Command: fmt.Sprintf("pinax plugin inspect %s --vault <vault> --json", shellQuote(registry.Plugins[0].ID))}}
	}
	return projection, nil
}

func (s *Service) PluginInspect(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	registry, err := loadPluginRegistry(req, "plugin.inspect")
	if err != nil {
		return errorProjection("plugin.inspect", err), err
	}
	plugin, ok := pluginruntime.FindPlugin(registry, strings.TrimSpace(req.PluginID))
	if err := pluginruntime.ValidateInstalled(ok); err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection("plugin.inspect", cmdErr), cmdErr
	}
	return pluginStateProjection("plugin.inspect", plugin, "Plugin inspected."), nil
}

func (s *Service) PluginEnable(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	return s.pluginSetEnabled(ctx, req, true)
}

func (s *Service) PluginDisable(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	return s.pluginSetEnabled(ctx, req, false)
}

func (s *Service) PluginPermissionsList(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	plugin, err := findInstalledPlugin(req, "plugin.permissions.list")
	if err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection("plugin.permissions.list", cmdErr), cmdErr
	}
	projection := pluginStateProjection("plugin.permissions.list", plugin, "Plugin permission grants listed.")
	projection.Facts["grants"] = fmt.Sprint(len(plugin.PermissionGrants))
	projection.Data = map[string]any{"plugin_id": plugin.ID, "grants": plugin.PermissionGrants}
	return projection, nil
}

func (s *Service) PluginPermissionsGrant(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	return s.pluginPermissionChange(ctx, req, true)
}

func (s *Service) PluginPermissionsRevoke(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	return s.pluginPermissionChange(ctx, req, false)
}

func (s *Service) pluginPermissionChange(ctx context.Context, req PluginRequest, grant bool) (domain.Projection, error) {
	_ = ctx
	command := "plugin.permissions.revoke"
	if grant {
		command = "plugin.permissions.grant"
	}
	if err := pluginruntime.ValidateApproval(req.Yes); err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	store := pluginruntime.Store{Root: root}
	var plugin pluginruntime.RegistryPlugin
	if grant {
		plugin, err = store.GrantPermission(strings.TrimSpace(req.PluginID), strings.TrimSpace(req.Permission), strings.TrimSpace(req.Capability))
	} else {
		plugin, err = store.RevokePermission(strings.TrimSpace(req.PluginID), strings.TrimSpace(req.Permission), strings.TrimSpace(req.Capability))
	}
	if err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	projection := pluginStateProjection(command, plugin, "Plugin permission grants updated.")
	projection.Facts["permission"] = strings.TrimSpace(req.Permission)
	projection.Facts["capability"] = strings.TrimSpace(req.Capability)
	projection.Facts["grants"] = fmt.Sprint(len(plugin.PermissionGrants))
	projection.Evidence = []string{".pinax/plugins/registry.json", ".pinax/events/plugin-audit.jsonl"}
	return projection, nil
}

func (s *Service) PluginDoctor(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	registry, err := loadPluginRegistry(req, "plugin.doctor")
	if err != nil {
		return errorProjection("plugin.doctor", err), err
	}
	projection := domain.NewProjection("plugin.doctor", "Plugin diagnostics completed.")
	projection.Facts["registry_readable"] = "true"
	projection.Facts["lock_readable"] = "true"
	projection.Facts["plugins"] = fmt.Sprint(len(registry.Plugins))
	projection.Facts["enabled"] = fmt.Sprint(pluginruntime.EnabledCount(registry))
	projection.Data = map[string]any{"plugins": registry.Plugins, "registry_readable": true, "lock_readable": true}
	if len(registry.Plugins) > 0 {
		projection.Actions = []domain.Action{{Name: "inspect", Command: fmt.Sprintf("pinax plugin inspect %s --vault <vault> --json", shellQuote(registry.Plugins[0].ID))}}
	}
	return projection, nil
}

func (s *Service) PluginUninstall(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	_ = ctx
	command := "plugin.uninstall"
	if err := pluginruntime.ValidateApproval(req.Yes); err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	removed, registry, err := (pluginruntime.Store{Root: root}).Uninstall(strings.TrimSpace(req.PluginID))
	if err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	projection := domain.NewProjection(command, "Plugin uninstalled.")
	projection.Facts["plugin_id"] = removed.ID
	projection.Facts["plugins"] = fmt.Sprint(len(registry.Plugins))
	projection.Facts["enabled"] = fmt.Sprint(pluginruntime.EnabledCount(registry))
	projection.Evidence = []string{".pinax/plugins/registry.json", ".pinax/plugins/plugin-lock.json", ".pinax/events/plugin-audit.jsonl"}
	projection.Data = map[string]any{"plugin": removed, "remaining_plugins": registry.Plugins}
	return projection, nil
}

func (s *Service) PluginRun(ctx context.Context, req PluginRequest) (domain.Projection, error) {
	command := "plugin.run"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	store := pluginruntime.Store{Root: root}
	registry, err := store.LoadRegistry()
	if err != nil {
		cmdErr := &domain.CommandError{Code: "plugin_registry_unreadable", Message: "Plugin registry could not be read", Hint: "Run pinax plugin doctor --vault <vault> --json"}
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	plugin, ok := pluginruntime.FindPlugin(registry, strings.TrimSpace(req.PluginID))
	if err := pluginruntime.ValidateInstalled(ok); err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	capability := strings.TrimSpace(req.Capability)
	if !plugin.Enabled {
		cmdErr := &domain.CommandError{Code: "plugin_disabled", Message: "Plugin is disabled", Hint: fmt.Sprintf("Run pinax plugin enable %s --vault <vault> --yes --json", shellQuote(plugin.ID))}
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	if err := pluginruntime.ValidateRunPermission(plugin, capability); err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	result, err := runInstalledPlugin(ctx, root, plugin, capability, req.DryRun)
	if err == nil {
		err = pluginruntime.ValidateActionPlanBoundary(plugin, result)
	}
	auditStatus := "success"
	errorCode := ""
	if err != nil {
		auditStatus = "failed"
		errorCode = pluginruntime.RunnerErrorCode(err)
	}
	if auditErr := store.AppendAudit(pluginruntime.AuditEvent{Type: command, PluginID: plugin.ID, Version: plugin.Version, Runtime: plugin.Runtime, Capability: capability, Status: auditStatus, ErrorCode: errorCode, ManifestSHA256: plugin.ManifestSHA256}); auditErr != nil {
		cmdErr := &domain.CommandError{Code: "plugin_audit_unwritable", Message: "Plugin audit could not be written", Hint: "Check vault permissions and retry"}
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	if err != nil {
		cmdErr := pluginCommandError(err)
		projection := domain.NewErrorProjection(command, cmdErr)
		projection.Facts["plugin_id"] = plugin.ID
		projection.Facts["runtime"] = string(plugin.Runtime)
		projection.Facts["capability"] = capability
		projection.Facts["write_status"] = fmt.Sprint(!req.DryRun)
		return projection, cmdErr
	}
	projection := domain.NewProjection(command, "Plugin capability executed.")
	projection.Facts["plugin_id"] = plugin.ID
	projection.Facts["runtime"] = string(plugin.Runtime)
	projection.Facts["capability"] = capability
	projection.Facts["result_status"] = result.Status
	projection.Facts["write_status"] = fmt.Sprint(!req.DryRun)
	for key, value := range result.Facts {
		if _, exists := projection.Facts[key]; !exists {
			projection.Facts[key] = value
		}
	}
	projection.Evidence = []string{".pinax/events/plugin-audit.jsonl"}
	projection.Data = map[string]any{"plugin_id": plugin.ID, "capability": capability, "runtime": plugin.Runtime, "result": result, "dry_run": req.DryRun}
	return projection, nil
}

func runInstalledPlugin(ctx context.Context, root string, plugin pluginruntime.RegistryPlugin, capability string, dryRun bool) (pluginruntime.ResultEnvelope, error) {
	input := map[string]any{"plugin_id": plugin.ID, "capability": capability, "dry_run": dryRun}
	budgets := pluginruntime.RunnerBudgetsFromManifest(plugin.Budgets)
	switch plugin.Runtime {
	case pluginruntime.RuntimeWASM:
		return pluginruntime.WASMRunner{}.Run(ctx, pluginruntime.RunRequest{Plugin: plugin, Capability: capability, Input: input, Budgets: budgets})
	case pluginruntime.RuntimePython, pluginruntime.RuntimeJavaScript, pluginruntime.RuntimeProcess:
		if strings.TrimSpace(plugin.RuntimeRoot) == "" || strings.TrimSpace(plugin.RuntimeEntrypoint) == "" {
			return pluginruntime.ResultEnvelope{}, &pluginruntime.RunnerError{Code: "plugin_runner_unavailable", Message: "Plugin runtime assets are not installed", Err: pluginruntime.ErrRunnerUnavailable}
		}
		manifest := pluginruntime.Manifest{ID: plugin.ID, Version: plugin.Version, Runtime: pluginruntime.Runtime{Kind: plugin.Runtime, Entrypoint: plugin.RuntimeEntrypoint}, Budgets: plugin.Budgets}
		return pluginruntime.ExternalRunner{}.Run(ctx, pluginruntime.ExternalRunRequest{Manifest: manifest, PluginRoot: filepath.Join(root, filepath.FromSlash(plugin.RuntimeRoot)), Capability: capability, Input: input, Budgets: budgets})
	default:
		return pluginruntime.ResultEnvelope{}, &pluginruntime.RunnerError{Code: "plugin_runtime_unsupported", Message: "Plugin runtime is not supported"}
	}
}

func (s *Service) pluginSetEnabled(ctx context.Context, req PluginRequest, enabled bool) (domain.Projection, error) {
	_ = ctx
	command := "plugin.disable"
	if enabled {
		command = "plugin.enable"
	}
	if err := pluginruntime.ValidateApproval(req.Yes); err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	plugin, err := (pluginruntime.Store{Root: root}).SetEnabled(strings.TrimSpace(req.PluginID), enabled)
	if err != nil {
		cmdErr := pluginCommandError(err)
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	summary := "Plugin disabled."
	if enabled {
		summary = "Plugin enabled."
	}
	projection := pluginStateProjection(command, plugin, summary)
	projection.Evidence = []string{".pinax/plugins/registry.json", ".pinax/events/plugin-audit.jsonl"}
	return projection, nil
}

func loadPluginRegistry(req PluginRequest, command string) (pluginruntime.Registry, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return pluginruntime.Registry{}, err
	}
	registry, err := (pluginruntime.Store{Root: root}).LoadRegistry()
	if err != nil {
		return pluginruntime.Registry{}, &domain.CommandError{Code: "plugin_registry_unreadable", Message: "Plugin registry could not be read", Hint: "Run pinax plugin doctor --vault <vault> --json"}
	}
	_ = command
	return registry, nil
}

func findInstalledPlugin(req PluginRequest, command string) (pluginruntime.RegistryPlugin, error) {
	registry, err := loadPluginRegistry(req, command)
	if err != nil {
		return pluginruntime.RegistryPlugin{}, err
	}
	plugin, ok := pluginruntime.FindPlugin(registry, strings.TrimSpace(req.PluginID))
	if err := pluginruntime.ValidateInstalled(ok); err != nil {
		return pluginruntime.RegistryPlugin{}, err
	}
	return plugin, nil
}

func pluginStateProjection(command string, plugin pluginruntime.RegistryPlugin, summary string) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["plugin_id"] = plugin.ID
	projection.Facts["version"] = plugin.Version
	projection.Facts["runtime"] = string(plugin.Runtime)
	projection.Facts["enabled"] = fmt.Sprint(plugin.Enabled)
	projection.Facts["scope"] = plugin.Scope
	projection.Facts["capabilities"] = fmt.Sprint(plugin.CapabilityCount)
	projection.Facts["digest_status"] = "sha256"
	projection.Data = map[string]any{"plugin": plugin}
	return projection
}

func pluginCommandError(err error) *domain.CommandError {
	if code := pluginruntime.RunnerErrorCode(err); code != "" {
		message := "Plugin runtime error"
		switch code {
		case "plugin_permission_denied":
			message = "Plugin permission denied"
		case "plugin_permission_invalid":
			message = "Plugin permission is invalid"
		}
		return &domain.CommandError{Code: code, Message: message, Hint: "Run pinax plugin permissions list <plugin-id> --vault <vault> --json"}
	}
	if validationErr, ok := err.(*pluginruntime.ValidationError); ok {
		message := "Plugin manifest is invalid"
		if validationErr.Code == "approval_required" {
			message = "Plugin state changes require --yes"
		} else if validationErr.Code == "plugin_not_installed" {
			message = "Plugin is not installed"
		} else if validationErr.Code == "plugin_manifest_secret_rejected" {
			message = "Plugin manifest contains secret-like content"
		} else if len(validationErr.Issues) > 0 && validationErr.Issues[0].Message != "" {
			message = validationErr.Issues[0].Message
		}
		return &domain.CommandError{Code: validationErr.Code, Message: message, Hint: "Run pinax plugin validate <plugin-path> --json after removing the reported issue"}
	}
	return &domain.CommandError{Code: "plugin_manifest_invalid", Message: "Plugin manifest is invalid", Hint: "Run pinax plugin validate <plugin-path> --json"}
}
