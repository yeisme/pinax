package cli

import (
	"strings"
	"testing"
)

// TestContinueBindRejectsBareScopeID 覆盖裸 id 不得静默绑定 workspace:<id>：
// 缺少显式 kind 的 --scope 必须在参数校验层拒绝。
func TestContinueBindRejectsBareScopeID(t *testing.T) {
	t.Parallel()
	_, _, err := runCLIWithStdin(t, "", "continue", "bind", "--vault", "fixture-vault", "--scope", "pinax")
	if err == nil {
		t.Fatal("bare scope id must be rejected")
	}
	if !strings.Contains(err.Error(), "invalid_scope") && !strings.Contains(err.Error(), "explicit kind:id") {
		t.Fatalf("error should explain explicit kind requirement: %v", err)
	}
}

// TestContinueBindAcceptsExplicitScopeKinds 确认合法 kind:id 形态不被误拒
// （project 不存在时应在 service 层报 project scope 不存在，而非格式错误）。
func TestContinueBindAcceptsExplicitScopeKinds(t *testing.T) {
	t.Parallel()
	_, _, err := runCLIWithStdin(t, "", "continue", "bind", "--vault", "fixture-vault", "--scope", "project:pinax")
	if err == nil {
		t.Fatal("nonexistent project scope must still fail in service validation")
	}
	if strings.Contains(err.Error(), "invalid_scope") || strings.Contains(err.Error(), "explicit kind:id") {
		t.Fatalf("explicit project scope must not be rejected as malformed: %v", err)
	}
}
