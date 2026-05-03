package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cluster"
	"github.com/yurifrl/nostos/internal/pxe"
	"github.com/yurifrl/nostos/internal/registry"
)

func newBuildCmd() *cobra.Command {
	var arch string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Download Talos assets + build iPXE binary",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			if err := pxe.BuildAll(cmd.Context(), cfg, p, arch); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Assets ready in %s\n", p.Assets())
			return nil
		}),
	}
	cmd.Flags().StringVar(&arch, "arch", "amd64", "target architecture (amd64|arm64)")
	return cmd
}

func newPxeCmd() *cobra.Command {
	var iface string
	var port int
	cmd := &cobra.Command{
		Use:     "pxe",
		Aliases: []string{"serve"},
		Short:   "Start PXE server (HTTP + dnsmasq) until Ctrl+C",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			_ = cfg // reserved for future wipe-aware serve
			srv := pxe.NewServer(p)
			if iface != "" {
				srv.Interface = iface
			}
			if port > 0 {
				srv.HTTPPort = port
			}
			ni, err := srv.Preflight()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			if err := srv.Start(ctx, ni); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"%s nostos pxe on %s (%s), :%d. Ctrl+C to stop.\n",
				lipgloss.NewStyle().Bold(true).Render("→"),
				ni.Interface, ni.IP, srv.HTTPPort,
			)
			return srv.Wait(ctx)
		}),
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (auto-detect if empty)")
	cmd.Flags().IntVar(&port, "port", pxe.DefaultHTTPPort, "HTTP port")
	return cmd
}

func newWipeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wipe NODE",
		Short: "Queue a one-shot disk wipe for NODE on its next PXE boot",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			n, err := registry.Get(cfg, args[0])
			if err != nil {
				return err
			}
			if err := cluster.QueueWipe(p.PendingWipes(), n.MAC); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Queued wipe for %s (%s). Reboot the node into PXE to execute.\n",
				args[0], n.MAC,
			)
			return nil
		}),
	}
}

func newBootstrapCmd() *cobra.Command {
	var timeoutMin int
	cmd := &cobra.Command{
		Use:   "bootstrap NODE",
		Short: "Bootstrap etcd on NODE (first controlplane only)",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			n, err := registry.Get(cfg, args[0])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if err := cluster.Bootstrap(ctx, cfg, p, n, time.Duration(timeoutMin)*time.Minute); err != nil {
				return err
			}
			if err := cluster.FetchKubeconfig(ctx, p, n); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: kubeconfig fetch: %v\n", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bootstrapped %s. kubeconfig: %s\n", args[0], p.Kubeconfig())
			return nil
		}),
	}
	cmd.Flags().IntVar(&timeoutMin, "timeout", 5, "minutes to wait for etcd health")
	return cmd
}

func newKubeconfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kubeconfig [NODE]",
		Short: "Refresh state/kubeconfig from a running controlplane",
		Args:  cobra.MaximumNArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			var name string
			if len(args) > 0 {
				name = args[0]
			} else {
				for n, node := range cfg.Nodes {
					if node.Role == "controlplane" {
						name = n
						break
					}
				}
				if name == "" {
					return fmt.Errorf("no controlplane node in config")
				}
			}
			n, err := registry.Get(cfg, name)
			if err != nil {
				return err
			}
			if err := cluster.FetchKubeconfig(cmd.Context(), p, n); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "kubeconfig written to %s\n", p.Kubeconfig())
			return nil
		}),
	}
}

func newNukeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "nuke",
		Short: "Remove state/ entirely (safe: regenerable from config.yaml + secrets)",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			_, p, err := loadConfig()
			if err != nil {
				return err
			}
			if _, statErr := os.Stat(p.State()); statErr != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to nuke.")
				return nil
			}
			if !yes {
				return fmt.Errorf("refusing without --yes (this removes %s)", p.State())
			}
			if err := os.RemoveAll(p.State()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", p.State())
			return nil
		}),
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func newUpCmd() *cobra.Command {
	var (
		skipWipe       bool
		serveTimeout   time.Duration
		bootTimeout    time.Duration
		bootstrapTO    time.Duration
	)
	cmd := &cobra.Command{
		Use:   "up NODE",
		Short: "End-to-end install: wipe → build → render → serve → bootstrap → Ready",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			n, err := registry.Get(cfg, args[0])
			if err != nil {
				return err
			}

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			events := make(chan cluster.Event, 32)
			done := make(chan error, 1)
			go func() {
				done <- cluster.Install(ctx, cfg, p, n, args[0],
					cluster.InstallOpts{
						SkipWipe:         skipWipe,
						ServeTimeout:     serveTimeout,
						BootTimeout:      bootTimeout,
						BootstrapTimeout: bootstrapTO,
					}, events)
				close(events)
			}()

			for ev := range events {
				printEvent(cmd, ev)
			}
			return <-done
		}),
	}
	cmd.Flags().BoolVar(&skipWipe, "skip-wipe", false, "don't queue a disk wipe (re-boot existing install)")
	cmd.Flags().DurationVar(&serveTimeout, "serve-timeout", 10*time.Minute, "max wait for PXE to fetch config")
	cmd.Flags().DurationVar(&bootTimeout, "boot-timeout", 10*time.Minute, "max wait for node to come back at static IP")
	cmd.Flags().DurationVar(&bootstrapTO, "bootstrap-timeout", 5*time.Minute, "max wait for etcd health")
	return cmd
}

func printEvent(cmd *cobra.Command, ev cluster.Event) {
	var icon, color string
	switch ev.Kind {
	case cluster.KindProgress:
		icon, color = "→", "11"
	case cluster.KindDownload:
		icon, color = "▼", "12"
	case cluster.KindConfigFetched:
		icon, color = "✓", "10"
	case cluster.KindNodeUp, cluster.KindApidUp:
		icon, color = "◉", "10"
	case cluster.KindBootstrapping:
		icon, color = "⚡", "13"
	case cluster.KindReady:
		icon, color = "✓", "10"
	case cluster.KindError:
		icon, color = "✗", "9"
	default:
		icon, color = "·", "8"
	}
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(icon)
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", styled, ev.Message)
}

// cfgRefreshCmd holds the `config refresh` subcommand tree (currently only cert).
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Config subcommands"}
	cmd.AddCommand(newConfigRefreshCmd())
	return cmd
}

func newConfigRefreshCmd() *cobra.Command {
	var hours int
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Regenerate admin client certificate (v0.1: not yet implemented in Go)",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			// pick first controlplane
			var name string
			for n, node := range cfg.Nodes {
				if node.Role == "controlplane" {
					name = n
					break
				}
			}
			if name == "" {
				return fmt.Errorf("no controlplane node in config")
			}
			n := cfg.Nodes[name]
			return cluster.RefreshAdminCert(cfg, p, n, hours)
		}),
	}
	cmd.Flags().IntVar(&hours, "hours", 876_000, "admin cert validity (hours)")
	return cmd
}

// ensureAssetsPath is a no-op stub referenced from elsewhere.
var _ = filepath.Join
