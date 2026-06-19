package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Vault    string         `mapstructure:"vault" yaml:"vault" json:"vault,omitempty"`
	Remote   RemoteConfig   `mapstructure:"remote" yaml:"remote" json:"remote"`
	Output   OutputConfig   `mapstructure:"output" yaml:"output" json:"output"`
	Editor   EditorConfig   `mapstructure:"editor" yaml:"editor" json:"editor"`
	Note     NoteConfig     `mapstructure:"note" yaml:"note" json:"note"`
	KB       KBConfig       `mapstructure:"kb" yaml:"kb" json:"kb"`
	Search   SearchConfig   `mapstructure:"search" yaml:"search" json:"search"`
	Storage  StorageConfig  `mapstructure:"storage" yaml:"storage" json:"storage"`
	Themes   ThemeSet       `mapstructure:"themes" yaml:"themes" json:"themes"`
	Markdown MarkdownConfig `mapstructure:"markdown" yaml:"markdown" json:"markdown"`
}

type RemoteConfig struct {
	APIURL string `mapstructure:"api_url" yaml:"api_url" json:"api_url,omitempty"`
}

type OutputConfig struct {
	Color    string         `mapstructure:"color" yaml:"color" json:"color"`
	Theme    string         `mapstructure:"theme" yaml:"theme" json:"theme"`
	Width    int            `mapstructure:"width" yaml:"width" json:"width"`
	Markdown MarkdownConfig `mapstructure:"markdown" yaml:"markdown" json:"markdown"`
}

type MarkdownConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Style   string `mapstructure:"style" yaml:"style" json:"style"`
	Pager   string `mapstructure:"pager" yaml:"pager" json:"pager,omitempty"`
}

type ThemeSet struct {
	Custom map[string]string `mapstructure:"custom" yaml:"custom" json:"custom,omitempty"`
}

type EditorConfig struct {
	Command string `mapstructure:"command" yaml:"command" json:"command,omitempty"`
}

type NoteConfig struct {
	Status string `mapstructure:"status" yaml:"status" json:"status,omitempty"`
	Kind   string `mapstructure:"kind" yaml:"kind" json:"kind,omitempty"`
}

type KBConfig struct {
	Sidecar KBSidecarConfig `mapstructure:"sidecar" yaml:"sidecar" json:"sidecar"`
}

type KBSidecarConfig struct {
	Executable     string `mapstructure:"executable" yaml:"executable" json:"executable,omitempty"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" yaml:"timeout_seconds" json:"timeout_seconds"`
}

type SearchConfig struct {
	Limit      int  `mapstructure:"limit" yaml:"limit" json:"limit"`
	AllowStale bool `mapstructure:"allow_stale" yaml:"allow_stale" json:"allow_stale"`
}

type StorageConfig struct {
	Backend  string `mapstructure:"backend" yaml:"backend" json:"backend,omitempty"`
	Bucket   string `mapstructure:"bucket" yaml:"bucket" json:"bucket,omitempty"`
	Region   string `mapstructure:"region" yaml:"region" json:"region,omitempty"`
	Prefix   string `mapstructure:"prefix" yaml:"prefix" json:"prefix,omitempty"`
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint" json:"endpoint,omitempty"`
	Profile  string `mapstructure:"profile" yaml:"profile" json:"profile,omitempty"`
	Token    string `mapstructure:"token" yaml:"token" json:"-"`
}

type SourceSet struct {
	UserConfig    string   `json:"user_config,omitempty"`
	ProjectConfig string   `json:"project_config,omitempty"`
	EnvKeys       []string `json:"env_keys,omitempty"`
	FlagKeys      []string `json:"flag_keys,omitempty"`
}

func (s SourceSet) Contains(value string) bool {
	if s.UserConfig == value || s.ProjectConfig == value {
		return true
	}
	for _, key := range s.EnvKeys {
		if key == value {
			return true
		}
	}
	for _, key := range s.FlagKeys {
		if key == value {
			return true
		}
	}
	return false
}

type LoadResult struct {
	Config  Config    `json:"config"`
	Sources SourceSet `json:"sources"`
}

type fileConfig struct {
	Config Config
	Set    map[string]bool
}

func (f fileConfig) IsSet(key string) bool {
	return f.Set[key]
}

type LoadOptions struct {
	VaultPath         string
	UserConfigPath    string
	ProjectConfigPath string
	ExplicitFlags     map[string]string
	Env               func(string) (string, bool)
}

type PathOptions struct {
	HomeDir       string
	XDGConfigHome string
	VaultPath     string
}

type Paths struct {
	User    string `json:"user"`
	Project string `json:"project"`
}

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ErrorCode(err error) string {
	var configErr *Error
	if errors.As(err, &configErr) {
		return configErr.Code
	}
	return ""
}

func DefaultConfig() Config {
	return Config{
		Output:  OutputConfig{Color: "auto", Theme: "pinax", Width: 100, Markdown: MarkdownConfig{Enabled: true, Style: "auto"}},
		Editor:  EditorConfig{},
		Note:    NoteConfig{Status: "active"},
		KB:      KBConfig{Sidecar: KBSidecarConfig{Executable: "pinax-lancedb-sidecar", TimeoutSeconds: 30}},
		Search:  SearchConfig{Limit: 20},
		Storage: StorageConfig{Backend: "local"},
		Themes:  ThemeSet{Custom: map[string]string{}},
	}
}

func ResolvePaths(opts PathOptions) Paths {
	xdg := strings.TrimSpace(opts.XDGConfigHome)
	if xdg == "" {
		home := strings.TrimSpace(opts.HomeDir)
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		xdg = filepath.Join(home, ".config")
	}
	vault := strings.TrimSpace(opts.VaultPath)
	if vault == "" {
		vault = "."
	}
	return Paths{User: filepath.Join(xdg, "pinax", "config.yaml"), Project: filepath.Join(vault, ".pinax", "config.yaml")}
}

func Load(opts LoadOptions) (LoadResult, error) {
	cfg := DefaultConfig()
	if opts.VaultPath != "" {
		cfg.Vault = opts.VaultPath
	}
	result := LoadResult{Config: cfg}
	if opts.Env == nil {
		opts.Env = os.LookupEnv
	}
	if opts.UserConfigPath != "" {
		loaded, ok, err := readConfigFile(opts.UserConfigPath)
		if err != nil {
			return LoadResult{}, err
		}
		if ok {
			mergeConfig(&cfg, loaded.Config, loaded.IsSet)
			result.Sources.UserConfig = opts.UserConfigPath
		}
	}
	if opts.ProjectConfigPath != "" {
		if !projectConfigInsideVault(opts.VaultPath, opts.ProjectConfigPath) {
			return LoadResult{}, &Error{Code: "config_path_outside_vault", Message: "项目配置路径必须位于 vault 的 .pinax/config.yaml 内"}
		}
		loaded, ok, err := readConfigFile(opts.ProjectConfigPath)
		if err != nil {
			return LoadResult{}, err
		}
		if ok {
			mergeConfig(&cfg, loaded.Config, loaded.IsSet)
			result.Sources.ProjectConfig = opts.ProjectConfigPath
		}
	}
	applyEnv(&cfg, &result.Sources, opts.Env)
	applyExplicitFlags(&cfg, &result.Sources, opts.ExplicitFlags)
	if err := cfg.Validate(); err != nil {
		return LoadResult{}, err
	}
	result.Config = cfg
	return result, nil
}

func projectConfigInsideVault(vaultPath, configPath string) bool {
	if strings.TrimSpace(vaultPath) == "" || strings.TrimSpace(configPath) == "" {
		return true
	}
	vaultAbs, err := filepath.Abs(vaultPath)
	if err != nil {
		return false
	}
	configAbs, err := filepath.Abs(configPath)
	if err != nil {
		return false
	}
	want := filepath.Join(vaultAbs, ".pinax", "config.yaml")
	if configAbs == want {
		return true
	}
	rel, err := filepath.Rel(vaultAbs, configAbs)
	return err == nil && rel == filepath.Join(".pinax", "config.yaml")
}

func readConfigFile(path string) (fileConfig, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileConfig{}, false, nil
		}
		return fileConfig{}, false, err
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return fileConfig{}, false, err
	}
	if err := rejectSecretLikeSettings(v.AllSettings(), nil); err != nil {
		return fileConfig{}, false, err
	}
	set := make(map[string]bool)
	for _, key := range configKeys() {
		set[key] = v.IsSet(key)
	}
	return fileConfig{Config: configFromViper(v, set), Set: set}, true, nil
}

func configFromViper(v *viper.Viper, set map[string]bool) Config {
	cfg := Config{}
	if set["vault"] {
		cfg.Vault = v.GetString("vault")
	}
	if set["remote.api_url"] {
		cfg.Remote.APIURL = v.GetString("remote.api_url")
	}
	if set["output.color"] {
		cfg.Output.Color = v.GetString("output.color")
	}
	if set["output.theme"] {
		cfg.Output.Theme = v.GetString("output.theme")
	}
	if set["output.width"] {
		cfg.Output.Width = v.GetInt("output.width")
	}
	if set["output.markdown.enabled"] {
		cfg.Output.Markdown.Enabled = v.GetBool("output.markdown.enabled")
	}
	if set["output.markdown.style"] {
		cfg.Output.Markdown.Style = v.GetString("output.markdown.style")
	}
	if set["output.markdown.pager"] {
		cfg.Output.Markdown.Pager = v.GetString("output.markdown.pager")
	}
	if set["editor.command"] {
		cfg.Editor.Command = v.GetString("editor.command")
	}
	if set["note.status"] {
		cfg.Note.Status = v.GetString("note.status")
	}
	if set["note.kind"] {
		cfg.Note.Kind = v.GetString("note.kind")
	}
	if set["kb.sidecar.executable"] {
		cfg.KB.Sidecar.Executable = v.GetString("kb.sidecar.executable")
	}
	if set["kb.sidecar.timeout_seconds"] {
		cfg.KB.Sidecar.TimeoutSeconds = v.GetInt("kb.sidecar.timeout_seconds")
	}
	if set["search.limit"] {
		cfg.Search.Limit = v.GetInt("search.limit")
	}
	if set["search.allow_stale"] {
		cfg.Search.AllowStale = v.GetBool("search.allow_stale")
	}
	if set["storage.backend"] {
		cfg.Storage.Backend = v.GetString("storage.backend")
	}
	if set["storage.bucket"] {
		cfg.Storage.Bucket = v.GetString("storage.bucket")
	}
	if set["storage.region"] {
		cfg.Storage.Region = v.GetString("storage.region")
	}
	if set["storage.prefix"] {
		cfg.Storage.Prefix = v.GetString("storage.prefix")
	}
	if set["storage.endpoint"] {
		cfg.Storage.Endpoint = v.GetString("storage.endpoint")
	}
	if set["storage.profile"] {
		cfg.Storage.Profile = v.GetString("storage.profile")
	}
	if set["storage.token"] {
		cfg.Storage.Token = v.GetString("storage.token")
	}
	if set["themes.custom"] {
		cfg.Themes.Custom = v.GetStringMapString("themes.custom")
	}
	return cfg
}

func configKeys() []string {
	return []string{
		"vault",
		"remote.api_url",
		"output.color",
		"output.theme",
		"output.width",
		"output.markdown.enabled",
		"output.markdown.style",
		"output.markdown.pager",
		"editor.command",
		"note.status",
		"note.kind",
		"kb.sidecar.executable",
		"kb.sidecar.timeout_seconds",
		"search.limit",
		"search.allow_stale",
		"storage.backend",
		"storage.bucket",
		"storage.region",
		"storage.prefix",
		"storage.endpoint",
		"storage.profile",
		"storage.token",
		"themes.custom",
	}
}

func mergeConfig(dst *Config, src Config, isSet func(string) bool) {
	if isSet("vault") && src.Vault != "" {
		dst.Vault = src.Vault
	}
	if isSet("remote.api_url") {
		dst.Remote.APIURL = src.Remote.APIURL
	}
	mergeOutput(&dst.Output, src.Output, isSet)
	if isSet("editor.command") {
		dst.Editor.Command = src.Editor.Command
	}
	if isSet("note.status") {
		dst.Note.Status = src.Note.Status
	}
	if isSet("note.kind") {
		dst.Note.Kind = src.Note.Kind
	}
	if isSet("kb.sidecar.executable") {
		dst.KB.Sidecar.Executable = src.KB.Sidecar.Executable
	}
	if isSet("kb.sidecar.timeout_seconds") {
		dst.KB.Sidecar.TimeoutSeconds = src.KB.Sidecar.TimeoutSeconds
	}
	if isSet("search.limit") {
		dst.Search.Limit = src.Search.Limit
	}
	if isSet("search.allow_stale") {
		dst.Search.AllowStale = src.Search.AllowStale
	}
	mergeStorage(&dst.Storage, src.Storage, isSet)
	if isSet("themes.custom") {
		dst.Themes.Custom = copyStringMap(src.Themes.Custom)
	}
}

func mergeOutput(dst *OutputConfig, src OutputConfig, isSet func(string) bool) {
	if isSet("output.color") {
		dst.Color = src.Color
	}
	if isSet("output.theme") {
		dst.Theme = src.Theme
	}
	if isSet("output.width") {
		dst.Width = src.Width
	}
	if isSet("output.markdown.style") {
		dst.Markdown.Style = src.Markdown.Style
	}
	if isSet("output.markdown.pager") {
		dst.Markdown.Pager = src.Markdown.Pager
	}
	if isSet("output.markdown.enabled") {
		dst.Markdown.Enabled = src.Markdown.Enabled
	}
}

func mergeStorage(dst *StorageConfig, src StorageConfig, isSet func(string) bool) {
	if isSet("storage.backend") {
		dst.Backend = src.Backend
	}
	if isSet("storage.bucket") {
		dst.Bucket = src.Bucket
	}
	if isSet("storage.region") {
		dst.Region = src.Region
	}
	if isSet("storage.prefix") {
		dst.Prefix = src.Prefix
	}
	if isSet("storage.endpoint") {
		dst.Endpoint = src.Endpoint
	}
	if isSet("storage.profile") {
		dst.Profile = src.Profile
	}
	if isSet("storage.token") {
		dst.Token = src.Token
	}
}

func applyEnv(cfg *Config, sources *SourceSet, env func(string) (string, bool)) {
	apply := func(key string, fn func(string)) {
		if value, ok := env(key); ok && strings.TrimSpace(value) != "" {
			fn(value)
			sources.EnvKeys = append(sources.EnvKeys, key)
		}
	}
	apply("PINAX_VAULT", func(v string) { cfg.Vault = v })
	apply("PINAX_API_URL", func(v string) { cfg.Remote.APIURL = v })
	apply("PINAX_OUTPUT_COLOR", func(v string) { cfg.Output.Color = v })
	apply("PINAX_OUTPUT_THEME", func(v string) { cfg.Output.Theme = v })
	apply("PINAX_OUTPUT_WIDTH", func(v string) {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Output.Width = n
		}
	})
	apply("PINAX_OUTPUT_MARKDOWN_ENABLED", func(v string) { cfg.Output.Markdown.Enabled = parseBool(v) })
	apply("PINAX_OUTPUT_MARKDOWN_STYLE", func(v string) { cfg.Output.Markdown.Style = v })
	apply("PINAX_EDITOR_COMMAND", func(v string) { cfg.Editor.Command = v })
	apply("PINAX_KB_SIDECAR", func(v string) { cfg.KB.Sidecar.Executable = v })
	apply("PINAX_KB_SIDECAR_TIMEOUT", func(v string) {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.KB.Sidecar.TimeoutSeconds = n
		}
	})
	apply("PINAX_SEARCH_LIMIT", func(v string) {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Search.Limit = n
		}
	})
	apply("PINAX_SEARCH_ALLOW_STALE", func(v string) { cfg.Search.AllowStale = parseBool(v) })
	apply("NO_COLOR", func(v string) { cfg.Output.Color = "never" })
	if cfg.Editor.Command == "" {
		apply("EDITOR", func(v string) { cfg.Editor.Command = v })
	}
}

func applyExplicitFlags(cfg *Config, sources *SourceSet, flags map[string]string) {
	for key, value := range flags {
		if strings.TrimSpace(value) == "" {
			continue
		}
		sources.FlagKeys = append(sources.FlagKeys, key)
		switch key {
		case "vault":
			cfg.Vault = value
		case "remote.api_url":
			cfg.Remote.APIURL = value
		case "output.color":
			cfg.Output.Color = value
		case "output.theme":
			cfg.Output.Theme = value
		case "output.width":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.Output.Width = n
			}
		case "output.markdown.enabled":
			cfg.Output.Markdown.Enabled = parseBool(value)
		case "output.markdown.style":
			cfg.Output.Markdown.Style = value
		case "editor.command":
			cfg.Editor.Command = value
		case "kb.sidecar.executable":
			cfg.KB.Sidecar.Executable = value
		case "kb.sidecar.timeout_seconds":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.KB.Sidecar.TimeoutSeconds = n
			}
		case "search.limit":
			if n, err := strconv.Atoi(value); err == nil {
				cfg.Search.Limit = n
			}
		case "search.allow_stale":
			cfg.Search.AllowStale = parseBool(value)
		}
	}
}

func (cfg Config) Validate() error {
	if !oneOf(cfg.Output.Color, "auto", "always", "never") {
		return configInvalid("output.color", cfg.Output.Color)
	}
	if !oneOf(cfg.Output.Theme, "pinax", "mono", "high-contrast", "custom") {
		return configInvalid("output.theme", cfg.Output.Theme)
	}
	if cfg.Output.Width < 20 || cfg.Output.Width > 300 {
		return configInvalid("output.width", fmt.Sprint(cfg.Output.Width))
	}
	if !oneOf(cfg.Output.Markdown.Style, "auto", "ascii", "dark", "light", "notty") {
		return configInvalid("output.markdown.style", cfg.Output.Markdown.Style)
	}
	if !oneOf(cfg.Output.Markdown.Pager, "", "never", "auto", "always") {
		return configInvalid("output.markdown.pager", cfg.Output.Markdown.Pager)
	}
	if cfg.Remote.APIURL != "" {
		parsed, err := url.Parse(cfg.Remote.APIURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || !oneOf(parsed.Scheme, "http", "https") {
			return configInvalid("remote.api_url", cfg.Remote.APIURL)
		}
	}
	if cfg.Search.Limit < 0 {
		return configInvalid("search.limit", fmt.Sprint(cfg.Search.Limit))
	}
	if strings.TrimSpace(cfg.KB.Sidecar.Executable) == "" {
		return configInvalid("kb.sidecar.executable", cfg.KB.Sidecar.Executable)
	}
	if cfg.KB.Sidecar.TimeoutSeconds < 1 || cfg.KB.Sidecar.TimeoutSeconds > 600 {
		return configInvalid("kb.sidecar.timeout_seconds", fmt.Sprint(cfg.KB.Sidecar.TimeoutSeconds))
	}
	if !oneOf(cfg.Storage.Backend, "", "local", "s3", "rclone") {
		return configInvalid("storage.backend", cfg.Storage.Backend)
	}
	if cfg.Storage.Backend == "s3" {
		if strings.TrimSpace(cfg.Storage.Bucket) == "" {
			return configInvalid("storage.bucket", "")
		}
		if strings.TrimSpace(cfg.Storage.Region) == "" {
			return configInvalid("storage.region", "")
		}
	}
	if cfg.Storage.Token != "" {
		return &Error{Code: "config_secret_rejected", Message: "配置包含 secret-like 字段 storage.token"}
	}
	for role, color := range cfg.Themes.Custom {
		if !validThemeRole(role) {
			return configInvalid("themes.custom."+role, role)
		}
		if !validColorValue(color) {
			return configInvalid("themes.custom."+role, color)
		}
	}
	return nil
}

func configInvalid(key, value string) error {
	return &Error{Code: "config_invalid", Message: key + " 不合法: " + value}
}
func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func rejectSecretLikeSettings(value any, path []string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := append(path, key)
			if secretLikeKey(key) {
				return &Error{Code: "config_secret_rejected", Message: "配置包含 secret-like 字段 " + strings.Join(childPath, ".")}
			}
			if err := rejectSecretLikeSettings(child, childPath); err != nil {
				return err
			}
		}
	case string:
		if secretLikeValue(typed) {
			return &Error{Code: "config_secret_rejected", Message: "配置包含疑似 secret 的值 " + strings.Join(path, ".")}
		}
	case []any:
		for _, child := range typed {
			if err := rejectSecretLikeSettings(child, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func secretLikeKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"token", "secret", "password", "cookie", "authorization", "webhook"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func secretLikeValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	markers := []string{"bearer ", "authorization:", "token=", "secret=", "password=", "cookie=", "/webhook/", "webhook_url"}
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func validThemeRole(role string) bool {
	return oneOf(role, "accent", "muted", "rule", "success", "warning", "danger", "key", "value", "path", "link", "code", "heading")
}

func validColorValue(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if len(value) == 4 || len(value) == 7 {
		if value[0] == '#' {
			for _, r := range value[1:] {
				if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
					return false
				}
			}
			return true
		}
	}
	if _, err := strconv.Atoi(value); err == nil {
		return true
	}
	return oneOf(value,
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white",
	)
}

func Value(cfg Config, key string) (string, bool) {
	switch key {
	case "vault":
		return cfg.Vault, true
	case "remote.api_url":
		return cfg.Remote.APIURL, true
	case "output.color":
		return cfg.Output.Color, true
	case "output.theme":
		return cfg.Output.Theme, true
	case "output.width":
		return strconv.Itoa(cfg.Output.Width), true
	case "output.markdown.enabled":
		return strconv.FormatBool(cfg.Output.Markdown.Enabled), true
	case "output.markdown.style":
		return cfg.Output.Markdown.Style, true
	case "output.markdown.pager":
		return cfg.Output.Markdown.Pager, true
	case "editor.command":
		return cfg.Editor.Command, true
	case "note.status":
		return cfg.Note.Status, true
	case "note.kind":
		return cfg.Note.Kind, true
	case "search.limit":
		return strconv.Itoa(cfg.Search.Limit), true
	case "search.allow_stale":
		return strconv.FormatBool(cfg.Search.AllowStale), true
	case "storage.backend":
		return cfg.Storage.Backend, true
	case "storage.bucket":
		return cfg.Storage.Bucket, true
	case "storage.region":
		return cfg.Storage.Region, true
	case "storage.prefix":
		return cfg.Storage.Prefix, true
	case "storage.endpoint":
		return cfg.Storage.Endpoint, true
	case "storage.profile":
		return cfg.Storage.Profile, true
	default:
		if role, ok := strings.CutPrefix(key, "themes.custom."); ok {
			value, exists := cfg.Themes.Custom[role]
			return value, exists
		}
		return "", false
	}
}

func SetValue(path, key, value string) error {
	if secretLikeKey(key) || secretLikeValue(value) {
		return &Error{Code: "config_secret_rejected", Message: "配置 key/value 疑似包含 secret"}
	}
	data, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	parsed, err := parseConfigValue(key, value)
	if err != nil {
		return err
	}
	setNestedValue(data, strings.Split(key, "."), parsed)
	return writeYAMLMap(path, data)
}

func UnsetValue(path, key string) error {
	data, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	unsetNestedValue(data, strings.Split(key, "."))
	return writeYAMLMap(path, data)
}

func readYAMLMap(path string) (map[string]any, error) {
	data := map[string]any{}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return data, nil
	}
	if err := yaml.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func writeYAMLMap(path string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func parseConfigValue(key, value string) (any, error) {
	switch key {
	case "output.width", "search.limit":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, configInvalid(key, value)
		}
		return parsed, nil
	case "output.markdown.enabled", "search.allow_stale":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true, nil
		case "0", "false", "no", "off":
			return false, nil
		default:
			return nil, configInvalid(key, value)
		}
	default:
		if _, ok := Value(DefaultConfig(), key); ok || strings.HasPrefix(key, "themes.custom.") {
			return value, nil
		}
		return nil, configInvalid(key, value)
	}
}

func setNestedValue(data map[string]any, parts []string, value any) {
	if len(parts) == 1 {
		data[parts[0]] = value
		return
	}
	child, _ := data[parts[0]].(map[string]any)
	if child == nil {
		child = map[string]any{}
		data[parts[0]] = child
	}
	setNestedValue(child, parts[1:], value)
}

func unsetNestedValue(data map[string]any, parts []string) bool {
	if len(parts) == 1 {
		delete(data, parts[0])
		return len(data) == 0
	}
	child, _ := data[parts[0]].(map[string]any)
	if child == nil {
		return false
	}
	if unsetNestedValue(child, parts[1:]) {
		delete(data, parts[0])
	}
	return len(data) == 0
}
