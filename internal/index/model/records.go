// Package model 定义 internal/index 本地索引投影的全部 GORM 模型。
//
// Markdown vault 是真源，SQLite 索引只是可重建投影。这里只放纯数据模型，
// 不放业务查询逻辑：普通业务读写必须通过 internal/index/query 生成的类型化 DAO，
// GORM runtime 只保留连接、迁移、事务和少数集中 helper。
package model

// IndexMetaRecord 记录索引 schema 版本与重建时间等元信息。
type IndexMetaRecord struct {
	Key       string `gorm:"primaryKey"`
	Value     string
	UpdatedAt string
}

// NoteRecord 是 vault note 在本地索引中的核心投影行。
type NoteRecord struct {
	Path            string `gorm:"primaryKey"`
	NoteID          string `gorm:"index"`
	Title           string
	Filename        string `gorm:"index"`
	Stem            string `gorm:"index"`
	ObjectKind      string `gorm:"index"`
	ManagedStatus   string `gorm:"index"`
	Project         string
	Group           string `gorm:"index"`
	Folder          string `gorm:"index"`
	Kind            string `gorm:"index"`
	Status          string `gorm:"index"`
	LifecycleStatus string `gorm:"index"`
	CreatedAt       string
	UpdatedAt       string
	SourceHash      string
	ModifiedUnix    int64
	Size            int64
	IsSystem        bool `gorm:"index"`
}

// NoteTextRecord 保存 note 的正文文本投影，用于搜索摘要。
type NoteTextRecord struct {
	NotePath  string `gorm:"primaryKey"`
	TitleText string
	BodyText  string
	Excerpt   string
	WordCount int
}

// TagRecord 记录 note 与 tag 的多值关系。
type TagRecord struct {
	ID       uint `gorm:"primaryKey"`
	NotePath string
	Tag      string `gorm:"index"`
}

// LinkRecord 记录 note 之间的 wiki/markdown 链接及其解析状态。
type LinkRecord struct {
	ID            uint `gorm:"primaryKey"`
	NotePath      string
	Target        string `gorm:"index"`
	TargetPath    string `gorm:"index"`
	Kind          string
	Broken        bool `gorm:"index"`
	SourceNoteID  string
	TargetNoteID  string
	TargetTitle   string
	TargetRaw     string
	TargetAlias   string
	TargetHeading string
	Status        string `gorm:"index"` // resolved|broken|ambiguous|external|ignored
	Line          int
	Evidence      string
}

// SearchTokenRecord 保存 note 的分词倒排索引。
type SearchTokenRecord struct {
	ID       uint   `gorm:"primaryKey"`
	Token    string `gorm:"index"`
	NotePath string `gorm:"index"`
	Field    string
	Count    int
	Weight   int
}

// AttachmentRecord 记录 note 引用的附件及其存在性。
type AttachmentRecord struct {
	ID            uint `gorm:"primaryKey"`
	NotePath      string
	ReferenceText string
	TargetPath    string `gorm:"index"`
	MediaType     string
	Exists        bool `gorm:"index"`
}

// AssetRecord 是 vault 资产文件在索引中的投影行。
type AssetRecord struct {
	Path          string `gorm:"primaryKey"`
	AssetID       string `gorm:"index"`
	Filename      string `gorm:"index"`
	Stem          string `gorm:"index"`
	Extension     string `gorm:"index"`
	MediaType     string `gorm:"index"`
	Size          int64
	ModifiedUnix  int64
	Width         int
	Height        int
	SHA256        string `gorm:"index"`
	ManagedStatus string `gorm:"index"`
	CreatedAt     string
	UpdatedAt     string
}

// AssetLinkRecord 记录 note 到资产的引用边及其状态。
type AssetLinkRecord struct {
	ID           uint   `gorm:"primaryKey"`
	AssetPath    string `gorm:"index"`
	SourceNoteID string `gorm:"index"`
	SourcePath   string `gorm:"index"`
	RawReference string
	LinkStyle    string `gorm:"index"`
	LinkKind     string `gorm:"index"`
	Line         int
	Status       string `gorm:"index"`
	MediaType    string `gorm:"index"`
}

// VaultFileRecord 是 vault 中全部文件（note 与 asset）的统一投影。
type VaultFileRecord struct {
	Path          string `gorm:"primaryKey"`
	Filename      string `gorm:"index"`
	Stem          string `gorm:"index"`
	Extension     string `gorm:"index"`
	MediaType     string `gorm:"index"`
	Size          int64
	ModifiedUnix  int64
	ObjectKind    string `gorm:"index"`
	ManagedStatus string `gorm:"index"`
}

// FolderRecord 记录 vault 文件夹结构与用途。
type FolderRecord struct {
	Path          string `gorm:"primaryKey"`
	Purpose       string `gorm:"index"`
	ManagedStatus string `gorm:"index"`
	Exists        bool   `gorm:"index"`
	Empty         bool   `gorm:"index"`
	Depth         int    `gorm:"index"`
	NoteCount     int
	AssetCount    int
	CreatedAt     string
	UpdatedAt     string
}

// DimensionCountRecord 记录 tag/group/folder/kind/status 维度聚合计数。
type DimensionCountRecord struct {
	ID        uint   `gorm:"primaryKey"`
	Dimension string `gorm:"index"`
	Value     string `gorm:"index"`
	Count     int
}

// PropertyDefinitionRecord 记录属性定义推断结果。
type PropertyDefinitionRecord struct {
	Name    string `gorm:"primaryKey"`
	Type    string `gorm:"index"`
	Source  string `gorm:"index"`
	Count   int
	Samples string
}

// PropertyValueRecord 记录每个 note 的属性取值。
type PropertyValueRecord struct {
	ID       uint   `gorm:"primaryKey"`
	NotePath string `gorm:"index"`
	Name     string `gorm:"index"`
	Type     string `gorm:"index"`
	Raw      string
	Value    string `gorm:"index"`
	Source   string `gorm:"index"`
}

// PromptAssetRecord stores the stable searchable projection for a reusable prompt asset.
type PromptAssetRecord struct {
	PromptAssetID      string `gorm:"primaryKey"`
	SchemaVersion      string `gorm:"index"`
	Title              string
	Domain             string `gorm:"index"`
	Lifecycle          string `gorm:"index"`
	Permission         string `gorm:"index"`
	OwnerProject       string `gorm:"index"`
	CurrentVersionID   string `gorm:"index"`
	PromptTemplateHash string `gorm:"index"`
	TagsJSON           string
	CreatedAt          string
	UpdatedAt          string
}

// PromptAssetVersionRecord stores versioned prompt body metadata for one prompt asset.
type PromptAssetVersionRecord struct {
	VersionID          string `gorm:"primaryKey"`
	PromptAssetID      string `gorm:"index"`
	PromptTemplate     string
	PromptTemplateHash string `gorm:"index"`
	VariablesJSON      string
	ConstraintsJSON    string
	ReviewGuidance     string
	CreatedAt          string
}

// PromptAssetSourceRefRecord links a prompt asset to note, URI, or evidence references.
type PromptAssetSourceRefRecord struct {
	ID            uint   `gorm:"primaryKey"`
	PromptAssetID string `gorm:"index"`
	VersionID     string `gorm:"index"`
	URI           string `gorm:"index"`
	Label         string
	Evidence      string
}

// PromptUsageFeedbackRecord records imported usage feedback without letting external projects mutate lifecycle directly.
type PromptUsageFeedbackRecord struct {
	FeedbackID         string `gorm:"primaryKey"`
	PromptAssetID      string `gorm:"index"`
	VersionID          string `gorm:"index"`
	PromptTemplateHash string `gorm:"index"`
	ExternalRunRef     string `gorm:"index"`
	Decision           string `gorm:"index"`
	Reason             string
	ArtifactRefsJSON   string
	ImportedAt         string
}

// AllModels 返回索引全部 GORM 模型，供 AutoMigrate 与 GORM Gen 复用。
func AllModels() []any {
	return []any{
		&IndexMetaRecord{},
		&NoteRecord{},
		&NoteTextRecord{},
		&TagRecord{},
		&LinkRecord{},
		&SearchTokenRecord{},
		&AttachmentRecord{},
		&AssetRecord{},
		&AssetLinkRecord{},
		&VaultFileRecord{},
		&FolderRecord{},
		&DimensionCountRecord{},
		&PropertyDefinitionRecord{},
		&PropertyValueRecord{},
		&PromptAssetRecord{},
		&PromptAssetVersionRecord{},
		&PromptAssetSourceRefRecord{},
		&PromptUsageFeedbackRecord{},
	}
}
