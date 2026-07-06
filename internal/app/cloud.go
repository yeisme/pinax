package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
)

func (s *Service) CloudBackendSetS3(_ context.Context, req CloudBackendSetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	bucket := strings.TrimSpace(req.Bucket)
	region := strings.TrimSpace(req.Region)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	deviceID := strings.TrimSpace(req.DeviceID)
	if bucket == "" || region == "" || workspaceID == "" || deviceID == "" {
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "s3 cloud backend configuration is incomplete", Hint: "Provide --bucket, --region, --workspace, and --device"}
		return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
	}
	prefix := strings.Trim(strings.TrimSpace(req.Prefix), "/")
	addressingStyle := normalizeCloudS3AddressingStyle(req.AddressingStyle)
	if addressingStyle == "invalid" {
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "s3 addressing style is invalid", Hint: "Use --addressing-style auto, path, or virtual-hosted"}
		return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
	}
	endpointURL := strings.TrimSpace(req.Endpoint)
	pathStyle := cloudS3PathStyle(endpointURL, addressingStyle)
	endpoint := buildS3CloudEndpoint(bucket, prefix, endpointURL, region, strings.TrimSpace(req.Profile), addressingStyle, pathStyle)
	secretRef := strings.TrimSpace(req.SecretRef)
	if secretRef == "" && strings.TrimSpace(req.Profile) != "" {
		secretRef = "profile://" + strings.TrimSpace(req.Profile)
	}
	state, err := pinaxcloud.Login(root, pinaxcloud.LoginRequest{Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: deviceID, SecretRef: secretRef, BackendKind: "s3-direct", S3: &pinaxcloud.S3Config{Bucket: bucket, Prefix: prefix, Endpoint: endpointURL, Region: region, Profile: strings.TrimSpace(req.Profile), AddressingStyle: addressingStyle, PathStyle: pathStyle}})
	if err != nil {
		projection, commandErr := cloudBackendSetErrorProjection(err)
		return projection, commandErr
	}
	projection := domain.NewProjection("cloud.backend.set", "S3 direct cloud backend configured.")
	addCloudStateFacts(&projection, state)
	projection.Facts["backend_kind"] = "s3-direct"
	projection.Facts["bucket"] = bucket
	projection.Facts["region"] = region
	if prefix != "" {
		projection.Facts["prefix"] = prefix
	}
	if strings.TrimSpace(req.Profile) != "" {
		projection.Facts["credential_source"] = "profile"
	}
	if addressingStyle != "" {
		projection.Facts["addressing_style"] = addressingStyle
	}
	projection.Facts["path_style"] = fmt.Sprint(pathStyle)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func cloudBackendSetErrorProjection(err error) (domain.Projection, error) {
	msg := err.Error()
	commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: msg, Hint: "Use a supported cloud backend such as server, s3, or rclone"}
	return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
}

func (s *Service) CloudBackendSetRclone(_ context.Context, req CloudBackendSetRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("cloud.backend.set", err), err
	}
	remoteName := strings.TrimSpace(req.Remote)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	deviceID := strings.TrimSpace(req.DeviceID)
	if remoteName == "" || workspaceID == "" || deviceID == "" {
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "rclone cloud backend configuration is incomplete", Hint: "Provide --remote, --workspace, and --device"}
		return domain.NewErrorProjection("cloud.backend.set", commandErr), commandErr
	}
	endpoint := rcloneEndpoint(remoteName)
	secretRef := strings.TrimSpace(req.SecretRef)
	if secretRef == "" {
		secretRef = "rclone://" + remoteName
	}
	state, err := pinaxcloud.Login(root, pinaxcloud.LoginRequest{Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: deviceID, SecretRef: secretRef, BackendKind: "rclone-direct"})
	if err != nil {
		projection, commandErr := cloudBackendSetErrorProjection(err)
		return projection, commandErr
	}
	projection := domain.NewProjection("cloud.backend.set", "Rclone direct cloud backend configured.")
	addCloudStateFacts(&projection, state)
	projection.Facts["backend_kind"] = "rclone-direct"
	projection.Facts["remote"] = remoteName
	projection.Facts["credential_source"] = "rclone"
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func rcloneEndpoint(remoteName string) string {
	remoteName = strings.TrimSpace(remoteName)
	name, rest, ok := strings.Cut(remoteName, ":")
	if !ok {
		return "rclone://" + strings.Trim(remoteName, "/")
	}
	return "rclone://" + strings.Trim(name, "/") + "/" + strings.Trim(rest, "/")
}

func buildS3CloudEndpoint(bucket, prefix, endpointURL, region, profile, addressingStyle string, pathStyle bool) string {
	endpoint := "s3://" + bucket
	if prefix != "" {
		endpoint += "/" + prefix
	}
	values := url.Values{}
	if endpointURL != "" {
		values.Set("endpoint", endpointURL)
	}
	if addressingStyle != "" {
		values.Set("addressing_style", addressingStyle)
	}
	if pathStyle {
		values.Set("path_style", "true")
	}
	if region != "" {
		values.Set("region", region)
	}
	if profile != "" {
		values.Set("profile", profile)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}

func normalizeCloudS3AddressingStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", "auto":
		return ""
	case "path", "path-style", "path_style":
		return "path"
	case "virtual", "virtual-hosted", "virtual_hosted":
		return "virtual-hosted"
	default:
		return "invalid"
	}
}

func cloudS3PathStyle(endpointURL, addressingStyle string) bool {
	switch addressingStyle {
	case "path":
		return true
	case "virtual-hosted":
		return false
	}
	return backendS3PathStyle(endpointURL)
}

func (s *Service) CloudLogin(_ context.Context, req CloudLoginRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.login", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("cloud.login", err), err
	}
	state, err := pinaxcloud.Login(root, pinaxcloud.LoginRequest{Endpoint: req.Endpoint, WorkspaceID: req.WorkspaceID, DeviceID: req.DeviceID, SecretRef: req.SecretRef, EncryptionSecretRef: req.EncryptionSecretRef})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unsupported remote scheme") || strings.Contains(msg, "invalid endpoint URI") || strings.Contains(msg, "endpoint URI must specify a scheme") {
			commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: msg, Hint: "Use a supported scheme, such as s3:// or file://"}
			return domain.NewErrorProjection("cloud.login", commandErr), commandErr
		}
		commandErr := &domain.CommandError{Code: "invalid_cloud_config", Message: "cloud login configuration is incomplete", Hint: "Provide --endpoint, --workspace, --device, and --secret-ref"}
		return domain.NewErrorProjection("cloud.login", commandErr), commandErr
	}
	projection := domain.NewProjection("cloud.login", "Cloud backend configured.")
	addCloudStateFacts(&projection, state)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax cloud status --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudStatus(_ context.Context, req CloudRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.status", err), err
	}
	state, err := pinaxcloud.Load(root)
	if err != nil {
		return cloudStateErrorProjection("cloud.status", root, err)
	}
	projection := domain.NewProjection("cloud.status", "Cloud backend status read.")
	addCloudStateFacts(&projection, state)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax cloud doctor --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudLogout(_ context.Context, req CloudRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.logout", err), err
	}
	if err := pinaxcloud.Logout(root); err != nil {
		return cloudStateErrorProjection("cloud.logout", root, err)
	}
	state, err := pinaxcloud.Load(root)
	if err != nil {
		return errorProjection("cloud.logout", err), err
	}
	projection := domain.NewProjection("cloud.logout", "Cloud device session logged out.")
	addCloudStateFacts(&projection, state)
	projection.Data = pinaxcloud.RedactedData(state)
	projection.Actions = []domain.Action{{Name: "login", Command: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}}
	return projection, nil
}

func (s *Service) CloudDoctor(_ context.Context, req CloudRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("cloud.doctor", err), err
	}
	result := pinaxcloud.Doctor(root)
	if !result.Configured {
		commandErr := &domain.CommandError{Code: result.Code, Message: result.Message, Hint: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}
		return domain.NewErrorProjection("cloud.doctor", commandErr), commandErr
	}
	projection := domain.NewProjection("cloud.doctor", "Cloud backend diagnostics passed.")
	projection.Facts["configured"] = "true"
	projection.Facts["backend_kind"] = result.BackendKind
	projection.Facts["auth_boundary"] = result.AuthBoundary
	projection.Facts["server_audit"] = fmt.Sprint(result.ServerAudit)
	projection.Facts["endpoint"] = result.Endpoint
	projection.Facts["workspace_id"] = result.Workspace
	projection.Facts["device_id"] = result.DeviceID
	projection.Facts["secret_ref_configured"] = "true"
	projection.Data = result
	projection.Actions = []domain.Action{{Name: "status", Command: fmt.Sprintf("pinax cloud status --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func addCloudStateFacts(projection *domain.Projection, state pinaxcloud.State) {
	projection.Facts["configured"] = "true"
	projection.Facts["backend_kind"] = state.Config.BackendKind
	if projection.Facts["backend_kind"] == "" {
		projection.Facts["backend_kind"] = "server"
	}
	projection.Facts["endpoint"] = state.Config.Endpoint
	projection.Facts["workspace_id"] = state.Config.WorkspaceID
	projection.Facts["device_id"] = state.Config.DeviceID
	projection.Facts["session_status"] = state.Session.Status
	projection.Facts["secret_ref_configured"] = fmt.Sprint(strings.TrimSpace(state.Config.SecretRef) != "")
	projection.Facts["encryption_secret_ref_configured"] = fmt.Sprint(strings.TrimSpace(pinaxcloud.EncryptionSecretRef(state.Config)) != "")
}

func cloudStateErrorProjection(command, root string, err error) (domain.Projection, error) {
	if pinaxcloud.IsNotConfigured(err) {
		commandErr := &domain.CommandError{Code: "cloud_not_configured", Message: "cloud backend is not configured", Hint: fmt.Sprintf("pinax cloud login --vault %s --endpoint <url> --workspace <id> --device <id> --secret-ref <ref>", shellQuote(root))}
		return domain.NewErrorProjection(command, commandErr), commandErr
	}
	return errorProjection(command, err), err
}
