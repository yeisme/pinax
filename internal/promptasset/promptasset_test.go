package promptasset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePromptAssetAcceptsValidFixture(t *testing.T) {
	asset := loadFixture(t, "valid.yaml")

	if err := Validate(asset); err != nil {
		t.Fatalf("Validate(valid) returned error: %v", err)
	}
	if asset.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", asset.SchemaVersion, SchemaVersion)
	}
	if len(asset.SourceRefs) != 1 || asset.SourceRefs[0].URI == "" {
		t.Fatalf("source refs not parsed: %#v", asset.SourceRefs)
	}
}

func TestValidatePromptAssetRejectsInvalidFixtures(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "missing_permission.yaml", want: "permission"},
		{name: "missing_prompt_template.yaml", want: "prompt_template"},
		{name: "invalid_lifecycle.yaml", want: "lifecycle"},
		{name: "invalid_permission.yaml", want: "permission"},
		{name: "invalid_variables.yaml", want: "variables"},
		{name: "invalid_source_refs.yaml", want: "source_refs[0].uri"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := loadFixture(t, tt.name)
			err := Validate(asset)
			if err == nil {
				t.Fatalf("Validate(%s) succeeded, want error containing %q", tt.name, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate(%s) error = %q, want substring %q", tt.name, err.Error(), tt.want)
			}
		})
	}
}

func TestLoadPromptAssetRejectsMalformedYAML(t *testing.T) {
	_, err := Load([]byte("schema_version: [broken\n"))
	if err == nil {
		t.Fatal("Load malformed YAML succeeded, want error")
	}
}

func loadFixture(t *testing.T, name string) Asset {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	asset, err := Load(b)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return asset
}
