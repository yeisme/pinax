// Package inputcontract defines the bounded public input adoption contract.
// Upload tickets and bytes stay on the existing input intake transport.
package inputcontract

const AttachmentSchema = "pinax.input.attachment.v1"

type AttachmentRequest struct {
	InputRequestID string `json:"input_request_id"`
	OperationID    string `json:"operation_id"`
	ObjectID       string `json:"object_id"`
	Revision       string `json:"revision"`
	Placement      string `json:"placement"`
	// Empty appends a reference. Otherwise this is the exact existing Markdown
	// reference to replace, not a filesystem path or permission to overwrite bytes.
	ReplaceReference string `json:"replace_reference,omitempty"`
}

type AttachmentProposal struct {
	SchemaVersion string            `json:"schema_version"`
	Request       AttachmentRequest `json:"request"`
	PreviewDigest string            `json:"preview_digest"`
	Reference     string            `json:"reference"`
	MIME          string            `json:"mime"`
	Size          int64             `json:"size"`
	SHA256        string            `json:"sha256"`
}

type AttachmentApply struct {
	Proposal AttachmentProposal `json:"proposal"`
	Confirm  bool               `json:"confirm"`
}
