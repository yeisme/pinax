package pinaxclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type WriterDailyEditRequest struct{ Date, Timezone, ObjectID, Revision, OperationID, Body string }
type WriterDailyEditPreview struct {
	Change        WriterChange `json:"change"`
	PreviewDigest string       `json:"preview_digest"`
	Path          string       `json:"path"`
	Before        string       `json:"before"`
	After         string       `json:"after"`
	State         string       `json:"state"`
}
type WriterDailyEditProposal struct {
	SchemaVersion string                 `json:"schema_version"`
	Date          string                 `json:"date"`
	Timezone      string                 `json:"timezone"`
	ObjectID      string                 `json:"object_id"`
	Preview       WriterDailyEditPreview `json:"preview"`
	PreviewDigest string                 `json:"preview_digest"`
}

func (c *Client) WriterDailyEditorSource(ctx context.Context, date, timezone, objectID, revision string) (WriterEditorSource, error) {
	var empty WriterEditorSource
	values := url.Values{"date": {date}, "timezone": {timezone}, "object_id": {objectID}, "revision": {revision}}
	payload := map[string]any{}
	for key, list := range values {
		payload[key] = list[0]
	}
	if catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyEditorSourceRequestV1Alpha1, payload) != nil {
		return empty, newError(CodeRequestInvalid, "Daily editor source requires a date and fixed identity", nil)
	}
	projection, err := c.getReaderProjection(ctx, "/v1/writer/journals/daily/editor-source", values)
	if err != nil {
		return empty, err
	}
	if projection.Command != "writer.daily.editor_source" {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily source command did not match", nil)
	}
	source, err := decodeWriterEditorSource(projection, objectID, revision)
	if err != nil {
		return empty, err
	}
	if source.ProtectedMetadata.Kind != "daily" {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily source kind did not match", nil)
	}
	return source, nil
}

func validDailyEditProposal(p WriterDailyEditProposal) bool {
	v := p.Preview
	if v.Change.Action != "replace" || v.Change.NoteRef != v.Path || v.Change.Body != v.After || !validSHA256Digest(v.Change.ExpectedRevision) {
		return false
	}
	if !validDailyPreview(WriterDailyProposal{Path: v.Path, After: v.After}) {
		return false
	}
	return len(v.Before) <= catalogschema.WriterMaxBodyBytes && utf8.ValidString(v.Before) && !strings.ContainsRune(v.Before, 0)
}

func (c *Client) WriterDailyEditPropose(ctx context.Context, req WriterDailyEditRequest) (WriterDailyEditProposal, error) {
	var empty WriterDailyEditProposal
	payload := map[string]any{"date": req.Date, "timezone": req.Timezone, "object_id": req.ObjectID, "revision": req.Revision, "operation_id": req.OperationID, "body": req.Body}
	if len(req.Body) > catalogschema.WriterMaxBodyBytes || !utf8.ValidString(req.Body) || strings.ContainsRune(req.Body, 0) || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyEditProposeRequestV1Alpha1, payload) != nil {
		return empty, newError(CodeRequestInvalid, "Daily edit request violated its schema or byte budget", nil)
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/journals/daily/edit:propose", payload, nil)
	if err != nil {
		return empty, err
	}
	var p WriterDailyEditProposal
	if err = validateDailyProjection(projection, catalogschema.WriterDailyEditProposalV1Alpha1, &p); err != nil {
		return empty, err
	}
	if projection.Command != "writer.daily.edit.propose" || p.Date != req.Date || p.Timezone != req.Timezone || p.ObjectID != req.ObjectID || p.Preview.Change.OperationID != req.OperationID || p.Preview.Change.ExpectedRevision != req.Revision || p.Preview.After != req.Body || !validDailyEditProposal(p) {
		return empty, newError(CodeUpstreamInvalidResponse, "Daily edit proposal did not match the original request", nil)
	}
	return p, nil
}

// Apply never automatically retries. Preserve the exact proposal and original
// operation on unknown outcomes; a new session cannot reuse the old HMAC.
func (c *Client) WriterDailyEditApply(ctx context.Context, p WriterDailyEditProposal, authorization string) (WriterApplyReceipt, error) {
	var empty WriterApplyReceipt
	raw, err := json.Marshal(map[string]any{"proposal": p, "authorization": authorization})
	if err != nil || len(raw) > 128*1024 || int64(len(raw)) > c.maxRequestBytes {
		return empty, newError(CodeRequestInvalid, "Daily edit exceeds its JSON request budget", nil)
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyEditApplyRequestV1Alpha1, payload) != nil || !validDailyEditProposal(p) {
		return empty, newError(CodeRequestInvalid, "Daily edit requires the original proposal and authorization", nil)
	}
	id := p.Preview.Change.OperationID
	projection, err := c.postWriterJSON(ctx, "/v1/writer/journals/daily/edit:apply", payload, nil)
	if err != nil {
		if ambiguousMutationError(err) {
			return empty, writerOutcomeUnknown(id, err)
		}
		return empty, err
	}
	unknown := func() (WriterApplyReceipt, error) {
		return empty, writerOutcomeUnknown(id, newError(CodeUpstreamInvalidResponse, "Daily edit receipt did not match its original identity", nil))
	}
	var receipt WriterApplyReceipt
	if err = validateDailyProjection(projection, catalogschema.WriterApplyReceiptV1Alpha1, &receipt); err != nil {
		return empty, writerOutcomeUnknown(id, err)
	}
	if projection.Command != "writer.daily.edit" || receipt.OperationID != id || receipt.CapabilityID != "writer.daily.edit" || receipt.BindingID != "rest.writer.daily.edit" || receipt.RequestDigest != p.PreviewDigest || receipt.RevisionBefore != p.Preview.Change.ExpectedRevision {
		return unknown()
	}
	if receipt.Status == "succeeded" {
		var result struct {
			NoteID        string `json:"note_id"`
			Path          string `json:"path"`
			ContentDigest string `json:"content_digest"`
			RecordDigest  string `json:"record_digest"`
		}
		var proof struct {
			OperationID   string `json:"operation_id"`
			Snapshot      string `json:"snapshot"`
			RecordEventID string `json:"record_event_id"`
		}
		if json.Unmarshal(receipt.Receipt, &proof) != nil || proof.OperationID != id || proof.Snapshot != receipt.ReceiptRef || proof.RecordEventID == "" {
			return unknown()
		}
		if json.Unmarshal(receipt.Result, &result) != nil || result.NoteID != p.ObjectID || result.Path != p.Preview.Path || result.ContentDigest != receipt.RevisionAfter || !validSHA256Digest(receipt.RevisionAfter) || !validSHA256Digest(result.RecordDigest) || receipt.ResourceRef != "pinax://note/"+p.ObjectID || !strings.HasPrefix(receipt.ReceiptRef, "snapshot:") || receipt.Error != nil || receipt.ReconcileRequired || receipt.Retryable || receipt.ReplaySafe {
			return unknown()
		}
	}
	return receipt, nil
}
