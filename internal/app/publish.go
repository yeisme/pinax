package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/publishops"
	"github.com/yeisme/pinax/internal/app/syncdaemon"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	"gopkg.in/yaml.v3"
)

type PublishRequest struct {
	VaultPath  string
	Profile    string
	Target     string
	Renderer   string
	Title      string
	BaseURL    string
	Theme      string
	Out        string
	Repo       string
	Branch     string
	Endpoint   string
	Method     string
	SecretRef  string
	GistID     string
	Visibility string
	Project    string
	Host       string
	Port       int
	Once       bool
	Watch      bool
	Yes        bool
	LiveEvents PublishEventSink
}

type PublishEventSink func(PublishEvent)

type PublishEvent struct {
	Type   string
	Status string
	Facts  map[string]string
}

func emitPublishEvent(sink PublishEventSink, eventType string, status string, facts map[string]string) {
	if sink == nil {
		return
	}
	if status == "" {
		status = "running"
	}
	safeFacts := make(map[string]string, len(facts))
	for key, value := range facts {
		if strings.TrimSpace(value) != "" {
			safeFacts[key] = value
		}
	}
	sink(PublishEvent{Type: eventType, Status: status, Facts: safeFacts})
}

func publishProfileEventFacts(profile domain.PublishProfile) map[string]string {
	return map[string]string{"profile": profile.Name, "target": string(profile.Target), "renderer": string(profile.Renderer)}
}

func (s *Service) PublishProfileInit(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.profile.init", err), err
	}
	name, commandErr := normalizePublishProfileName(req.Profile)
	if commandErr != nil {
		return domain.NewErrorProjection("publish.profile.init", commandErr), commandErr
	}
	target := domain.PublishTarget(strings.TrimSpace(req.Target))
	if target == "" {
		target = domain.PublishTargetGitHubPages
	}
	renderer := domain.PublishRenderer(strings.TrimSpace(req.Renderer))
	if renderer == "" {
		renderer = domain.PublishRendererPinaxWeb
	}
	profile := domain.NewDefaultPublishProfile(name, target, renderer)
	if strings.TrimSpace(req.Title) != "" {
		profile.Site.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		profile.Site.BaseURL = strings.TrimSpace(req.BaseURL)
	}
	if strings.TrimSpace(req.Theme) != "" {
		profile.Site.Theme.Value = strings.TrimSpace(req.Theme)
	}
	issues := publishops.ValidateProfile(profile)
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.profile.init", name, issues)
		return publishProfileProjection("publish.profile.init", profile, issues, "failed"), cmdErr
	}
	if err := writePublishProfile(root, profile); err != nil {
		return errorProjection("publish.profile.init", err), err
	}
	emitPublishEvent(req.LiveEvents, "profile_ready", "success", publishProfileEventFacts(profile))
	projection := publishProfileProjection("publish.profile.init", profile, nil, "success")
	projection.Summary = "Publish profile initialized."
	projection.Evidence = []string{publishProfileRelPath(name)}
	return projection, nil
}

func (s *Service) PublishProfileValidate(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.profile.validate")
	if err != nil {
		return errorProjection("publish.profile.validate", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.profile.validate", profile.Name, issues)
		return publishProfileProjection("publish.profile.validate", profile, issues, "failed"), cmdErr
	}
	projection := publishProfileProjection("publish.profile.validate", profile, nil, "success")
	projection.Summary = "Publish profile is valid."
	migrationActions := projection.Actions
	projection.Actions = []domain.Action{{Name: "plan", Command: fmt.Sprintf("pinax publish plan --profile %s --target %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	projection.Actions = append(projection.Actions, migrationActions...)
	return projection, nil
}

func (s *Service) PublishProfileShow(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.profile.show")
	if err != nil {
		return errorProjection("publish.profile.show", err), err
	}
	projection := publishProfileProjection("publish.profile.show", profile, issues, "success")
	projection.Summary = "Publish profile loaded."
	return projection, nil
}

func (s *Service) PublishProfileList(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.profile.list", err), err
	}
	profiles, err := listPublishProfiles(root)
	if err != nil {
		return errorProjection("publish.profile.list", err), err
	}
	projection := domain.NewProjection("publish.profile.list", "Publish profiles listed.")
	projection.Facts["profiles"] = fmt.Sprint(len(profiles))
	projection.Data = map[string]any{"profiles": profiles}
	if len(profiles) > 0 {
		projection.Actions = []domain.Action{{Name: "validate", Command: fmt.Sprintf("pinax publish profile validate %s --vault <vault> --json", shellQuote(profiles[0].Name))}}
	}
	return projection, nil
}

func (s *Service) PublishDoctor(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.doctor")
	if err != nil {
		return errorProjection("publish.doctor", err), err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.doctor", err), err
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	outSafe := true
	outDir := ""
	outExists := false
	scanStatus := "not_checked"
	scanFindings := 0
	latestReceipt := false
	previewApproved := false
	if strings.TrimSpace(req.Out) != "" {
		cleanOut, outErr := cleanPublishOutPath(req.Out)
		outSafe = outErr == nil
		if outErr != nil {
			issues = append(issues, domain.PublishValidationIssue{Code: outErr.Code, Field: "out", Severity: "error", Message: outErr.Message})
		} else {
			outDir = cleanOut
			if _, statErr := os.Stat(outDir); statErr == nil {
				outExists = true
				scan, scanErr := publishops.ScanPublishTree(outDir)
				if scanErr != nil {
					issues = append(issues, domain.PublishValidationIssue{Code: "publish_scan_failed", Field: "out", Severity: "warning", Message: scanErr.Error()})
					scanStatus = "failed"
				} else {
					scanFindings = len(scan.Findings)
					if scanFindings == 0 {
						scanStatus = "clean"
					} else {
						scanStatus = "blocked"
					}
				}
				if outputHash, hashErr := hashPublishTree(outDir); hashErr == nil {
					if _, receiptErr := latestPublishReceiptForOutput(root, profile.Name, outputHash); receiptErr == nil {
						latestReceipt = true
					}
					if _, previewErr := latestPreviewApprovalReceipt(root, profile.Name, outputHash); previewErr == nil {
						previewApproved = true
					}
				} else {
					issues = append(issues, domain.PublishValidationIssue{Code: "publish_hash_failed", Field: "out", Severity: "warning", Message: hashErr.Error()})
				}
			} else if errors.Is(statErr, os.ErrNotExist) {
				scanStatus = "missing"
			} else {
				issues = append(issues, domain.PublishValidationIssue{Code: "publish_output_unreadable", Field: "out", Severity: "warning", Message: statErr.Error()})
				scanStatus = "failed"
			}
		}
	}
	hugoAvailable := false
	if profile.Target == domain.PublishTargetGitHubPages && profile.Renderer == domain.PublishRendererHugo {
		_, hugoErr := exec.LookPath("hugo")
		hugoAvailable = hugoErr == nil
		if hugoErr != nil {
			issues = append(issues, domain.PublishValidationIssue{Code: "hugo_unavailable", Field: "hugo", Severity: "warning", Message: "Hugo executable was not found on PATH"})
		}
	}
	_, gitErr := exec.LookPath("git")
	gitAvailable := gitErr == nil
	_, vercelErr := exec.LookPath("vercel")
	vercelAvailable := vercelErr == nil
	_, wranglerErr := exec.LookPath("wrangler")
	wranglerAvailable := wranglerErr == nil
	if profile.Target == domain.PublishTargetVercel && !vercelAvailable {
		issues = append(issues, domain.PublishValidationIssue{Code: "publish_vercel_cli_missing", Field: "vercel", Severity: "warning", Message: "vercel CLI was not found on PATH"})
	}
	if profile.Target == domain.PublishTargetCloudflare && !wranglerAvailable {
		issues = append(issues, domain.PublishValidationIssue{Code: "publish_wrangler_cli_missing", Field: "wrangler", Severity: "warning", Message: "wrangler CLI was not found on PATH"})
	}
	if (profile.Target == domain.PublishTargetGitHubPages || profile.Target == domain.PublishTargetGitHubWiki) && !gitAvailable {
		issues = append(issues, domain.PublishValidationIssue{Code: "git_unavailable", Field: "git", Severity: "warning", Message: "git executable was not found on PATH"})
	}
	projection := domain.NewProjection("publish.doctor", "发布环境检查完成。")
	if len(issues) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["profile_issues"] = fmt.Sprint(len(publishops.ValidateProfile(profile)))
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Facts["hugo_available"] = fmt.Sprint(hugoAvailable)
	projection.Facts["git_available"] = fmt.Sprint(gitAvailable)
	projection.Facts["vercel_available"] = fmt.Sprint(vercelAvailable)
	projection.Facts["wrangler_available"] = fmt.Sprint(wranglerAvailable)
	projection.Facts["theme"] = profile.Site.Theme.Value
	projection.Facts["out_safe"] = fmt.Sprint(outSafe)
	projection.Facts["out_exists"] = fmt.Sprint(outExists)
	projection.Facts["scan_status"] = scanStatus
	projection.Facts["scan_findings"] = fmt.Sprint(scanFindings)
	projection.Facts["latest_receipt"] = fmt.Sprint(latestReceipt)
	projection.Facts["preview_approved"] = fmt.Sprint(previewApproved)
	projection.Data = map[string]any{"profile": profile.Name, "issues": issues}
	if len(publishops.ValidateProfile(profile)) == 0 {
		projection.Actions = publishDoctorActions(profile, outDir, outExists, latestReceipt, previewApproved)
	} else {
		projection.Actions = []domain.Action{{Name: "profile_validate", Command: fmt.Sprintf("pinax publish profile validate %s --vault <vault> --json", shellQuote(profile.Name))}}
	}
	return projection, nil
}

func publishDoctorActions(profile domain.PublishProfile, outDir string, outExists, latestReceipt, previewApproved bool) []domain.Action {
	outArg := "<out>"
	if outDir != "" {
		outArg = "<out>"
	}
	actions := []domain.Action{{Name: "build", Command: fmt.Sprintf("pinax publish build --profile %s --target %s --out %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)), outArg)}}
	if outExists && latestReceipt {
		actions = append(actions, domain.Action{Name: "preview_approve", Command: fmt.Sprintf("pinax publish preview approve --profile %s --out %s --vault <vault> --json", shellQuote(profile.Name), outArg)})
	}
	if previewApproved {
		action := domain.Action{Name: "deploy", Command: publishDoctorDeployCommand(profile, outArg)}
		if action.Command != "" {
			actions = append(actions, action)
		}
	}
	return actions
}

func publishDoctorDeployCommand(profile domain.PublishProfile, outArg string) string {
	switch profile.Target {
	case domain.PublishTargetVercel:
		project := strings.TrimSpace(profile.Deploy.Project)
		if project == "" {
			project = "<project>"
		} else {
			project = shellQuote(project)
		}
		return fmt.Sprintf("pinax publish deploy --profile %s --target vercel --out %s --project %s --yes --vault <vault> --json", shellQuote(profile.Name), outArg, project)
	case domain.PublishTargetCloudflare:
		project := strings.TrimSpace(profile.Deploy.Project)
		if project == "" {
			project = "<project>"
		} else {
			project = shellQuote(project)
		}
		return fmt.Sprintf("pinax publish deploy --profile %s --target cloudflare-pages --out %s --project %s --yes --vault <vault> --json", shellQuote(profile.Name), outArg, project)
	case domain.PublishTargetGitHubPages:
		repo := strings.TrimSpace(profile.Deploy.Repo)
		if repo == "" {
			repo = "<repo>"
		}
		branch := strings.TrimSpace(profile.Deploy.Branch)
		if branch == "" {
			branch = "gh-pages"
		}
		return fmt.Sprintf("pinax publish deploy --profile %s --target github-pages --out %s --repo %s --branch %s --yes --vault <vault> --json", shellQuote(profile.Name), outArg, shellQuote(repo), shellQuote(branch))
	default:
		return ""
	}
}

func readPublishProfileRequest(req PublishRequest, command string) (domain.PublishProfile, []domain.PublishValidationIssue, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return domain.PublishProfile{}, nil, err
	}
	name, commandErr := normalizePublishProfileName(req.Profile)
	if commandErr != nil {
		return domain.PublishProfile{}, nil, commandErr
	}
	profile, err := readPublishProfile(root, name)
	if err != nil {
		return domain.PublishProfile{}, nil, err
	}
	issues := publishops.ValidateProfile(profile)
	return profile, issues, nil
}

func normalizePublishProfileName(raw string) (string, *domain.CommandError) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", &domain.CommandError{Code: "publish_profile_required", Message: "publish profile name is required", Hint: "pinax publish profile init public --vault <vault> --json"}
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", &domain.CommandError{Code: "publish_profile_name_invalid", Message: "publish profile name must be one safe name", Hint: "Use a name like public or team-wiki"}
	}
	return name, nil
}

func publishProfileRelPath(name string) string {
	return filepath.ToSlash(filepath.Join(".pinax", "publish", "profiles", name+".yaml"))
}

func publishProfileAbsPath(root, name string) string {
	return filepath.Join(root, ".pinax", "publish", "profiles", name+".yaml")
}

func writePublishProfile(root string, profile domain.PublishProfile) error {
	path := publishProfileAbsPath(root, profile.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+profile.Name+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readPublishProfile(root, name string) (domain.PublishProfile, error) {
	body, err := os.ReadFile(publishProfileAbsPath(root, name))
	if errors.Is(err, os.ErrNotExist) {
		return domain.PublishProfile{}, &domain.CommandError{Code: "publish_profile_not_found", Message: "Publish profile not found", Hint: "Run pinax publish profile init " + shellQuote(name) + " --vault <vault> --json"}
	}
	if err != nil {
		return domain.PublishProfile{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	var profile domain.PublishProfile
	if err := decoder.Decode(&profile); err != nil {
		if strings.Contains(err.Error(), "field ") && strings.Contains(err.Error(), " not found in type ") {
			return domain.PublishProfile{}, &domain.CommandError{Code: "publish_profile_unknown_field", Message: "Publish profile contains an unknown field", Hint: "Recreate or repair the profile with pinax publish profile init"}
		}
		return domain.PublishProfile{}, &domain.CommandError{Code: "publish_profile_invalid", Message: err.Error(), Hint: "Recreate or repair the profile with pinax publish profile init"}
	}
	return profile, nil
}

func listPublishProfiles(root string) ([]domain.PublishProfile, error) {
	dir := filepath.Join(root, ".pinax", "publish", "profiles")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]domain.PublishProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		profile, err := readPublishProfile(root, name)
		if err != nil {
			var pathErr *fs.PathError
			if errors.As(err, &pathErr) {
				return nil, err
			}
			continue
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func publishValidationError(command, profile string, issues []domain.PublishValidationIssue) *domain.CommandError {
	return &domain.CommandError{Code: "publish_profile_invalid", Message: fmt.Sprintf("Publish profile %s has %d validation issue(s)", profile, len(issues)), Hint: "Fix the profile or recreate it with pinax publish profile init"}
}

func publishProfileProjection(command string, profile domain.PublishProfile, issues []domain.PublishValidationIssue, status string) domain.Projection {
	projection := domain.NewProjection(command, "Publish profile processed.")
	migrationPlan := publishProfileMigrationPlan(profile)
	projection.Status = status
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["issues"] = fmt.Sprint(len(issues))
	projection.Facts["migration.recommended"] = fmt.Sprint(migrationPlan.Recommended)
	projection.Evidence = []string{publishProfileRelPath(profile.Name)}
	projection.Data = map[string]any{"profile": profile, "issues": issues, "migration_plan": migrationPlan}
	if migrationPlan.Recommended {
		projection.Actions = append(projection.Actions, domain.Action{Name: "migrate_renderer", Command: migrationPlan.Command})
	}
	if len(issues) > 0 {
		for i, issue := range issues {
			projection.Facts[fmt.Sprintf("issue.%d.code", i+1)] = issue.Code
		}
		projection.Error = publishValidationError(command, profile.Name, issues)
	}
	return projection
}

func publishProfileMigrationPlan(profile domain.PublishProfile) domain.PublishProfileMigrationPlan {
	if profile.Target != domain.PublishTargetGitHubPages || profile.Renderer == domain.PublishRendererPinaxWeb {
		return domain.PublishProfileMigrationPlan{Recommended: false}
	}
	if profile.Renderer != domain.PublishRendererHugo {
		return domain.PublishProfileMigrationPlan{Recommended: false}
	}
	return domain.PublishProfileMigrationPlan{
		Recommended:  true,
		FromRenderer: profile.Renderer,
		ToRenderer:   domain.PublishRendererPinaxWeb,
		Reason:       "pinax-web is the canonical renderer for GitHub Pages publish profiles.",
		Command:      fmt.Sprintf("pinax publish profile init %s --target %s --renderer %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)), shellQuote(string(domain.PublishRendererPinaxWeb))),
	}
}

func (s *Service) PublishPlan(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.plan")
	if err != nil {
		return errorProjection("publish.plan", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.plan", profile.Name, issues)
		return publishProfileProjection("publish.plan", profile, issues, "failed"), cmdErr
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	emitPublishEvent(req.LiveEvents, "profile_ready", "success", publishProfileEventFacts(profile))
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.plan", err), err
	}
	facts, err := scanNoteFacts(root)
	if err != nil {
		return errorProjection("publish.plan", err), err
	}
	facts = ordinaryNoteFacts(facts)
	plan := domain.PublishPlan{ProfileName: profile.Name, Target: profile.Target, Renderer: profile.Renderer}
	selectedAssets := map[string]bool{}
	violatedAssets := map[string]bool{}
	linkGraphNotes := make([]domain.Note, 0, len(facts))
	for _, fact := range facts {
		eligibility := publishops.ClassifyNoteEligibility(profile, fact.note)
		if !eligibility.Selected {
			plan.Skipped = append(plan.Skipped, domain.PublishItem{ID: fact.note.ID, Kind: "note", Title: fact.note.Title, SourcePath: fact.note.Path, Reason: eligibility.Reason})
			continue
		}
		violations := publishops.ClassifyNoteViolations(fact.note)
		if len(violations) > 0 {
			plan.Violations = append(plan.Violations, violations...)
			continue
		}
		plan.Selected = append(plan.Selected, domain.PublishItem{ID: fact.note.ID, Kind: "note", Title: fact.note.Title, SourcePath: fact.note.Path, OutputPath: publishNoteOutputPath(profile.Target, fact.note)})
		plan.Sources = append(plan.Sources, publishSourceFromNote(fact.note))
		linkGraphNotes = append(linkGraphNotes, fact.note)
		if profile.Assets.IncludeLinkedAssets {
			for _, link := range pinaxassets.ExtractLinks(pinaxassets.LinkExtractionRequest{SourceNoteID: fact.note.ID, SourcePath: fact.note.Path, Body: fact.note.Body}) {
				item, violation := classifyPublishAsset(root, profile, link)
				if violation != nil {
					if !violatedAssets[violation.Path] {
						plan.Violations = append(plan.Violations, *violation)
						violatedAssets[violation.Path] = true
					}
					continue
				}
				if !selectedAssets[item.SourcePath] {
					plan.Selected = append(plan.Selected, item)
					selectedAssets[item.SourcePath] = true
				}
			}
		}
	}
	plan.LinkGraph = publishPlanLinkGraph(linkGraphNotes)
	projection := domain.NewProjection("publish.plan", "发布计划已生成。")
	if len(plan.Violations) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["source_count"] = fmt.Sprint(len(plan.Sources))
	projection.Facts["link_count"] = fmt.Sprint(len(plan.LinkGraph))
	projection.Facts["broken_link_count"] = fmt.Sprint(publishBrokenLinkCount(plan.LinkGraph))
	projection.Facts["selected_count"] = fmt.Sprint(publishItemCount(plan.Selected, "note"))
	projection.Facts["selected_asset_count"] = fmt.Sprint(publishItemCount(plan.Selected, "asset"))
	projection.Facts["skipped_count"] = fmt.Sprint(len(plan.Skipped))
	projection.Facts["asset_violation_count"] = fmt.Sprint(publishViolationClassCount(plan.Violations, domain.PublishViolationAssetNotAllowed))
	projection.Facts["blocking_count"] = fmt.Sprint(publishBlockingItemCount(plan.Violations))
	projection.Facts["manual_review_count"] = fmt.Sprint(len(plan.ManualReview))
	projection.Data = map[string]any{"plan": plan}
	emitPublishEvent(req.LiveEvents, "plan_checked", projection.Status, map[string]string{"profile": profile.Name, "target": string(profile.Target), "renderer": string(profile.Renderer), "selected_count": projection.Facts["selected_count"], "skipped_count": projection.Facts["skipped_count"], "blocking_count": projection.Facts["blocking_count"], "manual_review_count": projection.Facts["manual_review_count"]})
	if len(plan.Violations) == 0 {
		projection.Actions = []domain.Action{{Name: "build", Command: fmt.Sprintf("pinax publish build --profile %s --target %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	} else {
		projection.Actions = []domain.Action{{Name: "review", Command: fmt.Sprintf("pinax publish plan --profile %s --target %s --vault <vault> --json", shellQuote(profile.Name), shellQuote(string(profile.Target)))}}
	}
	return projection, nil
}

func (s *Service) PublishBuild(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.build")
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.build", profile.Name, issues)
		return publishProfileProjection("publish.build", profile, issues, "failed"), cmdErr
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	if profile.Target != domain.PublishTargetLocal && profile.Target != domain.PublishTargetGitHubWiki && profile.Target != domain.PublishTargetGitHubGist && profile.Target != domain.PublishTargetHTTP && (profile.Target != domain.PublishTargetGitHubPages || profile.Renderer != domain.PublishRendererHugo) {
		return publishPlaceholder("publish.build", req)
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.build", cmdErr), cmdErr
	}
	planProjection, err := s.PublishPlan(ctx, PublishRequest{VaultPath: root, Profile: profile.Name, Target: string(profile.Target), LiveEvents: req.LiveEvents})
	if err != nil {
		return planProjection, err
	}
	plan, err := publishPlanFromProjection(planProjection)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(plan.Violations) > 0 {
		err := &domain.CommandError{Code: "publish_plan_blocked", Message: "Publish build is blocked by plan violations", Hint: "Run pinax publish plan --profile " + shellQuote(profile.Name) + " --target " + shellQuote(string(profile.Target)) + " --vault <vault> --json"}
		projection := domain.NewErrorProjection("publish.build", err)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Data = map[string]any{"plan": plan}
		return projection, err
	}
	notes, err := publishNotesByPath(root)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	started := time.Now().UTC()
	if profile.Target == domain.PublishTargetGitHubPages && profile.Renderer == domain.PublishRendererHugo {
		return s.publishBuildHugoPages(ctx, root, outDir, profile, plan, notes, started)
	}
	emitPublishEvent(req.LiveEvents, "renderer_started", "running", map[string]string{"profile": profile.Name, "target": string(profile.Target), "renderer": string(profile.Renderer), "out": publishSafeOutLabel(outDir)})
	switch profile.Target {
	case domain.PublishTargetLocal:
		err = writePinaxWebStaticOutput(ctx, root, outDir, profile, plan, notes)
	case domain.PublishTargetGitHubGist, domain.PublishTargetHTTP:
		err = writeBundlePublishOutput(root, outDir, profile, plan, notes)
	default:
		err = writeWikiPublishOutput(root, outDir, profile, plan, notes)
	}
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	emitPublishEvent(req.LiveEvents, "renderer_completed", "success", map[string]string{"profile": profile.Name, "target": string(profile.Target), "renderer": string(profile.Renderer), "out": publishSafeOutLabel(outDir)})
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(scan.Findings) > 0 {
		emitPublishEvent(req.LiveEvents, "scan_completed", "failed", map[string]string{"profile": profile.Name, "target": string(profile.Target), "scan_findings": fmt.Sprint(len(scan.Findings))})
		err := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Review the redacted scan findings and rebuild"}
		projection := domain.NewErrorProjection("publish.build", err)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, err
	}
	emitPublishEvent(req.LiveEvents, "scan_completed", "success", map[string]string{"profile": profile.Name, "target": string(profile.Target), "scan_findings": fmt.Sprint(len(scan.Findings))})
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	finished := time.Now().UTC()
	receiptRel, err := publishops.WritePublishReceipt(root, domain.PublishReceipt{RunID: publishRunID(finished), ProfileName: profile.Name, Target: profile.Target, Renderer: profile.Renderer, StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), DurationMS: finished.Sub(started).Milliseconds(), Counts: map[string]int{"selected": publishItemCount(plan.Selected, "note"), "assets": publishItemCount(plan.Selected, "asset"), "violations": len(plan.Violations)}, OutputHash: outputHash, RedactionSummary: map[string]string{"scan_findings": fmt.Sprint(len(scan.Findings))}, DeployStatus: "not_deployed"})
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	emitPublishEvent(req.LiveEvents, "receipt_written", "success", map[string]string{"profile": profile.Name, "target": string(profile.Target), "renderer": string(profile.Renderer), "receipt": receiptRel, "selected_count": fmt.Sprint(publishItemCount(plan.Selected, "note")), "scan_findings": fmt.Sprint(len(scan.Findings))})
	summary := "Wiki 发布产物已生成。"
	if profile.Target == domain.PublishTargetGitHubGist || profile.Target == domain.PublishTargetHTTP {
		summary = "Markdown 分享产物已生成。"
	}
	projection := domain.NewProjection("publish.build", summary)
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["selected_count"] = fmt.Sprint(publishItemCount(plan.Selected, "note"))
	projection.Facts["asset_count"] = fmt.Sprint(publishItemCount(plan.Selected, "asset"))
	projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
	projection.Facts["output_hash"] = outputHash
	projection.Facts["manifest_path"] = "pinax-publish-manifest.json"
	if profile.Target == domain.PublishTargetLocal {
		projection.Facts["manifest_path"] = "pinax-data/manifest.json"
	}
	projection.Facts["receipt_path"] = receiptRel
	projection.Facts["receipt"] = receiptRel
	projection.Evidence = []string{projection.Facts["manifest_path"], receiptRel}
	projection.Data = map[string]any{"manifest_path": projection.Facts["manifest_path"], "receipt_path": receiptRel, "scan": scan, "output_hash": outputHash}
	return projection, nil
}

func (s *Service) publishBuildHugoPages(ctx context.Context, root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note, started time.Time) (domain.Projection, error) {
	runID := publishRunID(started)
	stageRel := filepath.ToSlash(filepath.Join(".pinax", "publish", "staging", runID))
	stageDir := filepath.Join(root, filepath.FromSlash(stageRel))
	staging, err := publishops.BuildHugoStagingProject(publishops.HugoStagingRequest{VaultRoot: root, StageRoot: stageDir, Profile: profile, Plan: plan, Notes: notes})
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	stageScan, err := publishops.ScanPublishTree(stageDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(stageScan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish staging output contains blocked content", Hint: "Review the redacted scan findings and rebuild"}
		projection := domain.NewErrorProjection("publish.build", cmdErr)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["scan_findings"] = fmt.Sprint(len(stageScan.Findings))
		projection.Data = map[string]any{"scan": stageScan}
		return projection, cmdErr
	}
	hugoResult, err := publishops.HugoAdapter{Timeout: 2 * time.Minute}.Build(ctx, stageDir, outDir)
	if err != nil {
		projection := errorProjection("publish.build", err)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["renderer"] = string(profile.Renderer)
		projection.Data = map[string]any{"hugo": hugoResult}
		return projection, err
	}
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	if len(scan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Review the redacted scan findings and rebuild"}
		projection := domain.NewErrorProjection("publish.build", cmdErr)
		projection.Facts["profile"] = profile.Name
		projection.Facts["target"] = string(profile.Target)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	}
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	finished := time.Now().UTC()
	receiptRel, err := publishops.WritePublishReceipt(root, domain.PublishReceipt{RunID: runID, ProfileName: profile.Name, Target: profile.Target, Renderer: profile.Renderer, StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), DurationMS: finished.Sub(started).Milliseconds(), Counts: map[string]int{"selected": publishItemCount(plan.Selected, "note"), "assets": publishItemCount(plan.Selected, "asset"), "violations": len(plan.Violations)}, OutputHash: outputHash, RedactionSummary: map[string]string{"scan_findings": fmt.Sprint(len(scan.Findings)), "staging_scan_findings": fmt.Sprint(len(stageScan.Findings))}, DeployStatus: "not_deployed"})
	if err != nil {
		return errorProjection("publish.build", err), err
	}
	projection := domain.NewProjection("publish.build", "Pages 发布产物已生成。")
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(profile.Target)
	projection.Facts["renderer"] = string(profile.Renderer)
	projection.Facts["selected_count"] = fmt.Sprint(publishItemCount(plan.Selected, "note"))
	projection.Facts["asset_count"] = fmt.Sprint(publishItemCount(plan.Selected, "asset"))
	projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
	projection.Facts["staging_scan_findings"] = fmt.Sprint(len(stageScan.Findings))
	projection.Facts["output_hash"] = outputHash
	projection.Facts["theme"] = staging.Theme
	projection.Facts["staging_files"] = fmt.Sprint(staging.FilesWritten)
	projection.Facts["manifest_path"] = filepath.ToSlash(filepath.Join(stageRel, "data", "pinax", "manifest.json"))
	projection.Facts["receipt_path"] = receiptRel
	projection.Facts["receipt"] = receiptRel
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(stageRel, "data", "pinax", "manifest.json")), receiptRel}
	projection.Data = map[string]any{"manifest_path": filepath.ToSlash(filepath.Join(stageRel, "data", "pinax", "manifest.json")), "receipt_path": receiptRel, "scan": scan, "staging_scan": stageScan, "hugo": hugoResult}
	return projection, nil
}

func (s *Service) PublishThemeList(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	themes := publishops.BuiltinThemeInfos()
	projection := domain.NewProjection("publish.theme.list", "发布主题已列出。")
	projection.Facts["themes"] = fmt.Sprint(len(themes))
	for i, theme := range themes {
		prefix := fmt.Sprintf("theme.%d", i+1)
		projection.Facts[prefix+".name"] = theme.Name
		projection.Facts[prefix+".source"] = theme.Source
		projection.Facts[prefix+".contract"] = theme.ContractVersion
		projection.Facts[prefix+".required_layouts"] = fmt.Sprint(len(theme.RequiredLayouts))
	}
	projection.Data = map[string]any{"themes": themes}
	return projection, nil
}

func (s *Service) PublishThemeEject(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.theme.eject", cmdErr), cmdErr
	}
	files, err := publishops.WriteBuiltinTheme(strings.TrimSpace(req.Theme), outDir)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_theme_unavailable", Message: err.Error(), Hint: "Run pinax publish theme list --json to inspect available built-in themes"}
		return domain.NewErrorProjection("publish.theme.eject", cmdErr), cmdErr
	}
	infos := publishops.BuiltinThemeInfos()
	info := infos[0]
	projection := domain.NewProjection("publish.theme.eject", "发布主题已导出。")
	projection.Facts["theme"] = info.Name
	projection.Facts["source"] = info.Source
	projection.Facts["contract"] = info.ContractVersion
	projection.Facts["files"] = fmt.Sprint(len(files))
	projection.Evidence = files
	projection.Data = map[string]any{"theme": info, "files": files}
	return projection, nil
}

func (s *Service) PublishDeploy(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.deploy")
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	if strings.TrimSpace(req.Target) != "" {
		profile.Target = domain.PublishTarget(strings.TrimSpace(req.Target))
	}
	if strings.TrimSpace(req.Repo) != "" {
		profile.Deploy.Mode = domain.PublishDeployModeGit
		profile.Deploy.Repo = strings.TrimSpace(req.Repo)
	}
	if strings.TrimSpace(req.Branch) != "" {
		profile.Deploy.Branch = strings.TrimSpace(req.Branch)
	}
	if strings.TrimSpace(req.Endpoint) != "" {
		profile.Deploy.Mode = domain.PublishDeployModeHTTP
		profile.Deploy.Endpoint = strings.TrimSpace(req.Endpoint)
	}
	if strings.TrimSpace(req.Method) != "" {
		profile.Deploy.Method = strings.TrimSpace(req.Method)
	}
	if strings.TrimSpace(req.SecretRef) != "" {
		profile.Deploy.SecretRef = strings.TrimSpace(req.SecretRef)
	}
	if strings.TrimSpace(req.GistID) != "" {
		profile.Deploy.Mode = domain.PublishDeployModeGist
		profile.Deploy.GistID = strings.TrimSpace(req.GistID)
	}
	if strings.TrimSpace(req.Visibility) != "" {
		profile.Deploy.Visibility = strings.TrimSpace(req.Visibility)
	}
	if strings.TrimSpace(req.Project) != "" {
		profile.Deploy.Project = strings.TrimSpace(req.Project)
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.deploy", profile.Name, issues)
		return publishProfileProjection("publish.deploy", profile, issues, "failed"), cmdErr
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	policy, err := publishops.ParseDeployPolicy(publishops.DeployPolicyRequest{VaultRoot: root, Profile: profile})
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_deploy_policy_invalid", Message: err.Error(), Hint: "Check deploy mode, repo, branch and strategy"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	if policy.Mode == domain.PublishDeployModeNone {
		projection := domain.NewProjection("publish.deploy", "发布部署已跳过。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		return projection, nil
	}
	if !req.Yes {
		cmdErr := &domain.CommandError{Code: "approval_required", Message: "Publish deploy requires --yes before writing to the delivery target", Hint: "Review the publish output and rerun with --yes"}
		projection := domain.NewErrorProjection("publish.deploy", cmdErr)
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["branch"] = policy.Branch
		return projection, cmdErr
	}
	if projection, err := validatePublishDeployInput(root, outDir, profile); err != nil {
		return projection, err
	}
	if policy.Mode == domain.PublishDeployModeGist {
		result, err := publishDeployGist(ctx, root, outDir, policy)
		if err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Check gh authentication and retry"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["files"] = fmt.Sprint(result.Files)
		projection.Facts["visibility"] = policy.Visibility
		if result.URL != "" {
			projection.Facts["url"] = result.URL
		}
		projection.Data = map[string]any{"files": result.Files, "url": result.URL}
		return projection, nil
	}
	if policy.Mode == domain.PublishDeployModeHTTP {
		result, err := publishDeployHTTP(ctx, root, outDir, policy)
		if err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Check the HTTP endpoint and retry"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["http_status"] = fmt.Sprint(result.StatusCode)
		projection.Facts["files"] = fmt.Sprint(result.Files)
		if result.URL != "" {
			projection.Facts["url"] = result.URL
		}
		projection.Data = map[string]any{"files": result.Files, "http_status": result.StatusCode, "url": result.URL}
		return projection, nil
	}
	if policy.Mode == domain.PublishDeployModeVercel {
		if _, err := exec.LookPath("vercel"); err != nil {
			cmdErr := &domain.CommandError{Code: "publish_vercel_cli_missing", Message: "vercel CLI was not found on PATH", Hint: "Install Vercel CLI, authenticate with vercel login, and retry"}
			projection := domain.NewErrorProjection("publish.deploy", cmdErr)
			projection.Facts["mode"] = string(policy.Mode)
			projection.Facts["target"] = string(policy.Target)
			projection.Facts["project"] = policy.Project
			return projection, cmdErr
		}
		result, err := publishDeployVercel(ctx, root, outDir, policy)
		if err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Check Vercel CLI authentication and retry"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["project"] = policy.Project
		projection.Facts["files"] = fmt.Sprint(result.Files)
		if result.URL != "" {
			projection.Facts["url"] = result.URL
		}
		projection.Data = map[string]any{"files": result.Files, "project": policy.Project, "url": result.URL}
		return projection, nil
	}
	if policy.Mode == domain.PublishDeployModeCloudflarePages {
		if _, err := exec.LookPath("wrangler"); err != nil {
			cmdErr := &domain.CommandError{Code: "publish_wrangler_cli_missing", Message: "wrangler CLI was not found on PATH", Hint: "Install Wrangler, authenticate with wrangler login, and retry"}
			projection := domain.NewErrorProjection("publish.deploy", cmdErr)
			projection.Facts["mode"] = string(policy.Mode)
			projection.Facts["target"] = string(policy.Target)
			projection.Facts["project"] = policy.Project
			return projection, cmdErr
		}
		result, err := publishDeployCloudflarePages(ctx, root, outDir, policy)
		if err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Check Wrangler authentication and retry"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
		projection.Facts["mode"] = string(policy.Mode)
		projection.Facts["target"] = string(policy.Target)
		projection.Facts["project"] = policy.Project
		projection.Facts["files"] = fmt.Sprint(result.Files)
		if result.URL != "" {
			projection.Facts["url"] = result.URL
		}
		projection.Data = map[string]any{"files": result.Files, "project": policy.Project, "url": result.URL}
		return projection, nil
	}
	if publishDeployRepoIsRemote(policy.Repo) {
		cmdErr := &domain.CommandError{Code: "publish_deploy_remote_unsupported", Message: "Remote publish deploy is not implemented yet", Hint: "Use a local deploy repository path for this step"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	result, err := publishDeployLocalGit(ctx, root, outDir, policy)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_deploy_failed", Message: publishRedactGitOutput(err.Error(), root), Hint: "Inspect the deploy repository and retry"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	projection := domain.NewProjection("publish.deploy", "发布产物已部署。")
	projection.Facts["mode"] = string(policy.Mode)
	projection.Facts["target"] = string(policy.Target)
	projection.Facts["branch"] = policy.Branch
	projection.Facts["strategy"] = policy.Strategy
	projection.Facts["files"] = fmt.Sprint(result.Files)
	projection.Facts["committed"] = fmt.Sprint(result.Committed)
	projection.Data = map[string]any{"files": result.Files, "committed": result.Committed}
	return projection, nil
}

func (s *Service) PublishPreviewApprove(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	profile, issues, err := readPublishProfileRequest(req, "publish.preview.approve")
	if err != nil {
		return errorProjection("publish.preview.approve", err), err
	}
	if len(issues) > 0 {
		cmdErr := publishValidationError("publish.preview.approve", profile.Name, issues)
		return publishProfileProjection("publish.preview.approve", profile, issues, "failed"), cmdErr
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.preview.approve", err), err
	}
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.preview.approve", cmdErr), cmdErr
	}
	if _, err := os.Stat(outDir); err != nil {
		cmdErr := &domain.CommandError{Code: "preview_build_required", Message: "Preview approval requires an existing publish output", Hint: "Run pinax publish build --profile " + shellQuote(profile.Name) + " --out <out> --vault <vault> --json"}
		projection := domain.NewErrorProjection("publish.preview.approve", cmdErr)
		projection.Facts["profile"] = profile.Name
		return projection, cmdErr
	}
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.preview.approve", err), err
	}
	if len(scan.Findings) > 0 {
		emitPublishEvent(req.LiveEvents, "scan_completed", "failed", map[string]string{"profile": profile.Name, "scan_findings": fmt.Sprint(len(scan.Findings))})
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Rebuild publish output before approval"}
		projection := domain.NewErrorProjection("publish.preview.approve", cmdErr)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	}
	emitPublishEvent(req.LiveEvents, "scan_completed", "success", map[string]string{"profile": profile.Name, "scan_findings": fmt.Sprint(len(scan.Findings))})
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.preview.approve", err), err
	}
	buildReceipt, err := latestPublishReceiptForOutput(root, profile.Name, outputHash)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "preview_build_required", Message: "Preview approval requires a matching publish build receipt", Hint: "Run pinax publish build --profile " + shellQuote(profile.Name) + " --out <out> --vault <vault> --json"}
		projection := domain.NewErrorProjection("publish.preview.approve", cmdErr)
		projection.Facts["profile"] = profile.Name
		projection.Facts["output_hash"] = outputHash
		return projection, cmdErr
	}
	now := time.Now().UTC()
	counts := map[string]int{
		"selected": publishReceiptCount(buildReceipt, "selected"),
		"skipped":  publishReceiptCount(buildReceipt, "skipped"),
		"blocking": publishReceiptCount(buildReceipt, "blocking"),
	}
	receiptRel, err := publishops.WritePublishReceipt(root, domain.PublishReceipt{RunID: publishRunID(now), ProfileName: profile.Name, Target: buildReceipt.Target, Renderer: buildReceipt.Renderer, StartedAt: now.Format(time.RFC3339), FinishedAt: now.Format(time.RFC3339), Counts: counts, OutputHash: outputHash, RedactionSummary: map[string]string{"scan_findings": fmt.Sprint(len(scan.Findings))}, DeployStatus: "preview_approved"})
	if err != nil {
		return errorProjection("publish.preview.approve", err), err
	}
	emitPublishEvent(req.LiveEvents, "preview_approved", "success", map[string]string{"profile": profile.Name, "target": string(buildReceipt.Target), "renderer": string(buildReceipt.Renderer), "receipt": receiptRel, "selected_count": fmt.Sprint(counts["selected"]), "scan_findings": fmt.Sprint(len(scan.Findings)), "output_hash": "present"})
	projection := domain.NewProjection("publish.preview.approve", "Publish preview approved.")
	projection.Facts["profile"] = profile.Name
	projection.Facts["target"] = string(buildReceipt.Target)
	projection.Facts["renderer"] = string(buildReceipt.Renderer)
	projection.Facts["approved"] = "true"
	projection.Facts["output_hash"] = outputHash
	projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
	projection.Facts["selected_count"] = fmt.Sprint(counts["selected"])
	projection.Facts["skipped_count"] = fmt.Sprint(counts["skipped"])
	projection.Facts["blocking_count"] = fmt.Sprint(counts["blocking"])
	projection.Facts["receipt"] = receiptRel
	projection.Evidence = []string{receiptRel}
	projection.Data = map[string]any{"receipt": receiptRel, "output_hash": outputHash, "scan": scan}
	return projection, nil
}

func (s *Service) PublishDev(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	if req.Watch {
		return s.publishDevWatch(ctx, req)
	}
	buildReq := req
	if strings.TrimSpace(buildReq.Target) == "" {
		buildReq.Target = string(domain.PublishTargetLocal)
	}
	buildProjection, err := s.PublishBuild(ctx, buildReq)
	if err != nil {
		projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_build_failed", Message: err.Error(), Hint: "Fix publish build errors and retry publish dev"})
		projection.Data = map[string]any{"build": buildProjection}
		return projection, err
	}
	serveProjection, err := s.PublishServe(ctx, req)
	if err != nil {
		projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_serve_failed", Message: err.Error(), Hint: "Check host and port, then retry publish dev"})
		projection.Data = map[string]any{"build": buildProjection, "serve": serveProjection}
		return projection, err
	}
	projection := domain.NewProjection("publish.dev", "发布开发预览已构建并启动。")
	projection.Facts["profile"] = strings.TrimSpace(req.Profile)
	projection.Facts["target"] = buildReq.Target
	projection.Facts["built"] = "true"
	projection.Facts["served"] = serveProjection.Facts["served"]
	projection.Facts["host"] = serveProjection.Facts["host"]
	projection.Facts["port"] = serveProjection.Facts["port"]
	projection.Facts["url"] = serveProjection.Facts["url"]
	projection.Data = map[string]any{"build": buildProjection, "serve": serveProjection}
	return projection, nil
}

func (s *Service) publishDevWatch(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("publish.dev", err), err
	}
	buildReq := req
	buildReq.VaultPath = root
	if strings.TrimSpace(buildReq.Target) == "" {
		buildReq.Target = string(domain.PublishTargetLocal)
	}
	buildProjection, err := s.PublishBuild(ctx, buildReq)
	if err != nil {
		projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_build_failed", Message: err.Error(), Hint: "Fix publish build errors and retry publish dev --watch"})
		projection.Data = map[string]any{"build": buildProjection}
		return projection, err
	}
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.dev", cmdErr), cmdErr
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if !publishServeHostAllowed(host) {
		cmdErr := &domain.CommandError{Code: "publish_serve_host_unsafe", Message: "publish dev host must be loopback", Hint: "Use --host 127.0.0.1"}
		return domain.NewErrorProjection("publish.dev", cmdErr), cmdErr
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(req.Port)))
	if err != nil {
		return errorProjection("publish.dev", err), err
	}
	server := &http.Server{Handler: http.FileServer(http.Dir(outDir))}
	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
		select {
		case <-serveErr:
		default:
		}
	}()
	addr := listener.Addr().(*net.TCPAddr)
	url := "http://" + net.JoinHostPort(host, fmt.Sprint(addr.Port)) + "/"
	emitPublishEvent(req.LiveEvents, "serve_ready", "success", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target, "host": host, "port": fmt.Sprint(addr.Port), "url": url})
	if err := publishDevSmoke(ctx, url); err != nil {
		projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_serve_failed", Message: err.Error(), Hint: "Check host and port, then retry publish dev --watch"})
		projection.Data = map[string]any{"build": buildProjection}
		return projection, err
	}
	emitPublishEvent(req.LiveEvents, "smoke_completed", "success", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target, "url": url})
	rendererDir, _ := pinaxWebRendererPackageDir()
	events, errorsCh, closeWatchers, err := publishDevWatchEvents(ctx, root, rendererDir)
	if err != nil {
		projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_watch_failed", Message: err.Error(), Hint: "Check vault and renderer directory permissions"})
		projection.Data = map[string]any{"build": buildProjection}
		return projection, err
	}
	defer closeWatchers()
	emitPublishEvent(req.LiveEvents, "watch_started", "running", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target})
	batches := publishDevDebounce(ctx, events, 250*time.Millisecond)
	rebuilds := 0
	failures := 0
	lastError := ""
	for {
		select {
		case batch, ok := <-batches:
			if !ok {
				return publishDevWatchProjection(req, buildReq.Target, host, addr.Port, url, rebuilds, failures, lastError), nil
			}
			if !publishDevWatchBatchAllowed(root, rendererDir, batch) {
				continue
			}
			emitPublishEvent(req.LiveEvents, "change_detected", "running", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target, "changes": fmt.Sprint(len(batch))})
			emitPublishEvent(req.LiveEvents, "rebuild_started", "running", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target})
			buildProjection, err = s.PublishBuild(ctx, buildReq)
			if err != nil {
				failures++
				lastError = err.Error()
				emitPublishEvent(req.LiveEvents, "rebuild_failed", "failed", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target, "rebuild_failures": fmt.Sprint(failures)})
				if req.Once {
					projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_rebuild_failed", Message: err.Error(), Hint: "The previous preview output is still being served; fix the changed file and retry"})
					projection.Facts["watched"] = "true"
					projection.Facts["rebuilds"] = fmt.Sprint(rebuilds)
					projection.Facts["rebuild_failures"] = fmt.Sprint(failures)
					projection.Data = map[string]any{"build": buildProjection}
					return projection, err
				}
				continue
			}
			rebuilds++
			lastError = ""
			emitPublishEvent(req.LiveEvents, "rebuild_completed", "success", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target, "rebuilds": fmt.Sprint(rebuilds)})
			if err := publishDevSmoke(ctx, url); err != nil {
				projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_serve_failed", Message: err.Error(), Hint: "The preview rebuilt but smoke failed; check the output directory"})
				projection.Facts["watched"] = "true"
				projection.Facts["rebuilds"] = fmt.Sprint(rebuilds)
				projection.Facts["rebuild_failures"] = fmt.Sprint(failures)
				projection.Data = map[string]any{"build": buildProjection}
				return projection, err
			}
			emitPublishEvent(req.LiveEvents, "smoke_completed", "success", map[string]string{"profile": strings.TrimSpace(req.Profile), "target": buildReq.Target, "url": url})
			if req.Once {
				return publishDevWatchProjection(req, buildReq.Target, host, addr.Port, url, rebuilds, failures, lastError), nil
			}
		case err := <-errorsCh:
			if err != nil {
				projection := domain.NewErrorProjection("publish.dev", &domain.CommandError{Code: "publish_dev_watch_failed", Message: err.Error(), Hint: "Restart publish dev --watch after checking the watched directories"})
				projection.Facts["watched"] = "true"
				projection.Facts["rebuilds"] = fmt.Sprint(rebuilds)
				projection.Facts["rebuild_failures"] = fmt.Sprint(failures)
				return projection, err
			}
		case err := <-serveErr:
			if err != nil {
				return errorProjection("publish.dev", err), err
			}
		case <-ctx.Done():
			return errorProjection("publish.dev", ctx.Err()), ctx.Err()
		}
	}
}

func publishDevWatchProjection(req PublishRequest, target, host string, port int, url string, rebuilds, failures int, lastError string) domain.Projection {
	projection := domain.NewProjection("publish.dev", "Publish dev preview is serving with watch mode.")
	projection.Facts["profile"] = strings.TrimSpace(req.Profile)
	projection.Facts["target"] = target
	projection.Facts["built"] = "true"
	projection.Facts["served"] = "true"
	projection.Facts["watched"] = "true"
	projection.Facts["host"] = host
	projection.Facts["port"] = fmt.Sprint(port)
	projection.Facts["url"] = url
	projection.Facts["rebuilds"] = fmt.Sprint(rebuilds)
	projection.Facts["rebuild_failures"] = fmt.Sprint(failures)
	if lastError != "" {
		projection.Facts["last_rebuild_error"] = lastError
	}
	projection.Data = map[string]any{"watch": map[string]any{"rebuilds": rebuilds, "rebuild_failures": failures}}
	return projection
}

func publishDevSmoke(ctx context.Context, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return fmt.Errorf("publish dev smoke returned status %d", resp.StatusCode)
	}
	return nil
}

func publishDevWatchEvents(ctx context.Context, root, rendererDir string) (<-chan syncdaemon.WatchEvent, <-chan error, func(), error) {
	watchRoots := []string{root, filepath.Join(root, ".pinax", "publish", "profiles")}
	if strings.TrimSpace(rendererDir) != "" {
		watchRoots = append(watchRoots, rendererDir)
	}
	events := make(chan syncdaemon.WatchEvent, 64)
	errorsCh := make(chan error, len(watchRoots))
	watchers := make([]syncdaemon.Watcher, 0, len(watchRoots))
	for _, watchRoot := range watchRoots {
		if _, err := os.Stat(watchRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, nil, err
		}
		watcher, err := syncdaemon.NewFSNotifyWatcher(watchRoot)
		if err != nil {
			for _, existing := range watchers {
				_ = existing.Close()
			}
			return nil, nil, nil, err
		}
		watchers = append(watchers, watcher)
		go func(w syncdaemon.Watcher) {
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-w.Events():
					if !ok {
						return
					}
					select {
					case events <- event:
					case <-ctx.Done():
						return
					}
				}
			}
		}(watcher)
		go func(w syncdaemon.Watcher) {
			for {
				select {
				case <-ctx.Done():
					return
				case err, ok := <-w.Errors():
					if !ok {
						return
					}
					select {
					case errorsCh <- err:
					case <-ctx.Done():
						return
					}
				}
			}
		}(watcher)
	}
	if len(watchers) == 0 {
		return nil, nil, nil, fmt.Errorf("no publish dev watch roots are available")
	}
	closeWatchers := func() {
		for _, watcher := range watchers {
			_ = watcher.Close()
		}
	}
	return events, errorsCh, closeWatchers, nil
}

func publishDevDebounce(ctx context.Context, in <-chan syncdaemon.WatchEvent, delay time.Duration) <-chan []syncdaemon.WatchEvent {
	if delay <= 0 {
		delay = 250 * time.Millisecond
	}
	out := make(chan []syncdaemon.WatchEvent, 1)
	go func() {
		defer close(out)
		var batch []syncdaemon.WatchEvent
		var timer *time.Timer
		var timerC <-chan time.Time
		flush := func() {
			if len(batch) == 0 {
				return
			}
			out <- publishDevCoalesceEvents(batch)
			batch = nil
		}
		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case event, ok := <-in:
				if !ok {
					flush()
					return
				}
				batch = append(batch, event)
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(delay)
				timerC = timer.C
			case <-timerC:
				flush()
				timerC = nil
			}
		}
	}()
	return out
}

func publishDevCoalesceEvents(events []syncdaemon.WatchEvent) []syncdaemon.WatchEvent {
	seen := map[string]bool{}
	out := make([]syncdaemon.WatchEvent, 0, len(events))
	for _, event := range events {
		path := strings.TrimSpace(event.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, syncdaemon.WatchEvent{Path: path})
	}
	return out
}

func publishDevWatchBatchAllowed(root, rendererDir string, batch []syncdaemon.WatchEvent) bool {
	for _, event := range batch {
		if publishDevWatchPathAllowed(root, rendererDir, event.Path) {
			return true
		}
	}
	return false
}

func publishDevWatchPathAllowed(root, rendererDir, path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if rel, ok := relWithin(root, absPath); ok {
		slash := filepath.ToSlash(rel)
		if strings.HasPrefix(slash, ".pinax/") {
			return (strings.HasPrefix(slash, ".pinax/publish/profiles/") || slash == ".pinax/publish/profiles") && publishDevWatchProfileExt(slash)
		}
		return strings.EqualFold(filepath.Ext(slash), ".md")
	}
	if strings.TrimSpace(rendererDir) == "" {
		return false
	}
	if rel, ok := relWithin(rendererDir, absPath); ok {
		slash := filepath.ToSlash(rel)
		if strings.HasPrefix(slash, "node_modules/") || strings.HasPrefix(slash, "dist/") {
			return false
		}
		switch strings.ToLower(filepath.Ext(slash)) {
		case ".ts", ".tsx", ".js", ".jsx", ".css", ".html", ".json":
			return true
		default:
			return false
		}
	}
	return false
}

func publishDevWatchProfileExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func relWithin(root, path string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absRoot, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", false
	}
	return rel, true
}

func (s *Service) PublishServe(ctx context.Context, req PublishRequest) (domain.Projection, error) {
	outDir, cmdErr := cleanPublishOutPath(req.Out)
	if cmdErr != nil {
		return domain.NewErrorProjection("publish.serve", cmdErr), cmdErr
	}
	if scan, err := publishops.ScanPublishTree(outDir); err != nil {
		return errorProjection("publish.serve", err), err
	} else if len(scan.Findings) > 0 {
		emitPublishEvent(req.LiveEvents, "scan_completed", "failed", map[string]string{"profile": strings.TrimSpace(req.Profile), "scan_findings": fmt.Sprint(len(scan.Findings))})
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Rebuild publish output before preview"}
		projection := domain.NewErrorProjection("publish.serve", cmdErr)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	} else {
		emitPublishEvent(req.LiveEvents, "scan_completed", "success", map[string]string{"profile": strings.TrimSpace(req.Profile), "scan_findings": fmt.Sprint(len(scan.Findings))})
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if !publishServeHostAllowed(host) {
		cmdErr := &domain.CommandError{Code: "publish_serve_host_unsafe", Message: "publish serve host must be loopback", Hint: "Use --host 127.0.0.1"}
		return domain.NewErrorProjection("publish.serve", cmdErr), cmdErr
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(req.Port)))
	if err != nil {
		return errorProjection("publish.serve", err), err
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{Handler: http.FileServer(http.Dir(outDir))}
	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	addr := listener.Addr().(*net.TCPAddr)
	url := "http://" + net.JoinHostPort(host, fmt.Sprint(addr.Port)) + "/"
	emitPublishEvent(req.LiveEvents, "serve_ready", "success", map[string]string{"profile": strings.TrimSpace(req.Profile), "host": host, "port": fmt.Sprint(addr.Port), "url": url})
	served := false
	if req.Once {
		resp, err := http.Get(url)
		if err != nil {
			_ = server.Shutdown(ctx)
			return errorProjection("publish.serve", err), err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		served = resp.StatusCode >= 200 && resp.StatusCode < 500
		emitPublishEvent(req.LiveEvents, "smoke_completed", map[bool]string{true: "success", false: "failed"}[served], map[string]string{"profile": strings.TrimSpace(req.Profile), "url": url, "http_status": fmt.Sprint(resp.StatusCode)})
		_ = server.Shutdown(ctx)
		if err := <-serveErr; err != nil {
			return errorProjection("publish.serve", err), err
		}
	} else {
		select {
		case err := <-serveErr:
			if err != nil {
				return errorProjection("publish.serve", err), err
			}
		case <-ctx.Done():
			_ = server.Shutdown(context.Background())
			return errorProjection("publish.serve", ctx.Err()), ctx.Err()
		}
	}
	projection := domain.NewProjection("publish.serve", "发布预览服务已启动。")
	projection.Facts["profile"] = strings.TrimSpace(req.Profile)
	projection.Facts["host"] = host
	projection.Facts["port"] = fmt.Sprint(addr.Port)
	projection.Facts["served"] = fmt.Sprint(served || !req.Once)
	projection.Facts["url"] = url
	return projection, nil
}

func publishSafeOutLabel(outDir string) string {
	cleaned := filepath.Clean(strings.TrimSpace(outDir))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return "<out>"
	}
	return filepath.ToSlash(filepath.Base(cleaned))
}

func publishServeHostAllowed(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validatePublishDeployInput(root, outDir string, profile domain.PublishProfile) (domain.Projection, error) {
	scan, err := publishops.ScanPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	if len(scan.Findings) > 0 {
		cmdErr := &domain.CommandError{Code: "publish_leak_detected", Message: "Publish output contains blocked content", Hint: "Rebuild publish output before deploy"}
		projection := domain.NewErrorProjection("publish.deploy", cmdErr)
		projection.Facts["scan_findings"] = fmt.Sprint(len(scan.Findings))
		projection.Data = map[string]any{"scan": scan}
		return projection, cmdErr
	}
	outputHash, err := hashPublishTree(outDir)
	if err != nil {
		return errorProjection("publish.deploy", err), err
	}
	if publishDeployRequiresPreview(profile.Target) {
		if _, err := latestPublishReceiptForOutput(root, profile.Name, outputHash); err != nil {
			cmdErr := &domain.CommandError{Code: "publish_deploy_validation_failed", Message: err.Error(), Hint: "Run pinax publish build for this profile and output before deploy"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		if _, err := latestPreviewApprovalReceipt(root, profile.Name, outputHash); err != nil {
			cmdErr := &domain.CommandError{Code: "preview_required", Message: "Publish deploy requires a matching preview approval receipt", Hint: "Run pinax publish preview approve --profile " + shellQuote(profile.Name) + " --out <out> --vault <vault> --json"}
			return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
		}
		return domain.Projection{}, nil
	}
	receipt, err := latestPublishReceipt(root, profile)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "publish_deploy_validation_failed", Message: err.Error(), Hint: "Run pinax publish build for this profile and output before deploy"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	if receipt.OutputHash != outputHash {
		cmdErr := &domain.CommandError{Code: "publish_deploy_validation_failed", Message: "Publish output hash does not match the latest receipt", Hint: "Rebuild publish output before deploy"}
		return domain.NewErrorProjection("publish.deploy", cmdErr), cmdErr
	}
	return domain.Projection{}, nil
}

func publishDeployRequiresPreview(target domain.PublishTarget) bool {
	return target == domain.PublishTargetGitHubPages || target == domain.PublishTargetVercel || target == domain.PublishTargetCloudflare
}

func latestPublishReceipt(root string, profile domain.PublishProfile) (domain.PublishReceipt, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "runs", "*", "receipt.json"))
	if err != nil {
		return domain.PublishReceipt{}, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return domain.PublishReceipt{}, err
		}
		var receipt domain.PublishReceipt
		if err := json.Unmarshal(body, &receipt); err != nil {
			return domain.PublishReceipt{}, err
		}
		if receipt.ProfileName == profile.Name && receipt.Target == profile.Target {
			return receipt, nil
		}
	}
	return domain.PublishReceipt{}, fmt.Errorf("matching publish receipt was not found")
}

func latestPublishReceiptForOutput(root, profileName, outputHash string) (domain.PublishReceipt, error) {
	return latestPublishReceiptMatching(root, func(receipt domain.PublishReceipt) bool {
		return receipt.ProfileName == profileName && receipt.OutputHash == outputHash && receipt.DeployStatus != "preview_approved"
	})
}

func latestPreviewApprovalReceipt(root, profileName, outputHash string) (domain.PublishReceipt, error) {
	return latestPublishReceiptMatching(root, func(receipt domain.PublishReceipt) bool {
		return receipt.ProfileName == profileName && receipt.OutputHash == outputHash && receipt.DeployStatus == "preview_approved"
	})
}

func latestPublishReceiptMatching(root string, match func(domain.PublishReceipt) bool) (domain.PublishReceipt, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "runs", "*", "receipt.json"))
	if err != nil {
		return domain.PublishReceipt{}, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return domain.PublishReceipt{}, err
		}
		var receipt domain.PublishReceipt
		if err := json.Unmarshal(body, &receipt); err != nil {
			return domain.PublishReceipt{}, err
		}
		if match(receipt) {
			return receipt, nil
		}
	}
	return domain.PublishReceipt{}, fmt.Errorf("matching publish receipt was not found")
}

func publishReceiptCount(receipt domain.PublishReceipt, key string) int {
	if receipt.Counts == nil {
		return 0
	}
	return receipt.Counts[key]
}

type publishDeployResult struct {
	Files      int
	Committed  bool
	StatusCode int
	URL        string
}

func publishDeployGist(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	contentPath := filepath.Join(outDir, "pinax-gist.md")
	if _, err := os.Stat(contentPath); err != nil {
		return publishDeployResult{}, err
	}
	args := []string{"gist"}
	if policy.GistID == "" {
		args = append(args, "create", contentPath, "--desc", "Pinax publish")
		if policy.Visibility == "public" {
			args = append(args, "--public")
		}
	} else {
		args = append(args, "edit", policy.GistID, contentPath)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return publishDeployResult{}, fmt.Errorf("gh gist deploy failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return publishDeployResult{Files: 1, URL: publishRedactGitOutput(strings.TrimSpace(stdout.String()), vaultRoot)}, nil
}

func publishDeployHTTP(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	endpoint, err := url.Parse(policy.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return publishDeployResult{}, fmt.Errorf("publish http endpoint is invalid")
	}
	manifest, err := os.ReadFile(filepath.Join(outDir, "pinax-publish-manifest.json"))
	if err != nil {
		return publishDeployResult{}, err
	}
	content, err := os.ReadFile(filepath.Join(outDir, "pinax-gist.md"))
	if err != nil {
		return publishDeployResult{}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("manifest", string(manifest)); err != nil {
		return publishDeployResult{}, err
	}
	if err := writer.WriteField("content", string(content)); err != nil {
		return publishDeployResult{}, err
	}
	if err := writer.Close(); err != nil {
		return publishDeployResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, policy.Method, endpoint.String(), &body)
	if err != nil {
		return publishDeployResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token, ok := publishTokenFromSecretRef(policy.SecretRef); ok {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return publishDeployResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return publishDeployResult{}, fmt.Errorf("publish http endpoint returned status %d", resp.StatusCode)
	}
	return publishDeployResult{Files: 2, StatusCode: resp.StatusCode, URL: publishHTTPResponseURL(respBody, vaultRoot)}, nil
}

func publishDeployVercel(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	cmd := exec.CommandContext(ctx, "vercel", "deploy", outDir, "--yes", "--prod", "--name", policy.Project)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return publishDeployResult{}, fmt.Errorf("vercel deploy failed: %w: %s", err, publishRedactGitOutput(strings.TrimSpace(stderr.String()), vaultRoot))
	}
	files, err := countPublishTreeFiles(outDir)
	if err != nil {
		return publishDeployResult{}, err
	}
	return publishDeployResult{Files: files, URL: publishRedactGitOutput(strings.TrimSpace(stdout.String()), vaultRoot)}, nil
}

func publishDeployCloudflarePages(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	cmd := exec.CommandContext(ctx, "wrangler", "pages", "deploy", outDir, "--project-name", policy.Project)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return publishDeployResult{}, fmt.Errorf("wrangler pages deploy failed: %w: %s", err, publishRedactGitOutput(strings.TrimSpace(stderr.String()), vaultRoot))
	}
	files, err := countPublishTreeFiles(outDir)
	if err != nil {
		return publishDeployResult{}, err
	}
	return publishDeployResult{Files: files, URL: publishRedactGitOutput(strings.TrimSpace(stdout.String()), vaultRoot)}, nil
}

func publishTokenFromSecretRef(secretRef string) (string, bool) {
	secretRef = strings.TrimSpace(secretRef)
	if !strings.HasPrefix(secretRef, "env:") {
		return "", false
	}
	token := os.Getenv(strings.TrimPrefix(secretRef, "env:"))
	return token, token != ""
}

func publishHTTPResponseURL(body []byte, root string) string {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return publishRedactGitOutput(strings.TrimSpace(payload.URL), root)
}

func publishDeployLocalGit(ctx context.Context, vaultRoot, outDir string, policy publishops.DeployPolicy) (publishDeployResult, error) {
	repoDir := policy.Repo
	if !filepath.IsAbs(repoDir) {
		repoDir = filepath.Join(vaultRoot, filepath.FromSlash(repoDir))
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return publishDeployResult{}, err
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return publishDeployResult{}, err
		}
		if err := runPublishGit(ctx, repoDir, "init"); err != nil {
			return publishDeployResult{}, err
		}
	}
	if err := runPublishGit(ctx, repoDir, "checkout", "-B", policy.Branch); err != nil {
		return publishDeployResult{}, err
	}
	if err := runPublishGit(ctx, repoDir, "config", "user.email", "pinax@example.local"); err != nil {
		return publishDeployResult{}, err
	}
	if err := runPublishGit(ctx, repoDir, "config", "user.name", "Pinax"); err != nil {
		return publishDeployResult{}, err
	}
	if err := cleanPublishDeployWorktree(repoDir); err != nil {
		return publishDeployResult{}, err
	}
	files, err := copyPublishTree(outDir, repoDir)
	if err != nil {
		return publishDeployResult{}, err
	}
	if err := runPublishGit(ctx, repoDir, "add", "-A"); err != nil {
		return publishDeployResult{}, err
	}
	status, err := publishGitOutput(ctx, repoDir, "status", "--porcelain")
	if err != nil {
		return publishDeployResult{}, err
	}
	committed := strings.TrimSpace(status) != ""
	if committed {
		if err := runPublishGit(ctx, repoDir, "commit", "-m", "pinax publish deploy"); err != nil {
			return publishDeployResult{}, err
		}
	}
	return publishDeployResult{Files: files, Committed: committed}, nil
}

func cleanPublishDeployWorktree(repoDir string) error {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(repoDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyPublishTree(srcRoot, dstRoot string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := writePublishFile(filepath.Join(dstRoot, rel), body); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func countPublishTreeFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		count++
		return nil
	})
	return count, err
}

func runPublishGit(ctx context.Context, dir string, args ...string) error {
	_, err := publishGitOutput(ctx, dir, args...)
	return err
}

func publishGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func publishDeployRepoIsRemote(repo string) bool {
	repo = strings.TrimSpace(repo)
	return strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")
}

var publishCredentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
var publishAuthorizationPattern = regexp.MustCompile(`(?i)authorization:\s*bearer\s+[^\s]+`)
var publishCookiePattern = regexp.MustCompile(`(?i)cookie:\s*[^\r\n]+`)
var publishTokenPattern = regexp.MustCompile(`(?i)token=[^\s]+`)
var publishAbsolutePathPattern = regexp.MustCompile(`(?i)(/home|/users)/[^\s'\"]+|[a-z]:\\[^\s'\"]+`)

func publishRedactGitOutput(value, root string) string {
	value = publishCredentialURLPattern.ReplaceAllString(value, "${1}[REDACTED_URL]@")
	value = publishAuthorizationPattern.ReplaceAllString(value, "[REDACTED]")
	value = publishCookiePattern.ReplaceAllString(value, "[REDACTED]")
	value = publishTokenPattern.ReplaceAllString(value, "token=[REDACTED]")
	if strings.TrimSpace(root) != "" {
		value = strings.ReplaceAll(value, root, "[REDACTED_PATH]")
	}
	value = publishAbsolutePathPattern.ReplaceAllString(value, "[REDACTED_PATH]")
	return value
}

func cleanPublishOutPath(raw string) (string, *domain.CommandError) {
	out := strings.TrimSpace(raw)
	if out == "" {
		return "", &domain.CommandError{Code: "publish_out_required", Message: "publish build output directory is required", Hint: "Use --out ./dist/wiki"}
	}
	if hasPinaxPathSegment(out) {
		return "", &domain.CommandError{Code: "publish_out_unsafe", Message: "publish output directory must not be inside .pinax", Hint: "Use a dedicated publish output directory such as ./dist/wiki"}
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", &domain.CommandError{Code: "publish_out_invalid", Message: err.Error(), Hint: "Use a valid output directory"}
	}
	return abs, nil
}

func hasPinaxPathSegment(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if part == ".pinax" {
			return true
		}
	}
	return false
}

func publishPlanFromProjection(projection domain.Projection) (domain.PublishPlan, error) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return domain.PublishPlan{}, fmt.Errorf("publish plan data missing")
	}
	plan, ok := data["plan"].(domain.PublishPlan)
	if !ok {
		return domain.PublishPlan{}, fmt.Errorf("publish plan payload missing")
	}
	return plan, nil
}

func publishNotesByPath(root string) (map[string]domain.Note, error) {
	facts, err := scanNoteFacts(root)
	if err != nil {
		return nil, err
	}
	notes := map[string]domain.Note{}
	for _, fact := range ordinaryNoteFacts(facts) {
		notes[fact.note.Path] = fact.note
	}
	return notes, nil
}

func writeWikiPublishOutput(root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	pageByTitle := publishWikiPageIndex(plan, notes)
	for _, item := range plan.Selected {
		switch item.Kind {
		case "note":
			note, ok := notes[item.SourcePath]
			if !ok {
				continue
			}
			if err := writePublishFile(filepath.Join(outDir, publishWikiPageFilename(note)), []byte(publishWikiNoteBody(note, pageByTitle))); err != nil {
				return err
			}
		case "asset":
			if err := copyPublishAsset(root, outDir, item.SourcePath); err != nil {
				return err
			}
		}
	}
	if err := writePublishFile(filepath.Join(outDir, "Home.md"), []byte(publishWikiHome(plan))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "_Sidebar.md"), []byte(publishWikiSidebar(plan, notes))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "Tags.md"), []byte(publishWikiTagsIndex(plan, notes))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "Types.md"), []byte(publishWikiTypesIndex(plan, notes))); err != nil {
		return err
	}
	if err := writePublishFile(filepath.Join(outDir, "Sources.md"), []byte(publishWikiSourcesIndex(plan, notes))); err != nil {
		return err
	}
	manifest := domain.PublishManifest{SchemaVersion: "pinax.publish_manifest.v1", ProfileName: profile.Name, Target: profile.Target, Renderer: string(profile.Renderer), Items: plan.Selected}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(outDir, "pinax-publish-manifest.json"), append(body, '\n'))
}

func writeBundlePublishOutput(root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	title := strings.TrimSpace(profile.Site.Title)
	if title == "" {
		title = profile.Name
	}
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, note := range publishSelectedNotes(plan, notes) {
		body := strings.TrimSpace(note.Body)
		body = strings.ReplaceAll(body, "../assets/", "assets/")
		body = strings.ReplaceAll(body, "../attachments/", "attachments/")
		if body == "" {
			body = "# " + note.Title
		}
		b.WriteString(body)
		b.WriteString("\n\n---\n\n")
	}
	if err := writePublishFile(filepath.Join(outDir, "pinax-gist.md"), []byte(b.String())); err != nil {
		return err
	}
	for _, item := range plan.Selected {
		if item.Kind == "asset" {
			if err := copyPublishAsset(root, outDir, item.SourcePath); err != nil {
				return err
			}
		}
	}
	manifest := domain.PublishManifest{SchemaVersion: "pinax.publish_manifest.v1", ProfileName: profile.Name, Target: profile.Target, Renderer: string(profile.Renderer), Items: plan.Selected}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(outDir, "pinax-publish-manifest.json"), append(body, '\n'))
}

func writePinaxWebStaticOutput(ctx context.Context, root, outDir string, profile domain.PublishProfile, plan domain.PublishPlan, notes map[string]domain.Note) error {
	bundleRoot := filepath.Join(root, ".pinax", "publish", "renderer", time.Now().UTC().Format("20060102T150405.000000000Z"), "bundle")
	if _, err := publishops.BuildPublishBundle(publishops.PublishBundleRequest{VaultRoot: root, BundleRoot: bundleRoot, Profile: profile, Plan: plan, Notes: notes}); err != nil {
		return err
	}
	rendererDir, err := pinaxWebRendererPackageDir()
	if err != nil {
		return &domain.CommandError{Code: "publish_renderer_failed", Message: err.Error(), Hint: "Run task publish:renderer:build from the Pinax project root"}
	}
	adapter := publishops.RendererAdapter{PackageDir: rendererDir, Timeout: 30 * time.Second}
	result, err := adapter.RenderStatic(ctx, publishops.RendererRequest{BundleRoot: bundleRoot, OutDir: outDir, BaseURL: profile.Site.BaseURL, Theme: profile.Site.Theme.Value, RendererVersion: string(profile.Renderer)})
	if err != nil {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = err.Error()
		}
		return &domain.CommandError{Code: "publish_renderer_failed", Message: message, Hint: "Run task publish:renderer:test and rebuild the publish output"}
	}
	return nil
}

func pinaxWebRendererPackageDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PINAX_WEB_RENDERER_DIR")); override != "" {
		return override, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "web", "pinax-web-renderer")
		if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("pinax-web renderer package was not found")
}

func writePublishFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func copyPublishAsset(root, outDir, rel string) error {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	return writePublishFile(filepath.Join(outDir, filepath.FromSlash(rel)), body)
}

func publishWikiPageFilename(note domain.Note) string {
	return publishSlug(note) + ".md"
}

func publishWikiNoteBody(note domain.Note, pageByTitle map[string]string) string {
	body := strings.TrimSpace(note.Body)
	body = rewriteWikiNoteLinks(body, pageByTitle)
	body = strings.ReplaceAll(body, "../assets/", "assets/")
	body = strings.ReplaceAll(body, "../attachments/", "attachments/")
	if body == "" {
		body = "# " + note.Title
	}
	return body + "\n"
}

var wikiNoteLinkPattern = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]+)?(?:\|([^\]]+))?\]\]`)

func rewriteWikiNoteLinks(body string, pageByTitle map[string]string) string {
	return wikiNoteLinkPattern.ReplaceAllStringFunc(body, func(raw string) string {
		match := wikiNoteLinkPattern.FindStringSubmatch(raw)
		if len(match) < 2 {
			return raw
		}
		target := strings.TrimSpace(match[1])
		display := target
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			display = strings.TrimSpace(match[2])
		}
		page, ok := pageByTitle[strings.ToLower(target)]
		if !ok {
			return display + " (unpublished)"
		}
		return "[[" + display + "|" + page + "]]"
	})
}

func publishWikiPageIndex(plan domain.PublishPlan, notes map[string]domain.Note) map[string]string {
	index := map[string]string{}
	for _, note := range publishSelectedNotes(plan, notes) {
		index[strings.ToLower(note.Title)] = strings.TrimSuffix(publishWikiPageFilename(note), ".md")
		if note.ID != "" {
			index[strings.ToLower(note.ID)] = strings.TrimSuffix(publishWikiPageFilename(note), ".md")
		}
	}
	return index
}

func publishWikiHome(plan domain.PublishPlan) string {
	var b strings.Builder
	b.WriteString("# Home\n\n")
	b.WriteString("## Entries\n\n")
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		b.WriteString("- [[")
		b.WriteString(strings.TrimSuffix(filepath.Base(item.OutputPath), filepath.Ext(item.OutputPath)))
		b.WriteString("|")
		b.WriteString(item.Title)
		b.WriteString("]]\n")
	}
	return b.String()
}

func publishWikiSidebar(plan domain.PublishPlan, notes map[string]domain.Note) string {
	var b strings.Builder
	b.WriteString("# Pages\n\n")
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		note, ok := notes[item.SourcePath]
		if !ok {
			continue
		}
		b.WriteString("- [[")
		b.WriteString(strings.TrimSuffix(publishWikiPageFilename(note), ".md"))
		b.WriteString("|")
		b.WriteString(note.Title)
		b.WriteString("]]\n")
	}
	return b.String()
}

func publishWikiTagsIndex(plan domain.PublishPlan, notes map[string]domain.Note) string {
	groups := map[string][]domain.Note{}
	for _, note := range publishSelectedNotes(plan, notes) {
		for _, tag := range note.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				groups[tag] = append(groups[tag], note)
			}
		}
	}
	return publishWikiGroupedIndex("Tags", groups)
}

func publishWikiTypesIndex(plan domain.PublishPlan, notes map[string]domain.Note) string {
	groups := map[string][]domain.Note{}
	for _, note := range publishSelectedNotes(plan, notes) {
		kind := strings.TrimSpace(note.Kind)
		if kind == "" {
			kind = "note"
		}
		groups[kind] = append(groups[kind], note)
	}
	return publishWikiGroupedIndex("Types", groups)
}

func publishWikiSourcesIndex(plan domain.PublishPlan, notes map[string]domain.Note) string {
	var b strings.Builder
	b.WriteString("# Sources\n\n")
	for _, note := range publishSelectedNotes(plan, notes) {
		if note.Kind != "source" {
			continue
		}
		b.WriteString("- [[")
		b.WriteString(strings.TrimSuffix(publishWikiPageFilename(note), ".md"))
		b.WriteString("|")
		b.WriteString(note.Title)
		b.WriteString("]]\n")
	}
	return b.String()
}

func publishWikiGroupedIndex(title string, groups map[string][]domain.Note) string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, key := range keys {
		b.WriteString("## ")
		b.WriteString(key)
		b.WriteString("\n\n")
		for _, note := range groups[key] {
			b.WriteString("- [[")
			b.WriteString(strings.TrimSuffix(publishWikiPageFilename(note), ".md"))
			b.WriteString("|")
			b.WriteString(note.Title)
			b.WriteString("]]\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func publishSelectedNotes(plan domain.PublishPlan, notes map[string]domain.Note) []domain.Note {
	selected := make([]domain.Note, 0)
	for _, item := range plan.Selected {
		if item.Kind != "note" {
			continue
		}
		note, ok := notes[item.SourcePath]
		if ok {
			selected = append(selected, note)
		}
	}
	return selected
}

func hashPublishTree(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		hash.Write(body)
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func publishRunID(t time.Time) string {
	return "run-" + t.UTC().Format("20060102T150405.000000000Z")
}

func publishPlaceholder(command string, req PublishRequest) (domain.Projection, error) {
	err := &domain.CommandError{Code: "publish_not_implemented", Message: "publish command is not implemented yet", Hint: "Continue the pinax-hugo-static-publish OpenSpec change"}
	projection := domain.NewErrorProjection(command, err)
	projection.Facts["profile"] = strings.TrimSpace(req.Profile)
	projection.Facts["target"] = strings.TrimSpace(req.Target)
	projection.Facts["renderer"] = strings.TrimSpace(req.Renderer)
	projection.Actions = []domain.Action{{Name: "implement", Command: "openspec status --change pinax-hugo-static-publish --json"}}
	return projection, err
}

func publishNoteOutputPath(target domain.PublishTarget, note domain.Note) string {
	slug := publishSlug(note)
	if target == domain.PublishTargetGitHubWiki {
		return slug + ".md"
	}
	if target == domain.PublishTargetGitHubGist || target == domain.PublishTargetHTTP {
		return "pinax-gist.md#" + slug
	}
	if target == domain.PublishTargetLocal {
		return filepath.ToSlash(filepath.Join("notes", slug, "index.html"))
	}
	return filepath.ToSlash(filepath.Join("entries", slug, "index.html"))
}

func publishSourceFromNote(note domain.Note) domain.PublishSource {
	return domain.PublishSource{ID: note.ID, Title: note.Title, SourcePath: note.Path, Kind: note.Kind, Status: note.Status, Project: note.Project, Folder: note.Folder}
}

func publishPlanLinkGraph(notes []domain.Note) []domain.NoteLink {
	if len(notes) == 0 {
		return nil
	}
	outgoing, _ := BuildEnhancedLinkGraph(notes)
	links := make([]domain.NoteLink, 0)
	for _, note := range notes {
		links = append(links, outgoing[note.Path]...)
	}
	return links
}

func classifyPublishAsset(root string, profile domain.PublishProfile, link domain.AssetLink) (domain.PublishItem, *domain.PublishViolation) {
	assetPath := strings.TrimSpace(filepath.ToSlash(link.AssetPath))
	if !isSafePublishAssetPath(assetPath) {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset path is not safe for publishing")
	}
	if !publishAssetExtensionAllowed(profile.Assets.AllowedExtensions, filepath.Ext(assetPath)) {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset extension is not allowed for publishing")
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(assetPath)))
	if err != nil || info.IsDir() {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset file is missing")
	}
	if profile.Assets.MaxBytes > 0 && info.Size() > profile.Assets.MaxBytes {
		return domain.PublishItem{}, publishAssetViolation(assetPath, "Linked asset exceeds the publish size limit")
	}
	return domain.PublishItem{Kind: "asset", SourcePath: assetPath, OutputPath: publishAssetOutputPath(assetPath)}, nil
}

func isSafePublishAssetPath(assetPath string) bool {
	if assetPath == "" || filepath.IsAbs(assetPath) || strings.HasPrefix(assetPath, ".pinax/") || strings.HasPrefix(assetPath, "../") || strings.Contains(assetPath, "/../") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(assetPath)))
	return clean == assetPath && clean != "." && clean != ".."
}

func publishAssetExtensionAllowed(allowed []string, ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	for _, candidate := range allowed {
		if strings.ToLower(strings.TrimSpace(candidate)) == ext {
			return true
		}
	}
	return false
}

func publishAssetOutputPath(assetPath string) string {
	if strings.HasPrefix(assetPath, "assets/") {
		return assetPath
	}
	return filepath.ToSlash(filepath.Join("assets", filepath.Base(assetPath)))
}

func publishAssetViolation(path, message string) *domain.PublishViolation {
	return &domain.PublishViolation{Class: domain.PublishViolationAssetNotAllowed, Path: path, Severity: "blocking", Message: message}
}

func publishItemCount(items []domain.PublishItem, kind string) int {
	count := 0
	for _, item := range items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func publishViolationClassCount(violations []domain.PublishViolation, class domain.PublishViolationClass) int {
	count := 0
	for _, violation := range violations {
		if violation.Class == class {
			count++
		}
	}
	return count
}

func publishBrokenLinkCount(links []domain.NoteLink) int {
	count := 0
	for _, link := range links {
		if link.Broken || link.Status == string(domain.LinkStatusBroken) {
			count++
		}
	}
	return count
}

func publishBlockingItemCount(violations []domain.PublishViolation) int {
	seen := map[string]bool{}
	for _, violation := range violations {
		key := violation.Path
		if key == "" {
			key = string(violation.Class)
		}
		seen[key] = true
	}
	return len(seen)
}

func publishSlug(note domain.Note) string {
	base := strings.TrimSpace(note.Title)
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path))
	}
	slug := publishSlugString(base)
	if slug == "" && strings.TrimSpace(note.Title) != "" {
		slug = publishSlugString(strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)))
	}
	if slug == "" {
		return "note"
	}
	return slug
}

func publishSlugString(base string) string {
	base = strings.ToLower(base)
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
