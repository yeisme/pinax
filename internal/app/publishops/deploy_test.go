package publishops

import (
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestParseDeployPolicyRejectsUnsafeTargets(t *testing.T) {
	vaultRoot := t.TempDir()
	profile := domain.NewDefaultPublishProfile("pages", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Deploy.Mode = domain.PublishDeployModeGit
	profile.Deploy.Branch = "gh-pages"
	profile.Deploy.Strategy = "clean-worktree"

	profile.Deploy.Repo = vaultRoot
	if _, err := ParseDeployPolicy(DeployPolicyRequest{VaultRoot: vaultRoot, Profile: profile}); err == nil {
		t.Fatalf("deploy to vault root should be rejected")
	}

	profile.Deploy.Repo = filepath.Join(vaultRoot, ".pinax", "publish-repo")
	if _, err := ParseDeployPolicy(DeployPolicyRequest{VaultRoot: vaultRoot, Profile: profile}); err == nil {
		t.Fatalf("deploy to .pinax should be rejected")
	}

	profile.Deploy.Repo = filepath.Join(vaultRoot, "notes", "publish-repo")
	if _, err := ParseDeployPolicy(DeployPolicyRequest{VaultRoot: vaultRoot, Profile: profile}); err == nil {
		t.Fatalf("deploy inside the vault should be rejected")
	}

	profile.Deploy.Repo = filepath.Join(vaultRoot, "public-repo")
	profile.Deploy.Strategy = "force"
	if _, err := ParseDeployPolicy(DeployPolicyRequest{VaultRoot: vaultRoot, Profile: profile}); err == nil {
		t.Fatalf("unknown deploy strategy should be rejected")
	}
}

func TestParseDeployPolicyNormalizesGitPolicy(t *testing.T) {
	vaultRoot := t.TempDir()
	repo := filepath.Join(t.TempDir(), "pages-repo")
	profile := domain.NewDefaultPublishProfile("pages", domain.PublishTargetGitHubPages, domain.PublishRendererHugo)
	profile.Deploy = domain.PublishDeploy{Mode: domain.PublishDeployModeGit, Repo: repo, Branch: "", Strategy: ""}

	policy, err := ParseDeployPolicy(DeployPolicyRequest{VaultRoot: vaultRoot, Profile: profile})
	if err != nil {
		t.Fatalf("parse deploy policy: %v", err)
	}
	if policy.Mode != domain.PublishDeployModeGit || policy.Repo != repo || policy.Branch != "gh-pages" || policy.Strategy != "clean-worktree" || policy.Target != domain.PublishTargetGitHubPages {
		t.Fatalf("policy = %#v", policy)
	}
}
