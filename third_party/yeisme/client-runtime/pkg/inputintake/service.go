package inputintake

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strings"
	"syscall"
	"time"
)

type Service struct {
	opt    Options
	origin string
}

func New(opt Options) (*Service, error) {
	u, err := url.Parse(opt.BaseURL)
	if !opt.Enabled {
		return nil, ErrDisabled
	}
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || opt.Owner == "" || opt.MaxBytes <= 0 || opt.MaxBytes >= 1<<62 || opt.Repository == nil || opt.Backend == nil || len(opt.MIME) == 0 || len(opt.Purposes) == 0 {
		return nil, ErrInvalid
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.TTL <= 0 {
		opt.TTL = 24 * time.Hour
	}
	opt.BaseURL = strings.TrimRight(opt.BaseURL, "/")
	return &Service{opt: opt, origin: u.Scheme + "://" + u.Host}, nil
}
func digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func token() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func validID(id string) bool {
	if len(id) != 36 || !strings.HasPrefix(id, "inp_") {
		return false
	}
	_, err := hex.DecodeString(id[4:])
	return err == nil
}
func matches(token, hash string) bool {
	return token != "" && hash != "" && subtle.ConstantTimeCompare([]byte(digest(token)), []byte(hash)) == 1
}
func includes(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (s *Service) now() time.Time { return s.opt.Now().UTC() }
func (s *Service) file(f File) error {
	if f.Name == "" || len(f.Name) > 255 || path.Base(f.Name) != f.Name || f.Name == "." || f.Name == ".." || strings.ContainsAny(f.Name, "\\\x00\r\n") || !includes(s.opt.MIME, f.MIME) || f.Size <= 0 || f.Size > s.opt.MaxBytes {
		return ErrInvalid
	}
	if f.SHA256 != "" {
		b, e := hex.DecodeString(f.SHA256)
		if e != nil || len(b) != 32 {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Service) owned(ctx context.Context, who Identity, id string) (Record, error) {
	if s.opt.Authorize != nil {
		if e := s.opt.Authorize(ctx, who); e != nil {
			return Record{}, ErrDenied
		}
	}
	if !validID(id) || who.Actor == "" || who.Project == "" {
		return Record{}, ErrDenied
	}
	r, e := s.opt.Repository.Get(ctx, id)
	if e != nil {
		return r, e
	}
	if r.Actor != who.Actor || r.Project != who.Project {
		return Record{}, ErrDenied
	}
	return r, nil
}
func view(r Record) View {
	v := View{MaxBytes: r.MaxBytes, MIME: strings.Split(r.AllowedMIME, ","), SchemaVersion: Schema, ID: r.ID, Project: r.Project, Purpose: r.Purpose, State: r.State, ExpiresAt: r.ExpiresAt}
	v.DomainState, v.FailureCode = r.DomainState, r.FailureCode
	if r.FailureCode != "" && r.State != "ready" && r.State != "cancelled" && r.State != "expired" {
		v.State, v.ResumeState = "failed", r.State
	}
	if r.Size > 0 {
		v.File = &File{Name: r.FileName, MIME: r.MIME, Size: r.Size, SHA256: r.SHA256}
	}
	if r.Ref != "" {
		v.Receipt = &Receipt{Ref: r.Ref, SHA256: r.SHA256, Size: r.Size, DomainState: r.DomainState}
	}
	return v
}
func (s *Service) save(ctx context.Context, r *Record) error {
	rev := r.Revision
	r.Revision++
	return s.opt.Repository.CAS(ctx, *r, rev)
}
func (s *Service) active(r Record) error {
	if r.State == "cancelled" || r.State == "ready" {
		return ErrConflict
	}
	if !s.now().Before(r.ExpiresAt) {
		return ErrExpired
	}
	return nil
}
func (s *Service) Status(ctx context.Context, who Identity, id string) (View, error) {
	r, e := s.owned(ctx, who, id)
	if e != nil {
		return View{}, e
	}
	if r.State != "ready" && r.State != "cancelled" && !s.now().Before(r.ExpiresAt) {
		r.State = "expired"
	}
	return view(r), nil
}
func (s *Service) Prepare(ctx context.Context, who Identity, p Prepare) (Access, error) {
	if s.opt.Authorize != nil {
		if e := s.opt.Authorize(ctx, who); e != nil {
			return Access{}, ErrDenied
		}
	}
	if who.Actor == "" || who.Project == "" || len(p.IdempotencyKey) == 0 || len(p.IdempotencyKey) > 256 || !includes(s.opt.Purposes, p.Purpose) {
		return Access{}, ErrInvalid
	}
	if p.File != nil {
		if e := s.file(*p.File); e != nil {
			return Access{}, e
		}
	}
	identity, _ := json.Marshal([]string{who.Actor, who.Project, p.IdempotencyKey})
	id := "inp_" + digest(string(identity))[:32]
	raw, _ := json.Marshal(p)
	intent := digest(string(raw))
	if r, e := s.opt.Repository.Get(ctx, id); e == nil {
		if r.Actor != who.Actor || r.Project != who.Project {
			return Access{}, ErrDenied
		}
		if r.IntentDigest != intent {
			return Access{}, ErrConflict
		}
		v, err := s.Status(ctx, who, id)
		return Access{Request: v}, err
	} else if e != ErrDenied {
		return Access{}, e
	}
	now := s.now()
	r := Record{MaxBytes: s.opt.MaxBytes, AllowedMIME: strings.Join(s.opt.MIME, ","), ID: id, Revision: 1, Actor: who.Actor, Project: who.Project, Purpose: p.Purpose, IntentDigest: intent, State: "awaiting_file", CreatedAt: now, ExpiresAt: now.Add(s.opt.TTL)}
	page, e := token()
	if e != nil {
		return Access{}, e
	}
	grant, e := token()
	if e != nil {
		return Access{}, e
	}
	r.PageHash = digest(page)
	r.PageExpires = minTime(now.Add(15*time.Minute), r.ExpiresAt)
	r.TransferHash = digest(grant)
	r.TransferExpires = minTime(now.Add(5*time.Minute), r.ExpiresAt)
	if e = s.opt.Repository.Create(ctx, r); e != nil {
		return Access{}, e
	}
	if p.File != nil {
		if _, e = s.Bind(ctx, who, id, *p.File); e != nil {
			return Access{}, e
		}
		r, e = s.owned(ctx, who, id)
		if e != nil {
			return Access{}, e
		}
	}
	return s.access(r, page, grant), nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (s *Service) access(r Record, page, grant string) Access {
	return Access{Request: view(r), PageURL: s.opt.BaseURL + Prefix + r.ID + "#page=" + page, TransferURL: s.opt.BaseURL + Prefix + r.ID + "#grant=" + grant}
}
func (s *Service) Renew(ctx context.Context, who Identity, id string) (Access, error) {
	r, e := s.owned(ctx, who, id)
	if e != nil {
		return Access{}, e
	}
	if e = s.active(r); e != nil {
		return Access{}, e
	}
	page, e := token()
	if e != nil {
		return Access{}, e
	}
	grant, e := token()
	if e != nil {
		return Access{}, e
	}
	r.PageHash = digest(page)
	r.PageExpires = minTime(s.now().Add(15*time.Minute), r.ExpiresAt)
	r.TransferHash = digest(grant)
	r.TransferExpires = minTime(s.now().Add(5*time.Minute), r.ExpiresAt)
	// Existing browser sessions are not silently revoked by transfer renewal.
	if e = s.save(ctx, &r); e != nil {
		return Access{}, e
	}
	return s.access(r, page, grant), nil
}
func (s *Service) Exchange(ctx context.Context, id, secret string) (string, time.Time, error) {
	r, e := s.opt.Repository.Get(ctx, id)
	if e != nil || !validID(id) {
		return "", time.Time{}, ErrDenied
	}
	if e = s.active(r); e != nil {
		return "", time.Time{}, e
	}
	if !s.now().Before(r.PageExpires) || !matches(secret, r.PageHash) {
		return "", time.Time{}, ErrDenied
	}
	if s.opt.Authorize != nil {
		if e := s.opt.Authorize(ctx, Identity{Actor: r.Actor, Project: r.Project}); e != nil {
			return "", time.Time{}, ErrDenied
		}
	}
	cookie, e := token()
	if e != nil {
		return "", time.Time{}, e
	}
	r.PageHash = ""
	r.BrowserHash = digest(cookie)
	r.BrowserExpires = minTime(s.now().Add(15*time.Minute), r.ExpiresAt)
	if e = s.save(ctx, &r); e != nil {
		return "", time.Time{}, e
	}
	return cookie, r.BrowserExpires, nil
}
func (s *Service) authenticate(ctx context.Context, id, secret string, browser bool) (Identity, error) {
	if !validID(id) {
		return Identity{}, ErrDenied
	}
	r, e := s.opt.Repository.Get(ctx, id)
	if e != nil {
		return Identity{}, ErrDenied
	}
	hash, expiry := r.TransferHash, r.TransferExpires
	if browser {
		hash, expiry = r.BrowserHash, r.BrowserExpires
	}
	if !matches(secret, hash) {
		return Identity{}, ErrDenied
	}
	if !s.now().Before(expiry) {
		return Identity{}, ErrExpired
	}
	who := Identity{Actor: r.Actor, Project: r.Project}
	if s.opt.Authorize != nil {
		if e := s.opt.Authorize(ctx, who); e != nil {
			return Identity{}, ErrDenied
		}
	}
	return who, nil
}
func (s *Service) Bind(ctx context.Context, who Identity, id string, f File) (View, error) {
	if e := s.file(f); e != nil {
		return View{}, e
	}
	r, e := s.owned(ctx, who, id)
	if e != nil {
		return View{}, e
	}
	if e = s.active(r); e != nil {
		return View{}, e
	}
	if r.MaxBytes > 0 && f.Size > r.MaxBytes {
		return View{}, ErrInvalid
	}
	if r.AllowedMIME != "" && !includes(strings.Split(r.AllowedMIME, ","), f.MIME) {
		return View{}, ErrInvalid
	}
	if r.Size > 0 && (r.FileName != f.Name || r.MIME != f.MIME || r.Size != f.Size || (f.SHA256 != "" && r.SHA256 != "" && !strings.EqualFold(r.SHA256, f.SHA256))) {
		return View{}, ErrConflict
	}
	if r.Session != "" {
		return view(r), nil
	}
	if r.Size == 0 {
		r.FileName = f.Name
		r.MIME = f.MIME
		r.Size = f.Size
		r.SHA256 = f.SHA256
		r.State = "binding"
		if e = s.save(ctx, &r); e != nil {
			return View{}, e
		}
	}
	// Stable ID makes an interrupted Begin safe to reconcile without a second file.
	session, e := s.opt.Backend.Begin(ctx, who, id, f)
	if e != nil {
		e = classifyBackendError(e)
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if current, getErr := s.owned(persist, who, id); getErr == nil && current.State == "binding" {
			current.FailureCode = failureCode(e, "preparation_failed")
			_ = s.save(persist, &current)
		}
		return View{}, e
	}
	r, e = s.owned(ctx, who, id)
	if e != nil {
		return View{}, e
	}
	if r.State == "ready" {
		return view(r), nil
	}
	if e = s.active(r); e != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.opt.Backend.Abort(cleanup, who, session)
		return View{}, e
	}
	if r.Session != "" {
		return view(r), nil
	}
	r.Session = session
	r.State = "prepared"
	r.FailureCode = ""
	if e = s.save(ctx, &r); e != nil {
		return View{}, e
	}
	return view(r), nil
}
func (s *Service) Put(ctx context.Context, who Identity, id string, size int64, body io.Reader) (View, error) {
	r, e := s.owned(ctx, who, id)
	if e != nil {
		return View{}, e
	}
	if e = s.active(r); e != nil {
		return View{}, e
	}
	if r.Session == "" || size != r.Size {
		return View{}, ErrInvalid
	}
	if r.State == "transferred" {
		// A retry may replay the original body, but cannot silently substitute it.
		sum := sha256.New()
		n, err := io.Copy(sum, io.LimitReader(contextReader{ctx: ctx, reader: body}, r.Size+1))
		if err != nil || n != r.Size || hex.EncodeToString(sum.Sum(nil)) != r.SHA256 {
			return View{}, ErrConflict
		}
		return view(r), nil
	}
	if r.State == "verifying" || (r.State == "transferring" && s.now().Before(r.LeaseUntil)) {
		return View{}, ErrConflict
	}
	r.State = "transferring"
	r.FailureCode = ""
	r.LeaseUntil = s.now().Add(10 * time.Minute)
	if e = s.save(ctx, &r); e != nil {
		return View{}, e
	}
	sum := sha256.New()
	limited := &io.LimitedReader{R: body, N: r.Size + 1}
	count := &counter{Reader: io.TeeReader(limited, sum)}
	operation, stop := context.WithTimeout(ctx, 9*time.Minute)
	defer stop()
	count.Reader = contextReader{ctx: operation, reader: count.Reader}
	e = s.opt.Backend.Put(operation, who, r.Session, File{r.FileName, r.MIME, r.Size, r.SHA256}, count)
	e = classifyBackendError(e)
	if e == nil && (count.n != r.Size || (r.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), r.SHA256))) {
		e = ErrInvalid
	}
	// Persist interruption state even when the request context has been cancelled.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, getErr := s.owned(persist, who, id)
	if getErr != nil {
		return View{}, getErr
	}
	if current.State != "transferring" || !current.LeaseUntil.Equal(r.LeaseUntil) {
		return View{}, ErrConflict
	}
	if e != nil {
		current.State = "prepared"
		current.FailureCode = failureCode(e, "transfer_failed")
	} else {
		current.State = "transferred"
		current.FailureCode = ""
		current.SHA256 = hex.EncodeToString(sum.Sum(nil))
	}
	current.LeaseUntil = time.Time{}
	if saveErr := s.save(persist, &current); saveErr != nil {
		return View{}, saveErr
	}
	if e != nil {
		return View{}, e
	}
	return view(current), nil
}

type counter struct {
	io.Reader
	n int64
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (c *counter) Read(b []byte) (int, error) { n, e := c.Reader.Read(b); c.n += int64(n); return n, e }
func (s *Service) Complete(ctx context.Context, who Identity, id string) (View, error) {
	r, e := s.owned(ctx, who, id)
	if e != nil {
		return View{}, e
	}
	if r.State == "ready" {
		return view(r), nil
	}
	if e = s.active(r); e != nil {
		return View{}, e
	}
	if r.State != "transferred" && r.State != "verifying" {
		return View{}, ErrConflict
	}
	if r.State == "verifying" && s.now().Before(r.LeaseUntil) {
		return View{}, ErrConflict
	}
	r.State = "verifying"
	r.FailureCode = ""
	r.LeaseUntil = s.now().Add(10 * time.Minute)
	if e = s.save(ctx, &r); e != nil {
		return View{}, e
	}
	operation, stop := context.WithTimeout(ctx, 9*time.Minute)
	defer stop()
	receipt, e := s.opt.Backend.Complete(operation, who, r.Session, File{r.FileName, r.MIME, r.Size, r.SHA256})
	e = classifyBackendError(e)
	if e == nil && (receipt.Ref == "" || receipt.Size != r.Size || !strings.EqualFold(receipt.SHA256, r.SHA256)) {
		e = ErrInvalid
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, getErr := s.owned(persist, who, id)
	if getErr != nil {
		return View{}, getErr
	}
	if current.State != "verifying" || !current.LeaseUntil.Equal(r.LeaseUntil) {
		return View{}, ErrConflict
	}
	if e != nil {
		current.State = "transferred"
		current.FailureCode = failureCode(e, "verification_failed")
		current.DomainState = ""
		if e == ErrAdmission {
			current.FailureCode, current.DomainState = "domain_authorization_required", "requires_authorization"
		}
	} else {
		current.State = "ready"
		current.FailureCode = ""
		current.Ref = receipt.Ref
		current.DomainState = receipt.DomainState
		// The existing short-lived grant may replay complete/status until expiry.
		current.PageHash = ""
	}
	current.LeaseUntil = time.Time{}
	if saveErr := s.save(persist, &current); saveErr != nil {
		return View{}, saveErr
	}
	if e != nil {
		return View{}, e
	}
	return view(current), nil
}
func (s *Service) Abort(ctx context.Context, who Identity, id string) (View, error) {
	r, e := s.owned(ctx, who, id)
	if e != nil {
		return View{}, e
	}
	if r.State == "cancelled" {
		if r.Session != "" {
			if err := s.opt.Backend.Abort(ctx, who, r.Session); err != nil {
				return View{}, err
			}
		}
		return view(r), nil
	}
	if r.State == "ready" {
		return View{}, ErrConflict
	}
	if (r.State == "transferring" || r.State == "verifying") && s.now().Before(r.LeaseUntil) {
		return View{}, ErrConflict
	}
	r.State = "cancelled"
	r.PageHash = ""
	r.TransferHash = ""
	if e = s.save(ctx, &r); e != nil {
		return View{}, e
	}
	if r.Session != "" {
		if e = s.opt.Backend.Abort(ctx, who, r.Session); e != nil {
			return View{}, e
		}
	}
	return view(r), nil
}
func (s *Service) Capabilities() map[string]any { return Describe(s.opt) }

// Describe is a pure projection, so discovery never opens owner state or storage.
func Describe(opt Options) map[string]any {
	return map[string]any{
		"schema_version": Schema, "owner": opt.Owner, "enabled": opt.Enabled, "owner_base_url": opt.BaseURL,
		"max_bytes": opt.MaxBytes, "mime_types": opt.MIME, "purposes": opt.Purposes,
		"transports": []string{"http_put", "browser"},
		"status":     map[string]string{"permission": "owner_scoped_identity_required", "storage": "configured_not_probed", "transport_reachability": "configured_not_probed", "downstream": "file_and_domain_admission_required"},
		"actions":    map[string]string{"prepare": "input.prepare", "status": "input.status", "renew": "input.renew", "abort": "input.abort"},
		"http": map[string]any{
			"grant_header": GrantHeader, "method": "PUT", "content_path": Prefix + "{input_request_id}/content", "metadata_path": Prefix + "{input_request_id}/file", "complete_path": Prefix + "{input_request_id}/complete", "status_path": Prefix + "{input_request_id}/status",
			"metadata_method": "POST", "complete_method": "POST", "status_method": "GET", "cancel_method": "POST", "cancel_path": Prefix + "{input_request_id}/abort",
			"file_schema": FileSchema(), "response_schema": ViewSchema(), "complete_body": map[string]any{},
		},
		"defaults":         map[string]any{"grant_ttl_seconds": 300, "page_exchange_ttl_seconds": 900, "browser_session_ttl_seconds": 900, "file_hash": "optional; owner computes SHA-256 while receiving", "one_file_per_request": true},
		"state_mapping":    map[string]string{"awaiting_file": "awaiting_file", "binding": "preparing_transfer", "prepared": "awaiting_transfer", "transferring": "transferring", "transferred": "awaiting_validation", "verifying": "validating", "ready": "input_available", "failed": "failed; use resume_state and original request", "cancelled": "cancelled", "expired": "expired"},
		"sensitive_output": map[string]any{"content_type": "resource_link", "page_fragment": "page", "transfer_fragment": "grant", "persist": false, "missing_link_recovery": "Recover by original request or idempotency key, then renew in a client preserving transient links"},
		"notes":            []string{"File paths and bytes never enter MCP JSON.", "Each HTTP capability is bound to the owner identity, project, request and admitted byte limit.", "Uploading does not approve generation, analysis or canonical acceptance."},
	}
}
func FileSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}, "mime": map[string]any{"type": "string"}, "size": map[string]any{"type": "integer", "minimum": 1}, "sha256": map[string]any{"type": "string", "pattern": "^[a-fA-F0-9]{64}$"}}, "required": []string{"name", "mime", "size"}}
}
func ViewSchema() map[string]any {
	props := map[string]any{}
	for _, key := range []string{"schema_version", "input_request_id", "project", "purpose", "state", "expires_at", "domain_state", "failure_code", "resume_state"} {
		props[key] = map[string]any{"type": "string"}
	}
	props["max_bytes"] = map[string]any{"type": "integer"}
	props["mime_types"] = map[string]any{"type": "array", "items": map[string]string{"type": "string"}}
	props["file"] = FileSchema()
	props["receipt"] = map[string]any{"type": "object", "properties": map[string]any{"ref": map[string]string{"type": "string"}, "sha256": map[string]string{"type": "string"}, "size": map[string]string{"type": "integer"}, "domain_state": map[string]string{"type": "string"}}, "required": []string{"ref", "sha256", "size", "domain_state"}, "additionalProperties": false}
	return map[string]any{"type": "object", "properties": props, "required": []string{"schema_version", "input_request_id", "project", "purpose", "state", "expires_at", "max_bytes", "mime_types"}, "additionalProperties": false}
}

func safeError(e error) error {
	switch e {
	case ErrDenied, ErrConflict, ErrInvalid, ErrDisabled, ErrExpired, ErrAdmission, ErrCapacity, ErrStorage:
		return e
	default:
		return fmt.Errorf("input operation failed; query status before retrying")
	}
}

func classifyBackendError(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return ErrCapacity
	}
	if errors.Is(err, fs.ErrPermission) {
		return ErrStorage
	}
	return err
}
func failureCode(err error, fallback string) string {
	switch err {
	case ErrCapacity:
		return "storage_capacity_unavailable"
	case ErrStorage:
		return "storage_unavailable"
	default:
		return fallback
	}
}
