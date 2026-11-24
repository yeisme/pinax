package inboxjudgment

import (
	"context"

	judgment "github.com/yeisme/judgment-sdk"
	"github.com/yeisme/judgment-sdk/providers"
)

// NewManagedJudgmentSDKBridge constructs the owner-scoped durable provider
// bridge. The host supplies its state store and current authorization policy.
// Disabled profiles are dormant and do not resolve credentials or make I/O.
func NewManagedJudgmentSDKBridge(options providers.Options, store providers.ReceiptStore) (*JudgmentSDKBridge, error) {
	if !options.Profile.Enabled {
		return nil, nil
	}
	authorize := options.Authorize
	options.Authorize = func(ctx context.Context, request *judgment.Request) error {
		if request == nil || request.Scope.OwnerID == nil || *request.Scope.OwnerID != "pinax" || authorize == nil {
			return judgment.NewError(judgment.CodeUnauthorized, judgment.SubmissionNotSubmitted, judgment.RetryNever, "", "owner_authorization_denied")
		}
		return authorize(ctx, request)
	}
	transport, err := providers.NewDurableTransport(options, store)
	if err != nil {
		return nil, err
	}
	caps, err := transport.Capabilities(context.Background())
	if err != nil {
		return nil, err
	}
	return NewJudgmentSDKBridge(transport, *caps), nil
}
