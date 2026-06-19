package publishops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type DeployPolicyRequest struct {
	VaultRoot string
	Profile   domain.PublishProfile
}

type DeployPolicy struct {
	Mode       domain.PublishDeployMode `json:"mode"`
	Target     domain.PublishTarget     `json:"target"`
	Repo       string                   `json:"repo,omitempty"`
	Branch     string                   `json:"branch,omitempty"`
	Strategy   string                   `json:"strategy,omitempty"`
	Endpoint   string                   `json:"endpoint,omitempty"`
	Method     string                   `json:"method,omitempty"`
	SecretRef  string                   `json:"secret_ref,omitempty"`
	GistID     string                   `json:"gist_id,omitempty"`
	Visibility string                   `json:"visibility,omitempty"`
}

func ParseDeployPolicy(req DeployPolicyRequest) (DeployPolicy, error) {
	profile := req.Profile
	policy := DeployPolicy{Mode: profile.Deploy.Mode, Target: profile.Target, Repo: strings.TrimSpace(profile.Deploy.Repo), Branch: strings.TrimSpace(profile.Deploy.Branch), Strategy: strings.TrimSpace(profile.Deploy.Strategy), Endpoint: strings.TrimSpace(profile.Deploy.Endpoint), Method: strings.ToUpper(strings.TrimSpace(profile.Deploy.Method)), SecretRef: strings.TrimSpace(profile.Deploy.SecretRef), GistID: strings.TrimSpace(profile.Deploy.GistID), Visibility: strings.TrimSpace(profile.Deploy.Visibility)}
	if policy.Mode == "" {
		policy.Mode = domain.PublishDeployModeNone
	}
	if policy.Mode == domain.PublishDeployModeNone {
		switch policy.Target {
		case domain.PublishTargetGitHubGist:
			policy.Mode = domain.PublishDeployModeGist
		case domain.PublishTargetHTTP:
			policy.Mode = domain.PublishDeployModeHTTP
		}
	}
	switch policy.Mode {
	case domain.PublishDeployModeNone:
		return policy, nil
	case domain.PublishDeployModeGit:
		if policy.Repo == "" {
			return DeployPolicy{}, fmt.Errorf("publish deploy git repo is required")
		}
	case domain.PublishDeployModeGist:
		if policy.Visibility == "" {
			policy.Visibility = "secret"
		}
		if policy.Visibility != "secret" && policy.Visibility != "public" {
			return DeployPolicy{}, fmt.Errorf("unsupported publish gist visibility %q", policy.Visibility)
		}
	case domain.PublishDeployModeHTTP:
		if policy.Endpoint == "" {
			return DeployPolicy{}, fmt.Errorf("publish deploy http endpoint is required")
		}
		if policy.Method == "" {
			policy.Method = "POST"
		}
		if policy.Method != "POST" && policy.Method != "PUT" {
			return DeployPolicy{}, fmt.Errorf("unsupported publish http method %q", policy.Method)
		}
	default:
		return DeployPolicy{}, fmt.Errorf("unsupported publish deploy mode %q", policy.Mode)
	}
	if policy.Mode == domain.PublishDeployModeGist || policy.Mode == domain.PublishDeployModeHTTP {
		return policy, nil
	}
	if policy.Branch == "" || (policy.Target == domain.PublishTargetGitHubWiki && policy.Branch == "gh-pages") {
		policy.Branch = defaultDeployBranch(policy.Target)
	}
	if policy.Strategy == "" {
		policy.Strategy = "clean-worktree"
	}
	if policy.Strategy != "clean-worktree" && policy.Strategy != "orphan" {
		return DeployPolicy{}, fmt.Errorf("unsupported publish deploy strategy %q", policy.Strategy)
	}
	if err := rejectUnsafeDeployRepo(req.VaultRoot, policy.Repo); err != nil {
		return DeployPolicy{}, err
	}
	return policy, nil
}

func defaultDeployBranch(target domain.PublishTarget) string {
	if target == domain.PublishTargetGitHubWiki {
		return "master"
	}
	return "gh-pages"
}

func rejectUnsafeDeployRepo(vaultRoot, repo string) error {
	if isRemoteGitRepo(repo) {
		return nil
	}
	if strings.TrimSpace(vaultRoot) == "" {
		return fmt.Errorf("publish deploy vault root is required")
	}
	vaultAbs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return err
	}
	repoPath := repo
	if !filepath.IsAbs(repoPath) {
		repoPath = filepath.Join(vaultAbs, filepath.FromSlash(repoPath))
	}
	repoAbs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	if repoAbs == vaultAbs {
		return fmt.Errorf("publish deploy repo must not be the vault root")
	}
	rel, err := filepath.Rel(vaultAbs, repoAbs)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".pinax" || strings.HasPrefix(rel, ".pinax/") || (rel != ".." && !strings.HasPrefix(rel, "../")) {
		return fmt.Errorf("publish deploy repo must not be inside the vault")
	}
	return nil
}

func isRemoteGitRepo(repo string) bool {
	repo = strings.TrimSpace(repo)
	return strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")
}
