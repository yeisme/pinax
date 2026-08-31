package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/connection"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

type connectionCheck struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type connectionDoctorReport struct {
	Descriptor     connection.Descriptor            `json:"descriptor"`
	Checks         map[string]connectionCheck       `json:"checks"`
	ManifestDigest string                           `json:"manifest_digest,omitempty"`
	Readiness      *pinaxclient.ConnectionReadiness `json:"readiness,omitempty"`
}

func addConnectionCommands(root *cobra.Command, ctx commandBuildContext) {
	connectionCmd := &cobra.Command{Use: "connection", Short: "Inspect and diagnose the Pinax owner connection"}
	connectionCmd.AddCommand(&cobra.Command{
		Use:   "inspect",
		Short: "Show the resolved non-sensitive connection descriptor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptor := resolvedConnectionDescriptor(cmd, ctx)
			projection := domain.NewProjection("connection.inspect", "Resolved Pinax connection descriptor.")
			applyConnectionFacts(&projection, descriptor)
			projection.Data = map[string]any{"descriptor": descriptor}
			if descriptor.ModeSource == "legacy_default" {
				projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
					Code:    "connection_mode_legacy_default",
					Message: "A non-loopback endpoint without an explicit connection mode is using the compatibility default.",
					Hint:    "Set remote.mode or pass --connection-mode.",
				})
			}
			return ctx.renderProjection(cmd, projection, nil)
		},
	})
	connectionCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Run read-only connection, manifest, auth, and readiness probes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := diagnoseConnection(cmd, ctx)
			projection := domain.NewProjection("connection.doctor", connectionDoctorSummary(report))
			applyConnectionFacts(&projection, report.Descriptor)
			for name, check := range report.Checks {
				projection.Facts[name+"_status"] = check.Status
				if check.Code != "" {
					projection.Facts[name+"_code"] = check.Code
				}
			}
			if report.ManifestDigest != "" {
				projection.Facts["manifest_digest"] = report.ManifestDigest
			}
			projection.Data = report
			return ctx.renderProjection(cmd, projection, nil)
		},
	})
	connectionCmd.AddCommand(&cobra.Command{
		Use:   "readiness",
		Short: "Show the six-layer connection readiness projection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			readiness := resolvedConnectionReadiness(cmd, ctx)
			projection := app.ConnectionReadinessValueProjection(readiness)
			return ctx.renderProjection(cmd, projection, nil)
		},
	})
	root.AddCommand(connectionCmd)
}

func resolvedConnectionReadiness(cmd *cobra.Command, ctx commandBuildContext) app.ConnectionReadiness {
	descriptor := resolvedConnectionDescriptor(cmd, ctx)
	if descriptor.Transport == connection.TransportEmbedded {
		return app.BuildConnectionReadiness(app.ConnectionReadinessOptions{
			Mode:           descriptor.Mode,
			Transport:      descriptor.Transport,
			OwnerAvailable: true,
		})
	}
	report := diagnoseConnection(cmd, ctx)
	if report.Readiness != nil {
		return appReadinessFromClient(*report.Readiness)
	}
	authMode := ""
	if report.Checks["auth"].Status == "ready" {
		authMode = "token-store"
	}
	readiness := app.BuildConnectionReadiness(app.ConnectionReadinessOptions{
		Mode:           descriptor.Mode,
		Transport:      descriptor.Transport,
		AuthMode:       authMode,
		OwnerAvailable: report.Checks["owner"].Status == "ready",
	})
	for _, name := range []string{"contract", "transport", "auth", "owner"} {
		check := report.Checks[name]
		if check.Status == "" {
			continue
		}
		layer := readiness.Layers[name]
		layer.Status = check.Status
		layer.EvidenceRefs = append(layer.EvidenceRefs, "connection_doctor")
		if check.Code != "" {
			layer.Blockers = append(layer.Blockers, check.Code)
		}
		readiness.Layers[name] = layer
	}
	readiness.Overall = app.SummarizeConnectionReadiness(readiness.Layers)
	return readiness
}

func appReadinessFromClient(value pinaxclient.ConnectionReadiness) app.ConnectionReadiness {
	layers := make(map[string]app.Readiness, len(value.Layers))
	for name, layer := range value.Layers {
		layers[name] = app.Readiness{
			Status:       layer.Status,
			Maturity:     layer.Maturity,
			Blockers:     append([]string{}, layer.Blockers...),
			NextActions:  append([]string{}, layer.NextActions...),
			EvidenceRefs: append([]string{}, layer.EvidenceRefs...),
		}
	}
	return app.ConnectionReadiness{
		SchemaVersion: value.SchemaVersion,
		Overall: app.Readiness{
			Status:       value.Overall.Status,
			Maturity:     value.Overall.Maturity,
			Blockers:     append([]string{}, value.Overall.Blockers...),
			NextActions:  append([]string{}, value.Overall.NextActions...),
			EvidenceRefs: append([]string{}, value.Overall.EvidenceRefs...),
		},
		Layers: layers,
	}
}

func resolvedConnectionDescriptor(cmd *cobra.Command, ctx commandBuildContext) connection.Descriptor {
	endpoint, endpointSource := remoteAPIURLSource(cmd, ctx)
	if endpointSource == "config" {
		endpointSource = configSourceForKey(*ctx.configResult, "remote.api_url")
	}
	mode := strings.TrimSpace(ctx.configResult.Config.Remote.Mode)
	modeSource := configSourceForKey(*ctx.configResult, "remote.mode")
	if mode == "" {
		modeSource = ""
	}
	return connection.Resolve(connection.ResolveInput{
		Endpoint:         endpoint,
		EndpointSource:   endpointSource,
		Mode:             mode,
		ModeSource:       modeSource,
		CredentialSource: remoteCredentialSource(ctx),
	})
}

func remoteCredentialSource(ctx commandBuildContext) string {
	sources := make([]string, 0, 4)
	if ctx.apiToken != nil && strings.TrimSpace(*ctx.apiToken) != "" {
		sources = append(sources, "flag_token")
	}
	if ctx.apiTokenFile != nil && strings.TrimSpace(*ctx.apiTokenFile) != "" {
		sources = append(sources, "flag_token_file")
	}
	if strings.TrimSpace(os.Getenv("PINAX_API_TOKEN")) != "" {
		sources = append(sources, "env_token")
	}
	if strings.TrimSpace(os.Getenv("PINAX_API_TOKEN_FILE")) != "" {
		sources = append(sources, "env_token_file")
	}
	if len(sources) == 0 {
		return "none"
	}
	if len(sources) > 1 {
		return "conflict"
	}
	return sources[0]
}

func diagnoseConnection(cmd *cobra.Command, ctx commandBuildContext) connectionDoctorReport {
	descriptor := resolvedConnectionDescriptor(cmd, ctx)
	report := connectionDoctorReport{
		Descriptor: descriptor,
		Checks: map[string]connectionCheck{
			"contract":  {Status: "not_configured", Message: "Contract probe has not run."},
			"transport": {Status: "not_configured", Message: "Transport probe has not run."},
			"auth":      {Status: "not_configured", Message: "Authentication probe has not run."},
			"owner":     {Status: "not_configured", Message: "Owner readiness probe has not run."},
		},
	}
	if descriptor.Transport == connection.TransportEmbedded {
		manifest, err := TransportManifest()
		if err != nil {
			report.Checks["contract"] = connectionCheck{Status: "blocked", Message: "Local transport manifest could not be compiled.", Code: "transport_manifest_invalid"}
		} else {
			report.ManifestDigest = manifest.Digest
			report.Checks["contract"] = connectionCheck{Status: "ready", Message: "Local transport manifest compiled successfully."}
		}
		report.Checks["transport"] = connectionCheck{Status: "ready", Message: "Embedded application service is selected."}
		report.Checks["auth"] = connectionCheck{Status: "not_applicable", Message: "Embedded invocation does not use bearer authentication."}
		report.Checks["owner"] = connectionCheck{Status: "ready", Message: "Local application service owns the vault state."}
		return report
	}
	if descriptor.Status == "blocked" {
		code := "connection_blocked"
		if len(descriptor.Blockers) > 0 {
			code = descriptor.Blockers[0]
		}
		report.Checks["transport"] = connectionCheck{Status: "blocked", Message: "Static connection policy blocked the remote probe.", Code: code}
		if code == "credential_required" || code == "credential_source_conflict" {
			report.Checks["auth"] = connectionCheck{Status: "blocked", Message: "A valid remote credential source is required.", Code: code}
		}
		return report
	}

	token, err := remoteAPIToken(ctx)
	if err != nil {
		report.Checks["auth"] = connectionCheck{Status: "blocked", Message: "Remote credential could not be loaded securely.", Code: "remote_api_token_unreadable"}
		return report
	}
	client, err := pinaxclient.New(pinaxclient.Config{BaseURL: descriptor.Endpoint, Token: token})
	if err != nil {
		report.Checks["transport"] = connectionCheck{Status: "blocked", Message: "Remote client policy rejected the endpoint.", Code: clientErrorCode(err)}
		return report
	}
	manifest, err := client.Manifest(cmd.Context())
	if err != nil {
		classifyConnectionProbeError(&report, err)
		return report
	}
	report.ManifestDigest = manifest.Digest
	report.Checks["transport"] = connectionCheck{Status: "ready", Message: "Owner endpoint returned a response without redirect."}
	report.Checks["auth"] = connectionCheck{Status: "ready", Message: "Owner accepted the read-only manifest request."}
	report.Checks["contract"] = connectionCheck{Status: "ready", Message: "Owner manifest identity and digest are valid."}

	readiness, err := client.Readiness(cmd.Context())
	if err != nil {
		report.Checks["owner"] = connectionCheck{Status: "degraded", Message: "Owner readiness could not be validated.", Code: clientErrorCode(err)}
		return report
	}
	report.Readiness = &readiness
	report.Checks["owner"] = connectionCheck{Status: readiness.Overall.Status, Message: "Owner readiness projection was validated."}
	return report
}

func classifyConnectionProbeError(report *connectionDoctorReport, err error) {
	var clientErr *pinaxclient.Error
	if errors.As(err, &clientErr) {
		switch {
		case clientErr.HTTPStatus == http.StatusUnauthorized || clientErr.HTTPStatus == http.StatusForbidden:
			report.Checks["transport"] = connectionCheck{Status: "ready", Message: "Owner endpoint is reachable."}
			report.Checks["auth"] = connectionCheck{Status: "blocked", Message: "Owner rejected the credential.", Code: clientErr.Code}
		case clientErr.Code == pinaxclient.CodeRequestFailed:
			report.Checks["transport"] = connectionCheck{Status: "blocked", Message: "Owner endpoint could not be reached.", Code: clientErr.Code}
		default:
			report.Checks["transport"] = connectionCheck{Status: "ready", Message: "Owner endpoint returned a response."}
			report.Checks["auth"] = connectionCheck{Status: "degraded", Message: "Authentication could not be distinguished from the invalid response.", Code: clientErr.Code}
			report.Checks["contract"] = connectionCheck{Status: "blocked", Message: "Owner manifest response was invalid.", Code: clientErr.Code}
		}
		return
	}
	report.Checks["transport"] = connectionCheck{Status: "blocked", Message: "Owner endpoint probe failed.", Code: "request_failed"}
}

func clientErrorCode(err error) string {
	var clientErr *pinaxclient.Error
	if errors.As(err, &clientErr) {
		return clientErr.Code
	}
	return "request_failed"
}

func applyConnectionFacts(projection *domain.Projection, descriptor connection.Descriptor) {
	projection.Facts["schema_version"] = descriptor.SchemaVersion
	projection.Facts["mode"] = descriptor.Mode
	projection.Facts["mode_source"] = descriptor.ModeSource
	projection.Facts["transport"] = descriptor.Transport
	projection.Facts["endpoint_source"] = descriptor.EndpointSource
	projection.Facts["credential_source"] = descriptor.CredentialSource
	projection.Facts["tls_required"] = fmt.Sprint(descriptor.TLSRequired)
	projection.Facts["redirect_policy"] = descriptor.RedirectPolicy
	projection.Facts["manifest_ref"] = descriptor.ManifestRef
	projection.Facts["readiness_status"] = descriptor.Status
	projection.Facts["maturity"] = descriptor.Maturity
}

func connectionDoctorSummary(report connectionDoctorReport) string {
	blocked := 0
	degraded := 0
	for _, check := range report.Checks {
		switch check.Status {
		case "blocked":
			blocked++
		case "degraded":
			degraded++
		}
	}
	if blocked > 0 {
		return fmt.Sprintf("Connection doctor found %d blocked read-only check(s).", blocked)
	}
	if degraded > 0 || report.Descriptor.Status == "degraded" {
		return "Connection doctor completed with migration or readiness warnings."
	}
	return "Connection doctor read-only checks passed."
}
