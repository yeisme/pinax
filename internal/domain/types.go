package domain

type Action struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

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

type Note struct {
	ID        string   `json:"id,omitempty"`
	Title     string   `json:"title"`
	Path      string   `json:"path"`
	Tags      []string `json:"tags,omitempty"`
	Body      string   `json:"body,omitempty"`
	Project   string   `json:"project,omitempty"`
	Folder    string   `json:"folder,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Status    string   `json:"status,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type PlanOperation struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
	Status string `json:"status"`
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
	NoteID      string   `json:"note_id,omitempty"`
	IssueCode   string   `json:"issue_code"`
	Reason      string   `json:"reason"`
	Status      string   `json:"status"`
	Evidence    []string `json:"evidence,omitempty"`
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
	Name          string   `json:"name"`
	Tags          []string `json:"tags,omitempty"`
	Group         string   `json:"group,omitempty"`
	Folder        string   `json:"folder,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Status        string   `json:"status,omitempty"`
	Sort          string   `json:"sort,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	CreatedAfter  string   `json:"created_after,omitempty"`
	UpdatedBefore string   `json:"updated_before,omitempty"`
	UpdatedAt     string   `json:"updated_at"`
}

type SavedViewRegistry struct {
	SchemaVersion string      `json:"schema_version"`
	Views         []SavedView `json:"views"`
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
	SavedPath     string            `json:"saved_path,omitempty"`
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
