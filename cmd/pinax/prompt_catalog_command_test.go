package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/promptbridge/testfixture"
)

// TestPromptCatalogCommandTreeIsAdditive locks the additive contract: the
// existing local prompt commands keep their identity while the federated
// repository/catalog groups are added alongside them.
func TestPromptCatalogCommandTreeIsAdditive(t *testing.T) {
	t.Parallel()
	helpOut := runCLI(t, "prompt", "--help")
	for _, want := range []string{
		"create", "import", "search", "show", "resolve", "lifecycle", "feedback",
		"repository", "catalog",
	} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("prompt help missing %q:\n%s", want, helpOut)
		}
	}
	repositoryOut := runCLI(t, "prompt", "repository", "--help")
	for _, want := range []string{"add", "list", "show", "remove", "enable", "disable", "sync", "doctor"} {
		if !strings.Contains(repositoryOut, want) {
			t.Fatalf("repository help missing %q:\n%s", want, repositoryOut)
		}
	}
	catalogOut := runCLI(t, "prompt", "catalog", "--help")
	for _, want := range []string{"search", "show", "resolve", "inspect", "validate", "preview", "install"} {
		if !strings.Contains(catalogOut, want) {
			t.Fatalf("catalog help missing %q:\n%s", want, catalogOut)
		}
	}
}

// TestPromptCatalogOutputModeParity verifies the same projection feeds the
// human, agent, and JSON renderers with stable machine keys.
func TestPromptCatalogOutputModeParity(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	source, err := testfixture.WriteRepository(filepath.Join(root, "repo"), testfixture.RepositoryOptions{
		RepositoryID: "parity", PackageID: "audio", SolutionID: "podcast",
		Title: "合同对齐旁白", WithContract: false,
	})
	if err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	addOut, addErr := runCLIWithRepoEnv(t, home, "prompt", "repository", "add", "parity", "--source", source, "--json")
	if addErr != nil {
		t.Fatalf("repository add failed: %s", addOut)
	}
	syncOut, syncErr := runCLIWithRepoEnv(t, home, "prompt", "repository", "sync", "parity", "--json")
	if syncErr != nil {
		t.Fatalf("repository sync failed: %s", syncOut)
	}

	jsonOut, jsonErr := runCLIWithRepoEnv(t, home, "prompt", "repository", "list", "--json")
	if jsonErr != nil {
		t.Fatalf("repository list failed: %s", jsonOut)
	}
	envelope := parsePromptEnvelope(t, jsonOut)
	if envelope["command"] != "prompt.repository.list" || envelope["facts"].(map[string]any)["results"] != "1" {
		t.Fatalf("list envelope = %#v", envelope)
	}
	if strings.Contains(jsonOut, "credential_ref") {
		t.Fatalf("list output exposed credential refs:\n%s", jsonOut)
	}

	agentOut, agentErr := runCLIWithRepoEnv(t, home, "prompt", "repository", "list", "--agent")
	if agentErr != nil {
		t.Fatalf("agent list failed: %s", agentOut)
	}
	for _, want := range []string{"command=prompt.repository.list", "fact.results=1", "fact.operation_id=promptrepo.repository.list.v1", "fact.repository.1.id=parity", "fact.repository.1.state=ready"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}

	defaultOut, defaultErr := runCLIWithRepoEnv(t, home, "prompt", "repository", "list")
	if defaultErr != nil {
		t.Fatalf("default list failed: %s", defaultOut)
	}
	for _, want := range []string{"parity", "Shared prompt repositories listed"} {
		if !strings.Contains(defaultOut, want) {
			t.Fatalf("default output missing %q:\n%s", want, defaultOut)
		}
	}
}

func runCLIWithRepoEnv(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("PROMPTREPO_HOME", home)
	t.Setenv("PROMPTREPO_CACHE", filepath.Join(home, "cache"))
	return runCLIExpectError(args...)
}
