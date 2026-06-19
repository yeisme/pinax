package publishops

import (
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type NoteEligibility struct {
	Selected bool   `json:"selected"`
	Reason   string `json:"reason"`
}

func ValidateProfile(profile domain.PublishProfile) []domain.PublishValidationIssue {
	var issues []domain.PublishValidationIssue
	if profile.SchemaVersion != "" && profile.SchemaVersion != domain.PublishProfileSchemaVersion {
		issues = appendIssue(issues, "publish_schema_version_invalid", "schema_version")
	}
	if profile.Target != domain.PublishTargetGitHubPages && profile.Target != domain.PublishTargetGitHubWiki && profile.Target != domain.PublishTargetGitHubGist && profile.Target != domain.PublishTargetHTTP {
		issues = appendIssue(issues, "publish_target_invalid", "target")
	}
	if profile.Renderer != domain.PublishRendererHugo && profile.Renderer != domain.PublishRendererNone {
		issues = appendIssue(issues, "publish_renderer_invalid", "renderer")
	}
	if profile.BodyPolicy != domain.PublishBodyPolicyPublishedNotesOnly {
		issues = appendIssue(issues, "publish_body_policy_invalid", "body_policy")
	}
	if !safeRelativePath(profile.Output.Path) {
		issues = appendIssue(issues, "publish_output_path_unsafe", "output.path")
	}
	if profile.Deploy.Repo != "" && !safeDeployRepo(profile.Deploy.Repo) {
		issues = appendIssue(issues, "publish_deploy_repo_unsafe", "deploy.repo")
	}
	if profile.Deploy.Endpoint != "" && !safeHTTPEndpoint(profile.Deploy.Endpoint) {
		issues = appendIssue(issues, "publish_deploy_endpoint_invalid", "deploy.endpoint")
	}
	if profile.Deploy.SecretRef != "" && !safeSecretRef(profile.Deploy.SecretRef) {
		issues = appendIssue(issues, "publish_secret_ref_unsupported", "deploy.secret_ref")
	}
	if !profile.Safety.BlockSecrets || !profile.Safety.BlockPrivateBodies || !profile.Safety.BlockPinaxInternals {
		issues = appendIssue(issues, "publish_safety_gate_disabled", "safety")
	}
	theme := strings.TrimSpace(profile.Site.Theme.Value)
	if strings.Contains(theme, "${") || strings.Contains(strings.ToLower(theme), "secret") {
		issues = appendIssue(issues, "publish_secret_ref_unsupported", "site.theme")
	}
	if profile.Site.Theme.ContractVersion != "" && profile.Site.Theme.ContractVersion != PublishThemeSchemaVersion {
		issues = appendIssue(issues, "publish_theme_contract_mismatch", "site.theme.contract_version")
	}
	if !safeThemeSource(theme) {
		issues = appendIssue(issues, "publish_theme_source_invalid", "site.theme")
	}
	return issues
}

func ClassifyNoteEligibility(profile domain.PublishProfile, note domain.Note) NoteEligibility {
	if !contains(profile.Selection.IncludeStatuses, note.Status) {
		return NoteEligibility{Reason: "status_not_allowed"}
	}
	if !contains(profile.Selection.IncludeTypes, note.Kind) {
		return NoteEligibility{Reason: "type_not_allowed"}
	}
	publishValue := strings.TrimSpace(note.Frontmatter["publish"])
	if publishValue == "" {
		publishValue = strings.TrimSpace(note.Frontmatter["published"])
	}
	if publishValue == "false" || !contains(profile.Selection.IncludePublishValues, publishValue) {
		return NoteEligibility{Reason: "publish_value_not_allowed"}
	}
	if contains(profile.Selection.ExcludePrivacyValues, note.Frontmatter["privacy"]) {
		return NoteEligibility{Reason: "privacy_excluded"}
	}
	return NoteEligibility{Selected: true, Reason: "selected"}
}

func ClassifyNoteViolations(note domain.Note) []domain.PublishViolation {
	body := note.Body
	lower := strings.ToLower(body)
	violations := make([]domain.PublishViolation, 0)
	add := func(class domain.PublishViolationClass) {
		violations = append(violations, domain.PublishViolation{Class: class, Path: note.Path, Severity: "blocking", Message: "Publish candidate contains blocked content"})
	}
	if strings.Contains(body, "Authorization:") || strings.Contains(lower, "bearer ") {
		add(domain.PublishViolationAuthorizationHeader)
	}
	if strings.Contains(body, "Cookie:") {
		add(domain.PublishViolationCookieHeader)
	}
	if strings.Contains(lower, "webhook") && (strings.Contains(lower, "http://") || strings.Contains(lower, "https://")) {
		add(domain.PublishViolationWebhookURL)
	}
	if strings.Contains(lower, "provider raw payload") || strings.Contains(lower, "raw_provider_payload") {
		add(domain.PublishViolationProviderPayload)
	}
	if strings.Contains(lower, "private_body") || strings.Contains(lower, "raw_body") || strings.Contains(lower, "private body") {
		add(domain.PublishViolationPrivateBodyLeak)
	}
	if strings.Contains(body, ".pinax/") {
		add(domain.PublishViolationPinaxInternalRef)
	}
	if strings.Contains(lower, "secret_") || strings.Contains(lower, "secret=") || strings.Contains(lower, "token=") {
		add(domain.PublishViolationSecretPattern)
	}
	if strings.Contains(body, "/Users/") || strings.Contains(body, "/home/") || strings.Contains(body, "C:\\") {
		add(domain.PublishViolationAbsolutePath)
	}
	return violations
}

func appendIssue(issues []domain.PublishValidationIssue, code, field string) []domain.PublishValidationIssue {
	return append(issues, domain.PublishValidationIssue{Code: code, Field: field, Severity: "error"})
}

func safeRelativePath(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	slash := filepath.ToSlash(raw)
	if filepath.IsAbs(raw) || strings.HasPrefix(slash, "../") || slash == ".." || strings.Contains(slash, "/../") {
		return false
	}
	if strings.HasPrefix(slash, ".pinax/") || slash == ".pinax" {
		return false
	}
	return true
}

func safeDeployRepo(raw string) bool {
	return !strings.Contains(filepath.ToSlash(raw), ".pinax/")
}

func safeHTTPEndpoint(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://127.0.0.1") || strings.HasPrefix(lower, "http://localhost")
}

func safeSecretRef(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	if !strings.HasPrefix(raw, "env:") {
		return false
	}
	name := strings.TrimPrefix(raw, "env:")
	if name == "" || strings.ContainsAny(name, " ${}/\\") {
		return false
	}
	return true
}

func safeThemeSource(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "builtin:"+BuiltinThemeName {
		return true
	}
	if !strings.HasPrefix(raw, "local:") {
		return false
	}
	return safeRelativePath(strings.TrimPrefix(raw, "local:"))
}

func contains(values []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range values {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}
