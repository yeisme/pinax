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
		if len(decodeMCPFrames(t, stdout.String())) >= wantResponses {
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
