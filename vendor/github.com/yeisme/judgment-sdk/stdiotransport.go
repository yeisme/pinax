package judgment

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// StdioTransportOptions configures the optional local stdio transport.
type StdioTransportOptions struct {
	// Argv explicitly names the executable and its arguments. There is no
	// shell interpolation and no implicit discovery: callers must configure
	// the executable path deliberately.
	Argv []string
	// MaxLineBytes bounds the single protocol frame (default 4 MiB).
	MaxLineBytes int64
	// Dir optionally sets the child working directory.
	Dir string
	// Env optionally overrides the child environment entirely.
	Env []string
}

const defaultMaxLine = 4 << 20

// StdioTransport runs a local adapter as a child process with a single
// JSON request/response exchange: one envelope line to the child's stdin,
// one envelope line from its stdout, with a version handshake. stdout
// carries protocol only; stderr is captured but never surfaced raw — only a
// redacted digest reference. Production integrations use the typed HTTP
// transport; stdio is for local, explicitly configured executables.
type StdioTransport struct {
	options StdioTransportOptions
}

// NewStdioTransport validates the explicit argv configuration.
func NewStdioTransport(opts StdioTransportOptions) (*StdioTransport, error) {
	if len(opts.Argv) == 0 || strings.TrimSpace(opts.Argv[0]) == "" {
		return nil, errors.New("judgment: stdio transport requires an explicit argv")
	}
	max := opts.MaxLineBytes
	if max <= 0 {
		max = defaultMaxLine
	}
	opts.MaxLineBytes = max
	return &StdioTransport{options: opts}, nil
}

type stdioEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
}

type stdioResponse struct {
	SchemaVersion string          `json:"schema_version"`
	OK            bool            `json:"ok"`
	Payload       json.RawMessage `json:"payload"`
}

// Capabilities performs one stdio exchange for capability discovery.
func (t *StdioTransport) Capabilities(ctx context.Context) (*Capabilities, error) {
	payload, err := t.exchange(ctx, "capabilities", []byte("{}"))
	if err != nil {
		return nil, err
	}
	caps, verr := ParseCapabilities(payload)
	if verr != nil {
		return nil, NewError(CodeInvalidResponse, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", verr.Reason)
	}
	return caps, nil
}

// Evaluate performs exactly one stdio exchange for an evaluate request.
func (t *StdioTransport) Evaluate(ctx context.Context, jr *Request) (*Result, error) {
	wire, err := jr.WireJSON()
	if err != nil {
		return nil, NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "request_wire_unavailable")
	}
	payload, xerr := t.exchange(ctx, "evaluate", wire)
	if xerr != nil {
		return nil, xerr
	}
	result, verr := ParseResult(payload, jr, DefaultPrecisionPolicy())
	if verr != nil {
		return nil, verr.AsResponseError()
	}
	return result, nil
}

func (t *StdioTransport) exchange(ctx context.Context, kind string, payload []byte) ([]byte, error) {
	frame, err := json.Marshal(stdioEnvelope{SchemaVersion: "1.0", Kind: kind, Payload: payload})
	if err != nil {
		return nil, NewError(CodeInvalidRequest, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "frame_build_failed")
	}
	cmd := exec.CommandContext(ctx, t.options.Argv[0], t.options.Argv[1:]...)
	if t.options.Dir != "" {
		cmd.Dir = t.options.Dir
	}
	if t.options.Env != nil {
		cmd.Env = t.options.Env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, NewError(CodeUnavailable, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "stdin_pipe_failed")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, NewError(CodeUnavailable, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "stdout_pipe_failed")
	}
	stderrBuf := &boundedBuffer{limit: 1 << 16}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return nil, NewError(CodeUnavailable, SubmissionNotSubmitted, RetrySafeBeforeSubmit, "", "child_start_failed")
	}
	writeErr := make(chan error, 1)
	go func() {
		_, werr := stdin.Write(append(frame, '\n'))
		if werr == nil {
			werr = stdin.Close()
		} else {
			stdin.Close()
		}
		writeErr <- werr
	}()
	reader := bufio.NewReaderSize(io.LimitReader(stdout, t.options.MaxLineBytes+1), 64*1024)
	line, readErr := reader.ReadBytes('\n')
	line = bytes.TrimRight(line, "\r\n")
	if int64(len(line)) > t.options.MaxLineBytes {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, "", "stdio_frame_oversize")
	}
	// Drain remaining stdout so the child can finish; protocol requires a
	// single frame, extra output is ignored without being surfaced.
	_, _ = io.Copy(io.Discard, reader)
	werr := <-writeErr
	waitErr := cmd.Wait()
	if readErr != nil && len(line) == 0 {
		return nil, t.childFailure(werr, waitErr, stderrBuf, "no_protocol_frame")
	}
	var resp stdioResponse
	if jerr := json.Unmarshal(line, &resp); jerr != nil {
		return nil, t.childFailure(werr, waitErr, stderrBuf, "frame_not_json")
	}
	// Version handshake: the child must speak the same major contract.
	major, _, _ := strings.Cut(resp.SchemaVersion, ".")
	if major != "1" || resp.SchemaVersion == "" {
		return nil, NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, "", "stdio_version_mismatch")
	}
	if !resp.OK {
		var envelope struct {
			Error *struct {
				Code            string `json:"code"`
				SubmissionState string `json:"submission_state"`
				RetryClass      string `json:"retry_class"`
				DiagnosticRef   string `json:"diagnostic_ref"`
			} `json:"error"`
		}
		if jerr := json.Unmarshal(resp.Payload, &envelope); jerr == nil && envelope.Error != nil &&
			validCode(envelope.Error.Code) && validSubmissionState(envelope.Error.SubmissionState) &&
			validRetryClass(envelope.Error.RetryClass) {
			return nil, NewError(envelope.Error.Code, envelope.Error.SubmissionState, envelope.Error.RetryClass,
				envelope.Error.DiagnosticRef, "stdio_child_error")
		}
		return nil, NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, "", "stdio_error_envelope_invalid")
	}
	return resp.Payload, nil
}

func (t *StdioTransport) childFailure(writeErr, waitErr error, stderr *boundedBuffer, detail string) *Error {
	// stderr is never surfaced; only a digest reference.
	sum := sha256.Sum256(stderr.bytes())
	ref := "stdio-stderr:sha256:" + hex.EncodeToString(sum[:6])
	_ = writeErr
	if waitErr != nil {
		return NewError(CodeUnavailable, SubmissionUnknown, RetryReconcileFirst, ref, detail)
	}
	return NewError(CodeInvalidResponse, SubmissionUnknown, RetryReconcileFirst, ref, detail)
}

// boundedBuffer collects at most limit bytes of child stderr for redacted
// diagnostics; content is never returned to callers.
type boundedBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := b.limit - len(b.buf)
	if room > 0 {
		if len(p) < room {
			room = len(p)
		}
		b.buf = append(b.buf, p[:room]...)
	}
	return len(p), nil
}

func (b *boundedBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}
