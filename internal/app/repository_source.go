package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

// repositorySourceStatus 是 repository source 的解析状态。
type repositorySourceStatus string

const (
	repoSourceResolved  repositorySourceStatus = "resolved"
	repoSourceStale     repositorySourceStatus = "stale"
	repoSourceMissing   repositorySourceStatus = "missing"
	repoSourceAmbiguous repositorySourceStatus = "ambiguous"
)

// repositorySourceResolution 是单个 repository source 的 bounded 解析结果。
// 不返回 source body；ObservedAt 用于 evidence freshness。
type repositorySourceResolution struct {
	Status     repositorySourceStatus
	ObservedAt time.Time
	Revision   string
}

// ResolveRepositorySource 解析一个 repository SourceRef。
//
// 边界判断（中文注释，review 时不可放宽）：
//  1. repoRoot 必须是 binding 的 canonical root（调用方保证）；本函数在其上
//     再做 containment 校验，防止将来调用方误传子目录造成权限扩大。
//  2. ref 拒绝绝对路径和任何 `..` 段（先按 / 分段再 Clean，拒绝 Windows
//     分隔符变体）；Clean 后再 EvalSymlinks 解析 symlink，最终路径必须
//     仍以 repoRoot 为前缀，否则按 boundary escape 拒绝。
//  3. 可选 revision 从 Span "rev:<sha>" 读取；记录值与当前
//     `git rev-parse HEAD:<path>` blob hash 不一致时计为 stale，
//     不替换原 evidence。
//  4. 通配符 ref 匹配多条时计为 ambiguous（计入 coverage 的 additive
//     ambiguous 分桶，整体视为 unresolved）。
func ResolveRepositorySource(ctx context.Context, repoRoot string, ref agentprotocol.SourceRef) (repositorySourceResolution, error) {
	if ref.Kind != agentprotocol.SourceKindRepository {
		return repositorySourceResolution{}, fmt.Errorf("source kind %q is not repository", ref.Kind)
	}
	canonicalRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return repositorySourceResolution{}, err
	}
	rel, err := sanitizeRepositoryRelRef(ref.Ref)
	if err != nil {
		return repositorySourceResolution{Status: repoSourceMissing}, nil
	}

	// ambiguous 语义：ref 不能唯一解析到单个文件对象。
	//  1. ref 指向目录（无法确定哪份文件是 evidence）→ ambiguous；
	//  2. ref 含 glob 通配符且匹配多条 → ambiguous；
	//     恰好一条 → resolved；零条 → missing。
	if strings.ContainsAny(ref.Ref, "*?[") {
		matches, globErr := filepath.Glob(filepath.Join(canonicalRoot, filepath.FromSlash(rel)))
		if globErr != nil {
			return repositorySourceResolution{Status: repoSourceMissing}, nil
		}
		files := 0
		var observed time.Time
		matchedRel := ""
		for _, match := range matches {
			canonicalMatch, evalErr := filepath.EvalSymlinks(match)
			if evalErr != nil || !repositoryPathWithinRoot(canonicalRoot, canonicalMatch) {
				continue
			}
			info, statErr := os.Stat(canonicalMatch)
			if statErr != nil || info.IsDir() {
				continue
			}
			files++
			if relMatch, relErr := filepath.Rel(canonicalRoot, match); relErr == nil {
				matchedRel = filepath.ToSlash(relMatch)
			}
			if info.ModTime().After(observed) {
				observed = info.ModTime()
			}
		}
		switch {
		case files > 1:
			return repositorySourceResolution{Status: repoSourceAmbiguous}, nil
		case files == 1:
			resolution := repositorySourceResolution{Status: repoSourceResolved, ObservedAt: observed.UTC()}
			if expectedRev := repositoryExpectedRevision(ref); expectedRev != "" {
				current, revErr := repositoryBlobHash(ctx, canonicalRoot, matchedRel)
				if revErr != nil || !strings.HasPrefix(current, expectedRev) {
					resolution.Status = repoSourceStale
				}
				resolution.Revision = current
			}
			return resolution, nil
		default:
			return repositorySourceResolution{Status: repoSourceMissing}, nil
		}
	}

	target := filepath.Join(canonicalRoot, filepath.FromSlash(rel))
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		// 文件不存在或 symlink 断裂：missing（不泄漏底层错误）。
		return repositorySourceResolution{Status: repoSourceMissing}, nil
	}
	// containment 校验：symlink 逃逸出 repoRoot 一律拒绝。
	if !repositoryPathWithinRoot(canonicalRoot, canonicalTarget) {
		return repositorySourceResolution{Status: repoSourceMissing}, nil
	}

	info, err := os.Stat(canonicalTarget)
	if err != nil {
		return repositorySourceResolution{Status: repoSourceMissing}, nil
	}
	if info.IsDir() {
		// ref 指向目录：无法唯一确定 evidence 对象。
		return repositorySourceResolution{Status: repoSourceAmbiguous}, nil
	}
	resolution := repositorySourceResolution{
		Status:     repoSourceResolved,
		ObservedAt: info.ModTime().UTC(),
	}

	// 可选 revision 漂移检测。
	expectedRev := repositoryExpectedRevision(ref)
	if expectedRev != "" {
		current, revErr := repositoryBlobHash(ctx, canonicalRoot, rel)
		if revErr != nil || !strings.HasPrefix(current, expectedRev) {
			resolution.Status = repoSourceStale
		}
		resolution.Revision = current
	}
	return resolution, nil
}

func repositoryExpectedRevision(ref agentprotocol.SourceRef) string {
	if strings.HasPrefix(ref.Span, "rev:") {
		return strings.TrimPrefix(ref.Span, "rev:")
	}
	return ""
}

// repositoryPathWithinRoot 对 direct 与 glob 分支使用同一个 symlink-resolved
// containment 判定，避免某个解析分支绕过 binding 边界。
func repositoryPathWithinRoot(canonicalRoot, canonicalTarget string) bool {
	return canonicalTarget == canonicalRoot || strings.HasPrefix(canonicalTarget, canonicalRoot+string(os.PathSeparator))
}

// sanitizeRepositoryRelRef 校验并规范化 repo-relative ref。
// 拒绝空串、绝对路径、`..` 段和反斜杠变体（防止 Windows 分隔符绕过检查）。
func sanitizeRepositoryRelRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty repository ref")
	}
	if filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") || strings.ContainsRune(ref, '\\') {
		return "", fmt.Errorf("absolute or windows-style repository ref is not allowed")
	}
	for _, segment := range strings.Split(ref, "/") {
		if segment == ".." {
			return "", fmt.Errorf("repository ref must not contain '..' segments")
		}
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref)))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("repository ref escapes the repository boundary")
	}
	return clean, nil
}

// repositoryBlobHash 返回当前 HEAD 下 path 的 blob hash（git object id）。
// 文件不在 HEAD 中（未提交/已删除）时返回错误，调用方按 stale 或 missing 处理。
func repositoryBlobHash(ctx context.Context, repoRoot, rel string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "HEAD:"+rel)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("blob hash unavailable")
	}
	return strings.TrimSpace(string(out)), nil
}
