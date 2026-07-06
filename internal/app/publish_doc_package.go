package app

import (
	"fmt"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/publishdocast"
)

func buildPublishDocPackage(root string, profile domain.PublishDocProfile, note domain.Note) (domain.PublishDocPackage, domain.PublishDocCrossDocSummary, error) {
	now := time.Now().UTC()
	pkg := domain.PublishDocPackage{
		SchemaVersion: domain.PublishDocPackageSchemaVersion,
		ID:            publishDocPackageID(note.ID, profile.Target, now),
		NoteID:        note.ID,
		NotePath:      note.Path,
		Target:        profile.Target,
		Title:         note.Title,
		ContentDigest: publishDocDigest(note),
		BodyMarkdown:  publishDocRenderedMarkdown(note, profile),
		RemotePath:    publishDocRemotePath(note, profile),
		CreatedAt:     now.Format(time.RFC3339),
	}
	crossDocSummary := domain.PublishDocCrossDocSummary{}
	if profile.Target == domain.PublishDocTargetLarkDoc && profile.ResolveDocRenderer() == domain.PublishDocRendererNativeDocx {
		crossDoc := publishDocAnalyzeCrossDocLinks(root, note, pkg.BodyMarkdown, profile.Target)
		pkg.BodyMarkdown = crossDoc.Body
		crossDocSummary = crossDoc.Summary
		if crossDocSummary.Total > 0 {
			pkg.CrossDocLinks = &crossDocSummary
		}
	}
	if profile.Target == domain.PublishDocTargetLarkDoc {
		pkg.Renderer = profile.ResolveDocRenderer()
		if pkg.Renderer == domain.PublishDocRendererNativeDocx {
			plan, warnings := publishDocBuildPlanFromMarkdown(note, profile, pkg.BodyMarkdown)
			planBody, marshalErr := publishdocast.MarshalNativePlan(plan)
			if marshalErr != nil {
				return domain.PublishDocPackage{}, domain.PublishDocCrossDocSummary{}, marshalErr
			}
			pkg.RenderRevision = domain.PublishDocRenderRevision
			pkg.NativePlan = planBody
			pkg.RenderWarnings = warnings
		}
	}
	return pkg, crossDocSummary, nil
}

// publishDocMigrationGuard 判断是否需要在 push 前强制迁移。
// 规则：profile 已切到 native-docx，但 active mapping 仍指向 Drive file（markdown-file）时，
// 不允许原地改义旧 file 对象，必须先 unlink 再以 native-docx 重新发布。
// 返回 nil 表示无迁移阻断，可继续后续校验/推送。
func publishDocMigrationGuard(profile domain.PublishDocProfile, mapping domain.PublishDocMapping, noteID string) *domain.CommandError {
	if profile.Target != domain.PublishDocTargetLarkDoc {
		return nil
	}
	if profile.ResolveDocRenderer() != domain.PublishDocRendererNativeDocx {
		return nil
	}
	if mapping.ExternalObject.ID == "" {
		return nil
	}
	if mapping.ResolveDocMappingRenderer() == domain.PublishDocRendererMarkdownFile || mapping.ExternalObject.Type == domain.PublishDocObjectTypeFile {
		return &domain.CommandError{Code: "publish_object_type_migration_required", Message: "Existing mapping points to a Drive file; switch to native document requires a new object", Hint: "Run pinax publish doc unlink --note " + shellQuote(noteID) + " --target lark-doc --vault <vault> --json then re-publish with --renderer native-docx"}
	}
	return nil
}

func validatePublishDocPackageForProfile(pkg domain.PublishDocPackage, profile domain.PublishDocProfile) *domain.CommandError {
	if pkg.Target != profile.Target {
		return &domain.CommandError{Code: "publish_package_target_mismatch", Message: "Publish package target does not match the selected profile", Hint: "Run pinax publish doc prepare again with the same --target used for push"}
	}
	if profile.Target != domain.PublishDocTargetLarkDoc {
		return nil
	}
	got := pkg.Renderer
	if got == "" {
		got = domain.PublishDocRendererMarkdownFile
	}
	expected := profile.ResolveDocRenderer()
	if got != expected {
		return &domain.CommandError{Code: "publish_package_renderer_mismatch", Message: "Publish package renderer does not match the current profile", Hint: "Run pinax publish doc prepare again after changing --renderer"}
	}
	return nil
}

func publishDocAddCrossDocFacts(projection *domain.Projection, summary domain.PublishDocCrossDocSummary) {
	if summary.Total == 0 {
		return
	}
	projection.Facts["cross_doc_total"] = fmt.Sprint(summary.Total)
	projection.Facts["cross_doc_links"] = fmt.Sprint(summary.Rewritten)
	projection.Facts["cross_doc_unpublished"] = fmt.Sprint(summary.Unpublished)
	projection.Facts["cross_doc_ambiguous"] = fmt.Sprint(summary.Ambiguous)
	projection.Facts["cross_doc_broken"] = fmt.Sprint(summary.Broken)
	projection.Facts["cross_doc_self"] = fmt.Sprint(summary.Self)
}

func publishDocMergeCrossDocSummary(dst *domain.PublishDocCrossDocSummary, src domain.PublishDocCrossDocSummary) {
	dst.Total += src.Total
	dst.Rewritten += src.Rewritten
	dst.Unpublished += src.Unpublished
	dst.Ambiguous += src.Ambiguous
	dst.Broken += src.Broken
	dst.Self += src.Self
	dst.Links = append(dst.Links, src.Links...)
}
