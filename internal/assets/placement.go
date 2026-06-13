package assets

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type AttachmentPlacementPolicy string

const (
	AttachmentPlacementPerNote     AttachmentPlacementPolicy = "per-note"
	AttachmentPlacementVaultFolder AttachmentPlacementPolicy = "vault-folder"
	AttachmentPlacementNoteFolder  AttachmentPlacementPolicy = "note-folder"
)

type AttachmentPlacementRequest struct {
	Root     string
	NoteID   string
	NotePath string
	Filename string
	Policy   AttachmentPlacementPolicy
}

func PlaceAttachment(req AttachmentPlacementRequest) (string, error) {
	policy := req.Policy
	if policy == "" {
		policy = AttachmentPlacementPerNote
	}
	filename := filepath.Base(strings.TrimSpace(req.Filename))
	if filename == "" || filename == "." || filename == string(os.PathSeparator) {
		return "", &domain.CommandError{Code: "attachment_filename_invalid", Message: "附件文件名无效", Hint: "传入带文件名的源文件路径"}
	}
	base, err := attachmentPlacementBase(req, policy)
	if err != nil {
		return "", err
	}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)
	for i := 0; i < 1000; i++ {
		candidateName := filename
		if i > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", stem, i+1, ext)
		}
		rel := filepath.ToSlash(filepath.Join(base, candidateName))
		abs, err := safeAttachmentPath(req.Root, rel)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return rel, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", &domain.CommandError{Code: "attachment_name_conflict", Message: "附件文件名冲突过多", Hint: "换一个源文件名后重试"}
}

func attachmentPlacementBase(req AttachmentPlacementRequest, policy AttachmentPlacementPolicy) (string, error) {
	switch policy {
	case AttachmentPlacementPerNote:
		owner := strings.TrimSpace(req.NoteID)
		if owner == "" {
			owner = stableAttachmentNoteID(req.NotePath)
		}
		return safeAttachmentRel(filepath.ToSlash(filepath.Join("attachments", owner)))
	case AttachmentPlacementVaultFolder:
		return safeAttachmentRel("attachments")
	case AttachmentPlacementNoteFolder:
		notePath, err := safeAttachmentRel(req.NotePath)
		if err != nil {
			return "", err
		}
		return safeAttachmentRel(filepath.ToSlash(filepath.Join(pathpkg.Dir(notePath), "assets")))
	default:
		return "", &domain.CommandError{Code: "attachment_placement_invalid", Message: "附件目录策略无效", Hint: "使用 per-note、vault-folder 或 note-folder"}
	}
}

func stableAttachmentNoteID(notePath string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(notePath)))
	return "note_" + hex.EncodeToString(sum[:])[:12]
}

func safeAttachmentPath(root, rel string) (string, error) {
	clean, err := safeAttachmentRel(rel)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func safeAttachmentRel(rel string) (string, error) {
	clean := pathpkg.Clean(filepath.ToSlash(strings.TrimSpace(rel)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, ".pinax/") || pathpkg.IsAbs(clean) {
		return "", &domain.CommandError{Code: "attachment_path_unsafe", Message: "附件路径越过 vault 边界", Hint: "选择 vault 内的附件路径"}
	}
	return clean, nil
}
