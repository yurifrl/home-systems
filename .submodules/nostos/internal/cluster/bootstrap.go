package cluster

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

// Bootstrap runs `talosctl bootstrap` against node. Idempotent on already-bootstrapped.
func Bootstrap(ctx context.Context, cfg *config.Config, p paths.Paths, node config.Node, timeout time.Duration) error {
	if err := requireTalosctl(); err != nil {
		return err
	}
	if node.Role != "controlplane" {
		return fmt.Errorf("bootstrap targets controlplane nodes; node role is %q", node.Role)
	}

	args := []string{
		"--talosconfig", p.Talosconfig(),
		"--nodes", node.IP,
		"--endpoints", node.IP,
		"bootstrap",
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(runCtx, "talosctl", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "AlreadyExists") || strings.Contains(strings.ToLower(msg), "already bootstrapped") {
			// idempotent success
		} else {
			return fmt.Errorf("talosctl bootstrap failed: %s", msg)
		}
	}
	return waitForEtcd(ctx, p, node, timeout)
}

func waitForEtcd(ctx context.Context, p paths.Paths, node config.Node, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, err := exec.CommandContext(c, "talosctl",
			"--talosconfig", p.Talosconfig(),
			"--nodes", node.IP,
			"--endpoints", node.IP,
			"service", "etcd",
		).CombinedOutput()
		cancel()
		if err == nil && strings.Contains(string(out), "Running") && strings.Contains(string(out), "OK") {
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("etcd did not become healthy on %s within %s", node.IP, timeout)
}

// FetchKubeconfig writes cluster kubeconfig to state/kubeconfig.
func FetchKubeconfig(ctx context.Context, p paths.Paths, node config.Node) error {
	if err := requireTalosctl(); err != nil {
		return err
	}
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, "talosctl",
		"--talosconfig", p.Talosconfig(),
		"--nodes", node.IP,
		"--endpoints", node.IP,
		"kubeconfig", "--force", p.Kubeconfig(),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubeconfig fetch: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func requireTalosctl() error {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return errors.New("talosctl not found; install: brew install siderolabs/tap/talosctl")
	}
	return nil
}
