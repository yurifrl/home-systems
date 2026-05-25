package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/secrets"
)

func newClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Cluster-level operations",
	}
	cmd.AddCommand(newClusterCleanupCmd())
	return cmd
}

// CleanupKubeNode is one entry in the dry-run JSON for kubectl side.
type CleanupKubeNode struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// CleanupTailscaleDevice is one entry in the dry-run JSON for Tailscale side.
type CleanupTailscaleDevice struct {
	DeviceID string    `json:"device_id"`
	Name     string    `json:"name"`
	LastSeen time.Time `json:"last_seen"`
	AgeDays  int       `json:"age_days"`
	Reason   string    `json:"reason"`
}

// CleanupPlan is what `cluster cleanup` emits with --dry-run.
type CleanupPlan struct {
	Status     string                   `json:"status"` // "preview" | "applied"
	K8sNodes   []CleanupKubeNode        `json:"k8s_nodes"`
	Tailscale  []CleanupTailscaleDevice `json:"tailscale_devices"`
	Mutations  []string                 `json:"mutations,omitempty"`
}

func newClusterCleanupCmd() *cobra.Command {
	var (
		yes        bool
		reallyYes  bool
		dryRun     bool
		kubeconfig string
		ageDays    int
	)
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Reconcile k8s + Tailscale state with nostos config; remove zombies",
		Long: "Lists kubectl nodes that are NotReady AND have no matching nostos.yaml entry, " +
			"and Tailscale devices that haven't pinged in >7d AND have no matching node. " +
			"With --yes the kubectl nodes are deleted; Tailscale deletions ALSO require --really-yes.",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			plan := CleanupPlan{Status: "preview"}

			// 1. kubectl nodes side.
			kubeNodes, err := kubectlListNodes(ctx, kubeconfig)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: kubectl get nodes failed: %v\n", err)
			}
			knownNodeNames := map[string]bool{}
			for name := range cfg.Nodes {
				knownNodeNames[name] = true
			}
			for _, n := range kubeNodes {
				if knownNodeNames[n.Name] {
					continue
				}
				if !strings.EqualFold(n.Status, "NotReady") {
					continue
				}
				plan.K8sNodes = append(plan.K8sNodes, CleanupKubeNode{
					Name: n.Name, Status: n.Status,
					Reason: fmt.Sprintf("NotReady AND not in nostos config (status=%s)", n.Status),
				})
			}

			// 2. Tailscale side.
			tsDevices, tsErr := tailscaleListDevices(ctx, cfg)
			if tsErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: tailscale list devices failed: %v\n", tsErr)
			}
			knownTailscaleAddrs := map[string]bool{}
			for _, n := range cfg.Nodes {
				if n.IP != "" {
					knownTailscaleAddrs[n.IP] = true
				}
			}
			now := time.Now()
			for _, d := range tsDevices {
				if !d.LastSeen.IsZero() && now.Sub(d.LastSeen) < time.Duration(ageDays)*24*time.Hour {
					continue
				}
				// Match by 100.x address against known node IPs OR node names.
				matched := false
				for _, addr := range d.Addresses {
					if knownTailscaleAddrs[addr] {
						matched = true
						break
					}
				}
				if matched {
					continue
				}
				if knownNodeNames[strings.SplitN(d.Hostname, ".", 2)[0]] {
					continue
				}
				if knownNodeNames[strings.SplitN(d.Name, ".", 2)[0]] {
					continue
				}
				age := 0
				if !d.LastSeen.IsZero() {
					age = int(now.Sub(d.LastSeen).Hours() / 24)
				}
				plan.Tailscale = append(plan.Tailscale, CleanupTailscaleDevice{
					DeviceID: d.ID,
					Name:     d.Name,
					LastSeen: d.LastSeen,
					AgeDays:  age,
					Reason:   fmt.Sprintf("lastSeen %dd ago, not in nostos config", age),
				})
			}

			// dry-run preview path.
			if dryRun || (!yes && !reallyYes) {
				if outputMode == "json" || dryRun {
					return outputJSON(plan)
				}
				printCleanupText(cmd, plan)
				return nil
			}

			// Apply.
			plan.Status = "applied"
			for _, n := range plan.K8sNodes {
				if err := kubectlDeleteNode(ctx, kubeconfig, n.Name); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "kubectl delete node %s: %v\n", n.Name, err)
					continue
				}
				plan.Mutations = append(plan.Mutations, "kubectl/deleted/"+n.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "deleted k8s node %s\n", n.Name)
			}

			// Tailscale deletions require --really-yes (security guard).
			if len(plan.Tailscale) > 0 {
				if !reallyYes {
					fmt.Fprintln(cmd.ErrOrStderr(), "tailscale deletions require --really-yes; skipping")
				} else {
					ts, err := buildTailscaleBackend(cfg)
					if err != nil {
						return err
					}
					for _, d := range plan.Tailscale {
						// Defense-in-depth: never delete a device seen in the last 1h.
						if !d.LastSeen.IsZero() && time.Since(d.LastSeen) < time.Hour {
							fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: lastSeen <1h ago\n", d.DeviceID)
							continue
						}
						if err := ts.DeleteDevice(ctx, d.DeviceID); err != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "tailscale delete %s: %v\n", d.DeviceID, err)
							continue
						}
						_ = appendAuditLog(d)
						plan.Mutations = append(plan.Mutations, "tailscale/deleted/"+d.DeviceID)
						fmt.Fprintf(cmd.OutOrStdout(), "deleted tailscale device %s (%s)\n", d.DeviceID, d.Name)
					}
				}
			}

			if outputMode == "json" {
				return outputJSON(plan)
			}
			printCleanupText(cmd, plan)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "apply kubectl deletions (Tailscale also requires --really-yes)")
	cmd.Flags().BoolVar(&reallyYes, "really-yes", false, "permit Tailscale device deletion (security guard)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "force JSON preview output and exit 0")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig path (default: KUBECONFIG env or ~/.kube/config)")
	cmd.Flags().IntVar(&ageDays, "age-days", 7, "Tailscale lastSeen threshold in days")
	return cmd
}

func printCleanupText(cmd *cobra.Command, plan CleanupPlan) {
	fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", plan.Status)
	if len(plan.K8sNodes) == 0 && len(plan.Tailscale) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "nothing to clean up")
		return
	}
	if len(plan.K8sNodes) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "k8s nodes (NotReady, not in config):")
		for _, n := range plan.K8sNodes {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", n.Name, n.Reason)
		}
	}
	if len(plan.Tailscale) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "tailscale devices (stale, not in config):")
		for _, d := range plan.Tailscale {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s) lastSeen=%s ageDays=%d\n", d.DeviceID, d.Name, d.LastSeen.Format(time.RFC3339), d.AgeDays)
		}
	}
	if plan.Status == "preview" {
		fmt.Fprintln(cmd.OutOrStdout(), "(preview only; pass --yes to delete k8s nodes, --really-yes to delete Tailscale devices)")
	}
}

// --- kubectl helpers ---

type kubeNode struct {
	Name   string
	Status string
}

func kubectlListNodes(ctx context.Context, kubeconfig string) ([]kubeNode, error) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		return nil, fmt.Errorf("kubectl not on PATH: %w", err)
	}
	args := []string{"get", "nodes", "-o", "json"}
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "kubectl", args...).Output()
	if err != nil {
		return nil, err
	}
	var raw struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	res := make([]kubeNode, 0, len(raw.Items))
	for _, it := range raw.Items {
		status := "Unknown"
		for _, c := range it.Status.Conditions {
			if c.Type == "Ready" {
				if c.Status == "True" {
					status = "Ready"
				} else {
					status = "NotReady"
				}
			}
		}
		res = append(res, kubeNode{Name: it.Metadata.Name, Status: status})
	}
	return res, nil
}

func kubectlDeleteNode(ctx context.Context, kubeconfig, name string) error {
	args := []string{"delete", "node", name}
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "kubectl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- tailscale helpers ---

func buildTailscaleBackend(cfg *config.Config) (*secrets.TailscaleBackend, error) {
	backends, err := secrets.BuildBackends(cfg)
	if err != nil {
		return nil, err
	}
	tb, ok := backends["tailscale"].(*secrets.TailscaleBackend)
	if !ok {
		return nil, fmt.Errorf("no tailscale backend configured")
	}
	return tb, nil
}

func tailscaleListDevices(ctx context.Context, cfg *config.Config) ([]secrets.Device, error) {
	if cfg.Secrets.Tailscale == nil {
		return nil, nil
	}
	tb, err := buildTailscaleBackend(cfg)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return tb.ListDevices(cctx)
}

// appendAuditLog writes a single line to ~/.local/state/nostos/audit.log.
func appendAuditLog(d CleanupTailscaleDevice) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".local", "state", "nostos")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "audit.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line := fmt.Sprintf("%s\ttailscale-delete\t%s\t%s\t%s\n",
		time.Now().UTC().Format(time.RFC3339), d.DeviceID, d.Name, d.Reason)
	_, err = f.WriteString(line)
	return err
}

// silence unused imports in some build matrices.
var _ = net.IPv4
