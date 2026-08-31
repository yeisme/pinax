package agentprotocol

import (
	"fmt"
	"strings"
)

// 稳定 source kind 常量（additive；unknown kind 继续按 unresolved 处理）。
const (
	// SourceKindNote 引用 vault note。
	SourceKindNote = "note"
	// SourceKindAsset 引用 vault asset。
	SourceKindAsset = "asset"
	// SourceKindRepository 引用绑定内 repository-relative 文件证据。
	// Ref 是 repo-relative path（可配合 Span="rev:<sha>" 做漂移检测）；
	// resolver 只允许访问当前 binding 的 canonical repository root，
	// 阻止 `..`、symlink escape、绝对路径注入和跨 binding 访问。
	SourceKindRepository = "repository"
)

// SourceRef 引用一个可验证的 evidence 来源。
// 不保存完整明文 body；只记录足以定位和审计的元数据。
type SourceRef struct {
	// Kind 是来源类型：note、receipt、task、asset、external_url 等。
	Kind string `json:"kind"`
	// Ref 是稳定标识：object_id、note_id、receipt ID 或 URL。
	Ref string `json:"ref"`
	// Label 是人类可读描述（如 note title 或 receipt summary），可选。
	Label string `json:"label,omitempty"`
	// Span 是可选的定位信息（如 heading path 或行号范围），可选。
	Span string `json:"span,omitempty"`
}

// Validate 检查 source ref 最小必填字段。
func (s SourceRef) Validate() error {
	if strings.TrimSpace(s.Kind) == "" {
		return NewStableError(ErrCodeValidationFailed, "source kind is required")
	}
	if strings.TrimSpace(s.Ref) == "" {
		return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("source ref for kind %q is required", s.Kind))
	}
	return nil
}

// SourceRefList 是一组 source ref，提供批量校验和去重。
type SourceRefList []SourceRef

// Validate 校验所有 ref。
func (l SourceRefList) Validate() error {
	for i, ref := range l {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("source[%d]: %w", i, err)
		}
	}
	return nil
}

// IsEmpty 判断 source ref 列表是否为空。
func (l SourceRefList) IsEmpty() bool {
	return len(l) == 0
}
