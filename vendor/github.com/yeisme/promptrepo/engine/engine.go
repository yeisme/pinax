package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/promptrepo"
	"github.com/yeisme/promptrepo/source"
	"golang.org/x/text/unicode/norm"
)

type Options struct {
	ConfigRoot   string
	CacheRoot    string
	LockTimeout  time.Duration
	StaleLockAge time.Duration
	Now          func() time.Time
	Sources      *source.Registry
}

type Manager struct {
	configRoot   string
	cacheRoot    string
	lockTimeout  time.Duration
	staleLockAge time.Duration
	now          func() time.Time
	sources      *source.Registry
}

type stateFile struct {
	SchemaVersion string                                  `json:"schema_version"`
	Profiles      map[string]promptrepo.RepositoryProfile `json:"profiles"`
	Health        map[string]promptrepo.RepositoryHealth  `json:"health"`
	Snapshots     map[string]promptrepo.Snapshot          `json:"snapshots"`
	Receipts      map[string]promptrepo.StageReceipt      `json:"receipts"`
}

func New(options Options) (*Manager, error) {
	configRoot := strings.TrimSpace(options.ConfigRoot)
	if configRoot == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		configRoot = filepath.Join(base, "yeisme", "promptrepo")
	}
	cacheRoot := strings.TrimSpace(options.CacheRoot)
	if cacheRoot == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		cacheRoot = filepath.Join(base, "yeisme", "promptrepo")
	}
	lockTimeout := options.LockTimeout
	if lockTimeout <= 0 {
		lockTimeout = 2 * time.Second
	}
	staleLockAge := options.StaleLockAge
	if staleLockAge <= 0 {
		staleLockAge = 30 * time.Minute
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	sources := options.Sources
	if sources == nil {
		sources = source.DefaultRegistry()
	}
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		return nil, err
	}
	return &Manager{configRoot: configRoot, cacheRoot: cacheRoot, lockTimeout: lockTimeout, staleLockAge: staleLockAge, now: now, sources: sources}, nil
}

func (m *Manager) AddRepository(_ context.Context, request promptrepo.AddRepositoryRequest) (promptrepo.RepositoryView, error) {
	profile := request.Profile
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Source = strings.TrimSpace(profile.Source)
	if profile.ID == "" || profile.Source == "" || !safeID(profile.ID) {
		return promptrepo.RepositoryView{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "repository id and source are required", false, nil)
	}
	kind, err := source.DetectKind(profile.Source)
	if err != nil {
		return promptrepo.RepositoryView{}, err
	}
	now := m.now().UTC()
	profile.SourceKind = kind
	if profile.Scope == "" {
		profile.Scope = "user"
	}
	if profile.Trust == "" {
		profile.Trust = "untrusted"
	}
	profile.Enabled = true
	profile.CreatedAt = now
	profile.UpdatedAt = now
	var view promptrepo.RepositoryView
	err = m.withWriteState(func(state *stateFile) error {
		if _, exists := state.Profiles[profile.ID]; exists {
			return promptrepo.NewError(promptrepo.CodeAlreadyExists, "repository already exists", false, nil)
		}
		state.Profiles[profile.ID] = profile
		health := promptrepo.RepositoryHealth{State: "configured", CheckedAt: now}
		state.Health[profile.ID] = health
		view = promptrepo.RepositoryView{Profile: profile, Health: health}
		return nil
	})
	return view, err
}

func (m *Manager) ListRepositories(_ context.Context, _ promptrepo.ListRepositoriesRequest) (promptrepo.RepositoryPage, error) {
	state, err := m.readState()
	if err != nil {
		return promptrepo.RepositoryPage{}, err
	}
	ids := make([]string, 0, len(state.Profiles))
	for id := range state.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	page := promptrepo.RepositoryPage{Repositories: make([]promptrepo.RepositoryView, 0, len(ids))}
	for _, id := range ids {
		page.Repositories = append(page.Repositories, viewFor(state, id))
	}
	return page, nil
}

func (m *Manager) ShowRepository(_ context.Context, id string) (promptrepo.RepositoryView, error) {
	state, err := m.readState()
	if err != nil {
		return promptrepo.RepositoryView{}, err
	}
	if _, ok := state.Profiles[id]; !ok {
		return promptrepo.RepositoryView{}, promptrepo.NewError(promptrepo.CodeNotFound, "repository was not found", false, nil)
	}
	return viewFor(state, id), nil
}

func (m *Manager) RemoveRepository(_ context.Context, id string) error {
	return m.withWriteState(func(state *stateFile) error {
		if _, ok := state.Profiles[id]; !ok {
			return promptrepo.NewError(promptrepo.CodeNotFound, "repository was not found", false, nil)
		}
		delete(state.Profiles, id)
		delete(state.Health, id)
		delete(state.Snapshots, id)
		return nil
	})
}

func (m *Manager) SetRepositoryEnabled(_ context.Context, id string, enabled bool) (promptrepo.RepositoryView, error) {
	var view promptrepo.RepositoryView
	err := m.withWriteState(func(state *stateFile) error {
		profile, ok := state.Profiles[id]
		if !ok {
			return promptrepo.NewError(promptrepo.CodeNotFound, "repository was not found", false, nil)
		}
		profile.Enabled = enabled
		profile.UpdatedAt = m.now().UTC()
		state.Profiles[id] = profile
		health := state.Health[id]
		if !enabled {
			health.State = "disabled"
		} else if _, ok := state.Snapshots[id]; ok {
			health.State = "ready"
		} else {
			health.State = "configured"
		}
		health.CheckedAt = m.now().UTC()
		state.Health[id] = health
		view = viewFor(state, id)
		return nil
	})
	return view, err
}

func (m *Manager) SyncRepositories(ctx context.Context, request promptrepo.SyncRequest) (promptrepo.SyncReceipt, error) {
	receipt := promptrepo.SyncReceipt{SchemaVersion: "promptrepo.sync_receipt.v0.1", StartedAt: m.now().UTC()}
	err := m.withWriteState(func(state *stateFile) error {
		ids, err := selectedRepositories(state, request)
		if err != nil {
			return err
		}
		for _, id := range ids {
			profile := state.Profiles[id]
			if !profile.Enabled {
				receipt.Results = append(receipt.Results, promptrepo.RepositorySyncResult{RepositoryID: id, State: "disabled"})
				continue
			}
			adapter, resolveErr := m.sources.Resolve(profile.Source)
			if resolveErr != nil {
				receipt.Results = append(receipt.Results, m.recordSyncFailure(state, id, resolveErr))
				continue
			}
			result, syncErr := adapter.Sync(ctx, profile, m.cacheRoot)
			if syncErr != nil {
				receipt.Results = append(receipt.Results, m.recordSyncFailure(state, id, syncErr))
				continue
			}
			catalog := result.Catalog
			for index := range catalog.Solutions {
				catalog.Solutions[index].RepositoryID = id
			}
			snapshot := promptrepo.Snapshot{RepositoryID: id, Revision: result.Revision, Digest: catalog.Digest, FetchedAt: m.now().UTC(), Catalog: catalog}
			state.Snapshots[id] = snapshot
			state.Health[id] = promptrepo.RepositoryHealth{State: "ready", CheckedAt: snapshot.FetchedAt, SnapshotAt: snapshot.FetchedAt}
			receipt.Results = append(receipt.Results, promptrepo.RepositorySyncResult{RepositoryID: id, State: "ready", Revision: snapshot.Revision, Digest: snapshot.Digest, SolutionCount: len(catalog.Solutions)})
		}
		return nil
	})
	receipt.FinishedAt = m.now().UTC()
	return receipt, err
}

func (m *Manager) Search(_ context.Context, request promptrepo.SearchRequest) (promptrepo.SearchResult, error) {
	state, err := m.readState()
	if err != nil {
		return promptrepo.SearchResult{}, err
	}
	requestedLocale := strings.TrimSpace(request.Locale)
	if requestedLocale == "" {
		requestedLocale = "zh-CN"
	}
	result := promptrepo.SearchResult{RankingProfile: promptrepo.RankingProfileV1, RequestedLocale: requestedLocale}
	normalizedQuery := normalizeSearch(request.Query)
	for id, snapshot := range state.Snapshots {
		profile := state.Profiles[id]
		if !profile.Enabled {
			continue
		}
		for _, solution := range snapshot.Catalog.Solutions {
			locale, display := chooseLocale(solution, requestedLocale, snapshot.Catalog.Repository.DefaultLocale)
			missing := missingCapabilities(solution.Capabilities, request.RequiredCapabilities)
			compatible := len(missing) == 0
			if !compatible && !request.IncludeIncompatible {
				continue
			}
			if !containsAll(solution.Tags, request.Tags) {
				continue
			}
			score, reasons := searchScore(normalizedQuery, display, solution)
			if normalizedQuery != "" && score == 0 {
				continue
			}
			ref := promptrepo.FormatRef(promptrepo.Ref{RepositoryID: id, PackageID: solution.PackageID, SolutionID: solution.ID, Version: solution.Version, Locale: locale})
			result.Results = append(result.Results, promptrepo.SolutionCard{
				Ref: ref, Title: display.Title, Summary: display.Summary, Locale: locale, Category: solution.Category,
				Tags: append([]string{}, solution.Tags...), Capabilities: append([]string{}, solution.Capabilities...), Rights: solution.Rights,
				Maturity: solution.Maturity, Trust: profile.Trust, Health: state.Health[id].State, Compatible: compatible,
				Missing: missing, MatchReasons: reasons, Score: score, SnapshotDigest: snapshot.Digest, SolutionDigest: solution.Digest,
			})
		}
	}
	sort.Slice(result.Results, func(i, j int) bool {
		if result.Results[i].Compatible != result.Results[j].Compatible {
			return result.Results[i].Compatible
		}
		if result.Results[i].Score != result.Results[j].Score {
			return result.Results[i].Score > result.Results[j].Score
		}
		if trustRank(result.Results[i].Trust) != trustRank(result.Results[j].Trust) {
			return trustRank(result.Results[i].Trust) > trustRank(result.Results[j].Trust)
		}
		return result.Results[i].Ref < result.Results[j].Ref
	})
	return result, nil
}

func (m *Manager) Resolve(_ context.Context, request promptrepo.ResolveRequest) (promptrepo.ResolvedSolution, error) {
	parsed, err := promptrepo.ParseRef(request.Ref)
	if err != nil {
		return promptrepo.ResolvedSolution{}, err
	}
	state, err := m.readState()
	if err != nil {
		return promptrepo.ResolvedSolution{}, err
	}
	snapshot, ok := state.Snapshots[parsed.RepositoryID]
	if !ok {
		return promptrepo.ResolvedSolution{}, promptrepo.NewError(promptrepo.CodeNotFound, "repository has no synchronized snapshot", false, nil)
	}
	profile := state.Profiles[parsed.RepositoryID]
	for _, solution := range snapshot.Catalog.Solutions {
		if solution.PackageID != parsed.PackageID || solution.ID != parsed.SolutionID || solution.Version != parsed.Version {
			continue
		}
		requestedLocale := parsed.Locale
		if request.Locale != "" {
			requestedLocale = request.Locale
		}
		locale, display := chooseLocale(solution, requestedLocale, snapshot.Catalog.Repository.DefaultLocale)
		missing := missingCapabilities(solution.Capabilities, request.RequiredCapabilities)
		return promptrepo.ResolvedSolution{
			Ref:      promptrepo.FormatRef(promptrepo.Ref{RepositoryID: parsed.RepositoryID, PackageID: parsed.PackageID, SolutionID: parsed.SolutionID, Version: parsed.Version, Locale: locale}),
			Solution: solution, Locale: locale, Display: display, Snapshot: snapshotMetadata(snapshot), Trust: profile.Trust,
			Health: state.Health[parsed.RepositoryID].State, Compatible: len(missing) == 0, Missing: missing,
		}, nil
	}
	return promptrepo.ResolvedSolution{}, promptrepo.NewError(promptrepo.CodeNotFound, "prompt solution was not found", false, nil)
}

func (m *Manager) ReadTemplate(ctx context.Context, request promptrepo.ReadTemplateRequest) (promptrepo.TemplateContent, error) {
	resolved, err := m.Resolve(ctx, promptrepo.ResolveRequest{Ref: request.Ref, Locale: request.Locale})
	if err != nil {
		return promptrepo.TemplateContent{}, err
	}
	role := strings.TrimSpace(request.Role)
	if role == "" {
		role = "main"
	}
	var selected *promptrepo.TemplateRole
	for index := range resolved.Solution.Templates {
		template := &resolved.Solution.Templates[index]
		if template.Role == role && template.Locale == resolved.Locale {
			selected = template
			break
		}
	}
	if selected == nil {
		return promptrepo.TemplateContent{}, promptrepo.NewError(promptrepo.CodeNotFound, "prompt template role and locale were not found", false, nil)
	}
	state, err := m.readState()
	if err != nil {
		return promptrepo.TemplateContent{}, err
	}
	profile, ok := state.Profiles[resolved.Snapshot.RepositoryID]
	if !ok || !profile.Enabled {
		return promptrepo.TemplateContent{}, promptrepo.NewError(promptrepo.CodeNotFound, "prompt repository is not enabled", false, nil)
	}
	adapter, err := m.sources.Resolve(profile.Source)
	if err != nil {
		return promptrepo.TemplateContent{}, err
	}
	payload, err := adapter.ReadTemplate(ctx, profile, resolved.Snapshot, *selected, m.cacheRoot)
	if err != nil {
		return promptrepo.TemplateContent{}, err
	}
	return promptrepo.TemplateContent{Ref: resolved.Ref, Role: selected.Role, Locale: selected.Locale, Path: selected.Path, Digest: selected.Digest, Body: string(payload), Snapshot: resolved.Snapshot}, nil
}

func (m *Manager) Stage(ctx context.Context, request promptrepo.StageRequest) (promptrepo.StageReceipt, error) {
	resolved, err := m.Resolve(ctx, request.ResolveRequest)
	if err != nil {
		return promptrepo.StageReceipt{}, err
	}
	if !resolved.Compatible {
		return promptrepo.StageReceipt{}, promptrepo.NewError(promptrepo.CodeIncompatible, "prompt solution is incompatible with the consumer", false, nil)
	}
	if strings.EqualFold(resolved.Solution.Rights, "blocked") || strings.EqualFold(resolved.Solution.Rights, "prohibited") {
		return promptrepo.StageReceipt{}, promptrepo.NewError(promptrepo.CodeRightsBlocked, "prompt solution rights block installation", false, nil)
	}
	if hasTemplateFor(resolved.Solution.Templates, "main", resolved.Locale) {
		if _, err := m.ReadTemplate(ctx, promptrepo.ReadTemplateRequest{Ref: resolved.Ref, Locale: resolved.Locale, Role: "main"}); err != nil {
			return promptrepo.StageReceipt{}, err
		}
	}
	stateName := "staged_pending_review"
	if strings.EqualFold(resolved.Solution.Rights, "unknown") {
		stateName = "review_only"
	}
	createdAt := m.now().UTC()
	idDigest := sha256.Sum256([]byte(request.Consumer + "\x00" + resolved.Ref + "\x00" + resolved.Snapshot.Digest))
	receipt := promptrepo.StageReceipt{
		SchemaVersion: promptrepo.ReceiptSchemaVersion, ID: "stage_" + hex.EncodeToString(idDigest[:8]), CreatedAt: createdAt,
		Consumer: request.Consumer, Ref: resolved.Ref, RepositoryID: resolved.Snapshot.RepositoryID,
		SnapshotRevision: resolved.Snapshot.Revision, SnapshotDigest: resolved.Snapshot.Digest, SolutionDigest: resolved.Solution.Digest,
		Locale: resolved.Locale, Rights: resolved.Solution.Rights, Trust: resolved.Trust, Compatibility: "compatible",
		State: stateName, ProviderCalls: "disabled",
	}
	if err := m.withWriteState(func(state *stateFile) error {
		state.Receipts[receipt.ID] = receipt
		return nil
	}); err != nil {
		return promptrepo.StageReceipt{}, err
	}
	return receipt, nil
}

func (m *Manager) recordSyncFailure(state *stateFile, id string, err error) promptrepo.RepositorySyncResult {
	code := promptrepo.ErrorCode(err)
	healthState := "degraded"
	if code == promptrepo.CodeDigestMismatch || code == promptrepo.CodeInvalidRequest {
		healthState = "quarantined"
	}
	state.Health[id] = promptrepo.RepositoryHealth{State: healthState, Code: code, Message: safeErrorMessage(err), CheckedAt: m.now().UTC()}
	return promptrepo.RepositorySyncResult{RepositoryID: id, State: healthState, ErrorCode: code, Message: safeErrorMessage(err)}
}

func (m *Manager) statePath() string { return filepath.Join(m.configRoot, "state.json") }
func (m *Manager) lockPath() string  { return filepath.Join(m.configRoot, "state.lock") }

func (m *Manager) readState() (*stateFile, error) {
	payload, err := os.ReadFile(m.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, err
	}
	var state stateFile
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "prompt repository state is invalid", false, err)
	}
	if state.SchemaVersion != promptrepo.StateSchemaVersion {
		return nil, promptrepo.NewError(promptrepo.CodeStateSchemaTooNew, "prompt repository state schema is not supported by this SDK", false, nil)
	}
	ensureStateMaps(&state)
	return &state, nil
}

func (m *Manager) withWriteState(update func(*stateFile) error) error {
	release, err := m.acquireLock()
	if err != nil {
		return err
	}
	defer release()
	state, err := m.readState()
	if err != nil {
		return err
	}
	if err := update(state); err != nil {
		return err
	}
	return m.writeState(state)
}

func (m *Manager) acquireLock() (func(), error) {
	deadline := time.Now().Add(m.lockTimeout)
	for {
		file, err := os.OpenFile(m.lockPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\ncreated_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = file.Close()
			return func() { _ = os.Remove(m.lockPath()) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(m.lockPath()); statErr == nil && time.Since(info.ModTime()) > m.staleLockAge {
			if removeErr := os.Remove(m.lockPath()); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, promptrepo.NewError(promptrepo.CodeStateLocked, "prompt repository state is locked by another process", true, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (m *Manager) writeState(state *stateFile) error {
	state.SchemaVersion = promptrepo.StateSchemaVersion
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(m.configRoot, "state-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, m.statePath())
}

func newState() *stateFile {
	return &stateFile{SchemaVersion: promptrepo.StateSchemaVersion, Profiles: map[string]promptrepo.RepositoryProfile{}, Health: map[string]promptrepo.RepositoryHealth{}, Snapshots: map[string]promptrepo.Snapshot{}, Receipts: map[string]promptrepo.StageReceipt{}}
}

func ensureStateMaps(state *stateFile) {
	if state.Profiles == nil {
		state.Profiles = map[string]promptrepo.RepositoryProfile{}
	}
	if state.Health == nil {
		state.Health = map[string]promptrepo.RepositoryHealth{}
	}
	if state.Snapshots == nil {
		state.Snapshots = map[string]promptrepo.Snapshot{}
	}
	if state.Receipts == nil {
		state.Receipts = map[string]promptrepo.StageReceipt{}
	}
}

func viewFor(state *stateFile, id string) promptrepo.RepositoryView {
	view := promptrepo.RepositoryView{Profile: state.Profiles[id], Health: state.Health[id]}
	if snapshot, ok := state.Snapshots[id]; ok {
		metadata := snapshotMetadata(snapshot)
		view.Snapshot = &metadata
	}
	return view
}

func snapshotMetadata(snapshot promptrepo.Snapshot) promptrepo.SnapshotMetadata {
	return promptrepo.SnapshotMetadata{RepositoryID: snapshot.RepositoryID, Revision: snapshot.Revision, Digest: snapshot.Digest, FetchedAt: snapshot.FetchedAt}
}

func hasTemplateFor(templates []promptrepo.TemplateRole, role, locale string) bool {
	for _, template := range templates {
		if template.Role == role && template.Locale == locale {
			return true
		}
	}
	return false
}

func selectedRepositories(state *stateFile, request promptrepo.SyncRequest) ([]string, error) {
	if request.All {
		ids := make([]string, 0, len(state.Profiles))
		for id := range state.Profiles {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids, nil
	}
	if len(request.RepositoryIDs) == 0 {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "sync requires repository ids or all=true", false, nil)
	}
	ids := append([]string{}, request.RepositoryIDs...)
	for _, id := range ids {
		if _, ok := state.Profiles[id]; !ok {
			return nil, promptrepo.NewError(promptrepo.CodeNotFound, "repository was not found", false, nil)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func chooseLocale(solution promptrepo.Solution, requested, fallback string) (string, promptrepo.LocalizedText) {
	for _, locale := range []string{requested, fallback, "zh-CN", "en"} {
		if display, ok := solution.Locales[locale]; ok {
			return locale, display
		}
	}
	locales := make([]string, 0, len(solution.Locales))
	for locale := range solution.Locales {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	if len(locales) == 0 {
		return "", promptrepo.LocalizedText{}
	}
	return locales[0], solution.Locales[locales[0]]
}

func missingCapabilities(actual, required []string) []string {
	set := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		set[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	var missing []string
	for _, value := range required {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := set[value]; !ok {
			missing = append(missing, value)
		}
	}
	sort.Strings(missing)
	return missing
}

func containsAll(actual, required []string) bool {
	set := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		set[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range required {
		if _, ok := set[strings.ToLower(strings.TrimSpace(value))]; !ok {
			return false
		}
	}
	return true
}

func searchScore(query string, display promptrepo.LocalizedText, solution promptrepo.Solution) (int, []string) {
	if query == "" {
		return 1, []string{"browse"}
	}
	score := 0
	var reasons []string
	if strings.Contains(normalizeSearch(display.Title), query) {
		score += 40
		reasons = append(reasons, "title")
	}
	for _, alias := range display.Aliases {
		if strings.Contains(normalizeSearch(alias), query) || strings.Contains(query, normalizeSearch(alias)) {
			score += 30
			reasons = append(reasons, "alias")
			break
		}
	}
	if strings.Contains(normalizeSearch(display.Summary), query) {
		score += 15
		reasons = append(reasons, "summary")
	}
	for _, tag := range solution.Tags {
		if strings.Contains(normalizeSearch(tag), query) {
			score += 10
			reasons = append(reasons, "tag")
			break
		}
	}
	return score, reasons
}

func normalizeSearch(value string) string {
	// x/text/norm versions that still support Go 1.24 can loop on invalid
	// UTF-8. Keep the public Go 1.24 compatibility floor while making the
	// normalization boundary safe before passing caller/catalog text to NFKC.
	value = strings.ToValidUTF8(value, "\uFFFD")
	value = norm.NFKC.String(strings.ToLower(strings.TrimSpace(value)))
	var builder strings.Builder
	space := false
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if !space {
				builder.WriteByte(' ')
				space = true
			}
			continue
		}
		space = false
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

func trustRank(value string) int {
	switch value {
	case "official":
		return 4
	case "verified":
		return 3
	case "user_trusted":
		return 2
	default:
		return 1
	}
}

func safeID(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func safeErrorMessage(err error) string {
	if typed, ok := err.(*promptrepo.Error); ok {
		return typed.Message
	}
	return "repository operation failed"
}
