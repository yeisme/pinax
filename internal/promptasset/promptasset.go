package promptasset

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = "yeisme.prompt_asset.v1"

var allowedLifecycles = map[string]struct{}{
	"draft":    {},
	"tested":   {},
	"accepted": {},
	"promoted": {},
	"retired":  {},
}

var allowedPermissions = map[string]struct{}{
	"unknown":  {},
	"internal": {},
	"public":   {},
}

type Asset struct {
	SchemaVersion  string              `yaml:"schema_version" json:"schema_version"`
	ID             string              `yaml:"id" json:"id"`
	Title          string              `yaml:"title,omitempty" json:"title,omitempty"`
	Domain         string              `yaml:"domain" json:"domain"`
	Tags           []string            `yaml:"tags,omitempty" json:"tags,omitempty"`
	Lifecycle      string              `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Permission     string              `yaml:"permission" json:"permission"`
	OwnerProject   string              `yaml:"owner_project,omitempty" json:"owner_project,omitempty"`
	Variables      map[string]Variable `yaml:"variables" json:"variables"`
	PromptTemplate string              `yaml:"prompt_template" json:"prompt_template"`
	Constraints    []string            `yaml:"constraints,omitempty" json:"constraints,omitempty"`
	ReviewGuidance string              `yaml:"review_guidance,omitempty" json:"review_guidance,omitempty"`
	SourceRefs     []SourceRef         `yaml:"source_refs,omitempty" json:"source_refs,omitempty"`
}

type Variable struct {
	Type        string `yaml:"type" json:"type"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type SourceRef struct {
	URI      string `yaml:"uri" json:"uri"`
	Label    string `yaml:"label,omitempty" json:"label,omitempty"`
	Evidence string `yaml:"evidence,omitempty" json:"evidence,omitempty"`
}

func Load(content []byte) (Asset, error) {
	var asset Asset
	if err := yaml.Unmarshal(content, &asset); err != nil {
		return Asset{}, fmt.Errorf("load prompt asset: %w", err)
	}
	return asset, nil
}

func Validate(asset Asset) error {
	var problems []string
	require := func(field, value string) {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, field+" is required")
		}
	}

	require("schema_version", asset.SchemaVersion)
	if strings.TrimSpace(asset.SchemaVersion) != "" && asset.SchemaVersion != SchemaVersion {
		problems = append(problems, fmt.Sprintf("schema_version must be %q", SchemaVersion))
	}
	require("id", asset.ID)
	require("domain", asset.Domain)
	require("permission", asset.Permission)
	require("prompt_template", asset.PromptTemplate)

	lifecycle := strings.TrimSpace(asset.Lifecycle)
	if lifecycle != "" {
		if _, ok := allowedLifecycles[lifecycle]; !ok {
			problems = append(problems, "lifecycle must be one of draft, tested, accepted, promoted, retired")
		}
	}

	permission := strings.TrimSpace(asset.Permission)
	if permission != "" {
		if _, ok := allowedPermissions[permission]; !ok {
			problems = append(problems, "permission must be one of unknown, internal, public")
		}
	}

	if asset.Variables == nil {
		problems = append(problems, "variables is required")
	} else if len(asset.Variables) == 0 {
		problems = append(problems, "variables must define at least one variable")
	}
	for name, variable := range asset.Variables {
		if strings.TrimSpace(name) == "" {
			problems = append(problems, "variables must not contain an empty name")
		}
		if strings.TrimSpace(variable.Type) == "" {
			problems = append(problems, fmt.Sprintf("variables[%s].type is required", name))
		}
	}

	for i, ref := range asset.SourceRefs {
		if strings.TrimSpace(ref.URI) == "" {
			problems = append(problems, fmt.Sprintf("source_refs[%d].uri is required", i))
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
