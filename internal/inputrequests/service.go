// Package inputrequests owns bounded text staging and Pinax input preview.
package inputrequests

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/sqlitedsn"
	intake "github.com/yeisme/runtime-plane/pkg/inputintake"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxBytes = 2 << 20

type row intake.Record

func (row) TableName() string { return "input_requests" }

type repository struct{ db *gorm.DB }

func (r repository) Get(ctx context.Context, id string) (intake.Record, error) {
	var v row
	e := r.db.WithContext(ctx).Where(&row{ID: id}).First(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = intake.ErrDenied
	}
	return intake.Record(v), e
}
func (r repository) Create(ctx context.Context, v intake.Record) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&row{}).Where(map[string]any{"project": v.Project}).Where(clause.Gt{Column: "expires_at", Value: v.CreatedAt}).Not(map[string]any{"state": []string{"ready", "cancelled"}}).Count(&count).Error; err != nil {
			return err
		}
		if count >= 128 {
			return intake.ErrCapacity
		}
		data := row(v)
		if err := tx.Create(&data).Error; err != nil {
			return intake.ErrConflict
		}
		return nil
	})
}
func (r repository) CAS(ctx context.Context, v intake.Record, revision uint64) error {
	data := row(v)
	result := r.db.WithContext(ctx).Model(&row{}).Where(map[string]any{"id": v.ID, "revision": revision}).Select("*").Updates(&data)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return intake.ErrConflict
	}
	return nil
}

type backend struct {
	project string
	dir     string
}

func (b backend) Begin(_ context.Context, _ intake.Identity, id string, _ intake.File) (string, error) {
	return id, nil
}
func (b backend) Put(ctx context.Context, _ intake.Identity, id string, f intake.File, reader io.Reader) error {
	if e := os.MkdirAll(b.dir, 0700); e != nil {
		return e
	}
	root, e := os.OpenRoot(b.dir)
	if e != nil {
		return e
	}
	defer func() { _ = root.Close() }()
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return e
	}
	name := "pending_" + hex.EncodeToString(nonce[:])
	file, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer func() { _ = file.Close(); _ = root.Remove(name) }()
	digest := sha256.New()
	n, e := io.Copy(io.MultiWriter(file, digest), io.LimitReader(reader, f.Size+1))
	if e != nil {
		return e
	}
	if n != f.Size || (f.SHA256 != "" && !strings.EqualFold(f.SHA256, hex.EncodeToString(digest.Sum(nil)))) {
		return intake.ErrInvalid
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = file.Sync(); e != nil {
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	if e = root.Link(name, id); errors.Is(e, os.ErrExist) {
		existing, e := root.Open(id)
		if e != nil {
			return e
		}
		defer func() { _ = existing.Close() }()
		sum := sha256.New()
		n, e = io.Copy(sum, io.LimitReader(existing, f.Size+1))
		if e != nil || n != f.Size || hex.EncodeToString(sum.Sum(nil)) != hex.EncodeToString(digest.Sum(nil)) {
			return intake.ErrConflict
		}
		return nil
	}
	return e
}
func (b backend) Complete(ctx context.Context, _ intake.Identity, id string, f intake.File) (intake.Receipt, error) {
	root, e := os.OpenRoot(b.dir)
	if e != nil {
		return intake.Receipt{}, e
	}
	defer func() { _ = root.Close() }()
	file, e := root.Open(id)
	if e != nil {
		return intake.Receipt{}, e
	}
	defer func() { _ = file.Close() }()
	body, e := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if e != nil || int64(len(body)) != f.Size {
		return intake.Receipt{}, intake.ErrInvalid
	}
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	if sha != f.SHA256 {
		return intake.Receipt{}, intake.ErrInvalid
	}
	if strings.HasPrefix(f.MIME, "text/") {
		if !utf8.Valid(body) || strings.ContainsRune(string(body), 0) {
			return intake.Receipt{}, intake.ErrInvalid
		}
	} else if http.DetectContentType(body) != f.MIME {
		return intake.Receipt{}, intake.ErrInvalid
	}
	return intake.Receipt{Ref: "pinax://input/" + id, Size: f.Size, SHA256: sha, DomainState: "requires_preview_confirmation"}, nil
}
func (b backend) Abort(_ context.Context, _ intake.Identity, id string) error {
	root, e := os.OpenRoot(b.dir)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	defer func() { _ = root.Close() }()
	e = root.Remove(id)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return e
}

type Service struct {
	*intake.Service
	repo repository
	root string
	app  *app.Service
}

func Open(root, base string, application *app.Service) (*Service, func(), error) {
	if _, e := os.Stat(filepath.Join(root, ".pinax", "config.yaml")); e != nil {
		return nil, func() {}, intake.ErrDisabled
	}
	path := filepath.Join(root, ".pinax", "input-requests.db")
	file, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, func() {}, e
	}
	_ = file.Close()
	db, e := sqlitedsn.Open(path)
	if e != nil {
		return nil, func() {}, e
	}
	sql, e := db.DB()
	if e != nil {
		return nil, func() {}, e
	}
	close := func() { _ = sql.Close() }
	if e = db.AutoMigrate(&row{}, &adoption{}); e != nil {
		close()
		return nil, func() {}, e
	}
	r := repository{db}
	common, e := intake.New(intake.Options{Owner: "pinax", Enabled: true, BaseURL: base, MaxBytes: MaxBytes, TTL: 15 * time.Minute, MIME: []string{"text/markdown", "text/plain", "image/png", "image/jpeg", "image/webp", "application/pdf"}, Purposes: []string{"markdown", "attachment"}, Repository: r, Backend: backend{root, filepath.Join(root, ".pinax", "input-files")}})
	if e != nil {
		close()
		return nil, func() {}, e
	}
	return &Service{common, r, root, application}, close, nil
}

type adoption struct {
	ID      string `gorm:"primaryKey"`
	Digest  string
	State   string
	Receipt string
	Intent  string
}

func (adoption) TableName() string { return "input_adoptions" }

type Preview struct {
	InputRef string `json:"input_ref"`
	Digest   string `json:"preview_digest"`
	State    string `json:"state"`
	Plan     any    `json:"plan,omitempty"`
}

func (s *Service) Preview(ctx context.Context, who intake.Identity, id, conflict, note string, confirm bool, expected string) (Preview, error) {
	v, e := s.Status(ctx, who, id)
	if e != nil {
		return Preview{}, e
	}
	if v.State != "ready" || v.File == nil {
		return Preview{}, intake.ErrConflict
	}
	if conflict != "skip" && conflict != "rename" && conflict != "overwrite" {
		return Preview{}, intake.ErrInvalid
	}
	intentBytes, _ := json.Marshal([]string{id, v.File.SHA256, conflict, note})
	intentHash := sha256.Sum256(intentBytes)
	intent := hex.EncodeToString(intentHash[:])
	var previous adoption
	if err := s.repo.db.WithContext(ctx).Where(&adoption{ID: id}).First(&previous).Error; err == nil {
		if previous.Intent != intent || (confirm && expected != previous.Digest) {
			return Preview{}, intake.ErrConflict
		}
		return Preview{InputRef: v.Receipt.Ref, Digest: previous.Digest, State: previous.State}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Preview{}, err
	}
	// Copy into a server-owned per-request directory so user names cannot escape staging.
	dir := filepath.Join(s.root, ".pinax", "input-files", id+"-import")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return Preview{}, e
	}
	source := filepath.Join(dir, v.File.Name)
	if e = os.Link(filepath.Join(s.root, ".pinax", "input-files", id), source); e != nil && !errors.Is(e, os.ErrExist) {
		return Preview{}, e
	}
	var projection domain.Projection
	var plan any
	var targetDigests []string
	if v.Purpose == "markdown" {
		if strings.ToLower(filepath.Ext(source)) != ".md" {
			return Preview{}, intake.ErrInvalid
		}
		projection, e = s.app.ImportMarkdown(ctx, app.ImportMarkdownRequest{VaultPath: s.root, Source: source, Conflict: conflict, DryRun: true})
		if e != nil {
			return Preview{}, e
		}
		items := []map[string]any{}
		data, _ := projection.Data.(map[string]any)
		plans, _ := data["plans"].([]domain.ImportPlan)
		root, err := os.OpenRoot(s.root)
		if err != nil {
			return Preview{}, err
		}
		defer func() { _ = root.Close() }()
		for _, item := range plans {
			items = append(items, map[string]any{"target": item.TargetPath, "status": item.Status, "conflict": item.Conflict})
			file, err := root.Open(item.TargetPath)
			if errors.Is(err, os.ErrNotExist) {
				targetDigests = append(targetDigests, "absent")
				continue
			}
			if err != nil {
				return Preview{}, err
			}
			h := sha256.New()
			_, err = io.Copy(h, file)
			_ = file.Close()
			if err != nil {
				return Preview{}, err
			}
			targetDigests = append(targetDigests, hex.EncodeToString(h.Sum(nil)))
		}
		plan = items
	} else {
		if note == "" {
			return Preview{}, intake.ErrInvalid
		}
		resolved, err := s.app.ShowNote(ctx, app.ShowNoteRequest{VaultPath: s.root, NoteRef: note})
		if err != nil {
			return Preview{}, err
		}
		h := sha256.Sum256([]byte(resolved.Body))
		targetDigests = append(targetDigests, hex.EncodeToString(h[:]))
		plan = map[string]any{"note_ref": resolved.ID, "attachment": v.File.Name}
	}
	raw, _ := json.Marshal([]any{id, v.File.SHA256, conflict, note, projection.Data, targetDigests})
	h := sha256.Sum256(raw)
	hash := hex.EncodeToString(h[:])
	preview := Preview{InputRef: v.Receipt.Ref, Digest: hash, State: "preview", Plan: plan}
	if !confirm {
		return preview, nil
	}
	if expected != hash {
		return Preview{}, intake.ErrConflict
	}
	// Claim before the existing writer. Unknown results remain unconfirmed, never replayed.
	claim := adoption{ID: id, Digest: hash, Intent: intent, State: "unconfirmed"}
	if e = s.repo.db.WithContext(ctx).Create(&claim).Error; e != nil {
		return Preview{}, intake.ErrConflict
	}
	if v.Purpose == "markdown" {
		projection, e = s.app.ImportMarkdown(ctx, app.ImportMarkdownRequest{VaultPath: s.root, Source: source, Conflict: conflict, Yes: true})
	} else {
		projection, e = s.app.AttachNoteFile(ctx, app.NoteAttachRequest{VaultPath: s.root, NoteRef: note, SourcePath: source, Mode: "copy"})
	}
	if e != nil {
		return Preview{}, e
	}
	result := s.repo.db.WithContext(ctx).Model(&adoption{}).Where(&adoption{ID: id}).Updates(map[string]any{"state": "adopted", "receipt": projection.Facts["receipt_path"]})
	if result.Error != nil {
		return Preview{}, result.Error
	}
	preview.State = "adopted"
	preview.Plan = nil
	return preview, nil
}
