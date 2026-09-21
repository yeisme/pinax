//go:build judgment_sdk

package inboxjudgment

import (
	"context"
	"strings"
	"testing"

	judgment "github.com/yeisme/judgment-sdk"
)

// judgment_sdk_bridge_test.go 在 build tag judgment_sdk 下验证真实公共 SDK
// 引用兼容性：bridge 通过真实 SDK client（ParseRequest 能力门 + 单次
// transport 调用 + ParseResult 校验）执行，消费端不感知 seam/SDK 差异。
// 运行：go test -tags judgment_sdk ./internal/inboxjudgment/

// judgmentSDKFixtureTransport 把 fixture responder 适配为 SDK Transport。
type judgmentSDKFixtureTransport struct {
	responder     func(context.Context, *judgment.Request) (*judgment.Result, error)
	caps          judgment.Capabilities
	evaluateCalls int
}

func (t *judgmentSDKFixtureTransport) Capabilities(context.Context) (*judgment.Capabilities, error) {
	caps := t.caps
	return &caps, nil
}

func (t *judgmentSDKFixtureTransport) Evaluate(ctx context.Context, req *judgment.Request) (*judgment.Result, error) {
	t.evaluateCalls++
	return t.responder(ctx, req)
}

func TestJudgmentSDKBridgeEndToEnd(t *testing.T) {
	model := "fixture:sdk-bridge"
	caps := JudgmentSDKFixtureCapabilities(model)
	transport := &judgmentSDKFixtureTransport{responder: JudgmentSDKFixtureResponder(), caps: caps}
	bridge := NewJudgmentSDKBridge(transport, caps)
	if _, err := bridge.DescribeCapabilities(context.Background()); err != nil {
		t.Fatalf("describe through the bridge: %v", err)
	}

	input := judgmentProjectionFixtureInput()
	authorizer := judgmentFixtureAuthorizer(input)
	consumer, err := NewInboxJudgmentConsumer(InboxJudgmentOptions{
		Mode:           InboxJudgmentModeAssist,
		Model:          model,
		AdapterVersion: "fixture-v1",
		Transport:      bridge,
		Capabilities:   judgmentSDKCapsToSeam(caps),
		Authorizer:     authorizer,
		Limits:         DefaultInboxJudgmentLimits(),
	})
	if err != nil {
		t.Fatalf("consumer over the sdk bridge: %v", err)
	}
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate over the sdk bridge: %v", err)
	}
	if outcome.Status != InboxJudgmentStatusSuggested {
		t.Fatalf("status = %s, want suggested", outcome.Status)
	}
	if transport.evaluateCalls != 1 {
		t.Errorf("one consumer evaluate must map to one sdk transport call, got %d", transport.evaluateCalls)
	}
	suggestion := outcome.Suggestion
	if !suggestion.Adoptable || len(suggestion.Links) != 1 || suggestion.Links[0].NoteID != "note-a" {
		t.Fatalf("sdk bridge must reproduce the seam suggestion shape: adoptable=%t links=%+v", suggestion.Adoptable, suggestion.Links)
	}
	// 与 seam fixture 相同的答案模型：zh/en 语言分布同时出现。
	if len(suggestion.Languages) != 2 {
		t.Errorf("language distribution mismatch: %+v", suggestion.Languages)
	}
}

func TestJudgmentSDKBridgeMapsSDKValidationErrors(t *testing.T) {
	model := "fixture:sdk-bridge-error"
	caps := JudgmentSDKFixtureCapabilities(model)
	transport := &judgmentSDKFixtureTransport{responder: JudgmentSDKFixtureResponder(), caps: caps}
	bridge := NewJudgmentSDKBridge(transport, caps)
	// 畸形 seam 请求（缺 source revision）必须在 bridge 侧 fail closed。
	request := judgmentWireFixtureRequest()
	request.Sources[0].Revision = ""
	_, bridgeErr := bridge.Evaluate(context.Background(), request)
	if bridgeErr == nil {
		t.Fatal("malformed request must fail through the sdk bridge")
	}
	transportError, ok := bridgeErr.(*JudgmentTransportError)
	if !ok {
		t.Fatalf("bridge errors must map to the seam transport error, got %T", bridgeErr)
	}
	if transportError.Code == "" {
		t.Error("mapped error must carry a stable code")
	}
	if strings.Contains(transportError.Message, "inbox excerpt") {
		t.Error("error must not echo inline text")
	}
}
