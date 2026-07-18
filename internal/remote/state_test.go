package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCloudStateLifecycle(t *testing.T) {
	root := t.TempDir()
	state, err := Login(root, LoginRequest{Endpoint: "https://cloud.example.test", WorkspaceID: "ws_123", DeviceID: "dev_laptop", SecretRef: "op://pinax/cloud-token", EncryptionSecretRef: "env://PINAX_SYNC_SECRET"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if state.Config.Endpoint != "https://cloud.example.test" || state.Config.WorkspaceID != "ws_123" || state.Session.DeviceID != "dev_laptop" {
		t.Fatalf("state = %#v", state)
	}
	if state.Config.SecretRef != "op://pinax/cloud-token" {
		t.Fatalf("secret ref not persisted as reference: %#v", state.Config)
	}
	if state.Config.EncryptionSecretRef != "env://PINAX_SYNC_SECRET" || EncryptionSecretRef(state.Config) != "env://PINAX_SYNC_SECRET" {
		t.Fatalf("encryption secret ref not persisted as reference: %#v", state.Config)
	}
	legacy := state.Config
	legacy.EncryptionSecretRef = ""
	if EncryptionSecretRef(legacy) != "op://pinax/cloud-token" {
		t.Fatalf("legacy encryption secret fallback = %q", EncryptionSecretRef(legacy))
	}
	asset := readYAMLCloudAsset(t, filepath.Join(root, ".pinax", "cloud", "config.yaml"))
	if strings.Contains(asset, "raw-token") || strings.Contains(asset, "Authorization") {
		t.Fatalf("cloud asset leaked raw secret material:\n%s", asset)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Session.Status != "active" || loaded.Session.SessionID == "" {
		t.Fatalf("loaded session = %#v", loaded.Session)
	}
	if err := Logout(root); err != nil {
		t.Fatalf("logout: %v", err)
	}
	loggedOut, err := Load(root)
	if err != nil {
		t.Fatalf("load after logout: %v", err)
	}
	if loggedOut.Session.Status != "logged_out" {
		t.Fatalf("logout status = %#v", loggedOut.Session)
	}
}

func TestCloudStateWritesStructuredYAMLS3Config(t *testing.T) {
	root := t.TempDir()
	state, err := Login(root, LoginRequest{
		Endpoint:    "s3://notes/pinax-sync?endpoint=http%3A%2F%2F10.10.1.102%3A9010&path_style=true&profile=ec&region=us-east-1",
		WorkspaceID: "ec",
		DeviceID:    "dev",
		SecretRef:   "profile://ec",
		BackendKind: "s3-direct",
		S3: &S3Config{
			Bucket:    "notes",
			Prefix:    "pinax-sync/",
			Endpoint:  "http://10.10.1.102:9010",
			Region:    "us-east-1",
			Profile:   "ec",
			PathStyle: true,
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if state.Config.Endpoint == "" || state.Config.S3 == nil || state.Config.S3.Endpoint != "http://10.10.1.102:9010" {
		t.Fatalf("structured s3 config not kept in memory: %#v", state.Config)
	}
	asset := readYAMLCloudAsset(t, filepath.Join(root, ".pinax", "cloud", "config.yaml"))
	for _, want := range []string{"backend_kind: s3-direct", "bucket: notes", "prefix: pinax-sync/", "endpoint: http://10.10.1.102:9010", "profile: ec", "path_style: true", "secret_ref: profile://ec"} {
		if !strings.Contains(asset, want) {
			t.Fatalf("yaml config missing %q:\n%s", want, asset)
		}
	}
	for _, escaped := range []string{"http%3A", "?endpoint=", "&profile="} {
		if strings.Contains(asset, escaped) {
			t.Fatalf("yaml config contains escaped endpoint fragment %q:\n%s", escaped, asset)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "cloud", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("s3 login should not write primary json config, err=%v", err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load yaml: %v", err)
	}
	if loaded.Config.Endpoint != state.Config.Endpoint || loaded.Config.S3 == nil || loaded.Config.S3.Profile != "ec" {
		t.Fatalf("loaded yaml config = %#v", loaded.Config)
	}
}

func TestCloudStateWritesVirtualHostedS3Config(t *testing.T) {
	root := t.TempDir()
	state, err := Login(root, LoginRequest{
		WorkspaceID: "yeisme-notes",
		DeviceID:    "windows-pc",
		SecretRef:   "profile://tencent-cos-pinax",
		BackendKind: "s3-direct",
		S3: &S3Config{
			Bucket:          "pinax-note-1322128555",
			Prefix:          "pinax-sync/",
			Endpoint:        "https://cos.ap-guangzhou.myqcloud.com",
			Region:          "ap-guangzhou",
			Profile:         "tencent-cos-pinax",
			AddressingStyle: "virtual-hosted",
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if state.Config.S3 == nil || state.Config.S3.PathStyle || state.Config.S3.AddressingStyle != "virtual-hosted" {
		t.Fatalf("s3 config = %#v", state.Config.S3)
	}
	if strings.Contains(state.Config.Endpoint, "path_style=true") || !strings.Contains(state.Config.Endpoint, "addressing_style=virtual-hosted") {
		t.Fatalf("endpoint did not preserve virtual-hosted style: %s", state.Config.Endpoint)
	}
	asset := readYAMLCloudAsset(t, filepath.Join(root, ".pinax", "cloud", "config.yaml"))
	for _, want := range []string{"endpoint: https://cos.ap-guangzhou.myqcloud.com", "addressing_style: virtual-hosted", "profile: tencent-cos-pinax"} {
		if !strings.Contains(asset, want) {
			t.Fatalf("yaml config missing %q:\n%s", want, asset)
		}
	}
	if strings.Contains(asset, "path_style: true") {
		t.Fatalf("virtual-hosted config should not force path_style:\n%s", asset)
	}
}

func TestCloudStateLoadsLegacyJSONConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "cloud"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := `{
  "schema_version": "pinax.cloud.config.v1",
  "backend_kind": "s3-direct",
  "endpoint": "s3://notes/pinax-sync?endpoint=http%3A%2F%2F10.10.1.102%3A9010&path_style=true&profile=ec&region=us-east-1",
  "workspace_id": "ec",
  "device_id": "dev",
  "secret_ref": "profile://ec",
  "created_at": "2026-06-11T00:00:00Z",
  "updated_at": "2026-06-11T00:00:00Z"
}
`
	if err := os.WriteFile(filepath.Join(root, ".pinax", "cloud", "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("load legacy json: %v", err)
	}
	if loaded.Config.Endpoint == "" || loaded.Config.S3 == nil || loaded.Config.S3.Endpoint != "http://10.10.1.102:9010" || loaded.Config.S3.Profile != "ec" {
		t.Fatalf("legacy json was not normalized to structured s3 config: %#v", loaded.Config)
	}
}

func TestCloudStateMissingConfig(t *testing.T) {
	root := t.TempDir()
	if _, err := Load(root); err == nil || !IsNotConfigured(err) {
		t.Fatalf("load without config err = %v", err)
	}
	result := Doctor(root)
	if result.Configured || result.Status != "failed" || result.Code != "cloud_not_configured" {
		t.Fatalf("doctor missing config = %#v", result)
	}
}

func TestCloudStateRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	cases := []LoginRequest{
		{WorkspaceID: "ws", DeviceID: "dev", SecretRef: "ref"},
		{Endpoint: "https://cloud.example.test", DeviceID: "dev", SecretRef: "ref"},
		{Endpoint: "https://cloud.example.test", WorkspaceID: "ws", SecretRef: "ref"},
	}
	for _, tc := range cases {
		if _, err := Login(root, tc); err == nil {
			t.Fatalf("Login(%#v) succeeded", tc)
		}
	}
}

func readYAMLCloudAsset(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatalf("asset yaml invalid: %v\n%s", err, b)
	}
	return string(b)
}
