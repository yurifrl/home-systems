// Package registry manages node operations: list, add, remove, render, probe.
package registry

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/secrets"
	"gopkg.in/yaml.v3"
)

// Reachability is the pill-style state shown by `nostos status`.
type Reachability string

const (
	Unknown  Reachability = "unknown"
	Up       Reachability = "up"
	Down     Reachability = "down"
	Refused  Reachability = "refused"
)

// NodeStatus is the per-node live state reported by Probe.
type NodeStatus struct {
	Name    string       `json:"name"`
	IP      string       `json:"ip"`
	Role    string       `json:"role"`
	Ping    Reachability `json:"ping"`
	Apid    Reachability `json:"apid"`
	Version string       `json:"version,omitempty"`
}

// List returns node entries in sorted order (by name).
func List(cfg *config.Config) []struct {
	Name string
	Node config.Node
} {
	names := make([]string, 0, len(cfg.Nodes))
	for n := range cfg.Nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]struct {
		Name string
		Node config.Node
	}, 0, len(names))
	for _, n := range names {
		out = append(out, struct {
			Name string
			Node config.Node
		}{Name: n, Node: cfg.Nodes[n]})
	}
	return out
}

// Get returns the named node or a helpful error listing known names.
func Get(cfg *config.Config, name string) (config.Node, error) {
	if n, ok := cfg.Nodes[name]; ok {
		return n, nil
	}
	known := []string{}
	for k := range cfg.Nodes {
		known = append(known, k)
	}
	sort.Strings(known)
	listStr := "(none)"
	if len(known) > 0 {
		listStr = strings.Join(known, ", ")
	}
	return config.Node{}, fmt.Errorf("no such node %q; known: %s", name, listStr)
}

// Render writes `state/configs/<mac-hyphenated>.yaml` from the node's template
// after resolving secret URIs via the configured backend.
func Render(cfg *config.Config, p paths.Paths, name string, runValidate bool) (string, error) {
	node, err := Get(cfg, name)
	if err != nil {
		return "", err
	}
	tmplPath := p.Templates() + "/" + node.Template
	body, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("template %s not found for node %q: %w", tmplPath, name, err)
	}

	backends, err := secrets.BuildBackends(cfg)
	if err != nil {
		return "", err
	}
	rendered, err := secrets.ResolveTemplate(string(body), backends)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(p.Configs(), 0o755); err != nil {
		return "", err
	}
	out := p.Configs() + "/" + node.MACHyphen() + ".yaml"
	if err := os.WriteFile(out, []byte(rendered), 0o600); err != nil {
		return "", err
	}

	if runValidate {
		if err := talosctlValidate(out); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Add writes a new node entry to config.yaml atomically. Fails if the name already exists.
func Add(cfgPath, name string, node config.Node) error {
	raw := map[string]any{}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	nodes, _ := raw["nodes"].(map[string]any)
	if nodes == nil {
		nodes = map[string]any{}
		raw["nodes"] = nodes
	}
	if _, exists := nodes[name]; exists {
		return fmt.Errorf("node %q already exists in %s", name, cfgPath)
	}
	nodes[name] = map[string]any{
		"mac":          node.MAC,
		"ip":           node.IP,
		"role":         node.Role,
		"arch":         node.Arch,
		"install_disk": node.InstallDisk,
		"template":     node.Template,
	}
	return atomicWriteYAML(cfgPath, raw)
}

// Remove deletes a node entry from config.yaml atomically.
func Remove(cfgPath, name string) error {
	raw := map[string]any{}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	nodes, _ := raw["nodes"].(map[string]any)
	if _, exists := nodes[name]; !exists {
		return fmt.Errorf("no such node %q in %s", name, cfgPath)
	}
	delete(nodes, name)
	return atomicWriteYAML(cfgPath, raw)
}

// Probe checks ping + apid TCP:50000 for a node.
func Probe(node config.Node, timeout time.Duration) NodeStatus {
	s := NodeStatus{IP: node.IP, Role: node.Role}
	s.Ping = pingProbe(node.IP, timeout)
	s.Apid = tcpProbe(node.IP, 50000, timeout)
	if s.Apid == Up {
		s.Version = talosctlVersion(node.IP)
	}
	return s
}

// --- internals ---

func atomicWriteYAML(path string, data any) error {
	tmp := path + ".tmp"
	enc, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, enc, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func talosctlValidate(path string) error {
	if _, err := exec.LookPath("talosctl"); err != nil {
		// Not installed — skip with a warning-style no-op.
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "talosctl", "validate", "--config", path, "--mode", "metal").CombinedOutput()
	if err != nil {
		return fmt.Errorf("talosctl validate rejected %s: %s", path, strings.TrimSpace(string(out)))
	}
	return nil
}

func pingProbe(ip string, timeout time.Duration) Reachability {
	if _, err := exec.LookPath("ping"); err != nil {
		return Unknown
	}
	var waitFlag []string
	if runtime.GOOS == "darwin" {
		waitFlag = []string{"-W", fmt.Sprintf("%d", int(timeout.Milliseconds()))}
	} else {
		waitFlag = []string{"-W", fmt.Sprintf("%d", int(timeout.Seconds()))}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()
	args := append([]string{"-c", "1"}, waitFlag...)
	args = append(args, ip)
	if err := exec.CommandContext(ctx, "ping", args...).Run(); err != nil {
		return Down
	}
	return Up
}

func tcpProbe(ip string, port int, timeout time.Duration) Reachability {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err == nil {
		_ = conn.Close()
		return Up
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return Down
	}
	if opErr, ok := err.(*net.OpError); ok {
		if strings.Contains(strings.ToLower(opErr.Error()), "refused") {
			return Refused
		}
	}
	return Down
}

func talosctlVersion(ip string) string {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "talosctl", "version",
		"--nodes", ip, "--endpoints", ip, "--short").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Server:") || strings.HasPrefix(line, "Tag:") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}
