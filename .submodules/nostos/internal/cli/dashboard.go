package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/jsonio"
	"github.com/yurifrl/nostos/internal/dashboard"
	"github.com/yurifrl/nostos/internal/dashboard/actions"
	"github.com/yurifrl/nostos/internal/dashboard/cache"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
	"github.com/yurifrl/nostos/internal/dashboard/tui"
	"github.com/yurifrl/nostos/internal/execx"
)

// newDashboardCmd wires `nostos dashboard`.
func newDashboardCmd() *cobra.Command {
	var once bool
	var fields string
	var noUpstream bool
	var dispatchMode string

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Live single-pane TUI for cluster + nodes + ArgoCD apps",
		Long: "Bubble Tea v2 dashboard. Use --once --output json to emit a single snapshot " +
			"and exit (CI/headless friendly).",
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			if once {
				snap := dashboard.BuildSnapshot(ctx, cfg, dashboard.Options{
					HiddenMACs:    loadHiddenDevices(),
					FetchUpstream: !noUpstream,
				})
				if outputMode == "json" {
					if fields != "" {
						f, err := parseFields(fields, snapshotFields)
						if err != nil {
							return err
						}
						m, err := jsonio.ToMap(snap)
						if err != nil {
							return err
						}
						return jsonio.EncodePretty(cmd.OutOrStdout(), jsonio.ProjectFields(m, f))
					}
					return jsonio.EncodePretty(cmd.OutOrStdout(), snap)
				}
				renderTextSnapshot(cmd, snap)
				return nil
			}

			// Interactive TUI path.
			model := tui.Model{Dispatcher: pickDispatcher(dispatchMode)}

			// Cold-start cache: render the cached snapshot first if any, then
			// kick off a live refresh in the background.
			if st, ok := cache.Load(); ok {
				cached := st.Snap
				cache.MarkCached(&cached)
				model.Snap = cached
				model.Cached = true
			} else {
				model.Snap = dashboard.BuildSnapshot(ctx, cfg, dashboard.Options{
					HiddenMACs:    loadHiddenDevices(),
					FetchUpstream: !noUpstream,
				})
				_ = cache.Save(model.Snap)
			}

			p := tea.NewProgram(model, tea.WithContext(ctx))
			// Background refresh tick.
			go func() {
				// Always do an immediate refresh to clear any cache prefix.
				fresh := dashboard.BuildSnapshot(ctx, cfg, dashboard.Options{
					HiddenMACs:    loadHiddenDevices(),
					FetchUpstream: !noUpstream,
				})
				_ = cache.Save(fresh)
				p.Send(tui.SnapshotMsg(fresh))

				t := time.NewTicker(5 * time.Second)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						s := dashboard.BuildSnapshot(ctx, cfg, dashboard.Options{
							HiddenMACs: loadHiddenDevices(),
						})
						_ = cache.Save(s)
						p.Send(tui.SnapshotMsg(s))
					}
				}
			}()
			_, err = p.Run()
			return err
		}),
	}

	cmd.Flags().BoolVar(&once, "once", false, "run all checks once, emit a snapshot, exit")
	cmd.Flags().BoolVar(&noUpstream, "no-upstream", false, "skip the HTTP upstream-version probe")
	cmd.Flags().StringVar(&fields, "fields", "", "comma-separated subset of "+joinFields(snapshotFields))
	cmd.Flags().StringVar(&dispatchMode, "dispatch", "", "action dispatcher: '' (real, default), 'mock' (no-op, for smoke tests)")
	return cmd
}

// pickDispatcher selects the action dispatcher implementation. Honors the
// NOSTOS_DASHBOARD_DISPATCH_DRY_RUN env var as a forced override.
func pickDispatcher(mode string) actions.Dispatcher {
	if mode == "mock" || os.Getenv("NOSTOS_DASHBOARD_DISPATCH_DRY_RUN") == "1" {
		return actions.NoopDispatcher{}
	}
	return actions.New(execx.OSCommander{})
}

// loadHiddenDevices reads ${XDG_CONFIG_HOME:-~/.config}/nostos/dashboard.toml
// and returns the lowercased MAC set under [hidden_devices].
//
// The TOML parser is intentionally trivial — v0.3 only reads `mac = "..."`
// entries inside the [hidden_devices] table; richer parsing comes in v0.4
// with the BurntSushi/toml dep.
func loadHiddenDevices() map[string]bool {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".config")
		}
	}
	if base == "" {
		return nil
	}
	path := filepath.Join(base, "nostos", "dashboard.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	inSection := false
	for _, line := range splitLines(string(data)) {
		line = trimSpace(line)
		if line == "" || hasPrefix(line, "#") {
			continue
		}
		if line == "[hidden_devices]" {
			inSection = true
			continue
		}
		if hasPrefix(line, "[") {
			inSection = false
			continue
		}
		if !inSection {
			continue
		}
		// expect: key = "aa:bb:cc:dd:ee:ff"
		i := indexByte(line, '"')
		j := lastIndexByte(line, '"')
		if i < 0 || j <= i {
			continue
		}
		out[toLower(line[i+1:j])] = true
	}
	return out
}

// snapshotFields is the field allowlist for `--fields` projection on the snapshot.
var snapshotFields = []string{
	"schema_version", "aggregate_state", "imperative", "kubeconfig_present",
	"cluster", "nodes", "apps", "discoveries", "checks", "upstream_diff", "generated_at",
}

func renderTextSnapshot(cmd *cobra.Command, snap snapshot.Snapshot) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "[ %s ]  %s\n", snap.Cluster.Name, snap.AggregateState)
	fmt.Fprintf(w, "Cluster: %d/%d ready  · kubeconfig=%v\n",
		snap.Cluster.NodesReady, snap.Cluster.NodesConfigured, snap.Cluster.KubeconfigPresent)
	for _, n := range snap.Nodes {
		fmt.Fprintf(w, "  %s  %-10s  %-15s  %-12s  %s\n",
			sevSymbol(n.Severity), n.Name, n.IP, n.Role, n.Version)
	}
	for _, d := range snap.Discoveries {
		fmt.Fprintf(w, "  ?  %-10s  %-15s  %s\n", d.Hostname, d.IP, d.Bucket)
	}
	fmt.Fprintln(w, "[ checks ]")
	for _, c := range snap.Checks {
		fmt.Fprintf(w, "  %s [%s] %s\n", sevSymbol(c.Severity), c.ID, c.Message)
	}
}

func sevSymbol(s snapshot.Severity) string {
	switch s {
	case snapshot.SevInfo:
		return "[OK]"
	case snapshot.SevWarn:
		return "[WARN]"
	case snapshot.SevFail:
		return "[FAIL]"
	}
	return "[?]"
}

// --- tiny ASCII helpers (avoid pulling strings into helpers.go which is busy) ---

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func toLower(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
