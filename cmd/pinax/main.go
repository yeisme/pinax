package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/dashboard"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/mcpserver"
	"github.com/yeisme/pinax/internal/output"
	"golang.org/x/term"
)

var version = "dev"

const pinaxHelpTemplate = `{{with (or .Long .Short)}}简介
  {{.}}

{{end}}{{if or .Runnable .HasSubCommands}}用法
  {{.UseLine}}

{{end}}{{if .HasAvailableSubCommands}}可用命令
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}
{{end}}{{if .HasAvailableLocalFlags}}参数
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasAvailableInheritedFlags}}全局参数
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasExample}}示例
{{.Example}}

{{end}}{{if .HasSubCommands}}使用 "{{.CommandPath}} [command] --help" 查看子命令说明。
{{end}}`

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
	svc := app.NewService()
	var jsonMode bool
	var agentMode bool
	var eventsMode bool
	var explainMode bool
	var vaultPath string
	var yes bool
	var snapshotMessage string
	var title string
	var projectName string
	var projectDescription string
	var projectNotesPrefix string
	var storageRoot string
	var s3Bucket string
	var s3Region string
	var s3Prefix string
	var s3Endpoint string
	var s3Profile string
	var noteProject string
	var noteGroup string
	var noteFolder string
	var noteKind string
	var noteTags string
	var noteTemplate string
	var noteBody string
	var noteFrom string
	var noteDir string
	var noteSlug string
	var noteStatus string
	var noteUseStdin bool
	var noteDryRun bool
	var noteOpen bool
	var noteListTag string
	var noteListProject string
	var noteListStatus string
	var noteListSort string
	var noteListPathPrefix string
	var noteListCreatedAfter string
	var noteListUpdatedBefore string
	var noteRecent bool
	var noteLimit int
	var noteEditor string
	var noteHard bool
	var journalDate string
	var journalPrev bool
	var journalNext bool
	var templateSourcePath string
	var templateBody string
	var templateUseStdin bool
	var templateOverwrite bool
	var templateVars []string
	var syncTarget string
	var staleAfter string
	var repairSave bool
	var repairPlanID string
	var organizeSave bool
	var searchLinkTarget string
	var searchHasAttachment bool
	var searchCreatedAfter string
	var searchUpdatedAfter string
	var searchAllowStale bool
	var importConflict string
	var importDryRun bool
	var dashboardPort int
	var backendName string
	var backendRoot string
	var backendRemote string
	var backendDryRun bool
	var planFromPeriod string
	var planWithTaskBridge bool
	var planDryRun bool
	var planSave bool

	cmd := &cobra.Command{
		Use:           "pinax",
		Short:         "本地优先 Markdown vault 笔记 CLI",
		Long:          "Pinax 管理本地 Markdown vault 笔记、索引投影、Git 版本建议和本地 dashboard。",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "help" || cmd.CommandPath() == "pinax completion" {
				return nil
			}
			return validateOutputMode(cmd, jsonMode, agentMode, eventsMode, explainMode)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().BoolVar(&jsonMode, "json", false, "输出 JSON envelope")
	cmd.PersistentFlags().BoolVar(&agentMode, "agent", false, "输出 agent key=value")
	cmd.PersistentFlags().BoolVar(&eventsMode, "events", false, "输出 NDJSON 事件流")
	cmd.PersistentFlags().BoolVar(&explainMode, "explain", false, "输出中文可审查解释")
	cmd.PersistentFlags().StringVar(&vaultPath, "vault", ".", "Pinax vault 路径")
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "cli.flag", "flag_error", err.Error(), cmd.CommandPath()+" --help")
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "显示 Pinax 版本",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection := domain.NewProjection("system.version", fmt.Sprintf("pinax %s", version))
			projection.Facts["version"] = version
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, nil)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "统计本地 Markdown vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.VaultStats(cmd.Context(), app.VaultStatsRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "检查本地 Markdown vault 健康度",
		RunE: func(cmd *cobra.Command, args []string) error {
			duration, parseErr := parseDurationDays(staleAfter)
			if parseErr != nil {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "vault.doctor", "invalid_stale_after", parseErr.Error(), "使用类似 90d 或 2160h 的值")
			}
			projection, err := svc.VaultDoctor(cmd.Context(), app.VaultDoctorRequest{VaultPath: vaultPath, StaleAfter: duration})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	doctorCmd.Flags().StringVar(&staleAfter, "stale-after", "90d", "stale note 阈值，例如 90d 或 2160h")
	cmd.AddCommand(doctorCmd)

	dashboardCmd := &cobra.Command{
		Use:   "dashboard",
		Short: "启动只读本地 Markdown vault dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dashboard.ListenAndServe(cmd.Context(), svc, vaultPath, dashboardPort, func(format string, args ...any) {
				fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
			})
		},
	}
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", 0, "dashboard localhost 端口，0 表示自动分配")
	cmd.AddCommand(dashboardCmd)

	initCmd := &cobra.Command{
		Use:     "init [vault]",
		Short:   "初始化本地 Pinax Markdown vault",
		Long:    "初始化本地 Pinax Markdown vault。未提供 vault 参数时使用 --vault 指定路径；默认是当前目录。",
		Example: "pinax init\npinax init ./my-notes --title \"我的知识库\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "vault.init", "too_many_arguments", "init 最多接收一个 vault 路径", "运行 pinax init --help 查看用法")
			}
			targetVault := vaultPath
			if len(args) == 1 {
				targetVault = args[0]
			}
			projection, err := svc.InitVault(cmd.Context(), app.InitVaultRequest{VaultPath: targetVault, Title: title})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	initCmd.Flags().StringVar(&title, "title", "", "vault 标题")
	cmd.AddCommand(initCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "校验本地 Pinax vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ValidateVault(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})

	dailyRequest := func() app.DailyRequest {
		return app.DailyRequest{VaultPath: vaultPath, Editor: noteEditor, Body: noteBody, Date: journalDate, Prev: journalPrev, Next: journalNext}
	}
	addJournalFlags := func(c *cobra.Command, period string, includeEditor bool, includeBody bool) {
		c.Flags().StringVar(&journalDate, "date", "", "目标日期，格式 YYYY-MM-DD")
		_ = c.RegisterFlagCompletionFunc("date", journalDateCompletion(period, func() string { return vaultPath }))
		c.Flags().BoolVar(&journalPrev, "prev", false, "读取上一天")
		c.Flags().BoolVar(&journalNext, "next", false, "读取下一天")
		if includeEditor {
			c.Flags().StringVar(&noteEditor, "editor", "", "编辑器命令，默认读取 EDITOR")
		}
		if includeBody {
			c.Flags().StringVar(&noteBody, "body", "", "追加正文")
		}
	}

	dailyCmd := &cobra.Command{Use: "daily", Short: "管理 daily note 工作流"}
	dailyOpenCmd := &cobra.Command{Use: "open", Short: "创建或打开今天的 daily note", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.DailyOpen(cmd.Context(), dailyRequest())
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	addJournalFlags(dailyOpenCmd, "daily", true, false)
	dailyCmd.AddCommand(dailyOpenCmd)
	dailyCmd.AddCommand(&cobra.Command{Use: "show", Short: "读取今天的 daily note", RunE: func(cmd *cobra.Command, args []string) error {
		mode := selectedMode(jsonMode, agentMode, eventsMode, explainMode)
		projection, err := svc.DailyShow(cmd.Context(), dailyRequest())
		return renderJournalProjection(cmd, mode, projection, err, "daily", func(date string) (domain.Projection, error) {
			req := dailyRequest()
			req.Date = date
			req.Prev = false
			req.Next = false
			return svc.DailyShow(cmd.Context(), req)
		})
	}})
	dailyAppendCmd := &cobra.Command{Use: "append", Short: "追加内容到今天的 daily note", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.DailyAppend(cmd.Context(), dailyRequest())
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	addJournalFlags(dailyAppendCmd, "daily", false, true)
	for _, child := range dailyCmd.Commands() {
		if child.Name() == "show" {
			addJournalFlags(child, "daily", false, false)
		}
	}
	dailyCmd.AddCommand(dailyAppendCmd)
	cmd.AddCommand(dailyCmd)

	for _, journal := range []struct {
		name   string
		short  string
		open   func(context.Context, app.DailyRequest) (domain.Projection, error)
		show   func(context.Context, app.DailyRequest) (domain.Projection, error)
		append func(context.Context, app.DailyRequest) (domain.Projection, error)
	}{
		{name: "weekly", short: "管理 weekly note 工作流", open: svc.WeeklyOpen, show: svc.WeeklyShow, append: svc.WeeklyAppend},
		{name: "monthly", short: "管理 monthly note 工作流", open: svc.MonthlyOpen, show: svc.MonthlyShow, append: svc.MonthlyAppend},
	} {
		journal := journal
		journalCmd := &cobra.Command{Use: journal.name, Short: journal.short}
		openCmd := &cobra.Command{Use: "open", Short: "创建或打开 " + journal.name + " note", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := journal.open(cmd.Context(), dailyRequest())
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		}}
		showCmd := &cobra.Command{Use: "show", Short: "读取 " + journal.name + " note", RunE: func(cmd *cobra.Command, args []string) error {
			mode := selectedMode(jsonMode, agentMode, eventsMode, explainMode)
			projection, err := journal.show(cmd.Context(), dailyRequest())
			return renderJournalProjection(cmd, mode, projection, err, journal.name, func(date string) (domain.Projection, error) {
				req := dailyRequest()
				req.Date = date
				req.Prev = false
				req.Next = false
				return journal.show(cmd.Context(), req)
			})
		}}
		appendCmd := &cobra.Command{Use: "append", Short: "追加内容到 " + journal.name + " note", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := journal.append(cmd.Context(), dailyRequest())
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		}}
		addJournalFlags(openCmd, journal.name, true, false)
		addJournalFlags(showCmd, journal.name, false, false)
		addJournalFlags(appendCmd, journal.name, false, true)
		journalCmd.AddCommand(openCmd, showCmd, appendCmd)
		cmd.AddCommand(journalCmd)
	}

	inboxCmd := &cobra.Command{Use: "inbox", Short: "管理 inbox 捕获和整理工作流"}
	inboxCaptureCmd := &cobra.Command{Use: "capture <title>", Short: "快速捕获一篇 inbox 笔记", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "inbox.capture", "argument_required", "inbox capture 需要一个标题", "pinax inbox capture <title> --vault <vault>")
		}
		projection, err := svc.InboxCapture(cmd.Context(), app.CreateNoteRequest{VaultPath: vaultPath, Title: args[0], Tags: splitCSV(noteTags), Body: noteBody, Slug: noteSlug})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	inboxCaptureCmd.Flags().StringVar(&noteBody, "body", "", "笔记正文")
	inboxCaptureCmd.Flags().StringVar(&noteTags, "tags", "", "逗号分隔标签")
	inboxCaptureCmd.Flags().StringVar(&noteSlug, "slug", "", "文件名 slug")
	inboxCmd.AddCommand(inboxCaptureCmd)
	inboxCmd.AddCommand(&cobra.Command{Use: "list", Short: "列出 inbox 笔记", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.InboxList(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	inboxTriageCmd := &cobra.Command{Use: "triage <note>", Short: "把 inbox 笔记整理到项目和文件夹", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "inbox.triage", "argument_required", "inbox triage 需要一个 note 引用", "pinax inbox triage <note> --group <group> --vault <vault>")
		}
		projection, err := svc.InboxTriage(cmd.Context(), app.InboxTriageRequest{VaultPath: vaultPath, NoteRef: args[0], Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	inboxTriageCmd.Flags().StringVar(&noteGroup, "group", "", "目标分组或项目 slug")
	inboxTriageCmd.Flags().StringVar(&noteFolder, "folder", "", "目标相对文件夹")
	inboxTriageCmd.Flags().StringVar(&noteKind, "kind", "", "目标用途分类")
	inboxTriageCmd.Flags().StringVar(&noteStatus, "status", "", "目标状态")
	inboxCmd.AddCommand(inboxTriageCmd)
	cmd.AddCommand(inboxCmd)

	for _, dimension := range []string{"tag", "folder", "kind", "group"} {
		dim := dimension
		dimCmd := &cobra.Command{Use: dim, Short: "列出 " + dim + " 组织视图"}
		dimCmd.AddCommand(&cobra.Command{Use: "list", Short: "列出 " + dim + " counts", RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ListDimension(cmd.Context(), app.VaultRequest{VaultPath: vaultPath}, dim)
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		}})
		cmd.AddCommand(dimCmd)
	}

	viewCmd := &cobra.Command{Use: "view", Short: "管理保存的笔记检索视图"}
	viewSaveCmd := &cobra.Command{Use: "save <name>", Short: "保存一组笔记检索条件", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "view.save", "argument_required", "view save 需要名称", "pinax view save <name> --vault <vault>")
		}
		projection, err := svc.SaveView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0], Tags: splitCSV(noteListTag), Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteListStatus, Sort: noteListSort, Limit: noteLimit, CreatedAfter: noteListCreatedAfter, UpdatedBefore: noteListUpdatedBefore})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	viewSaveCmd.Flags().StringVar(&noteListTag, "tag", "", "按标签过滤，可逗号分隔")
	viewSaveCmd.Flags().StringVar(&noteGroup, "group", "", "按分组过滤")
	viewSaveCmd.Flags().StringVar(&noteFolder, "folder", "", "按文件夹过滤")
	viewSaveCmd.Flags().StringVar(&noteKind, "kind", "", "按用途分类过滤")
	viewSaveCmd.Flags().StringVar(&noteListStatus, "status", "", "按状态过滤")
	viewSaveCmd.Flags().StringVar(&noteListCreatedAfter, "created-after", "", "按创建日期下限过滤，格式 YYYY-MM-DD 或 RFC3339")
	viewSaveCmd.Flags().StringVar(&noteListUpdatedBefore, "updated-before", "", "按更新日期上限过滤，格式 YYYY-MM-DD 或 RFC3339")
	viewSaveCmd.Flags().StringVar(&noteListSort, "sort", "", "排序：updated、path、title")
	viewSaveCmd.Flags().IntVar(&noteLimit, "limit", 0, "限制返回数量")
	viewCmd.AddCommand(viewSaveCmd)
	viewCmd.AddCommand(&cobra.Command{Use: "list", Short: "列出保存视图", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.ListViews(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	viewCmd.AddCommand(&cobra.Command{Use: "show <name>", Short: "按保存视图检索笔记", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "view.show", "argument_required", "view show 需要名称", "pinax view show <name> --vault <vault>")
		}
		projection, err := svc.ShowView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	viewDeleteCmd := &cobra.Command{Use: "delete <name>", Short: "删除保存视图", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "view.delete", "argument_required", "view delete 需要名称", "pinax view delete <name> --vault <vault> --yes")
		}
		projection, err := svc.DeleteView(cmd.Context(), app.ViewRequest{VaultPath: vaultPath, Name: args[0], Yes: yes})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	viewDeleteCmd.Flags().BoolVar(&yes, "yes", false, "确认删除保存视图")
	viewCmd.AddCommand(viewDeleteCmd)
	cmd.AddCommand(viewCmd)

	noteCmd := &cobra.Command{Use: "note", Short: "管理本地 Markdown 笔记"}
	noteCreateRun := func(commandName string) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.new", "argument_required", commandName+" 需要一个标题", "pinax note "+commandName+" <title> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.new", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			stdinBody := ""
			if noteUseStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.new", "stdin_read_failed", err.Error(), "检查 stdin 输入后重试")
				}
				stdinBody = string(b)
			}
			project := noteProject
			if project == "" {
				project = noteGroup
			}
			projection, err := svc.CreateNote(cmd.Context(), app.CreateNoteRequest{VaultPath: vaultPath, Title: args[0], Project: project, Folder: noteFolder, Kind: noteKind, Tags: splitCSV(noteTags), Template: noteTemplate, Vars: vars, Body: noteBody, SourcePath: noteFrom, StdinBody: stdinBody, Dir: noteDir, Slug: noteSlug, Status: noteStatus, DryRun: noteDryRun})
			if err != nil {
				return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
			}
			if noteOpen && !noteDryRun {
				if path := projection.Facts["path"]; path != "" {
					editProjection, editErr := svc.EditNote(cmd.Context(), app.NoteEditRequest{VaultPath: vaultPath, NoteRef: path, Editor: noteEditor})
					if editErr != nil {
						return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), editProjection, editErr)
					}
					projection.Facts["opened"] = "true"
					for _, key := range []string{"editor", "editor_executable", "editor_args"} {
						if value := editProjection.Facts[key]; value != "" {
							projection.Facts[key] = value
						}
					}
				}
			}
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, nil)
		}
	}
	addCreateFlags := func(c *cobra.Command) {
		c.Flags().StringVar(&noteProject, "project", "", "项目 slug")
		c.Flags().StringVar(&noteGroup, "group", "", "分组 slug，等价于未设置 --project 时的项目")
		c.Flags().StringVar(&noteFolder, "folder", "", "项目或 notes 下的相对文件夹")
		c.Flags().StringVar(&noteKind, "kind", "", "笔记用途分类，例如 fleeting、reference、project、daily")
		c.Flags().StringVar(&noteTags, "tags", "", "逗号分隔标签")
		c.Flags().StringVar(&noteTemplate, "template", "", "模板名称")
		c.Flags().StringArrayVar(&templateVars, "var", nil, "模板变量，格式 key=value，可重复")
		c.Flags().StringVar(&noteBody, "body", "", "笔记正文")
		c.Flags().StringVar(&noteFrom, "from", "", "从 Markdown 文件读取正文")
		c.Flags().BoolVar(&noteUseStdin, "stdin", false, "从 stdin 读取正文")
		c.Flags().StringVar(&noteDir, "dir", "", "notes/ 下的目标目录")
		c.Flags().StringVar(&noteSlug, "slug", "", "文件名 slug")
		c.Flags().StringVar(&noteStatus, "status", "", "frontmatter status")
		c.Flags().BoolVar(&noteDryRun, "dry-run", false, "只预览计划，不写文件")
		c.Flags().BoolVar(&noteOpen, "open", false, "创建后用编辑器打开")
		c.Flags().StringVar(&noteEditor, "editor", "", "编辑器命令，默认读取 EDITOR")
	}
	noteNewCmd := &cobra.Command{Use: "new <title>", Short: "创建一篇本地 Markdown 笔记", Example: "pinax note new \"研究日志\" --body 正文 --tags pinax --vault ./my-notes", RunE: noteCreateRun("new")}
	addCreateFlags(noteNewCmd)
	noteCreateCmd := &cobra.Command{Use: "create <title>", Short: "创建一篇本地 Markdown 笔记", Example: "pinax note create \"会议\" --stdin --vault ./my-notes", RunE: noteCreateRun("create")}
	addCreateFlags(noteCreateCmd)
	noteCmd.AddCommand(noteNewCmd, noteCreateCmd)

	noteListCmd := &cobra.Command{Use: "list", Short: "列出本地笔记", RunE: func(cmd *cobra.Command, args []string) error {
		group := noteGroup
		if group == "" {
			group = noteListProject
		}
		projection, err := svc.ListNotesQuery(cmd.Context(), app.NoteListRequest{VaultPath: vaultPath, Tags: splitCSV(noteListTag), Project: noteListProject, Group: group, Folder: noteFolder, Kind: noteKind, Status: noteListStatus, CreatedAfter: noteListCreatedAfter, UpdatedBefore: noteListUpdatedBefore, Recent: noteRecent, Limit: noteLimit, Sort: noteListSort, PathPrefix: noteListPathPrefix})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	noteListCmd.Flags().StringVar(&noteListTag, "tag", "", "按标签过滤")
	noteListCmd.Flags().StringVar(&noteListProject, "project", "", "按项目过滤")
	noteListCmd.Flags().StringVar(&noteGroup, "group", "", "按分组过滤")
	noteListCmd.Flags().StringVar(&noteFolder, "folder", "", "按文件夹过滤")
	noteListCmd.Flags().StringVar(&noteKind, "kind", "", "按用途分类过滤")
	noteListCmd.Flags().StringVar(&noteListStatus, "status", "", "按状态过滤")
	noteListCmd.Flags().StringVar(&noteListCreatedAfter, "created-after", "", "按创建日期下限过滤，格式 YYYY-MM-DD 或 RFC3339")
	noteListCmd.Flags().StringVar(&noteListUpdatedBefore, "updated-before", "", "按更新日期上限过滤，格式 YYYY-MM-DD 或 RFC3339")
	noteListCmd.Flags().BoolVar(&noteRecent, "recent", false, "按最近更新排序")
	noteListCmd.Flags().IntVar(&noteLimit, "limit", 0, "限制返回数量")
	noteListCmd.Flags().StringVar(&noteListSort, "sort", "", "排序：updated、path、title")
	noteListCmd.Flags().StringVar(&noteListPathPrefix, "path-prefix", "", "按路径前缀过滤")
	noteCmd.AddCommand(noteListCmd)

	noteShowRun := func(command string) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), command, "argument_required", "需要一个 note 路径、标题或 note_id", "pinax note show <note> --vault <vault>")
			}
			projection, err := svc.ShowNoteProjection(cmd.Context(), app.ShowNoteRequest{VaultPath: vaultPath, NoteRef: args[0]})
			projection.Command = command
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		}
	}
	noteCmd.AddCommand(&cobra.Command{Use: "show <note>", Short: "读取一篇本地笔记", Example: "pinax note show note_01 --vault ./my-notes\npinax note show \"Inbox Note\" --vault ./my-notes", RunE: noteShowRun("note.show")})
	noteCmd.AddCommand(&cobra.Command{Use: "read <note>", Short: "读取一篇本地笔记", RunE: noteShowRun("note.show")})
	noteCmd.AddCommand(&cobra.Command{Use: "links <note>", Short: "列出笔记出链", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.links", "argument_required", "note links 需要一个 note 引用", "pinax note links <note> --vault <vault>")
		}
		projection, err := svc.NoteLinks(cmd.Context(), app.NoteLinkRequest{VaultPath: vaultPath, NoteRef: args[0]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteCmd.AddCommand(&cobra.Command{Use: "backlinks <note>", Short: "列出笔记反链", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.backlinks", "argument_required", "note backlinks 需要一个 note 引用", "pinax note backlinks <note> --vault <vault>")
		}
		projection, err := svc.NoteBacklinks(cmd.Context(), app.NoteLinkRequest{VaultPath: vaultPath, NoteRef: args[0]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteCmd.AddCommand(&cobra.Command{Use: "orphans", Short: "列出没有入边和出边的笔记", RunE: func(cmd *cobra.Command, args []string) error {
		projection, err := svc.NoteOrphans(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteCmd.AddCommand(&cobra.Command{Use: "attach <note> <file>", Short: "复制文件到 vault 并追加附件引用", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.attach", "argument_required", "note attach 需要 note 和源文件", "pinax note attach <note> <file> --vault <vault>")
		}
		projection, err := svc.AttachNoteFile(cmd.Context(), app.NoteAttachRequest{VaultPath: vaultPath, NoteRef: args[0], SourcePath: args[1]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteCmd.AddCommand(&cobra.Command{Use: "attachments <note>", Short: "列出笔记附件引用", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.attachments", "argument_required", "note attachments 需要一个 note 引用", "pinax note attachments <note> --vault <vault>")
		}
		projection, err := svc.NoteAttachments(cmd.Context(), app.NoteLinkRequest{VaultPath: vaultPath, NoteRef: args[0]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})

	noteEditRun := func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.edit", "argument_required", "note edit 需要一个 note 引用", "pinax note edit <note> --vault <vault>")
		}
		projection, err := svc.EditNote(cmd.Context(), app.NoteEditRequest{VaultPath: vaultPath, NoteRef: args[0], Editor: noteEditor})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}
	noteEditCmd := &cobra.Command{Use: "edit <note>", Short: "用编辑器打开笔记", RunE: noteEditRun}
	noteEditCmd.Flags().StringVar(&noteEditor, "editor", "", "编辑器命令，默认读取 EDITOR")
	noteOpenCmd := &cobra.Command{Use: "open <note>", Short: "用编辑器打开笔记", RunE: noteEditRun}
	noteOpenCmd.Flags().StringVar(&noteEditor, "editor", "", "编辑器命令，默认读取 EDITOR")
	noteCmd.AddCommand(noteEditCmd, noteOpenCmd)

	noteCmd.AddCommand(&cobra.Command{Use: "rename <note> <title>", Short: "重命名笔记", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.rename", "argument_required", "note rename 需要 note 和新标题", "pinax note rename <note> <title> --vault <vault>")
		}
		projection, err := svc.RenameNote(cmd.Context(), app.NoteMutationRequest{VaultPath: vaultPath, NoteRef: args[0], Title: args[1]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteCmd.AddCommand(&cobra.Command{Use: "move <note> <dir>", Short: "移动笔记到 notes/ 下目录", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.move", "argument_required", "note move 需要 note 和目录", "pinax note move <note> <dir> --vault <vault>")
		}
		projection, err := svc.MoveNote(cmd.Context(), app.NoteMutationRequest{VaultPath: vaultPath, NoteRef: args[0], TargetDir: args[1]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteCmd.AddCommand(&cobra.Command{Use: "archive <note>", Short: "归档笔记", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.archive", "argument_required", "note archive 需要一个 note", "pinax note archive <note> --vault <vault>")
		}
		projection, err := svc.ArchiveNote(cmd.Context(), app.NoteMutationRequest{VaultPath: vaultPath, NoteRef: args[0]})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}})
	noteDeleteCmd := &cobra.Command{Use: "delete <note>", Short: "删除或移入回收站", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.delete", "argument_required", "note delete 需要一个 note", "pinax note delete <note> --vault <vault> --yes")
		}
		projection, err := svc.DeleteNote(cmd.Context(), app.NoteDeleteRequest{VaultPath: vaultPath, NoteRef: args[0], Yes: yes, Hard: noteHard})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	noteDeleteCmd.Flags().BoolVar(&yes, "yes", false, "确认删除或移入回收站")
	noteDeleteCmd.Flags().BoolVar(&noteHard, "hard", false, "真实删除文件，必须同时提供 --yes")
	noteCmd.AddCommand(noteDeleteCmd)

	noteTagCmd := &cobra.Command{Use: "tag", Short: "管理笔记标签"}
	for _, op := range []string{"add", "remove", "set"} {
		operation := op
		noteTagCmd.AddCommand(&cobra.Command{Use: operation + " <note> <tag>...", Short: "更新笔记标签", RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.tag", "argument_required", "note tag 需要 note 和至少一个 tag", "pinax note tag "+operation+" <note> <tag> --vault <vault>")
			}
			projection, err := svc.TagNote(cmd.Context(), app.NoteTagRequest{VaultPath: vaultPath, NoteRef: args[0], Operation: operation, Tags: args[1:]})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		}})
	}
	noteCmd.AddCommand(noteTagCmd)
	cmd.AddCommand(noteCmd)

	searchCmd := &cobra.Command{
		Use:     "search <query>",
		Short:   "搜索本地笔记",
		Example: "pinax search \"项目复盘\" --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "note.search", "argument_required", "search 需要一个查询词", "pinax search <query> --vault <vault>")
			}
			projection, err := svc.SearchProjection(cmd.Context(), app.SearchRequest{VaultPath: vaultPath, Query: args[0], Tags: splitCSV(noteTags), Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus, CreatedAfter: searchCreatedAfter, UpdatedAfter: searchUpdatedAfter, LinkTarget: searchLinkTarget, HasAttachment: searchHasAttachment, Limit: noteLimit, Sort: noteListSort, AllowStale: searchAllowStale})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	searchCmd.Flags().StringVar(&noteTags, "tag", "", "按标签过滤")
	searchCmd.Flags().StringVar(&noteGroup, "group", "", "按分组过滤")
	searchCmd.Flags().StringVar(&noteFolder, "folder", "", "按文件夹过滤")
	searchCmd.Flags().StringVar(&noteKind, "kind", "", "按用途分类过滤")
	searchCmd.Flags().StringVar(&noteStatus, "status", "", "按状态过滤")
	searchCmd.Flags().StringVar(&searchCreatedAfter, "created-after", "", "按创建日期下限过滤，格式 YYYY-MM-DD 或 RFC3339")
	searchCmd.Flags().StringVar(&searchUpdatedAfter, "updated-after", "", "按更新日期下限过滤，格式 YYYY-MM-DD 或 RFC3339")
	searchCmd.Flags().StringVar(&searchLinkTarget, "link-target", "", "按链接目标过滤")
	searchCmd.Flags().BoolVar(&searchHasAttachment, "has-attachment", false, "只返回包含附件引用的笔记")
	searchCmd.Flags().BoolVar(&searchAllowStale, "allow-stale", false, "允许使用 stale index 返回 partial 结果")
	searchCmd.Flags().StringVar(&noteListSort, "sort", "", "排序：relevance、updated、created、title、path")
	searchCmd.Flags().IntVar(&noteLimit, "limit", 0, "限制返回数量")
	cmd.AddCommand(searchCmd)

	importCmd := &cobra.Command{Use: "import", Short: "导入本地 Markdown 内容"}
	importMarkdownCmd := &cobra.Command{Use: "markdown <source>", Short: "导入本地 Markdown 文件或目录", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "import.markdown", "argument_required", "import markdown 需要源文件或目录", "pinax import markdown <source> --vault <vault>")
		}
		projection, err := svc.ImportMarkdown(cmd.Context(), app.ImportMarkdownRequest{VaultPath: vaultPath, Source: args[0], Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus, Tags: splitCSV(noteTags), Conflict: importConflict, DryRun: importDryRun, Yes: yes})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	importMarkdownCmd.Flags().StringVar(&noteGroup, "group", "", "导入目标分组")
	importMarkdownCmd.Flags().StringVar(&noteFolder, "folder", "", "导入目标文件夹")
	importMarkdownCmd.Flags().StringVar(&noteKind, "kind", "", "导入笔记用途分类")
	importMarkdownCmd.Flags().StringVar(&noteStatus, "status", "", "导入笔记状态")
	importMarkdownCmd.Flags().StringVar(&noteTags, "tags", "", "导入笔记标签，逗号分隔")
	importMarkdownCmd.Flags().StringVar(&importConflict, "conflict", "skip", "冲突策略：skip、rename、overwrite")
	importMarkdownCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "只输出导入计划，不写 vault")
	importMarkdownCmd.Flags().BoolVar(&yes, "yes", false, "确认执行导入写入")
	importCmd.AddCommand(importMarkdownCmd)
	cmd.AddCommand(importCmd)

	exportCmd := &cobra.Command{Use: "export", Short: "导出本地 Markdown 内容"}
	exportMarkdownCmd := &cobra.Command{Use: "markdown <output-dir>", Short: "导出 Markdown bundle", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "export.markdown", "argument_required", "export markdown 需要输出目录", "pinax export markdown <output-dir> --vault <vault>")
		}
		projection, err := svc.ExportMarkdown(cmd.Context(), app.ExportMarkdownRequest{VaultPath: vaultPath, OutputDir: args[0], Tags: splitCSV(noteListTag), Group: noteGroup, Folder: noteFolder, Kind: noteKind, Status: noteStatus})
		return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
	}}
	exportMarkdownCmd.Flags().StringVar(&noteListTag, "tag", "", "按标签过滤")
	exportMarkdownCmd.Flags().StringVar(&noteGroup, "group", "", "按分组过滤")
	exportMarkdownCmd.Flags().StringVar(&noteFolder, "folder", "", "按文件夹过滤")
	exportMarkdownCmd.Flags().StringVar(&noteKind, "kind", "", "按用途分类过滤")
	exportMarkdownCmd.Flags().StringVar(&noteStatus, "status", "", "按状态过滤")
	exportCmd.AddCommand(exportMarkdownCmd)
	cmd.AddCommand(exportCmd)

	projectCmd := &cobra.Command{Use: "project", Short: "管理 vault 内项目"}
	projectCreateCmd := &cobra.Command{
		Use:     "create <slug>",
		Short:   "创建 vault 项目",
		Example: "pinax project create research --name \"研究\" --notes-prefix notes/research --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "project.create", "argument_required", "project create 需要一个 slug", "pinax project create <slug> --name <name> --vault <vault>")
			}
			projection, err := svc.CreateProject(cmd.Context(), app.ProjectRequest{VaultPath: vaultPath, Slug: args[0], Name: projectName, Description: projectDescription, NotesPrefix: projectNotesPrefix})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	projectCreateCmd.Flags().StringVar(&projectName, "name", "", "项目名称")
	projectCreateCmd.Flags().StringVar(&projectDescription, "description", "", "项目描述")
	projectCreateCmd.Flags().StringVar(&projectNotesPrefix, "notes-prefix", "", "项目笔记路径前缀")
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出 vault 项目",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ListProjects(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:     "switch <slug>",
		Short:   "切换当前 vault 项目",
		Example: "pinax project switch research --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "project.switch", "argument_required", "project switch 需要一个 slug", "pinax project switch <slug> --vault <vault>")
			}
			projection, err := svc.SwitchProject(cmd.Context(), app.ProjectRequest{VaultPath: vaultPath, Slug: args[0]})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	cmd.AddCommand(projectCmd)

	storageCmd := &cobra.Command{Use: "storage", Short: "配置 vault storage backend"}
	storageSetLocalCmd := &cobra.Command{
		Use:     "set-local",
		Short:   "配置本地 storage backend",
		Example: "pinax storage set-local --root ./my-notes --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.SetLocalStorage(cmd.Context(), app.StorageRequest{VaultPath: vaultPath, Root: storageRoot})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	storageSetLocalCmd.Flags().StringVar(&storageRoot, "root", "", "本地 storage 根目录")
	storageCmd.AddCommand(storageSetLocalCmd)
	storageSetS3Cmd := &cobra.Command{
		Use:     "set-s3",
		Short:   "配置 S3 storage backend",
		Long:    "配置 S3 storage backend。该命令只写入 backend profile，不连接 S3，不保存 access key 或 secret。",
		Example: "pinax storage set-s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.SetS3Storage(cmd.Context(), app.StorageRequest{VaultPath: vaultPath, Bucket: s3Bucket, Region: s3Region, Prefix: s3Prefix, Endpoint: s3Endpoint, Profile: s3Profile})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	storageSetS3Cmd.Flags().StringVar(&s3Bucket, "bucket", "", "S3 bucket 名称")
	storageSetS3Cmd.Flags().StringVar(&s3Region, "region", "", "S3 region")
	storageSetS3Cmd.Flags().StringVar(&s3Prefix, "prefix", "", "S3 object key 前缀")
	storageSetS3Cmd.Flags().StringVar(&s3Endpoint, "endpoint", "", "S3 兼容 endpoint URL")
	storageSetS3Cmd.Flags().StringVar(&s3Profile, "profile", "", "S3 credential profile 名称，不保存 secret")
	storageCmd.AddCommand(storageSetS3Cmd)
	storageCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "查看 storage backend 状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.StorageStatus(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	storageCmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "诊断 storage backend 配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.StorageDoctor(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	cmd.AddCommand(storageCmd)

	templateCmd := &cobra.Command{Use: "template", Short: "管理 Markdown 模板"}
	templateCreateCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "创建 Markdown 模板",
		Example: `pinax template create "视频学习" --vault ./my-notes
pinax template create meeting --from ./meeting.md --vault ./my-notes
pinax template create daily-review --body "# {{date}}" --vault ./my-notes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.create", "argument_required", "template create 需要一个模板名称", "pinax template create <name> --vault <vault>")
			}
			body := templateBody
			if templateUseStdin {
				b, readErr := io.ReadAll(cmd.InOrStdin())
				if readErr != nil {
					return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.create", "stdin_read_failed", readErr.Error(), "检查 stdin 输入后重试")
				}
				body = string(b)
			}
			projection, err := svc.CreateTemplate(cmd.Context(), app.TemplateRequest{VaultPath: vaultPath, Name: args[0], SourcePath: templateSourcePath, Body: body, UseStdin: templateUseStdin, Overwrite: templateOverwrite})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	templateCreateCmd.Flags().StringVar(&templateSourcePath, "from", "", "从 Markdown 文件创建模板")
	templateCreateCmd.Flags().StringVar(&templateBody, "body", "", "从命令参数创建模板正文")
	templateCreateCmd.Flags().BoolVar(&templateUseStdin, "stdin", false, "从 stdin 读取模板正文")
	templateCreateCmd.Flags().BoolVar(&templateOverwrite, "overwrite", false, "覆盖已存在模板")
	templateCmd.AddCommand(templateCreateCmd)
	templateCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "初始化内置模板",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.InitTemplates(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	templateCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出模板",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ListTemplates(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	templateCmd.AddCommand(&cobra.Command{
		Use:     "show <name>",
		Short:   "读取模板",
		Example: "pinax template show mermaid --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.show", "argument_required", "template show 需要一个模板名称", "pinax template show <name> --vault <vault>")
			}
			projection, err := svc.ShowTemplate(cmd.Context(), app.TemplateRequest{VaultPath: vaultPath, Name: args[0]})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	templateValidateCmd := &cobra.Command{
		Use:     "validate <name>",
		Short:   "校验模板",
		Example: "pinax template validate meeting --vault ./my-notes --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.validate", "argument_required", "template validate 需要一个模板名称", "pinax template validate <name> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.validate", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			projection, err := svc.ValidateTemplate(cmd.Context(), app.TemplateRequest{VaultPath: vaultPath, Name: args[0], Title: title, Project: noteProject, Tags: splitCSV(noteTags), Vars: vars})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	templateValidateCmd.Flags().StringArrayVar(&templateVars, "var", nil, "模板变量，格式 key=value，可重复")
	templateValidateCmd.Flags().StringVar(&title, "title", "", "模板标题")
	templateValidateCmd.Flags().StringVar(&noteProject, "project", "", "项目 slug")
	templateValidateCmd.Flags().StringVar(&noteTags, "tags", "", "逗号分隔标签")
	templateCmd.AddCommand(templateValidateCmd)
	templateDeleteCmd := &cobra.Command{
		Use:     "delete <name>",
		Short:   "删除自定义模板",
		Example: "pinax template delete meeting --vault ./my-notes --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.delete", "argument_required", "template delete 需要一个模板名称", "pinax template delete <name> --vault <vault> --yes")
			}
			projection, err := svc.DeleteTemplate(cmd.Context(), app.TemplateRequest{VaultPath: vaultPath, Name: args[0], Yes: yes})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	templateDeleteCmd.Flags().BoolVar(&yes, "yes", false, "确认删除模板")
	templateCmd.AddCommand(templateDeleteCmd)
	templateRenderCmd := &cobra.Command{
		Use:     "render <name>",
		Short:   "渲染模板",
		Example: "pinax template render mermaid --title \"架构\" --project research --tags pinax,sync --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.render", "argument_required", "template render 需要一个模板名称", "pinax template render <name> --title <title> --vault <vault>")
			}
			vars, parseErr := splitKeyValueVars(templateVars)
			if parseErr != nil {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "template.render", parseErr.Code, parseErr.Message, parseErr.Hint)
			}
			projection, err := svc.RenderTemplate(cmd.Context(), app.TemplateRequest{VaultPath: vaultPath, Name: args[0], Title: title, Project: noteProject, Tags: splitCSV(noteTags), Vars: vars})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	templateRenderCmd.Flags().StringVar(&title, "title", "", "模板标题")
	templateRenderCmd.Flags().StringVar(&noteProject, "project", "", "项目 slug")
	templateRenderCmd.Flags().StringVar(&noteTags, "tags", "", "逗号分隔标签")
	templateRenderCmd.Flags().StringArrayVar(&templateVars, "var", nil, "模板变量，格式 key=value，可重复")
	templateCmd.AddCommand(templateRenderCmd)
	cmd.AddCommand(templateCmd)

	indexCmd := &cobra.Command{Use: "index", Short: "管理本地 SQLite 索引"}
	indexCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "初始化本地索引数据库",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.InitIndex(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	indexCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "检查本地索引状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.IndexStatus(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	indexCmd.AddCommand(&cobra.Command{
		Use:   "rebuild",
		Short: "重建本地索引投影",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.RebuildIndex(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	cmd.AddCommand(indexCmd)

	syncCmd := &cobra.Command{Use: "sync", Short: "生成和执行同步计划"}
	syncDiffCmd := &cobra.Command{
		Use:   "diff",
		Short: "生成同步差异计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.SyncDiff(cmd.Context(), app.SyncRequest{VaultPath: vaultPath, Target: syncTarget})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	syncDiffCmd.Flags().StringVar(&syncTarget, "target", "git", "同步目标：git、s3 或 cloud")
	syncCmd.AddCommand(syncDiffCmd)
	syncPushCmd := &cobra.Command{
		Use:   "push",
		Short: "记录同步 push 状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.SyncPush(cmd.Context(), app.SyncRequest{VaultPath: vaultPath, Target: syncTarget, Yes: yes})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	syncPushCmd.Flags().StringVar(&syncTarget, "target", "git", "同步目标：git、s3 或 cloud")
	syncPushCmd.Flags().BoolVar(&yes, "yes", false, "确认执行同步状态写入")
	syncCmd.AddCommand(syncPushCmd)
	syncPullCmd := &cobra.Command{
		Use:   "pull",
		Short: "记录同步 pull 状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.SyncPull(cmd.Context(), app.SyncRequest{VaultPath: vaultPath, Target: syncTarget, Yes: yes})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	syncPullCmd.Flags().StringVar(&syncTarget, "target", "git", "同步目标：git、s3 或 cloud")
	syncPullCmd.Flags().BoolVar(&yes, "yes", false, "确认执行同步状态写入")
	syncCmd.AddCommand(syncPullCmd)
	cmd.AddCommand(syncCmd)

	metadataCmd := &cobra.Command{Use: "metadata", Short: "规划和应用笔记 metadata"}
	metadataCmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "预览 metadata 补齐计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanMetadata(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	metadataApplyCmd := &cobra.Command{
		Use:     "apply",
		Short:   "应用 metadata 补齐计划",
		Long:    "应用 metadata 补齐计划。该命令会写入本地 Markdown frontmatter，必须显式提供 --yes。先运行 pinax metadata plan 审核计划。",
		Example: "pinax metadata plan --vault ./my-notes --json\npinax metadata apply --vault ./my-notes --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ApplyMetadata(cmd.Context(), app.ApplyRequest{VaultPath: vaultPath, Yes: yes})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	metadataApplyCmd.Flags().BoolVar(&yes, "yes", false, "确认执行本地写入")
	metadataCmd.AddCommand(metadataApplyCmd)
	cmd.AddCommand(metadataCmd)

	repairCmd := &cobra.Command{Use: "repair", Short: "规划和应用 vault 维护动作"}
	repairPlanCmd := &cobra.Command{
		Use:     "plan",
		Short:   "从 doctor issue 生成维护计划",
		Long:    "从 vault doctor issue 生成可审查的 repair plan。默认只输出计划，不写 Markdown 或 .pinax 资产；提供 --save 时通过 service 写入 .pinax/repair-plans/<plan_id>.json。",
		Example: "pinax repair plan --vault ./my-notes --json\npinax repair plan --vault ./my-notes --save --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanRepair(cmd.Context(), app.RepairPlanRequest{VaultPath: vaultPath, Save: repairSave})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	repairPlanCmd.Flags().BoolVar(&repairSave, "save", false, "保存 repair plan 到 .pinax/repair-plans")
	repairCmd.AddCommand(repairPlanCmd)
	repairApplyCmd := &cobra.Command{
		Use:     "apply",
		Short:   "应用受保护的低风险 repair 计划",
		Long:    "应用已保存的 repair plan。该命令会写入本地 vault，必须显式提供 --yes，并且需要 Git snapshot 保护，或通过 --snapshot-message 先创建 snapshot。",
		Example: "pinax repair plan --vault ./my-notes --save --json\npinax git snapshot --vault ./my-notes --message \"repair 前快照\"\npinax repair apply --vault ./my-notes --plan repair-abc123 --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ApplyRepair(cmd.Context(), app.RepairApplyRequest{VaultPath: vaultPath, PlanID: repairPlanID, Yes: yes, SnapshotMessage: snapshotMessage})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	repairApplyCmd.Flags().StringVar(&repairPlanID, "plan", "", "repair plan id 或 .pinax/repair-plans 内的相对路径")
	repairApplyCmd.Flags().BoolVar(&yes, "yes", false, "确认执行本地写入")
	repairApplyCmd.Flags().StringVar(&snapshotMessage, "snapshot-message", "", "apply 前自动创建 Git snapshot 的消息")
	repairCmd.AddCommand(repairApplyCmd)
	cmd.AddCommand(repairCmd)

	organizeCmd := &cobra.Command{Use: "organize", Short: "规划和应用笔记结构整理"}
	organizeSuggestCmd := &cobra.Command{
		Use:     "suggest",
		Short:   "生成 agent 可审查的整理建议计划",
		Long:    "生成 agent 可审查的整理建议计划。默认只输出计划，不写 Markdown 或 .pinax 资产；提供 --save 时通过 service 写入 .pinax/organize-plans/<plan_id>.json。",
		Example: "pinax organize suggest --vault ./my-notes --json\npinax organize suggest --vault ./my-notes --save --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.SuggestOrganize(cmd.Context(), app.OrganizeSuggestRequest{VaultPath: vaultPath, Save: organizeSave})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	organizeSuggestCmd.Flags().BoolVar(&organizeSave, "save", false, "保存 organize plan 到 .pinax/organize-plans")
	organizeCmd.AddCommand(organizeSuggestCmd)
	organizeCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出已保存的 organize plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ListOrganizePlans(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	organizeCmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "预览结构整理计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanOrganize(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	organizeApplyCmd := &cobra.Command{
		Use:     "apply",
		Short:   "应用结构整理计划",
		Long:    "应用结构整理计划。该命令会移动本地笔记文件，必须显式提供 --yes，并且需要先运行 pinax organize suggest --save 生成并审核计划。落地前必须存在 Git snapshot，或通过 --snapshot-message 让 Pinax 先创建 Git snapshot。",
		Example: "pinax organize suggest --vault ./my-notes --save --json\npinax git snapshot --vault ./my-notes --message \"整理前快照\"\npinax organize apply --vault ./my-notes --plan organize-abc123 --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ApplyOrganize(cmd.Context(), app.ApplyRequest{VaultPath: vaultPath, PlanID: repairPlanID, Yes: yes, SnapshotMessage: snapshotMessage})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	organizeApplyCmd.Flags().StringVar(&repairPlanID, "plan", "", "organize plan id 或 .pinax/organize-plans 内的相对路径")
	organizeApplyCmd.Flags().BoolVar(&yes, "yes", false, "确认执行本地写入")
	organizeApplyCmd.Flags().StringVar(&snapshotMessage, "snapshot-message", "", "apply 前自动创建 Git snapshot 的消息")
	organizeCmd.AddCommand(organizeApplyCmd)
	cmd.AddCommand(organizeCmd)

	gitCmd := &cobra.Command{Use: "git", Short: "管理本地 Git 保护操作"}
	gitSnapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "创建整理前 Git snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.GitSnapshot(cmd.Context(), app.SnapshotRequest{VaultPath: vaultPath, Message: snapshotMessage})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	gitSnapshotCmd.Flags().StringVar(&snapshotMessage, "message", "", "Git snapshot 提交消息")
	gitCmd.AddCommand(gitSnapshotCmd)
	cmd.AddCommand(gitCmd)

	planCmd := &cobra.Command{Use: "plan", Short: "管理个人计划工作流"}
	planDailyCmd := &cobra.Command{
		Use:   "daily",
		Short: "生成每日计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanDaily(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, WithTaskBridge: planWithTaskBridge, DryRun: planDryRun, Yes: yes, Save: planSave})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	planDailyCmd.Flags().BoolVar(&planWithTaskBridge, "taskbridge", false, "从 TaskBridge 读取任务事实")
	planDailyCmd.Flags().BoolVar(&planDryRun, "dry-run", false, "只预览计划，不写入")
	planDailyCmd.Flags().BoolVar(&planSave, "save", false, "保存计划快照")
	planDailyCmd.Flags().BoolVar(&yes, "yes", false, "确认写入计划")
	planCmd.AddCommand(planDailyCmd)
	planWeeklyCmd := &cobra.Command{
		Use:   "weekly",
		Short: "生成每周计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanWeekly(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, WithTaskBridge: planWithTaskBridge, DryRun: planDryRun, Yes: yes, Save: planSave})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	planWeeklyCmd.Flags().BoolVar(&planWithTaskBridge, "taskbridge", false, "从 TaskBridge 读取任务事实")
	planWeeklyCmd.Flags().BoolVar(&planDryRun, "dry-run", false, "只预览计划，不写入")
	planWeeklyCmd.Flags().BoolVar(&planSave, "save", false, "保存计划快照")
	planWeeklyCmd.Flags().BoolVar(&yes, "yes", false, "确认写入计划")
	planCmd.AddCommand(planWeeklyCmd)
	planMonthlyCmd := &cobra.Command{
		Use:   "monthly",
		Short: "生成每月计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanMonthly(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, WithTaskBridge: planWithTaskBridge, DryRun: planDryRun, Yes: yes, Save: planSave})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	planMonthlyCmd.Flags().BoolVar(&planWithTaskBridge, "taskbridge", false, "从 TaskBridge 读取任务事实")
	planMonthlyCmd.Flags().BoolVar(&planDryRun, "dry-run", false, "只预览计划，不写入")
	planMonthlyCmd.Flags().BoolVar(&planSave, "save", false, "保存计划快照")
	planMonthlyCmd.Flags().BoolVar(&yes, "yes", false, "确认写入计划")
	planCmd.AddCommand(planMonthlyCmd)
	planActionsCmd := &cobra.Command{
		Use:   "actions",
		Short: "生成 TaskBridge action 草稿",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanActions(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath, FromPeriod: planFromPeriod, Save: planSave})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	planActionsCmd.Flags().StringVar(&planFromPeriod, "from", "daily", "来源计划期间：daily、weekly")
	planActionsCmd.Flags().BoolVar(&planSave, "save", false, "保存 action 草稿")
	planCmd.AddCommand(planActionsCmd)
	planSnapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "生成计划快照",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.PlanSnapshot(cmd.Context(), app.PlanningRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	planCmd.AddCommand(planSnapshotCmd)
	cmd.AddCommand(planCmd)

	backendCmd := &cobra.Command{Use: "backend", Short: "管理 vault 后端 provider"}
	backendCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出 vault 所有 backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.ListBackends(cmd.Context(), app.VaultRequest{VaultPath: vaultPath})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	})
	backendAddCmd := &cobra.Command{
		Use:     "add <kind>",
		Short:   "添加 backend profile",
		Example: "pinax backend add s3 --name work-s3 --bucket notes --region us-east-1 --vault ./my-notes\npinax backend add rclone --name work-drive --remote workdrive:pinax --vault ./my-notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return renderCommandError(cmd, selectedMode(jsonMode, agentMode, eventsMode, explainMode), "backend.add", "argument_required", "backend add 需要 backend 类型", "pinax backend add <kind> --name <name> --vault <vault>")
			}
			projection, err := svc.AddBackend(cmd.Context(), app.BackendAddRequest{VaultPath: vaultPath, Name: backendName, Kind: args[0], Root: backendRoot, Bucket: s3Bucket, Region: s3Region, Prefix: s3Prefix, Endpoint: s3Endpoint, Profile: s3Profile, Remote: backendRemote})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendAddCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendAddCmd.Flags().StringVar(&backendRoot, "root", "", "本地 backend 根目录")
	backendAddCmd.Flags().StringVar(&s3Bucket, "bucket", "", "S3 bucket 名称")
	backendAddCmd.Flags().StringVar(&s3Region, "region", "", "S3 region")
	backendAddCmd.Flags().StringVar(&s3Prefix, "prefix", "", "S3 object key 前缀")
	backendAddCmd.Flags().StringVar(&s3Endpoint, "endpoint", "", "S3 兼容 endpoint URL")
	backendAddCmd.Flags().StringVar(&s3Profile, "profile", "", "S3 credential profile 名称")
	backendAddCmd.Flags().StringVar(&backendRemote, "remote", "", "rclone remote 路径")
	backendCmd.AddCommand(backendAddCmd)
	backendStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看 backend 状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.BackendStatus(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: backendName})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendStatusCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendCmd.AddCommand(backendStatusCmd)
	backendDoctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "诊断 backend 配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.BackendDoctor(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: backendName})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendDoctorCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendCmd.AddCommand(backendDoctorCmd)
	backendCapabilitiesCmd := &cobra.Command{
		Use:   "capabilities",
		Short: "查看 backend 能力",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.BackendCapabilities(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: backendName})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendCapabilitiesCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendCmd.AddCommand(backendCapabilitiesCmd)
	backendDiffCmd := &cobra.Command{
		Use:   "diff",
		Short: "生成 backend dry-run 同步计划",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.BackendDiff(cmd.Context(), app.BackendPlanRequest{VaultPath: vaultPath, Name: backendName})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendDiffCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendCmd.AddCommand(backendDiffCmd)
	backendPushCmd := &cobra.Command{
		Use:   "push",
		Short: "执行 backend push 同步",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.BackendPush(cmd.Context(), app.BackendPlanRequest{VaultPath: vaultPath, Name: backendName, DryRun: backendDryRun, Yes: yes})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendPushCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendPushCmd.Flags().BoolVar(&backendDryRun, "dry-run", false, "只预览计划，不写入")
	backendPushCmd.Flags().BoolVar(&yes, "yes", false, "确认执行写入")
	backendCmd.AddCommand(backendPushCmd)
	backendPullCmd := &cobra.Command{
		Use:   "pull",
		Short: "执行 backend pull 同步",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.BackendPull(cmd.Context(), app.BackendPlanRequest{VaultPath: vaultPath, Name: backendName, DryRun: backendDryRun, Yes: yes})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendPullCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendPullCmd.Flags().BoolVar(&backendDryRun, "dry-run", false, "只预览计划，不写入")
	backendPullCmd.Flags().BoolVar(&yes, "yes", false, "确认执行写入")
	backendCmd.AddCommand(backendPullCmd)
	backendRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "移除 backend profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := svc.RemoveBackend(cmd.Context(), app.BackendRequest{VaultPath: vaultPath, Name: backendName})
			return renderProjection(cmd.OutOrStdout(), selectedMode(jsonMode, agentMode, eventsMode, explainMode), projection, err)
		},
	}
	backendRemoveCmd.Flags().StringVar(&backendName, "name", "", "backend 名称")
	backendCmd.AddCommand(backendRemoveCmd)
	cmd.AddCommand(backendCmd)

	mcpCmd := &cobra.Command{Use: "mcp", Short: "启动 Pinax MCP surface"}
	mcpCmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "通过 stdio 启动只读 MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.Serve(context.Background(), svc, vaultPath, os.Stdin, cmd.OutOrStdout())
		},
	})
	cmd.AddCommand(mcpCmd)

	applyHelpTemplate(cmd)
	return cmd
}

func applyHelpTemplate(cmd *cobra.Command) {
	cmd.SetHelpTemplate(pinaxHelpTemplate)
	for _, child := range cmd.Commands() {
		applyHelpTemplate(child)
	}
}

func parseDurationDays(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 90 * 24 * time.Hour, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(value, "d") + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil
	}
	return time.ParseDuration(value)
}

func selectedMode(jsonMode, agentMode, eventsMode, explainMode bool) output.Mode {
	switch {
	case jsonMode:
		return output.ModeJSON
	case agentMode:
		return output.ModeAgent
	case eventsMode:
		return output.ModeEvents
	case explainMode:
		return output.ModeExplain
	default:
		return output.ModeSummary
	}
}

func validateOutputMode(cmd *cobra.Command, jsonMode, agentMode, eventsMode, explainMode bool) error {
	selected := 0
	for _, enabled := range []bool{jsonMode, agentMode, eventsMode, explainMode} {
		if enabled {
			selected++
		}
	}
	if selected <= 1 {
		return nil
	}
	errMode := selectedMode(jsonMode, agentMode, eventsMode, explainMode)
	return renderCommandError(cmd, errMode, "cli.output_mode", "output_mode_conflict", "一次只能选择一个输出模式", "只保留一个输出模式：--json、--agent、--events 或 --explain")
}

func renderCommandError(cmd *cobra.Command, mode output.Mode, command, code, message, hint string) error {
	err := &domain.CommandError{Code: code, Message: message, Hint: hint}
	projection := domain.NewErrorProjection(command, err)
	projection.Actions = []domain.Action{{Name: "help", Command: hint}}
	return renderProjection(cmd.OutOrStdout(), mode, projection, err)
}

func splitKeyValueVars(values []string) (map[string]string, *domain.CommandError) {
	vars := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, &domain.CommandError{Code: "template_variable_invalid", Message: "模板变量必须是 key=value", Hint: "使用 --var client=Acme"}
		}
		vars[key] = val
	}
	return vars, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func renderProjection(w io.Writer, mode output.Mode, projection domain.Projection, err error) error {
	if renderErr := output.Render(w, mode, projection); renderErr != nil {
		return renderErr
	}
	return err
}

func renderJournalProjection(cmd *cobra.Command, mode output.Mode, projection domain.Projection, err error, period string, load func(date string) (domain.Projection, error)) error {
	if err != nil || mode != output.ModeSummary || !isTerminalIO(cmd) {
		return renderProjection(cmd.OutOrStdout(), mode, projection, err)
	}
	loader := func(direction int, current domain.Projection) (domain.Projection, error) {
		date, dateErr := shiftedJournalDate(period, current.Facts["date"], direction)
		if dateErr != nil {
			return domain.Projection{}, dateErr
		}
		return load(date)
	}
	return output.RunJournalPager(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), projection, loader)
}

func isTerminalIO(cmd *cobra.Command) bool {
	in, inOK := cmd.InOrStdin().(*os.File)
	out, outOK := cmd.OutOrStdout().(*os.File)
	return inOK && outOK && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

func shiftedJournalDate(period, key string, direction int) (string, error) {
	switch period {
	case "weekly":
		date, err := isoWeekDate(key)
		if err != nil {
			return "", err
		}
		return date.AddDate(0, 0, direction*7).Format("2006-01-02"), nil
	case "monthly":
		date, err := time.Parse("2006-01", key)
		if err != nil {
			return "", err
		}
		return date.AddDate(0, direction, 0).Format("2006-01-02"), nil
	default:
		date, err := time.Parse("2006-01-02", key)
		if err != nil {
			return "", err
		}
		return date.AddDate(0, 0, direction).Format("2006-01-02"), nil
	}
}

func isoWeekDate(key string) (time.Time, error) {
	var year int
	var week int
	if _, err := fmt.Sscanf(key, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, err
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()+6)%7)
	return monday.AddDate(0, 0, (week-1)*7), nil
}

func journalDateCompletion(period string, vaultPathValue func() string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		items, err := existingJournalDateCompletions(vaultPathValue(), period)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if toComplete == "" {
			return items, cobra.ShellCompDirectiveNoFileComp
		}
		filtered := make([]string, 0, len(items))
		for _, item := range items {
			value, _, _ := strings.Cut(item, "\t")
			if strings.HasPrefix(value, toComplete) {
				filtered = append(filtered, item)
			}
		}
		return filtered, cobra.ShellCompDirectiveNoFileComp
	}
}

func existingJournalDateCompletions(vaultPath, period string) ([]string, error) {
	root := strings.TrimSpace(vaultPath)
	if root == "" {
		root = "."
	}
	dir := filepath.Join(root, "notes", period)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".md")
		if !validJournalKey(period, key) || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, journalCompletionItem(period, key))
	}
	return items, nil
}

func validJournalKey(period, key string) bool {
	switch period {
	case "weekly":
		_, err := isoWeekDate(key)
		return err == nil
	case "monthly":
		_, err := time.Parse("2006-01", key)
		return err == nil
	default:
		_, err := time.Parse("2006-01-02", key)
		return err == nil
	}
}

func journalCompletionItem(period, key string) string {
	switch period {
	case "weekly":
		start, _ := isoWeekDate(key)
		_, week := start.ISOWeek()
		end := start.AddDate(0, 0, 6)
		return fmt.Sprintf("%s\tweek%d(%s--%s)", key, week, start.Format("2006-01-02"), end.Format("2006-01-02"))
	case "monthly":
		start, _ := time.Parse("2006-01", key)
		end := start.AddDate(0, 1, -1)
		return fmt.Sprintf("%s\t%s--%s", key, start.Format("2006-01-02"), end.Format("2006-01-02"))
	default:
		return key + "\t" + key
	}
}
