package domain

// DailyInteractionSchemaVersion identifies the additive interaction projection
// shared by the local capture, retrieval, review, planning and continuity
// workflows.
const DailyInteractionSchemaVersion = "pinax.daily_interaction.v1"

// DailyInteraction is derived from an existing command projection. It is
// intentionally stateless: it never creates a session, receipt, or durable
// workflow record.
type DailyInteraction struct {
	SchemaVersion         string   `json:"schema_version"`
	Phase                 string   `json:"phase"`
	State                 string   `json:"state"`
	ReadOnly              bool     `json:"read_only"`
	Writes                bool     `json:"writes"`
	RemoteEffect          bool     `json:"remote_effect"`
	Scope                 string   `json:"scope,omitempty"`
	SourceStatus          string   `json:"source_status"`
	FreshnessStatus       string   `json:"freshness_status"`
	RecommendedNextAction *Action  `json:"recommended_next_action,omitempty"`
	RecoveryActions       []Action `json:"recovery_actions,omitempty"`
	WarningCodes          []string `json:"warning_codes,omitempty"`
}
