package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
)

// lifecycleSessionInput 返回一段完整 stdio 会话：握手 + 注册表 + 投影读取。
func lifecycleSessionInput() string {
	return strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"pinax://readiness"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"pinax://manifest"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/list"}`,
	}, "\n") + "\n"
}

// TestMCPStdinEOFDrainsPendingResponsesAndExitsCleanly 验证 gateway 关闭
// Pinax stdio MCP 的 stdin（EOF）后：进程排空已接收请求的响应并干净退出
// （nil 错误、stdout 每行都是完整 JSON-RPC 帧、无孤儿挂起）。
func TestMCPStdinEOFDrainsPendingResponsesAndExitsCleanly(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	var stdout strings.Builder
	serveDone := make(chan error, 1)
	vault := t.TempDir()
	go func() {
		serveDone <- Serve(context.Background(), app.NewService(), vault, reader, &stdout)
	}()

	// 逐帧写入，随后立即关闭 stdin（EOF）：最后一帧的响应必须在退出前排空。
	input := bufio.NewScanner(strings.NewReader(lifecycleSessionInput()))
	for input.Scan() {
		if _, err := writer.Write(append(input.Bytes(), '\n')); err != nil {
			t.Fatalf("write frame: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() on stdin EOF must exit cleanly, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve() did not exit after stdin EOF (orphan process)")
	}

	responses := parseMCPFrameResponses(t, stdout.String())
	if len(responses) != 5 {
		t.Fatalf("drained responses = %d, want 5: %q", len(responses), stdout.String())
	}
	for i, resp := range responses {
		if resp.Error != nil {
			t.Fatalf("response %d unexpected error: %#v", i+1, resp.Error)
		}
		if resp.ID == nil {
			t.Fatalf("response %d missing id: %#v", i+1, resp)
		}
	}
	// 排空即无截断：stdout 每一行都是完整 JSON-RPC 帧（parseMCPFrameResponses 已验证）。
	if strings.Count(strings.TrimRight(stdout.String(), "\n"), "\n") != 4 {
		t.Fatalf("stdout frame count mismatch: %q", stdout.String())
	}
}

// TestMCPRestartProjectionConsistency 验证崩溃/重启后 registry 与 resource
// 投影一致：两个独立 Serve 会话（模拟重启前后）在同一 vault 上对同一组
// 请求产生逐字节一致的响应——投影源于 vault/manifest 状态，不含进程内
// 可变缓存漂移。
func TestMCPRestartProjectionConsistency(t *testing.T) {
	t.Parallel()
	vault := t.TempDir()

	runSession := func() []string {
		var stdout strings.Builder
		err := Serve(context.Background(), app.NewService(), vault,
			strings.NewReader(lifecycleSessionInput()), &stdout)
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
		var frames []string
		for _, line := range strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var probe map[string]any
			if err := json.Unmarshal([]byte(line), &probe); err != nil {
				t.Fatalf("non-parseable frame %q: %v", line, err)
			}
			frames = append(frames, line)
		}
		if len(frames) != 5 {
			t.Fatalf("session frames = %d, want 5: %q", len(frames), stdout.String())
		}
		return frames
	}

	before := runSession()
	after := runSession()
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("projection frame %d drifted across restart:\nbefore: %s\nafter:  %s", i+1, before[i], after[i])
		}
	}
}

// TestMCPReadinessProjectsLifecycleFacts 验证 pinax://readiness 在六层
// readiness 之外投影 lifecycle 事实（gateway supervise 语义）。
func TestMCPReadinessProjectsLifecycleFacts(t *testing.T) {
	t.Parallel()
	server := NewServer(app.NewService(), t.TempDir())
	projection, err := server.resourceProjection(context.Background(), "pinax://readiness")
	if err != nil {
		t.Fatalf("resourceProjection(readiness): %v", err)
	}
	for key, want := range map[string]string{
		"lifecycle_exit":               "stdin_eof_drain_exit",
		"lifecycle_restart_projection": "vault_state_consistent",
		"lifecycle_remote_writes":      "gateway_approval_only",
	} {
		if got := projection.Facts[key]; got != want {
			t.Fatalf("lifecycle fact %s = %q, want %q; facts=%#v", key, got, want, projection.Facts)
		}
	}
}
