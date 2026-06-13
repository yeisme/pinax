package research

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const ExternalServiceConfigSchemaVersion = "pinax.research.config.v1"

type ExternalServiceConfig struct {
	SchemaVersion string       `json:"schema_version"`
	Hermes        HermesConfig `json:"hermes"`
}

func SaveExternalServiceConfig(root string, config ExternalServiceConfig) error {
	if config.SchemaVersion == "" {
		config.SchemaVersion = ExternalServiceConfigSchemaVersion
	}
	path := researchConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func LoadExternalServiceConfig(root string) (ExternalServiceConfig, error) {
	b, err := os.ReadFile(researchConfigPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ExternalServiceConfig{SchemaVersion: ExternalServiceConfigSchemaVersion}, nil
		}
		return ExternalServiceConfig{}, err
	}
	var config ExternalServiceConfig
	if err := json.Unmarshal(b, &config); err != nil {
		return ExternalServiceConfig{}, err
	}
	if config.SchemaVersion == "" {
		config.SchemaVersion = ExternalServiceConfigSchemaVersion
	}
	return config, nil
}

func ResolveAdapter(root string) (Adapter, ExternalServiceConfig, error) {
	config, err := LoadExternalServiceConfig(root)
	if err != nil {
		return nil, ExternalServiceConfig{}, err
	}
	return NewHermesAdapter(config.Hermes, NewFakeAdapter(nil)), config, nil
}

func researchConfigPath(root string) string {
	return filepath.Join(root, ".pinax", "briefing", "research.json")
}
