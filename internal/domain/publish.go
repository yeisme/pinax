package domain

import "gopkg.in/yaml.v3"

const PublishProfileSchemaVersion = "pinax.publish_profile.v1"

type PublishTarget string

const (
	PublishTargetLocal       PublishTarget = "local"
	PublishTargetGitHubPages PublishTarget = "github-pages"
	PublishTargetVercel      PublishTarget = "vercel"
	PublishTargetCloudflare  PublishTarget = "cloudflare-pages"
	PublishTargetGitHubWiki  PublishTarget = "github-wiki"
	PublishTargetGitHubGist  PublishTarget = "github-gist"
	PublishTargetHTTP        PublishTarget = "http"
)

type PublishRenderer string

const (
	PublishRendererPinaxWeb PublishRenderer = "pinax-web"
	PublishRendererHugo     PublishRenderer = "hugo"
	PublishRendererNone     PublishRenderer = "none"
)

type PublishProfileMigrationPlan struct {
	Recommended  bool            `json:"recommended"`
	FromRenderer PublishRenderer `json:"from_renderer,omitempty"`
	ToRenderer   PublishRenderer `json:"to_renderer,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Command      string          `json:"command,omitempty"`
}

type PublishBodyPolicy string

const PublishBodyPolicyPublishedNotesOnly PublishBodyPolicy = "published-notes-only"

type PublishDeployMode string

const (
	PublishDeployModeNone            PublishDeployMode = "none"
	PublishDeployModeGit             PublishDeployMode = "git"
	PublishDeployModeGist            PublishDeployMode = "gist"
	PublishDeployModeHTTP            PublishDeployMode = "http"
	PublishDeployModeVercel          PublishDeployMode = "vercel"
	PublishDeployModeCloudflarePages PublishDeployMode = "cloudflare-pages"
)

type PublishThemeSource struct {
	Value           string `json:"value" yaml:"value"`
	ContractVersion string `json:"contract_version,omitempty" yaml:"contract_version,omitempty"`
}

func (s PublishThemeSource) MarshalYAML() (any, error) {
	return s.Value, nil
}

func (s *PublishThemeSource) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		s.Value = value.Value
		return nil
	}
	type rawThemeSource PublishThemeSource
	var raw rawThemeSource
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*s = PublishThemeSource(raw)
	return nil
}

type PublishThemeContract struct {
	SchemaVersion   string   `json:"schema_version" yaml:"schema_version"`
	RequiredLayouts []string `json:"required_layouts,omitempty" yaml:"required_layouts,omitempty"`
}

type PublishProfile struct {
	SchemaVersion string             `json:"schema_version" yaml:"schema_version"`
	Name          string             `json:"name" yaml:"name"`
	Target        PublishTarget      `json:"target" yaml:"target"`
	Renderer      PublishRenderer    `json:"renderer" yaml:"renderer"`
	Site          PublishSitePolicy  `json:"site" yaml:"site"`
	Selection     PublishSelection   `json:"selection" yaml:"selection"`
	BodyPolicy    PublishBodyPolicy  `json:"body_policy" yaml:"body_policy"`
	Assets        PublishAssetPolicy `json:"assets" yaml:"assets"`
	Safety        PublishSafetyGate  `json:"safety" yaml:"safety"`
	Output        PublishOutput      `json:"output" yaml:"output"`
	Deploy        PublishDeploy      `json:"deploy" yaml:"deploy"`
}

type PublishSitePolicy struct {
	Title   string             `json:"title" yaml:"title"`
	BaseURL string             `json:"base_url" yaml:"base_url"`
	Theme   PublishThemeSource `json:"theme" yaml:"theme"`
}

type PublishSelection struct {
	IncludePublishValues []string `json:"include_publish_values" yaml:"include_publish_values"`
	IncludeStatuses      []string `json:"include_statuses" yaml:"include_statuses"`
	IncludeTypes         []string `json:"include_types" yaml:"include_types"`
	ExcludePrivacyValues []string `json:"exclude_privacy_values" yaml:"exclude_privacy_values"`
}

type PublishAssetPolicy struct {
	IncludeLinkedAssets bool     `json:"include_linked_assets" yaml:"include_linked_assets"`
	AllowedExtensions   []string `json:"allowed_extensions" yaml:"allowed_extensions"`
	MaxBytes            int64    `json:"max_bytes" yaml:"max_bytes"`
}

type PublishSafetyGate struct {
	BlockSecrets        bool `json:"block_secrets" yaml:"block_secrets"`
	BlockPrivateBodies  bool `json:"block_private_bodies" yaml:"block_private_bodies"`
	BlockPinaxInternals bool `json:"block_pinax_internals" yaml:"block_pinax_internals"`
}

type PublishOutput struct {
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
}

type PublishDeploy struct {
	Mode       PublishDeployMode `json:"mode" yaml:"mode"`
	Repo       string            `json:"repo,omitempty" yaml:"repo,omitempty"`
	Branch     string            `json:"branch,omitempty" yaml:"branch,omitempty"`
	Strategy   string            `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	Endpoint   string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Method     string            `json:"method,omitempty" yaml:"method,omitempty"`
	SecretRef  string            `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
	GistID     string            `json:"gist_id,omitempty" yaml:"gist_id,omitempty"`
	Visibility string            `json:"visibility,omitempty" yaml:"visibility,omitempty"`
	Project    string            `json:"project,omitempty" yaml:"project,omitempty"`
}

type PublishValidationIssue struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
}

type PublishItem struct {
	ID         string `json:"id,omitempty"`
	Kind       string `json:"kind"`
	Title      string `json:"title,omitempty"`
	SourcePath string `json:"source_path,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type PublishSource struct {
	ID         string `json:"id,omitempty"`
	Title      string `json:"title,omitempty"`
	SourcePath string `json:"source_path"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`
	Project    string `json:"project,omitempty"`
	Folder     string `json:"folder,omitempty"`
}

type PublishViolationClass string

const (
	PublishViolationSecretPattern       PublishViolationClass = "secret_pattern"
	PublishViolationProviderPayload     PublishViolationClass = "provider_payload"
	PublishViolationAuthorizationHeader PublishViolationClass = "authorization_header"
	PublishViolationCookieHeader        PublishViolationClass = "cookie_header"
	PublishViolationWebhookURL          PublishViolationClass = "webhook_url"
	PublishViolationAbsolutePath        PublishViolationClass = "absolute_path"
	PublishViolationPinaxInternalRef    PublishViolationClass = "pinax_internal_reference"
	PublishViolationPrivateBodyLeak     PublishViolationClass = "private_body_leak"
	PublishViolationAssetNotAllowed     PublishViolationClass = "asset_not_allowed"
)

type PublishViolation struct {
	Class    PublishViolationClass `json:"class"`
	Path     string                `json:"path,omitempty"`
	Severity string                `json:"severity,omitempty"`
	Message  string                `json:"message,omitempty"`
}

type PublishPlan struct {
	ProfileName  string             `json:"profile_name"`
	Target       PublishTarget      `json:"target"`
	Renderer     PublishRenderer    `json:"renderer,omitempty"`
	Selected     []PublishItem      `json:"selected,omitempty"`
	Skipped      []PublishItem      `json:"skipped,omitempty"`
	Violations   []PublishViolation `json:"violations,omitempty"`
	ManualReview []PublishItem      `json:"manual_review,omitempty"`
	Sources      []PublishSource    `json:"sources,omitempty"`
	LinkGraph    []NoteLink         `json:"link_graph,omitempty"`
}

type PublishThemeInfo struct {
	Name            string   `json:"name"`
	Source          string   `json:"source"`
	ContractVersion string   `json:"contract_version"`
	RequiredLayouts []string `json:"required_layouts,omitempty"`
}

type PublishManifest struct {
	SchemaVersion string        `json:"schema_version"`
	ProfileName   string        `json:"profile_name"`
	Target        PublishTarget `json:"target"`
	Renderer      string        `json:"renderer,omitempty"`
	Items         []PublishItem `json:"items,omitempty"`
	OutputHash    string        `json:"output_hash,omitempty"`
}

type PublishReceipt struct {
	SchemaVersion    string            `json:"schema_version"`
	RunID            string            `json:"run_id"`
	ProfileName      string            `json:"profile_name"`
	Target           PublishTarget     `json:"target"`
	Renderer         PublishRenderer   `json:"renderer,omitempty"`
	StartedAt        string            `json:"started_at"`
	FinishedAt       string            `json:"finished_at,omitempty"`
	DurationMS       int64             `json:"duration_ms,omitempty"`
	VaultVersion     string            `json:"vault_version,omitempty"`
	VaultHash        string            `json:"vault_hash,omitempty"`
	Counts           map[string]int    `json:"counts,omitempty"`
	OutputHash       string            `json:"output_hash,omitempty"`
	RedactionSummary map[string]string `json:"redaction_summary,omitempty"`
	DeployStatus     string            `json:"deploy_status,omitempty"`
}

type PublishScanFinding struct {
	Class    PublishViolationClass `json:"class"`
	Path     string                `json:"path"`
	Severity string                `json:"severity,omitempty"`
	Message  string                `json:"message,omitempty"`
	Size     int64                 `json:"size,omitempty"`
	SHA256   string                `json:"sha256,omitempty"`
	Binary   bool                  `json:"binary,omitempty"`
}

type PublishScanReport struct {
	FilesScanned int                  `json:"files_scanned"`
	Findings     []PublishScanFinding `json:"findings,omitempty"`
}

func NewDefaultPublishProfile(name string, target PublishTarget, renderer PublishRenderer) PublishProfile {
	return PublishProfile{
		SchemaVersion: PublishProfileSchemaVersion,
		Name:          name,
		Target:        target,
		Renderer:      renderer,
		Site: PublishSitePolicy{
			Title: name,
			Theme: PublishThemeSource{Value: "builtin:pinax-encyclopedia", ContractVersion: "pinax.publish_theme.v1"},
		},
		Selection: PublishSelection{
			IncludePublishValues: []string{"public"},
			IncludeStatuses:      []string{"active"},
			IncludeTypes:         []string{"concept", "person", "org", "project", "source", "timeline", "index"},
			ExcludePrivacyValues: []string{"private", "secret"},
		},
		BodyPolicy: PublishBodyPolicyPublishedNotesOnly,
		Assets:     PublishAssetPolicy{IncludeLinkedAssets: true, AllowedExtensions: []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".pdf"}, MaxBytes: 10 << 20},
		Safety:     PublishSafetyGate{BlockSecrets: true, BlockPrivateBodies: true, BlockPinaxInternals: true},
		Deploy:     PublishDeploy{Mode: PublishDeployModeNone, Branch: "gh-pages", Strategy: "clean-worktree"},
	}
}
