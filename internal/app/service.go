package app

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/app/noteops"
	"github.com/yeisme/pinax/internal/app/searchops"
	"github.com/yeisme/pinax/internal/app/templateops"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	gitstore "github.com/yeisme/pinax/internal/git"
	"github.com/yeisme/pinax/internal/identity"
	noteindex "github.com/yeisme/pinax/internal/index"
	"github.com/yeisme/pinax/internal/markdownnote"
	notesearch "github.com/yeisme/pinax/internal/search"
	"github.com/yeisme/pinax/internal/templateengine"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

type Service struct {
	versionBackend    pinaxversion.VersionBackend
	identityAllocator *identity.Allocator
}

func NewService() *Service { return NewServiceWithVersionBackend(pinaxversion.NewLocalBackend()) }

func NewServiceWithVersionBackend(backend pinaxversion.VersionBackend) *Service {
	if backend == nil {
		backend = pinaxversion.NewLocalBackend()
	}
	return &Service{versionBackend: backend, identityAllocator: identity.NewAllocator()}
}

func (s *Service) allocateObjectID(kind identity.ObjectKind, root, locator string) (string, error) {
	if !kind.Valid() {
		return "", fmt.Errorf("invalid object kind %q", kind)
	}
	if s.identityAllocator == nil {
		s.identityAllocator = identity.NewAllocator()
	}
	id, err := s.identityAllocator.Allocate(string(kind) + ":create:" + root + ":" + filepath.ToSlash(locator))
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func currentTimeUTC() time.Time {
	value := strings.TrimSpace(os.Getenv("PINAX_TEST_NOW"))
	if value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

type noteLinkGraph struct {
	notes    []domain.Note
	outgoing map[string][]domain.NoteLink
	incoming map[string][]domain.NoteLink
}

func buildNoteLinkGraph(root string) (noteLinkGraph, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return noteLinkGraph{}, err
	}
	byTitle := map[string]domain.Note{}
	byPath := map[string]domain.Note{}
	for _, note := range notes {
		byTitle[strings.ToLower(note.Title)] = note
		byPath[note.Path] = note
	}
	graph := noteLinkGraph{notes: notes, outgoing: map[string][]domain.NoteLink{}, incoming: map[string][]domain.NoteLink{}}
	for _, note := range notes {
		for _, link := range noteGraphLinks(note, byTitle, byPath) {
			graph.outgoing[note.Path] = append(graph.outgoing[note.Path], link)
			if link.TargetPath != "" && !link.Broken {
				graph.incoming[link.TargetPath] = append(graph.incoming[link.TargetPath], link)
			}
		}
	}
	for path := range graph.outgoing {
		sortNoteLinks(graph.outgoing[path])
	}
	for path := range graph.incoming {
		sortNoteLinks(graph.incoming[path])
	}
	return graph, nil
}

func noteGraphLinks(note domain.Note, byTitle map[string]domain.Note, byPath map[string]domain.Note) []domain.NoteLink {
	links := make([]domain.NoteLink, 0)
	seen := map[string]bool{}
	for _, rawTarget := range wikiLinksInBody(note.Body) {
		target := normalizeWikiLinkTarget(rawTarget)
		if target == "" {
			continue
		}
		resolved := byTitle[strings.ToLower(target)]
		link := domain.NoteLink{SourcePath: note.Path, SourceTitle: note.Title, Target: target, Kind: "wiki", Broken: resolved.Path == ""}
		if resolved.Path != "" {
			link.TargetPath = resolved.Path
			link.TargetTitle = resolved.Title
		}
		key := link.Kind + "\x00" + link.Target
		if !seen[key] {
			links = append(links, link)
			seen[key] = true
		}
	}
	for _, rawTarget := range markdownLinksInBody(note.Body) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || !strings.EqualFold(filepath.Ext(target), ".md") {
			continue
		}
		targetPath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(note.Path), target)))
		resolved := byPath[targetPath]
		link := domain.NoteLink{SourcePath: note.Path, SourceTitle: note.Title, Target: target, TargetPath: targetPath, Kind: "markdown", Broken: resolved.Path == ""}
		if resolved.Path != "" {
			link.TargetTitle = resolved.Title
		}
		key := link.Kind + "\x00" + link.TargetPath
		if !seen[key] {
			links = append(links, link)
			seen[key] = true
		}
	}
	return links
}

func markdownLinksInBody(body string) []string {
	links := make([]string, 0)
	seen := map[string]bool{}
	for _, match := range vaultMarkdownLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		links = append(links, target)
	}
	sort.Strings(links)
	return links
}

func normalizeWikiLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if before, _, ok := strings.Cut(target, "|"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func normalizeMarkdownLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
		return ""
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func sortNoteLinks(links []domain.NoteLink) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].SourcePath == links[j].SourcePath {
			return links[i].Target < links[j].Target
		}
		return links[i].SourcePath < links[j].SourcePath
	})
}

func countResolvedLinks(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if !link.Broken {
			count++
		}
	}
	return count
}

func countBrokenLinks(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if link.Broken {
			count++
		}
	}
	return count
}

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

func copyFile(source, target string) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, b, 0o644)
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

func planMarkdownImport(root, source string, req ImportMarkdownRequest) ([]domain.ImportPlan, error) {
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &domain.CommandError{Code: "import_source_missing", Message: "Import source does not exist", Hint: "Check the Markdown file or directory path"}
	}
	if err != nil {
		return nil, err
	}
	sources := []string{}
	if info.IsDir() {
		if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".md") {
				sources = append(sources, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	} else if strings.EqualFold(filepath.Ext(source), ".md") {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	plans := make([]domain.ImportPlan, 0, len(sources))
	used := map[string]bool{}
	for _, item := range sources {
		targetRel, err := importTargetRel(source, item, info.IsDir(), req)
		if err != nil {
			return nil, err
		}
		plan := domain.ImportPlan{SourcePath: item, TargetPath: targetRel, Status: "write"}
		if used[targetRel] || fileExistsPath(root, targetRel) {
			plan.Conflict = "exists"
			switch strings.TrimSpace(req.Conflict) {
			case "rename":
				plan.TargetPath, err = uniqueImportRel(root, targetRel, used)
				if err != nil {
					return nil, err
				}
				plan.Status = "rename"
			case "overwrite":
				plan.Status = "overwrite"
			case "skip", "":
				plan.Status = "skip"
			default:
				return nil, &domain.CommandError{Code: "invalid_import_conflict", Message: "Unknown import conflict policy", Hint: "Use --conflict skip, rename, or overwrite"}
			}
		}
		used[plan.TargetPath] = true
		plans = append(plans, plan)
	}
	return plans, nil
}

func importTargetRel(sourceRoot, sourceFile string, sourceIsDir bool, req ImportMarkdownRequest) (string, error) {
	name := filepath.Base(sourceFile)
	if sourceIsDir {
		rel, err := filepath.Rel(sourceRoot, sourceFile)
		if err != nil {
			return "", err
		}
		name = filepath.ToSlash(rel)
	}
	base := "notes"
	if strings.TrimSpace(req.Group) != "" {
		base = filepath.ToSlash(filepath.Join(base, strings.TrimSpace(req.Group)))
	}
	if strings.TrimSpace(req.Folder) != "" {
		folder, err := validateOptionalNoteFolder(req.Folder)
		if err != nil {
			return "", err
		}
		base = filepath.ToSlash(filepath.Join(base, folder))
	}
	return validateNoteDir(filepath.ToSlash(filepath.Join(base, name)))
}

func uniqueImportRel(root, targetRel string, used map[string]bool) (string, error) {
	dir := filepath.Dir(targetRel)
	base := filepath.Base(targetRel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	ext := filepath.Ext(base)
	for i := 2; i < 1000; i++ {
		candidate := filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext)))
		if !used[candidate] && !fileExistsPath(root, candidate) {
			return candidate, nil
		}
	}
	return "", &domain.CommandError{Code: "import_name_conflict", Message: "Too many import filename conflicts", Hint: "Choose another target group or filename, then retry"}
}

func fileExistsPath(root, rel string) bool {
	path, err := safeJoin(root, rel)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func countImportPlans(plans []domain.ImportPlan, status string) int {
	count := 0
	for _, plan := range plans {
		if plan.Status == status {
			count++
		}
	}
	return count
}

func writeReceipt(root, kind string, payload map[string]any) (string, error) {
	now := time.Now().UTC()
	rel := filepath.ToSlash(filepath.Join(".pinax", "receipts", fmt.Sprintf("%s-%s.json", kind, now.Format("20060102T150405Z"))))
	payload["schema_version"] = "pinax.receipt.v1"
	payload["kind"] = kind
	payload["created_at"] = now.Format(time.RFC3339)
	return rel, writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), payload)
}

func copyVaultFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, b, 0o644)
}

func (s *Service) ListNotes(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	return s.ListNotesQuery(ctx, NoteListRequest{VaultPath: req.VaultPath})
}

func (s *Service) ListNotesQuery(_ context.Context, req NoteListRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.list", err), err
	}
	if periodUpdatedAfter, err := noteListPeriodUpdatedAfter(req.Period, req.UpdatedAfter); err != nil {
		return errorProjection("note.list", err), err
	} else if periodUpdatedAfter != "" {
		req.UpdatedAfter = periodUpdatedAfter
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("note.list", err), err
	}
	facts = ordinaryNoteFacts(facts)
	// 默认排除 discarded 笔记，除非显式请求
	if req.Status != "discarded" {
		kept := make([]noteFact, 0, len(facts))
		for _, f := range facts {
			if f.note.Status != "discarded" {
				kept = append(kept, f)
			}
		}
		facts = kept
	}
	notes := make([]domain.Note, 0, len(facts))
	for _, fact := range facts {
		note := fact.note
		if !noteMatchesQuery(note, req) {
			continue
		}
		notes = append(notes, note)
	}
	sortNotes(notes, req)
	total := len(notes)
	if req.Limit > 0 && len(notes) > req.Limit {
		notes = notes[:req.Limit]
	}
	projection := domain.NewProjection("note.list", "Local notes listed.")
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["count"] = fmt.Sprint(len(notes))
	projection.Facts["total"] = fmt.Sprint(total)
	projection.Facts["returned"] = fmt.Sprint(len(notes))
	if req.Recent {
		projection.Facts["recent"] = "true"
	}
	if req.Recent || req.Sort == "" || req.Sort == "updated" {
		projection.Facts["sort"] = "updated"
	} else {
		projection.Facts["sort"] = req.Sort
	}
	if len(req.Tags) > 0 {
		projection.Facts["filter.tag"] = strings.Join(req.Tags, ",")
	}
	if req.Project != "" {
		projection.Facts["filter.project"] = req.Project
	}
	if req.Group != "" {
		projection.Facts["filter.group"] = req.Group
	}
	if req.Folder != "" {
		projection.Facts["filter.folder"] = req.Folder
	}
	if req.Kind != "" {
		projection.Facts["filter.kind"] = req.Kind
	}
	if req.Status != "" {
		projection.Facts["filter.status"] = req.Status
	}
	if req.CreatedAfter != "" {
		projection.Facts["filter.created_after"] = req.CreatedAfter
	}
	if req.UpdatedAfter != "" {
		projection.Facts["filter.updated_after"] = req.UpdatedAfter
	}
	if req.UpdatedBefore != "" {
		projection.Facts["filter.updated_before"] = req.UpdatedBefore
	}
	if req.Period != "" {
		projection.Facts["filter.period"] = req.Period
	}
	if req.PathPrefix != "" {
		projection.Facts["filter.path_prefix"] = req.PathPrefix
	}
	properties, err := selectedNoteProperties(notes, req.Properties, req.StrictProperties)
	if err != nil {
		return domain.NewErrorProjection("note.list", err.(*domain.CommandError)), err
	}
	data := map[string]any{"notes": notes, "filters": req, "total": total, "returned": len(notes)}
	if len(req.Properties) > 0 {
		projection.Facts["properties"] = strings.Join(req.Properties, ",")
		data["properties"] = properties
	}
	projection.Data = data
	return projection, nil
}

func noteListPeriodUpdatedAfter(period, explicitUpdatedAfter string) (string, error) {
	period = strings.TrimSpace(period)
	if period == "" {
		return strings.TrimSpace(explicitUpdatedAfter), nil
	}
	if strings.TrimSpace(explicitUpdatedAfter) != "" {
		return "", &domain.CommandError{Code: "note_list_period_conflict", Message: "--period and --updated-after cannot be used together", Hint: "Use either pinax note list --period daily or pinax note list --updated-after <date>"}
	}
	now := currentTimeUTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var boundary time.Time
	switch period {
	case "5h":
		boundary = now.Add(-5 * time.Hour)
	case "daily", "day", "today":
		boundary = dayStart
	case "weekly", "week", "this-week":
		weekdayOffset := (int(now.Weekday()) + 6) % 7
		boundary = dayStart.AddDate(0, 0, -weekdayOffset)
	case "monthly", "month", "this-month":
		boundary = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return "", &domain.CommandError{Code: "note_list_period_invalid", Message: "Invalid note list period: " + period, Hint: "Use one of: 5h, daily, weekly, monthly"}
	}
	return boundary.Format(time.RFC3339), nil
}

func selectedNoteProperties(notes []domain.Note, names []string, strict bool) (map[string]map[string]domain.PropertyValue, error) {
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			cleaned = append(cleaned, name)
		}
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	found := map[string]bool{}
	out := map[string]map[string]domain.PropertyValue{}
	for _, note := range notes {
		values := noteindex.ExtractProperties(note)
		selected := map[string]domain.PropertyValue{}
		for _, name := range cleaned {
			if value, ok := values[name]; ok {
				selected[name] = value
				found[name] = true
			}
		}
		out[note.Path] = selected
	}
	if strict {
		missing := make([]string, 0)
		for _, name := range cleaned {
			if !found[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return nil, &domain.CommandError{Code: "property_not_found", Message: "Property not found: " + strings.Join(missing, ","), Hint: "Run pinax database schema infer --vault <vault> to view available properties"}
		}
	}
	return out, nil
}

func (s *Service) ListDimension(_ context.Context, req VaultRequest, dimension string) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(dimension+".list", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(dimension+".list", err), err
	}
	counts := map[string]int{}
	for _, note := range notes {
		for _, value := range noteDimensionValues(note, dimension) {
			counts[value]++
		}
	}
	items := make([]domain.DimensionCount, 0, len(counts))
	for value, count := range counts {
		items = append(items, domain.DimensionCount{Value: value, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Value < items[j].Value
		}
		return items[i].Count > items[j].Count
	})
	projection := domain.NewProjection(dimension+".list", "Organize views listed.")
	projection.Facts["dimension"] = dimension
	projection.Facts["dimensions"] = fmt.Sprint(len(items))
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Data = map[string]any{"dimension": dimension, "items": items}
	return projection, nil
}

func (s *Service) SaveView(_ context.Context, req ViewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.save", err), err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "view_name_required", Message: "view save requires a name", Hint: "pinax view save <name> --vault <vault>"}
		return domain.NewErrorProjection("view.save", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.save", err), err
	}
	view := domain.SavedView{Name: name, Tags: cleanTags(req.Tags), Group: strings.TrimSpace(req.Group), Folder: strings.TrimSpace(req.Folder), Kind: strings.TrimSpace(req.Kind), Status: strings.TrimSpace(req.Status), Sort: normalizedListSort(req.Sort), Limit: req.Limit, CreatedAfter: req.CreatedAfter, UpdatedBefore: req.UpdatedBefore, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	upsertSavedView(&registry, view)
	if err := saveSavedViews(root, registry); err != nil {
		return errorProjection("view.save", err), err
	}
	projection := domain.NewProjection("view.save", "View saved.")
	projection.Facts["view"] = name
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "views.json"))}
	projection.Data = map[string]any{"view": view}
	return projection, nil
}

func (s *Service) ListViews(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.list", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.list", err), err
	}
	projection := domain.NewProjection("view.list", "Views listed.")
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Data = registry
	return projection, nil
}

func (s *Service) ShowView(ctx context.Context, req ViewRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.show", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.show", err), err
	}
	view, ok := findSavedView(registry, req.Name)
	if !ok {
		err := &domain.CommandError{Code: "view_not_found", Message: "Saved view not found", Hint: "pinax view list --vault <vault>"}
		return domain.NewErrorProjection("view.show", err), err
	}
	projection, err := s.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Tags: view.Tags, Group: view.Group, Folder: view.Folder, Kind: view.Kind, Status: view.Status, CreatedAfter: view.CreatedAfter, UpdatedBefore: view.UpdatedBefore, Sort: view.Sort, Limit: view.Limit})
	projection.Command = "view.show"
	projection.Summary = "View queried."
	projection.Facts["view"] = view.Name
	projection.Data = map[string]any{"view": view, "result": projection.Data}
	return projection, err
}

func (s *Service) DeleteView(_ context.Context, req ViewRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "view delete requires --yes", Hint: "Add --yes after confirming"}
		return domain.NewErrorProjection("view.delete", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("view.delete", err), err
	}
	registry, err := loadSavedViews(root)
	if err != nil {
		return errorProjection("view.delete", err), err
	}
	removed := removeSavedView(&registry, req.Name)
	if err := saveSavedViews(root, registry); err != nil {
		return errorProjection("view.delete", err), err
	}
	projection := domain.NewProjection("view.delete", "View deleted.")
	projection.Facts["view"] = req.Name
	projection.Facts["removed"] = fmt.Sprint(removed)
	projection.Facts["views"] = fmt.Sprint(len(registry.Views))
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "views.json"))}
	return projection, nil
}

func noteDimensionValues(note domain.Note, dimension string) []string {
	switch dimension {
	case "tag":
		return cleanTags(noteAllTags(note))
	case "folder":
		if strings.TrimSpace(note.Folder) != "" {
			return []string{note.Folder}
		}
		dir := filepath.ToSlash(filepath.Dir(note.Path))
		if dir == "." {
			return []string{""}
		}
		return []string{strings.TrimPrefix(dir, "notes/")}
	case "kind":
		return []string{strings.TrimSpace(note.Kind)}
	case "group":
		return []string{strings.TrimSpace(note.Project)}
	default:
		return []string{""}
	}
}

func loadSavedViews(root string) (domain.SavedViewRegistry, error) {
	registry := domain.SavedViewRegistry{SchemaVersion: "pinax.views.v1", Views: []domain.SavedView{}}
	path := filepath.Join(root, ".pinax", "views.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		return registry, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = "pinax.views.v1"
	}
	if registry.Views == nil {
		registry.Views = []domain.SavedView{}
	}
	return registry, nil
}

func saveSavedViews(root string, registry domain.SavedViewRegistry) error {
	if strings.TrimSpace(registry.SchemaVersion) == "" {
		registry.SchemaVersion = "pinax.views.v1"
	}
	if registry.Views == nil {
		registry.Views = []domain.SavedView{}
	}
	sort.Slice(registry.Views, func(i, j int) bool { return registry.Views[i].Name < registry.Views[j].Name })
	return writeJSONAsset(filepath.Join(root, ".pinax", "views.json"), registry)
}

func upsertSavedView(registry *domain.SavedViewRegistry, view domain.SavedView) {
	for i, existing := range registry.Views {
		if existing.Name == view.Name {
			registry.Views[i] = view
			return
		}
	}
	registry.Views = append(registry.Views, view)
}

func findSavedView(registry domain.SavedViewRegistry, name string) (domain.SavedView, bool) {
	for _, view := range registry.Views {
		if view.Name == strings.TrimSpace(name) {
			return view, true
		}
	}
	return domain.SavedView{}, false
}

func removeSavedView(registry *domain.SavedViewRegistry, name string) bool {
	name = strings.TrimSpace(name)
	for i, view := range registry.Views {
		if view.Name != name {
			continue
		}
		registry.Views = append(registry.Views[:i], registry.Views[i+1:]...)
		return true
	}
	return false
}

func normalizedListSort(sortMode string) string {
	sortMode = strings.TrimSpace(sortMode)
	switch sortMode {
	case "", "updated":
		return "updated"
	case "path", "title":
		return sortMode
	default:
		return "updated"
	}
}

func (s *Service) CreateNote(ctx context.Context, req CreateNoteRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		err := &domain.CommandError{Code: "title_required", Message: "note new requires a title", Hint: "pinax note new <title> --vault <vault>"}
		return domain.NewErrorProjection("note.new", err), err
	}
	safeTags, tagErr := normalizeTagsForWrite(req.Tags)
	if tagErr != nil {
		return domain.NewErrorProjection("note.new", tagErr), tagErr
	}
	req.Tags = safeTags
	templateName := strings.TrimSpace(req.Template)
	var templateDoc templateengine.TemplateDocument
	var templatePathPattern string
	templateDefaults := map[string]string{}
	templateOverrides := []string{}
	templateMeta := templateengine.Metadata{}
	templateSource := ""
	if templateName != "" {
		doc, err := parseTemplateForProjection(root, templateName)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		if templateDocumentIsDesignDraft(doc) {
			err := &domain.CommandError{Code: "template_design_not_executable", Message: "Template is still a draft and cannot be used for note creation", Hint: "Publish the draft as an executable schema_version: pinax.template.v2 template first"}
			return domain.NewErrorProjection("note.new", err), err
		}
		templateDoc = doc
		templateMeta, templateSource = templateProjectionMetadata(root, templateName, doc.Metadata)
		templatePathPattern = doc.Metadata.Output.PathPattern
		templateDefaults = doc.Metadata.Defaults
		if req.Kind == "" && templateDefaults["kind"] != "" {
			req.Kind = templateDefaults["kind"]
		} else if req.Kind != "" && templateDefaults["kind"] != "" && req.Kind != templateDefaults["kind"] {
			templateOverrides = append(templateOverrides, "kind")
		}
		if req.Status == "" && templateDefaults["status"] != "" {
			req.Status = templateDefaults["status"]
		} else if req.Status != "" && templateDefaults["status"] != "" && req.Status != templateDefaults["status"] {
			templateOverrides = append(templateOverrides, "status")
		}
		if req.Dir != "" || req.Folder != "" || req.Slug != "" || req.Project != "" {
			templateOverrides = append(templateOverrides, "path")
		}
		if len(req.Tags) == 0 && templateDefaults["tags"] != "" {
			req.Tags = splitCommaValues(templateDefaults["tags"])
			var tagErr *domain.CommandError
			req.Tags, tagErr = normalizeTagsForWrite(req.Tags)
			if tagErr != nil {
				return domain.NewErrorProjection("note.new", tagErr), tagErr
			}
		} else if len(req.Tags) > 0 && templateDefaults["tags"] != "" {
			templateOverrides = append(templateOverrides, "tags")
		}
	}
	body, err := noteBodyFromRequest(req)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	folder, err := validateOptionalNoteFolder(req.Folder)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	kind := strings.TrimSpace(req.Kind)
	var rel string
	if templatePathPattern != "" && req.Dir == "" && req.Folder == "" && req.Slug == "" && req.Project == "" {
		templateRel, err := renderTemplateOutputPath(templateDoc, req)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		rel, err = nextNotePath(root, templateRel)
		if err != nil {
			return errorProjection("note.new", err), err
		}
	} else {
		prefix, err := noteCreatePrefix(root, req)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		slug := strings.TrimSpace(req.Slug)
		if slug == "" {
			slug = slugify(req.Title)
		}
		if slug == "" {
			slug = deterministicShortID(req.Title)
		}
		if err := validateNoteSlug(slug); err != nil {
			return errorProjection("note.new", err), err
		}
		rel, err = nextNotePath(root, filepath.ToSlash(filepath.Join(prefix, slug+".md")))
		if err != nil {
			return errorProjection("note.new", err), err
		}
	}
	if body == "" {
		body = "# " + req.Title + "\n"
	}
	if strings.TrimSpace(req.Template) != "" && req.Body == "" && req.SourcePath == "" && req.StdinBody == "" {
		rendered, err := s.renderTemplateBody(ctx, root, TemplateRequest{VaultPath: root, Name: req.Template, Title: req.Title, Project: req.Project, Tags: req.Tags, Vars: req.Vars}, true)
		if err != nil {
			return errorProjection("note.new", err), err
		}
		body = rendered
	}
	noteID, err := s.allocateObjectID(identity.KindNote, root, rel)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	content := buildNoteContentWithObjectID(noteID, req.Title, req.Project, folder, kind, cleanTags(req.Tags), req.Status, now, body)
	projection := domain.NewProjection("note.new", "Note created.")
	projection.Facts["path"] = rel
	projection.Facts["planned_path"] = rel
	projection.Facts["title"] = req.Title
	projection.Facts["note_id"] = noteID
	projection.Facts["tags"] = strings.Join(cleanTags(req.Tags), ",")
	if req.Project != "" {
		projection.Facts["project"] = req.Project
		projection.Facts["group"] = req.Project
	}
	if folder != "" {
		projection.Facts["folder"] = folder
	}
	if kind != "" {
		projection.Facts["kind"] = kind
	}
	if req.Status != "" {
		projection.Facts["status"] = req.Status
	}
	if templateName != "" {
		projection.Facts["template"] = templateName
		templateUseID := deterministicShortID(templateName + ":" + rel)
		projection.Facts["template_use_id"] = templateUseID
		projection.Facts["effective_path"] = rel
		if templateMeta.ScenarioID != "" {
			projection.Facts["scenario_id"] = templateMeta.ScenarioID
		}
		if templateMeta.Pack.ID != "" {
			projection.Facts["template_pack"] = templateMeta.Pack.ID
		}
		if templateMeta.ProofGate.Status != "" {
			projection.Facts["proof_gate.status"] = templateMeta.ProofGate.Status
		}
		if templatePathPattern != "" {
			projection.Facts["template.path_pattern"] = templatePathPattern
		}
		if len(templateDefaults) > 0 {
			projection.Facts["template.defaults_source"] = templateName
		}
		if len(templateOverrides) > 0 {
			projection.Facts["template.overrides"] = strings.Join(templateOverrides, ",")
		}
	}
	data := map[string]any{"note": domain.Note{ID: noteID, Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Body: strings.TrimSpace(body), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status, CreatedAt: now, UpdatedAt: now}, "planned_path": rel, "frontmatter_preview": strings.SplitN(content, "---\n\n", 2)[0] + "---", "body_preview": strings.TrimSpace(body)}
	projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax note show %s --vault %s", shellQuote(rel), shellQuote(root))}}
	if templateName != "" {
		nextActions := templateMetadataActions(templateMeta.AfterCreateActions)
		nextActions = append(nextActions, projection.Actions...)
		data["template_use"] = map[string]any{
			"template_use_id":      projection.Facts["template_use_id"],
			"template":             templateName,
			"template_pack":        templateMeta.Pack.ID,
			"scenario_id":          templateMeta.ScenarioID,
			"effective_path":       rel,
			"source":               templateSource,
			"proof_gate":           templateMeta.ProofGate,
			"next_actions":         nextActions,
			"after_create_actions": templateMeta.AfterCreateActions,
		}
		projection.Actions = nextActions
	}
	projection.Data = data
	if req.DryRun {
		projection.Summary = "Note create plan generated."
		projection.Facts["ledger_status"] = "preview"
		projection.Facts["record_event"] = string(domain.RecordEventNoteCreated)
		projection.Facts["version_backend"] = "none"
		return projection, nil
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("note.new", err), err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return errorProjection("note.new", err), err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorProjection("note.new", err), err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return errorProjection("note.new", err), err
	}
	dailyIndexRel, dailyErr := appendDailyIndex(root, domain.Note{ID: noteID, Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status})
	if dailyErr != nil {
		if code := templateengine.ErrorCode(dailyErr); strings.HasPrefix(code, "managed_block_") {
			projection.Status = "partial"
			projection.Facts["daily_index"] = dailyIndexRel
			projection.Facts["daily_index_status"] = code
			projection.Actions = append(projection.Actions, domain.Action{Name: "upgrade_daily_template", Command: fmt.Sprintf("pinax journal daily show --template journal.daily --vault %s --json", shellQuote(root))})
		} else {
			return errorProjection("note.new", dailyErr), dailyErr
		}
	} else if dailyIndexRel != "" {
		projection.Facts["daily_index"] = dailyIndexRel
		projection.Facts["daily_index_status"] = "updated"
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("note.new", err), err
	}
	projection.Facts["index_updated"] = "true"
	projection.Evidence = []string{dailyIndexRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	note := domain.Note{ID: noteID, Title: req.Title, Path: rel, Tags: cleanTags(req.Tags), Body: strings.TrimSpace(body), Project: req.Project, Folder: folder, Kind: kind, Status: req.Status, CreatedAt: now, UpdatedAt: now}
	recordEvent, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteCreated, "note.new:"+note.ID+":"+rel, note, "")
	if recordErr != nil {
		return errorProjection("note.new", recordErr), recordErr
	}
	applyRecordEventFacts(&projection, recordEvent)
	eventFacts := map[string]string{"path": rel, "title": req.Title}
	if templateName != "" {
		eventFacts["template"] = templateName
		eventFacts["template_use_id"] = projection.Facts["template_use_id"]
		eventFacts["scenario_id"] = templateMeta.ScenarioID
	}
	_ = appendEvent(root, "note.new", "success", eventFacts)
	return projection, nil
}

type resolverNoteAmbiguousError struct {
	*domain.CommandError
	Result ResolverResult
}

func (e *resolverNoteAmbiguousError) Unwrap() error { return e.CommandError }

func strongestResolverResult(result ResolverResult) ResolverResult {
	if len(result.Candidates) <= 1 {
		return result
	}
	bestScore := result.Candidates[0].Score
	for _, candidate := range result.Candidates[1:] {
		if candidate.Score > bestScore {
			bestScore = candidate.Score
		}
	}
	strongest := make([]domain.VaultObjectCandidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		if candidate.Score == bestScore {
			strongest = append(strongest, candidate)
		}
	}
	result.Candidates = strongest
	result.Facts.Candidates = len(strongest)
	result.Facts.Ambiguous = len(strongest) > 1
	result.Facts.MatchField = ""
	if len(strongest) > 0 && len(strongest[0].MatchFields) > 0 {
		result.Facts.MatchField = strongest[0].MatchFields[0]
	}
	return result
}

func (s *Service) ResolveNote(ctx context.Context, req ShowNoteRequest) (domain.Note, error) {
	note, _, err := s.ResolveNoteWithResolver(ctx, req)
	return note, err
}

func (s *Service) ResolveNoteWithResolver(ctx context.Context, req ShowNoteRequest) (domain.Note, ResolverResult, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return domain.Note{}, ResolverResult{}, err
	}
	result, err := s.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: req.NoteRef, Scope: "registered", Kind: "note"})
	if err != nil {
		return domain.Note{}, result, err
	}
	result = strongestResolverResult(result)
	if len(result.Candidates) > 1 {
		return domain.Note{}, result, &resolverNoteAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "Note reference has multiple candidates", Hint: "Retry with a note_id or full path"}, Result: result}
	}
	if len(result.Candidates) == 0 {
		return domain.Note{}, result, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax note list to view available notes"}
	}
	notes, err := scanNotes(root)
	if err != nil {
		return domain.Note{}, result, err
	}
	for _, note := range notes {
		if note.Path == result.Candidates[0].Path {
			return note, result, nil
		}
	}
	return domain.Note{}, result, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax index refresh, then retry"}
}

func (s *Service) ShowNote(ctx context.Context, req ShowNoteRequest) (domain.Note, error) {
	return s.ResolveNote(ctx, req)
}

func (s *Service) ShowNoteProjection(ctx context.Context, req ShowNoteRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.show", err), err
	}
	note, resolverResult, err := s.ResolveNoteWithResolver(ctx, req)
	if err != nil {
		projection := errorProjection("note.show", err)
		if amb, ok := err.(*resolverNoteAmbiguousError); ok {
			projection.Data = map[string]any{"candidates": amb.Result.Candidates}
			projection.Facts["candidates"] = fmt.Sprint(len(amb.Result.Candidates))
		}
		return projection, err
	}
	view := strings.TrimSpace(req.View)
	if view == "" {
		view = "source"
	}
	displayRequested := strings.TrimSpace(req.Display) != ""
	display := domain.NoteDisplayKind("")
	if displayRequested {
		var displayErr *domain.CommandError
		display, displayErr = parseNoteDisplayKind(req.Display)
		if displayErr != nil {
			return domain.NewErrorProjection("note.show", displayErr), displayErr
		}
	}
	projection := domain.NewProjection("note.show", "Local note read.")
	projection.Facts["path"] = note.Path
	projection.Facts["title"] = note.Title
	projection.Facts["note_id"] = note.ID
	projection.Facts["object_id"] = note.ID
	projection.Facts["object_kind"] = "note"
	projection.Facts["view"] = view
	if tags := cleanTags(note.Tags); len(tags) > 0 {
		projection.Facts["tags"] = strings.Join(tags, ",")
	}
	body := note.Body
	queryCount := 0
	projection.Facts["resolver.match_field"] = string(resolverResult.Facts.MatchField)
	projection.Facts["resolver.candidates"] = fmt.Sprint(resolverResult.Facts.Candidates)
	queryFacts := map[string]map[string]string{}
	renderRuns := []renderRunReceipt{}
	embeddedAssets := []pinaxassets.EmbeddedAssetPreview{}
	databaseTabs := []domain.DatabaseTab{}
	if req.Runs {
		runs, err := listNoteRenderRuns(root, note.Path)
		if err != nil {
			return errorProjection("note.show", err), err
		}
		renderRuns = runs
		projection.Facts["runs"] = fmt.Sprint(len(runs))
	}
	if view == "rendered" {
		if req.Snapshot != "" {
			snapshot, run, err := loadNoteRenderedSnapshot(root, note.Path, req.Snapshot)
			if err != nil {
				return errorProjection("note.show", err), err
			}
			body = snapshot
			projection.Facts["snapshot"] = req.Snapshot
			projection.Facts["run_id"] = run.RunID
		} else {
			rendered, facts, err := s.renderNoteQueryBlocks(ctx, root, note.Body)
			if err != nil {
				return errorProjection("note.show", err), err
			}
			body = rendered.Body
			queryCount = rendered.QueryCount
			queryFacts = facts
			databaseTabs = rendered.DatabaseTabs
		}
		if req.EmbedAttachments != "" {
			preview, err := pinaxassets.RenderEmbeddedPreview(pinaxassets.RenderPreviewRequest{Root: root, SourcePath: note.Path, Body: body, Mode: req.EmbedAttachments, MaxDepth: req.MaxEmbedDepth, MaxBytes: req.MaxEmbedBytes})
			if err != nil {
				return errorProjection("note.show", err), err
			}
			body = preview.Body
			embeddedAssets = preview.EmbeddedAssets
			projection.Facts["embedded_assets"] = fmt.Sprint(len(embeddedAssets))
			projection.Facts["embed_attachments"] = req.EmbedAttachments
		}
	} else if view != "source" {
		err := &domain.CommandError{Code: "note_view_invalid", Message: "note show --view only supports source or rendered", Hint: "Use --view source or --view rendered"}
		return domain.NewErrorProjection("note.show", err), err
	}
	projection.Facts["query_count"] = fmt.Sprint(queryCount)
	if len(queryFacts) > 0 {
		projection.Facts["queries"] = fmt.Sprint(len(queryFacts))
	}
	if len(databaseTabs) > 0 {
		projection.Facts["database_tabs"] = fmt.Sprint(len(databaseTabs))
	}
	data := map[string]any{"note": note, "body": body, "view": view, "query_facts": queryFacts, "render_runs": renderRuns}
	if displayRequested {
		displayNote := buildNoteDisplay(note, display, domain.NoteExposureLocalDetail)
		if display == domain.NoteDisplayBody {
			displayNote.Body = body
			displayNote.Exposure = domain.NoteExposureLocalBody
		}
		projection.Facts["display"] = string(display)
		data = map[string]any{"note": displayNote, "view": view, "query_facts": queryFacts, "render_runs": renderRuns}
	}
	if len(embeddedAssets) > 0 {
		data["embedded_assets"] = embeddedAssets
	}
	if len(databaseTabs) > 0 {
		data["database_tabs"] = databaseTabs
	}
	projection.Data = data
	return projection, nil
}

func (s *Service) RefreshNoteRendered(ctx context.Context, req NoteRefreshRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "Writing back rendered managed blocks requires --yes", Hint: "Add --yes after confirming"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	note, err := s.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: req.NoteRef})
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	rendered, facts, err := s.renderNoteQueryBlocks(ctx, root, note.Body)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	if req.Snapshot != "" {
		snapshot, run, err := loadNoteRenderedSnapshot(root, note.Path, req.Snapshot)
		if err != nil {
			return errorProjection("note.refresh", err), err
		}
		rendered = renderedNoteBody{Body: snapshot, ByName: map[string]string{"active": snapshot}, QueryCount: 0}
		facts = map[string]map[string]string{"snapshot": {"run_id": run.RunID}}
	}
	if rendered.QueryCount == 0 {
		err := &domain.CommandError{Code: "render_query_not_found", Message: "No pinax-sql query block found in the note", Hint: "Add a ```pinax-sql <name> query block, then retry"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	updatedBody, changed := replaceManagedRenderBlocks(note.Body, rendered.ByName)
	if changed == 0 {
		err := &domain.CommandError{Code: "render_block_not_found", Message: "No writable pinax render managed block found", Hint: "Add <!-- pinax:render <name> start --> and end marker"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return errorProjection("note.refresh", err), err
	}
	oldBody := note.Body
	newContent := strings.Replace(string(content), oldBody, updatedBody, 1)
	if newContent == string(content) {
		err := &domain.CommandError{Code: "render_refresh_failed", Message: "Cannot locate the rendered block to write back in the note body", Hint: "Check whether note frontmatter and body were modified externally"}
		return domain.NewErrorProjection("note.refresh", err), err
	}
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return errorProjection("note.refresh", err), err
	}
	_ = refreshIndex(root)
	projection := domain.NewProjection("note.refresh", "Rendered managed block refreshed.")
	projection.Facts["path"] = note.Path
	projection.Facts["view"] = "rendered"
	projection.Facts["changed_blocks"] = fmt.Sprint(changed)
	projection.Facts["query_count"] = fmt.Sprint(rendered.QueryCount)
	var savedRun *renderRunReceipt
	if req.SaveRun != "" {
		run, err := saveNoteRenderRun(root, note.Path, req.SaveRun, rendered.Body)
		if err != nil {
			return errorProjection("note.refresh", err), err
		}
		projection.Facts["run_saved"] = "true"
		projection.Facts["run_id"] = run.RunID
		projection.Facts["run_name"] = run.Name
		savedRun = &run
	}
	projection.Data = map[string]any{"path": note.Path, "changed_blocks": changed, "query_facts": facts, "render_run": savedRun}
	projection.Evidence = []string{note.Path}
	return projection, nil
}

type renderedNoteBody struct {
	Body         string
	ByName       map[string]string
	QueryCount   int
	DatabaseTabs []domain.DatabaseTab
}

var noteQueryFencePattern = regexp.MustCompile("(?ms)^```(pinax-sql|pinax-dataview)(?:[ \\t]+(?:name=)?([A-Za-z_][A-Za-z0-9_:-]*))?[ \\t]*\\n(.*?)\\n```[ \\t]*(?:\\n|$)")
var noteDatabaseViewFencePattern = regexp.MustCompile("(?ms)^```pinax-database-view(?:[ \\t]+([A-Za-z_][A-Za-z0-9_:-]*))?[ \\t]*\\n(?:([A-Za-z_][A-Za-z0-9_:-]*)[ \\t]*\\n)?```[ \\t]*(?:\\n|$)")
var managedRenderBlockPattern = regexp.MustCompile("(?ms)<!-- pinax:render ([A-Za-z_][A-Za-z0-9_:-]*) start -->.*?<!-- pinax:render ([A-Za-z_][A-Za-z0-9_:-]*) end -->")
var managedDataviewBlockPattern = regexp.MustCompile("(?ms)<!-- pinax:managed name=([A-Za-z_][A-Za-z0-9_:-]*) -->.*?<!-- /pinax:managed -->")

func (s *Service) renderNoteQueryBlocks(ctx context.Context, root, body string) (renderedNoteBody, map[string]map[string]string, error) {
	byName := map[string]string{}
	facts := map[string]map[string]string{}
	databaseTabs := []domain.DatabaseTab{}
	count := 0
	var firstErr error
	rendered := noteQueryFencePattern.ReplaceAllStringFunc(body, func(block string) string {
		if firstErr != nil {
			return block
		}
		match := noteQueryFencePattern.FindStringSubmatch(block)
		if len(match) < 4 {
			return block
		}
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])
		if name == "" {
			name = fmt.Sprintf("query%d", count+1)
		}
		query := strings.TrimSpace(match[3])
		var projection domain.Projection
		var err error
		if kind == "pinax-dataview" {
			projection, err = s.DataviewRun(ctx, DataviewRequest{VaultPath: root, Query: query, Limit: 50, LazyIndex: true})
		} else {
			projection, err = s.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: query, Limit: 50, LazyIndex: true})
		}
		if err != nil {
			firstErr = templateQueryCommandError(name, err)
			return block
		}
		result := templateQueryResultFromProjection(projection)
		markdown := templateops.QueryResultMarkdown(result)
		byName[name] = markdown
		facts[name] = projection.Facts
		count++
		return markdown + "\n"
	})
	rendered = noteDatabaseViewFencePattern.ReplaceAllStringFunc(rendered, func(block string) string {
		if firstErr != nil {
			return block
		}
		match := noteDatabaseViewFencePattern.FindStringSubmatch(block)
		if len(match) < 3 {
			return block
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			name = strings.TrimSpace(match[2])
		}
		if name == "" {
			firstErr = &domain.CommandError{Code: "database_tab_view_required", Message: "pinax-database-view fence requires a saved view name", Hint: "Use ```pinax-database-view <name>"}
			return block
		}
		projection, err := s.RenderDatabaseView(ctx, ViewRequest{VaultPath: root, Name: name})
		if err != nil {
			firstErr = databaseTabCommandError(name, err)
			return block
		}
		tab := databaseTabFromProjection(name, projection)
		markdown := databaseTabMarkdown(tab)
		databaseTabs = append(databaseTabs, tab)
		byName[name] = markdown
		facts[name] = projection.Facts
		count++
		return markdown + "\n"
	})
	if firstErr != nil {
		return renderedNoteBody{}, nil, firstErr
	}
	return renderedNoteBody{Body: rendered, ByName: byName, QueryCount: count, DatabaseTabs: databaseTabs}, facts, nil
}

func databaseTabCommandError(name string, err error) *domain.CommandError {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) && commandErr.Code == "view_not_found" {
		return &domain.CommandError{Code: "database_tab_view_not_found", Message: "Database tab saved view not found: " + name, Hint: "Run pinax database view list --vault <vault>"}
	}
	message := "Database tab render failed: " + name
	if err != nil && err.Error() != "" {
		message += ": " + err.Error()
	}
	return &domain.CommandError{Code: "database_tab_render_failed", Message: message, Hint: "Run pinax database view render " + shellQuote(name) + " --vault <vault> --json"}
}

func databaseTabFromProjection(name string, projection domain.Projection) domain.DatabaseTab {
	data, _ := projection.Data.(map[string]any)
	if tab, ok := data["database_tab"].(domain.DatabaseTab); ok {
		return tab
	}
	render, _ := data["render"].(domain.DatabaseViewRender)
	display := projection.Facts["display"]
	if display == "" {
		display = render.Display
	}
	return domain.DatabaseTab{Name: name, View: projection.Facts["view"], Display: display, Rows: render.RowCount, Render: render, Facts: projection.Facts}
}

func databaseTabMarkdown(tab domain.DatabaseTab) string {
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(tab.Name)
	b.WriteString("\n\n")
	switch tab.Display {
	case "table":
		if tab.Render.Table != nil {
			b.WriteString(tableResultMarkdown(*tab.Render.Table))
		}
	case "list":
		for _, item := range tab.Render.List {
			b.WriteString("- ")
			b.WriteString(item.Title)
			if item.Path != "" {
				b.WriteString(" (")
				b.WriteString(item.Path)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
	case "board":
		for _, column := range tab.Render.Board.Columns {
			b.WriteString("#### ")
			b.WriteString(column.ID)
			b.WriteString("\n")
			for _, item := range column.Items {
				b.WriteString("- ")
				b.WriteString(item.Title)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	case "calendar":
		for _, event := range tab.Render.Calendar.Events {
			b.WriteString("- ")
			b.WriteString(event.Date)
			b.WriteString(" - ")
			b.WriteString(event.Title)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func tableResultMarkdown(result domain.TableResult) string {
	converted := templateengine.QueryResult{Columns: result.Columns, Rows: make([]map[string]string, 0, len(result.Rows))}
	for _, row := range result.Rows {
		values := map[string]string{}
		for _, column := range result.Columns {
			values[column] = databaseViewRowValue(row, column)
		}
		converted.Rows = append(converted.Rows, values)
	}
	return templateops.QueryResultMarkdown(converted)
}

func replaceManagedRenderBlocks(body string, rendered map[string]string) (string, int) {
	changed := 0
	updated := managedRenderBlockPattern.ReplaceAllStringFunc(body, func(block string) string {
		match := managedRenderBlockPattern.FindStringSubmatch(block)
		if len(match) < 3 || match[1] != match[2] {
			return block
		}
		name := match[1]
		content, ok := rendered[name]
		if !ok {
			return block
		}
		changed++
		return fmt.Sprintf("<!-- pinax:render %s start -->\n%s\n<!-- pinax:render %s end -->", name, strings.TrimSpace(content), name)
	})
	updated = managedDataviewBlockPattern.ReplaceAllStringFunc(updated, func(block string) string {
		match := managedDataviewBlockPattern.FindStringSubmatch(block)
		if len(match) < 2 {
			return block
		}
		name := match[1]
		content, ok := rendered[name]
		if !ok {
			return block
		}
		changed++
		return fmt.Sprintf("<!-- pinax:managed name=%s -->\n%s\n<!-- /pinax:managed -->", name, strings.TrimSpace(content))
	})
	return updated, changed
}

func (s *Service) SearchNotes(ctx context.Context, req SearchRequest) (result SearchResult, err error) {
	searchReq := toSearchOpsRequest(req)
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return SearchResult{}, err
	}
	monitorFacts := monitorQueryFacts("query", req.Query)
	monitorFacts["engine_requested"] = searchops.NormalizedEngine(req.Engine)
	monitorFacts["lazy_index"] = searchops.NormalizedLazyIndex(req.LazyIndex)
	monitorFacts["limit"] = fmt.Sprint(req.Limit)
	rec := startMonitorRun(root, "note.search", monitorFacts)
	defer func() { rec.Finish("", err) }()
	searchReq.VaultPath = root
	endStep := rec.BeginStep("search.validate", nil)
	if err := searchops.ValidateVersionAware(searchReq); err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	if err := searchops.ValidateDateFilters(searchReq); err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	endStep(nil)
	engine := searchops.NormalizedEngine(searchReq.Engine)
	endStep = rec.BeginStep("notes.scan", nil)
	notes, err := scanNotes(root)
	if err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	notes = ordinaryNotes(notes)
	endStep(nil)
	endStep = rec.BeginStep("link_filter.build", map[string]string{"active": fmt.Sprint(strings.TrimSpace(req.LinkTarget) != "")})
	linkFilter, err := searchops.BuildLinkTargetFilter(notes, req.LinkTarget, func(notes []domain.Note) map[string][]domain.NoteLink {
		outgoing, _ := BuildEnhancedLinkGraph(notes)
		return outgoing
	})
	if err != nil {
		endStep(err)
		return SearchResult{}, err
	}
	endStep(nil)
	endStep = rec.BeginStep("index.inspect", nil)
	status, err := noteindex.Inspect(root, notes)
	endStep(err)
	indexLoaded := ""
	if err == nil && engine == "auto" && searchops.LazyIndexAllowed(searchReq, status, notes) {
		select {
		case <-ctx.Done():
			return SearchResult{}, ctx.Err()
		default:
		}
		endStep = rec.BeginStep("index.lazy_rebuild", map[string]string{"index_status": status.Status})
		if _, rebuildErr := noteindex.Rebuild(root, notes); rebuildErr == nil {
			endStep(nil)
			endStep = rec.BeginStep("index.inspect_after_lazy_rebuild", nil)
			if rebuiltStatus, inspectErr := noteindex.Inspect(root, notes); inspectErr == nil {
				status = rebuiltStatus
				indexLoaded = "lazy_rebuild"
			}
			endStep(nil)
		} else {
			endStep(rebuildErr)
		}
	}
	if engine == "native" {
		endStep = rec.BeginStep("native.search", nil)
		result := notesearch.Notes(ctx, root, req.Query, notes)
		endStep(nil)
		indexStatus := "missing"
		if err == nil && status.Status != "" {
			indexStatus = status.Status
		}
		return searchops.ResultFromFallback(searchReq, result.Engine, result.Notes, indexStatus, linkFilter), nil
	}
	if engine == "index" && (err != nil || (status.Status != "fresh" && (status.Status != "stale" || !req.AllowStale))) {
		indexStatus := "missing"
		if err == nil && status.Status != "" {
			indexStatus = status.Status
		}
		return SearchResult{}, &domain.CommandError{Code: "search_index_unavailable", Message: "Search index is not available for --engine index: " + indexStatus, Hint: fmt.Sprintf("pinax index refresh --vault %s --json", shellQuote(root))}
	}
	if err == nil && (status.Status == "fresh" || (status.Status == "stale" && req.AllowStale)) {
		indexReq := searchops.BuildIndexRequest(searchReq, linkFilter)
		endStep = rec.BeginStep("index.search", map[string]string{"index_status": status.Status})
		result, searchErr := noteindex.Search(root, indexReq)
		endStep(searchErr)
		if searchErr == nil && (result.Returned > 0 || strings.TrimSpace(req.Query) == "") {
			result.IndexStatus = status.Status
			return searchops.ResultFromIndex(searchReq, indexLoaded, result, linkFilter), nil
		}
	}
	endStep = rec.BeginStep("native.search", map[string]string{"fallback": "true"})
	nativeResult := notesearch.Notes(ctx, root, req.Query, notes)
	endStep(nil)
	indexStatus := "missing"
	if err == nil && status.Status != "" {
		indexStatus = status.Status
	}
	return searchops.ResultFromFallback(searchReq, nativeResult.Engine, nativeResult.Notes, indexStatus, linkFilter), nil
}

type SearchResult = searchops.Result

func (s *Service) SearchProjection(ctx context.Context, req SearchRequest) (domain.Projection, error) {
	result, err := s.SearchNotes(ctx, req)
	if err != nil {
		return errorProjection("note.search", err), err
	}
	return searchops.Projection(toSearchOpsRequest(req), result, shellQuote), nil
}

func parseUserDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func (s *Service) PlanMetadata(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	ops := make([]domain.PlanOperation, 0)
	query := strings.TrimSpace(req.Query)
	if query != "" {
		resolverResult, err := s.ResolveVaultObject(ctx, ResolverRequest{VaultPath: root, Query: query, Scope: "registered_or_adoptable", Kind: "all"})
		if err != nil {
			return errorProjection("metadata.plan", err), err
		}
		candidates := resolverResult.Candidates
		if len(candidates) > 1 {
			err := &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "metadata plan query matched multiple candidates", Hint: "Retry with a more specific note_id, filename, or full path"}
			projection := domain.NewErrorProjection("metadata.plan", err)
			projection.Facts["candidates"] = fmt.Sprint(len(candidates))
			projection.Data = map[string]any{"candidates": candidates}
			return projection, err
		}
		if len(candidates) == 1 {
			candidate := candidates[0]
			if candidate.ObjectKind == "file" {
				ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: candidate.Path, Reason: "Add Pinax metadata to adoptable Markdown", Status: "planned"})
			} else {
				matchedNote := false
				for _, note := range notes {
					if note.Path != candidate.Path {
						continue
					}
					matchedNote = true
					if noteNeedsMetadataInVault(root, note) {
						ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: note.Path, Reason: "Add missing Pinax frontmatter", Status: "planned"})
					}
					ops = append(ops, durableSourceMetadataOperations(note)...)
				}
				if !matchedNote {
					ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: candidate.Path, Reason: "Add missing Pinax frontmatter", Status: "planned"})
				}
			}
		}
		projection := domain.NewProjection("metadata.plan", "Metadata plan generated.")
		projection.Facts["query"] = query
		projection.Facts["writes"] = "false"
		projection.Facts["candidates"] = fmt.Sprint(len(candidates))
		projection.Facts["planned_updates"] = fmt.Sprint(len(ops))
		projection.Data = map[string]any{"operations": ops, "candidates": candidates}
		if len(candidates) == 1 && candidates[0].ObjectKind == "file" {
			projection.Actions = []domain.Action{{Name: "adopt", Command: fmt.Sprintf("pinax record adopt %s --plan --vault %s --json", shellQuote(query), shellQuote(root))}}
		} else if len(ops) > 0 {
			projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax metadata apply --vault %s --yes", shellQuote(root))}}
		}
		return projection, nil
	}
	for _, note := range notes {
		if noteNeedsMetadataInVault(root, note) {
			ops = append(ops, domain.PlanOperation{Kind: "metadata_update", Path: note.Path, Reason: "Add missing Pinax frontmatter", Status: "planned"})
		}
		ops = append(ops, durableSourceMetadataOperations(note)...)
	}
	boundOperations, err := bindPlanOperations(ctx, root, ops)
	if err != nil {
		return errorProjection("metadata.plan", err), err
	}
	ops = boundOperations
	projection := domain.NewProjection("metadata.plan", "Metadata plan generated.")
	projection.Facts["planned_updates"] = fmt.Sprint(len(ops))
	projection.Data = map[string]any{"operations": ops}
	if len(ops) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax metadata apply --vault %s --yes", shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) ApplyMetadata(ctx context.Context, req ApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "metadata apply requires --yes", Hint: "Run pinax metadata plan first, then add --yes after confirming"}
		return domain.NewErrorProjection("metadata.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("metadata.apply", err), err
	}
	beforeBindingsByPath, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return errorProjection("metadata.apply", err), err
	}
	beforeBindings := managedBindingsByObject(beforeBindingsByPath)
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("metadata.apply", err), err
	}
	applied := 0
	changedPaths := make([]string, 0)
	for _, note := range notes {
		if !noteNeedsMetadata(note) {
			continue
		}
		path, err := safeJoin(root, note.Path)
		if err != nil {
			return errorProjection("metadata.apply", err), err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return errorProjection("metadata.apply", err), err
		}
		if strings.TrimSpace(note.ID) == "" {
			objectID, allocateErr := s.allocateObjectID(identity.KindNote, root, note.Path)
			if allocateErr != nil {
				return errorProjection("metadata.apply", allocateErr), allocateErr
			}
			note.ID = objectID
		}
		updated := ensureFrontmatter(note, string(content))
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return errorProjection("metadata.apply", err), err
		}
		parsed := parseNote(note.Path, updated)
		if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMetadataUpdated, "metadata.apply:"+parsed.ID+":"+note.Path, parsed, ""); err != nil {
			return errorProjection("metadata.apply", err), err
		}
		applied++
		changedPaths = append(changedPaths, note.Path)
		_ = appendEvent(root, "metadata.apply", "success", map[string]string{"path": note.Path})
	}
	projection := domain.NewProjection("metadata.apply", "Metadata applied.")
	projection.Facts["applied_updates"] = fmt.Sprint(applied)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	receipt, receiptErr := writeObjectApplyReceipt(ctx, root, "metadata.apply", "", "", beforeBindings, changedPaths)
	if receiptErr != nil {
		return errorProjection("metadata.apply", receiptErr), receiptErr
	}
	addApplyReceiptProjection(&projection, receipt)
	projection.Data = map[string]any{"applied_updates": applied, "receipt": receipt}
	return projection, nil
}

func (s *Service) PlanOrganize(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.plan", err), err
	}
	ops, err := planOrganize(root)
	if err != nil {
		return errorProjection("organize.plan", err), err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("organize.plan", err), err
	}
	ops = append(ops, organizeFactOperations(root, organizeCandidateFacts(facts))...)
	moves := 0
	manualReview := 0
	for _, op := range ops {
		if op.Kind == "move" && op.Status == "planned" {
			moves++
		}
		if op.Status == "manual_review" {
			manualReview++
		}
	}
	projection := domain.NewProjection("organize.plan", "Organize plan generated.")
	projection.Facts["planned_moves"] = fmt.Sprint(moves)
	projection.Facts["manual_review"] = fmt.Sprint(manualReview)
	projection.Data = map[string]any{"operations": ops}
	if moves > 0 {
		projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before organize"))}}
	}
	return projection, nil
}

func (s *Service) SuggestOrganize(_ context.Context, req OrganizeSuggestRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.suggest", err), err
	}
	plan, err := buildOrganizePlan(root)
	if err != nil {
		return errorProjection("organize.suggest", err), err
	}
	if req.Save {
		if err := saveOrganizePlan(root, &plan); err != nil {
			return errorProjection("organize.suggest", err), err
		}
	}
	projection := domain.NewProjection("organize.suggest", "Organize suggestions generated.")
	if len(plan.Operations) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["plan_id"] = plan.PlanID
	projection.Facts["operations"] = fmt.Sprint(len(plan.Operations))
	projection.Facts["automatic"] = fmt.Sprint(countOrganizeOperations(plan.Operations, "automatic"))
	projection.Facts["manual_review"] = fmt.Sprint(countOrganizeOperations(plan.Operations, "manual_review"))
	projection.Facts["risk.low"] = fmt.Sprint(countOrganizeRisks(plan.Operations, "low"))
	projection.Facts["risk.medium"] = fmt.Sprint(countOrganizeRisks(plan.Operations, "medium"))
	projection.Facts["risk.review"] = fmt.Sprint(countOrganizeRisks(plan.Operations, "review"))
	if plan.SavedPath != "" {
		projection.Facts["saved_path"] = plan.SavedPath
		projection.Evidence = []string{plan.SavedPath}
	}
	projection.Data = plan
	if plan.SavedPath == "" && len(plan.Operations) > 0 {
		projection.Actions = []domain.Action{{Name: "save", Command: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}}
	} else if plan.SavedPath != "" && len(plan.Operations) > 0 {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes --snapshot-message %s", shellQuote(root), shellQuote(plan.PlanID), shellQuote("snapshot before organize"))}}
	}
	return projection, nil
}

func (s *Service) ListOrganizePlans(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.list", err), err
	}
	plans, err := listOrganizePlans(root)
	if err != nil {
		return errorProjection("organize.list", err), err
	}
	projection := domain.NewProjection("organize.list", "Organize plans listed.")
	projection.Facts["plans"] = fmt.Sprint(len(plans))
	projection.Data = map[string]any{"plans": plans}
	if len(plans) == 0 {
		projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}}
	} else {
		projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax organize apply --vault %s --plan %s --yes --snapshot-message %s", shellQuote(root), shellQuote(plans[0].PlanID), shellQuote("snapshot before organize"))}}
	}
	return projection, nil
}

func (s *Service) ApplyOrganize(ctx context.Context, req ApplyRequest) (domain.Projection, error) {
	if !req.Yes {
		vault := strings.TrimSpace(req.VaultPath)
		if vault == "" {
			vault = "."
		}
		if root, err := cleanVaultPath(vault); err == nil {
			vault = root
		}
		hint := fmt.Sprintf("Run pinax organize plan --vault %s --save first, review the plan, then run pinax organize apply --vault %s --plan <plan_id> --yes --snapshot-message %s", shellQuote(vault), shellQuote(vault), shellQuote("snapshot before organize"))
		err := &domain.CommandError{Code: "approval_required", Message: "organize apply requires --yes", Hint: hint}
		return domain.NewErrorProjection("organize.apply", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("organize.apply", err), err
	}
	var savedPlan *domain.OrganizePlan
	if strings.TrimSpace(req.PlanID) != "" {
		plan, err := loadOrganizePlan(root, req.PlanID)
		if err != nil {
			return errorProjection("organize.apply", err), err
		}
		if err := ensureOrganizePlanFresh(ctx, root, &plan); err != nil {
			projection := errorProjection("organize.apply", err)
			projection.Actions = []domain.Action{{Name: "replan", Command: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}}
			projection.Data = map[string]any{"plan_id": plan.PlanID}
			return projection, err
		}
		savedPlan = &plan
	}
	beforeBindingsByPath, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return errorProjection("organize.apply", err), err
	}
	beforeBindings := managedBindingsByObject(beforeBindingsByPath)
	snapshotID := ""
	if req.SnapshotMessage != "" {
		if _, err := s.GitSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: req.SnapshotMessage}); err != nil {
			return errorProjection("organize.apply", err), err
		}
		snapshotID = filepath.ToSlash(filepath.Join(".pinax", "last_snapshot"))
	}
	if !gitstore.HasSnapshot(root) {
		err := &domain.CommandError{Code: "snapshot_required", Message: "Organizing structure requires an explicit version snapshot first", Hint: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before organize"))}
		projection := domain.NewErrorProjection("organize.apply", err)
		projection.Actions = []domain.Action{{Name: "snapshot", Command: err.Hint}}
		return projection, err
	}
	ops, err := organizeApplyOperations(root, savedPlan)
	if err != nil {
		return errorProjection("organize.apply", err), err
	}
	appliedMetadata := 0
	changedPaths := make([]string, 0)
	for _, op := range ops {
		if op.Status != "planned" || (op.Kind != "tag_patch" && op.Kind != "status_patch") {
			continue
		}
		if err := applyOrganizeMetadataOperation(root, op); err != nil {
			return errorProjection("organize.apply", err), err
		}
		appliedMetadata++
		changedPaths = append(changedPaths, op.Path)
		_ = appendEvent(root, "organize.apply", "success", map[string]string{"kind": op.Kind, "path": op.Path})
	}
	appliedMoves := 0
	skipped := 0
	for _, op := range ops {
		if op.Kind == "tag_patch" || op.Kind == "status_patch" {
			continue
		}
		if op.Kind != "move" || op.Status != "planned" {
			skipped++
			continue
		}
		source, err := safeJoin(root, op.Path)
		if err != nil {
			return errorProjection("organize.apply", err), err
		}
		target, err := safeJoin(root, op.Target)
		if err != nil {
			return errorProjection("organize.apply", err), err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return errorProjection("organize.apply", err), err
		}
		if err := os.Rename(source, target); err != nil {
			return errorProjection("organize.apply", err), err
		}
		content, readErr := os.ReadFile(target)
		if readErr != nil {
			return errorProjection("organize.apply", readErr), readErr
		}
		movedNote := parseNote(op.Target, string(content))
		if movedNote.ID != "" {
			if _, recordErr := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteMoved, "organize.move:"+movedNote.ID+":"+op.Path+":"+op.Target, movedNote, op.Path); recordErr != nil {
				return errorProjection("organize.apply", recordErr), recordErr
			}
		}
		changedPaths = append(changedPaths, op.Path, op.Target)
		appliedMoves++
		_ = appendEvent(root, "organize.apply", "success", map[string]string{"from": op.Path, "to": op.Target})
	}
	if savedPlan != nil {
		_ = refreshIndex(root)
	}
	projection := domain.NewProjection("organize.apply", "Organize structure applied.")
	if savedPlan != nil {
		projection.Facts["plan_id"] = savedPlan.PlanID
	}
	projection.Facts["applied_moves"] = fmt.Sprint(appliedMoves)
	projection.Facts["applied_metadata"] = fmt.Sprint(appliedMetadata)
	projection.Facts["applied"] = fmt.Sprint(appliedMoves + appliedMetadata)
	projection.Facts["skipped"] = fmt.Sprint(skipped)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
	planID := ""
	if savedPlan != nil {
		planID = savedPlan.PlanID
	}
	receipt, receiptErr := writeObjectApplyReceipt(ctx, root, "organize.apply", planID, snapshotID, beforeBindings, changedPaths)
	if receiptErr != nil {
		return errorProjection("organize.apply", receiptErr), receiptErr
	}
	addApplyReceiptProjection(&projection, receipt)
	projection.Data = map[string]any{"applied_moves": appliedMoves, "applied_metadata": appliedMetadata, "skipped": skipped, "receipt": receipt}
	return projection, nil
}

func applyOrganizeMetadataOperation(root string, op domain.PlanOperation) error {
	fields := map[string]string{}
	switch op.Kind {
	case "tag_patch":
		tags, err := normalizeTagsForWrite(strings.Split(op.Target, ","))
		if err != nil {
			return err
		}
		fields["tags"] = formatTags(tags)
	case "status_patch":
		fields["status"] = op.Target
	default:
		return nil
	}
	return applyRepairFrontmatterPatch(root, op.Path, fields)
}

func lifecycleFeedbackID(assetID, lifecycle, reason string) string {
	sum := sha1.Sum([]byte(assetID + "\x00" + lifecycle + "\x00" + reason))
	return "lifecycle_" + assetID + "_" + hex.EncodeToString(sum[:])[:12]
}

func (s *Service) VersionStatus(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.status", err), err
	}
	status, err := s.versionBackend.Status(context.Background(), pinaxversion.StatusRequest{Root: root})
	if err != nil {
		return errorProjection("version.status", err), err
	}
	projection := domain.NewProjection("version.status", "Version backend status checked.")
	projection.Facts["version_backend"] = status.Backend
	projection.Facts["snapshot_supported"] = fmt.Sprint(status.Capabilities.SnapshotSupported)
	projection.Facts["changed_paths_supported"] = fmt.Sprint(status.Capabilities.ChangedPathsSupported)
	projection.Facts["read_at_revision_supported"] = fmt.Sprint(status.Capabilities.ReadAtRevision)
	projection.Facts["diff_supported"] = fmt.Sprint(status.Capabilities.DiffSupported)
	projection.Facts["worktree_state"] = status.WorktreeState
	if status.CurrentRevision != "" {
		projection.Facts["current_revision"] = status.CurrentRevision
	}
	if status.LastSnapshotID != "" {
		projection.Facts["last_snapshot_id"] = status.LastSnapshotID
	}
	if status.LastSnapshotAt != "" {
		projection.Facts["last_snapshot_at"] = status.LastSnapshotAt
	}
	projection.Actions = []domain.Action{{Name: "snapshot", Command: fmt.Sprintf("pinax version snapshot --vault %s --message %s", shellQuote(root), shellQuote("snapshot before organize"))}}
	projection.Data = status
	return projection, nil
}

func (s *Service) VersionBackends(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.backends", err), err
	}
	backends := pinaxversion.AvailableBackends()
	projection := domain.NewProjection("version.backends", "Version backends listed.")
	projection.Facts["active_backend"] = "local"
	projection.Facts["backends"] = fmt.Sprint(len(backends))
	for i, backend := range backends {
		prefix := fmt.Sprintf("backend.%d.", i+1)
		projection.Facts[prefix+"name"] = backend.Name
		projection.Facts[prefix+"active"] = fmt.Sprint(backend.Active)
		projection.Facts[prefix+"snapshot_supported"] = fmt.Sprint(backend.Capabilities.SnapshotSupported)
	}
	projection.Evidence = []string{root}
	projection.Data = map[string]any{"backends": backends}
	return projection, nil
}

func (s *Service) VersionSnapshot(ctx context.Context, req SnapshotRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("version.snapshot", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("version.snapshot", err), err
	}
	if strings.TrimSpace(req.Message) == "" {
		err := &domain.CommandError{Code: "message_required", Message: "version snapshot requires --message", Hint: "Rerun and provide --message"}
		return domain.NewErrorProjection("version.snapshot", err), err
	}
	snapshot, err := s.versionBackend.Snapshot(ctx, pinaxversion.SnapshotRequest{Root: root, Message: req.Message})
	if err != nil {
		return errorProjection("version.snapshot", err), err
	}
	_ = appendEvent(root, "version.snapshot", "success", map[string]string{"snapshot_id": snapshot.SnapshotID})
	projection := domain.NewProjection("version.snapshot", "Version snapshot recorded.")
	projection.Facts["snapshot_id"] = snapshot.SnapshotID
	projection.Facts["version_backend"] = snapshot.Backend
	projection.Facts["message"] = snapshot.Message
	projection.Facts["files"] = fmt.Sprint(snapshot.Files)
	projection.Facts["bytes"] = fmt.Sprint(snapshot.Bytes)
	projection.Facts["content_hash"] = snapshot.ContentHash
	projection.Evidence = snapshot.Evidence
	projection.Data = map[string]any{"snapshot": snapshot}
	return projection, nil
}

type SnapshotRequest struct {
	VaultPath string
	Message   string
}

func (s *Service) GitSnapshot(ctx context.Context, req SnapshotRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("git.snapshot", err), err
	}
	if req.Message == "" {
		err := &domain.CommandError{Code: "message_required", Message: "Git snapshot requires --message", Hint: "Rerun and provide --message"}
		return domain.NewErrorProjection("git.snapshot", err), err
	}
	if err := gitstore.Snapshot(ctx, root, req.Message); err != nil {
		return errorProjection("git.snapshot", err), err
	}
	projection := domain.NewProjection("git.snapshot", "Git snapshot recorded.")
	projection.Facts["vault"] = root
	projection.Facts["message"] = req.Message
	projection.Evidence = []string{".pinax/last_snapshot"}
	return projection, nil
}
func appendDailyIndex(root string, note domain.Note) (string, error) {
	date := currentTimeUTC().Format("2006-01-02")
	root, rel, _, err := ensureJournalNote(root, DailyRequest{Date: date})
	if err != nil {
		return "", err
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return rel, err
	}
	content := string(contentBytes)
	blocks, err := templateengine.InspectManagedBlocks(content)
	if err != nil {
		return rel, err
	}
	capture := ""
	for _, block := range blocks {
		if block.Name == "daily-captures" {
			capture = content[block.ContentStart:block.ContentEnd]
			break
		}
	}
	if capture == "" {
		return rel, &templateengine.Error{Code: "managed_block_missing", Message: "daily-captures managed block is missing"}
	}
	if strings.Contains(capture, note.Path) {
		return rel, nil
	}
	line := strings.TrimSpace(dailyIndexLine(note))
	replacement := line
	if existing := strings.TrimSpace(capture); existing != "" {
		replacement = existing + "\n" + line
	}
	updated, err := templateengine.ReplaceManagedBlock(content, "daily-captures", replacement)
	if err != nil {
		return rel, err
	}
	// 缺失、重复或未闭合托管区块时上面已经 fail closed；这里只写回明确的 daily-captures 区块内容。
	return rel, os.WriteFile(path, []byte(updated), 0o644)
}

func ensureJournalNote(vaultPath string, req DailyRequest) (string, string, string, error) {
	return NewService().ensureJournalNote(vaultPath, "daily", req)
}

func (s *Service) ensureJournalNote(vaultPath, period string, req DailyRequest) (string, string, string, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", "", "", err
	}
	if err := ensureVaultAssets(root); err != nil {
		return "", "", "", err
	}
	date, err := journalDate(period, req)
	if err != nil {
		return "", "", "", err
	}
	key := journalKey(period, date)
	templateName := journalTemplateName(period, req)
	rel, body, err := journalTemplateRender(root, templateName, period, key)
	if err != nil {
		return "", "", "", err
	}
	if rel == "" {
		rel = filepath.ToSlash(filepath.Join(period, key+".md"))
	}
	for _, candidate := range []string{rel, filepath.ToSlash(filepath.Join("notes", period, key+".md"))} {
		path, err := safeJoin(root, candidate)
		if err != nil {
			return "", "", "", err
		}
		exists, err := existingJournalNoteCandidate(path, candidate)
		if err != nil {
			return "", "", "", err
		}
		if exists {
			return root, candidate, key, nil
		}
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	title := journalTitle(period, key)
	journalObjectID, err := s.allocateObjectID(identity.KindNote, root, rel)
	if err != nil {
		return "", "", "", err
	}
	content := buildNoteContentWithObjectID(journalObjectID, title, "", period, period, []string{period}, "journal", now, body)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", "", "", err
	}
	if err := refreshIndex(root); err != nil {
		return "", "", "", err
	}
	journalNote := parseNote(rel, content)
	if _, err := appendNoteRecordEvent(context.Background(), root, domain.RecordEventNoteCreated, "journal.create:"+journalNote.ID+":"+rel, journalNote, ""); err != nil {
		return "", "", "", err
	}
	_ = appendEvent(root, period+".create", "success", map[string]string{"path": rel, "template": templateName})
	return root, rel, key, nil
}

func existingJournalNoteCandidate(path, rel string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	// legacy `notes/daily/*` 只有真正的 journal note 才复用；旧 daily index 是系统导航页，不能阻止根目录 daily note 创建。
	if strings.HasPrefix(filepath.ToSlash(rel), "notes/daily/") {
		content, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		meta, _ := splitFrontmatter(string(content))
		if isPinaxNoteFrontmatter(meta) && isSystemIndexNote(parseNote(rel, string(content))) {
			return false, nil
		}
	}
	return true, nil
}

func journalTemplateName(period string, req DailyRequest) string {
	if name := strings.TrimSpace(req.Template); name != "" {
		return name
	}
	return "journal." + period
}

func journalTemplateRender(root, templateName, period, key string) (string, string, error) {
	body, err := loadTemplate(root, templateName)
	if err != nil {
		return "", "", err
	}
	doc, err := templateengine.ParseDocument(templateName, body)
	if err != nil {
		return "", "", templateEngineCommandError(err)
	}
	rel := journalPathFromPattern(doc.Metadata.Output.PathPattern, period, key)
	title := journalTitle(period, key)
	rendered, err := templateengine.New().Render(doc, templateengine.Context{Title: title, Date: key, Vars: map[string]string{"date": key}})
	if err != nil {
		return "", "", templateEngineCommandError(err)
	}
	return rel, rendered.Body, nil
}

func journalPathFromPattern(pattern, period, key string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return filepath.ToSlash(filepath.Join(period, key+".md"))
	}
	for _, token := range []string{"{{ .Date }}", "{{.Date}}", "{{ .Week }}", "{{.Week}}", "{{ .Month }}", "{{.Month}}"} {
		pattern = strings.ReplaceAll(pattern, token, key)
	}
	return filepath.ToSlash(pattern)
}

func journalDate(period string, req DailyRequest) (time.Time, error) {
	date := time.Now().UTC()
	if value := strings.TrimSpace(req.Date); value != "" {
		parsed, err := parseJournalDateValue(period, value)
		if err != nil {
			return time.Time{}, &domain.CommandError{Code: "invalid_journal_date", Message: "journal date must be YYYY-MM-DD, YYYY-Www, or YYYY-MM", Hint: "Use --date 2026-06-06, --date 2026-W23, or --date 2026-06"}
		}
		date = parsed.UTC()
	}
	if req.Prev {
		date = shiftJournalDate(period, date, -1)
	}
	if req.Next {
		date = shiftJournalDate(period, date, 1)
	}
	return date, nil
}

func shiftJournalDate(period string, date time.Time, direction int) time.Time {
	switch period {
	case "weekly":
		return date.AddDate(0, 0, direction*7)
	case "monthly":
		return date.AddDate(0, direction, 0)
	default:
		return date.AddDate(0, 0, direction)
	}
}

func parseJournalDateValue(period, value string) (time.Time, error) {
	switch period {
	case "weekly":
		if date, err := parseJournalISOWeek(value); err == nil {
			return date, nil
		}
	case "monthly":
		if date, err := time.Parse("2006-01", value); err == nil {
			return date, nil
		}
	}
	return parseUserDate(value)
}

func parseJournalISOWeek(value string) (time.Time, error) {
	var year int
	var week int
	if _, err := fmt.Sscanf(value, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, err
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()+6)%7)
	return monday.AddDate(0, 0, (week-1)*7), nil
}

func journalKey(period string, date time.Time) string {
	switch period {
	case "weekly":
		year, week := date.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)
	case "monthly":
		return date.Format("2006-01")
	default:
		return date.Format("2006-01-02")
	}
}

func journalTitle(period, key string) string {
	if period == "daily" {
		return "Daily-" + key
	}
	return journalLabel(period) + " " + key
}

func journalNoteShellFriendlyAlias(note domain.Note) string {
	path := filepath.ToSlash(strings.TrimPrefix(note.Path, "notes/"))
	if !strings.HasPrefix(path, "daily/") || filepath.Ext(path) != ".md" {
		return ""
	}
	key := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if _, err := time.Parse("2006-01-02", key); err != nil {
		return ""
	}
	if note.Title == "Daily "+key || note.Title == "Daily-"+key {
		return "Daily-" + key
	}
	return ""
}

func journalLabel(period string) string {
	switch period {
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	default:
		return "Daily"
	}
}

func appendFile(path, text string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func dailyIndexLine(note domain.Note) string {
	parts := []string{"- " + note.Path}
	for _, tag := range cleanTags(note.Tags) {
		parts = append(parts, "#"+tag)
	}
	if note.Project != "" {
		parts = append(parts, "group="+note.Project)
	}
	if note.Folder != "" {
		parts = append(parts, "folder="+note.Folder)
	}
	if note.Kind != "" {
		parts = append(parts, "kind="+note.Kind)
	}
	if note.Status != "" {
		parts = append(parts, "status="+note.Status)
	}
	return strings.Join(parts, " | ") + "\n"
}

func saveProjectRegistryProjection(root string, registry domain.ProjectRegistry, project domain.Project, created bool) (domain.Projection, error) {
	if err := saveProjectRegistry(root, registry); err != nil {
		return errorProjection("project.create", err), err
	}
	status := "updated"
	if created {
		status = "created"
	}
	_ = appendEvent(root, "project.create", "success", map[string]string{"project": project.Slug, "status": status})
	projection := domain.NewProjection("project.create", "Project created.")
	projection.Facts["project"] = project.Slug
	projection.Facts["name"] = project.Name
	projection.Facts["notes_prefix"] = project.NotesPrefix
	projection.Facts["current_project"] = registry.CurrentProject
	projection.Data = map[string]any{"project": project, "registry": registry}
	projection.Actions = []domain.Action{{Name: "switch", Command: fmt.Sprintf("pinax project switch %s --vault %s", shellQuote(project.Slug), shellQuote(root))}}
	return projection, nil
}

func loadProjectRegistry(root string) (domain.ProjectRegistry, error) {
	registry := domain.ProjectRegistry{SchemaVersion: "pinax.projects.v1", Projects: []domain.Project{}}
	path := filepath.Join(root, ".pinax", "projects.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		return registry, err
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = "pinax.projects.v1"
	}
	if registry.Projects == nil {
		registry.Projects = []domain.Project{}
	}
	return registry, nil
}

func saveProjectRegistry(root string, registry domain.ProjectRegistry) error {
	registry.SchemaVersion = "pinax.projects.v1"
	if registry.Projects == nil {
		registry.Projects = []domain.Project{}
	}
	sort.Slice(registry.Projects, func(i, j int) bool { return registry.Projects[i].Slug < registry.Projects[j].Slug })
	return writeJSONAsset(filepath.Join(root, ".pinax", "projects.json"), registry)
}

func validateProjectSlug(slug string) error {
	if slug == "" {
		return &domain.CommandError{Code: "project_slug_required", Message: "Project requires a slug", Hint: "Run pinax project create <slug> --name <name>"}
	}
	for _, r := range slug {
		if unicode.IsLower(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return &domain.CommandError{Code: "invalid_project_slug", Message: "Project slug may only contain lowercase letters, numbers, -, and _", Hint: "For example, pinax project create research"}
	}
	return nil
}

func validateProjectPrefix(prefix string) error {
	clean := filepath.ToSlash(filepath.Clean(prefix))
	if clean == "." || filepath.IsAbs(prefix) || strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, ".pinax") {
		return &domain.CommandError{Code: "unsafe_project_prefix", Message: "Project notes prefix must be inside the vault and must not point to .pinax", Hint: "Use a prefix like notes/research"}
	}
	return nil
}

func loadStorageProfile(root string) (domain.StorageProfile, error) {
	defaultProfile := domain.StorageProfile{SchemaVersion: "pinax.storage.v1", Backend: "local", Local: &domain.LocalStorage{Root: root}}
	path := filepath.Join(root, ".pinax", "storage.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultProfile, nil
	}
	if err != nil {
		return domain.StorageProfile{}, err
	}
	profile := domain.StorageProfile{}
	if err := json.Unmarshal(b, &profile); err != nil {
		return domain.StorageProfile{}, err
	}
	if profile.SchemaVersion == "" {
		profile.SchemaVersion = "pinax.storage.v1"
	}
	if profile.Backend == "" {
		profile.Backend = "local"
	}
	return profile, nil
}

func saveStorageProfile(root string, profile domain.StorageProfile) error {
	profile.SchemaVersion = "pinax.storage.v1"
	return writeJSONAsset(filepath.Join(root, ".pinax", "storage.json"), profile)
}

func storageProjection(command, summary string, profile domain.StorageProfile) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["backend"] = profile.Backend
	switch profile.Backend {
	case "s3":
		if profile.S3 != nil {
			projection.Facts["bucket"] = profile.S3.Bucket
			projection.Facts["region"] = profile.S3.Region
			if profile.S3.Prefix != "" {
				projection.Facts["prefix"] = profile.S3.Prefix
			}
			if profile.S3.Endpoint != "" {
				projection.Facts["endpoint"] = profile.S3.Endpoint
			}
			credentialSource := "environment"
			if profile.S3.Profile != "" {
				credentialSource = "profile:" + profile.S3.Profile
			}
			projection.Facts["credential_source"] = credentialSource
		}
	case "local":
		if profile.Local != nil {
			projection.Facts["root"] = profile.Local.Root
		}
	}
	projection.Data = map[string]any{"storage": profile, "network_checked": false}
	return projection
}

func writeJSONAsset(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func ensureVaultAssets(root string) error {
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		return err
	}
	return ensureEventLog(root)
}

func planOrganize(root string) ([]domain.PlanOperation, error) {
	notes, err := scanNotes(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, note := range notes {
		seen[note.Path] = note.Path
	}
	ops := make([]domain.PlanOperation, 0)
	for _, note := range notes {
		if !isOrganizeRootNoteCandidate(note.Path) {
			continue
		}
		slug := slugify(note.Title)
		if slug == "" {
			slug = strings.TrimSuffix(strings.ToLower(filepath.Base(note.Path)), filepath.Ext(note.Path))
		}
		target := filepath.ToSlash(filepath.Join("notes", slug+".md"))
		if note.Path == target {
			continue
		}
		if existing, ok := seen[target]; ok && existing != note.Path {
			ops = append(ops, domain.PlanOperation{Kind: "move", Path: note.Path, Target: target, Reason: "Target path already exists", Status: "conflict"})
			continue
		}
		ops = append(ops, domain.PlanOperation{Kind: "move", Path: note.Path, Target: target, Reason: "Place under notes/ by title", Status: "planned"})
	}
	return ops, nil
}

func buildOrganizePlan(root string) (domain.OrganizePlan, error) {
	ops, err := planOrganize(root)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	facts = organizeCandidateFacts(facts)
	ops = append(ops, organizeFactOperations(root, facts)...)
	created := time.Now().UTC()
	planID := organizePlanID(root, ops, created)
	plan := domain.OrganizePlan{
		SchemaVersion: "pinax.organize_plan.v1",
		PlanID:        planID,
		CreatedAt:     created.Format(time.RFC3339),
		ExpiresAt:     created.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		VaultRoot:     root,
		SourceCommand: fmt.Sprintf("pinax organize plan --vault %s", shellQuote(root)),
		SourceFacts:   organizeSourceFacts(facts),
		Operations:    make([]domain.OrganizeOperation, 0, len(ops)),
		Status:        "planned",
	}
	for _, op := range ops {
		plan.Operations = append(plan.Operations, organizeOperationFromPlan(planID, op))
	}
	if err := bindOrganizePlanObjects(context.Background(), root, &plan); err != nil {
		return domain.OrganizePlan{}, err
	}
	return plan, nil
}

func organizeCandidateFacts(facts []noteFact) []noteFact {
	candidates := make([]noteFact, 0, len(facts))
	for _, fact := range facts {
		if isOrganizeFactCandidate(fact.rel) {
			candidates = append(candidates, fact)
		}
	}
	return candidates
}

func isOrganizeFactCandidate(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if strings.HasPrefix(rel, "notes/") {
		return true
	}
	return isOrganizeRootNoteCandidate(rel)
}

func isOrganizeRootNoteCandidate(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || strings.Contains(rel, "/") || !strings.EqualFold(filepath.Ext(rel), ".md") {
		return false
	}
	switch strings.ToLower(filepath.Base(rel)) {
	case "agents.md", "claude.md", "readme.md", "license.md", "contributing.md":
		return false
	default:
		return true
	}
}

func organizeOperationFromPlan(planID string, op domain.PlanOperation) domain.OrganizeOperation {
	mode := "manual_review"
	risk := "review"
	if op.Status == "planned" && (op.Kind == "move" || op.Kind == "tag_patch" || op.Kind == "status_patch") {
		mode = "automatic"
		risk = "low"
	}
	before := map[string]string{"path": op.Path}
	after := map[string]string{"path": op.Target}
	switch op.Kind {
	case "tag_patch":
		before = map[string]string{"tags": ""}
		after = map[string]string{"tags": op.Target}
	case "kind_patch":
		before = map[string]string{"kind": ""}
		after = map[string]string{"kind": op.Target}
	case "status_patch":
		before = map[string]string{"status": ""}
		after = map[string]string{"status": op.Target}
	case "link_resolution":
		before = map[string]string{"link_target": op.Target}
		after = map[string]string{"resolution": "manual"}
	case "link_rewrite":
		before = map[string]string{"link_target": op.Target}
		after = map[string]string{"rewrite": "manual"}
	case "orphan_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": "orphan"}
	case "attachment_repair":
		before = map[string]string{"attachment": op.Target}
		after = map[string]string{"repair": "manual"}
	case "manual_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": "required"}
	case "source_move":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"path": op.Target}
	case "source_review":
		before = map[string]string{"path": op.Path}
		after = map[string]string{"review": op.Target}
	}
	evidence := op.Evidence
	if len(evidence) == 0 {
		evidence = []string{"path=" + op.Path, "target=" + op.Target}
	}
	return domain.OrganizeOperation{
		OperationID: organizeOperationID(planID, op),
		Kind:        op.Kind,
		Mode:        mode,
		Risk:        risk,
		Path:        op.Path,
		Target:      op.Target,
		Before:      before,
		After:       after,
		Reason:      op.Reason,
		Evidence:    evidence,
		Status:      op.Status,
	}
}

func organizeFactOperations(root string, facts []noteFact) []domain.PlanOperation {
	notes := notesFromFacts(facts)
	outgoing, incoming := BuildEnhancedLinkGraph(notes)
	ops := make([]domain.PlanOperation, 0)
	for _, fact := range facts {
		if info, ok := durableSourceInfo(fact.note); ok {
			target := durableSourceTargetPath(info)
			if target != "" && fact.rel != target {
				ops = append(ops, domain.PlanOperation{Kind: "source_move", Path: fact.rel, Target: target, Reason: "Move durable GitHub source note under sources/github/", Status: "manual_review", Evidence: []string{"source_url=" + info.URL, "repo=" + info.Owner + "/" + info.Repo}})
			}
			if missing := missingDurableSourceSections(fact.note.Body); len(missing) > 0 {
				ops = append(ops, domain.PlanOperation{Kind: "source_review", Path: fact.rel, Target: strings.Join(missing, ","), Reason: "Missing durable source sections", Status: "manual_review", Evidence: []string{"source_url=" + info.URL, "missing=" + strings.Join(missing, ",")}})
			}
		}
		inlineTags := cleanTags(noteAllTags(fact.note))
		if len(fact.note.Tags) == 0 && len(inlineTags) > 0 {
			ops = append(ops, domain.PlanOperation{Kind: "tag_patch", Path: fact.rel, Target: strings.Join(inlineTags, ","), Reason: "Add frontmatter tags from inline body tags", Status: "planned"})
		}
		if strings.TrimSpace(fact.note.Kind) == "" {
			ops = append(ops, domain.PlanOperation{Kind: "kind_patch", Path: fact.rel, Target: inferNoteKind(fact.note), Reason: "Missing kind classification; confirmation required", Status: "manual_review"})
		}
		if strings.TrimSpace(fact.note.Status) == "" {
			ops = append(ops, domain.PlanOperation{Kind: "status_patch", Path: fact.rel, Target: "active", Reason: "Missing status; active is recommended", Status: "planned"})
		}
		for _, link := range outgoing[fact.rel] {
			switch {
			case link.Status == string(domain.LinkStatusBroken) || link.Broken:
				ops = append(ops, domain.PlanOperation{Kind: "link_resolution", Path: fact.rel, Target: link.Target, Reason: "Unresolved link requires manual target confirmation", Status: "manual_review", Evidence: linkEvidence(link)})
			case link.Status == string(domain.LinkStatusAmbiguous):
				ops = append(ops, domain.PlanOperation{Kind: "link_rewrite", Path: fact.rel, Target: link.Target, Reason: "Link target has multiple candidates; manually confirm body wording", Status: "manual_review", Evidence: linkEvidence(link)})
			}
		}
		for _, attachment := range noteAttachmentsFromBody(root, fact.note) {
			if !attachment.Exists {
				ops = append(ops, domain.PlanOperation{Kind: "attachment_repair", Path: fact.rel, Target: attachment.TargetPath, Reason: "Attachment reference is missing and needs repair or removal", Status: "manual_review"})
			}
		}
		if len(outgoing[fact.rel]) == 0 && len(incoming[fact.rel]) == 0 {
			ops = append(ops, domain.PlanOperation{Kind: "orphan_review", Path: fact.rel, Target: fact.note.Title, Reason: "Note has no bidirectional links; manually confirm archive or add context", Status: "manual_review", Evidence: []string{"title=" + fact.note.Title, "graph=incoming:0,outgoing:0"}})
		}
		if !fact.hasFrontmatter {
			ops = append(ops, domain.PlanOperation{Kind: "manual_review", Path: fact.rel, Target: "frontmatter", Reason: "Missing Pinax frontmatter; metadata confirmation required", Status: "manual_review"})
		}
	}
	return ops
}

func inferNoteKind(note domain.Note) string {
	path := strings.ToLower(note.Path)
	for _, tag := range noteAllTags(note) {
		switch strings.ToLower(tag) {
		case "daily":
			return "daily"
		case "meeting":
			return "meeting"
		case "project":
			return "project"
		}
	}
	if strings.Contains(path, "daily/") {
		return "daily"
	}
	return "reference"
}

type durableSourceCandidate struct {
	URL   string
	Owner string
	Repo  string
}

var githubRepoURLPattern = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(?:[/?#][^\s)\]]*)?`)

func durableSourceInfo(note domain.Note) (durableSourceCandidate, bool) {
	if value := strings.TrimSpace(note.Frontmatter["source_url"]); value != "" {
		if candidate, ok := parseGitHubRepoURL(value); ok {
			return candidate, true
		}
	}
	if candidate, ok := firstGitHubRepoURL(note.Body); ok {
		return candidate, true
	}
	if candidate, ok := ownerRepoFromTitle(note.Title); ok && durableSourceTagSignal(note) {
		candidate.URL = "https://github.com/" + candidate.Owner + "/" + candidate.Repo
		return candidate, true
	}
	return durableSourceCandidate{}, false
}

func durableSourceMetadataOperations(note domain.Note) []domain.PlanOperation {
	info, ok := durableSourceInfo(note)
	if !ok {
		return nil
	}
	targets := []string{"source_url=" + info.URL}
	if strings.TrimSpace(note.Kind) != "source" {
		targets = append(targets, "kind=source")
	}
	tags := durableSourceRecommendedTags(note)
	if len(tags) > 0 {
		targets = append(targets, "tags="+strings.Join(tags, ","))
	}
	for _, key := range []string{"last_checked_at", "source_license", "review_after"} {
		if strings.TrimSpace(note.Frontmatter[key]) == "" {
			targets = append(targets, key+"=<review>")
		}
	}
	return []domain.PlanOperation{{Kind: "source_metadata", Path: note.Path, Target: strings.Join(targets, ";"), Reason: "Review durable source metadata for external GitHub reference", Status: "manual_review", Evidence: []string{"source_url=" + info.URL, "repo=" + info.Owner + "/" + info.Repo}}}
}

func durableSourceRecommendedTags(note domain.Note) []string {
	recommended := []string{"source/github", "reference/source"}
	existing := map[string]bool{}
	for _, tag := range noteAllTags(note) {
		existing[strings.ToLower(tag)] = true
	}
	missing := make([]string, 0, len(recommended))
	for _, tag := range recommended {
		if !existing[strings.ToLower(tag)] {
			missing = append(missing, tag)
		}
	}
	return missing
}

func durableSourceTargetPath(info durableSourceCandidate) string {
	slug := slugify(info.Owner + "-" + info.Repo)
	if slug == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join("sources", "github", slug+".md"))
}

func firstGitHubRepoURL(body string) (durableSourceCandidate, bool) {
	match := githubRepoURLPattern.FindStringSubmatch(body)
	if len(match) < 3 {
		return durableSourceCandidate{}, false
	}
	raw := match[0]
	if candidate, ok := parseGitHubRepoURL(raw); ok {
		return candidate, true
	}
	return durableSourceCandidate{URL: "https://github.com/" + match[1] + "/" + strings.TrimSuffix(match[2], ".git"), Owner: match[1], Repo: strings.TrimSuffix(match[2], ".git")}, true
}

func parseGitHubRepoURL(raw string) (durableSourceCandidate, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
		return durableSourceCandidate{}, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return durableSourceCandidate{}, false
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	return durableSourceCandidate{URL: "https://github.com/" + parts[0] + "/" + repo, Owner: parts[0], Repo: repo}, true
}

func ownerRepoFromTitle(title string) (durableSourceCandidate, bool) {
	parts := strings.Split(strings.TrimSpace(title), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(title, " \t\n") {
		return durableSourceCandidate{}, false
	}
	return durableSourceCandidate{Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}, true
}

func durableSourceTagSignal(note domain.Note) bool {
	for _, tag := range noteAllTags(note) {
		switch strings.ToLower(tag) {
		case "github", "source", "reference", "repo", "source/github", "reference/source":
			return true
		}
	}
	return false
}

func missingDurableSourceSections(body string) []string {
	required := []string{"Use decision", "Risk and boundary", "Verification", "Related notes"}
	lower := strings.ToLower(body)
	missing := make([]string, 0, len(required))
	for _, section := range required {
		if !strings.Contains(lower, strings.ToLower(section)) {
			missing = append(missing, section)
		}
	}
	return missing
}

func organizeSourceFacts(facts []noteFact) map[string]string {
	source := map[string]string{"notes": fmt.Sprint(len(facts))}
	for _, fact := range facts {
		path := "note." + fact.rel
		source[path+".mtime"] = fact.modTime.UTC().Format(time.RFC3339Nano)
		source[path+".size"] = fmt.Sprint(fact.size)
		source[path+".sha1"] = noteFactHash(fact)
	}
	return source
}

func organizePlanID(root string, ops []domain.PlanOperation, created time.Time) string {
	parts := []string{root, created.Format(time.RFC3339Nano)}
	for _, op := range ops {
		parts = append(parts, op.Kind, op.Path, op.Target, op.Status)
	}
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "organize-" + hex.EncodeToString(h[:])[:12]
}

func organizeOperationID(planID string, op domain.PlanOperation) string {
	h := sha1.Sum([]byte(planID + "\x00" + op.Kind + "\x00" + op.Path + "\x00" + op.Target))
	return "op-" + hex.EncodeToString(h[:])[:12]
}

func saveOrganizePlan(root string, plan *domain.OrganizePlan) error {
	dir, err := safeJoin(root, ".pinax/organize-plans")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Join(".pinax", "organize-plans", plan.PlanID+".json"))
	path, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	plan.SavedPath = rel
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func loadOrganizePlan(root, planRef string) (domain.OrganizePlan, error) {
	planRef = strings.TrimSpace(planRef)
	if planRef == "" {
		return domain.OrganizePlan{}, &domain.CommandError{Code: "plan_required", Message: "organize plan id cannot be empty", Hint: "Run pinax organize plan --save to generate a plan"}
	}
	rel := planRef
	if !strings.HasPrefix(rel, ".pinax/organize-plans/") {
		rel = filepath.ToSlash(filepath.Join(".pinax", "organize-plans", planRef+".json"))
	}
	path, err := safeJoin(root, rel)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return domain.OrganizePlan{}, err
	}
	var plan domain.OrganizePlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return domain.OrganizePlan{}, err
	}
	if plan.SchemaVersion != "pinax.organize_plan.v1" {
		return domain.OrganizePlan{}, &domain.CommandError{Code: "organize_plan_schema_invalid", Message: "organize plan schema is not supported", Hint: "Rerun pinax organize plan --save"}
	}
	if plan.SavedPath == "" {
		plan.SavedPath = filepath.ToSlash(rel)
	}
	return plan, nil
}

func listOrganizePlans(root string) ([]domain.OrganizePlanSummary, error) {
	dir, err := safeJoin(root, ".pinax/organize-plans")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.OrganizePlanSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	plans := make([]domain.OrganizePlanSummary, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		plan, err := loadOrganizePlan(root, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if err != nil {
			continue
		}
		plans = append(plans, domain.OrganizePlanSummary{PlanID: plan.PlanID, CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt, Status: plan.Status, Operations: len(plan.Operations), SavedPath: plan.SavedPath})
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].CreatedAt > plans[j].CreatedAt })
	return plans, nil
}

func ensureOrganizePlanFresh(ctx context.Context, root string, plan *domain.OrganizePlan) error {
	if plan == nil {
		return &domain.CommandError{Code: "plan_required", Message: "organize plan is required", Hint: "Rerun pinax organize plan --save"}
	}
	if plan.Status != "planned" {
		return &domain.CommandError{Code: "organize_plan_not_planned", Message: "organize plan status is not applicable", Hint: "Rerun pinax organize plan --save"}
	}
	expires, err := time.Parse(time.RFC3339, plan.ExpiresAt)
	if err == nil && time.Now().UTC().After(expires) {
		return &domain.CommandError{Code: "plan_stale", Message: "organize plan has expired", Hint: "pinax organize plan --vault <vault> --save"}
	}
	// 校验前必须与 buildOrganizePlan 保持同一套候选事实：计划保存时通过
	// organizeCandidateFacts 过滤掉 daily/journal 等非组织候选笔记，这里如果不
	// 同样过滤，任何包含日志的 vault 都会因为 facts 数量不一致被误判为 stale。
	objectBound, err := rebaseOrganizePlanObjects(ctx, root, plan)
	if err != nil {
		return err
	}
	if objectBound {
		return nil
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return err
	}
	current := organizeSourceFacts(organizeCandidateFacts(facts))
	if len(current) != len(plan.SourceFacts) {
		return &domain.CommandError{Code: "plan_stale", Message: "organize plan does not match current vault facts", Hint: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}
	}
	for key, value := range plan.SourceFacts {
		if current[key] != value {
			return &domain.CommandError{Code: "plan_stale", Message: "organize plan does not match current vault facts", Hint: fmt.Sprintf("pinax organize plan --vault %s --save", shellQuote(root))}
		}
	}
	return nil
}

func organizeApplyOperations(root string, plan *domain.OrganizePlan) ([]domain.PlanOperation, error) {
	if plan == nil {
		return planOrganize(root)
	}
	ops := make([]domain.PlanOperation, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		status := op.Status
		if status == "" {
			status = "planned"
		}
		if op.Mode == "manual_review" {
			status = "manual_review"
		}
		ops = append(ops, domain.PlanOperation{Kind: op.Kind, Path: op.Path, Target: op.Target, Reason: op.Reason, Status: status})
	}
	return ops, nil
}

func countOrganizeOperations(ops []domain.OrganizeOperation, mode string) int {
	count := 0
	for _, op := range ops {
		if op.Mode == mode {
			count++
		}
	}
	return count
}

func countOrganizeRisks(ops []domain.OrganizeOperation, risk string) int {
	count := 0
	for _, op := range ops {
		if op.Risk == risk {
			count++
		}
	}
	return count
}

func scanNotes(root string) ([]domain.Note, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return nil, err
	}
	notes := make([]domain.Note, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipVaultWalkDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		meta, _, _ := markdownnote.ParseFrontmatter(content)
		if !isPinaxNoteFrontmatter(meta) {
			return nil
		}
		note := parseNote(filepath.ToSlash(rel), string(content))
		if isSystemIndexNote(note) {
			return nil
		}
		notes = append(notes, note)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	return notes, nil
}

type indexRefreshScanItem struct {
	path       string
	note       domain.Note
	failedPath string
	err        error
}

func scanIndexRefreshNotes(root string) ([]domain.Note, []string, error) {
	root, err := cleanVaultPath(root)
	if err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipVaultWalkDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, nil, nil
	}
	workers := indexRefreshWorkerCount(len(paths))
	jobs := make(chan string)
	results := make(chan indexRefreshScanItem, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				results <- scanIndexRefreshNote(root, path)
			}
		}()
	}
	go func() {
		for _, path := range paths {
			jobs <- path
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	notes := make([]domain.Note, 0, len(paths))
	failedPaths := make([]string, 0)
	var firstErr error
	for item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		if item.failedPath != "" {
			failedPaths = append(failedPaths, item.failedPath)
			continue
		}
		if item.note.Path != "" {
			notes = append(notes, item.note)
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	sort.Strings(failedPaths)
	return notes, failedPaths, nil
}

func scanIndexRefreshNote(root, path string) indexRefreshScanItem {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return indexRefreshScanItem{err: err}
	}
	rel = filepath.ToSlash(rel)
	content, err := os.ReadFile(path)
	if err != nil {
		return indexRefreshScanItem{path: rel, failedPath: rel}
	}
	meta, _, _ := markdownnote.ParseFrontmatter(content)
	if !isPinaxNoteFrontmatter(meta) {
		return indexRefreshScanItem{path: rel}
	}
	note := parseNote(rel, string(content))
	if isSystemIndexNote(note) || isSystemJournalNote(note) {
		return indexRefreshScanItem{path: rel}
	}
	if strings.TrimSpace(note.Path) == "" || strings.TrimSpace(note.ID) == "" {
		return indexRefreshScanItem{path: rel, failedPath: rel}
	}
	return indexRefreshScanItem{path: rel, note: note}
}

func indexRefreshWorkerCount(total int) int {
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	if total > 0 && workers > total {
		workers = total
	}
	return workers
}

func ordinaryNotes(notes []domain.Note) []domain.Note {
	ordinary := make([]domain.Note, 0, len(notes))
	for _, note := range notes {
		if isSystemIndexNote(note) || isSystemJournalNote(note) {
			continue
		}
		ordinary = append(ordinary, note)
	}
	return ordinary
}

func ordinaryNoteFacts(facts []noteFact) []noteFact {
	ordinary := make([]noteFact, 0, len(facts))
	for _, fact := range facts {
		if isSystemIndexNote(fact.note) || isSystemJournalNote(fact.note) {
			continue
		}
		ordinary = append(ordinary, fact)
	}
	return ordinary
}

func shouldSkipVaultWalkDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "dist"
}

func isPinaxNoteFrontmatter(meta map[string]string) bool {
	return meta["schema_version"] == "pinax.note.v1"
}

func isSystemIndexNote(note domain.Note) bool {
	path := filepath.ToSlash(note.Path)
	if note.Kind != "index" {
		return false
	}
	// index page 是系统导航页，不参与普通知识卡片的 search/orphan/stat；旧 notes/daily index 继续按 legacy 系统页过滤。
	return strings.HasPrefix(path, "index/") || strings.HasPrefix(path, "notes/index/") || strings.HasPrefix(path, "notes/daily/")
}

func isSystemJournalNote(note domain.Note) bool {
	path := filepath.ToSlash(note.Path)
	if note.Kind != "daily" && note.Kind != "weekly" && note.Kind != "monthly" {
		return false
	}
	return strings.HasPrefix(path, note.Kind+"/") || strings.HasPrefix(path, "notes/"+note.Kind+"/")
}
func parseNote(rel, content string) domain.Note {
	doc, err := markdownnote.ParseFull(rel, []byte(content))
	if err == nil {
		return doc.Note
	}
	meta, body := splitFrontmatter(content)
	title := meta["title"]
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	return domain.Note{ID: meta["note_id"], Title: title, Path: rel, Tags: parseTags(meta["tags"]), Labels: parseTags(meta["labels"]), Body: strings.TrimSpace(body), Frontmatter: meta, Project: meta["project"], Subproject: meta["subproject"], Folder: meta["folder"], Kind: meta["kind"], Status: meta["status"], BoardColumn: meta["board_column"], Milestone: meta["milestone"], Priority: meta["priority"], Due: meta["due"], DueAt: meta["due_at"], BlockedBy: parseTags(meta["blocked_by"]), CreatedAt: meta["created_at"], UpdatedAt: meta["updated_at"]}
}

func splitFrontmatter(content string) (map[string]string, string) {
	meta := map[string]string{}
	if !strings.HasPrefix(content, "---\n") {
		return meta, content
	}
	scanner := bufio.NewScanner(strings.NewReader(content[4:]))
	lines := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			remaining := strings.TrimPrefix(content, "---\n"+strings.Join(lines, "\n")+"\n---")
			for _, item := range lines {
				key, value, ok := strings.Cut(item, ":")
				if ok {
					meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
				}
			}
			return meta, strings.TrimPrefix(remaining, "\n")
		}
		lines = append(lines, line)
	}
	return meta, content
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "[]")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "\"")
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

func noteNeedsMetadata(note domain.Note) bool {
	return note.ID == "" || note.Title == "" || len(note.Tags) == 0
}

func noteNeedsMetadataInVault(root string, note domain.Note) bool {
	if noteNeedsMetadata(note) {
		return true
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return true
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	meta, _ := splitFrontmatter(string(payload))
	return strings.TrimSpace(meta["schema_version"]) == ""
}
func ensureFrontmatter(note domain.Note, content string) string {
	meta, body := splitFrontmatter(content)
	if meta["schema_version"] == "" {
		meta["schema_version"] = "pinax.note.v1"
	}
	if meta["note_id"] == "" && strings.TrimSpace(note.ID) != "" {
		meta["note_id"] = note.ID
	}
	if meta["title"] == "" {
		meta["title"] = note.Title
	}
	if meta["tags"] == "" {
		meta["tags"] = "[]"
	}
	// 固定 frontmatter key 顺序，避免 agent 或用户多次 apply 造成无意义 diff。
	keys := []string{"schema_version", "note_id", "title", "tags"}
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(meta[key])
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimLeft(body, "\n"))
	return b.String()
}

// deterministicShortID returns a short reproducible token for non-identity uses
// such as fallback slugs, template runs, trash receipts and inferred board items.
func deterministicShortID(value string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(value)))
	return "note_" + hex.EncodeToString(sum[:])[:12]
}

func slugify(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	lastDash := false
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func findProject(root, slug string) (domain.Project, error) {
	registry, err := loadProjectRegistry(root)
	if err != nil {
		return domain.Project{}, err
	}
	for _, project := range registry.Projects {
		if project.Slug == slug {
			return project, nil
		}
	}
	return domain.Project{}, &domain.CommandError{Code: "project_not_found", Message: "Project not found", Hint: "Run pinax project list to view available projects"}
}

func nextNotePath(root, rel string) (string, error) {
	base := strings.TrimSuffix(rel, filepath.Ext(rel))
	ext := filepath.Ext(rel)
	candidate := rel
	for i := 2; ; i++ {
		path, err := safeJoin(root, candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return filepath.ToSlash(candidate), nil
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

func buildNoteContentWithStatus(title, rel, project, folder, kind string, tags []string, status, now, body string) string {
	return buildNoteContentWithObjectID(legacyNoteIDForPath(rel), title, project, folder, kind, tags, status, now, body)
}

func buildNoteContentWithObjectID(noteID, title, project, folder, kind string, tags []string, status, now, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: pinax.note.v1\n")
	b.WriteString("note_id: ")
	b.WriteString(noteID)
	b.WriteString("\n")
	b.WriteString("title: ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString("tags: ")
	b.WriteString(formatTags(cleanTags(tags)))
	b.WriteString("\n")
	if project != "" {
		b.WriteString("project: ")
		b.WriteString(project)
		b.WriteString("\n")
	}
	if folder != "" {
		b.WriteString("folder: ")
		b.WriteString(folder)
		b.WriteString("\n")
	}
	if kind != "" {
		b.WriteString("kind: ")
		b.WriteString(kind)
		b.WriteString("\n")
	}
	if status != "" {
		b.WriteString("status: ")
		b.WriteString(status)
		b.WriteString("\n")
	}
	b.WriteString("created_at: ")
	b.WriteString(now)
	b.WriteString("\nupdated_at: ")
	b.WriteString(now)
	b.WriteString("\n---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	return b.String()
}

func noteBodyFromRequest(req CreateNoteRequest) (string, error) {
	sources := 0
	for _, value := range []string{req.Body, req.SourcePath, req.StdinBody} {
		if value != "" {
			sources++
		}
	}
	if sources > 1 {
		return "", &domain.CommandError{Code: "note_source_conflict", Message: "note new can use only one body source", Hint: "Keep only one of --body, --from, or --stdin"}
	}
	if req.SourcePath != "" {
		b, err := os.ReadFile(req.SourcePath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if req.StdinBody != "" {
		return req.StdinBody, nil
	}
	return req.Body, nil
}

type editorCommand struct {
	Raw        string   `json:"raw"`
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

func parseEditorCommand(value string) (editorCommand, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return editorCommand{}, &domain.CommandError{Code: "editor_not_configured", Message: "Editor is not configured", Hint: "Set $EDITOR or pass --editor"}
	}
	parts, err := splitCommandLine(value)
	if err != nil {
		return editorCommand{}, &domain.CommandError{Code: "editor_parse_failed", Message: "Editor command could not be parsed", Hint: "Use a simple command or pass a wrapper script"}
	}
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return editorCommand{}, &domain.CommandError{Code: "editor_not_configured", Message: "Editor is not configured", Hint: "Set $EDITOR or pass --editor"}
	}
	return editorCommand{Raw: value, Executable: parts[0], Args: parts[1:]}, nil
}

func splitCommandLine(value string) ([]string, error) {
	var parts []string
	var b strings.Builder
	quote := rune(0)
	escaped := false
	for _, r := range value {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t', '\n':
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts, nil
}

func noteCreatePrefix(root string, req CreateNoteRequest) (string, error) {
	if req.Dir != "" {
		return validateNoteDir(req.Dir)
	}
	folder, err := validateOptionalNoteFolder(req.Folder)
	if err != nil {
		return "", err
	}
	// 默认笔记写在 vault 根内容区；显式 --dir 和 project prefix 继续保留旧 `notes/` 兼容语义。
	base := ""
	if req.Project != "" {
		project, err := findProject(root, req.Project)
		if err != nil {
			return "", err
		}
		base = project.NotesPrefix
	}
	if folder != "" {
		return filepath.ToSlash(filepath.Join(base, folder)), nil
	}
	return base, nil
}

func validateOptionalNoteFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "", nil
	}
	clean := filepath.ToSlash(filepath.Clean(folder))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(folder) || clean == ".pinax" || strings.HasPrefix(clean, ".pinax/") || clean == "notes" || strings.HasPrefix(clean, "notes/") {
		return "", &domain.CommandError{Code: "unsafe_note_folder", Message: "note folder must be a relative directory under Project or notes", Hint: "Use a folder like inbox, reference, or work/research"}
	}
	return clean, nil
}

func validateNoteDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "notes", nil
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(dir) || clean == ".pinax" || strings.HasPrefix(clean, ".pinax/") {
		return "", &domain.CommandError{Code: "unsafe_note_path", Message: "note directory must be under vault notes/", Hint: "Use a directory like work or notes/work"}
	}
	if clean == "notes" || strings.HasPrefix(clean, "notes/") {
		return clean, nil
	}
	return filepath.ToSlash(filepath.Join("notes", clean)), nil
}

func validateNoteSlug(slug string) error {
	clean := filepath.ToSlash(filepath.Clean(slug))
	if clean == "." || clean == ".." || strings.Contains(clean, "/") || strings.HasPrefix(clean, ".") || filepath.IsAbs(slug) {
		return &domain.CommandError{Code: "invalid_note_slug", Message: "note slug must be a single safe filename", Hint: "Use a slug like daily-review"}
	}
	return nil
}

type noteRefAmbiguousError struct {
	*domain.CommandError
	Ref        string
	Candidates []domain.Note
}

func (e *noteRefAmbiguousError) Unwrap() error { return e.CommandError }

func resolveNoteRef(notes []domain.Note, ref string) (domain.Note, error) {
	// 只接受确定性匹配：标题必须唯一，否则宁可失败并返回候选，避免误打开或误改用户笔记。
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return domain.Note{}, &domain.CommandError{Code: "note_ref_required", Message: "note reference is required", Hint: "Provide a note_id, path, or title"}
	}
	needle := filepath.ToSlash(strings.TrimPrefix(ref, "notes/"))
	var titleMatches []domain.Note
	var stemMatches []domain.Note
	for _, note := range notes {
		if note.ID == ref || note.Path == ref || strings.TrimPrefix(note.Path, "notes/") == needle {
			return note, nil
		}
		if strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)) == ref {
			stemMatches = append(stemMatches, note)
		}
		if note.Title == ref || journalNoteShellFriendlyAlias(note) == ref {
			titleMatches = append(titleMatches, note)
		}
	}
	if len(stemMatches) == 1 {
		return stemMatches[0], nil
	}
	if len(stemMatches) > 1 {
		return domain.Note{}, &noteRefAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "Note reference has multiple candidates", Hint: "Retry with a note_id or full path"}, Ref: ref, Candidates: stemMatches}
	}
	if len(titleMatches) == 1 {
		return titleMatches[0], nil
	}
	if len(titleMatches) > 1 {
		return domain.Note{}, &noteRefAmbiguousError{CommandError: &domain.CommandError{Code: "note_ref_ambiguous", Message: "Note reference has multiple candidates", Hint: "Retry with a note_id or full path"}, Ref: ref, Candidates: titleMatches}
	}
	return domain.Note{}, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax note list to view available notes"}
}

func noteMatchesQuery(note domain.Note, req NoteListRequest) bool {
	// List query only applies explicit dimensions; it avoids fuzzy title/body matching so CLI output remains predictable.
	return noteops.MatchesList(note, noteops.ListRequest{Project: req.Project, Group: req.Group, Folder: req.Folder, Kind: req.Kind, Status: req.Status, CreatedAfter: req.CreatedAfter, UpdatedAfter: req.UpdatedAfter, UpdatedBefore: req.UpdatedBefore, PathPrefix: req.PathPrefix, Tags: req.Tags})
}

func sortNotes(notes []domain.Note, req NoteListRequest) {
	sort.SliceStable(notes, func(i, j int) bool {
		switch req.Sort {
		case "path":
			return notes[i].Path < notes[j].Path
		case "title":
			return notes[i].Title < notes[j].Title
		default:
			return notes[i].UpdatedAt > notes[j].UpdatedAt
		}
	})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s *Service) loadMutableNoteForWrite(ctx context.Context, vaultPath, noteRef string) (string, domain.Note, string, string, map[string]string, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", domain.Note{}, "", "", nil, err
	}
	result, err := s.ResolveManagedObjectForMutation(ctx, ResolverRequest{VaultPath: root, Query: noteRef, Scope: "registered", Kind: "note"})
	if err != nil {
		if len(result.Candidates) > 1 {
			return "", domain.Note{}, "", "", nil, &resolverNoteAmbiguousError{CommandError: &domain.CommandError{Code: domain.ErrorCodeVaultObjectRefAmbiguous, Message: "note write query matched multiple candidates", Hint: "Retry with a more specific note_id, filename, or full path"}, Result: result}
		}
		return "", domain.Note{}, "", "", nil, err
	}
	if len(result.Candidates) == 0 {
		return "", domain.Note{}, "", "", nil, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax note list to view available notes"}
	}
	notes, err := scanNotes(root)
	if err != nil {
		return "", domain.Note{}, "", "", nil, err
	}
	var note domain.Note
	for _, candidate := range notes {
		if candidate.Path == result.Candidates[0].Path {
			note = candidate
			break
		}
	}
	if note.Path == "" {
		return "", domain.Note{}, "", "", nil, &domain.CommandError{Code: "note_not_found", Message: "Note not found", Hint: "Run pinax index refresh, then retry"}
	}
	return loadMutableResolvedNote(root, note)
}

func loadMutableResolvedNote(root string, note domain.Note) (string, domain.Note, string, string, map[string]string, error) {
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return "", domain.Note{}, "", "", nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", domain.Note{}, "", "", nil, err
	}
	meta, _ := splitFrontmatter(string(b))
	if meta["schema_version"] == "" {
		meta["schema_version"] = "pinax.note.v1"
	}
	if strings.TrimSpace(meta["note_id"]) == "" {
		return "", domain.Note{}, "", "", nil, &domain.CommandError{Code: "identity_migration_required", Message: "note has no canonical object identity", Hint: "Run pinax record identity audit, then save and apply an identity migration plan"}
	}
	if meta["title"] == "" {
		meta["title"] = note.Title
	}
	return root, note, path, string(b), meta, nil
}

func loadMutableNote(vaultPath, noteRef string) (string, domain.Note, string, string, map[string]string, string, error) {
	root, err := cleanVaultPath(vaultPath)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	note, err := resolveNoteRef(notes, noteRef)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	path, err := safeJoin(root, note.Path)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", domain.Note{}, "", "", nil, "", err
	}
	meta, body := splitFrontmatter(string(b))
	if meta["schema_version"] == "" {
		meta["schema_version"] = "pinax.note.v1"
	}
	if strings.TrimSpace(meta["note_id"]) == "" {
		return "", domain.Note{}, "", "", nil, "", &domain.CommandError{Code: "identity_migration_required", Message: "note has no canonical object identity", Hint: "Run pinax record identity audit, then save and apply an identity migration plan"}
	}
	if meta["title"] == "" {
		meta["title"] = note.Title
	}
	if meta["tags"] == "" {
		meta["tags"] = formatTags(note.Tags)
	}
	return root, note, path, string(b), meta, body, nil
}

func patchFrontmatterFields(content string, fields map[string]string) (string, bool) {
	return patchFrontmatterFieldsRemoving(content, fields, nil)
}

func patchFrontmatterFieldsRemoving(content string, fields map[string]string, removeKeys []string) (string, bool) {
	if !strings.HasPrefix(content, "---\n") {
		meta := map[string]string{}
		for k, v := range fields {
			meta[k] = v
		}
		return renderFrontmatter(meta, strings.TrimLeft(content, "\n")), true
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		meta := map[string]string{}
		for k, v := range fields {
			meta[k] = v
		}
		return renderFrontmatter(meta, content), true
	}
	frontStart := 4
	frontEnd := 4 + end
	front := content[frontStart:frontEnd]
	body := strings.TrimPrefix(content[frontEnd+len("\n---"):], "\n")
	lines := strings.Split(front, "\n")
	seen := map[string]bool{}
	remove := map[string]bool{}
	for _, key := range removeKeys {
		remove[key] = true
	}
	kept := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			kept = append(kept, line)
			continue
		}
		key = strings.TrimSpace(key)
		if remove[key] {
			continue
		}
		if value, ok := fields[key]; ok {
			line = key + ": " + value
			seen[key] = true
		}
		kept = append(kept, line)
	}
	for _, key := range orderedFrontmatterKeys(fields) {
		if !seen[key] && strings.TrimSpace(fields[key]) != "" {
			kept = append(kept, key+": "+fields[key])
		}
	}
	return "---\n" + strings.Join(kept, "\n") + "\n---\n\n" + strings.TrimLeft(body, "\n"), false
}

func orderedFrontmatterKeys(fields map[string]string) []string {
	preferred := []string{"schema_version", "note_id", "title", "tags", "project", "folder", "kind", "status", "created_at", "updated_at"}
	seen := map[string]bool{}
	var keys []string
	for _, key := range preferred {
		if _, ok := fields[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var extra []string
	for key := range fields {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return append(keys, extra...)
}

func commitNoteContent(currentPath, targetPath, content string) error {
	// 同目录临时文件可让“最终替换”尽量接近原子操作；在 commit 前失败时原文件保持不变。
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".pinax-note-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	cleanup = false
	if filepath.Clean(currentPath) != filepath.Clean(targetPath) {
		if err := os.Remove(currentPath); err != nil {
			return err
		}
	}
	return nil
}

func uniqueTrashRel(root, notePath string, now time.Time) (string, error) {
	base := filepath.ToSlash(filepath.Join(".pinax", "trash", now.UTC().Format("20060102"), strings.TrimPrefix(notePath, "notes/")))
	candidate := base
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 2; ; i++ {
		path, err := safeJoin(root, candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}

func renderFrontmatter(meta map[string]string, body string) string {
	keys := []string{"schema_version", "note_id", "title", "tags", "project", "folder", "kind", "status", "created_at", "updated_at"}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range keys {
		seen[key] = true
		if value := strings.TrimSpace(meta[key]); value != "" {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}
	extra := make([]string, 0)
	for key := range meta {
		if !seen[key] && strings.TrimSpace(meta[key]) != "" {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(meta[key])
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimLeft(body, "\n"))
	return b.String()
}

func noteMutationProjection(command, summary, path string, meta map[string]string) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["path"] = path
	projection.Facts["note_id"] = meta["note_id"]
	projection.Facts["title"] = meta["title"]
	projection.Data = map[string]any{"path": path, "frontmatter": meta}
	return projection
}

func mergeTags(existing, add []string) []string {
	seen := map[string]bool{}
	for _, tag := range cleanTags(existing) {
		seen[tag] = true
	}
	for _, tag := range cleanTags(add) {
		seen[tag] = true
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func removeTags(existing, remove []string) []string {
	blocked := map[string]bool{}
	for _, tag := range cleanTags(remove) {
		blocked[tag] = true
	}
	out := make([]string, 0)
	for _, tag := range cleanTags(existing) {
		if !blocked[tag] {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

var templateVariablePattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_:-]*)\s*\}\}`)
var templateVariableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_:-]*$`)

func cleanTemplateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !isTemplateNameSafe(name) {
		return "", &domain.CommandError{Code: "invalid_template_name", Message: "template name may only contain letters, numbers, -, _, and .", Hint: "For example, pinax template create meeting or index.home"}
	}
	return name, nil
}

func isTemplateNameSafe(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			continue
		}
		if i > 0 && (r == '-' || r == '_' || r == '.') {
			continue
		}
		return false
	}
	return true
}

func templatePath(root, name string) (string, error) {
	return safeJoin(root, filepath.ToSlash(filepath.Join(".pinax", "templates", name+".md")))
}

func templateBodyWithRequestedEngine(body, engine string) (string, error) {
	engine = strings.TrimSpace(engine)
	if engine == "" {
		return body, nil
	}
	if engine != templateengine.EngineSimple && engine != templateengine.EngineGoTemplate {
		return "", &domain.CommandError{Code: "template_engine_invalid", Message: "template engine is unsupported", Hint: "Use simple or go-template"}
	}
	if strings.HasPrefix(body, "---\n") || engine == templateengine.EngineSimple {
		return body, nil
	}
	return "---\nschema_version: pinax.template.v2\nengine: " + engine + "\nkind: note\n---\n\n" + body, nil
}

func parseTemplateForProjection(root, name string) (templateengine.TemplateDocument, error) {
	body, err := loadTemplate(root, name)
	if err != nil {
		return templateengine.TemplateDocument{}, err
	}
	doc, err := templateengine.ParseDocument(name, body)
	if err != nil {
		return templateengine.TemplateDocument{}, templateEngineCommandError(err)
	}
	return doc, nil
}

func templateProjectionMetadata(root, name string, meta templateengine.Metadata) (templateengine.Metadata, string) {
	source := templateSource(root, name)
	workflow := templateWorkflowMetadata(name, source, meta)
	if source == "vault-local" {
		if _, ok := builtInTemplates()[name]; ok {
			workflow.Lifecycle = "overridden"
		}
	}
	return workflow, source
}

func templateSource(root, name string) string {
	path, err := templatePath(root, name)
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return "vault-local"
		}
	}
	if _, ok := builtInTemplates()[name]; ok {
		return "builtin"
	}
	return "unknown"
}

func requiredTemplateVariables(meta templateengine.Metadata) []string {
	vars := make([]string, 0, len(meta.VariableSchema))
	for key, variable := range meta.VariableSchema {
		if variable.Required {
			vars = append(vars, key)
		}
	}
	sort.Strings(vars)
	return vars
}

func missingTemplateVariables(meta templateengine.Metadata, values map[string]string) []string {
	required := requiredTemplateVariables(meta)
	missing := make([]string, 0, len(required))
	for _, key := range required {
		if values == nil || strings.TrimSpace(values[key]) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func templateSourceBody(req TemplateRequest, name string) (string, error) {
	sources := 0
	if req.SourcePath != "" {
		sources++
	}
	if req.Body != "" {
		sources++
	}
	if req.UseStdin {
		sources++
	}
	if sources == 0 {
		return templateDesignBody(name), nil
	}
	if sources > 1 {
		return "", &domain.CommandError{Code: "template_source_conflict", Message: "template create can use only one template source", Hint: "Keep only one of --from, --body, or --stdin"}
	}
	if req.SourcePath != "" {
		b, err := os.ReadFile(req.SourcePath)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return req.Body, nil
}

func templateDesignBody(name string) string {
	return fmt.Sprintf("---\nschema_version: pinax.template_design.v1\nkind: template_design\ntitle: %s\n---\n\n## Template Body\n\n# {{title}}\n", name)
}

func templateHasDesignFrontmatter(body string) bool {
	return strings.Contains(body, "schema_version: pinax.template_design.v1") && strings.Contains(body, "kind: template_design")
}

func validateTemplateVars(vars map[string]string) error {
	for key := range vars {
		if !templateVariableNamePattern.MatchString(key) {
			return &domain.CommandError{Code: "template_variable_invalid", Message: "template variable key is invalid", Hint: "Use --var key=value; key may only contain letters, numbers, _, :, or -, and cannot start with a number"}
		}
	}
	return nil
}

func templateVariables(body string) []string {
	seen := map[string]bool{}
	for _, match := range templateVariablePattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			seen[match[1]] = true
		}
	}
	vars := make([]string, 0, len(seen))
	for key := range seen {
		vars = append(vars, key)
	}
	sort.Strings(vars)
	return vars
}

func validateTemplateContent(body string, req TemplateRequest) []domain.Issue {
	issues := make([]domain.Issue, 0)
	if strings.TrimSpace(body) == "" {
		issues = append(issues, domain.Issue{Code: "template_empty", Message: "Template is empty"})
	}
	doc, err := templateengine.ParseDocument(req.Name, body)
	if err != nil {
		issues = append(issues, domain.Issue{Code: templateengine.ErrorCode(err), Message: err.Error()})
	}
	for _, issue := range doc.Issues {
		issues = append(issues, domain.Issue{Code: issue.Code, Message: issue.Message})
	}
	// 代码围栏是 Markdown/Mermaid/YAML 模板最容易破坏生成结果的地方；这里仅跟踪 fence 奇偶，保持实现可审计且不解析 Markdown 全语法。
	fenceOpen := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenceOpen = !fenceOpen
		}
	}
	if fenceOpen {
		issues = append(issues, domain.Issue{Code: "template_fence_unclosed", Message: "Markdown code fence is unclosed"})
	}
	if err := validateTemplateVars(req.Vars); err != nil {
		issues = append(issues, domain.Issue{Code: "template_variable_invalid", Message: err.Error()})
	}
	if doc.Engine != templateengine.EngineGoTemplate {
		for _, key := range templateVariables(body) {
			if !templateVariableNamePattern.MatchString(key) {
				issues = append(issues, domain.Issue{Code: "template_variable_invalid", Message: "template variable key is invalid: " + key})
			}
		}
	}
	return issues
}

func listTemplates(root string) ([]string, error) {
	dir := filepath.Join(root, ".pinax", "templates")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".md"))
	}
	sort.Strings(names)
	return names, nil
}

func loadTemplate(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "note"
	}
	name, err := cleanTemplateName(name)
	if err != nil {
		return "", err
	}
	path, err := templatePath(root, name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if body, ok := builtInTemplates()[name]; ok {
			return body, nil
		}
		return "", &domain.CommandError{Code: "template_not_found", Message: "Template not found", Hint: "Run pinax template init to initialize built-in templates"}
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Service) renderTemplateBody(ctx context.Context, root string, req TemplateRequest, lazyIndex bool) (string, error) {
	body, err := loadTemplate(root, req.Name)
	if err != nil {
		return "", err
	}
	for _, issue := range validateTemplateContent(body, TemplateRequest{Name: req.Name, Vars: req.Vars}) {
		if issue.Code == "template_variable_invalid" {
			return "", &domain.CommandError{Code: issue.Code, Message: issue.Message, Hint: "Use --var key=value; key may only contain letters, numbers, _, :, or -, and cannot start with a number"}
		}
		if issue.Code == "template_frontmatter_unclosed" || issue.Code == "template_fence_unclosed" || issue.Code == "template_schema_invalid" {
			return "", &domain.CommandError{Code: "template_invalid", Message: issue.Message, Hint: "Run pinax template validate <name> first to fix the template"}
		}
	}
	doc, err := templateengine.ParseDocument(req.Name, body)
	if err != nil {
		return "", templateEngineCommandError(err)
	}
	if templateDocumentIsDesignDraft(doc) {
		return "", &domain.CommandError{Code: "template_design_not_executable", Message: "Template is still a draft and cannot be used for preview, render, or note creation", Hint: "Publish the draft as an executable schema_version: pinax.template.v2 template first"}
	}
	req = applyTemplateExample(req, doc.Metadata)
	renderCtx := templateEngineContext(req)
	queries, err := s.executeTemplateQueries(ctx, root, doc.Metadata.Queries, lazyIndex)
	if err != nil {
		return "", err
	}
	renderCtx.Queries = queries
	rendered, err := templateengine.New().Render(doc, renderCtx)
	if err != nil {
		return "", templateEngineCommandError(err)
	}
	return rendered.Body, nil
}

func templateDocumentIsDesignDraft(doc templateengine.TemplateDocument) bool {
	if doc.Metadata.SchemaVersion == "pinax.template_design.v1" || doc.Metadata.Kind == "template_design" || doc.Metadata.Lifecycle == "draft_design" {
		return true
	}
	for _, issue := range doc.Issues {
		if issue.Code == "template_design_legacy" {
			return true
		}
	}
	return false
}

func missingTemplateVariableCommand(root string, req TemplateRequest, command string) string {
	variable := "key"
	if doc, err := parseTemplateForProjection(root, req.Name); err == nil {
		for key, meta := range doc.Metadata.Variables {
			if meta.Required {
				if req.Vars == nil || req.Vars[key] == "" {
					variable = key
					break
				}
			}
		}
	}
	verb := "render"
	if command == "template.preview" {
		verb = "preview"
	}
	return fmt.Sprintf("pinax template %s %s --var %s=... --vault %s --json", verb, shellQuote(req.Name), variable, shellQuote(root))
}

func renderTemplateOutputPath(doc templateengine.TemplateDocument, req CreateNoteRequest) (string, error) {
	pattern := strings.TrimSpace(doc.Metadata.Output.PathPattern)
	if pattern == "" {
		return "", &domain.CommandError{Code: "template_output_path_invalid", Message: "Template is missing output.path_pattern", Hint: "Check template metadata"}
	}
	rendered, err := templateengine.New().Render(templateengine.TemplateDocument{Name: doc.Name + ":output", Engine: doc.Engine, Body: pattern}, templateengine.Context{Title: req.Title, Project: req.Project, Tags: req.Tags, Vars: req.Vars})
	if err != nil {
		return "", templateEngineCommandError(err)
	}
	rel := strings.TrimSpace(rendered.Body)
	if rel == "" {
		return "", &domain.CommandError{Code: "template_output_path_invalid", Message: "Template output.path_pattern generated an empty path", Hint: "Check template output.path_pattern"}
	}
	if filepath.Ext(rel) == "" {
		rel += ".md"
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if templateOutputPathForbidden(rel) {
		return "", &domain.CommandError{Code: "template_output_path_invalid", Message: "Template output path is outside allowed vault content areas", Hint: "Use a root-relative Markdown path like inbox/{{ .Title }}.md"}
	}
	return rel, nil
}

func templateOutputPathForbidden(rel string) bool {
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) || filepath.Ext(rel) != ".md" {
		return true
	}
	first := rel
	if before, _, ok := strings.Cut(rel, "/"); ok {
		first = before
	}
	switch first {
	case ".pinax", ".git", "attachments", "temp", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func (s *Service) explainTemplateQueries(ctx context.Context, queries map[string]templateengine.TemplateQueryDeclaration) map[string]domain.Projection {
	if len(queries) == 0 {
		return nil
	}
	explained := make(map[string]domain.Projection, len(queries))
	for name, query := range queries {
		projection, err := s.QueryExplain(ctx, QueryRequest{SQL: query.SQL})
		if err != nil {
			projection = domain.NewErrorProjection("query.explain", templateQueryCommandError(name, err))
		}
		explained[name] = projection
	}
	return explained
}

func (s *Service) executeTemplateQueries(ctx context.Context, root string, queries map[string]templateengine.TemplateQueryDeclaration, lazyIndex bool) (map[string]templateengine.QueryResult, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if !lazyIndex {
		notes, err := scanNotes(root)
		if err != nil {
			return nil, err
		}
		status, _ := noteindex.Inspect(root, ordinaryNotes(notes))
		if status.Status != "fresh" {
			return nil, &domain.CommandError{Code: "template_index_required", Message: "Template preview query requires a fresh local index", Hint: "Run pinax index rebuild --vault " + shellQuote(root)}
		}
	}
	results := make(map[string]templateengine.QueryResult, len(queries))
	for name, query := range queries {
		limit := query.MaxRows
		if limit <= 0 {
			limit = 50
		}
		projection, err := s.QueryRun(ctx, QueryRequest{VaultPath: root, SQL: query.SQL, Limit: limit, LazyIndex: lazyIndex})
		if err != nil {
			if query.Required {
				return nil, templateQueryCommandError(name, err)
			}
			continue
		}
		results[name] = templateQueryResultFromProjection(projection)
	}
	return results, nil
}

func applyTemplateExample(req TemplateRequest, meta templateengine.Metadata) TemplateRequest {
	if req.Title == "" && meta.Example.Title != "" {
		req.Title = meta.Example.Title
	}
	if req.Project == "" && meta.Example.Project != "" {
		req.Project = meta.Example.Project
	}
	if len(req.Tags) == 0 && len(meta.Example.Tags) > 0 {
		req.Tags = append([]string(nil), meta.Example.Tags...)
	}
	if len(meta.Example.Vars) > 0 {
		merged := make(map[string]string, len(meta.Example.Vars)+len(req.Vars))
		for key, value := range meta.Example.Vars {
			merged[key] = value
		}
		for key, value := range req.Vars {
			merged[key] = value
		}
		req.Vars = merged
	}
	return req
}

func templateQueryCommandError(name string, err error) *domain.CommandError {
	message := "Template query failed: " + name
	if err != nil && err.Error() != "" {
		message = message + ": " + err.Error()
	}
	return &domain.CommandError{Code: "template_query_execute_failed", Message: message, Hint: "Run pinax query explain <sql> --vault <vault> to check the query, or run pinax index sync --vault <vault> and retry"}
}

func templateQueryResultFromProjection(projection domain.Projection) templateengine.QueryResult {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return templateengine.QueryResult{}
	}
	result, ok := data["result"].(domain.TableResult)
	if !ok {
		return templateengine.QueryResult{}
	}
	converted := templateengine.QueryResult{Columns: result.Columns, Rows: make([]map[string]string, 0, len(result.Rows))}
	for _, row := range result.Rows {
		values := make(map[string]string, len(result.Columns))
		for _, column := range result.Columns {
			switch column {
			case "title":
				values[column] = row.Note.Title
			case "path":
				values[column] = row.Note.Path
			case "id":
				values[column] = row.Note.ID
			case "project", "group":
				values[column] = row.Note.Project
			case "kind":
				values[column] = row.Note.Kind
			case "status":
				values[column] = row.Note.Status
			case "tags":
				values[column] = strings.Join(row.Note.Tags, ", ")
			default:
				if value, ok := row.Values[column]; ok {
					values[column] = value.String()
				}
			}
		}
		converted.Rows = append(converted.Rows, values)
	}
	return converted
}

func templateEngineContext(req TemplateRequest) templateengine.Context {
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	datetime := now.Format(time.RFC3339)
	if req.Title == "" {
		req.Title = "Untitled"
	}
	return templateengine.Context{
		Title:    req.Title,
		Date:     date,
		DateTime: datetime,
		Project:  req.Project,
		Tags:     cleanTags(req.Tags),
		Vars:     req.Vars,
	}
}

func templateEngineCommandError(err error) error {
	code := templateengine.ErrorCode(err)
	switch code {
	case "template_variable_missing":
		return &domain.CommandError{Code: code, Message: err.Error(), Hint: "Use --var key=value to provide the missing variable"}
	case "template_parse_failed", "template_schema_invalid", "template_frontmatter_unclosed":
		return &domain.CommandError{Code: code, Message: err.Error(), Hint: "Fix the template and retry"}
	case "template_render_failed":
		return &domain.CommandError{Code: code, Message: err.Error(), Hint: "Check template context and function arguments"}
	default:
		return err
	}
}

func cleanVaultPath(path string) (string, error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func safeJoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.Contains(filepath.ToSlash(rel), "../") || strings.HasPrefix(filepath.ToSlash(rel), "..") {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "Path escapes the vault boundary"}
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	clean := filepath.Clean(path)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return "", &domain.CommandError{Code: "unsafe_path", Message: "Path escapes the vault boundary"}
	}
	return clean, nil
}

func ensureEventLog(root string) error {
	path := filepath.Join(root, ".pinax", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func appendEvent(root, eventType, status string, facts map[string]string) error {
	if err := ensureEventLog(root); err != nil {
		return err
	}
	event := map[string]any{
		"schema_version": "pinax.event.v1",
		"type":           eventType,
		"status":         status,
		"ts":             time.Now().UTC().Format(time.RFC3339),
		"facts":          facts,
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(root, ".pinax", "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func errorProjection(command string, err error) domain.Projection {
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		projection := domain.NewErrorProjection(command, commandErr)
		var resolverAmbiguous *resolverNoteAmbiguousError
		if errors.As(err, &resolverAmbiguous) {
			projection.Facts["candidates"] = fmt.Sprint(len(resolverAmbiguous.Result.Candidates))
			projection.Data = map[string]any{"candidates": resolverAmbiguous.Result.Candidates}
		}
		return projection
	}
	return domain.NewErrorProjection(command, &domain.CommandError{Code: "internal_error", Message: err.Error()})
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:-]+$`)

func shellQuote(value string) string {
	if shellSafe.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
