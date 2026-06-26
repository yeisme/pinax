package domain

const ProjectBoardSchemaVersion = "pinax.project_board.v1"

type BoardItemSourceKind string

const (
	BoardItemSourceNote         BoardItemSourceKind = "note"
	BoardItemSourceManagedTask  BoardItemSourceKind = "managed_task"
	BoardItemSourceInlineTask   BoardItemSourceKind = "inline_task"
	BoardItemSourceTaskBridge   BoardItemSourceKind = "taskbridge"
	BoardItemSourceManualReview BoardItemSourceKind = "manual_review"
)

type NoteDisplayKind string

const (
	NoteDisplayCard    NoteDisplayKind = "card"
	NoteDisplayDetail  NoteDisplayKind = "detail"
	NoteDisplayContext NoteDisplayKind = "context"
	NoteDisplayBody    NoteDisplayKind = "body"
)

type NoteExposure string

const (
	NoteExposurePublic      NoteExposure = "public"
	NoteExposureAgent       NoteExposure = "agent"
	NoteExposureLocalDetail NoteExposure = "local_detail"
	NoteExposureLocalBody   NoteExposure = "local_body"
)

type ProjectBoard struct {
	SchemaVersion    string                `json:"schema_version"`
	ProjectSlug      string                `json:"project_slug"`
	Subproject       string                `json:"subproject,omitempty"`
	WorkspacePath    string                `json:"workspace_path,omitempty"`
	Workspace        *ProjectWorkspace     `json:"workspace,omitempty"`
	Title            string                `json:"title"`
	Columns          []BoardColumn         `json:"columns"`
	Items            []BoardItem           `json:"items"`
	Facts            ProjectBoardFacts     `json:"facts"`
	Warnings         []ProjectBoardWarning `json:"warnings,omitempty"`
	SourceSnapshotID string                `json:"source_snapshot_id,omitempty"`
	GeneratedAt      string                `json:"generated_at"`
}

type ProjectBoardConfig struct {
	SchemaVersion string        `json:"schema_version"`
	ProjectSlug   string        `json:"project_slug"`
	Subproject    string        `json:"subproject,omitempty"`
	Columns       []BoardColumn `json:"columns"`
	Query         string        `json:"query,omitempty"`
	UpdatedAt     string        `json:"updated_at"`
}

const ProjectBoardViewSchemaVersion = "pinax.project_board_view.v1"

type ProjectBoardView struct {
	SchemaVersion string   `json:"schema_version"`
	ProjectSlug   string   `json:"project_slug"`
	Subproject    string   `json:"subproject,omitempty"`
	View          string   `json:"view"`
	Columns       []string `json:"columns,omitempty"`
	GroupBy       string   `json:"group_by,omitempty"`
	Sort          string   `json:"sort,omitempty"`
	Display       string   `json:"display,omitempty"`
	UpdatedAt     string   `json:"updated_at"`
}

type BoardColumn struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Order    int    `json:"order"`
	WIPLimit int    `json:"wip_limit,omitempty"`
}

type BoardItem struct {
	ItemID        string              `json:"item_id"`
	Title         string              `json:"title"`
	Column        string              `json:"column"`
	SourceKind    BoardItemSourceKind `json:"source_kind"`
	SourceStatus  string              `json:"source_status,omitempty"`
	NoteID        string              `json:"note_id,omitempty"`
	Path          string              `json:"path,omitempty"`
	SourceLine    int                 `json:"source_line,omitempty"`
	Project       string              `json:"project,omitempty"`
	Subproject    string              `json:"subproject,omitempty"`
	WorkspacePath string              `json:"workspace_path,omitempty"`
	Tags          []string            `json:"tags,omitempty"`
	Labels        []string            `json:"labels,omitempty"`
	Status        string              `json:"status,omitempty"`
	Milestone     string              `json:"milestone,omitempty"`
	Priority      string              `json:"priority,omitempty"`
	Due           string              `json:"due,omitempty"`
	DueAt         string              `json:"due_at,omitempty"`
	BlockedBy     []string            `json:"blocked_by,omitempty"`
	EvidenceRefs  []string            `json:"evidence_refs,omitempty"`
	Writable      bool                `json:"writable"`
	Note          *NoteDisplay        `json:"note,omitempty"`
	AgentContext  *AgentContext       `json:"agent_context,omitempty"`
}

const TaskAdoptionSchemaVersion = "pinax.task_adoption.v1"

type TaskAdoption struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	Title         string `json:"title"`
	Project       string `json:"project"`
	Subproject    string `json:"subproject,omitempty"`
	SourcePath    string `json:"source_path"`
	SourceLine    int    `json:"source_line"`
	SourceStatus  string `json:"source_status"`
	Column        string `json:"column"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ProjectBoardWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type ProjectBoardFacts struct {
	TotalItems    int            `json:"total_items"`
	Inbox         int            `json:"inbox,omitempty"`
	Next          int            `json:"next,omitempty"`
	Doing         int            `json:"doing,omitempty"`
	Blocked       int            `json:"blocked,omitempty"`
	Review        int            `json:"review,omitempty"`
	Done          int            `json:"done,omitempty"`
	ColumnCounts  map[string]int `json:"column_counts,omitempty"`
	WritableItems int            `json:"writable_items,omitempty"`
	IndexStatus   string         `json:"index_status,omitempty"`
	Engine        string         `json:"engine,omitempty"`
	SnapshotID    string         `json:"snapshot_id,omitempty"`
}

type NoteDisplay struct {
	NoteID            string          `json:"note_id,omitempty"`
	Title             string          `json:"title"`
	Path              string          `json:"path"`
	Display           NoteDisplayKind `json:"display"`
	Exposure          NoteExposure    `json:"exposure"`
	Project           string          `json:"project,omitempty"`
	Subproject        string          `json:"subproject,omitempty"`
	WorkspacePath     string          `json:"workspace_path,omitempty"`
	BoardColumn       string          `json:"board_column,omitempty"`
	Kind              string          `json:"kind,omitempty"`
	Status            string          `json:"status,omitempty"`
	Tags              []string        `json:"tags,omitempty"`
	Labels            []string        `json:"labels,omitempty"`
	Milestone         string          `json:"milestone,omitempty"`
	Priority          string          `json:"priority,omitempty"`
	DueAt             string          `json:"due_at,omitempty"`
	BlockedBy         []string        `json:"blocked_by,omitempty"`
	UpdatedAt         string          `json:"updated_at,omitempty"`
	Excerpt           string          `json:"excerpt,omitempty"`
	Body              string          `json:"body,omitempty"`
	LinksCount        int             `json:"links_count,omitempty"`
	BacklinksCount    int             `json:"backlinks_count,omitempty"`
	AttachmentsCount  int             `json:"attachments_count,omitempty"`
	Related           []NoteDisplay   `json:"related,omitempty"`
	RelatedCount      int             `json:"related_count,omitempty"`
	Actions           []Action        `json:"actions,omitempty"`
	AgentContext      *AgentContext   `json:"agent_context,omitempty"`
	RedactionWarnings []string        `json:"redaction_warnings,omitempty"`
}
