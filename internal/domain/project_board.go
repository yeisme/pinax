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
	Columns       []BoardColumn `json:"columns"`
	Query         string        `json:"query,omitempty"`
	UpdatedAt     string        `json:"updated_at"`
}

type BoardColumn struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Order    int    `json:"order"`
	WIPLimit int    `json:"wip_limit,omitempty"`
}

type BoardItem struct {
	ItemID       string              `json:"item_id"`
	Title        string              `json:"title"`
	Column       string              `json:"column"`
	SourceKind   BoardItemSourceKind `json:"source_kind"`
	NoteID       string              `json:"note_id,omitempty"`
	Path         string              `json:"path,omitempty"`
	Project      string              `json:"project,omitempty"`
	Tags         []string            `json:"tags,omitempty"`
	Status       string              `json:"status,omitempty"`
	Priority     string              `json:"priority,omitempty"`
	Due          string              `json:"due,omitempty"`
	EvidenceRefs []string            `json:"evidence_refs,omitempty"`
	Writable     bool                `json:"writable"`
	Note         *NoteDisplay        `json:"note,omitempty"`
}

type ProjectBoardWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type ProjectBoardFacts struct {
	TotalItems    int    `json:"total_items"`
	Inbox         int    `json:"inbox,omitempty"`
	Next          int    `json:"next,omitempty"`
	Doing         int    `json:"doing,omitempty"`
	Blocked       int    `json:"blocked,omitempty"`
	Review        int    `json:"review,omitempty"`
	Done          int    `json:"done,omitempty"`
	WritableItems int    `json:"writable_items,omitempty"`
	IndexStatus   string `json:"index_status,omitempty"`
	Engine        string `json:"engine,omitempty"`
	SnapshotID    string `json:"snapshot_id,omitempty"`
}

type NoteDisplay struct {
	NoteID            string          `json:"note_id,omitempty"`
	Title             string          `json:"title"`
	Path              string          `json:"path"`
	Display           NoteDisplayKind `json:"display"`
	Exposure          NoteExposure    `json:"exposure"`
	Project           string          `json:"project,omitempty"`
	BoardColumn       string          `json:"board_column,omitempty"`
	Kind              string          `json:"kind,omitempty"`
	Status            string          `json:"status,omitempty"`
	Tags              []string        `json:"tags,omitempty"`
	UpdatedAt         string          `json:"updated_at,omitempty"`
	Excerpt           string          `json:"excerpt,omitempty"`
	Body              string          `json:"body,omitempty"`
	LinksCount        int             `json:"links_count,omitempty"`
	BacklinksCount    int             `json:"backlinks_count,omitempty"`
	AttachmentsCount  int             `json:"attachments_count,omitempty"`
	Related           []NoteDisplay   `json:"related,omitempty"`
	RelatedCount      int             `json:"related_count,omitempty"`
	Actions           []Action        `json:"actions,omitempty"`
	RedactionWarnings []string        `json:"redaction_warnings,omitempty"`
}
