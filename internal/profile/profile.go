package profile

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
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

func SecretsPath() string {
	return filepath.Join(ConfigDir(), "secrets.yaml")
}

// EnsureStoredSecret returns a stable user-level secret reference, creating the
// secret once when it does not exist. The secret value never enters project or
// vault state.
func EnsureStoredSecret(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid stored secret name")
	}
	secrets, err := loadSecrets()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(secrets[name]) == "" {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", fmt.Errorf("generate stored secret: %w", err)
		}
		secrets[name] = base64.RawURLEncoding.EncodeToString(raw)
		if err := saveSecrets(secrets); err != nil {
			return "", err
		}
	}
	return "stored://" + name, nil
}

// SetStoredSecret stores a specific plaintext value under name in the user-level
// secret store and returns a stored:// reference. Used by the sync repo compiler
// to persist a freshly unlocked secret as a device-local reference so the
// plaintext never enters the generated runtime config. The value never enters
// project or vault state.
func SetStoredSecret(name, value string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid stored secret name")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("empty stored secret value")
	}
	secrets, err := loadSecrets()
	if err != nil {
		return "", err
	}
	secrets[name] = value
	if err := saveSecrets(secrets); err != nil {
		return "", err
	}
	return "stored://" + name, nil
}

func loadSecrets() (map[string]string, error) {
	data, err := os.ReadFile(SecretsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read secrets file: %w", err)
	}
	var cfg struct {
		Secrets map[string]string `yaml:"secrets"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse secrets file: %w", err)
	}
	if cfg.Secrets == nil {
		cfg.Secrets = map[string]string{}
	}
	return cfg.Secrets, nil
}

func saveSecrets(secrets map[string]string) error {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(struct {
		Secrets map[string]string `yaml:"secrets"`
	}{Secrets: secrets})
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}
	if err := os.WriteFile(SecretsPath(), data, 0o600); err != nil {
		return fmt.Errorf("write secrets file: %w", err)
	}
	return nil
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
	case strings.HasPrefix(ref, "stored://"):
		name := strings.TrimPrefix(ref, "stored://")
		secrets, err := loadSecrets()
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(secrets[name])
		if value == "" {
			return "", fmt.Errorf("stored secret %s is not configured", name)
		}
		return value, nil
	case strings.HasPrefix(ref, "plain:"):
		return strings.TrimPrefix(ref, "plain:"), nil
	case strings.HasPrefix(ref, "profile://"):
		profileName := strings.TrimPrefix(ref, "profile://")
		return resolveAWSProfileSecret(profileName)
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

// resolveAWSProfileSecret reads the AWS shared credentials INI file and returns
// the aws_secret_access_key for the named profile. No subprocess, no external
// INI dependency. The credentials path defaults to ~/.aws/credentials but can
// be overridden with the AWS_SHARED_CREDENTIALS_FILE environment variable.
func resolveAWSProfileSecret(profileName string) (string, error) {
	if strings.TrimSpace(profileName) == "" {
		return "", fmt.Errorf("profile name is required")
	}
	path := strings.TrimSpace(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for AWS credentials: %w", err)
		}
		path = filepath.Join(homeDir, ".aws", "credentials")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read AWS credentials file %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	inSection := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.TrimSpace(line[1:len(line)-1]) == profileName
			continue
		}
		if !inSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "aws_secret_access_key" {
			secret := strings.TrimSpace(value)
			if secret == "" {
				return "", fmt.Errorf("aws_secret_access_key is empty for profile %q in %s", profileName, path)
			}
			return secret, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan AWS credentials file %s: %w", path, err)
	}
	return "", fmt.Errorf("profile %q not found in %s", profileName, path)
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
