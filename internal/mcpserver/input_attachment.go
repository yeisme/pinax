package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/yeisme/pinax/pkg/inputcontract"
	intake "github.com/yeisme/runtime-plane/pkg/inputintake"
)

func inputAttachmentTools() []Tool {
	tools := []Tool{
		{Name: "pinax.input.attachment.preview", Description: "Preview a staged attachment upload or reference replacement against an exact note identity and revision. Does not adopt the file.", Readonly: true, InputSchema: inputcontract.AttachmentRequestSchema()},
		{Name: "pinax.input.attachment.apply", Description: "Confirm the exact attachment preview. Snapshot and publish the reference with an original operation receipt; never retry with a new identity after an unknown result.", InputSchema: inputcontract.AttachmentApplySchema()},
	}
	for _, action := range []string{"status", "reconcile"} {
		tools = append(tools, Tool{Name: "pinax.input.attachment." + action, Description: "Inspect the original attachment operation without replaying adoption; available after the upload ticket expires.", Readonly: true, InputSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"operation_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}, "required": []string{"operation_id"}}})
	}
	return tools
}

func (s *Server) inputAttachmentCall(ctx context.Context, name string, raw []byte, who intake.Identity) (any, error) {
	switch name {
	case "pinax.input.attachment.preview":
		var request inputcontract.AttachmentRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, err
		}
		return s.input.AttachmentPreview(ctx, who, request)
	case "pinax.input.attachment.apply":
		var request inputcontract.AttachmentApply
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, err
		}
		return s.input.AttachmentApply(ctx, who, request)
	default:
		var request struct {
			OperationID string `json:"operation_id"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, err
		}
		return s.input.AttachmentOperation(ctx, who, request.OperationID, name == "pinax.input.attachment.reconcile")
	}
}
