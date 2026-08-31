package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestContinueBaseline_ExplicitFlagsEnvelope 固化旧 `pinax continue` 显式参数合同：
// --vault/--scope/--task/--intent/--handoff/--max-items/--max-chars 保持既有语义，
// JSON top-level envelope（command/status/mode/facts/summary/data）不变。
func TestContinueBaseline_ExplicitFlagsEnvelope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "continue",
		"--vault", root,
		"--scope", "project:baseline",
		"--task", "finish the baseline contract",
		"--intent", "contract pinning",
		"--handoff", "h_missing_000",
		"--max-items", "5",
		"--max-chars", "800",
		"--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("continue baseline envelope invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "continue" || envelope["status"] != "success" || envelope["mode"] != "json" {
		t.Fatalf("top-level envelope changed: %#v", envelope)
	}
	facts, ok := envelope["facts"].(map[string]any)
	if !ok {
		t.Fatalf("facts missing: %#v", envelope)
	}
	for _, key := range []string{"schema_version", "section_count", "handoff_status", "truncated", "source_coverage", "experimental"} {
		if _, ok := facts[key]; !ok {
			t.Fatalf("baseline facts missing %s: %#v", key, facts)
		}
	}
	if facts["handoff_status"] != "missing" {
		t.Fatalf("explicit unknown --handoff must stay missing, got %#v", facts["handoff_status"])
	}
	// 旧 consumer 不需要新字段：recorded run id 只能由 --record-run 产生。
	if _, ok := facts["continuity_run_id"]; ok {
		t.Fatalf("plain continue must not emit continuity_run_id: %#v", facts)
	}
	if envelope["data"] == nil {
		t.Fatalf("baseline envelope must keep data field: %#v", envelope)
	}
}

// TestContinueBaseline_NonRecordedReadOnly 证明默认 continue 调用是 read-only：
// 编译 continuity pack 但不写 continuity run receipt 或 feedback table。
func TestContinueBaseline_NonRecordedReadOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	runCLI(t, "continue", "--vault", root, "--scope", "workspace:default", "--task", "read only check", "--json")

	dbPath := filepath.Join(root, ".pinax", "memory", "agent_memory.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("agent memory db should exist after continue: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open agent memory db: %v", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer func() { _ = sqlDB.Close() }()
	}
	for _, table := range []string{"continuity_runs", "continuity_feedback_events"} {
		var name string
		err := db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name).Error
		if err != nil {
			t.Fatalf("query sqlite_master: %v", err)
		}
		if name == table {
			t.Fatalf("read-only continue must not create %s", table)
		}
	}
}

// TestContinueBaseline_NoVaultFlagKeepsLegacyDefault 固化未绑定、无显式 scope 时
// 继续走 workspace:default legacy fallback，且命令成功（additive binding warning
// 允许存在，但不得改变 status 或旧 facts 语义）。
func TestContinueBaseline_NoVaultFlagKeepsLegacyDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "continue", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("legacy default envelope invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "success" {
		t.Fatalf("legacy default continue must succeed: %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["handoff_status"] != "missing" {
		t.Fatalf("legacy default handoff_status changed: %#v", facts)
	}
	if !strings.Contains(facts["schema_version"].(string), "agent_continuity") {
		t.Fatalf("schema_version changed: %#v", facts)
	}
}
