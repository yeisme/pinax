package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	judgment "github.com/yeisme/judgment-sdk"
	"github.com/yeisme/judgment-sdk/providers"
	pinaxconfig "github.com/yeisme/pinax/internal/config"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/inboxjudgment"
)

type InboxReviewRequest struct {
	VaultPath string
	Limit     int
	After     string
}

type InboxReviewItem struct {
	NoteID  string          `json:"note_id,omitempty"`
	Path    string          `json:"path"`
	Title   string          `json:"title"`
	Source  string          `json:"source"`
	Status  string          `json:"status,omitempty"`
	Snippet string          `json:"snippet,omitempty"`
	Reason  string          `json:"reason"`
	Related []SearchShowRef `json:"related,omitempty"`
	Actions []domain.Action `json:"actions"`
}

type InboxReviewResult struct {
	Items     []InboxReviewItem `json:"items"`
	Total     int               `json:"total"`
	Remaining int               `json:"remaining"`
	NextAfter string            `json:"next_after,omitempty"`
}

// InboxReview 只装配现有笔记投影，不写索引/审阅状态，也不调用模型。
func (s *Service) InboxReview(_ context.Context, req InboxReviewRequest) (domain.Projection, error) {
	const command = "inbox.review"
	if req.Limit < 1 || req.Limit > 20 {
		err := &domain.CommandError{Code: "invalid_limit", Message: "Review batch size must be between 1 and 20", Hint: "Use --limit 5"}
		return errorProjection(command, err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if info, statErr := os.Stat(filepath.Join(root, ".pinax", "config.yaml")); statErr != nil || !info.Mode().IsRegular() {
		err := &domain.CommandError{Code: "vault_not_initialized", Message: "Review requires an initialized local vault", Hint: "Run pinax vault list --agent and select the intended local vault with --vault"}
		return errorProjection(command, err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection(command, err), err
	}
	notes = ordinaryNotes(notes)
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	pending := make([]domain.Note, 0)
	seen := map[string]bool{}
	for _, note := range notes {
		if !isDailyIntakeNote(note) || seen[stableNoteRef(note)] {
			continue
		}
		seen[stableNoteRef(note)] = true
		pending = append(pending, note)
	}
	start := 0
	if req.After != "" {
		cursor, resolveErr := resolveNoteRef(pending, req.After)
		if resolveErr != nil {
			err := &domain.CommandError{Code: "review_cursor_stale", Message: "The review cursor is missing or ambiguous in the pending list", Hint: "Run pinax inbox review again without --after; no notes were changed"}
			return errorProjection(command, err), err
		}
		for i, note := range pending {
			if note.Path == cursor.Path {
				start = i + 1
				break
			}
		}
	}
	end := min(start+req.Limit, len(pending))
	result := InboxReviewResult{Items: []InboxReviewItem{}, Total: len(pending), Remaining: len(pending) - end}
	projection := domain.NewProjection(command, "Pending notes ready for review; nothing changed.")
	projection.Facts["writes"] = "false"
	projection.Facts["total"] = fmt.Sprint(result.Total)
	projection.Facts["returned"] = fmt.Sprint(end - start)
	projection.Facts["remaining"] = fmt.Sprint(result.Remaining)
	projection.Facts["suggestion_basis"] = "shared_tags_only"
	for i, note := range pending[start:end] {
		source := "index"
		if note.Status == "inbox" {
			source = "inbox"
		}
		ref := stableNoteRef(note)
		related := searchShowNeighbors(notes, note)
		reason := "No tag-based destination evidence; read before choosing a folder."
		if len(related) > 0 {
			reason = "Related notes share tags; review them before choosing a destination."
		}
		actions := []domain.Action{
			{Name: "inspect", Command: fmt.Sprintf("pinax search show %s --vault %s", shellQuote(ref), shellQuote(root))},
			{Name: "open", Command: fmt.Sprintf("pinax note open %s --vault %s", shellQuote(ref), shellQuote(root))},
		}
		result.Items = append(result.Items, InboxReviewItem{NoteID: note.ID, Path: note.Path, Title: note.Title, Source: source, Status: note.Status, Snippet: searchShowSnippet(note), Reason: reason, Related: related, Actions: actions})
		prefix := fmt.Sprintf("item.%d.", i+1)
		projection.Facts[prefix+"ref"] = ref
		projection.Facts[prefix+"title"] = note.Title
		projection.Facts[prefix+"source"] = source
		projection.Facts[prefix+"snippet"] = searchShowSnippet(note)
		projection.Facts[prefix+"reason"] = reason
		projection.Actions = append(projection.Actions, domain.Action{Name: fmt.Sprintf("inspect_%d", i+1), Command: actions[0].Command})
	}
	if result.Remaining > 0 {
		result.NextAfter = stableNoteRef(pending[end-1])
		projection.Actions = append(projection.Actions, domain.Action{Name: "next", Command: fmt.Sprintf("pinax inbox review --after %s --limit %d --vault %s", shellQuote(result.NextAfter), req.Limit, shellQuote(root))})
	}
	refs := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		refs = append(refs, item.NoteID)
	}
	if selected, err := selectedOrganizeNotes(root, refs); err == nil {
		selection := make([]domain.OrganizeSelection, 0, len(selected))
		for _, note := range selected {
			selection = append(selection, domain.OrganizeSelection{ObjectID: note.ID})
		}
		projection.Actions = append(projection.Actions, domain.Action{Name: "organize_preview", Command: scopedOrganizeCommand(root, selection)})
	}
	projection.Data = result
	return projection, nil
}

func isDailyIntakeNote(note domain.Note) bool {
	if note.Status == "archived" || note.Status == "discarded" || note.Status == "system" {
		return false
	}
	return note.Status == "inbox" || (strings.HasPrefix(note.Path, "notes/index/") && note.Project == "")
}

// inbox judgment 宿主接线：config 只提供开关与模式，state 是本 owner 的
// 持久回执库，执行 profile 由显式、非秘密的环境选择给出。默认 dormant：
// 不装配 SDK、不读取凭据、不发起任何外部调用。
//
// 本文件不改变领域真源：suggestion/evidence 只作为建议返回，采纳仍经原
// inbox 审阅门；vault 写入、tag、move 与长期记忆确认都不在这里发生。

const (
	// InboxJudgmentCommand 是 CLI/API 共享的命令标识。
	InboxJudgmentCommand = "inbox.judgment"

	// judgmentOwner 是公共 SDK 合同里的 owner 身份。
	judgmentOwner = "pinax"
	// judgmentDefaultProject 是未显式配置 project 时使用的 owner 作用域。
	judgmentDefaultProject = "inbox-judgment"

	// judgmentDefaultLimit 是候选上下文的默认条数。
	judgmentDefaultLimit = 5
	// judgmentMaxExcerptRunes 限制单条候选/收件箱正文进入投影前的长度。
	judgmentMaxExcerptRunes = 1200
)

// judgment 执行 profile 的显式环境选择。只保存引用与选择，不保存密钥：
// 凭据值始终来自进程环境或宿主注入的 resolver。
const (
	JudgmentEnvProfile          = "PINAX_JUDGMENT_PROFILE"
	JudgmentEnvProfileRevision  = "PINAX_JUDGMENT_PROFILE_REVISION"
	JudgmentEnvProvider         = "PINAX_JUDGMENT_PROVIDER"
	JudgmentEnvModel            = "PINAX_JUDGMENT_MODEL"
	JudgmentEnvCredentialSource = "PINAX_JUDGMENT_CREDENTIAL_SOURCE"
	JudgmentEnvCredentialRef    = "PINAX_JUDGMENT_CREDENTIAL_REF"
	JudgmentEnvCredentialEnv    = "PINAX_JUDGMENT_CREDENTIAL_ENV"
	JudgmentEnvProject          = "PINAX_JUDGMENT_PROJECT"
	JudgmentEnvAttempt          = "PINAX_JUDGMENT_ATTEMPT"
)

var judgmentAttemptPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// InboxJudgmentProfile 是显式、非秘密的执行 profile 选择。
// 空值表示未配置；启用模式缺少它一律 fail closed。
type InboxJudgmentProfile struct {
	ID               string
	Revision         uint64
	Provider         string
	Model            string
	CredentialSource string
	CredentialRef    string
	CredentialEnv    string
	Project          string
}

// Configured 报告 profile 是否声明了执行所需的全部引用。
func (p InboxJudgmentProfile) Configured() bool {
	return strings.TrimSpace(p.ID) != "" &&
		strings.TrimSpace(p.Provider) != "" &&
		strings.TrimSpace(p.Model) != "" &&
		strings.TrimSpace(p.CredentialSource) != ""
}

// InboxJudgmentRequest 是 CLI/RPC/REST 共享的入口请求。
type InboxJudgmentRequest struct {
	VaultPath string
	// NoteRef 为空时选择当前第一个待处理 inbox 笔记。
	NoteRef string
	// Limit 是候选上下文条数；0 使用默认值，上限由 owner 合同固定。
	Limit int
	// DryRun 只构建确定性投影，零 provider 调用、零凭据读取。
	DryRun bool
	// AttemptID 显式要求一个新的执行 attempt；为空时复用原 attempt 身份，
	// 因此未知结果不会被自动重提。
	AttemptID string
	// Profile 为测试与未来显式 flag 保留；为空时读取环境选择。
	Profile InboxJudgmentProfile
	// Actor 是 owner-local 主体标识，进入 projection 的 principal digest。
	Actor string
}

// InboxJudgmentProfileFromEnvironment 读取显式非秘密执行 profile 选择。
func InboxJudgmentProfileFromEnvironment() InboxJudgmentProfile {
	revision := uint64(0)
	if raw := strings.TrimSpace(os.Getenv(JudgmentEnvProfileRevision)); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
			revision = parsed
		}
	}
	project := strings.TrimSpace(os.Getenv(JudgmentEnvProject))
	if project == "" {
		project = judgmentDefaultProject
	}
	return InboxJudgmentProfile{
		ID:               strings.TrimSpace(os.Getenv(JudgmentEnvProfile)),
		Revision:         revision,
		Provider:         strings.TrimSpace(os.Getenv(JudgmentEnvProvider)),
		Model:            strings.TrimSpace(os.Getenv(JudgmentEnvModel)),
		CredentialSource: strings.TrimSpace(os.Getenv(JudgmentEnvCredentialSource)),
		CredentialRef:    strings.TrimSpace(os.Getenv(JudgmentEnvCredentialRef)),
		CredentialEnv:    strings.TrimSpace(os.Getenv(JudgmentEnvCredentialEnv)),
		Project:          project,
	}
}

// InboxJudgment 评估一条 inbox 笔记的关联/重复建议。它只在显式启用且
// 注入了执行 profile 时调用外部判断；否则返回 off 投影或明确的配置错误。
func (s *Service) InboxJudgment(ctx context.Context, req InboxJudgmentRequest) (domain.Projection, error) {
	const command = InboxJudgmentCommand
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if info, statErr := os.Stat(root + string(os.PathSeparator) + ".pinax" + string(os.PathSeparator) + "config.yaml"); statErr != nil || !info.Mode().IsRegular() {
		cerr := &domain.CommandError{Code: "vault_not_initialized", Message: "Inbox judgment requires an initialized local vault", Hint: "Run pinax vault list --agent and select the intended local vault with --vault"}
		return errorProjection(command, cerr), cerr
	}
	gate, gateErr := judgmentGateFor(root)
	if gateErr != nil {
		return errorProjection(command, gateErr), gateErr
	}
	if gate.Dormant() {
		return dormantInboxJudgmentProjection(), nil
	}

	profile := req.Profile
	if !profile.Configured() {
		profile = InboxJudgmentProfileFromEnvironment()
	}
	if !profile.Configured() {
		cerr := &domain.CommandError{
			Code:    "judgment_profile_not_configured",
			Message: "Experimental inbox judgment is enabled but no execution profile is configured",
			Hint: fmt.Sprintf("Set %s, %s, %s and %s (non-secret references only); no adapter or credential is discovered automatically",
				JudgmentEnvProfile, JudgmentEnvProvider, JudgmentEnvModel, JudgmentEnvCredentialSource),
		}
		return errorProjection(command, cerr), cerr
	}
	executionProfile, profileErr := profile.executionProfile()
	if profileErr != nil {
		return errorProjection(command, profileErr), profileErr
	}
	if profile.Project == "" {
		profile.Project = judgmentDefaultProject
	}

	notes, noteErr := judgmentVaultNotes(root)
	if noteErr != nil {
		return errorProjection(command, noteErr), noteErr
	}
	inboxNote, noteErr := judgmentInboxNote(notes, req.NoteRef)
	if noteErr != nil {
		return errorProjection(command, noteErr), noteErr
	}
	limit := req.Limit
	if limit <= 0 {
		limit = judgmentDefaultLimit
	}
	candidates := judgmentCandidates(notes, inboxNote, limit)
	if len(candidates) == 0 {
		cerr := &domain.CommandError{
			Code:    "judgment_candidates_required",
			Message: "Inbox judgment requires at least one authorized candidate note",
			Hint:    "Tag the inbox note (or add related tagged notes) so candidates stay inside the current vault; nothing was sent anywhere",
		}
		return errorProjection(command, cerr), cerr
	}
	input := inboxjudgment.InboxJudgmentInput{
		VaultRoot:     root,
		Principal:     judgmentPrincipal(req.Actor),
		InboxNoteID:   judgmentNoteID(inboxNote),
		InboxRevision: inboxNote.ContentDigest,
		InboxText:     judgmentExcerpt(inboxNote.Body),
		Candidates:    candidates,
	}
	allowed := make([]string, 0, len(candidates)+1)
	allowed = append(allowed, input.InboxNoteID)
	for _, candidate := range candidates {
		allowed = append(allowed, candidate.NoteID)
	}
	authorizer := inboxjudgment.VaultJudgmentAuthorizer{VaultRoot: root, AllowedNoteIDs: allowed}
	limits := inboxjudgment.DefaultInboxJudgmentLimits()
	limits.MaxCandidates = inboxjudgment.InboxJudgmentDefaultMaxCandidates
	binding := inboxjudgment.InboxJudgmentBinding{
		TransportProvider:  judgmentTransportProvider(executionProfile.Provider),
		Mode:               gate.Mode,
		Model:              executionProfile.Model,
		AdapterVersion:     judgmentAdapterVersion(executionProfile.Provider),
		QuestionSetID:      inboxjudgment.InboxJudgmentQuestionSetID,
		QuestionSetVersion: inboxjudgment.InboxJudgmentQuestionSetVersion,
		QuestionSetDigest:  inboxjudgment.InboxJudgmentQuestionSetDigest(),
		PolicyID:           inboxjudgment.InboxJudgmentPolicyID,
		PolicyVersion:      inboxjudgment.InboxJudgmentPolicyVersion,
		PolicyDigest:       inboxjudgment.InboxJudgmentPolicyDigest(),
	}
	projection, buildErr := inboxjudgment.BuildInboxJudgmentProjection(ctx, binding, authorizer, input, limits)
	if buildErr != nil {
		return errorProjection(command, buildErr), buildErr
	}
	if req.DryRun {
		return dryRunInboxJudgmentProjection(projection, inboxNote, gate.Mode), nil
	}

	attemptID, attemptErr := judgmentAttemptID(req.AttemptID, projection.Digest)
	if attemptErr != nil {
		return errorProjection(command, attemptErr), attemptErr
	}
	requestID := judgmentRequestID(projection.Digest)
	store, storeErr := inboxjudgment.OpenReceiptStore(root)
	if storeErr != nil {
		cerr := &domain.CommandError{Code: "judgment_receipt_store_unavailable", Message: "Judgment receipt store is unavailable", Hint: "Check vault write permissions for .pinax/api, or run the dry-run preview instead"}
		return errorProjection(command, cerr), cerr
	}
	defer func() { _ = store.Close() }()

	authorize := judgmentSourceAuthorizer(profile.Project, allowed)
	bridge, bridgeErr := inboxjudgment.NewManagedJudgmentSDKBridge(providers.Options{
		Profile:     executionProfile,
		Credentials: providers.Resolver{Environment: os.LookupEnv},
		Authorize:   authorize,
	}, store)
	if bridgeErr != nil {
		cerr := judgmentCommandError(bridgeErr)
		return errorProjection(command, cerr), cerr
	}
	if bridge == nil {
		cerr := &domain.CommandError{Code: "judgment_not_enabled", Message: "Judgment profile is disabled", Hint: "Enable the explicit execution profile or keep judgment.mode=off"}
		return errorProjection(command, cerr), cerr
	}
	caps, capsErr := bridge.DescribeCapabilities(ctx)
	if capsErr != nil {
		cerr := judgmentCommandError(capsErr)
		return errorProjection(command, cerr), cerr
	}
	consumer, consumerErr := gate.Assemble(inboxjudgment.InboxJudgmentOptions{
		ProjectID:      profile.Project,
		RequestID:      requestID,
		AttemptID:      attemptID,
		Model:          executionProfile.Model,
		AdapterVersion: caps.AdapterVersion,
		Transport:      bridge,
		Capabilities:   caps,
		Authorizer:     authorizer,
		Limits:         limits,
		ReviewRefs:     []string{stableNoteRef(inboxNote)},
	})
	if consumerErr != nil {
		cerr := judgmentCommandError(consumerErr)
		return errorProjection(command, cerr), cerr
	}
	outcome, evaluateErr := consumer.Evaluate(ctx, input)
	if evaluateErr != nil {
		cerr := judgmentCommandError(evaluateErr)
		return errorProjection(command, cerr), cerr
	}
	return inboxJudgmentOutcomeProjection(outcome, inboxNote, gate.Mode, profile, requestID, attemptID, len(candidates))
}

// judgmentGateFor 读取 vault 配置里的实验开关并 fail-fast 校验组合。
// app 层自己读取配置，使 CLI、REST 与 RPC 三种入口共享同一门。
func judgmentGateFor(root string) (inboxjudgment.ExperimentalJudgmentGate, *domain.CommandError) {
	paths := pinaxconfig.ResolvePaths(pinaxconfig.PathOptions{VaultPath: root})
	result, err := pinaxconfig.Load(pinaxconfig.LoadOptions{VaultPath: root, UserConfigPath: paths.User, ProjectConfigPath: paths.Project})
	if err != nil {
		return inboxjudgment.ExperimentalJudgmentGate{}, &domain.CommandError{Code: "config_invalid", Message: "Judgment switch configuration is invalid", Hint: "Run pinax config doctor --json and fix judgment.enabled/judgment.mode"}
	}
	gate := inboxjudgment.ExperimentalJudgmentGate{Enabled: result.Config.Judgment.Enabled, Mode: result.Config.Judgment.Mode}
	if err := gate.Validate(); err != nil {
		return gate, &domain.CommandError{Code: "config_invalid", Message: "Judgment switch configuration is invalid", Hint: "Set judgment.enabled=true with judgment.mode=shadow or assist, or keep judgment.mode=off"}
	}
	return gate, nil
}

func judgmentVaultNotes(root string) ([]domain.Note, *domain.CommandError) {
	notes, err := scanNotes(root)
	if err != nil {
		var cerr *domain.CommandError
		if errors.As(err, &cerr) {
			return nil, cerr
		}
		return nil, &domain.CommandError{Code: "vault_scan_failed", Message: "Vault notes could not be scanned for judgment", Hint: "Run pinax vault doctor --json and retry"}
	}
	return ordinaryNotes(notes), nil
}

func judgmentInboxNote(notes []domain.Note, ref string) (domain.Note, *domain.CommandError) {
	if strings.TrimSpace(ref) != "" {
		note, err := resolveNoteRef(notes, ref)
		if err != nil {
			return domain.Note{}, &domain.CommandError{Code: "note_not_found", Message: "The selected inbox note is missing or ambiguous", Hint: "Run pinax inbox list --json and pass a stable note reference"}
		}
		if !isDailyIntakeNote(note) {
			return domain.Note{}, &domain.CommandError{Code: "note_not_inbox", Message: "Inbox judgment only evaluates pending inbox or unclassified index notes", Hint: "Pick a note from pinax inbox review"}
		}
		return note, nil
	}
	for _, note := range notes {
		if isDailyIntakeNote(note) {
			return note, nil
		}
	}
	return domain.Note{}, &domain.CommandError{Code: "no_pending_inbox_note", Message: "There is no pending inbox note to judge", Hint: "Capture a note with pinax inbox capture, or pass an explicit note reference"}
}

// judgmentCandidates 复用已有确定性证据：共享 tag 的笔记与内容 digest 全等
// 的重复笔记。它不猜测语义关联，也不把候选顺序当成模型结论。
func judgmentCandidates(notes []domain.Note, inbox domain.Note, limit int) []inboxjudgment.InboxJudgmentCandidate {
	if limit <= 0 {
		limit = judgmentDefaultLimit
	}
	if limit > inboxjudgment.InboxJudgmentDefaultMaxCandidates {
		limit = inboxjudgment.InboxJudgmentDefaultMaxCandidates
	}
	inboxID := judgmentNoteID(inbox)
	tags := map[string]bool{}
	for _, tag := range inbox.Tags {
		if value := strings.TrimSpace(tag); value != "" {
			tags[value] = true
		}
	}
	type ranked struct {
		note     domain.Note
		exact    bool
		overlap  int
		position int
	}
	items := make([]ranked, 0, len(notes))
	for index, note := range notes {
		if note.Path == inbox.Path || note.ContentDigest == "" {
			continue
		}
		if judgmentNoteID(note) == inboxID {
			continue
		}
		overlap := 0
		for _, tag := range note.Tags {
			if tags[strings.TrimSpace(tag)] {
				overlap++
			}
		}
		exact := inbox.ContentDigest != "" && note.ContentDigest == inbox.ContentDigest
		if overlap == 0 && !exact {
			continue
		}
		items = append(items, ranked{note: note, exact: exact, overlap: overlap, position: index})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].exact != items[j].exact {
			return items[i].exact
		}
		if items[i].overlap != items[j].overlap {
			return items[i].overlap > items[j].overlap
		}
		return items[i].position < items[j].position
	})
	if len(items) > limit {
		items = items[:limit]
	}
	candidates := make([]inboxjudgment.InboxJudgmentCandidate, 0, len(items))
	for index, item := range items {
		candidates = append(candidates, inboxjudgment.InboxJudgmentCandidate{
			CandidateID:    judgmentNoteID(item.note),
			NoteID:         judgmentNoteID(item.note),
			SourceRevision: item.note.ContentDigest,
			SourceDigest:   item.note.ContentDigest,
			InlineText:     judgmentExcerpt(item.note.Body),
			BaselineRank:   index + 1,
		})
	}
	return candidates
}

func judgmentNoteID(note domain.Note) string {
	if id := strings.TrimSpace(note.ID); id != "" {
		return id
	}
	return stableNoteRef(note)
}

func judgmentExcerpt(text string) string {
	trimmed := strings.TrimSpace(text)
	runes := []rune(trimmed)
	if len(runes) <= judgmentMaxExcerptRunes {
		return trimmed
	}
	return string(runes[:judgmentMaxExcerptRunes])
}

func judgmentPrincipal(actor string) string {
	if value := strings.TrimSpace(actor); value != "" {
		return value
	}
	return "owner:local"
}

// judgmentRequestID 把投影 digest 绑定成稳定 request 身份：同一输入与同一
// profile 的重跑落在同一 request 上，因此未知结果只会复用原回执。
func judgmentRequestID(projectionDigest string) string {
	digest := strings.TrimPrefix(projectionDigest, "sha256:")
	if len(digest) > 40 {
		digest = digest[:40]
	}
	if digest == "" {
		digest = "unbound"
	}
	return "judgment-" + digest
}

// judgmentAttemptID 默认复用原 attempt；显式传入才创建新 attempt，让“再次
// 提交”成为用户可见的决定，而不是自动重试。
func judgmentAttemptID(explicit, projectionDigest string) (string, *domain.CommandError) {
	if strings.TrimSpace(explicit) != "" {
		if !judgmentAttemptPattern.MatchString(strings.TrimSpace(explicit)) {
			return "", &domain.CommandError{Code: "invalid_attempt_id", Message: "Judgment attempt id is not a valid opaque reference", Hint: "Use letters, digits, dot, dash, colon or underscore, starting with a letter or digit"}
		}
		return strings.TrimSpace(explicit), nil
	}
	digest := strings.TrimPrefix(projectionDigest, "sha256:")
	if len(digest) > 16 {
		digest = digest[:16]
	}
	if digest == "" {
		digest = "unbound"
	}
	return "judgment-" + digest + "-a1", nil
}

// judgmentSourceAuthorizer 在 SDK 分发前复核 owner 作用域与 source 授权：
// 只有本次投影里已授权的 inbox 笔记与候选可以离开发起进程。
func judgmentSourceAuthorizer(project string, allowed []string) func(context.Context, *judgment.Request) error {
	permitted := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		permitted["inbox:"+id] = true
		permitted["note:"+id] = true
	}
	return func(_ context.Context, request *judgment.Request) error {
		if request == nil {
			return errors.New("judgment_request_missing")
		}
		if request.Scope.OwnerID == nil || *request.Scope.OwnerID != judgmentOwner {
			return errors.New("judgment_owner_scope_denied")
		}
		if request.Scope.ProjectID != nil && project != "" && *request.Scope.ProjectID != project {
			return errors.New("judgment_project_scope_denied")
		}
		for _, source := range request.Sources {
			if !permitted[source.SourceID] {
				return errors.New("judgment_source_scope_denied")
			}
		}
		return nil
	}
}

func (p InboxJudgmentProfile) executionProfile() (providers.ExecutionProfile, *domain.CommandError) {
	revision := p.Revision
	if revision == 0 {
		revision = 1
	}
	profile := providers.ExecutionProfile{
		ID:         p.ID,
		Revision:   revision,
		Provider:   p.Provider,
		Model:      p.Model,
		Enabled:    true,
		Credential: providers.CredentialBinding{Source: p.CredentialSource, Ref: p.CredentialRef, Env: p.CredentialEnv},
	}
	if err := profile.Validate(); err != nil {
		return providers.ExecutionProfile{}, &domain.CommandError{Code: "judgment_profile_invalid", Message: "Judgment execution profile selection is not a supported reference set", Hint: "Use a supported provider/model pair with an env or shared credential reference; secrets never belong in configuration"}
	}
	return profile, nil
}

func judgmentTransportProvider(provider string) string {
	switch provider {
	case "openai_decisions":
		return "openai"
	case "typesafe":
		return "typesafe"
	case "openrouter":
		return "openrouter"
	default:
		return provider
	}
}

func judgmentAdapterVersion(provider string) string {
	switch provider {
	case "openai_decisions":
		return "openai-decisions-sdk-0.2.0"
	case "typesafe":
		return "typesafe-judgment"
	case "openrouter":
		return "openrouter-decisions"
	default:
		return provider
	}
}

// judgmentCommandError 把判断边界的稳定错误码映射成 CLI envelope 错误。
// 未知结果保留 reconcile 语义，不伪装成“没有建议”。
func judgmentCommandError(err error) *domain.CommandError {
	var transportErr *inboxjudgment.JudgmentTransportError
	if errors.As(err, &transportErr) {
		code := transportErr.Code
		if code == "" {
			code = inboxjudgment.JudgmentCodeUnavailable
		}
		return &domain.CommandError{Code: code, Message: "Judgment provider call did not complete", Hint: judgmentErrorHint(code)}
	}
	var cerr *domain.CommandError
	if errors.As(err, &cerr) {
		return cerr
	}
	return &domain.CommandError{Code: "judgment_unavailable", Message: "Inbox judgment is unavailable", Hint: "Check the local judgment profile and provider readiness; the vault and the original review flow are unchanged"}
}

func judgmentErrorHint(code string) string {
	switch code {
	case inboxjudgment.JudgmentCodeOutcomeUnknown:
		return "The original attempt is unresolved: reconcile the recorded receipt first and pass an explicit --attempt only after confirming the provider side"
	case inboxjudgment.JudgmentCodeUnauthorized:
		return "Check the owner scope and the explicit execution profile; judgment never falls back to another credential"
	case inboxjudgment.JudgmentCodeRateLimited:
		return "Wait for the provider budget to reset; the original attempt was not submitted"
	case inboxjudgment.JudgmentCodeInvalidRequest, inboxjudgment.JudgmentCodeUnsupportedCapability:
		return "Adjust the explicit selection or the bounded candidate set; nothing was written to the vault"
	case inboxjudgment.JudgmentCodeInvalidResponse:
		return "The stored or returned result failed validation; keep the original review flow and do not resubmit"
	default:
		return "The provider boundary is unavailable; the original review flow stays authoritative"
	}
}

func dormantInboxJudgmentProjection() domain.Projection {
	projection := domain.NewProjection(InboxJudgmentCommand, "Experimental inbox judgment is off; the original inbox review flow is authoritative.")
	projection.Facts["judgment_mode"] = inboxjudgment.InboxJudgmentModeOff
	projection.Facts["judgment_status"] = inboxjudgment.InboxJudgmentStatusOff
	projection.Facts["provider_dispatch"] = "none"
	projection.Facts["credential_reads"] = "none"
	projection.Facts["writes"] = "false"
	projection.Actions = []domain.Action{{Name: "Review inbox notes", Command: "pinax inbox review --json"}}
	return projection
}

func dryRunInboxJudgmentProjection(projection inboxjudgment.InboxJudgmentProjection, inbox domain.Note, mode string) domain.Projection {
	result := domain.NewProjection(InboxJudgmentCommand, "Deterministic inbox judgment projection built; no provider call and no credential read.")
	result.Facts["judgment_mode"] = mode
	result.Facts["judgment_status"] = "planned"
	result.Facts["provider_dispatch"] = "none"
	result.Facts["credential_reads"] = "none"
	result.Facts["writes"] = "false"
	result.Facts["inbox_ref"] = stableNoteRef(inbox)
	result.Facts["candidates"] = strconv.Itoa(len(projection.Candidates))
	result.Facts["projection_digest"] = projection.Digest
	result.Facts["deterministic_findings"] = strconv.Itoa(len(projection.Deterministic))
	result.Data = projection
	result.Actions = []domain.Action{{Name: "Evaluate for real", Command: fmt.Sprintf("pinax inbox judgment --note %s --json", shellQuote(stableNoteRef(inbox)))}}
	return result
}

func inboxJudgmentOutcomeProjection(outcome inboxjudgment.InboxJudgmentOutcome, inbox domain.Note, mode string, profile InboxJudgmentProfile, requestID, attemptID string, candidateCount int) (domain.Projection, error) {
	if outcome.Suggestion != nil {
		handoff, err := outcome.Suggestion.ReviewHandoff(outcome.Evidence, []string{stableNoteRef(inbox)})
		if err != nil {
			return errorProjection(InboxJudgmentCommand, err), err
		}
		projection, err := handoff.Projection()
		if err != nil {
			return errorProjection(InboxJudgmentCommand, err), err
		}
		projection.Facts["judgment_mode"] = mode
		projection.Facts["judgment_status"] = outcome.Status
		projection.Facts["provider_dispatch"] = "completed"
		projection.Facts["inbox_ref"] = stableNoteRef(inbox)
		projection.Facts["candidates"] = strconv.Itoa(candidateCount)
		projection.Facts["model"] = profile.Model
		projection.Facts["request_id"] = requestID
		projection.Facts["attempt_id"] = attemptID
		projection.Facts["writes"] = "false"
		return projection, nil
	}
	projection := domain.NewProjection(InboxJudgmentCommand, "Inbox judgment produced no suggestion; the original inbox review flow stays authoritative.")
	projection.Status = "partial"
	projection.Facts["judgment_mode"] = mode
	projection.Facts["judgment_status"] = outcome.Status
	projection.Facts["provider_dispatch"] = judgmentDispatchFact(outcome.Evidence)
	projection.Facts["inbox_ref"] = stableNoteRef(inbox)
	projection.Facts["candidates"] = strconv.Itoa(candidateCount)
	projection.Facts["model"] = profile.Model
	projection.Facts["request_id"] = requestID
	projection.Facts["attempt_id"] = attemptID
	projection.Facts["reason_code"] = outcome.Evidence.ReasonCode
	projection.Facts["submission_state"] = outcome.Evidence.SubmissionState
	projection.Facts["retry_class"] = outcome.Evidence.RetryClass
	projection.Facts["writes"] = "false"
	projection.Warnings = append(projection.Warnings, domain.ProjectionWarning{
		Code:    judgmentWarningCode(outcome.Evidence),
		Message: "Inbox judgment did not produce an adoptable suggestion",
		Hint:    "Reconcile the original attempt before retrying; do not resubmit automatically",
	})
	projection.Actions = []domain.Action{
		{Name: "Reconcile original attempt", Command: "pinax inbox judgment --attempt <explicit-new-attempt>"},
		{Name: "Review inbox notes", Command: "pinax inbox review --json"},
	}
	projection.Data = outcome.Evidence
	return projection, nil
}

func judgmentDispatchFact(evidence inboxjudgment.InboxJudgmentEvidence) string {
	switch evidence.SubmissionState {
	case inboxjudgment.JudgmentSubmissionNotSubmitted:
		return "not_submitted"
	case inboxjudgment.JudgmentSubmissionSubmitted:
		return "submitted"
	case inboxjudgment.JudgmentSubmissionUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func judgmentWarningCode(evidence inboxjudgment.InboxJudgmentEvidence) string {
	if evidence.ReasonCode != "" {
		return evidence.ReasonCode
	}
	return evidence.Status
}
