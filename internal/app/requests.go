package app

import (
	"time"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/domain"
)

// Request DTOs for app.Service operations. Each command family has a dedicated
// request type so the Service facade keeps a stable, typed entry surface.

type InitVaultRequest struct {
	VaultPath string
	Title     string
}

type VaultRequest struct {
	VaultPath string
	Query     string
	// TrustFields 开启 metadata plan/apply 的 trust_fields 回填（显式 opt-in，
	// 默认 plan 输出与既有行为完全一致）。
	TrustFields bool
	StaleAfter  string
	AgentVersion string
}

type VaultIgnoreRequest struct {
	VaultPath string
	Yes       bool
}

type IndexRefreshRequest struct {
	VaultPath    string
	ChangedSince string
}

type IndexLookupRequest struct {
	VaultPath string
	Query     string
	Scope     string
	Kind      string
}

type VaultObjectCandidate = domain.VaultObjectCandidate

type AssetRequest struct {
	VaultPath       string
	Source          string
	Ref             string
	Target          string
	PathStyle       string
	ContextNote     string
	IncludePaths    bool
	PreviewAs       string
	MaxPreviewBytes int
}

type PromptAssetRequest struct {
	VaultPath string
	From      string
	Ref       string
	Query     string
	Domain    string
	Tag       string
	Lifecycle string
	To        string
	Reason    string
	Limit     int
}

type IndexRepairRequest struct {
	VaultPath string
	Kind      string
	DryRun    bool
	Yes       bool
}

type VaultStatsRequest struct {
	VaultPath string
}

type VaultDoctorRequest struct {
	VaultPath  string
	StaleAfter time.Duration
}

type RepairPlanRequest struct {
	VaultPath string
	Save      bool
}

type RepairApplyRequest struct {
	VaultPath       string
	PlanID          string
	Yes             bool
	SnapshotMessage string
}

type OrganizeSuggestRequest struct {
	VaultPath string
	Save      bool
}

type DailyRequest struct {
	VaultPath string
	Editor    string
	Body      string
	Date      string
	Prev      bool
	Template  string
	Next      bool
}

type InboxTriageRequest struct {
	VaultPath    string
	NoteRef      string
	PathStyle    string
	IncludePaths bool
	Group        string
	Folder       string
	Kind         string
	Status       string
}

type ViewRequest struct {
	VaultPath     string
	Name          string
	Tags          []string
	Group         string
	Folder        string
	Kind          string
	Status        string
	Sort          string
	Limit         int
	CreatedAfter  string
	UpdatedBefore string
	Yes           bool
	Language      string
	Query         string
	Columns       []string
	Display       string
	GroupBy       string
	CalendarField string
	BoardColumn   string
}

type DatabaseSchemaRequest struct {
	VaultPath string
	Name      string
	Type      string
	Values    []string
}

type QueryRequest struct {
	VaultPath string
	SQL       string
	LazyIndex bool
	Limit     int
	Sort      string
	Cursor    string
}

type DataviewRequest struct {
	VaultPath string
	Query     string
	LazyIndex bool
	Limit     int
	Sort      string
	Cursor    string
}

type SearchRequest struct {
	VaultPath     string
	Query         string
	Tags          []string
	Group         string
	Folder        string
	Kind          string
	Status        string
	CreatedAfter  string
	UpdatedAfter  string
	LinkTarget    string
	HasAttachment bool
	Limit         int
	Sort          string
	AllowStale    bool
	Engine        string
	LazyIndex     string
	At            string
	IncludeDirty  bool
	ChangedSince  string
	Revision      string
	Trust         string
	Stale         string
	Facets        bool
	TrustAware    bool
}

func toSearchOpsRequest(req SearchRequest) searchops.Request {
	return searchops.Request{VaultPath: req.VaultPath, Query: req.Query, Tags: req.Tags, Group: req.Group, Folder: req.Folder, Kind: req.Kind, Status: req.Status, CreatedAfter: req.CreatedAfter, UpdatedAfter: req.UpdatedAfter, LinkTarget: req.LinkTarget, HasAttachment: req.HasAttachment, Limit: req.Limit, Sort: req.Sort, AllowStale: req.AllowStale, Engine: req.Engine, LazyIndex: req.LazyIndex, At: req.At, IncludeDirty: req.IncludeDirty, ChangedSince: req.ChangedSince, Revision: req.Revision, Trust: req.Trust, Stale: req.Stale, Facets: req.Facets, TrustAware: req.TrustAware}
}

type CreateNoteRequest struct {
	VaultPath  string
	Title      string
	Project    string
	Folder     string
	Kind       string
	Tags       []string
	Template   string
	Vars       map[string]string
	Body       string
	SourcePath string
	StdinBody  string
	Dir        string
	Slug       string
	Status     string
	DryRun     bool
	// The following fields are internal recovery inputs. Public and legacy
	// callers leave them empty and retain the existing note creation behavior.
	PlannedPath          string
	ObjectID             string
	RecordIdempotencyKey string
	RecordEvidence       []string
}

type TemplateRequest struct {
	VaultPath  string
	Name       string
	Title      string
	Project    string
	Tags       []string
	SourcePath string
	Body       string
	UseStdin   bool
	Vars       map[string]string
	Yes        bool
	Overwrite  bool
	Engine     string
	SaveRun    string
	Run        string
	Runs       bool
	Pack       string
	UseCase    string
	Intent     string
}

type IndexPageRequest struct {
	VaultPath string
	Name      string
	Template  string
}

type ShowNoteRequest struct {
	VaultPath        string
	NoteRef          string
	View             string
	Display          string
	Snapshot         string
	Runs             bool
	EmbedAttachments string
	MaxEmbedDepth    int
	MaxEmbedBytes    int
	MaxPreviewBytes  int
}

type NoteRefreshRequest struct {
	VaultPath string
	NoteRef   string
	Rendered  bool
	Yes       bool
	SaveRun   string
	Snapshot  string
}

type NoteLinkRequest struct {
	VaultPath      string
	NoteRef        string
	PathStyle      string
	IncludePaths   bool
	All            bool
	BrokenOnly     bool
	Kind           string
	Status         string
	IncludeIgnored bool
	Limit          int
}

type NoteAttachRequest struct {
	VaultPath  string
	NoteRef    string
	SourcePath string
	Placement  string
	LinkStyle  string
	Embed      bool
	Mode       string
	Rename     string
	Yes        bool
}

type ImportMarkdownRequest struct {
	VaultPath string
	Source    string
	Group     string
	Folder    string
	Kind      string
	Status    string
	Tags      []string
	Conflict  string
	DryRun    bool
	Yes       bool
}

type ExportMarkdownRequest struct {
	VaultPath string
	OutputDir string
	Tags      []string
	Group     string
	Folder    string
	Kind      string
	Status    string
}

type KnowledgeExportProjectionRequest struct {
	VaultPath     string
	Output        string
	Allowlist     []string
	AllowlistFile string
	FromPackage   string
}

type NoteListRequest struct {
	VaultPath        string
	Tags             []string
	Project          string
	Group            string
	Folder           string
	Kind             string
	Status           string
	CreatedAfter     string
	UpdatedAfter     string
	UpdatedBefore    string
	Period           string
	Recent           bool
	Limit            int
	Sort             string
	PathPrefix       string
	Properties       []string
	StrictProperties bool
}

type NoteMutationRequest struct {
	VaultPath string
	NoteRef   string
	Title     string
	TargetDir string
	Yes       bool
	DryRun    bool
}

type NoteDeleteRequest struct {
	VaultPath string
	NoteRef   string
	Yes       bool
	Hard      bool
}

type NoteTagRequest struct {
	VaultPath string
	NoteRef   string
	Operation string
	Tags      []string
}

// NoteVerifyRequest 是 pinax note verify 的请求。Actor 为空且未配置 identity 时
// 必须由调用方 fail-closed；service 层再次校验，双保险。
type NoteVerifyRequest struct {
	VaultPath string
	NoteRef   string
	Actor     string
	Note      string
}

type NotePropertyRequest struct {
	VaultPath string
	NoteRef   string
	Operation string
	Key       string
	Value     string
}

type NoteTagBulkRequest struct {
	VaultPath string
	Operation string
	OldTag    string
	NewTag    string
	DryRun    bool
	Yes       bool
}

type NoteFolderBulkRequest struct {
	VaultPath string
	Operation string
	OldFolder string
	NewFolder string
	DryRun    bool
	Yes       bool
}

type NoteEditRequest struct {
	VaultPath string
	NoteRef   string
	Editor    string
}

type ProjectRequest struct {
	VaultPath   string
	Slug        string
	Name        string
	Description string
	NotesPrefix string
}

type StorageRequest struct {
	VaultPath string
	Root      string
	Bucket    string
	Region    string
	Prefix    string
	Endpoint  string
	Profile   string
}

type ApplyRequest struct {
	VaultPath       string
	PlanID          string
	Yes             bool
	SnapshotMessage string
	TrustFields     bool
	StaleAfter      string
	AgentVersion    string
}

type SyncRequest struct {
	VaultPath      string
	Target         string
	Yes            bool
	DryRun         bool
	BaseRevision   string
	RemoteRevision string
	Endpoint       string
	WorkspaceID    string
	DeviceID       string
	SecretRef      string
	PathPolicy     string
	Preview        string
	Limit          int
	ContentDiff    bool
	Progress       string
	LiveEvents     SyncEventSink
	// ProjectUnlockSource, when set and the runtime config is in repository-
	// encrypted S3 credential mode, unlocks the typed bundle and injects it as
	// the AWS SDK credentials provider for the sync run. Nil keeps the existing
	// device-profile credential resolution.
	ProjectUnlockSource projectsecrets.UnlockSource
}

type CloudLoginRequest struct {
	VaultPath           string
	Endpoint            string
	WorkspaceID         string
	DeviceID            string
	SecretRef           string
	EncryptionSecretRef string
}

type CloudBackendSetRequest struct {
	VaultPath           string
	Kind                string
	Bucket              string
	Region              string
	Prefix              string
	Endpoint            string
	Profile             string
	AddressingStyle     string
	Remote              string
	WorkspaceID         string
	DeviceID            string
	SecretRef           string
	EncryptionSecretRef string
}

type CloudRequest struct {
	VaultPath string
}

type BriefingRecipeRequest struct {
	VaultPath string
	Topic     string
	Limit     int
	Source    string
}

type BriefingRunRequest struct {
	VaultPath string
	DryRun    bool
	Yes       bool
}

type FeishuDeliveryRequest struct {
	VaultPath  string
	WebhookURL string
	SecretRef  string
	Title      string
	Text       string
	DryRun     bool
	Yes        bool
}
