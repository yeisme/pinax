package domain

const (
	IdentityAuditSchemaVersion         = "pinax.identity_audit.v1"
	IdentityMigrationPlanSchemaVersion = "pinax.identity_migration_plan.v1"
)

type IdentityIssue struct {
	Code         string   `json:"code"`
	Severity     string   `json:"severity"`
	ObjectID     string   `json:"object_id,omitempty"`
	LegacyID     string   `json:"legacy_id,omitempty"`
	Path         string   `json:"path,omitempty"`
	RelatedPaths []string `json:"related_paths,omitempty"`
	Message      string   `json:"message"`
}

type IdentityAuditReport struct {
	SchemaVersion  string          `json:"schema_version"`
	VaultPath      string          `json:"vault_path"`
	Notes          int             `json:"notes"`
	Canonical      int             `json:"canonical"`
	Legacy         int             `json:"legacy"`
	Missing        int             `json:"missing"`
	Duplicates     int             `json:"duplicates"`
	Conflicts      int             `json:"conflicts"`
	PathCollisions int             `json:"path_collisions"`
	Issues         []IdentityIssue `json:"issues"`
}

type IdentityMigrationOperation struct {
	OperationID string `json:"operation_id"`
	Kind        string `json:"kind"`
	ObjectKind  string `json:"object_kind"`
	Path        string `json:"path"`
	FromID      string `json:"from_id,omitempty"`
	ToObjectID  string `json:"to_object_id"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
}

type IdentityMigrationPlan struct {
	SchemaVersion string                       `json:"schema_version"`
	PlanID        string                       `json:"plan_id"`
	CreatedAt     string                       `json:"created_at"`
	VaultPath     string                       `json:"vault_path"`
	SavedPath     string                       `json:"saved_path,omitempty"`
	Issues        []IdentityIssue              `json:"issues"`
	Operations    []IdentityMigrationOperation `json:"operations"`
}

const IdentityMigrationReceiptSchemaVersion = "pinax.identity_migration_receipt.v1"

type IdentityMigrationReceipt struct {
	SchemaVersion   string   `json:"schema_version"`
	PlanID          string   `json:"plan_id"`
	Status          string   `json:"status"`
	Phase           string   `json:"phase"`
	SnapshotID      string   `json:"snapshot_id,omitempty"`
	StartedAt       string   `json:"started_at"`
	UpdatedAt       string   `json:"updated_at"`
	CompletedPaths  []string `json:"completed_paths,omitempty"`
	RestoreCommands []string `json:"restore_commands,omitempty"`
	ErrorCode       string   `json:"error_code,omitempty"`
	ErrorMessage    string   `json:"error_message,omitempty"`
	SavedPath       string   `json:"saved_path"`
}
