package domain

import "encoding/json"

const PublishDocProfileSchemaVersion = "pinax.publish_doc_profile.v1"
const PublishDocPackageSchemaVersion = "pinax.publish_doc_package.v1"
const PublishDocMappingSchemaVersion = "pinax.publish_doc_mapping.v1"
const PublishDocReceiptSchemaVersion = "pinax.publish_doc_receipt.v1"

// PublishDocRenderRevision 标记原生文档渲染产物的 schema revision。
// 当 native render plan 结构变化时递增，供 mapping 兼容读取判断。
const PublishDocRenderRevision = "pinax.publish.render.v1"

type PublishDocTarget string

const (
	PublishDocTargetNotionPage PublishDocTarget = "notion-page"
	PublishDocTargetLarkDoc    PublishDocTarget = "lark-doc"
)

// PublishDocRenderer 描述 lark-doc 发布使用哪种远端渲染器。
// native-docx 生成飞书原生 Docs/Docx；markdown-file 保留旧的 Drive .md file 上传。
type PublishDocRenderer string

const (
	// PublishDocRendererNativeDocx 是新 lark-doc profile 的默认渲染器，
	// 产出飞书原生 docx/doc 对象而非 Drive file。
	PublishDocRendererNativeDocx PublishDocRenderer = "native-docx"
	// PublishDocRendererMarkdownFile 是 legacy/fallback 渲染器，
	// 仅在 provider 缺少原生文档能力或用户显式选择时使用。
	PublishDocRendererMarkdownFile PublishDocRenderer = "markdown-file"
)

// PublishDocRenderWarning 记录原生渲染过程中无法完美表达的内容。
// 所有不支持或回退的内容必须产生稳定 warning code，不得静默丢弃。
type PublishDocRenderWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// PublishDocCrossDocLink records how one local note reference is handled during
// native document publishing. The raw local body remains the source of truth;
// this is package/run evidence for rewritten or unresolved cross-doc links.
type PublishDocCrossDocLink struct {
	Kind         string   `json:"kind"`
	Raw          string   `json:"raw"`
	Label        string   `json:"label,omitempty"`
	Status       string   `json:"status"`
	SourceNoteID string   `json:"source_note_id"`
	TargetNoteID string   `json:"target_note_id,omitempty"`
	TargetTitle  string   `json:"target_title,omitempty"`
	URL          string   `json:"url,omitempty"`
	Occurrences  int      `json:"occurrences,omitempty"`
	Candidates   []string `json:"candidates,omitempty"`
}

// PublishDocCrossDocSummary aggregates vault-local note references found while
// preparing a publish package.
type PublishDocCrossDocSummary struct {
	Total       int                      `json:"total"`
	Rewritten   int                      `json:"rewritten"`
	Unpublished int                      `json:"unpublished"`
	Ambiguous   int                      `json:"ambiguous"`
	Broken      int                      `json:"broken"`
	Self        int                      `json:"self"`
	Links       []PublishDocCrossDocLink `json:"links,omitempty"`
}

// 远端对象类型。docx/doc 为飞书原生文档；file 为 Drive 文件（仅 markdown-file renderer 产生）。
const (
	PublishDocObjectTypeDocx = "docx"
	PublishDocObjectTypeDoc  = "doc"
	PublishDocObjectTypeFile = "file"
	PublishDocObjectTypePage = "page"
)

// 渲染 warning code 常量，供 package/receipt/mapping/output 统一引用。
const (
	PublishDocWarningMermaidUnavailable = "mermaid_render_unavailable"
	PublishDocWarningSVGUnavailable     = "svg_render_unavailable"
	PublishDocWarningUnsupportedHTML    = "unsupported_html_omitted"
	PublishDocWarningRemoteImageLinked  = "remote_image_linked"
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
	// Renderer 是 additive 字段：新 lark-doc profile 默认 native-docx；
	// 旧 profile 缺字段时按 target 推断（lark-doc→markdown-file 兼容读取）。
	Renderer PublishDocRenderer `json:"renderer,omitempty" yaml:"renderer,omitempty"`
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
	// 以下为 native renderer additive 字段。markdown-file renderer 不填。
	Renderer       PublishDocRenderer         `json:"renderer,omitempty"`
	RenderRevision string                     `json:"render_revision,omitempty"`
	RenderWarnings []PublishDocRenderWarning  `json:"render_warnings,omitempty"`
	NativePlan     json.RawMessage            `json:"native_plan,omitempty"` // provider-neutral plan JSON（raw，可读）
	CrossDocLinks  *PublishDocCrossDocSummary `json:"cross_doc_links,omitempty"`
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
	// 以下为 additive 字段。旧 mapping 缺字段时仍可读取。
	// Renderer 为空时按 ExternalObject.Type 推断（file→markdown-file，docx/doc→native-docx）。
	Renderer          PublishDocRenderer        `json:"renderer,omitempty"`
	RenderRevision    string                    `json:"render_revision,omitempty"`
	RenderWarnings    []PublishDocRenderWarning `json:"render_warnings,omitempty"`
	RenderAssetBlocks []string                  `json:"render_asset_blocks,omitempty"`
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
		// 新 lark-doc profile 默认生成飞书原生文档而非 Drive .md file。
		profile.Renderer = PublishDocRendererNativeDocx
	case PublishDocTargetNotionPage:
		profile.Provider = "notion"
	}
	return profile
}

// ResolveDocRenderer 推断 profile 的有效 renderer。
// 新 profile 由 NewPublishDocProfile 显式写入 native-docx，不会缺字段。
// 旧 profile（磁盘上已存在、无 renderer 字段）按 markdown-file 读取以保持兼容；
// 用户可用 `profile set --renderer native-docx` 显式迁移。
func (p PublishDocProfile) ResolveDocRenderer() PublishDocRenderer {
	if p.Renderer != "" {
		return p.Renderer
	}
	if p.Target == PublishDocTargetLarkDoc {
		return PublishDocRendererMarkdownFile
	}
	return ""
}

// ResolveDocMappingRenderer 推断 mapping 的有效 renderer。
// 旧 mapping 缺 Renderer 时按 ExternalObject.Type 反推：file→markdown-file，其余→native-docx。
func (m PublishDocMapping) ResolveDocMappingRenderer() PublishDocRenderer {
	if m.Renderer != "" {
		return m.Renderer
	}
	switch m.ExternalObject.Type {
	case PublishDocObjectTypeFile:
		return PublishDocRendererMarkdownFile
	case PublishDocObjectTypeDocx, PublishDocObjectTypeDoc, "document":
		return PublishDocRendererNativeDocx
	}
	if m.Target == PublishDocTargetLarkDoc {
		return PublishDocRendererNativeDocx
	}
	return ""
}
