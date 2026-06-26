package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const RegistrySchemaVersion = "pinax.plugin_registry.v1"
const LockSchemaVersion = "pinax.plugin_lock.v1"
const AuditSchemaVersion = "pinax.plugin_audit.v1"

type Registry struct {
	SchemaVersion string            `json:"schema_version"`
	Plugins       []RegistryPlugin  `json:"plugins"`
	UpdatedAt     string            `json:"updated_at"`
	Facts         map[string]string `json:"facts,omitempty"`
}

type RegistryPlugin struct {
	ID                string            `json:"id"`
	Name              string            `json:"name,omitempty"`
	Version           string            `json:"version"`
	Runtime           RuntimeKind       `json:"runtime"`
	RuntimeEntrypoint string            `json:"runtime_entrypoint,omitempty"`
	RuntimeRoot       string            `json:"runtime_root,omitempty"`
	Enabled           bool              `json:"enabled"`
	Scope             string            `json:"scope"`
	ManifestSHA256    string            `json:"manifest_sha256"`
	CapabilityCount   int               `json:"capability_count"`
	Capabilities      []Capability      `json:"capabilities,omitempty"`
	PermissionSummary string            `json:"permission_summary"`
	PermissionGrants  []PermissionGrant `json:"permission_grants,omitempty"`
	Budgets           Budgets           `json:"budgets,omitempty"`
	InstalledAt       string            `json:"installed_at"`
	UpdatedAt         string            `json:"updated_at"`
}

type PermissionGrant struct {
	Permission string `json:"permission"`
	Capability string `json:"capability,omitempty"`
	GrantedAt  string `json:"granted_at"`
}

type LockFile struct {
	SchemaVersion string      `json:"schema_version"`
	Plugins       []LockEntry `json:"plugins"`
	UpdatedAt     string      `json:"updated_at"`
}

type LockEntry struct {
	ID                 string      `json:"id"`
	Version            string      `json:"version"`
	Runtime            RuntimeKind `json:"runtime"`
	RuntimeEntrypoint  string      `json:"runtime_entrypoint,omitempty"`
	RuntimeRoot        string      `json:"runtime_root,omitempty"`
	ManifestSHA256     string      `json:"manifest_sha256"`
	EntrypointSHA256   string      `json:"entrypoint_sha256,omitempty"`
	InstalledByCommand string      `json:"installed_by_command"`
	InstalledAt        string      `json:"installed_at"`
}

type AuditEvent struct {
	SchemaVersion  string      `json:"schema_version"`
	Type           string      `json:"type"`
	PluginID       string      `json:"plugin_id"`
	Version        string      `json:"version,omitempty"`
	Runtime        RuntimeKind `json:"runtime,omitempty"`
	Capability     string      `json:"capability,omitempty"`
	Status         string      `json:"status"`
	ErrorCode      string      `json:"error_code,omitempty"`
	ManifestSHA256 string      `json:"manifest_sha256,omitempty"`
	At             string      `json:"at"`
}

type Store struct {
	Root string
	Now  func() time.Time
}

func (s Store) Install(path, scope string) (RegistryPlugin, error) {
	if scope == "" {
		scope = "vault"
	}
	if scope != "vault" {
		return RegistryPlugin{}, &ValidationError{Code: "plugin_scope_invalid", Issues: []ValidationIssue{{Code: "plugin_scope_invalid", Field: "scope", Message: "Plugin scope must be vault"}}}
	}
	result, err := ValidateManifestPath(path)
	if err != nil {
		return RegistryPlugin{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	registry, err := s.LoadRegistry()
	if err != nil {
		return RegistryPlugin{}, err
	}
	plugin := RegistryPlugin{ID: result.Manifest.ID, Name: result.Manifest.Name, Version: result.Manifest.Version, Runtime: result.Manifest.Runtime.Kind, RuntimeEntrypoint: filepath.ToSlash(filepath.Clean(result.Manifest.Runtime.Entrypoint)), Enabled: false, Scope: scope, ManifestSHA256: result.Digest, CapabilityCount: result.CapabilityCount, Capabilities: result.Manifest.Capabilities, PermissionSummary: result.PermissionSummary, Budgets: result.Manifest.Budgets, InstalledAt: now, UpdatedAt: now}
	if runtimeNeedsPackagedRoot(plugin.Runtime) {
		root, err := s.packageExternalRuntime(result, plugin.ID)
		if err != nil {
			return RegistryPlugin{}, err
		}
		plugin.RuntimeRoot = root
	}
	for i, existing := range registry.Plugins {
		if existing.ID == plugin.ID {
			plugin.Enabled = existing.Enabled
			plugin.InstalledAt = existing.InstalledAt
			registry.Plugins[i] = plugin
			return plugin, s.writeInstallAssets(registry, result, plugin, now)
		}
	}
	registry.Plugins = append(registry.Plugins, plugin)
	return plugin, s.writeInstallAssets(registry, result, plugin, now)
}

func (s Store) LoadRegistry() (Registry, error) {
	path := s.registryPath()
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Registry{SchemaVersion: RegistrySchemaVersion, Plugins: []RegistryPlugin{}, Facts: map[string]string{}}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var registry Registry
	if err := json.Unmarshal(body, &registry); err != nil {
		return Registry{}, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = RegistrySchemaVersion
	}
	return registry, nil
}

func (s Store) SetEnabled(id string, enabled bool) (RegistryPlugin, error) {
	registry, err := s.LoadRegistry()
	if err != nil {
		return RegistryPlugin{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	for i := range registry.Plugins {
		if registry.Plugins[i].ID != id {
			continue
		}
		registry.Plugins[i].Enabled = enabled
		registry.Plugins[i].UpdatedAt = now
		plugin := registry.Plugins[i]
		if err := s.writeRegistry(registry, now); err != nil {
			return RegistryPlugin{}, err
		}
		if err := s.appendAudit(AuditEvent{Type: eventType(enabled), PluginID: plugin.ID, Version: plugin.Version, Runtime: plugin.Runtime, Status: "success", ManifestSHA256: plugin.ManifestSHA256, At: now}); err != nil {
			return RegistryPlugin{}, err
		}
		return plugin, nil
	}
	return RegistryPlugin{}, &ValidationError{Code: "plugin_not_installed", Issues: []ValidationIssue{{Code: "plugin_not_installed", Field: "plugin_id", Message: "Plugin is not installed"}}}
}

func (s Store) GrantPermission(id, permission, capability string) (RegistryPlugin, error) {
	if err := ValidatePermissionName(permission); err != nil {
		return RegistryPlugin{}, err
	}
	registry, err := s.LoadRegistry()
	if err != nil {
		return RegistryPlugin{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	for i := range registry.Plugins {
		if registry.Plugins[i].ID != id {
			continue
		}
		if !HasPermission(registry.Plugins[i], permission, capability) {
			registry.Plugins[i].PermissionGrants = append(registry.Plugins[i].PermissionGrants, PermissionGrant{Permission: permission, Capability: capability, GrantedAt: now})
		}
		registry.Plugins[i].UpdatedAt = now
		plugin := registry.Plugins[i]
		if err := s.writeRegistry(registry, now); err != nil {
			return RegistryPlugin{}, err
		}
		if err := s.appendAudit(AuditEvent{Type: "plugin.permissions.grant", PluginID: plugin.ID, Version: plugin.Version, Runtime: plugin.Runtime, Status: "success", ManifestSHA256: plugin.ManifestSHA256, At: now}); err != nil {
			return RegistryPlugin{}, err
		}
		return plugin, nil
	}
	return RegistryPlugin{}, &ValidationError{Code: "plugin_not_installed", Issues: []ValidationIssue{{Code: "plugin_not_installed", Field: "plugin_id", Message: "Plugin is not installed"}}}
}

func (s Store) RevokePermission(id, permission, capability string) (RegistryPlugin, error) {
	if err := ValidatePermissionName(permission); err != nil {
		return RegistryPlugin{}, err
	}
	registry, err := s.LoadRegistry()
	if err != nil {
		return RegistryPlugin{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	for i := range registry.Plugins {
		if registry.Plugins[i].ID != id {
			continue
		}
		grants := make([]PermissionGrant, 0, len(registry.Plugins[i].PermissionGrants))
		for _, grant := range registry.Plugins[i].PermissionGrants {
			if grant.Permission == permission && grant.Capability == capability {
				continue
			}
			grants = append(grants, grant)
		}
		registry.Plugins[i].PermissionGrants = grants
		registry.Plugins[i].UpdatedAt = now
		plugin := registry.Plugins[i]
		if err := s.writeRegistry(registry, now); err != nil {
			return RegistryPlugin{}, err
		}
		if err := s.appendAudit(AuditEvent{Type: "plugin.permissions.revoke", PluginID: plugin.ID, Version: plugin.Version, Runtime: plugin.Runtime, Status: "success", ManifestSHA256: plugin.ManifestSHA256, At: now}); err != nil {
			return RegistryPlugin{}, err
		}
		return plugin, nil
	}
	return RegistryPlugin{}, &ValidationError{Code: "plugin_not_installed", Issues: []ValidationIssue{{Code: "plugin_not_installed", Field: "plugin_id", Message: "Plugin is not installed"}}}
}

func (s Store) Uninstall(id string) (RegistryPlugin, Registry, error) {
	registry, err := s.LoadRegistry()
	if err != nil {
		return RegistryPlugin{}, Registry{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	var removed RegistryPlugin
	plugins := make([]RegistryPlugin, 0, len(registry.Plugins))
	for _, plugin := range registry.Plugins {
		if plugin.ID == id {
			removed = plugin
			continue
		}
		plugins = append(plugins, plugin)
	}
	if removed.ID == "" {
		return RegistryPlugin{}, Registry{}, &ValidationError{Code: "plugin_not_installed", Issues: []ValidationIssue{{Code: "plugin_not_installed", Field: "plugin_id", Message: "Plugin is not installed"}}}
	}
	registry.Plugins = plugins
	if err := s.writeRegistry(registry, now); err != nil {
		return RegistryPlugin{}, Registry{}, err
	}
	lock, err := s.loadLock()
	if err != nil {
		return RegistryPlugin{}, Registry{}, err
	}
	lockEntries := make([]LockEntry, 0, len(lock.Plugins))
	for _, entry := range lock.Plugins {
		if entry.ID != id {
			lockEntries = append(lockEntries, entry)
		}
	}
	lock.Plugins = lockEntries
	if err := s.writeLock(lock, now); err != nil {
		return RegistryPlugin{}, Registry{}, err
	}
	if removed.RuntimeRoot != "" {
		if err := os.RemoveAll(filepath.Join(s.Root, filepath.FromSlash(removed.RuntimeRoot))); err != nil {
			return RegistryPlugin{}, Registry{}, err
		}
	}
	if err := s.appendAudit(AuditEvent{Type: "plugin.uninstall", PluginID: removed.ID, Version: removed.Version, Runtime: removed.Runtime, Status: "success", ManifestSHA256: removed.ManifestSHA256, At: now}); err != nil {
		return RegistryPlugin{}, Registry{}, err
	}
	return removed, registry, nil
}

func (s Store) writeInstallAssets(registry Registry, result ValidationResult, plugin RegistryPlugin, now string) error {
	if err := s.writeRegistry(registry, now); err != nil {
		return err
	}
	lock, err := s.loadLock()
	if err != nil {
		return err
	}
	entry := LockEntry{ID: plugin.ID, Version: plugin.Version, Runtime: plugin.Runtime, RuntimeEntrypoint: plugin.RuntimeEntrypoint, RuntimeRoot: plugin.RuntimeRoot, ManifestSHA256: plugin.ManifestSHA256, EntrypointSHA256: entrypointDigest(result), InstalledByCommand: "pinax plugin install", InstalledAt: now}
	updated := false
	for i := range lock.Plugins {
		if lock.Plugins[i].ID == entry.ID {
			lock.Plugins[i] = entry
			updated = true
		}
	}
	if !updated {
		lock.Plugins = append(lock.Plugins, entry)
	}
	if err := s.writeLock(lock, now); err != nil {
		return err
	}
	return s.appendAudit(AuditEvent{Type: "plugin.install", PluginID: plugin.ID, Version: plugin.Version, Runtime: plugin.Runtime, Status: "success", ManifestSHA256: plugin.ManifestSHA256, At: now})
}

func (s Store) writeRegistry(registry Registry, now string) error {
	registry.SchemaVersion = RegistrySchemaVersion
	registry.UpdatedAt = now
	sort.Slice(registry.Plugins, func(i, j int) bool { return registry.Plugins[i].ID < registry.Plugins[j].ID })
	return writeJSONFile(s.registryPath(), registry)
}

func (s Store) loadLock() (LockFile, error) {
	body, err := os.ReadFile(s.lockPath())
	if os.IsNotExist(err) {
		return LockFile{SchemaVersion: LockSchemaVersion, Plugins: []LockEntry{}}, nil
	}
	if err != nil {
		return LockFile{}, err
	}
	var lock LockFile
	if err := json.Unmarshal(body, &lock); err != nil {
		return LockFile{}, err
	}
	return lock, nil
}

func (s Store) writeLock(lock LockFile, now string) error {
	lock.SchemaVersion = LockSchemaVersion
	lock.UpdatedAt = now
	sort.Slice(lock.Plugins, func(i, j int) bool { return lock.Plugins[i].ID < lock.Plugins[j].ID })
	return writeJSONFile(s.lockPath(), lock)
}

func (s Store) appendAudit(event AuditEvent) error {
	if event.At == "" {
		event.At = s.now().UTC().Format(time.RFC3339)
	}
	event.SchemaVersion = AuditSchemaVersion
	path := filepath.Join(s.Root, ".pinax", "events", "plugin-audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.Write(append(body, '\n'))
	return err
}

func (s Store) AppendAudit(event AuditEvent) error {
	return s.appendAudit(event)
}

func (s Store) registryPath() string {
	return filepath.Join(s.Root, ".pinax", "plugins", "registry.json")
}
func (s Store) lockPath() string {
	return filepath.Join(s.Root, ".pinax", "plugins", "plugin-lock.json")
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func entrypointDigest(result ValidationResult) string {
	path := filepath.Join(filepath.Dir(result.ManifestPath), filepath.Clean(result.Manifest.Runtime.Entrypoint))
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func runtimeNeedsPackagedRoot(kind RuntimeKind) bool {
	switch kind {
	case RuntimePython, RuntimeJavaScript, RuntimeProcess:
		return true
	default:
		return false
	}
}

func (s Store) packageExternalRuntime(result ValidationResult, pluginID string) (string, error) {
	sourceRoot := filepath.Dir(result.ManifestPath)
	destRel := filepath.ToSlash(filepath.Join(".pinax", "plugins", "runners", pluginID))
	destRoot := filepath.Join(s.Root, filepath.FromSlash(destRel))
	if err := os.RemoveAll(destRoot); err != nil {
		return "", err
	}
	if err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		cleanRel := filepath.Clean(rel)
		if cleanRel == ".pinax" || strings.HasPrefix(cleanRel, ".pinax"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destRoot, cleanRel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	}); err != nil {
		return "", err
	}
	return destRel, nil
}

func eventType(enabled bool) string {
	if enabled {
		return "plugin.enable"
	}
	return "plugin.disable"
}

func FindPlugin(registry Registry, id string) (RegistryPlugin, bool) {
	for _, plugin := range registry.Plugins {
		if plugin.ID == id {
			return plugin, true
		}
	}
	return RegistryPlugin{}, false
}

func EnabledCount(registry Registry) int {
	count := 0
	for _, plugin := range registry.Plugins {
		if plugin.Enabled {
			count++
		}
	}
	return count
}

func ValidateApproval(yes bool) error {
	if yes {
		return nil
	}
	return &ValidationError{Code: "approval_required", Issues: []ValidationIssue{{Code: "approval_required", Field: "yes", Message: "Plugin state changes require --yes"}}}
}

func ValidateInstalled(ok bool) error {
	if ok {
		return nil
	}
	return &ValidationError{Code: "plugin_not_installed", Issues: []ValidationIssue{{Code: "plugin_not_installed", Field: "plugin_id", Message: "Plugin is not installed"}}}
}

func ScopeOrDefault(scope string) string {
	if scope == "" {
		return "vault"
	}
	return scope
}

func RegistryAssetPaths() []string {
	return []string{".pinax/plugins/registry.json", ".pinax/plugins/plugin-lock.json", ".pinax/events/plugin-audit.jsonl"}
}

func StoreError(code, message string) error {
	return &ValidationError{Code: code, Issues: []ValidationIssue{{Code: code, Message: message}}}
}

func FormatBool(value bool) string { return fmt.Sprint(value) }
