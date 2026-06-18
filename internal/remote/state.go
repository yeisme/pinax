package remote

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ConfigSchemaVersion  = "pinax.cloud.config.v1"
	SessionSchemaVersion = "pinax.cloud.session.v1"
)

var ErrNotConfigured = errors.New("cloud not configured")

type LoginRequest struct {
	Endpoint            string
	WorkspaceID         string
	DeviceID            string
	SecretRef           string
	EncryptionSecretRef string
	BackendKind         string
	S3                  *S3Config
	Now                 time.Time
}

type Config struct {
	SchemaVersion       string    `json:"schema_version" yaml:"schema_version"`
	BackendKind         string    `json:"backend_kind,omitempty" yaml:"backend_kind,omitempty"`
	Endpoint            string    `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	WorkspaceID         string    `json:"workspace_id" yaml:"workspace_id"`
	DeviceID            string    `json:"device_id" yaml:"device_id"`
	SecretRef           string    `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
	EncryptionSecretRef string    `json:"encryption_secret_ref,omitempty" yaml:"encryption_secret_ref,omitempty"`
	S3                  *S3Config `json:"s3,omitempty" yaml:"s3,omitempty"`
	CreatedAt           string    `json:"created_at" yaml:"created_at"`
	UpdatedAt           string    `json:"updated_at" yaml:"updated_at"`
}

type S3Config struct {
	Bucket    string `json:"bucket" yaml:"bucket"`
	Prefix    string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Endpoint  string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Region    string `json:"region,omitempty" yaml:"region,omitempty"`
	Profile   string `json:"profile,omitempty" yaml:"profile,omitempty"`
	PathStyle bool   `json:"path_style,omitempty" yaml:"path_style,omitempty"`
}

type DeviceSession struct {
	SchemaVersion string `json:"schema_version"`
	SessionID     string `json:"session_id"`
	DeviceID      string `json:"device_id"`
	Status        string `json:"status"`
	IssuedAt      string `json:"issued_at"`
	UpdatedAt     string `json:"updated_at"`
}

type State struct {
	Config  Config        `json:"config"`
	Session DeviceSession `json:"session"`
}

type DoctorResult struct {
	Configured   bool   `json:"configured"`
	Status       string `json:"status"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"message"`
	BackendKind  string `json:"backend_kind,omitempty"`
	AuthBoundary string `json:"auth_boundary,omitempty"`
	ServerAudit  bool   `json:"server_audit"`
	Endpoint     string `json:"endpoint,omitempty"`
	Workspace    string `json:"workspace_id,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
}

func Login(root string, req LoginRequest) (State, error) {
	root, err := cleanRoot(root)
	if err != nil {
		return State{}, err
	}
	endpoint := strings.TrimRight(strings.TrimSpace(req.Endpoint), "/")
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	deviceID := strings.TrimSpace(req.DeviceID)
	secretRef := strings.TrimSpace(req.SecretRef)
	encryptionSecretRef := strings.TrimSpace(req.EncryptionSecretRef)
	s3Config := normalizeS3Config(req.S3)
	if endpoint == "" && s3Config != nil {
		endpoint = endpointFromS3Config(*s3Config)
	}
	if endpoint == "" || workspaceID == "" || deviceID == "" {
		return State{}, fmt.Errorf("cloud endpoint, workspace and device are required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return State{}, fmt.Errorf("invalid endpoint URI: %w", err)
	}
	if u.Scheme == "" {
		return State{}, fmt.Errorf("endpoint URI must specify a scheme: %s", endpoint)
	}
	if !IsSupportedScheme(u.Scheme) {
		return State{}, fmt.Errorf("unsupported remote scheme: %s", u.Scheme)
	}
	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	existing, _ := Load(root)
	createdAt := now.Format(time.RFC3339)
	if existing.Config.CreatedAt != "" {
		createdAt = existing.Config.CreatedAt
	}
	backendKind := strings.TrimSpace(req.BackendKind)
	if backendKind == "" {
		backendKind = backendKindForEndpoint(endpoint)
	}
	config := Config{SchemaVersion: ConfigSchemaVersion, BackendKind: backendKind, Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: deviceID, SecretRef: secretRef, EncryptionSecretRef: encryptionSecretRef, S3: s3Config, CreatedAt: createdAt, UpdatedAt: now.Format(time.RFC3339)}
	config = normalizeConfig(config)
	session := DeviceSession{SchemaVersion: SessionSchemaVersion, SessionID: sessionID(root, workspaceID, deviceID, now), DeviceID: deviceID, Status: "active", IssuedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}
	if err := writeYAML(configPath(root), configForDisk(config), 0o600); err != nil {
		return State{}, err
	}
	if err := os.Remove(legacyConfigPath(root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	if err := writeJSON(sessionPath(root), session, 0o600); err != nil {
		return State{}, err
	}
	return State{Config: config, Session: session}, nil
}

func Load(root string) (State, error) {
	root, err := cleanRoot(root)
	if err != nil {
		return State{}, err
	}
	var config Config
	if err := readYAML(configPath(root), &config); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := readJSON(legacyConfigPath(root), &config); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return State{}, ErrNotConfigured
				}
				return State{}, err
			}
		} else {
			return State{}, err
		}
	}
	config = normalizeConfig(config)
	session := DeviceSession{SchemaVersion: SessionSchemaVersion, DeviceID: config.DeviceID, Status: "not_logged_in"}
	if err := readJSON(sessionPath(root), &session); err != nil && !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	return State{Config: config, Session: session}, nil
}

func Logout(root string) error {
	state, err := Load(root)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state.Session.Status = "logged_out"
	state.Session.UpdatedAt = now
	if state.Session.SchemaVersion == "" {
		state.Session.SchemaVersion = SessionSchemaVersion
	}
	if state.Session.DeviceID == "" {
		state.Session.DeviceID = state.Config.DeviceID
	}
	return writeJSON(sessionPath(root), state.Session, 0o600)
}

func (s State) GetStore(ctx context.Context) (BlobStore, error) {
	return NewStore(ctx, s.Config.Endpoint)
}

func Doctor(root string) DoctorResult {
	state, err := Load(root)
	if err != nil {
		if IsNotConfigured(err) {
			return DoctorResult{Configured: false, Status: "failed", Code: "cloud_not_configured", Message: "cloud backend 尚未配置"}
		}
		return DoctorResult{Configured: false, Status: "failed", Code: "cloud_state_invalid", Message: err.Error()}
	}
	backendKind := resolvedBackendKind(state.Config)
	u, err := url.Parse(state.Config.Endpoint)
	authBoundary, serverAudit := backendBoundaryFacts(backendKind)
	if err != nil {
		return DoctorResult{Configured: true, Status: "failed", Code: "invalid_endpoint", Message: fmt.Sprintf("invalid endpoint URI: %v", err), BackendKind: backendKind, AuthBoundary: authBoundary, ServerAudit: serverAudit, Endpoint: state.Config.Endpoint, Workspace: state.Config.WorkspaceID, DeviceID: state.Config.DeviceID}
	}
	if !IsSupportedScheme(u.Scheme) {
		return DoctorResult{Configured: true, Status: "failed", Code: "unsupported_scheme", Message: fmt.Sprintf("unsupported remote scheme: %s", u.Scheme), BackendKind: backendKind, AuthBoundary: authBoundary, ServerAudit: serverAudit, Endpoint: state.Config.Endpoint, Workspace: state.Config.WorkspaceID, DeviceID: state.Config.DeviceID}
	}
	return DoctorResult{Configured: true, Status: "success", Message: "cloud state 可读取", BackendKind: backendKind, AuthBoundary: authBoundary, ServerAudit: serverAudit, Endpoint: state.Config.Endpoint, Workspace: state.Config.WorkspaceID, DeviceID: state.Config.DeviceID}
}

func IsNotConfigured(err error) bool {
	return errors.Is(err, ErrNotConfigured)
}

func RedactedData(state State) map[string]any {
	secretConfigured := strings.TrimSpace(state.Config.SecretRef) != ""
	encryptionSecretConfigured := strings.TrimSpace(EncryptionSecretRef(state.Config)) != ""
	return map[string]any{
		"config": map[string]any{
			"schema_version":                   state.Config.SchemaVersion,
			"backend_kind":                     resolvedBackendKind(state.Config),
			"endpoint":                         state.Config.Endpoint,
			"workspace_id":                     state.Config.WorkspaceID,
			"device_id":                        state.Config.DeviceID,
			"secret_ref_configured":            secretConfigured,
			"encryption_secret_ref_configured": encryptionSecretConfigured,
			"provider_ref_configured":          secretConfigured,
			"s3":                               state.Config.S3,
			"created_at":                       state.Config.CreatedAt,
			"updated_at":                       state.Config.UpdatedAt,
		},
		"session": state.Session,
	}
}

func resolvedBackendKind(cfg Config) string {
	if strings.TrimSpace(cfg.BackendKind) != "" {
		return cfg.BackendKind
	}
	return backendKindForEndpoint(cfg.Endpoint)
}

func backendBoundaryFacts(kind string) (string, bool) {
	switch kind {
	case "server":
		return "pinax_cloud_server", true
	case "s3-direct", "rclone-direct", "embedded":
		return "provider_credentials", false
	default:
		return "provider_credentials", false
	}
}

func backendKindForEndpoint(endpoint string) string {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "unknown"
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return "server"
	case "s3":
		return "s3-direct"
	case "rclone":
		return "rclone-direct"
	case "file":
		return "embedded"
	default:
		return strings.ToLower(u.Scheme)
	}
}

func cleanRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Abs(root)
}

func configPath(root string) string {
	return filepath.Join(root, ".pinax", "cloud", "config.yaml")
}

func legacyConfigPath(root string) string {
	return filepath.Join(root, ".pinax", "cloud", "config.json")
}

func sessionPath(root string) string {
	return filepath.Join(root, ".pinax", "cloud", "session.json")
}

func normalizeConfig(config Config) Config {
	if config.SchemaVersion == "" {
		config.SchemaVersion = ConfigSchemaVersion
	}
	config.Endpoint = strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	config.WorkspaceID = strings.TrimSpace(config.WorkspaceID)
	config.DeviceID = strings.TrimSpace(config.DeviceID)
	config.SecretRef = strings.TrimSpace(config.SecretRef)
	config.EncryptionSecretRef = strings.TrimSpace(config.EncryptionSecretRef)
	config.BackendKind = strings.TrimSpace(config.BackendKind)
	config.S3 = normalizeS3Config(config.S3)
	if config.S3 == nil && backendKindForEndpoint(config.Endpoint) == "s3-direct" {
		config.S3 = s3ConfigFromEndpoint(config.Endpoint)
	}
	if config.Endpoint == "" && config.S3 != nil {
		config.Endpoint = endpointFromS3Config(*config.S3)
	}
	if config.BackendKind == "" {
		config.BackendKind = backendKindForEndpoint(config.Endpoint)
	}
	return config
}

func EncryptionSecretRef(config Config) string {
	if ref := strings.TrimSpace(config.EncryptionSecretRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(config.SecretRef)
}

func configForDisk(config Config) Config {
	config = normalizeConfig(config)
	if resolvedBackendKind(config) == "s3-direct" && config.S3 != nil {
		config.Endpoint = ""
	}
	return config
}

func normalizeS3Config(config *S3Config) *S3Config {
	if config == nil {
		return nil
	}
	normalized := *config
	normalized.Bucket = strings.TrimSpace(normalized.Bucket)
	normalized.Prefix = strings.Trim(strings.TrimSpace(normalized.Prefix), "/")
	if normalized.Prefix != "" {
		normalized.Prefix += "/"
	}
	normalized.Endpoint = strings.TrimRight(strings.TrimSpace(normalized.Endpoint), "/")
	normalized.Region = strings.TrimSpace(normalized.Region)
	normalized.Profile = strings.TrimSpace(normalized.Profile)
	if normalized.Bucket == "" {
		return nil
	}
	if normalized.Endpoint != "" {
		normalized.PathStyle = true
	}
	return &normalized
}

func endpointFromS3Config(config S3Config) string {
	endpoint := "s3://" + strings.TrimSpace(config.Bucket)
	if prefix := strings.Trim(strings.TrimSpace(config.Prefix), "/"); prefix != "" {
		endpoint += "/" + prefix
	}
	values := url.Values{}
	if endpointURL := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/"); endpointURL != "" {
		values.Set("endpoint", endpointURL)
		values.Set("path_style", "true")
	} else if config.PathStyle {
		values.Set("path_style", "true")
	}
	if region := strings.TrimSpace(config.Region); region != "" {
		values.Set("region", region)
	}
	if profile := strings.TrimSpace(config.Profile); profile != "" {
		values.Set("profile", profile)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}

func s3ConfigFromEndpoint(endpoint string) *S3Config {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || strings.ToLower(u.Scheme) != "s3" || strings.TrimSpace(u.Host) == "" {
		return nil
	}
	q := u.Query()
	return normalizeS3Config(&S3Config{
		Bucket:    u.Host,
		Prefix:    strings.TrimPrefix(u.Path, "/"),
		Endpoint:  strings.TrimSpace(q.Get("endpoint")),
		Region:    strings.TrimSpace(q.Get("region")),
		Profile:   strings.TrimSpace(q.Get("profile")),
		PathStyle: strings.EqualFold(q.Get("path_style"), "true") || strings.EqualFold(q.Get("path"), "auto") || strings.EqualFold(q.Get("path"), "on"),
	})
}

func readYAML(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, value)
}

func readJSON(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func writeYAML(path string, value any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, perm)
}

func writeJSON(path string, value any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), perm)
}

func sessionID(root, workspaceID, deviceID string, now time.Time) string {
	h := sha1.Sum([]byte(root + "\x00" + workspaceID + "\x00" + deviceID + "\x00" + now.Format(time.RFC3339Nano)))
	return "cloud_sess_" + hex.EncodeToString(h[:])[:20]
}
