package research

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConnectorsConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	config := ExternalServiceConfig{SchemaVersion: ExternalServiceConfigSchemaVersion, Connectors: ConnectorsConfig{Endpoint: "https://connector.example.test", Capability: "daily_hot_notes"}}
	if err := SaveExternalServiceConfig(root, config); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "briefing", "research.json")); err != nil {
		t.Fatalf("research config missing: %v", err)
	}
	loaded, err := LoadExternalServiceConfig(root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Connectors.Endpoint != config.Connectors.Endpoint || loaded.Connectors.Capability != "daily_hot_notes" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestLoadExternalServiceConfigRejectsV1Schema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".pinax", "briefing", "research.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":"pinax.research.config.v1","connectors":{}}`), 0o600); err != nil {
		t.Fatalf("write v1 config: %v", err)
	}
	_, err := LoadExternalServiceConfig(root)
	if !errors.Is(err, ErrUnsupportedExternalServiceConfigSchema) {
		t.Fatalf("LoadExternalServiceConfig() error = %v, want unsupported schema", err)
	}
}

func TestLoadExternalServiceConfigRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".pinax", "briefing", "research.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":"pinax.research.config.v2","retired_service":{}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadExternalServiceConfig(root); err == nil {
		t.Fatal("LoadExternalServiceConfig() accepted an unknown retired field")
	}
}

func TestConnectorsResolverFallsBackWhenUnconfigured(t *testing.T) {
	root := t.TempDir()
	adapter, config, err := ResolveAdapter(root)
	if err != nil {
		t.Fatalf("resolve adapter: %v", err)
	}
	if config.Connectors.Endpoint != "" {
		t.Fatalf("unexpected endpoint: %#v", config)
	}
	resp, err := adapter.Search(ResearchRequest{Topic: "AI tooling", Limit: 1})
	if err != nil {
		t.Fatalf("fallback search: %v", err)
	}
	if resp.Provider != "fake" || len(resp.Evidence) != 1 {
		t.Fatalf("fallback resp = %#v", resp)
	}
}
