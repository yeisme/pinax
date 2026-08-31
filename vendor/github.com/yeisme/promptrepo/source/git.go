package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yeisme/promptrepo"
)

var githubPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type GitAdapter struct{}

func (GitAdapter) Kind() string { return "git" }

func (GitAdapter) Sync(ctx context.Context, profile promptrepo.RepositoryProfile, cacheRoot string) (SyncResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "git executable is not available", false, err)
	}
	remote, err := normalizeGitRemote(profile.Source)
	if err != nil {
		return SyncResult{}, err
	}
	revision := strings.TrimSpace(profile.Revision)
	if revision == "" {
		revision = strings.TrimSpace(profile.Channel)
	}
	if revision == "" {
		revision = "HEAD"
	}
	if strings.HasPrefix(revision, "-") || strings.ContainsAny(revision, "\x00\n\r") {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid Git revision", false, nil)
	}
	sum := sha256.Sum256([]byte(profile.Source))
	repositoryPath := filepath.Join(cacheRoot, "git", hex.EncodeToString(sum[:12])+".git")
	if err := os.MkdirAll(filepath.Dir(repositoryPath), 0o700); err != nil {
		return SyncResult{}, sourceError("git", err)
	}
	if _, err := os.Stat(repositoryPath); os.IsNotExist(err) {
		if output, cloneErr := runGit(ctx, "-c", "core.hooksPath=/dev/null", "clone", "--bare", "--no-tags", remote, repositoryPath); cloneErr != nil {
			return SyncResult{}, classifyGitError(profile, output, cloneErr)
		}
	}
	output, err := runGit(ctx, "-c", "core.hooksPath=/dev/null", "--git-dir", repositoryPath, "fetch", "--prune", "origin", revision)
	if err != nil {
		return SyncResult{}, classifyGitError(profile, output, err)
	}
	commitRaw, err := runGit(ctx, "--git-dir", repositoryPath, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return SyncResult{}, sourceError("git", err)
	}
	commit := strings.TrimSpace(commitRaw)
	if !regexp.MustCompile(`^[a-f0-9]{40,64}$`).MatchString(commit) {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "Git source did not resolve to an exact commit", false, nil)
	}
	catalogObject := commit + ":catalog.json"
	size, err := gitObjectSize(ctx, repositoryPath, catalogObject)
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeNotFound, "catalog.json is not present at the resolved Git commit", false, err)
	}
	if size > maxCatalogBytes {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeInvalidRequest, "catalog exceeds size limit", false, nil)
	}
	payload, err := runGit(ctx, "--git-dir", repositoryPath, "show", catalogObject)
	if err != nil {
		return SyncResult{}, promptrepo.NewError(promptrepo.CodeNotFound, "catalog.json is not present at the resolved Git commit", false, err)
	}
	catalog, err := decodeCatalog([]byte(payload))
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Revision: commit, Catalog: catalog}, nil
}

func (GitAdapter) ReadTemplate(ctx context.Context, profile promptrepo.RepositoryProfile, snapshot promptrepo.SnapshotMetadata, template promptrepo.TemplateRole, cacheRoot string) ([]byte, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40,64}$`).MatchString(snapshot.Revision) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "Git snapshot revision is not an exact commit", false, nil)
	}
	if strings.HasPrefix(template.Path, "/") || strings.Contains(template.Path, "..") || strings.ContainsAny(template.Path, "\x00\n\r") {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid Git prompt template path", false, nil)
	}
	sum := sha256.Sum256([]byte(profile.Source))
	repositoryPath := filepath.Join(cacheRoot, "git", hex.EncodeToString(sum[:12])+".git")
	templateObject := snapshot.Revision + ":" + template.Path
	size, err := gitObjectSize(ctx, repositoryPath, templateObject)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "prompt template is not present at the resolved Git commit", false, err)
	}
	if size > maxTemplateBytes {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "prompt template exceeds size limit", false, nil)
	}
	payload, err := runGit(ctx, "--git-dir", repositoryPath, "show", templateObject)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "prompt template is not present at the resolved Git commit", false, err)
	}
	content := []byte(payload)
	if err := verifyTemplateDigest(content, template.Digest); err != nil {
		return nil, err
	}
	return content, nil
}

func (GitAdapter) ReadCompanion(ctx context.Context, profile promptrepo.RepositoryProfile, snapshot promptrepo.SnapshotMetadata, companionPath string, cacheRoot string) ([]byte, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40,64}$`).MatchString(snapshot.Revision) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "Git snapshot revision is not an exact commit", false, nil)
	}
	if !safeObjectPath(companionPath) {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid Git template contract companion path", false, nil)
	}
	sum := sha256.Sum256([]byte(profile.Source))
	repositoryPath := filepath.Join(cacheRoot, "git", hex.EncodeToString(sum[:12])+".git")
	companionObject := snapshot.Revision + ":" + companionPath
	size, err := gitObjectSize(ctx, repositoryPath, companionObject)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "template contract companion is not present at the resolved Git commit", false, err)
	}
	if size > maxCompanionBytes {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template contract companion exceeds size limit", false, nil)
	}
	payload, err := runGit(ctx, "--git-dir", repositoryPath, "show", companionObject)
	if err != nil {
		return nil, promptrepo.NewError(promptrepo.CodeNotFound, "template contract companion is not present at the resolved Git commit", false, err)
	}
	if len(payload) > maxCompanionBytes {
		return nil, promptrepo.NewError(promptrepo.CodeInvalidRequest, "template contract companion exceeds size limit", false, nil)
	}
	return []byte(payload), nil
}

func normalizeGitRemote(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid Git repository URI", false, err)
	}
	switch parsed.Scheme {
	case "git+file", "git+https", "git+ssh":
		return strings.TrimPrefix(raw, "git+"), nil
	case "github":
		owner := parsed.Host
		repository := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
		if !githubPart.MatchString(owner) || !githubPart.MatchString(repository) {
			return "", promptrepo.NewError(promptrepo.CodeInvalidRequest, "github source must be github://owner/repository", false, nil)
		}
		return fmt.Sprintf("https://github.com/%s/%s.git", owner, repository), nil
	default:
		return "", promptrepo.NewError(promptrepo.CodeUnsupportedSourceScheme, "unsupported Git repository scheme", false, nil)
	}
}

func runGit(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	payload, err := command.CombinedOutput()
	return string(payload), err
}

func gitObjectSize(ctx context.Context, repositoryPath, object string) (int64, error) {
	raw, err := runGit(ctx, "--git-dir", repositoryPath, "cat-file", "-s", object)
	if err != nil {
		return 0, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || size < 0 {
		return 0, fmt.Errorf("Git object size is invalid")
	}
	return size, nil
}

func classifyGitError(profile promptrepo.RepositoryProfile, output string, cause error) error {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "authentication") || strings.Contains(lower, "authorization") || strings.Contains(lower, "could not read username") {
		code := promptrepo.CodeAuthRequired
		if profile.CredentialRef != "" {
			code = promptrepo.CodeAuthorizationFailed
		}
		return promptrepo.NewError(code, "Git repository authorization failed", false, cause)
	}
	return promptrepo.NewError(promptrepo.CodeSourceFetchFailed, "Git repository fetch failed", true, cause)
}
