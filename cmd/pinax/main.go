package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pinax",
		Short: "本地优先统一笔记 Agent CLI",
		Long:  "Pinax 管理本地 Markdown 笔记 vault、索引投影、Git 版本建议和外部 CLI provider 工作流。",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "显示 Pinax 版本",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "pinax %s\n", version)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "检查 Pinax 本地开发底座",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "Pinax 开发底座可用。后续能力请先进入 OpenSpec change。")
		},
	})

	return cmd
}
