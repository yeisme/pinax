package vaultregistry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	RegistrySchemaVersion = "pinax.vault_registry.v1"
	CacheSchemaVersion    = "pinax.vault_cache.v1"
)

type Paths struct {
	ConfigDir string
	CacheDir  string
}

type Registry struct {
	SchemaVersion string                `yaml:"schema_version" json:"schema_version"`
	Default       string                `yaml:"default,omitempty" json:"default,omitempty"`
	Locals        map[string]LocalVault `yaml:"locals,omitempty" json:"locals,omitempty"`
}

type LocalVault struct {
	Path      string    `yaml:"path" json:"path"`
	Name      string    `yaml:"name,omitempty" json:"name,omitempty"`
	CreatedAt time.Time `yaml:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt time.Time `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type Cache struct {
	SchemaVersion string                 `json:"schema_version"`
	Profiles      map[string]RemoteEntry `json:"profiles"`
}

type RemoteEntry struct {
	Profile   string        `json:"profile"`
	Endpoint  string        `json:"endpoint,omitempty"`
	Workspace string        `json:"workspace,omitempty"`
	FetchedAt time.Time     `json:"fetched_at"`
	Vaults    []RemoteVault `json:"vaults"`
}

type RemoteVault struct {
	ID        string `json:"id"`
	Label     string `json:"label,omitempty"`
	Selector  string `json:"selector"`
	Workspace string `json:"workspace,omitempty"`
	Revision  string `json:"revision,omitempty"`
}

type ResolveInfo struct {
	Kind string
	Name string
}

type RemoteRefreshRequest struct {
	Profile   string
	Endpoint  string
	Workspace string
	Token     string
	Client    *http.Client
}

func DefaultPaths() Paths {
	return Paths{ConfigDir: defaultConfigDir(), CacheDir: defaultCacheDir()}
}

func defaultConfigDir() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "pinax")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".pinax")
	}
	return filepath.Join(home, ".config", "pinax")
}

func defaultCacheDir() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); dir != "" {
		return filepath.Join(dir, "pinax", "vaults")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".cache", "pinax", "vaults")
}

func RegistryPath(paths Paths) string {
	configDir := paths.ConfigDir
	if configDir == "" {
		configDir = DefaultPaths().ConfigDir
	}
	return filepath.Join(configDir, "vaults.yaml")
}

func CachePath(paths Paths) string {
	cacheDir := paths.CacheDir
	if cacheDir == "" {
		cacheDir = DefaultPaths().CacheDir
	}
	return filepath.Join(cacheDir, "cache.json")
}

func LoadRegistry(paths Paths) (Registry, error) {
	path := RegistryPath(paths)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newRegistry(), nil
		}
		return Registry{}, err
	}
	var registry Registry
	if err := yaml.Unmarshal(b, &registry); err != nil {
		return Registry{}, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = RegistrySchemaVersion
	}
	if registry.Locals == nil {
		registry.Locals = map[string]LocalVault{}
	}
	return registry, nil
}

func SaveRegistry(paths Paths, registry Registry) error {
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = RegistrySchemaVersion
	}
	if registry.Locals == nil {
		registry.Locals = map[string]LocalVault{}
	}
	path := RegistryPath(paths)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(registry)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func RegisterLocal(paths Paths, alias, vaultPath string, makeDefault bool) error {
	alias = strings.TrimSpace(alias)
	if err := validateAlias(alias); err != nil {
		return err
	}
	if strings.TrimSpace(vaultPath) == "" {
		return fmt.Errorf("vault path is required")
	}
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return err
	}
	registry, err := LoadRegistry(paths)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	item := registry.Locals[alias]
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	item.Path = abs
	item.Name = alias
	registry.Locals[alias] = item
	if makeDefault || registry.Default == "" {
		registry.Default = alias
	}
	return SaveRegistry(paths, registry)
}

func UseDefault(paths Paths, alias string) error {
	alias = strings.TrimSpace(alias)
	registry, err := LoadRegistry(paths)
	if err != nil {
		return err
	}
	if _, ok := registry.Locals[alias]; !ok {
		return fmt.Errorf("unknown vault alias: %s", alias)
	}
	registry.Default = alias
	return SaveRegistry(paths, registry)
}

func ResolveSelector(paths Paths, selector string) (string, ResolveInfo, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = DefaultAlias(paths)
	}
	if selector == "" {
		return ".", ResolveInfo{Kind: "path"}, nil
	}
	if isPathLike(selector) {
		return selector, ResolveInfo{Kind: "path"}, nil
	}
	registry, err := LoadRegistry(paths)
	if err != nil {
		return selector, ResolveInfo{Kind: "path"}, nil
	}
	if item, ok := registry.Locals[selector]; ok {
		return item.Path, ResolveInfo{Kind: "local", Name: selector}, nil
	}
	if strings.Contains(selector, ":") {
		return selector, ResolveInfo{Kind: "remote", Name: selector}, nil
	}
	return selector, ResolveInfo{Kind: "path"}, nil
}

func DefaultAlias(paths Paths) string {
	registry, err := LoadRegistry(paths)
	if err != nil {
		return ""
	}
	if _, ok := registry.Locals[registry.Default]; ok {
		return registry.Default
	}
	return ""
}

func CompletionItems(paths Paths) ([]string, error) {
	registry, err := LoadRegistry(paths)
	if err != nil {
		return nil, err
	}
	items := make([]string, 0, len(registry.Locals))
	for alias, local := range registry.Locals {
		description := "local vault " + local.Path
		if alias == registry.Default {
			description += " default"
		}
		items = append(items, alias+"\t"+description)
	}
	cache, err := LoadCache(paths)
	if err == nil {
		profiles := make([]string, 0, len(cache.Profiles))
		for profileName := range cache.Profiles {
			profiles = append(profiles, profileName)
		}
		sort.Strings(profiles)
		for _, profileName := range profiles {
			entry := cache.Profiles[profileName]
			for _, vault := range entry.Vaults {
				if vault.Selector == "" {
					continue
				}
				desc := "remote vault profile=" + entry.Profile
				workspace := vault.Workspace
				if workspace == "" {
					workspace = entry.Workspace
				}
				if workspace != "" {
					desc += " workspace=" + workspace
				}
				if vault.Label != "" {
					desc += " label=" + vault.Label
				}
				items = append(items, vault.Selector+"\t"+desc)
			}
		}
	}
	sort.Strings(items)
	return items, nil
}

func LoadCache(paths Paths) (Cache, error) {
	b, err := ReadCacheBytes(paths)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newCache(), nil
		}
		return Cache{}, err
	}
	var cache Cache
	if err := json.Unmarshal(b, &cache); err != nil {
		return Cache{}, err
	}
	if cache.SchemaVersion == "" {
		cache.SchemaVersion = CacheSchemaVersion
	}
	if cache.Profiles == nil {
		cache.Profiles = map[string]RemoteEntry{}
	}
	return cache, nil
}

func ReadCacheBytes(paths Paths) ([]byte, error) {
	return os.ReadFile(CachePath(paths))
}

func SaveCache(paths Paths, cache Cache) error {
	if cache.SchemaVersion == "" {
		cache.SchemaVersion = CacheSchemaVersion
	}
	if cache.Profiles == nil {
		cache.Profiles = map[string]RemoteEntry{}
	}
	path := CachePath(paths)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func RefreshRemote(paths Paths, req RemoteRefreshRequest) (RemoteEntry, error) {
	profileName := strings.TrimSpace(req.Profile)
	if profileName == "" {
		return RemoteEntry{}, fmt.Errorf("profile is required")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(req.Endpoint), "/")
	if endpoint == "" {
		return RemoteEntry{}, fmt.Errorf("profile endpoint is required")
	}
	client := req.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	listURL := endpoint + "/v1/vaults"
	httpReq, err := http.NewRequest(http.MethodGet, listURL, nil)
	if err != nil {
		return RemoteEntry{}, err
	}
	if req.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.Token)
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return RemoteEntry{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return RemoteEntry{}, fmt.Errorf("remote vault discovery failed: %s", res.Status)
	}
	var raw map[string]any
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&raw); err != nil {
		return RemoteEntry{}, err
	}
	vaults := parseRemoteVaults(raw)
	entry := RemoteEntry{Profile: profileName, Endpoint: endpoint, Workspace: req.Workspace, FetchedAt: time.Now().UTC(), Vaults: vaults}
	cache, err := LoadCache(paths)
	if err != nil {
		return RemoteEntry{}, err
	}
	cache.Profiles[profileName] = entry
	if err := SaveCache(paths, cache); err != nil {
		return RemoteEntry{}, err
	}
	return entry, nil
}

func parseRemoteVaults(raw map[string]any) []RemoteVault {
	value := raw["vaults"]
	if data, ok := raw["data"].(map[string]any); ok {
		if dataVaults, ok := data["vaults"]; ok {
			value = dataVaults
		}
	}
	items, _ := value.([]any)
	vaults := make([]RemoteVault, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := stringValue(m["id"])
		selector := stringValue(m["selector"])
		if selector == "" && id != "" {
			selector = "cloud:" + id
		}
		if selector == "" {
			continue
		}
		vaults = append(vaults, RemoteVault{ID: id, Label: stringValue(m["label"]), Selector: selector, Workspace: stringValue(m["workspace"]), Revision: firstString(m, "revision", "last_seen_revision")})
	}
	sort.Slice(vaults, func(i, j int) bool { return vaults[i].Selector < vaults[j].Selector })
	return vaults
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func validateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("vault alias is required")
	}
	if strings.ContainsAny(alias, `/\:`) || strings.HasPrefix(alias, ".") || strings.HasPrefix(alias, "~") {
		return fmt.Errorf("vault alias must be a simple name")
	}
	return nil
}

func isPathLike(selector string) bool {
	if selector == "." || selector == ".." || strings.HasPrefix(selector, "./") || strings.HasPrefix(selector, "../") || strings.HasPrefix(selector, "~/") || filepath.IsAbs(selector) {
		return true
	}
	return strings.ContainsAny(selector, `/\`)
}

func newRegistry() Registry {
	return Registry{SchemaVersion: RegistrySchemaVersion, Locals: map[string]LocalVault{}}
}

func newCache() Cache {
	return Cache{SchemaVersion: CacheSchemaVersion, Profiles: map[string]RemoteEntry{}}
}

func RedactedRemoteEntry(entry RemoteEntry) map[string]any {
	return map[string]any{"profile": entry.Profile, "endpoint": entry.Endpoint, "workspace": entry.Workspace, "fetched_at": entry.FetchedAt, "vaults": entry.Vaults}
}

func MarshalRegistryForData(registry Registry, cache Cache) map[string]any {
	locals := make(map[string]LocalVault, len(registry.Locals))
	for key, value := range registry.Locals {
		locals[key] = value
	}
	return map[string]any{"default": registry.Default, "locals": locals, "remote_cache": cache.Profiles}
}

func IsRemoteSelector(selector string) bool {
	selector = strings.TrimSpace(selector)
	return strings.Contains(selector, ":") && !isPathLike(selector)
}

func PrettyRemoteList(entry RemoteEntry) []string {
	items := make([]string, 0, len(entry.Vaults))
	for _, vault := range entry.Vaults {
		var b bytes.Buffer
		b.WriteString(vault.Selector)
		if vault.Label != "" {
			b.WriteString("\t")
			b.WriteString(vault.Label)
		}
		items = append(items, b.String())
	}
	return items
}
