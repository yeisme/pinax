package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const PluginCallSchema = "pinax.plugin.call.v1"
const PluginResultSchema = "pinax.plugin.result.v1"

var ErrRunnerUnavailable = errors.New("plugin runner unavailable")

type RunnerBudgets struct {
	TimeoutMS      int `json:"timeout_ms"`
	MaxInputBytes  int `json:"max_input_bytes"`
	MaxOutputBytes int `json:"max_output_bytes"`
	MaxMemoryMB    int `json:"max_memory_mb"`
}

type RunRequest struct {
	Plugin     RegistryPlugin
	Capability string
	Input      any
	Budgets    RunnerBudgets
}

type CallEnvelope struct {
	SchemaVersion string            `json:"schema_version"`
	CallID        string            `json:"call_id"`
	PluginID      string            `json:"plugin_id"`
	Capability    string            `json:"capability"`
	Input         json.RawMessage   `json:"input"`
	Permissions   RuntimePermission `json:"permissions"`
	Budgets       RunnerBudgets     `json:"budgets"`
}

type RuntimePermission struct {
	Network         bool     `json:"network"`
	Env             []string `json:"env"`
	FilesystemRead  string   `json:"filesystem_read"`
	FilesystemWrite string   `json:"filesystem_write"`
}

type ResultEnvelope struct {
	SchemaVersion string            `json:"schema_version"`
	Status        string            `json:"status"`
	Facts         map[string]string `json:"facts,omitempty"`
	Data          any               `json:"data,omitempty"`
	ActionPlan    any               `json:"action_plan,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
}

type RunnerError struct {
	Code    string
	Message string
	Err     error
}

func (e *RunnerError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *RunnerError) Unwrap() error { return e.Err }

type WASMRunner struct {
	Invoke func(context.Context, CallEnvelope) (ResultEnvelope, error)
}

func (r WASMRunner) Run(ctx context.Context, req RunRequest) (ResultEnvelope, error) {
	if r.Invoke == nil {
		return ResultEnvelope{}, &RunnerError{Code: "plugin_runner_unavailable", Message: "WASM runtime adapter is not configured", Err: ErrRunnerUnavailable}
	}
	budgets := defaultRunnerBudgets(req.Budgets)
	input, err := boundedInput(req.Input, budgets.MaxInputBytes)
	if err != nil {
		return ResultEnvelope{}, err
	}
	runCtx := ctx
	cancel := func() {}
	if budgets.TimeoutMS > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(budgets.TimeoutMS)*time.Millisecond)
	}
	defer cancel()
	call := CallEnvelope{SchemaVersion: PluginCallSchema, CallID: callID(req.Plugin.ID, req.Capability), PluginID: req.Plugin.ID, Capability: req.Capability, Input: input, Permissions: wasmDefaultPermissions(), Budgets: budgets}
	result, err := r.Invoke(runCtx, call)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return ResultEnvelope{}, &RunnerError{Code: "plugin_budget_exceeded", Message: "Plugin runtime exceeded timeout budget"}
		}
		return ResultEnvelope{}, &RunnerError{Code: "plugin_runner_failed", Message: "Plugin runtime failed", Err: err}
	}
	if err := validateResult(result, budgets.MaxOutputBytes); err != nil {
		return ResultEnvelope{}, err
	}
	return result, nil
}

func RunnerErrorCode(err error) string {
	var runnerErr *RunnerError
	if errors.As(err, &runnerErr) {
		return runnerErr.Code
	}
	return ""
}

func defaultRunnerBudgets(b RunnerBudgets) RunnerBudgets {
	if b.TimeoutMS <= 0 {
		b.TimeoutMS = 3000
	}
	if b.MaxInputBytes <= 0 {
		b.MaxInputBytes = 262144
	}
	if b.MaxOutputBytes <= 0 {
		b.MaxOutputBytes = 262144
	}
	if b.MaxMemoryMB <= 0 {
		b.MaxMemoryMB = 64
	}
	return b
}

func RunnerBudgetsFromManifest(b Budgets) RunnerBudgets {
	return defaultRunnerBudgets(RunnerBudgets(b))
}

func boundedInput(input any, maxBytes int) (json.RawMessage, error) {
	// 插件输入只能是经过 Pinax 投影后的 bounded JSON；这里先移除明显敏感键，
	// 防止后续真实 WASM adapter 接入前测试夹具误把 auth/prompt 类字段送进 runner。
	sanitized := sanitizePluginInput(input)
	body, err := json.Marshal(sanitized)
	if err != nil {
		return nil, &RunnerError{Code: "plugin_input_invalid", Message: "Plugin input cannot be encoded", Err: err}
	}
	if len(body) > maxBytes {
		return nil, &RunnerError{Code: "plugin_budget_exceeded", Message: "Plugin input exceeded byte budget"}
	}
	return body, nil
}

func sanitizePluginInput(input any) any {
	switch typed := input.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, value := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "cookie") || strings.Contains(lower, "prompt") {
				continue
			}
			out[key] = sanitizePluginInput(value)
		}
		return out
	case map[string]string:
		out := map[string]string{}
		for key, value := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "cookie") || strings.Contains(lower, "prompt") {
				continue
			}
			out[key] = value
		}
		return out
	default:
		return typed
	}
}

func wasmDefaultPermissions() RuntimePermission {
	// WASM 首版默认无网络、无 env、无宿主文件系统；需要 WASI 时必须另走权限评估。
	return RuntimePermission{Network: false, Env: []string{}, FilesystemRead: "none", FilesystemWrite: "none"}
}

func validateResult(result ResultEnvelope, maxBytes int) error {
	if result.SchemaVersion != PluginResultSchema {
		return &RunnerError{Code: "plugin_result_invalid", Message: "Plugin result schema_version is invalid"}
	}
	if result.Status != "success" && result.Status != "partial" && result.Status != "failed" {
		return &RunnerError{Code: "plugin_result_invalid", Message: "Plugin result status is invalid"}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return &RunnerError{Code: "plugin_result_invalid", Message: "Plugin result cannot be encoded", Err: err}
	}
	if len(body) > maxBytes {
		return &RunnerError{Code: "plugin_budget_exceeded", Message: "Plugin output exceeded byte budget"}
	}
	return nil
}

func callID(pluginID, capability string) string {
	return fmt.Sprintf("plugincall_%s_%s", strings.ReplaceAll(pluginID, "-", "_"), strings.ReplaceAll(capability, "-", "_"))
}
