package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/cli"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/sqlitedsn"
)

var version = "dev"

func main() {
	root := newRootCommand()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := root.ExecuteContext(ctx)
	// 进程退出前关闭全部 SQLite 连接：checkpoint WAL 回主库并移除 -wal/-shm
	// sidecar，vault 树不在命令间残留运行时产物。
	sqlitedsn.CloseAll()
	if err != nil {
		var commandErr *domain.CommandError
		if !errors.As(err, &commandErr) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	return cli.NewRootCommandWithDeps(cli.Deps{Version: version, Service: newRootService()})
}

// newRootService applies a deterministic frozen clock only inside the test
// binary: tests set PINAX_TEST_MODE=1 together with PINAX_TEST_NOW, and the
// shipped binary ignores both. The domain clock itself has no environment
// override (app.Service.WithNowFunc is the only injection path).
func newRootService() *app.Service {
	svc := app.NewService()
	if os.Getenv("PINAX_TEST_MODE") != "1" {
		return svc
	}
	raw := strings.TrimSpace(os.Getenv("PINAX_TEST_NOW"))
	if raw == "" {
		return svc
	}
	frozen, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		if day, dayErr := time.Parse("2006-01-02", raw); dayErr == nil {
			frozen = day
		}
	}
	if frozen.IsZero() {
		return svc
	}
	return svc.WithNowFunc(func() time.Time { return frozen.UTC() })
}
