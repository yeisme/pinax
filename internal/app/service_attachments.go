package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
)

func uniqueAttachmentRelWithPlacement(root string, note domain.Note, filename string, placement pinaxassets.AttachmentPlacementPolicy) (string, error) {
	return pinaxassets.PlaceAttachment(pinaxassets.AttachmentPlacementRequest{Root: root, NoteID: note.ID, NotePath: note.Path, Filename: filename, Policy: placement})
}

func registeredAttachmentRel(root, source string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absSource)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, ".pinax/") {
		return "", &domain.CommandError{Code: "asset_outside_vault", Message: "register mode only accepts files inside the vault", Hint: "Use a file inside the vault, or switch to --mode copy"}
	}
	if _, err := safeJoin(root, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func normalizedAttachmentMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", "copy":
		return "copy"
	case "move":
		return "move"
	case "register":
		return "register"
	default:
		return ""
	}
}

func normalizedAttachmentPlacement(placement string) pinaxassets.AttachmentPlacementPolicy {
	switch strings.TrimSpace(placement) {
	case "":
		return pinaxassets.AttachmentPlacementPerNote
	default:
		return pinaxassets.AttachmentPlacementPolicy(strings.TrimSpace(placement))
	}
}

func attachmentReference(notePath, attachmentRel, style string, embed bool) (string, string, error) {
	style = strings.TrimSpace(style)
	if style == "" || style == "auto" {
		style = "markdown"
	}
	switch style {
	case "markdown":
		return style, markdownAttachmentReferenceWithEmbed(notePath, attachmentRel, embed), nil
	case "wiki":
		return style, wikiAttachmentReference(attachmentRel, embed), nil
	default:
		return "", "", &domain.CommandError{Code: "attachment_link_style_invalid", Message: "Attachment link style is invalid", Hint: "Use --link-style markdown, wiki, or auto"}
	}
}

func markdownAttachmentReferenceWithEmbed(notePath, attachmentRel string, embed bool) string {
	rel, err := filepath.Rel(filepath.Dir(filepath.FromSlash(notePath)), filepath.FromSlash(attachmentRel))
	if err != nil {
		rel = filepath.FromSlash(attachmentRel)
	}
	rel = filepath.ToSlash(rel)
	label := filepath.Base(attachmentRel)
	if embed || attachmentMediaType(attachmentRel) == "image" {
		return fmt.Sprintf("![%s](%s)", label, rel)
	}
	return fmt.Sprintf("[%s](%s)", label, rel)
}

func wikiAttachmentReference(attachmentRel string, embed bool) string {
	if embed || attachmentMediaType(attachmentRel) == "image" {
		return fmt.Sprintf("![[%s]]", attachmentRel)
	}
	return fmt.Sprintf("[[%s]]", attachmentRel)
}

func noteAttachmentsFromBody(root string, note domain.Note) []domain.NoteAttachment {
	links := pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: note.ID, SourcePath: note.Path, Body: note.Body})
	attachments := make([]domain.NoteAttachment, 0, len(links))
	for _, link := range links {
		abs := filepath.Join(root, filepath.FromSlash(link.AssetPath))
		_, statErr := os.Stat(abs)
		attachments = append(attachments, domain.NoteAttachment{NotePath: note.Path, ReferenceText: link.RawReference, Path: link.AssetPath, TargetPath: link.AssetPath, MediaType: attachmentMediaType(link.AssetPath), Exists: statErr == nil})
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].TargetPath < attachments[j].TargetPath })
	return attachments
}

func attachmentMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".pdf", ".doc", ".docx", ".txt":
		return "document"
	case ".mp3", ".wav", ".ogg":
		return "audio"
	case ".mp4", ".mov", ".webm":
		return "video"
	default:
		return "file"
	}
}

func countMissingAttachments(attachments []domain.NoteAttachment) int {
	count := 0
	for _, attachment := range attachments {
		if !attachment.Exists {
			count++
		}
	}
	return count
}
