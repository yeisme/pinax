// Package inputintake provides an opt-in input-request protocol and HTTP adapter.
// Owners supply persistence, authorization and all file/domain operations.
package inputintake

import (
	"context"
	"errors"
	"io"
	"time"
)

const Schema = "yeisme.input_intake.v1"
const Prefix = "/input-requests/"
const GrantHeader = "X-Input-Grant"

var ErrDenied = errors.New("input request is unavailable to this identity")
var ErrConflict = errors.New("input request changed; read its status before retrying")
var ErrInvalid = errors.New("input request parameters are invalid")
var ErrDisabled = errors.New("input intake is not configured")
var ErrExpired = errors.New("input request authorization expired; renew through the owner")
var ErrAdmission = errors.New("file transfer succeeded; domain authorization is required")
var ErrCapacity = errors.New("input storage capacity is unavailable; recover the original request after the owner restores capacity")
var ErrStorage = errors.New("input storage is unavailable; recover the original request after the owner restores storage")

// Identity must come from the owner's authenticated transport, never HTTP input.
type Identity struct {
	Actor   string
	Project string
}
type File struct {
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}
type Prepare struct {
	Purpose        string `json:"purpose"`
	IdempotencyKey string `json:"idempotency_key"`
	File           *File  `json:"file,omitempty"`
}
type Receipt struct {
	Ref         string `json:"ref"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	DomainState string `json:"domain_state"`
}

// Record is persisted by the owner. Token material is represented by digests only.
// JSON encoders must use View rather than Record.
type Record struct {
	MaxBytes        int64     `json:"-"`
	AllowedMIME     string    `json:"-"`
	ID              string    `json:"-"`
	Revision        uint64    `json:"-"`
	Actor           string    `json:"-"`
	Project         string    `json:"-"`
	Purpose         string    `json:"-"`
	IntentDigest    string    `json:"-"`
	State           string    `json:"-"`
	FileName        string    `json:"-"`
	MIME            string    `json:"-"`
	Size            int64     `json:"-"`
	SHA256          string    `json:"-"`
	Session         string    `json:"-"`
	Ref             string    `json:"-"`
	DomainState     string    `json:"-"`
	FailureCode     string    `json:"-"`
	CreatedAt       time.Time `json:"-"`
	ExpiresAt       time.Time `json:"-"`
	LeaseUntil      time.Time `json:"-"`
	PageHash        string    `json:"-"`
	PageExpires     time.Time `json:"-"`
	BrowserHash     string    `json:"-"`
	BrowserExpires  time.Time `json:"-"`
	TransferHash    string    `json:"-"`
	TransferExpires time.Time `json:"-"`
}
type View struct {
	MaxBytes      int64     `json:"max_bytes"`
	MIME          []string  `json:"mime_types"`
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"input_request_id"`
	Project       string    `json:"project"`
	Purpose       string    `json:"purpose"`
	State         string    `json:"state"`
	ExpiresAt     time.Time `json:"expires_at"`
	File          *File     `json:"file,omitempty"`
	Receipt       *Receipt  `json:"receipt,omitempty"`
	DomainState   string    `json:"domain_state,omitempty"`
	FailureCode   string    `json:"failure_code,omitempty"`
	ResumeState   string    `json:"resume_state,omitempty"`
}

// Access contains transient capabilities. Do not persist it or audit its body.
type Access struct {
	Request     View   `json:"request"`
	PageURL     string `json:"-"`
	TransferURL string `json:"-"`
}

// Repository must implement atomic create-if-absent and compare-and-swap.
// Get returns ErrDenied for a missing record. CAS increments Revision by one.
type Repository interface {
	Get(context.Context, string) (Record, error)
	Create(context.Context, Record) error
	CAS(context.Context, Record, uint64) error
}

// Backend is scoped to this owner. Begin and Complete MUST be idempotent for ID.
// Put streams bounded bytes; Complete verifies real media before publishing a ref.
type Backend interface {
	Begin(context.Context, Identity, string, File) (string, error)
	Put(context.Context, Identity, string, File, io.Reader) error
	Complete(context.Context, Identity, string, File) (Receipt, error)
	Abort(context.Context, Identity, string) error
}
type Options struct {
	Authorize  func(context.Context, Identity) error
	Owner      string
	BaseURL    string
	Enabled    bool
	MaxBytes   int64
	MIME       []string
	Purposes   []string
	TTL        time.Duration
	Repository Repository
	Backend    Backend
	Now        func() time.Time
}
