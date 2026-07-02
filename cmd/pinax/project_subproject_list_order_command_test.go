package main

import (
	"strings"
	"testing"
	"time"
)

func TestProjectSubprojectListDefaultsToCurrentProjectAndCreationOrder(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "history-learning", "--name", "History Learning", "--notes-prefix", "notes/history-learning", "--vault", root, "--json")

	for _, slug := range []string{"china-xia-shang-zhou", "china-qin-han", "china-five-dynasties-song"} {
		runCLI(t, "project", "subproject", "create", "history-learning", slug, "--vault", root, "--json")
		time.Sleep(20 * time.Millisecond)
	}

	out := runCLI(t, "project", "subproject", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=project.subproject.list", "fact.project=history-learning", "fact.subprojects=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("subproject list missing %q:\n%s", want, out)
		}
	}
	idxXia := strings.Index(out, "fact.subproject.1=china-xia-shang-zhou")
	idxQin := strings.Index(out, "fact.subproject.2=china-qin-han")
	idxFive := strings.Index(out, "fact.subproject.3=china-five-dynasties-song")
	if idxXia < 0 || idxQin < 0 || idxFive < 0 || idxXia >= idxQin || idxQin >= idxFive {
		t.Fatalf("subproject list did not preserve creation order:\n%s", out)
	}
}
