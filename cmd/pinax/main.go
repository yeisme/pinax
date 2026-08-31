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
)

var version = "dev"

func main() {
	root := newRootCommand()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
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
