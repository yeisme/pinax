package domain

const ApplyReceiptSchemaVersion = "pinax.apply_receipt.v1"

type ApplyReceipt struct {
	SchemaVersion string               `json:"schema_version"`
	ReceiptID     string               `json:"receipt_id"`
	Command       string               `json:"command"`
	PlanID        string               `json:"plan_id,omitempty"`
	Status        string               `json:"status"`
	SnapshotID    string               `json:"snapshot_id,omitempty"`
	LedgerSeq     uint64               `json:"ledger_seq"`
	ChangedPaths  []string             `json:"changed_paths,omitempty"`
	Objects       []ApplyReceiptObject `json:"objects,omitempty"`
	SyncReady     bool                 `json:"sync_ready"`
	SyncReason    string               `json:"sync_reason"`
	SavedPath     string               `json:"saved_path"`
	CreatedAt     string               `json:"created_at"`
}

type ApplyReceiptObject struct {
	ObjectID              string          `json:"object_id"`
	ObjectKind            string          `json:"object_kind"`
	BeforePath            string          `json:"before_path,omitempty"`
	AfterPath             string          `json:"after_path,omitempty"`
	ExpectedRecordVersion uint64          `json:"expected_record_version,omitempty"`
	AfterRecordVersion    uint64          `json:"after_record_version,omitempty"`
	BeforeRevision        ContentRevision `json:"before_revision,omitempty"`
	AfterRevision         ContentRevision `json:"after_revision,omitempty"`
}
