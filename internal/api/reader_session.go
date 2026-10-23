package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
	"github.com/yeisme/pinax/pkg/localsession"
)

// Supervised local session (pinax-human-reading-api-v1 P05).
//
// `pinax api serve --session-stdin` boots a readonly loopback reader server
// from a private stdin first frame: the bearer exists only in memory, stdout
// carries redacted ready/error/shutdown events, and closing stdin drains and
// stops the server. No daemon, no service registration, no remote listeners,
// and legacy api serve / MCP flags keep their existing behavior.

// Session event schema versions for the host protocol on stdout.
const (
	LocalSessionReadyEventV1Alpha1 = localsession.EventVersion
	sessionBootstrapMaxFrameBytes  = catalogschema.ReaderSessionBootstrapMaxFrame
	sessionDrainDeadline           = 5 * time.Second
)

// sessionBootstrapDeadline is a var so tests can shorten the first-frame
// wait without changing production defaults.
var sessionBootstrapDeadline time.Duration = 10 * time.Second

// SessionBootstrapFrame is the versioned private stdin first frame
// (pinax.local_session.bootstrap.v1alpha1). Secret fields never reach logs.
type SessionBootstrapFrame = localsession.Bootstrap

// SessionEvent is one redacted stdout protocol message.
type SessionEvent = localsession.Event

// ReadSessionBootstrap reads and validates the private first frame. The
// host keeps stdin open for liveness after the frame, so exactly one
// newline-terminated frame is consumed — never until EOF. The frame is
// capped at 16 KiB; unknown fields, unknown schema versions, unknown
// scopes, mutation requests and malformed secrets are rejected.
func ReadSessionBootstrap(reader io.Reader) (SessionBootstrapFrame, error) {
	// Byte-precise read up to the newline: unlike a buffered reader this
	// never consumes past the frame, so any trailing input remains in the
	// stream and the session drain below can police it.
	limited := io.LimitReader(reader, sessionBootstrapMaxFrameBytes+1)
	var line []byte
	single := make([]byte, 1)
	readErr := error(nil)
	for {
		n, err := limited.Read(single)
		if n > 0 {
			if single[0] == '\n' {
				break
			}
			line = append(line, single[0])
			if len(line) > sessionBootstrapMaxFrameBytes {
				return SessionBootstrapFrame{}, fmt.Errorf("bootstrap frame exceeds %d bytes", sessionBootstrapMaxFrameBytes)
			}
			continue
		}
		readErr = err
		break
	}
	if readErr != nil && readErr != io.EOF {
		return SessionBootstrapFrame{}, fmt.Errorf("the bootstrap frame could not be read")
	}
	frameBytes := strings.TrimRight(string(line), "\r\n \t")
	if frameBytes == "" {
		return SessionBootstrapFrame{}, fmt.Errorf("bootstrap frame is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(frameBytes))
	decoder.DisallowUnknownFields()
	var frame SessionBootstrapFrame
	if err := decoder.Decode(&frame); err != nil {
		return SessionBootstrapFrame{}, fmt.Errorf("bootstrap frame is not valid closed JSON")
	}
	// The bootstrap channel carries EXACTLY ONE versioned frame: trailing
	// content (garbage, a second JSON object, or stray closing delimiters)
	// makes the first frame illegal. More() is not enough — it reports
	// false at `}` / `]` — so true end-of-input is proven by a Token()
	// round-trip that returns io.EOF.
	if _, tokenErr := decoder.Token(); tokenErr != io.EOF {
		return SessionBootstrapFrame{}, fmt.Errorf("bootstrap frame has trailing content after the single JSON object")
	}
	if frame.SchemaVersion != catalogschema.LocalSessionBootstrapV1Alpha1 {
		return SessionBootstrapFrame{}, fmt.Errorf("unsupported bootstrap schema version")
	}
	if len(frame.Scopes) == 0 {
		return SessionBootstrapFrame{}, fmt.Errorf("at least one scope is required")
	}
	allowed := map[string]bool{
		app.ReaderScopeMetadata: true, app.ReaderScopeBody: true,
		app.ReaderScopeRelations: true, app.ReaderScopeAssets: true,
		// pinax-human-reading-write-v1: the writer allowlist extension slot.
		// Writer scopes are owner-granted only; a writer session must declare
		// readonly=false, a reader-only session stays readonly.
		app.WriterScopeCapture: true, app.WriterScopeEditor: true, app.WriterScopeOrganizer: true,
	}
	seen := map[string]bool{}
	writerScope := false
	for _, scope := range frame.Scopes {
		if !allowed[scope] {
			return SessionBootstrapFrame{}, fmt.Errorf("unknown scope in the allowlist request")
		}
		if seen[scope] {
			return SessionBootstrapFrame{}, fmt.Errorf("duplicate scope in the allowlist request")
		}
		seen[scope] = true
		if app.IsWriterScope(scope) {
			writerScope = true
		}
	}
	// readonly ⇔ writer scopes: a writer session must explicitly declare
	// readonly=false, and a reader-only session can never enable mutation.
	if writerScope && frame.Readonly {
		return SessionBootstrapFrame{}, fmt.Errorf("writer sessions must declare readonly=false")
	}
	if !writerScope && !frame.Readonly {
		return SessionBootstrapFrame{}, fmt.Errorf("reader sessions are readonly; bootstrap cannot enable mutation")
	}
	if len(frame.SessionNonce) < 16 || len(frame.SessionNonce) > 128 {
		return SessionBootstrapFrame{}, fmt.Errorf("session_nonce must be 16-128 characters")
	}
	if len(frame.Bearer) < 43 || len(frame.Bearer) > 64 {
		return SessionBootstrapFrame{}, fmt.Errorf("bearer must be a 256-bit secret")
	}
	if strings.TrimSpace(frame.BindingID) == "" {
		return SessionBootstrapFrame{}, fmt.Errorf("binding_id is required")
	}
	if strings.TrimSpace(frame.VaultRoot) == "" {
		return SessionBootstrapFrame{}, fmt.Errorf("vault_root is required")
	}
	return frame, nil
}

// ReadSessionBootstrapWithDeadline applies the 10s first-frame deadline.
func ReadSessionBootstrapWithDeadline(ctx context.Context, reader io.Reader) (SessionBootstrapFrame, error) {
	type result struct {
		frame SessionBootstrapFrame
		err   error
	}
	results := make(chan result, 1)
	go func() {
		frame, err := ReadSessionBootstrap(reader)
		results <- result{frame, err}
	}()
	deadline, cancel := context.WithTimeout(ctx, sessionBootstrapDeadline)
	defer cancel()
	select {
	case outcome := <-results:
		return outcome.frame, outcome.err
	case <-deadline.Done():
		return SessionBootstrapFrame{}, fmt.Errorf("bootstrap frame did not arrive within %s", sessionBootstrapDeadline)
	}
}

// ServeSupervisedSessionOptions configures the supervised reader server.
type ServeSupervisedSessionOptions struct {
	Service       *app.Service
	Frame         SessionBootstrapFrame
	Stdin         io.Reader // liveness channel; closed stdin stops the server
	Stdout        io.Writer // redacted host protocol
	Stderr        io.Writer // human diagnostics; never secrets
	Manifest      ManifestProvider
	RecoveryStdin bool // explicit private host handshake; never enabled by bootstrap fields
}

// ServeSupervisedSession runs the readonly loopback reader surface for one
// bootstrap frame. Only the reader route group is exposed; the legacy API,
// temp tokens and audit writes stay out of this mode entirely. EOF on stdin
// stops new requests and drains for at most five seconds before shutdown.
func ServeSupervisedSession(ctx context.Context, options ServeSupervisedSessionOptions) error {
	vaultRoot := options.Frame.VaultRoot
	absolute, err := filepath.Abs(vaultRoot)
	if err == nil {
		vaultRoot = absolute
	}
	info, statErr := os.Stat(vaultRoot)
	if statErr != nil || !info.IsDir() {
		return sessionStartupFailure(options.Stdout, "session_vault_unavailable", "The selected vault root is not a readable directory")
	}
	// A plain directory is not a vault: the supervised session opens an
	// EXISTING vault and must fail safely at startup (a valid vault that
	// merely lacks an index still boots; the endpoints report that). The
	// marker is the .pinax directory created by vault init — never
	// auto-created here.
	pinaxInfo, pinaxErr := os.Stat(filepath.Join(vaultRoot, ".pinax"))
	if pinaxErr != nil || !pinaxInfo.IsDir() {
		return sessionStartupFailure(options.Stdout, "session_vault_invalid", "The selected vault root is not an initialized pinax vault")
	}

	sessions, err := NewReaderSessionStore()
	if err != nil {
		return sessionStartupFailure(options.Stdout, "session_store_unavailable", "The reader session store could not start")
	}
	// Session identity must be unique per session even when the same
	// binding starts twice within the same second: a random 128-bit
	// component guarantees freshness without leaking bearer or nonce.
	randomID := make([]byte, 16)
	if _, randomErr := rand.Read(randomID); randomErr != nil {
		return sessionStartupFailure(options.Stdout, "session_store_unavailable", "The reader session store could not start")
	}
	sessionID := "rss_" + hex.EncodeToString(randomID)
	if err := sessions.Create(ReaderSession{
		SessionID: sessionID,
		BindingID: options.Frame.BindingID,
		Nonce:     options.Frame.SessionNonce,
		Scopes:    append([]string(nil), options.Frame.Scopes...),
		CreatedAt: time.Now().UTC(),
	}, options.Frame.Bearer); err != nil {
		return sessionStartupFailure(options.Stdout, "session_conflict", "The reader session could not be registered")
	}

	defer sessions.Revoke(sessionID)
	if options.RecoveryStdin {
		if err := prepareSessionRecovery(ctx, options, sessions, sessionID, vaultRoot); err != nil {
			return sessionStartupFailure(options.Stdout, "session_recovery_invalid", "The private recovery authorization handshake failed")
		}
	}

	// pinax-human-reading-write-v1: a frame carrying writer scopes opens
	// the supervised writer surface alongside the reader routes; writer
	// routes still enforce their own scope, so a reader-only session never
	// gains a writable route.
	exposeGroups := []string{"reader"}
	for _, scope := range options.Frame.Scopes {
		if app.IsWriterScope(scope) {
			exposeGroups = append(exposeGroups, "writer")
			break
		}
	}
	server := NewServerWithOptions(options.Service, vaultRoot, ServerOptions{
		ReaderSessions: sessions,
		ExposeGroups:   exposeGroups,
		Manifest:       options.Manifest,
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return sessionStartupFailure(options.Stdout, "session_listen_failed", "The loopback listener could not start")
	}
	address := listener.Addr().String()

	contractDigest := ""
	if options.Manifest != nil {
		if manifest, manifestErr := options.Manifest(); manifestErr == nil {
			contractDigest = manifest.Facts["digest"]
		}
	}
	readonly := !options.Frame.HasWriterScope()
	if err := writeSessionEvent(options.Stdout, SessionEvent{
		SchemaVersion: LocalSessionReadyEventV1Alpha1, Type: "ready",
		SessionID: sessionID, SessionNonce: options.Frame.SessionNonce,
		BindingID: options.Frame.BindingID, Address: "http://" + address,
		ContractDigest: contractDigest, Readonly: &readonly,
	}); err != nil {
		_ = listener.Close()
		return err
	}

	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	stdinClosed := make(chan struct{})
	closeStdinOnce := sync.OnceFunc(func() { close(stdinClosed) })
	illegalInput := make(chan struct{}, 1)
	go func() {
		// After the bootstrap and optional opted-in recovery grant, stdin
		// may hold only whitespace until EOF. Any further content — garbage
		// or another JSON object — is an illegal bootstrap and fails the
		// session instead of being silently drained.
		scanner := bufio.NewScanner(options.Stdin)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) != "" {
				illegalInput <- struct{}{}
				return
			}
		}
		if scanner.Err() != nil {
			illegalInput <- struct{}{}
			return
		}
		closeStdinOnce()
	}()
	go func() {
		<-ctx.Done()
		closeStdinOnce()
	}()
	// Drained signals that in-flight requests finished (or the drain deadline
	// forced the remaining own connections closed). Shutdown itself blocks
	// until both; the shutdown event is only emitted afterwards.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-stdinClosed
		drain, cancel := context.WithTimeout(context.Background(), sessionDrainDeadline)
		defer cancel()
		if err := httpServer.Shutdown(drain); err != nil {
			// Deadline exceeded: force-close the remaining own connections
			// instead of leaving them dangling behind a reported shutdown.
			_ = httpServer.Close()
		}
	}()
	serveDone := make(chan struct{})
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			_ = writeSessionEvent(options.Stdout, SessionEvent{
				SchemaVersion: LocalSessionReadyEventV1Alpha1, Type: "error",
				Code: "session_serve_failed", Message: "The reader server stopped unexpectedly",
			})
		}
		close(serveDone)
	}()
	select {
	case <-drained:
		// Normal EOF path: requests drained, listener closed, safe to report.
	case <-serveDone:
		// Serve ended on its own (fatal error): drain waits for nothing.
		<-drained
	case <-illegalInput:
		// Illegal post-frame input: connections are force-closed and the
		// session fails with a redacted error event — never a graceful
		// shutdown that would make the illegal bootstrap look accepted.
		_ = httpServer.Close()
		_ = writeSessionEvent(options.Stdout, SessionEvent{
			SchemaVersion: LocalSessionReadyEventV1Alpha1, Type: "error",
			Code: "session_bootstrap_invalid", Message: "The supervised session bootstrap channel received unexpected input after the single frame",
		})
		return fmt.Errorf("session_bootstrap_invalid: unexpected input after the bootstrap frame")
	}
	return writeSessionEvent(options.Stdout, SessionEvent{
		SchemaVersion: LocalSessionReadyEventV1Alpha1, Type: "shutdown",
		SessionID: sessionID, BindingID: options.Frame.BindingID,
	})
}

// sessionStartupFailure emits the redacted machine event for a failed
// session start and returns a non-nil error: a successfully written error
// event is still a failed startup and must exit non-zero.
func sessionStartupFailure(writer io.Writer, code, message string) error {
	_ = writeSessionEvent(writer, SessionEvent{
		SchemaVersion: LocalSessionReadyEventV1Alpha1, Type: "error",
		Code: code, Message: message,
	})
	return fmt.Errorf("%s: %s", code, message)
}

func writeSessionEvent(writer io.Writer, event SessionEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, string(encoded))
	return err
}

// WriteSessionError emits one redacted error event for host consumption.
func WriteSessionError(writer io.Writer, code, message string) error {
	return writeSessionEvent(writer, SessionEvent{
		SchemaVersion: LocalSessionReadyEventV1Alpha1, Type: "error",
		Code: code, Message: message,
	})
}
