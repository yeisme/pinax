package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMCPOfficialSDKRuntimeCandidateProcess 以真实进程验证
// PINAX_MCP_RUNTIME=official-sdk candidate（pinax-mcp-official-sdk-v1 §2.1）：
// 标准 initialize → notifications/initialized → tools/resources 全链路，
// 且私有 server/discover 握手在 candidate 中按设计不可用。
func TestMCPOfficialSDKRuntimeCandidateProcess(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	frames := []string{
		`{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{}}`,
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pinax-e2e-sdk","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"call","method":"tools/call","params":{"name":"pinax.git.snapshot_plan","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest"}}`,
	}

	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "mcp", "serve", "--vault", vault)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "PINAX_MCP_RUNTIME=official-sdk")
	// 官方 SDK 在 stdin EOF 时立即拆除会话；真实客户端在会话期间保持
	// stdio 打开，因此测试以显式管道驱动，读全响应后再关闭。
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start candidate: %v", err)
	}
	for _, frame := range frames {
		if _, err := io.WriteString(stdin, frame+"\n"); err != nil {
			t.Fatalf("write frame: %v", err)
		}
	}
	// 等待全部响应落盘后关闭 stdin 结束会话。
	deadline := time.Now().Add(10 * time.Second)
	wantResponses := 6
	for time.Now().Before(deadline) {
		if countDecodedFrames(stdout.String()) >= wantResponses {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = stdin.Close()
	runErr := cmd.Wait()
	if runErr != nil {
		t.Fatalf("candidate serve failed: %v\nstderr=%s", runErr, stderr.String())
	}
	if strings.Contains(stderr.String(), vault) {
		t.Fatalf("candidate leaked vault path on stderr: %q", stderr.String())
	}
	responses := decodeMCPFrames(t, stdout.String())
	if len(responses) != wantResponses {
		t.Fatalf("candidate responses = %d, want %d: %s", len(responses), wantResponses, stdout.String())
	}

	discover := responseByID(t, responses, "discover")
	if discover == nil || discover["error"] == nil {
		t.Fatalf("private server/discover must be unavailable in the candidate runtime: %#v", discover)
	}

	initialize := responseResult(t, responses, "init")
	if initialize["protocolVersion"] != "2025-11-25" {
		t.Fatalf("candidate initialize = %#v", initialize)
	}
	serverInfo, _ := initialize["serverInfo"].(map[string]any)
	if serverInfo["name"] != "pinax" {
		t.Fatalf("candidate serverInfo = %#v", serverInfo)
	}

	tools := responseResult(t, responses, "tools")
	toolList, _ := tools["tools"].([]any)
	if len(toolList) != 20 {
		t.Fatalf("candidate tools = %d, want 20", len(toolList))
	}

	call := responseResult(t, responses, "call")
	structured, _ := call["structuredContent"].(map[string]any)
	if structured["schema_version"] != "pinax.mcp.tool_result.v1" || structured["status"] != "success" {
		t.Fatalf("candidate call structured content = %#v", structured)
	}

	resources := responseResult(t, responses, "resources")
	resourceList, _ := resources["resources"].([]any)
	if len(resourceList) == 0 {
		t.Fatalf("candidate resources empty")
	}

	manifest := responseResult(t, responses, "manifest")
	contents, _ := manifest["contents"].([]any)
	if len(contents) == 0 {
		t.Fatalf("candidate manifest contents empty")
	}
}

// countDecodedFrames 宽容计数已完整落盘的 JSON-RPC 帧；轮询期间行可能只写了一半，
// 不应让等待循环在半行上 fatal。
func countDecodedFrames(stdout string) int {
	n := 0
	for _, line := range strings.Split(stdout, "\n") {
		var frame map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &frame) == nil {
			n++
		}
	}
	return n
}

// runMCPPiped 以保持 stdin 打开的管道驱动 serve（SDK runtime 在 stdin EOF
// 即拆除会话，一次性 Reader 与真实客户端行为不符），等待 wantResponses 帧后关闭。
func runMCPPiped(t *testing.T, vault string, extraEnv []string, frames []string, wantResponses int) ([]map[string]any, string) {
	t.Helper()
	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "mcp", "serve", "--vault", vault)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	cmd.Env = append(cmd.Env, extraEnv...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	for _, frame := range frames {
		if _, err := io.WriteString(stdin, frame+"\n"); err != nil {
			t.Fatalf("write frame: %v", err)
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if countDecodedFrames(stdout.String()) >= wantResponses {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("serve failed: %v\nstderr=%s", err, stderr.String())
	}
	return decodeMCPFrames(t, stdout.String()), stderr.String()
}

func decodeMCPFrames(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("non-JSON MCP stdout frame: %v\n%s", err, line)
		}
		responses = append(responses, response)
	}
	return responses
}

// TestMCPRuntimeDefaultSwitch 验证 pinax-mcp-official-sdk-v1 §4.4 的默认切换与兼容窗口：
// 缺省 PINAX_MCP_RUNTIME 走官方 SDK（私有 server/discover 不可用），
// PINAX_MCP_RUNTIME=legacy 显式回退手写实现（server/discover 可用），
// 未知取值 fail closed 报错退出。
func TestMCPRuntimeDefaultSwitch(t *testing.T) {
	t.Parallel()

	frames := []string{
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pinax-e2e-switch","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"unknown","method":"tools/call","params":{"name":"__runtime_probe__","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`,
	}

	runServe := func(env string) ([]map[string]any, string) {
		vault := t.TempDir()
		cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "mcp", "serve", "--vault", vault)
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
		if env != "" {
			cmd.Env = append(cmd.Env, "PINAX_MCP_RUNTIME="+env)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start serve: %v", err)
		}
		for _, frame := range frames {
			if _, err := io.WriteString(stdin, frame+"\n"); err != nil {
				t.Fatalf("write frame: %v", err)
			}
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if countDecodedFrames(stdout.String()) >= 5 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatalf("serve (env=%q) failed: %v\nstderr=%s", env, err, stderr.String())
		}
		return decodeMCPFrames(t, stdout.String()), stderr.String()
	}

	// 缺省 runtime = 官方 SDK：标准 initialize 结果不含 legacy 私有 read_only 字段，
	// 未注册工具按 SDK 侧错误码 -32602 拒绝（冻结差异表）。
	responses, _ := runServe("")
	initialize := responseResult(t, responses, "init")
	if _, ok := initialize["read_only"]; ok {
		t.Fatalf("default runtime must not project legacy read_only envelope: %#v", initialize)
	}
	if tools := responseResult(t, responses, "tools"); tools == nil {
		t.Fatalf("default runtime tools/list missing")
	}

	// 私有 server/discover 握手在两个 runtime 均经 Server.Handle 单点可用
	// （4.4 受控探针确认；无 _meta 的错误语义两个 runtime 一致）。
	responses, _ = runServe("legacy")
	legacyInit := responseResult(t, responses, "init")
	if legacyInit["read_only"] != true {
		t.Fatalf("legacy runtime must keep read_only envelope: %#v", legacyInit)
	}
	discover := responseByID(t, responses, "discover")
	if discover == nil || discover["result"] == nil {
		t.Fatalf("legacy runtime must keep server/discover: %#v", discover)
	}
	responses, _ = runServe("")
	discover = responseByID(t, responses, "discover")
	if discover == nil || discover["result"] == nil {
		t.Fatalf("server/discover with protocol _meta must stay available on the default runtime: %#v", discover)
	}
	unknown := responseByID(t, responses, "unknown")
	if unknown == nil {
		t.Fatalf("unknown tool probe response missing")
	}
	unknownErr, _ := unknown["error"].(map[string]any)
	if code, _ := unknownErr["code"].(float64); code != -32602 {
		t.Fatalf("default runtime unknown tool error = %#v, want -32602", unknown["error"])
	}

	// 未知取值 fail closed。
	vault := t.TempDir()
	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "mcp", "serve", "--vault", vault)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "PINAX_MCP_RUNTIME=future-runtime")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("unknown PINAX_MCP_RUNTIME must fail closed")
	}
	if !strings.Contains(stderr.String(), "unknown PINAX_MCP_RUNTIME") {
		t.Fatalf("unknown runtime error message = %q", stderr.String())
	}
}
