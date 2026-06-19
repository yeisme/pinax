package publishops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestValidateProfileRejectsUnsafeValues(t *testing.T) {
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Output.Path = "../site"
	profile.Safety.BlockSecrets = false

	issues := ValidateProfile(profile)
	if !hasIssue(issues, "publish_output_path_unsafe") {
		t.Fatalf("expected unsafe output path issue, got %#v", issues)
	}
	if !hasIssue(issues, "publish_safety_gate_disabled") {
		t.Fatalf("expected disabled safety gate issue, got %#v", issues)
	}
}

func TestValidateProfileRejectsUnknownEnums(t *testing.T) {
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Target = domain.PublishTarget("ftp")
	profile.Renderer = domain.PublishRenderer("jekyll")
	profile.BodyPolicy = domain.PublishBodyPolicy("all-notes")

	issues := ValidateProfile(profile)
	for _, code := range []string{"publish_target_invalid", "publish_renderer_invalid", "publish_body_policy_invalid"} {
		if !hasIssue(issues, code) {
			t.Fatalf("expected %s issue, got %#v", code, issues)
		}
	}
}

func TestValidateProfileRejectsThemeContractMismatch(t *testing.T) {
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Site.Theme.ContractVersion = "pinax.publish_theme.v0"
	issues := ValidateProfile(profile)
	if !hasIssue(issues, "publish_theme_contract_mismatch") {
		t.Fatalf("expected theme contract mismatch issue, got %#v", issues)
	}
}

func TestValidateProfileRejectsUnsafeLocalThemePath(t *testing.T) {
	for _, value := range []string{"local:../theme", "local:.pinax/theme", "local:/tmp/theme", "remote:https://example.invalid/theme"} {
		profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
		profile.Site.Theme.Value = value
		issues := ValidateProfile(profile)
		if !hasIssue(issues, "publish_theme_source_invalid") {
			t.Fatalf("theme %q should be rejected, got %#v", value, issues)
		}
	}
}

func TestClassifyNoteEligibilityUsesSafeDefaults(t *testing.T) {
	profile := domain.NewDefaultPublishProfile("public", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)

	selected := ClassifyNoteEligibility(profile, domain.Note{Title: "Public", Status: "active", Kind: "concept", Frontmatter: map[string]string{"publish": "public"}})
	if !selected.Selected || selected.Reason != "selected" {
		t.Fatalf("public note should be selected: %#v", selected)
	}

	for _, note := range []domain.Note{
		{Title: "Draft", Status: "draft", Kind: "concept", Frontmatter: map[string]string{"publish": "public"}},
		{Title: "Private", Status: "active", Kind: "concept", Frontmatter: map[string]string{"privacy": "private", "publish": "public"}},
		{Title: "Unpublished", Status: "active", Kind: "concept", Frontmatter: map[string]string{"publish": "false"}},
	} {
		got := ClassifyNoteEligibility(profile, note)
		if got.Selected || got.Reason == "" {
			t.Fatalf("unsafe note should be skipped with reason: %#v", got)
		}
	}
}

func TestClassifyNoteViolationsCoversPublishSafetyClasses(t *testing.T) {
	cases := []struct {
		name string
		body string
		want domain.PublishViolationClass
	}{
		{name: "secret pattern", body: "token=raw-token", want: domain.PublishViolationSecretPattern},
		{name: "provider payload", body: "raw_provider_payload: {...}", want: domain.PublishViolationProviderPayload},
		{name: "authorization header", body: "Authorization: Bearer raw-token", want: domain.PublishViolationAuthorizationHeader},
		{name: "cookie header", body: "Cookie: session=raw", want: domain.PublishViolationCookieHeader},
		{name: "webhook url", body: "webhook https://hooks.example.invalid/raw", want: domain.PublishViolationWebhookURL},
		{name: "absolute path", body: "source /home/alice/private.md", want: domain.PublishViolationAbsolutePath},
		{name: "pinax internal reference", body: "see .pinax/events/raw.jsonl", want: domain.PublishViolationPinaxInternalRef},
		{name: "private body leak", body: "private_body: internal draft", want: domain.PublishViolationPrivateBodyLeak},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations := ClassifyNoteViolations(domain.Note{Path: "notes/unsafe.md", Body: tc.body})
			if !hasViolation(violations, tc.want) {
				t.Fatalf("expected %s violation, got %#v", tc.want, violations)
			}
		})
	}
}

func TestScanPublishTreeFindsLeaksWithoutEchoingSensitiveContent(t *testing.T) {
	root := t.TempDir()
	writePublishOpsFile(t, root, "public/index.html", "Authorization: Bearer RAW_TOKEN_SENTINEL\nCookie: session=RAW_COOKIE_SENTINEL\nwebhook https://hooks.example.invalid/raw\nprovider_payload: RAW_PROVIDER_SENTINEL\nraw_body: RAW_BODY_SENTINEL\nprivate body RAW_PRIVATE_BODY_SENTINEL\n/home/alice/private.md\n")
	writePublishOpsFile(t, root, "public/.pinax/events.jsonl", "safe text\n")
	writePublishOpsFile(t, root, "public/assets/secret-token.bin", string([]byte{0x00, 0x01, 0x02, 't', 'o', 'k', 'e', 'n', '='})+"BINARY_TOKEN_SENTINEL")

	report, err := ScanPublishTree(root)
	if err != nil {
		t.Fatalf("scan publish tree: %v", err)
	}
	for _, class := range []domain.PublishViolationClass{domain.PublishViolationAuthorizationHeader, domain.PublishViolationCookieHeader, domain.PublishViolationWebhookURL, domain.PublishViolationProviderPayload, domain.PublishViolationPrivateBodyLeak, domain.PublishViolationAbsolutePath, domain.PublishViolationPinaxInternalRef, domain.PublishViolationSecretPattern} {
		if !hasTreeFinding(report.Findings, class) {
			t.Fatalf("expected %s finding, got %#v", class, report.Findings)
		}
	}
	for _, finding := range report.Findings {
		if filepath.IsAbs(finding.Path) {
			t.Fatalf("finding path should be relative: %#v", finding)
		}
		if finding.Path == "public/assets/secret-token.bin" && (!finding.Binary || finding.Size == 0 || finding.SHA256 == "") {
			t.Fatalf("binary finding missing size/hash evidence: %#v", finding)
		}
		if finding.Message == "" || containsAny(finding.Message, []string{"RAW_TOKEN_SENTINEL", "RAW_COOKIE_SENTINEL", "RAW_PROVIDER_SENTINEL", "RAW_BODY_SENTINEL", "RAW_PRIVATE_BODY_SENTINEL", "BINARY_TOKEN_SENTINEL", "/home/alice"}) {
			t.Fatalf("finding message leaked sensitive content: %#v", finding)
		}
	}
}

func TestWriteRedactedEvidenceCoversPublishSurfaces(t *testing.T) {
	root := t.TempDir()
	writePublishOpsFile(t, root, "staging/content/index.md", "token=RAW_STAGING_TOKEN\n")
	writePublishOpsFile(t, root, "pages/index.html", "Authorization: Bearer RAW_PAGES_TOKEN\n")
	writePublishOpsFile(t, root, "wiki/Home.md", "webhook https://hooks.example.invalid/wiki\n")
	evidencePath := filepath.Join(root, ".pinax", "publish", "runs", "run-1", "evidence.json")

	report, err := WriteRedactedEvidence(evidencePath, []PublishEvidenceSurface{
		{Name: "stdout", Text: "Authorization: Bearer RAW_STDOUT_TOKEN"},
		{Name: "stderr", Text: "Cookie: session=RAW_COOKIE"},
		{Name: "events", Text: `{"provider_payload":"RAW_EVENT_PAYLOAD"}`},
		{Name: "manifest", Text: "secret=RAW_MANIFEST_SECRET"},
		{Name: "receipt", Text: "private_body: RAW_RECEIPT_BODY"},
		{Name: "hugo_staging", Root: filepath.Join(root, "staging")},
		{Name: "pages_output", Root: filepath.Join(root, "pages")},
		{Name: "wiki_output", Root: filepath.Join(root, "wiki")},
	})
	if err != nil {
		t.Fatalf("write redacted evidence: %v", err)
	}
	if report.FindingsCount == 0 || len(report.Surfaces) != 8 {
		t.Fatalf("evidence report = %#v", report)
	}
	body := mustReadPublishOpsFile(t, evidencePath)
	for _, want := range []string{"stdout", "stderr", "events", "manifest", "receipt", "hugo_staging", "pages_output", "wiki_output", "authorization_header", "private_body_leak"} {
		if !strings.Contains(body, want) {
			t.Fatalf("evidence missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"RAW_STDOUT_TOKEN", "RAW_COOKIE", "RAW_EVENT_PAYLOAD", "RAW_MANIFEST_SECRET", "RAW_RECEIPT_BODY", "RAW_STAGING_TOKEN", "RAW_PAGES_TOKEN", "hooks.example.invalid"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("evidence leaked %q:\n%s", forbidden, body)
		}
	}
}

func TestWritePublishReceiptCreatesStructuredRunReceipt(t *testing.T) {
	root := t.TempDir()
	receipt := domain.PublishReceipt{
		RunID:            "run-1",
		ProfileName:      "public",
		Target:           domain.PublishTargetGitHubPages,
		Renderer:         domain.PublishRendererHugo,
		StartedAt:        "2026-06-18T00:00:00Z",
		FinishedAt:       "2026-06-18T00:00:03Z",
		VaultVersion:     "rev_abc123",
		VaultHash:        "vault_hash_123",
		Counts:           map[string]int{"selected": 2, "assets": 1, "violations": 0},
		OutputHash:       "output_hash_123",
		DurationMS:       3000,
		DeployStatus:     "not_deployed",
		RedactionSummary: map[string]string{"findings": "0"},
	}
	path, err := WritePublishReceipt(root, receipt)
	if err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	if path != filepath.ToSlash(filepath.Join(".pinax", "publish", "runs", "run-1", "receipt.json")) {
		t.Fatalf("receipt path = %q", path)
	}
	body := mustReadPublishOpsFile(t, filepath.Join(root, filepath.FromSlash(path)))
	for _, want := range []string{"pinax.publish_receipt.v1", "run-1", "public", "rev_abc123", "vault_hash_123", "output_hash_123", "not_deployed", `"duration_ms": 3000`} {
		if !strings.Contains(body, want) {
			t.Fatalf("receipt missing %q:\n%s", want, body)
		}
	}

	if _, err := WritePublishReceipt(root, domain.PublishReceipt{RunID: "../bad"}); err == nil {
		t.Fatalf("expected unsafe run id to be rejected")
	}
}

func hasIssue(issues []domain.PublishValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasViolation(violations []domain.PublishViolation, class domain.PublishViolationClass) bool {
	for _, violation := range violations {
		if violation.Class == class {
			return true
		}
	}
	return false
}

func hasTreeFinding(findings []domain.PublishScanFinding, class domain.PublishViolationClass) bool {
	for _, finding := range findings {
		if finding.Class == class {
			return true
		}
	}
	return false
}

func writePublishOpsFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadPublishOpsFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
