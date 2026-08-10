package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCandidateWritesBoundedCandidateMatrix(t *testing.T) {
	setTestPlatform(t, "darwin", "arm64", "14.6.1")
	bin := writeFakePinax(t, fakePinaxOptions{})
	parent := t.TempDir()
	opts := candidateOptions{
		RunID:             "candidate-success",
		ParentDir:         parent,
		PinaxBinary:       bin,
		ReleaseProvenance: "v0.1.12-rc.1@sha256:" + strings.Repeat("a", 64),
		InstallChannel:    "archive",
	}

	if err := runCandidate(opts); err != nil {
		t.Fatalf("runCandidate: %v", err)
	}
	matrix := readMatrix(t, parent, opts.RunID)
	if matrix.Status != "candidate" || matrix.Platform.GOOS != "darwin" || matrix.Platform.GOARCH != "arm64" || matrix.Platform.MacOSVersion != "14.6.1" {
		t.Fatalf("matrix = %#v", matrix)
	}
	if matrix.InstallChannel != "archive" || matrix.ReleaseProvenance != opts.ReleaseProvenance || matrix.StageStatus("bootstrap_contract") != "passed" || matrix.StageStatus("outbound_contract") != "passed" || matrix.StageStatus("recovery_contract") != "passed" {
		t.Fatalf("matrix stages = %#v", matrix)
	}
	for _, stage := range []string{"bootstrap_pull", "outbound_round_trip", "recovery_matrix"} {
		if matrix.StageStatus(stage) != "not_run" {
			t.Fatalf("runtime stage %q = %q, want not_run", stage, matrix.StageStatus(stage))
		}
	}
	payload, err := os.ReadFile(matrixPath(parent, opts.RunID))
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	if strings.Contains(string(payload), bin) || strings.Contains(string(payload), "/") {
		t.Fatalf("matrix leaks a local binary path: %s", payload)
	}
}

func TestRunCandidateRejectsNonDarwinAndWritesFailureMatrix(t *testing.T) {
	setTestPlatform(t, "linux", "amd64", "")
	parent := t.TempDir()
	opts := candidateOptions{
		RunID:             "not-darwin",
		ParentDir:         parent,
		PinaxBinary:       writeFakePinax(t, fakePinaxOptions{}),
		ReleaseProvenance: "v0.1.12-rc.1",
		InstallChannel:    "archive",
	}

	err := runCandidate(opts)
	if errorCode(err) != "macos_platform_required" {
		t.Fatalf("runCandidate error = %v", err)
	}
	matrix := readMatrix(t, parent, opts.RunID)
	if matrix.Status != "unverified" || matrix.FailureCode != "macos_platform_required" {
		t.Fatalf("failure matrix = %#v", matrix)
	}
}

func TestRunCandidateRejectsMissingReleaseProvenance(t *testing.T) {
	setTestPlatform(t, "darwin", "arm64", "14.6.1")
	parent := t.TempDir()
	opts := candidateOptions{
		RunID:          "missing-provenance",
		ParentDir:      parent,
		PinaxBinary:    writeFakePinax(t, fakePinaxOptions{}),
		InstallChannel: "archive",
	}

	err := runCandidate(opts)
	if errorCode(err) != "macos_release_provenance_invalid" {
		t.Fatalf("runCandidate error = %v", err)
	}
	matrix := readMatrix(t, parent, opts.RunID)
	if matrix.Status != "unverified" || matrix.FailureCode != "macos_release_provenance_invalid" || matrix.StageStatus("binary_provenance") != "failed" {
		t.Fatalf("failure matrix = %#v", matrix)
	}
}

func TestRunCandidateRejectsMissingRequiredContractStage(t *testing.T) {
	setTestPlatform(t, "darwin", "amd64", "13.7.2")
	parent := t.TempDir()
	opts := candidateOptions{
		RunID:             "missing-stage",
		ParentDir:         parent,
		PinaxBinary:       writeFakePinax(t, fakePinaxOptions{MissingPushUnlock: true}),
		ReleaseProvenance: "v0.1.12-rc.1",
		InstallChannel:    "homebrew",
	}

	err := runCandidate(opts)
	if errorCode(err) != "macos_support_stage_missing" {
		t.Fatalf("runCandidate error = %v", err)
	}
	matrix := readMatrix(t, parent, opts.RunID)
	if matrix.Status != "unverified" || matrix.FailureCode != "macos_support_stage_missing" || matrix.StageStatus("outbound_contract") != "failed" {
		t.Fatalf("failure matrix = %#v", matrix)
	}
}

func TestRunCandidateRejectsUnsafeVersionWithoutLeakingIt(t *testing.T) {
	setTestPlatform(t, "darwin", "arm64", "15.0")
	parent := t.TempDir()
	const unsafeVersion = "secret-token"
	opts := candidateOptions{
		RunID:             "unsafe-version",
		ParentDir:         parent,
		PinaxBinary:       writeFakePinax(t, fakePinaxOptions{Version: unsafeVersion}),
		ReleaseProvenance: "v0.1.12-rc.1",
		InstallChannel:    "archive",
	}

	err := runCandidate(opts)
	if errorCode(err) != "macos_release_version_invalid" {
		t.Fatalf("runCandidate error = %v", err)
	}
	payload, readErr := os.ReadFile(matrixPath(parent, opts.RunID))
	if readErr != nil {
		t.Fatalf("read failure matrix: %v", readErr)
	}
	if strings.Contains(string(payload), unsafeVersion) {
		t.Fatalf("failure matrix leaks unsafe version: %s", payload)
	}
}

func TestRunCandidateNeverPersistsHelpBody(t *testing.T) {
	setTestPlatform(t, "darwin", "arm64", "14.6.1")
	parent := t.TempDir()
	const unsafeHelpBody = "secret=must-not-be-recorded"
	opts := candidateOptions{
		RunID:             "bounded-help-output",
		ParentDir:         parent,
		PinaxBinary:       writeFakePinax(t, fakePinaxOptions{PushHelpSuffix: " " + unsafeHelpBody}),
		ReleaseProvenance: "v0.1.12-rc.1",
		InstallChannel:    "archive",
	}

	if err := runCandidate(opts); err != nil {
		t.Fatalf("runCandidate: %v", err)
	}
	payload, err := os.ReadFile(matrixPath(parent, opts.RunID))
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	if strings.Contains(string(payload), unsafeHelpBody) {
		t.Fatalf("matrix leaks help output: %s", payload)
	}
}

type fakePinaxOptions struct {
	MissingPushUnlock bool
	PushHelpSuffix    string
	Version           string
}

func writeFakePinax(t *testing.T, opts fakePinaxOptions) string {
	t.Helper()
	version := opts.Version
	if version == "" {
		version = "0.1.12-rc.1"
	}
	pushHelp := "--unlock keychain --dry-run --yes"
	if opts.MissingPushUnlock {
		pushHelp = "--dry-run --yes"
	}
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  \"version --json\") printf '%s\\n' '{\"spec_version\":\"1.0\",\"mode\":\"json\",\"command\":\"system.version\",\"status\":\"success\",\"facts\":{\"version\":\"" + version + "\"}}' ;;\n" +
		"  \"sync repo bootstrap --help\") printf '%s\\n' '--unlock prompt --remember-keychain --pull' ;;\n" +
		"  \"sync repo doctor --help\") printf '%s\\n' 'Diagnose repository sync configuration' ;;\n" +
		"  \"sync diff --help\") printf '%s\\n' '--unlock keychain' ;;\n" +
		"  \"sync push --help\") printf '%s\\n' '" + pushHelp + opts.PushHelpSuffix + "' ;;\n" +
		"  \"sync pull --help\") printf '%s\\n' '--unlock keychain --yes' ;;\n" +
		"  *) printf '%s\\n' 'unexpected arguments' >&2; exit 64 ;;\n" +
		"esac\n"
	path := filepath.Join(t.TempDir(), "pinax")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pinax: %v", err)
	}
	return path
}

func setTestPlatform(t *testing.T, goos, goarch, macOSVersion string) {
	t.Helper()
	originalPlatform := detectPlatform
	originalMacOSVersion := detectMacOSVersion
	detectPlatform = func() platformTuple { return platformTuple{GOOS: goos, GOARCH: goarch} }
	detectMacOSVersion = func() (string, error) { return macOSVersion, nil }
	t.Cleanup(func() {
		detectPlatform = originalPlatform
		detectMacOSVersion = originalMacOSVersion
	})
}

func readMatrix(t *testing.T, parent, runID string) supportMatrix {
	t.Helper()
	payload, err := os.ReadFile(matrixPath(parent, runID))
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	var matrix supportMatrix
	if err := json.Unmarshal(payload, &matrix); err != nil {
		t.Fatalf("parse matrix: %v\n%s", err, payload)
	}
	return matrix
}
