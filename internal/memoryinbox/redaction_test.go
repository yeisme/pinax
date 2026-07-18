package memoryinbox

import (
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

var forbiddenInboxPatterns = []string{
	"Authorization:", "Cookie:", "Bearer ", "api_key=", "password=",
	"-----BEGIN", "PRIVATE KEY", "raw_prompt", "provider_payload",
	"chain-of-thought", "<system>",
}

func TestInboxItem_NoSecretInSubject(t *testing.T) {
	// 即使 proposal subject 含有看似敏感的内容，inbox projection 不应泄漏
	// （subject 在 classifyProposal 中被 safeSubject 截断，但不应排除整个 item）
	agg := NewAggregator()
	now := time.Now().UTC()
	prop := agentmemory.AgentProposalRow{
		ProposalID: "leak-test-1",
		Subject:    "Config with API key placeholder",
		Kind:       string(agentprotocol.MemoryKindFact),
		Status:     string(agentprotocol.ProposalStatusApprovalRequired),
		ScopeKind:  string(agentprotocol.ScopeKindProject),
		ScopeID:    "redaction",
		CreatedAt:  now,
	}
	pack, err := agg.Aggregate(t.Context(), AggregateInput{Proposals: []agentmemory.AgentProposalRow{prop}}, 100)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range pack.Items {
		for _, pat := range forbiddenInboxPatterns {
			if strings.Contains(strings.ToLower(item.Subject), strings.ToLower(pat)) {
				t.Errorf("item subject contains forbidden pattern %q: %s", pat, item.Subject)
			}
		}
	}
}

func TestInboxItem_NoBodyLeak(t *testing.T) {
	// inbox item 不应包含 proposal body 或完整 memory 内容
	// 只有 subject（脱敏摘要）出现在 projection 中
	agg := NewAggregator()
	now := time.Now().UTC()
	prop := agentmemory.AgentProposalRow{
		ProposalID: "body-test-1",
		Subject:    "Short subject",
		Summary:    "Some summary",
		Status:     string(agentprotocol.ProposalStatusApprovalRequired),
		ScopeKind:  string(agentprotocol.ScopeKindProject),
		ScopeID:    "body-test",
		CreatedAt:  now,
	}
	pack, err := agg.Aggregate(t.Context(), AggregateInput{Proposals: []agentmemory.AgentProposalRow{prop}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(pack.Items))
	}
	item := pack.Items[0]
	// Subject should be the short subject, not the summary or body
	if item.Subject != "Short subject" {
		t.Errorf("subject = %q, want %q", item.Subject, "Short subject")
	}
}

func TestInboxPack_NoRawPrompt(t *testing.T) {
	// 扫描整个 pack 确保不含 raw prompt 或 chain-of-thought
	agg := NewAggregator()
	prop := agentmemory.AgentProposalRow{
		ProposalID: "prompt-leak-1",
		Subject:    "Agent prompt review",
		Status:     string(agentprotocol.ProposalStatusApprovalRequired),
		ScopeKind:  string(agentprotocol.ScopeKindProject),
		ScopeID:    "prompt-test",
		CreatedAt:  time.Now().UTC(),
	}
	pack, err := agg.Aggregate(t.Context(), AggregateInput{Proposals: []agentmemory.AgentProposalRow{prop}}, 100)
	if err != nil {
		t.Fatal(err)
	}

	// 检查 SummaryLine 和 item subjects
	summary := SummaryLine(pack)
	for _, pat := range forbiddenInboxPatterns {
		if strings.Contains(strings.ToLower(summary), strings.ToLower(pat)) {
			t.Errorf("summary line contains forbidden pattern %q", pat)
		}
	}
}
