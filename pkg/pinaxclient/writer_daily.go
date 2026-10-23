package pinaxclient

import (
	"context"
	"encoding/json"
	"path"
	"strings"
	"unicode/utf8"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

const WriterDailyProposalSchemaV1Alpha1 = catalogschema.WriterDailyProposalV1Alpha1

type WriterDailyProposal struct {
	SchemaVersion        string `json:"schema_version"`
	OperationID          string `json:"operation_id"`
	Date                 string `json:"date"`
	Timezone             string `json:"timezone"`
	ObjectID             string `json:"object_id"`
	Path                 string `json:"path"`
	Title                string `json:"title"`
	Before               string `json:"before"`
	After                string `json:"after"`
	TemplateDigest       string `json:"template_digest"`
	PreviewDigest        string `json:"preview_digest"`
	CreatedAt            string `json:"created_at"`
	BindingID            string `json:"binding_id"`
	FrontmatterProtected bool   `json:"frontmatter_protected"`
}

func validateDailyProjection(projection Projection, schema string, target any) error {
	var value any
	if json.Unmarshal(projection.Data, &value) != nil || catalogschema.ValidatePayload(catalogschema.Default(), schema, value) != nil {
		return newError(CodeUpstreamInvalidResponse, "Daily writer response violated its schema", nil)
	}
	return decodeProjectionData(projection, target)
}

func (c *Client) WriterDailyPropose(ctx context.Context, date, timezone, operationID string) (WriterDailyProposal, error) {
	var empty WriterDailyProposal
	payload := map[string]any{"date": date, "timezone": timezone, "operation_id": operationID}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyProposeRequestV1Alpha1, payload) != nil {
		return empty, newError(CodeRequestInvalid, "Daily proposal requires a date, timezone and operation identity", nil)
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/journals/daily:propose", payload, nil)
	if err != nil {
		return empty, err
	}
	var preview WriterDailyProposal
	if err := validateDailyProjection(projection, catalogschema.WriterDailyProposalV1Alpha1, &preview); err != nil {
		return empty, err
	}
	if projection.Command != "writer.daily.propose" || preview.Date != date || preview.Timezone != timezone || preview.OperationID != operationID || !validDailyPreview(preview) {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily proposal identity did not match", nil)
	}
	return preview, nil
}

func validDailyPreview(preview WriterDailyProposal) bool {
	if len(preview.After) > catalogschema.WriterMaxBodyBytes || !utf8.ValidString(preview.After) || strings.ContainsRune(preview.After, 0) {
		return false
	}
	if path.IsAbs(preview.Path) || path.Clean(preview.Path) != preview.Path || strings.ContainsAny(preview.Path, "\\:\x00{}") || !strings.EqualFold(path.Ext(preview.Path), ".md") {
		return false
	}
	for _, part := range strings.Split(preview.Path, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// WriterDailyApply never retries automatically. Preserve the exact preview and
// original operation after a transport failure; only explicit same-identity
// replay can recover it in this supervised session.
func (c *Client) WriterDailyApply(ctx context.Context, preview WriterDailyProposal, authorization string) (WriterApplyReceipt, error) {
	var empty WriterApplyReceipt
	encoded, err := json.Marshal(map[string]any{"proposal": preview, "authorization": authorization})
	if err != nil {
		return empty, newError(CodeRequestInvalid, "Daily request could not be encoded", nil)
	}
	if len(encoded) > 128*1024 || int64(len(encoded)) > c.maxRequestBytes {
		return empty, newError(CodeRequestInvalid, "Daily request exceeds the JSON transport budget", nil)
	}
	var payload map[string]any
	if json.Unmarshal(encoded, &payload) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyApplyRequestV1Alpha1, payload) != nil || !validDailyPreview(preview) {
		return empty, newError(CodeRequestInvalid, "Daily apply requires the exact preview and authorization assertion", nil)
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/journals/daily:apply", payload, nil)
	if err != nil {
		if ambiguousMutationError(err) {
			return empty, writerOutcomeUnknown(preview.OperationID, err)
		}
		return empty, err
	}
	var receipt WriterApplyReceipt
	if err := validateDailyProjection(projection, catalogschema.WriterApplyReceiptV1Alpha1, &receipt); err != nil {
		return empty, writerOutcomeUnknown(preview.OperationID, err)
	}
	if projection.Command != "writer.daily.apply" || receipt.OperationID != preview.OperationID || receipt.CapabilityID != "writer.daily.apply" || receipt.BindingID != "rest.writer.daily.apply" || receipt.RequestDigest != preview.PreviewDigest {
		return empty, writerOutcomeUnknown(preview.OperationID, newError(CodeUpstreamInvalidResponse, "Daily receipt identity did not match", nil))
	}
	if receipt.Status == "succeeded" {
		var result struct {
			NoteID        string `json:"note_id"`
			Path          string `json:"path"`
			Date          string `json:"date"`
			Timezone      string `json:"timezone"`
			ContentDigest string `json:"content_digest"`
		}
		if json.Unmarshal(receipt.Result, &result) != nil || result.NoteID != preview.ObjectID || result.Path != preview.Path || result.Date != preview.Date || result.Timezone != preview.Timezone || result.ContentDigest != receipt.RevisionAfter || !strings.HasPrefix(receipt.RevisionAfter, "sha256:") || receipt.Error != nil || receipt.ReconcileRequired || receipt.Retryable || receipt.ReplaySafe || receipt.ResourceRef != "pinax://note/"+preview.ObjectID {
			return empty, writerOutcomeUnknown(preview.OperationID, newError(CodeUpstreamInvalidResponse, "Daily success receipt is inconsistent", nil))
		}
	}
	return receipt, nil
}
