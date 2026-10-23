package pinaxclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type WriterTagTarget struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision"`
}
type WriterTagsPlanRequest struct {
	OperationID string            `json:"operation_id"`
	Operation   string            `json:"operation"`
	OldTag      string            `json:"old_tag"`
	NewTag      string            `json:"new_tag"`
	Targets     []WriterTagTarget `json:"targets"`
}
type WriterTagPlanItem struct {
	ObjectID string   `json:"object_id"`
	Revision string   `json:"revision"`
	Path     string   `json:"path"`
	Before   []string `json:"before"`
	After    []string `json:"after"`
}
type WriterTagsPlan struct {
	SchemaVersion string              `json:"schema_version"`
	OperationID   string              `json:"operation_id"`
	Operation     string              `json:"operation"`
	OldTag        string              `json:"old_tag"`
	OldTagID      string              `json:"old_tag_id"`
	NewTag        string              `json:"new_tag"`
	NewTagID      string              `json:"new_tag_id"`
	Items         []WriterTagPlanItem `json:"items"`
	PreviewDigest string              `json:"preview_digest"`
}
type WriterTagItemResult struct {
	ObjectID      string `json:"object_id"`
	OperationID   string `json:"operation_id"`
	Status        string `json:"status"`
	RevisionAfter string `json:"revision_after,omitempty"`
}
type WriterTagsResult struct {
	Items []WriterTagItemResult `json:"items"`
}

// Raw receipt fields remain closed by the dedicated tag schema, not the
// wider legacy apply schema. Result and Receipt decode as WriterTagsResult.
type WriterTagsReceipt struct{ WriterApplyReceipt }

func tagIdentity(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + hex.EncodeToString(sum[:])
}
func canonicalClientTag(tag string) bool {
	if len(tag) == 0 || len(tag) > 512 || !utf8.ValidString(tag) {
		return false
	}
	for _, part := range strings.Split(tag, "/") {
		if part == "" {
			return false
		}
	}
	for _, r := range tag {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-' && r != '/' {
			return false
		}
	}
	return true
}
func tagsPayload(id string, value any, limit int) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > limit {
		return nil, newError(CodeRequestInvalid, "Tag request exceeds its JSON budget", nil)
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil || catalogschema.ValidatePayload(catalogschema.Default(), id, payload) != nil {
		return nil, newError(CodeRequestInvalid, "Tag request violated its closed schema", nil)
	}
	return payload, nil
}
func validTagsPlan(plan WriterTagsPlan) bool {
	if !canonicalClientTag(plan.OldTag) || plan.OldTagID != tagIdentity("tag-v1:", plan.OldTag) {
		return false
	}
	if plan.Operation == "rename" {
		if !canonicalClientTag(plan.NewTag) || plan.NewTag == plan.OldTag || plan.NewTagID != tagIdentity("tag-v1:", plan.NewTag) {
			return false
		}
	} else if plan.Operation != "delete" || plan.NewTag != "" || plan.NewTagID != "" {
		return false
	}
	previous := ""
	for _, item := range plan.Items {
		if item.ObjectID <= previous || !validDailyPreview(WriterDailyProposal{Path: item.Path}) {
			return false
		}
		previous = item.ObjectID
		set := map[string]bool{}
		found := false
		for _, tag := range item.Before {
			if !canonicalClientTag(tag) || set[tag] {
				return false
			}
			set[tag] = true
			if tag == plan.OldTag {
				found = true
			}
		}
		if !found {
			return false
		}
		delete(set, plan.OldTag)
		if plan.Operation == "rename" {
			set[plan.NewTag] = true
		}
		expected := make([]string, 0, len(set))
		for tag := range set {
			expected = append(expected, tag)
		}
		sort.Strings(expected)
		actual := append([]string{}, item.After...)
		sort.Strings(actual)
		if !reflect.DeepEqual(expected, actual) {
			return false
		}
	}
	return true
}

func (c *Client) WriterTagsPlan(ctx context.Context, req WriterTagsPlanRequest) (WriterTagsPlan, error) {
	var empty WriterTagsPlan
	payload, err := tagsPayload(catalogschema.WriterTagsPlanRequestV1Alpha1, req, 128*1024)
	if err != nil {
		return empty, err
	}
	if !canonicalClientTag(req.OldTag) || (req.Operation == "rename" && (!canonicalClientTag(req.NewTag) || req.NewTag == req.OldTag)) {
		return empty, newError(CodeRequestInvalid, "Tag paths must be canonical", nil)
	}
	targets := map[string]string{}
	for _, target := range req.Targets {
		if _, exists := targets[target.ObjectID]; exists {
			return empty, newError(CodeRequestInvalid, "Tag targets must be unique", nil)
		}
		targets[target.ObjectID] = target.Revision
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/tags:plan", payload, nil)
	if err != nil {
		return empty, err
	}
	var plan WriterTagsPlan
	if err := validateDailyProjection(projection, catalogschema.WriterTagsPlanV1Alpha1, &plan); err != nil {
		return empty, err
	}
	if projection.Command != "writer.tags.plan" || plan.OperationID != req.OperationID || plan.Operation != req.Operation || plan.OldTag != req.OldTag || plan.NewTag != req.NewTag || len(plan.Items) != len(targets) || !validTagsPlan(plan) {
		return empty, newError(CodeUpstreamInvalidResponse, "Tag plan did not match its request", nil)
	}
	for _, item := range plan.Items {
		if targets[item.ObjectID] != item.Revision {
			return empty, newError(CodeUpstreamInvalidResponse, "Tag plan changed a fixed target", nil)
		}
	}
	return plan, nil
}

// Apply never retries. Ambiguous outcomes preserve the parent operation ID;
// recover with WriterTagsLookup rather than automatically resubmitting.
func (c *Client) WriterTagsApply(ctx context.Context, plan WriterTagsPlan, authorization string) (WriterTagsReceipt, error) {
	var empty WriterTagsReceipt
	payload, err := tagsPayload(catalogschema.WriterTagsApplyRequestV1Alpha1, map[string]any{"plan": plan, "authorization": authorization}, 128*1024)
	if err != nil {
		return empty, err
	}
	if !validTagsPlan(plan) {
		return empty, newError(CodeRequestInvalid, "Tag apply requires an unchanged consistent plan", nil)
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/tags:apply", payload, nil)
	if err != nil {
		if ambiguousMutationError(err) {
			return empty, writerOutcomeUnknown(plan.OperationID, err)
		}
		return empty, err
	}
	receipt, err := decodeTagsReceipt(projection, "writer.tags.apply", plan.OperationID)
	if err == nil && receipt.RequestDigest != plan.PreviewDigest {
		err = newError(CodeUpstreamInvalidResponse, "Tag receipt digest changed", nil)
	}
	if err == nil && len(receipt.Result) > 0 {
		var result WriterTagsResult
		_ = json.Unmarshal(receipt.Result, &result)
		if len(result.Items) != len(plan.Items) {
			err = newError(CodeUpstreamInvalidResponse, "Tag receipt target count changed", nil)
		} else {
			for i, item := range result.Items {
				if item.ObjectID != plan.Items[i].ObjectID {
					err = newError(CodeUpstreamInvalidResponse, "Tag receipt target changed", nil)
					break
				}
			}
		}
	}
	if err != nil {
		return empty, writerOutcomeUnknown(plan.OperationID, err)
	}
	return receipt, nil
}

func (c *Client) WriterTagsLookup(ctx context.Context, operationID string) (WriterTagsReceipt, error) {
	var empty WriterTagsReceipt
	payload, err := tagsPayload(catalogschema.WriterTagsLookupRequestV1Alpha1, map[string]any{"operation_id": operationID}, 2048)
	if err != nil {
		return empty, err
	}
	projection, err := c.postWriterJSON(ctx, "/v1/writer/tags:lookup", payload, nil)
	if err != nil {
		return empty, err
	}
	return decodeTagsReceipt(projection, "writer.tags.lookup", operationID)
}

func decodeTagsReceipt(projection Projection, command, id string) (WriterTagsReceipt, error) {
	var receipt WriterTagsReceipt
	bad := func() (WriterTagsReceipt, error) {
		return WriterTagsReceipt{}, newError(CodeUpstreamInvalidResponse, "Tag receipt identity or state is inconsistent", nil)
	}
	if err := validateDailyProjection(projection, catalogschema.WriterTagsReceiptV1Alpha1, &receipt); err != nil {
		return receipt, err
	}
	if projection.Command != command || receipt.OperationID != id {
		return bad()
	}
	for _, stamp := range []string{receipt.CreatedAt, receipt.UpdatedAt, receipt.AcceptedAt} {
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
			return bad()
		}
	}
	for _, stamp := range []string{receipt.ApplyingAt, receipt.CompletedAt} {
		if stamp != "" {
			if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
				return bad()
			}
		}
	}
	unresolved := receipt.Status == "applying" || receipt.Status == "reconcile_required"
	if receipt.ReconcileRequired != unresolved || (unresolved && receipt.RequiredAction != "inspect_and_reconcile") || (!unresolved && receipt.RequiredAction != "") {
		return bad()
	}
	var result WriterTagsResult
	if len(receipt.Result) > 0 {
		if json.Unmarshal(receipt.Result, &result) != nil {
			return bad()
		}
		previous := ""
		pending := false
		for _, item := range result.Items {
			if item.ObjectID <= previous || item.OperationID != tagIdentity("tag-item-", id+"\x00"+item.ObjectID) {
				return bad()
			}
			previous = item.ObjectID
			if item.Status == "succeeded" && pending {
				return bad()
			}
			pending = pending || item.Status != "succeeded"
			if (item.Status == "succeeded") != validSHA256Digest(item.RevisionAfter) {
				return bad()
			}
		}
	}
	if receipt.Status == "succeeded" {
		if receipt.Error != nil || receipt.ApplyingAt == "" || receipt.CompletedAt == "" || len(result.Items) == 0 || len(receipt.Receipt) == 0 {
			return bad()
		}
		var proof WriterTagsResult
		if json.Unmarshal(receipt.Receipt, &proof) != nil || !reflect.DeepEqual(result, proof) {
			return bad()
		}
		for _, item := range result.Items {
			if item.Status != "succeeded" {
				return bad()
			}
		}
	} else if len(receipt.Receipt) > 0 {
		return bad()
	}
	if receipt.Status == "accepted" && (len(receipt.Result) > 0 || receipt.ApplyingAt != "" || receipt.CompletedAt != "" || receipt.Error != nil) {
		return bad()
	}
	if receipt.Status == "applying" && receipt.CompletedAt != "" {
		return bad()
	}
	return receipt, nil
}
