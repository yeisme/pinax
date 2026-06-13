package e2e

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var sharedBinDir string

func TestMain(m *testing.M) {
	// 1. Create a temporary directory for compiling all executables once
	tmpDir, err := os.MkdirTemp("", "pinax-e2e-bin-*")
	if err != nil {
		log.Fatalf("failed to create temp bin dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	sharedBinDir = tmpDir

	// 2. Locate the repository root
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		log.Fatalf("failed to locate repo root: %v", err)
	}

	// 3. Compile pinax CLI
	pinaxBin := filepath.Join(sharedBinDir, "pinax")
	cmd := exec.Command("go", "build", "-trimpath", "-o", pinaxBin, "./cmd/pinax")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("failed to compile pinax CLI: %v\n%s", err, string(out))
	}

	// 4. Compile fake-lark-cli
	larkBin := filepath.Join(sharedBinDir, "lark-cli")
	cmdLark := exec.Command("go", "build", "-trimpath", "-o", larkBin, "./tests/e2e/fakes/fake-lark-cli")
	cmdLark.Dir = repoRoot
	if out, err := cmdLark.CombinedOutput(); err != nil {
		log.Fatalf("failed to compile fake-lark-cli: %v\n%s", err, string(out))
	}

	// 5. Compile fake-ntn
	ntnBin := filepath.Join(sharedBinDir, "ntn")
	cmdNtn := exec.Command("go", "build", "-trimpath", "-o", ntnBin, "./tests/e2e/fakes/fake-ntn")
	cmdNtn.Dir = repoRoot
	if out, err := cmdNtn.CombinedOutput(); err != nil {
		log.Fatalf("failed to compile fake-ntn: %v\n%s", err, string(out))
	}

	// 6. Run all tests in the package
	code := m.Run()

	os.Exit(code)
}

func runE2ETestScript(t *testing.T, dir string, extraSetup func(env *testscript.Env) error) {
	testscript.Run(t, testscript.Params{
		Dir: dir,
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"validate-json-envelope": cmdValidateJSONEnvelope,
			"validate-agent-format":  cmdValidateAgentFormat,
			"validate-clean-stdout":  cmdValidateCleanStdout,
		},
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			if extraSetup != nil {
				return extraSetup(env)
			}
			return nil
		},
	})
}

func cmdValidateJSONEnvelope(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 1 {
		ts.Fatalf("usage: validate-json-envelope <file>")
	}
	content := ts.ReadFile(args[0])
	content = strings.TrimSpace(content)
	if content == "" {
		if neg {
			return
		}
		ts.Fatalf("JSON envelope is empty")
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		if neg {
			return
		}
		ts.Fatalf("invalid JSON: %v\nContent:\n%s", err, content)
	}

	checkField := func(name string) string {
		v, ok := envelope[name].(string)
		if !ok || v == "" {
			if neg {
				return ""
			}
			ts.Fatalf("missing or invalid %q in JSON envelope: %s", name, content)
		}
		return v
	}

	checkField("spec_version")
	checkField("mode")
	checkField("command")
	status := checkField("status")

	if status != "success" && status != "failed" && status != "partial" {
		if neg {
			return
		}
		ts.Fatalf("invalid status %q in JSON envelope: %s", status, content)
	}

	if neg {
		ts.Fatalf("envelope is valid, but expected invalid")
	}
}

func cmdValidateAgentFormat(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 1 {
		ts.Fatalf("usage: validate-agent-format <file>")
	}
	content := ts.ReadFile(args[0])
	lines := strings.Split(content, "\n")
	hasSpecVer := false
	hasModeAgent := false
	hasCommand := false
	hasStatus := false

	var errs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			errs = append(errs, "invalid agent output line (missing '='): "+line)
			continue
		}
		key := parts[0]
		val := parts[1]

		if key == "" {
			errs = append(errs, "empty key in agent line: "+line)
			continue
		}

		if strings.Contains(val, " ") {
			isDoubleQuoted := strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")
			isSingleQuoted := strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")
			if !isDoubleQuoted && !isSingleQuoted {
				errs = append(errs, "value with spaces must be quoted: "+line)
			}
		}

		switch key {
		case "spec_version":
			hasSpecVer = true
		case "mode":
			if val == "agent" {
				hasModeAgent = true
			}
		case "command":
			hasCommand = true
		case "status":
			hasStatus = true
		}
	}

	if !hasSpecVer {
		errs = append(errs, "missing required key 'spec_version'")
	}
	if !hasModeAgent {
		errs = append(errs, "missing or invalid 'mode=agent'")
	}
	if !hasCommand {
		errs = append(errs, "missing required key 'command'")
	}
	if !hasStatus {
		errs = append(errs, "missing required key 'status'")
	}

	if len(errs) > 0 {
		if neg {
			return
		}
		ts.Fatalf("agent format validation failed:\n%s", strings.Join(errs, "\n"))
	}

	if neg {
		ts.Fatalf("agent format is valid, but expected invalid")
	}
}

func cmdValidateCleanStdout(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 1 {
		ts.Fatalf("usage: validate-clean-stdout <file>")
	}
	content := ts.ReadFile(args[0])
	hasANSI := strings.Contains(content, "\x1b")
	if hasANSI {
		if neg {
			return
		}
		ts.Fatalf("file contains ANSI escape sequences: %q", content)
	}
	if neg {
		ts.Fatalf("file does not contain ANSI escape sequences, but expected to contain them")
	}
}
