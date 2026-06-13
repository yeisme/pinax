package research

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHermesConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	config := ExternalServiceConfig{SchemaVersion: ExternalServiceConfigSchemaVersion, Hermes: HermesConfig{Endpoint: "https://hermes.example.test", Capability: "daily_hot_notes"}}
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
	if loaded.Hermes.Endpoint != config.Hermes.Endpoint || loaded.Hermes.Capability != "daily_hot_notes" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestHermesResolverFallsBackWhenUnconfigured(t *testing.T) {
	root := t.TempDir()
	adapter, config, err := ResolveAdapter(root)
	if err != nil {
		t.Fatalf("resolve adapter: %v", err)
	}
	if config.Hermes.Endpoint != "" {
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
