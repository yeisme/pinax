package domain

const ScopedOrganizePlanSchema = "pinax.organize_plan.scoped.v1"

// OrganizeSelection pins even selected objects that need no operation.
type OrganizeSelection struct {
	ObjectID                string          `json:"object_id"`
	ObservedPath            string          `json:"observed_path"`
	ExpectedRecordVersion   uint64          `json:"expected_record_version,omitempty"`
	ExpectedContentRevision ContentRevision `json:"expected_content_revision"`
}

type OrganizeOperationResult struct {
	OperationID string `json:"operation_id"`
	ObjectID    string `json:"object_id"`
	Status      string `json:"status"`
	ErrorCode   string `json:"error_code,omitempty"`
}
