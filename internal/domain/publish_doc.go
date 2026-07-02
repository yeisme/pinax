package domain

const PublishDocProfileSchemaVersion = "pinax.publish_doc_profile.v1"
const PublishDocPackageSchemaVersion = "pinax.publish_doc_package.v1"
const PublishDocMappingSchemaVersion = "pinax.publish_doc_mapping.v1"
const PublishDocReceiptSchemaVersion = "pinax.publish_doc_receipt.v1"

type PublishDocTarget string

const (
	PublishDocTargetNotionPage PublishDocTarget = "notion-page"
	PublishDocTargetLarkDoc    PublishDocTarget = "lark-doc"
)

type PublishDocStatus string

const (
	PublishDocStatusPackagePrepared PublishDocStatus = "package_prepared"
	PublishDocStatusDryRunVerified  PublishDocStatus = "dry_run_verified"
	PublishDocStatusPublished       PublishDocStatus = "published"
	PublishDocStatusLinked          PublishDocStatus = "linked"
	PublishDocStatusStale           PublishDocStatus = "stale"
	PublishDocStatusFailed          PublishDocStatus = "failed"
	PublishDocStatusDetached        PublishDocStatus = "detached"
)

type PublishDocProfile struct {
	SchemaVersion string                    `json:"schema_version" yaml:"schema_version"`
	Target        PublishDocTarget          `json:"target" yaml:"target"`
	Provider      string                    `json:"provider" yaml:"provider"`
	As            string                    `json:"as,omitempty" yaml:"as,omitempty"`
	Workspace     string                    `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	ParentPage    string                    `json:"parent_page,omitempty" yaml:"parent_page,omitempty"`
	Space         string                    `json:"space,omitempty" yaml:"space,omitempty"`
	Folder        string                    `json:"folder,omitempty" yaml:"folder,omitempty"`
	Layout        string                    `json:"layout,omitempty" yaml:"layout,omitempty"`
	Template      string                    `json:"template,omitempty" yaml:"template,omitempty"`
	IndexPage     bool                      `json:"index_page,omitempty" yaml:"index_page,omitempty"`
	IndexObject   *PublishDocExternalObject `json:"index_object,omitempty" yaml:"index_object,omitempty"`
}

type PublishDocPackage struct {
	SchemaVersion string           `json:"schema_version"`
	ID            string           `json:"id"`
	NoteID        string           `json:"note_id"`
	NotePath      string           `json:"note_path"`
	Target        PublishDocTarget `json:"target"`
	Title         string           `json:"title"`
	ContentDigest string           `json:"content_digest"`
	BodyMarkdown  string           `json:"body_markdown,omitempty"`
	RemotePath    string           `json:"remote_path,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

type PublishDocExternalObject struct {
	Provider string `json:"provider"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	URL      string `json:"url,omitempty"`
}

type PublishDocMapping struct {
	SchemaVersion     string                   `json:"schema_version"`
	NoteID            string                   `json:"note_id"`
	Target            PublishDocTarget         `json:"target"`
	Provider          string                   `json:"provider"`
	ExternalObject    PublishDocExternalObject `json:"external_object"`
	RemotePath        string                   `json:"remote_path,omitempty"`
	RemoteFolderToken string                   `json:"remote_folder_token,omitempty"`
	ContentDigest     string                   `json:"content_digest"`
	PublishStatus     PublishDocStatus         `json:"publish_status"`
	LastPublishedAt   string                   `json:"last_published_at,omitempty"`
	UpdatedAt         string                   `json:"updated_at"`
}

type PublishDocFolderMapping struct {
	SchemaVersion string           `json:"schema_version"`
	Target        PublishDocTarget `json:"target"`
	Provider      string           `json:"provider"`
	RemotePath    string           `json:"remote_path"`
	FolderToken   string           `json:"folder_token"`
	UpdatedAt     string           `json:"updated_at"`
}

type PublishDocReceipt struct {
	SchemaVersion string           `json:"schema_version"`
	RunID         string           `json:"run_id"`
	Command       string           `json:"command"`
	NoteID        string           `json:"note_id,omitempty"`
	PackageID     string           `json:"package_id,omitempty"`
	Target        PublishDocTarget `json:"target,omitempty"`
	Status        string           `json:"status"`
	StartedAt     string           `json:"started_at"`
	FinishedAt    string           `json:"finished_at"`
	ExternalURL   string           `json:"external_url,omitempty"`
}

func NewPublishDocProfile(target PublishDocTarget) PublishDocProfile {
	profile := PublishDocProfile{SchemaVersion: PublishDocProfileSchemaVersion, Target: target}
	switch target {
	case PublishDocTargetLarkDoc:
		profile.Provider = "lark"
		profile.Layout = "mirror"
		profile.Template = "vault"
		profile.IndexPage = true
	case PublishDocTargetNotionPage:
		profile.Provider = "notion"
	}
	return profile
}
