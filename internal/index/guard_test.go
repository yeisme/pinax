package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoDirectGormBusinessQueries 守护 internal/index 普通业务代码不得重新引入
// 直接 GORM 业务查询、database/sql、Raw/Exec 或硬编码 SQL。
//
// 规则（pinax-gorm-gen-database-access 治理要求）：
//   - 普通 index 业务文件只能通过 internal/index/query 生成的类型化 DAO 访问 SQLite 投影。
//   - schema.go 是唯一允许 PRAGMA/raw schema 探测的集中 helper，显式 allowlist。
//   - 连接（gorm.Open）、迁移（AutoMigrate）、事务（Transaction）和类型引用不算业务查询。
//
// 检测的直接 GORM 链针对常见 *gorm.DB 变量名 db/tx 调用查询构造方法，
// 例如 db.Find( / tx.Where( / db.Create(；gen DAO 的 q.X.WithContext(ctx).Find( 不会命中。
func TestNoDirectGormBusinessQueries(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	// 只扫描本包业务文件；生成的 query/、模型 model/ 和生成器 gormgen/ 不在扫描范围。
	excluded := map[string]bool{
		filepath.Join(wd, "query"):   true,
		filepath.Join(wd, "model"):   true,
		filepath.Join(wd, "gormgen"): true,
	}
	// schema.go 允许集中 PRAGMA schema 探测。
	rawAllowlist := map[string]bool{filepath.Join(wd, "schema.go"): true}

	// database/sql 作为业务访问层被禁止。
	reDatabaseSQL := regexp.MustCompile(`"database/sql"`)
	// 硬编码 SQL 动词字符串（带前后空格，避免误伤标识符）。
	reSQLVerb := regexp.MustCompile(`"(?i:\s*(SELECT|INSERT\s+INTO|UPDATE\s+\w|DELETE\s+FROM|DROP\s+TABLE|ALTER\s+TABLE|PRAGMA)\b)`)
	// 直接 *gorm.DB 业务查询链：db./tx. 后跟查询构造方法。
	reGormChain := regexp.MustCompile(`\b(db|tx)\.(Raw|Exec|Where|Find|First|Take|Last|Create|Save|Delete|Order|Model|Count|Updates?|UpdateColumns?|Pluck|Scan|Group|Having|Join|Distinct|Select|Limit|Offset|Row|Rows)\s*\(`)

	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(wd, name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := stripComments(string(body))

		if loc := reDatabaseSQL.FindStringIndex(src); loc != nil {
			t.Errorf("%s: 禁止业务文件直接依赖 database/sql（offset %d）", name, loc[0])
		}
		if !rawAllowlist[path] {
			if loc := reGormChain.FindStringIndex(src); loc != nil {
				t.Errorf("%s: 检测到直接 GORM 业务查询链 %q（offset %d），应改用 internal/index/query 生成的类型化 DAO",
					name, reGormChain.FindString(src[loc[0]:]), loc[0])
			}
			if loc := reSQLVerb.FindStringIndex(src); loc != nil {
				t.Errorf("%s: 检测到硬编码 SQL 字符串 %q（offset %d），普通业务不得手写 SQL",
					name, reSQLVerb.FindString(src[loc[0]:]), loc[0])
			}
		}
	}

	// 防御：确保 excluded 目录存在，避免扫描范围意外缩窄后守卫形同虚设。
	for dir := range excluded {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("守卫扫描排除目录不存在，扫描范围可能漂移: %s", dir)
		}
	}
}

// stripComments 移除 Go 注释，避免注释里的示例文本触发守卫。
func stripComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		// 行注释
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// 块注释
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i += 2
			continue
		}
		// 字符串字面量保留（SQL 动词检测需要它们），但跳过反引号原始字符串以容忍生成器文档。
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

// TestGuardRegexDetectsViolations 证明守卫正则能命中违规且放过合法 gen DAO 调用。
func TestGuardRegexDetectsViolations(t *testing.T) {
	reGormChain := regexp.MustCompile(`\b(db|tx)\.(Raw|Exec|Where|Find|First|Take|Last|Create|Save|Delete|Order|Model|Count|Updates?|UpdateColumns?|Pluck|Scan|Group|Having|Join|Distinct|Select|Limit|Offset|Row|Rows)\s*\(`)
	reSQLVerb := regexp.MustCompile(`"(?i:\s*(SELECT|INSERT\s+INTO|UPDATE\s+\w|DELETE\s+FROM|DROP\s+TABLE|ALTER\s+TABLE|PRAGMA)\b)`)
	reDatabaseSQL := regexp.MustCompile(`"database/sql"`)

	for _, bad := range []string{
		`db.Find(&records)`,
		`tx.Where("path = ?", v).Delete(&m)`,
		`db.Create(&record)`,
		`tx.First(&record, "path = ?", p)`,
		`db.Model(&NoteTextRecord{}).Where("x = ?", y).Count(&n)`,
		`db.Raw("PRAGMA schema_version")`,
		`tx.Exec("DELETE FROM note_records")`,
		`"SELECT * FROM notes"`,
		`"DELETE FROM note_records WHERE path = ?"`,
		`"PRAGMA schema_version"`,
		`import "database/sql"`,
	} {
		matched := reGormChain.MatchString(bad) || reSQLVerb.MatchString(bad) || reDatabaseSQL.MatchString(bad)
		if !matched {
			t.Errorf("守卫应命中违规样本 %q", bad)
		}
	}

	for _, good := range []string{
		`q.NoteRecord.WithContext(ctx).Find()`,
		`q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.NotePath.Eq(p)).Count()`,
		`query.Use(db)`,
		`db.Transaction(func(tx *gorm.DB) error {`,
		`db.AutoMigrate(model.AllModels()...)`,
		`gorm.Open(sqlite.Open(path), &gorm.Config{})`,
		`errors.Is(err, gorm.ErrRecordNotFound)`,
		`q.AssetRecord.WithContext(ctx).Session(globalUpdate()).Delete()`,
	} {
		if reGormChain.MatchString(good) || reSQLVerb.MatchString(good) || reDatabaseSQL.MatchString(good) {
			t.Errorf("守卫误报合法 gen DAO 调用 %q", good)
		}
	}
}
