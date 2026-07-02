package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWASMRunnerBuildsSandboxedCallEnvelope(t *testing.T) {
	plugin := RegistryPlugin{ID: "project-dashboard", Version: "0.1.0", Runtime: RuntimeWASM, Enabled: true}
	runner := WASMRunner{Invoke: func(ctx context.Context, call CallEnvelope) (ResultEnvelope, error) {
		if call.SchemaVersion != PluginCallSchema || call.PluginID != "project-dashboard" || call.Capability != "render_dashboard" {
			t.Fatalf("call identity = %#v", call)
		}
		if call.Permissions.Network || len(call.Permissions.Env) != 0 || call.Permissions.FilesystemRead != "none" || call.Permissions.FilesystemWrite != "none" {
			t.Fatalf("wasm call was not sandboxed: %#v", call.Permissions)
		}
		if call.Budgets.TimeoutMS != 50 || call.Budgets.MaxOutputBytes != 1024 {
			t.Fatalf("call budgets = %#v", call.Budgets)
		}
		if strings.Contains(string(call.Input), "Authorization") || strings.Contains(string(call.Input), "raw prompt") {
			t.Fatalf("call input leaked sensitive data: %s", call.Input)
		}
		return ResultEnvelope{SchemaVersion: PluginResultSchema, Status: "success", Facts: map[string]string{"cards": "2"}, Data: map[string]any{"title": "Dashboard"}}, nil
	}}

	result, err := runner.Run(context.Background(), RunRequest{Plugin: plugin, Capability: "render_dashboard", Input: map[string]any{"task": "summarize", "Authorization": "Bearer raw", "raw prompt": "hidden"}, Budgets: RunnerBudgets{TimeoutMS: 50, MaxInputBytes: 4096, MaxOutputBytes: 1024, MaxMemoryMB: 64}})
	if err != nil {
		t.Fatalf("run wasm: %v", err)
	}
	if result.Status != "success" || result.Facts["cards"] != "2" {
		t.Fatalf("result = %#v", result)
	}
}

func TestWASMRunnerBudgetAndSandboxFailures(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		runner := WASMRunner{Invoke: func(ctx context.Context, call CallEnvelope) (ResultEnvelope, error) {
			<-ctx.Done()
			return ResultEnvelope{}, ctx.Err()
		}}
		_, err := runner.Run(context.Background(), RunRequest{Plugin: RegistryPlugin{ID: "slow", Runtime: RuntimeWASM, Enabled: true}, Capability: "render", Budgets: RunnerBudgets{TimeoutMS: 1, MaxInputBytes: 1024, MaxOutputBytes: 1024, MaxMemoryMB: 64}})
		if code := RunnerErrorCode(err); code != "plugin_budget_exceeded" {
			t.Fatalf("timeout code = %s err=%v", code, err)
		}
	})

	t.Run("output budget", func(t *testing.T) {
		runner := WASMRunner{Invoke: func(ctx context.Context, call CallEnvelope) (ResultEnvelope, error) {
			return ResultEnvelope{SchemaVersion: PluginResultSchema, Status: "success", Data: strings.Repeat("x", 2048)}, nil
		}}
		_, err := runner.Run(context.Background(), RunRequest{Plugin: RegistryPlugin{ID: "large", Runtime: RuntimeWASM, Enabled: true}, Capability: "render", Budgets: RunnerBudgets{TimeoutMS: 100, MaxInputBytes: 1024, MaxOutputBytes: 128, MaxMemoryMB: 64}})
		if code := RunnerErrorCode(err); code != "plugin_budget_exceeded" {
			t.Fatalf("output code = %s err=%v", code, err)
		}
	})

	t.Run("unsupported default adapter", func(t *testing.T) {
		_, err := (WASMRunner{}).Run(context.Background(), RunRequest{Plugin: RegistryPlugin{ID: "missing", Runtime: RuntimeWASM, Enabled: true}, Capability: "render", Budgets: RunnerBudgets{TimeoutMS: 100, MaxInputBytes: 1024, MaxOutputBytes: 1024, MaxMemoryMB: 64}})
		if !errors.Is(err, ErrRunnerUnavailable) || RunnerErrorCode(err) != "plugin_runner_unavailable" {
			t.Fatalf("default adapter err = %v", err)
		}
	})
}
