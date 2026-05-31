package cluster

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/registry"
)

// RefreshAdminCert mints a fresh os:admin talosconfig signed by the cluster's
// Talos OS CA, writing it to p.Talosconfig(). It does NOT touch the cluster:
// the CA (cert + key) is resolved from the configured secrets backend by
// rendering a controlplane machineconfig, then talosctl mints a new client
// cert offline. Use when the admin client cert has expired.
//
// Flow (all local, no cluster mutation):
//  1. render a controlplane node -> machineconfig carrying machine.ca {crt,key}
//  2. talosctl gen secrets --from-controlplane-config -> secrets bundle
//  3. talosctl gen config --with-secrets --output-types talosconfig -> talosconfig
//
// The `hours` argument is accepted for API compatibility; talosctl controls the
// emitted client-cert lifetime (currently ~1 year), so a stale cert is always
// re-mintable by re-running this command.
func RefreshAdminCert(cfg *config.Config, p paths.Paths, node config.Node, hours int) error {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return fmt.Errorf("talosctl not found on PATH")
	}

	// 1. Render a controlplane machineconfig (resolves machine.ca from secrets).
	cpName, err := firstControlplane(cfg)
	if err != nil {
		return err
	}
	rendered, err := registry.Render(cfg, p, cpName, false)
	if err != nil {
		return fmt.Errorf("render controlplane %q: %w", cpName, err)
	}

	// Sanity-check the rendered config actually carries a CA cert+key.
	if _, _, err := ExtractCAFromRenderedConfig(rendered); err != nil {
		return fmt.Errorf("rendered config lacks usable machine.ca: %w", err)
	}

	tmp, err := os.MkdirTemp("", "nostos-cert-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	secretsPath := tmp + "/secrets.yaml"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 2. Build a secrets bundle from the controlplane machineconfig.
	if out, err := exec.CommandContext(ctx, "talosctl", "gen", "secrets",
		"--from-controlplane-config", rendered,
		"-o", secretsPath, "--force").CombinedOutput(); err != nil {
		return fmt.Errorf("talosctl gen secrets: %s", strings.TrimSpace(string(out)))
	}

	// 3. Mint a fresh talosconfig (os:admin client cert) from the bundle.
	if err := os.MkdirAll(p.TalosDir(), 0o700); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "talosctl", "gen", "config",
		cfg.Cluster.Name, cfg.Cluster.Endpoint,
		"--with-secrets", secretsPath,
		"--output-types", "talosconfig",
		"-o", p.Talosconfig(), "--force").CombinedOutput(); err != nil {
		return fmt.Errorf("talosctl gen config: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// firstControlplane returns the lexically-first controlplane node name, or an
// error if none is registered.
func firstControlplane(cfg *config.Config) (string, error) {
	names := []string{}
	for n, node := range cfg.Nodes {
		if node.Role == "controlplane" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no controlplane node in config")
	}
	sort.Strings(names)
	return names[0], nil
}

// ExtractCAFromRenderedConfig reads the base64-decoded machine.ca (cert + key)
// from a rendered machineconfig. Exported for use by a future native cert-gen
// implementation.
func ExtractCAFromRenderedConfig(path string) (certPEM, keyPEM []byte, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Machine struct {
			CA struct {
				Crt string `yaml:"crt"`
				Key string `yaml:"key"`
			} `yaml:"ca"`
		} `yaml:"machine"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	if doc.Machine.CA.Crt == "" || doc.Machine.CA.Key == "" {
		return nil, nil, fmt.Errorf("no machine.ca block in %s", path)
	}
	certPEM, err = base64.StdEncoding.DecodeString(doc.Machine.CA.Crt)
	if err != nil {
		return nil, nil, fmt.Errorf("decode ca.crt: %w", err)
	}
	keyPEM, err = base64.StdEncoding.DecodeString(doc.Machine.CA.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("decode ca.key: %w", err)
	}
	// sanity: certPEM must be PEM
	if b, _ := pem.Decode(certPEM); b == nil {
		return nil, nil, fmt.Errorf("ca.crt is not PEM")
	}
	return certPEM, keyPEM, nil
}
