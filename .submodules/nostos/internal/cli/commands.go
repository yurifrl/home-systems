package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/dryrun"
	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cluster"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/pxe"
	"github.com/yurifrl/nostos/internal/registry"
)

func newBuildCmd() *cobra.Command {
	var arch string
	var legacy bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Download Talos assets + build iPXE binary",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			// Default: multi-arch build over all nodes in config. Pass --arch
			// (or --legacy) to fall back to the single-arch v0.1 path.
			if arch != "" || legacy {
				if arch == "" {
					arch = "amd64"
				}
				if err := pxe.BuildAll(cmd.Context(), cfg, p, arch); err != nil {
					return err
				}
				if outputMode == "json" {
					return outputJSON(map[string]string{"status": "built", "assets": p.Assets(), "arch": arch})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Assets ready in %s\n", p.Assets())
				return nil
			}
			specs := pxe.CollectAssetSpecs(cfg)
			if err := pxe.BuildAllNodes(cmd.Context(), cfg, p); err != nil {
				return err
			}
			if outputMode == "json" {
				rows := make([]map[string]any, 0, len(specs))
				for _, s := range specs {
					rows = append(rows, map[string]any{
						"schematic": s.Schematic,
						"arch":      s.Arch,
						"version":   s.Version,
						"rpi":       s.IsRPi,
					})
				}
				return outputJSON(map[string]any{"status": "built", "assets": p.Assets(), "specs": rows})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Assets ready in %s (built %d schematic/arch pairs)\n", p.Assets(), len(specs))
			for _, s := range specs {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s%s\n", s.Schematic[:12]+"...", s.Arch, rpiTag(s.IsRPi))
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&arch, "arch", "", "target architecture (amd64|arm64); empty = multi-arch over all nodes")
	cmd.Flags().BoolVar(&legacy, "legacy", false, "force single-arch v0.1 path (cluster default schematic only)")
	return cmd
}

func rpiTag(isRPi bool) string {
	if isRPi {
		return " [+rpi-firmware]"
	}
	return ""
}

func newPxeCmd() *cobra.Command {
	var iface string
	var port int
	var fullDHCP bool
	var logJSON string
	cmd := &cobra.Command{
		Use:     "pxe",
		Aliases: []string{"serve"},
		Short:   "Start PXE server (HTTP + dnsmasq) until Ctrl+C",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			_ = cfg
			srv := pxe.NewServer(p)
			if iface != "" {
				srv.Interface = iface
			}
			if port > 0 {
				srv.HTTPPort = port
			}
			if fullDHCP {
				srv.ProxyMode = false
			}
			srv.LogJSONPath = logJSON
			ni, err := srv.Preflight()
			if err != nil {
				if errors.Is(err, pxe.ErrSudoRequired) {
					return errs.Auth("E_SUDO_REQUIRED", err.Error()).WithHint("run: nostos pxe setup")
				}
				return err
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			if err := srv.Start(ctx, ni); err != nil {
				return err
			}
			mode := "proxy"
			if !srv.ProxyMode {
				mode = "full-dhcp"
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"%s nostos pxe on %s (%s), :%d, mode=%s. Ctrl+C to stop.\n",
				lipgloss.NewStyle().Bold(true).Render("→"),
				ni.Interface, ni.IP, srv.HTTPPort, mode,
			)
			return srv.Wait(ctx)
		}),
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (auto-detect if empty)")
	cmd.Flags().IntVar(&port, "port", pxe.DefaultHTTPPort, "HTTP port")
	cmd.Flags().BoolVar(&fullDHCP, "full-dhcp", false, "act as full DHCP server instead of PXE proxy (use when no other DHCP on LAN)")
	cmd.Flags().StringVar(&logJSON, "log-json", "", "also append the PXE event stream as NDJSON to this file (for detached tailing)")
	cmd.AddCommand(newPxeSetupCmd())
	cmd.AddCommand(newPxeDoctorCmd())
	cmd.AddCommand(newPxeStatusCmd())
	return cmd
}

// pxeStatusFields is the projection schema for `nostos pxe status --fields`.
var pxeStatusFields = []string{"name", "mac", "ip", "interface", "phase", "last_seen", "known", "enrollable"}

// pxeStatusRow is one node/observed-MAC row emitted by `nostos pxe status`.
type pxeStatusRow struct {
	Name       string `json:"name,omitempty"`
	MAC        string `json:"mac,omitempty"`
	IP         string `json:"ip,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Phase      string `json:"phase"`
	LastSeen   string `json:"last_seen,omitempty"`
	Known      bool   `json:"known"`
	Enrollable bool   `json:"enrollable"`
}

func newPxeStatusCmd() *cobra.Command {
	var fieldsRaw string
	cmd := &cobra.Command{
		Use:   "status [NODE]",
		Short: "Show per-node PXE boot lifecycle state (from the event stream)",
		Long: "Show per-node PXE boot lifecycle state derived from the event stream.\n\n" +
			"Unknown MACs that PXE-boot show up here with known=no and enrollable=true,\n" +
			"along with the interface they were first seen on. nostos never auto-provisions\n" +
			"unknown hardware; it only surfaces it. To enroll an unknown node:\n\n" +
			"  1. Add it to config.yaml (mac/ip/role/disk/template) or run `nostos node add`.\n" +
			"  2. Run `nostos node install <name>` to provision it.\n\n" +
			"Config nodes that have never been observed stay known=yes, enrollable=false.",
		Args: cobra.MaximumNArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			fields, err := parseFields(fieldsRaw, pxeStatusFields)
			if err != nil {
				return err
			}
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}

			store := pxe.NewEventStore(p.State())
			events, err := store.Load()
			if err != nil {
				return errs.FromGo(err)
			}
			states := pxe.FoldState(events)

			// Build MAC(hyphen) -> node-name index for the join + known flag.
			macToName := map[string]string{}
			for _, e := range registry.List(cfg) {
				if mh := e.Node.MACHyphen(); mh != "" {
					macToName[mh] = e.Name
				}
			}

			// Single-node OBJECT mode.
			if len(args) == 1 {
				node, err := registry.Get(cfg, args[0])
				if err != nil {
					return errs.NotFound("E_NODE_NOT_FOUND", err.Error())
				}
				row := pxeStatusRow{Name: args[0], MAC: node.MACHyphen(), IP: node.IP, Phase: string(pxe.PhaseUnknown), Known: true}
				if st, ok := states[node.MACHyphen()]; ok && node.MACHyphen() != "" {
					row.Phase = phaseOrUnknown(st.Phase)
					if st.IP != "" {
						row.IP = st.IP
					}
					row.Interface = st.Interface
					row.LastSeen = formatLastSeen(st.LastSeen)
				} else {
					row.Phase = "unknown"
				}
				// A config node is always Known, hence never enrollable.
				return emitObjectOutput(row, fields, func() { printPxeStatusRow(cmd.OutOrStdout(), row) })
			}

			// LIST mode: one row per config node + any observed-but-unknown MAC.
			rows := make([]pxeStatusRow, 0, len(macToName)+len(states))
			seen := map[string]bool{}
			for _, e := range registry.List(cfg) {
				mh := e.Node.MACHyphen()
				row := pxeStatusRow{Name: e.Name, MAC: mh, IP: e.Node.IP, Phase: "unknown", Known: true}
				if st, ok := states[mh]; ok && mh != "" {
					row.Phase = phaseOrUnknown(st.Phase)
					if st.IP != "" {
						row.IP = st.IP
					}
					row.Interface = st.Interface
					row.LastSeen = formatLastSeen(st.LastSeen)
				}
				// Known config nodes are never enrollable, observed or not.
				rows = append(rows, row)
				if mh != "" {
					seen[mh] = true
				}
			}
			// Observed MACs not in config: these were seen in the event stream but
			// are not Known, so they are enrollable.
			observed := make([]string, 0, len(states))
			for key := range states {
				observed = append(observed, key)
			}
			sort.Strings(observed)
			for _, key := range observed {
				st := states[key]
				if st.MAC != "" && seen[st.MAC] {
					continue
				}
				rows = append(rows, pxeStatusRow{
					MAC:        st.MAC,
					IP:         st.IP,
					Interface:  st.Interface,
					Phase:      phaseOrUnknown(st.Phase),
					LastSeen:   formatLastSeen(st.LastSeen),
					Known:      false,
					Enrollable: true,
				})
			}

			// In text mode, nudge the operator toward the enroll path when there is
			// at least one enrollable (observed + unknown) MAC. Keep stdout clean;
			// the hint goes to stderr. JSON consumers rely on the enrollable field,
			// so the hint is suppressed in json mode to keep the stream parseable.
			if outputMode != "json" {
				printEnrollHint(cmd.ErrOrStderr(), rows)
			}

			return emitListOutput(rows, pxeStatusFields, fields, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "%-12s  %-18s  %-16s  %-8s  %-12s  %-20s  %-5s  %s\n",
					"NAME", "MAC", "IP", "IFACE", "PHASE", "LAST-SEEN", "KNOWN", "ENROLL")
				for _, r := range rows {
					printPxeStatusRow(w, r)
				}
			})
		}),
	}
	cmd.Flags().StringVar(&fieldsRaw, "fields", "", "comma-separated subset of "+joinFields(pxeStatusFields))
	return cmd
}

func phaseOrUnknown(p pxe.Phase) string {
	if p == pxe.PhaseUnknown {
		return "unknown"
	}
	return string(p)
}

func formatLastSeen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func printPxeStatusRow(w io.Writer, r pxeStatusRow) {
	name := r.Name
	if name == "" {
		name = "—"
	}
	mac := r.MAC
	if mac == "" {
		mac = "—"
	}
	ip := r.IP
	if ip == "" {
		ip = "—"
	}
	iface := r.Interface
	if iface == "" {
		iface = "—"
	}
	last := r.LastSeen
	if last == "" {
		last = "—"
	}
	known := "no"
	if r.Known {
		known = "yes"
	}
	enroll := "—"
	if r.Enrollable {
		enroll = "yes"
	}
	fmt.Fprintf(w, "%-12s  %-18s  %-16s  %-8s  %-12s  %-20s  %-5s  %s\n", name, mac, ip, iface, r.Phase, last, known, enroll)
}

// printEnrollHint writes a one-line actionable hint to w (stderr) when rows
// contains at least one enrollable (observed + unknown) MAC. It is a no-op
// when there are none, so clean fleets print nothing.
func printEnrollHint(w io.Writer, rows []pxeStatusRow) {
	n := 0
	first := pxeStatusRow{}
	for _, r := range rows {
		if r.Enrollable {
			if n == 0 {
				first = r
			}
			n++
		}
	}
	if n == 0 {
		return
	}
	noun := "MAC"
	if n > 1 {
		noun = "MACs"
	}
	where := ""
	if first.Interface != "" {
		where = " on " + first.Interface
	}
	mac := first.MAC
	if mac == "" {
		mac = "?"
	}
	fmt.Fprintf(w, "hint: %d unknown %s seen (%s%s). Enroll: add it to config.yaml (or run `nostos node add`), then `nostos node install <name>`.\n",
		n, noun, mac, where)
}

func newPxeSetupCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install a scoped NOPASSWD sudoers drop-in so the PXE server runs sudo-less",
		Long: "Install a scoped NOPASSWD sudoers drop-in at " + pxe.SudoersDropInPath + " so\n" +
			"`nostos pxe` (serve) can run dnsmasq without a password prompt. This is the\n" +
			"one interactive step a human runs once; afterwards serving is sudo-less.",
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			if dryRun {
				plan := dryrun.New("pxe.setup")
				plan.Add("build", "build scoped NOPASSWD sudoers drop-in for the current user (dnsmasq + pkill)")
				plan.AddArgv("validate", "syntax-check the drop-in with visudo",
					[]string{"visudo", "-cf", "<tmpfile>"}, nil)
				plan.AddArgv("install", "install the drop-in to "+pxe.SudoersDropInPath+" (interactive sudo)",
					[]string{"sudo", "install", "-m", "0440", "-o", "root", "-g", "wheel", "<tmpfile>", pxe.SudoersDropInPath}, nil)
				return emitDryRun(plan)
			}
			if err := pxe.InstallSudoers(cmd.OutOrStdout()); err != nil {
				return err
			}
			return emitObjectOutput(
				map[string]any{"status": "installed", "path": pxe.SudoersDropInPath, "dnsmasq": pxe.DnsmasqBinary()},
				nil,
				func() {
					fmt.Fprintf(cmd.OutOrStdout(),
						"%s installed sudoers drop-in at %s; PXE server now runs sudo-less\n",
						lipgloss.NewStyle().Bold(true).Render("✓"), pxe.SudoersDropInPath,
					)
				},
			)
		}),
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview planned actions as JSON; no subprocesses are spawned")
	return cmd
}

func newPxeDoctorCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Preflight self-diagnosis for the PXE server (interfaces, ports, sudo, assets, HTTP self-test)",
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			_, p, err := loadConfig()
			if err != nil {
				return err
			}
			report := pxe.RunDoctor(p, port)
			textFn := func() { printDoctorReport(cmd.OutOrStdout(), report) }
			if report.OK {
				// Single object on stdout (json) or the human table (text).
				return emitObjectOutput(report, nil, textFn)
			}
			// Failure: keep EXACTLY one object on stdout. In text mode print the
			// table ourselves, then return the typed error (hint on stderr, exit
			// 10). In json mode print nothing here — HandleExit emits the error
			// object (which carries the full report under details) as the sole
			// stdout JSON object.
			if outputMode != "json" {
				textFn()
			}
			return errs.Validation("E_PXE_DOCTOR", "pxe doctor found failing checks").
				WithDetails(map[string]any{"report": report}).
				WithHint("fix the failing checks above")
		}),
	}
	cmd.Flags().IntVar(&port, "port", pxe.DefaultHTTPPort, "HTTP port to test for bindability (match what serve would use)")
	return cmd
}

// printDoctorReport renders a human-readable check table (✓/✗ per check).
func printDoctorReport(w io.Writer, report pxe.DoctorReport) {
	okStyle := lipgloss.NewStyle().Bold(true)
	for _, c := range report.Checks {
		mark := "✓"
		if !c.OK {
			mark = "✗"
		}
		fmt.Fprintf(w, "%s %-18s %s\n", okStyle.Render(mark), c.Name, c.Detail)
		if c.Hint != "" {
			fmt.Fprintf(w, "    hint: %s\n", c.Hint)
		}
	}
	summary := "all checks passed"
	if !report.OK {
		summary = "one or more checks failed"
	}
	fmt.Fprintf(w, "%s %s\n", okStyle.Render("→"), summary)
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
			if err := cluster.FetchKubeconfig(ctx, cfg, p, n); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: kubeconfig fetch: %v\n", err)
			}
			if tsCtx, err := cluster.ConfigureTailscaleContext(ctx, cfg, p); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: tailscale context: %v\n", err)
			} else if tsCtx != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "tailscale context added: %s\n", tsCtx)
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
		Short: "Refresh kubeconfig from a running controlplane",
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
			if err := cluster.FetchKubeconfig(cmd.Context(), cfg, p, n); err != nil {
				return err
			}
			tsCtx, tsErr := cluster.ConfigureTailscaleContext(cmd.Context(), cfg, p)
			if outputMode == "json" {
				res := map[string]string{"status": "fetched", "path": p.Kubeconfig(), "node": name}
				if tsCtx != "" {
					res["tailscale_context"] = tsCtx
				}
				if tsErr != nil {
					res["tailscale_warning"] = tsErr.Error()
				}
				return outputJSON(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "kubeconfig written to %s\n", p.Kubeconfig())
			if tsErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: tailscale context: %v\n", tsErr)
			} else if tsCtx != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "tailscale context added: %s\n", tsCtx)
			}
			return nil
		}),
	}
}

func newNukeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "nuke",
		Short: "Remove nostos runtime state entirely (safe: regenerable from config.yaml + secrets)",
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
		skipWipe     bool
		serveTimeout time.Duration
		bootTimeout  time.Duration
		bootstrapTO  time.Duration
	)
	cmd := &cobra.Command{
		Use:   "up NODE",
		Short: "[deprecated] alias for `nostos node install` — end-to-end install",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "deprecated: nostos up; use 'nostos node install <name>'")
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
			if err := <-done; err != nil {
				if errors.Is(err, pxe.ErrSudoRequired) {
					return errs.Auth("E_SUDO_REQUIRED", err.Error()).WithHint("run: nostos pxe setup")
				}
				return err
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&skipWipe, "skip-wipe", false, "don't queue a disk wipe (re-boot existing install)")
	cmd.Flags().DurationVar(&serveTimeout, "serve-timeout", 0, "max wait for PXE to fetch config (0 = wait indefinitely)")
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
		Short: "Mint a fresh admin talosconfig from the cluster CA (offline; no cluster mutation)",
		Long: "Regenerate ~/.talos/config with a fresh os:admin client certificate.\n" +
			"Resolves the Talos OS CA from the secrets backend by rendering a\n" +
			"controlplane machineconfig, then mints a new client cert via talosctl.\n" +
			"Use when the admin cert has expired. Does NOT touch the cluster.",
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			if err := cluster.RefreshAdminCert(cfg, p, config.Node{}, hours); err != nil {
				return errs.FromGo(err)
			}
			if outputMode == "json" {
				return outputJSON(map[string]string{"status": "refreshed", "talosconfig": p.Talosconfig()})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Fresh admin talosconfig written to %s\n", p.Talosconfig())
			return nil
		}),
	}
	cmd.Flags().IntVar(&hours, "hours", 876_000, "requested admin cert validity (hours; talosctl controls actual lifetime)")
	return cmd
}

// ensureAssetsPath is a no-op stub referenced from elsewhere.
var _ = filepath.Join
