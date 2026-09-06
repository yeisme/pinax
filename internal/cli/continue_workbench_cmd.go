package cli

import (
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// addContinueWorkbenchSubcommand 注册 additive experimental
// `pinax continue workbench`——Workbench BFF 消费的 typed projection facade
// （pinax-workbench-continuity-projection-v1）。既有 continue 命令不变。
func addContinueWorkbenchSubcommand(parent *cobra.Command, ctx commandBuildContext) {
	var packetOnly bool
	var ttlSeconds int

	cmd := &cobra.Command{
		Use:   "workbench [projectRef]",
		Short: "Experimental: typed continuity projection facade for Workbench BFF",
		Long: `Return the frozen pinax.workbench.continuity_projection.v1 envelope for an
opaque projectRef (= binding_id), or emit the provider packet with --packet.

The facade resolves bindings exact-by-id only; missing/disabled/invalid states
return the envelope with a stable recovery action instead of guessing scope.
Projection freshness is derived from evidence observation time (refresh-only).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if packetOnly {
				projection := domain.NewProjection("continue.workbench.packet", "Provider packet emitted.")
				packet := app.WorkbenchProviderPacket()
				projection.Data = map[string]any{"packet": packet}
				projection.Facts["identity"] = packet.Contract.Identity
				projection.Facts["version"] = packet.Contract.Version
				projection.Facts["digest"] = packet.Contract.Digest
				projection.Facts["actions"] = "3"
				projection.Facts["writes"] = "false"
				projection.Actions = []domain.Action{{Name: "consume", Command: "pinax continue workbench <projectRef> --json"}}
				return ctx.renderProjection(cmd, projection, nil)
			}
			projectRef := ""
			if len(args) > 0 {
				projectRef = args[0]
			}
			envelope, err := agentSvc.WorkbenchContinuityProjection(cmd.Context(), app.WorkbenchProjectionRequest{
				ProjectRef: projectRef,
				TTLSeconds: ttlSeconds,
			})
			if err != nil {
				return renderAgentError(cmd, ctx, "continue.workbench", err)
			}
			projection := domain.NewProjection("continue.workbench", "Workbench continuity projection emitted.")
			projection.Facts["project_ref"] = envelope.ProjectRef
			projection.Facts["binding_status"] = envelope.Binding.Status
			projection.Facts["ready"] = strconv.FormatBool(envelope.Binding.Ready)
			projection.Facts["digest"] = envelope.Contract.Digest
			projection.Facts["writes"] = "false"
			if envelope.Recovery != nil {
				projection.Facts["recovery_code"] = envelope.Recovery.Code
			}
			projection.Data = map[string]any{"projection": envelope}
			return ctx.renderProjection(cmd, projection, nil)
		},
	}
	cmd.Flags().BoolVar(&packetOnly, "packet", false, "Emit the pinax.provider_packet.v1 provider packet instead of a projection")
	cmd.Flags().IntVar(&ttlSeconds, "ttl", 0, "Projection TTL seconds (default 600, min 60); freshness basis stays evidence-observed time")
	parent.AddCommand(cmd)
}
