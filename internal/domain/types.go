package domain

type Action struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

const AgentContextSchemaVersion = "pinax.agent_context.v1"

type AgentContext struct {
	SchemaVersion string                `json:"schema_version"`
	ContextID     string                `json:"context_id"`
	SourceKind    string                `json:"source_kind"`
	DisplayTitle  string                `json:"display_title"`
	Refs          []AgentContextRef     `json:"refs"`
	Snippets      []AgentContextSnippet `json:"snippets"`
	Evidence      []string              `json:"evidence"`
	BodyExposure  string                `json:"body_exposure"`
	Actions       []Action              `json:"actions"`
}

type AgentContextRef struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Path  string `json:"path,omitempty"`
	Title string `json:"title,omitempty"`
}

type AgentContextSnippet struct {
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

type StableErrorCode = string

const (
	ErrorCodeVaultObjectRefAmbiguous        StableErrorCode = "vault_object_ref_ambiguous"
	ErrorCodeVersionReadUnavailable         StableErrorCode = "version_read_unavailable"
	ErrorCodeVersionChangedPathsUnavailable StableErrorCode = "version_changed_paths_unavailable"
	ErrorCodeAssetNotFound                  StableErrorCode = "asset_not_found"
	ErrorCodeAssetRefAmbiguous              StableErrorCode = "asset_ref_ambiguous"
	ErrorCodeAssetPayloadForbidden          StableErrorCode = "asset_payload_forbidden"
)

type CommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

type Projection struct {
	SpecVersion string            `json:"spec_version"`
	Mode        string            `json:"mode"`
	Command     string            `json:"command"`
	Status      string            `json:"status"`
	Summary     string            `json:"summary,omitempty"`
	Facts       map[string]string `json:"facts,omitempty"`
	Actions     []Action          `json:"actions,omitempty"`
	Evidence    []string          `json:"evidence,omitempty"`
	Data        any               `json:"data,omitempty"`
	Error       *CommandError     `json:"error,omitempty"`
}

// VaultObjectKind identifies the kind of vault object returned by shared resolver paths.
type VaultObjectKind = string

const (
	VaultObjectKindNote  VaultObjectKind = "note"
	VaultObjectKindAsset VaultObjectKind = "asset"
	VaultObjectKindFile  VaultObjectKind = "file"
)

// ManagedStatus describes whether a vault object is already managed by Pinax metadata.
type ManagedStatus = string

const (
	ManagedStatusRegistered ManagedStatus = "registered"
	ManagedStatusAdoptable  ManagedStatus = "adoptable"
	ManagedStatusManaged    ManagedStatus = "managed"
	ManagedStatusUnmanaged  ManagedStatus = "unmanaged"
	ManagedStatusMissing    ManagedStatus = "missing"
)

// MatchField records which object field matched a user query.
type MatchField = string

const (
	MatchFieldNoteID   MatchField = "note_id"
	MatchFieldAssetID  MatchField = "asset_id"
	MatchFieldPath     MatchField = "path"
	MatchFieldFilename MatchField = "filename"
	MatchFieldStem     MatchField = "stem"
	MatchFieldTitle    MatchField = "title"
	MatchFieldAlias    MatchField = "alias"
	MatchFieldContent  MatchField = "content"
	MatchFieldSHA256   MatchField = "sha256"
)

// VaultObjectCandidate is the stable resolver candidate shape shared by lookup, notes, records, assets, and version plans.
type VaultObjectCandidate struct {
	ObjectKind    VaultObjectKind `json:"object_kind"`
	Path          string          `json:"path"`
	Title         string          `json:"title,omitempty"`
	NoteID        string          `json:"note_id,omitempty"`
	AssetID       string          `json:"asset_id,omitempty"`
	ManagedStatus ManagedStatus   `json:"managed_status"`
	MatchFields   []MatchField    `json:"match_fields"`
	Score         int             `json:"score"`
	MediaType     string          `json:"media_type,omitempty"`
	IndexStatus   string          `json:"index_status,omitempty"`
}

// ResolverFacts captures compact resolver evidence for command facts and JSON data.
type ResolverFacts struct {
	Query       string     `json:"query,omitempty"`
	Scope       string     `json:"scope,omitempty"`
	Kind        string     `json:"kind,omitempty"`
	Candidates  int        `json:"candidates,omitempty"`
	MatchField  MatchField `json:"match_field,omitempty"`
	Ambiguous   bool       `json:"ambiguous,omitempty"`
	IndexStatus string     `json:"index_status,omitempty"`
}

// VersionCapabilities declares which version operations a backend can serve.
type VersionCapabilities struct {
	SnapshotSupported     bool `json:"snapshot_supported"`
	ChangedPathsSupported bool `json:"changed_paths_supported"`
	ReadAtRevision        bool `json:"read_at_revision_supported"`
	DiffSupported         bool `json:"diff_supported"`
}

// VersionStatus is the stable status projection for the active vault version backend.
type VersionStatus struct {
	Backend         string              `json:"backend"`
	Capabilities    VersionCapabilities `json:"capabilities"`
	WorktreeState   string              `json:"worktree_state"`
	CurrentRevision string              `json:"current_revision,omitempty"`
	LastSnapshotID  string              `json:"last_snapshot_id,omitempty"`
	LastSnapshotAt  string              `json:"last_snapshot_at,omitempty"`
}

// VersionSnapshot records vault content evidence created by a version backend.
type VersionSnapshot struct {
	SnapshotID  string        `json:"snapshot_id"`
	Backend     string        `json:"backend"`
	Message     string        `json:"message"`
	CreatedAt   string        `json:"created_at"`
	Files       int           `json:"files"`
	Bytes       int64         `json:"bytes"`
	ContentHash string        `json:"content_hash"`
	LedgerSeq   uint64        `json:"ledger_seq,omitempty"`
	IndexEpoch  uint64        `json:"index_epoch,omitempty"`
	FileFacts   []ChangedPath `json:"file_facts,omitempty"`
	Evidence    []string      `json:"evidence"`
}

// ChangedPath describes a backend-reported changed vault path candidate.
type ChangedPath struct {
	Path         string          `json:"path"`
	ChangeKind   string          `json:"change_kind,omitempty"`
	ModifiedUnix int64           `json:"modified_unix,omitempty"`
	ObjectKind   VaultObjectKind `json:"object_kind,omitempty"`
	ContentHash  string          `json:"content_hash,omitempty"`
	SizeBytes    int64           `json:"size_bytes,omitempty"`
	Evidence     []string        `json:"evidence,omitempty"`
}

// DiffSummary is a compact version diff projection for notes, assets, and vault files.
type DiffSummary struct {
	BaseRevision   string        `json:"base_revision"`
	TargetRevision string        `json:"target_revision"`
	FilesChanged   int           `json:"files_changed"`
	Additions      int           `json:"additions,omitempty"`
	Deletions      int           `json:"deletions,omitempty"`
	ChangedPaths   []ChangedPath `json:"changed_paths,omitempty"`
}

// VersionedFile is the bounded content view for a file read through a version backend.
type VersionedFile struct {
	Path        string   `json:"path"`
	Revision    string   `json:"revision"`
	Backend     string   `json:"backend"`
	ContentHash string   `json:"content_hash,omitempty"`
	SizeBytes   int64    `json:"size_bytes,omitempty"`
	Content     string   `json:"content,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

// Asset is the stable vault asset metadata shape stored in CLI-authored manifests and projections.
type Asset struct {
	ID            string        `json:"id"`
	Path          string        `json:"path"`
	Filename      string        `json:"filename"`
	Stem          string        `json:"stem"`
	Extension     string        `json:"extension"`
	MediaType     string        `json:"media_type"`
	Size          int64         `json:"size"`
	ModifiedUnix  int64         `json:"modified_unix,omitempty"`
	Width         int           `json:"width,omitempty"`
	Height        int           `json:"height,omitempty"`
	SHA256        string        `json:"sha256"`
	ManagedStatus ManagedStatus `json:"managed_status"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	DisplayPath   string        `json:"display_path,omitempty"`
}

// AssetManifest is the CLI-authored asset registry stored under .pinax/assets/manifest.json.
type AssetManifest struct {
	SchemaVersion string  `json:"schema_version"`
	Assets        []Asset `json:"assets"`
}

// AssetLink records a Markdown or wiki reference from a note to a vault asset.
type AssetLink struct {
	AssetID      string `json:"asset_id,omitempty"`
	AssetPath    string `json:"asset_path"`
	SourceNoteID string `json:"source_note_id,omitempty"`
	SourcePath   string `json:"source_path"`
	RawReference string `json:"raw_reference"`
	LinkStyle    string `json:"link_style"`
	LinkKind     string `json:"link_kind"`
	Line         int    `json:"line,omitempty"`
	Status       string `json:"status"`
}

// AssetVerification records one asset integrity check result.
type AssetVerification struct {
	Asset  Asset  `json:"asset"`
	Status string `json:"status"`
	SHA256 string `json:"sha256,omitempty"`
}

// AssetVerifyResult is the aggregate result for asset verify commands.
type AssetVerifyResult struct {
	Verified  int                 `json:"verified"`
	Missing   int                 `json:"missing"`
	Changed   int                 `json:"changed"`
	Unmanaged int                 `json:"unmanaged"`
	Orphan    int                 `json:"orphan"`
	Failed    int                 `json:"failed"`
	Results   []AssetVerification `json:"results"`
}

// AssetOperationPlan is a no-write plan for high-risk asset moves, removes, repairs, or restores.
type AssetOperationPlan struct {
	PlanID           string          `json:"plan_id"`
	AssetID          string          `json:"asset_id,omitempty"`
	Path             string          `json:"path"`
	Operation        string          `json:"operation"`
	Risk             string          `json:"risk"`
	RequiresSnapshot bool            `json:"requires_snapshot"`
	Operations       []PlanOperation `json:"operations,omitempty"`
}

type Note struct {
	ID          string            `json:"id,omitempty"`
	Title       string            `json:"title"`
	Path        string            `json:"path"`
	Tags        []string          `json:"tags,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	Body        string            `json:"body,omitempty"`
	Frontmatter map[string]string `json:"-"`
	Project     string            `json:"project,omitempty"`
	Subproject  string            `json:"subproject,omitempty"`
	Folder      string            `json:"folder,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Status      string            `json:"status,omitempty"`
	BoardColumn string            `json:"board_column,omitempty"`
	Milestone   string            `json:"milestone,omitempty"`
	Priority    string            `json:"priority,omitempty"`
	Due         string            `json:"due,omitempty"`
	DueAt       string            `json:"due_at,omitempty"`
	BlockedBy   []string          `json:"blocked_by,omitempty"`
	CreatedAt   string            `json:"created_at,omitempty"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
}

const ProjectWorkspaceSchemaVersion = "pinax.project_workspace.v1"
const CurrentWorkspaceSchemaVersion = "pinax.current_workspace.v1"

type ProjectWorkspaceDirectory struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

type ProjectWorkspace struct {
	SchemaVersion string                      `json:"schema_version"`
	Project       string                      `json:"project"`
	Subproject    string                      `json:"subproject"`
	Title         string                      `json:"title"`
	Template      string                      `json:"template"`
	WorkspacePath string                      `json:"workspace_path"`
	Directories   []ProjectWorkspaceDirectory `json:"directories"`
	Status        string                      `json:"status"`
	CreatedAt     string                      `json:"created_at"`
	UpdatedAt     string                      `json:"updated_at"`
}

type CurrentWorkspace struct {
	SchemaVersion string `json:"schema_version"`
	Project       string `json:"project"`
	Subproject    string `json:"subproject"`
	WorkspacePath string `json:"workspace_path"`
	UpdatedAt     string `json:"updated_at"`
}

type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type PlanOperation struct {
	Kind     string   `json:"kind"`
	Path     string   `json:"path"`
	Target   string   `json:"target,omitempty"`
	Reason   string   `json:"reason"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence,omitempty"`
}

type RepairPlan struct {
	SchemaVersion      string            `json:"schema_version"`
	PlanID             string            `json:"plan_id"`
	CreatedAt          string            `json:"created_at"`
	ExpiresAt          string            `json:"expires_at"`
	VaultRoot          string            `json:"vault_root"`
	SourceCommand      string            `json:"source_command"`
	SourceFacts        map[string]string `json:"source_facts"`
	IssueSnapshot      []VaultIssue      `json:"issue_snapshot"`
	Operations         []RepairOperation `json:"operations"`
	SkippedIssues      []VaultIssue      `json:"skipped_issues,omitempty"`
	Status             string            `json:"status"`
	ScanDurationMillis int64             `json:"scan_duration_ms"`
	SavedPath          string            `json:"saved_path,omitempty"`
}

type RepairOperation struct {
	OperationID string   `json:"operation_id"`
	Kind        string   `json:"kind"`
	Mode        string   `json:"mode"`
	Risk        string   `json:"risk"`
	Path        string   `json:"path,omitempty"`
	Target      string   `json:"target,omitempty"`
	NoteID      string   `json:"note_id,omitempty"`
	IssueCode   string   `json:"issue_code"`
	Reason      string   `json:"reason"`
	Status      string   `json:"status"`
	Evidence    []string `json:"evidence,omitempty"`
}

// RestorePlan 是 version restore 生成的只读恢复计划，restore apply 据此把单个 vault
// 文件从历史 revision 安全写回本地 Markdown。它不发明内容，只复用 version backend
// 已有的历史快照，并记录 snapshot id 以校验目标 vault 与 revision 一致。
type RestorePlan struct {
	SchemaVersion  string        `json:"schema_version"`
	PlanID         string        `json:"plan_id"`
	CreatedAt      string        `json:"created_at"`
	ExpiresAt      string        `json:"expires_at"`
	VaultRoot      string        `json:"vault_root"`
	VaultHash      string        `json:"vault_hash"`
	Path           string        `json:"path"`
	Revision       string        `json:"revision"`
	GitCommit      string        `json:"git_commit,omitempty"`
	VersionBackend string        `json:"version_backend"`
	SnapshotID     string        `json:"snapshot_id,omitempty"`
	ContentHash    string        `json:"content_hash,omitempty"`
	Operation      PlanOperation `json:"operation"`
	SavedPath      string        `json:"saved_path,omitempty"`
}

type OrganizePlan struct {
	SchemaVersion string              `json:"schema_version"`
	PlanID        string              `json:"plan_id"`
	CreatedAt     string              `json:"created_at"`
	ExpiresAt     string              `json:"expires_at"`
	VaultRoot     string              `json:"vault_root"`
	SourceCommand string              `json:"source_command"`
	SourceFacts   map[string]string   `json:"source_facts"`
	Operations    []OrganizeOperation `json:"operations"`
	Status        string              `json:"status"`
	SavedPath     string              `json:"saved_path,omitempty"`
}

type OrganizeOperation struct {
	OperationID string            `json:"operation_id"`
	Kind        string            `json:"kind"`
	Mode        string            `json:"mode"`
	Risk        string            `json:"risk"`
	Path        string            `json:"path,omitempty"`
	Target      string            `json:"target,omitempty"`
	Before      map[string]string `json:"before,omitempty"`
	After       map[string]string `json:"after,omitempty"`
	Reason      string            `json:"reason"`
	Evidence    []string          `json:"evidence,omitempty"`
	Status      string            `json:"status"`
}

type OrganizePlanSummary struct {
	PlanID     string `json:"plan_id"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	Status     string `json:"status"`
	Operations int    `json:"operations"`
	SavedPath  string `json:"saved_path"`
}

type Project struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	NotesPrefix string `json:"notes_prefix"`
	CreatedAt   string `json:"created_at"`
}

type ProjectRegistry struct {
	SchemaVersion  string    `json:"schema_version"`
	CurrentProject string    `json:"current_project,omitempty"`
	Projects       []Project `json:"projects"`
}

type SavedView struct {
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name"`
	Tags          []string          `json:"tags,omitempty"`
	Group         string            `json:"group,omitempty"`
	Folder        string            `json:"folder,omitempty"`
	Kind          string            `json:"kind,omitempty"`
	Status        string            `json:"status,omitempty"`
	Sort          string            `json:"sort,omitempty"`
	Sorts         []string          `json:"sorts,omitempty"`
	Language      string            `json:"language,omitempty"`
	Query         string            `json:"query,omitempty"`
	Columns       []string          `json:"columns,omitempty"`
	GroupBy       string            `json:"group_by,omitempty"`
	CalendarField string            `json:"calendar_field,omitempty"`
	BoardColumn   string            `json:"board_column,omitempty"`
	Filters       map[string]string `json:"filters,omitempty"`
	Display       map[string]string `json:"display,omitempty"`
	Limit         int               `json:"limit,omitempty"`
	CreatedAfter  string            `json:"created_after,omitempty"`
	UpdatedBefore string            `json:"updated_before,omitempty"`
	UpdatedAt     string            `json:"updated_at"`
}

type SavedViewRegistry struct {
	SchemaVersion string      `json:"schema_version"`
	Views         []SavedView `json:"views"`
}

type FolderPurpose string

const (
	FolderPurposeNotes   FolderPurpose = "notes"
	FolderPurposeAssets  FolderPurpose = "assets"
	FolderPurposeGeneric FolderPurpose = "generic"
)

type FolderRecord struct {
	Path          string        `json:"path"`
	Purpose       FolderPurpose `json:"purpose"`
	ManagedStatus ManagedStatus `json:"managed_status"`
	CreatedAt     string        `json:"created_at,omitempty"`
	UpdatedAt     string        `json:"updated_at,omitempty"`
}

type FolderRegistry struct {
	SchemaVersion string         `json:"schema_version"`
	Folders       []FolderRecord `json:"folders"`
}

type FolderInfo struct {
	Path          string        `json:"path"`
	Purpose       FolderPurpose `json:"purpose"`
	ManagedStatus ManagedStatus `json:"managed_status"`
	Exists        bool          `json:"exists"`
	Empty         bool          `json:"empty"`
	Depth         int           `json:"depth"`
	NoteCount     int           `json:"note_count"`
	AssetCount    int           `json:"asset_count"`
	CreatedAt     string        `json:"created_at,omitempty"`
	UpdatedAt     string        `json:"updated_at,omitempty"`
}

type FolderOperationPlan struct {
	Operation string          `json:"operation"`
	Path      string          `json:"path"`
	Target    string          `json:"target,omitempty"`
	Purpose   FolderPurpose   `json:"purpose,omitempty"`
	DryRun    bool            `json:"dry_run"`
	Writes    bool            `json:"writes"`
	Effects   []PlanOperation `json:"effects,omitempty"`
}

type StorageProfile struct {
	SchemaVersion string        `json:"schema_version"`
	Backend       string        `json:"backend"`
	Local         *LocalStorage `json:"local,omitempty"`
	S3            *S3Storage    `json:"s3,omitempty"`
}

type LocalStorage struct {
	Root string `json:"root"`
}

type S3Storage struct {
	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	Prefix   string `json:"prefix,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Profile  string `json:"profile,omitempty"`
}

func NewProjection(command, summary string) Projection {
	return Projection{
		SpecVersion: "1.0",
		Command:     command,
		Status:      "success",
		Summary:     summary,
		Facts:       map[string]string{},
	}
}

func NewErrorProjection(command string, err *CommandError) Projection {
	return Projection{
		SpecVersion: "1.0",
		Command:     command,
		Status:      "failed",
		Summary:     err.Message,
		Error:       err,
		Facts:       map[string]string{},
	}
}

type VaultStats struct {
	VaultPath           string         `json:"vault_path"`
	NoteCount           int            `json:"note_count"`
	TagCount            int            `json:"tag_count"`
	DirectoryCounts     map[string]int `json:"directory_counts"`
	FrontmatterCoverage int            `json:"frontmatter_coverage"`
	RecentUpdates       int            `json:"recent_updates"`
	ScanDurationMillis  int64          `json:"scan_duration_ms"`
	IndexStatus         string         `json:"index_status"`
	IndexPath           string         `json:"index_path,omitempty"`
	Notes               []NoteStat     `json:"notes,omitempty"`
}

type NoteStat struct {
	ID             string   `json:"id,omitempty"`
	Title          string   `json:"title"`
	Path           string   `json:"path"`
	Tags           []string `json:"tags,omitempty"`
	HasFrontmatter bool     `json:"has_frontmatter"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	SizeBytes      int64    `json:"size_bytes"`
}

type DimensionCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// LinkKind 枚举链接类型。
type LinkKind string

const (
	LinkKindWiki     LinkKind = "wiki"
	LinkKindMarkdown LinkKind = "markdown"
)

// ValidLinkKinds 返回所有合法 kind 值，用于校验。
func ValidLinkKinds() []LinkKind {
	return []LinkKind{LinkKindWiki, LinkKindMarkdown}
}

// IsValidLinkKind 校验 kind 是否在允许列表中。
func IsValidLinkKind(kind string) bool {
	for _, k := range ValidLinkKinds() {
		if string(k) == kind {
			return true
		}
	}
	return false
}

// LinkStatus 枚举链接解析状态。
type LinkStatus string

const (
	LinkStatusResolved  LinkStatus = "resolved"
	LinkStatusBroken    LinkStatus = "broken"
	LinkStatusAmbiguous LinkStatus = "ambiguous"
	LinkStatusExternal  LinkStatus = "external"
	LinkStatusIgnored   LinkStatus = "ignored"
)

// ValidLinkStatuses 返回所有合法 status 值，用于校验。
func ValidLinkStatuses() []LinkStatus {
	return []LinkStatus{LinkStatusResolved, LinkStatusBroken, LinkStatusAmbiguous, LinkStatusExternal, LinkStatusIgnored}
}

// IsValidLinkStatus 校验 status 是否在允许列表中。
func IsValidLinkStatus(status string) bool {
	for _, s := range ValidLinkStatuses() {
		if string(s) == status {
			return true
		}
	}
	return false
}

// NoteLink 描述单条双联边。Broken 字段保持向后兼容；
// 新代码应优先读取 Status 字段判断解析结果。
type NoteLink struct {
	SourcePath  string `json:"source_path"`
	SourceTitle string `json:"source_title"`
	Target      string `json:"target"`
	TargetPath  string `json:"target_path,omitempty"`
	TargetTitle string `json:"target_title,omitempty"`
	Kind        string `json:"kind"`
	Broken      bool   `json:"broken"`

	// 扩展字段：双联图谱增强。
	SourceNoteID  string              `json:"source_note_id,omitempty"`
	TargetNoteID  string              `json:"target_note_id,omitempty"`
	TargetRaw     string              `json:"target_raw,omitempty"`
	TargetAlias   string              `json:"target_alias,omitempty"`
	TargetHeading string              `json:"target_heading,omitempty"`
	Status        string              `json:"status,omitempty"`
	Line          int                 `json:"line,omitempty"`
	Evidence      string              `json:"evidence,omitempty"`
	Candidates    []NoteLinkCandidate `json:"candidates,omitempty"`
}

// NoteLinkCandidate 描述歧义链接的候选目标。
type NoteLinkCandidate struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	NoteID string `json:"note_id,omitempty"`
}

// NoteGraphProjection 描述图谱查询的完整投影。
type NoteGraphProjection struct {
	Engine      string            `json:"engine"`
	IndexStatus string            `json:"index_status,omitempty"`
	TotalNotes  int               `json:"total_notes"`
	TotalLinks  int               `json:"total_links"`
	Resolved    int               `json:"resolved"`
	Broken      int               `json:"broken"`
	Ambiguous   int               `json:"ambiguous"`
	Ignored     int               `json:"ignored"`
	Orphans     int               `json:"orphans"`
	Facts       map[string]string `json:"facts,omitempty"`
	NextActions []Action          `json:"next_actions,omitempty"`
}

type NoteAttachment struct {
	NotePath      string `json:"note_path"`
	ReferenceText string `json:"reference_text"`
	TargetPath    string `json:"target_path"`
	Path          string `json:"path,omitempty"`
	DisplayPath   string `json:"display_path,omitempty"`
	MediaType     string `json:"media_type"`
	Exists        bool   `json:"exists"`
}

type ImportPlan struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	Status     string `json:"status"`
	Conflict   string `json:"conflict,omitempty"`
}

type VaultDoctorReport struct {
	VaultPath string         `json:"vault_path"`
	Issues    []VaultIssue   `json:"issues"`
	Counts    map[string]int `json:"counts"`
	Stats     VaultStats     `json:"stats"`
}

type VaultIssue struct {
	Code        string   `json:"issue_code"`
	Severity    string   `json:"severity"`
	Path        string   `json:"path,omitempty"`
	NoteID      string   `json:"note_id,omitempty"`
	Message     string   `json:"message"`
	Evidence    []string `json:"evidence,omitempty"`
	NextActions []Action `json:"next_actions,omitempty"`
}

// BackendKind 枚举后端类型。允许 local、s3、rclone、onedrive 和未来 pinax-cloud。
type BackendKind string

const (
	BackendLocal      BackendKind = "local"
	BackendS3         BackendKind = "s3"
	BackendRclone     BackendKind = "rclone"
	BackendOneDrive   BackendKind = "onedrive"
	BackendPinaxCloud BackendKind = "pinax-cloud"
)

// ValidBackendKinds 返回所有合法 kind 值，用于校验。
func ValidBackendKinds() []BackendKind {
	return []BackendKind{BackendLocal, BackendS3, BackendRclone, BackendOneDrive, BackendPinaxCloud}
}

// IsValidBackendKind 校验 kind 是否在允许列表中。
func IsValidBackendKind(kind string) bool {
	for _, k := range ValidBackendKinds() {
		if string(k) == kind {
			return true
		}
	}
	return false
}

// BackendProfile 描述单个后端配置。CLI service 写入 .pinax/backends.json。
type BackendProfile struct {
	Name             string      `json:"name"`
	Kind             BackendKind `json:"kind"`
	Root             string      `json:"root,omitempty"`
	Bucket           string      `json:"bucket,omitempty"`
	Region           string      `json:"region,omitempty"`
	Prefix           string      `json:"prefix,omitempty"`
	Endpoint         string      `json:"endpoint,omitempty"`
	Profile          string      `json:"profile,omitempty"`
	Remote           string      `json:"remote,omitempty"`
	CredentialSource string      `json:"credential_source,omitempty"`
	Capabilities     []string    `json:"capabilities"`
	CreatedAt        string      `json:"created_at"`
	UpdatedAt        string      `json:"updated_at"`
}

// BackendRegistry 描述 .pinax/backends.json 的完整 schema。
type BackendRegistry struct {
	SchemaVersion  string           `json:"schema_version"`
	DefaultBackend string           `json:"default_backend,omitempty"`
	Backends       []BackendProfile `json:"backends"`
}

// BackendCapability 表示后端支持的单项能力。
type BackendCapability struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Supported   bool   `json:"supported"`
}

// BackendDiffItem 描述 diff/push/pull 计划中的单个文件操作。
type BackendDiffItem struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"` // create, update, delete, conflict
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// SyncConflictEntry describes a local conflict sidecar produced by sync pull.
type SyncConflictEntry struct {
	File     string `json:"file"`
	MainPath string `json:"main_path"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// SyncConflictDetail contains conflict inspection data. Machine/event renderers should
// prefer paths and diff metadata over raw bodies unless a user explicitly requested show/diff.
type SyncConflictDetail struct {
	Conflict SyncConflictEntry `json:"conflict"`
	Diff     string            `json:"diff,omitempty"`
	MainBody string            `json:"main_body,omitempty"`
	Body     string            `json:"body,omitempty"`
}

// SyncConflictResolutionReceipt is the safe receipt persisted after a conflict resolution.
type SyncConflictResolutionReceipt struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	ConflictFile  string `json:"conflict_file"`
	MainPath      string `json:"main_path"`
	Resolution    string `json:"resolution"`
	ReceiptPath   string `json:"receipt_path"`
	CreatedAt     string `json:"created_at"`
}

// BackendPlan 描述后端同步计划。
type BackendPlan struct {
	SchemaVersion string            `json:"schema_version"`
	PlanID        string            `json:"plan_id"`
	BackendName   string            `json:"backend_name"`
	Direction     string            `json:"direction"` // push, pull
	Items         []BackendDiffItem `json:"items"`
	ConflictCount int               `json:"conflict_count"`
	TotalCount    int               `json:"total_count"`
	DryRun        bool              `json:"dry_run"`
	Status        string            `json:"status"` // planned, applied, failed
	CreatedAt     string            `json:"created_at"`
}

// PlanningPeriod 枚举计划期间。
type PlanningPeriod string

const (
	PlanningDaily   PlanningPeriod = "daily"
	PlanningWeekly  PlanningPeriod = "weekly"
	PlanningMonthly PlanningPeriod = "monthly"
)

// PlanningSnapshot 记录计划快照，由 service 写入 .pinax/planning/snapshots/。
type PlanningSnapshot struct {
	SchemaVersion string            `json:"schema_version"`
	SnapshotID    string            `json:"snapshot_id"`
	Source        string            `json:"source"`
	CapturedAt    string            `json:"captured_at"`
	Facts         map[string]string `json:"facts"`
	Risks         []PlanningRisk    `json:"risks,omitempty"`
	TaskBridge    *TaskBridgePlan   `json:"taskbridge,omitempty"`
	SavedPath     string            `json:"saved_path,omitempty"`
}

// TaskBridgePlan records normalized daily task facts captured from TaskBridge.
type TaskBridgePlan struct {
	SchemaVersion string                 `json:"schema_version"`
	CapturedAt    string                 `json:"captured_at"`
	Date          string                 `json:"date"`
	Status        string                 `json:"status"`
	Summary       map[string]int         `json:"summary,omitempty"`
	Tasks         []TaskBridgePlanTask   `json:"tasks,omitempty"`
	Actions       []TaskBridgePlanAction `json:"actions,omitempty"`
	Warnings      []string               `json:"warnings,omitempty"`
}

// TaskBridgePlanTask is the bounded task fact shape Pinax stores in planning snapshots.
type TaskBridgePlanTask struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status,omitempty"`
	Source       string `json:"source,omitempty"`
	Priority     string `json:"priority,omitempty"`
	Reason       string `json:"reason,omitempty"`
	SectionID    string `json:"section_id,omitempty"`
	SectionTitle string `json:"section_title,omitempty"`
}

// TaskBridgePlanAction records suggested TaskBridge actions without executing them.
type TaskBridgePlanAction struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	TaskID               string `json:"task_id"`
	Reason               string `json:"reason,omitempty"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

// PlanningRisk 记录计划风险项。
type PlanningRisk struct {
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Evidence []string `json:"evidence,omitempty"`
}

// PlanningDecision 记录计划决策。
type PlanningDecision struct {
	SchemaVersion string           `json:"schema_version"`
	DecisionID    string           `json:"decision_id"`
	Period        PlanningPeriod   `json:"period"`
	Selected      []string         `json:"selected"`
	Deferred      []string         `json:"deferred,omitempty"`
	Reasons       []PlanningReason `json:"reasons,omitempty"`
	NextActions   []Action         `json:"next_actions,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

// PlanningReason 记录选择/推迟原因。
type PlanningReason struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

// PlanningActionDraft 记录 TaskBridge action file 草稿。
type PlanningActionDraft struct {
	SchemaVersion        string            `json:"schema_version"`
	ActionID             string            `json:"action_id"`
	SourcePeriod         string            `json:"source_period"`
	SourceDecision       string            `json:"source_decision"`
	SourceSnapshot       string            `json:"source_snapshot"`
	RequiresConfirmation bool              `json:"requires_confirmation"`
	Tasks                []ActionDraftTask `json:"tasks"`
	EvidenceRefs         []string          `json:"evidence_refs,omitempty"`
	SavedPath            string            `json:"saved_path,omitempty"`
	CreatedAt            string            `json:"created_at"`
}

// ActionDraftTask 记录单条 action 草稿。
type ActionDraftTask struct {
	ActionID             string `json:"action_id"`
	TaskID               string `json:"task_id"`
	Title                string `json:"title,omitempty"`
	Kind                 string `json:"kind"`
	Priority             string `json:"priority,omitempty"`
	ProjectSlug          string `json:"project_slug,omitempty"`
	Reason               string `json:"reason,omitempty"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}
