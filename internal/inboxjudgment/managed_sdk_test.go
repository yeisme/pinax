package inboxjudgment

import (
	"context"
	"errors"
	"net/http"
	"testing"

	judgment "github.com/yeisme/judgment-sdk"
	"github.com/yeisme/judgment-sdk/providers"
)

type managedTestStore struct{ receipt *providers.Receipt }

func (s *managedTestStore) Claim(_ context.Context, r providers.Receipt) (providers.Receipt, bool, error) {
	if s.receipt != nil {
		return *s.receipt, false, nil
	}
	s.receipt = &r
	return r, true, nil
}
func (s *managedTestStore) Finish(_ context.Context, r providers.Receipt) error {
	s.receipt = &r
	return nil
}

type managedTestRoundTrip func(*http.Request) (*http.Response, error)

func (f managedTestRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestManagedSDKDormantAndUnknownReplay(t *testing.T) {
	bridge, err := NewManagedJudgmentSDKBridge(providers.Options{}, nil)
	if err != nil || bridge != nil {
		t.Fatal("disabled bridge performed assembly")
	}
	calls := 0
	opts := providers.Options{Profile: providers.ExecutionProfile{ID: "review", Revision: 1, Provider: "openai_decisions", Model: "gpt-6-luna", Enabled: true, Credential: providers.CredentialBinding{Source: "env", Env: "OPENAI_API_KEY"}}, Authorize: func(context.Context, *judgment.Request) error { return nil }, Credentials: providers.Resolver{Environment: func(string) (string, bool) { return "synthetic-key", true }}, RoundTripper: managedTestRoundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("synthetic interruption") })}
	store := &managedTestStore{}
	bridge, err = NewManagedJudgmentSDKBridge(opts, store)
	if err != nil {
		t.Fatal(err)
	}
	request := judgmentWireFixtureRequest()
	request.Model.Provider = "openai"
	request.Model.Model = "gpt-6-luna"
	for i := 0; i < 2; i++ {
		_, err = bridge.Evaluate(context.Background(), request)
		var e *JudgmentTransportError
		if !errors.As(err, &e) || e.Code != JudgmentCodeOutcomeUnknown {
			t.Fatal("unknown not retained", err)
		}
	}
	if calls != 1 {
		t.Fatalf("provider called %d times", calls)
	}
	request.Scope.Owner = "foreign"
	_, err = bridge.Evaluate(context.Background(), request)
	var e *JudgmentTransportError
	if !errors.As(err, &e) || e.Code != JudgmentCodeUnauthorized || calls != 1 {
		t.Fatal("foreign owner reached provider", err)
	}
}
