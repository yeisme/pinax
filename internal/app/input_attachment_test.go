package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/pkg/inputcontract"
)

func attachmentFixture(t *testing.T) (string, *Service, inputcontract.AttachmentRequest, StagedAttachment, string) {
	t.Helper()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(t.Context(), InitVaultRequest{VaultPath: root, Title: "Attachment fixture"}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateNote(t.Context(), CreateNoteRequest{VaultPath: root, Title: "Attachment note", Body: "original", Dir: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	note, revision, err := svc.CollaborationRead(t.Context(), root, created.Facts["path"], "edit", writerPolicy())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("attachment fixture")
	sum := sha256.Sum256(data)
	return root, svc, inputcontract.AttachmentRequest{InputRequestID: "input-fixture", OperationID: "attachment-fixture", ObjectID: note.ID, Revision: revision, Placement: "per-note"}, StagedAttachment{Name: "fixture.txt", MIME: "text/plain", SHA256: hex.EncodeToString(sum[:]), Bytes: data}, note.Path
}

func TestInputAttachmentUploadReplaceAndReplay(t *testing.T) {
	for _, placement := range []string{"per-note", "vault-folder", "note-folder"} {
		t.Run(placement, func(t *testing.T) {
			root, svc, req, file, notePath := attachmentFixture(t)
			req.Placement = placement
			preview, err := svc.InputAttachmentPreview(t.Context(), root, req, file)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(root, notePath))
			view, err := svc.InputAttachmentApply(t.Context(), root, inputcontract.AttachmentApply{Proposal: preview, Confirm: true}, file, OperationAccess{OwnerLocal: true})
			if err != nil || view.Status != "succeeded" || view.RevisionAfter == req.Revision || !strings.HasPrefix(view.ReceiptRef, "snapshot:") {
				t.Fatalf("apply: %v, %s", err, view.Status)
			}
			var proof attachmentProof
			if err := json.Unmarshal(view.Result, &proof); err != nil {
				t.Fatal(err)
			}
			originalAsset, err := os.ReadFile(filepath.Join(root, proof.AssetPath))
			if err != nil || string(originalAsset) != string(file.Bytes) {
				t.Fatal("missing attachment readback", err)
			}
			after, _ := os.ReadFile(filepath.Join(root, notePath))
			if string(before) == string(after) || !strings.Contains(string(after), preview.Reference) {
				t.Fatal("reference was not atomically published")
			}
			replayed, err := NewService().InputAttachmentApply(t.Context(), root, inputcontract.AttachmentApply{Proposal: preview, Confirm: true}, file, OperationAccess{OwnerLocal: true})
			if err != nil || replayed.OperationID != view.OperationID || replayed.RevisionAfter != view.RevisionAfter {
				t.Fatal("replay changed receipt", err)
			}
			req.InputRequestID, req.OperationID = "input-replace", "attachment-replace"
			req.Revision, req.ReplaceReference = view.RevisionAfter, preview.Reference
			file.Bytes = []byte("replacement fixture")
			sum := sha256.Sum256(file.Bytes)
			file.SHA256 = hex.EncodeToString(sum[:])
			replacement, err := svc.InputAttachmentPreview(t.Context(), root, req, file)
			if err != nil {
				t.Fatal(err)
			}
			result, err := svc.InputAttachmentApply(t.Context(), root, inputcontract.AttachmentApply{Proposal: replacement, Confirm: true}, file, OperationAccess{OwnerLocal: true})
			if err != nil || result.Status != "succeeded" {
				t.Fatal("replace failed", err)
			}
			final, _ := os.ReadFile(filepath.Join(root, notePath))
			old, _ := os.ReadFile(filepath.Join(root, proof.AssetPath))
			if strings.Contains(string(final), preview.Reference) || !strings.Contains(string(final), replacement.Reference) || string(old) != string(originalAsset) {
				t.Fatal("replacement corrupted old attachment or references")
			}
		})
	}
}

func TestInputAttachmentInvalidProposalHasNoMutation(t *testing.T) {
	for _, scenario := range []string{"mime", "size", "hash", "revision", "identity", "placement", "reference", "unconfirmed", "tamper", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root, svc, req, file, notePath := attachmentFixture(t)
			preview, err := svc.InputAttachmentPreview(t.Context(), root, req, file)
			if err != nil {
				t.Fatal(err)
			}
			apply := inputcontract.AttachmentApply{Proposal: preview, Confirm: true}
			switch scenario {
			case "mime":
				file.MIME = "image/png"
			case "size":
				file.Bytes = make([]byte, (2<<20)+1)
			case "hash":
				file.SHA256 = strings.Repeat("0", 64)
			case "revision":
				apply.Proposal.Request.Revision = "sha256:" + strings.Repeat("0", 64)
			case "identity":
				apply.Proposal.Request.ObjectID = "missing"
			case "placement":
				apply.Proposal.Request.Placement = "../outside"
			case "reference":
				apply.Proposal.Request.ReplaceReference = "[missing](../outside)"
			case "unconfirmed":
				apply.Confirm = false
			case "tamper":
				apply.Proposal.Reference = "[wrong](outside)"
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "attachments")); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(filepath.Join(root, notePath))
			if _, err := svc.InputAttachmentApply(t.Context(), root, apply, file, OperationAccess{OwnerLocal: true}); err == nil {
				t.Fatal("invalid request accepted")
			}
			after, _ := os.ReadFile(filepath.Join(root, notePath))
			if string(before) != string(after) {
				t.Fatal("invalid request changed source")
			}
			if _, err := os.Stat(filepath.Join(root, ".pinax/api/operations.sqlite")); !os.IsNotExist(err) {
				t.Fatal("invalid request created operation receipt")
			}
		})
	}
}

func TestInputAttachmentStalePreviewRejectsMetadataChange(t *testing.T) {
	root, svc, req, file, notePath := attachmentFixture(t)
	preview, err := svc.InputAttachmentPreview(t.Context(), root, req, file)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, notePath))
	changed := strings.Replace(string(raw), "title: Attachment note", "title: Changed metadata", 1)
	if changed == string(raw) {
		t.Fatal("fixture title not found")
	}
	if err := os.WriteFile(filepath.Join(root, notePath), []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = svc.InputAttachmentApply(t.Context(), root, inputcontract.AttachmentApply{Proposal: preview, Confirm: true}, file, OperationAccess{OwnerLocal: true})
	if domain.ErrorCode(err) != "revision_conflict" {
		t.Fatalf("metadata revision guard: %v", err)
	}
}

func TestInputAttachmentMIMEAndSizeMatrix(t *testing.T) {
	for mime, body := range map[string][]byte{
		"text/plain": []byte("plain"), "text/markdown": []byte("# markdown"),
		"image/png": []byte("\x89PNG\r\n\x1a\nfixture"), "image/jpeg": []byte("\xff\xd8\xfffixture"),
		"image/webp": []byte("RIFF\x00\x00\x00\x00WEBPVP8 fixture"), "application/pdf": []byte("%PDF-1.7\nfixture"),
	} {
		t.Run(mime, func(t *testing.T) {
			root, svc, req, file, _ := attachmentFixture(t)
			file.MIME, file.Bytes = mime, body
			sum := sha256.Sum256(body)
			file.SHA256 = hex.EncodeToString(sum[:])
			if _, err := svc.InputAttachmentPreview(t.Context(), root, req, file); err != nil {
				t.Fatal(err)
			}
		})
	}
	root, svc, req, file, _ := attachmentFixture(t)
	file.Bytes = []byte(strings.Repeat("a", 2<<20))
	sum := sha256.Sum256(file.Bytes)
	file.SHA256 = hex.EncodeToString(sum[:])
	if _, err := svc.InputAttachmentPreview(t.Context(), root, req, file); err != nil {
		t.Fatal("exact size limit rejected", err)
	}
	file.Bytes = append(file.Bytes, 'a')
	sum = sha256.Sum256(file.Bytes)
	file.SHA256 = hex.EncodeToString(sum[:])
	if _, err := svc.InputAttachmentPreview(t.Context(), root, req, file); domain.ErrorCode(err) != "attachment_invalid" {
		t.Fatal("oversize accepted", err)
	}
}

func TestInputAttachmentRejectsAmbiguousIdentity(t *testing.T) {
	root, svc, req, file, notePath := attachmentFixture(t)
	raw, err := os.ReadFile(filepath.Join(root, notePath))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "duplicate.md"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InputAttachmentPreview(t.Context(), root, req, file); domain.ErrorCode(err) != "note_identity_ambiguous" {
		t.Fatalf("ambiguous identity: %v", err)
	}
}
