package profile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile represents a named backend connection configuration.
type Profile struct {
	Endpoint     string `yaml:"endpoint" json:"endpoint"`
	Workspace    string `yaml:"workspace" json:"workspace,omitempty"`
	Device       string `yaml:"device" json:"device,omitempty"`
	SecretRef    string `yaml:"secret_ref,omitempty" json:"secret_ref,omitempty"`
	DefaultScope string `yaml:"default_scope,omitempty" json:"default_scope,omitempty"`
}

// ProfilesConfig holds all profiles and defaults.
type ProfilesConfig struct {
	Profiles map[string]Profile `yaml:"profiles"`
	Defaults struct {
		Profile      string `yaml:"profile,omitempty"`
		WriteProfile string `yaml:"write_profile,omitempty"`
	} `yaml:"defaults,omitempty"`
}

// ConfigDir returns the directory for pinax config files.
func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "pinax")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".pinax")
	}
	return filepath.Join(home, ".config", "pinax")
}

// ProfilesPath returns the path to profiles.yaml.
func ProfilesPath() string {
	return filepath.Join(ConfigDir(), "profiles.yaml")
}

// Load reads profiles from disk.
func Load() (*ProfilesConfig, error) {
	path := ProfilesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProfilesConfig{Profiles: make(map[string]Profile)}, nil
		}
		return nil, fmt.Errorf("read profiles file: %w", err)
	}
	var cfg ProfilesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse profiles file: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	return &cfg, nil
}

// Save writes profiles to disk.
func Save(cfg *ProfilesConfig) error {
	path := ProfilesPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal profiles: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write profiles file: %w", err)
	}
	return nil
}

// ResolveSecretRef resolves a secret reference to the actual secret value.
func ResolveSecretRef(ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	switch {
	case strings.HasPrefix(ref, "env://"):
		varName := strings.TrimPrefix(ref, "env://")
		val := os.Getenv(varName)
		if val == "" {
			return "", fmt.Errorf("environment variable %s is not set", varName)
		}
		return val, nil
	case strings.HasPrefix(ref, "keychain://"):
		account := strings.TrimPrefix(ref, "keychain://")
		return resolveKeychain(account)
	case strings.HasPrefix(ref, "plain:"):
		return strings.TrimPrefix(ref, "plain:"), nil
	default:
		return ref, nil
	}
}

// resolveKeychain attempts to retrieve a secret from the system keychain.
func resolveKeychain(account string) (string, error) {
	service := "pinax"
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("security", "find-generic-password", "-s", service, "-a", account, "-w").Output()
		if err != nil {
			return "", fmt.Errorf("get secret from macOS Keychain: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if runtime.GOOS == "linux" {
		// Try secret-tool (libsecret)
		out, err := exec.Command("secret-tool", "lookup", "service", service, "account", account).Output()
		if err != nil {
			return "", fmt.Errorf("get secret from Linux secret-service: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("keychain is not supported on this operating system")
}

// ResolveTarget resolves a target string to backend connection parameters.
// If target matches a profile name, returns that profile's parameters.
// If target is a URI (contains ://), returns it as-is for endpoint.
func ResolveTarget(target string) (endpoint, workspace, device, secretRef string, err error) {
	if target == "" {
		return "", "", "", "", fmt.Errorf("target must not be empty")
	}
	// Check if it's a URI
	if strings.Contains(target, "://") {
		return target, "", "", "", nil
	}
	// Check known non-URI targets
	switch target {
	case "cloud", "git", "s3":
		return target, "", "", "", nil
	}
	// Try as profile name
	cfg, err := Load()
	if err != nil {
		return "", "", "", "", err
	}
	p, ok := cfg.Profiles[target]
	if !ok {
		// Pass through as-is
		return target, "", "", "", nil
	}
	return p.Endpoint, p.Workspace, p.Device, p.SecretRef, nil
}
