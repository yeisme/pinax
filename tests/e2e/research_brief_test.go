package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestResearchBriefWorkflow(t *testing.T) {
	t.Parallel()
	// 这里只验证 CLI 流程；合成来源、预写简报及模拟采纳不能充当真实用户效果。
	testscript.Run(t, testscript.Params{
		Dir: "testdata/research_brief/scripts",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars,
				"PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"))
			return nil
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"vault-digest": researchVaultDigest,
			"next-second": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) != 0 {
					ts.Fatalf("usage: next-second")
				}
				// 旧 Init 按秒重写时间，跨秒后比较才能稳定检测伪只读索引写入。
				time.Sleep(1100 * time.Millisecond)
			},
			"source-id": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) != 1 {
					ts.Fatalf("usage: source-id <CLI-output-file>")
				}
				var result struct {
					Facts map[string]string `json:"facts"`
				}
				if err := json.Unmarshal([]byte(ts.ReadFile(args[0])), &result); err != nil {
					ts.Fatalf("invalid CLI envelope: %v", err)
				}
				id := result.Facts["note_id"]
				if id == "" {
					ts.Fatalf("CLI did not return a source identity")
				}
				ts.Setenv("SOURCE_ID", id)
			},
		},
	})
}

func researchVaultDigest(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: vault-digest <fixture-vault> <digest-file>")
	}
	root := ts.MkAbs(args[0])
	var manifest strings.Builder
	// 连目录和文件模式一起比较，防止只读检查漏掉新建的索引、回执或临时目录。
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			_, _ = fmt.Fprintf(&manifest, "%s %s\n", rel, info.Mode())
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected non-regular fixture file: %s", rel)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		_, _ = fmt.Fprintf(&manifest, "%s %s %x\n", rel, info.Mode(), digest.Sum(nil))
		return closeErr
	})
	if err != nil {
		ts.Fatalf("cannot fingerprint fixture vault: %v", err)
	}
	if err := os.WriteFile(ts.MkAbs(args[1]), []byte(manifest.String()), 0o600); err != nil {
		ts.Fatalf("cannot write fixture digest: %v", err)
	}
}
