package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestSummaryColorModes(t *testing.T) {
	projection := domain.NewProjection("test.summary", "颜色输出测试。")
	projection.Facts["notes"] = "2"

	t.Setenv("NO_COLOR", "")
	t.Setenv("PINAX_COLOR", "always")
	var colored bytes.Buffer
	if err := Render(&colored, ModeSummary, projection); err != nil {
		t.Fatalf("render colored summary: %v", err)
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatalf("summary with PINAX_COLOR=always missing ANSI:\n%s", colored.String())
	}
	for _, old := range []string{"\x1b[38;5;63m", "\x1b[38;5;141m", "\x1b[1;38;5;42m"} {
		if strings.Contains(colored.String(), old) {
			t.Fatalf("summary still uses old high-saturation palette %q:\n%s", old, colored.String())
		}
	}
	for _, want := range []string{"\x1b[38;5;240m", "\x1b[1;38;5;250m", "\x1b[1;38;5;34msuccess"} {
		if !strings.Contains(colored.String(), want) {
			t.Fatalf("summary missing refined palette token %q:\n%s", want, colored.String())
		}
	}

	t.Setenv("PINAX_COLOR", "never")
	var plain bytes.Buffer
	if err := Render(&plain, ModeSummary, projection); err != nil {
		t.Fatalf("render plain summary: %v", err)
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("summary with PINAX_COLOR=never contains ANSI:\n%s", plain.String())
	}
}

func TestMachineOutputsNeverUseANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("PINAX_COLOR", "always")
	projection := domain.NewProjection("test.machine", "颜色输出测试。")
	projection.Facts["notes"] = "2"

	for _, mode := range []Mode{ModeJSON, ModeAgent, ModeEvents} {
		var out bytes.Buffer
		if err := Render(&out, mode, projection); err != nil {
			t.Fatalf("render %s: %v", mode, err)
		}
		if strings.Contains(out.String(), "\x1b[") {
			t.Fatalf("%s output contains ANSI:\n%s", mode, out.String())
		}
	}
}
