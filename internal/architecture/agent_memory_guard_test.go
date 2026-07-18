package architecture

import (
	"strings"
	"testing"
)

// TestAgentMemoryCorePackagesAreProviderNeutral 确保核心 agent memory 包不依赖
// 具体 runtime SDK、CLI 渲染、MCP 框架或 provider 实现。
// 核心包必须保持 provider-neutral，runtime-specific 信息只通过 adapter metadata 传递。
func TestAgentMemoryCorePackagesAreProviderNeutral(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// 核心包：必须只依赖标准库和自身，不引入 GORM、CLI、MCP、provider SDK。
	corePackages := []string{
		"internal/agentprotocol",
	}

	// forbiddenImportPrefixes 是核心包禁止引入的 import 前缀。
	forbiddenImportPrefixes := []string{
		modulePath + "/internal/cli",
		modulePath + "/internal/output",
		modulePath + "/internal/mcpserver",
		modulePath + "/internal/api",
		modulePath + "/internal/app",
		"github.com/spf13/cobra",
		"gorm.io",
		"github.com/openai",    // Codex / OpenAI SDK
		"github.com/anthropic", // Claude SDK
	}

	for _, pkg := range corePackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			checkGoImports(t, repoRoot, []string{pkg}, func(path, imp string) {
				for _, forbidden := range forbiddenImportPrefixes {
					if strings.HasPrefix(imp, forbidden) {
						t.Fatalf("%s imports forbidden %s (provider-neutral core must not depend on CLI/MCP/API/GORM/provider SDK)", path, imp)
					}
				}
			})
		})
	}
}

// TestAgentProtocolDoesNotImportRuntimeSDKs 确保协议包不引入
// Codex、Cohors 或 Hermes runtime 代码。
func TestAgentProtocolDoesNotImportRuntimeSDKs(t *testing.T) {
	repoRoot := findRepoRoot(t)
	runtimePrefixes := []string{
		"codex",
		"cohors",
		"hermes",
		"ntn",
		"lark",
	}
	checkGoImports(t, repoRoot, []string{"internal/agentprotocol"}, func(path, imp string) {
		for _, rt := range runtimePrefixes {
			if strings.Contains(strings.ToLower(imp), rt) {
				t.Fatalf("%s imports runtime %s (protocol must be provider-neutral)", path, imp)
			}
		}
	})
}
