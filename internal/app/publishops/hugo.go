package publishops

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/redaction"
)

type HugoAdapter struct {
	Executable string
	Timeout    time.Duration
}

var hugoAbsolutePathPattern = regexp.MustCompile(`(?i)(/home|/users)/[^\s'"]+|[a-z]:\\[^\s'"]+`)

type HugoCallResult struct {
	CallID     string `json:"call_id"`
	Version    string `json:"version,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

func (a HugoAdapter) Version(ctx context.Context) (HugoCallResult, error) {
	result, err := a.run(ctx, "version")
	result.Version = strings.TrimSpace(result.Version)
	return result, err
}

func (a HugoAdapter) Build(ctx context.Context, source, destination string) (HugoCallResult, error) {
	return a.run(ctx, "--source", source, "--destination", destination)
}

func redactHugoStderr(value string) string {
	return hugoAbsolutePathPattern.ReplaceAllString(redaction.Cloud(value), "[REDACTED_PATH]")
}

func (a HugoAdapter) run(ctx context.Context, args ...string) (HugoCallResult, error) {
	exe := strings.TrimSpace(a.Executable)
	if exe == "" {
		exe = "hugo"
	}
	callID := "hugo-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if a.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.Timeout)
		defer cancel()
	}
	started := time.Now()
	cmd := exec.CommandContext(ctx, exe, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(started).Milliseconds()
	if duration == 0 {
		duration = 1
	}
	result := HugoCallResult{CallID: callID, Version: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(redactHugoStderr(stderr.String())), DurationMS: duration}
	if ctx.Err() != nil {
		return result, fmt.Errorf("hugo command timed out: %w", ctx.Err())
	}
	if err != nil {
		return result, fmt.Errorf("hugo command failed: %w", err)
	}
	return result, nil
}
