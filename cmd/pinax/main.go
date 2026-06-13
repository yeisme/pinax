package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/cli"
	"github.com/yeisme/pinax/internal/domain"
)

var version = "dev"

func main() {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		var commandErr *domain.CommandError
		if !errors.As(err, &commandErr) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	return cli.NewRootCommand(version)
}
