package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/secrets"
)

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Inspect and validate secret backends",
	}
	cmd.AddCommand(
		newSecretsListCmd(),
		newSecretsTestCmd(),
		newSecretsKeysCmd(),
	)
	return cmd
}

type backendStatus struct {
	Scheme string `json:"scheme"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func newSecretsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured secret backends with Validate() status",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			backends, err := secrets.BuildBackends(cfg)
			if err != nil {
				return err
			}
			schemes := make([]string, 0, len(backends))
			for s := range backends {
				schemes = append(schemes, s)
			}
			sort.Strings(schemes)
			rows := make([]backendStatus, 0, len(schemes))
			for _, s := range schemes {
				row := backendStatus{Scheme: s, Status: "PASS"}
				if err := backends[s].Validate(); err != nil {
					row.Status = "FAIL"
					row.Error = err.Error()
				}
				rows = append(rows, row)
			}
			if outputMode == "json" {
				return outputJSON(rows)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-6s  %s\n", "SCHEME", "STATUS", "DETAIL")
			for _, r := range rows {
				detail := r.Error
				if detail == "" {
					detail = "ok"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-6s  %s\n", r.Scheme, r.Status, detail)
			}
			return nil
		}),
	}
}

func newSecretsTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [scheme]",
		Short: "Run Validate() against one (or all) backends; mints+revokes a smoke key for tailscale",
		Args:  cobra.MaximumNArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			backends, err := secrets.BuildBackends(cfg)
			if err != nil {
				return err
			}
			targets := []string{}
			if len(args) == 1 {
				if _, ok := backends[args[0]]; !ok {
					return fmt.Errorf("no backend registered for scheme %q", args[0])
				}
				targets = []string{args[0]}
			} else {
				for s := range backends {
					targets = append(targets, s)
				}
				sort.Strings(targets)
			}
			rows := make([]backendStatus, 0, len(targets))
			anyFail := false
			for _, scheme := range targets {
				row := backendStatus{Scheme: scheme, Status: "PASS"}
				b := backends[scheme]
				if err := b.Validate(); err != nil {
					row.Status = "FAIL"
					row.Error = err.Error()
					anyFail = true
					rows = append(rows, row)
					continue
				}
				if scheme == "tailscale" {
					ts, ok := b.(*secrets.TailscaleBackend)
					if ok {
						ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
						minted, mErr := ts.MintKey(ctx, secrets.MintKeyOpts{
							Reusable:      false,
							Ephemeral:     false,
							Preauthorized: true,
							Tags:          ts.DefaultTags,
							ExpirySeconds: 60,
							Description:   "nostos-test",
						})
						if mErr != nil {
							row.Status = "FAIL"
							row.Error = fmt.Sprintf("mint smoke key: %v", mErr)
							anyFail = true
							cancel()
							rows = append(rows, row)
							continue
						}
						if rErr := ts.RevokeKey(ctx, minted.ID); rErr != nil {
							row.Status = "FAIL"
							row.Error = fmt.Sprintf("revoke smoke key %s: %v", minted.ID, rErr)
							anyFail = true
						} else {
							row.Error = fmt.Sprintf("minted+revoked smoke key id=%s", minted.ID)
						}
						cancel()
					}
				}
				rows = append(rows, row)
			}
			if outputMode == "json" {
				if err := outputJSON(rows); err != nil {
					return err
				}
			} else {
				for _, r := range rows {
					detail := r.Error
					if detail == "" {
						detail = "ok"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-4s  %s\n", r.Scheme, r.Status, detail)
				}
			}
			if anyFail {
				return fmt.Errorf("one or more backends FAILED")
			}
			return nil
		}),
	}
}

func newSecretsKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Tailscale auth-key inspection",
	}
	cmd.AddCommand(newSecretsKeysListCmd(), newSecretsKeysRevokeCmd())
	return cmd
}

func loadTailscaleBackend() (*secrets.TailscaleBackend, error) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, err
	}
	backends, err := secrets.BuildBackends(cfg)
	if err != nil {
		return nil, err
	}
	b, ok := backends["tailscale"]
	if !ok {
		return nil, fmt.Errorf("tailscale backend not configured (add secrets.tailscale to config.yaml)")
	}
	ts, ok := b.(*secrets.TailscaleBackend)
	if !ok {
		return nil, fmt.Errorf("internal: tailscale backend has unexpected type")
	}
	return ts, nil
}

func newSecretsKeysListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Tailscale auth keys (id, description, expires, tags, used)",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			ts, err := loadTailscaleBackend()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			keys, err := ts.ListKeys(ctx)
			if err != nil {
				return err
			}
			if outputMode == "json" {
				return outputJSON(keys)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-22s  %-30s  %-25s  %-5s  %s\n",
				"ID", "DESCRIPTION", "EXPIRES", "USED", "TAGS")
			for _, k := range keys {
				exp := ""
				if !k.Expires.IsZero() {
					exp = k.Expires.UTC().Format(time.RFC3339)
				}
				tagStr := ""
				for i, t := range k.EffectiveTags() {
					if i > 0 {
						tagStr += ","
					}
					tagStr += t
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s  %-30s  %-25s  %-5v  %s\n",
					k.ID, truncate(k.Description, 30), exp, k.Used, tagStr)
			}
			return nil
		}),
	}
}

func newSecretsKeysRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke KEY_ID",
		Short: "Delete a Tailscale auth key by id",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			ts, err := loadTailscaleBackend()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			if err := ts.RevokeKey(ctx, args[0]); err != nil {
				return err
			}
			if outputMode == "json" {
				return outputJSON(map[string]string{"revoked": args[0]})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Revoked %s\n", args[0])
			return nil
		}),
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
