package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/provider"
	"github.com/yeisme/pinax/internal/publishdocast"
)

// whiteboardTagRe 匹配 lark-cli docs +fetch XML 输出里的 whiteboard 开标签。
// 形如 <whiteboard id="..." token="UcBvw..."></whiteboard>。
var whiteboardTagRe = regexp.MustCompile(`<whiteboard\s[^>]*>`)
var assetBlockIDAttrRe = regexp.MustCompile(`\bid="([^"]+)"`)
var whiteboardTokenAttrRe = regexp.MustCompile(`\stoken="([^"]+)"`)

// publishDocBuildPlan 把 note 正文 + vault template 输出解析为 provider-neutral native plan。
// 返回 plan 和 domain 层 render warnings（写入 package 和 mapping）。
// 复杂转换逻辑（AST、资产解析）委托给 publishdocast 包；app 层只负责 note → body 编排。
func publishDocBuildPlan(note domain.Note, profile domain.PublishDocProfile) (publishdocast.NativePlan, []domain.PublishDocRenderWarning) {
	return publishDocBuildPlanFromMarkdown(note, publishDocRenderedMarkdown(note, profile))
}

func publishDocBuildPlanFromMarkdown(note domain.Note, body string) (publishdocast.NativePlan, []domain.PublishDocRenderWarning) {
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = note.ID
	}
	doc := publishdocast.Parse(title, body)
	// vault template 在正文首行写了 # {title}；native plan 的 Title 字段已承载文档标题，
	// 正文第一屏不重复同名 H1，避免飞书原生文档标题与首块重复。
	doc.Blocks = publishDocStripDuplicateTitleHeading(doc.Blocks, title)
	opts := publishdocast.AssetOptions{NoteDir: publishDocNoteDir(note)}
	plan := publishdocast.BuildNativePlan(doc, opts)
	warnings := make([]domain.PublishDocRenderWarning, 0, len(plan.Warnings))
	for _, w := range plan.Warnings {
		warnings = append(warnings, domain.PublishDocRenderWarning{Code: w.Code, Message: w.Message, Detail: w.Detail})
	}
	return plan, warnings
}

// publishDocStripDuplicateTitleHeading 移除正文首个与文档标题同名的 H1 块。
// 仅去重首块，保留正文后续合法 H1；中文全角标点差异不做模糊匹配，要求精确同名。
func publishDocStripDuplicateTitleHeading(blocks []publishdocast.Block, title string) []publishdocast.Block {
	if len(blocks) == 0 || strings.TrimSpace(title) == "" {
		return blocks
	}
	first := blocks[0]
	if first.Type != publishdocast.BlockHeading || first.Level != 1 {
		return blocks
	}
	if strings.TrimSpace(first.Text) == strings.TrimSpace(title) {
		return blocks[1:]
	}
	return blocks
}

// publishDocNoteDir 返回 note 在 vault 中的相对目录，用于解析相对图片/附件。
func publishDocNoteDir(note domain.Note) string {
	dir := filepath.ToSlash(filepath.Dir(strings.TrimSpace(note.Path)))
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

// publishDocExternalObjectType 根据 renderer 和 provider 返回类型决定 external_object.type。
// native-docx 优先用 provider inspect 返回的 docx/doc；markdown-file 为 file。
func publishDocExternalObjectType(profile domain.PublishDocProfile, result publishDocProviderResult) string {
	if profile.Target == domain.PublishDocTargetLarkDoc {
		if profile.ResolveDocRenderer() == domain.PublishDocRendererMarkdownFile {
			return domain.PublishDocObjectTypeFile
		}
		// native-docx：优先用 provider 返回的具体类型（docx/doc），否则默认 docx。
		if result.ObjectType != "" {
			return result.ObjectType
		}
		return domain.PublishDocObjectTypeDocx
	}
	return domain.PublishDocObjectTypePage
}

// publishDocNativeProviderCreate 通过 lark-cli docs +create 创建原生文档。
// 把渲染后的 Markdown 正文写入临时文件，交给 lark-cli docs +create --doc-format markdown。
// lark-cli 负责把 Markdown 转为飞书原生 docx 块（而非上传 .md file）。
// 创建后执行资产插入管线：Mermaid/SVG 服务端渲染为原生白板，本地图片 media-insert。
// 不允许静默降级到 markdown +create：若 lark-cli 缺少原生文档能力，返回 provider_capability_missing。
// 返回 asset warnings：单个资产渲染失败产生 warning，不阻塞发布。
func publishDocNativeProviderCreate(ctx context.Context, root string, profile domain.PublishDocProfile, pkg domain.PublishDocPackage, note domain.Note, folderToken string) (publishDocProviderResult, []domain.PublishDocRenderWarning, []string, *domain.CommandError) {
	adapter := publishDocNativeAdapter(profile)
	if err := publishDocNativeCheckCapabilityWithAdapter(ctx, adapter); err != nil {
		return publishDocProviderResult{}, nil, nil, err
	}
	contentPath, cleanup, writeErr := publishDocWriteNativeContent(note, pkg)
	if writeErr != nil {
		return publishDocProviderResult{}, nil, nil, writeErr
	}
	defer cleanup()
	result, err := adapter.CreateNativeDoc(ctx, provider.NativeDocCreateInput{Title: publishDocNativeFileName(note), FolderToken: folderToken, ContentPath: contentPath})
	if cmdErr := publishDocNativeCommandError(err); cmdErr != nil {
		return publishDocProviderResult{}, nil, nil, cmdErr
	}
	parsed := publishDocNativeProviderResult(result)
	if parsed.ObjectType == "" {
		parsed.ObjectType = publishDocNativeInspectType(ctx, adapter, parsed.URL, parsed.ID)
	}
	assetWarnings, assetBlocks := publishDocNativeInsertAssets(ctx, root, profile, parsed.ID, pkg, nil)
	return parsed, assetWarnings, assetBlocks, nil
}

// publishDocNativeProviderUpdate 通过 lark-cli docs +update --command overwrite 更新已有原生文档。
// 更新后同样执行资产插入管线。
func publishDocNativeProviderUpdate(ctx context.Context, root string, profile domain.PublishDocProfile, mapping domain.PublishDocMapping, pkg domain.PublishDocPackage, note domain.Note) (publishDocProviderResult, []domain.PublishDocRenderWarning, []string, *domain.CommandError) {
	adapter := publishDocNativeAdapter(profile)
	if err := publishDocNativeCheckCapabilityWithAdapter(ctx, adapter); err != nil {
		return publishDocProviderResult{}, nil, nil, err
	}
	contentPath, cleanup, writeErr := publishDocWriteNativeContent(note, pkg)
	if writeErr != nil {
		return publishDocProviderResult{}, nil, nil, writeErr
	}
	defer cleanup()
	docToken := mapping.ExternalObject.ID
	result, err := adapter.UpdateNativeDoc(ctx, provider.NativeDocUpdateInput{DocToken: docToken, ExistingURL: mapping.ExternalObject.URL, ContentPath: contentPath})
	if cmdErr := publishDocNativeCommandError(err); cmdErr != nil {
		return publishDocProviderResult{}, nil, nil, cmdErr
	}
	parsed := publishDocNativeProviderResult(result)
	if parsed.ID == "" {
		parsed.ID = docToken
	}
	if parsed.URL == "" {
		parsed.URL = mapping.ExternalObject.URL
	}
	if parsed.ObjectType == "" {
		parsed.ObjectType = publishDocNativeInspectType(ctx, adapter, parsed.URL, parsed.ID)
	}
	assetWarnings, assetBlocks := publishDocNativeInsertAssets(ctx, root, profile, parsed.ID, pkg, mapping.RenderAssetBlocks)
	return parsed, assetWarnings, assetBlocks, nil
}

type publishDocLarkNativeRunner struct{}

func (publishDocLarkNativeRunner) Run(ctx context.Context, cmd provider.LarkNativeCLICommand) ([]byte, error) {
	out, err := runPublishDocCLIDir(ctx, cmd.Dir, cmd.Name, cmd.Args, cmd.Stdin)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func publishDocNativeAdapter(profile domain.PublishDocProfile) *provider.LarkNativeDocCLIAdapter {
	return provider.NewLarkNativeDocCLIAdapter(profile, publishDocLarkNativeRunner{})
}

func publishDocNativeProviderResult(result provider.NativeDocResult) publishDocProviderResult {
	return publishDocProviderResult{ID: result.Token, URL: result.URL, ObjectType: result.Type}
}

func publishDocNativeCommandError(err error) *domain.CommandError {
	if err == nil {
		return nil
	}
	var cmdErr *domain.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr
	}
	if errors.Is(err, provider.ErrCapabilityMissing) {
		return &domain.CommandError{Code: provider.CapabilityErrorCode, Message: "Provider does not support native document creation", Hint: "Upgrade lark-cli to a version with the docs domain, or use --renderer markdown-file"}
	}
	return &domain.CommandError{Code: "provider_preflight_failed", Message: "Provider CLI failed", Hint: publishDocProviderSafeHint("provider_preflight_failed")}
}

// publishDocNativeInsertAssets 在文档创建/更新后插入 Mermaid/SVG/图片资产。
// Mermaid/SVG：append 空白 whiteboard → fetch 拿 token → whiteboard +update 服务端渲染。
// 本地图片：docs +media-insert --file <vault-relative>（cwd 设为 vault root 满足 lark-cli 安全限制）。
// 每个资产独立失败：失败产生 warning，不阻塞其他资产或整体发布。
func publishDocNativeInsertAssets(ctx context.Context, root string, profile domain.PublishDocProfile, docToken string, pkg domain.PublishDocPackage, previousBlockIDs []string) ([]domain.PublishDocRenderWarning, []string) {
	if strings.TrimSpace(docToken) == "" || len(pkg.NativePlan) == 0 {
		return nil, nil
	}
	plan, err := publishdocast.UnmarshalNativePlan(pkg.NativePlan)
	if err != nil {
		return []domain.PublishDocRenderWarning{{Code: "native_plan_unparseable", Message: "Native plan could not be parsed; assets skipped", Detail: err.Error()}}, nil
	}
	var warnings []domain.PublishDocRenderWarning
	var assetBlocks []string
	// 更新场景下 overwrite 不清理之前 append 的 whiteboard/img 块，会导致重复推送累积资产。
	// 只清理上次 mapping manifest 记录的 Pinax 资产块，避免误删用户在云端手工添加的图片/附件。
	publishDocClearAssetBlocks(ctx, profile, docToken, previousBlockIDs)
	for _, asset := range plan.Assets {
		switch asset.Kind {
		case "svg":
			blockID, w := publishDocInsertWhiteboardAsset(ctx, profile, docToken, "svg", asset.Source)
			if w != nil {
				warnings = append(warnings, *w)
			}
			if blockID != "" {
				assetBlocks = append(assetBlocks, blockID)
			}
		case "svg-file":
			source, readErr := publishDocReadVaultAsset(root, asset.Source)
			if readErr != nil {
				warnings = append(warnings, domain.PublishDocRenderWarning{Code: domain.PublishDocWarningSVGUnavailable, Message: "SVG file could not be read for rendering", Detail: asset.Source})
				continue
			}
			blockID, w := publishDocInsertWhiteboardAsset(ctx, profile, docToken, "svg", source)
			if w != nil {
				warnings = append(warnings, *w)
			}
			if blockID != "" {
				assetBlocks = append(assetBlocks, blockID)
			}
		case "remote-svg":
			source, dlErr := publishDocDownloadRemoteAsset(ctx, asset.Source)
			if dlErr != nil {
				warnings = append(warnings, domain.PublishDocRenderWarning{Code: "remote_image_download_failed", Message: "Remote SVG could not be downloaded for rendering", Detail: firstLineSafe(asset.Source)})
				continue
			}
			blockID, w := publishDocInsertWhiteboardAsset(ctx, profile, docToken, "svg", source)
			if w != nil {
				warnings = append(warnings, *w)
			}
			if blockID != "" {
				assetBlocks = append(assetBlocks, blockID)
			}
		case "remote-image":
			relPath, dlErr := publishDocDownloadRemoteAssetToFile(ctx, root, asset.Source)
			if dlErr != nil {
				warnings = append(warnings, domain.PublishDocRenderWarning{Code: "remote_image_download_failed", Message: "Remote image could not be downloaded for upload", Detail: firstLineSafe(asset.Source)})
				continue
			}
			blockID, w := publishDocInsertImageAsset(ctx, root, profile, docToken, relPath, asset.Alt)
			if w != nil {
				warnings = append(warnings, *w)
			}
			if blockID != "" {
				assetBlocks = append(assetBlocks, blockID)
			}
		case "image":
			blockID, w := publishDocInsertImageAsset(ctx, root, profile, docToken, asset.Source, asset.Alt)
			if w != nil {
				warnings = append(warnings, *w)
			}
			if blockID != "" {
				assetBlocks = append(assetBlocks, blockID)
			}
		case "attachment":
			blockID, w := publishDocInsertAttachmentAsset(ctx, root, profile, docToken, asset.Source, asset.Alt)
			if w != nil {
				warnings = append(warnings, *w)
			}
			if blockID != "" {
				assetBlocks = append(assetBlocks, blockID)
			}
		}
	}
	return warnings, assetBlocks
}

// publishDocInsertWhiteboardAsset 把 Mermaid/SVG 源码渲染为原生白板并插入文档。
// 三步：(1) append 空白 whiteboard (2) fetch 拿 whiteboard token (3) whiteboard +update 服务端渲染。
func publishDocInsertWhiteboardAsset(ctx context.Context, profile domain.PublishDocProfile, docToken, inputFormat, source string) (string, *domain.PublishDocRenderWarning) {
	if strings.TrimSpace(source) == "" {
		return "", nil
	}
	if _, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "docs", "+update", "--doc", docToken, "--command", "append", "--content", "<whiteboard type=\"blank\" data-pinax-managed=\"true\"></whiteboard>", "--json"), ""); err != nil {
		return "", &domain.PublishDocRenderWarning{Code: warnCodeForFormat(inputFormat), Message: inputFormat + " diagram could not be inserted (append whiteboard failed)", Detail: firstLineSafe(source)}
	}
	ref := publishDocFetchLastWhiteboardRefRetry(ctx, profile, docToken)
	if ref.Token == "" {
		return "", &domain.PublishDocRenderWarning{Code: warnCodeForFormat(inputFormat), Message: inputFormat + " diagram inserted but token could not be resolved", Detail: firstLineSafe(source)}
	}
	sourcePath, cleanup, writeErr := publishDocWriteAssetSource(ref.Token, source)
	if writeErr != nil {
		return "", &domain.PublishDocRenderWarning{Code: warnCodeForFormat(inputFormat), Message: inputFormat + " diagram source could not be staged", Detail: firstLineSafe(source)}
	}
	defer cleanup()
	if _, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "whiteboard", "+update", "--whiteboard-token", ref.Token, "--input_format", inputFormat, "--source", "@"+sourcePath, "--overwrite", "--json"), ""); err != nil {
		return "", &domain.PublishDocRenderWarning{Code: warnCodeForFormat(inputFormat), Message: inputFormat + " diagram server-side render failed", Detail: firstLineSafe(source)}
	}
	return ref.ID, nil
}

// publishDocInsertImageAsset 用 docs +media-insert 上传本地图片到文档。
// lark-cli 要求 --file 是 cwd 内相对路径，故 cmd.Dir 设为 vault root。
// alt 作为 caption；PNG 自动读取像素宽高传 --width/--height（其他格式回退自动宽高比）。
func publishDocInsertImageAsset(ctx context.Context, root string, profile domain.PublishDocProfile, docToken, relPath, alt string) (string, *domain.PublishDocRenderWarning) {
	relPath = strings.TrimSpace(filepath.ToSlash(relPath))
	if relPath == "" {
		return "", nil
	}
	input := provider.MediaUploadInput{Root: root, DocToken: docToken, Path: relPath, MediaType: "image", Caption: alt}
	if w, h := publishDocReadImageDimensions(root, relPath); w > 0 && h > 0 {
		const maxWidth = 800
		if w > maxWidth {
			input.Width = maxWidth
		}
	}
	blockID, err := publishDocNativeAdapter(profile).UploadMedia(ctx, input)
	if err != nil {
		return "", &domain.PublishDocRenderWarning{Code: "image_upload_failed", Message: "Local image upload failed", Detail: relPath}
	}
	return blockID, nil
}

// publishDocClearAssetBlocks 删除文档中已有的 Pinax 资产块（whiteboard + img）。
// 保证重复 push（update）时资产不累积：overwrite 只清正文，不清 append 的资产块。
// 清理失败不阻塞发布（最坏情况是旧资产残留，不影响新资产插入）。
func publishDocClearAssetBlocks(ctx context.Context, profile domain.PublishDocProfile, docToken string, previousBlockIDs []string) {
	blockIDs := publishDocManagedAssetBlockIDs("", previousBlockIDs)
	if len(blockIDs) == 0 {
		return
	}
	combined := strings.Join(blockIDs, ",")
	_, _ = runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "docs", "+update", "--doc", docToken, "--command", "block_delete", "--block-id", combined, "--json"), "")
}

// publishDocManagedAssetBlockIDs returns only Pinax-owned block IDs recorded in mapping manifest.
// It intentionally ignores unmarked blocks fetched from the remote document to avoid deleting user-added content.
func publishDocManagedAssetBlockIDs(_ string, manifest []string) []string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(manifest))
	for _, id := range manifest {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// publishDocFetchLastWhiteboardRefRetry 重试 fetch 直到拿到 whiteboard token 或耗尽重试。
// Feishu 文档在连续 append/delete 后有短暂一致性延迟，重试保证读到最新 whiteboard。
type publishDocWhiteboardRef struct {
	ID    string
	Token string
}

func publishDocFetchLastWhiteboardRefRetry(ctx context.Context, profile domain.PublishDocProfile, docToken string) publishDocWhiteboardRef {
	delays := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}
	for _, delay := range delays {
		ref := publishDocFetchLastWhiteboardRef(ctx, profile, docToken)
		if ref.Token != "" {
			return ref
		}
		select {
		case <-ctx.Done():
			return publishDocWhiteboardRef{}
		case <-time.After(delay):
		}
	}
	return publishDocFetchLastWhiteboardRef(ctx, profile, docToken)
}

// publishDocFetchLastWhiteboardToken fetch 文档 with-ids，提取最后一个 whiteboard 的 token。
func publishDocFetchLastWhiteboardRef(ctx context.Context, profile domain.PublishDocProfile, docToken string) publishDocWhiteboardRef {
	out, err := runPublishDocCLI(ctx, "lark-cli", publishDocLarkArgs(profile, "docs", "+fetch", "--doc", docToken, "--detail", "with-ids", "--scope", "full", "--doc-format", "xml", "--json"), "")
	if err != nil {
		return publishDocWhiteboardRef{}
	}
	var payload map[string]any
	if err := json.Unmarshal(provider.ExtractJSON(out), &payload); err != nil {
		return publishDocWhiteboardRef{}
	}
	data, _ := payload["data"].(map[string]any)
	doc, _ := data["document"].(map[string]any)
	content, _ := doc["content"].(string)
	// 提取最后一个 whiteboard 的 id/token：token="..." 出现在 <whiteboard ...> 标签里。
	last := publishDocWhiteboardRef{}
	for _, tag := range whiteboardTagRe.FindAllString(content, -1) {
		if idm := assetBlockIDAttrRe.FindStringSubmatch(tag); len(idm) > 1 {
			last.ID = idm[1]
		}
		if tm := whiteboardTokenAttrRe.FindStringSubmatch(tag); len(tm) > 1 {
			last.Token = tm[1]
		}
	}
	return last
}

// publishDocReadVaultAsset 读取 vault 内相对路径资产（svg 文件源码）。
func publishDocReadVaultAsset(root, relPath string) (string, error) {
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	body, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// publishDocWriteAssetSource 把 mermaid/svg 源码写入临时文件，供 --source @file 使用。
func publishDocWriteAssetSource(wbToken, source string) (string, func(), *domain.CommandError) {
	dir, err := os.MkdirTemp(".", ".pinax-publish-asset-*")
	if err != nil {
		return "", func() {}, &domain.CommandError{Code: "publish_temp_file_failed", Message: "Failed to create temporary asset directory", Hint: "Check current directory permissions"}
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, sanitizePublishDocID(wbToken)+".txt")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		cleanup()
		return "", func() {}, &domain.CommandError{Code: "publish_temp_file_failed", Message: "Failed to write temporary asset source", Hint: "Check current directory permissions"}
	}
	return filepath.ToSlash(path), cleanup, nil
}

func warnCodeForFormat(inputFormat string) string {
	if inputFormat == "mermaid" {
		return domain.PublishDocWarningMermaidUnavailable
	}
	return domain.PublishDocWarningSVGUnavailable
}

func firstLineSafe(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		if idx > 80 {
			return s[:80]
		}
		return s[:idx]
	}
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

// publishDocNativeCheckCapability 探测 lark-cli 是否支持原生文档创建。
// 用 docs +create --dry-run 验证命令可用且身份就绪；缺能力时返回 provider_capability_missing，
// app 层不得静默降级到 markdown-file。
func publishDocNativeCheckCapability(ctx context.Context, profile domain.PublishDocProfile) *domain.CommandError {
	return publishDocNativeCheckCapabilityWithAdapter(ctx, publishDocNativeAdapter(profile))
}

func publishDocNativeCheckCapabilityWithAdapter(ctx context.Context, adapter provider.LarkNativeDocAdapter) *domain.CommandError {
	return publishDocNativeCommandError(adapter.CheckCapability(ctx))
}

// publishDocNativeInspectType 用 lark-cli drive +inspect 查询远端对象类型（docx/doc/file）。
func publishDocNativeInspectType(ctx context.Context, adapter provider.LarkNativeDocAdapter, url, docToken string) string {
	target := url
	if target == "" && docToken != "" {
		target = docToken
	}
	objType, err := adapter.InspectObjectType(ctx, target)
	if err != nil {
		return ""
	}
	return objType
}

// publishDocWriteNativeContent 把渲染后的 Markdown 正文写入临时文件，返回路径和 cleanup。
// lark-cli docs +create 的 --content 支持 @file 前缀读取文件内容。
func publishDocWriteNativeContent(note domain.Note, pkg domain.PublishDocPackage) (string, func(), *domain.CommandError) {
	dir, err := os.MkdirTemp(".", ".pinax-publish-doc-native-*")
	if err != nil {
		return "", func() {}, &domain.CommandError{Code: "publish_temp_file_failed", Message: "Failed to create temporary content directory", Hint: "Check current directory permissions"}
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	body := strings.TrimSpace(pkg.BodyMarkdown)
	if body == "" {
		body = publishDocMarkdown(note)
	}
	path := filepath.Join(dir, sanitizePublishDocID(note.ID)+"_content.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		cleanup()
		return "", func() {}, &domain.CommandError{Code: "publish_temp_file_failed", Message: "Failed to write temporary native content", Hint: "Check current directory permissions"}
	}
	return filepath.ToSlash(path), cleanup, nil
}

func publishDocNativeFileName(note domain.Note) string {
	name := strings.TrimSpace(note.Title)
	if name == "" {
		name = note.ID
	}
	return name
}
