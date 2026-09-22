package inboxjudgment

import (
	"context"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// ExperimentalJudgmentGate 是用户可配置的实验性（experimental）inbox
// judgment 开关面：enabled 默认 false，mode 为 off/shadow/assist，off 即
// 完全休眠。它由配置层投影（enabled/mode 两字段直映射，无其他来源），
// 本包不读取配置文件、不发现 adapter、不读取凭据。
//
// 休眠（未启用或 mode=off）保证：inbox 流程零 judgment 装配
// （Assemble 返回 nil consumer）、零 transport 调用，且
// AcceptInboxJudgmentSuggestion 一律拒绝并提示未启用；关闭即恢复原
// inbox 审阅流程。启用模式沿用既有 consumer/采纳门语义，transport、
// authorizer 与精确模型 pin 仍须显式注入。
type ExperimentalJudgmentGate struct {
	Enabled bool
	Mode    string
}

// normalizedMode 归一化空 mode 为 off。
func (g ExperimentalJudgmentGate) normalizedMode() string {
	if strings.TrimSpace(g.Mode) == "" {
		return InboxJudgmentModeOff
	}
	return g.Mode
}

// Validate fail-fast 拒绝非法组合：未知 mode，或未启用却要求 shadow/assist
// （半开配置不允许静默降级）。
func (g ExperimentalJudgmentGate) Validate() error {
	switch g.normalizedMode() {
	case InboxJudgmentModeOff:
		return nil
	case InboxJudgmentModeShadow, InboxJudgmentModeAssist:
		if !g.Enabled {
			return judgmentInvalidRequest("judgment mode "+g.Mode+" requires the experimental judgment switch to be enabled", "Set judgment.enabled=true (experimental) or keep judgment.mode=off")
		}
		return nil
	default:
		return judgmentInvalidRequest("unknown judgment mode", "Use off, shadow, or assist")
	}
}

// Dormant 报告配置是否完全休眠（未启用或 mode=off）。
func (g ExperimentalJudgmentGate) Dormant() bool {
	return !g.Enabled || g.normalizedMode() == InboxJudgmentModeOff
}

// Assemble 装配 consumer：休眠配置零装配（nil consumer + nil error，inbox
// 流程不触碰任何 judgment 组件）；启用配置把 mode 固化为开关声明的模式
// （开关优先于注入 options 携带的 mode），并沿用既有
// InboxJudgmentOptions 校验——transport、authorizer 与精确模型 pin 必须
// 显式注入，绝不自动发现。
func (g ExperimentalJudgmentGate) Assemble(options InboxJudgmentOptions) (*InboxJudgmentConsumer, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	if g.Dormant() {
		return nil, nil
	}
	options.Mode = g.normalizedMode()
	return NewInboxJudgmentConsumer(options)
}

// AcceptSuggestion 是实验开关后面的显式采纳门：休眠配置一律拒绝并提示
// 未启用（原 inbox 审阅流程保持权威）；启用配置沿用
// AcceptInboxJudgmentSuggestion 的既有语义（shadow 不可采纳、stale/权限
// 撤销阻断、只枚举原入口命令）。
func (g ExperimentalJudgmentGate) AcceptSuggestion(ctx context.Context, suggestion InboxJudgmentSuggestion, evidence InboxJudgmentEvidence, authorizer InboxJudgmentAuthorizer, currentRevisions map[string]string, projectionCandidates []InboxJudgmentCandidate) (InboxJudgmentAcceptance, error) {
	if err := g.Validate(); err != nil {
		return InboxJudgmentAcceptance{}, err
	}
	if g.Dormant() {
		return InboxJudgmentAcceptance{}, &domain.CommandError{
			Code:    "judgment_not_enabled",
			Message: "experimental inbox judgment is not enabled; the original inbox review flow stays authoritative",
			Hint:    "Set judgment.enabled=true with judgment.mode=shadow or assist (experimental), re-evaluate explicitly, then accept through the original review flow",
		}
	}
	return AcceptInboxJudgmentSuggestion(ctx, suggestion, evidence, authorizer, currentRevisions, projectionCandidates)
}
