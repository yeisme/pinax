package publishops

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/redaction"
)

type RendererAdapter struct {
	Executable string
	PackageDir string
	Timeout    time.Duration
}

type RendererRequest struct {
	BundleRoot      string
	OutDir          string
	BaseURL         string
	Theme           string
	RendererVersion string
}

type RendererCallResult struct {
	CallID     string `json:"call_id"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

func (a RendererAdapter) RenderStatic(ctx context.Context, req RendererRequest) (RendererCallResult, error) {
	args := []string{"--bundle", req.BundleRoot, "--out", req.OutDir}
	if strings.TrimSpace(req.BaseURL) != "" {
		args = append(args, "--base-url", strings.TrimSpace(req.BaseURL))
	}
	if strings.TrimSpace(req.Theme) != "" {
		args = append(args, "--theme", strings.TrimSpace(req.Theme))
	}
	if strings.TrimSpace(req.RendererVersion) != "" {
		args = append(args, "--renderer-version", strings.TrimSpace(req.RendererVersion))
	}
	return a.run(ctx, args...)
}

func (a RendererAdapter) run(ctx context.Context, args ...string) (RendererCallResult, error) {
	exe := strings.TrimSpace(a.Executable)
	cmdArgs := args
	if exe == "" {
		exe = "bun"
		cmdArgs = append([]string{"run", "render:static", "--"}, args...)
	}
	callID := "renderer-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if a.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.Timeout)
		defer cancel()
	}
	started := time.Now()
	cmd := exec.CommandContext(ctx, exe, cmdArgs...)
	if strings.TrimSpace(a.PackageDir) != "" {
		cmd.Dir = a.PackageDir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(started).Milliseconds()
	if duration == 0 {
		duration = 1
	}
	result := RendererCallResult{CallID: callID, Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(redactRendererStderr(stderr.String())), DurationMS: duration}
	if ctx.Err() != nil {
		return result, fmt.Errorf("renderer command timed out: %w", ctx.Err())
	}
	if err != nil {
		return result, fmt.Errorf("renderer command failed: %w", err)
	}
	return result, nil
}

func redactRendererStderr(value string) string {
	return hugoAbsolutePathPattern.ReplaceAllString(redaction.Cloud(value), "[REDACTED_PATH]")
}
