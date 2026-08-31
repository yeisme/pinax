// Package continuitybinding 实现 user-level continuity binding registry。
//
// binding 把当前 Git repository/worktree 精确映射到一个已注册 vault 和
// bounded scope。registry 是 CLI/service 独占写入的 versioned YAML 文件，
// 存放在 user-level config 目录（owner-only 0600），不进入 repository、
// vault note 或 Git。Agent 和用户不得手写该文件。
//
// 第一 slice 只支持 exact canonical worktree root 匹配：不按目录名、Git
// remote、语义相似度或最近使用记录猜测绑定，避免错误合并两个本地 checkout。
package continuitybinding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// BindingSchemaVersion 是 continuity binding 的逻辑 schema 版本。
const BindingSchemaVersion = "pinax.continuity_binding.v1"

// 稳定错误码（additive；不回写已有 agentprotocol 错误码语义）。
const (
	// ErrCodeBindingAmbiguous 表示同一 canonical worktree 存在多个 enabled exact binding。
	ErrCodeBindingAmbiguous = "continuity_binding_ambiguous"
	// ErrCodeBindingInvalid 表示 registry 或 binding record 损坏/目标失效。
	ErrCodeBindingInvalid = "continuity_binding_invalid"
)

// BindingStatus 是 binding 解析状态机，用于 status projection 与自动解析。
type BindingStatus string

const (
	// StatusReady 表示 repository、vault、scope 与 registry 均可解析。
	StatusReady BindingStatus = "ready"
	// StatusMissing 表示当前 worktree 没有 enabled binding。
	StatusMissing BindingStatus = "missing"
	// StatusDisabled 表示 binding 存在但被显式禁用（rollback 保留数据）。
	StatusDisabled BindingStatus = "disabled"
	// StatusInvalid 表示 binding record 或目标 vault/scope 不可解析。
	StatusInvalid BindingStatus = "invalid"
	// StatusAmbiguous 表示同一 worktree 有多个 enabled exact binding（fail closed）。
	StatusAmbiguous BindingStatus = "ambiguous"
	// StatusNotARepository 表示当前目录不属于可解析的 Git worktree。
	StatusNotARepository BindingStatus = "not_a_repository"
)

// Binding 是一条 continuity binding 记录（pinax.continuity_binding.v1）。
// canonical_repo_root 只用于本机解析；machine output 只暴露 digest 和
// bounded basename，不泄漏完整 home 路径。
type Binding struct {
	// BindingID 是 opaque 稳定标识（bind_<digest>）。
	BindingID string `yaml:"binding_id" json:"binding_id"`
	// SchemaVersion 固定为 pinax.continuity_binding.v1。
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	// CanonicalRepoRoot 是 canonical（symlink-resolved）worktree 绝对路径。
	CanonicalRepoRoot string `yaml:"canonical_repo_root" json:"-"`
	// RepoRootDigest 是 CanonicalRepoRoot 的 bounded sha256 digest。
	RepoRootDigest string `yaml:"repo_root_digest" json:"repo_root_digest"`
	// VaultRef 是已注册 vault selector（alias 或绝对路径），必须可解析。
	VaultRef string `yaml:"vault_ref" json:"vault_ref"`
	// ScopeKind 是 bounded scope kind（如 project）。
	ScopeKind string `yaml:"scope_kind" json:"scope_kind"`
	// ScopeID 是 bounded scope 标识（如 pinax）。
	ScopeID string `yaml:"scope_id" json:"scope_id"`
	// Enabled 控制该 binding 是否参与自动解析。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// CreatedAt 是创建时间。
	CreatedAt time.Time `yaml:"created_at" json:"created_at"`
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
}

// Validate 检查 binding 记录完整性。
// 校验失败返回 continuity_binding_invalid，不写任何部分状态。
func (b Binding) Validate() error {
	if strings.TrimSpace(b.BindingID) == "" {
		return newInvalid("binding_id is required")
	}
	if b.SchemaVersion != BindingSchemaVersion {
		return newInvalid(fmt.Sprintf("schema_version must be %s", BindingSchemaVersion))
	}
	if strings.TrimSpace(b.CanonicalRepoRoot) == "" {
		return newInvalid("canonical_repo_root is required")
	}
	// 防御性约束：本地路径不允许携带 remote URL 或 userinfo 形态，
	// 防止把远端凭据或 URL 误写进 registry 后又被其他工具扩散。
	if strings.Contains(b.CanonicalRepoRoot, "://") || looksLikeSCPRemote(b.CanonicalRepoRoot) {
		return newInvalid("canonical_repo_root must be a local path without URL or userinfo")
	}
	if !filepath.IsAbs(b.CanonicalRepoRoot) {
		return newInvalid("canonical_repo_root must be an absolute local path")
	}
	if strings.TrimSpace(b.RepoRootDigest) == "" {
		return newInvalid("repo_root_digest is required")
	}
	if b.RepoRootDigest != Digest(b.CanonicalRepoRoot) {
		return newInvalid("repo_root_digest does not match canonical_repo_root")
	}
	if strings.TrimSpace(b.VaultRef) == "" {
		return newInvalid("vault_ref is required")
	}
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKind(b.ScopeKind), ID: b.ScopeID}
	if err := scope.Validate(); err != nil {
		return newInvalid(fmt.Sprintf("scope: %s", err.Error()))
	}
	return nil
}

// looksLikeSCPRemote 识别 git@example.com:org/repo 这类不含 :// 的
// remote/userinfo 形态。Windows drive letter 没有 @，不会被误判。
func looksLikeSCPRemote(value string) bool {
	at := strings.Index(value, "@")
	colon := strings.Index(value, ":")
	if at < 0 || colon <= at {
		return false
	}
	firstSeparator := strings.IndexAny(value, `/\\`)
	return firstSeparator < 0 || colon < firstSeparator
}

func newInvalid(message string) *agentprotocol.StableError {
	return agentprotocol.NewStableError(ErrCodeBindingInvalid, message)
}

// Digest 计算 canonical worktree root 的 bounded sha256 digest（16 hex chars）。
// digest 用于 machine output 与 receipt 关联，不可逆推出完整路径。
func Digest(canonicalRoot string) string {
	sum := sha256.Sum256([]byte(canonicalRoot))
	return hex.EncodeToString(sum[:8])
}

// RedactedRoot 返回 bounded basename，用于默认 machine output。
// 只暴露路径最后一段，避免泄漏用户 home 目录结构。
func RedactedRoot(canonicalRoot string) string {
	base := filepath.Base(canonicalRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "[repo]"
	}
	return base
}

// CanonicalizeRepoRoot 把任意 repo 路径规范化为 canonical worktree root。
// 顺序：Abs → EvalSymlinks。Windows 下大小写不敏感但保留原样，
// 由 registry 的 exact key 比较负责一致性（测试覆盖）。
//
// 注意：这里不做 prefix 匹配。两个路径即使共享前缀也只在完全相等时
// 命中同一 binding，这是 fail-closed 的关键边界。
func CanonicalizeRepoRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("repo path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repo path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize repo path %s: %w", abs, err)
	}
	return canonical, nil
}

// Scope 返回 binding 的 protocol scope。
func (b Binding) Scope() agentprotocol.Scope {
	return agentprotocol.Scope{Kind: agentprotocol.ScopeKind(b.ScopeKind), ID: b.ScopeID}
}
