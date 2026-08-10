// Package main validates a macOS Pinax release candidate without touching a
// vault or a remote repository. It writes a bounded platform-support matrix
// beneath the repository-standard integration-evidence run directory.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/testkit/evidence"
)

const (
	supportMatrixSchemaVersion = "pinax.macos_support_matrix.v1"
	defaultEvidenceParent      = "temp/integration-test-runs"
	commandTimeout             = 15 * time.Second
)

var (
	runIDPattern            = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	macOSVersionPattern     = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}(?:\.[0-9]{1,2})?$`)
	platformValuePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
	candidateVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?(?:\+[0-9A-Za-z][0-9A-Za-z.-]*)?$`)
	provenancePattern       = regexp.MustCompile(`^(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?(?:\+[0-9A-Za-z][0-9A-Za-z.-]*)?)(?:@sha256:[a-f0-9]{64})?$`)
	detectPlatform          = func() platformTuple { return platformTuple{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH} }
	detectMacOSVersion      = macOSProductVersion
	runCandidateCommand     = runCommand
)

type candidateOptions struct {
	RunID             string
	ParentDir         string
	PinaxBinary       string
	ReleaseProvenance string
	InstallChannel    string
}

type candidateInput struct {
	PinaxBinary       string `json:"pinax_binary"`
	ReleaseProvenance string `json:"release_provenance"`
	InstallChannel    string `json:"install_channel"`
}

type platformTuple struct {
	GOOS   string
	GOARCH string
}

type supportPlatform struct {
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	MacOSVersion string `json:"macos_version,omitempty"`
}

type supportStage struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// supportMatrix is intentionally small and contains only safe identifiers.
// It must never contain a local binary path, command output, sync receipt, or
// credential material.
type supportMatrix struct {
	SchemaVersion     string          `json:"schema_version"`
	Status            string          `json:"status"`
	Platform          supportPlatform `json:"platform"`
	InstallChannel    string          `json:"install_channel,omitempty"`
	ReleaseProvenance string          `json:"release_provenance,omitempty"`
	CandidateVersion  string          `json:"candidate_version,omitempty"`
	FailureCode       string          `json:"failure_code,omitempty"`
	Stages            []supportStage  `json:"stages"`
}

func (m supportMatrix) StageStatus(id string) string {
	for _, stage := range m.Stages {
		if stage.ID == id {
			return stage.Status
		}
	}
	return ""
}

func (m *supportMatrix) setStageStatus(id, status string) {
	for index := range m.Stages {
		if m.Stages[index].ID == id {
			m.Stages[index].Status = status
			return
		}
	}
	m.Stages = append(m.Stages, supportStage{ID: id, Status: status})
}

type candidateError struct {
	code string
}

func (e *candidateError) Error() string {
	return e.code
}

func errorCode(err error) string {
	var candidateErr *candidateError
	if errors.As(err, &candidateErr) {
		return candidateErr.code
	}
	return ""
}

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

func runMain(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("macosreleaseevidence", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	child := flags.Bool("child", false, "run the candidate probe under the evidence wrapper")
	runID := flags.String("run-id", "", "evidence run id")
	pinaxBinary := flags.String("pinax", "", "path to the release-candidate pinax binary")
	provenance := flags.String("provenance", "", "release version with an optional sha256 digest")
	installChannel := flags.String("install-channel", "", "archive, homebrew, or go-install")
	optionsFile := flags.String("options-file", "", "internal child options file")
	if err := flags.Parse(args); err != nil {
		_, _ = fmt.Fprintln(stderr, "invalid macOS release evidence arguments")
		return 2
	}

	if *child {
		return runChild(*runID, *optionsFile, stdout, stderr)
	}

	runIDValue := strings.TrimSpace(*runID)
	if runIDValue == "" {
		runIDValue = time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	}
	if !safeRunID(runIDValue) {
		_, _ = fmt.Fprintln(stderr, "invalid macOS release evidence run id")
		return 2
	}

	optionsPath, err := writeCandidateInput(candidateInput{
		PinaxBinary:       *pinaxBinary,
		ReleaseProvenance: *provenance,
		InstallChannel:    *installChannel,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "unable to prepare macOS release evidence")
		return 1
	}
	defer func() { _ = os.Remove(optionsPath) }()

	command := []string{
		"go", "run", "./internal/testkit/macosreleaseevidence",
		"--child", "--run-id", runIDValue, "--options-file", optionsPath,
	}
	result, evidenceErr := evidence.Run(evidence.Config{
		RunID:             runIDValue,
		ParentDir:         defaultEvidenceParent,
		Command:           command,
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             "component",
		ExtraChecks: map[string]any{
			"macos_release_candidate": true,
			"remote_sync_write":       false,
			"runtime_sync_stages":     "not_run",
		},
	})
	if evidenceErr != nil {
		_, _ = fmt.Fprintln(stderr, "macOS release evidence infrastructure failure")
		if result.ExitCode == 0 {
			return 1
		}
	}
	if result.ExitCode == 0 {
		_, _ = fmt.Fprintln(stdout, "macOS release candidate evidence completed; remote sync stages remain not_run")
	}
	return result.ExitCode
}

func runChild(runID, optionsPath string, stdout, stderr io.Writer) int {
	input, err := readCandidateInput(optionsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "macOS release candidate validation failed: macos_candidate_options_invalid")
		return 1
	}
	opts := candidateOptions{
		RunID:             runID,
		ParentDir:         defaultEvidenceParent,
		PinaxBinary:       input.PinaxBinary,
		ReleaseProvenance: input.ReleaseProvenance,
		InstallChannel:    input.InstallChannel,
	}
	if err := runCandidate(opts); err != nil {
		_, _ = fmt.Fprintln(stderr, "macOS release candidate validation failed:", errorCode(err))
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "macOS release candidate contract probe passed")
	return 0
}

func runCandidate(opts candidateOptions) error {
	if !safeRunID(opts.RunID) {
		return &candidateError{code: "macos_run_id_invalid"}
	}
	if strings.TrimSpace(opts.ParentDir) == "" {
		opts.ParentDir = defaultEvidenceParent
	}

	platform := detectPlatform()
	matrix := newSupportMatrix(platform)
	fail := func(stage, code string) error {
		if stage != "" {
			matrix.setStageStatus(stage, "failed")
		}
		matrix.Status = "unverified"
		matrix.FailureCode = code
		if err := writeMatrix(opts.ParentDir, opts.RunID, matrix); err != nil {
			return &candidateError{code: "macos_matrix_write_failed"}
		}
		return &candidateError{code: code}
	}

	if platform.GOOS != "darwin" {
		return fail("", "macos_platform_required")
	}
	if platform.GOARCH != "arm64" && platform.GOARCH != "amd64" {
		return fail("", "macos_arch_unsupported")
	}
	macOSVersion, err := detectMacOSVersion()
	if err != nil || !macOSVersionPattern.MatchString(macOSVersion) {
		return fail("", "macos_version_unavailable")
	}
	matrix.Platform.MacOSVersion = macOSVersion

	provenance := strings.TrimSpace(opts.ReleaseProvenance)
	provenanceMatch := provenancePattern.FindStringSubmatch(provenance)
	if provenanceMatch == nil {
		return fail("binary_provenance", "macos_release_provenance_invalid")
	}
	matrix.ReleaseProvenance = provenance
	channel := strings.TrimSpace(opts.InstallChannel)
	if !validInstallChannel(channel) {
		return fail("binary_provenance", "macos_install_channel_invalid")
	}
	matrix.InstallChannel = channel

	binary := strings.TrimSpace(opts.PinaxBinary)
	if err := validateCandidateBinary(binary); err != nil {
		return fail("binary_provenance", errorCode(err))
	}
	version, err := candidateBinaryVersion(binary)
	if err != nil || !candidateVersionPattern.MatchString(version) || canonicalVersion(version) != canonicalVersion(provenanceMatch[1]) {
		return fail("binary_provenance", "macos_release_version_invalid")
	}
	matrix.CandidateVersion = version
	matrix.setStageStatus("binary_provenance", "passed")

	if err := requireHelpFlags(binary, []string{"sync", "repo", "bootstrap", "--help"}, "--unlock", "--remember-keychain", "--pull"); err != nil {
		return fail("bootstrap_contract", errorCode(err))
	}
	matrix.setStageStatus("bootstrap_contract", "passed")

	if err := requireHelpFlags(binary, []string{"sync", "pull", "--help"}, "--unlock", "--yes"); err != nil {
		return fail("inbound_contract", errorCode(err))
	}
	matrix.setStageStatus("inbound_contract", "passed")

	if err := requireHelpFlags(binary, []string{"sync", "diff", "--help"}, "--unlock"); err != nil {
		return fail("outbound_contract", errorCode(err))
	}
	if err := requireHelpFlags(binary, []string{"sync", "push", "--help"}, "--unlock", "--dry-run", "--yes"); err != nil {
		return fail("outbound_contract", errorCode(err))
	}
	matrix.setStageStatus("outbound_contract", "passed")

	if err := requireHelp(binary, []string{"sync", "repo", "doctor", "--help"}); err != nil {
		return fail("recovery_contract", errorCode(err))
	}
	matrix.setStageStatus("recovery_contract", "passed")

	matrix.Status = "candidate"
	matrix.FailureCode = ""
	if err := writeMatrix(opts.ParentDir, opts.RunID, matrix); err != nil {
		return &candidateError{code: "macos_matrix_write_failed"}
	}
	return nil
}

func newSupportMatrix(platform platformTuple) supportMatrix {
	return supportMatrix{
		SchemaVersion: supportMatrixSchemaVersion,
		Status:        "unverified",
		Platform: supportPlatform{
			GOOS:   safePlatformValue(platform.GOOS),
			GOARCH: safePlatformValue(platform.GOARCH),
		},
		Stages: []supportStage{
			{ID: "binary_provenance", Status: "pending"},
			{ID: "bootstrap_contract", Status: "pending"},
			{ID: "inbound_contract", Status: "pending"},
			{ID: "outbound_contract", Status: "pending"},
			{ID: "recovery_contract", Status: "pending"},
			{ID: "bootstrap_pull", Status: "not_run"},
			{ID: "outbound_round_trip", Status: "not_run"},
			{ID: "recovery_matrix", Status: "not_run"},
		},
	}
}

func matrixPath(parentDir, runID string) string {
	return filepath.Join(parentDir, runID, "artifacts", "platform-support.json")
}

func writeMatrix(parentDir, runID string, matrix supportMatrix) error {
	path := matrixPath(parentDir, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}

func validateCandidateBinary(binary string) error {
	if binary == "" {
		return &candidateError{code: "macos_release_binary_missing"}
	}
	info, err := os.Stat(binary)
	if err != nil || !info.Mode().IsRegular() {
		return &candidateError{code: "macos_release_binary_missing"}
	}
	if info.Mode().Perm()&0o111 == 0 {
		return &candidateError{code: "macos_release_binary_not_executable"}
	}
	return nil
}

func candidateBinaryVersion(binary string) (string, error) {
	payload, err := runCandidateCommand(binary, "version", "--json")
	if err != nil {
		return "", &candidateError{code: "macos_release_version_invalid"}
	}
	var envelope struct {
		SpecVersion string `json:"spec_version"`
		Mode        string `json:"mode"`
		Command     string `json:"command"`
		Status      string `json:"status"`
		Facts       struct {
			Version string `json:"version"`
		} `json:"facts"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.SpecVersion != "1.0" || envelope.Mode != "json" || envelope.Command != "system.version" || envelope.Status != "success" {
		return "", &candidateError{code: "macos_release_version_invalid"}
	}
	return strings.TrimSpace(envelope.Facts.Version), nil
}

func requireHelpFlags(binary string, args []string, required ...string) error {
	payload, err := runCandidateCommand(binary, args...)
	if err != nil {
		return &candidateError{code: "macos_support_stage_command_failed"}
	}
	for _, flagName := range required {
		if !strings.Contains(string(payload), flagName) {
			return &candidateError{code: "macos_support_stage_missing"}
		}
	}
	return nil
}

func requireHelp(binary string, args []string) error {
	payload, err := runCandidateCommand(binary, args...)
	if err != nil || len(strings.TrimSpace(string(payload))) == 0 {
		return &candidateError{code: "macos_support_stage_command_failed"}
	}
	return nil
}

func runCommand(binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	return exec.CommandContext(ctx, binary, args...).CombinedOutput()
}

func macOSProductVersion() (string, error) {
	payload, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(payload)), nil
}

func validInstallChannel(channel string) bool {
	switch channel {
	case "archive", "homebrew", "go-install":
		return true
	default:
		return false
	}
}

func safeRunID(runID string) bool {
	return runIDPattern.MatchString(runID)
}

func safePlatformValue(value string) string {
	if platformValuePattern.MatchString(value) {
		return value
	}
	return "unavailable"
}

func canonicalVersion(value string) string {
	return strings.TrimPrefix(value, "v")
}

func writeCandidateInput(input candidateInput) (string, error) {
	file, err := os.CreateTemp("", "pinax-macos-release-options-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	payload, marshalErr := json.Marshal(input)
	if marshalErr == nil {
		_, marshalErr = file.Write(append(payload, '\n'))
	}
	closeErr := file.Close()
	if marshalErr != nil {
		_ = os.Remove(path)
		return "", marshalErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	return path, nil
}

func readCandidateInput(path string) (candidateInput, error) {
	if strings.TrimSpace(path) == "" {
		return candidateInput{}, errors.New("options_file_missing")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return candidateInput{}, err
	}
	var input candidateInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return candidateInput{}, err
	}
	return input, nil
}
