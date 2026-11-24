package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	judgment "github.com/yeisme/judgment-sdk"
	"time"
)

// Receipt is owner-held execution evidence. It excludes inputs and credentials.
// Result contains only the validated, bounded neutral result, never provider JSON.
type Receipt struct {
	SchemaVersion   string          `json:"schema_version"`
	Owner           string          `json:"owner"`
	Project         string          `json:"project"`
	RequestID       string          `json:"request_id"`
	AttemptID       string          `json:"attempt_id"`
	InputDigest     string          `json:"input_digest"`
	ProfileDigest   string          `json:"profile_digest"`
	State           string          `json:"state"`
	ErrorCode       string          `json:"error_code,omitempty"`
	SubmissionState string          `json:"submission_state,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// ReceiptStore is implemented in the host's durable state. Claim must be an
// atomic insert-if-absent keyed by owner/project/request/attempt, comparing both
// input and profile digests on conflict. Finish must preserve that binding.
type ReceiptStore interface {
	Claim(context.Context, Receipt) (Receipt, bool, error)
	Finish(context.Context, Receipt) error
}

type Runtime struct {
	Options Options
	Store   ReceiptStore
}

// DurableTransport exposes durable execution to existing SDK transport seams.
type DurableTransport struct {
	runtime   Runtime
	transport *Transport
}

func NewDurableTransport(options Options, store ReceiptStore) (*DurableTransport, error) {
	if store == nil {
		return nil, reject(judgment.CodeInvalidRequest, "durable_owner_context_required")
	}
	t, err := New(options)
	if err != nil {
		return nil, err
	}
	return &DurableTransport{runtime: Runtime{Options: options, Store: store}, transport: t}, nil
}
func (t *DurableTransport) Capabilities(ctx context.Context) (*judgment.Capabilities, error) {
	return t.transport.Capabilities(ctx)
}
func (t *DurableTransport) Evaluate(ctx context.Context, req *judgment.Request) (*judgment.Result, error) {
	result, _, err := t.runtime.Evaluate(ctx, req)
	return result, err
}

// Evaluate records dispatch intent before external I/O. An unfinished claim is
// deliberately sticky: restart cannot prove whether the provider accepted it.
func (r Runtime) Evaluate(ctx context.Context, req *judgment.Request) (*judgment.Result, *Receipt, error) {
	if r.Store == nil || r.Options.Authorize == nil || req == nil || req.Scope.OwnerID == nil || req.Scope.ProjectID == nil {
		return nil, nil, reject(judgment.CodeInvalidRequest, "durable_owner_context_required")
	}
	if err := r.Options.Authorize(ctx, req); err != nil {
		return nil, nil, reject(judgment.CodeUnauthorized, "owner_authorization_denied")
	}
	profileDigest, err := r.Options.Profile.Digest()
	if err != nil {
		return nil, nil, err
	}
	inputDigest, err := req.InputDigest()
	if err != nil {
		return nil, nil, err
	}
	if !r.Options.Profile.Enabled {
		return nil, nil, reject(judgment.CodeUnauthorized, "judgment_profile_disabled")
	}
	transport, err := New(r.Options)
	if err != nil {
		return nil, nil, reject(judgment.CodeInvalidRequest, "judgment_configuration_invalid")
	}
	now := time.Now().UTC()
	receipt := Receipt{SchemaVersion: "1.0", Owner: *req.Scope.OwnerID, Project: *req.Scope.ProjectID, RequestID: req.RequestID, AttemptID: req.AttemptID, InputDigest: inputDigest, ProfileDigest: profileDigest, State: "dispatching", CreatedAt: now, UpdatedAt: now}
	existing, claimed, err := r.Store.Claim(ctx, receipt)
	if err != nil {
		return nil, nil, errors.New("judgment_attempt_conflict_or_store_unavailable")
	}
	if !claimed {
		if existing.InputDigest != inputDigest || existing.ProfileDigest != profileDigest || existing.Owner != receipt.Owner || existing.Project != receipt.Project || existing.RequestID != receipt.RequestID || existing.AttemptID != receipt.AttemptID {
			return nil, &existing, errors.New("judgment_attempt_conflict")
		}
		if existing.State == "completed" {
			result, ve := judgment.ParseResult(existing.Result, req, judgment.DefaultPrecisionPolicy())
			if ve != nil {
				return nil, &existing, invalid("stored_result_invalid")
			}
			return result, &existing, nil
		}
		if existing.State == "failed" && existing.SubmissionState == judgment.SubmissionNotSubmitted {
			return nil, &existing, reject(existing.ErrorCode, "original_attempt_not_submitted")
		}
		return nil, &existing, judgment.NewError(judgment.CodeOutcomeUnknown, judgment.SubmissionUnknown, judgment.RetryReconcileFirst, "", "original_attempt_requires_review")
	}
	result, callErr := judgment.NewClient(transport, judgment.ClientOptions{KnownExtensions: []string{InputsExtension}}).Evaluate(ctx, req)
	receipt.UpdatedAt = time.Now().UTC()
	if callErr != nil {
		receipt.State = "failed"
		receipt.ErrorCode = callErr.Code
		receipt.SubmissionState = callErr.SubmissionState
		if callErr.SubmissionState != judgment.SubmissionNotSubmitted {
			receipt.State = "unknown"
		}
	} else {
		receipt.State = "completed"
		receipt.SubmissionState = judgment.SubmissionSubmitted
		receipt.Result, err = result.MarshalResult()
		if err != nil {
			return nil, &receipt, invalid("result_encoding_failed")
		}
	}
	// Cancellation of the caller must not discard the final durable observation.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = r.Store.Finish(saveCtx, receipt); err != nil {
		receipt.State = "unknown"
		receipt.Result = nil
		return nil, &receipt, judgment.NewError(judgment.CodeOutcomeUnknown, judgment.SubmissionUnknown, judgment.RetryReconcileFirst, "", "result_persistence_uncertain")
	}
	if callErr != nil {
		return nil, &receipt, callErr
	}
	return result, &receipt, nil
}

// Summary is the only payload suitable for a shared management report.
type Summary struct {
	SchemaVersion string        `json:"schema_version"`
	Owner         string        `json:"owner"`
	Project       string        `json:"project"`
	RequestID     string        `json:"request_id"`
	AttemptID     string        `json:"attempt_id"`
	InputDigest   string        `json:"input_digest"`
	ProfileDigest string        `json:"profile_digest"`
	State         string        `json:"state"`
	ErrorCode     string        `json:"error_code,omitempty"`
	ObservedAt    time.Time     `json:"observed_at"`
	ResultDigest  string        `json:"result_digest,omitempty"`
	Usage         *SummaryUsage `json:"usage,omitempty"`
}

type SummaryUsage struct {
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	Cost         *float64 `json:"cost"`
}

func (r Receipt) Summary() (Summary, error) {
	s := Summary{SchemaVersion: "1.0", Owner: r.Owner, Project: r.Project, RequestID: r.RequestID, AttemptID: r.AttemptID, InputDigest: r.InputDigest, ProfileDigest: r.ProfileDigest, State: r.State, ErrorCode: r.ErrorCode, ObservedAt: r.UpdatedAt}
	if len(r.Result) > 0 {
		var usage struct {
			Usage *SummaryUsage `json:"usage"`
		}
		if json.Unmarshal(r.Result, &usage) != nil {
			return Summary{}, errors.New("judgment_result_invalid")
		}
		s.Usage = usage.Usage
		tree, e := judgment.ParseTree(r.Result)
		if e != nil {
			return Summary{}, fmt.Errorf("judgment_result_invalid")
		}
		s.ResultDigest, e = judgment.Digest(tree)
		if e != nil {
			return Summary{}, e
		}
	}
	return s, nil
}
