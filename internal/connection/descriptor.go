package connection

import (
	"net"
	"net/url"
	"strings"
)

const (
	DescriptorSchemaV1 = "pinax.connection_descriptor.v1"

	ModeLocalVault        = "local-vault"
	ModeRemoteService     = "remote-service"
	ModeSelfHostedService = "self-hosted-service"

	TransportEmbedded     = "embedded"
	TransportLoopbackHTTP = "loopback-http"
	TransportHTTPS        = "https"
)

type ResolveInput struct {
	Endpoint         string
	EndpointSource   string
	Mode             string
	ModeSource       string
	CredentialSource string
}

type Descriptor struct {
	SchemaVersion       string   `json:"schema_version"`
	Mode                string   `json:"mode"`
	ModeSource          string   `json:"mode_source"`
	Transport           string   `json:"transport"`
	Endpoint            string   `json:"endpoint,omitempty"`
	EndpointSource      string   `json:"endpoint_source"`
	CredentialSource    string   `json:"credential_source"`
	Workspace           string   `json:"workspace"`
	Timeout             string   `json:"timeout"`
	TLSRequired         bool     `json:"tls_required"`
	RedirectPolicy      string   `json:"redirect_policy"`
	RequestedCapability []string `json:"requested_capabilities,omitempty"`
	ManifestRef         string   `json:"manifest_ref"`
	Status              string   `json:"status"`
	Maturity            string   `json:"maturity"`
	Blockers            []string `json:"blockers,omitempty"`
	NextActions         []string `json:"next_actions,omitempty"`
}

func Resolve(input ResolveInput) Descriptor {
	descriptor := Descriptor{
		SchemaVersion:    DescriptorSchemaV1,
		EndpointSource:   normalizedSource(input.EndpointSource, "default"),
		CredentialSource: normalizedSource(input.CredentialSource, "none"),
		Workspace:        "default",
		Timeout:          "15s",
		RedirectPolicy:   "reject",
		ManifestRef:      "pinax://manifest",
		Status:           "ready",
		Maturity:         "first-support",
	}

	endpoint := strings.TrimSpace(input.Endpoint)
	mode := strings.TrimSpace(input.Mode)
	if endpoint == "" {
		descriptor.Mode = ModeLocalVault
		descriptor.ModeSource = "default"
		descriptor.Transport = TransportEmbedded
		descriptor.EndpointSource = "default"
		descriptor.CredentialSource = "not_applicable"
		descriptor.TLSRequired = false
		descriptor.Maturity = "mature"
		return descriptor
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		descriptor.Mode = normalizedMode(mode, ModeRemoteService)
		descriptor.ModeSource = normalizedSource(input.ModeSource, "legacy_default")
		descriptor.Transport = "invalid"
		descriptor.Status = "blocked"
		descriptor.Blockers = append(descriptor.Blockers, "endpoint_invalid")
		descriptor.NextActions = append(descriptor.NextActions, "Set remote.api_url to an absolute HTTPS or loopback HTTP URL")
		return descriptor
	}
	descriptor.Endpoint = sanitizedEndpoint(parsed)
	descriptor.ManifestRef = strings.TrimSuffix(descriptor.Endpoint, "/") + "/v1/manifest"
	loopback := isLoopback(parsed.Hostname())
	descriptor.TLSRequired = !loopback
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		block(&descriptor, "endpoint_unsafe", "Remove userinfo, query, and fragment values from the owner endpoint")
	}
	switch {
	case parsed.Scheme == "http" && loopback:
		descriptor.Transport = TransportLoopbackHTTP
	case parsed.Scheme == "https":
		descriptor.Transport = TransportHTTPS
	default:
		descriptor.Transport = parsed.Scheme
		descriptor.Status = "blocked"
		descriptor.Blockers = append(descriptor.Blockers, "tls_required")
		descriptor.NextActions = append(descriptor.NextActions, "Use HTTPS for non-loopback owner endpoints")
	}

	if mode != "" {
		descriptor.Mode = mode
		descriptor.ModeSource = normalizedSource(input.ModeSource, "explicit")
	} else if loopback {
		descriptor.Mode = ModeLocalVault
		descriptor.ModeSource = "endpoint_classification"
	} else {
		descriptor.Mode = ModeRemoteService
		descriptor.ModeSource = "legacy_default"
		degrade(&descriptor, "connection_mode_legacy_default", "Set remote.mode or pass --connection-mode")
	}

	if !validMode(descriptor.Mode) {
		block(&descriptor, "connection_mode_invalid", "Use local-vault, remote-service, or self-hosted-service")
	}
	if !loopback && descriptor.Mode == ModeLocalVault {
		block(&descriptor, "connection_mode_endpoint_conflict", "Use remote-service or self-hosted-service for a non-loopback endpoint")
	}
	if descriptor.CredentialSource == "conflict" {
		block(&descriptor, "credential_source_conflict", "Keep only one remote API credential source")
	} else if !loopback && descriptor.CredentialSource == "none" {
		block(&descriptor, "credential_required", "Configure an owner-only token file or approved user-level secret ref")
	} else if !loopback && (descriptor.CredentialSource == "flag_token" || descriptor.CredentialSource == "env_token") {
		degrade(&descriptor, "raw_token_compatibility", "Prefer --api-token-file or PINAX_API_TOKEN_FILE for non-loopback endpoints")
	}
	return descriptor
}

func block(descriptor *Descriptor, blocker, action string) {
	descriptor.Status = "blocked"
	descriptor.Blockers = appendUnique(descriptor.Blockers, blocker)
	descriptor.NextActions = appendUnique(descriptor.NextActions, action)
}

func degrade(descriptor *Descriptor, blocker, action string) {
	if descriptor.Status == "ready" {
		descriptor.Status = "degraded"
	}
	descriptor.Blockers = appendUnique(descriptor.Blockers, blocker)
	descriptor.NextActions = appendUnique(descriptor.NextActions, action)
}

func sanitizedEndpoint(parsed *url.URL) string {
	copy := *parsed
	copy.User = nil
	copy.RawQuery = ""
	copy.Fragment = ""
	return strings.TrimSuffix(copy.String(), "/")
}

func isLoopback(host string) bool {
	normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if normalized == "localhost" {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

func validMode(mode string) bool {
	switch mode {
	case ModeLocalVault, ModeRemoteService, ModeSelfHostedService:
		return true
	default:
		return false
	}
}

func normalizedMode(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func normalizedSource(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
