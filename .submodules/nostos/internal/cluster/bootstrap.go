package cluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/secrets"
	"gopkg.in/yaml.v3"
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

// FetchKubeconfig writes cluster kubeconfig to the nostos state directory.
func FetchKubeconfig(ctx context.Context, cfg *config.Config, p paths.Paths, node config.Node) error {
	if err := requireTalosctl(); err != nil {
		return err
	}
	if err := ensureTalosconfig(cfg, p); err != nil {
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

// ConfigureTailscaleContext adds a remote kubeconfig context that reaches the
// cluster's kube-apiserver through the Tailscale operator's API server proxy,
// merged into the same kubeconfig file FetchKubeconfig writes. It is a no-op
// (returns "", nil) when cluster.tailscale_operator is unset.
//
// `tailscale configure kubeconfig` switches current-context to the proxy; we
// restore the LAN context as the default so local tooling is unaffected. The
// returned string is the name of the context that was added.
func ConfigureTailscaleContext(ctx context.Context, cfg *config.Config, p paths.Paths) (string, error) {
	host := strings.TrimSpace(cfg.Cluster.TailscaleOperator)
	if host == "" {
		return "", nil
	}
	if _, err := exec.LookPath("tailscale"); err != nil {
		return "", errors.New("tailscale not found; install tailscale to add the remote context")
	}
	kubeconfig := p.Kubeconfig()
	if _, err := os.Stat(kubeconfig); err != nil {
		return "", fmt.Errorf("kubeconfig %s: %w", kubeconfig, err)
	}
	// Capture the LAN context talosctl set so we can restore it as default;
	// `tailscale configure kubeconfig` switches current-context to the proxy.
	lanContext := kubeconfigCurrentContext(kubeconfig)

	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, "tailscale", "configure", "kubeconfig", host)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tailscale configure kubeconfig %s: %s", host, strings.TrimSpace(string(out)))
	}

	// The context tailscale just added is now current-context.
	tsContext := kubeconfigCurrentContext(kubeconfig)
	if lanContext != "" && lanContext != tsContext {
		if err := setKubeconfigCurrentContext(kubeconfig, lanContext); err != nil {
			return "", fmt.Errorf("restore current-context %q: %w", lanContext, err)
		}
	}
	return tsContext, nil
}

// kubeconfigCurrentContext returns the current-context of a kubeconfig file, or
// "" if it cannot be read.
func kubeconfigCurrentContext(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var doc struct {
		CurrentContext string `yaml:"current-context"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return ""
	}
	return strings.TrimSpace(doc.CurrentContext)
}

// setKubeconfigCurrentContext rewrites only the current-context key, preserving
// the rest of the file structure.
func setKubeconfigCurrentContext(path, name string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		return err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("kubeconfig %s: unexpected structure", path)
	}
	m := root.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "current-context" {
			m.Content[i+1].Value = name
			m.Content[i+1].Tag = "!!str"
			m.Content[i+1].Style = 0
			out, err := yaml.Marshal(&root)
			if err != nil {
				return err
			}
			return os.WriteFile(path, out, 0o600)
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "current-context"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

func ensureTalosconfig(cfg *config.Config, p paths.Paths) error {
	if validTalosconfig(p.Talosconfig()) {
		return nil
	}
	for _, candidate := range talosconfigCandidates(p) {
		if !validTalosconfig(candidate) {
			continue
		}
		body, err := os.ReadFile(candidate)
		if err != nil {
			return fmt.Errorf("read talosconfig %s: %w", candidate, err)
		}
		backends, err := secrets.BuildBackends(cfg)
		if err != nil {
			return err
		}
		rendered, err := secrets.ResolveTemplate(string(body), backends)
		if err != nil {
			return fmt.Errorf("resolve talosconfig %s: %w", candidate, err)
		}
		if err := os.MkdirAll(filepath.Dir(p.Talosconfig()), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p.Talosconfig(), []byte(rendered), 0o600); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("no valid talosconfig found (checked %s)", strings.Join(talosconfigCandidates(p), ", "))
}

func talosconfigCandidates(p paths.Paths) []string {
	candidates := []string{
		filepath.Join(filepath.Dir(p.Root()), "talos", "talosconfig"),
		filepath.Join(p.Root(), "state", "talosconfig"),
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".talos", "config"),
			filepath.Join(home, ".talos", "talosconfig"),
		)
	}
	return candidates
}

func validTalosconfig(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return false
	}
	var tc struct {
		Context  string         `yaml:"context"`
		Contexts map[string]any `yaml:"contexts"`
	}
	if err := yaml.Unmarshal(body, &tc); err != nil {
		return false
	}
	return strings.TrimSpace(tc.Context) != "" && len(tc.Contexts) > 0
}

func requireTalosctl() error {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return errors.New("talosctl not found; install: brew install siderolabs/tap/talosctl")
	}
	return nil
}
