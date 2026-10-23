package main

import (
	"strings"
	"testing"
)

func TestKBCommandsAreRemoved(t *testing.T) {
	t.Parallel()
	// 顶层入口不存在时，历史子命令都走同一 Cobra 分支，无需初始化 vault 重复验证。
	out, err := runCLIExpectError("kb")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("removed kb command should be unknown: err=%v out=%s", err, out)
	}
	if strings.Contains(out+err.Error(), "kb_decoupled") {
		t.Fatalf("removed compatibility stub is still exposed: %s", out)
	}
}
