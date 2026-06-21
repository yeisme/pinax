package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ExternalRunner struct{}

type ExternalRunRequest struct {
	Manifest   Manifest
	PluginRoot string
	Capability string
	Input      any
	Budgets    RunnerBudgets
}

func (r ExternalRunner) Run(ctx context.Context, req ExternalRunRequest) (ResultEnvelope, error) {
	budgets := defaultRunnerBudgets(req.Budgets)
	input, err := boundedInput(req.Input, budgets.MaxInputBytes)
	if err != nil {
		return ResultEnvelope{}, err
	}
	executable, args, err := externalCommand(req.Manifest, req.PluginRoot)
	if err != nil {
		return ResultEnvelope{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(budgets.TimeoutMS)*time.Millisecond)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "pinax-plugin-run-*")
	if err != nil {
		return ResultEnvelope{}, &RunnerError{Code: "plugin_runner_unavailable", Message: "Plugin temp cwd could not be created", Err: err}
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	call := CallEnvelope{SchemaVersion: PluginCallSchema, CallID: callID(req.Manifest.ID, req.Capability), PluginID: req.Manifest.ID, Capability: req.Capability, Input: input, Permissions: externalDefaultPermissions(), Budgets: budgets}
	stdin, err := json.Marshal(call)
	if err != nil {
		return ResultEnvelope{}, &RunnerError{Code: "plugin_input_invalid", Message: "Plugin call cannot be encoded", Err: err}
	}
	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Dir = tempDir
	cmd.Env = limitedRunnerEnv()
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return ResultEnvelope{}, &RunnerError{Code: "plugin_budget_exceeded", Message: "Plugin runtime exceeded timeout budget"}
		}
		return ResultEnvelope{}, &RunnerError{Code: "plugin_runner_failed", Message: "Plugin process failed", Err: err}
	}
	if stdout.Len() > budgets.MaxOutputBytes {
		return ResultEnvelope{}, &RunnerError{Code: "plugin_budget_exceeded", Message: "Plugin output exceeded byte budget"}
	}
	var result ResultEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return ResultEnvelope{}, &RunnerError{Code: "plugin_result_invalid", Message: "Plugin process returned invalid JSON", Err: err}
	}
	if err := validateResult(result, budgets.MaxOutputBytes); err != nil {
		return ResultEnvelope{}, err
	}
	return result, nil
}

func externalCommand(manifest Manifest, pluginRoot string) (string, []string, error) {
	entrypoint := filepath.Clean(strings.TrimSpace(manifest.Runtime.Entrypoint))
	if entrypoint == "" || filepath.IsAbs(entrypoint) || strings.HasPrefix(entrypoint, "..") {
		return "", nil, &RunnerError{Code: "plugin_entrypoint_invalid", Message: "Plugin entrypoint must be a safe relative path"}
	}
	fullEntrypoint, err := filepath.Abs(filepath.Join(pluginRoot, entrypoint))
	if err != nil {
		return "", nil, &RunnerError{Code: "plugin_entrypoint_invalid", Message: "Plugin entrypoint path could not be resolved", Err: err}
	}
	switch manifest.Runtime.Kind {
	case RuntimePython:
		python, err := exec.LookPath("python3")
		if err != nil {
			return "", nil, &RunnerError{Code: "plugin_runner_unavailable", Message: "Python runner python3 is unavailable", Err: ErrRunnerUnavailable}
		}
		return python, []string{fullEntrypoint}, nil
	case RuntimeJavaScript:
		node, err := exec.LookPath("node")
		if err != nil {
			return "", nil, &RunnerError{Code: "plugin_runner_unavailable", Message: "JavaScript runner node is unavailable", Err: ErrRunnerUnavailable}
		}
		return node, []string{fullEntrypoint}, nil
	case RuntimeProcess:
		return fullEntrypoint, nil, nil
	default:
		return "", nil, &RunnerError{Code: "plugin_runtime_unsupported", Message: "Plugin runtime is not an external runner"}
	}
}

func externalDefaultPermissions() RuntimePermission {
	// JS/Python/process 首版是 trusted runner，但仍不继承宿主 env，cwd 固定在临时目录，输入只走 JSON envelope。
	return RuntimePermission{Network: false, Env: []string{}, FilesystemRead: "none", FilesystemWrite: "temp"}
}

func limitedRunnerEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	if logPath := strings.TrimSpace(os.Getenv("PINAX_RUNNER_LOG")); logPath != "" {
		env = append(env, "PINAX_RUNNER_LOG="+logPath)
	}
	return env
}
