package continuityevidence

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yeisme/pinax/internal/agentprotocol"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test.sqlite"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	store, err := Open(db)
	if err != nil {
		t.Fatalf("open continuity evidence store: %v", err)
	}
	return store
}

func sampleRun(id, runtime, class string, startedAt time.Time) ContinuityRunRow {
	return ContinuityRunRow{
		RunID:          id,
		BindingDigest:  "0123456789abcdef",
		ScopeKind:      "project",
		ScopeIDDigest:  "fedcba9876543210",
		Runtime:        runtime,
		TaskClass:      class,
		StartedAt:      startedAt,
		HandoffStatus:  "consumed",
		SourceTotal:    2,
		SourceResolved: 2,
	}
}

// TestContinuityRunRecordRoundTrip 覆盖 fresh DB 建表、写入、读取。
func TestContinuityRunRecordRoundTrip(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	run := sampleRun("run_a", "codex", string(TaskClassImplementationDebugging), now)
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	loaded, err := store.GetRun(context.Background(), "run_a")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loaded.Runtime != "codex" || loaded.SourceResolved != 2 {
		t.Fatalf("run round trip mismatch: %#v", loaded)
	}
}

// TestContinuityRunRejectsInvalidEnumsBeforeWrite 覆盖 invalid enum 在写入前失败。
func TestContinuityRunRejectsInvalidEnumsBeforeWrite(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	bad := sampleRun("run_bad", "codex", "coffee_tasting", time.Now().UTC())
	err := store.CreateRun(context.Background(), bad)
	if se, ok := err.(*agentprotocol.StableError); !ok || se.Code != agentprotocol.ErrCodeValidationFailed {
		t.Fatalf("invalid task class must fail before write: %#v", err)
	}
	if _, getErr := store.GetRun(context.Background(), "run_bad"); getErr == nil {
		t.Fatal("invalid enum run must not be persisted")
	}
	if err := store.CreateRun(context.Background(), sampleRun("run_noruntime", "", "implementation_debugging", time.Now().UTC())); err == nil {
		t.Fatal("missing runtime must fail validation")
	}
}

// TestContinuityFeedbackAppendOnlySupersede 覆盖 append-only 与 supersession fold。
func TestContinuityFeedbackAppendOnlySupersede(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := store.CreateRun(context.Background(), sampleRun("run_fb", "claude-code", string(TaskClassProductSpecDocs), now)); err != nil {
		t.Fatalf("create run: %v", err)
	}
	first := ContinuityFeedbackEventRow{
		FeedbackID: "fb_1", RunID: "run_fb", EventKind: EventKindOutcome,
		Outcome: string(OutcomeCorrected), SubmittedAt: now.Add(time.Minute),
	}
	if err := store.AppendFeedback(context.Background(), first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	// 用户改选：追加 superseding event，旧 event 保留。
	second := ContinuityFeedbackEventRow{
		FeedbackID: "fb_2", RunID: "run_fb", EventKind: EventKindOutcome,
		Outcome: string(OutcomeTrusted), SubmittedAt: now.Add(2 * time.Minute),
		SupersedesFeedbackID: "fb_1",
	}
	if err := store.AppendFeedback(context.Background(), second); err != nil {
		t.Fatalf("append supersede: %v", err)
	}
	events, err := store.ListFeedbackSince(context.Background(), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("list feedback: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("append-only audit must keep both events, got %d", len(events))
	}
	latest := LatestOutcome(events)
	if latest["run_fb"] != OutcomeTrusted {
		t.Fatalf("latest fold = %s, want trusted", latest["run_fb"])
	}
}

// TestContinuityFeedbackRejectsInvalidOutcome 覆盖四值枚举校验与空 run。
func TestContinuityFeedbackRejectsInvalidOutcome(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	bad := ContinuityFeedbackEventRow{
		FeedbackID: "fb_bad", RunID: "run_x", EventKind: EventKindOutcome,
		Outcome: "loved_it", SubmittedAt: time.Now().UTC(),
	}
	if se, ok := store.AppendFeedback(context.Background(), bad).(*agentprotocol.StableError); !ok || se.Code != agentprotocol.ErrCodeValidationFailed {
		t.Fatalf("invalid outcome must fail validation: %#v", store.AppendFeedback(context.Background(), bad))
	}
	noRun := ContinuityFeedbackEventRow{
		FeedbackID: "fb_norun", EventKind: EventKindOutcome,
		Outcome: string(OutcomeTrusted), SubmittedAt: time.Now().UTC(),
	}
	if err := store.AppendFeedback(context.Background(), noRun); err == nil {
		t.Fatal("outcome feedback without run_id must fail")
	}
}

// TestContinuityFeedbackWeeklyReviewValidation 覆盖 weekly review 非负整数约束。
func TestContinuityFeedbackWeeklyReviewValidation(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	neg := -1
	bad := ContinuityFeedbackEventRow{
		FeedbackID: "fb_wr_bad", EventKind: EventKindWeeklyReview,
		ReviewSeconds: &neg, SubmittedAt: time.Now().UTC(),
	}
	if err := store.AppendFeedback(context.Background(), bad); err == nil {
		t.Fatal("negative review_seconds must fail")
	}
	zero := 0
	good := ContinuityFeedbackEventRow{
		FeedbackID: "fb_wr_ok", EventKind: EventKindWeeklyReview,
		ReviewSeconds: &zero, SubmittedAt: time.Now().UTC(),
	}
	if err := store.AppendFeedback(context.Background(), good); err != nil {
		t.Fatalf("zero review_seconds must be allowed (empty inbox): %v", err)
	}
}

// TestContinuityEvidenceMigrationIdempotent 覆盖旧 DB 自动迁移幂等：
// 重复 Open 不报错，已有行保留。
func TestContinuityEvidenceMigrationIdempotent(t *testing.T) {
	t.Parallel()
	dbPath := "file:" + t.TempDir() + "/migrate.sqlite"
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	first, err := Open(db)
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := first.CreateRun(context.Background(), sampleRun("run_keep", "codex", "release_operations", time.Now().UTC())); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	// 第二次 migrate（模拟旧 DB 重启路径）。
	if _, err := Open(db); err != nil {
		t.Fatalf("second migrate must be idempotent: %v", err)
	}
	if _, err := first.GetRun(context.Background(), "run_keep"); err != nil {
		t.Fatalf("existing rows must survive remigration: %v", err)
	}
}

// TestContinuityRunNoContentFields 用反射证明 run receipt 模型不含正文/path/prompt 字段。
// 这是 evidence redaction 的结构性保证（而不是靠运行时正则兜底）。
func TestContinuityRunNoContentFields(t *testing.T) {
	t.Parallel()
	forbidden := []string{"title", "body", "prompt", "transcript", "path", "payload", "token", "secret"}
	check := func(name string) {
		lower := strings.ToLower(name)
		for _, word := range forbidden {
			if strings.Contains(lower, word) {
				t.Fatalf("evidence model field %q must not contain content/secret semantics", name)
			}
		}
	}
	runType := reflect.TypeOf(ContinuityRunRow{})
	for i := 0; i < runType.NumField(); i++ {
		check(runType.Field(i).Name)
	}
	eventType := reflect.TypeOf(ContinuityFeedbackEventRow{})
	for i := 0; i < eventType.NumField(); i++ {
		check(eventType.Field(i).Name)
	}
}
