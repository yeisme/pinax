package sqlitedsn

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoBareSQLiteOpen 守卫：internal/ 与 cmd/ 的非测试 Go 源不得出现裸
// `sqlite.Open(path)` 直连（即绕过统一 DSN helper 打开 SQLite）。
//
// 规则（pinax-local-async-substrate-v1）：所有 SQLite 打开点（operation、
// promptasset、agentmemory、memory ledger 及后续新增）必须经本包 Open /
// OpenReadOnly，保证 WAL + busy_timeout + foreign_keys 与 4/4 池界。
// 本包自身是唯一允许触碰 glebarez/sqlite dialector 的位置。
func TestNoBareSQLiteOpen(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	repoRoot := filepath.Join(wd, "..", "..")

	// 本包是统一 helper 本体，是唯一豁免。
	excluded := map[string]bool{wd: true}

	reBareOpen := regexp.MustCompile(`sqlite\.Open\s*\(`)
	reDriverImport := regexp.MustCompile(`"github\.com/glebarez/sqlite"`)

	violations := 0
	for _, scanDir := range []string{filepath.Join(repoRoot, "internal"), filepath.Join(repoRoot, "cmd")} {
		if _, err := os.Stat(scanDir); err != nil {
			t.Fatalf("scan dir missing, guard scope may have drifted: %s: %v", scanDir, err)
		}
		err := filepath.WalkDir(scanDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if excluded[path] {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := stripGoComments(string(body))
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			if loc := reBareOpen.FindStringIndex(src); loc != nil {
				violations++
				t.Errorf("%s: 禁止裸 sqlite.Open(...) 直连（offset %d），应使用 internal/sqlitedsn.Open/OpenReadOnly 统一 DSN",
					rel, loc[0])
			}
			if loc := reDriverImport.FindStringIndex(src); loc != nil {
				violations++
				t.Errorf("%s: 禁止在 helper 之外导入 glebarez/sqlite dialector（offset %d），SQLite 打开统一经 internal/sqlitedsn",
					rel, loc[0])
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scanDir, err)
		}
	}
	if violations == 0 {
		// 防御：扫描必须至少覆盖已知的四个业务打开点所在包，避免目录漂移后守卫形同虚设。
		for _, known := range []string{
			filepath.Join(repoRoot, "internal", "operation"),
			filepath.Join(repoRoot, "internal", "promptasset"),
			filepath.Join(repoRoot, "internal", "agentmemory"),
			filepath.Join(repoRoot, "internal", "memory"),
		} {
			if _, err := os.Stat(known); err != nil {
				t.Errorf("已知打开点包缺失，扫描范围可能漂移: %s", known)
			}
		}
	}
}

// TestGuardRegexDetectsViolations 证明守卫正则能命中违规样本且放过经统一 helper 的合法调用。
func TestGuardRegexDetectsViolations(t *testing.T) {
	t.Parallel()
	reBareOpen := regexp.MustCompile(`sqlite\.Open\s*\(`)
	reDriverImport := regexp.MustCompile(`"github\.com/glebarez/sqlite"`)

	for _, bad := range []string{
		`db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})`,
		`db, err := gorm.Open(sqlite.Open(filepath.Join(root, ".pinax", "index.sqlite")), nil)`,
		"gorm.Open(sqlite.Open(\n\t\tfmt.Sprintf(\"file:%s?mode=ro\", p)), &gorm.Config{})",
	} {
		if !reBareOpen.MatchString(bad) {
			t.Errorf("守卫应命中违规样本 %q", bad)
		}
	}
	if !reDriverImport.MatchString(`import "github.com/glebarez/sqlite"`) {
		t.Error("守卫应命中 dialector 导入样本")
	}

	for _, good := range []string{
		`db, err := sqlitedsn.Open(databasePath)`,
		`db, err := sqlitedsn.OpenReadOnly(indexPath)`,
		`return sqlitedsn.Open(filepath.Join(root, ".pinax", "index.sqlite"))`,
	} {
		if reBareOpen.MatchString(good) || reDriverImport.MatchString(good) {
			t.Errorf("守卫误报合法统一 helper 调用 %q", good)
		}
	}
}

// stripGoComments 移除 Go 注释，避免注释中的示例文本触发守卫。
func stripGoComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i += 2
			continue
		}
		if src[i] == '`' {
			i++
			for i < len(src) && src[i] != '`' {
				i++
			}
			if i < len(src) {
				i++
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}
