// Package evidence writes local integration/e2e evidence for Pinax test runs.
//
// 证据目录结构由 integration evidence runner 写入，供 QA、review 和 closeout
// 使用。本包把核心写入逻辑从 main 中抽离，让测试可以直接验证：
//  1. 成功和失败都会生成完整证据目录；
//  2. 失败时保留原始退出码，不吞 stderr；
//  3. 证据文件不含 token、Authorization header、raw provider payload、绝对路径
//     等敏感串——所有写入 stdout/stderr/command/env 的内容都经过 Redact 处理。
package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SchemaVersion 是 evidence summary 的稳定 schema 版本。
const SchemaVersion = "yeisme.integration_test_evidence.v1"

// EnvSchemaVersion 是 env.json 的稳定 schema 版本。
const EnvSchemaVersion = "yeisme.integration_test_env.v1"

// Project 标识证据归属的子项目。
const Project = "cli/pinax"

// Layer 标识证据来源的测试层。
const Layer = "integration-evidence"

// 脱敏正则：覆盖 Authorization header、bearer token、常见 secret 形式和绝对路径。
var (
	authorizationPattern = regexp.MustCompile(`(?i)Authorization\s*[:=]\s*Bearer\s+[^\s,;"']+`)
	bearerTokenPattern   = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
	tokenKVPattern       = regexp.MustCompile(`(?i)(token|api_key|apikey|secret|password|passwd|access_key|secret_key)\s*[:=]\s*[^\s,;"']+`)
	// CLI 密钥旗标：--api-key <value>、--token <value> 等后面跟一个裸值。
	secretFlagPattern = regexp.MustCompile(`(?i)(--api-key|--apikey|--token|--secret|--password|--passwd|--access-key|--secret-key)\s+[^\s,;"']+`)
	// 绝对路径：Unix 和 Windows 风格。把 CWD 和临时目录前缀替换为占位符，
	// 避免泄露开发机器的完整路径结构。
	unixAbsPathPattern    = regexp.MustCompile(`/(tmp|var|home|Users|workspaces|root|opt|usr|etc|mnt|media|private)/[^\s"',:;)\]]*`)
	windowsAbsPathPattern = regexp.MustCompile(`[A-Z]:\\[^\s"',:;)\]]*`)
)

// Redaction 描述本次证据写入应用的脱敏策略，写入 summary.redaction。
type Redaction struct {
	Applied          bool     `json:"applied"`
	SchemaVersion    string   `json:"schema_version"`
	ScannedSurfaces  []string `json:"scanned_surfaces"`
	ForbiddenClasses []string `json:"forbidden_classes"`
	PathRedacted     bool     `json:"path_redacted"`
}

// forbiddenClasses 是 Redaction 报告的受保护敏感类别。
var forbiddenClasses = []string{
	"authorization_header",
	"bearer_token",
	"secret_kv",
	"absolute_path",
}

// Redact 对一段文本执行脱敏：替换 Authorization/Bearer、token/secret/password
// 键值对、CLI 密钥旗标值，以及绝对路径。返回脱敏后的文本。这是证据写入前的
// 必经关卡。
func Redact(input string) string {
	out := authorizationPattern.ReplaceAllString(input, "Authorization=Bearer [REDACTED]")
	out = bearerTokenPattern.ReplaceAllString(out, "Bearer [REDACTED]")
	out = tokenKVPattern.ReplaceAllString(out, "${1}=[REDACTED]")
	out = secretFlagPattern.ReplaceAllString(out, "${1} [REDACTED]")
	out = unixAbsPathPattern.ReplaceAllString(out, "[REDACTED_PATH]/")
	out = windowsAbsPathPattern.ReplaceAllString(out, "[REDACTED_PATH]/")
	return out
}

// Config 控制 Run 的行为。
type Config struct {
	// RunID 是本次运行的唯一标识，用作证据子目录名。
	RunID string
	// ParentDir 是存放运行子目录的父目录，例如 temp/integration-test-runs。
	ParentDir string
	// Command 是要执行并采集证据的命令。
	Command []string
	// PassThroughStdout/Stderr 不为 nil 时，命令原始（未脱敏）输出同时写到
	// 对应 writer；证据文件始终写入脱敏后的版本。
	PassThroughStdout io.Writer
	PassThroughStderr io.Writer
	// ExtraChecks 是写入 summary.json checks 字段的额外键。
	ExtraChecks map[string]any
}

// Result 描述一次证据运行的结果。
type Result struct {
	ExitCode int
	RunDir   string
	Summary  Summary
}

type summary struct {
	SchemaVersion string         `json:"schema_version"`
	Project       string         `json:"project"`
	Layer         string         `json:"layer"`
	RunID         string         `json:"run_id"`
	Status        string         `json:"status"`
	Command       []string       `json:"command"`
	ExitCode      int            `json:"exit_code"`
	StartedAt     string         `json:"started_at"`
	EndedAt       string         `json:"ended_at"`
	FinishedAt    string         `json:"finished_at"`
	DurationMS    int64          `json:"duration_ms"`
	Evidence      evidenceFiles  `json:"evidence"`
	Checks        map[string]any `json:"checks"`
	Redaction     Redaction      `json:"redaction"`
}

type evidenceFiles struct {
	Command  string `json:"command"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Env      string `json:"env"`
	Artifact string `json:"artifact_dir"`
}

type envInfo struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	CWD           string `json:"cwd"`
	GoVersion     string `json:"go_version"`
	Network       string `json:"network"`
	Redacted      bool   `json:"redacted"`
}

// Summary 是 summary.json 的公开表示，供测试断言使用。
type Summary struct {
	SchemaVersion string
	Project       string
	Layer         string
	RunID         string
	Status        string
	ExitCode      int
	FinishedAt    string
	Checks        map[string]any
	Redaction     Redaction
}

// Run 执行 cfg.Command，把 stdout/stderr/env/summary 写入证据目录，并返回退出码。
// 成功和失败都会写完整证据；失败时保留原始退出码。所有证据文件都经过 Redact
// 脱敏，确保不泄露 token、secret 或绝对路径。
func Run(cfg Config) (Result, error) {
	if cfg.RunID == "" {
		cfg.RunID = time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	}
	started := time.Now().UTC()
	runDir := filepath.Join(cfg.ParentDir, cfg.RunID)
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create evidence dir: %w", err)
	}
	// command.txt 写入脱敏后的 argv，避免泄露路径或注入的 secret 参数。
	if err := os.WriteFile(filepath.Join(runDir, "command.txt"), []byte(Redact(strings.Join(cfg.Command, " "))+"\n"), 0o644); err != nil {
		return Result{}, fmt.Errorf("write command.txt: %w", err)
	}
	if err := writeEnv(runDir, cfg.RunID); err != nil {
		return Result{}, fmt.Errorf("write env.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "README.txt"), []byte("Integration test artifacts are generated by internal/testkit/evidence.\n"), 0o644); err != nil {
		return Result{}, fmt.Errorf("write artifacts README: %w", err)
	}

	stdoutFile, err := os.Create(filepath.Join(runDir, "stdout.log"))
	if err != nil {
		return Result{}, fmt.Errorf("create stdout log: %w", err)
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(filepath.Join(runDir, "stderr.log"))
	if err != nil {
		return Result{}, fmt.Errorf("create stderr log: %w", err)
	}
	defer func() { _ = stderrFile.Close() }()

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	ended := time.Now().UTC()

	// 证据文件写入脱敏后的版本；pass-through 写原始版本（供终端实时查看）。
	redactedStdout := Redact(stdoutBuf.String())
	redactedStderr := Redact(stderrBuf.String())
	if cfg.PassThroughStdout != nil {
		_, _ = io.WriteString(cfg.PassThroughStdout, stdoutBuf.String())
	}
	if cfg.PassThroughStderr != nil {
		_, _ = io.WriteString(cfg.PassThroughStderr, stderrBuf.String())
	}
	if _, err := stdoutFile.WriteString(redactedStdout); err != nil {
		return Result{ExitCode: exitCode, RunDir: runDir}, fmt.Errorf("write stdout.log: %w", err)
	}
	if _, err := stderrFile.WriteString(redactedStderr); err != nil {
		return Result{ExitCode: exitCode, RunDir: runDir}, fmt.Errorf("write stderr.log: %w", err)
	}

	status := "success"
	if exitCode != 0 {
		status = "failed"
	}
	checks := map[string]any{
		"stdout_bytes": len(redactedStdout),
		"stderr_bytes": len(redactedStderr),
		"redacted":     true,
	}
	for k, v := range cfg.ExtraChecks {
		checks[k] = v
	}
	redaction := Redaction{
		Applied:          true,
		SchemaVersion:    "yeisme.redaction.v1",
		ScannedSurfaces:  []string{"stdout", "stderr", "command", "env"},
		ForbiddenClasses: forbiddenClasses,
		PathRedacted:     true,
	}
	s := summary{
		SchemaVersion: SchemaVersion,
		Project:       Project,
		Layer:         Layer,
		RunID:         cfg.RunID,
		Status:        status,
		Command:       splitRedacted(cfg.Command),
		ExitCode:      exitCode,
		StartedAt:     started.Format(time.RFC3339),
		EndedAt:       ended.Format(time.RFC3339),
		FinishedAt:    ended.Format(time.RFC3339),
		DurationMS:    ended.Sub(started).Milliseconds(),
		Evidence: evidenceFiles{
			Command:  "command.txt",
			Stdout:   "stdout.log",
			Stderr:   "stderr.log",
			Env:      "env.json",
			Artifact: "artifacts",
		},
		Checks:    checks,
		Redaction: redaction,
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), s); err != nil {
		return Result{ExitCode: exitCode, RunDir: runDir}, fmt.Errorf("write summary: %w", err)
	}
	return Result{
		ExitCode: exitCode,
		RunDir:   runDir,
		Summary: Summary{
			SchemaVersion: s.SchemaVersion,
			Project:       s.Project,
			Layer:         s.Layer,
			RunID:         s.RunID,
			Status:        s.Status,
			ExitCode:      s.ExitCode,
			FinishedAt:    s.FinishedAt,
			Checks:        s.Checks,
			Redaction:     s.Redaction,
		},
	}, nil
}

// splitRedacted 对 argv 数组脱敏，保持 JSON 数组结构。密钥旗标（如 --api-key）
// 的下一个元素被视为它的值并整体替换，避免逐元素脱敏时漏掉裸值。
func splitRedacted(command []string) []string {
	out := make([]string, 0, len(command))
	secretFlagValues := map[string]bool{
		"--api-key": true, "--apikey": true, "--token": true,
		"--secret": true, "--password": true, "--passwd": true,
		"--access-key": true, "--secret-key": true,
	}
	for i := 0; i < len(command); i++ {
		arg := command[i]
		// 如果当前元素是密钥旗标且存在下一个元素，把值替换为 [REDACTED]。
		if secretFlagValues[strings.ToLower(arg)] && i+1 < len(command) {
			out = append(out, Redact(arg), "[REDACTED]")
			i++
			continue
		}
		out = append(out, Redact(arg))
	}
	return out
}

func writeEnv(runDir, runID string) error {
	goVersion := "unknown"
	if out, err := exec.Command("go", "version").Output(); err == nil {
		goVersion = strings.TrimSpace(string(out))
	}
	cwd, _ := os.Getwd()
	// env.json 的 cwd 也脱敏绝对路径，避免泄露开发机器结构。
	return writeJSON(filepath.Join(runDir, "env.json"), envInfo{SchemaVersion: EnvSchemaVersion, RunID: runID, CWD: Redact(cwd), GoVersion: goVersion, Network: "not_required", Redacted: true})
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
