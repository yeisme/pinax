// Package providers implements optional in-process judgment transports.
// The core SDK remains provider-neutral; secrets and authorization are injected.
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	judgment "github.com/yeisme/judgment-sdk"
)

const InputsExtension = "structured_inputs.v1"

type CredentialBinding struct {
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	Env    string `json:"env,omitempty"`
}

// ExecutionProfile contains references only, never keys or caller-controlled URLs.
type ExecutionProfile struct {
	ID         string            `json:"id"`
	Revision   uint64            `json:"revision"`
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	Credential CredentialBinding `json:"credential"`
	Enabled    bool              `json:"enabled"`
}

var safeRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$`)
var profileID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var safeEnv = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
var credentialRef = regexp.MustCompile(`^(yeisme-credential://)?(openai|typesafe|openrouter)/[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var secretShape = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9_-]{20,}|eyJ[a-z0-9_-]{10,})`)

type ProviderOption struct {
	ID    string `json:"id"`
	Model string `json:"model"`
}

func AvailableProviders() []ProviderOption {
	return []ProviderOption{{"openai_decisions", "gpt-6-luna"}, {"typesafe", "jev-1.13.0"}, {"openrouter", "typesafe/jev-1.13"}}
}

func (p ExecutionProfile) Validate() error {
	if !profileID.MatchString(p.ID) || secretShape.MatchString(p.ID) || p.Revision == 0 || p.Revision > 9007199254740991 {
		return errors.New("judgment_profile_invalid")
	}
	models := map[string]string{"openai_decisions": "gpt-6-luna", "typesafe": "jev-1.13.0", "openrouter": "typesafe/jev-1.13"}
	if models[p.Provider] == "" || models[p.Provider] != p.Model {
		return errors.New("judgment_profile_model_unsupported")
	}
	switch p.Credential.Source {
	case "shared":
		ref := strings.TrimPrefix(p.Credential.Ref, "yeisme-credential://")
		namespace := p.Provider
		if namespace == "openai_decisions" {
			namespace = "openai"
		}
		if !credentialRef.MatchString(p.Credential.Ref) || !strings.HasPrefix(ref, namespace+"/") || secretShape.MatchString(p.Credential.Ref) || p.Credential.Env != "" {
			return errors.New("judgment_credential_binding_invalid")
		}
	case "env":
		if !safeEnv.MatchString(p.Credential.Env) || p.Credential.Ref != "" {
			return errors.New("judgment_credential_binding_invalid")
		}
	default:
		return errors.New("judgment_credential_source_required")
	}
	return nil
}

func (p ExecutionProfile) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	tree, err := judgment.ParseTree(b)
	if err != nil {
		return "", err
	}
	return judgment.Digest(tree)
}

// Resolver never falls back after a failed, missing, expired or revoked grant.
// Hosts provide Shared through credentialctl and Environment through their own
// process lookup. This package never reads the process environment itself.
type Resolver struct {
	Shared      func(context.Context, string) (string, error)
	Environment func(string) (string, bool)
}

func (r Resolver) Resolve(ctx context.Context, b CredentialBinding) (string, error) {
	switch b.Source {
	case "shared":
		if r.Shared != nil && b.Env == "" && credentialRef.MatchString(b.Ref) && !secretShape.MatchString(b.Ref) {
			if value, err := r.Shared(ctx, b.Ref); err == nil && value != "" {
				return value, nil
			}
		}
	case "env":
		if r.Environment != nil && b.Ref == "" && safeEnv.MatchString(b.Env) {
			if value, ok := r.Environment(b.Env); ok && value != "" {
				return value, nil
			}
		}
	}
	return "", errors.New("judgment_credential_unavailable")
}
