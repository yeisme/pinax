package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
)

func addTokenCommands(root *cobra.Command, ctx commandBuildContext) {
	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Manage API tokens",
	}

	tokenCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			label, _ := cmd.Flags().GetString("label")
			scopeStr, _ := cmd.Flags().GetString("scope")
			groupsStr, _ := cmd.Flags().GetString("groups")
			expiresStr, _ := cmd.Flags().GetString("expires")

			scope := parseScopeFlag(scopeStr, groupsStr)
			var expiresAt string
			if expiresStr != "" {
				d, err := parseDuration(expiresStr)
				if err != nil {
					return renderCommandError(cmd, ctx.outputMode(), "token.create", "invalid_expires", "invalid expiration: "+err.Error(), "Use a duration such as 30d, 24h, or 1h30m")
				}
				expiresAt = time.Now().UTC().Add(d).Format(time.RFC3339)
			}

			store, err := tokenStore(ctx)
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.create", "store_error", err.Error(), "Check the vault directory")
			}

			rec, secret := api.GenerateTokenRecord(label, scope, expiresAt, "manual")
			if err := store.Create(rec); err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.create", "create_error", err.Error(), "")
			}

			return renderTokenSecretResult(cmd, ctx, "token.create", "Token created.", rec, secret, "")
		},
	}
	tokenCreateCmd.Flags().String("label", "", "Token label")
	tokenCreateCmd.Flags().String("scope", "read", "Permission scope (comma-separated: read,write,admin)")
	tokenCreateCmd.Flags().String("groups", "", "Allowed route groups (comma-separated; empty means all)")
	tokenCreateCmd.Flags().String("expires", "", "Expiration (for example, 30d or 24h)")

	tokenListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all API tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := tokenStore(ctx)
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.list", "store_error", err.Error(), "Check the vault directory")
			}
			tokens, err := store.List()
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.list", "list_error", err.Error(), "")
			}
			if len(tokens) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tokens.")
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-20s %-10s %s\n", "ID", "Label", "Created", "Scope", "Expires")
			for _, t := range tokens {
				scopes := make([]string, 0, len(t.Scope))
				for s := range t.Scope {
					scopes = append(scopes, string(s))
				}
				expires := "-"
				if t.ExpiresAt != "" {
					expires = t.ExpiresAt
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-20s %-10s %s\n",
					t.ID, t.Label, t.CreatedAt, strings.Join(scopes, ","), expires)
			}
			return nil
		},
	}

	tokenRevokeCmd := &cobra.Command{
		Use:   "revoke [token-id]",
		Short: "Revoke an API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := tokenStore(ctx)
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.revoke", "store_error", err.Error(), "Check the vault directory")
			}
			if err := store.Delete(args[0]); err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.revoke", "delete_error", err.Error(), "")
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Revoked token: %s\n", args[0])
			return nil
		},
	}

	tokenRotateCmd := &cobra.Command{
		Use:   "rotate [token-id]",
		Short: "Rotate an API token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label, _ := cmd.Flags().GetString("label")

			store, err := tokenStore(ctx)
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.rotate", "store_error", err.Error(), "Check the vault directory")
			}
			old, err := store.Get(args[0])
			if err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.rotate", "not_found", err.Error(), "")
			}
			newLabel := label
			if newLabel == "" {
				newLabel = old.Label
			}
			rec, secret := api.GenerateTokenRecord(newLabel, old.Scope, old.ExpiresAt, "rotate")
			rec.RotatedFrom = old.ID
			if err := store.Create(rec); err != nil {
				return renderCommandError(cmd, ctx.outputMode(), "token.rotate", "create_error", err.Error(), "")
			}
			if err := store.Delete(old.ID); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to delete old token: %s\n", err)
			}
			return renderTokenSecretResult(cmd, ctx, "token.rotate", "Token rotated.", rec, secret, old.ID)
		},
	}
	tokenRotateCmd.Flags().String("label", "", "New token label")

	tokenCmd.AddCommand(tokenCreateCmd, tokenListCmd, tokenRevokeCmd, tokenRotateCmd)
	root.AddCommand(tokenCmd)
}

func renderTokenSecretResult(cmd *cobra.Command, ctx commandBuildContext, command, summary string, rec *api.TokenRecord, secret string, rotatedFrom string) error {
	mode := ctx.outputMode()
	if mode == output.ModeSummary {
		if rotatedFrom == "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Token ID:  %s\n", rec.ID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Secret:    %s\n", secret)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Store the secret safely; this value will not be shown again.")
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "New token ID:  %s\n", rec.ID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Secret:       %s\n", secret)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Rotated from:  %s\n", rotatedFrom)
		}
		return nil
	}
	projection := domain.NewProjection(command, summary)
	projection.Facts["token_id"] = rec.ID
	projection.Facts["label"] = rec.Label
	projection.Facts["secret_delivery"] = "interactive_only"
	if rotatedFrom != "" {
		projection.Facts["rotated_from"] = rotatedFrom
	}
	projection.Data = map[string]any{
		"token_id":        rec.ID,
		"label":           rec.Label,
		"expires_at":      rec.ExpiresAt,
		"rotated_from":    rotatedFrom,
		"secret_delivery": "interactive_only",
	}
	return renderProjectionWithOptions(cmd.OutOrStdout(), mode, *ctx.renderOptions, projection, nil)
}

func tokenStore(ctx commandBuildContext) (api.TokenStore, error) {
	path := filepath.Join(*ctx.vaultPath, ".pinax", "tokens", "tokens.json")
	return api.NewFileTokenStore(path)
}

func parseScopeFlag(scopeStr, groupsStr string) map[api.TokenScope]api.ScopeTarget {
	scopes := make(map[api.TokenScope]api.ScopeTarget)
	var groups []string
	if groupsStr != "" {
		groups = strings.Split(groupsStr, ",")
	}
	for _, s := range strings.Split(scopeStr, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		scopes[api.TokenScope(s)] = api.ScopeTarget{Groups: groups}
	}
	if len(scopes) == 0 {
		scopes[api.ScopeRead] = api.ScopeTarget{Groups: groups}
	}
	return scopes
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid day count: %s", days)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
